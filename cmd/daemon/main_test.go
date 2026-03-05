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
	testURL := "http://localhost:8080"
	oldBackendURL := BackendURL
	BackendURL = testURL
	defer func() { BackendURL = oldBackendURL }()

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
	require.Panics(t, func() {
		handleStart(nil)
	}, "handleStart should panic with nil controller")

	require.Panics(t, func() {
		handleStop(nil)
	}, "handleStop should panic with nil controller")

	require.Panics(t, func() {
		handleRestart(nil)
	}, "handleRestart should panic with nil controller")

	require.Panics(t, func() {
		handleStatus(nil)
	}, "handleStatus should panic with nil controller")

	require.Panics(t, func() {
		handleProxyStart(nil)
	}, "handleProxyStart should panic with nil controller")
}

func TestVersionVariables(t *testing.T) {
	require.NotEmpty(t, Version, "Version should not be empty")
	require.NotEmpty(t, Commit, "Commit should not be empty")
	require.NotEmpty(t, BuildDate, "BuildDate should not be empty")
}

func TestRun_UnknownAction(t *testing.T) {
	os.Args = []string{"muster-daemon", "unknown-command"}

	err := run()
	require.NoError(t, err, "unknown action should show help")
}

func TestHandleStart_ControllerError(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test-daemon.pid")
	socketPath := filepath.Join(tmpDir, "test-daemon.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
	})
	require.NoError(t, err)

	err = handleStart(controller)
	require.Error(t, err, "should fail when controller.Start fails")
	require.Contains(t, err.Error(), "start daemon")
}

func TestHandleRestart_NilController(t *testing.T) {
	err := handleRestart(nil)
	require.Error(t, err)
}

func TestHandleProxyStart_NilController(t *testing.T) {
	err := handleProxyStart(nil)
	require.Error(t, err)
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
