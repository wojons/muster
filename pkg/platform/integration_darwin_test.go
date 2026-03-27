//go:build darwin
// +build darwin

// Platform-specific integration tests for macOS

package platform

import (
	"os/exec"
	"testing"
)

// TestMacOSKeychainIntegration tests macOS Keychain-specific features
func TestMacOSKeychainIntegration(t *testing.T) {
	// Verify security command interface works
	security, err := exec.LookPath("security")
	if err != nil {
		t.Skip("security command not available")
	}

	// Test keychain list
	cmd := exec.Command(security, "list-keychains")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to list keychains: %v", err)
	}

	t.Logf("Available keychains: %s", string(output))

	// Test credential add/delete/retrieval operations
	testService := "muster-test-service"
	testAccount := "muster-test-account"
	testPassword := "muster-test-password"

	// Add credential
	cmd = exec.Command(security, "add-generic-password", "-s", testService, "-a", testAccount, "-w", testPassword)
	if err := cmd.Run(); err != nil {
		t.Logf("Failed to add password: %v", err)
		t.Skip("Cannot add to keychain")
	}
	defer func() {
		// Clean up
		cmd := exec.Command(security, "delete-generic-password", "-s", testService, "-a", testAccount)
		_ = cmd.Run()
	}()

	// Find credential
	cmd = exec.Command(security, "find-generic-password", "-s", testService, "-a", testAccount, "-w")
	output, err = cmd.Output()
	if err != nil {
		t.Errorf("Failed to find password: %v", err)
	} else if string(output) != testPassword {
		t.Errorf("Expected %s, got %s", testPassword, string(output))
	}

	t.Log("macOS Keychain integration verified")
}

// TestKeychainBackendMacOS tests macOS Keychain backend
func TestKeychainBackendMacOS(t *testing.T) {
	storage := DefaultSecureStorage()
	if storage == nil {
		t.Fatal("DefaultSecureStorage should return non-nil instance")
	}

	testService := "muster-test-service"
	testAccount := "muster-test-account"
	testValue := "muster-test-value"

	// Test Store/Retrieve/Delete using security command
	err := storage.Store(testService, testAccount, testValue)
	if err != nil {
		t.Logf("SecureStorage.Store error: %v", err)
		t.Skip("macOS Keychain not accessible")
	}
	defer func() {
		_ = storage.Delete(testService, testAccount)
	}()

	value, err := storage.Retrieve(testService, testAccount)
	if err != nil {
		t.Errorf("SecureStorage.Retrieve failed: %v", err)
	} else if value != testValue {
		t.Errorf("Expected %s, got %s", testValue, value)
	}

	err = storage.Delete(testService, testAccount)
	if err != nil {
		t.Errorf("SecureStorage.Delete failed: %v", err)
	}

	t.Log("macOS Keychain backend verified")
}
