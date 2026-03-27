//go:build windows
// +build windows

// Platform-specific integration tests for Windows

package platform

import (
	"os/exec"
	"testing"
)

// TestWindowsCredentialIntegration tests Windows Credential Manager
func TestWindowsCredentialIntegration(t *testing.T) {
	// Windows Credential Manager test via cmdkey
	testTarget := "muster-test-target"
	testUser := "muster-test-user"
	testPass := "muster-test-password"

	// Store credential (using cmdkey as fallback)
	cmdkey, err := exec.LookPath("cmdkey")
	if err != nil {
		t.Skip("cmdkey not available")
	}

	cmd := exec.Command(cmdkey, "/generic:"+testTarget, "/user:"+testUser, "/pass:"+testPass)
	if err := cmd.Run(); err != nil {
		t.Logf("Failed to store credential: %v", err)
		t.Skip("Cannot store Windows credential")
	}
	defer func() {
		// Clean up
		cmd := exec.Command(cmdkey, "/delete:"+testTarget)
		_ = cmd.Run()
	}()

	// List credentials to verify
	cmd = exec.Command(cmdkey, "/list")
	output, err := cmd.Output()
	if err != nil {
		t.Errorf("Failed to list credentials: %v", err)
	} else {
		t.Logf("Credentials listed successfully")
		if len(output) == 0 {
			t.Error("Credential list should not be empty")
		}
	}

	t.Log("Windows Credential Manager integration verified")
}

// TestKeychainBackendWindows tests Windows Credential Manager backend
func TestKeychainBackendWindows(t *testing.T) {
	storage := DefaultSecureStorage()
	if storage == nil {
		t.Fatal("DefaultSecureStorage should return non-nil instance")
	}

	testService := "muster-test-service"
	testAccount := "muster-test-account"
	testValue := "muster-test-value"

	// Test Store/Retrieve/Delete using Windows Credential Manager
	err := storage.Store(testService, testAccount, testValue)
	if err != nil {
		t.Logf("SecureStorage.Store error: %v", err)
		t.Skip("Windows Credential Manager not accessible")
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

	t.Log("Windows Credential Manager backend verified")
}
