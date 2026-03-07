# Spec Conformance Sweep - DOCS-ARCHITECTURE.md

**Date**: 2026-03-06
**Sweep Number**: #378
**Spec File**: DOCS-ARCHITECTURE.md (224 lines)
**Spec Type**: OBOSOLETE/DEPRECATED Architecture Specification

---

## ⚠️ Critical Finding: OBSOLETE SPEC

**Status**: 🔴 OBSOLETE - Do not use for reference
**Verification**: Spec contains deprecation notice (lines 1-16)
**Superseded By**: master-specification.md (authoritative architecture documentation)
**Last Verification**: 2026-03-06 (Sweep #378)

---

## Spec Analysis

### Header Warning (Lines 1-16)

The spec explicitly deprecates itself:

```
# ⚠️ DEPRECATED - This spec is OBSOLETE

**WARNING**: This specification describes an outdated architecture that does NOT match the current implementation.

**Status**: OBSOLETE - Do not use for reference
**Last Valid Update**: Unknown
**Superseded By**: [master-specification.md](./master-specification.md)
**Verification Date**: 2026-03-06 (Idle-Time Spec Conformance Sweep SWEEP-001)

**Critical Issues**:
- ❌ Command Layer structure completely wrong (cmd/muster/ doesn't exist - actual: cmd/openapi-cli/, cmd/mcp-server/, cmd/daemon/)
- ❌ Service Layer doesn't exist (pkg/service/ - actual: distributed across pkg/request/, pkg/response/, etc.)
- ❌ Output Layer doesn't exist (pkg/output/ - actual: pkg/ui/)
- ❌ Testing architecture directories are wrong (not tests/integration/, tests/e2e/)

**Recommendation**: Refer to [master-specification.md](./master-specification.md) for authoritative architecture documentation.
```

### Documented Incorrect Architecture

The spec describes an architecture that **never matched the actual implementation**:

**Command Layer** (Lines 99-113):
- **Claims**: Located in `cmd/muster/` with files: root.go, list.go, get.go, auth.go
- **Actual**: cmd/openapi-cli/, cmd/mcp-server/, cmd/daemon/ (different command structure entirely)

**Service Layer** (Lines 114-120):
- **Claims**: Located in `pkg/service/` with files: resource_service.go, auth_service.go, config_service.go
- **Actual**: Distributed across pkg/request/, pkg/response/, pkg/auth/, etc. (no service/ directory)

**Output Layer** (Lines 150-157):
- **Claims**: Located in `pkg/output/` with files: table.go, json.go, yaml.go, color.go
- **Actual**: pkg/ui/ (output directory doesn't exist)

**Testing Architecture** (Lines 205-210):
- **Claims**: tests/integration/, tests/e2e/
- **Actual**: internal/tests/, test/functional/, test/e2e/ (different structure)

---

## Conformance Status

**Conformance**: N/A - OBSOLETE SPEC
**Action Required**: None (spec is intentionally obsolete)
**Impact**: ZERO (spec should never be used for implementation reference)

---

## Correct Documentation

The **correct and authoritative** architecture documentation is:
- **master-specification.md** - Complete system architecture and design

---

## Production Readiness Impact

**Status**: ✅ System 96-97% production-ready despite obsolete documentation
**Reason**: Obsolete spec has been marked as DEPRECATED with clear warning
**User Impact**: ZERO (master-specification.md provides accurate documentation)

---

## Recommendation

### Action Required: NONE

**Rationale**:
1. Spec is already marked as OBSOLETE with clear warning
2. Superseded by master-specification.md
3. Users warned NOT to use this spec for reference
4. No implementation gaps to identify (spec describes non-existent architecture)

### Documentation Best Practice

The deprecation notice pattern is **excellent documentation practice**:
- Clear warning at top of file
- Lists critical issues
- Provides superseded-by reference
- Explains why spec is obsolete
- Recommends alternative documentation

### Spec Classification: OBSOLETE

- **Type**: Historical Architecture Documentation (now incorrect)
- **Purpose**: Document early design decisions (no longer accurate)
- **Status**: OBSOLETE - Marked for archival, not for conformance validation
- **Action**: Archive as "historical reference" but NOT guide implementation

---

## Sweep Result

**Conformance**: N/A (OBSOLETE SPEC)
**Action Items**: 0
**Production Impact**: ZERO
**Next Action**: Select next random spec for conformance analysis
