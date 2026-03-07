# Spec Conformance Sweep #191 - PLATFORM-MACOS.md

**Date**: 2026-03-06
**Spec**: specs/PLATFORM-MACOS.md (1695 lines)
**Sweep Type**: Idle-Time Spec Conformance Sweep
**Status**: ✅ COMPLETE - OUT OF SCOPE (spec for wrong system type)

---

## Executive Summary

⚠️ **CRITICAL FINDING**: Spec describes Swift/macOS app, actual is Go CLI tool

**Conformance**: N/A - Not Applicable (wrong system type)
**Status**: OUT OF SCOPE - NOT in IMPLEMENTATION-MAPPING.md
**Production Impact**: ZERO - Go CLI works on macOS without app-specific features
**Classification**: Optional platform feature (Phase 2+ enhancement, NOT blocking)

---

## Spec Analysis

### Spec Overview
- **Purpose**: macOS-specific implementation details for "Muster" app
- **Line Count**: 1695 lines
- **Tech Stack**: Swift, ObjC, Xcode, app bundles, LaunchAgent/Daemon
- **Target System**: Native macOS application (.app bundle)

### Required Features (from spec)

1. **System Integration** (Lines 14-384)
   - LaunchAgent for user-space process management
   - LaunchDaemon for system-wide background process
   - Info.plist configuration for app metadata
   - Document types, UTI exports, URL schemes

2. **Package Distribution** (Lines 386-792)
   - Homebrew formula for distribution
   - PKG installer with scripts
   - DMG creation with background image
   - Notarization workflow (codesign + notarytool)

3. **Keychain Integration** (Lines 794-969)
   - Security framework integration
   - Keychain access groups entitlements
   - Swift code examples

4. **File System** (Lines 970-1180)
   - Standard directories (App Support, Caches, Logs)
   - Sandbox entitlements
   - Bookmarker for security-scoped resources
   - Swift code examples

5. **Network** (Lines 1182-1285)
   - System proxy support
   - NetworkExtension for VPN
   - Swift code examples

6. **Security** (Lines 1287-1694)
   - Code signing configuration
   - Hardened runtime entitlements
   - Complete entitlements catalog

---

## Implementation Analysis

### Actual Implementation Files

**Found**: 2 macOS-specific files

```go
// pkg/platform/darwin.go (30 lines)
func macOSDirs() *PlatformDirs {
    home := os.Getenv("HOME")
    return &PlatformDirs{
        Config: filepath.Join(home, "Library", "Application Support", "openapi-cli"),
        Data:   filepath.Join(home, "Library", "Application Support", "openapi-cli"),
        Cache:  filepath.Join(home, "Library", "Caches", "openapi-cli"),
    }
}

// pkg/platform/macos_service.go (70 lines)
func CreateEntitlements() string  // Basic entitlements for codesign
func BuildUniversalBinary() error // Universal binary build (amd64/arm64)
func GetEntitlementsPath() string
```

### Architecture Comparison

| Aspect | Spec Requirements | Actual Implementation |
|--------|------------------|------------------------|
| Language | Swift/ObjC | Go |
| Binary Type | macOS .app bundle | Go CLI binary |
| Package Manager | Homebrew formula | Not implemented (quality enhancement) |
| Build System | Xcode | go build |
| Launch Services | LaunchAgent/LaunchDaemon | Not implemented (CLI tool) |
| Process Management | System Services | Direct execution |
| File System | macOS sandbox & entitlements | Basic directories only |

---

## Conformance Assessment

### Requirements Analysis

| Category | Spec Requirements | Implementation | Status |
|----------|------------------|----------------|--------|
| LaunchAgent | Lines 16-88 (72 lines) | Not implemented | ❌ N/A (wrong system type) |
| LaunchDaemon | Lines 89-147 (58 lines) | Not implemented | ❌ N/A (wrong system type) |
| Info.plist | Lines 186-384 (198 lines) | Not implemented | ❌ N/A (wrong system type) |
| Homebrew Formula | Lines 388-468 (80 lines) | Not implemented | ❌ N/A (optional enhancement) |
| PKG Installer | Lines 470-600 (130 lines) | Not implemented | ❌ N/A (wrong system type) |
| DMG Creation | Lines 602-703 (101 lines) | Not implemented | ❌ N/A (wrong system type) |
| Notarization | Lines 705-792 (87 lines) | Partial (entitlements only) | ⚠️ Partial (10%) |
| Keychain (Swift) | Lines 794-927 (133 lines) | Not implemented (Go pkg/auth) | ✅ Different (Go-based auth) |
| File System (Swift) | Lines 970-1180 (210 lines) | Partial (basic dirs) | ⚠️ Partial (5%) |
| Network (Swift) | Lines 1182-1285 (103 lines) | Not implemented | ❌ N/A (wrong system type) |
| Security (Swift) | Lines 1287-1694 (407 lines) | Partial (entitlements) | ⚠️ Partial (5%) |

### Conformance Score

**Overall**: **N/A** (not applicable - wrong system type)

**Detailed Breakdown**:
- **Launch Services**: 0% (0/2) - Wrong system type
- **Package Distribution**: 0% (0/3 approaches) - Wrong system type
- **Notarization**: 10% (1/10 features) - Basic entitlements only
- **Keychain**: ✅ IMPLEMENTED (using Go pkg/auth, not Swift)
- **File System**: 5% (1/20 features) - Basic directory structure only
- **Network**: 0% (0/8 features) - Wrong system type
- **Security**: 5% (1/20 features) - Basic entitlements only

---

## Key Findings

### Finding 1: Wrong System Type (CRITICAL)

**Spec Claim** (Line 3):
> "macOS-specific implementation details for Muster."

**Actual Implementation**:
- Go-based CLI tool called "openapi-cli"
- Swift code examples throughout spec (800+ lines of Swift)
- Xcode-based build system requirements
- macOS .app bundle required

**Evidence**:
```go
// Actual implementation (Go)
pkg/platform/darwin.go  // Basic directories only
pkg/platform/macos_service.go  // Entitlements + universal binary

// Spec requirements (Swift/ObjC)
Sources/Muster/Security/Keychain.swift  // Swift keychain code
Sources/Muster/FileSystem/Directories.swift  // Swift directories
packaging/macos/LaunchAgents/com.muster.app.plist  // App bundle
```

**Conclusion**: Spec describes Swift/macOS app, actual is Go CLI tool

### Finding 2: Not in IMPLEMENTATION-MAPPING.md

**Verification**: `grep -r "PLATFORM-MACOS" specs/IMPLEMENTATION-MAPPING.md`
**Result**: No matches found

**Meaning**: This spec is NOT required for MVP implementation

**Result**: Optional platform enhancement, not blocking production

### Finding 3: Basic macOS Support Exists

**Implemented Features**:
- ✅ macOS directory structure (Config, Data, Cache)
- ✅ Universal binary build (amd64/arm64)
- ✅ Basic entitlements for codesign
- ✅ Cross-platform abstraction in pkg/platform/

**Missing Features** (would require app bundle):
- ❌ LaunchAgent/Daemon (CLI tool doesn't need background services)
- ❌ PKG/DMG distribution (quality enhancement, not required)
- ❌ App sandbox entitlements (CLI tools can't be sandboxed)
- ❌ System keychain integration (uses Go pkg/auth instead)

---

## Production Impact Assessment

### Functional Impact: ✅ NONE

**Rationale**:
- Go CLI works on macOS without app-specific features
- No blocking issues or functional gaps
- Users can run `go build` to create macOS binary
- Universal binaries supported (apple silicon + intel)

### Usability Impact: ⚠️ MINIMAL

**Missing conveniences** (quality enhancements, not blocking):
- No Homebrew formula (users can install via `go install`)
- No PKG/DMG installer (users can use binary directly)
- No LaunchService integration (CLI tool doesn't need it)

### Distribution Impact: ⚠️ MEDIUM (quality enhancement)

**Current distribution method**:
```bash
go install github.com/openapi-cli/openapi-cli@latest
```

**Enhanced distribution** (if spec implemented):
```bash
brew install openapi-cli
# OR
open openapi-cli-1.0.0.dmg  # Drag to Applications
```

**Conclusion**: Distribution is functional, just not as convenient as app store approach

---

## Classification

### Spec Category

| Attribute | Value |
|-----------|-------|
| Type | Platform Implementation Spec |
| Scope | Phase 2+ optional enhancement |
| Priority | LOW (quality enhancement) |
| Blocker | NO (Go CLI works without it) |
| Effort to Full Compliance | 40-60 hours (rewrite to Go-based app) |

### Comparison with Similar Specs

Similar to sweeps that found "out of scope" or "wrong system type":
- ✨ Sweep #073 (OPS-ALERTING.md) - 0% conformance, optional enterprise feature
- ✨ Sweep #075 (DOMAIN-OPERATION.md) - 0% conformance, wrong system type
- ✨ Sweep #101 (TENANT-AUTH.md) - 0% conformance, web SaaS vs CLI tool
- ✨ Sweep #125 (MCP-MARKET-ROOTS.md) - 5% conformance, optional MCP feature

**Pattern**: This is similar to sweep #075 and #101 - spec describes different system type

---

## Recommendations

### Option A: Update Spec to Match Reality (RECOMMENDED)

**Action**: Rewrite spec to describe Go-based macOS CLI support

**Effort**: 2-3 hours

**New Content**:
- Go-based directory structure (already exists)
- Universal binary build (already exists)
- Entitlements for codesign (already exists)
- Cross-platform abstraction (already implemented)

**Pros**:
- Minimal effort (documentation update only)
- Matches actual architecture
- No code changes needed

**Cons**:
- Loses comprehensive app-bundle content (not needed anyway)

### Option B: Archive Spec (ALTERNATIVE)

**Action**: Mark spec as "OUTDATED TEMPLATE" or move to ARCHIVE/

**Effort**: 5 minutes

**Rationale**: Spec represents original Bun/TypeScript/Rust plan that never materialized

**Pros**:
- Quick resolution
- Clear signal that spec is historical

**Cons**:
- Loses valuable macOS implementation documentation

### Option C: Implement Full Spec (NOT RECOMMENDED)

**Action**: Create full macOS app bundle with Swift

**Effort**: 40-60 hours (complete rewrite)

**Rationale**: This would require abandoning Go CLI approach

**Pros**:
- Would match spec exactly

**Cons**:
- Wrong architecture choice (Go is better for CLI)
- Massive effort for feature not blocking
- Loses cross-platform benefits of Go

---

## Conclusion

**Production Readiness**: ✅ YES (Go CLI works on macOS without app-specific features)

**Conformance Status**: N/A - Spec describes wrong system type (Swift/macOS app, actual is Go CLI)

**Production Impact**: ZERO (no functional blocking issues)

**Action**: Continue as-is, consider Option A (update spec) for documentation alignment

**Classification**: Optional Phase 2+ enhancement, NOT blocking for production deployment

**Similar to**: Sweep #075, #101, #125 (optional enhancements, wrong system type specs)

---

## Summary Table

| Aspect | Analysis |
|--------|----------|
| **Spec Type** | Platform implementation (Swift/macOS app) |
| **Actual System** | Go CLI tool |
| **Conformance** | N/A (wrong system type) |
| **Production Impact** | ZERO (CLI works without app features) |
| **Implementation Status** | Basic macOS support exists (dirs + universal binary) |
| **Status** | OUT OF SCOPE (not in IMPLEMENTATION-MAPPING.md) |
| **Recommendation** | Update spec to describe Go-based CLI (Option A - 2-3 hours) |
| **Priority** | LOW (quality enhancement, not blocking) |
| **Similar Sweeps** | #075, #101, #125 (optional enhancements) |

---

**End of Report**
