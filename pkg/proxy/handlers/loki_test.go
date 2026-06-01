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

// TestLokiHandlerProxiesViaRewrite verifies the Rewrite-based reverse proxy
// forwards to the target host, strips the /loki prefix, preserves the query, and
// rewrites auth + X-Forwarded.
func TestLokiHandlerProxiesViaRewrite(t *testing.T) {
	var got struct {
		host, path, rawQuery, auth, xff string
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.host = r.Host
		got.path = r.URL.Path
		got.rawQuery = r.URL.RawQuery
		got.auth = r.Header.Get("Authorization")
		got.xff = r.Header.Get("X-Forwarded-For")

		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer backend.Close()

	u, err := url.Parse(backend.URL)
	require.NoError(t, err)

	h := NewLokiHandler(logrus.New(), []LokiConfig{{
		Name:     "primary",
		URL:      backend.URL,
		Username: "user",
		Password: "pass",
	}})

	req := httptest.NewRequest(http.MethodGet, "/loki/loki/api/v1/query?query=up", nil)
	req.Header.Set(DatasourceHeader, "primary")
	req.Header.Set("Authorization", "Bearer sandbox-token")
	req.RemoteAddr = "203.0.113.10:6666"

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	res := rec.Result()
	defer func() { _ = res.Body.Close() }()

	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, u.Host, got.host)               // Host rewritten to target
	assert.Equal(t, "/loki/api/v1/query", got.path) // only the /loki mount prefix stripped
	assert.Equal(t, "query=up", got.rawQuery)       // query preserved
	assert.NotEqual(t, "Bearer sandbox-token", got.auth)
	assert.True(t, strings.HasPrefix(got.auth, "Basic "), "expected basic auth, got %q", got.auth)
	assert.Equal(t, "203.0.113.10", got.xff) // SetXForwarded
}
