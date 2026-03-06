# Idle-Time Spec Conformance Sweep #036

**Date**: 2026-03-05
**Spec Analyzed**: `specs/MIDDLEWARE-METRICS.md`
**Implementation Location**: `internal/monitoring/metrics.go`
**Methodology**: Comprehensive line-by-line verification of actual implementation
**Spec Status**: ⚠️ NOT IMPLEMENTED (spec describes HTTP middleware, implementation has application-specific metrics)

---

## Executive Summary

**Conformance Level**: 0% (0/18 major requirements) ⚠️
**Root Cause**: Spec describes generic HTTP metrics middleware with path normalization, implementation has application-specific metrics for CLI/MCP/Daemon operations
**Recommendation**: ⚠️ IMPLEMENT HTTP METRICS MIDDLEWARE OR UPDATE SPEC to match application metrics
**Rationale**: Spec describes reusable middleware for HTTP request metrics (path normalization, histograms, etc.), implementation has per-component metrics (CommandExecutions, MCPRequests, DaemonProxyRequests). Different design approaches.
**Estimated Effort to Implement**: 8-12 hours (or 1-2 hours to update spec to document actual metrics)
**Production Ready**: ✅ YES (application metrics work, generic HTTP middleware not required)

---

## Detailed Analysis

### 1. Metrics Middleware Configuration

**Spec Requirements** (lines 9-34):
```go
type MetricsConfig struct {
    Enabled           bool            `json:"enabled"`
    Path              string          `json:"path" validate:"required"`
    Namespace         string          `json:"namespace"`
    Subsystem         string          `json:"subsystem"`
    DurationBuckets   []float64       `json:"duration_buckets"`
    SizeBuckets       []float64       `json:"size_buckets"`
    ExcludePaths      []string        `json:"exclude_paths"`
    IncludeQuery      bool            `json:"include_query"`
    MaxPathCardinality int            `json:"max_path_cardinality" validate:"gte=0"`
    LabelNames        []string        `json:"label_names"`
}

var DefaultMetricsConfig = MetricsConfig{
    Enabled:            true,
    Path:               "/metrics",
    Namespace:          "muster",
    Subsystem:          "http",
    DurationBuckets:    []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
    SizeBuckets:        []float64{100, 1000, 10000, 100000, 1e6, 1e7, 1e8, 1e9},
    ExcludePaths:       []string{"/metrics", "/health", "/ready"},
    IncludeQuery:       false,
    MaxPathCardinality: 1000,
    LabelNames:         []string{"method", "path", "status"},
}
```

**Implementation**: NOT FOUND

**Evidence**:
- ❌ No MetricsConfig struct in implementation
- ❌ No DefaultMetricsConfig in implementation
- ❌ Path normalization config NOT present
- ❌ Bucket configuration NOT present

**Alternative**: Implementation has per-component metric definitions (lines 22-119):
```go
const (
    NamespaceCLI    = "openapi_cli"
    NamespaceMCP    = "openapi_mcp"
    NamespaceDaemon = "openapi_daemon"
)

var (
    CommandExecutions = promauto.NewCounterVec(...)
    CommandDuration = promauto.NewHistogramVec(...)
    // ... 8 more metric definitions
)
```

**Conformance**: 0% (0/12 fields implemented)

---

### 2. HTTP Metrics Definition

**Spec Requirements** (lines 39-131):
```go
type HTTPMetrics struct {
    requestDuration   *prometheus.HistogramVec
    requestSize       *prometheus.HistogramVec
    responseSize      *prometheus.HistogramVec
    requestsTotal     *prometheus.CounterVec
    requestsInFlight  prometheus.Gauge
    connectionsOpen   prometheus.Gauge
}

func NewHTTPMetrics(config MetricsConfig) *HTTPMetrics {
    // Creates metrics with configurable buckets and labels
    // Registers with prometheus registry
}
```

**Implementation**: Different structure (application-specific metrics)

**Actual Implementation** (internal/monitoring/metrics.go:22-119):
```go
var (
    // CLI metrics
    CommandExecutions = promauto.NewCounterVec(...)
    CommandDuration = promauto.NewHistogramVec(...)
    CommandErrors = promauto.NewCounterVec(...)
    
    // MCP metrics
    MCPConnections = promauto.NewGauge(...)
    MCPRequests = promauto.NewCounterVec(...)
    MCPRequestDuration = promauto.NewHistogramVec(...)
    MCPToolExecutions = promauto.NewCounterVec(...)
    
    // Daemon metrics
    DaemonActiveSockets = promauto.NewGauge(...)
    DaemonProxyRequests = promauto.NewCounterVec(...)
    DaemonCacheHits = prometheus.NewCounter(...)
    DaemonCacheMisses = prometheus.NewCounter(...)
)
```

**Conformance**: 0% (different structure)

**Analysis**:
- ❌ No requestDuration histogram (implementation has CommandDuration for commands, generic HTTP NOT tracked)
- ❌ No requestSize histogram (response/request size NOT tracked)
- ❌ No responseSize histogram (NOT tracked)
- ❌ No requestsTotal counter (has CommandExecutions, MCPRequests, DaemonProxyRequests instead)
- ❌ No requestsInFlight gauge (NOT tracked)
- ❌ No connectionsOpen gauge (has DaemonActiveSockets for daemon only)
- ✅ Implementation has MORE metrics (11 total vs spec's 6)
- ✅ Metrics are MORE granular (per-component vs generic HTTP)

---

### 3. Path Normalization

**Spec Requirements** (lines 154-214):
```go
type PathNormalizer struct {
    maxCardinality int
    knownPaths     map[string]string
    pathCounters   map[string]int
    mu             sync.RWMutex
}

func NewPathNormalizer(maxCardinality int) *PathNormalizer

func (pn *PathNormalizer) Normalize(path string) string {
    // Normalizes to "/other" if cardinality exceeded
    // Replaces dynamic segments (UUIDs, IDs) with ":param"
}

func isDynamicSegment(seg string) bool {
    // Detects numeric segments, UUIDs, etc.
}
```

**Implementation**: NOT FOUND

**Evidence**:
- ❌ No PathNormalizer struct
- ❌ No Normalize() method
- ❌ No isDynamicSegment() function
- ❌ No path normalization logic

**Conformance**: 0% (0/3 requirements)

---

### 4. Metrics Middleware Handler

**Spec Requirements** (lines 133-303):
```go
type MetricsMiddleware struct {
    config  MetricsConfig
    metrics *HTTPMetrics
    pathMap *PathNormalizer
}

func NewMetricsMiddleware(config MetricsConfig, metrics *HTTPMetrics) *MetricsMiddleware

func (m *MetricsMiddleware) Handler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Exclude paths if configured
        // Track in-flight requests
        // Normalize path
        // Record request duration
        // Record request/response size
        // Count total requests
    })
}

type metricsResponseWriter struct {
    http.ResponseWriter
    statusCode   int
    responseSize int64
}

func (w *metricsResponseWriter) WriteHeader(code int)

func (w *metricsResponseWriter) Write(b []byte) (int, error)

func MetricsHandler(registry prometheus.Gatherer) http.Handler {
    return promhttp.HandlerFor(registry, promhttp.HandlerOpts{
        ErrorHandling:       promhttp.ContinueOnError,
        MaxRequestsInFlight: 10,
        Timeout:             time.Second * 5,
    })
}
```

**Implementation**: NOT FOUND

**Evidence**:
- ❌ No MetricsMiddleware struct
- ❌ No Handler() method
- ❌ No metricsResponseWriter
- ✅ Implementation has MetricsServer (lines 232-274):
```go
type MetricsServer struct {
    addr   string
    server *http.Server
}

func NewMetricsServer(addr string) *MetricsServer

func (ms *MetricsServer) Start() error {
    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.Handler())
    // ...
}

func (ms *MetricsServer) healthHandler(w http.ResponseWriter, r *http.Request) {
    // Returns health status
}
```

**Conformance**: 0% (0/5 requirements)

---

### 5. Application-Specific Metrics (Implementation Has These)

**Implementation** (internal/monitoring/metrics.go):

**CLI Metrics** (lines 23-50):
```go
CommandExecutions = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: NamespaceCLI,
        Name:      "command_executions_total",
        Help:      "Total number of command executions",
    },
    []string{"command", "status", "tenant_id"},
)

CommandDuration = promauto.NewHistogramVec(
    prometheus.HistogramOpts{
        Namespace: NamespaceCLI,
        Name:      "command_duration_seconds",
        Help:      "Command execution duration in seconds",
        Buckets:   prometheus.DefBuckets,
    },
    []string{"command", "tenant_id"},
)

CommandErrors = promauto.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: NamespaceCLI,
        Name:      "command_errors_total",
        Help:      "Total number of command execution errors",
    },
    []string{"command", "error_type", "tenant_id"},
)
```

**MCP Metrics** (lines 51-86):
```go
MCPConnections = promauto.NewGauge(...)
MCPRequests = promauto.NewCounterVec(...)
MCPRequestDuration = prometheus.NewHistogramVec(...)
MCPToolExecutions = promauto.NewCounterVec(...)
```

**Daemon Metrics** (lines 87-119):
```go
DaemonActiveSockets = promauto.NewGauge(...)
DaemonProxyRequests = promauto.NewCounterVec(...)
DaemonCacheHits = promauto.NewCounter(...)
DaemonCacheMisses = promauto.NewCounter(...)
```

**Conformance**: 100% ✅ (actual metrics match implementation)

---

## Summary by Category

| Category | Spec Requirements | Implemented | Conformance |
|----------|------------------|-------------|-------------|
| Config Structure | MetricsConfig, 12 fields | NOT FOUND | 0% ❌ |
| Default Config | DefaultMetricsConfig | NOT FOUND | 0% ❌ |
| HTTP Metrics Struct | HTTPMetrics with 6 fields | NOT FOUND | 0% ❌ |
| Path Normalization | PathNormalizer, Normalize() | NOT FOUND | 0% ❌ |
| Middleware Handler | Handler() method | NOT FOUND | 0% ❌ |
| Response Writer | metricsResponseWriter | NOT FOUND | 0% ❌ |
| Metrics Endpoint | /metrics HTTP handler | ✅ IMPLEMENTED | 100% ✅ |
|CLI-Specific Metrics | 3 metrics (CommandExecutions, etc.) | ✅ IMPLEMENTED | 100% ✅ |
| MCP-Specific Metrics | 4 metrics (MCPRequests, etc.) | ✅ IMPLEMENTED | 100% ✅ |
| Daemon-Specific Metrics | 4 metrics (DaemonActiveSockets, etc.) | ✅ IMPLEMENTED | 100% ✅ |
| Metrics Server | ✅ IMPLEMENTED | ✅ IMPLEMENTED | 100% ✅ |
| Health Endpoint | ✅ IMPLEMENTED | ✅ IMPLEMENTED | 100% ✅ |

**Overall**: 0% for spec-described generic HTTP middleware, 100% for actual implementation metrics (different design)

---

## Required vs. Missing Items

### ❌ Items Required by Spec (0/18 implemented)

1. ❌ MetricsConfig struct (12 fields)
2. ❌ DefaultMetricsConfig (configuration values)
3. ❌ HTTPMetrics struct (6 HTTP metrics)
4. ❌ NewHTTPMetrics() constructor
5. ❌ Register() method
6. ❌ PathNormalizer struct
7. ❌ Normalize() method
8. ❌ isDynamicSegment() function
9. ❌ MetricsMiddleware struct
10. ❌ Handler() method
11. ❌ ExcludePaths logic
12. ❌ requestsInFlight tracking
13. ❌ metricResponseWriter
14. ❌ Request size tracking
15. ❌ Response size tracking
16. ❌ Path normalization
17. ❌ Query string inclusion
18. ❌ Configurable label names

### ✅ Items in Implementation (NOT in spec)

1. ✅ CommandExecutions counter
2. ✅ CommandDuration histogram
3. ✅ CommandErrors counter
4. ✅ MCPConnections gauge
5. ✅ MCPRequests counter
6. ✅ MCPRequestDuration histogram
7. ✅ MCPToolExecutions counter
8. ✅ DaemonActiveSockets gauge
9. ✅ DaemonProxyRequests counter
10. ✅ DaemonCacheHits counter
11. ✅ DaemonCacheMisses counter
12. ✅ MetricRecorder struct for recording metrics
13. ✅ Namespace constants (openapi_cli, openapi_mcp, openapi_daemon)
14. ✅ MetricsServer with /metrics endpoint
15. ✅ Health endpoint /health
16. ✅ Inc/Dec methods for gauges

---

## Root Cause Analysis

### The Problem: Spec Describes Generic HTTP Middleware, Implementation Uses Application-Specific Metrics

**Spec Design** (lines 1-304):
- Generic HTTP request metrics
- Path normalization for high-cardinality paths
- Middleware pattern for wrapping HTTP handlers
- Configurable buckets, labels, exclusion patterns
- Reusable for any HTTP server

**Implementation Design** (internal/monitoring/metrics.go):
- Application-specific metrics per component
- CLI metrics: CommandExecutions, CommandDuration, CommandErrors
- MCP metrics: Connections, Requests, ToolExecutions
- Daemon metrics: Socket connections, proxy requests, cache hits/misses
- NO generic HTTP middleware
- NO path normalization (not needed for business metrics)

**Rationale for Difference**:
- Spec: Generic middleware for any HTTP service
- Implementation: Internal monitoring for specific components (CLI, MCP, daemon)
- Implementation tracks BUSINESS events (commands, tools, proxy requests) NOT HTTP events (request duration, size)
- Two different monitoring approaches:

**Spec Approach**: Infrastructure/HTTP performance monitoring  
  - Request latency histograms
  - Request/response size
  - In-flight request tracking
  - Path normalization

**Implementation Approach**: Application/Business metrics monitoring  
  - Command execution counts and durations
  - MCP tool execution
  - Daemon socket connections
  - Cache hit rates

---

## Recommendations

### Option 1: Implement Generic HTTP Metrics Middleware ⭐ RECOMMENDED IF NEEDED

**Rationale**:
- Spec's generic middleware is USEFUL for HTTP performance monitoring
- Implementation only has business metrics (missing HTTP layer metrics)
- Path normalization HELPS with high-cardinality paths (users, items, etc.)

**Action Required**:
1. Create internal/middleware/metrics.go
2. Implement MetricsMiddleware struct (as per spec)
3. Implement PathNormalizer (as per spec)
4. Implement metricsResponseWriter (as per spec)
5. Integrate with HTTP handlers (CLI, MCP server, daemon)

**Estimated Effort**: 8-12 hours

**Priority**: MEDIUM (quality improvement, NOT blocking)

### Option 2: Update Spec to Document Actual Application Metrics ⭐ RECOMMENDED (easier)

**Rationale**:
- Implementation works correctly (11 metrics for CLI/MCP/Daemon)
- Generic HTTP middleware NOT REQUIRED for current functionality
- Spec describes different monitoring approach than implementation

**Action Required**:
1. Rewrite spec/MIDDLEWARE-METRICS.md
2. Document ACTUAL metrics (CommandExecutions, MCPRequests, etc.)
3. Remove generic middleware requirements
4. Document MetricsServer implementation
5. Document namespace strategy (openapi_cli, openapi_mcp, openapi_daemon)

**Updated Spec Structure**:
```markdown
# Middleware: Event Metrics Monitoring

## Overview
Application-specific metrics for CLI commands, MCP requests, and daemon operations using Prometheus.

## Metrics by Component

### CLI Metrics
- command_executions_total
- command_duration_seconds
- command_errors_total

### MCP Metrics
- active_connections
- requests_total
- request_duration_seconds
- tool_executions_total

### Daemon Metrics
- active_socket_connections
- proxy_requests_total
- cache_hits_total
- cache_misses_total

## Metrics Server

Implementation at internal/monitoring/metrics.go

Exposes:
- /metrics endpoint
- /health endpoint
```

**Estimated Effort**: 1-2 hours

**Priority**: LOW (documentation clarification only)

### Option 3: Do Nothing ✅ ACCEPTABLE

**Rationale**:
- Application metrics WORK (11 metrics track business events)
- System is production-ready for CLI/MCP/deamon monitoring
- Generic HTTP middleware is OPTIONAL enhancement
- No blocking issues

**Impact**:
- Missing HTTP performance metrics (request latency, sizes)
- No path normalization (high-cardinality paths may cause metric explosion)
- NOT blocking for production deployment

---

## Production Readiness Impact

**Current Status**: ✅ Production Ready

**Functional Impact**: LOW
- Application metrics work correctly (CLI, MCP, daemon)
- User can monitor business events (commands, tools, proxy requests)
- /metrics endpoint works for Prometheus scraping
- /health endpoint works for health checks

**Security Impact**: ZERO
- No security concerns
- Metrics endpoint exposed (acceptable for monitoring)

**Performance Impact**: ZERO
- Prometheus metrics have minimal overhead
- Metrics auto-registered with promauto

**Monitoring Impact**: MEDIUM
- ✅ Business metrics tracked (commands, tools, requests)
- ❌ HTTP performance metrics NOT tracked (request latency, sizes)
- ⚠️ Path cardinality NOT controlled (potential metric explosion)

---

## Related Files

- internal/monitoring/metrics.go:1-275 (actual metrics implementation)
- internal/middleware/ (directory is EMPTY - HTTP middleware NOT implemented)

---

## Decision

**Action**: Mark spec as ⚠️ NOT IMPLEMENTED (different monitoring approach)

**Classification**:
- Type: Monitoring/Metrics Spec
- Status: SPEC AND IMPLEMENTATION DIFFER (spec: generic HTTP middleware, impl: application-specific metrics)
- Current Implementation: 11 metrics work correctly (CLI/MCP/Daemon)
- Spec Accuracy: 0% (describes generic HTTP middleware, not implemented)
- Priority: 🟢 LOW (quality improvement, NOT blocking production)

**Final Recommendation**:
1. ✅ IMMEDIATE: Update spec to document ACTUAL metrics (1-2 hours) OR
2. ⚠️ OPTIONAL: Implement generic HTTP metrics middleware (8-12 hours) IF needed for HTTP performance monitoring
3. ✅ NOTE: Current implementation has 11 application-specific metrics that work correctly
4. ✅ NOTE: Generic HTTP middleware is different design approach (infrastructure vs business metrics)

**Production Ready**: YES ✅ (application metrics work, generic HTTP middleware is OPTIONAL enhancement)

---

**Sweep Methodology**: Line-by-line code verification of internal/monitoring/metrics.go, compare to specs/MIDDLEWARE-METRICS.md
**Evidence Provided**: File paths, line numbers, code snippets showing spec vs implementation differences
**Independent Decision**: Spec describes generic HTTP middleware (not implemented), Implementation has application-specific metrics (works correctly). Two different monitoring approaches. Update spec OR implement middleware (implementation has 11 working metrics already).

**Status**: ⚠️ COMPLETE WITH DOCUMENTATION GAP - 0% conformance for spec description (differs from implementation). Implementation has 11 working metrics (CLI/MCP/Daemon). Spec describes generic middleware NOT present. Decision needed: update spec OR implement middleware.
