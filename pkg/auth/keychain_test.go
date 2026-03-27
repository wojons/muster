package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewKeychain(t *testing.T) {
	kc, err := NewKeychain()
	if err != nil {
		t.Fatalf("NewKeychain() error = %v", err)
	}
	if kc == nil {
		t.Fatal("NewKeychain() returned nil keychain")
	}
}

func TestMacOSKeychain_Set(t *testing.T) {
	kc := NewMockMacOSKeychain()
	err := kc.Set("test-service", "test-account", "test-password")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
}

func TestMacOSKeychain_Get(t *testing.T) {
	kc := NewMockMacOSKeychain()
	_ = kc.Set("test-service-get", "test-account-get", "test-password-get")

	password, err := kc.Get("test-service-get", "test-account-get")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if password != "test-password-get" {
		t.Errorf("Get() = %v, want %v", password, "test-password-get")
	}
}

func TestMacOSKeychain_GetNotFound(t *testing.T) {
	kc := NewMockMacOSKeychain()
	_, err := kc.Get("nonexistent-service", "nonexistent-account")
	if err != ErrNotFound {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestMacOSKeychain_Delete(t *testing.T) {
	kc := NewMockMacOSKeychain()
	_ = kc.Set("test-service-del", "test-account-del", "test-password-del")

	err := kc.Delete("test-service-del", "test-account-del")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = kc.Get("test-service-del", "test-account-del")
	if err != ErrNotFound {
		t.Errorf("Get() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestMacOSKeychain_List(t *testing.T) {
	kc := NewMockMacOSKeychain()
	_ = kc.Set("test-service-list", "test-account-list", "test-password-list")

	accounts, err := kc.List("test-service-list")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(accounts) == 0 {
		t.Error("List() returned empty accounts")
	}
}

func TestWindowsKeychain_Set(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}
	testPath := filepath.Join(homeDir, ".muster-test-credentials")
	defer func() {
		_ = os.RemoveAll(testPath)
	}()

	fc := &FileKeychain{
		basePath: testPath,
	}
	wkc := &WindowsKeychain{filechain: fc}

	err = wkc.Set("test-service-win", "test-account-win", "test-password-win")
	if err != nil {
		t.Fatalf("WindowsKeychain Set() error = %v", err)
	}

	password, err := wkc.Get("test-service-win", "test-account-win")
	if err != nil {
		t.Fatalf("WindowsKeychain Get() error = %v", err)
	}
	if password != "test-password-win" {
		t.Errorf("WindowsKeychain Get() = %v, want %v", password, "test-password-win")
	}

	err = wkc.Delete("test-service-win", "test-account-win")
	if err != nil {
		t.Fatalf("WindowsKeychain Delete() error = %v", err)
	}

	_, err = wkc.Get("test-service-win", "test-account-win")
	if err != ErrNotFound {
		t.Errorf("WindowsKeychain Get() after Delete() error = %v, want ErrNotFound", err)
	}
}

func TestSecretServiceKeychain_Fallback(t *testing.T) {
	kc, err := NewSecretServiceKeychain()
	if err == nil {
		t.Error("Expected NewSecretServiceKeychain to return error on this platform")
	}
	if err != ErrSecretServiceNotAvailable {
		t.Logf("Note: Got error %v (expected ErrSecretServiceNotAvailable on non-Linux)", err)
	}
	if kc != nil {
		t.Error("Expected nil SecretServiceKeychain on error")
	}
}

// SEC-002: Tests for weak master key fallback removal
func TestFileKeychain_NoWeakOSFallback(t *testing.T) {
	// This test verifies that the weak OS-based fallback has been removed.
	// The old code used: masterKey = []byte("muster-fallback-master-key-" + runtime.GOOS)
	// which was a security vulnerability because it was predictable.
	// The fix (SEC-002) removes this and fails closed instead.
	//
	// NOTE: getMasterKeyFromFile() auto-generates a secure random key if one doesn't exist,
	// so the only way to trigger the error is if file operations fail (e.g., permission denied).
	// This test verifies the error handling path exists and returns the correct error.

	// Create a file path that's not a directory to cause MkdirAll to fail
	tempFile, err := os.CreateTemp("", "not-a-dir-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}()

	// Use the file path as the basePath - this will cause MkdirAll to fail
	// because it can't create a directory where a file exists
	k := &FileKeychain{basePath: tempFile.Name()}

	// Try to derive key - this should fail because MkdirAll will fail
	_, err = k.deriveKey("test-service", "test-account")
	if err == nil {
		t.Fatal("deriveKey should return error when master key cannot be obtained")
	}

	// Should be ErrMissingMasterKey, indicating we fail closed instead of using weak key
	if !errors.Is(err, ErrMissingMasterKey) {
		t.Errorf("Expected ErrMissingMasterKey (fails closed), got: %v", err)
		t.Log("This verifies that the weak 'muster-fallback-master-key-<OS>' fallback has been removed")
	}
}

func TestFileKeychain_SecureMasterKeyGeneration(t *testing.T) {
	// This test verifies that the secure file-based master key generation works.
	// Unlike the weak fallback, this generates a random 32-byte key.

	tempDir := t.TempDir()
	k := &FileKeychain{basePath: tempDir}

	// First operation should generate a secure random master key
	err := k.Set("test-service", "test-account", "test-password")
	if err != nil {
		t.Fatalf("Failed to set password (should generate master key): %v", err)
	}

	// Verify we can retrieve it
	password, err := k.Get("test-service", "test-account")
	if err != nil {
		t.Fatalf("Failed to get password: %v", err)
	}

	if password != "test-password" {
		t.Errorf("Expected 'test-password', got '%s'", password)
	}
}

func TestFileKeychain_MasterKeyFilePermissions(t *testing.T) {
	// Verify that generated master key file has secure permissions

	tempDir := t.TempDir()
	k := &FileKeychain{basePath: tempDir}

	// First operation should generate the master key
	err := k.Set("test-service", "test-account", "test-password")
	if err != nil {
		t.Fatalf("Failed to set password: %v", err)
	}

	// Verify master key file was created
	masterKeyFile := filepath.Join(tempDir, ".master-key")
	if _, err := os.Stat(masterKeyFile); os.IsNotExist(err) {
		t.Fatal("Master key file should have been created")
	}

	// Verify master key file has secure permissions
	info, err := os.Stat(masterKeyFile)
	if err != nil {
		t.Fatalf("Failed to stat master key file: %v", err)
	}

	// Permissions should be 0600 (owner read/write only)
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("Master key file permissions should be 0600, got %04o", mode)
	}
}

func TestErrMissingMasterKey_Message(t *testing.T) {
	// Verify the error message contains helpful guidance
	errMsg := ErrMissingMasterKey.Error()

	if errMsg == "" {
		t.Error("ErrMissingMasterKey should have a descriptive message")
	}

	// Should contain guidance on how to fix the issue
	if !strings.Contains(errMsg, "muster auth init") {
		t.Error("Error message should contain 'muster auth init' guidance")
	}

	// Should indicate it's a master key issue
	if !strings.Contains(errMsg, "master key") {
		t.Error("Error message should mention 'master key'")
	}
}

func TestFileKeychain_DeriveKey_DeterministicWithSameMasterKey(t *testing.T) {
	// Verify that the same service/account always derives the same key
	// when using the same master key

	tempDir := t.TempDir()
	k := &FileKeychain{basePath: tempDir}

	// Create master key by setting a password
	err := k.Set("test-service", "test-account", "test-password")
	if err != nil {
		t.Fatalf("Failed to set password: %v", err)
	}

	// Derive key twice for same service/account
	key1, err := k.deriveKey("test-service", "test-account")
	if err != nil {
		t.Fatalf("Failed to derive key first time: %v", err)
	}

	key2, err := k.deriveKey("test-service", "test-account")
	if err != nil {
		t.Fatalf("Failed to derive key second time: %v", err)
	}

	// Keys should be identical
	if string(key1) != string(key2) {
		t.Error("Derived keys should be deterministic for same service/account")
	}

	// Different account should derive different key
	key3, err := k.deriveKey("test-service", "different-account")
	if err != nil {
		t.Fatalf("Failed to derive key for different account: %v", err)
	}

	if string(key1) == string(key3) {
		t.Error("Different accounts should derive different keys")
	}
}

// TestSanitizeKeychainParam tests the input validation for service/account parameters
func TestSanitizeKeychainParam(t *testing.T) {
	tests := []struct {
		name        string
		param       string
		paramName   string
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid simple string",
			param:     "my-service",
			paramName: "service",
			wantErr:   false,
		},
		{
			name:      "valid string with numbers",
			param:     "service-123",
			paramName: "service",
			wantErr:   false,
		},
		{
			name:        "empty string rejected",
			param:       "",
			paramName:   "service",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "dollar sign rejected",
			param:       "$(whoami)",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
		{
			name:        "backtick rejected",
			param:       "`whoami`",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
		{
			name:        "pipe rejected",
			param:       "service|cat /etc/passwd",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
		{
			name:        "ampersand rejected",
			param:       "service&malicious",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
		{
			name:        "semicolon rejected",
			param:       "service;rm -rf /",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
		{
			name:        "parentheses rejected",
			param:       "service(malicious)",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
		{
			name:        "newline rejected",
			param:       "service\ntest",
			paramName:   "service",
			wantErr:     true,
			errContains: "cannot contain whitespace",
		},
		{
			name:        "carriage return rejected",
			param:       "service\rtest",
			paramName:   "service",
			wantErr:     true,
			errContains: "cannot contain whitespace",
		},
		{
			name:        "tab rejected",
			param:       "service\ttest",
			paramName:   "service",
			wantErr:     true,
			errContains: "cannot contain whitespace",
		},
		{
			name:        "path traversal rejected",
			param:       "../../../etc/passwd",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains suspicious pattern",
		},
		{
			name:        "double dollar paren rejected",
			param:       "$((cat /etc/passwd))",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
		{
			name:        "quote rejected",
			param:       "service\"test",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
		{
			name:        "single quote rejected",
			param:       "service'test",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
		{
			name:        "asterisk rejected",
			param:       "service*",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
		{
			name:        "hash rejected",
			param:       "service#comment",
			paramName:   "service",
			wantErr:     true,
			errContains: "contains invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sanitizeKeychainParam(tt.param, tt.paramName)

			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeKeychainParam() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && tt.errContains != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, got nil", tt.errContains)
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Error %q does not contain %q", err.Error(), tt.errContains)
				}
			}
		})
	}
}

// TestMacOSKeychain_CommandInjectionPrevention tests that command injection attacks are prevented
func TestMacOSKeychain_CommandInjectionPrevention(t *testing.T) {
	kc := &MacOSKeychain{}

	injectionAttempts := []struct {
		service string
		account string
		desc    string
	}{
		{
			service: "foo -s x -a x -w $(whoami > /tmp/pwned) #",
			account: "test",
			desc:    "shell command injection in service",
		},
		{
			service: "test",
			account: "$(cat /etc/passwd)",
			desc:    "shell command substitution in account",
		},
		{
			service: "test; rm -rf /",
			account: "test",
			desc:    "command chaining in service",
		},
		{
			service: "test",
			account: "../../../etc/passwd",
			desc:    "path traversal in account",
		},
		{
			service: "test|cat /etc/passwd",
			account: "test",
			desc:    "pipe injection in service",
		},
		{
			service: "test",
			account: "test&malicious",
			desc:    "background command in account",
		},
	}

	for _, attempt := range injectionAttempts {
		t.Run(attempt.desc, func(t *testing.T) {
			// Set should reject injection attempt
			err := kc.Set(attempt.service, attempt.account, "password")
			if err == nil {
				t.Errorf("Set() should reject injection attempt: %s", attempt.desc)
			}

			// Get should reject injection attempt
			_, err = kc.Get(attempt.service, attempt.account)
			if err == nil {
				t.Errorf("Get() should reject injection attempt: %s", attempt.desc)
			}

			// Delete should reject injection attempt
			err = kc.Delete(attempt.service, attempt.account)
			if err == nil {
				t.Errorf("Delete() should reject injection attempt: %s", attempt.desc)
			}
		})
	}
}

// TestFuzzKeychainServiceNameInjection is a fuzz test for service name injection
func FuzzKeychainServiceNameInjection(f *testing.F) {
	// Seed corpus with known injection patterns
	injectionPatterns := []string{
		"$(whoami)",
		"`whoami`",
		"test;id",
		"test|cat /etc/passwd",
		"test&whoami",
		"test\nmalicious",
		"../../../etc/passwd",
		"$((cat /etc/passwd))",
		"test\"malicious",
		"test'malicious",
		"test*malicious",
		"test#comment",
	}

	for _, pattern := range injectionPatterns {
		f.Add(pattern, "test-account")
	}

	f.Fuzz(func(t *testing.T, service, account string) {
		kc := &MacOSKeychain{}

		// Set should either succeed with valid input or fail with validation error
		err := kc.Set(service, account, "test-password")

		// If error occurs, verify it's a validation error (not command execution)
		if err != nil {
			// Errors should contain validation messages
			errMsg := err.Error()
			validationIndicators := []string{
				"cannot be empty",
				"contains invalid character",
				"cannot contain whitespace",
				"contains suspicious pattern",
			}

			// At least one validation indicator should be present for malicious inputs
			hasValidationMsg := false
			for _, vi := range validationIndicators {
				if strings.Contains(errMsg, vi) {
					hasValidationMsg = true
					break
				}
			}

			// For obviously malicious inputs, require validation error
			isObviouslyMalicious := strings.Contains(service, "$(") ||
				strings.Contains(service, "`") ||
				strings.Contains(service, "|") ||
				strings.Contains(service, ";") ||
				strings.Contains(service, "../")

			if isObviouslyMalicious && !hasValidationMsg {
				t.Errorf("Set(%q, %q) should reject with validation error, got: %v", service, account, err)
			}
		}
	})
}
