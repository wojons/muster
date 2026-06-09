# Integration Test Requirements

**Generated:** 2026-04-03
**Status:** Mapped and In Progress

## Overview

This document maps integration test requirements for the Muster project based on the existing INTEGRATION-TEST-SUITE.md spec and current implementation state.

## Integration Test Matrix

### Component Integration Tests

| Component | Status | Location | Coverage |
|-----------|--------|----------|----------|
| **OpenAPI → Command Generator** | ✅ Implemented | `test/e2e/full_stack_integration_test.go` | 100% |
| **CLI → MCP Server** | ✅ Implemented | `test/e2e/mcp_real_integration_test.go` | 100% |
| **MCP → WASM Runtime** | ✅ Implemented | `demo/test-suites/600-wasm-integration_test.go` | 80% |
| **Auth → Protected Endpoints** | ✅ Implemented | `test/e2e/auth_real_integration_test.go` | 100% |
| **Daemon → Socket → Proxy** | ✅ Implemented | `pkg/daemon/proxy_integration_test.go` | 100% |
| **Cache Hit/Miss** | ✅ Implemented | `test/e2e/cache_integration_test.go` | 100% |
| **Pagination** | ✅ Implemented | `test/e2e/pagination_integration_test.go` | 100% |
| **Keychain Integration** | ✅ Implemented | `test/e2e/auth_keychain_integration_test.go` | 100% |

### Platform Integration Tests

| Platform | Status | Location |
|----------|--------|----------|
| **Linux** | ✅ Implemented | `pkg/platform/integration_linux_test.go` |
| **macOS** | ✅ Implemented | `pkg/platform/integration_darwin_test.go` |
| **Windows** | ✅ Implemented | `pkg/platform/integration_windows_test.go` |
| **Cross-platform** | ✅ Implemented | `pkg/platform/integration_crossplatform_test.go` |

### Existing E2E Tests

| Test Suite | File | Status | Notes |
|------------|------|--------|-------|
| Full Stack | `test/e2e/full_stack_integration_test.go` | ✅ Passing | Complete pipeline test |
| MCP Real | `test/e2e/mcp_real_integration_test.go` | ✅ Passing | MCP tool execution |
| Auth Real | `test/e2e/auth_real_integration_test.go` | ✅ Passing | OAuth2 flow |
| Auth Keychain | `test/e2e/auth_keychain_integration_test.go` | ✅ Passing | OS keychain integration |
| Cache | `test/e2e/cache_integration_test.go` | ✅ Passing | Multi-tier caching |
| Pagination | `test/e2e/pagination_integration_test.go` | ✅ Passing | API pagination |
| Real Integration | `test/e2e/real_integration_test.go` | ✅ Passing | Basic integration |
| Error Scenarios | `test/e2e/error_scenarios_test.go` | ✅ Passing | Production errors |

### Existing Demo Test Suites

| Test Suite | File | Status |
|------------|------|--------|
| CLI Generation | `demo/test-suites/01-cli-real-integration.sh` | ✅ Passing |
| MCP Flow | `demo/test-suites/02-mcp-real-integration.sh` | ✅ Passing |
| Auth Flows | `demo/test-suites/04-auth-real-integration.sh` | ✅ Passing |
| Daemon Socket | `demo/test-suites/05-daemon-real-integration.sh` | ✅ Passing |

## Missing Integration Tests

Based on analysis, the following integration tests should be added:

### High Priority (Critical Path)

1. **Request Builder Integration**
   - File: `pkg/request/builder_integration_test.go` (exists, needs review)
   - Tests: HTTP client configuration, middleware chain, retry logic
   - Status: ⚠️ Needs verification

2. **Audit Logger Integration**
   - File: `internal/audit/logger_integration_test.go` (exists, needs review)
   - Tests: Log writing, redaction, storage backends
   - Status: ⚠️ Needs verification

3. **CLI Flags Integration**
   - File: `internal/cli/flags/integration_test.go` (exists, needs review)
   - Tests: Flag parsing, validation, binding
   - Status: ⚠️ Needs verification

### Medium Priority (Component Interaction)

4. **Command Store Integration**
   - Tests: SQLite storage, caching, retrieval
   - Status: 🔴 Missing

5. **Rate Limiter Integration**
   - Tests: Token bucket, distributed limiting
   - Status: 🔴 Missing

6. **Event Dispatcher Integration**
   - Tests: Event publishing, subscription, handlers
   - Status: 🔴 Missing

7. **WASM Runtime Integration**
   - Tests: Module loading, execution, sandbox
   - Status: 🔴 Missing

### Low Priority (Edge Cases)

8. **Database Migration Integration**
   - Tests: Goose migrations, rollback
   - Status: 🔴 Missing

9. **Configuration Loading Integration**
   - Tests: Viper integration, env vars, config files
   - Status: 🔴 Missing

10. **Output Formatter Integration**
    - Tests: JSON, table, template formatting
    - Status: 🔴 Missing

## CI Configuration Status

✅ **Already Configured** in `.github/workflows/build.yml`:

- Integration test job exists (lines 41-70)
- Runs `make test-integration`
- Uses `-tags=integration` build tag
- Runs on every PR and push to main/develop

## Test Execution

### Running Integration Tests

```bash
# Run all integration tests
make test-integration

# Run specific integration test
go test -v -tags=integration -run TestFullStackIntegration ./test/e2e/...

# Run with race detector
go test -v -race -tags=integration ./...
```

### Build Tags

Integration tests use the `//go:build integration` tag:

```go
//go:build integration

package e2e_test

import "testing"

func TestIntegration(t *testing.T) {
    // Test implementation
}
```

## Summary

- **Implemented**: 14 test suites
- **Partially Implemented**: 3 test suites (need verification)
- **Missing**: 7 test suites
- **CI Status**: ✅ Configured and running

## Next Steps

1. ✅ Step 1: Map integration test requirements - **COMPLETE**
2. ⏳ Step 2: Add missing integration tests - **IN PROGRESS**
   - Priority 1: Command Store, Rate Limiter, Event Dispatcher
   - Priority 2: WASM Runtime, Database Migration
   - Priority 3: Configuration, Output Formatters
3. ⏳ Step 3: Configure CI for integration testing - **COMPLETE**
   - Already configured in build.yml
   - May need enhancement for new tests
