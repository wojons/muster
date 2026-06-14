package monitoring_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/internal/monitoring"
)

// ---------------------------------------------------------------------------
// Const value verification
// ---------------------------------------------------------------------------

func TestNamespaces(t *testing.T) {
	tests := []struct {
		name     string
		actual   string
		expected string
	}{
		{name: "NamespaceCLI", actual: monitoring.NamespaceCLI, expected: "openapi_cli"},
		{name: "NamespaceMCP", actual: monitoring.NamespaceMCP, expected: "openapi_mcp"},
		{name: "NamespaceDaemon", actual: monitoring.NamespaceDaemon, expected: "openapi_daemon"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.actual)
		})
	}
}

// ---------------------------------------------------------------------------
// MetricRecorder construction
// ---------------------------------------------------------------------------

func TestNewMetricRecorder(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)
	require.NotNil(t, mr, "NewMetricRecorder should return a non-nil recorder")
}

func TestNewMetricRecorder_NilLogger(t *testing.T) {
	// Should not panic when given a nil logger pointer.
	mr := monitoring.NewMetricRecorder("tenant-nil", nil)
	require.NotNil(t, mr)
}

// ---------------------------------------------------------------------------
// Recording methods — verify they do not panic or deadlock
// ---------------------------------------------------------------------------

func TestRecordCommandExecution(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)
	require.NotNil(t, mr)

	// Should not panic.
	mr.RecordCommandExecution("list", "success", 150*time.Millisecond)
}

func TestRecordCommandError(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordCommandError("list", "timeout")
}

func TestRecordMCPRequest(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordMCPRequest("tools/call", 80*time.Millisecond)
}

func TestRecordToolExecution(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordToolExecution("read_file", "success")
}

func TestRecordProxyRequest(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordProxyRequest("/api/v1/execute", "POST")
}

func TestRecordCacheHitAndMiss(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordCacheHit()
	mr.RecordCacheMiss()
}

// ---------------------------------------------------------------------------
// Connection tracking — verify inc/dec symmetry (no panic, no negative)
// ---------------------------------------------------------------------------

func TestIncDecMCPConnections(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.IncMCPConnections()
	mr.DecMCPConnections()
	// Two calls: no expected state leak, just must not panic.
}

func TestIncDecDaemonSocketConnections(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.IncDaemonSocketConnections()
	mr.DecDaemonSocketConnections()
}

// ---------------------------------------------------------------------------
// MetricsServer lifecycle
// ---------------------------------------------------------------------------

func findFreeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to find free port")
	addr := l.Addr().String()
	l.Close()
	return addr
}

func TestMetricsServer_StartAndShutdown(t *testing.T) {
	addr := findFreeAddr(t)
	srv := monitoring.NewMetricsServer(addr)
	require.NotNil(t, srv)

	// Start — should return nil and bind the address.
	err := srv.Start()
	require.NoError(t, err, "Start should not return an error")

	// Give the goroutine a moment to bind.
	time.Sleep(50 * time.Millisecond)

	// Verify the /metrics endpoint responds.
	resp, err := http.Get("http://" + addr + "/metrics")
	require.NoError(t, err, "should be able to GET /metrics")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify /health endpoints respond.
	for _, path := range []string{"/health", "/health/live", "/health/ready", "/health/startup"} {
		resp, err := http.Get("http://" + addr + path)
		require.NoError(t, err, "GET %s should succeed", path)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "GET %s should return 200", path)
	}

	// Shutdown — graceful.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = srv.Shutdown(ctx)
	require.NoError(t, err, "Shutdown should not return an error")
}

func TestMetricsServer_ShutdownWithoutStart(t *testing.T) {
	// Calling Shutdown on a server that was never started should be a no-op.
	srv := monitoring.NewMetricsServer("127.0.0.1:9999")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := srv.Shutdown(ctx)
	assert.NoError(t, err, "Shutdown on unstarted server should be a no-op")
}

func TestMetricsServer_StartInvalidAddress(t *testing.T) {
	// Binding to a clearly invalid address should not block Start (it runs in a
	// goroutine and logs the error asynchronously), but Start itself returns nil.
	// We verify that the call returns without error and that the goroutine does
	// not cause a panic.
	srv := monitoring.NewMetricsServer("127.0.0.1:1") // port < 1024 — may be privileged
	err := srv.Start()
	assert.NoError(t, err, "Start returns nil even when the goroutine will fail")

	// Let the goroutine try to bind (and fail).
	time.Sleep(50 * time.Millisecond)

	// Shutdown cleanly.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}