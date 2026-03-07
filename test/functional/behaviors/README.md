# Behavior Tests

This directory contains tests for system behavior under various conditions.

## What are Behavior Tests?

Behavior tests verify how the system behaves under different conditions, including:
- Caching behavior
- Error recovery
- Concurrency
- Rate limiting
- Resource management

## Categories

### Caching Behavior
Tests for cache hit/miss, eviction, and consistency.

### Error Recovery
Tests for graceful error handling and fallback mechanisms.

### Concurrency
Tests for race conditions and thread safety.

### Rate Limiting
Tests for rate limiting and throttling behavior.

## Guidelines

1. Test REAL behavior (not mocked)
2. Use actual system components
3. Verify side effects occur
4. Test edge cases and boundary conditions
5. Ensure cleanup of resources
6. No flaky tests (ensure deterministic behavior)

## Example

```go
func TestSystem_CachingBehavior(t *testing.T) {
    // First request (cache miss)
    result1 := makeRequest("get", "/api/pets")
    assert.False(t, result1.fromCache)
    
    // Second request (cache hit)
    result2 := makeRequest("get", "/api/pets")
    assert.True(t, result2.fromCache)
}
```
