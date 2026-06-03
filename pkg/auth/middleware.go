package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// AuthUser is the authenticated user info attached to request context.
type AuthUser struct {
	Subject     string
	Username    string
	Groups      []string
	GitHubLogin string
	GitHubID    int64
	Orgs        []string
}

type authUserKeyType string

const authUserKey authUserKeyType = "auth_user"

// GetAuthUser returns the authenticated user from context.
func GetAuthUser(ctx context.Context) *AuthUser {
	user, _ := ctx.Value(authUserKey).(*AuthUser)
	return user
}

// Middleware returns bearer-token validation middleware.
func (s *authorizationServer) Middleware() func(http.Handler) http.Handler {
	if !s.cfg.Enabled {
		return func(next http.Handler) http.Handler { return next }
	}

	publicPaths := map[string]bool{
		"/":                                     true,
		"/health":                               true,
		"/ready":                                true,
		"/.well-known/oauth-protected-resource": true,
		"/.well-known/oauth-authorization-server": true,
	}

	publicPrefixes := []string{"/auth/", "/.well-known/"}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip public paths.
			if publicPaths[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}
			for _, prefix := range publicPrefixes {
				if strings.HasPrefix(r.URL.Path, prefix) {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Get token from Authorization header.
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				baseURL := s.issuerURL
				s.writeUnauthorized(w, baseURL, "missing or invalid Authorization header")
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			baseURL := s.issuerURL

			// Validate token.
			claims := &tokenClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
				if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
					return nil, fmt.Errorf("unexpected signing method")
				}
				return s.secretKey, nil
			}, jwt.WithIssuer(baseURL), jwt.WithExpirationRequired())

			if err != nil || !token.Valid {
				s.writeUnauthorized(w, baseURL, "invalid token")
				return
			}

			// Validate audience (RFC 8707).
			audienceValid := false
			for _, aud := range claims.Audience {
				if aud == baseURL {
					audienceValid = true
					break
				}
			}
			if !audienceValid {
				s.writeUnauthorized(w, baseURL, "token audience mismatch")
				return
			}

			// Attach user info to context.
			ctx := context.WithValue(r.Context(), authUserKey, &AuthUser{
				Subject:     claims.Subject,
				Username:    claims.GitHubLogin,
				Groups:      append([]string(nil), claims.Orgs...),
				GitHubLogin: claims.GitHubLogin,
				GitHubID:    claims.GitHubID,
				Orgs:        claims.Orgs,
			})

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
