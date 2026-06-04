package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// ClickHouseConfig holds ClickHouse proxy configuration for a single datasource.
type ClickHouseConfig struct {
	Name        string
	RouteName   string
	Description string
	Host        string
	Port        int
	Database    string
	Username    string
	Password    string
	Secure      bool
	SkipVerify  bool
	Timeout     int
}

// ClickHouseHandler handles requests to ClickHouse datasources. Datasources may
// be added or removed at runtime (e.g. by autodiscovery), so all access to the
// datasource map and name list is guarded by mu.
type ClickHouseHandler struct {
	log logrus.FieldLogger

	mu       sync.RWMutex
	clusters map[string]*clickhouseCluster
	names    []string
}

type clickhouseCluster struct {
	cfg   ClickHouseConfig
	proxy *httputil.ReverseProxy
}

// NewClickHouseHandler creates a new ClickHouse handler.
func NewClickHouseHandler(log logrus.FieldLogger, configs []ClickHouseConfig) *ClickHouseHandler {
	h := &ClickHouseHandler{
		log:      log.WithField("handler", "clickhouse"),
		clusters: make(map[string]*clickhouseCluster, len(configs)),
	}

	for _, cfg := range configs {
		h.names = appendUniqueName(h.names, cfg.Name)
		h.clusters[handlerRouteName(cfg.Name, cfg.RouteName)] = h.createCluster(cfg)
	}

	return h
}

func (h *ClickHouseHandler) createCluster(cfg ClickHouseConfig) *clickhouseCluster {
	scheme := "https"
	if !cfg.Secure {
		scheme = "http"
	}

	targetURL := &url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
	}

	rp := &httputil.ReverseProxy{Transport: newProxyTransport(cfg.SkipVerify)}

	rp.Rewrite = func(pr *httputil.ProxyRequest) {
		pr.SetURL(targetURL)
		pr.SetXForwarded()

		// Remove the sandbox's Authorization header (Bearer token) before adding our own.
		pr.Out.Header.Del("Authorization")

		// Add basic auth for ClickHouse.
		if cfg.Username != "" {
			pr.Out.SetBasicAuth(cfg.Username, cfg.Password)
		}

		// Add default database as query param if not already set.
		q := pr.Out.URL.Query()
		if q.Get("database") == "" && cfg.Database != "" {
			q.Set("database", cfg.Database)
		}

		pr.Out.URL.RawQuery = q.Encode()

		// Set the outbound Host to the target host. SetURL only sets URL.Host,
		// but Go's http.Client uses req.Host for the Host header when sending requests.
		// Without this, Cloudflare rejects requests with mismatched Host headers.
		pr.Out.Host = pr.Out.URL.Host

		// Also delete any existing Host header to avoid conflicts.
		pr.Out.Header.Del("Host")
	}

	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		h.log.WithError(err).WithField("datasource", cfg.Name).Error("Proxy error")
		writeError(w, http.StatusBadGateway, "proxy error: %v", err)
	}

	return &clickhouseCluster{
		cfg:   cfg,
		proxy: rp,
	}
}

// ServeHTTP handles ClickHouse requests. The datasource is specified via X-Datasource header.
func (h *ClickHouseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	datasourceName := r.Header.Get(DatasourceHeader)
	if datasourceName == "" {
		writeError(w, http.StatusBadRequest, "missing %s header", DatasourceHeader)

		return
	}

	h.mu.RLock()
	cluster, ok := h.clusters[datasourceRoute(r, datasourceName)]
	h.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "unknown datasource: %s", datasourceName)

		return
	}

	// Strip /clickhouse prefix from path, keep the rest for the upstream.
	path := strings.TrimPrefix(r.URL.Path, "/clickhouse")
	if path == "" {
		path = "/"
	}

	r.URL.Path = path

	if cluster.cfg.Timeout > 0 {
		timeoutCtx, cancel := context.WithTimeout(r.Context(), time.Duration(cluster.cfg.Timeout)*time.Second)
		defer cancel()

		r = r.WithContext(timeoutCtx)
	}

	h.log.WithFields(logrus.Fields{
		"datasource": datasourceName,
		"path":       path,
		"method":     r.Method,
	}).Debug("Proxying ClickHouse request")

	cluster.proxy.ServeHTTP(w, r)
}

// Clusters returns the list of configured ClickHouse datasource names.
func (h *ClickHouseHandler) Clusters() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return append([]string(nil), h.names...)
}

// ClusterConfig returns the current configuration for a datasource name.
func (h *ClickHouseHandler) ClusterConfig(name string) (ClickHouseConfig, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if cluster, ok := h.clusters[name]; ok {
		return cluster.cfg, true
	}

	for _, cluster := range h.clusters {
		if cluster.cfg.Name == name {
			return cluster.cfg, true
		}
	}

	return ClickHouseConfig{}, false
}

// AddCluster adds or replaces a ClickHouse datasource at runtime.
func (h *ClickHouseHandler) AddCluster(cfg ClickHouseConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.names = appendUniqueName(h.names, cfg.Name)
	h.clusters[handlerRouteName(cfg.Name, cfg.RouteName)] = h.createCluster(cfg)
}

// RemoveCluster removes a ClickHouse datasource at runtime.
func (h *ClickHouseHandler) RemoveCluster(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for routeName, cluster := range h.clusters {
		if cluster.cfg.Name == name {
			delete(h.clusters, routeName)
		}
	}

	h.names = removeName(h.names, name)
}

// HasCluster reports whether a datasource is currently configured.
func (h *ClickHouseHandler) HasCluster(name string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if _, ok := h.clusters[name]; ok {
		return true
	}

	for _, cluster := range h.clusters {
		if cluster.cfg.Name == name {
			return true
		}
	}

	return false
}

func appendUniqueName(names []string, name string) []string {
	for _, existing := range names {
		if existing == name {
			return names
		}
	}

	return append(names, name)
}

func removeName(names []string, name string) []string {
	for i, existing := range names {
		if existing == name {
			return append(names[:i], names[i+1:]...)
		}
	}

	return names
}
