package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Note: DatasourceHeader is defined in clickhouse.go

// PrometheusConfig holds Prometheus proxy configuration for a single instance.
type PrometheusConfig struct {
	Name        string
	RouteName   string
	Description string
	URL         string
	Username    string
	Password    string
	SkipVerify  bool
	Timeout     int
}

// PrometheusHandler handles requests to Prometheus instances.
type PrometheusHandler struct {
	log       logrus.FieldLogger
	instances map[string]*prometheusInstance
	names     []string
}

type prometheusInstance struct {
	cfg   PrometheusConfig
	proxy *httputil.ReverseProxy
}

// NewPrometheusHandler creates a new Prometheus handler.
func NewPrometheusHandler(log logrus.FieldLogger, configs []PrometheusConfig) *PrometheusHandler {
	h := &PrometheusHandler{
		log:       log.WithField("handler", "prometheus"),
		instances: make(map[string]*prometheusInstance, len(configs)),
	}

	for _, cfg := range configs {
		h.names = appendUniqueName(h.names, cfg.Name)
		h.instances[handlerRouteName(cfg.Name, cfg.RouteName)] = h.createInstance(cfg)
	}

	return h
}

func (h *PrometheusHandler) createInstance(cfg PrometheusConfig) *prometheusInstance {
	targetURL, err := url.Parse(cfg.URL)
	if err != nil {
		h.log.WithError(err).WithField("instance", cfg.Name).Error("Failed to parse URL")

		return nil
	}

	// Create reverse proxy.
	rp := &httputil.ReverseProxy{Transport: newProxyTransport(cfg.SkipVerify)}

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
		h.log.WithError(err).WithField("instance", cfg.Name).Error("Proxy error")
		http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
	}

	return &prometheusInstance{
		cfg:   cfg,
		proxy: rp,
	}
}

// ServeHTTP handles Prometheus requests. The instance is specified via X-Datasource header.
func (h *PrometheusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Extract instance name from header.
	instanceName := r.Header.Get(DatasourceHeader)
	if instanceName == "" {
		http.Error(w, fmt.Sprintf("missing %s header", DatasourceHeader), http.StatusBadRequest)

		return
	}

	instance, ok := h.instances[datasourceRoute(r, instanceName)]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown instance: %s", instanceName), http.StatusNotFound)

		return
	}

	if instance == nil {
		http.Error(w, fmt.Sprintf("instance %s not properly configured", instanceName), http.StatusInternalServerError)

		return
	}

	// Strip /prometheus prefix from path, keep the rest for the upstream.
	path := strings.TrimPrefix(r.URL.Path, "/prometheus")
	if path == "" {
		path = "/"
	}

	r.URL.Path = path

	if instance.cfg.Timeout > 0 {
		timeoutCtx, cancel := context.WithTimeout(r.Context(), time.Duration(instance.cfg.Timeout)*time.Second)
		defer cancel()

		r = r.WithContext(timeoutCtx)
	}

	h.log.WithFields(logrus.Fields{
		"instance": instanceName,
		"path":     path,
		"method":   r.Method,
	}).Debug("Proxying Prometheus request")

	instance.proxy.ServeHTTP(w, r)
}

// Instances returns the list of configured instance names.
func (h *PrometheusHandler) Instances() []string {
	return append([]string(nil), h.names...)
}
