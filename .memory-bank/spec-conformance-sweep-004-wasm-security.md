# Spec Conformance Sweep Report - WASM-SANDBOX-SECURITY.md
**Date**: 2026-03-05
**Spec File**: specs/WASM-SANDBOX-SECURITY.md
**Status**: 60% CONFORMANT (12/20 requirements met)

---

## Executive Summary

Critical security spec `WASM-SANDBOX-SECURITY.md` analyzed for spec-implementation conformance. Implementation meets 60% of requirements with CRITICAL gaps in TOCTOU prevention and deterministic execution guarantees.

**Critical Findings**: 2 CRITICAL gaps (TOCTOU prevention, nonce replay protection)
**Overall Risk**: HIGH (security-critical features missing, system NOT production-ready without these)

---

## Requirements Analysis

### ✅ Requirements Fully Met (12)

| # | Requirement | Evidence | Location |
|---|-------------|----------|----------|
| 1 | Memory Isolation: Per-module memory | wasmtime.Memory isolation enforced | pkg/wasm/runtime.go:67-71 |
| 2 | Capacity-Based: System-call allow-list | CapabilitySet with category/resource filtering | pkg/wasm/capability.go:28-32 |
| 3 | Capability: Grant/Revoke/Check | Full capability management API | pkg/wasm/capability.go:42-122 |
| 4 | WASI Pre-open Directories | SandboxedFS with allowedDirs, readOnlyDirs | pkg/wasm/sandbox.go:14-41 |
| 5 | Network Access Control: Policy | NetworkPolicy with allowedHosts/blockedHosts | pkg/wasm/types.go:69-107 |
| 6 | Network: Host-based allow-list | IsAllowed() checks host/port against policy | pkg/wasm/types.go:76-107 |
| 7 | Deterministic: IEEE 754 Floating Point | CanonicalNaN, CanonicalizeFloat64 | pkg/wasm/determinism.go:26-57 |
| 8 | Deterministic: Fixed-Point Arithmetic | FixedPoint type for deterministic ops | pkg/wasm/determinism.go:59-110 |
| 9 | Deterministic: DeterministicFloat | DeterministicFloat struct with NaN canonicalization | pkg/wasm/determinism.go:112-174 |
| 10 | Deterministic: SortMapKeys | Deterministic map key sorting | pkg/wasm/determinism.go:177-186 |
| 11 | Resource Bounds: SandboxLimits | MaxMemory, MaxExecutionTime, MaxFileSize, etc. | pkg/wasm/types.go:35-67 |
| 12 | Resource Bounds: Limit Enforcement | Limits checked before execution | pkg/wasm/runtime.go:59-64, 25-29 |

### ❌ Gaps Found (8)

#### Gap 1: Atomic Signature + Timestamp Verification (CRITICAL)
**Requirement**: Signature verification MUST check timestamp and signature atomically (spec lines 322-370)

**Gap**: No implementation of atomic signature + timestamp verification found. Only SHA256 checksums in inventory (pkg/inventory/manager.go:34) but no cryptographic signature verification with timestamp validation.

**Impact**: CRITICAL - TOCTOU vulnerability exists, signatures can be replayed with old timestamps

**Remediation**:
```go
// pkg/wasm/verification.go (FILE DOES NOT EXIST)

type AtomicVerifier struct {
    clock   func() time.Time
    maxSkew time.Duration
}

func (av *AtomicVerifier) VerifyAtomic(
    module []byte,
    signature []byte,
    timestamp time.Time,
    publicKey ed25519.PublicKey,
) error {
    now := av.clock()
    if timestamp.After(now.Add(av.maxSkew)) {
        return &VerificationError{Code: ErrTimestampFuture}
    }
    if timestamp.Before(now.Add(-24 * time.Hour)) {
        return &VerificationError{Code: ErrTimestampExpired}
    }
    signedData := append(module, []byte(timestamp.Format(time.RFC3339))...)
    if !ed25519.Verify(publicKey, signedData, signature) {
        return &VerificationError{Code: ErrSignatureInvalid}
    }
    // Check revocation atomically
    if av.revocationList.IsRevoked(publicKey) {
        return &VerificationError{Code: ErrKeyRevoked}
    }
    return nil
}
```

**Priority**: CRITICAL (security vulnerability, TOCTOU attack possible)

**Estimated Effort**: 4-6 hours (file creation + ed25519 API integration + testing)

---

#### Gap 2: Nonce-Based Replay Prevention (CRITICAL)
**Requirement**: Nonce-based replay prevention for stateful operations (spec lines 377-414)

**Gap**: No NonceStore implementation. Replay attacks possible if vulnerable modules are reused.

**Impact**: CRITICAL - Replay attacks possible with old valid signatures

**Remediation**:
```go
// pkg/wasm/nonce.go (FILE DOES NOT EXIST)

type NonceStore struct {
    store   map[string]time.Time
    mu      sync.RWMutex
    ttl     time.Duration
    cleanup *time.Ticker
}

func (ns *NonceStore) CheckNonce(nonce string) error {
    ns.mu.Lock()
    defer ns.mu.Unlock()

    if _, exists := ns.store[nonce]; exists {
        return &VerificationError{Code: ErrNonceReused, Nonce: nonce}
    }
    ns.store[nonce] = time.Now()
    return nil
}

func (ns *NonceStore) Cleanup() {
    ns.mu.Lock()
    defer ns.mu.Unlock()
    now := time.Now()
    for nonce, ts := range ns.store {
        if now.Sub(ts) > ns.ttl {
            delete(ns.store, nonce)
        }
    }
}
```

**Priority**: CRITICAL (security vulnerability, replay attacks possible)

**Estimated Effort**: 2-3 hours (file creation + cleanup logic + testing)

---

#### Gap 3: Fuel Metering for Time-Bounded Execution (HIGH)
**Requirement**: Fuel metering ensures execution cannot exceed deterministic limits (spec lines 453-489)

**Gap**: Fuel metering NOT implemented. Comment in pkg/wasm/runtime.go:69-70 acknowledges this as "future enhancement".

**Impact**: HIGH - Deterministic execution time cannot be guaranteed without fuel

**Remediation**:
```go
// pkg/wasm/fuel.go (FILE DOES NOT EXIST)

type FuelMeter struct {
    remaining uint64
    limit     uint64
    mu        sync.RWMutex
}

func (fm *FuelMeter) Consume(amount uint64) error {
    fm.mu.Lock()
    defer fm.mu.Unlock()

    if amount > fm.remaining {
        return &ExecutionError{Code: ErrFuelExhausted}
    }
    fm.remaining -= amount
    return nil
}

var FuelCosts = map[string]uint64{
    "local.get":    1,
    "local.set":    1,
    "i32.add":      1,
    "call":         5,
    "call_indirect": 10,
}
```

**Priority**: HIGH (deterministic execution guaranteed without fuel metering)

**Estimated Effort**: 3-4 hours (file creation + wasmtime fuel integration + testing)

---

#### Gap 4: Wasmtime Deterministic Configuration (MEDIUM)
**Requirement**: SetWasmThreads(false), SetWasmReferenceTypes(false), etc. (spec lines 437-444)

**Gap**: Wasmtime engine created with default config. Determinism flags NOT explicitly set to match spec requirements.

**Current Code** (pkg/wasm/runtime.go:67):
```go
engine := wasmtime.NewEngine()
```

**Missing Code** (from spec lines 436-444):
```go
config := wasmtime.NewConfig()
config.SetConsumeFuel(true)
config.SetWasmSIMD(false)              // ✗ NOT SET
config.SetWasmRelaxedSimd(false)       // ✗ NOT SET
config.SetWasmMultiValue(false)        // ✗ NOT SET
config.SetWasmTailCall(false)          // ✗ NOT SET
config.SetWasmBulkMemory(false)        // ✗ NOT SET
config.SetWasmReferenceTypes(false)    // ✗ NOT SET
engine := wasmtime.NewEngineWithConfig(config)
```

**Impact**: MEDIUM - Platform-specific optimizations may introduce non-determinism

**Remediation**: Update pkg/wasm/runtime.go:67 to use deterministic config (2 hours effort)

**Priority**: MEDIUM (quality enhancement, not blocking but affects determinism guarantee)

**Estimated Effort**: 2 hours

---

#### Gap 5: Host Function Memory Safety (MEDIUM)
**Requirement**: Host functions exposed to WASM must not receive or return raw pointers (spec lines 88-133)

**Gap**: No SafeHostFunction implementation found. Host function exports not verified for memory safety.

**Impact**: MEDIUM - Potential for host memory exposure if host functions improperly implemented

**Remediation**:
```go
// pkg/wasm/host.go (FILE DOES NOT EXIST)

type SafeHostFunction struct {
    fn func([]byte) ([]byte, error)
}

func (rt *Runtime) ExportSafe(name string, fn func([]byte) ([]byte, error)) error {
    wrapped := func(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
        // Validate args are i32 pointers (not references)
        if len(args) != 2 || args[0].I32() < 0 || args[1].I32() <= 0 {
            return nil, wasmtime.NewTrap("invalid arguments")
        }
        // Copy input from module memory
        input, err := rt.memory.CopyFromModule(ptr, size)
        // Execute host function
        output, err := fn(input)
        // Copy output back to module memory
        outPtr, err := rt.memory.CopyToModule(output)
        return []wasmtime.Val{wasmtime.ValI32(int32(outPtr))}, nil
    }
    return rt.linker.FuncWrap(name, wrapped)
}
```

**Priority**: MEDIUM (memory safety guarantee, but wasmtime provides some protection)

**Estimated Effort**: 3-4 hours (file creation + SafeHostFunction integration + testing)

---

#### Gap 6: Aggregate Memory Limits (MEDIUM)
**Requirement**: RuntimePool with totalLimit, perModuleLimit enforcement (spec lines 142-190)

**Gap**: No RuntimePool implementation to enforce aggregate memory across multiple modules.

**Current Implementation**: Single module execution only, no pool-level tracking. Individual module limits enforced, but not aggregate.

**Impact**: MEDIUM - Multiple concurrent modules could collectively exhaust host memory

**Remediation**:
```go
// pkg/wasm/pool.go (FILE DOES NOT EXIST)

type RuntimePool struct {
    totalLimit    int64
    used          atomic.Int64
    perModuleLimit int64
    mu            sync.RWMutex
    modules       map[string]*Runtime
}

func (pool *RuntimePool) NewModule(name string, wasmBytes []byte) (*Runtime, error) {
    moduleSize := int64(len(wasmBytes))

    pool.mu.Lock()
    defer pool.mu.Unlock()

    if moduleSize > pool.perModuleLimit {
        return nil, &MemoryError{Code: ErrModuleTooLarge}
    }
    if pool.used.Load()+moduleSize > pool.totalLimit {
        return nil, &MemoryError{Code: ErrPoolExhausted}
    }
    pool.used.Add(moduleSize)
    // ... create runtime
}
```

**Priority**: MEDIUM (DoS protection improvement, but single-module limits in place)

**Estimated Effort**: 2-3 hours (file creation + pool implementation + testing)

---

#### Gap 7: Memory Copy-In/Copy-Out Semantics (LOW)
**Requirement**: Explicit CopyToModule, CopyFromModule for data exchange (spec lines 60-84)

**Gap**: No MemoryIsolation struct or explicit copy methods found. Wasmtime provides isolation implicitly, but spec requires explicit interface.

**Impact**: LOW - Wasmtime provides isolation, but interface not explicit per spec

**Remediation**: Create pkg/wasm/memory.go with MemoryIsolation, CopyToModule, CopyFromModule (2 hours effort)

**Priority**: LOW (quality improvement, wasmtime provides protection)

**Estimated Effort**: 2 hours

---

#### Gap 8: WASI Configuration Security (LOW)
**Requirement**: DefaultWASIConfig with Preopens only to temp directory (spec lines 244-262)

**Gap**: No WASIConfig implementation found. File system access via SandboxedFS but no explicit WASI configuration.

**Impact**: LOW - SandboxedFS provides directory restrictions, but WASI-specific config missing

**Remediation**: Create pkg/wasm/wasi.go with WASIConfig, Preopens, InheritEnv, InheritArgs (1 hour effort)

**Priority**: LOW (quality improvement, functionality exists via SandboxedFS)

**Estimated Effort**: 1 hour

---

## Risk Assessment

### Overall Risk Level: HIGH ❌

**Critical Gaps** (2):
1. Atomic signature + timestamp verification (CRITICAL - TOCTOU vulnerability)
2. Nonce-based replay prevention (CRITICAL - replay attack possible)

**Missing Files** (6):
1. pkg/wasm/verification.go (signature verification)
2. pkg/wasm/nonce.go (nonce replay prevention)
3. pkg/wasm/fuel.go (fuel metering)
4. pkg/wasm/host.go (host function memory safety)
5. pkg/wasm/pool.go (aggregate memory limits)
6. pkg/wasm/wasi.go (WASI configuration)

### Attack Scenarios Enabled by Gaps

**TOCTOU Attack** (Gap 1):
1. Attacker obtains valid signature for old benign module
2. Attacker replays signature with malicious module and old timestamp
3. Without atomic verification, signature check passes but timestamp not validated
4. **Impact**: Malicious code execution with valid signature

**Replay Attack** (Gap 2):
1. Valid signature issued for module at time T
2. Attacker replays same signature at time T+100 years
3. Without nonce check, signature accepted as valid
4. **Impact**: Unauthorized module execution with stale authorization

**DoS via Determinism** (Gap 3):
1. Malicious module uses SIMD operations (platform-specific)
2. Module runs on Intel with AVX2 (fast), ARM Neon (different results)
3. Without fuel metering, execution time varies
4. **Impact**: Non-deterministic output, potential DoS via CPU exhaustion

**Host Memory Exposure** (Gap 5):
1. Host function improperly exported without memory copy
2. WASM module receives raw pointer to host memory
3. Module reads/writes unprotected host memory
4. **Impact**: Host memory corruption, privilege escalation

---

## Comparison to Previous Sweeps

| Sweep | Spec | Conformance | Risk Level |
|-------|------|-------------|------------|
| #1 | DEPENDENCIES.md | 85% (17/20) | LOW |
| #2 | MCP-SERVER-IMPLEMENTATION.md | 100% (EXCEEDS SPEC) | NONE |
| #3 | OPS-ALERTING.md | 0% (16/16 reqs - ops enhancement) | LOW (optional) |
| #4 | **WASM-SANDBOX-SECURITY.md** | **60% (12/20)** | **HIGH (cr gaps)** |

**Trend**: First HIGH-RISK gap found. Previous sweeps had LOW or NO risk. WASM security has CRITICAL missing implementation.

---

## Recommendations

### Immediate Remediation (Security-Critical)

**Priority 1: Close TOCTOU Vulnerability** (8-10 hours total)
1. Create pkg/wasm/verification.go with AtomicVerifier (4-6 hours)
2. Implement ed25519 signature verification with timestamp checks
3. Create pkg/wasm/nonce.go with NonceStore (2-3 hours)
4. Add cleanup ticker for expired nonces (1 hour)
5. Write comprehensive tests for TOCTOU prevention (1-2 hours)

**Priority 2: Add Fuel Metering** (3-4 hours total)
1. Create pkg/wasm/fuel.go with FuelMeter (2-3 hours)
2. Integrate with wasmtime AddFuel() API
3. Define fuel costs per operation
4. Add tests for燃料 exhaustion

### Next Priority (Quality Improvements)

**Priority 3: Deterministic Configuration** (2 hours)
1. Update pkg/wasm/runtime.go:67 to use wasmtime deterministic config
2. Add config.SetWasmSIMD(false), etc.

**Priority 4: Aggregate Memory Limits** (2-3 hours)
1. Create pkg/wasm/pool.go with RuntimePool
2. Enforce aggregate limits across concurrent modules

**Priority 5: Host Function Safety** (3-4 hours)
1. Create pkg/wasm/host.go with SafeHostFunction
2. Host function wrapper for memory safety

**Total Estimated Effort**: 18-23 hours

**Timeline**: 2-3 days for full remediation (1 day for critical TOCTOU + 1-2 days for quality improvements)

---

## Production Readiness Impact

**Current Status**: ❌ NOT PRODUCTION-READY (WASM sandbox security requires these gaps fixed)

**Missing Security Invariants**:
- ❌ TOCTOU Invariant: Signature verification and timestamp checks are NOT atomic
- ❌ Replay Prevention Invariant: No nonce-based replay protection

**Invariants THAT ARE SECURE**:
- ✅ Memory Isolation: wasmtime enforced
- ✅ Capability: CapabilitySet implementation
- ✅ Resource Bounds: SandboxLimits enforced
- ✅ Determinism: IEEE 754 floating point (except fuel metering)

**Before Deployment**:
- ✅ MUST implement atomic signature + timestamp verification
- ✅ MUST implement nonce-based replay prevention
- ✅ SHOULD implement fuel metering for deterministic guarantees
- ✅ SHOULD improve deterministic configuration
- ✅ SHOULD add aggregate memory limits

---

## Files Verified

### Spec Requirements Analysis
- Lines 1-525: WASM-SANDBOX-SECURITY.md spec content
- Sections analyzed:
  - Memory Isolation Guarantees (lines 19-85)
  - Capability-Based System-Call Interposition (lines 197-316)
  - TOCTOU Prevention (lines 318-415)
  - Deterministic Execution Guarantees (lines 417-489)
  - Security Invariants (lines 491-499)

### Implementation Verification
- pkg/wasm/runtime.go: ✅ EXISTS (basic execution, limits enforcement)
- pkg/wasm/capability.go: ✅ EXISTS (full capability API)
- pkg/wasm/determinism.go: ✅ EXISTS (IEEE 754, DeterministicFloat, SortMapKeys)
- pkg/wasm/sandbox.go: ✅ EXISTS (SandboxedFS, directory restrictions)
- pkg/wasm/types.go: ✅ EXISTS (NetworkPolicy, SandboxLimits, Runtime)

### Implementation Missing (Critical Gaps)
- pkg/wasm/verification.go: ❌ DOES NOT EXIST (signature + timestamp verification)
- pkg/wasm/nonce.go: ❌ DOES NOT EXIST (replay prevention)
- pkg/wasm/fuel.go: ❌ DOES NOT EXIST (fuel metering)
- pkg/wasm/host.go: ❌ DOES NOT EXIST (host function safety)
- pkg/wasm/pool.go: ❌ DOES NOT EXIST (aggregate memory limits)
- pkg/wasm/wasi.go: ❌ DOES NOT EXIST (WASI configuration)
- pkg/wasm/memory.go: ❌ DOES NOT EXIST (MemoryIsolation, CopyToModule, CopyFromModule)

---

## Next Steps

1. **Create TODO tasks** for each gap identified
2. **Start with CRITICAL gaps** (TOCTOU prevention, replay attack prevention)
3. **Add tasks to Memory Bank progress.md** with estimated effort
4. **Implement atomic signature verification** first (8-10 hours)
5. **Continue with fuel metering** second (3-4 hours)
6. **Address quality improvements** (deterministic config, aggregate limits)
7. **Re-run spec conformance sweep** after remediation

---

**Sweep Methodology**:
- Random spec selection using `find specs -name "*.md" | shuf -n 1`
- Requirements extraction from spec sections
- Implementation verification via code inspection
- Gap assessment with priority assignment
- Risk evaluation and recommendation
- Security impact analysis

**Total Effort**: Sweep #4 complete (WASM-SANDBOX-SECURITY.md analysis)
**Specs Covered**: 4/228 (1.8%)
**Sweeps Completed**: 4
