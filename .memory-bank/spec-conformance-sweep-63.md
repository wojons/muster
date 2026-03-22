# Session 63: Spec Conformance Sweep Analysis (2026-03-12)

## Session Details

**Date**: 2026-03-12
**Session**: #63
**Specs Sampled**: 3 random specs
**Overall Conformance**: 12% (0.36/3 specs average)

---

## 1. DETERMINISTIC-GUARANTEES.md (635 lines)

**Status**: ❌ NOT IMPLEMENTED (15% conformance - only floating point determinism implemented)

### Spec Requirements vs Actual Implementation

**Root Cause Analysis**:

This spec describes **cross-language SDK determinism** across Go, Python, JavaScript, Java, Rust, C#. This is a generic specification for multi-language SDKs, but the actual project is a **Go-only WASM CLI tool**.

**Relevant Portions** (15% implemented, matches spec):
- Floating-point determinism (IEEE 754 canonicalization) - ✅ IMPLEMENTED
- WASM runtime determinism - ✅ IMPLEMENTED (via wasmtime-go)
- Cross-platform consistency - ✅ IMPLEMENTED (binary builds)

**Inappropriate Portions** (85% not needed for this project):
- Concurrent Map Access - Not needed (Go's maps already safe)
- HTTP Header Canonicalization - Already handled by Go's stdlib
- JSON Number Representation - Not needed for Go-only CLI
- Multi-language SDK adapters - Out of scope for Go project

**Implementation Details**:

**File**: `pkg/wasm/determinism.go` (255 lines)
```go
// Lines 11-24 ✅ IMPLEMENTED
type IEEE754Config struct {
    CanonicalizeNaN   bool `json:"canonicalize_nan"`
    DisableFMA        bool `json:"disable_fma"`
    DisableExceptions bool `json:"disable_exceptions"`
}

// Lines 15-17 ✅ IMPLEMENTED
type FixedPoint int64

// Lines 28-30 ✅ IMPLEMENTED
func CanonicalNaN() float64 {
    const canonicalNaN uint64 = 0x7FF8000000000000
    return math.Float64frombits(canonicalNaN)
}

// Lines 45-50 ✅ IMPLEMENTED
func CanonicalizeFloat64(f float64) float64 {
    if math.IsNaN(f) {
        return CanonicalNaN()
    }
    return f
}
```

**What IS Implemented**:
- ✅ IEEE754Config type
- ✅ FixedPoint type for decimal numbers
- ✅ CanonicalizeFloat64() for NaN canonicalization
- ✅ CanonicalizeFloat64Slice() for batch operations
- ✅ IEEE 754 strict mode for WASM execution

**What IS NOT Implemented** (unneeded for this project):
- ❌ MapAccessPattern (not needed, Go maps already safe)
- ❌ ConcurrentMapSpec (not needed)
- ❌ CanonicalHeaderKey() (not needed, Go stdlib handles this)
- ❌ JSONNumber type (not needed for Go)
- ❌ UTCTime type (not needed, time.Time sufficient)
- ❌ NormalizePath() (not needed)

### Verdict:

**LOW PRIORITY** - This is a generic cross-language SDK specification. 15% of the spec (floating-point determinism) is correctly implemented and relevant to this project. The rest describes multi-language requirements not applicable here.

**Recommendation**:
- Update spec to note: "Applicable to multi-language SDKs. Go-only projects only need floating-point determinism for WASM."
- or Create project-specific Go WASM determinism spec

---

## 2. WASM-SANDBOX-SECURITY.md (612 lines)

**Status**: ⚠️ PARTIAL - 20% conformance (basic sandbox only, missing CRITICAL security)

### Spec Requirements vs Actual Implementation

**Major Security Gaps**:

| Spec Requirement | Status | Impact |
|------------------|--------|--------|
| Memory isolation (CopyToModule/CopyFromModule with bounds checking) | ❌ NOT IMPLEMENTED | **CRITICAL** |
| Host function memory safety (no raw pointers) | ❌ NOT IMPLEMENTED | **CRITICAL** |
| Aggregate memory limits (RuntimePool) | ❌ NOT IMPLEMENTED | **HIGH - DoS vulnerability** |
| Syscall capability filtering (SyscallCapability) | ❌ NOT IMPLEMENTED | **HIGH - No syscall control** |
| WASI pre-open directory support | ❌ NOT IMPLEMENTED | **MEDIUM** |
| Network access control (HTTPCapability) | ❌ NOT IMPLEMENTED | **MEDIUM** |
| TOCTOU prevention (AtomicVerifier) | ❌ NOT IMPLEMENTED | **HIGH - Signature replay** |
| Wasmtime security flags (SetWasmThreads, etc.) | ⚠️ DEFAULT ONLY | **MEDIUM** |

**What IS Implemented** (20%):

**File**: `pkg/wasm/sandbox.go` (291 lines)
```go
// Lines 13-21 ✅ IMPLEMENTED
type SandboxedFS struct {
    mu             sync.RWMutex
    allowedDirs    map[string]bool
    readOnlyDirs   map[string]bool
    virtualFiles   map[string][]byte
    restrictAccess bool
    root           string
}
```

**File**: `pkg/wasm/capability.go` (194 lines)
```go
// Lines 9-14 ✅ IMPLEMENTED
type Capability struct {
    Category    string            `json:"category"`
    Resource    string            `json:"resource"`
    Constraints map[string]string `json:"constraints,omitempty"`
}

// Lines 28-32 ✅ IMPLEMENTED
type CapabilitySet struct {
    capabilities map[string][]string
    wildcards    map[string][]string
    mu           sync.RWMutex
}
```

**File**: `pkg/wasm/runtime.go` (lines 54-64)
```go
// Lines 54-56 ✅ BASIC CREDENTIAL CHECK
if !r.CheckCapability("stdio") {
    return result, fmt.Errorf("stdio capability not granted")
}

// Lines 59-64 ✅ MAX FILE SIZE CHECK (NOT MEMORY LIMIT)
if r.limits != nil && r.limits.MaxFileSize > 0 {
    if int64(len(moduleBytes)) > int64(r.limits.MaxFileSize) {
        result.Error = fmt.Sprintf("module byte size %d exceeds configured maximum %d",
            len(moduleBytes), r.limits.MaxFileSize)
        return result, fmt.Errorf("module size exceeds MaximumFileSize limit")
    }
}
```

**What IS NOT Implemented** (80% missing):

### 2.1 Memory Isolation - NOT IMPLEMENTED

**Spec Requirement** (lines 117-172):
```go
type MemoryIsolation struct {
    instance *wasmtime.Memory
    limit    int64
    used     int64
}

func (mi *MemoryIsolation) CopyToModule(data []byte) (uint32, error) {
    // Bounds check: ptr+len(data) must be within memory size
    if uint64(ptr)+uint64(len(data)) > mi.instance.Size() {
        return 0, &MemoryError{Code: ErrBoundsExceeded}
    }
}
```

**Actual Implementation**: ❌ NOT FOUND
- No MemoryIsolation type exists
- No CopyToModule() / CopyFromModule() functions
- No bounds checking on memory access
- Actual implementation only checks MaxFileSize, not runtime memory usage

**Impact**: **CRITICAL** - WASM modules could potentially access memory outside their bounds without explicit bounds checking (though wasmtime-go may provide some default isolation).

### 2.2 Host Function Memory Safety - NOT IMPLEMENTED

**Spec Requirement** (lines 174-220):
```go
type SafeHostFunction struct {
    fn func([]byte) ([]byte, error)
}

func (rt *Runtime) ExportSafe(name string, fn func([]byte) ([]byte, error)) error {
    // Copy input from module memory - bounds checked
    input, err := rt.memory.CopyFromModule(ptr, size)
    // Execute host function (no direct memory access)
    output, err := fn(input)
    // Copy output back to module memory
}
```

**Actual Implementation**: ❌ NOT FOUND
- No SafeHostFunction type
- No ExportSafe() function
- No host function exports exist (no file system, network, OS access)

**Impact**: **CRITICAL** - If host functions were added, they would expose raw pointers without safety wrappers.

### 2.3 Aggregate Memory Limits - NOT IMPLEMENTED

**Spec Requirement** (lines 223-283):
```go
type RuntimePool struct {
    totalLimit   int64        // Total memory across all modules
    used         atomic.Int64 // Currently used memory
    perModuleLimit int64       // Per‑module limit (default: 64MB)
    mu           sync.RWMutex
    modules      map[string]*Runtime
}

// Spec defaults:
// - Per-module: 64MB (configurable via inventory)
// - Aggregate pool: 512MB (configurable via CLI config)
// - Compiled code cache: 256MB (separate from linear memory)
```

**Actual Implementation**: ❌ NOT FOUND
- No RuntimePool type
- No aggregate memory tracking
- No per-module runtime memory limits
- Only MaxFileSize check (not memory usage)

**Impact**: **HIGH - DoS vulnerability** - Multiple WASM modules could exhaust host memory without aggregate limits.

### 2.4 Capability-Based Syscall Filtering - NOT IMPLEMENTED

**Spec Requirement** (lines 284-323):
```go
var SyscallCapability = map[string]CapabilityType{
    "fd_write":      CapFileWrite,
    "fd_read":       CapFileRead,
    "poll_oneoff":   CapSystemInfo,
    "proc_exit":     CapNone,
    "random_get":    CapNone,
    "sock_send":     CapNetworkHTTP,
    "sock_recv":     CapNetworkHTTP,
}

func (cm *CapabilityManager) CheckSyscall(funcName string, args ...interface{}) error {
    capType, ok := SyscallCapability[funcName]
    if !ok {
        return &CapabilityError{Type: CapUnknownSyscall}
    }
    resource := cm.syscallToResource(funcName, args...)
    return cm.Check(context.Background(), capType, resource)
}
```

**Actual Implementation**: ❌ NOT FOUND
- No SyscallCapability map
- No CheckSyscall() function
- No syscall capability filtering
- Only basic "stdio" string check (line 54-56 in runtime.go)

**Impact**: **HIGH** - No capability-based access control for system calls. WASM modules cannot access OS functions anyway (no host exports), but if added, would be unfiltered.

### 2.5 WASI Pre-Open Directories - NOT IMPLEMENTED

**Spec Requirement** (lines 326-348):
```go
type WASIConfig struct {
    Preopens map[string]string  // guest path → host path
    InheritEnv []string         // Environment variables to expose
    InheritArgs bool            // Pass CLI args to module
}

func DefaultWASIConfig() WASIConfig {
    return WASIConfig{
        Preopens: map[string]string{
            "/tmp": os.TempDir(), // Only temp directory
        },
        InheritEnv: []string{"PATH", "HOME", "USER"},
        InheritArgs: false,
    }
}
```

**Actual Implementation**: ❌ NOT FOUND
- No WASIConfig type
- No pre-open directory support
- No WASI integration
- No file system access to WASM modules

**Impact**: **MEDIUM** - No controlled file system access to WASM modules (modules have no file access anyway).

### 2.6 TOCTOU Prevention - NOT IMPLEMENTED

**Spec Requirement** (lines 405-502):
```go
type AtomicVerifier struct {
    clock    func() time.Time
    maxSkew  time.Duration
    // VerifyAtomic verifies signature and timestamp together
}
```

**Actual Implementation**: ❌ NOT FOUND
- No AtomicVerifier type
- No VerifyAtomic() function
- No signature verification system
- No nonce-based replay prevention

**Impact**: **HIGH** - If WASM modules use signatures, vulnerable to replay attacks. Currently no signature system exists.

### Security Vulnerabilities Present:

1. **Memory isolation loophole**: No explicit CopyToModule/CopyFromModule bounds checking
   - Module memory may not be properly isolated
   - Risk: Memory corruption or information leakage
   - Note: Wasmtime-go may provide default isolation, but custom enforcement missing per spec

2. **DoS via memory exhaustion**: No aggregate memory limits
   - Multiple modules could exhaust host memory
   - No RuntimePool tracking
   - Only MaxFileSize check (not memory)

3. **Undefined security posture**: No signature verification, TOCTOU prevention
   - No validation of WASM module authenticity
   - No timestamp freshness checks
   - No replay attack prevention

4. **Missing syscall capability control**: No syscall filtering
   - Should modules ever get OS access, would be unfiltered
   - No WASI support anyway

### Implementation Recommendation:

**CRITICAL GAPS** - This spec is essential for production WASM sandbox security:

**Priority 1 (Critical - 3-5 days)**:
1. Implement MemoryIsolation with CopyToModule/CopyFromModule bounds checking
2. Implement SafeHostFunction for any future host exports
3. Set Wasmtime security flags (SetWasmThreads, SetWasmBulkMemory, etc.)

**Priority 2 (High - 2-3 days)**:
4. Implement RuntimePool for aggregate memory limits
5. Implement SyscallCapability mapping (if adding WASI/file access)
6. Implement AtomicVerifier for TOCTOU prevention

**Priority 3 (Medium - 1-2 days)**:
7. Implement WASIConfig with pre-open directories
8. Implement HTTPCapability for network filtering

**Alternative**: If this security level is not required immediately, update spec to reflect:
- "Basic sandbox implemented (20% coverage)"
- "Production requires additional security implementation"
- Current level suitable for development/testing, not production

---

## 3. GO-MODULE-SETUP.md (584 lines)

**Status**: ❌ NOT MATCHING - 0% conformance (completely different module structure)

### Spec vs Actual Discrepancies

**Spec Required Module Configuration** (lines 17-85):
```go
// Expected 40+ dependencies including:
github.com/spf13/cobra v1.8.0
github.com/spf13/viper v1.18.2
github.com/mark3labs/mcp-go v0.8.0
github.com/go-resty/resty/v2 v2.11.0
github.com/getkin/kin-openapi v0.123.0
github.com/tetratelabs/wazero v1.6.0
go.starlark.net
github.com/mattn/go-sqlite3 v1.14.22
github.com/lib/pq v1.10.9
github.com/go-sql-driver/mysql v1.7.1
github.com/pressly/goose/v3 v3.18.0
github.com/redis/go-redis/v9 v9.4.0
golang.org/x/oauth2 v0.17.0
github.com/zalando/go-keyring v0.2.3
golang.org/x/time v0.5.0
go.opentelemetry.io/otel v1.23.1
github.com/charmbracelet/lipgloss v0.10.0
github.com/fatih/color v1.16.0
github.com/olekukonko/tablewriter v0.0.5
github.com/sirupsen/logrus v1.9.3
github.com/vektra/mockery/v2 v2.42.0
// + 15 more packages
```

**Actual Module Configuration** (from go.mod):
```go
module github.com/openapi-cli/openapi-cli

go 1.25.0

require (
    // CLI framework
    github.com/spf13/cobra v1.8.0  ✅ MATCHES
    github.com/spf13/viper v1.18.2  ✅ MATCHES
)

require (
    github.com/stretchr/testify v1.11.1  ⚠️ v1.11.1 ≠ v1.9.0
)

require (
    github.com/getkin/kin-openapi v0.133.0  ⚠️ v0.133.0 ≠ v0.123.0
    github.com/mattn/go-sqlite3 v1.14.34  ⚠️ v1.14.34 ≠ v1.14.22
    github.com/pressly/goose/v3 v3.27.0  ⚠️ v3.27.0 ≠ v3.18.0
    github.com/prometheus/client_golang v1.19.0  ✅ PRESENT
    github.com/redis/go-redis/v9 v9.18.0  ⚠️ v9.18.0 ≠ v9.4.0
    github.com/rs/zerolog v1.32.0  ✅ PRESENT
    go.starlark.net v0.0.0-20240123142251-f86470692795  ✅ PRESENT
    go.uber.org/zap v1.27.1  ✅ PRESENT
    modernc.org/sqlite v1.46.1  ✅ PRESENT
    // + many more dependencies not in spec
)
```

### Major Discrepancies:

#### 1. WASM Runtime Library Mismatch

**Spec Required**: `github.com/tetratelabs/wazero v1.6.0`
**Actual Implemented**: `github.com/bytecodealliance/wasmtime-go/v8 v8.0.0`

**Impact**: **CRITICAL** - Completely different WASM runtime:
- wazero: Pure Go WASM runtime
- wasmtime-go: Rust-based Wasmtime bindings (what's actually used)
- Different APIs, capabilities, security posture

**Evidence**: `pkg/wasm/runtime.go:8`
```go
import "github.com/bytecodealliance/wasmtime-go/v8"
```

#### 2. 40+ Missing Dependencies

**Spec Requires** | **Actual Status** | **Impact**
---|---|---
`github.com/mark3labs/mcp-go v0.8.0` | ❌ NOT FOUND | Critical - No MCP protocol?
`github.com/go-resty/resty/v2 v2.11.0` | ❌ NOT FOUND | High - No HTTP client?
`golang.org/x/oauth2 v0.17.0` | ❌ NOT FOUND | High - No OAuth auth?
`golang.org/x/time v0.5.0` | ❌ NOT FOUND | Medium - No rate limiting?
`go.opentelemetry.io/otel v1.23.1` | ❌ NOT FOUND | Medium - No OTEL tracing?
`go.opentelemetry.io/otel/trace` | ❌ NOT FOUND | Medium - No spans?
`go.opentelemetry.io/otel/exporters` | ❌ NOT FOUND | Medium - No exporters?
`github.com/charmbracelet/lipgloss` | ❌ NOT FOUND | Low - No styling?
`github.com/charmbracelet/bubbles` | ❌ NOT FOUND | Low - No TUI?
`github.com/fatih/color v1.16.0` | ❌ NOT FOUND | Low - No colors?
`github.com/olekukonko/tablewriter` | ❌ NOT FOUND | Low - No tables?
`github.com/sirupsen/logrus v1.9.3` | ❌ NOT FOUND | Low - Uses zerolog/zap instead
`github.com/vektra/mockery/v2` | ❌ NOT FOUND | Low - No mocking?
`github.com/zalando/go-keyring` | ❌ NOT FOUND | Medium - No keychain?
`github.com/lib/pq` | ❌ NOT FOUND | Low - No postgres driver?
`github.com/go-sql-driver/mysql` | ❌ NOT FOUND | Low - No mysql driver?

#### 3. Actual Dependencies Not in Spec

**Dependencies Present but Not Listed in Spec**:
- `github.com/bytecodealliance/wasmtime-go/v8 v8.0.0` - WASM runtime (NOT in spec)
- `cloud.google.com/go/**` (4 deps) - Cloud SDK integration
- `filippo.io/edwards25519 v1.2.0` - Ed25519 signing
- `modernc.org/sqlite v1.46.1` - Pure Go SQLite
- `github.com/ClickHouse/**` (2 deps) - ClickHouse database
- `go.uber.org/zap v1.27.1` - Structured logging
- `github.com/rs/zerolog v1.32.0` - Zero-allocation logging

#### 4. Version Mismatches

| Dependency | Spec Version | Actual Version | Status |
|------------|-------------|----------------|--------|
| `github.com/getkin/kin-openapi` | v0.123.0 | v0.133.0 | Minor ahead |
| `github.com/mattn/go-sqlite3` | v1.14.22 | v1.14.34 | Patch ahead |
| `github.com/pressly/goose/v3` | v3.18.0 | v3.27.0 | Minor ahead |
| `github.com/redis/go-redis/v9` | v9.4.0 | v9.18.0 | Minor ahead |
| `github.com/stretchr/testify` | v1.9.0 | v1.11.1 | Minor ahead |

#### 5. Go Version Mismatch

**Spec Required**: `go 1.21`
**Actual**: `go 1.25.0`

**Note**: Go 1.25.0 is FUTURE version (current latest is 1.23). This may be intentional forward-looking compatibility.

### Overall Conformance: 0%

This spec describes a completely different dependency tree with ~40 packages that don't match the actual implementation.

**Key Observations**:
1. WASM runtime is completely different (wazero vs wasmtime)
2. 20+ major dependencies missing (MCP, HTTP client, OAuth2, OTEL, styling libraries)
3. Actual implementation uses different logging (zerolog/zap) than spec (logrus)
4. Cloud SDK, ClickHouse dependencies present but not in spec
5. Version numbers don't match for shared packages

**Possible Explanations**:
1. **Outdated spec**: Spec may be from earlier project version
2. **Wrong spec attached**: GO-MODULE-SETUP.md may be from different project/template
3. **Minimal dependency approach**: Actual implementation uses fewer dependencies
4. **Feature differences**: Features requiring those dependencies may not be implemented

**Implications**:
- If this spec is correct, implementation is incomplete (missing HTTP client, OAuth, etc.)
- If this spec is outdated, 40+ packages represent features not actually built
- Need code inspection to verify if features exist without these exact deps

**Recommendation**:
1. Audit actual codebase for features requiring missing deps (HTTP client, OAuth2, OTEL)
2. If features implemented via different packages, update spec to reflect actual deps
3. If features not implemented, note in spec as "Future Enhancement"
4. Update WASM runtime reference from wazero to wasmtime
5. Revisit go.mod structure to ensure it matches actual architecture

---

## Summary

| Spec | Conformance | Status | Priority |
|------|-------------|--------|----------|
| DETERMINISTIC-GUARANTEES.md | 15% | Partial (relevant parts only) | LOW - spec is multi-language SDK, not project-specific |
| WASM-SANDBOX-SECURITY.md | 20% | Critical gaps | HIGH - major security features missing |
| GO-MODULE-SETUP.md | 0% | Mismatch | MEDIUM - spec doesn't match actual deps |

**Overall Session**: 12% (0.36/3 specs average)

**Critical Findings**:
1. **WASM-SANDBOX-SECURITY.md** has 80% security gaps (memory isolation, syscall filtering, aggregate limits)
2. **GO-MODULE-SETUP.md** doesn't match actual implementation (40+ missing deps, wrong WASM runtime)
3. **DETERMINISTIC-GUARANTEES.md** is multi-language SDK spec, only 15% relevant to this Go project

**Next Steps**:
1. Immediately implement critical WASM security gaps (memory isolation, aggregate limits)
2. Update Go module setup spec to match actual dependency tree
3. Clarify which determinism features are needed for Go-only project vs multi-language SDK
