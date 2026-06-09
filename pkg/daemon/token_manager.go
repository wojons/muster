package daemon

import (
	"strings"
	"sync"
)

// TokenManager manages authentication tokens for the daemon proxy.
type TokenManager struct {
	tokens map[string]bool
	mu     sync.RWMutex
}

// NewTokenManager creates a new token manager.
func NewTokenManager() *TokenManager {
	return &TokenManager{
		tokens: make(map[string]bool),
	}
}

// AddToken registers a valid token.
func (tm *TokenManager) AddToken(token string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tokens[token] = true
}

// RemoveToken revokes a token.
func (tm *TokenManager) RemoveToken(token string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	delete(tm.tokens, token)
}

// Validate checks if a token is valid.
func (tm *TokenManager) Validate(token string) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tokens[token]
}

// ValidateFromHeader extracts and validates a Bearer token from an Authorization header.
func (tm *TokenManager) ValidateFromHeader(authHeader string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return false
	}
	token := strings.TrimPrefix(authHeader, prefix)
	return tm.Validate(token)
}
