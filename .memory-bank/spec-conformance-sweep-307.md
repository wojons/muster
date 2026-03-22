# Spec Conformance Sweep - Session 307

**Date**: 2026-03-12
**Session ID**: 307
**Specs Analyzed**: 3
**Cumulative Sample**: 123/227 specs (54.2%)

---

## Executive Summary

| Spec | Status | Conformance | Notes |
|------|--------|-------------|-------|
| DEPENDENCIES.md | ✅ COMPLETE | PARTIAL 70% | Missing some drivers, OAuth2, wazero |
| WASM-SANDBOX-SECURITY.md | ✅ COMPLETE | NOT IMPLEMENTED <10% | Critical security features missing |
| BUILTIN-COMMANDS.md | ✅ COMPLETE | PARTIAL 50% | Commands exist as stubs, many missing |

**Pattern**: 1 critical security spec not implemented, 2 other partial implementations

---

## Analysis Details

### 1. DEPENDENCIES.md

**Spec Requirements**:
- Go module configuration with specific dependencies
- CGO-enabled dependencies (SQLite drivers)
- Platform-specific dependencies (Linux/macOS/Windows)
- Version pinning strategy
- Build tags for database selection
- Development dependencies (golangci-lint, sqlc, mockgen, goimports, delve)
- Dependency audit schedule and checklist

**Actual Implementation**:
- go.mod has core CLI dependencies (cobra, viper)
- Makefile has deps management: deps, deps-update, tidy, vendor
- CGO enabled for SQLite (CGO_ENABLED := 1)
- Evidence:
  - `go.mod:1-85` - dependencies listed
  - `Makefile:31` - CGO configuration
  - `Makefile:427-451` - dependency management targets

**What's Implemented**:
- ✅ CLI framework: cobra v1.8.0, viper v1.18.2
- ✅ Database: mattn/go-sqlite3 v1.14.34, goose v3.27.0
- ✅ Dependencies: Makefile targets for deps, tidy, vendor
- ✅ CGO configuration for cross-platform builds
- ✅ Build system for multiple platforms (Makefile:34-40)

**What's Missing**:
- ❌ PostgreSQL driver: lib/pq v1.10.9 (not in go.mod)
- ❌ MySQL driver: go-sql-driver/mysql v1.7.1 (not in go.mod)
- ❌ OAuth2 library: golang.org/x/oauth2 v0.18.0 (not in go.mod)
- ❌ WASM runtime: wasmtime instead of wazero v1.6.0
- ⚠️ sqlc used but not listed as require dependency
- ⚠️ golangci-lint, go-licenses not integrated as build targets

**Conformance Rating**: 70%
- Core Go modules: ✅ Implemented
- Dependency management: ✅ Implemented
- CGO configuration: ✅ Implemented
- Platform builds: ✅ Implemented
- Specific drivers: ⚠️ Some missing
- Development tools: ⚠️ Partial

**Architectural Differences**:
- WASM runtime choice: wasmtime vs. wazero
- Some dependencies not pinned to exact spec versions

---

### 2. WASM-SANDBOX-SECURITY.md

**Spec Requirements**: Critical security specification for WASM isolation

**Required Components**:
1. **Memory Isolation Guarantees** (spec:106-177)
   - `internal/core/wasm/memory.go` with `MemoryIsolation` struct
   - Linear memory boundaries per module
   - No shared memory with host
   - Copy-in/copy-out semantics for data exchange
   - `CopyToModule()`, `CopyFromModule()` with bounds checking

2. **Capability-Based System-Call Interposition** (spec:284-403)
   - `internal/core/wasm/capability.go` with `SyscallCapability` map
   - System-call allow-list: fd_write, fd_read, sock_send, sock_recv
   - WASI configuration with pre-opened directories
   - Network access control with host-based allow-list

3. **Time-of-Check-Time-of-Use (TOCTOU) Prevention** (spec:405-502)
   - `internal/core/wasm/verification.go` with `AtomicVerifier`
   - Atomic signature + timestamp verification
   - Nonce-based replay prevention
   - Revocation checking

4. **Deterministic Execution Guarantees** (spec:504-577)
   - `internal/core/wasm/determinism.go` with `DeterministicConfig`
   - Platform-independent floating-point (IEEE 754-2019)
   - Fuel metering for time-bounded execution
   - Disable SIMD, reference types, bulk memory

5. **Aggregate Memory Limits** (spec:227-277)
   - `RuntimePool` with aggregate tracking
   - Per-module limit enforcement
   - Total pool limits

**Actual Implementation**:
- Location: `pkg/wasm/sandbox.go:1-291`
- Type: Only filesystem sandboxing
- Evidence: `pkg/wasm/sandbox.go:14-291`

**What's Implemented**:
- ✅ Sandbox filesystem structure: `SandboxedFS` type
- ✅ Directory access control: AllowDirectory(), AllowSubdirectory()
- ✅ Read-only file handling: SetReadOnly()
- ✅ Virtual file support: SetVirtualFile(), GetVirtualFile()
- ✅ Path normalization and validation
- ✅ File operations with permission checks

**What's Missing**:
- ❌ Memory isolation: NO linear memory boundary enforcement
- ❌ Capability enforcement: NO WASI system-call interposition
- ❌ TOCTOU prevention: NO atomic signature verification
- ❌ Deterministic execution: NO fuel metering or platform guarantees
- ❌ Aggregate limits: NO runtime pool with memory tracking
- ❌ Memory isolation: NO copy-in/copy-out semantics
- ❌ Host function safety: NO pointer checking
- ❌ WASM execution: No actual WASM runtime integration

**Conformance Rating**: <10%
- Filesystem sandbox: ✅ Implemented
- Memory isolation: ❌ NOT IMPLEMENTED
- Capability enforcement: ❌ NOT IMPLEMENTED
- TOCTOU prevention: ❌ NOT IMPLEMENTED
- Deterministic execution: ❌ NOT IMPLEMENTED
- Aggregate limits: ❌ NOT IMPLEMENTED

**Critical Issue**: This is a security-critical specification for WASM isolation, but almost none of the required security features are implemented. The current `pkg/wasm/sandbox.go` only provides basic filesystem sandboxing, which is insufficient for secure WASM execution as specified.

---

### 3. BUILTIN-COMMANDS.md

**Spec Requirements**:
- Root command `muster` with persistent flags
- Config subcommands: get, set, profiles, use
- Auth subcommands: login (with oauth, token, basic), logout, status
- Version command with build details
- Documentation commands: generate, serve
- Proper config loading and profile management
- Full persistence with validation

**Actual Implementation**:
- Location: `internal/builtin/` (config, auth)
- Status: Command structures exist with partial functionality
- Evidence:
  - `internal/builtin/config.go:1-142` - profile management
  - `internal/builtin/auth.go:1-99` - auth command framework

**What's Implemented**:
- ✅ config command structure (config.go:10-32)
- ✅ profile list command (config.go:41-68)
- ✅ profile show command (config.go:70-91)
- ✅ profile switch command (config.go:93-115)
- ✅ profile delete command (config.go:117-139)
- ✅ auth command structure (auth.go:10-22)
- ✅ auth login command with OAuth2, token, basic flags (auth.go:25-72)
- ✅ auth logout command (auth.go:75-84)
- ✅ auth status command (auth.go:88-99)

**What's Missing**:
- ❌ config get [key] command - not implemented
- ❌ config set [key] [value] command - not implemented
- ❌ Config value manipulation - only profiles implemented
- ❌ Auth token validation - login is stub only
- ❌ OAuth2 flow - no actual OAuth implementation
- ❌ Credential storage - no token persistence
- ❌ Version command - not found
- ❌ docs generate command - not found
- ❌ docs serve command - not found
- ❌ Persistent config - config loading exists but no value storage

**Conformance Rating**: 50%
- Command structure: ✅ Implemented
- Profile management: ✅ Implemented
- Auth framework: ✅ Implemented
- Config get/set: ❌ NOT IMPLEMENTED
- Actual auth logic: ❌ STUBS ONLY
- Version command: ❌ NOT IMPLEMENTED
- Docs commands: ❌ NOT IMPLEMENTED
- Full persistence: ⚠️ Partial

**Architectural Differences**:
- Spec defines full implementations with persistence
- Actual has command framework without backend logic
- Auth commands are interactive shells without actual authentication

---

## Cumulative Statistics

### Conformance Breakdown (123/227 specs = 54.2% sampled)

| Category | Count | % of Sampled |
|----------|-------|--------------|
| ✅ Fully Implemented | 27 | 22.0% |
| ⚠️ Partial Implementation | 23 | 18.7% |
| ⏭️ Meta-Docs / Out of Scope | 72 | 58.5% |
| ❌ Not Started | 1 | 0.8% |

### Session 307 Contribution

- Critical security gaps found: 1 (WASM-SANDBOX-SECURITY.md)
- Partial implementations: 2 (DEPENDENCIES.md, BUILTIN-COMMANDS.md)
- New implementation gaps documented:
  - WASM memory isolation and capability enforcement
  - Built-in command implementations (config get/set, docs, version)
  - Auth flow implementations (OAuth2, token storage)

---

## Recommendations

### For WASM-SANDBOX-SECURITY.md (CRITICAL):

**Priority**: 🔴 HIGH - Security-critical specification

1. **Must Implement for Secure WASM Execution**:
   - Memory isolation with `MemoryIsolation` type
   - Capability-based system call interposition
   - TOCTOU prevention for signature verification
   - Deterministic execution guarantees
   - Aggregate memory limits

2. **Implementation Order**:
   a. Memory isolation (foundational)
   b. Capability enforcement (security layer)
   c. TOCTOU prevention (verification)
   d. Deterministic execution (platform consistency)

3. **Integration Points**:
   - Connect to pkg/wasm runtime
   - Integrate with inventory caching
   - Use with CLI-generated commands

---

### For BUILTIN-COMMANDS.md:

**Priority**: 🟡 MEDIUM - User-facing commands

1. **Complete Command Implementations**:
   - Add `config get [key]` with actual config retrieval
   - Add `config set [key] [value]` with persistence
   - Implement version command with build details
   - Implement docs generate/serve commands

2. **Auth Implementation**:
   - Replace stubs with actual token validation
   - Implement full OAuth2 flow
   - Add credential storage (keychain integration)
   - Implement auth status checking

3. **Integration Needed**:
   - Connect to pkg/config persistence layer
   - Connect to pkg/auth authentication
   - Use pkg/openapi for docs generation

---

### For DEPENDENCIES.md:

**Priority**: 🟢 LOW - Administrative

1. **Review Missing Dependencies**:
   - Decide if PostgreSQL/MySQL drivers needed
   - Add OAuth2 dependency if required
   - Document WASM runtime choice (wasmtime vs wazero)
   - Consider adding development tool dependencies

2. **Update Alignment**:
   - Update spec or implementation as appropriate
   - Document architectural decisions

---

## Evidence Links

**DEPENDENCIES.md**:
- Spec definition: `specs/DEPENDENCIES.md:1-458`
- Go modules: `go.mod:1-85`
- Makefile targets: `Makefile:31, 427-451`

**WASM-SANDBOX-SECURITY.md**:
- Spec definition: `specs/WASM-SANDBOX-SECURITY.md:1-612`
- Actual implementation: `pkg/wasm/sandbox.go:1-291`
- Required missing modules:
  - `internal/core/wasm/memory.go`
  - `internal/core/wasm/capability.go`
  - `internal/core/wasm/verification.go`
  - `internal/core/wasm/determinism.go`

**BUILTIN-COMMANDS.md**:
- Spec definition: `specs/BUILTIN-COMMANDS.md:1-600`
- Config command: `internal/builtin/config.go:1-142`
- Auth command: `internal/builtin/auth.go:1-99`

---

**End of Session 307**
**Next Session**: 308
**Cumulative Sample**: 123/227 specs (54.2%)
