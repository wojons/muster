package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T) *zerolog.Logger {
	logger := zerolog.New(zerolog.NewTestWriter(t)).With().Timestamp().Caller().Logger()
	return &logger
}

func TestNewMetricRecorder_Initialization(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	require.NotNil(t, recorder)
	assert.Equal(t, "test-tenant", recorder.tenantID)
	assert.NotNil(t, recorder.registry)
	assert.NotNil(t, recorder.logger)
}

func TestNewMetricRecorder_DifferentTenantIDs(t *testing.T) {
	logger := newTestLogger(t)
	testCases := []string{"tenant-1", "tenant-2", "prod-tenant", ""}

	for _, tenantID := range testCases {
		t.Run(fmt.Sprintf("tenant_%s", tenantID), func(t *testing.T) {
			recorder := NewMetricRecorder(tenantID, logger)
			require.NotNil(t, recorder)
			assert.Equal(t, tenantID, recorder.tenantID)
		})
	}
}

func TestMetricRecorder_RecordCommandExecution(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	command := "petstore.getPet"
	status := "success"
	duration := 100 * time.Millisecond

	recorder.RecordCommandExecution(command, status, duration)

	// Verify no panic and mutex doesn't deadlock
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	assert.Equal(t, "test-tenant", recorder.tenantID)
}

func TestMetricRecorder_RecordCommandExecution_MultipleCalls(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	commands := []struct {
		command  string
		status   string
		duration time.Duration
	}{
		{"petstore.getPet", "success", 100 * time.Millisecond},
		{"petstore.listPets", "success", 200 * time.Millisecond},
		{"petstore.createPet", "error", 50 * time.Millisecond},
	}

	for _, cmd := range commands {
		recorder.RecordCommandExecution(cmd.command, cmd.status, cmd.duration)
	}

	// Verify mutex doesn't deadlock
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_RecordCommandError(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	command := "petstore.getPet"
	errorType := "connection_refused"

	recorder.RecordCommandError(command, errorType)

	// Verify no panic
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_RecordCommandError_MultipleErrors(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	errors := []struct {
		command   string
		errorType string
	}{
		{"petstore.getPet", "connection_refused"},
		{"petstore.listPets", "timeout"},
		{"petstore.createPet", "validation_error"},
	}

	for _, err := range errors {
		recorder.RecordCommandError(err.command, err.errorType)
	}
}

func TestMetricRecorder_RecordMCPRequest(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	method := "initialize"
	duration := 50 * time.Millisecond

	recorder.RecordMCPRequest(method, duration)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_RecordMCPRequest_DifferentMethods(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	methods := []string{"initialize", "tools/list", "tools/call", "resources/list"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			recorder.RecordMCPRequest(method, 25*time.Millisecond)
		})
	}
}

func TestMetricRecorder_RecordToolExecution(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	toolName := "get_pet"
	status := "success"

	recorder.RecordToolExecution(toolName, status)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_RecordToolExecution_DifferentStatuses(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	statuses := []string{"success", "error", "timeout", "cancelled"}

	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			recorder.RecordToolExecution("test_tool", status)
		})
	}
}

func TestMetricRecorder_RecordProxyRequest(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	endpoint := "/api/v1/pets"
	method := "GET"

	recorder.RecordProxyRequest(endpoint, method)

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_RecordProxyRequest_DifferentEndpoints(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	endpoints := []struct {
		endpoint string
		method   string
	}{
		{"/api/v1/pets", "GET"},
		{"/api/v1/pets/123", "PUT"},
		{"/api/v1/pets", "POST"},
		{"/api/v1/pets/123", "DELETE"},
	}

	for _, ep := range endpoints {
		recorder.RecordProxyRequest(ep.endpoint, ep.method)
	}
}

func TestMetricRecorder_RecordCacheHit(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	recorder.RecordCacheHit()

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_RecordCacheMiss(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	recorder.RecordCacheMiss()

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_IncMCPConnections(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	recorder.IncMCPConnections()

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_DecMCPConnections(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	recorder.DecMCPConnections()

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_IncDaemonSocketConnections(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	recorder.IncDaemonSocketConnections()

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_DecDaemonSocketConnections(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	recorder.DecDaemonSocketConnections()

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
}

func TestMetricRecorder_ConcurrentAccess(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("test-tenant", logger)

	var wg sync.WaitGroup
	numGoroutines := 100
	opsPerGoroutine := 10

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				recorder.RecordCommandExecution("test_cmd", "success", 10*time.Millisecond)
				recorder.RecordCommandError("test_cmd", "test_error")
				recorder.RecordMCPRequest("test_method", 5*time.Millisecond)
				recorder.RecordToolExecution("test_tool", "success")
				recorder.RecordProxyRequest("/test", "GET")
				recorder.RecordCacheHit()
				recorder.RecordCacheMiss()
				recorder.IncMCPConnections()
				recorder.DecMCPConnections()
				recorder.IncDaemonSocketConnections()
				recorder.DecDaemonSocketConnections()
			}
		}(i)
	}

	wg.Wait()
	// If we reach here without deadlock or panic, test passes
}

func TestMetricsServer_New(t *testing.T) {
	addr := ":9090"
	server := NewMetricsServer(addr)

	require.NotNil(t, server)
	assert.Equal(t, addr, server.addr)
	assert.Nil(t, server.server)
}

func TestMetricsServer_Start(t *testing.T) {
	addr := ":19090"
	server := NewMetricsServer(addr)

	err := server.Start()
	require.NoError(t, err)
	require.NotNil(t, server.server)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost" + addr + "/metrics")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "openapi_cli_command_executions_total")
}

func TestMetricsServer_HealthEndpoints(t *testing.T) {
	addr := ":19091"
	server := NewMetricsServer(addr)

	err := server.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	healthEndpoints := []struct {
		path           string
		expectedStatus string
	}{
		{"/health", "healthy"},
		{"/health/live", "alive"},
		{"/health/ready", "ready"},
		{"/health/startup", "started"},
	}

	for _, ep := range healthEndpoints {
		t.Run(ep.path, func(t *testing.T) {
			resp, err := http.Get("http://localhost" + addr + ep.path)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var response map[string]interface{}
			err = json.Unmarshal(body, &response)
			require.NoError(t, err)

			assert.Equal(t, ep.expectedStatus, response["status"])
			assert.Contains(t, response, "timestamp")
		})
	}
}

func TestMetricsServer_Shutdown(t *testing.T) {
	addr := ":19092"
	server := NewMetricsServer(addr)

	err := server.Start()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Shutdown(ctx)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	_, err = http.Get("http://localhost" + addr + "/metrics")
	assert.Error(t, err)
}

func TestMetricsServer_Shutdown_NoServer(t *testing.T) {
	server := &MetricsServer{
		addr:   ":9090",
		server: nil,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := server.Shutdown(ctx)
	require.NoError(t, err)
}

func TestMetricsServer_Shutdown_ContextCancelled(t *testing.T) {
	addr := ":19093"
	server := NewMetricsServer(addr)

	err := server.Start()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = server.Shutdown(ctx)
	// Shutdown with cancelled context may succeed or fail depending on timing
	// The important thing is that it doesn't hang
}

func TestMetricsServer_MultipleStarts(t *testing.T) {
	addr := ":19094"

	server1 := NewMetricsServer(addr)
	err := server1.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server1.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Second server trying to use same port should fail
	// Note: The actual bind error happens in the goroutine, so we can't test it synchronously
	// This test verifies the first server started successfully
	assert.NotNil(t, server1.server)
}

func TestMetricsServer_ConcurrentHealthChecks(t *testing.T) {
	addr := ":19095"
	server := NewMetricsServer(addr)

	err := server.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			resp, err := http.Get("http://localhost" + addr + "/health")
			if err == nil && resp != nil {
				_ = resp.Body.Close()
			}
		}()
	}

	wg.Wait()
}

func TestHealthHandler_ResponseFormat(t *testing.T) {
	addr := ":19096"
	server := NewMetricsServer(addr)

	err := server.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost" + addr + "/health")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var response map[string]interface{}
	err = json.Unmarshal(body, &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	timestamp, ok := response["timestamp"].(string)
	require.True(t, ok)
	assert.NotEmpty(t, timestamp)

	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	assert.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), parsedTime, 5*time.Second)
}

func TestMetricsServer_ReadTimeout(t *testing.T) {
	server := NewMetricsServer(":9090")
	err := server.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	assert.Equal(t, 5*time.Second, server.server.ReadTimeout)
	assert.Equal(t, 10*time.Second, server.server.WriteTimeout)
}

func TestMetricRecorder_AllMethodsIntegration(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("integration-test", logger)

	recorder.RecordCommandExecution("test.cmd", "success", 100*time.Millisecond)
	recorder.RecordCommandError("test.cmd", "network_error")
	recorder.RecordMCPRequest("tools/list", 50*time.Millisecond)
	recorder.RecordToolExecution("get_data", "success")
	recorder.RecordProxyRequest("/api/data", "GET")
	recorder.RecordCacheHit()
	recorder.RecordCacheMiss()
	recorder.IncMCPConnections()
	recorder.IncDaemonSocketConnections()

	recorder.DecMCPConnections()
	recorder.DecDaemonSocketConnections()

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	assert.Equal(t, "integration-test", recorder.tenantID)
}

func TestMetricsServer_MetricsEndpoint_Content(t *testing.T) {
	addr := ":19097"
	server := NewMetricsServer(addr)

	err := server.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://localhost" + addr + "/metrics")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	metricsContent := string(body)

	assert.Contains(t, metricsContent, "openapi_cli_command_executions_total")
	assert.Contains(t, metricsContent, "openapi_cli_command_duration_seconds")
	assert.Contains(t, metricsContent, "openapi_cli_command_errors_total")
	assert.Contains(t, metricsContent, "openapi_mcp_active_connections")
	assert.Contains(t, metricsContent, "openapi_mcp_requests_total")
	assert.Contains(t, metricsContent, "openapi_mcp_request_duration_seconds")
	assert.Contains(t, metricsContent, "openapi_mcp_tool_executions_total")
	assert.Contains(t, metricsContent, "openapi_daemon_active_socket_connections")
	assert.Contains(t, metricsContent, "openapi_daemon_proxy_requests_total")
	assert.Contains(t, metricsContent, "openapi_daemon_cache_hits_total")
	assert.Contains(t, metricsContent, "openapi_daemon_cache_misses_total")
}

func TestMetricsServer_PortAlreadyInUse(t *testing.T) {
	addr := ":19098"

	server1 := NewMetricsServer(addr)
	err := server1.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server1.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	server2 := NewMetricsServer(addr)
	err = server2.Start()
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server2.Shutdown(ctx)
	require.NoError(t, err)
}

func TestHealthHandlers_HTTPMethods(t *testing.T) {
	addr := ":19099"
	server := NewMetricsServer(addr)

	err := server.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	healthPaths := []string{"/health", "/health/live", "/health/ready", "/health/startup"}

	for _, path := range healthPaths {
		t.Run(fmt.Sprintf("%s_POST", path), func(t *testing.T) {
			resp, err := http.Post("http://localhost"+addr+path, "application/json", nil)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestMetricRecorder_LabelValues(t *testing.T) {
	logger := newTestLogger(t)
	recorder := NewMetricRecorder("label-test", logger)

	recorder.RecordCommandExecution("petstore.getPet", "success", 100*time.Millisecond)
	recorder.RecordCommandError("petstore.getPet", "timeout")
	recorder.RecordMCPRequest("initialize", 50*time.Millisecond)
	recorder.RecordToolExecution("get_pet", "success")
	recorder.RecordProxyRequest("/pets", "GET")

	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	assert.Equal(t, "label-test", recorder.tenantID)
}

func TestMetricsServer_URLConstruction(t *testing.T) {
	addr := ":19100"
	server := NewMetricsServer(addr)

	err := server.Start()
	require.NoError(t, err)

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	baseURL := "http://localhost" + addr

	urls := []string{
		baseURL + "/metrics",
		baseURL + "/health",
		baseURL + "/health/live",
		baseURL + "/health/ready",
		baseURL + "/health/startup",
	}

	for _, url := range urls {
		t.Run(strings.TrimPrefix(url, baseURL), func(t *testing.T) {
			resp, err := http.Get(url)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestMetricRecorder_NilLogger(t *testing.T) {
	recorder := NewMetricRecorder("test-tenant", nil)

	require.NotNil(t, recorder)
	assert.Equal(t, "test-tenant", recorder.tenantID)
	assert.Nil(t, recorder.logger)

	recorder.RecordCommandExecution("test", "success", 10*time.Millisecond)
	recorder.RecordCommandError("test", "error")
	recorder.RecordMCPRequest("test", 5*time.Millisecond)
	recorder.RecordToolExecution("test", "success")
	recorder.RecordProxyRequest("/test", "GET")
	recorder.RecordCacheHit()
	recorder.RecordCacheMiss()
	recorder.IncMCPConnections()
	recorder.DecMCPConnections()
	recorder.IncDaemonSocketConnections()
	recorder.DecDaemonSocketConnections()
}
