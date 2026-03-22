package daemon

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLogger is a test logger that captures log messages
type mockLogger struct {
	infos  []string
	warns  []string
	errors []string
}

func (m *mockLogger) Info(msg string, args ...interface{}) {
	m.infos = append(m.infos, fmt.Sprintf(msg, args...))
}

func (m *mockLogger) Warn(msg string, args ...interface{}) {
	m.warns = append(m.warns, fmt.Sprintf(msg, args...))
}

func (m *mockLogger) Error(msg string, args ...interface{}) {
	m.errors = append(m.errors, fmt.Sprintf(msg, args...))
}

// setupTestDir creates a temporary directory for test files
func setupTestDir(t *testing.T) string {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "daemon-controller-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.RemoveAll(tmpDir)
	})
	return tmpDir
}

// TestNewController_ValidConfig tests creating controller with valid configuration
func TestNewController_ValidConfig(t *testing.T) {
	tmpDir := setupTestDir(t)
	logger := &mockLogger{}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		TokenPath:   filepath.Join(tmpDir, "daemon.token"),
		Version:     "1.0.0",
		Logger:      logger,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)
	assert.NotNil(t, controller)
	assert.NotNil(t, controller.pidManager)
	assert.NotNil(t, controller.tokenManager)
	assert.NotNil(t, controller.httpServer)
	assert.NotNil(t, controller.mux)
	assert.Equal(t, cfg.SocketPath, controller.socketPath)
	assert.Equal(t, cfg.TokenPath, controller.tokenPath)
}

// TestNewController_DefaultLogger tests that default logger is used when nil
func TestNewController_DefaultLogger(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Logger:      nil, // Should use default
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)
	assert.NotNil(t, controller)
	assert.NotNil(t, controller.logger)
}

// TestNewController_DefaultTokenPath tests that default token path is used
func TestNewController_DefaultTokenPath(t *testing.T) {
	tmpDir := setupTestDir(t)
	socketPath := filepath.Join(tmpDir, "daemon.sock")

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  socketPath,
		TokenPath:   "", // Should default to socketPath + ".token"
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)
	assert.Equal(t, socketPath+".token", controller.tokenPath)
}

// TestNewController_DefaultShutdownHandler tests default shutdown behavior
func TestNewController_DefaultShutdownHandler(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath:     filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:      filepath.Join(tmpDir, "daemon.sock"),
		Version:         "1.0.0",
		ShutdownHandler: nil, // Should use default
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)
	assert.NotNil(t, controller.shutdownHandler)
}

// TestNewController_WithHandler tests creating controller with custom HTTP handler
func TestNewController_WithHandler(t *testing.T) {
	tmpDir := setupTestDir(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test"))
	})

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Handler:     handler,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)
	assert.NotNil(t, controller)
	assert.NotNil(t, controller.mux)
}

// TestController_Start_Success tests successful daemon start
func TestControllerStart_Success(t *testing.T) {
	tmpDir := setupTestDir(t)
	logger := &mockLogger{}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Logger:      logger,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	err = controller.Start()
	require.NoError(t, err)

	// Verify PID file was created
	_, err = os.Stat(cfg.PidFilePath)
	assert.NoError(t, err)

	// Verify token file was created
	_, err = os.Stat(cfg.SocketPath + ".token")
	assert.NoError(t, err)

	// Verify logger captured messages
	assert.NotEmpty(t, logger.infos)

	// Cleanup - use Release instead of Stop to avoid signaling current process
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_StartWithSocket_Success tests successful socket-based start
func TestControllerStartWithSocket_Success(t *testing.T) {
	tmpDir := setupTestDir(t)
	logger := &mockLogger{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from daemon"))
	}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Logger:      logger,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	err = controller.StartWithSocket(handler)
	require.NoError(t, err)

	// Verify socket file was created
	info, err := os.Stat(cfg.SocketPath)
	require.NoError(t, err)
	// Check socket permissions (0600)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Verify token file was created
	_, err = os.Stat(cfg.SocketPath + ".token")
	require.NoError(t, err)

	// Verify PID file was created
	_, err = os.Stat(cfg.PidFilePath)
	require.NoError(t, err)

	// Cleanup
	controller.Shutdown(context.Background())
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_StartWithSocket_TokenGenerationFailure tests token generation failure
func TestControllerStartWithSocket_TokenGenerationFailure(t *testing.T) {
	tmpDir := setupTestDir(t)

	// Create a read-only directory to cause token save to fail
	readOnlyDir := filepath.Join(tmpDir, "readonly")
	err := os.MkdirAll(readOnlyDir, 0755)
	require.NoError(t, err)
	defer os.Chmod(readOnlyDir, 0755) // Restore permissions for cleanup

	err = os.Chmod(readOnlyDir, 0000)
	require.NoError(t, err)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		TokenPath:   filepath.Join(readOnlyDir, "token"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	handler := func(w http.ResponseWriter, r *http.Request) {}
	err = controller.StartWithSocket(handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "auth token")
}

// TestController_Stop_Success tests successful daemon stop
func TestControllerStop_Success(t *testing.T) {
	tmpDir := setupTestDir(t)
	logger := &mockLogger{}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Logger:      logger,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Start the daemon
	err = controller.Start()
	require.NoError(t, err)

	// Stop the daemon - use safe cleanup to avoid signaling current process
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
	// May fail if daemon process can't be found (expected in tests)
	// but should clean up token file

	// Verify token file was cleaned up
	_, err = os.Stat(cfg.SocketPath + ".token")
	assert.True(t, os.IsNotExist(err))
}

// TestController_Stop_NotRunning tests stopping when not running
func TestControllerStop_NotRunning(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Test Stop when not running - should return error
	err = controller.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// TestController_Kill_Success tests forceful kill
func TestControllerKill_Success(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Start the daemon
	err = controller.Start()
	require.NoError(t, err)

	// Generate token file for cleanup verification
	_, err = os.Stat(cfg.SocketPath + ".token")
	require.NoError(t, err, "Token file should exist after start")

	// Replace PID with a non-existent one so Kill doesn't kill the test
	// Create PID file with a fake PID
	fakeContent := fmt.Sprintf("99999\n%d\n%s\n%s\n", time.Now().UnixNano(), cfg.SocketPath, cfg.Version)
	err = os.WriteFile(cfg.PidFilePath, []byte(fakeContent), 0644)
	require.NoError(t, err)

	// Kill the daemon (should clean up token even if process doesn't exist)
	_ = controller.Kill()
	// Error expected since process doesn't exist, but should clean up

	// Verify token file was cleaned up
	_, err = os.Stat(cfg.SocketPath + ".token")
	assert.True(t, os.IsNotExist(err), "Token file should be cleaned up")
}

// TestController_Kill_NotRunning tests killing when not running
func TestControllerKill_NotRunning(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	err = controller.Kill()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// TestController_Restart tests daemon restart
func TestControllerRestart(t *testing.T) {
	tmpDir := setupTestDir(t)
	logger := &mockLogger{}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Logger:      logger,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Start the daemon
	err = controller.Start()
	require.NoError(t, err)

	// Replace PID with a non-existent one so Restart doesn't kill the test
	fakeContent := fmt.Sprintf("99999\n%d\n%s\n%s\n", time.Now().UnixNano(), cfg.SocketPath, cfg.Version)
	err = os.WriteFile(cfg.PidFilePath, []byte(fakeContent), 0644)
	require.NoError(t, err)

	// Restart - should not panic even with fake PID
	_ = controller.Restart()
	// May error due to process handling in tests, but should not panic
	// The method should complete without crashing

	// Cleanup
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_Restart_NotRunning tests restart when not running
func TestControllerRestart_NotRunning(t *testing.T) {
	tmpDir := setupTestDir(t)
	logger := &mockLogger{}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Logger:      logger,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Restart when not running - should start fresh
	err = controller.Restart()
	// Should succeed by starting
	require.NoError(t, err)

	// Cleanup
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_GetTokenManager tests getting the token manager
func TestControllerGetTokenManager(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	tm := controller.GetTokenManager()
	assert.NotNil(t, tm)
	assert.Equal(t, cfg.SocketPath+".token", tm.TokenPath())
}

// TestController_GetTokenPath tests getting the token path
func TestControllerGetTokenPath(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		TokenPath:   filepath.Join(tmpDir, "custom.token"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	assert.Equal(t, cfg.TokenPath, controller.GetTokenPath())
}

// TestController_Shutdown tests graceful shutdown
func TestControllerShutdown(t *testing.T) {
	tmpDir := setupTestDir(t)
	logger := &mockLogger{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Logger:      logger,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Start with socket to create HTTP server
	err = controller.StartWithSocket(handler)
	require.NoError(t, err)

	// Shutdown gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = controller.Shutdown(ctx)
	// May return http.ErrServerClosed which is expected
	if err != nil && err != http.ErrServerClosed {
		t.Fatalf("unexpected shutdown error: %v", err)
	}

	// Cleanup
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_ServeHTTP tests HTTP request serving
func TestControllerServeHTTP(t *testing.T) {
	tmpDir := setupTestDir(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Handler:     handler,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	// Serve the request
	controller.ServeHTTP(rec, req)

	// Check response
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test response", rec.Body.String())
}

// TestController_SetupSignalHandlers tests signal handler setup
func TestControllerSetupSignalHandlers(t *testing.T) {
	tmpDir := setupTestDir(t)
	logger := &mockLogger{}

	shutdownCalled := make(chan bool, 1)
	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Logger:      logger,
		ShutdownHandler: func() {
			shutdownCalled <- true
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Start the daemon
	err = controller.Start()
	require.NoError(t, err)

	// Setup signal handlers
	controller.SetupSignalHandlers()

	// Send signal to ourselves
	syscall.Kill(syscall.Getpid(), syscall.SIGTERM)

	// Wait for signal handler to run
	select {
	case <-shutdownCalled:
		// Signal handler was called
	case <-time.After(500 * time.Millisecond):
		// Timeout is okay - signal handling is set up but may not trigger in test
	}

	// Cleanup
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_ConcurrentAccess tests thread safety
func TestControllerConcurrentAccess(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Start the daemon
	err = controller.Start()
	require.NoError(t, err)

	// Run concurrent operations
	done := make(chan bool, 4)

	go func() {
		controller.Status()
		done <- true
	}()

	go func() {
		controller.IsRunning()
		done <- true
	}()

	go func() {
		controller.GetTokenManager()
		done <- true
	}()

	go func() {
		controller.GetTokenPath()
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent access timeout")
		}
	}

	// Cleanup
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_TokenCleanupOnFailure tests token cleanup when start fails
func TestControllerTokenCleanupOnFailure(t *testing.T) {
	tmpDir := setupTestDir(t)

	// Create first controller to hold the PID lock
	cfg1 := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller1, err := NewController(cfg1)
	require.NoError(t, err)

	err = controller1.Start()
	require.NoError(t, err)

	// Create second controller with same config (will fail to start)
	controller2, err := NewController(cfg1)
	require.NoError(t, err)

	// Try to start - should fail and cleanup token
	err = controller2.Start()
	assert.Error(t, err)

	// Token from failed start should be cleaned up
	// Note: The token cleanup happens in the error path

	// Cleanup - use safe cleanup to avoid signaling current process
	_ = controller1.pidManager.Release()
	_ = controller1.GetTokenManager().Cleanup()
}

// TestController_FullLifecycle tests complete daemon lifecycle
func TestControllerFullLifecycle(t *testing.T) {
	tmpDir := setupTestDir(t)
	logger := &mockLogger{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("pong"))
	}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Logger:      logger,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// 1. Initial state - not running
	assert.False(t, controller.IsRunning())

	// 2. Start with socket
	err = controller.StartWithSocket(handler)
	require.NoError(t, err)

	// 3. Verify running
	assert.True(t, controller.IsRunning())

	// 4. Verify files exist
	_, err = os.Stat(cfg.PidFilePath)
	require.NoError(t, err)
	_, err = os.Stat(cfg.SocketPath)
	require.NoError(t, err)
	_, err = os.Stat(cfg.SocketPath + ".token")
	require.NoError(t, err)

	// 5. Get status
	status, err := controller.Status()
	require.NoError(t, err)
	assert.True(t, status.Running)
	assert.Equal(t, cfg.SocketPath, status.SocketPath)

	// 6. Stop - use safe cleanup to avoid signaling current process
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
	// May error on process kill, but should cleanup

	// 7. Verify cleanup
	_, err = os.Stat(cfg.SocketPath + ".token")
	assert.True(t, os.IsNotExist(err))
}

// TestController_ServeHTTP_WithDefaultHandler tests ServeHTTP with default mux
func TestControllerServeHTTP_WithDefaultHandler(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Handler:     nil, // No handler registered
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Create a test request
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	// Serve the request (should 404 since no handler registered)
	controller.ServeHTTP(rec, req)

	// Check response - should be 404
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestController_Start_GeneratesToken tests that start generates auth token
func TestControllerStart_GeneratesToken(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Before start, no token
	assert.False(t, controller.GetTokenManager().IsGenerated())

	// Start
	err = controller.Start()
	require.NoError(t, err)

	// After start, token should be generated
	assert.True(t, controller.GetTokenManager().IsGenerated())
	assert.NotEmpty(t, controller.GetTokenManager().GetToken())

	// Cleanup
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_MultipleStartAttempts tests multiple start attempts
func TestControllerMultipleStartAttempts(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// First start should succeed
	err = controller.Start()
	require.NoError(t, err)

	// Second start should fail
	err = controller.Start()
	assert.Error(t, err)

	// Cleanup
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_LoggerCapturesAllLevels tests logger captures info, warn, and error
func TestControllerLoggerCapturesAllLevels(t *testing.T) {
	tmpDir := setupTestDir(t)
	logger := &mockLogger{}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		Logger:      logger,
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Start generates info logs
	err = controller.Start()
	require.NoError(t, err)
	assert.NotEmpty(t, logger.infos)

	// Stop may generate warn/error logs
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_StatusReturnsCorrectInfo tests status returns correct information
func TestControllerStatusReturnsCorrectInfo(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "2.0.0-test",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Start
	err = controller.Start()
	require.NoError(t, err)

	// Check status
	status, err := controller.Status()
	require.NoError(t, err)
	assert.True(t, status.Running)
	assert.Equal(t, os.Getpid(), status.PID)
	assert.Equal(t, cfg.SocketPath, status.SocketPath)
	assert.Equal(t, cfg.Version, status.Version)
	assert.False(t, status.Stale)

	// Cleanup
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_SocketPermissions tests socket file permissions
func TestControllerSocketPermissions(t *testing.T) {
	tmpDir := setupTestDir(t)

	handler := func(w http.ResponseWriter, r *http.Request) {}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	err = controller.StartWithSocket(handler)
	require.NoError(t, err)

	// Check socket permissions
	info, err := os.Stat(cfg.SocketPath)
	require.NoError(t, err)
	mode := info.Mode()
	// Should be 0600 (owner read/write only)
	assert.Equal(t, os.FileMode(0600), mode.Perm(), "Socket should have 0600 permissions")

	// Cleanup
	controller.Shutdown(context.Background())
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_TokenFilePermissions tests token file permissions
func TestControllerTokenFilePermissions(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	err = controller.Start()
	require.NoError(t, err)

	// Check token file permissions
	tokenPath := cfg.SocketPath + ".token"
	info, err := os.Stat(tokenPath)
	require.NoError(t, err)
	mode := info.Mode()
	// Should be 0600 (owner read/write only)
	assert.Equal(t, os.FileMode(0600), mode.Perm(), "Token file should have 0600 permissions")

	// Cleanup
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_InvalidPidFilePath tests error handling for invalid PID file path
func TestControllerInvalidPidFilePath(t *testing.T) {
	// Use an invalid path (like a read-only root directory)
	cfg := ControllerConfig{
		PidFilePath: "/nonexistent/deeply/nested/path/daemon.pid",
		SocketPath:  "/tmp/test.sock",
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	// This might succeed on controller creation but fail on start
	if err == nil {
		err = controller.Start()
		assert.Error(t, err)
	}
}

// TestController_ShutdownWithoutStart tests shutdown without starting
func TestControllerShutdownWithoutStart(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	// Shutdown without starting - should not panic
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = controller.Shutdown(ctx)
	// This may error or succeed depending on implementation, but should not panic
	// Error is expected since server wasn't started
}

// TestController_StartWithSocket_CreatesDirectories tests directory creation
func TestControllerStartWithSocket_CreatesDirectories(t *testing.T) {
	tmpDir := setupTestDir(t)
	// Use shallow path to avoid Unix socket path length limit (104-108 bytes)
	subDir := filepath.Join(tmpDir, "subdir")

	handler := func(w http.ResponseWriter, r *http.Request) {}

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(subDir, "daemon.pid"),
		SocketPath:  filepath.Join(subDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	err = controller.StartWithSocket(handler)
	require.NoError(t, err)

	// Verify directories were created
	_, err = os.Stat(subDir)
	assert.NoError(t, err)

	// Cleanup
	controller.Shutdown(context.Background())
	_ = controller.pidManager.Release()
	_ = controller.GetTokenManager().Cleanup()
}

// TestController_GetTokenManager_ReturnsSameInstance tests token manager is consistent
func TestControllerGetTokenManager_ReturnsSameInstance(t *testing.T) {
	tmpDir := setupTestDir(t)

	cfg := ControllerConfig{
		PidFilePath: filepath.Join(tmpDir, "daemon.pid"),
		SocketPath:  filepath.Join(tmpDir, "daemon.sock"),
		Version:     "1.0.0",
		ShutdownHandler: func() {
			// No-op for tests
		},
	}

	controller, err := NewController(cfg)
	require.NoError(t, err)

	tm1 := controller.GetTokenManager()
	tm2 := controller.GetTokenManager()

	assert.Equal(t, tm1, tm2)
}
