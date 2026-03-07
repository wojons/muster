# Functional Test Suite

This directory contains functional tests that verify REAL-WORLD SYSTEM BEHAVIOR, not just unit tests in isolation.

## Philosophy

**Unit Tests**: Individual functions in isolation
**Integration Tests**: Component connections  
**Functional Tests**: REAL-WORLD SYSTEM BEHAVIOR

## Structure

```
test/functional/
├── harness/      # System test harness infrastructure
├── workflows/    # End-to-end user workflow tests
├── states/       # System state transition tests
├── behaviors/    # System behavior under various conditions
└── README.md     # This file
```

## Requirements

Every functional test MUST:
1. Test REAL system behavior (not mocks)
2. Verify actual state changes
3. Assert side effects
4. Clean up resources
5. Be reproducible
6. Run in parallel with other tests
7. Complete in <5 seconds

Every functional test MUST NOT:
1. Use mocks for system components
2. Depend on external services
3. Modify global state
4. Leave resources allocated
5. Have race conditions
6. Be flaky

## Usage

Run all functional tests:
```bash
go test ./test/functional/... -v
```

Run specific category:
```bash
go test ./test/functional/workflows/... -v
go test ./test/functional/states/... -v
go test ./test/functional/behaviors/... -v
```

## Test Categories

### Workflow Tests
Test complete user journeys through the system.

### State Tests
Test system state transitions and initialization.

### Behavior Tests
Test system behavior under various conditions (caching, error recovery, concurrency, etc.).

## Coverage Goals

- System initialization: 100%
- User workflows: 80%+
- State transitions: 90%+
- Error scenarios: 70%+

## Writing Tests

See individual category README.md files for detailed guidance.
