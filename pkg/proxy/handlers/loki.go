package handlers

import (
	"net/http/httputil"
	"net/url"

	"github.com/sirupsen/logrus"
)

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
// The datasource is specified via the X-Datasource header.
type LokiHandler struct {
	*datasourceHandler
}

// NewLokiHandler creates a new Loki handler.
func NewLokiHandler(log logrus.FieldLogger, configs []LokiConfig) *LokiHandler {
	h := &LokiHandler{
		datasourceHandler: newDatasourceHandler(log, "loki", "/loki"),
	}

	for _, cfg := range configs {
		h.datasources[handlerRouteName(cfg.Name, cfg.RouteName)] = h.createDatasource(cfg)
	}

	return h
}

func (h *LokiHandler) createDatasource(cfg LokiConfig) *datasourceProxy {
	targetURL, err := url.Parse(cfg.URL)
	if err != nil {
		h.log.WithError(err).WithField("datasource", cfg.Name).Error("Failed to parse URL")

		return nil
	}

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

	rp.ErrorHandler = h.proxyErrorHandler(cfg.Name)

	return &datasourceProxy{proxy: rp}
}
