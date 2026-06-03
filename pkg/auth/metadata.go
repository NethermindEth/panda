package auth

import (
	"encoding/json"
	"net/http"
)

// handleResourceMetadata returns RFC 9728 protected resource metadata.
func (s *authorizationServer) handleResourceMetadata(w http.ResponseWriter, r *http.Request) {
	baseURL := s.issuerURL

	metadata := map[string]any{
		"resource":                 baseURL,
		"authorization_servers":    []string{baseURL},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         []string{"mcp"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=3600")
	_ = json.NewEncoder(w).Encode(metadata)
}

// handleServerMetadata returns RFC 8414 authorization server metadata.
func (s *authorizationServer) handleServerMetadata(w http.ResponseWriter, r *http.Request) {
	baseURL := s.issuerURL

	metadata := map[string]any{
		"issuer":                                baseURL,
		"authorization_endpoint":                baseURL + "/auth/authorize",
		"token_endpoint":                        baseURL + "/auth/token",
		"device_authorization_endpoint":         baseURL + "/auth/device/code",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      []string{"mcp"},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "max-age=3600")
	_ = json.NewEncoder(w).Encode(metadata)
}
