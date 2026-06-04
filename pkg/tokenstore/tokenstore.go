// Package tokenstore provides an in-memory registry of ephemeral runtime
// tokens. Each registered value is keyed by an opaque token that expires after
// a fixed TTL, with a background loop reaping expired entries.
package tokenstore

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

// Store is a TTL-bounded registry mapping opaque tokens to their backing values.
type Store struct {
	mu      sync.RWMutex
	tokens  map[string]entry
	ttl     time.Duration
	stopCh  chan struct{}
	stopped bool
}

type entry struct {
	value     string
	expiresAt time.Time
}

// New creates a Store whose tokens expire after ttl and starts its cleanup loop.
func New(ttl time.Duration) *Store {
	store := &Store{
		tokens: make(map[string]entry, 64),
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}

	go store.cleanupLoop()

	return store
}

// Register stores value under a freshly generated token and returns the token.
func (s *Store) Register(value string) string {
	token := generateToken()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokens[token] = entry{
		value:     value,
		expiresAt: time.Now().Add(s.ttl),
	}

	return token
}

// Validate returns the value registered for token, or an empty string if the
// token is unknown or has expired.
func (s *Store) Validate(token string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, ok := s.tokens[token]
	if !ok {
		return ""
	}

	if time.Now().After(entry.expiresAt) {
		return ""
	}

	return entry.value
}

// Revoke removes the first token whose registered value equals value.
func (s *Store) Revoke(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for token, entry := range s.tokens {
		if entry.value == value {
			delete(s.tokens, token)
			return
		}
	}
}

// Stop terminates the background cleanup loop. It is safe to call more than once.
func (s *Store) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}

	s.stopped = true
	s.mu.Unlock()

	close(s.stopCh)
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

func (s *Store) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for token, entry := range s.tokens {
		if now.After(entry.expiresAt) {
			delete(s.tokens, token)
		}
	}
}

func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random token: " + err.Error())
	}

	return base64.URLEncoding.EncodeToString(b)
}
