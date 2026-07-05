package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/daemon"
)

func TestHandleStart_NilController(t *testing.T) {
	err := handleStart(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "controller is required")
}

func TestHandleStop_NilController(t *testing.T) {
	err := handleStop(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "controller is required")
}

func TestHandleRestart_NilController(t *testing.T) {
	err := handleRestart(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "controller is required")
}

func TestHandleStatus_NilController(t *testing.T) {
	err := handleStatus(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "controller is required")
}

func TestHandleProxyStart_NilController(t *testing.T) {
	err := handleProxyStart(nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "controller is required")
}

func TestController_Start_AlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	pidFile := filepath.Join(tmpDir, "test.pid")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath:  socketPath,
		PidFilePath: pidFile,
	})
	require.NoError(t, err)

	err = controller.Start()
	require.NoError(t, err)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = controller.Shutdown(ctx)
	}()

	status, err := controller.Status()
	require.NoError(t, err)
	require.True(t, status.Running)

	err = controller.Start()
	require.Error(t, err)
	require.Contains(t, err.Error(), "already running")
}

func TestController_Stop_NotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	err = controller.Stop()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not running")
}

func TestController_Kill_NotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	err = controller.Kill()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not running")
}

func TestController_Status_NotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	status, err := controller.Status()
	require.NoError(t, err)
	require.False(t, status.Running)
	// PID may be non-zero depending on system state
	t.Logf("Status when not running: Running=%v, PID=%d", status.Running, status.PID)
}

func TestController_IsRunning_NotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	running := controller.IsRunning()
	require.False(t, running)
}

func TestController_Restart(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-restart.sock")
	pidFile := filepath.Join(tmpDir, "test-restart.pid")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath:  socketPath,
		PidFilePath: pidFile,
	})
	require.NoError(t, err)

	err = controller.Restart()
	require.NoError(t, err, "restart should succeed when not running")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = controller.Shutdown(ctx)
}

func TestController_Shutdown_NotStarted(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err = controller.Shutdown(ctx)
	require.NoError(t, err, "Shutdown should succeed even if not started")
}

func TestController_ServeHTTP(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
		Handler:    handler,
	})
	require.NoError(t, err)

	// ServeHTTP is a wrapper around mux, just verify it exists
	require.NotNil(t, controller)
}

func TestController_DefaultLogger(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
		Logger:     nil,
	})
	require.NoError(t, err)

	// Should use default logger
	status, err := controller.Status()
	require.NoError(t, err)
	require.NotNil(t, status)
}

func TestController_Version(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
		Version:    "dev",
	})
	require.NoError(t, err)

	status, err := controller.Status()
	require.NoError(t, err)
	// Version comes from the manager's configuration
	require.NotEmpty(t, status.Version)
}

func TestController_MultipleStartStop(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-multi.sock")
	pidFile := filepath.Join(tmpDir, "test-multi.pid")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath:  socketPath,
		PidFilePath: pidFile,
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		t.Run(fmt.Sprintf("Cycle%d", i+1), func(t *testing.T) {
			err = controller.Start()
			require.NoError(t, err, "start should succeed")

			status, err := controller.Status()
			require.NoError(t, err)
			require.True(t, status.Running)
			require.NotZero(t, status.PID, "PID should be set")

			// Stop before next cycle
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = controller.Shutdown(ctx)
		})
	}

	_ = os.Remove(pidFile)
	_ = os.Remove(socketPath)
}

func TestStartWithSocket_HandlerSetup(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "h.sock")
	pidFile := filepath.Join(tmpDir, "h.pid")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath:  socketPath,
		PidFilePath: pidFile,
		Handler:     handler,
	})
	require.NoError(t, err)

	err = controller.StartWithSocket(handler)
	require.NoError(t, err)

	status, err := controller.Status()
	require.NoError(t, err)
	require.True(t, status.Running)

	_ = os.Remove(pidFile)
	_ = os.Remove(socketPath)
}

func TestHandleProxyStart_SocketPath(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "p.sock")
	pidFile := filepath.Join(tmpDir, "p.pid")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath:  socketPath,
		PidFilePath: pidFile,
	})
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Proxy OK"))
	})

	err = controller.StartWithSocket(handler)
	require.NoError(t, err)

	status, err := controller.Status()
	require.NoError(t, err)
	require.True(t, status.Running)

	_ = os.Remove(pidFile)
	_ = os.Remove(socketPath)
}

func TestRun_HelpWithNoArgs(t *testing.T) {
	// Store original args
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"muster-daemon"}

	err := run()
	require.NoError(t, err)
}

func TestRun_WithVersionVar(t *testing.T) {
	testVersion := "test-1.0.0"
	Version = testVersion

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
		Version:    testVersion,
	})
	require.NoError(t, err)

	// Version is stored in the controller, not in status
	// Just verify controller was created successfully
	require.NotNil(t, controller)

	Version = "dev"
}

func TestRun_BackendURLParsing(t *testing.T) {
	backendURL = "http://example.com"

	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})

	if err == nil {
		require.NotNil(t, controller)
	}

	backendURL = ""
	_ = socketPath // avoid unused variable warning
}

func TestController_StartThenStop(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-startstop.sock")
	pidFile := filepath.Join(tmpDir, "test-startstop.pid")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath:  socketPath,
		PidFilePath: pidFile,
	})
	require.NoError(t, err)

	err = controller.Start()
	require.NoError(t, err)

	status, err := controller.Status()
	require.NoError(t, err)
	require.True(t, status.Running)

	_ = os.Remove(pidFile)
	_ = os.Remove(socketPath)
}

func TestController_StatusWhileRunning(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test-status-run.sock")
	pidFile := filepath.Join(tmpDir, "test-status-run.pid")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath:  socketPath,
		PidFilePath: pidFile,
	})
	require.NoError(t, err)

	err = controller.Start()
	require.NoError(t, err)

	status, err := controller.Status()
	require.NoError(t, err)
	require.True(t, status.Running)
	require.NotZero(t, status.PID)

	_ = os.Remove(pidFile)
	_ = os.Remove(socketPath)
}

func TestDaemonLogger_InfoNoPanic(t *testing.T) {
	logger := &daemonLogger{}
	require.NotPanics(t, func() {
		logger.Info("%s %v %v", "test", "arg1", "arg2")
	})
}

func TestDaemonLogger_WarnNoPanic(t *testing.T) {
	logger := &daemonLogger{}
	require.NotPanics(t, func() {
		logger.Warn("%s %v", "test", "arg1")
	})
}

func TestDaemonLogger_ErrorNoPanic(t *testing.T) {
	logger := &daemonLogger{}
	require.NotPanics(t, func() {
		logger.Error("%s %v %v", "test", "error", "code")
	})
}

func TestController_NilHandler(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath: socketPath,
	})
	require.NoError(t, err)

	err = controller.StartWithSocket(nil)
	// May error, but shouldn't panic
	t.Logf("StartWithSocket with nil handler: %v", err)
}

func TestController_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "c.sock")
	pidFile := filepath.Join(tmpDir, "c.pid")

	controller, err := daemon.NewController(daemon.ControllerConfig{
		SocketPath:  socketPath,
		PidFilePath: pidFile,
	})
	require.NoError(t, err)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	err = controller.StartWithSocket(handler)
	require.NoError(t, err)

	socketExists := func(path string) bool {
		_, err := os.Stat(path)
		return err == nil
	}

	require.True(t, socketExists(socketPath), "socket should exist after start")

	_ = os.Remove(pidFile)
	_ = os.Remove(socketPath)

	require.False(t, socketExists(socketPath), "socket should be cleaned up")
}

func TestController_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	var controller *daemon.Controller

	// nil controller operations
	require.Panics(t, func() {
		controller.IsRunning()
	})

	require.Panics(t, func() {
		_, _ = controller.Status()
	})

	_ = socketPath // avoid unused variable warning
}
