// Package auth provides simplified GitHub-based OAuth for local product edges.
//
// This implements a minimal OAuth 2.1 authorization server that:
// - Delegates identity verification to GitHub
// - Issues signed bearer tokens with proper resource (audience) binding per RFC 8707
// - Validates bearer tokens on protected endpoints
//
// Two client flows are supported:
// 1. PKCE authorization code flow (local browser callback)
// 2. Device authorization grant (RFC 8628, for SSH/headless environments)
package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/panda/pkg/auth/github"
)

const (
	// Authorization code TTL.
	authCodeTTL = 5 * time.Minute

	// Default access token TTL.
	defaultAccessTokenTTL = 1 * time.Hour

	// Default refresh token TTL.
	defaultRefreshTokenTTL = 30 * 24 * time.Hour

	// deviceCodeTTL is how long a user has to complete the device flow.
	deviceCodeTTL = 15 * time.Minute

	// devicePollInterval is the minimum polling interval in seconds per RFC 8628.
	devicePollInterval = 5

	// userCodeAlphabet is uppercase consonants only (no vowels to avoid offensive words).
	userCodeAlphabet = "BCDFGHJKLMNPQRSTVWXZ"

	// userCodeHalfLen is the number of characters per half of the XXXX-XXXX user code.
	userCodeHalfLen = 4
)

type githubClient interface {
	GetAuthorizationURL(redirectURI, state, scope string) string
	ExchangeCode(ctx context.Context, code, redirectURI string) (*github.TokenResponse, error)
	GetUser(ctx context.Context, accessToken string) (*github.GitHubUser, error)
}

// AuthorizationServer is the GitHub-backed OAuth authorization server.
type AuthorizationServer interface {
	Start(ctx context.Context) error
	Stop() error
	Enabled() bool
	Middleware() func(http.Handler) http.Handler
	MountRoutes(r chi.Router)
}

// authorizationServer implements AuthorizationServer.
type authorizationServer struct {
	log             logrus.FieldLogger
	cfg             Config
	github          githubClient
	secretKey       []byte
	allowedOrgs     []string
	issuerURL       string
	accessTokenTTL  time.Duration
	refreshTokenTTL time.Duration

	// Pending authorization requests (state -> pendingAuth).
	pending   map[string]*pendingAuth
	pendingMu sync.RWMutex

	// Authorization codes (code -> issuedCode).
	codes   map[string]*issuedCode
	codesMu sync.RWMutex

	// Device authorizations (RFC 8628).
	devices   map[string]*deviceAuth // device_code -> deviceAuth
	userCodes map[string]string      // normalized user_code -> device_code
	devicesMu sync.RWMutex

	// Refresh sessions (opaque refresh token -> refreshSession).
	refreshSessions   map[string]*refreshSession
	refreshSessionsMu sync.RWMutex

	// Lifecycle.
	stopCh chan struct{}
}

// pendingAuth stores state during the OAuth flow.
type pendingAuth struct {
	ClientID      string
	RedirectURI   string
	CodeChallenge string
	Resource      string
	State         string
	CreatedAt     time.Time
	DeviceCode    string // non-empty for device flow callbacks
}

// issuedCode is an issued authorization code.
type issuedCode struct {
	Code          string
	ClientID      string
	RedirectURI   string
	Resource      string
	CodeChallenge string
	GitHubLogin   string
	GitHubID      int64
	GitHubToken   string
	Orgs          []string
	CreatedAt     time.Time
	Used          bool
}

// deviceAuth stores a pending device authorization request (RFC 8628).
type deviceAuth struct {
	DeviceCode   string
	UserCode     string
	ClientID     string
	Resource     string
	CreatedAt    time.Time
	ExpiresAt    time.Time
	LastPolledAt time.Time
	Authorized   bool
	GitHubLogin  string
	GitHubID     int64
	GitHubToken  string
	Orgs         []string
}

type refreshSession struct {
	ClientID          string
	Resource          string
	GitHubLogin       string
	GitHubID          int64
	GitHubAccessToken string
	Orgs              []string
	CreatedAt         time.Time
	ExpiresAt         time.Time
}

// tokenClaims are the JWT claims for access tokens.
type tokenClaims struct {
	jwt.RegisteredClaims
	GitHubLogin string   `json:"github_login"`
	GitHubID    int64    `json:"github_id"`
	Orgs        []string `json:"orgs,omitempty"`
}

// NewAuthorizationServer creates a new GitHub-backed OAuth authorization server.
// A fixed issuer URL is required so token metadata and validation do not trust
// inbound Host or X-Forwarded-* headers.
func NewAuthorizationServer(log logrus.FieldLogger, cfg Config) (AuthorizationServer, error) {
	log = log.WithField("component", "auth")

	if !cfg.Enabled {
		log.Info("Authentication is disabled")
		return &authorizationServer{log: log, cfg: cfg}, nil
	}

	cfg.IssuerURL = strings.TrimRight(strings.TrimSpace(cfg.IssuerURL), "/")
	if cfg.IssuerURL == "" {
		return nil, fmt.Errorf("issuer_url is required when auth is enabled")
	}

	if cfg.AccessTokenTTL <= 0 {
		cfg.AccessTokenTTL = defaultAccessTokenTTL
	}

	if cfg.RefreshTokenTTL <= 0 {
		cfg.RefreshTokenTTL = defaultRefreshTokenTTL
	}

	if cfg.GitHub == nil {
		return nil, fmt.Errorf("github configuration is required when auth is enabled")
	}

	if cfg.Tokens.SecretKey == "" {
		return nil, fmt.Errorf("tokens.secret_key is required when auth is enabled")
	}

	s := &authorizationServer{
		log:             log,
		cfg:             cfg,
		github:          github.NewClient(log, cfg.GitHub.ClientID, cfg.GitHub.ClientSecret),
		secretKey:       []byte(cfg.Tokens.SecretKey),
		allowedOrgs:     cfg.AllowedOrgs,
		issuerURL:       cfg.IssuerURL,
		accessTokenTTL:  cfg.AccessTokenTTL,
		refreshTokenTTL: cfg.RefreshTokenTTL,
		pending:         make(map[string]*pendingAuth, 32),
		codes:           make(map[string]*issuedCode, 32),
		devices:         make(map[string]*deviceAuth, 16),
		userCodes:       make(map[string]string, 16),
		refreshSessions: make(map[string]*refreshSession, 32),
		stopCh:          make(chan struct{}),
	}

	log.WithFields(logrus.Fields{
		"allowed_orgs": cfg.AllowedOrgs,
	}).Info("Auth service created")

	return s, nil
}

func (s *authorizationServer) Start(ctx context.Context) error {
	if !s.cfg.Enabled {
		return nil
	}

	// Start cleanup goroutine.
	go s.cleanupLoop()

	s.log.Info("Auth service started")
	return nil
}

func (s *authorizationServer) Stop() error {
	if !s.cfg.Enabled {
		return nil
	}

	close(s.stopCh)
	s.log.Info("Auth service stopped")
	return nil
}

func (s *authorizationServer) Enabled() bool {
	return s.cfg.Enabled
}

// MountRoutes mounts auth routes.
func (s *authorizationServer) MountRoutes(r chi.Router) {
	if !s.cfg.Enabled {
		return
	}

	// Discovery endpoints.
	r.Get("/.well-known/oauth-protected-resource", s.handleResourceMetadata)
	r.Get("/.well-known/oauth-authorization-server", s.handleServerMetadata)

	// OAuth endpoints.
	r.Get("/auth/authorize", s.handleAuthorize)
	r.Get("/auth/callback", s.handleCallback)
	r.Post("/auth/token", s.handleToken)

	// Device authorization endpoints (RFC 8628).
	r.Post("/auth/device/code", s.handleDeviceCode)
	r.Get("/auth/device", s.handleDevicePage)
	r.Post("/auth/device/verify", s.handleDeviceVerify)
}

// cleanupLoop periodically removes expired pending auths and codes.
func (s *authorizationServer) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanup()
		case <-s.stopCh:
			return
		}
	}
}

func (s *authorizationServer) cleanup() {
	now := time.Now()

	s.pendingMu.Lock()
	for key, p := range s.pending {
		if now.Sub(p.CreatedAt) > authCodeTTL {
			delete(s.pending, key)
		}
	}
	s.pendingMu.Unlock()

	s.codesMu.Lock()
	for key, c := range s.codes {
		if now.Sub(c.CreatedAt) > authCodeTTL || c.Used {
			delete(s.codes, key)
		}
	}
	s.codesMu.Unlock()

	s.devicesMu.Lock()
	for key, d := range s.devices {
		if now.After(d.ExpiresAt) {
			delete(s.userCodes, normalizeUserCode(d.UserCode))
			delete(s.devices, key)
		}
	}
	s.devicesMu.Unlock()

	s.refreshSessionsMu.Lock()
	for key, session := range s.refreshSessions {
		if now.After(session.ExpiresAt) {
			delete(s.refreshSessions, key)
		}
	}
	s.refreshSessionsMu.Unlock()
}
