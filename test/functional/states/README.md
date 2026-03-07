# State Tests

This directory contains tests for system state transitions.

## What are State Tests?

State tests verify that the system transitions correctly between different states throughout its lifecycle.

## System States

### Available States
- `stopped` - System not running
- `initializing` - System starting up
- `ready` - System ready to handle requests
- `error` - System in error state
- `stopping` - System shutting down

## State Transitions to Test

### Initialization
```
stopped → initializing → ready
stopped → initializing → error
```

### Normal Operation
```
ready → executing (command running)
executing → ready (command done)
ready → error (command failed)
error → ready (recovered)
```

### Shutdown
```
ready → stopping → stopped
error → stopping → stopped
```

## Guidelines

1. Test ALL state transitions
2. Verify invalid transitions fail
3. Test error states and recovery
4. Ensure cleanup on all transitions
5. Race conditions should be covered

## Example

```go
func TestSystem_StateTransitions(t *testing.T) {
    state := harness.NewState()
    
    // Start stopped
    assert.Equal(t, "stopped", state.GetState())
    
    // Initialize
    state.SetInitialized(true)
    assert.Equal(t, "initializing", state.GetState())
    
    // Ready after init
    state.SetReady(true)
    assert.Equal(t, "ready", state.GetState())
    
    // Error state
    state.SetError(errors.New("test"))
    assert.Equal(t, "error", state.GetState())
}
```
