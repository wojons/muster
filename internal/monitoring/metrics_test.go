package monitoring

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
)

func nopLogger() *zerolog.Logger {
	l := zerolog.Nop()
	return &l
}

func TestNewMetricRecorder(t *testing.T) {
	logger := nopLogger()
	mr := NewMetricRecorder("tenant-1", logger)

	if mr == nil {
		t.Fatal("NewMetricRecorder returned nil")
	}
	if mr.tenantID != "tenant-1" {
		t.Errorf("tenantID = %q, want %q", mr.tenantID, "tenant-1")
	}
	if mr.registry == nil {
		t.Error("registry is nil")
	}
	if mr.logger == nil {
		t.Error("logger is nil")
	}
}

func TestMetricRecorder_RecordCommandExecution(t *testing.T) {
	mr := NewMetricRecorder("t1", nopLogger())

	// Should not panic
	mr.RecordCommandExecution("discover", "success", 150*time.Millisecond)
	mr.RecordCommandExecution("generate", "error", 0)

	// Verify metrics were recorded by gathering
	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	var foundExec, foundDur bool
	for _, mf := range metrics {
		switch mf.GetName() {
		case "openapi_cli_command_executions_total":
			foundExec = true
			if len(mf.GetMetric()) == 0 {
				t.Error("command_executions_total has no metrics")
			}
		case "openapi_cli_command_duration_seconds":
			foundDur = true
			if len(mf.GetMetric()) == 0 {
				t.Error("command_duration_seconds has no metrics")
			}
		}
	}
	if !foundExec {
		t.Error("command_executions_total not found in gathered metrics")
	}
	if !foundDur {
		t.Error("command_duration_seconds not found in gathered metrics")
	}
}

func TestMetricRecorder_RecordCommandError(t *testing.T) {
	mr := NewMetricRecorder("t1", nopLogger())

	mr.RecordCommandError("discover", "network_timeout")
	mr.RecordCommandError("generate", "parse_error")

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "openapi_cli_command_errors_total" {
			found = true
			if len(mf.GetMetric()) == 0 {
				t.Error("command_errors_total has no metrics")
			}
		}
	}
	if !found {
		t.Error("command_errors_total not found in gathered metrics")
	}
}

func TestMetricRecorder_RecordMCPRequest(t *testing.T) {
	mr := NewMetricRecorder("t1", nopLogger())

	mr.RecordMCPRequest("tools/list", 25*time.Millisecond)
	mr.RecordMCPRequest("tools/call", 100*time.Millisecond)

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	var foundReq, foundDur bool
	for _, mf := range metrics {
		switch mf.GetName() {
		case "openapi_mcp_requests_total":
			foundReq = true
		case "openapi_mcp_request_duration_seconds":
			foundDur = true
		}
	}
	if !foundReq {
		t.Error("mcp_requests_total not found")
	}
	if !foundDur {
		t.Error("mcp_request_duration_seconds not found")
	}
}

func TestMetricRecorder_RecordToolExecution(t *testing.T) {
	mr := NewMetricRecorder("t1", nopLogger())

	mr.RecordToolExecution("get-Pet", "success")
	mr.RecordToolExecution("delete-Pet", "error")

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "openapi_mcp_tool_executions_total" {
			found = true
			if len(mf.GetMetric()) == 0 {
				t.Error("tool_executions_total has no metrics")
			}
		}
	}
	if !found {
		t.Error("tool_executions_total not found")
	}
}

func TestMetricRecorder_RecordProxyRequest(t *testing.T) {
	mr := NewMetricRecorder("t1", nopLogger())

	mr.RecordProxyRequest("/api/v1/tasks", "GET")
	mr.RecordProxyRequest("/api/v1/tasks", "POST")

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "openapi_daemon_proxy_requests_total" {
			found = true
			if len(mf.GetMetric()) == 0 {
				t.Error("proxy_requests_total has no metrics")
			}
		}
	}
	if !found {
		t.Error("proxy_requests_total not found")
	}
}

func TestMetricRecorder_RecordCacheHit(t *testing.T) {
	mr := NewMetricRecorder("t1", nopLogger())
	mr.RecordCacheHit()
	mr.RecordCacheHit()

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "openapi_daemon_cache_hits_total" {
			found = true
			if len(mf.GetMetric()) > 0 && mf.GetMetric()[0].GetCounter().GetValue() != 2 {
				t.Errorf("cache_hits = %v, want 2", mf.GetMetric()[0].GetCounter().GetValue())
			}
		}
	}
	if !found {
		t.Error("cache_hits_total not found")
	}
}

func TestMetricRecorder_RecordCacheMiss(t *testing.T) {
	mr := NewMetricRecorder("t1", nopLogger())
	mr.RecordCacheMiss()

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "openapi_daemon_cache_misses_total" {
			found = true
			if len(mf.GetMetric()) > 0 && mf.GetMetric()[0].GetCounter().GetValue() != 1 {
				t.Errorf("cache_misses = %v, want 1", mf.GetMetric()[0].GetCounter().GetValue())
			}
		}
	}
	if !found {
		t.Error("cache_misses_total not found")
	}
}

func TestMetricRecorder_IncDecMCPConnections(t *testing.T) {
	mr := NewMetricRecorder("t1", nopLogger())

	mr.IncMCPConnections()
	mr.IncMCPConnections()
	mr.DecMCPConnections()

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "openapi_mcp_active_connections" {
			found = true
			if len(mf.GetMetric()) > 0 && mf.GetMetric()[0].GetGauge().GetValue() != 1 {
				t.Errorf("active_connections = %v, want 1", mf.GetMetric()[0].GetGauge().GetValue())
			}
		}
	}
	if !found {
		t.Error("active_connections not found")
	}
}

func TestMetricRecorder_IncDecDaemonSocketConnections(t *testing.T) {
	mr := NewMetricRecorder("t1", nopLogger())

	mr.IncDaemonSocketConnections()
	mr.IncDaemonSocketConnections()
	mr.IncDaemonSocketConnections()
	mr.DecDaemonSocketConnections()

	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "openapi_daemon_active_socket_connections" {
			found = true
			if len(mf.GetMetric()) > 0 && mf.GetMetric()[0].GetGauge().GetValue() != 2 {
				t.Errorf("active_socket_connections = %v, want 2", mf.GetMetric()[0].GetGauge().GetValue())
			}
		}
	}
	if !found {
		t.Error("active_socket_connections not found")
	}
}

func TestMetricRecorder_ConcurrentAccess(t *testing.T) {
	mr := NewMetricRecorder("t1", nopLogger())
	done := make(chan struct{})
	const goroutines = 20

	for i := 0; i < goroutines; i++ {
		go func() {
			mr.RecordCommandExecution("cmd", "success", time.Millisecond)
			mr.IncMCPConnections()
			mr.RecordCacheHit()
			mr.DecMCPConnections()
			done <- struct{}{}
		}()
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Verify no panics — metrics were recorded
	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}
	if len(metrics) == 0 {
		t.Error("no metrics gathered after concurrent access")
	}
}

func TestMetricRecorder_DifferentTenantIDs(t *testing.T) {
	mr1 := NewMetricRecorder("tenant-a", nopLogger())
	mr2 := NewMetricRecorder("tenant-b", nopLogger())

	mr1.RecordCommandExecution("discover", "success", 0)
	mr2.RecordCommandExecution("discover", "success", 0)

	// Both tenants should have recorded to the same global metrics.
	// We verify the metrics exist and have non-zero value —
	// exact count is unreliable due to shared prometheus state.
	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather failed: %v", err)
	}

	found := false
	for _, mf := range metrics {
		if mf.GetName() == "openapi_cli_command_executions_total" {
			found = true
			if len(mf.GetMetric()) == 0 {
				t.Error("command_executions_total has no metrics")
			}
		}
	}
	if !found {
		t.Error("command_executions_total not found")
	}
}

func TestNewMetricsServer(t *testing.T) {
	ms := NewMetricsServer(":0")
	if ms == nil {
		t.Fatal("NewMetricsServer returned nil")
	}
	if ms.addr != ":0" {
		t.Errorf("addr = %q, want %q", ms.addr, ":0")
	}
}

func TestMetricsServer_StartAndShutdown(t *testing.T) {
	ms := NewMetricsServer(":19999")
	if err := ms.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give server a moment to start
	time.Sleep(50 * time.Millisecond)

	// Health check
	resp, err := http.Get("http://localhost:19999/health")
	if err != nil {
		// Try again after a short delay
		time.Sleep(100 * time.Millisecond)
		resp, err = http.Get("http://localhost:19999/health")
		if err != nil {
			t.Fatalf("health endpoint unreachable: %v", err)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Metrics endpoint
	resp2, err := http.Get("http://localhost:19999/metrics")
	if err != nil {
		t.Fatalf("metrics endpoint unreachable: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("metrics status = %d, want %d", resp2.StatusCode, http.StatusOK)
	}

	// Shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := ms.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestMetricsServer_HealthEndpoints(t *testing.T) {
	ms := NewMetricsServer(":19998")
	if err := ms.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		ms.Shutdown(ctx)
	}()

	// Wait for server to be ready
	time.Sleep(50 * time.Millisecond)

	tests := []struct {
		name   string
		path   string
		want   int
	}{
		{"health", "/health", http.StatusOK},
		{"liveness", "/health/live", http.StatusOK},
		{"readiness", "/health/ready", http.StatusOK},
		{"startup", "/health/startup", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Get("http://localhost:19998" + tt.path)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				resp, err = http.Get("http://localhost:19998" + tt.path)
				if err != nil {
					t.Fatalf("%s unreachable: %v", tt.name, err)
				}
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.want {
				t.Errorf("%s status = %d, want %d", tt.name, resp.StatusCode, tt.want)
			}

			ct := resp.Header.Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("%s Content-Type = %q, want application/json", tt.name, ct)
			}
		})
	}
}

func TestMetricsServer_ShutdownNilServer(t *testing.T) {
	ms := &MetricsServer{
		addr: ":0",
	}
	ctx := context.Background()
	err := ms.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown with nil server returned error: %v", err)
	}
}

func TestNamespaceConstants(t *testing.T) {
	if NamespaceCLI != "openapi_cli" {
		t.Errorf("NamespaceCLI = %q, want openapi_cli", NamespaceCLI)
	}
	if NamespaceMCP != "openapi_mcp" {
		t.Errorf("NamespaceMCP = %q, want openapi_mcp", NamespaceMCP)
	}
	if NamespaceDaemon != "openapi_daemon" {
		t.Errorf("NamespaceDaemon = %q, want openapi_daemon", NamespaceDaemon)
	}
}

func TestGlobalMetricsRegistered(t *testing.T) {
	// Verify all global prometheus metrics are non-nil
	globals := []prometheus.Collector{
		CommandExecutions,
		CommandDuration,
		CommandErrors,
		MCPConnections,
		MCPRequests,
		MCPRequestDuration,
		MCPToolExecutions,
		DaemonActiveSockets,
		DaemonProxyRequests,
		DaemonCacheHits,
		DaemonCacheMisses,
	}

	for i, c := range globals {
		if c == nil {
			t.Errorf("global metric[%d] is nil", i)
		}
	}
}
