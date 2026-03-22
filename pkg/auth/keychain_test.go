package auth

import (
	"errors"
	"os"
	"os/exec"
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
	if _, err := execLookPath("security"); err != nil {
		t.Skip("security command not available, skipping test")
	}
	kc := &MacOSKeychain{}
	err := kc.Set("test-service", "test-account", "test-password")
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
}

func TestMacOSKeychain_Get(t *testing.T) {
	if _, err := execLookPath("security"); err != nil {
		t.Skip("security command not available, skipping test")
	}
	kc := &MacOSKeychain{}
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
	if _, err := execLookPath("security"); err != nil {
		t.Skip("security command not available, skipping test")
	}
	kc := &MacOSKeychain{}
	_, err := kc.Get("nonexistent-service", "nonexistent-account")
	if err != ErrNotFound {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func execLookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func TestMacOSKeychain_Delete(t *testing.T) {
	if _, err := execLookPath("security"); err != nil {
		t.Skip("security command not available, skipping test")
	}
	kc := &MacOSKeychain{}
	kc.Set("test-service-del", "test-account-del", "test-password-del")

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
	if _, err := execLookPath("security"); err != nil {
		t.Skip("security command not available, skipping test")
	}
	kc := &MacOSKeychain{}
	kc.Set("test-service-list", "test-account-list", "test-password-list")

	accounts, err := kc.List("test-service-list")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(accounts) == 0 {
		t.Error("List() returned empty accounts")
	}
}

func TestWindowsKeychain_Set(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	testPath := filepath.Join(homeDir, ".muster-test-credentials")
	defer os.RemoveAll(testPath)

	fc := &FileKeychain{
		basePath: testPath,
	}
	wkc := &WindowsKeychain{filechain: fc}

	err := wkc.Set("test-service-win", "test-account-win", "test-password-win")
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
		t.Error("Expected NewSecretServiceKeychain to return error")
	}
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
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
	defer os.Remove(tempFile.Name())
	tempFile.Close()

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
