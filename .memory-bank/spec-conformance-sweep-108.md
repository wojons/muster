# Spec Conformance Sweep #108 - HTTP-CANONICALIZATION

**Date**: 2026-03-05
**Spec Analyzed**: specs/HTTP-CANONICALIZATION.md
**Conformance**: 0% (0/3 requirements met)
**Status**: NOT REQUIRED for MVP (backlog quality enhancement)

---

## Specification Details

**File**: specs/HTTP-CANONICALIZATION.md (85 lines)
**Purpose**: HTTP canonicalization for determinism
**Category**: Bible Standard determinism

---

## Requirements Analysis

### Requirement 1: CanonicalizeURL Function
**Spec Definition** (lines 15-38):
```go
func CanonicalizeURL(rawURL string) (string, error)
```

Required Features:
- Lowercase scheme and host
- Remove default ports (http=80, https=443)
- Sort query parameters

**Implementation Status**: ❌ NOT IMPLEMENTED

**Search Results**:
- grep `CanonicalizeURL`: No files found
- grep `canonicalize.*url`: No files found

**Evidence**: No canonicalize URL functionality exists in codebase.

---

### Requirement 2: CanonicalizeHeaders Function
**Spec Definition** (lines 44-60):
```go
func CanonicalizeHeaders(headers http.Header) http.Header
```

Required Features:
- Lowercase header names
- Sort header values

**Implementation Status**: ❌ NOT IMPLEMENTED

**Search Results**:
- grep `CanonicalizeHeaders`: No files found
- grep `canonicalize.*header`: No files found

**Evidence**: No header canonicalization functionality exists.

---

### Requirement 3: CanonicalizeRequest Function
**Spec Definition** (lines 66-80):
```go
func CanonicalizeRequest(req *http.Request) (*http.Request, error)
```

Required Features:
- Calls CanonicalizeURL on req.URL
- Calls CanonicalizeHeaders on req.Header

**Implementation Status**: ❌ NOT IMPLEMENTED

**Search Results**:
- grep `CanonicalizeRequest`: No files found
- grep `canonicalize.*request`: No files found

**Evidence**: No request canonicalization functionality exists.

---

## Implementation Reality

**Existing HTTP Infrastructure**:
- pkg/request/builder.go (423 lines) - HTTP request builder
  - Builder struct with baseURL, path, method, queryParams, headers
  - NewBuilder() - creates new builder
  - WithQueryParam(), WithHeader(), WithBody() methods
  - Build() - creates http.Request
  - Execute() - executes HTTP request
  - ExecuteWithResult() - executes and returns result
  - NO canonicalization anywhere in builder

**Key Finding**: Builder builds requests but does NOT canonicalize URLs or headers.

---

## Scope Analysis

### Implementation-Mapping.md Check
**Required Specs**: 13 specs listed in Sections 1.1-1.3
- HTTP-CANONICALIZATION.md: ❌ NOT LISTED
- Conclusion: Not required for MVP implementation

### TODO.md Check
**Location**: TODO.md line 374
**Context**: "### 2.4 Utilities & Helpers" → "Add HTTP utilities per specs/HTTP-CANONICALIZATION.md"
**Phase**: Phase 3: Platform Abstraction
**Category**: Utilities & Helpers (backlog item)

### master-specification.md Check
**Referenced Under**: "Determinism" (line 911)
**Purpose Listed**: Bible Standard determinism guarantee
**Note**: Quality enhancement, not blocker

---

## Impact Analysis

**Functional Impact**: ZERO ✅
- System builds HTTP requests correctly without canonicalization
- Builder pattern works end-to-end
- No user-facing issues without canonicalization

**Bible Standard Impact**: PARTIAL ⚠️
- System not 100% Bible Standard compliant (no HTTP canonicalization)
- Non-deterministic URLs/headers across different AI systems
- Optional enhancement quality goal

**Production Readiness**: YES ✅
- System functions correctly
- No blocking issues
- Quality enhancement only

---

## Conformance Summary

| Requirement | Status | Evidence |
|-------------|--------|----------|
| 1. CanonicalizeURL | ❌ NOT IMPLEMENTED | No files found |
| 2. CanonicalizeHeaders | ❌ NOT IMPLEMENTED | No files found |
| 3. CanonicalizeRequest | ❌ NOT IMPLEMENTED | No files found |

**Overall Conformance**: 0% (0/3 requirements met)

---

## Recommendation

**Classification**: Quality Enhancement (NOT REQUIRED for MVP)

**Priority**: LOW (backlog item, Phase 3+)

**Decision**: ✅ NOT REQUIRED - System production-ready without this feature

**Rationale**:
1. Not in IMPLEMENTATION-MAPPING.md (required specs)
2. Backlog item in TODO.md (not blocker)
3. Bible Standard quality goal (optional enhancement)
4. System functions correctly without canonicalization
5. Zero production impact

**Estimated Effort**: 4-6 hours (if implementing later)
- Create pkg/request/canonical.go (2h)
- Implement CanonicalizeURL() (1h)
- Implement CanonicalizeHeaders() (1h)
- Implement CanonicalizeRequest() (1h)
- Add unit tests (1h)

**When to Implement**: Phase 2+ enhancement (improving Bible Standard determinism)

---

## Conformance Verdict

**Status**: ✅ OUT OF SCOPE - BACKLOG QUALITY ENHANCEMENT

**Similar Specs**:
- TIME-ZONE-SPEC.md (also in TODO.md backlog)
- CRYPTO-IMPLEMENTATION.md (also in TODO.md backlog)
- Platform abstraction utilities (Phase 3 enhancements)

**Production Impact**: NONE
**Bible Standard Impact**: OPTIONAL QUALITY GOAL
**Blocking Issue**: NO

---

## Documentation

**Spec File**: specs/HTTP-CANONICALIZATION.md
**TODO.md Reference**: Line 374 (Phase 2.4 Utilities & Helpers)
**master-specification.md Reference**: Line 911 (Determinism section)
**IMPLEMENTATION-MAPPING.md**: NOT listed (optional enhancement)

---

## Next Steps (Optional - Phase 2+)

If improving Bible Standard determinism:

1. Implement CanonicalizeURL()
   - Add pkg/request/canonical.go
   - Lowercase scheme/host
   - Remove default ports
   - Sort query parameters

2. Implement CanonicalizeHeaders()
   - Lowercase header names
   - Sort header values

3. Implement CanonicalizeRequest()
   - Integrate URL + header canonicalization
   - Add unit tests

4. Update Builder to canonicalize (optional)
   - Add canonicalization to Build()
   - Maintain backward compatibility

**Note**: NOT REQUIRED for MVP or production deployment. This is a quality enhancement for Bible Standard determinism compliance.
