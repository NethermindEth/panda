package handlers

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
)

// Note: DatasourceHeader is defined in clickhouse.go

// LokiConfig holds Loki proxy configuration for a single datasource.
type LokiConfig struct {
	Name        string
	RouteName   string
	Description string
	URL         string
	Username    string
	Password    string
}

// LokiHandler handles requests to Loki datasources.
type LokiHandler struct {
	log         logrus.FieldLogger
	datasources map[string]*lokiDatasource
}

type lokiDatasource struct {
	cfg   LokiConfig
	proxy *httputil.ReverseProxy
}

// NewLokiHandler creates a new Loki handler.
func NewLokiHandler(log logrus.FieldLogger, configs []LokiConfig) *LokiHandler {
	h := &LokiHandler{
		log:         log.WithField("handler", "loki"),
		datasources: make(map[string]*lokiDatasource, len(configs)),
	}

	for _, cfg := range configs {
		h.datasources[handlerRouteName(cfg.Name, cfg.RouteName)] = h.createDatasource(cfg)
	}

	return h
}

func (h *LokiHandler) createDatasource(cfg LokiConfig) *lokiDatasource {
	targetURL, err := url.Parse(cfg.URL)
	if err != nil {
		h.log.WithError(err).WithField("datasource", cfg.Name).Error("Failed to parse URL")

		return nil
	}

	// Create reverse proxy.
	rp := &httputil.ReverseProxy{Transport: newProxyTransport(false)}

	rp.Rewrite = func(pr *httputil.ProxyRequest) {
		pr.SetURL(targetURL)
		pr.SetXForwarded()

		// Remove the sandbox's Authorization header (Bearer token) before adding our own.
		pr.Out.Header.Del("Authorization")

		// Add basic auth if configured.
		if cfg.Username != "" {
			pr.Out.SetBasicAuth(cfg.Username, cfg.Password)
		}

		// Set the outbound Host to the target host. SetURL only sets URL.Host,
		// but Go's http.Client uses req.Host for the Host header when sending requests.
		pr.Out.Host = pr.Out.URL.Host

		// Also delete any existing Host header to avoid conflicts.
		pr.Out.Header.Del("Host")
	}

	// Error handler.
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		h.log.WithError(err).WithField("datasource", cfg.Name).Error("Proxy error")
		http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
	}

	return &lokiDatasource{
		cfg:   cfg,
		proxy: rp,
	}
}

// ServeHTTP handles Loki requests. The datasource is specified via X-Datasource header.
func (h *LokiHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract datasource name from header.
	datasourceName := r.Header.Get(DatasourceHeader)
	if datasourceName == "" {
		http.Error(w, fmt.Sprintf("missing %s header", DatasourceHeader), http.StatusBadRequest)

		return
	}

	datasource, ok := h.datasources[datasourceRoute(r, datasourceName)]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown datasource: %s", datasourceName), http.StatusNotFound)

		return
	}

	if datasource == nil {
		http.Error(w, fmt.Sprintf("datasource %s not properly configured", datasourceName), http.StatusInternalServerError)

		return
	}

	// Strip /loki prefix from path, keep the rest for the upstream.
	path := strings.TrimPrefix(r.URL.Path, "/loki")
	if path == "" {
		path = "/"
	}

	r.URL.Path = path

	h.log.WithFields(logrus.Fields{
		"datasource": datasourceName,
		"path":       path,
		"method":     r.Method,
	}).Debug("Proxying Loki request")

	datasource.proxy.ServeHTTP(w, r)
}
