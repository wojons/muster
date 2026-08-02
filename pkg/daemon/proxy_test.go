package daemon

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/cache"
)

// socketTestDir returns a SHORT temp directory for Unix socket tests.
// t.TempDir() embeds the test name under a long TMPDIR (e.g. GitHub's
// /var/folders/.../T/TestProxyServer_Start_ReplacesOldListener123456/001),
// pushing the socket path past macOS's 104-byte sockaddr_un.sun_path limit —
// bind() then fails with EINVAL ("bind: invalid argument").
func socketTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ms-")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// =============================================================================
// Controller tests
// =============================================================================

// TestNewController verifies that NewController creates a valid Controller
// with all fields populated from ControllerConfig.
func TestNewController(t *testing.T) {
	cfg := ControllerConfig{
		PidFilePath: "/tmp/muster-test.pid",
		SocketPath:  "/tmp/muster-test.sock",
		Version:     "1.0.0",
		Handler:     http.NewServeMux(),
	}

	ctrl, err := NewController(cfg)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	assert.Equal(t, "/tmp/muster-test.pid", ctrl.pidFile)
	assert.Equal(t, "/tmp/muster-test.sock", ctrl.socketPath)
	assert.Equal(t, "1.0.0", ctrl.version)
	assert.NotNil(t, ctrl.handler)
	assert.Equal(t, "/tmp/muster-test.sock.token", ctrl.tokenPath)
	assert.False(t, ctrl.running)
	assert.Nil(t, ctrl.proxyServer)
}

// TestNewController_Defaults verifies that NewController handles zero-value
// config fields gracefully.
func TestNewController_Defaults(t *testing.T) {
	cfg := ControllerConfig{}

	ctrl, err := NewController(cfg)
	require.NoError(t, err)
	require.NotNil(t, ctrl)

	assert.Empty(t, ctrl.pidFile)
	assert.Empty(t, ctrl.socketPath)
	assert.Empty(t, ctrl.version)
	assert.Nil(t, ctrl.handler)
	assert.Equal(t, ".token", ctrl.tokenPath) // empty socket + ".token"
}

// TestController_Start verifies that Start() transitions the controller
// to the running state.
func TestController_Start(t *testing.T) {
	ctrl, _ := NewController(ControllerConfig{Version: "1.0.0"})

	// Should not be running initially
	assert.False(t, ctrl.IsRunning())

	// Start
	err := ctrl.Start()
	require.NoError(t, err)
	assert.True(t, ctrl.IsRunning())

	// Second start should fail
	err = ctrl.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

// TestController_Stop verifies that Stop() transitions the controller
// from running to stopped.
func TestController_Stop(t *testing.T) {
	ctrl, _ := NewController(ControllerConfig{Version: "1.0.0"})

	// Stop when not running should fail
	err := ctrl.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")

	// Start then stop
	require.NoError(t, ctrl.Start())
	assert.True(t, ctrl.IsRunning())

	err = ctrl.Stop()
	require.NoError(t, err)
	assert.False(t, ctrl.IsRunning())
}

// TestController_Stop_WithProxyServer verifies that Stop() also shuts down
// an attached proxy server.
func TestController_Stop_WithProxyServer(t *testing.T) {
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")

	ctrl, err := NewController(ControllerConfig{
		SocketPath: socketPath,
		Version:    "1.0.0",
	})
	require.NoError(t, err)

	// Start with a proxy server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	err = ctrl.StartWithSocket(handler)
	require.NoError(t, err)
	assert.True(t, ctrl.IsRunning())

	// Stop should shut down the proxy server too
	err = ctrl.Stop()
	require.NoError(t, err)
	assert.False(t, ctrl.IsRunning())
	assert.Nil(t, ctrl.proxyServer)

	// Socket should be cleaned up
	_, err = os.Stat(socketPath)
	assert.True(t, os.IsNotExist(err), "socket file should be removed")
}

// TestController_Status verifies that Status() returns correct
// DaemonStatus for both running and stopped states.
func TestController_Status(t *testing.T) {
	ctrl, _ := NewController(ControllerConfig{Version: "2.3.1"})

	// Before start
	status, err := ctrl.Status()
	require.NoError(t, err)
	assert.False(t, status.Running)
	assert.Equal(t, "2.3.1", status.Version)
	assert.True(t, status.StartTime.IsZero())

	// After start
	require.NoError(t, ctrl.Start())
	status, err = ctrl.Status()
	require.NoError(t, err)
	assert.True(t, status.Running)
	assert.Equal(t, "2.3.1", status.Version)
	assert.False(t, status.StartTime.IsZero())
	assert.Equal(t, os.Getpid(), status.PID)

	// After stop
	require.NoError(t, ctrl.Stop())
	status, err = ctrl.Status()
	require.NoError(t, err)
	assert.False(t, status.Running)
}

// TestController_IsRunning verifies the IsRunning() method.
func TestController_IsRunning(t *testing.T) {
	ctrl, _ := NewController(ControllerConfig{Version: "0.0.1"})

	assert.False(t, ctrl.IsRunning(), "should not be running before Start")

	require.NoError(t, ctrl.Start())
	assert.True(t, ctrl.IsRunning(), "should be running after Start")

	require.NoError(t, ctrl.Stop())
	assert.False(t, ctrl.IsRunning(), "should not be running after Stop")
}

// TestController_Restart verifies the Restart() lifecycle transition.
func TestController_Restart(t *testing.T) {
	ctrl, _ := NewController(ControllerConfig{Version: "1.0.0"})

	// Restart from stopped state
	err := ctrl.Restart()
	require.NoError(t, err)
	assert.True(t, ctrl.IsRunning())

	// Restart from running state
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")
	ctrl2, _ := NewController(ControllerConfig{
		SocketPath: socketPath,
		Version:    "1.0.0",
	})
	require.NoError(t, ctrl2.StartWithSocket(http.NewServeMux()))
	assert.True(t, ctrl2.IsRunning())
	assert.NotNil(t, ctrl2.proxyServer)

	err = ctrl2.Restart()
	require.NoError(t, err)
	assert.True(t, ctrl2.IsRunning())
	assert.Nil(t, ctrl2.proxyServer) // proxy server stopped during restart
}

// TestController_ServeHTTP verifies the HTTP handler delegation.
func TestController_ServeHTTP(t *testing.T) {
	ctrl, _ := NewController(ControllerConfig{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"custom"}`))
		}),
	})

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rec := httptest.NewRecorder()
	ctrl.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "custom")

	// Controller with nil handler returns 500
	ctrl2, _ := NewController(ControllerConfig{})
	rec2 := httptest.NewRecorder()
	ctrl2.ServeHTTP(rec2, req)
	assert.Equal(t, http.StatusInternalServerError, rec2.Code)
	assert.Contains(t, rec2.Body.String(), "no handler configured")
}

// TestController_GetTokenPath verifies token path derivation.
func TestController_GetTokenPath(t *testing.T) {
	ctrl, _ := NewController(ControllerConfig{
		SocketPath: "/var/run/muster.sock",
	})
	assert.Equal(t, "/var/run/muster.sock.token", ctrl.GetTokenPath())
}

// TestController_Handler verifies the Handler() accessor.
func TestController_Handler(t *testing.T) {
	h := http.NewServeMux()
	ctrl, _ := NewController(ControllerConfig{Handler: h})
	assert.Equal(t, h, ctrl.Handler())

	ctrl2, _ := NewController(ControllerConfig{})
	assert.Nil(t, ctrl2.Handler())
}

// TestController_Shutdown verifies graceful shutdown.
func TestController_Shutdown(t *testing.T) {
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")

	ctrl, err := NewController(ControllerConfig{
		SocketPath: socketPath,
		Version:    "1.0.0",
	})
	require.NoError(t, err)

	require.NoError(t, ctrl.StartWithSocket(http.NewServeMux()))
	assert.True(t, ctrl.IsRunning())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = ctrl.Shutdown(ctx)
	require.NoError(t, err)
	assert.False(t, ctrl.IsRunning())
}

// TestController_Kill verifies immediate termination.
func TestController_Kill(t *testing.T) {
	// Kill when not running
	ctrl, _ := NewController(ControllerConfig{Version: "1.0.0"})
	err := ctrl.Kill()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")

	// Kill when running with proxy server
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")

	ctrl2, err := NewController(ControllerConfig{
		SocketPath: socketPath,
		Version:    "1.0.0",
	})
	require.NoError(t, err)

	require.NoError(t, ctrl2.StartWithSocket(http.NewServeMux()))
	assert.True(t, ctrl2.IsRunning())

	err = ctrl2.Kill()
	require.NoError(t, err)
	assert.False(t, ctrl2.IsRunning())
}

// =============================================================================
// ProxyServer tests
// =============================================================================

// TestNewProxyServer verifies that NewProxyServer creates a valid ProxyServer.
func TestNewProxyServer(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	socketPath := "/tmp/test-proxy.sock"
	ps := NewProxyServer(socketPath, handler)

	require.NotNil(t, ps)
	assert.Equal(t, socketPath, ps.socketPath)
	assert.NotNil(t, ps.handler)
	assert.NotNil(t, ps.shutdownChan)

	// Test with nil handler (default handler is created)
	ps2 := NewProxyServer(socketPath, nil)
	require.NotNil(t, ps2)
	assert.NotNil(t, ps2.handler)
}

// TestProxyServer_Start tests starting a ProxyServer on a Unix socket.
func TestProxyServer_Start(t *testing.T) {
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	ps := NewProxyServer(socketPath, handler)

	err := ps.Start()
	require.NoError(t, err)

	// Verify the socket exists
	_, err = os.Stat(socketPath)
	require.NoError(t, err, "socket should exist after start")

	// Connect to the socket and make an HTTP request
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	resp, err := client.Get("http://unix/any-path")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", string(body))

	// Clean up
	err = ps.Stop()
	require.NoError(t, err)
}

// TestProxyServer_Start_ReplacesOldListener verifies that calling Start() a second
// time replaces the old listener — no error is returned. The old socket is removed
// and a new listener is created.
func TestProxyServer_Start_ReplacesOldListener(t *testing.T) {
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")

	ps := NewProxyServer(socketPath, nil)
	require.NoError(t, ps.Start())

	// Second Start replaces the old listener (no error — Start()
	// removes the existing socket and binds again).
	err := ps.Start()
	require.NoError(t, err)

	// The socket should exist and accept connections.
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}
	resp, err := client.Get("http://unix/health")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	require.NoError(t, ps.Stop())
}

// TestProxyServer_Stop verifies clean shutdown.
func TestProxyServer_Stop(t *testing.T) {
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")

	ps := NewProxyServer(socketPath, nil)
	require.NoError(t, ps.Start())

	err := ps.Stop()
	require.NoError(t, err)

	// Socket should be removed
	_, err = os.Stat(socketPath)
	assert.True(t, os.IsNotExist(err), "socket should be removed after stop")
}

// TestProxyServer_Stop_NotStarted verifies that stopping a never-started
// server is a no-op.
func TestProxyServer_Stop_NotStarted(t *testing.T) {
	ps := NewProxyServer("/tmp/nonexistent.sock", nil)
	err := ps.Stop()
	require.NoError(t, err) // no-op when server is nil
}

// TestProxyServer_HealthEndpoint verifies the default health endpoints.
func TestProxyServer_HealthEndpoint(t *testing.T) {
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")

	ps := NewProxyServer(socketPath, nil) // nil handler → createDefaultHandler
	require.NoError(t, ps.Start())
	defer ps.Stop()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{"/health", http.StatusOK, "healthy"},
		{"/health/live", http.StatusOK, "alive"},
		{"/health/ready", http.StatusOK, "ready"},
		{"/health/startup", http.StatusOK, "started"},
		{"/", http.StatusOK, "Muster daemon is running"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := client.Get("http://unix" + tt.path)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			body, _ := io.ReadAll(resp.Body)
			assert.Contains(t, string(body), tt.wantBody)
		})
	}
}

// TestProxyServer_IsRunning verifies the IsRunning() method.
func TestProxyServer_IsRunning(t *testing.T) {
	ps := NewProxyServer("/tmp/test.sock", nil)
	assert.False(t, ps.IsRunning(), "should not be running before Start")

	// After start, IsRunning should return true (server exists and not shutdown)
	ps.server = &http.Server{}
	assert.True(t, ps.IsRunning(), "should be running when server is set")

	// After shutdown signal
	close(ps.shutdownChan)
	assert.False(t, ps.IsRunning(), "should not be running after shutdown signal")
}

// TestProxyServer_SocketPath verifies the SocketPath() accessor.
func TestProxyServer_SocketPath(t *testing.T) {
	ps := NewProxyServer("/var/run/muster.sock", nil)
	assert.Equal(t, "/var/run/muster.sock", ps.SocketPath())
}

// =============================================================================
// isHealthEndpoint tests
// =============================================================================

func TestIsHealthEndpoint(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/health", true},
		{"/health/", true},
		{"/health/live", true},
		{"/health/live/", true},
		{"/health/ready", true},
		{"/health/startup", true},
		{"/api/tasks", false},
		{"/", false},
		{"/healthz", false},
		{"/healthcheck", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			assert.Equal(t, tt.expected, isHealthEndpoint(tt.path))
		})
	}
}

// =============================================================================
// sanitizeProxyPath tests
// =============================================================================

func TestSanitizeProxyPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "/"},
		{"/api/tasks", "/api/tasks"},
		{"/api/tasks/../tasks", ""}, // path traversal
		{"/api/../../etc/passwd", ""},
		{"api", "/api"}, // missing leading slash
		{"/api/./tasks", "/api/tasks"},
		{"/api//tasks", "/api/tasks"},
		{"/api", "/api"},
		{"path\x00traversal", ""},  // null byte
		{"/path\r\ninjection", ""}, // newline smuggling
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeProxyPath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// isHopHeader tests
// =============================================================================

func TestIsHopHeader(t *testing.T) {
	hopHeaders := []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate",
		"Proxy-Authorization", "Te", "Trailers",
		"Transfer-Encoding", "Upgrade",
	}
	for _, h := range hopHeaders {
		assert.True(t, isHopHeader(h), "expected hop-by-hop header: %s", h)
	}

	nonHopHeaders := []string{
		"Content-Type", "Authorization", "X-Request-ID",
		"Accept", "User-Agent", "",
	}
	for _, h := range nonHopHeaders {
		assert.False(t, isHopHeader(h), "expected non-hop header: %s", h)
	}
}

// =============================================================================
// NewAuthenticatedProxyServer tests
// =============================================================================

// TestNewAuthenticatedProxyServer verifies that NewAuthenticatedProxyServer
// creates a proxy server with token authentication middleware.
func TestNewAuthenticatedProxyServer(t *testing.T) {
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")

	tm := NewTokenManager()
	tm.AddToken("test-token-123")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	ps := NewAuthenticatedProxyServer(socketPath, handler, tm)
	require.NotNil(t, ps)
	assert.Equal(t, tm, ps.tokenManager)
	assert.NotNil(t, ps.handler)

	require.NoError(t, ps.Start())
	defer ps.Stop()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	// Health endpoint should be accessible without token
	resp, err := client.Get("http://unix/health")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Non-health endpoint without token should return 401
	req, _ := http.NewRequest(http.MethodGet, "http://unix/api/tasks", nil)
	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// Non-health endpoint with valid token should succeed
	req2, _ := http.NewRequest(http.MethodGet, "http://unix/api/tasks", nil)
	req2.Header.Set("Authorization", "Bearer test-token-123")
	resp, err = client.Do(req2)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

// TestNewAuthenticatedProxyServer_InvalidToken verifies that an invalid
// token is rejected with 401.
func TestNewAuthenticatedProxyServer_InvalidToken(t *testing.T) {
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")

	tm := NewTokenManager()
	tm.AddToken("valid-token")

	ps := NewAuthenticatedProxyServer(socketPath, nil, tm)
	require.NoError(t, ps.Start())
	defer ps.Stop()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	req, _ := http.NewRequest(http.MethodGet, "http://unix/api/tasks", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

// TestNewAuthenticatedProxyServer_NilHandler verifies that a nil handler
// falls back to the default handler.
func TestNewAuthenticatedProxyServer_NilHandler(t *testing.T) {
	socketDir := socketTestDir(t)
	socketPath := filepath.Join(socketDir, "muster.sock")

	tm := NewTokenManager()

	ps := NewAuthenticatedProxyServer(socketPath, nil, tm)
	require.NotNil(t, ps)
	assert.NotNil(t, ps.handler)

	require.NoError(t, ps.Start())
	defer ps.Stop()

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	// Health endpoint accessible without token on nil handler too
	resp, err := client.Get("http://unix/health")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}

// =============================================================================
// SetupSignalHandlers tests
// =============================================================================

// TestSetupSignalHandlers_NilController verifies that SetupSignalHandlers
// does not panic when called on a nil controller.
func TestSetupSignalHandlers_NilController(t *testing.T) {
	var c *Controller
	assert.NotPanics(t, func() {
		c.SetupSignalHandlers()
	})
}

// TestSetupSignalHandlers_NonNilController verifies that SetupSignalHandlers
// does not panic and is a no-op on a valid controller.
func TestSetupSignalHandlers_NonNilController(t *testing.T) {
	cfg := ControllerConfig{
		PidFilePath: "/tmp/muster-test.pid",
		SocketPath:  "/tmp/muster-test.sock",
		Version:     "1.0.0",
	}
	ctrl, err := NewController(cfg)
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		ctrl.SetupSignalHandlers()
	})

	// Verify no side effects — controller state is unchanged
	assert.False(t, ctrl.running)
	assert.Equal(t, "/tmp/muster-test.sock", ctrl.socketPath)
}

// TestSetupSignalHandlers_Idempotent verifies that calling SetupSignalHandlers
// multiple times is safe and does not change state.
func TestSetupSignalHandlers_Idempotent(t *testing.T) {
	cfg := ControllerConfig{
		PidFilePath: "/tmp/muster-test.pid",
		SocketPath:  "/tmp/muster-test.sock",
	}
	ctrl, err := NewController(cfg)
	require.NoError(t, err)

	ctrl.SetupSignalHandlers()
	ctrl.SetupSignalHandlers()
	ctrl.SetupSignalHandlers()

	assert.False(t, ctrl.running)
}

// =============================================================================
// mockCache implements cache.Cache for testing NewProxyHandlerWithCache.
// =============================================================================

type mockCache struct {
	getCalled    bool
	setCalled    bool
	deleteCalled bool
	clearCalled  bool
	closeCalled  bool
	statsCalled  bool
}

func (m *mockCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	m.getCalled = true
	return nil, false, nil
}

func (m *mockCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	m.setCalled = true
	return nil
}

func (m *mockCache) Delete(ctx context.Context, key string) error {
	m.deleteCalled = true
	return nil
}

func (m *mockCache) Clear(ctx context.Context) error {
	m.clearCalled = true
	return nil
}

func (m *mockCache) Close() error {
	m.closeCalled = true
	return nil
}

func (m *mockCache) Stats() cache.Stats {
	m.statsCalled = true
	return cache.Stats{}
}

var _ cache.Cache = (*mockCache)(nil)

// =============================================================================
// NewProxyHandlerWithCache tests
// =============================================================================

// TestNewProxyHandlerWithCache_Valid verifies that NewProxyHandlerWithCache
// returns a properly configured ProxyHandler with the given backendURL, cache,
// and TTL.
func TestNewProxyHandlerWithCache_Valid(t *testing.T) {
	backendURL := "http://example.com/api"
	c := &mockCache{}
	ttl := 5 * time.Minute

	handler := NewProxyHandlerWithCache(backendURL, c, ttl)

	require.NotNil(t, handler)
	assert.Equal(t, backendURL, handler.backendURL)
	assert.True(t, handler.forwardedFor)
	assert.NotNil(t, handler.httpClient)
	assert.Equal(t, 30*time.Second, handler.httpClient.Timeout)
	assert.NotNil(t, handler.httpClient.Transport)
}

// TestNewProxyHandlerWithCache_NilCache verifies that NewProxyHandlerWithCache
// handles a nil cache (passes it through to NewCacheRoundTripper).
func TestNewProxyHandlerWithCache_NilCache(t *testing.T) {
	backendURL := "http://example.com/api"

	handler := NewProxyHandlerWithCache(backendURL, nil, 1*time.Minute)

	require.NotNil(t, handler)
	assert.Equal(t, backendURL, handler.backendURL)
	assert.NotNil(t, handler.httpClient)
}

// TestNewProxyHandlerWithCache_ZeroTTL verifies that a zero TTL is accepted.
func TestNewProxyHandlerWithCache_ZeroTTL(t *testing.T) {
	backendURL := "http://localhost:8080"
	c := &mockCache{}

	handler := NewProxyHandlerWithCache(backendURL, c, 0)

	require.NotNil(t, handler)
	assert.Equal(t, backendURL, handler.backendURL)
	assert.NotNil(t, handler.httpClient)
}

// TestNewProxyHandlerWithCache_LargeTTL verifies that a large TTL is accepted.
func TestNewProxyHandlerWithCache_LargeTTL(t *testing.T) {
	backendURL := "http://backend.internal"
	c := &mockCache{}

	handler := NewProxyHandlerWithCache(backendURL, c, 24*time.Hour)

	require.NotNil(t, handler)
	assert.Equal(t, backendURL, handler.backendURL)
}

// TestNewProxyHandlerWithCache_EmptyBackendURL verifies that an empty backend
// URL is accepted by the constructor (validation happens at ServeHTTP time).
func TestNewProxyHandlerWithCache_EmptyBackendURL(t *testing.T) {
	c := &mockCache{}

	handler := NewProxyHandlerWithCache("", c, time.Minute)

	require.NotNil(t, handler)
	assert.Equal(t, "", handler.backendURL)
	assert.NotNil(t, handler.httpClient)
}
