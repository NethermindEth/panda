package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPrometheusHandlerProxiesViaRewrite verifies the Rewrite-based reverse proxy
// (which replaced the deprecated Director) forwards requests to the upstream
// correctly: the target Host is set, X-Forwarded-* headers are populated, the
// inbound Authorization is stripped and replaced with configured basic auth, and
// the /prometheus path prefix is stripped while the query is preserved.
func TestPrometheusHandlerProxiesViaRewrite(t *testing.T) {
	var got struct {
		host, path, rawQuery, auth, xff, xfProto, xfHost string
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.host = r.Host
		got.path = r.URL.Path
		got.rawQuery = r.URL.RawQuery
		got.auth = r.Header.Get("Authorization")
		got.xff = r.Header.Get("X-Forwarded-For")
		got.xfProto = r.Header.Get("X-Forwarded-Proto")
		got.xfHost = r.Header.Get("X-Forwarded-Host")

		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "backend-ok")
	}))
	defer backend.Close()

	backendURL, err := url.Parse(backend.URL)
	require.NoError(t, err)

	h := NewPrometheusHandler(logrus.New(), []PrometheusConfig{{
		Name:     "primary",
		URL:      backend.URL,
		Username: "user",
		Password: "pass",
	}})

	req := httptest.NewRequest(http.MethodGet, "/prometheus/api/v1/query?q=up", nil)
	req.Header.Set(DatasourceHeader, "primary")
	req.Header.Set("Authorization", "Bearer sandbox-token") // should be stripped/replaced
	req.RemoteAddr = "203.0.113.7:12345"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	res := rec.Result()
	defer func() { _ = res.Body.Close() }()

	body, _ := io.ReadAll(res.Body)

	require.Equal(t, http.StatusOK, res.StatusCode, "proxy should return the backend status")
	assert.Equal(t, "backend-ok", string(body))

	// /prometheus prefix stripped, rest of the path and the query preserved.
	assert.Equal(t, "/api/v1/query", got.path)
	assert.Equal(t, "q=up", got.rawQuery)

	// Host rewritten to the upstream target (pr.Out.Host = pr.Out.URL.Host).
	assert.Equal(t, backendURL.Host, got.host)

	// Inbound bearer token stripped; configured basic auth applied.
	assert.NotEqual(t, "Bearer sandbox-token", got.auth)
	assert.True(t, strings.HasPrefix(got.auth, "Basic "), "expected basic auth, got %q", got.auth)

	// SetXForwarded populated the forwarding headers.
	assert.Equal(t, "203.0.113.7", got.xff)
	assert.NotEmpty(t, got.xfProto)
	assert.NotEmpty(t, got.xfHost)
}
