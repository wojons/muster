# Metrics Standardization Guide

## Overview

This document describes the standardized metrics naming convention and methodology used in Muster, following Prometheus best practices and the RED/USE methodologies for observability.

## Metric Naming Convention

All metrics follow the format: `<namespace>_<subsystem>_<name>_<unit>_<suffix>`

### Metric Types and Suffixes

| Type | Suffix | Example |
|------|--------|---------|
| Counter | `_total` | `http_requests_total` |
| Gauge | none | `memory_usage_bytes` |
| Histogram | `_bucket`, `_sum`, `_count` | `request_duration_seconds_bucket` |
| Summary | `_sum`, `_count` | `request_latency_seconds_sum` |

### Base Units

As per Prometheus best practices, always use base units:

- **Time**: seconds (not milliseconds, microseconds)
- **Bytes**: bytes (not kilobytes, megabytes)
- **Percent**: ratio (0.0-1.0) or percent (0-100)

## RED Method (Request-focused)

For each service endpoint or operation, track these three metrics:

### Rate: Total Requests

```go
requestsTotal := promauto.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "muster",
        Name:      "requests_total",
        Help:      "Total number of requests (RED: Rate)",
    },
    []string{"method", "endpoint", "status"},
)
```

### Errors: Total Errors

```go
errorsTotal := promauto.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "muster",
        Name:      "errors_total",
        Help:      "Total number of errors (RED: Errors)",
    },
    []string{"method", "endpoint", "error_type"},
)
```

### Duration: Request Latency

```go
requestDuration := promauto.NewHistogramVec(
    prometheus.HistogramOpts{
        Namespace: "muster",
        Name:      "request_duration_seconds",
        Help:      "Request duration in seconds (RED: Duration)",
        Buckets:   prometheus.DefBuckets,
    },
    []string{"method", "endpoint", "status"},
)
```

## USE Method (Resource-focused)

For each resource (CPU, memory, connections, etc.), track these three metrics:

### Utilization: Percentage Used

```go
utilizationGauge := promauto.NewGaugeVec(
    prometheus.GaugeOpts{
        Namespace: "muster",
        Name:      "utilization_percent",
        Help:      "Resource utilization percentage (USE: Utilization)",
    },
    []string{"resource"},
)
```

### Saturation: Time Waiting

```go
saturationGauge := promauto.NewGaugeVec(
    prometheus.GaugeOpts{
        Namespace: "muster",
        Name:      "saturation_wait_seconds",
        Help:      "Time waiting for resource in seconds (USE: Saturation)",
    },
    []string{"resource"},
)
```

### Errors: Resource Errors

```go
resourceErrors := promauto.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "muster",
        Name:      "resource_errors_total",
        Help:      "Total resource errors (USE: Errors)",
    },
    []string{"resource", "error_type"},
)
```

## Cardinality Controls

To prevent metrics explosion and high storage costs, follow these guidelines:

### What NOT to Use as Labels

❌ **High-cardinality values that should never be used as labels:**

- User IDs
- Session IDs
- Request IDs
- IP addresses (unless in a CIDR block)
- Timestamps
- UUIDs
- SHA256/MD5 hashes
- Unbounded enum values

### Safe Label Values

✅ **Low-cardinality values that are safe to use:**

- HTTP methods: GET, POST, PUT, DELETE
- Status code categories: 2xx, 4xx, 5xx
- Operation names: getPet, listPets, createPet
- Resource types: cpu, memory, disk, network
- Error types: connection_refused, timeout, validation_error
- Tenant IDs (if bounded)

### Endpoint Normalization

Dynamic endpoints should be normalized to prevent high cardinality:

```go
// Bad - creates separate metric for each user
"/api/users/12345"
"/api/users/67890"

// Good - normalized to pattern
"/api/users/:id"
```

The `NormalizeEndpoint()` function automatically handles this:

```go
normalized := monitoring.NormalizeEndpoint("/api/users/550e8400-e29b-41d4-a716-446655440000")
// Result: "/api/users/:id"
```

### Status Code Grouping

Group HTTP status codes to reduce cardinality:

```go
code := 404
normalized := monitoring.NormalizeStatusCode(code)
// Result: "4xx"
```

## Cardinality Limits

The following limits are enforced to prevent metrics explosion:

| Limit | Value |
|-------|-------|
| Max label values per label | 100 |
| Max label name length | 1024 characters |
| Max label value length | 1024 characters |
| Max metric name length | 1024 characters |

## Standard Labels

Use these standard labels consistently across metrics:

| Label | Description | Example |
|-------|-------------|---------|
| `method` | HTTP method or operation type | GET, POST, createPet |
| `endpoint` | API endpoint or operation name | /api/users, getPet |
| `status` | Response status code category | 2xx, 4xx, 5xx |
| `error_type` | Type of error for categorization | timeout, validation_error |
| `resource` | Resource name | cpu, memory, db_connections |
| `tenant_id` | Tenant identifier (low cardinality) | tenant-1, tenant-2 |
| `component` | Service component name | api, daemon, mcp |
| `version` | Application version | v1.2.3 |

## Usage Examples

### Basic RED Metrics

```go
// Initialize standard metrics
metrics := monitoring.NewStandardMetrics("muster")

// Record a request
start := time.Now()
// ... handle request ...
duration := time.Since(start)

status := "200"
if err != nil {
    status = "500"
}

metrics.RecordRequest("GET", "/api/users", status, duration)
```

### USE Metrics

```go
// Record CPU utilization
metrics.SetUtilization("cpu", 75.5)

// Record database connection wait time
metrics.RecordSaturation("db_connections", 20*time.Millisecond)

// Record resource error
metrics.RecordResourceError("memory", "oom")
```

### Custom Metrics

```go
// Validate label values before use
userRole := getUserRole()
if monitoring.IsValidLabelValue(userRole) {
    myCounter.WithLabelValues(userRole).Inc()
}

// Or use the cardinality tracker
if metrics.cardinality.TrackValue("role", userRole) {
    myCounter.WithLabelValues(userRole).Inc()
} else {
    // Cardinality limit exceeded, use "other"
    myCounter.WithLabelValues("other").Inc()
}
```

## Validation Functions

Use these helper functions to validate metric names and labels:

```go
// Validate metric name
if err := monitoring.ValidateMetricName("my_metric_total"); err != nil {
    log.Printf("Invalid metric name: %v", err)
}

// Validate label name
if err := monitoring.ValidateLabelName("method"); err != nil {
    log.Printf("Invalid label name: %v", err)
}

// Validate label value
if err := monitoring.ValidateLabelValue("GET"); err != nil {
    log.Printf("Invalid label value: %v", err)
}

// Check if value is safe for use as label
if monitoring.IsValidLabelValue(userID) {
    // Safe to use
} else {
    // Too high cardinality, group into "other"
}
```

## Reserved Labels

The following label names are reserved by Prometheus and cannot be used:

- `__name__`
- `le`
- `instance`
- `job`

## Best Practices

1. **Use StandardMetrics**: Prefer `NewStandardMetrics()` over creating metrics manually
2. **Normalize Endpoints**: Always use `NormalizeEndpoint()` for dynamic URLs
3. **Group Status Codes**: Use `NormalizeStatusCode()` to group HTTP codes
4. **Validate Cardinality**: Check `IsValidLabelValue()` before using dynamic values
5. **Document Metrics**: Add clear descriptions following the format in examples
6. **Test Metrics**: Write tests that verify metrics are recorded correctly
7. **Monitor Cardinality**: Watch for unexpected cardinality growth in production

## Metric Documentation

All metrics should be documented with:

1. **Namespace**: The application or service name
2. **Subsystem**: The component within the application
3. **Name**: The metric name (with appropriate suffix)
4. **Help**: Clear description following RED/USE methodology
5. **Labels**: Document all labels and their expected values

Example:

```go
promauto.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "muster",
        Subsystem: "api",
        Name:      "requests_total",
        Help:      "Total API requests (RED: Rate)",
    },
    []string{"method", "endpoint", "status"},
)
```

## Migration Guide

When migrating existing metrics to the standard format:

1. **Audit existing metrics**: Identify all metrics and their labels
2. **Check cardinality**: Verify no high-cardinality labels
3. **Update naming**: Rename to follow convention
4. **Update labels**: Normalize to standard labels
5. **Test**: Ensure all metrics are recorded correctly
6. **Document**: Update dashboards and alerts
7. **Deprecate**: Mark old metrics as deprecated
8. **Remove**: Remove old metrics after migration period

## Related Documentation

- [Prometheus Best Practices](https://prometheus.io/docs/practices/naming/)
- [RED Method](https://www.weave.works/blog/the-red-method-key-metrics-for-microservices-architecture/)
- [USE Method](http://www.brendangregg.com/usemethod.html)
- [Cardinality Guide](https://www.robustperception.io/cardinality-is-key/)
