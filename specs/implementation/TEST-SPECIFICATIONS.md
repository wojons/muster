# Test Specifications

> STATUS: HISTORICAL — the paths referenced here come from an early layout that was never
> built (`internal/core/inventory`, `internal/core/http`, and the `test/` tree it
> sketches). Tests live beside each package as `*_test.go`; the end-to-end gate is
> `demo/run-all.sh` with the `Makefile` test targets. Treat the paths below as design-time
> labels, not real paths.

## Overview

Complete testing strategy including unit tests, integration tests, end-to-end tests, and test utilities.

## 1. Test Structure

```
openapi-cli/
├── internal/
│   └── ...
├── test/
│   ├── unit/                    # Unit tests
│   │   ├── core/
│   │   │   ├── inventory_test.go
│   │   │   ├── http_test.go
│   │   │   ├── auth_test.go
│   │   │   └── ...
│   │   └── ...
│   ├── integration/             # Integration tests
│   │   ├── api_test.go
│   │   ├── mcp_test.go
│   │   └── daemon_test.go
│   ├── e2e/                     # End-to-end tests
│   │   └── scenarios/
│   ├── fixtures/                # Test fixtures
│   │   ├── openapi/
│   │   ├── inventories/
│   │   └── wasm/
│   └── helpers/                 # Test helpers
│       └── ...
└── mocks/                       # Generated mocks
    └── ...
```

## 2. Unit Tests

### Inventory Tests

```go
// test/unit/core/inventory_test.go

package inventory_test

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
    
    "github.com/openapi-cli/core/internal/core/inventory"
    "github.com/openapi-cli/core/internal/core/http"
)

// MockHTTPClient implements HTTPClient for testing
type MockHTTPClient struct {
    mock.Mock
}

func (m *MockHTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
    args := m.Called(ctx, req)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*http.Response), args.Error(1)
}

func TestManager_Fetch_Success(t *testing.T) {
    // Setup
    mockHTTP := new(MockHTTPClient)
    mockHTTP.On("Do", mock.Anything, mock.MatchedBy(func(req *http.Request) bool {
        return req.URL == "https://api.example.com/_inventory.json"
    })).Return(&http.Response{
        StatusCode: 200,
        Body:       []byte(inventoryJSON),
        Headers:    map[string]string{"ETag": "abc123"},
    }, nil)
    
    store := new(MockStore)
    store.On("SaveInventory", mock.Anything, mock.Anything).Return(nil)
    
    manager, err := inventory.NewManager(inventory.Options{
        HTTPClient: mockHTTP,
        Store:      store,
    })
    require.NoError(t, err)
    
    // Execute
    ctx := context.Background()
    inv, err := manager.Fetch(ctx, "https://api.example.com", inventory.FetchOptions{})
    
    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, inv)
    assert.Equal(t, "1.2.3", inv.Version)
    
    mockHTTP.AssertExpectations(t)
    store.AssertExpectations(t)
}

func TestManager_Fetch_CacheHit(t *testing.T) {
    // Setup - cache has valid inventory
    cachedInv := &inventory.Inventory{
        Host:       "https://api.example.com",
        Version:    "1.2.3",
        Content:    json.RawMessage(inventoryJSON),
        SHA256:     "abc123",
        CachedAt:   time.Now(),
        CacheUntil: time.Now().Add(1 * time.Hour),
    }
    
    store := new(MockStore)
    store.On("GetCached", mock.Anything, "https://api.example.com", "1.2.3").Return(cachedInv, nil)
    
    manager, err := inventory.NewManager(inventory.Options{
        Store: store,
    })
    require.NoError(t, err)
    
    // Execute
    ctx := context.Background()
    inv, err := manager.Fetch(ctx, "https://api.example.com", inventory.FetchOptions{
        Version: "1.2.3",
    })
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, "1.2.3", inv.Version)
    // HTTP client should not be called
}

func TestManager_Fetch_ExpiredCache(t *testing.T) {
    // Setup - cache expired
    expiredInv := &inventory.Inventory{
        Host:       "https://api.example.com",
        Version:    "1.2.3",
        CacheUntil: time.Now().Add(-1 * time.Hour), // Expired
    }
    
    store := new(MockStore)
    store.On("GetCached", mock.Anything, mock.Anything, mock.Anything).Return(expiredInv, nil)
    
    mockHTTP := new(MockHTTPClient)
    mockHTTP.On("Do", mock.Anything, mock.Anything).Return(&http.Response{
        StatusCode: 200,
        Body:       []byte(inventoryJSON),
    }, nil)
    
    manager, _ := inventory.NewManager(inventory.Options{
        HTTPClient: mockHTTP,
        Store:      store,
    })
    
    // Execute
    ctx := context.Background()
    _, err := manager.Fetch(ctx, "https://api.example.com", inventory.FetchOptions{})
    
    // Assert - should fetch new version
    assert.NoError(t, err)
    mockHTTP.AssertCalled(t, "Do", mock.Anything, mock.Anything)
}

func TestManager_Fetch_NetworkError(t *testing.T) {
    // Setup
    mockHTTP := new(MockHTTPClient)
    mockHTTP.On("Do", mock.Anything, mock.Anything).Return(nil, errors.New("network error"))
    
    store := new(MockStore)
    store.On("GetCached", mock.Anything, mock.Anything, mock.Anything).Return(nil, db.ErrNotFound)
    
    manager, _ := inventory.NewManager(inventory.Options{
        HTTPClient: mockHTTP,
        Store:      store,
    })
    
    // Execute
    ctx := context.Background()
    _, err := manager.Fetch(ctx, "https://api.example.com", inventory.FetchOptions{})
    
    // Assert - should fail since no cache and network down
    assert.Error(t, err)
    assert.IsType(t, &inventory.InventoryError{}, err)
    assert.True(t, err.(OpenAPICliError).IsRetryable())
}

func TestManager_MarkGood(t *testing.T) {
    // Setup
    store := new(MockStore)
    store.On("MarkGood", mock.Anything, "https://api.example.com", "1.2.3").Return(nil)
    
    manager, _ := inventory.NewManager(inventory.Options{
        Store: store,
    })
    
    // Execute
    ctx := context.Background()
    err := manager.MarkGood(ctx, "https://api.example.com", "1.2.3")
    
    // Assert
    assert.NoError(t, err)
    store.AssertExpectations(t)
}

// Table-driven tests for version parsing
func TestInventoryVersionParsing(t *testing.T) {
    tests := []struct {
        name     string
        version  string
        expected semver.Version
        valid    bool
    }{
        {"simple", "1.2.3", semver.Version{Major: 1, Minor: 2, Patch: 3}, true},
        {"with v", "v1.2.3", semver.Version{Major: 1, Minor: 2, Patch: 3}, true},
        {"prerelease", "1.2.3-beta.1", semver.Version{Major: 1, Minor: 2, Patch: 3, Prerelease: "beta.1"}, true},
        {"invalid", "not-a-version", semver.Version{}, false},
        {"empty", "", semver.Version{}, false},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            v, err := semver.Parse(tt.version)
            if tt.valid {
                assert.NoError(t, err)
                assert.Equal(t, tt.expected, v)
            } else {
                assert.Error(t, err)
            }
        })
    }
}
```

### HTTP Client Tests

```go
// test/unit/core/http_test.go

func TestClient_Retry_Success(t *testing.T) {
    // Setup - first request fails, second succeeds
    callCount := 0
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        callCount++
        if callCount == 1 {
            w.WriteHeader(http.StatusInternalServerError)
            return
        }
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`{"success": true}`))
    }))
    defer server.Close()
    
    client := http.NewClient(http.Options{
        Retry: retry.Config{
            MaxAttempts:  3,
            InitialDelay: 1 * time.Millisecond,
        },
    })
    
    // Execute
    ctx := context.Background()
    resp, err := client.Get(ctx, server.URL+"/test", nil)
    
    // Assert
    assert.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode)
    assert.Equal(t, 2, callCount) // Retried once
}

func TestClient_RateLimit(t *testing.T) {
    // Setup
    rl := ratelimit.NewBucket(100*time.Millisecond, 2) // 2 requests per 100ms
    
    client := http.NewClient(http.Options{
        RateLimiter: rl,
    })
    
    ctx := context.Background()
    
    // First 2 requests should succeed
    _, err1 := client.Get(ctx, "http://example.com/test1", nil)
    assert.NoError(t, err1)
    
    _, err2 := client.Get(ctx, "http://example.com/test2", nil)
    assert.NoError(t, err2)
    
    // Third request should be rate limited
    _, err3 := client.Get(ctx, "http://example.com/test3", nil)
    assert.Error(t, err3)
    assert.Equal(t, ErrRateLimited, errors.Code(err3))
}

func TestClient_CircuitBreaker(t *testing.T) {
    // Setup - server always fails
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusInternalServerError)
    }))
    defer server.Close()
    
    cb := circuitbreaker.New(circuitbreaker.Config{
        FailureThreshold: 3,
        Timeout:          100 * time.Millisecond,
    })
    
    client := http.NewClient(http.Options{
        CircuitBreaker: cb,
    })
    
    ctx := context.Background()
    
    // First 3 requests fail (trip threshold)
    for i := 0; i < 3; i++ {
        _, _ = client.Get(ctx, server.URL+"/test", nil)
    }
    
    // Circuit should be open now
    assert.Equal(t, circuitbreaker.StateOpen, cb.State())
    
    // Next request should fail fast (circuit open)
    _, err := client.Get(ctx, server.URL+"/test", nil)
    assert.Error(t, err)
    assert.Equal(t, ErrCircuitOpen, errors.Code(err))
}
```

### WASM Tests

```go
// test/unit/core/wasm_test.go

func TestRuntime_LoadModule_Success(t *testing.T) {
    // Setup
    wasmBytes := wasmtest.LoadFixture("test.wasm")
    
    runtime, err := wasm.NewRuntime(wasm.Options{})
    require.NoError(t, err)
    defer runtime.Close()
    
    // Execute
    module, err := runtime.LoadModule(context.Background(), wasm.ModuleSpec{
        Name:    "test",
        Content: wasmBytes,
        SHA256:  calculateSHA256(wasmBytes),
    })
    
    // Assert
    assert.NoError(t, err)
    assert.NotNil(t, module)
}

func TestRuntime_Execute_CapabilityViolation(t *testing.T) {
    // Setup - module tries to access filesystem but not allowed
    wasmBytes := wasmtest.LoadFixture("filesystem_access.wasm")
    
    runtime, _ := wasm.NewRuntime(wasm.Options{
        Capabilities: wasm.Capability{
            Filesystem: false, // Not allowed
        },
    })
    defer runtime.Close()
    
    module, _ := runtime.LoadModule(context.Background(), wasm.ModuleSpec{
        Content: wasmBytes,
    })
    
    // Execute - should fail due to capability violation
    _, err := runtime.Execute(context.Background(), module.Hash(), "accessFile", nil)
    
    // Assert
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "capability violation")
}

func TestRuntime_Execute_Timeout(t *testing.T) {
    // Setup - module runs forever
    wasmBytes := wasmtest.LoadFixture("infinite_loop.wasm")
    
    runtime, _ := wasm.NewRuntime(wasm.Options{
        MaxExecutionTime: 100 * time.Millisecond,
    })
    defer runtime.Close()
    
    module, _ := runtime.LoadModule(context.Background(), wasm.ModuleSpec{
        Content: wasmBytes,
    })
    
    // Execute with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    _, err := runtime.Execute(ctx, module.Hash(), "run", nil)
    
    // Assert
    assert.Error(t, err)
    assert.True(t, errors.IsTimeout(err))
}
```

## 3. Integration Tests

### API Integration

```go
// test/integration/api_test.go

func TestAPI_InventoryFetch_RealServer(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }
    
    // Setup real server
    server := setupTestServer()
    defer server.Close()
    
    // Execute
    client := api.NewClient(server.URL)
    inv, err := client.FetchInventory(context.Background(), api.FetchOptions{})
    
    // Assert
    require.NoError(t, err)
    assert.NotNil(t, inv)
    assert.NotEmpty(t, inv.Version)
    assert.NotEmpty(t, inv.OpenAPI.URL)
}

func TestAPI_RevocationCheck(t *testing.T) {
    // Setup server with revoked inventory
    server := setupTestServerWithRevocation()
    defer server.Close()
    
    client := api.NewClient(server.URL)
    
    // Should detect revocation
    _, err := client.FetchInventory(context.Background(), api.FetchOptions{})
    assert.Error(t, err)
    assert.Equal(t, ErrInventoryRevoked, errors.Code(err))
}
```

### MCP Integration

```go
// test/integration/mcp_test.go

func TestMCP_Server_Lifecycle(t *testing.T) {
    // Setup MCP server
    server, err := mcp.NewServer(mcp.Options{
        Inventory: testInventory,
    })
    require.NoError(t, err)
    
    // Use stdio transport with pipes
    stdin := new(bytes.Buffer)
    stdout := new(bytes.Buffer)
    transport := mcp.NewStdioTransportWithIO(stdin, stdout)
    
    // Run server in goroutine
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    go server.Serve(ctx, transport)
    
    // Send initialize request
    initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
    stdin.WriteString(initReq + "\n")
    
    // Wait for response
    time.Sleep(100 * time.Millisecond)
    
    // Check response
    response := stdout.String()
    assert.Contains(t, response, "initialize")
    assert.Contains(t, response, "result")
}

func TestMCP_Server_ToolExecution(t *testing.T) {
    // Setup with test inventory containing "users_list" tool
    server, _ := mcp.NewServer(mcp.Options{
        Inventory: testInventoryWithUsersEndpoint,
    })
    
    transport := setupStdioTransport()
    ctx := context.Background()
    
    go server.Serve(ctx, transport)
    
    // Call users_list tool
    callReq := `{
        "jsonrpc":"2.0",
        "id":2,
        "method":"tools/call",
        "params":{
            "name":"users_list",
            "arguments":{"limit":10}
        }
    }`
    transport.Send(callReq)
    
    // Get response
    response := transport.Receive()
    
    // Assert
    assert.Contains(t, response, "result")
    assert.NotContains(t, response, "error")
}
```

## 4. End-to-End Tests

```go
// test/e2e/scenarios/basic_usage_test.go

func TestE2E_BasicWorkflow(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping e2e test")
    }
    
    // Build CLI binary
    cliPath := buildCLIBinary(t)
    
    // Start test API server
    apiServer := setupTestAPIServer()
    defer apiServer.Close()
    
    // Test scenarios
    t.Run("fetch_inventory", func(t *testing.T) {
        cmd := exec.Command(cliPath, "--host", apiServer.URL, "--help")
        output, err := cmd.CombinedOutput()
        
        assert.NoError(t, err)
        assert.Contains(t, string(output), "users")
        assert.Contains(t, string(output), "posts")
    })
    
    t.Run("list_users", func(t *testing.T) {
        cmd := exec.Command(cliPath, "--host", apiServer.URL, "users", "list", "--limit", "5")
        output, err := cmd.CombinedOutput()
        
        assert.NoError(t, err)
        // Verify JSON output
        var users []map[string]interface{}
        err = json.Unmarshal(output, &users)
        assert.NoError(t, err)
        assert.Len(t, users, 5)
    })
    
    t.Run("offline_mode", func(t *testing.T) {
        // Kill API server
        apiServer.Close()
        
        // Should work with cached data
        cmd := exec.Command(cliPath, "--host", apiServer.URL, "--offline", "users", "list")
        output, err := cmd.CombinedOutput()
        
        assert.NoError(t, err)
        assert.Contains(t, string(output), "user")
    })
}

func TestE2E_CacheManagement(t *testing.T) {
    cliPath := buildCLIBinary(t)
    apiServer := setupTestAPIServer()
    defer apiServer.Close()
    
    // Populate cache
    exec.Command(cliPath, "--host", apiServer.URL, "users", "list").Run()
    
    // Check cache info
    cmd := exec.Command(cliPath, "cache", "info")
    output, err := cmd.CombinedOutput()
    assert.NoError(t, err)
    assert.Contains(t, string(output), "Inventory Versions: 1")
    
    // Clear cache
    cmd = exec.Command(cliPath, "cache", "clear")
    output, err = cmd.CombinedOutput()
    assert.NoError(t, err)
    assert.Contains(t, string(output), "Cache cleared")
    
    // Verify cleared
    cmd = exec.Command(cliPath, "cache", "info")
    output, _ = cmd.CombinedOutput()
    assert.Contains(t, string(output), "Inventory Versions: 0")
}
```

## 5. Test Fixtures

```go
// test/fixtures/inventories.go

package fixtures

// SampleInventory returns a test inventory
func SampleInventory() *inventory.Inventory {
    return &inventory.Inventory{
        Version:       "1.2.3",
        MinCLIVersion: "1.0.0",
        MaxCLIVersion: "2.0.0",
        OpenAPI: inventory.OpenAPISource{
            Source: "embedded",
            Content: []byte(sampleOpenAPISpec()),
        },
        Config: inventory.SystemConfig{
            RateLimiting: inventory.RateLimitConfig{
                RequestsPerMin: 100,
            },
        },
    }
}

func sampleOpenAPISpec() string {
    return `{
        "openapi": "3.0.0",
        "info": {
            "title": "Test API",
            "version": "1.0.0"
        },
        "paths": {
            "/users": {
                "get": {
                    "operationId": "users_list",
                    "summary": "List users",
                    "parameters": [
                        {
                            "name": "limit",
                            "in": "query",
                            "schema": {"type": "integer", "default": 10}
                        }
                    ]
                }
            }
        }
    }`
}
```

## 6. Benchmarks

```go
// test/benchmark/performance_test.go

func BenchmarkInventoryFetch_Cached(b *testing.B) {
    manager := setupCachedInventoryManager()
    ctx := context.Background()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := manager.GetCached(ctx, "https://api.example.com", "1.2.3")
        if err != nil {
            b.Fatal(err)
        }
    }
}

func BenchmarkWASMExecution(b *testing.B) {
    runtime, _ := wasm.NewRuntime(wasm.Options{})
    module, _ := runtime.LoadModule(context.Background(), wasm.ModuleSpec{
        Content: wasmtest.LoadFixture("benchmark.wasm"),
    })
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = runtime.Execute(context.Background(), module.Hash(), "process", []byte(`{"data": "test"}`))
    }
}

func BenchmarkHTTPClient_Retry(b *testing.B) {
    client := http.NewClient(http.Options{
        Retry: retry.Config{
            MaxAttempts: 3,
        },
    })
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = client.Get(context.Background(), "http://localhost:8080/test", nil)
    }
}
```

## 7. Test Commands

```makefile
# Makefile.test

.PHONY: test test-unit test-integration test-e2e test-all test-coverage

test-unit:
	go test -v -short ./test/unit/...

test-integration:
	go test -v ./test/integration/...

test-e2e:
	go test -v ./test/e2e/...

test-all:
	go test -v ./...

test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

test-coverage-ci:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out | grep total | awk '{print $3}'

benchmark:
	go test -bench=. -benchmem ./test/benchmark/...
```

---

## Deterministic Guarantees

This test specification must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic Test Execution

Test execution must produce identical results across platforms:

- **Test ordering**: Deterministic test execution order (alphabetical by package, then test name)
- **Random number generation**: Seeded RNG with deterministic sequences for test fixtures
- **Timing tests**: UTC‑based timing with deterministic thresholds

### 2. Platform‑Independent Test Fixtures

Test fixtures must be platform‑independent:

- **File paths**: Use `NormalizePath()` from `PLATFORM-BEHAVIOR.md`
- **Environment variables**: Use `GetCanonicalEnv()` from `PLATFORM-BEHAVIOR.md`
- **Temporary directories**: Platform‑appropriate temp directories with deterministic naming

### 3. Deterministic Test Assertions

Test assertions must be deterministic:

- **Floating‑point comparisons**: Epsilon‑based comparisons with platform‑appropriate tolerances
- **JSON comparisons**: Sorted key comparison for deterministic output
- **Error message matching**: Deterministic error message formatting

### 4. Cross‑Platform Test Coverage

Test coverage must be consistent across platforms:

- **Coverage measurement**: Identical coverage calculation methodology
- **Race detection**: Same race detection semantics across platforms
- **Benchmark comparisons**: Platform‑normalized benchmark results

### 5. Testing Determinism Itself

The test suite must include tests for determinism:

```go
//go:build linux || darwin || windows
func TestDeterminism(t *testing.T) {
	// Verify test execution determinism
	// Verify fixture determinism
	// Verify assertion determinism
}
```

### 6. Integration with SDK Generation

When generating SDKs for 12 languages, test specifications must:

- Produce identical test coverage
- Implement identical test fixtures
- Enforce identical assertion patterns

**Verification**: Run `./scripts/validate‑determinism‑tests.sh` to confirm cross‑platform consistency.

This test specification provides comprehensive unit, integration, and end-to-end tests with fixtures and benchmarks for Ralph Loop.
