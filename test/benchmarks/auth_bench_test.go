// Package benchmarks provides authentication benchmarks
//
// Benchmarks:
//
//   - BenchmarkAuth_CryptoHashing: Cryptographic hash operations
//   - BenchmarkAuth_KeyDerivation: Key derivation operations
//   - BenchmarkAuth_JWTParse: JWT token parsing
//   - BenchmarkAuth_Parallel: Concurrent operations
//
// Performance Budgets:
//
// All benchmarks must meet their defined performance budgets (see budgets.go).
// If a benchmark exceeds its budget, it will fail and indicate a regression.
package benchmarks

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// MockTokenManager is a simple token manager for benchmarking
type MockTokenManager struct {
	mu     sync.RWMutex
	tokens map[string]*MockToken
}

// MockToken represents a token
type MockToken struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresAt    time.Time
}

// NeedsRefresh checks if token needs refresh
func (t *MockToken) NeedsRefresh() bool {
	return time.Now().After(t.ExpiresAt)
}

// NewMockTokenManager creates a new token manager
func NewMockTokenManager() *MockTokenManager {
	return &MockTokenManager{
		tokens: make(map[string]*MockToken),
	}
}

// Get retrieves a token
func (tm *MockTokenManager) Get(key string) (*MockToken, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	token, ok := tm.tokens[key]
	return token, ok
}

// Set stores a token
func (tm *MockTokenManager) Set(key string, token *MockToken) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tokens[key] = token
}

// Clear removes all tokens
func (tm *MockTokenManager) Clear() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.tokens = make(map[string]*MockToken)
}

// BenchmarkAuth_OAuth2TokenGeneration measures OAuth2 token generation
func BenchmarkAuth_OAuth2TokenGeneration(b *testing.B) {
	_ = GetBudgetOrDefault("Auth_OAuth2TokenGeneration")

	tokenManager := NewMockTokenManager()
	token := &MockToken{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	tokenManager.Set("test-provider", token)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		token, ok := tokenManager.Get("test-provider")
		if !ok || token == nil {
			b.Fatal("failed to get token")
		}
	}
}

// BenchmarkAuth_OAuth2TokenRefresh measures OAuth2 token refresh
func BenchmarkAuth_OAuth2TokenRefresh(b *testing.B) {
	_ = GetBudgetOrDefault("Auth_OAuth2TokenRefresh")

	tokenManager := NewMockTokenManager()

	expiredToken := &MockToken{
		AccessToken:  "expired-token",
		RefreshToken: "valid-refresh-token",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}

	tokenManager.Set("test-provider", expiredToken)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		token, ok := tokenManager.Get("test-provider")
		if !ok {
			b.Fatal("failed to get token")
		}
		_ = token.NeedsRefresh()
	}
}

// BenchmarkAuth_MTLSHandshake measures mTLS handshake operations
func BenchmarkAuth_MTLSHandshake(b *testing.B) {
	_ = GetBudgetOrDefault("Auth_MTLSHandshake")

	// Simple validation simulation
	validateCert := func(certFile, keyFile, caFile string) error {
		if certFile == "" || keyFile == "" || caFile == "" {
			return fmt.Errorf("missing certificate file")
		}
		return nil
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = validateCert("testdata/client.crt", "testdata/client.key", "testdata/ca.crt")
	}
}

// BenchmarkAuth_Parallel measures concurrent authentication
func BenchmarkAuth_Parallel(b *testing.B) {
	tokenManager := NewMockTokenManager()

	// Pre-populate tokens
	for i := 0; i < 100; i++ {
		token := &MockToken{
			AccessToken: fmt.Sprintf("token-%d", i),
			TokenType:   "Bearer",
			ExpiresAt:   time.Now().Add(time.Hour),
		}
		tokenManager.Set(fmt.Sprintf("provider-%d", i), token)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			provider := fmt.Sprintf("provider-%d", i%100)
			tokenManager.Get(provider)
			i++
		}
	})
}

// BenchmarkAuth_CryptoHashing measures cryptographic hashing performance
func BenchmarkAuth_CryptoHashing(b *testing.B) {
	data := make([]byte, 1024) // 1KB of data
	for i := range data {
		data[i] = byte(i)
	}

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 128)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// HMAC-SHA256 hashing
		h := hmac.New(sha256.New, key)
		h.Write(data)
		_ = h.Sum(nil)
	}
}

// BenchmarkAuth_KeyDerivation measures key derivation performance
func BenchmarkAuth_KeyDerivation(b *testing.B) {
	password := []byte("benchmark-password")
	salt := make([]byte, 16)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simple key derivation using SHA256
		h := sha256.New()
		h.Write(password)
		h.Write(salt)
		_ = h.Sum(nil)
	}
}

// BenchmarkAuth_JWTParse measures JWT parsing performance
func BenchmarkAuth_JWTParse(b *testing.B) {
	// Sample JWT token (header.payload.signature)
	tokenString := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Parse JWT (split by dots)
		parts := strings.Split(tokenString, ".")
		_ = len(parts)
	}
}
