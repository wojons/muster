# Functional Test Suite

## Overview

Functional tests verify REAL-WORLD SYSTEM BEHAVIOR, not just unit tests in isolation. These tests prove that the system works end-to-end with actual binary execution.

## Test Categories

### 1. System State Tests
Verify the system can start, initialize, and reach ready state:
- System initialization and startup
- State transitions (stopped → initializing → ready → executing)
- Daemon socket creation and binding
- Cleanup and shutdown

### 2. User Workflow Tests
Complete user journeys through the system:
- CLI workflow: Load spec → Generate commands → Execute
- MCP workflow: Start server → List tools → Execute → Response
- Auth flow: OAuth2 → Token storage → Authenticated requests
- Daemon workflow: Start → Socket connect → Proxy → Response

### 3. System Behavior Tests
Test system behaves correctly under various conditions:
- Caching behavior (cache miss → store → cache hit)
- Rate limiting behavior
- Authentication behavior
- Command generation behavior

### 4. Error Recovery Tests
Verify system recovers gracefully from errors:
- Network timeouts and failures
- Invalid requests handling
- Authentication failures
- Resource exhaustion

### 5. Concurrency Tests
Test system handles concurrent operations:
- Multiple concurrent requests
- Parallel command execution
- Concurrent state transitions
- Resource cleanup under load

### 6. State Transition Tests
Verify system state transitions correctly:
- State machine correctness
- Invalid transition prevention
- State persistence
- State restoration

## Design Principles

### Every Functional Test MUST:

1. **Test Real System Behavior** - Use actual binary execution, not mocks
2. **Verify Actual State Changes** - Assert system states, not just return values
3. **Assert Side Effects** - Verify files created, sockets bound, processes running
4. **Clean Up Resources** - All tests must clean up after themselves
5. **Be Reproducible** - Deterministic and idempotent
6. **Run in Parallel** - No interference between tests
7. **Complete Rapidly** - Each test <5 seconds

### Every Functional Test MUST NOT:

1. Use mocks for system components
2. Depend on external services
3. Modify global state
4. Leave resources allocated
5. Have race conditions
6. Be flaky

## Usage

### Run All Functional Tests

```bash
go test -v ./test/functional/...
```

### Run Specific Category

```bash
# System state tests
go test -v ./test/functional/states/...

# Workflow tests
go test -v ./test/functional/workflows/...

# Behavior tests
go test -v ./test/functional/behaviors/...
```

### Run with Coverage

```bash
go test -v -cover ./test/functional/...
```

## Harness API

The `test/functional/harness` package provides utilities:

```go
// Create a new test system
system := harness.NewSystem(t)
system.Build()
system.Start()
defer system.Stop()

// Assert system is ready
assertions := harness.NewSystemAssertions(t, system)
assertions.Ready()

// Execute CLI command
output, err := system.ExecuteCLI("list", "users")

// Make HTTP request
result := system.MakeRequest("/api/v1/users")

// Verify response
assertions.CacheHit(response)
```

## Test Structure

### System State Example

```go
func TestSystemInitialization(t *testing.T) {
    system := harness.NewSystem(t)
    system.Build()

    // System starts in stopped state
    require.Equal(t, harness.StateStopped, system.State())

    // Initialize transitions to initializing
    system.Start()
    require.Equal(t, harness.StateInitializing, system.State())

    // Wait for ready state
    require.Eventually(t, func() bool {
        return system.Ready()
    }, 5*time.Second)

    // Verify daemon is running
    require.True(t, system.IsRunning())

    // Cleanup
    system.Stop()
}
```

### User Workflow Example

```go
func TestCLIWorkflow(t *testing.T) {
    env := harness.NewTestEnvironment(t)
    system := env.Setup()
    defer env.Cleanup()

    // Step 1: Load OpenAPI spec
    output, err := system.ExecuteCLI("load", "petstore.yaml")
    require.NoError(t, err)
    require.Contains(t, output, "Loaded spec")

    // Step 2: Generate commands
    output, err = system.ExecuteCLI("generate")
    require.NoError(t, err)
    require.Contains(t, output, "Generated 15 commands")

    // Step 3: Execute command
    output, err = system.ExecuteCLI("pets", "list")
    require.NoError(t, err)
    require.NotEmpty(t, output)
}
```

## Current Coverage

- ✅ Harness infrastructure complete (system, state, assertions, fixtures)
- ⏳ System state tests (in progress)
- ⏳ User workflow tests (pending)
- ⏳ System behavior tests (pending)
- ⏳ Error recovery tests (pending)
- ⏳ Concurrency tests (pending)

## Goals

1. **Real System Testing**: All tests execute actual binaries
2. **Complete Coverage**: 100% of critical user workflows tested
3. **Fast Execution**: Full suite runs in <2 minutes
4. **Reliable**: No flaky tests, all reproducible
5. **Maintainable**: Clear, documented, easy to understand
