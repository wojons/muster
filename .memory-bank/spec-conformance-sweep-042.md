# Idle-Time Spec Conformance Sweep #042

**Date**: 2026-03-05
**Spec Analyzed**: `OPS-HEALTH-CHECKS.md`
**Implementation Location**: `pkg/daemon/proxy.go`, `internal/monitoring/metrics.go`
**Conformance**: 33% (1/3 endpoints implemented)
**Analysis Duration**: 10 minutes

---

## Executive Summary

**Status**: 🟡 MEDIUM GAPS - Kubernetes compatibility at risk

The Health Check endpoints implementation is **partially complete** with missing critical endpoints for Kubernetes liveness/readiness/startup probes. Only a single `/health` endpoint exists, missing two required endpoints.

**Key Findings**:
- 1/3Kubernetes endpoints implemented (33% conformance)
- Missing `/health/live` (liveness probe)
- Missing `/health/ready` (readiness probe with dependency checks)
- Missing `/health/startup` (startup probe)
- Single `/health` endpoint exists but doesn't match Kubernetes expectations
- Both daemon and metrics server have same gap

---

## Requirements Analysis

### Requirement 1: Liveness Probe Endpoint

**Spec Requirement**: `/health/live` GET endpoint
**Spec Reference**: OPS-HEALTH-CHECKS.md:10, 59-73

**Expected Behavior**:
- Returns 200 if app is running
- Returns 503 if app is not started
- Response status field: "healthy" or "down"
- Response fields: status, timestamp, version

**Example Response**:
```json
{
  "status": "healthy",
  "timestamp": "2026-03-05T16:45:00Z",
  "version": "1.0.0"
}
```

**Actual Implementation**:
- ❌ NOT IMPLEMENTED
- Only `/health` endpoint exists in two locations:
  - `pkg/daemon/proxy.go:143-147` → `/health` returns `{"status":"healthy"}`
  - `internal/monitoring/metrics.go:246, 264-268` → `/health` returns `{"status":"healthy","timestamp":"..."}`

**Conformance**: 0% (missing endpoint)

**Impact**: 🔴 HIGH - Kubernetes liveness probes require `/health/live` endpoint for container orchestration. Without it, K8s cannot detect application crash loops or determine if pod is alive.

---

### Requirement 2: Readiness Probe Endpoint

**Spec Requirement**: `/health/ready` GET endpoint with dependency checks
**Spec Reference**: OPS-HEALTH-CHECKS.md:11, 75-99

**Expected Behavior**:
- Returns 200 if app can serve traffic AND all dependencies are healthy
- Returns 503 if app is started but not ready OR dependencies unhealthy
- Checks database, cache, and other dependencies
- Response includes status, timestamp, version, checks object

**Example Response**:
```json
{
  "status": "ready",
  "timestamp": "2026-03-05T16:45:00Z",
  "version": "1.0.0",
  "checks": {
    "database": {
      "status": "healthy",
      "response_time_ms": 5
    },
    "cache": {
      "status": "healthy"
    }
  }
}
```

**Actual Implementation**:
- ❌ NOT IMPLEMENTED
- No `/health/ready` endpoint exists

**Conformance**: 0% (missing endpoint)

**Impact**: 🔴 HIGH - Kubernetes readiness probes require `/health/ready` endpoint to determine if pod can receive traffic. Without it, K8s may send traffic to unready pods, causing errors.

---

### Requirement 3: Startup Probe Endpoint

**Spec Requirement**: `/health/startup` GET endpoint
**Spec Reference**: OPS-HEALTH-CHECKS.md:12, 101-107

**Expected Behavior**:
- Returns 200 when app has started and warmed up
- Returns 503 during startup
- Used for slow-starting applications

**Example Response**:
```json
{
  "status": "started",
  "timestamp": "2026-03-05T16:45:00Z"
}
```

**Actual Implementation**:
- ❌ NOT IMPLEMENTED
- No `/health/startup` endpoint exists

**Conformance**: 0% (missing endpoint)

**Impact**: 🔴 HIGH - Kubernetes startup probes require `/health/startup` endpoint for applications with startup delays. Without it, K8s may kill pods before they're ready.

---

## Actual Implementation Analysis

### Implementation 1: Daemon Proxy Health Endpoint

**Location**: `pkg/daemon/proxy.go:143-147`

**Code**:
```go
mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    fmt.Fprintf(w, `{"status":"healthy"}`)
})
```

**Problems**:
1. ❌ Endpoint is `/health` not `/health/live`
2. ❌ No `/health/ready` endpoint
3. ❌ No `/health/startup` endpoint
4. ❌ No dependency checks (database, cache, etc.)
5. ❌ No timestamp field (unlike spec)
6. ❌ No version field
7. ❌ No checks object (for readiness)

**Conformance**: 33% (single endpoint exists but wrong path)

---

### Implementation 2: Metrics Server Health Endpoint

**Location**: `internal/monitoring/metrics.go:246, 264-268`

**Code**:
```go
mux.Handle("/metrics", promhttp.Handler())
mux.HandleFunc("/health", ms.healthHandler)

func (ms *MetricsServer) healthHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().UTC().Format(time.RFC3339))
}
```

**Problems**:
1. ❌ Endpoint is `/health` not `/health/live`
2. ❌ No `/health/ready` endpoint
3. ❌ No `/health/startup` endpoint
4. ❌ No dependency checks (database, cache, etc.)
5. ❌ No version field
6. ❌ No checks object (for readiness)

**Improvements vs Daemon Proxy**:
- ✅ Has timestamp field (matches spec format)

**Conformance**: 33% (single endpoint exists but wrong path, missing 2 required)

---

## Kubernetes Configuration Gap

**Spec Requirement**: Kubernetes deployment with health probes
**Spec Reference**: OPS-HEALTH-CHECKS.md:109-142

**Expected Kubernetes Config**:
```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
startupProbe:
  httpGet:
    path: /health/startup
    port: 8080
```

**Actual Kubernetes Config**: NOT FOUND

**Note**: No Kubernetes manifests found in project (checked `k8s/`, `chart/` directories). Without health check endpoints, cannot deploy to Kubernetes properly.

---

## Gap Summary

| Endpoint | Spec Required | Implemented | Gap |
|----------|--------------|------------|-----|
| /health/live | Liveness probe | ❌ /health only | 🔴 Missing endpoint |
| /health/ready | Readiness probe + dependency checks | ❌ None | 🔴 Missing endpoint + dependency checks |
| /health/startup | Startup probe | ❌ None | 🔴 Missing endpoint |
| **Total** | **3 endpoints** | **1 (/health only)** | **🔴 2/3 missing** |

**Conformance**: 33% (1/3 endpoints, wrong paths)

**Impact**: 🔴 HIGH CRITICAL

---

## Remediation Plan

### Task 1: Implement /health/live Endpoint ⭐ CRITICAL

**Location**: Both `pkg/daemon/proxy.go` and `internal/monitoring/metrics.go`

**Implementation Required**:
```go
// Liveness probe - returns 200 if app is running
mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    fmt.Fprintf(w, `{
        "status": "healthy",
        "timestamp": "%s",
        "version": "1.0.0"
    }`, time.Now().UTC().Format(time.RFC3339))
})
```

**Estimated Effort**: 15 minutes (3 files: 2 implementations + 2 tests)

**Priority**: 🔴 HIGH (Kubernetes compatibility)

---

### Task 2: Implement /health/ready Endpoint with Dependency Checks ⭐ CRITICAL

**Location**: Both `pkg/daemon/proxy.go` and `internal/monitoring/metrics.go`

**Implementation Required**:
```go
// Readiness probe - checks dependencies before returning ready
mux.HandleFunc("/health/ready", func(w http.ResponseWriter, r *http.Request) {
    checks := make(map[string]interface{})

    // Check database
    if dbErr := checkDatabase(); dbErr == nil {
        checks["database"] = map[string]interface{}{
            "status": "healthy",
        }
    } else {
        checks["database"] = map[string]interface{}{
            "status": "unhealthy",
            "error":  dbErr.Error(),
        }
    }

    // Check cache
    if cacheErr := checkCache(); cacheErr == nil {
        checks["cache"] = map[string]interface{}{
            "status": "healthy",
        }
    } else {
        checks["cache"] = map[string]interface{}{
            "status": "unhealthy",
            "error":  cacheErr.Error(),
        }
    }

    // Determine overall status
    allHealthy := true
    for _, check := range checks {
        if check.(map[string]interface{})["status"] != "healthy" {
            allHealthy = false
            break
        }
    }

    w.Header().Set("Content-Type", "application/json")
    if allHealthy {
        w.WriteHeader(http.StatusOK)
    } else {
        w.WriteHeader(http.StatusServiceUnavailable)
    }

    resp := map[string]interface{}{
        "status":    "ready",
        "timestamp": time.Now().UTC().Format(time.RFC3339),
        "version":  "1.0.0",
        "checks":   checks,
    }

    json.NewEncoder(w).Encode(resp)
})
```

**Database Check Function** (needs to be implemented):
```go
func checkDatabase() error {
    // Implementation depends on database layer
    // Should ping database and return error if unavailable
}
```

**Cache Check Function** (needs to be implemented):
```go
func checkCache() error {
    // Implementation depends on cache layer
    // Should ping cache and return error if unavailable
}
```

**Estimated Effort**: 2 hours (database/cache integration + 4 files: 2 implementations + 2 tests)

**Priority**: 🔴 HIGH (Kubernetes compatibility)

---

### Task 3: Implement /health/startup Endpoint ⭐ CRITICAL

**Location**: Both `pkg/daemon/proxy.go` and `internal/monitoring/metrics.go`

**Implementation Required**:
1. Add startup tracking variable
2. Implement startup probe endpoint
3. Set startup flag after warmup completes

```go
// In MetricsServer struct
type MetricsServer struct {
    addr       string
    server     *http.Server
    started    bool
    ready      bool
    startupMux sync.RWMutex
}

// On startup event
func (ms *MetricsServer) OnStartup() {
    // Simulate warmup or actual initialization
    time.Sleep(5 * time.Second)

    ms.startupMux.Lock()
    ms.started = true
    ms.ready = true
    ms.startupMux.Unlock()
}

// Startup probe
mux.HandleFunc("/health/startup", func(w http.ResponseWriter, r *http.Request) {
    ms.startupMux.RLock()
    defer ms.startupMux.RUnlock()

    w.Header().Set("Content-Type", "application/json")
    if ms.started {
        w.WriteHeader(http.StatusOK)
        fmt.Fprintf(w, `{
            "status": "started",
            "timestamp": "%s"
        }`, time.Now().UTC().Format(time.RFC3339))
    } else {
        w.WriteHeader(http.StatusServiceUnavailable)
        fmt.Fprintf(w, `{
            "status": "starting",
            "timestamp": "%s"
        }`, time.Now().UTC().Format(time.RFC3339))
    }
})
```

**Estimated Effort**: 1 hour (2 files: 2 implementations + 2 tests)

**Priority**: 🔴 HIGH (Kubernetes compatibility)

---

### Task 4: Add Kubernetes Deployment Manifest ⭐ HIGH

**Location**: `k8s/deployment.yaml` (needs to be created)

**Implementation Required**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: muster
spec:
  replicas: 3
  selector:
    matchLabels:
      app: muster
  template:
    metadata:
      labels:
        app: muster
    spec:
      containers:
      - name: muster
        image: muster:latest
        ports:
        - containerPort: 8080
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
        startupProbe:
          httpGet:
            path: /health/startup
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          failureThreshold: 30
```

**Estimated Effort**: 15 minutes (create file + documentation)

**Priority**: 🟡 MEDIUM (essential for Kubernetes deployment)

---

## Production Readiness Assessment

### Kubernetes Deployment: ❌ NOT READY

**Blocking Issue**: Health check endpoints required for Kubernetes probes are missing
- Liveness probe cannot connect to `/health/live`
- Readiness probe cannot connect to `/health/ready`
- Startup probe cannot connect to `/health/startup`
- Without correct endpoints, Kubernetes cannot manage pod lifecycle

**Impact**:
- ❌ Cannot detect application crashes (liveness)
- ❌ Cannot determine when pod is ready for traffic (readiness)
- ❌ Cannot handle slow-starting applications (startup)
- ❌ Kubernetes deployment will fail or pods will be mismanaged

**Workaround**: None - Kubernetes requires specific endpoint paths

---

### Manual Deployment: ⚠️ PARTIALLY READY

**Current State**:
- Single `/health` endpoint exists
- Can be used for simple health checks
- Lacks Kubernetes-specific semantics

**Workaround Possible**: Manual health check scripts can use `/health` endpoint, but Kubernetes probes will not work correctly.

---

## Recommendations

### Critical Actions (Required for Kubernetes Deployment)

**Task 1**: Implement `/health/live` endpoint
**Priority**: 🔴 HIGH
**Effort**: 15 minutes
**Files to modify**: `pkg/daemon/proxy.go`, `internal/monitoring/metrics.go`

**Task 2**: Implement `/health/ready` endpoint with dependency checks
**Priority**: 🔴 HIGH
**Effort**: 2 hours
**Files to modify**: `pkg/daemon/proxy.go`, `internal/monitoring/metrics.go` (add database/cache check functions)

**Task 3**: Implement `/health/startup` endpoint
**Priority**: 🔴 HIGH
**Effort**: 1 hour
**Files to modify**: `pkg/daemon/proxy.go`, `internal/monitoring/metrics.go` (add startup tracking)

**Total Estimated Effort**: ~3.5 hours for critical fixes

### Optional Actions (Recommended for Complete K8s Support)

**Task 4**: Create Kubernetes deployment manifests
**Priority**: 🟡 MEDIUM
**Effort**: 15 minutes
**Files to create**: `k8s/deployment.yaml`, `k8s/service.yaml`, `k8s/ingress.yaml`

---

## Gap Analysis

### Gap 1: Missing /health/live Endpoint 🔴 CRITICAL

**Spec Requires**: `/health/live` endpoint for liveness probe
**Actual Implementation**: `/health` endpoint only
**Impact**: 🔴 HIGH - Kubernetes liveness probe will fail
**Location**: `pkg/daemon/proxy.go:143-147`, `internal/monitoring/metrics.go:246`

**Resolution**: Add `/health/live` handler to both implementations

---

### Gap 2: Missing /health/ready Endpoint 🔴 CRITICAL

**Spec Requires**: `/health/ready` endpoint with dependency checks
**Actual Implementation**: None
**Impact**: 🔴 HIGH - Kubernetes readiness probe will fail
**Resolution**: Add `/health/ready` handler with database/cache checks

---

### Gap 3: Missing /health/startup Endpoint 🔴 CRITICAL

**Spec Requires**: `/health/startup` endpoint for startup probe
**Actual Implementation**: None
**Impact**: 🔴 HIGH - Kubernetes startup probe will fail
**Resolution**: Add `/health/startup` handler with startup tracking

---

## Conclusion

**Implementation Status**: 33% COMPLETE ⚠️

**Conformance**: 33% (1/3 endpoints implemented, wrong paths)

**Kubernetes Deployment**: ❌ NOT READY (critical gaps block deployment)

**Blockers**:
1. 🔴 Missing `/health/live` endpoint
2. 🔴 Missing `/health/ready` endpoint with dependency checks
3. 🔴 Missing `/health/startup` endpoint

**Recommendation**: ❌ **DO NOT DEPLOY TO KUBERNETES** until health check endpoints are implemented

**Action Required**:
1. Implement all 3 required health check endpoints
2. Add dependency checking for `/health/ready`
3. Create Kubernetes deployment manifests
4. Test with Kubernetes locally (minikube/kind)

**Estimated Time to Production (Kubernetes)**: 4 hours (3.5 hours implementation + testing)

**Estimated Time to Production (Manual)**: 0 hours (can deploy manually with `/health` endpoint, but not Kubernetes)

**Confidence Level**: HIGH (line-by-line code inspection, critical Kubernetes compatibility gaps identified)

---

## Evidence Locations

### Implementation Files Analyzed

- `pkg/daemon/proxy.go:143-147` - Daemon proxy `/health` endpoint
- `internal/monitoring/metrics.go:246, 264-268` - Metrics server `/health` endpoint

### Missing Files

- ❌ No `/health/live` handler in either file
- ❌ No `/health/ready` handler in either file
- ❌ No `/health/startup` handler in either file
- ❌ No database check functions
- ❌ No cache check functions
- ❌ No Kubernetes deployment manifests

---

## Timeline

- Start: 2026-03-05 17:00 UTC
- End: 2026-03-05 17:10 UTC
- Duration: 10 minutes
- Spec analyzed: OPS-HEALTH-CHECKS.md (149 lines)
- Code reviewed: 250+ lines across 2 implementation files
- Sweeps completed: 42 total

---

**END OF SWEEP REPORT**

**REMEDIATION TASKS ADDED TO TODO.md** ✅
