# Observability & Monitoring

## Overview

Comprehensive observability specification for metrics, logging, tracing, and health checks.

## 1. Metrics

### Prometheus Metrics

```go
package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
    // Request metrics
    RequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "openapi_cli_request_duration_seconds",
            Help:    "HTTP request duration",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "endpoint", "status"},
    )
    
    RequestTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "openapi_cli_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "endpoint", "status"},
    )
    
    // Cache metrics
    CacheHit = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "openapi_cli_cache_hits_total",
            Help: "Cache hit count",
        },
        []string{"type"}, // inventory, module, spec
    )
    
    CacheMiss = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "openapi_cli_cache_misses_total",
            Help: "Cache miss count",
        },
        []string{"type"},
    )
    
    CacheSize = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "openapi_cli_cache_size_bytes",
            Help: "Cache size in bytes",
        },
        []string{"type"},
    )
    
    // Inventory metrics
    InventoryFetchDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "openapi_cli_inventory_fetch_duration_seconds",
            Help:    "Inventory fetch duration",
            Buckets: []float64{0.1, 0.5, 1, 2, 5, 10},
        },
    )
    
    InventoryVersion = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "openapi_cli_inventory_version",
            Help: "Current inventory version (as label)",
        },
        []string{"host", "version"},
    )
    
    // WASM metrics
    WASMExecutionDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "openapi_cli_wasm_execution_duration_seconds",
            Help:    "WASM module execution duration",
            Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5},
        },
        []string{"module"},
    )
    
    WASMMemoryUsage = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "openapi_cli_wasm_memory_bytes",
            Help: "WASM module memory usage",
        },
        []string{"module"},
    )
    
    // Rate limiting metrics
    RateLimitHits = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "openapi_cli_rate_limit_hits_total",
            Help: "Rate limit hits",
        },
        []string{"type"},
    )
    
    RateLimitRemaining = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "openapi_cli_rate_limit_remaining",
            Help: "Remaining rate limit quota",
        },
        []string{"type"},
    )
    
    // Auth metrics
    AuthFailures = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "openapi_cli_auth_failures_total",
            Help: "Authentication failures",
        },
        []string{"type", "reason"},
    )
    
    // Daemon metrics (if in daemon mode)
    ActiveConnections = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "openapi_cli_active_connections",
            Help: "Number of active client connections",
        },
    )
    
    ConnectionDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "openapi_cli_connection_duration_seconds",
            Help:    "Client connection duration",
            Buckets: prometheus.DefBuckets,
        },
    )
)

func init() {
    prometheus.MustRegister(
        RequestDuration,
        RequestTotal,
        CacheHit,
        CacheMiss,
        CacheSize,
        InventoryFetchDuration,
        InventoryVersion,
        WASMExecutionDuration,
        WASMMemoryUsage,
        RateLimitHits,
        RateLimitRemaining,
        AuthFailures,
        ActiveConnections,
        ConnectionDuration,
    )
}
```

### Metrics Endpoint

```go
// Expose metrics when in daemon mode
func (d *Daemon) metricsHandler(w http.ResponseWriter, r *http.Request) {
    promhttp.Handler().ServeHTTP(w, r)
}

// Or write metrics to file for CLI mode
func (ic *InventoryClient) writeMetrics(path string) error {
    metrics, err := prometheus.DefaultGatherer.Gather()
    if err != nil {
        return err
    }
    
    var buf bytes.Buffer
    for _, mf := range metrics {
        expfmt.MetricFamilyToText(&buf, mf)
    }
    
    return os.WriteFile(path, buf.Bytes(), 0644)
}
```

## 2. Structured Logging

### Log Format

```go
package logging

import (
    "github.com/charmbracelet/log"
    "github.com/rs/zerolog"
)

// Structured logger with context
type Logger struct {
    inner zerolog.Logger
}

func NewLogger() *Logger {
    zl := zerolog.New(os.Stderr).
        With().
        Timestamp().
        Caller().
        Logger()
    
    return &Logger{inner: zl}
}

func (l *Logger) Info(msg string, fields ...interface{}) {
    l.inner.Info().Fields(fields).Msg(msg)
}

func (l *Logger) Error(msg string, fields ...interface{}) {
    l.inner.Error().Fields(fields).Msg(msg)
}

// Request-scoped logger
func (l *Logger) WithContext(ctx context.Context) *Logger {
    if reqID := ctx.Value("request_id"); reqID != nil {
        return &Logger{inner: l.inner.With().Str("request_id", reqID.(string)).Logger()}
    }
    return l
}
```

### Log Levels

```go
const (
    DebugLevel = iota  // Detailed debugging
    InfoLevel          // Normal operations
    WarnLevel          // Warnings
    ErrorLevel         // Errors
    FatalLevel         // Fatal errors
)

// Log level configuration
func configureLogging(level string) {
    switch level {
    case "debug":
        zerolog.SetGlobalLevel(zerolog.DebugLevel)
    case "info":
        zerolog.SetGlobalLevel(zerolog.InfoLevel)
    case "warn":
        zerolog.SetGlobalLevel(zerolog.WarnLevel)
    case "error":
        zerolog.SetGlobalLevel(zerolog.ErrorLevel)
    }
}
```

### Key Log Events

```go
// Inventory operations
log.Info("inventory_fetched",
    "host", host,
    "version", inv.Version,
    "duration_ms", duration.Milliseconds(),
    "cache_hit", cacheHit,
)

log.Warn("inventory_fetch_failed",
    "host", host,
    "error", err,
    "fallback_to_cache", true,
)

// WASM execution
log.Info("wasm_module_loaded",
    "module", moduleName,
    "sha256", hash,
    "size_bytes", size,
)

log.Error("wasm_execution_failed",
    "module", moduleName,
    "error", err,
    "duration_ms", duration.Milliseconds(),
)

// Auth events
log.Info("auth_success",
    "method", authMethod,
    "user", userID,
)

log.Warn("auth_failure",
    "method", authMethod,
    "reason", failureReason,
    "remote_addr", remoteAddr,
)

// Security events
log.Error("revocation_enforced",
    "type", rev.Type,
    "id", rev.ID,
    "reason", rev.Reason,
    "severity", rev.Severity,
)

log.Info("certificate_rotated",
    "old_cert", oldCert.Serial,
    "new_cert", newCert.Serial,
    "not_after", newCert.NotAfter,
)
```

## 3. Tracing

### OpenTelemetry Integration

```go
package tracing

import (
    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("openapi-cli")

// Start a span
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
    return tracer.Start(ctx, name, opts...)
}

// Add attributes to span
func SetAttributes(span trace.Span, attrs ...attribute.KeyValue) {
    span.SetAttributes(attrs...)
}

// Trace HTTP requests
func (c *HTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
    ctx, span := tracer.Start(ctx, "http_request",
        trace.WithAttributes(
            attribute.String("http.method", req.Method),
            attribute.String("http.url", req.URL.String()),
        ),
    )
    defer span.End()
    
    resp, err := c.client.Do(req)
    if err != nil {
        span.RecordError(err)
        return nil, err
    }
    
    span.SetAttributes(
        attribute.Int("http.status_code", resp.StatusCode),
        attribute.Int64("http.response_size", resp.ContentLength),
    )
    
    return resp, nil
}

// Trace WASM execution
func (w *WASMRuntime) Execute(ctx context.Context, module string, input []byte) ([]byte, error) {
    ctx, span := tracer.Start(ctx, "wasm_execute",
        trace.WithAttributes(
            attribute.String("wasm.module", module),
            attribute.Int("wasm.input_size", len(input)),
        ),
    )
    defer span.End()
    
    start := time.Now()
    output, err := w.execute(ctx, module, input)
    duration := time.Since(start)
    
    span.SetAttributes(
        attribute.Int64("wasm.duration_ms", duration.Milliseconds()),
        attribute.Int("wasm.output_size", len(output)),
    )
    
    if err != nil {
        span.RecordError(err)
    }
    
    return output, err
}
```

### Trace Export

```go
// Jaeger export
func initTracer() (*sdktrace.TracerProvider, error) {
    exp, err := jaeger.New(jaeger.WithCollectorEndpoint(
        jaeger.WithEndpoint("http://localhost:14268/api/traces"),
    ))
    if err != nil {
        return nil, err
    }
    
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exp),
        sdktrace.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceNameKey.String("openapi-cli"),
            semconv.ServiceVersionKey.String(version),
        )),
    )
    
    otel.SetTracerProvider(tp)
    return tp, nil
}
```

## 4. Health Checks

### Health Check Endpoint

```go
package health

type HealthChecker struct {
    checks map[string]HealthCheck
}

type HealthCheck func(ctx context.Context) error

type HealthStatus struct {
    Status    string            `json:"status"` // healthy, degraded, unhealthy
    Checks    map[string]Check  `json:"checks"`
    Timestamp time.Time         `json:"timestamp"`
}

type Check struct {
    Status    string `json:"status"`
    Error     string `json:"error,omitempty"`
    ResponseTime int64 `json:"response_time_ms"`
}

func (h *HealthChecker) Register(name string, check HealthCheck) {
    h.checks[name] = check
}

func (h *HealthChecker) Check(ctx context.Context) HealthStatus {
    status := HealthStatus{
        Status:    "healthy",
        Checks:    make(map[string]Check),
        Timestamp: time.Now(),
    }
    
    for name, check := range h.checks {
        start := time.Now()
        err := check(ctx)
        duration := time.Since(start)
        
        checkStatus := Check{
            Status:       "healthy",
            ResponseTime: duration.Milliseconds(),
        }
        
        if err != nil {
            checkStatus.Status = "unhealthy"
            checkStatus.Error = err.Error()
            status.Status = "degraded"
        }
        
        status.Checks[name] = checkStatus
    }
    
    // If any critical check failed, mark as unhealthy
    for name, check := range status.Checks {
        if check.Status == "unhealthy" && h.isCritical(name) {
            status.Status = "unhealthy"
            break
        }
    }
    
    return status
}

// HTTP handler
func (h *HealthChecker) Handler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    status := h.Check(ctx)
    
    code := http.StatusOK
    if status.Status == "degraded" {
        code = http.StatusServiceUnavailable
    } else if status.Status == "unhealthy" {
        code = http.StatusInternalServerError
    }
    
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(status)
}
```

### Standard Health Checks

```go
func (ic *InventoryClient) healthChecks() *health.HealthChecker {
    h := health.NewHealthChecker()
    
    // Inventory connectivity
    h.Register("inventory", func(ctx context.Context) error {
        _, err := ic.fetchInventory(ctx)
        return err
    })
    
    // Cache health
    h.Register("cache", func(ctx context.Context) error {
        if ic.cache.IsCorrupted() {
            return fmt.Errorf("cache corruption detected")
        }
        return nil
    })
    
    // WASM runtime
    h.Register("wasm", func(ctx context.Context) error {
        return ic.wasmRuntime.HealthCheck()
    })
    
    // Disk space (for cache)
    h.Register("disk", func(ctx context.Context) error {
        var stat syscall.Statfs_t
        if err := syscall.Statfs(ic.cacheDir, &stat); err != nil {
            return err
        }
        
        available := stat.Bavail * uint64(stat.Bsize)
        if available < 1<<30 { // Less than 1GB
            return fmt.Errorf("low disk space: %d bytes available", available)
        }
        return nil
    })
    
    return h
}
```

## 5. Audit Logging

### Audit Events

```go
package audit

type AuditEvent struct {
    Timestamp   time.Time              `json:"timestamp"`
    EventType   string                 `json:"event_type"`
    UserID      string                 `json:"user_id,omitempty"`
    ClientID    string                 `json:"client_id"`
    Fingerprint string                 `json:"fingerprint,omitempty"`
    Details     map[string]interface{} `json:"details"`
}

type AuditLogger struct {
    sink io.Writer
}

func (al *AuditLogger) Log(eventType string, details map[string]interface{}) {
    event := AuditEvent{
        Timestamp: time.Now(),
        EventType: eventType,
        ClientID:  getClientID(),
        Details:   details,
    }
    
    if userID := getCurrentUser(); userID != "" {
        event.UserID = userID
    }
    
    if fp := getFingerprint(); fp != "" {
        event.Fingerprint = fp
    }
    
    data, _ := json.Marshal(event)
    al.sink.Write(data)
    al.sink.Write([]byte("\n"))
}

// Key audit events
func LogInventoryFetch(host, version string, cacheHit bool) {
    audit.Log("INVENTORY_FETCH", map[string]interface{}{
        "host":      host,
        "version":   version,
        "cache_hit": cacheHit,
    })
}

func LogWASMExecution(module string, duration time.Duration, success bool) {
    audit.Log("WASM_EXECUTE", map[string]interface{}{
        "module":   module,
        "duration": duration.Milliseconds(),
        "success":  success,
    })
}

func LogAuthSuccess(method, userID string) {
    audit.Log("AUTH_SUCCESS", map[string]interface{}{
        "method":  method,
        "user_id": userID,
    })
}

func LogAuthFailure(method, reason string) {
    audit.Log("AUTH_FAILURE", map[string]interface{}{
        "method": method,
        "reason": reason,
    })
}

func LogRevocationEnforced(revType, id, reason string) {
    audit.Log("REVOCATION_ENFORCED", map[string]interface{}{
        "type":   revType,
        "id":     id,
        "reason": reason,
    })
}
```

### Audit Log Storage

```go
// Rotate audit logs daily
type AuditLogRotator struct {
    dir       string
    maxSize   int64 // bytes
    maxAge    int   // days
    current   *os.File
    currentSize int64
}

func (r *AuditLogRotator) Write(p []byte) (n int, err error) {
    if r.currentSize+int64(len(p)) > r.maxSize {
        if err := r.rotate(); err != nil {
            return 0, err
        }
    }
    
    n, err = r.current.Write(p)
    r.currentSize += int64(n)
    return
}

func (r *AuditLogRotator) rotate() error {
    if r.current != nil {
        r.current.Close()
    }
    
    filename := filepath.Join(r.dir, fmt.Sprintf("audit-%s.log", time.Now().Format("2006-01-02")))
    f, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
    if err != nil {
        return err
    }
    
    r.current = f
    r.currentSize = 0
    
    // Cleanup old logs
    r.cleanupOldLogs()
    
    return nil
}
```

## 6. Debugging Tools

### Debug Mode

```bash
# Enable debug logging
openapi-cli --debug --host https://api.example.com users list

# Debug with tracing
openapi-cli --debug --trace --host https://api.example.com users list

# Export debug info
openapi-cli debug --export > debug-info.json
```

### Debug Commands

```go
// CLI debug commands
var debugCmd = &cobra.Command{
    Use:   "debug",
    Short: "Debug utilities",
}

var configCmd = &cobra.Command{
    Use:   "config",
    Short: "Show effective configuration",
    Run: func(cmd *cobra.Command, args []string) {
        config := ic.GetEffectiveConfig()
        printJSON(config)
    },
}

var cacheCmd = &cobra.Command{
    Use:   "cache",
    Short: "Cache diagnostics",
}

var cacheInfoCmd = &cobra.Command{
    Use:   "info",
    Short: "Show cache statistics",
    Run: func(cmd *cobra.Command, args []string) {
        stats := ic.cache.Stats()
        fmt.Printf("Cache Directory: %s\n", stats.Path)
        fmt.Printf("Total Size: %s\n", humanize.Bytes(uint64(stats.Size)))
        fmt.Printf("Inventory Versions: %d\n", stats.InventoryCount)
        fmt.Printf("WASM Modules: %d\n", stats.ModuleCount)
        fmt.Printf("Hit Rate: %.2f%%\n", stats.HitRate*100)
    },
}

var cacheValidateCmd = &cobra.Command{
    Use:   "validate",
    Short: "Validate cache integrity",
    Run: func(cmd *cobra.Command, args []string) {
        errors := ic.cache.Validate()
        if len(errors) == 0 {
            fmt.Println("✓ Cache is valid")
        } else {
            fmt.Printf("✗ Found %d errors:\n", len(errors))
            for _, err := range errors {
                fmt.Printf("  - %s\n", err)
            }
        }
    },
}
```

### Diagnostic Report

```go
func (ic *InventoryClient) GenerateDiagnosticReport() map[string]interface{} {
    return map[string]interface{}{
        "version":    version,
        "go_version": runtime.Version(),
        "timestamp":  time.Now(),
        "config": map[string]interface{}{
            "host":          ic.config.Host,
            "inventory_url": ic.config.InventoryURL,
        },
        "cache": ic.cache.Stats(),
        "inventory": map[string]interface{}{
            "current_version": ic.currentVersion,
            "last_fetch":      ic.lastFetch,
        },
        "network": map[string]interface{}{
            "connectivity": ic.testConnectivity(),
            "latency_ms":   ic.measureLatency(),
        },
        "system": map[string]interface{}{
            "goroutines": runtime.NumGoroutine(),
            "memory_mb":  runtime.MemStats().Alloc / 1024 / 1024,
        },
    }
}
```

## 7. Alerting Rules

### Prometheus Alerting

```yaml
groups:
  - name: openapi_cli
    rules:
      - alert: HighErrorRate
        expr: |
          (
            sum(rate(openapi_cli_requests_total{status=~"5.."}[5m]))
            /
            sum(rate(openapi_cli_requests_total[5m]))
          ) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate detected"
          
      - alert: CacheCorruption
        expr: openapi_cli_cache_corruptions_total > 0
        for: 0m
        labels:
          severity: critical
        annotations:
          summary: "Cache corruption detected"
          
      - alert: InventoryFetchFailure
        expr: |
          increase(openapi_cli_inventory_fetch_failures_total[1h]) > 5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Repeated inventory fetch failures"
          
      - alert: WASMExecutionFailure
        expr: |
          increase(openapi_cli_wasm_execution_failures_total[1h]) > 10
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Multiple WASM execution failures"
```

## Summary

- **Metrics**: Prometheus-style, available via HTTP endpoint in daemon mode
- **Logging**: Structured JSON with request correlation
- **Tracing**: OpenTelemetry for request flow visualization
- **Health**: Comprehensive health checks with degradation detection
- **Audit**: Tamper-evident audit logs for security events
- **Debugging**: Built-in diagnostic commands and debug mode

This provides full observability for production deployments.
