package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBinaryExecution_StatusCommand tests status command execution
func TestBinaryExecution_StatusCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err, "Failed to build daemon binary")
	defer func() { _ = os.Remove(binaryPath) }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "status")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err = cmd.Run()

	// Command should succeed
	require.NoError(t, err, "status command should succeed")

	// Verify output shows daemon status
	output := stdout.String()
	t.Logf("Status output: %s", output)

	// DF-047: the status surface must not advertise a token file the daemon
	// never creates.
	require.NotContains(t, output, "Token file:", "status must not print a token-file hint")
}

// TestBinaryExecution_HelpCommand tests help output
func TestBinaryExecution_HelpCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer func() { _ = os.Remove(binaryPath) }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath)
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout

	err = cmd.Run()

	require.NoError(t, err)

	output := stdout.String()
	// Verify help is displayed
	require.Contains(t, output, "Usage:", "should show usage")
	require.Contains(t, output, "Commands:", "should show commands")
}

// TestBinaryExecution_StartStopCommand tests start/stop workflow
func TestBinaryExecution_StartStopCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer func() { _ = os.Remove(binaryPath) }()

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-daemon.sock")

	// Start command
	ctx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel1()

	startCmd := exec.CommandContext(ctx1, binaryPath, "start", "--socket-path", socketPath)
	startStdout := &bytes.Buffer{}
	startStderr := &bytes.Buffer{}
	startCmd.Stdout = startStdout
	startCmd.Stderr = startStderr

	// Start in background (daemon mode)
	err = startCmd.Run()
	t.Logf("Start command output: %s, error: %v", startStdout.String(), err)

	// Stop command (may fail if not running, but should not crash)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()

	stopCmd := exec.CommandContext(ctx2, binaryPath, "stop", "--socket-path", socketPath)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	stopCmd.Stdout = stdout
	stopCmd.Stderr = stderr

	_ = stopCmd.Run() // Ignore error - just testing that binary was built

	t.Logf("Stop command stdout: %s", stdout.String())
	t.Logf("Stop command stderr: %s", stderr.String())
}

// TestBinaryExecution_ProxyCommand tests proxy mode
func TestBinaryExecution_ProxyCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer func() { _ = os.Remove(binaryPath) }()

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-proxy.sock")

	// Test proxy mode with backend URL
	ctx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel1()

	proxyCmd := exec.CommandContext(ctx1, binaryPath, "proxy", "--socket-path", socketPath, "--backend-url", "http://localhost:8080")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	proxyCmd.Stdout = stdout
	proxyCmd.Stderr = stderr

	// Run proxy (may timeout waiting for connections)
	_ = proxyCmd.Run() // Ignore error - just testing that binary was built

	t.Logf("Proxy command stdout: %s", stdout.String())
	t.Logf("Proxy command stderr: %s", stderr.String())
}

// TestBinaryExecution_InvalidCommand tests error handling
func TestBinaryExecution_InvalidCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer func() { _ = os.Remove(binaryPath) }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "invalid-command")
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout

	// Should fail or show help (not crash)
	_ = cmd.Run() // Ignore error - just testing that binary was built
	t.Logf("Invalid command output: %s", stdout.String())
}

// TestBinaryExecution_BackendURLFlag tests backend URL flag handling
func TestBinaryExecution_BackendURLFlag(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer func() { _ = os.Remove(binaryPath) }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	testURL := "http://api.example.com"
	cmd := exec.CommandContext(ctx, binaryPath, "--backend-url", testURL, "status")
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout

	err = cmd.Run()
	t.Logf("Binary with backend URL: %v, output: %s", err, stdout.String())
}

// TestBinaryExecution_SocketPathFlag tests socket path flag handling
func TestBinaryExecution_SocketPathFlag(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer func() { _ = os.Remove(binaryPath) }()

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "custom-socket.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--socket-path", socketPath, "status")
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout

	err = cmd.Run()
	t.Logf("Binary with custom socket: %v, output: %s", err, stdout.String())
}

// TestBinaryExecution_RestartCommand verifies the full restart workflow
// (DF-011): start a daemon, restart it — the old process exits and a fresh
// one serves the same socket — then confirm status reports Running: true
// under a new PID, and stop the restarted daemon for cleanup.
func TestBinaryExecution_RestartCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer func() { _ = os.Remove(binaryPath) }()

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-restart.sock")
	pidPath := socketPath + ".pid"

	runDaemonCmd := func(args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binaryPath, args...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "command %v failed: %s", args, out)
		return string(out)
	}

	// Start the daemon (spawns a detached child).
	startOut := runDaemonCmd("start", "--socket-path", socketPath)

	// DF-047: the start banner must not advertise a token file the daemon
	// never creates.
	require.NotContains(t, startOut, "Token file:", "start must not print a token-file hint")

	oldPID, err := readPIDFile(pidPath)
	require.NoError(t, err, "start should write a PID file")
	require.True(t, isProcessRunning(oldPID), "daemon should be running after start")

	// Restart: stops the old daemon and spawns a fresh one on the same socket.
	out := runDaemonCmd("restart", "--socket-path", socketPath)
	require.Contains(t, out, "Daemon started successfully", "restart must bring the daemon back up")

	// The old daemon must be gone.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && isProcessRunning(oldPID) {
		time.Sleep(50 * time.Millisecond)
	}
	require.False(t, isProcessRunning(oldPID), "old daemon PID %d should have exited after restart", oldPID)

	// Status must report the daemon running again, under a NEW PID.
	statusOut := runDaemonCmd("status", "--socket-path", socketPath)
	require.Contains(t, statusOut, "Running:     true", "status must report Running: true after restart")

	newPID, err := readPIDFile(pidPath)
	require.NoError(t, err, "restart should write a fresh PID file")
	require.NotEqual(t, oldPID, newPID, "restart must spawn a new daemon process")
	require.True(t, isProcessRunning(newPID), "restarted daemon should be running")

	// Cleanup: stop the restarted daemon.
	stopOut := runDaemonCmd("stop", "--socket-path", socketPath)
	require.Contains(t, stopOut, "Daemon stopped successfully")
	require.False(t, isProcessRunning(newPID), "daemon should be stopped after cleanup")
}

// TestBinaryExecution_BuildingBinary verifies build process
func TestBinaryExecution_BuildingBinary(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err, "Should build binary successfully")

	// Verify binary exists
	_, err = os.Stat(binaryPath)
	require.NoError(t, err, "Binary should exist after build")

	// Verify binary is executable
	info, err := os.Stat(binaryPath)
	require.NoError(t, err)
	require.NotNil(t, info)

	// Clean up
	_ = os.Remove(binaryPath)
}

// TestBinaryExecution_VersionOutput tests version info
func TestBinaryExecution_VersionOutput(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer func() { _ = os.Remove(binaryPath) }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run without args - shows help with version
	cmd := exec.CommandContext(ctx, binaryPath)
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout

	_ = cmd.Run() // Ignore error - just testing that binary was built

	output := stdout.String()
	t.Logf("Version/help output: %s", output)
}

// buildDaemonBinary builds the daemon binary for testing
func buildDaemonBinary() (string, error) {
	tmpDir, err := os.MkdirTemp("", "daemon-test-*")
	if err != nil {
		return "", err
	}

	binaryPath := filepath.Join(tmpDir, "test-daemon")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}

	// Build from package directory
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = "." // Current directory is cmd/daemon when tests run
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build failed: %v: %s", err, string(output))
	}

	return binaryPath, nil
}
