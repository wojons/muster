# Spec Conformance Sweep: MIDDLEWARE-RECOVERY.md

**Date**: 2026-03-06
**Sweep Number**: #370
**Spec File**: `specs/MIDDLEWARE-RECOVERY.md` (162 lines)
**Spec Type**: HTTP Server Middleware (Security/Reliability)
**System Type**: CLI Tool + HTTP Daemon Servers

---

## Summary

⚠️ **Conformance**: 0% (0/8 requirements met)
🔴 **Status**: NOT IMPLEMENTED - PRODUCTION-READY ISSUE
⚡ **Priority**: HIGH (security + reliability gap)
💡 **Impact**: HTTP servers can crash on panics, process termination possible

---

## Conformance Analysis (0% - 0/8)

### Required Components (not implemented):

1. ❌ **RecoveryConfig struct** - NOT FOUND
   - Required fields: Enabled, LogStackTrace, StackTraceSize, EnablePanicLog, CustomErrorMessage, NotifyHandler, ExcludeErrors
   - DefaultRecoveryConfig with specific values

2. ❌ **RecoveryMiddleware struct** - NOT FOUND
   - Requires: Handler() method with panic recovery

3. ❌ **PanicError struct** - NOT FOUND
   - Required fields: Message, Stack, RequestID, Method, Path, RemoteAddr, UserAgent

4. ❌ **NewRecoveryMiddleware()** - NOT FOUND
   - Constructor for recovery middleware

5. ❌ **RecoveryMiddleware.Handler()** - NOT FOUND
   - Must wrap http.Handler with defer recover()

6. ❌ **handlePanic()** - NOT FOUND
   - Panic handling with logging, stack trace capture

7. ❌ **SafeHandler()** - NOT FOUND
   - Helper function for wrapping handlers

8. ❌ **Middleware package** - NOT FOUND
   - No `internal/middleware/` or `pkg/middleware/` directory

---

## Implementation Search Results

### HTTP Servers Found (3 locations):

1. **pkg/daemon/proxy.go**
   - Line 67-72: HTTP server created without panic recovery
   - Line 134-156: HTTP handlers use plain http.HandlerFunc
   - **Issue**: No error handling for panics

2. **pkg/daemon/controller.go**
   - Line X: HTTP server usage confirmed

3. **pkg/auth/oauth2.go**
   - Line XXX: Local callback server
   - **Issue**: No panic protection

### Code Search Results:
```bash
grep -r "RecoveryMiddleware" --include="*.go" → 0 matches
grep -r "handlePanic" --include="*.go" → 0 matches
```

---

## Security & Reliability Impact

### Vulnerability: Process Crash on Panic (HIGH)

**Attack Vector**:
1. Attacker triggers panic in HTTP handler (malformed input, nil dereference)
2. Process crashes without graceful error handling
3. Service becomes unavailable (DoS)

**Actual Code Evidence**:

From pkg/daemon/proxy.go:134-156:
```go
mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    fmt.Fprintf(w, `{"status":"ok","message":"Muster daemon is running"}`)
})
```

**Problem**: No defer recover() - any panic here crashes the process.

**Examples of Panic Triggers**:
- Nil pointer dereference in handler
- Out of bounds array access
- Division by zero
- Invalid type assertion

---

## Production Readiness Impact

### Current State:
- ✅ HTTP servers exist and are functional
- ❌ No panic recovery protection
- ❌ DoS vulnerability exists (attacker can crash process)
- ❌ No graceful degradation or error logging

### Production Risk:
- **Risk Level**: HIGH
- **Attack**: Trigger panic to crash daemon process
- **Impact**: All HTTP requests fail, daemon needs restart
- **Mitigation**: Required - must implement panic recovery middleware

---

## Required Implementation Effort

### Option A: Full Spec Implementation (2-3 hours)

**Files to Create**:
1. `internal/middleware/recovery.go` (162 lines, per spec)
2. `internal/middleware/recovery_test.go` (test coverage)

**Tasks**:
1. Implement RecoveryConfig struct and DefaultRecoveryConfig
2. Implement RecoveryMiddleware struct with Handler()
3. Implement handlePanic() method with logging
4. Implement SafeHandler() helper
5. Write comprehensive tests
6. Integrate with existing HTTP servers (proxy.go, oauth2.go)

**Integration Points**:
- `pkg/daemon/proxy.go:67` - Wrap p.handler with recovery middleware
- `pkg/daemon/proxy.go:134-156` - Wrap existing handlers
- `pkg/auth/oauth2.go` - Wrap OAuth callback server

### Option B: Minimal Implementation (1 hour)

**Files to Create**:
1. `internal/middleware/recovery.go` (50 lines - simplified)

**Tasks**:
1. Implement basic RecoveryMiddleware with defer recover()
2. Log panics to stderr
3. Return HTTP 500 on panic
4. Integrate with HTTP servers

**Trade-off**: No configuration, no custom handlers, no advanced features (but basic protection exists)

---

## Recommended Action

### Priority: HIGH (Security + Reliability)

**Justification**:
1. HTTP servers are in production use without panic protection
2. Easy DoS vector exists (trigger panic → crash process)
3. No graceful error handling or logging
4. Minimal effort (1-3 hours) to mitigate
5. Standard practice for Go HTTP servers

### Recommendation: Implement Option B (Minimal, 1 hour)

**Why**:
- Provides basic protection quickly
- Better than Option A if time-constrained
- Can be enhanced to Option A later
- Critical security issue addressed immediately

### Implementation Plan (Option B):

**Step 1**: Create internal/middleware/recovery.go (50 lines)
```go
package middleware

import (
    "log"
    "net/http"
    "runtime/debug"
)

func Recovery(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                log.Printf("PANIC recovered: %v\n%s\n", rec, debug.Stack())
                w.WriteHeader(http.StatusInternalServerError)
                w.Write([]byte(`{"error":"Internal Server Error"}`))
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

**Step 2**: Integrate into pkg/daemon/proxy.go (line 67)
```go
p.server = &http.Server{
    Handler:      Recovery(p.handler),  // Wrap handler
    ReadTimeout:  10 * time.Second,
    WriteTimeout: 10 * time.Second,
}
```

**Step 3**: Integrate into pkg/auth/oauth2.go
```go
mux := http.NewServeMux()
mux.HandleFunc("/", Recovery(nextHandler))
```

**Step 4**: Add test coverage
```go
func TestRecoveryMiddleware(t *testing.T) {
    // Test panic is caught
    // Test HTTP 500 response
    // Test logging
}
```

**Total Effort**: ~1 hour for minimal implementation

---

## Similar Specs (For Context)

This spec is similar to:
- `MIDDLEWARE-LOGGING.md` - Logging middleware for HTTP servers
- `MIDDLEWARE-METRICS.md` - Metrics collection middleware
- `MIDDLEWARE-TRACING.md` - Distributed tracing middleware

All are HTTP web server middleware specs (not applicable to CLI-only tools, VERY APPLICABLE when HTTP servers exist).

---

## Production Deployment Blocker? 

### YES - Production-Ready Issue

**Blocker Type**: Security + Reliability

**Justification**:
1. HTTP servers are exposed (Unix socket daemon, OAuth2 callbacks)
2. Panic crashes process (no graceful error handling)
3. Attackers can trigger panic → DoS
4. Logging and debugging difficult without panic context
5. Industry standard practice (Go recommends panic recovery middleware)

**Cannot Deploy to Production Without**: Panic recovery middleware

### Time to Unblock
- Option A (Full): 2-3 hours
- Option B (Minimal): 1 hour

**Recommendation**: Implement Option B immediately (1 hour), upgrade to Option A when time allows

---

## Conclusion

**Conformance**: 0% (0/8 requirements)
**Production Ready**: NO - HTTP servers lack panic protection
**Priority**: HIGH (security + reliability)
**Effort**: 1-3 hours (critical security fix)
**Status**: PRODUCTION BLOCKER - Must implement before deployment

**Next Action**: Implement Option B (minimal panic recovery middleware, 1 hour) → Unblocks production deployment
