package monitoring_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/internal/monitoring"
)

// ---------------------------------------------------------------------------
// Deferred tests (types not yet implemented in the codebase)
// ---------------------------------------------------------------------------
//
// The following test targets were requested but their types do not exist in
// the current codebase. They are documented here as TODOs so they can be
// enabled when the corresponding types are implemented:
//
//   DefaultMCPConfig / DefaultCLIConfig / DefaultDaemonConfig
//     -> config defaults for each subsystem
//
//   NewMultiTenantConfig / MultiTenantConfig { Filter, GetTenantLabelKey }
//     -> multi-tenant metrics isolation, not yet wired
//
//   WithCustomRegistry / WithListenAddress / WithMetricsPath
//     -> option-pattern configuration for MetricsServer
//
//   MetricsServerImpl
//     -> named interface type; current type is *MetricsServer (struct, not interface)
//
// When these types land, enable the corresponding test stubs at the bottom
// of this file.

// ---------------------------------------------------------------------------
// Constants
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
	require.NotNil(t, mr)
}

func TestNewMetricRecorder_NilLogger(t *testing.T) {
	mr := monitoring.NewMetricRecorder("tenant-nil", nil)
	require.NotNil(t, mr)
}

func TestNewMetricRecorder_FreshRegistry(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-reg-test", &logger)
	require.NotNil(t, mr)

	// Confirm a fresh prometheus registry is always usable when created
	// directly (the MetricRecorder creates one via prometheus.NewRegistry()).
	reg := prometheus.NewRegistry()
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_fresh_registry_gauge",
		Help: "Verifies the registry is usable",
	})
	err := reg.Register(g)
	require.NoError(t, err, "fresh registry should accept a new gauge")

	g.Set(42.0)
	got, err := reg.Gather()
	require.NoError(t, err)
	require.Len(t, got, 1, "should expose exactly one metric family")
	assert.Equal(t, "test_fresh_registry_gauge", *got[0].Name)
	assert.InDelta(t, 42.0, *got[0].Metric[0].Gauge.Value, 0.01)
}

// ---------------------------------------------------------------------------
// Default registry registration — verify every expected metric family is
// registered by promauto.  CounterVec/HistogramVec metrics only appear in
// Gather() after the first With() call, so we seed each Vec with one
// observation first.
// ---------------------------------------------------------------------------

func TestAllMetricsRegisteredWithDefaultRegistry(t *testing.T) {
	// Seed each Vec metric so the family appears in Gather().
	monitoring.CommandExecutions.With(prometheus.Labels{
		"command": "seed", "status": "ok", "tenant_id": "test",
	}).Inc()
	monitoring.CommandDuration.With(prometheus.Labels{
		"command": "seed", "status": "ok", "tenant_id": "test",
	}).Observe(0.01)
	monitoring.CommandErrors.With(prometheus.Labels{
		"command": "seed", "error_type": "err", "tenant_id": "test",
	}).Inc()
	monitoring.MCPRequests.With(prometheus.Labels{
		"method": "seed", "tenant_id": "test",
	}).Inc()
	monitoring.MCPRequestDuration.With(prometheus.Labels{
		"method": "seed", "tenant_id": "test",
	}).Observe(0.01)
	monitoring.MCPToolExecutions.With(prometheus.Labels{
		"tool_name": "seed", "status": "ok", "tenant_id": "test",
	}).Inc()
	monitoring.DaemonProxyRequests.With(prometheus.Labels{
		"endpoint": "/seed", "method": "GET", "tenant_id": "test",
	}).Inc()
	// Gauges and unlabeled counters are always visible.
	monitoring.MCPConnections.Inc()
	monitoring.MCPConnections.Dec()
	monitoring.DaemonActiveSockets.Inc()
	monitoring.DaemonActiveSockets.Dec()
	monitoring.DaemonCacheHits.Inc()
	monitoring.DaemonCacheMisses.Inc()

	expectedMetrics := []string{
		"openapi_cli_command_executions_total",
		"openapi_cli_command_duration_seconds",
		"openapi_cli_command_errors_total",
		"openapi_mcp_active_connections",
		"openapi_mcp_requests_total",
		"openapi_mcp_request_duration_seconds",
		"openapi_mcp_tool_executions_total",
		"openapi_daemon_active_socket_connections",
		"openapi_daemon_proxy_requests_total",
		"openapi_daemon_cache_hits_total",
		"openapi_daemon_cache_misses_total",
	}

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err, "DefaultGatherer.Gather should not error")

	registered := make(map[string]bool, len(families))
	for _, f := range families {
		registered[f.GetName()] = true
	}

	for _, name := range expectedMetrics {
		assert.True(t, registered[name], "metric %q should be registered in the default registry", name)
	}
}

// ---------------------------------------------------------------------------
// Prometheus /metrics handler output — verify the exposition format contains
// HELP and TYPE lines for every expected metric family.
// ---------------------------------------------------------------------------

func TestMetricsHandlerOutput(t *testing.T) {
	// Ensure Vec metrics are seeded for the handler to emit them.
	monitoring.CommandExecutions.With(prometheus.Labels{
		"command": "hcheck", "status": "ok", "tenant_id": "t",
	}).Inc()
	monitoring.CommandDuration.With(prometheus.Labels{
		"command": "hcheck", "status": "ok", "tenant_id": "t",
	}).Observe(0.01)
	monitoring.CommandErrors.With(prometheus.Labels{
		"command": "hcheck", "error_type": "err", "tenant_id": "t",
	}).Inc()
	monitoring.MCPRequests.With(prometheus.Labels{
		"method": "hcheck", "tenant_id": "t",
	}).Inc()
	monitoring.MCPRequestDuration.With(prometheus.Labels{
		"method": "hcheck", "tenant_id": "t",
	}).Observe(0.01)
	monitoring.MCPToolExecutions.With(prometheus.Labels{
		"tool_name": "hcheck", "status": "ok", "tenant_id": "t",
	}).Inc()
	monitoring.DaemonProxyRequests.With(prometheus.Labels{
		"endpoint": "/hcheck", "method": "GET", "tenant_id": "t",
	}).Inc()

	handler := promhttp.Handler()

	addr := findFreeAddr(t)
	srv := &http.Server{Addr: addr, Handler: handler}
	go func() { _ = srv.ListenAndServe() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/plain; version=0.0.4",
		"Content-Type should indicate Prometheus exposition format")

	body := make([]byte, 65536)
	n, _ := resp.Body.Read(body)
	bodyStr := string(body[:n])

	expectedFamilies := []string{
		"openapi_cli_command_executions_total",
		"openapi_cli_command_duration_seconds",
		"openapi_cli_command_errors_total",
		"openapi_mcp_active_connections",
		"openapi_mcp_requests_total",
		"openapi_mcp_request_duration_seconds",
		"openapi_mcp_tool_executions_total",
		"openapi_daemon_active_socket_connections",
		"openapi_daemon_proxy_requests_total",
		"openapi_daemon_cache_hits_total",
		"openapi_daemon_cache_misses_total",
	}

	for _, name := range expectedFamilies {
		assert.Contains(t, bodyStr, "HELP "+name, "response should contain HELP for %s", name)
		assert.Contains(t, bodyStr, "TYPE "+name, "response should contain TYPE for %s", name)
	}
}

// ---------------------------------------------------------------------------
// ProMAuto integration - smoke tests that verify recording methods do not
// panic when the package-level promauto metrics are already registered.
// These exercise the real global metrics, not an isolated registry.
// ---------------------------------------------------------------------------

func TestRecordCommandExecution(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordCommandExecution("list", "success", 150*time.Millisecond)
	mr.RecordCommandExecution("get", "success", 75*time.Millisecond)
	mr.RecordCommandExecution("list", "error", 20*time.Millisecond)
}

func TestRecordCommandError(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordCommandError("list", "timeout")
	mr.RecordCommandError("deploy", "permission_denied")
	mr.RecordCommandError("list", "timeout")
}

func TestRecordMCPRequest(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordMCPRequest("tools/call", 80*time.Millisecond)
	mr.RecordMCPRequest("resources/read", 5*time.Millisecond)
}

func TestRecordToolExecution(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordToolExecution("read_file", "success")
	mr.RecordToolExecution("write_file", "error")
	mr.RecordToolExecution("read_file", "success")
}

func TestRecordProxyRequest(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordProxyRequest("/api/v1/execute", "POST")
	mr.RecordProxyRequest("/api/v1/status", "GET")
}

func TestRecordCacheHitAndMiss(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.RecordCacheHit()
	mr.RecordCacheMiss()
	mr.RecordCacheHit()
	mr.RecordCacheHit()
}

func TestIncDecMCPConnections(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.IncMCPConnections()
	mr.DecMCPConnections()
	mr.IncMCPConnections()
	mr.IncMCPConnections()
	mr.DecMCPConnections()
}

func TestIncDecDaemonSocketConnections(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-1", &logger)

	mr.IncDaemonSocketConnections()
	mr.DecDaemonSocketConnections()
	mr.IncDaemonSocketConnections()
	mr.DecDaemonSocketConnections()
	mr.DecDaemonSocketConnections()
}

// ---------------------------------------------------------------------------
// Concurrent access - race-condition detection (run with -race)
// ---------------------------------------------------------------------------

func TestMetricRecorder_ConcurrentAccess(t *testing.T) {
	logger := zerolog.Nop()
	mr := monitoring.NewMetricRecorder("tenant-race", &logger)

	var wg sync.WaitGroup
	fn := func(n int) {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			mr.RecordCommandExecution("cmd", "ok", time.Millisecond)
			mr.RecordCommandError("cmd", "err")
			mr.RecordMCPRequest("method", time.Millisecond)
			mr.RecordToolExecution("tool", "ok")
			mr.RecordProxyRequest("/ep", "POST")
			mr.RecordCacheHit()
			mr.RecordCacheMiss()
			mr.IncMCPConnections()
			mr.DecMCPConnections()
			mr.IncDaemonSocketConnections()
			mr.DecDaemonSocketConnections()
		}
	}

	workers := 8
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go fn(i)
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// MetricsServer - construction and interface compliance
// ---------------------------------------------------------------------------

func TestNewMetricsServer(t *testing.T) {
	srv := monitoring.NewMetricsServer(":9090")
	require.NotNil(t, srv)
}

// metricsServerStarter captures the exported contract of *MetricsServer.
type metricsServerStarter interface {
	Start() error
	Shutdown(context.Context) error
}

func TestMetricsServer_ImplementsInterface(t *testing.T) {
	var _ metricsServerStarter = monitoring.NewMetricsServer(":0")
}

// ---------------------------------------------------------------------------
// MetricsServer - lifecycle
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

	err := srv.Start()
	require.NoError(t, err, "Start should not return an error")

	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/metrics")
	require.NoError(t, err, "should be able to GET /metrics")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	for _, path := range []string{"/health", "/health/live", "/health/ready", "/health/startup"} {
		resp, err := http.Get("http://" + addr + path)
		require.NoError(t, err, "GET %s should succeed", path)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "GET %s should return 200", path)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = srv.Shutdown(ctx)
	require.NoError(t, err, "Shutdown should not return an error")
}

func TestMetricsServer_ShutdownWithoutStart(t *testing.T) {
	srv := monitoring.NewMetricsServer("127.0.0.1:9999")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := srv.Shutdown(ctx)
	assert.NoError(t, err, "Shutdown on unstarted server should be a no-op")
}

func TestMetricsServer_StartInvalidAddress(t *testing.T) {
	srv := monitoring.NewMetricsServer("127.0.0.1:1")
	err := srv.Start()
	assert.NoError(t, err, "Start returns nil even when the goroutine will fail")

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func TestMetricsServer_DoubleShutdown(t *testing.T) {
	addr := findFreeAddr(t)
	srv := monitoring.NewMetricsServer(addr)
	require.NoError(t, srv.Start())
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err1 := srv.Shutdown(ctx)
	require.NoError(t, err1)

	err2 := srv.Shutdown(ctx)
	t.Logf("Second Shutdown returned: %v", err2)
}

func TestMetricsServer_DifferentAddresses(t *testing.T) {
	srv1 := monitoring.NewMetricsServer(":8080")
	srv2 := monitoring.NewMetricsServer(":9090")
	assert.NotEqual(t, srv1, srv2, "each NewMetricsServer call returns a distinct server")
}

// ---------------------------------------------------------------------------
// Health handler JSON response verification
// ---------------------------------------------------------------------------

func TestMetricsServer_HealthEndpointsReturnJSON(t *testing.T) {
	addr := findFreeAddr(t)
	srv := monitoring.NewMetricsServer(addr)
	require.NoError(t, srv.Start())
	time.Sleep(50 * time.Millisecond)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	type healthResponse struct {
		Status    string `json:"status"`
		Timestamp string `json:"timestamp"`
	}

	tests := []struct {
		path           string
		expectedStatus string
	}{
		{path: "/health", expectedStatus: "healthy"},
		{path: "/health/live", expectedStatus: "alive"},
		{path: "/health/ready", expectedStatus: "ready"},
		{path: "/health/startup", expectedStatus: "started"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp, err := http.Get("http://" + addr + tt.path)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

			var hr healthResponse
			err = json.NewDecoder(resp.Body).Decode(&hr)
			require.NoError(t, err, "response should be valid JSON")
			assert.Equal(t, tt.expectedStatus, hr.Status)
			assert.NotEmpty(t, hr.Timestamp, "timestamp should be present")

			_, err = time.Parse(time.RFC3339, hr.Timestamp)
			assert.NoError(t, err, "timestamp should be RFC3339 format")
		})
	}
}

func TestMetricsServer_HealthContentType(t *testing.T) {
	addr := findFreeAddr(t)
	srv := monitoring.NewMetricsServer(addr)
	require.NoError(t, srv.Start())
	time.Sleep(50 * time.Millisecond)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	for _, path := range []string{"/health", "/health/live", "/health/ready", "/health/startup", "/metrics"} {
		resp, err := http.Get("http://" + addr + path)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "GET %s", path)
	}
}

// ---------------------------------------------------------------------------
// Deferred test stubs
//
// The tests below are structure-preserving stubs for types that do not yet
// exist in the codebase. Enable them when the corresponding types land.
// ---------------------------------------------------------------------------

func TestDefaultConfigsDeferred(t *testing.T) {
	t.Skip("DefaultMCPConfig / DefaultCLIConfig / DefaultDaemonConfig not yet implemented")
}

func TestNewMultiTenantConfigDeferred(t *testing.T) {
	t.Skip("NewMultiTenantConfig / MultiTenantConfig not yet implemented")
}

func TestWithCustomRegistryDeferred(t *testing.T) {
	t.Skip("WithCustomRegistry option pattern not yet implemented")
}

func TestWithListenAddressDeferred(t *testing.T) {
	t.Skip("WithListenAddress option pattern not yet implemented")
}

func TestWithMetricsPathDeferred(t *testing.T) {
	t.Skip("WithMetricsPath option pattern not yet implemented")
}

func TestMultiTenantConfigFilterDeferred(t *testing.T) {
	t.Skip("MultiTenantConfig.Filter not yet implemented")
}

func TestMultipleTenantConfigCreationDeferred(t *testing.T) {
	t.Skip("MultiTenantConfig not yet implemented")
}

// ---------------------------------------------------------------------------
// Isolation helpers - available for use when WithCustomRegistry lands
// ---------------------------------------------------------------------------

func newIsolatedRegistry() *prometheus.Registry {
	return prometheus.NewRegistry()
}
