//go:build darwin
// +build darwin

package darwin

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeychainGetSetDelete tests basic keychain operations
func TestKeychainGetSetDelete(t *testing.T) {
	kc := NewKeychain()
	service := "test-service-" + t.Name()
	account := "test-account"
	password := "test-password-123"

	// Clean up any existing test entries
	_ = kc.Delete(service, account)

	// Test Set
	err := kc.Set(service, account, password)
	require.NoError(t, err, "Set should succeed")

	// Test Get
	retrieved, err := kc.Get(service, account)
	require.NoError(t, err, "Get should succeed")
	assert.Equal(t, password, retrieved, "Retrieved password should match")

	// Test Update
	newPassword := "new-password-456"
	err = kc.Set(service, account, newPassword)
	require.NoError(t, err, "Update should succeed")

	retrieved, err = kc.Get(service, account)
	require.NoError(t, err, "Get after update should succeed")
	assert.Equal(t, newPassword, retrieved, "Updated password should match")

	// Test Delete
	err = kc.Delete(service, account)
	require.NoError(t, err, "Delete should succeed")

	// Verify deletion
	_, err = kc.Get(service, account)
	assert.Error(t, err, "Get should fail after deletion")

	// Clean up
	_ = kc.Delete(service, account)
}

// TestKeychainEmptyService tests validation of empty service
func TestKeychainEmptyService(t *testing.T) {
	kc := NewKeychain()
	err := kc.Set("", "account", "password")
	assert.Error(t, err, "Set should fail with empty service")
	assert.Contains(t, err.Error(), "service cannot be empty")
}

// TestKeychainEmptyAccount tests validation of empty account
func TestKeychainEmptyAccount(t *testing.T) {
	kc := NewKeychain()
	err := kc.Set("service", "", "password")
	assert.Error(t, err, "Set should fail with empty account")
	assert.Contains(t, err.Error(), "account cannot be empty")
}

// TestKeychainEmptyPassword tests validation of empty password
func TestKeychainEmptyPassword(t *testing.T) {
	kc := NewKeychain()
	err := kc.Set("service", "account", "")
	assert.Error(t, err, "Set should fail with empty password")
	assert.Contains(t, err.Error(), "password cannot be empty")
}

// TestKeychainNotFound tests retrieval of non-existent item
func TestKeychainNotFound(t *testing.T) {
	kc := NewKeychain()
	_, err := kc.Get("nonexistent-service-12345", "nonexistent-account")
	assert.Error(t, err, "Get should fail for non-existent item")
	assert.Contains(t, err.Error(), "keychain item not found")
}

// TestKeychainMasterKey tests master key storage and retrieval
func TestKeychainMasterKey(t *testing.T) {
	kc := NewKeychain()

	// Clean up any existing master key
	_ = kc.Delete("muster-master-key", "muster")

	// Generate a test master key
	testKey := []byte("test-master-key-for-unit-testing-32bytes")

	// Store master key
	err := kc.SetMasterKey(testKey)
	require.NoError(t, err, "SetMasterKey should succeed")

	// Retrieve master key
	retrievedKey, err := kc.GetMasterKey()
	require.NoError(t, err, "GetMasterKey should succeed")
	assert.Equal(t, testKey, retrievedKey, "Retrieved key should match original")

	// Clean up
	_ = kc.Delete("muster-master-key", "muster")
}

// TestLaunchdLoadUnload tests launchd plist loading/unloading
func TestLaunchdLoadUnload(t *testing.T) {
	// Create a temporary plist file
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "com.muster.test.plist")

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.muster.test</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/echo</string>
        <string>test</string>
    </array>
    <key>RunAtLoad</key>
    <false/>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(plistContent), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)

	// Test Load
	err = ld.Load(plistPath)
	// Note: This may fail in CI environments without launchd
	if err != nil {
		t.Skip("launchctl load not available (may fail in CI)")
	}

	// Test Unload
	err = ld.Unload(plistPath)
	assert.NoError(t, err, "Unload should succeed")

	// Verify unload (may fail if already unloaded)
	err = ld.Unload(plistPath)
	assert.NoError(t, err, "Unload should succeed even if already unloaded")
}

// TestLaunchdPlistPaths tests plist path functions
func TestLaunchdPlistPaths(t *testing.T) {
	agentPath := AgentPlistPath("com.muster.test")
	assert.Contains(t, agentPath, "Library/LaunchAgents")
	assert.Contains(t, agentPath, "com.muster.test.plist")

	daemonPath := DaemonPlistPath("com.muster.daemon.test")
	assert.Contains(t, daemonPath, "/Library/LaunchDaemons")
	assert.Contains(t, daemonPath, "com.muster.daemon.test.plist")
}

// TestLaunchdEnsureLogDirectory tests log directory creation
func TestLaunchdEnsureLogDirectory(t *testing.T) {
	ld := NewLaunchd(true)

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "logs", "test.log")

	err := ld.EnsureLogDirectory(logPath)
	require.NoError(t, err)

	// Verify directory was created
	logDir := filepath.Dir(logPath)
	_, err = os.Stat(logDir)
	assert.NoError(t, err, "Log directory should exist")
}

// TestLaunchdGetLogPaths tests log path extraction
func TestLaunchdGetLogPaths(t *testing.T) {
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "test.plist")

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
    <key>StandardOutPath</key>
    <string>/var/log/test/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/test/stderr.log</string>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(plistContent), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)
	stdout, stderr, err := ld.GetLogPaths(plistPath)

	require.NoError(t, err)
	assert.Equal(t, "/var/log/test/stdout.log", stdout)
	assert.Equal(t, "/var/log/test/stderr.log", stderr)
}

// TestKeychainList tests listing accounts for a service
func TestKeychainList(t *testing.T) {
	kc := NewKeychain()
	service := "test-list-service-" + t.Name()

	// Clean up
	accounts, _ := kc.List(service)
	for _, acct := range accounts {
		_ = kc.Delete(service, acct)
	}

	// Add test entries
	err := kc.Set(service, "account1", "password1")
	require.NoError(t, err)

	err = kc.Set(service, "account2", "password2")
	require.NoError(t, err)

	// List accounts
	list, err := kc.List(service)
	require.NoError(t, err)

	// We should find our accounts (may include other test accounts if parallel tests run)
	found := 0
	for _, acct := range list {
		if acct == "account1" || acct == "account2" {
			found++
		}
	}
	assert.GreaterOrEqual(t, found, 2, "Should find both test accounts")

	// Clean up
	_ = kc.Delete(service, "account1")
	_ = kc.Delete(service, "account2")
}

// TestNewKeychain tests constructor
func TestNewKeychain(t *testing.T) {
	kc := NewKeychain()
	assert.NotNil(t, kc)
}

// TestNewLaunchd tests constructor
func TestNewLaunchd(t *testing.T) {
	// User mode
	ldUser := NewLaunchd(true)
	assert.NotNil(t, ldUser)
	assert.True(t, ldUser.userMode)

	// System mode
	ldSystem := NewLaunchd(false)
	assert.NotNil(t, ldSystem)
	assert.False(t, ldSystem.userMode)
}

// TestMax tests helper function
func TestMax(t *testing.T) {
	assert.Equal(t, 5, max(3, 5))
	assert.Equal(t, 10, max(10, 5))
	assert.Equal(t, 0, max(0, 0))
	assert.Equal(t, -1, max(-5, -1))
}

// TestSecurityCommandExists verifies 'security' command is available
func TestSecurityCommandExists(t *testing.T) {
	cmd := exec.Command("security", "-h")
	err := cmd.Run()
	// security command should exist on macOS
	// It may return error on -h flag but command should be found
	assert.NoError(t, err, "security command should exist on macOS")
}

// TestLaunchctlCommandExists verifies 'launchctl' command is available
func TestLaunchctlCommandExists(t *testing.T) {
	cmd := exec.Command("launchctl", "version")
	err := cmd.Run()
	// launchctl should exist on macOS
	assert.NoError(t, err, "launchctl command should exist on macOS")
}
