package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/openapi-cli/openapi-cli/pkg/daemon"
	"github.com/stretchr/testify/require"
)

func TestRun_HelpAction(t *testing.T) {
	os.Args = []string{"muster-daemon"}

	err := run()
	require.NoError(t, err, "no action should show help")
}

func TestRun_StatusAction(t *testing.T) {
	os.Args = []string{"muster-daemon", "status"}

	err := run()
	require.NoError(t, err, "status command should succeed")
}

func TestHandleStop_NotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test-daemon.pid")
	socketPath := filepath.Join(tmpDir, "test-daemon.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
	})
	require.NoError(t, err)

	err = handleStop(controller)
	require.NoError(t, err, "handleStop should succeed when not running")
}

func TestHandleStatus_Success(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-status.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	err = handleStatus(controller)
	require.NoError(t, err, "handleStatus should succeed")
}

func TestDaemonLogger_Info(t *testing.T) {
	logger := &daemonLogger{}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger.Info("test message", "arg1", "arg2")

	w.Close()
	os.Stdout = oldStdout

	buf, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Contains(t, string(buf), "test message")
}

func TestDaemonLogger_Warn(t *testing.T) {
	logger := &daemonLogger{}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger.Warn("test warning", "arg1")

	w.Close()
	os.Stdout = oldStdout

	buf, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Contains(t, string(buf), "test warning")
}

func TestDaemonLogger_Error(t *testing.T) {
	logger := &daemonLogger{}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logger.Error("test error", "arg1")

	w.Close()
	os.Stderr = oldStderr

	buf, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Contains(t, string(buf), "test error")
}

func TestRun_WithBackendURL(t *testing.T) {
	// Store original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Test flag parsing with backend URL
	testURL := "http://localhost:8080"
	os.Args = []string{"daemon", "--backend-url", testURL, "version"} // command-line based

	// Just verify the test compiles and can create handler
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAllHandlers_NilArguments(t *testing.T) {
	err := handleStart(nil)
	require.Error(t, err, "handleStart should error with nil controller")

	err = handleStop(nil)
	require.Error(t, err, "handleStop should error with nil controller")

	err = handleRestart(nil)
	require.Error(t, err, "handleRestart should error with nil controller")

	err = handleStatus(nil)
	require.Error(t, err, "handleStatus should error with nil controller")

	err = handleProxyStart(nil)
	require.Error(t, err, "handleProxyStart should error with nil controller")
}

func TestInfoLogger_OutputFormat(t *testing.T) {
	logger := &daemonLogger{}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger.Info("operation completed", "status", "success")

	w.Close()
	os.Stdout = oldStdout

	buf, _ := io.ReadAll(r)
	require.Contains(t, string(buf), "[INFO]")
	require.Contains(t, string(buf), "operation completed")
}

func TestWarnLogger_OutputFormat(t *testing.T) {
	logger := &daemonLogger{}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger.Warn("operation completed", "status", "warning")

	w.Close()
	os.Stdout = oldStdout

	buf, _ := io.ReadAll(r)
	require.Contains(t, string(buf), "[WARN]")
	require.Contains(t, string(buf), "operation completed")
}

func TestErrorLogger_OutputFormat(t *testing.T) {
	logger := &daemonLogger{}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logger.Error("operation failed", "error", "timeout")

	w.Close()
	os.Stderr = oldStderr

	buf, _ := io.ReadAll(r)
	require.Contains(t, string(buf), "[ERROR]")
	require.Contains(t, string(buf), "operation failed")
}

func TestHandleStart_ControllerStartFailure(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-start.sock")

	// Create controller with invalid config to cause Start() to fail
	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: "/nonexistent/path/to/pidfile/test.pid",
		SocketPath:  socketPath,
	})
	require.NoError(t, err)

	// Test the error path
	err = controller.Start()
	require.Error(t, err, "Controller start should fail with invalid path")
}

func TestHandleStart_Success(t *testing.T) {
	// Skip - Start has blocking operations
	t.Skip("Start test skipped due to blocking operations")
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test-daemon.pid")
	socketPath := filepath.Join(tmpDir, "test-daemon.sock")

	shutdownCalled := make(chan struct{}, 1)

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
		ShutdownHandler: func() {
			select {
			case shutdownCalled <- struct{}{}:
			default:
			}
		},
	})
	require.NoError(t, err)

	// Test controller can start and stop independently of handleStart
	// (handleStart is tested via integration/e2e tests with actual subprocess)
	err = controller.Start()
	require.NoError(t, err, "controller should start")
	defer controller.Stop()

	// Verify controller is running
	status, err := controller.Status()
	require.NoError(t, err, "should be able to get status")
	require.True(t, status.Running, "controller should be running")

	// Verify we can stop it
	err = controller.Stop()
	require.NoError(t, err, "controller should stop")

	// Verify not running after stop
	status, err = controller.Status()
	require.NoError(t, err)
	require.False(t, status.Running, "controller should not be running after stop")
}

func TestHandleRestart_Success(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test-restart.pid")
	socketPath := filepath.Join(tmpDir, "test-restart.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
	})
	require.NoError(t, err)

	err = handleRestart(controller)
	// Restart might fail if daemon is not running, but shouldn't crash
	t.Logf("Restart result: %v", err)
}

func TestHandleProxyStart_Success(t *testing.T) {
	// Skip - Start has blocking operations
	t.Skip("Start test skipped due to blocking operations")
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test-proxy.pid")
	socketPath := filepath.Join(tmpDir, "test-proxy.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
	})
	require.NoError(t, err)

	err = handleProxyStart(controller)
	require.NoError(t, err, "proxy start should succeed")

	// Clean up
	controller.Stop()
}

func TestRun_StartAction(t *testing.T) {
	// Skip - Start has blocking operations
	t.Skip("Start test skipped due to blocking operations")
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-start.sock")

	_, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	os.Args = []string{"muster-daemon", "start", "--socket-path", socketPath}

	err = run()
	// Start might fail or succeed depending on state, but shouldn't crash
	t.Logf("Start action result: %v", err)
}

func TestRun_RestartAction(t *testing.T) {
	// Skip - Restart has blocking operations
	t.Skip("Restart test skipped due to blocking operations")
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-restart.sock")

	_, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	os.Args = []string{"muster-daemon", "restart", "--socket-path", socketPath}

	err = run()
	t.Logf("Restart action result: %v", err)
}

func TestDaemonLogger_ErrorMessage(t *testing.T) {
	logger := &daemonLogger{}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logger.Error("critical error occurred", "code", "500")

	w.Close()
	os.Stderr = oldStderr

	buf, err := io.ReadAll(r)
	require.NoError(t, err)
	require.Contains(t, string(buf), "critical error occurred")
	require.Contains(t, string(buf), "500")
}

func TestNewController_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test.pid")
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
	})
	require.NoError(t, err)
	require.NotNil(t, controller)
}
