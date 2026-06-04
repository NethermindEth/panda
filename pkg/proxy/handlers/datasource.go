// Package handlers provides reverse proxy handlers for each datasource type.
package handlers

import (
	"context"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// DatasourceHeader is the HTTP header used to specify which datasource to route to.
const DatasourceHeader = "X-Datasource"

type datasourceRouteContextKey struct{}

// WithDatasourceRoute stores the selected backend route for the current request.
func WithDatasourceRoute(ctx context.Context, routeName string) context.Context {
	return context.WithValue(ctx, datasourceRouteContextKey{}, routeName)
}

// datasourceProxy is a single resolved upstream behind a reverse proxy.
// timeout, when positive, bounds each proxied request.
type datasourceProxy struct {
	proxy   *httputil.ReverseProxy
	timeout time.Duration
}

// datasourceHandler is a generic reverse-proxy handler keyed by the X-Datasource
// header. The concrete ClickHouse/Prometheus/Loki handlers build their per-type
// upstreams and delegate request routing here.
type datasourceHandler struct {
	log         logrus.FieldLogger
	kind        string
	pathPrefix  string
	datasources map[string]*datasourceProxy
}

// newDatasourceHandler creates a generic datasource handler. kind labels logs and
// the handler component; pathPrefix is the mount prefix stripped from each request.
func newDatasourceHandler(log logrus.FieldLogger, kind, pathPrefix string) *datasourceHandler {
	return &datasourceHandler{
		log:         log.WithField("handler", kind),
		kind:        kind,
		pathPrefix:  pathPrefix,
		datasources: make(map[string]*datasourceProxy),
	}
}

// ServeHTTP routes a request to the upstream named by the X-Datasource header.
func (h *datasourceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	datasourceName := r.Header.Get(DatasourceHeader)
	if datasourceName == "" {
		writeError(w, http.StatusBadRequest, "missing %s header", DatasourceHeader)

		return
	}

	datasource, ok := h.datasources[datasourceRoute(r, datasourceName)]
	if !ok {
		writeError(w, http.StatusNotFound, "unknown datasource: %s", datasourceName)

		return
	}

	if datasource == nil {
		writeError(w, http.StatusInternalServerError, "datasource %s not properly configured", datasourceName)

		return
	}

	path := strings.TrimPrefix(r.URL.Path, h.pathPrefix)
	if path == "" {
		path = "/"
	}

	r.URL.Path = path

	if datasource.timeout > 0 {
		timeoutCtx, cancel := context.WithTimeout(r.Context(), datasource.timeout)
		defer cancel()

		r = r.WithContext(timeoutCtx)
	}

	h.log.WithFields(logrus.Fields{
		"datasource": datasourceName,
		"path":       path,
		"method":     r.Method,
	}).Debugf("Proxying %s request", h.kind)

	datasource.proxy.ServeHTTP(w, r)
}

// proxyErrorHandler logs upstream proxy failures and returns a 502 to the caller.
func (h *datasourceHandler) proxyErrorHandler(datasourceName string) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, _ *http.Request, err error) {
		h.log.WithError(err).WithField("datasource", datasourceName).Error("Proxy error")
		writeError(w, http.StatusBadGateway, "proxy error: %v", err)
	}
}

func datasourceRoute(r *http.Request, fallback string) string {
	routeName, _ := r.Context().Value(datasourceRouteContextKey{}).(string)
	if routeName == "" {
		return fallback
	}

	return routeName
}

func handlerRouteName(name, routeName string) string {
	if routeName != "" {
		return routeName
	}

	return name
}
