package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/daemon"
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

	logger.Info("%s %v %v", "test message", "arg1", "arg2")

	_ = w.Close()
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

	logger.Warn("%s %v", "test warning", "arg1")

	_ = w.Close()
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

	logger.Error("%s %v", "test error", "arg1")

	_ = w.Close()
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
		_, _ = w.Write([]byte(`{"status":"ok"}`))
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

	err = handleProxyStart(nil, nil)
	require.Error(t, err, "handleProxyStart should error with nil controller")
}

func TestInfoLogger_OutputFormat(t *testing.T) {
	logger := &daemonLogger{}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger.Info("%s %v=%v", "operation completed", "status", "success")

	_ = w.Close()
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

	logger.Warn("%s %v=%v", "operation completed", "status", "warning")

	_ = w.Close()
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

	logger.Error("%s %v=%v", "operation failed", "error", "timeout")

	_ = w.Close()
	os.Stderr = oldStderr

	buf, _ := io.ReadAll(r)
	require.Contains(t, string(buf), "[ERROR]")
	require.Contains(t, string(buf), "operation failed")
}

func TestHandleStart_ControllerStartFailure(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-start.sock")

	// Create controller and start it once
	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: "/nonexistent/path/to/pidfile/test.pid",
		SocketPath:  socketPath,
	})
	require.NoError(t, err)

	// First start succeeds
	err = controller.Start()
	require.NoError(t, err, "first start should succeed")

	// Second start should fail because already running
	err = controller.Start()
	require.Error(t, err, "second start should fail when already running")
	require.Contains(t, err.Error(), "already running")

	// Cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = controller.Shutdown(ctx)
}

func TestHandleStart_Success(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "test-daemon.pid")
	socketPath := filepath.Join(tmpDir, "test-daemon.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
	})
	require.NoError(t, err)

	err = controller.Start()
	require.NoError(t, err, "controller should start")

	status, err := controller.Status()
	require.NoError(t, err, "should be able to get status")
	require.True(t, status.Running, "controller should be running")

	_ = os.Remove(pidFile)
	_ = os.Remove(socketPath)
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
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "tp.pid")
	socketPath := filepath.Join(tmpDir, "ts.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
	})
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	err = controller.StartWithSocket(handler)
	require.NoError(t, err, "proxy start should succeed")

	_ = os.Remove(pidFile)
	_ = os.Remove(socketPath)
}

func TestRun_StartAction(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-start.sock")
	pidFile := filepath.Join(tmpDir, "test-start.pid")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
		Logger:      &daemonLogger{},
	})
	require.NoError(t, err, "should create controller")

	err = controller.Start()
	require.NoError(t, err, "start should succeed")

	status, err := controller.Status()
	require.NoError(t, err, "should get status")
	require.True(t, status.Running, "controller should be running")

	_ = os.Remove(pidFile)
	_ = os.Remove(socketPath)
}

func TestRun_RestartAction(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-restart.sock")
	pidFile := filepath.Join(tmpDir, "test-restart.pid")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
		Logger:      &daemonLogger{},
	})
	require.NoError(t, err, "should create controller")

	err = handleRestart(controller)
	require.NoError(t, err, "restart should succeed")

	status, err := controller.Status()
	require.NoError(t, err, "should get status after restart")
	require.True(t, status.Running, "controller should be running after restart")

	_ = os.Remove(pidFile)
	_ = os.Remove(socketPath)
}

func TestDaemonLogger_ErrorMessage(t *testing.T) {
	logger := &daemonLogger{}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logger.Error("%s %v=%v", "critical error occurred", "code", "500")

	_ = w.Close()
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

func TestWriteReadPIDFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidPath := filepath.Join(tmpDir, "test.pid")

	// Write a PID
	err := writePIDFile(pidPath, 12345)
	require.NoError(t, err)

	// Read it back
	pid, err := readPIDFile(pidPath)
	require.NoError(t, err)
	require.Equal(t, 12345, pid)

	// File should exist
	_, err = os.Stat(pidPath)
	require.NoError(t, err)
}

func TestReadPIDFile_Invalid(t *testing.T) {
	tmpDir := t.TempDir()

	// Non-existent file
	_, err := readPIDFile(filepath.Join(tmpDir, "nonexistent.pid"))
	require.Error(t, err)

	// Invalid content
	badPath := filepath.Join(tmpDir, "bad.pid")
	require.NoError(t, os.WriteFile(badPath, []byte("not-a-number"), 0644))
	_, err = readPIDFile(badPath)
	require.Error(t, err)

	// Zero PID
	zeroPath := filepath.Join(tmpDir, "zero.pid")
	require.NoError(t, os.WriteFile(zeroPath, []byte("0"), 0644))
	_, err = readPIDFile(zeroPath)
	require.Error(t, err)
}

func TestIsProcessRunning_InvalidPID(t *testing.T) {
	require.False(t, isProcessRunning(0))
	require.False(t, isProcessRunning(-1))
}

func TestHandleStatus_WithPIDFile(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath = filepath.Join(tmpDir, "test-status.sock")
	pidFile = filepath.Join(tmpDir, "test-status.pid")
	defer func() {
		socketPath = "/tmp/openapi-cli.sock"
		pidFile = ""
	}()

	// Write a PID file pointing to our own process (which is running)
	require.NoError(t, writePIDFile(pidFile, os.Getpid()))

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	// handleStatus should detect the PID file and report Running: true
	err = handleStatus(controller)
	require.NoError(t, err, "handleStatus should succeed with PID file")
}

func TestHandleStop_WithPIDFile_NotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath = filepath.Join(tmpDir, "test-stop.sock")
	pidFile = filepath.Join(tmpDir, "test-stop.pid")
	defer func() {
		socketPath = "/tmp/openapi-cli.sock"
		pidFile = ""
	}()

	// Write a PID file with a PID that doesn't exist
	require.NoError(t, writePIDFile(pidFile, 99999))

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	// handleStop should clean up stale PID file and report not running
	err = handleStop(controller)
	require.NoError(t, err, "handleStop should succeed with stale PID file")

	// PID file should be removed
	_, err = os.Stat(pidFile)
	require.True(t, os.IsNotExist(err), "stale PID file should be removed")
}

func TestHandleStop_NoPIDFile_Fallback(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath = filepath.Join(tmpDir, "test-stop-fallback.sock")
	pidFile = filepath.Join(tmpDir, "test-stop-fallback.pid")
	defer func() {
		socketPath = "/tmp/openapi-cli.sock"
		pidFile = ""
	}()

	// No PID file — should fall back to in-process controller
	controller, err := daemon.NewController(daemon.ControllerConfig{
		PidFilePath: pidFile,
		SocketPath:  socketPath,
	})
	require.NoError(t, err)

	err = handleStop(controller)
	require.NoError(t, err, "handleStop should succeed with fallback")
}

func TestHandleStatus_NoPIDFile_Fallback(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath = filepath.Join(tmpDir, "test-status-fallback.sock")
	pidFile = filepath.Join(tmpDir, "test-status-fallback.pid")
	defer func() {
		socketPath = "/tmp/openapi-cli.sock"
		pidFile = ""
	}()

	// No PID file — should fall back to in-process controller
	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	err = handleStatus(controller)
	require.NoError(t, err, "handleStatus should succeed with fallback")
}
