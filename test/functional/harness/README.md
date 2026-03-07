# Test Harness Infrastructure

This directory provides the core test harness infrastructure for functional testing.

## Components

### system.go
Provides System struct for managing the complete system under test.

**Usage**:
```go
system := harness.NewSystem(t)
defer system.Cleanup(t)

err := system.Initialize(t)
require.NoError(t, err)
require.True(t, system.IsReady())
```

### state.go
Provides State struct for tracking system state during tests.

**Usage**:
```go
state := harness.NewState()
state.SetInitialized(true)
state.SetReady(true)
state.SetMetric("commands_executed", 5)
```

### assertions.go
Provides custom assertion functions for functional testing.

**Usage**:
```go
harness.AssertEqual(t, "status", "ready", system.GetState())
harness.AssertTrue(t, "is_ready", system.IsReady())
harness.AssertContains(t, "commands", commandNames, "list")
```

### fixtures.go
Provides test fixtures and mock servers for testing.

**Usage**:
```go
server := harness.NewMockServer(t, func(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte(`{"status":"ok"}`))
})

ctx := harness.NewTestContext(t)
```

## Guidelines

1. Always use system.Cleanup() for resource cleanup
2. Always call NewSystem(t) in tests (requires t.Helper())
3. Use custom assertions for better error messages
4. Use mock servers instead of real external services
5. Context should be created with NewTestContext for proper cleanup
