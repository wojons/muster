# Logging Standards

This document defines the structured logging standards for Muster.

## Overview

Muster uses structured JSON logging via [zerolog](https://github.com/rs/zerolog) for all log output. This enables:
- Machine-readable log parsing
- Correlation across distributed systems
- Efficient log aggregation and search
- Standardized log analysis

## Log Levels

| Level | Usage | Example |
|-------|-------|---------|
| TRACE | Detailed tracing information | Function entry/exit, variable values |
| DEBUG | Development debugging | Request/response details, state changes |
| INFO | Normal operations | Service startup, configuration loaded |
| WARN | Recoverable issues | Degraded performance, retry attempts |
| ERROR | Operation failures | Failed requests, authentication errors |
| FATAL | Unrecoverable errors | Cannot start, critical dependency failure |

## Log Schema

All log entries follow this JSON schema:

```json
{
  "level": "info",
  "timestamp": "2026-04-02T10:30:00.123Z",
  "message": "Request processed successfully",
  "service": "muster-api",
  "version": "1.0.0",
  "correlation_id": "abc123-def456",
  "trace_id": "trace-xyz789",
  "span_id": "span-abc123",
  "component": "http-server",
  "operation": "handle_request",
  "duration_ms": 45.2,
  "tenant_id": "tenant-123",
  "user_id": "user-456",
  "request_id": "req-789",
  "method": "GET",
  "path": "/api/v1/pets",
  "status_code": 200,
  "client_ip": "192.168.1.100",
  "user_agent": "Muster-CLI/1.0.0",
  "error": null,
  "stack_trace": null,
  "metadata": {
    "custom_field": "value"
  }
}
```

### Required Fields

- `level`: Log level (trace, debug, info, warn, error, fatal)
- `timestamp`: ISO8601 formatted timestamp with millisecond precision
- `message`: Human-readable log message
- `service`: Service name (muster-cli, muster-mcp, muster-daemon)

### Context Fields

- `correlation_id`: Unique identifier for tracking requests across services
- `trace_id`: OpenTelemetry trace ID for distributed tracing
- `span_id`: OpenTelemetry span ID
- `component`: Subsystem or module name
- `operation`: Operation being performed

### Request Fields (when applicable)

- `request_id`: Unique request identifier
- `method`: HTTP method or operation type
- `path`: Request path or resource
- `status_code`: HTTP status code or operation result code
- `duration_ms`: Operation duration in milliseconds
- `client_ip`: Client IP address
- `user_agent`: Client user agent string

### User/Tenant Fields (when applicable)

- `tenant_id`: Tenant identifier for multi-tenant operations
- `user_id`: User identifier
- `session_id`: Session identifier

### Error Fields (when applicable)

- `error`: Error message or error type
- `error_code`: Standardized error code
- `stack_trace`: Stack trace for errors (debug/fatal only)

## Field Naming Conventions

- Use snake_case for field names
- Keep names short but descriptive
- Use consistent terminology across the codebase
- Avoid abbreviations except for standard ones (id, ip, url)

## Sensitive Data Handling

Sensitive fields are automatically redacted:

- password, passwd
- secret, api_key, apikey
- token, access_token, refresh_token
- credential, client_secret
- private_key, encryption_key
- master_key, signing_key
- certificate_key

Redacted values appear as `***REDACTED***`.

## Log Output

### Development

```json
{
  "level": "info",
  "timestamp": "2026-04-02T10:30:00.123Z",
  "message": "Server started",
  "service": "muster-daemon",
  "port": 8080
}
```

### Production

- Output: JSON to stdout/stderr
- Collection: Log aggregation system (ELK/Loki)
- Retention: 30 days hot, 90 days cold
- Sampling: INFO+ 100%, DEBUG 10%, TRACE 1%

## Best Practices

1. **Always use structured logging**: Use `log.Info().Str("key", "value").Msg("message")`
2. **Include correlation IDs**: Pass context through all operations
3. **Log at appropriate levels**: Don't spam with INFO, use DEBUG for details
4. **Add context gradually**: Build up log context as operations progress
5. **Never log sensitive data**: Let the redaction handle it, but don't intentionally log secrets
6. **Use consistent keys**: Same concept → same field name everywhere
7. **Log errors with context**: Include what was being attempted when error occurred
8. **Log durations**: Always log operation timing for performance analysis

## Examples

### Basic Logging

```go
log.Info().
    Str("component", "api").
    Str("operation", "get_pet").
    Int("pet_id", 123).
    Msg("Retrieved pet successfully")
```

### With Context

```go
ctx := log.WithCorrelationID(ctx, correlationID)
log.FromContext(ctx).Info().
    Str("method", "GET").
    Str("path", "/api/v1/pets/123").
    Int("status_code", 200).
    Float64("duration_ms", 45.2).
    Msg("Request completed")
```

### Error Logging

```go
log.Error().
    Err(err).
    Str("component", "database").
    Str("operation", "query").
    Str("table", "pets").
    Msg("Failed to query database")
```

## Configuration

Configure logging via environment variables or config file:

```yaml
logging:
  level: info
  format: json
  output: stdout
  correlation_id_header: X-Correlation-ID
  enable_caller: false
  enable_stacktrace: false
```

## Migration Guide

When migrating from unstructured logging:

1. Replace `fmt.Printf` with structured logging
2. Add correlation ID propagation to context
3. Update log aggregation queries to use JSON fields
4. Configure log retention policies
5. Set up log-based alerting

## Related Documents

- [Metrics Standardization](./METRICS-STANDARDS.md)
- [OpenTelemetry Integration](./tracing.md)
- [Security Logging](./security-logging.md)
