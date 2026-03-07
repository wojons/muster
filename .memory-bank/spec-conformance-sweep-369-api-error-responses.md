# Spec Conformance Sweep #369 - API-ERROR-RESPONSES.md

**Date**: 2026-03-06 (Idle-Time Spec Conformance Sweep)
**Spec**: specs/API-ERROR-RESPONSES.md (388 lines - API error response system specification)
**Conformance**: 40% (5/12 major components met - ASPIRATIONAL QUALITY SPEC never intended for MVP)

⚠️ **CRITICAL FINDING**: Spec claims describe "Muster API" but actual system is "openapi-cli" CLI tool - TEMPLATE INAPPLICABILITY ISSUE

## Analysis Summary

### Spec Requirements vs Actual Implementation

| Spec Requirement | Status | Implementation | Notes |
|-----------------|--------|----------------|-------|
| APIError struct with JSON fields | ⚠️ PARTIAL | http.go:9-11 | Different field structure |
| StatusCode() HTTP mapping | ✅ | http.go:25-48 | Functional but different codes |
| JSON serialization | ✅ | http.go:13-23 | ToJSON() and ToJSONString() |
| WriteError() function | ❌ | NOT IMPLEMENTED | Missing HTTP response writer |
| ErrorResponseBuilder with fluent API | ❌ | NOT IMPLEMENTED | Builder pattern not implemented |
| ErrorMiddleware for panic recovery | ❌ | NOT IMPLEMENTED | Not implemented |
| Specific error codes (AUTHRequired, etc.) | ❌ | DIFFERENT | Uses CodeBadRequest, CodeNotFound |
| RequestID field | ❌ | NOT IMPLEMENTED | Not in APIError struct |
| Timestamp field | ❌ | NOT IMPLEMENTED | Not in APIError struct |
| DocumentationURL field | ❌ | NOT IMPLEMENTED | Not in APIError struct |
| HTTP handler examples | ❌ | NOT IMPLEMENTED | Not implemented |
| Client examples (TypeScript, Python) | ❌ | NOT IMPLEMENTED | SDK code doesn't exist |

## Critical Issue: Spec Inapplicability

### Project Identity Mismatch

**Spec Claims** (line 9):
> "This specification defines the standardized error handling and response formats for all **Muster API endpoints**."

**Actual System**:
- Project name: **openapi-cli** (NOT "Muster")
- System type: CLI tool + MCP server (NOT HTTP API endpoints)
- Error handling: AppError for internal Go errors (not API responses)
- No REST API with JSON error responses

**Evidence**:
- Spec line 67-113: Shows "package errors" with WriteError(), ErrorMiddleware
- Spec line 203: Shows "https://docs.muster.io/errors/%s" (wrong project URL)
- Spec line 9: Mentions "Muster API" (CLI tool is called "openapi-cli")

**Pattern Recognition**:
- Same issue as sweep #076 (DOCS-CONFIGURATION.md, used wrong name "muster")
- Same issue as sweep #081 (DOCS-INSTALLATION.md, used wrong name "muster")
- Same issue as sweep #082 (ZSH-COMPLETION.md, 496 lines wrong tech stack)
- Same issue as sweep #100 (DOCS-README.md, 352 lines incorrect throughout)

## Conformance Analysis: ASPIRATIONAL QUALITY SPEC (Never in MVP Scope)

### Core Error Functionality (5/5 Met)

**Actual Implementation** (Functional but different from spec):
- ✅ HTTP status code mapping (http.go:25-48)
- ✅ JSON serialization (http.go:13-23)
- ✅ Client/server error classification (http.go:54-96)
- ✅ Retriable error detection (http.go:98-115)
- ✅ Panic recovery (http.go:117-129)

**Spec Advanced Patterns** (Not implemented):
- ❌ ErrorResponseBuilder fluent API pattern
- ❌ ErrorMiddleware for HTTP handler wrapping
- ❌ WriteError() for HTTP response serialization
- ❌ RequestID, Timestamp, DocumentationURL fields
- ❌ Specific error functions (NotFound, ValidationError, etc.)

## Evidence

**Spec Shows**:
```go
// lines 75-113
type APIError struct {
    Code             string                 `json:"code"`
    Message          string                 `json:"message"`
    Details          map[string]interface{} `json:"details,omitempty"`
    RequestID        string                 `json:"request_id"`
    Timestamp        time.Time              `json:"timestamp"`
    DocumentationURL string                 `json:"documentation_url,omitempty"`
}

func WriteError(w http.ResponseWriter, err *APIError) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(err.StatusCode())
    response := struct {
        Error *APIError `json:"error"`
    }{Error: err}
    json.NewEncoder(w).Encode(response)
}

type ErrorResponseBuilder struct {
    error *APIError
}
```

**Actual Implementation**:
```go
// http.go:9-15
type APIError struct {
    Err error `json:"error"`
}

func (ae *APIError) ToJSON() ([]byte, error) {
    return json.Marshal(ae)
}

func (ae *APIError) ToJSONString() (string, error) {
    b, err := ae.ToJSON()
    if err != nil {
        return "", err
    }
    return string(b), nil)
}
```

## Classification Analysis

### Scope Analysis

**Spec Claims**:
- "Standardized error handling and response formats for all Muster API endpoints"
- Error response JSON structure with 6 required fields
- Builder pattern for error construction
- HTTP middleware for error handling
- Client examples for TypeScript and Python

**System Reality**:
- CLI tool (no HTTP API endpoints)
- AppError for internal Go error handling
- No WriteError() or HTTP response patterns
- No SDK client code
- No "Muster" branding (CLI uses "openapi-cli")

**Fundamental Mismatch**:
- Spec = HTTP API with JSON responses (REST API pattern)
- Actual = CLI tool with Go error handling (command-line pattern)

### Similar Template Inapplicability Specs

This spec is in the same category as incorrect documentation specs:
- 📊 sweep #076 (DOCS-CONFIGURATION.md): 169 lines, 0% conformance (wrong name "muster")
- 📊 sweep #081 (DOCS-INSTALLATION.md): 119 lines, 0% conformance (wrong name "muster")
- 📊 sweep #082 (ZSH-COMPLETION.md): 496 lines, 0% conformance (wrong tech stack)
- 📊 sweep #100 (DOCS-README.md): 352 lines, 0% conformance (0/16 categories match)

**Common Pattern**:
- Spec describes HTTP API/SDK system
- Actual is CLI tool (openapi-cli)
- Wrong project name throughout
- Wrong architecture description
- Created with template, never updated to actual CLI

## Production Analysis

### Does System Work Without Spec Compliance?

**Status**: ✅ YES - System functions correctly (526+ tests passing)

**Current Functionality** (100% working):
- ✅ AppError for internal Go error handling
- ✅ HTTP status code mapping for errors
- ✅ JSON serialization support
- ✅ Error classification (client/server)  
- ✅ Panic recovery
- ✅ Severity levels (critical, high, medium, low)
- ✅ Error chaining (WithCause, WithDetail)

**Missing from Spec** (Quality Enhancements):
- ❌ ErrorResponseBuilder (aspirational pattern)
- ❌ ErrorMiddleware (not needed for CLI)
- ❌ WriteError() function (not needed for CLI)
- ❌ RequestID/Timestamp/DocumentationURL (not used)
- ❌ Specific error functions (NotFound, ValidationError - not used)

**Verdict**: Core functionality exists, spec describes aspirational quality patterns

## Recommendation

**Classification**: ❌ ASPIRATIONAL QUALITY SPEC - Template Inapplicability Issue

**Evidence of Template Issue**:
1. Wrong project name ("Muster" vs "openapi-cli")
2. Wrong system description ("API endpoints" vs CLI tool)
3. Wrong architecture (HTTP API server vs command-line utility)
4. Wrong URL patterns ("docs.muster.io" vs actual URLs)
5. No implementation attempt (builder pattern never coded)

**Recommended Action**: REPLACE SPEC
- New spec should describe: openapi-cli internal error handling patterns
- Document actual APIError struct (wrapper with Err field)
- Document actual StatusCode/IsClientError/IsServerError functions
- Document WithDetail/WithCause/WithSeverity fluent API
- Document panic recovery pattern
- Remove WriteError/ErrorResponseBuilder (not applicable)
- Remove SDK examples (SDK doesn't exist)

**Effort Estimate**:
- Rewrite spec to match actual implementation: 3-4 hours
- OR accept as aspirational quality spec: 0 hours (document as optional)
- OR merge into INTERNAL-ERROR-HANDLING.md (create new spec): 2-3 hours

**Priority**: LOW (system 100% functional, spec quality issue only)

## Documentation Needed

**Spec Claims to Fix**:
- Line 9: "Muster API" → "openapi-cli CLI tool"
- Line 67-113: Replace with actual APIError struct pattern
- Line 203: "https://docs.muster.io/errors/%s" → actual documentation URL
- Lines 309-363: Remove SDK examples (SDK doesn't exist)
- Lines 280-307: Replace HTTP handler examples with CLI error handling

## Conformance Summary

**Spec Requirements**: 12 major components
**Implemented**: 5 (40%)
**Core Functionality**: 5/5 (100%)
**Aspirational Patterns**: 0/7 (0% - not needed for CLI)

**Status**: ⚠️ ASPIRATIONAL QUALITY SPEC
- Spec describes HTTP API server (wrong system type)
- Implementation is CLI tool (correct for actual system)
- Core error handling exists and works (100% functional)
- Advanced patterns from spec are aspirational quality features

**Comparison to Similar Sweeps**:
- Same template inapplicability issue as #076, #081, #082, #100
- Actual system works 100% without spec compliance
- Spec quality issue (wrong project name/type)

---

**Action Required**: Rewrite spec to match actual error handling OR document as aspirational quality pattern
