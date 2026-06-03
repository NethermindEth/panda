package handlers

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allowAfter is a GlobalTriggerRateLimiter that denies once its budget is spent.
type allowAfter struct {
	remaining int
}

func (a *allowAfter) Allow(_ string) bool {
	if a.remaining <= 0 {
		return false
	}

	a.remaining--

	return true
}

func newTestGitHubHandler(t *testing.T, limiter GlobalTriggerRateLimiter) *GitHubHandler {
	t.Helper()

	log := logrus.New()
	log.SetOutput(io.Discard)

	return NewGitHubHandler(log, GitHubConfig{Token: "test-token"}, limiter)
}

func triggerRequest(t *testing.T, body string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/github/actions/trigger", strings.NewReader(body))

	return httptest.NewRecorder(), req
}

func TestGitHubHandlerRejectsDisallowedRepository(t *testing.T) {
	t.Parallel()

	h := newTestGitHubHandler(t, &allowAfter{remaining: 10})
	rec, req := triggerRequest(t, `{"repository":"someone/else","workflow":"build.yml"}`)

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestGitHubHandlerEnforcesWorkflowCooldown(t *testing.T) {
	t.Parallel()

	h := newTestGitHubHandler(t, &allowAfter{remaining: 10})
	h.recordTrigger("build.yml")

	rec, req := triggerRequest(t, `{"repository":"`+defaultAllowedRepository+`","workflow":"build.yml"}`)
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), "cooldown")
}

func TestGitHubHandlerEnforcesGlobalTriggerBudget(t *testing.T) {
	t.Parallel()

	h := newTestGitHubHandler(t, &allowAfter{remaining: 0})

	rec, req := triggerRequest(t, `{"repository":"`+defaultAllowedRepository+`","workflow":"build.yml"}`)
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), "global trigger limit")
}

func TestGitHubHandlerNilGlobalLimiterSkipsBudget(t *testing.T) {
	t.Parallel()

	// With a nil limiter the global budget is disabled; a fresh workflow passes
	// the rate-limit gates and proceeds to dispatch (which fails against the
	// unreachable GitHub API, surfacing as a bad gateway rather than a 429).
	h := newTestGitHubHandler(t, nil)
	h.httpClient = &http.Client{Transport: errRoundTripper{}}

	rec, req := triggerRequest(t, `{"repository":"`+defaultAllowedRepository+`","workflow":"build.yml"}`)
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

type errRoundTripper struct{}

func (errRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, errors.New("dispatch unreachable")
}
