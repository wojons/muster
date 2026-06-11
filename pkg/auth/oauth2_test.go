package auth

import (
	"fmt"
)

// mockRevocationStore implements RevocationChecker for testing
type mockRevocationStore struct {
	revokedTokens map[string]bool
}

func newMockRevocationStore() *mockRevocationStore {
	return &mockRevocationStore{
		revokedTokens: make(map[string]bool),
	}
}

func (m *mockRevocationStore) IsRevoked(token string) (bool, error) {
	return m.revokedTokens[token], nil
}

func (m *mockRevocationStore) revoke(token string) {
	m.revokedTokens[token] = true
}

// errorRevocationStore implements RevocationChecker and always returns an error
type errorRevocationStore struct{}

func (e *errorRevocationStore) IsRevoked(token string) (bool, error) {
	return false, fmt.Errorf("database connection failed")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
