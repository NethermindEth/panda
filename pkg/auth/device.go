package auth

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// handleDeviceCode handles POST /auth/device/code (RFC 8628 device authorization request).
func (s *authorizationServer) handleDeviceCode(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "invalid form data")
		return
	}

	clientID := r.FormValue("client_id")
	resource := r.FormValue("resource")

	if clientID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "client_id is required")
		return
	}

	if resource == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "resource is required")
		return
	}

	if !s.isExpectedResource(resource) {
		s.writeError(w, http.StatusBadRequest, "invalid_target", "unsupported resource")
		return
	}

	deviceCode, err := s.generateRandomToken(32)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "server_error", "failed to generate device code")
		return
	}

	userCode, err := s.generateUniqueUserCode()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "server_error", "failed to generate user code")
		return
	}

	now := time.Now()
	dev := &deviceAuth{
		DeviceCode: deviceCode,
		UserCode:   userCode,
		ClientID:   clientID,
		Resource:   resource,
		CreatedAt:  now,
		ExpiresAt:  now.Add(deviceCodeTTL),
	}

	s.devicesMu.Lock()
	s.devices[deviceCode] = dev
	s.userCodes[normalizeUserCode(userCode)] = deviceCode
	s.devicesMu.Unlock()

	baseURL := s.issuerURL

	s.log.WithFields(logrus.Fields{
		"client_id": clientID,
		"user_code": userCode,
	}).Info("Device authorization started")

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"device_code":      deviceCode,
		"user_code":        userCode,
		"verification_uri": baseURL + "/auth/device",
		"expires_in":       int(deviceCodeTTL.Seconds()),
		"interval":         devicePollInterval,
	})
}

// handleDevicePage handles GET /auth/device (browser page to enter user code).
func (s *authorizationServer) handleDevicePage(w http.ResponseWriter, r *http.Request) {
	userCode := r.URL.Query().Get("code")

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(buildDevicePage(userCode, "")))
}

// handleDeviceVerify handles POST /auth/device/verify (user code form submission).
func (s *authorizationServer) handleDeviceVerify(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(buildDevicePage("", "Invalid request.")))

		return
	}

	userCode := r.FormValue("user_code")
	normalized := normalizeUserCode(userCode)

	if normalized == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(buildDevicePage("", "Please enter a code.")))

		return
	}

	// Look up device auth by user code and copy needed fields while holding the lock.
	s.devicesMu.RLock()
	deviceCode, ok := s.userCodes[normalized]

	var (
		valid       bool
		devClient   string
		devResource string
	)

	if ok {
		if dev := s.devices[deviceCode]; dev != nil && !dev.Authorized && time.Now().Before(dev.ExpiresAt) {
			valid = true
			devClient = dev.ClientID
			devResource = dev.Resource
		}
	}

	s.devicesMu.RUnlock()

	if !valid {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(buildDevicePage(userCode, "Invalid or expired code. Check the code in your terminal and try again.")))

		return
	}

	// Create a pending auth that links to this device code.
	githubState, err := s.generateState()
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(buildDevicePage(userCode, "Something went wrong. Please try again.")))

		return
	}

	s.pendingMu.Lock()
	s.pending[githubState] = &pendingAuth{
		ClientID:   devClient,
		Resource:   devResource,
		CreatedAt:  time.Now(),
		DeviceCode: deviceCode,
	}
	s.pendingMu.Unlock()

	// Redirect to GitHub for authentication.
	baseURL := s.issuerURL
	callbackURL := baseURL + "/auth/callback"
	githubURL := s.github.GetAuthorizationURL(callbackURL, githubState, "read:user read:org")

	s.log.WithFields(logrus.Fields{
		"user_code":   userCode,
		"device_code": deviceCode,
	}).Info("Device verification started, redirecting to GitHub")

	http.Redirect(w, r, githubURL, http.StatusFound)
}

// handleDeviceTokenGrant handles the device_code grant type in POST /auth/token.
func (s *authorizationServer) handleDeviceTokenGrant(w http.ResponseWriter, r *http.Request) {
	deviceCode := r.FormValue("device_code")
	clientID := r.FormValue("client_id")

	if deviceCode == "" || clientID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request", "missing required parameters")
		return
	}

	s.devicesMu.Lock()
	dev, ok := s.devices[deviceCode]

	if !ok {
		s.devicesMu.Unlock()
		s.writeError(w, http.StatusBadRequest, "invalid_grant", "invalid device code")

		return
	}

	if time.Now().After(dev.ExpiresAt) {
		delete(s.userCodes, normalizeUserCode(dev.UserCode))
		delete(s.devices, deviceCode)
		s.devicesMu.Unlock()
		s.writeError(w, http.StatusBadRequest, "expired_token", "device code has expired")

		return
	}

	if dev.ClientID != clientID {
		s.devicesMu.Unlock()
		s.writeError(w, http.StatusBadRequest, "invalid_grant", "client_id mismatch")

		return
	}

	if !dev.Authorized {
		now := time.Now()

		// RFC 8628 section 3.5: clients polling faster than the advertised
		// interval are told to back off via slow_down.
		tooFast := !dev.LastPolledAt.IsZero() &&
			now.Sub(dev.LastPolledAt) < devicePollInterval*time.Second
		dev.LastPolledAt = now

		if tooFast {
			s.devicesMu.Unlock()
			s.writeError(w, http.StatusBadRequest, "slow_down", "polling too frequently")

			return
		}

		s.devicesMu.Unlock()
		s.writeError(w, http.StatusBadRequest, "authorization_pending", "waiting for user to authorize")

		return
	}

	// Consume the device auth — tokens are issued once.
	login := dev.GitHubLogin
	ghID := dev.GitHubID
	githubToken := dev.GitHubToken
	orgs := dev.Orgs
	resource := dev.Resource

	delete(s.userCodes, normalizeUserCode(dev.UserCode))
	delete(s.devices, deviceCode)
	s.devicesMu.Unlock()

	baseURL := s.issuerURL

	accessToken, err := s.issueAccessToken(baseURL, resource, login, ghID, orgs)
	if err != nil {
		s.log.WithError(err).Error("Failed to sign device token")
		s.writeError(w, http.StatusInternalServerError, "server_error", "failed to create token")

		return
	}

	refreshToken, err := s.issueRefreshToken(clientID, resource, login, ghID, githubToken, orgs)
	if err != nil {
		s.log.WithError(err).Error("Failed to create device refresh session")
		s.writeError(w, http.StatusInternalServerError, "server_error", "failed to create refresh token")

		return
	}

	s.log.WithFields(logrus.Fields{
		"login":     login,
		"client_id": clientID,
	}).Info("Device token issued")

	s.writeTokenResponse(w, accessToken, refreshToken)
}

// generateUserCode creates a random user code in XXXX-XXXX format using consonants only.
func (s *authorizationServer) generateUserCode() (string, error) {
	b := make([]byte, userCodeHalfLen*2)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}

	code := make([]byte, userCodeHalfLen*2)
	for i := range code {
		code[i] = userCodeAlphabet[int(b[i])%len(userCodeAlphabet)]
	}

	return string(code[:userCodeHalfLen]) + "-" + string(code[userCodeHalfLen:]), nil
}

// generateUniqueUserCode generates a user code that doesn't collide with existing codes.
func (s *authorizationServer) generateUniqueUserCode() (string, error) {
	const maxRetries = 5

	for range maxRetries {
		code, err := s.generateUserCode()
		if err != nil {
			return "", err
		}

		s.devicesMu.RLock()
		_, exists := s.userCodes[normalizeUserCode(code)]
		s.devicesMu.RUnlock()

		if !exists {
			return code, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique user code after %d attempts", maxRetries)
}

// normalizeUserCode strips hyphens/spaces and uppercases a user code for comparison.
func normalizeUserCode(code string) string {
	code = strings.ToUpper(code)
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ReplaceAll(code, " ", "")

	return code
}
