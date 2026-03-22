# Ralph Wiggum Loop - Task Queue (Baby Steps)

## Status: 78% Complete | Target: Production Ready

---

## 🔴 P0: BLOCKING PRODUCTION (Execute First)

### SEC-001: OAuth2 State Validation (CRITICAL)
- [ ] Add state parameter validation in Exchange() method
- [ ] Compare returned state with stored state
- [ ] Reject requests with mismatched state
- [ ] Add unit tests for state validation
- [ ] **Location:** pkg/auth/oauth2.go:94-128

### SEC-002: Remove Weak Master Key Fallback (CRITICAL)
- [ ] Remove fallback key "muster-fallback-master-key-" + runtime.GOOS
- [ ] Fail closed instead of using weak fallback
- [ ] Add proper error message for missing master key
- [ ] Add unit tests
- [ ] **Location:** pkg/auth/keychain.go:288

### SEC-003: Daemon Socket Authentication (CRITICAL)
- [ ] Add token-based authentication for Unix socket
- [ ] Create token file with restricted permissions
- [ ] Validate token on each request
- [ ] Log unauthorized access attempts
- [ ] **Location:** pkg/daemon/proxy.go, cmd/daemon/main.go

### SEC-004: HTTP Response Body Error Handling (CRITICAL)
- [ ] Handle io.ReadAll errors in pkg/dsl/builtins.go
- [ ] Add size validation before read
- [ ] Return proper errors instead of ignoring
- [ ] Fix 4 locations: lines 112, 169, 226, 282

---

## 🟠 P1: HIGH PRIORITY

### IMPL-002: HTTP Retry with Backoff (2-3 days) ✅ COMPLETED
- [x] Create pkg/retry/doc.go
- [x] Create pkg/retry/retrier.go with Retrier struct
- [x] Create pkg/retry/config.go with RetryConfig
- [x] Create pkg/retry/backoff.go with exponential backoff
- [x] Create pkg/retry/jitter.go with jitter implementations
- [x] Implement Do(ctx, fn func() error) error
- [x] Add unit tests
- [x] Create pkg/retry/errors.go with error types
- [x] All tests passing (21 tests)
- [ ] Integrate with pkg/client/client.go (deferred - pkg/client empty)

### IMPL-003: CLI Flag Parsing (3-4 days)
- [ ] Create internal/cli/flags/doc.go
- [ ] Create internal/cli/flags/parser.go
- [ ] Create internal/cli/flags/validators.go
- [ ] Implement URLValidator
- [ ] Implement IPValidator
- [ ] Implement EmailValidator
- [ ] Implement PathValidator
- [ ] Add unit tests

### IMPL-004: Command Aliases (2-3 days)
- [ ] Create internal/alias/doc.go
- [ ] Create internal/alias/manager.go with Manager struct
- [ ] Create internal/alias/expander.go with recursive detection
- [ ] Create internal/alias/storage.go with YAML persistence
- [ ] Implement Define(), Remove(), Get(), List(), Expand()
- [ ] Add unit tests

### IMPL-005: Event Handler Registry (2-3 days)
- [ ] Create pkg/events/doc.go
- [ ] Create pkg/events/registry.go with HandlerRegistry
- [ ] Create pkg/events/emitter.go with Emitter
- [ ] Create pkg/events/middleware.go with chain support
- [ ] Implement Register/Unregister/GetHandlers/Execute
- [ ] Add unit tests

### IMPL-006: Daemon Config Hot Reload (2-3 days)
- [ ] Create internal/daemon/config/doc.go
- [ ] Create internal/daemon/config/manager.go
- [ ] Create internal/daemon/config/watcher.go with fsnotify
- [ ] Implement Load(), Save(), Watch(), ReloadCh
- [ ] Add unit tests

---

## 🟡 P2: MEDIUM PRIORITY

### FEAT-001: MCP Roots Protocol
- [ ] Create pkg/mcp/roots.go with RootManager
- [ ] Implement SetRoots(), GetRoots(), AddRoot(), RemoveRoot()
- [ ] Implement isWithinRoots() validation
- [ ] Add roots/list handler

### FEAT-002: HTTP Canonicalization
- [ ] Create pkg/httputil/canonical.go
- [ ] Implement CanonicalizeURL()
- [ ] Implement CanonicalizeHeaders()
- [ ] Add unit tests

### FEAT-003: Viper Advanced Integration
- [ ] Add viper.BindPFlag() for CLI flags
- [ ] Implement WatchConfig() for hot reload
- [ ] Implement SetProfile() for profile switching

### AUDIT-002: Distributed Tracing
- [ ] Create internal/telemetry/doc.go
- [ ] Create internal/telemetry/tracer.go with Tracer interface
- [ ] Create internal/telemetry/span.go with Span struct
- [ ] Create internal/telemetry/otel.go with OpenTelemetry
- [ ] Add tracing to MCP, WASM, HTTP client
- [ ] Add unit tests

---

## 🟢 P3: TEST COVERAGE (0% Files)

### TEST-001: Core Package Tests
- [ ] Create pkg/cmdstore/store_test.go
- [ ] Create pkg/cache/memory_test.go
- [ ] Create pkg/config/loader_test.go
- [ ] Create pkg/daemon/controller_test.go
- [ ] Create pkg/inventory/manager_test.go

### TEST-002: MCP Package Tests
- [ ] Create pkg/mcp/converter_test.go
- [ ] Create pkg/mcp/handlers/prompts_test.go
- [ ] Create pkg/mcp/handlers/tools_test.go
- [ ] Create pkg/mcp/handlers/lifecycle_test.go

### TEST-003: Builtin Commands Tests
- [ ] Create internal/builtin/generate_test.go
- [ ] Create internal/builtin/config_test.go
- [ ] Create internal/builtin/auth_test.go
- [ ] Create internal/builtin/daemon_test.go

---

## 🔵 P4: PLATFORM INTEGRATION

### PLAT-001: Platform Security Stubs
- [ ] Option A: Complete seccomp BPF filter implementation
- [ ] Option B: Remove stubs and document as future work
- [ ] Either implement or remove pkg/platform/seccomp.go
- [ ] Either implement or remove pkg/platform/selinux.go
- [ ] Either implement or remove pkg/platform/networkmanager.go

### PLAT-002: Keychain Integration
- [ ] Implement SecretServiceKeychain with D-Bus
- [ ] Implement WindowsKeychain with wincred
- [ ] Add unit tests for all platforms

---

## 📊 SUMMARY

| Priority | Tasks | Effort |
|----------|-------|--------|
| P0 CRITICAL | 4 | 2-3 days |
| P1 HIGH | 6 | 12-18 days |
| P2 MEDIUM | 4 | 5-8 days |
| P3 TESTS | 3 | 3-5 days |
| P4 PLATFORM | 2 | 3-5 days |
| **TOTAL** | **19** | **25-39 days** |

---

## Execution Order

1. SEC-001 → SEC-002 → SEC-003 → SEC-004 (Security Critical)
2. IMPL-002 → IMPL-003 → IMPL-004 → IMPL-005 → IMPL-006 (Features)
3. FEAT-001 → FEAT-002 → FEAT-003 → AUDIT-002 (Medium)
4. TEST-001 → TEST-002 → TEST-003 (Coverage)
5. PLAT-001 → PLAT-002 (Platform)

---

## Current Status

- **Iteration:** 1702
- **Pending Tasks:** 50+
- **Completion:** 78%
- **Last Updated:** 2026-03-22
