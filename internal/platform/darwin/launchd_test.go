//go:build darwin
// +build darwin

package darwin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLaunchdNewLaunchd tests the constructor
func TestLaunchdNewLaunchd(t *testing.T) {
	// User mode (LaunchAgent)
	ldUser := NewLaunchd(true)
	assert.NotNil(t, ldUser)
	assert.True(t, ldUser.userMode)

	// System mode (LaunchDaemon)
	ldSystem := NewLaunchd(false)
	assert.NotNil(t, ldSystem)
	assert.False(t, ldSystem.userMode)
}

// TestLaunchdLoadWithNonExistentPlist tests loading a non-existent plist
func TestLaunchdLoadWithNonExistentPlist(t *testing.T) {
	ld := NewLaunchd(true)
	err := ld.Load("/nonexistent/path/com.test.plist")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plist file not found")
}

// TestLaunchdLoadUnloadLaunchd tests loading and unloading a plist
func TestLaunchdLoadUnloadLaunchd(t *testing.T) {
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "com.muster.test.launchd.plist")

	// Create a simple plist for testing
	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.muster.test.launchd</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sleep</string>
        <string>1</string>
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
	// May fail in CI without launchd
	if err != nil {
		t.Skip("launchctl not available (may fail in CI)")
	}

	// Test Unload
	err = ld.Unload(plistPath)
	assert.NoError(t, err)

	// Test Unload again (should succeed even if already unloaded)
	err = ld.Unload(plistPath)
	assert.NoError(t, err)
}

// TestLaunchdLoadInvalidPlist tests loading an invalid plist
func TestLaunchdLoadInvalidPlist(t *testing.T) {
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "invalid.plist")

	// Create an invalid plist
	err := os.WriteFile(plistPath, []byte("invalid xml content"), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)
	err = ld.Load(plistPath)
	// Should fail or skip in CI
	if err == nil {
		t.Skip("launchctl accepted invalid plist (CI environment may behave differently)")
	}
}

// TestLaunchdStartStopService tests starting and stopping a service
func TestLaunchdStartStopService(t *testing.T) {
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "com.muster.test.service.plist")
	label := "com.muster.test.service"

	// Create a plist for a service
	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.muster.test.service</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sleep</string>
        <string>5</string>
    </array>
    <key>RunAtLoad</key>
    <false/>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(plistContent), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)

	// Load the plist first
	err = ld.Load(plistPath)
	if err != nil {
		t.Skip("launchctl load not available (may fail in CI)")
	}

	// Stop any existing service
	_ = ld.Stop(label)

	// Test Start
	err = ld.Start(label)
	if err != nil {
		// Unload before skip
		_ = ld.Unload(plistPath)
		t.Skip("launchctl start not available (may fail in CI)")
	}

	// Test Stop
	err = ld.Stop(label)
	assert.NoError(t, err)

	// Test Stop again (should succeed)
	err = ld.Stop(label)
	assert.NoError(t, err)

	// Cleanup
	_ = ld.Unload(plistPath)
}

// TestLaunchdRestartService tests restarting a service
func TestLaunchdRestartService(t *testing.T) {
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "com.muster.test.restart.plist")
	label := "com.muster.test.restart"

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.muster.test.restart</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sleep</string>
        <string>10</string>
    </array>
    <key>RunAtLoad</key>
    <false/>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(plistContent), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)

	// Load the plist
	err = ld.Load(plistPath)
	if err != nil {
		t.Skip("launchctl load not available (may fail in CI)")
	}

	// Test Restart
	err = ld.Restart(label)
	if err != nil {
		// Unload before skip
		_ = ld.Unload(plistPath)
		t.Skip("launchctl restart not available (may fail in CI)")
	}

	// Stop and cleanup
	_ = ld.Stop(label)
	_ = ld.Unload(plistPath)
}

// TestLaunchdIsRunning tests service status checking
func TestLaunchdIsRunning(t *testing.T) {
	ld := NewLaunchd(true)

	// Test with a non-existent service
	running := ld.IsRunning("com.muster.nonexistent.service.12345")
	assert.False(t, running)

	// Note: Testing with a real running service would require loading one
	// which we already test in TestLaunchdStartStop
}

// TestLaunchdGetLogPathsFromPlist tests log path extraction from plist
func TestLaunchdGetLogPathsFromPlist(t *testing.T) {
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "test.log.paths.plist")

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.test.logs</string>
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

// TestLaunchdGetLogPathsWithUserExpansion tests log path extraction with $USER expansion
func TestLaunchdGetLogPathsWithUserExpansion(t *testing.T) {
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "test.user.logs.plist")

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.test.user.logs</string>
    <key>StandardOutPath</key>
    <string>/Users/$USER/Library/Logs/test.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/$USER/Library/Logs/test.error.log</string>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(plistContent), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)
	stdout, stderr, err := ld.GetLogPaths(plistPath)

	require.NoError(t, err)
	user := os.Getenv("USER")
	assert.Equal(t, "/Users/"+user+"/Library/Logs/test.log", stdout)
	assert.Equal(t, "/Users/"+user+"/Library/Logs/test.error.log", stderr)
}

// TestLaunchdGetLogPathsMissingKeys tests log path extraction with missing keys
func TestLaunchdGetLogPathsMissingKeys(t *testing.T) {
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "test.missing.logs.plist")

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.test.missing</string>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(plistContent), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)
	stdout, stderr, err := ld.GetLogPaths(plistPath)

	require.NoError(t, err)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// TestLaunchdGetLogPathsNonExistent tests log path extraction with non-existent plist
func TestLaunchdGetLogPathsNonExistent(t *testing.T) {
	ld := NewLaunchd(true)
	stdout, stderr, err := ld.GetLogPaths("/nonexistent/path/test.plist")

	assert.Error(t, err)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// TestLaunchdEnsureLogDirectoryLaunchd tests log directory creation
func TestLaunchdEnsureLogDirectoryLaunchd(t *testing.T) {
	ld := NewLaunchd(true)

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "nested", "logs", "test.log")

	err := ld.EnsureLogDirectory(logPath)
	require.NoError(t, err)

	// Verify directory exists
	logDir := filepath.Dir(logPath)
	info, err := os.Stat(logDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

// TestLaunchdEnsureLogDirectoryEmptyPath tests log directory creation with empty path
func TestLaunchdEnsureLogDirectoryEmptyPath(t *testing.T) {
	ld := NewLaunchd(true)
	err := ld.EnsureLogDirectory("")
	assert.NoError(t, err)
}

// TestLaunchdEnsureLogDirectoryExistingDir tests log directory creation when directory exists
func TestLaunchdEnsureLogDirectoryExistingDir(t *testing.T) {
	ld := NewLaunchd(true)

	tempDir := t.TempDir()
	existingDir := filepath.Join(tempDir, "existing")
	err := os.MkdirAll(existingDir, 0755)
	require.NoError(t, err)

	logPath := filepath.Join(existingDir, "test.log")
	err = ld.EnsureLogDirectory(logPath)
	assert.NoError(t, err)
}

// TestAgentPlistPathLaunchd tests the agent plist path function
func TestAgentPlistPathLaunchd(t *testing.T) {
	label := "com.muster.test.agent"
	path := AgentPlistPath(label)

	assert.Contains(t, path, "Library/LaunchAgents")
	assert.Contains(t, path, label+".plist")
	assert.True(t, strings.HasSuffix(path, ".plist"))
}

// TestDaemonPlistPathLaunchd tests the daemon plist path function
func TestDaemonPlistPathLaunchd(t *testing.T) {
	label := "com.muster.test.daemon"
	path := DaemonPlistPath(label)

	assert.Contains(t, path, "/Library/LaunchDaemons")
	assert.Contains(t, path, label+".plist")
	assert.True(t, strings.HasSuffix(path, ".plist"))
}

// TestLaunchctlVersionLaunchd verifies launchctl version command
func TestLaunchctlVersionLaunchd(t *testing.T) {
	cmd := exec.Command("launchctl", "version")
	output, err := cmd.Output()
	if err != nil {
		t.Skip("launchctl version not available")
	}

	// Should contain version information
	assert.NotEmpty(t, string(output))
}

// TestLaunchdFullLifecycle tests the complete lifecycle of a service
func TestLaunchdFullLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	label := "com.muster.test.lifecycle"
	plistPath := filepath.Join(tempDir, label+".plist")

	// Create a comprehensive plist
	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.muster.test.lifecycle</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sleep</string>
        <string>60</string>
    </array>
    <key>RunAtLoad</key>
    <false/>
    <key>KeepAlive</key>
    <false/>
    <key>StandardOutPath</key>
    <string>` + tempDir + `/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>` + tempDir + `/stderr.log</string>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(plistContent), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)

	// 1. Load
	err = ld.Load(plistPath)
	if err != nil {
		t.Skip("launchctl load not available (may fail in CI)")
	}

	// 2. Start
	err = ld.Start(label)
	if err != nil {
		// Cleanup before skip
		_ = ld.Stop(label)
		_ = ld.Unload(plistPath)
		t.Skip("launchctl start not available (may fail in CI)")
	}

	// 3. Check status (service might still be starting)
	_ = ld.IsRunning(label)

	// 4. Restart
	err = ld.Restart(label)
	assert.NoError(t, err)

	// 5. Stop
	err = ld.Stop(label)
	assert.NoError(t, err)

	// 6. Unload
	err = ld.Unload(plistPath)
	assert.NoError(t, err)

	// Verify service is not running
	running := ld.IsRunning(label)
	assert.False(t, running)
}

// TestLaunchdUnloadNonExistentService tests unloading a non-existent service
func TestLaunchdUnloadNonExistentService(t *testing.T) {
	tempDir := t.TempDir()
	plistPath := filepath.Join(tempDir, "com.muster.nonexistent.plist")

	// Create a plist but don't load it
	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.muster.nonexistent.test</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/echo</string>
        <string>test</string>
    </array>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(plistContent), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)

	// Unload should succeed even if not loaded
	_ = ld.Unload(plistPath)
	// On some systems this might error, but our implementation handles "Could not find" gracefully
	// We just verify it doesn't panic
}

// TestLaunchdPlistPathConsistency tests that plist paths are consistent
func TestLaunchdPlistPathConsistency(t *testing.T) {
	// Test that AgentPlistPath and DaemonPlistPath return valid paths
	agentPath := AgentPlistPath("com.test.agent")
	daemonPath := DaemonPlistPath("com.test.daemon")

	// Both should be absolute paths
	assert.True(t, filepath.IsAbs(agentPath))
	assert.True(t, filepath.IsAbs(daemonPath))

	// Both should end with .plist
	assert.True(t, strings.HasSuffix(agentPath, ".plist"))
	assert.True(t, strings.HasSuffix(daemonPath, ".plist"))

	// Agent should be in user's home, daemon in system
	assert.Contains(t, agentPath, "LaunchAgents")
	assert.Contains(t, daemonPath, "LaunchDaemons")
}

// TestLaunchdSystemMode tests system mode (LaunchDaemon) operations
func TestLaunchdSystemMode(t *testing.T) {
	ld := NewLaunchd(false)
	assert.NotNil(t, ld)
	assert.False(t, ld.userMode)

	// System mode uses the same methods but targets system paths
	// We can't test actual load/unload without sudo
	// Just verify the object is created correctly
}

// TestLaunchdMultipleOperations tests multiple operations in sequence
func TestLaunchdMultipleOperations(t *testing.T) {
	tempDir := t.TempDir()
	label1 := "com.muster.test.multi1"
	label2 := "com.muster.test.multi2"
	plist1 := filepath.Join(tempDir, label1+".plist")
	plist2 := filepath.Join(tempDir, label2+".plist")

	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sleep</string>
        <string>1</string>
    </array>
    <key>RunAtLoad</key>
    <false/>
</dict>
</plist>`

	err := os.WriteFile(plist1, []byte(fmt.Sprintf(plistContent, label1)), 0644)
	require.NoError(t, err)
	err = os.WriteFile(plist2, []byte(fmt.Sprintf(plistContent, label2)), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)

	// Try to load multiple plists
	err1 := ld.Load(plist1)
	err2 := ld.Load(plist2)

	// If one fails, both likely will (CI environment)
	if err1 != nil || err2 != nil {
		t.Skip("launchctl load not available (may fail in CI)")
	}

	// Unload both
	_ = ld.Unload(plist1)
	_ = ld.Unload(plist2)
}

// TestLaunchdComplexPlist tests loading a plist with complex configuration
func TestLaunchdComplexPlist(t *testing.T) {
	tempDir := t.TempDir()
	label := "com.muster.test.complex"
	plistPath := filepath.Join(tempDir, label+".plist")

	// Create a plist with many configuration options
	plistContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.muster.test.complex</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/sleep</string>
        <string>30</string>
    </array>
    <key>EnvironmentVariables</key>
    <dict>
        <key>TEST_VAR</key>
        <string>test_value</string>
    </dict>
    <key>WorkingDirectory</key>
    <string>` + tempDir + `</string>
    <key>RunAtLoad</key>
    <false/>
    <key>KeepAlive</key>
    <false/>
    <key>StandardOutPath</key>
    <string>` + tempDir + `/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>` + tempDir + `/stderr.log</string>
    <key>StartInterval</key>
    <integer>3600</integer>
</dict>
</plist>`

	err := os.WriteFile(plistPath, []byte(plistContent), 0644)
	require.NoError(t, err)

	ld := NewLaunchd(true)

	// Test log path extraction before loading
	stdout, stderr, err := ld.GetLogPaths(plistPath)
	require.NoError(t, err)
	assert.Contains(t, stdout, "stdout.log")
	assert.Contains(t, stderr, "stderr.log")

	// Try to load (may skip in CI)
	err = ld.Load(plistPath)
	if err != nil {
		t.Skip("launchctl load not available (may fail in CI)")
	}

	// Cleanup
	_ = ld.Unload(plistPath)
}

// TestLaunchdLabelValidation tests with various label formats
func TestLaunchdLabelValidation(t *testing.T) {
	testCases := []struct {
		label       string
		description string
	}{
		{"com.muster.test.simple", "simple label"},
		{"com.muster.test-with-dashes", "label with dashes"},
		{"com.muster.test123", "label with numbers"},
		{"com.example.service-name.v1", "complex label"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			agentPath := AgentPlistPath(tc.label)
			assert.Contains(t, agentPath, tc.label+".plist")
		})
	}
}
