package wasm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// NetworkPolicy.IsAllowed tests
// =============================================================================

func TestNetworkPolicy_IsAllowed_AllowOutbound(t *testing.T) {
	policy := &NetworkPolicy{AllowOutbound: true}
	assert.True(t, policy.IsAllowed("example.com", 443))
	assert.True(t, policy.IsAllowed("any-host.org", 8080))
}

func TestNetworkPolicy_IsAllowed_BlockedHostTakesPriority(t *testing.T) {
	policy := &NetworkPolicy{
		AllowOutbound: true,
		BlockedHosts:  []string{"evil.com"},
	}
	assert.False(t, policy.IsAllowed("evil.com", 443), "blocked host denied even with AllowOutbound")
	assert.True(t, policy.IsAllowed("good.com", 443))
}

func TestNetworkPolicy_IsAllowed_Allowlist(t *testing.T) {
	policy := &NetworkPolicy{
		AllowedHosts: []string{"api.example.com"},
		AllowedPorts: []int{443, 8443},
	}

	assert.True(t, policy.IsAllowed("api.example.com", 443))
	assert.True(t, policy.IsAllowed("api.example.com", 8443))
	assert.False(t, policy.IsAllowed("api.example.com", 80), "port not in allowlist")
	assert.False(t, policy.IsAllowed("other.example.com", 443), "host not in allowlist")
}

func TestNetworkPolicy_IsAllowed_BlockedInAllowlist(t *testing.T) {
	policy := &NetworkPolicy{
		AllowedHosts: []string{"api.example.com"},
		AllowedPorts: []int{443},
		BlockedHosts: []string{"api.example.com"},
	}
	// Blocked hosts always take priority
	assert.False(t, policy.IsAllowed("api.example.com", 443))
}

func TestNetworkPolicy_IsAllowed_EmptyPolicy(t *testing.T) {
	policy := &NetworkPolicy{}
	assert.False(t, policy.IsAllowed("example.com", 80))
	assert.False(t, policy.IsAllowed("example.com", 443))
}

func TestNetworkPolicy_IsAllowed_WildcardHost(t *testing.T) {
	// Blocked hosts are matched by exact string, not wildcard
	policy := &NetworkPolicy{
		AllowOutbound: true,
		BlockedHosts:  []string{"evil.com"},
	}
	assert.True(t, policy.IsAllowed("sub.evil.com", 443), "exact match only — subdomain not blocked")
}

// =============================================================================
// CapabilitySet tests
// =============================================================================

func TestNewCapabilitySet(t *testing.T) {
	cs := NewCapabilitySet()
	require.NotNil(t, cs)
	assert.NotNil(t, cs.caps)
	assert.Empty(t, cs.caps)
}

func TestCapabilitySet_Grant(t *testing.T) {
	cs := NewCapabilitySet()
	err := cs.Grant("network.http")
	require.NoError(t, err)
	assert.True(t, cs.caps["network.http"])
}

func TestCapabilitySet_Grant_Duplicate(t *testing.T) {
	cs := NewCapabilitySet()
	require.NoError(t, cs.Grant("network.http"))
	require.NoError(t, cs.Grant("network.http"))
	assert.True(t, cs.caps["network.http"])
}

func TestCapabilitySet_Grant_Multiple(t *testing.T) {
	cs := NewCapabilitySet()
	require.NoError(t, cs.Grant("network.http"))
	require.NoError(t, cs.Grant("file.read"))
	assert.Len(t, cs.caps, 2)
	assert.True(t, cs.caps["network.http"])
	assert.True(t, cs.caps["file.read"])
}

func TestCapabilitySet_Revoke(t *testing.T) {
	cs := NewCapabilitySet()
	require.NoError(t, cs.Grant("network.http"))
	require.NoError(t, cs.Grant("file.read"))

	err := cs.Revoke("network.http")
	require.NoError(t, err)
	assert.False(t, cs.caps["network.http"])
	assert.True(t, cs.caps["file.read"])
}

func TestCapabilitySet_Revoke_NonExistent(t *testing.T) {
	cs := NewCapabilitySet()
	err := cs.Revoke("nonexistent")
	require.NoError(t, err) // revoking non-existent is not an error
}

func TestCapabilitySet_Check(t *testing.T) {
	cs := NewCapabilitySet()
	require.NoError(t, cs.Grant("network.http"))

	assert.True(t, cs.Check("network.http"))
	assert.False(t, cs.Check("file.read"))
}

func TestCapabilitySet_Check_Wildcard(t *testing.T) {
	cs := NewCapabilitySet()
	require.NoError(t, cs.Grant("file./data/*"))

	assert.True(t, cs.Check("file./data/test.txt"))
	assert.True(t, cs.Check("file./data/sub/deep.json"))
	assert.False(t, cs.Check("file./other/file.txt"))
}

func TestCapabilitySet_Check_NilSet(t *testing.T) {
	var cs *CapabilitySet
	assert.False(t, cs.Check("anything"))
}

// =============================================================================
// Runtime.CheckNetworkAccess tests
// =============================================================================

func TestRuntime_CheckNetworkAccess_NilPolicy(t *testing.T) {
	r := &Runtime{} // networkPolicy is nil
	assert.True(t, r.CheckNetworkAccess("example.com", 443))
	assert.True(t, r.CheckNetworkAccess("any-host.org", 80))
}

func TestRuntime_CheckNetworkAccess_BlockedHost(t *testing.T) {
	r := &Runtime{
		networkPolicy: &NetworkPolicy{
			BlockedHosts: []string{"evil.com"},
		},
	}
	assert.False(t, r.CheckNetworkAccess("evil.com", 443))
	assert.True(t, r.CheckNetworkAccess("good.com", 443))
}

func TestRuntime_CheckNetworkAccess_NoAllowedHosts(t *testing.T) {
	// When AllowedHosts is empty but BlockedHosts is not, non-blocked hosts are allowed
	r := &Runtime{
		networkPolicy: &NetworkPolicy{
			BlockedHosts: []string{"evil.com"},
		},
	}
	assert.True(t, r.CheckNetworkAccess("good.com", 443))
}

func TestRuntime_CheckNetworkAccess_EmptyAllowedPorts(t *testing.T) {
	// When AllowedPorts is empty, any port on allowed hosts passes
	r := &Runtime{
		networkPolicy: &NetworkPolicy{
			AllowedHosts: []string{"api.example.com"},
		},
	}
	assert.True(t, r.CheckNetworkAccess("api.example.com", 443))
	assert.True(t, r.CheckNetworkAccess("api.example.com", 8080))
	assert.False(t, r.CheckNetworkAccess("other.com", 443))
}

func TestRuntime_CheckNetworkAccess_WithAllowedPorts(t *testing.T) {
	r := &Runtime{
		networkPolicy: &NetworkPolicy{
			AllowedHosts: []string{"api.example.com"},
			AllowedPorts: []int{443},
		},
	}
	assert.True(t, r.CheckNetworkAccess("api.example.com", 443))
	assert.False(t, r.CheckNetworkAccess("api.example.com", 80))
}

// =============================================================================
// isValidCapabilityName tests
// =============================================================================

func TestIsValidCapabilityName(t *testing.T) {
	tests := []struct {
		name    string
		valid   bool
	}{
		{"", false},
		{"network", true},
		{"network.http", true},
		{"file.read", true},
		{"FILE.READ", true},
		{"file./data/*", true},
		{"cap-with-dashes", true},
		{"underscore_case", true},
		{"colon:separated", true},
		{"mixed.Case-123", true},
		{"spaces not allowed", false},
		{"special!char", false},
		{"backtick`char", false},
		{"emoji🚀", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidCapabilityName(tt.name))
		})
	}
}

// =============================================================================
// CompileWAT tests
// =============================================================================

func TestCompileWAT(t *testing.T) {
	wat := `(module)`
	result, err := CompileWAT(context.Background(), wat)
	require.NoError(t, err)
	// CompileWAT is a stub that returns WAT text as-is
	assert.Equal(t, []byte(wat), result)
}

func TestCompileWAT_Empty(t *testing.T) {
	result, err := CompileWAT(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, []byte(""), result)
}

// =============================================================================
// Runtime.closeWithContext tests
// =============================================================================

func TestRuntime_CloseWithContext(t *testing.T) {
	// Runtime with nil underlying wazero runtime
	r := &Runtime{}
	err := r.closeWithContext(context.Background())
	require.NoError(t, err)
}

func TestRuntime_Close(t *testing.T) {
	r := &Runtime{}
	err := r.Close()
	require.NoError(t, err)
}

// =============================================================================
// NewRuntimeWithTracer tests
// =============================================================================

func TestNewRuntimeWithTracer(t *testing.T) {
	r := NewRuntimeWithTracer(nil, DefaultLimits(), nil)
	require.NotNil(t, r)

	// With nil tracer
	r2 := NewRuntimeWithTracer(DefaultRuntimeConfig(), nil, nil)
	require.NotNil(t, r2)
}

// =============================================================================
// captureResponseWriter.Flush tests
// =============================================================================

func TestCaptureResponseWriter_Flush_EmptyPlugin(t *testing.T) {
	// Flush with empty plugin path passes through
	interceptor := NewInterceptor()
	w := httptest.NewRecorder()
	crw := &captureResponseWriter{
		ResponseWriter: w,
		pluginPath:     "",
		interceptor:    interceptor,
		buf:            []byte(`{"hello":"world"}`),
		statusCode:     200,
	}

	err := crw.Flush()
	require.NoError(t, err)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, `{"hello":"world"}`, w.Body.String())
}

func TestCaptureResponseWriter_Flush_DefaultStatusCode(t *testing.T) {
	// When statusCode is 0, defaults to 200
	interceptor := NewInterceptor()
	w := httptest.NewRecorder()
	crw := &captureResponseWriter{
		ResponseWriter: w,
		pluginPath:     "",
		interceptor:    interceptor,
		buf:            []byte(`{"ok":true}`),
		statusCode:     0,
	}

	err := crw.Flush()
	require.NoError(t, err)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, `{"ok":true}`, w.Body.String())
}

func TestCaptureResponseWriter_Flush_Non200Status(t *testing.T) {
	interceptor := NewInterceptor()
	w := httptest.NewRecorder()
	crw := &captureResponseWriter{
		ResponseWriter: w,
		pluginPath:     "",
		interceptor:    interceptor,
		buf:            []byte(`{"error":"not found"}`),
		statusCode:     404,
	}

	err := crw.Flush()
	require.NoError(t, err)
	assert.Equal(t, 404, w.Code)
	assert.Equal(t, `{"error":"not found"}`, w.Body.String())
}

func TestCaptureResponseWriter_Flush_EmptyBody(t *testing.T) {
	// Empty body with empty plugin passes through
	interceptor := NewInterceptor()
	w := httptest.NewRecorder()
	crw := &captureResponseWriter{
		ResponseWriter: w,
		pluginPath:     "",
		interceptor:    interceptor,
		buf:            nil,
		statusCode:     200,
	}

	err := crw.Flush()
	require.NoError(t, err)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "", w.Body.String())
}

func TestCaptureResponseWriter_WriteHeader_Internal(t *testing.T) {
	w := httptest.NewRecorder()
	crw := &captureResponseWriter{ResponseWriter: w}
	crw.WriteHeader(201)
	assert.Equal(t, 201, crw.statusCode)
	assert.NotEqual(t, 201, w.Code, "WriteHeader on captureResponseWriter should capture, not write through")
}

func TestCaptureResponseWriter_Write_Buffering(t *testing.T) {
	w := httptest.NewRecorder()
	crw := &captureResponseWriter{ResponseWriter: w}
	n, err := crw.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(crw.buf))
	assert.Empty(t, w.Body.String(), "Write should buffer, not write through")

	// Second write appends
	n, err = crw.Write([]byte(" world"))
	require.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, "hello world", string(crw.buf))
}

// =============================================================================
// Interceptor tests (without WASM modules)
// =============================================================================

func TestInterceptor_UnloadPlugin_NotLoaded(t *testing.T) {
	interceptor := NewInterceptor()
	// Unloading a plugin that was never loaded returns an error
	err := interceptor.UnloadPlugin(context.Background(), "/nonexistent.wasm")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not loaded")
}

func TestInterceptor_RegisteredPlugins_Empty(t *testing.T) {
	interceptor := NewInterceptor()
	plugins := interceptor.RegisteredPlugins()
	assert.Empty(t, plugins)
}

func TestInterceptor_Close_NoPlugins(t *testing.T) {
	interceptor := NewInterceptor()
	err := interceptor.Close(context.Background())
	require.NoError(t, err)
}

// =============================================================================
// SandboxLimits tests (additional edge cases)
// =============================================================================

func TestSandboxLimits_Validate_AllValid(t *testing.T) {
	l := &SandboxLimits{
		MaxMemory:            256 * 1024 * 1024,
		MaxExecutionTime:     5000000000, // 5s
		MaxFileSize:          1048576,
		MaxFileDescriptors:   16,
		MaxConcurrentThreads: 4,
		MaxFuel:              1000000,
	}
	err := l.Validate()
	require.NoError(t, err)
}

func TestSandboxLimits_Validate_MissingField(t *testing.T) {
	tests := []struct {
		name   string
		limits *SandboxLimits
	}{
		{"zero memory", &SandboxLimits{MaxExecutionTime: 1, MaxFileSize: 1, MaxFileDescriptors: 1, MaxFuel: 1}},
		{"zero fuel", &SandboxLimits{MaxMemory: 1, MaxExecutionTime: 1, MaxFileSize: 1, MaxFileDescriptors: 1}},
		{"zero execution time", &SandboxLimits{MaxMemory: 1, MaxFileSize: 1, MaxFileDescriptors: 1, MaxFuel: 1}},
		{"zero file size", &SandboxLimits{MaxMemory: 1, MaxExecutionTime: 1, MaxFileDescriptors: 1, MaxFuel: 1}},
		{"zero file descriptors", &SandboxLimits{MaxMemory: 1, MaxExecutionTime: 1, MaxFileSize: 1, MaxFuel: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.limits.Validate()
			assert.Error(t, err)
		})
	}
}

// =============================================================================
// Interceptor.Middleware tests (without WASM modules)
// =============================================================================

func TestInterceptor_Middleware_EmptyPluginPath(t *testing.T) {
	interceptor := NewInterceptor()
	handler := interceptor.Middleware("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ok")
}

func TestInterceptor_Middleware_UnloadedPlugin(t *testing.T) {
	interceptor := NewInterceptor()
	handler := interceptor.Middleware("/not-loaded.wasm")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pass-through"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "pass-through", rec.Body.String())
}

// =============================================================================
// isJSONContentType tests (exported from middleware_test.go, adding edge cases)
// =============================================================================

func TestIsJSONContentType_EdgeCases(t *testing.T) {
	assert.True(t, isJSONContentType("application/json"))
	assert.True(t, isJSONContentType("application/ld+json"))
	assert.True(t, isJSONContentType("application/problem+json"))
	assert.False(t, isJSONContentType("text/plain"))
	assert.False(t, isJSONContentType(""))
	assert.False(t, isJSONContentType("application/xml"))
}
