package auth

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type RevocationStore interface {
	Revoke(token string, expiresAt time.Time) error
	IsRevoked(token string) (bool, error)
	Cleanup(before time.Time) error
}

type RevocationManager struct {
	store RevocationStore
}

func NewRevocationManager(store RevocationStore) *RevocationManager {
	return &RevocationManager{store: store}
}

func (rm *RevocationManager) RevokeOAuth2Token(token, tokenType, revocationURL string) error {
	data := url.Values{
		"token":           {token},
		"token_type_hint": {tokenType},
	}

	resp, err := http.PostForm(revocationURL, data)
	if err != nil {
		return fmt.Errorf("revocation request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("revocation failed: %d", resp.StatusCode)
	}

	return rm.store.Revoke(token, time.Now().Add(24*time.Hour))
}

func (rm *RevocationManager) RevokeAPIKey(key string) error {
	return rm.store.Revoke(hashKey(key), time.Now().Add(365*24*time.Hour))
}

func hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash)
}

func (rm *RevocationManager) ValidateToken(token string) error {
	revoked, err := rm.store.IsRevoked(token)
	if err != nil {
		return fmt.Errorf("revocation check failed: %w", err)
	}

	if revoked {
		return &ErrTokenRevoked{Token: token}
	}

	return nil
}

type ErrTokenRevoked struct {
	Token string
}

func (e *ErrTokenRevoked) Error() string {
	return "token has been revoked"
}

type MemoryRevocationStore struct {
	revoked map[string]time.Time
	mu      sync.RWMutex
}

func NewMemoryRevocationStore() *MemoryRevocationStore {
	return &MemoryRevocationStore{
		revoked: make(map[string]time.Time),
	}
}

func (s *MemoryRevocationStore) Revoke(token string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.revoked[token] = expiresAt
	return nil
}

func (s *MemoryRevocationStore) IsRevoked(token string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	expiresAt, exists := s.revoked[token]
	if !exists {
		return false, nil
	}

	if time.Now().After(expiresAt) {
		return false, nil
	}

	return true, nil
}

func (s *MemoryRevocationStore) Cleanup(before time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for token, expiresAt := range s.revoked {
		if expiresAt.Before(before) {
			delete(s.revoked, token)
		}
	}

	return nil
}

type RevokedToken struct {
	Token     string
	RevokedAt time.Time
	ExpiresAt time.Time
}

func (rm *RevocationManager) RevokeToken(token string) error {
	return rm.store.Revoke(token, time.Now().Add(24*time.Hour))
}

func (rm *RevocationManager) CleanupExpired(ctx context.Context) error {
	return nil
}
