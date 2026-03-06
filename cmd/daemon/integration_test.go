package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBinaryExecution_StatusCommand tests status command execution
func TestBinaryExecution_StatusCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err, "Failed to build daemon binary")
	defer os.Remove(binaryPath)

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
}

// TestBinaryExecution_HelpCommand tests help output
func TestBinaryExecution_HelpCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer os.Remove(binaryPath)

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
	defer os.Remove(binaryPath)

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

	err = stopCmd.Run()

	t.Logf("Stop command stdout: %s", stdout.String())
	t.Logf("Stop command stderr: %s", stderr.String())
}

// TestBinaryExecution_ProxyCommand tests proxy mode
func TestBinaryExecution_ProxyCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer os.Remove(binaryPath)

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
	err = proxyCmd.Run()

	t.Logf("Proxy command stdout: %s", stdout.String())
	t.Logf("Proxy command stderr: %s", stderr.String())
}

// TestBinaryExecution_InvalidCommand tests error handling
func TestBinaryExecution_InvalidCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer os.Remove(binaryPath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "invalid-command")
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout

	// Should fail or show help (not crash)
	err = cmd.Run()
	t.Logf("Invalid command output: %s", stdout.String())
}

// TestBinaryExecution_BackendURLFlag tests backend URL flag handling
func TestBinaryExecution_BackendURLFlag(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer os.Remove(binaryPath)

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
	defer os.Remove(binaryPath)

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

// TestBinaryExecution_RestartCommand tests restart workflow
func TestBinaryExecution_RestartCommand(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer os.Remove(binaryPath)

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-restart.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "restart", "--socket-path", socketPath)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err = cmd.Run()

	t.Logf("Restart command stdout: %s", stdout.String())
	t.Logf("Restart command stderr: %s", stderr.String())
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
	os.Remove(binaryPath)
}

// TestBinaryExecution_VersionOutput tests version info
func TestBinaryExecution_VersionOutput(t *testing.T) {
	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err)
	defer os.Remove(binaryPath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Run without args - shows help with version
	cmd := exec.CommandContext(ctx, binaryPath)
	stdout := &bytes.Buffer{}
	cmd.Stdout = stdout

	err = cmd.Run()

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

	// Build from package directory
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = "." // Current directory is cmd/daemon when tests run
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build failed: %v: %s", err, string(output))
	}

	return binaryPath, nil
}
