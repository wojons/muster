# Trace-Based Debugging Guide

This guide explains how to use distributed tracing to debug cross-service requests in Muster.

## Overview

Muster uses OpenTelemetry for distributed tracing, allowing you to:
- Track requests across multiple services
- Identify performance bottlenecks
- Debug failures with full request context
- Correlate logs with specific traces

## Quick Start

### 1. Enable Tracing

Tracing is enabled by default in Muster. Configure the tracer provider:

```go
import "github.com/anomalyco/muster/internal/telemetry"

config := telemetry.TracingConfig{
    Enabled:          true,
    ServiceName:      "muster-api",
    ServiceVersion:   "1.0.0",
    ExporterEndpoint: "localhost:4317",  // OTLP collector
    ExporterProtocol: "grpc",
    SamplingRate:     1.0,  // Sample all requests (reduce in production)
}

provider, err := telemetry.NewTracerProvider(ctx, config)
if err != nil {
    log.Fatal(err)
}
defer provider.Shutdown(ctx)
```

### 2. Create Spans

Spans represent operations in your application:

```go
tracer := telemetry.NewTracer(tp.Tracer("muster-api"))

// Simple span
ctx, span := tracer.Start(ctx, "process-request")
defer span.End()

// Span with attributes
ctx, span := tracer.Start(ctx, "cache-lookup")
span.SetAttribute("cache.key", userID)
span.SetAttribute("cache.ttl", 300)
defer span.End()
```

### 3. Propagate Context

Pass context through your application to maintain trace continuity:

```go
// In HTTP handlers (automatic via middleware)
router.Use(telemetry.TracingMiddleware(config))

// Manual context propagation
func ProcessUser(ctx context.Context, userID string) {
    ctx, span := tracer.Start(ctx, "process-user")
    defer span.End()
    
    // Pass ctx to child operations
    user, err := db.GetUser(ctx, userID)
    cache.Set(ctx, userID, user)
}
```

## Common Debugging Scenarios

### Scenario 1: Slow Request Investigation

**Problem**: API request taking >2 seconds

**Debug Steps**:

1. **Find the trace ID** in logs:
   ```
   {"level":"info","trace_id":"abc123","span_id":"def456","message":"Request started"}
   ```

2. **View in Jaeger** (or your trace viewer):
   - Navigate to: `http://jaeger:16686/trace/abc123`
   - Visualize the span tree:
     ```
     process-request (2.3s)
     ├── authenticate-user (150ms) ✓
     ├── fetch-user-data (2.1s) ✗ ← BOTTLENECK
     │   ├── cache-lookup (5ms) ✓
     │   └── database-query (2.095s) ← ISSUE HERE
     └── format-response (50ms) ✓
     ```

3. **Identify root cause**:
   - Missing database index
   - N+1 query problem
   - Cache miss causing DB load

**Code fix**:
```go
// Add span to identify slow queries
ctx, span := tracer.Start(ctx, "database-query")
span.SetAttribute("db.statement", query)
span.SetAttribute("db.system", "postgresql")
defer func() {
    span.SetAttribute("db.rows_affected", rows)
    span.End()
}()

rows, err := db.QueryContext(ctx, query)
```

### Scenario 2: Intermittent Failure Investigation

**Problem**: Random 500 errors

**Debug Steps**:

1. **Filter traces by error status**:
   ```
   status.code=ERROR AND http.status_code=500
   ```

2. **Compare successful vs failed traces**:
   - Successful trace:
     ```
     process-request (200ms) [status: OK]
     ├── validate-input (10ms)
     ├── authenticate (50ms)
     └── process (140ms)
     ```
   
   - Failed trace:
     ```
     process-request (error) [status: ERROR]
     ├── validate-input (10ms)
     ├── authenticate (50ms)
     └── call-external-service [error: timeout]
         └── http.request (30s) ← TIMEOUT
     ```

3. **Root cause**: External service timeout causing context cancellation

**Code fix**:
```go
// Add timeout and retry spans
ctx, span := tracer.Start(ctx, "call-external-service")
defer span.End()

ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
defer cancel()

resp, err := client.Do(req.WithContext(ctx))
if err != nil {
    span.SetError(err)  // Records error in trace
    span.SetStatus(trace.Status{Code: trace.StatusCodeError, Description: "timeout"})
    return err
}
```

### Scenario 3: Cross-Service Request Tracing

**Problem**: Request flows through multiple services, need full picture

**Architecture**:
```
Client → API Gateway → Muster API → Database
                     ↓
                   Cache
                     ↓
                   Auth Service
```

**Trace Flow**:
1. **Client** includes traceparent header:
   ```
   traceparent: 00-abc123def456789-0123456789abcdef-01
   ```

2. **API Gateway** extracts and propagates:
   ```go
   ctx := telemetry.ExtractTraceContext(r)
   // Automatically propagated to downstream services
   ```

3. **View complete trace**:
   ```
   api-gateway: handle-request
   ├── muster-api: process-command
   │   ├── cache: lookup
   │   ├── database: query
   │   └── auth-service: validate-token
   └── api-gateway: send-response
   ```

**Key insight**: All spans share the same trace ID, allowing you to see the full request lifecycle.

### Scenario 4: Memory Leak Detection

**Problem**: Gradual memory increase over time

**Debug Steps**:

1. **Add memory metrics to spans**:
   ```go
   var m runtime.MemStats
   runtime.ReadMemStats(&m)
   
   span.SetAttribute("memory.alloc_mb", m.Alloc/1024/1024)
   span.SetAttribute("memory.heap_alloc_mb", m.HeapAlloc/1024/1024)
   span.SetAttribute("memory.gc_runs", m.NumGC)
   ```

2. **Compare traces over time**:
   - Day 1: `memory.heap_alloc_mb: 50`
   - Day 7: `memory.heap_alloc_mb: 500`
   - Day 14: `memory.heap_alloc_mb: 1200` ← Memory leak confirmed

3. **Identify leaking operation**:
   Look for spans with increasing attributes or events not being cleaned up.

### Scenario 5: Concurrency Issues

**Problem**: Race condition causing data corruption

**Debug Steps**:

1. **Add goroutine IDs to spans**:
   ```go
   span.SetAttribute("goroutine.id", getGoroutineID())
   span.SetAttribute("goroutine.count", runtime.NumGoroutine())
   ```

2. **Trace concurrent operations**:
   ```
   process-batch
   ├── process-item-1 [goroutine.id: 47]
   │   └── database-update [goroutine.id: 47]
   ├── process-item-2 [goroutine.id: 48]
   │   └── database-update [goroutine.id: 48]
   └── process-item-3 [goroutine.id: 49]
       └── database-update [goroutine.id: 49] [error: unique constraint violation]
   ```

3. **Root cause**: Concurrent updates without proper locking

## Advanced Techniques

### Custom Span Events

Add events for important lifecycle moments:

```go
span.AddEvent("cache-miss", map[string]interface{}{
    "cache.key": key,
    "cache.ttl": ttl,
})

span.AddEvent("database-query-start")
// ... execute query ...
span.AddEvent("database-query-end", map[string]interface{}{
    "db.duration_ms": time.Since(start).Milliseconds(),
    "db.rows_affected": rows,
})
```

### Baggage for Cross-Cutting Context

Pass context that should be available to all spans:

```go
// Set baggage at request start
ctx = baggage.ContextWithBaggage(ctx,
    baggage.FromContext(ctx).
        Member("tenant.id", tenantID).
        Member("user.id", userID).
        Member("request.id", requestID),
)

// Access in any span
baggage := baggage.FromContext(ctx)
tenantID := baggage.Member("tenant.id").Value()
span.SetAttribute("tenant.id", tenantID)
```

### Sampling Strategies

**Development**: Sample 100% of requests
```go
SamplingRate: 1.0
```

**Production**: Use head-based sampling
```go
SamplingRate: 0.1  // Sample 10% of requests
```

**Error-focused**: Always sample errors
```go
// Custom sampler that captures all errors
func ErrorSampler(parent sdktrace.SamplingParameters) sdktrace.SamplingResult {
    if hasError(parent) {
        return sdktrace.RecordAndSample
    }
    return sdktrace.Drop
}
```

## Integration with Other Tools

### Correlating Logs and Traces

Add trace context to logs:

```go
import "github.com/anomalyco/muster/pkg/log"

logger := log.WithContext(ctx).
    WithField("trace_id", telemetry.GetTraceID(ctx)).
    WithField("span_id", telemetry.GetSpanID(ctx))

logger.Info("Processing request")
// Output: {"level":"info","trace_id":"abc123","span_id":"def456","message":"Processing request"}
```

### Metrics from Spans

Generate RED metrics from spans:

```go
// Rate: Count of requests
totalRequests.WithLabelValues(spanName).Inc()

// Errors: Count of error spans
if span.Status().Code == trace.StatusCodeError {
    errorRequests.WithLabelValues(spanName).Inc()
}

// Duration: Histogram of span durations
requestDuration.WithLabelValues(spanName).Observe(duration.Seconds())
```

### Alerting on Traces

Configure alerts for trace patterns:

```yaml
# Example AlertManager rule
groups:
  - name: tracing-alerts
    rules:
      - alert: HighErrorRate
        expr: |
          (
            sum(rate(traces_span_count{status="ERROR"}[5m]))
            /
            sum(rate(traces_span_count[5m]))
          ) > 0.05
        annotations:
          summary: "High error rate detected in traces"
          
      - alert: SlowOperation
        expr: |
          histogram_quantile(0.95, 
            sum(rate(traces_span_duration_seconds_bucket[5m])) by (span_name, le)
          ) > 2
        annotations:
          summary: "95th percentile latency > 2s"
```

## Best Practices

### Do's

✅ **Do** create spans for significant operations
- HTTP requests (automatic via middleware)
- Database queries
- Cache operations
- External API calls
- Business logic blocks

✅ **Do** use semantic attributes
```go
span.SetAttribute("http.method", "GET")
span.SetAttribute("http.status_code", 200)
span.SetAttribute("db.system", "postgresql")
span.SetAttribute("cache.hit", true)
```

✅ **Do** propagate context everywhere
```go
// Good
func Process(ctx context.Context, data []byte) error {
    ctx, span := tracer.Start(ctx, "process")
    defer span.End()
    return db.Save(ctx, data)  // Pass ctx!
}

// Bad
func Process(ctx context.Context, data []byte) error {
    ctx, span := tracer.Start(ctx, "process")
    defer span.End()
    return db.Save(data)  // Context lost!
}
```

✅ **Do** handle errors properly
```go
if err != nil {
    span.SetError(err)
    span.SetStatus(trace.Status{Code: trace.StatusCodeError, Description: err.Error()})
    return err
}
```

### Don'ts

❌ **Don't** create spans for trivial operations
- Simple getter/setter methods
- Fast in-memory operations (< 1ms)
- Loop iterations (create one span for the whole loop)

❌ **Don't** add sensitive data to spans
```go
// Bad - PII in spans
span.SetAttribute("user.password", password)
span.SetAttribute("credit_card.number", cardNumber)

// Good - Only non-sensitive identifiers
span.SetAttribute("user.id", userID)
span.SetAttribute("payment.transaction_id", txnID)
```

❌ **Don't** forget to end spans
```go
// Always use defer
ctx, span := tracer.Start(ctx, "operation")
defer span.End()

// Or use helper
telemetry.WithSpan(ctx, "operation", func(ctx context.Context) {
    // Your code here - span auto-ended
})
```

## Troubleshooting

### Traces Not Appearing

**Check**:
1. Exporter endpoint is correct
2. Network connectivity to collector
3. Sampling rate > 0
4. Service name is set

**Debug**:
```go
// Enable debug logging
config := telemetry.TracingConfig{
    Enabled:     true,
    DebugMode:   true,  // Log all spans
}
```

### Broken Trace Chains

**Cause**: Context not propagated

**Fix**:
```go
// Ensure ctx is passed to all child operations
result, err := processData(ctx, input)  // ✓ Good
result, err := processData(input)       // ✗ Bad - context lost
```

### High Memory Usage

**Cause**: Too many spans or large attributes

**Fix**:
```go
// Limit span attributes
span.SetAttribute("data.size", len(data))  // ✓ Good
span.SetAttribute("data.content", data)   // ✗ Bad - too large

// Use sampling in production
SamplingRate: 0.1  // Sample 10%
```

## Reference

### Key Span Attributes

| Attribute | Description | Example |
|-----------|-------------|---------|
| `http.method` | HTTP method | `GET`, `POST` |
| `http.url` | Full URL | `https://api.example.com/v1/users` |
| `http.status_code` | Response status | `200`, `404` |
| `db.system` | Database type | `postgresql`, `mysql` |
| `db.statement` | SQL query | `SELECT * FROM users` |
| `db.rows_affected` | Rows modified | `42` |
| `cache.hit` | Cache lookup result | `true`, `false` |
| `cache.key` | Cache key | `user:12345` |
| `error` | Error flag | `true` |
| `error.message` | Error description | `connection timeout` |

### Trace Context Headers

| Header | Description |
|--------|-------------|
| `traceparent` | W3C trace context (trace-id, span-id, trace-flags) |
| `tracestate` | Vendor-specific trace context |
| `baggage` | User-defined context items |

---

For more information, see:
- [OpenTelemetry Documentation](https://opentelemetry.io/docs/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
- [Jaeger Documentation](https://www.jaegertracing.io/docs/)
