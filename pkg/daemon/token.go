package daemon

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TokenManager handles generation, storage, and validation of authentication tokens
// for Unix socket connections to the Muster daemon.
//
// Security properties:
//   - 32-byte cryptographically random tokens (256 bits)
//   - Base64 URL encoding (43 characters)
//   - File permissions 0600 (owner read/write only)
//   - Constant-time comparison to prevent timing attacks
//   - Token valid only for daemon lifetime (regenerated on restart)
//
// Usage:
//
//	manager := NewTokenManager("/path/to/token/file")
//	if err := manager.Generate(); err != nil { /* handle */ }
//	defer manager.Cleanup()
type TokenManager struct {
	mu        sync.RWMutex
	token     string
	tokenPath string
}

// NewTokenManager creates a new token manager for the given token file path.
// The tokenPath should be an absolute path where the token will be stored.
func NewTokenManager(tokenPath string) *TokenManager {
	return &TokenManager{
		tokenPath: tokenPath,
	}
}

// Generate creates a new cryptographically secure random token.
// The token is 32 bytes (256 bits) encoded as base64 URL (43 characters).
// Returns an error if random generation fails.
func (tm *TokenManager) Generate() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Generate 32 random bytes
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Errorf("failed to generate random token: %w", err)
	}

	// Encode as base64 URL (no padding)
	tm.token = base64.RawURLEncoding.EncodeToString(bytes)
	return nil
}

// Save writes the token to the token file with 0600 permissions.
// Creates parent directories if they don't exist.
// Returns an error if the token hasn't been generated or if writing fails.
func (tm *TokenManager) Save() error {
	tm.mu.RLock()
	token := tm.token
	tokenPath := tm.tokenPath
	tm.mu.RUnlock()

	if token == "" {
		return fmt.Errorf("no token generated, call Generate() first")
	}

	// Ensure parent directory exists with 0700 permissions (owner only)
	dir := filepath.Dir(tokenPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create token directory: %w", err)
	}

	// Use atomic file creation to prevent race conditions
	// Create temp file in same directory to ensure same filesystem
	tmpFile, err := os.CreateTemp(dir, ".token.*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary token file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Write token content
	if _, err := tmpFile.Write([]byte(token)); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write token content: %w", err)
	}

	// Set secure permissions before closing
	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to set token permissions: %w", err)
	}

	// Close file before rename (required on Windows)
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Atomic rename to final location
	if err := os.Rename(tmpPath, tokenPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to rename token file: %w", err)
	}

	return nil
}

// Load reads the token from the token file.
// Useful for clients that need to read the token to authenticate.
// Returns an error if the file doesn't exist or can't be read.
func (tm *TokenManager) Load() (string, error) {
	tm.mu.RLock()
	tokenPath := tm.tokenPath
	tm.mu.RUnlock()

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("token file not found: %w", err)
		}
		return "", fmt.Errorf("failed to read token file: %w", err)
	}

	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file is empty")
	}

	return token, nil
}

// Validate checks if the provided token matches the current token.
// Uses constant-time comparison to prevent timing attacks.
// Returns true if the token is valid, false otherwise.
func (tm *TokenManager) Validate(providedToken string) bool {
	tm.mu.RLock()
	expectedToken := tm.token
	tm.mu.RUnlock()

	if expectedToken == "" {
		return false
	}

	// Constant-time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare([]byte(providedToken), []byte(expectedToken)) == 1
}

// ValidateFromHeader extracts and validates the token from an Authorization header.
// Expected format: "Bearer <token>"
// Returns true if the header contains a valid token, false otherwise.
func (tm *TokenManager) ValidateFromHeader(authHeader string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return false
	}

	token := strings.TrimSpace(authHeader[len(prefix):])
	return tm.Validate(token)
}

// GetToken returns the current token value.
// Returns empty string if no token has been generated.
func (tm *TokenManager) GetToken() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.token
}

// Cleanup removes the token file.
// Should be called on daemon shutdown.
// Returns nil if the file doesn't exist (idempotent).
func (tm *TokenManager) Cleanup() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Clear in-memory token
	tm.token = ""

	// Remove token file (ignore "not found" error)
	if err := os.Remove(tm.tokenPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove token file: %w", err)
	}

	return nil
}

// TokenPath returns the path where the token is stored.
func (tm *TokenManager) TokenPath() string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tokenPath
}

// IsGenerated returns true if a token has been generated.
func (tm *TokenManager) IsGenerated() bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.token != ""
}

// FileExists checks if the token file exists.
func (tm *TokenManager) FileExists() bool {
	tm.mu.RLock()
	tokenPath := tm.tokenPath
	tm.mu.RUnlock()

	_, err := os.Stat(tokenPath)
	return err == nil
}
