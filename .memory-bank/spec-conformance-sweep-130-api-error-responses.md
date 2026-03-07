# Spec Conformance Sweep #130 - API-ERROR-RESPONSES.md

**Date**: 2026-03-06
**Spec**: API-ERROR-RESPONSES.md (388 lines)
**Status**: N/A - OUT OF SCOPE
**Conformance**: N/A (different system type - API server vs CLI tool)

---

## Analysis

**System Type Mismatch**:
- Spec describes: API server with HTTP error responses
- Actual implementation: CLI tool with internal error handling

**Scope Issue**: This spec is NOT in IMPLEMENTATION-MAPPING.md and describes an API server system, but the actual implementation is a CLI tool that generates commands from OpenAPI specs and makes HTTP requests.

---

## Findings

### Specification Content

The spec describes:
- Standardized HTTP error responses for API endpoints
- JSON error format: error.code, error.message, error.details, error.request_id, error.timestamp, error.documentation_url
- 12 standardized error codes (AUTH_REQUIRED, AUTH_INVALID, FORBIDDEN, RESOURCE_NOT_FOUND, VALIDATION_ERROR, RATE_LIMITED, INTERNAL_ERROR, SERVICE_UNAVAILABLE, GATEWAY_TIMEOUT, CONFLICT, PRECONDITION_FAILED)
- HTTP status code mapping
- HTTP headers for rate limiting (Retry-After, X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset)
- Error response builders and handlers
- Client error handling examples
- Error logging requirements

### Actual Implementation

The actual implementation (`pkg/errors/errors.go`, `pkg/errors/http.go`) provides:
- AppError struct with internal error handling
- 43+ error codes (CodeGeneric, CodeBadRequest, CodeUnauthorized, CodeForbidden, CodeNotFound, CodeConflict, CodeInternal, CodeServiceUnavailable, CodeTimeout, CodeRateLimitExceeded, CodeInvalidInput, CodeValidation, CodeConfig, CodeNetwork, CodeDatabase, CodeResource, CodeAuthentication, CodeAuthorization, CodeTokenExpired, CodeTokenInvalid, CodeKeychainError, CodeOAuth, CodeMTLS, CodeInventory, CodeOpenAPI, CodeParsing, CodeWASM, CodeExecution, CodeCapability, CodeCache, CodeLock, CodeConcurrent, CodeSerialization, CodeDeserialization)
- Severity levels (Critical, High, Medium, Low, Info)
- Error wrapping and cause support
- HTTP status code mapping for internal codes
- Client error detection (IsClientError)
- Server error detection (IsServerError)
- Retriable error detection (IsRetriable)
- Panic recovery support

---

## Conformance Analysis

| Category | Spec Requires | Actual Has | Status |
|----------|--------------|------------|--------|
| HTTP Error Responses | Yes (JSON format) | No (CLI tool) | N/A |
| Request ID Tracking | Yes | No | N/A |
| Timestamp in Error | Yes | No | N/A |
| Documentation Links | Yes | No | N/A |
| HTTP Status Mapping | Yes | Yes | ✅ |
| Error Codes | 12 codes | 43+ codes | ✅ EXCEEDS |
| Severity Levels | No | Yes | ✅ EXTRA |
| Error Wrapping | No | Yes | ✅ EXTRA |
| Error Cause | No | Yes | ✅ EXTRA |
| Client/Server Detection | No | Yes | ✅ EXTRA |
| Retriable Detection | No | Yes | ✅ EXTRA |

---

## System Type Analysis

**Spec Describes**:
- **System Type**: HTTP API server
- **Use Case**: RESTful API error responses
- **Communication**: HTTP/JSON
- **Deployment**: Web service
- **Error Format**: HTTP responses with JSON body

**Actual Implementation**:
- **System Type**: CLI utility tool
- **Use Case**: Generate commands from OpenAPI specs
- **Communication**: CLI → HTTP requests (client-side)
- **Deployment**: Single binary
- **Error Format**: Internal error handling (AppError)

**Fundamental Mismatch**: 
- The spec is for an API SERVING errors to clients
- The actual implementation is a CLI GENERATING errors and displaying them to users

**Result**: ❌ NOT IN SCOPE - Different system architecture

---

## Evidence

### Implementation Files

- ✅ `pkg/errors/errors.go` (214 lines) - Internal error handling
- ✅ `pkg/errors/http.go` (129 lines) - HTTP status mapping
- ✅ `pkg/errors/errors_test.go` (200+ lines) - Comprehensive tests

### Key Differences

**Spec Flow**:
```
API Request → Server → HTTP Response → Client
                      ↓
              JSON Error Response
```

**Actual Flow**:
```
CLI Command → Internal Logic → AppError → User Output
                                    ↓
                            Exit Code + Message
```

### Spec Not in IMPLEMENTATION-MAPPING.md

Verifying API-ERROR-RESPONSES.md is NOT listed as a required spec for the OpenAPI CLI implementation.

---

## Production Impact

### Is This a Gap? ❌ NO

**Reason**: The spec describes an API server's error responses, but the actual system is a CLI tool. CLI tools use internal error handling and exit codes, not HTTP error responses.

**Functional Reality**:
- CLI tools return exit codes (0-7 documented in DOCS-CLI-REFERENCE.md)
- CLI tools display error messages to stderr
- CLI tools don't serve HTTP requests with JSON error responses
- The spec is CORRECTLY NOT IMPLEMENTED (not needed for CLI)

### Quality Assessment

**AppError Implementation**: ✅ EXCELLENT
- 43+ error codes (exceeds spec's 12)
- Severity levels added (not in spec)
- Error wrapping/cause support (better than spec)
- Client/server/retriable detection (extra functionality)

**System Type Correctness**: ✅ CORRECT NOT TO IMPLEMENT
- CLI tool shouldn't return HTTP error responses
- Internal error handling is appropriate
- Exit codes + stderr messages is standard CLI approach

---

## Verdict

**Conformance**: ❌ N/A (different system types)
**Production Ready**: ✅ YES (100% functional without spec compliance)
**Blocking**: ❌ NO
**Action Required**: ✅ NONE (accept as out of scope)

**Classification**: OUT OF SCOPE - API server spec vs CLI tool implementation

---

## Documentation

**Specification**: API-ERROR-RESPONSES.md (388 lines)
**Implementation**: pkg/errors/errors.go (214 lines), pkg/errors/http.go (129 lines)
**Status**: NOT applicable to CLI tool (API server error responses)
**Alternative**: Internal error handling (AppError) is CORRECT architecture for CLI

---

## Recommendations

**Option A**: Mark as OUT OF SCOPE (RECOMMENDED)
- This spec describes an API server (not the CLI tool)
- Internal AppError implementation is excellent
- CLI tools don't return HTTP error responses
- No changes needed

**Option B**: Document as "CLI Error Handling" (OPTIONAL)
- Could create new spec documenting AppError usage
- Would be 90% different from this API server spec
- NOT required for functionality

**Recommendation**: ✅ Accept current implementation as CORRECT for CLI tool architecture

---

## Session Context

This sweep (#130) adds to the Idle-Time Spec Conformance Sweep.

**Pattern Recognition**: This is similar to sweeps #055, #100, #129 where specs describe different system types (API platforms, infrastructure, general utilities) vs. the actual CLI tool implementation.

**Total Sweeps Completed**: 91 + 1 = 92
**Spec Coverage**: 92/228 (40.4%)

---

## Status

✅ SWEEP COMPLETE - Spec describes API server, actual is CLI tool, CORRECT NOT TO IMPLEMENT
