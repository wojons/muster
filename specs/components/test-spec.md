# Testing Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-02-28  
**Component**: Testing Strategy & Implementation  
**Referenced Files**: 9  

## Overview

Complete testing specification covering unit, integration, end‑to‑end, performance, security, and chaos engineering tests. This component ensures that all 132 specifications are validated through executable tests, achieving 90%+ test coverage and deterministic test outcomes across environments.

## 1. Testing Pyramid & Strategy

### 1.1 Testing Pyramid
```
       /\
      /  \        E2E Tests (5%)
     /----\       ~100 tests
    /      \
   /--------\     Integration Tests (15%)
  /          \    ~300 tests
 /------------\   
/              \  Unit Tests (80%)
----------------  ~2000 tests
```

**Reference**: `TESTING-STRATEGY.md` – Defines the overall test distribution, naming conventions, build tags, and coverage targets.

### 1.2 Test Organization
```
internal/
├── cache/
│   ├── cache.go
│   ├── cache_test.go           # Unit tests
│   └── cache_integration_test.go  # Integration tests
├── client/
│   ├── client.go
│   ├── client_test.go
│   └── client_integration_test.go
cmd/
├── cli/
│   ├── main.go
│   └── main_e2e_test.go        # E2E tests
└── e2e/
    └── scenarios/              # E2E test scenarios
        ├── auth_test.go
        └── api_flows_test.go
```

**Reference**: `TEST-SPECIFICATIONS.md` – Contains the complete test‑file layout and example implementations for each component.

## 2. Unit Testing

### 2.1 Framework & Conventions
- **Framework**: `github.com/stretchr/testify` (assert, require, mock)
- **Naming**: `Test<FunctionName>_<Scenario>` (e.g., `TestCache_Get_Hit`)
- **Assertions**: `assert.Equal`, `assert.NoError`, `require.NotNil`, etc.
- **Benchmarks**: `Benchmark<FunctionName>` with `testing.B`

**Reference**: `UNIT-TEST-FRAMEWORK.md` – Detailed examples of testify usage, mock generation with mockgen, and benchmark patterns.

### 2.2 Unit Test Examples
```go
func TestInventoryManager_Fetch_Success(t *testing.T) {
    mockHTTP := new(MockHTTPClient)
    mockHTTP.On("Do", mock.Anything, mock.Anything).
        Return(&http.Response{
            StatusCode: 200,
            Body:       []byte(inventoryJSON),
        }, nil)

    manager, _ := inventory.NewManager(inventory.Options{
        HTTPClient: mockHTTP,
    })
    ctx := context.Background()
    inv, err := manager.Fetch(ctx, "https://api.example.com", inventory.FetchOptions{})

    assert.NoError(t, err)
    assert.NotNil(t, inv)
    assert.Equal(t, "1.2.3", inv.Version)
}
```

**Reference**: `TEST-SPECIFICATIONS.md` – Contains ~2000 lines of example unit tests for each core component (inventory, HTTP client, authentication, caching, database, etc.).

## 3. Integration Testing

### 3.1 Integration Test Setup
- **Build tag**: `//go:build integration`
- **Global setup/teardown**: `TestMain` with database provisioning, Docker container lifecycle.
- **Test‑level isolation**: Unique database per test, ephemeral containers, network namespaces.

**Reference**: `INTEGRATION-TEST-SUITE.md` – Provides reusable utilities (`testutil.NewTestDB`, `testutil.NewRedisContainer`) and integration test patterns.

### 3.2 Database Integration Example
```go
//go:build integration

func TestUserRepository_Create(t *testing.T) {
    testDB := testutil.NewTestDB(t)
    repo := NewUserRepository(testDB.DB)

    user := &User{Name: "Alice", Email: "alice@example.com"}
    err := repo.Create(context.Background(), user)

    assert.NoError(t, err)
    assert.NotZero(t, user.ID)
}
```

### 3.3 HTTP Client Integration
```go
func TestHTTPClient_Timeout(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(2 * time.Second) // Simulate slow response
    }))
    defer server.Close()

    client := NewHTTPClient(HTTPClientOptions{Timeout: 1 * time.Second})
    _, err := client.Get(context.Background(), server.URL)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "context deadline exceeded")
}
```

## 4. End‑to‑End Testing

### 4.1 E2E Test Playbook
- **Build tag**: `//go:build e2e`
- **Binary compilation**: Build fresh CLI binary before test run.
- **Scenario‑based tests**: Complete user workflows (authentication, inventory fetch, command generation, execution).

**Reference**: `E2E-TEST-PLAYBOOK.md` – Step‑by‑step playbook for CLI, MCP server, and daemon end‑to‑end tests.

### 4.2 CLI E2E Example
```go
func TestE2E_Auth_LoginFlow(t *testing.T) {
    // Start mock OAuth2 server
    oauthServer := startMockOAuthServer(t)
    defer oauthServer.Close()

    // Run login command
    cmd := exec.Command(binaryPath, "auth", "login", "--provider", "test")
    cmd.Env = append(os.Environ(), "OAUTH2_SERVER="+oauthServer.URL)
    output, err := cmd.CombinedOutput()

    assert.NoError(t, err)
    assert.Contains(t, string(output), "Login successful")
}
```

### 4.3 MCP Server E2E Example
```go
func TestE2E_MCP_ToolRegistration(t *testing.T) {
    // Start MCP server as subprocess
    mcpCmd := exec.Command(binaryPath, "mcp", "serve")
    // Establish stdio communication, send initialize request
    // Verify tool list includes dynamically generated commands
}
```

## 5. Performance & Load Testing

### 5.1 Benchmark Suite
- **Micro‑benchmarks**: Per‑function performance with `go test -bench`.
- **Memory profiling**: Heap allocation tracking.
- **CPU profiling**: Flame‑graph generation.

**Reference**: `TEST-SPECIFICATIONS.md` – Includes benchmark examples for critical paths (OpenAPI parsing, WASM module loading, HTTP request handling).

### 5.2 Load Testing
- **Concurrent users**: Simulate 100+ concurrent CLI commands.
- **Rate‑limit verification**: Ensure token‑bucket algorithm works under load.
- **Database load**: Concurrent read/write operations with isolation.

## 6. Security Testing

### 6.1 Authentication & Authorization Tests
- **OAuth2 flow validation**: PKCE, state parameter, token refresh.
- **mTLS certificate verification**: Client certificate validation, revocation checking.
- **API key security**: Key rotation, leakage detection.

### 6.2 WASM Sandbox Security
- **Capability boundary tests**: Ensure WASM modules cannot escape sandbox.
- **Memory limit enforcement**: Detect and prevent memory exhaustion attacks.
- **System call restriction**: Block unauthorized host system calls.

**Reference**: `SECURITY-TESTING.md` (referenced in `security-spec.md`) – Detailed adversarial test cases for authentication, crypto, and sandbox security.

## 7. Chaos Engineering

### 7.1 Failure Injection
- **Network failures**: Simulate timeouts, packet loss, DNS failures.
- **Service degradation**: Slow responses, partial availability.
- **Resource exhaustion**: Memory, disk space, file‑descriptor limits.

### 7.2 Recovery Validation
- **Circuit‑breaker state transitions**: CLOSED → OPEN → HALF_OPEN → CLOSED.
- **Retry with backoff**: Exponential backoff with jitter under repeated failures.
- **Cache fallback**: Verify fallback to cached inventory when network is unavailable.

**Reference**: `RELIABILITY-RETRY.md`, `CIRCUIT-BREAKER-IMPLEMENTATION.md` – Include test scenarios for each failure mode.

## 8. Coverage Requirements

### 8.1 Minimum Coverage Targets
| Component | Unit | Integration | E2E |
|-----------|------|-------------|-----|
| `pkg/` libraries | 90% | 70% | – |
| `internal/` packages | 85% | 65% | – |
| `cmd/` binaries | 80% | 60% | 40% |
| **Overall** | **85%** | **65%** | **35%** |

### 8.2 Coverage Reporting
- **Per‑package reports**: `go tool cover -func=coverage.out`
- **HTML visualization**: `go tool cover -html=coverage.out -o coverage.html`
- **CI integration**: Fail pipeline if coverage drops below thresholds.

**Reference**: `TESTING-STRATEGY.md` – Defines coverage targets and CI enforcement.

## 9. Mocking Strategy

### 9.1 Mock Generation
- **Tool**: `github.com/golang/mock/mockgen`
- **Source‑based generation**: `//go:generate mockgen -source=...`
- **Interface coverage**: Mock all external dependencies (HTTP client, database, keychain, etc.).

### 9.2 Mock Usage Patterns
```go
ctrl := gomock.NewController(t)
defer ctrl.Finish()

mockHTTP := NewMockHTTPClient(ctrl)
mockHTTP.EXPECT().
    Do(gomock.Any(), gomock.Any()).
    Return(&http.Response{StatusCode: 200}, nil)

// Inject mock and test
```

**Reference**: `UNIT-TEST-FRAMEWORK.md` – Complete mockgen configuration and usage examples.

## 10. Test Automation

### 10.1 Makefile Targets
```makefile
test: test-unit test-integration

test-unit:
	go test -v -race -coverprofile=coverage.unit.out ./...

test-integration:
	go test -v -tags=integration -coverprofile=coverage.integration.out ./...

test-e2e:
	go test -v -tags=e2e ./cmd/e2e/...

coverage:
	go tool cover -html=coverage.unit.out -o coverage.html
```

### 10.2 CI/CD Integration
- **GitHub Actions**: Run unit, integration, and E2E tests on every push.
- **Parallel execution**: Split test suites across runners for speed.
- **Flaky‑test detection**: Retry failing tests, report intermittent failures.

**Reference**: `CI-CD-PIPELINE.md`, `GITHUB-ACTIONS-WORKFLOWS.md` – Define the complete test automation pipeline.

## 11. Assumption Validation

### 11.1 Documented Assumptions
The adversarial refinement process identified **47 explicit assumptions** (e.g., “OpenAPI spec will always be valid JSON”, “Network timeout defaults to 30 seconds”). Each assumption has a corresponding validation test.

### 11.2 Assumption Test Suite
```go
func TestAssumption_OpenAPI_ValidJSON(t *testing.T) {
    // Attempt to parse malformed JSON
    _, err := ParseOpenAPI([]byte("{invalid json}"))
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "invalid JSON")
}

func TestAssumption_NetworkTimeout_DefaultsTo30s(t *testing.T) {
    cfg := DefaultConfig()
    assert.Equal(t, 30*time.Second, cfg.NetworkTimeout)
}
```

**Reference**: `ASSUMPTION-TESTING.md` – Lists all 47 assumptions and their validation tests.

## 12. Cross‑Platform Testing

### 12.1 Platform‑Specific Tests
- **Linux**: systemd integration, AppArmor profiles, cgroup limits.
- **macOS**: Keychain access, LaunchAgent, Universal Binary.
- **Windows**: Windows Service, Credential Manager, path handling.

### 12.2 Consistency Verification
- **Deterministic outputs**: Same command produces identical output on all platforms.
- **Behavioral parity**: Authentication, file‑system operations, network stack behave identically.

**Reference**: `PLATFORM-LINUX.md`, `PLATFORM-MACOS.md`, `PLATFORM-WINDOWS.md` – Include platform‑specific test suites.

## 13. Test Data Management

### 13.1 Fixture Directory
```
test/fixtures/
├── openapi/
│   ├── github-openapi.json
│   ├── stripe-openapi.json
│   └── minimal-openapi.json
├── inventories/
│   ├── signed-inventory.json
│   └── unsigned-inventory.json
└── wasm/
    ├── validation.wasm
    └── transformation.wasm
```

### 13.2 Fixture Integrity
- **Cryptographic signatures**: Signed inventory fixtures with valid Ed25519 signatures.
- **Hash verification**: SHA‑256 checksums for WASM modules.
- **Version pinning**: Fixtures are pinned to specific OpenAPI spec versions.

**Reference**: `TEST-SPECIFICATIONS.md` – Describes fixture structure and integrity checks.

## 14. Deterministic Test Execution

### 14.1 Seed Control
- **Random number generation**: Use fixed seeds for reproducible tests.
- **Time freezing**: `time.Now()` overridden with deterministic clock in tests.
- **Network simulation**: Mock HTTP responses with exact timing.

### 14.2 Bible Standard Compliance
All tests must pass **identically** across:
- Different Go versions (1.20‑1.23)
- Different operating systems (Linux, macOS, Windows)
- Different architectures (amd64, arm64)
- Different test runners (local, CI, container)

**Reference**: `BIBLE-STANDARD-VALIDATION.md` – Validation checklist for deterministic test execution.

---

## Referenced Files

1. `TEST-SPECIFICATIONS.md` – Comprehensive test implementation (7,000+ lines)
2. `TESTING-STRATEGY.md` – Testing pyramid, naming, coverage targets
3. `UNIT-TEST-FRAMEWORK.md` – testify usage, mock generation, benchmarks
4. `INTEGRATION-TEST-SUITE.md` – Database, HTTP, container integration tests
5. `E2E-TEST-PLAYBOOK.md` – CLI, MCP server, daemon end‑to‑end scenarios
6. `SECURITY-TESTING.md` – Authentication, crypto, sandbox security tests
7. `ASSUMPTION-TESTING.md` – 47 assumption validation tests
8. `RELIABILITY-RETRY.md` – Retry logic and circuit‑breaker tests
9. `CIRCUIT-BREAKER-IMPLEMENTATION.md` – Failure‑injection test cases

---
**Total referenced lines**: ~12,000  
**Test coverage target**: 90%+  
**Deterministic execution**: ✅ Bible‑Standard compliant