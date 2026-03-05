# API Error Responses Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

## Overview

This specification defines the standardized error handling and response formats for all Muster API endpoints. All API errors follow a consistent JSON structure with standard HTTP status codes, error codes, and human-readable messages to ensure predictable client behavior and clear debugging capabilities.

### Design Principles

- **Consistency**: All errors use the same JSON structure
- **Clarity**: Error messages are human-readable and actionable
- **Debugging**: Request IDs and timestamps enable traceability
- **Documentation**: Every error links to detailed documentation
- **Extensibility**: Details field allows additional context

## Standard Error Format

```json
{
  "error": {
    "code": "RESOURCE_NOT_FOUND",
    "message": "The requested resource was not found",
    "details": {
      "resource_type": "pod",
      "resource_id": "abc-123"
    },
    "request_id": "req-550e8400",
    "timestamp": "2024-01-15T10:30:00Z",
    "documentation_url": "https://docs.muster.io/errors/RESOURCE_NOT_FOUND"
  }
}
```

### Field Descriptions

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `error.code` | string | Yes | Machine-readable error code (UPPER_SNAKE_CASE) |
| `error.message` | string | Yes | Human-readable error description |
| `error.details` | object | No | Additional context specific to error type |
| `error.request_id` | string | Yes | Unique identifier for request tracing |
| `error.timestamp` | ISO 8601 | Yes | When the error occurred (UTC) |
| `error.documentation_url` | string | No | Link to detailed error documentation |

## Error Codes

| Code | Description | HTTP Status | Retryable |
|------|-------------|-------------|-----------|
| `AUTH_REQUIRED` | Authentication required | 401 | No |
| `AUTH_INVALID` | Invalid credentials | 401 | No |
| `FORBIDDEN` | Permission denied | 403 | No |
| `RESOURCE_NOT_FOUND` | Resource not found | 404 | No |
| `VALIDATION_ERROR` | Request validation failed | 422 | No |
| `RATE_LIMITED` | Too many requests | 429 | Yes (after delay) |
| `INTERNAL_ERROR` | Server error | 500 | Yes (with backoff) |
| `SERVICE_UNAVAILABLE` | Service temporarily unavailable | 503 | Yes (with backoff) |
| `GATEWAY_TIMEOUT` | Upstream timeout | 504 | Yes (with backoff) |
| `CONFLICT` | Resource conflict | 409 | No |
| `PRECONDITION_FAILED` | Precondition failed | 412 | No |

## HTTP Status Mapping

```go
package errors

import (
    "encoding/json"
    "net/http"
    "time"
)

type APIError struct {
    Code             string                 `json:"code"`
    Message          string                 `json:"message"`
    Details          map[string]interface{} `json:"details,omitempty"`
    RequestID        string                 `json:"request_id"`
    Timestamp        time.Time              `json:"timestamp"`
    DocumentationURL string                 `json:"documentation_url,omitempty"`
}

func (e *APIError) Error() string {
    return e.Message
}

func (e *APIError) StatusCode() int {
    switch e.Code {
    case "AUTH_REQUIRED", "AUTH_INVALID":
        return http.StatusUnauthorized
    case "FORBIDDEN":
        return http.StatusForbidden
    case "RESOURCE_NOT_FOUND":
        return http.StatusNotFound
    case "VALIDATION_ERROR":
        return http.StatusUnprocessableEntity
    case "RATE_LIMITED":
        return http.StatusTooManyRequests
    case "CONFLICT":
        return http.StatusConflict
    case "PRECONDITION_FAILED":
        return http.StatusPreconditionFailed
    case "INTERNAL_ERROR":
        return http.StatusInternalServerError
    case "SERVICE_UNAVAILABLE":
        return http.StatusServiceUnavailable
    case "GATEWAY_TIMEOUT":
        return http.StatusGatewayTimeout
    default:
        return http.StatusInternalServerError
    }
}
```

## Error Response Schema

```go
func NewError(code string, message string, details map[string]interface{}) *APIError {
    return &APIError{
        Code:      code,
        Message:   message,
        Details:   details,
        Timestamp: time.Now().UTC(),
    }
}

func NotFound(resourceType, resourceID string) *APIError {
    return NewError(
        "RESOURCE_NOT_FOUND",
        fmt.Sprintf("%s with id '%s' was not found", resourceType, resourceID),
        map[string]interface{}{
            "resource_type": resourceType,
            "resource_id":   resourceID,
        },
    )
}

func ValidationError(field string, reason string) *APIError {
    return NewError(
        "VALIDATION_ERROR",
        fmt.Sprintf("Validation failed for field '%s': %s", field, reason),
        map[string]interface{}{
            "field":  field,
            "reason": reason,
        },
    )
}

func RateLimit(remaining int, resetAt time.Time) *APIError {
    return &APIError{
        Code:    "RATE_LIMITED",
        Message: "Rate limit exceeded. Please slow down your requests.",
        Details: map[string]interface{}{
            "remaining": remaining,
            "reset_at":  resetAt.Format(time.RFC3339),
        },
        Timestamp: time.Now().UTC(),
    }
}
```

## Error Response Builder

```go
type ErrorResponseBuilder struct {
    error *APIError
}

func NewErrorResponse() *ErrorResponseBuilder {
    return &ErrorResponseBuilder{
        error: &APIError{
            Timestamp: time.Now().UTC(),
        },
    }
}

func (b *ErrorResponseBuilder) WithCode(code string) *ErrorResponseBuilder {
    b.error.Code = code
    return b
}

func (b *ErrorResponseBuilder) WithMessage(message string) *ErrorResponseBuilder {
    b.error.Message = message
    return b
}

func (b *ErrorResponseBuilder) WithDetail(key string, value interface{}) *ErrorResponseBuilder {
    if b.error.Details == nil {
        b.error.Details = make(map[string]interface{})
    }
    b.error.Details[key] = value
    return b
}

func (b *ErrorResponseBuilder) WithRequestID(requestID string) *ErrorResponseBuilder {
    b.error.RequestID = requestID
    return b
}

func (b *ErrorResponseBuilder) Build() *APIError {
    if b.error.DocumentationURL == "" {
        b.error.DocumentationURL = fmt.Sprintf("https://docs.muster.io/errors/%s", b.error.Code)
    }
    return b.error
}
```

## Examples

### 404 Not Found

```json
{
  "error": {
    "code": "RESOURCE_NOT_FOUND",
    "message": "Pod with id 'web-server-abc' was not found",
    "details": {
      "resource_type": "pod",
      "resource_id": "web-server-abc",
      "namespace": "production"
    },
    "request_id": "req-550e8400",
    "timestamp": "2024-01-15T10:30:00Z",
    "documentation_url": "https://docs.muster.io/errors/RESOURCE_NOT_FOUND"
  }
}
```

### 422 Validation Error

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Validation failed for field 'replicas': must be between 1 and 100",
    "details": {
      "field": "replicas",
      "reason": "must be between 1 and 100",
      "provided_value": 0,
      "constraints": {
        "min": 1,
        "max": 100
      }
    },
    "request_id": "req-660f9511",
    "timestamp": "2024-01-15T10:30:00Z"
  }
}
```

### 429 Rate Limited

```json
{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Rate limit exceeded. Please slow down your requests.",
    "details": {
      "remaining": 0,
      "reset_at": "2024-01-15T10:31:00Z",
      "limit": 100,
      "window": "1m"
    },
    "request_id": "req-770g0622",
    "timestamp": "2024-01-15T10:30:00Z"
  }
}
```

**HTTP Headers:**

```http
Retry-After: 60
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1705317060
```

## Go Handler Integration

```go
func WriteError(w http.ResponseWriter, err *APIError) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(err.StatusCode())
    
    response := struct {
        Error *APIError `json:"error"`
    }{Error: err}
    
    json.NewEncoder(w).Encode(response)
}

func ErrorMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                err := NewError("INTERNAL_ERROR", "Internal server error", nil).
                    WithRequestID(middleware.GetReqID(r.Context())).Build()
                WriteError(w, err)
            }
        }()
        
        next.ServeHTTP(w, r)
    })
}
```

## Client Error Handling

### TypeScript Client Example

```typescript
class APIError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: Record<string, unknown>,
    public requestId?: string,
    public timestamp?: string
  ) {
    super(message);
    this.name = 'APIError';
  }
}

async function handleAPIResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const errorData = await response.json();
    throw new APIError(
      errorData.error.code,
      errorData.error.message,
      errorData.error.details,
      errorData.error.request_id,
      errorData.error.timestamp
    );
  }
  return response.json();
}
```

### Python Client Example

```python
from typing import Dict, Any, Optional
from datetime import datetime

class APIError(Exception):
    def __init__(
        self,
        code: str,
        message: str,
        details: Optional[Dict[str, Any]] = None,
        request_id: Optional[str] = None,
        timestamp: Optional[str] = None
    ):
        super().__init__(message)
        self.code = code
        self.message = message
        self.details = details
        self.request_id = request_id
        self.timestamp = timestamp
```

## Error Logging

All errors SHOULD be logged with:
- Full error structure
- Request context (method, path, client IP)
- Stack trace for 5xx errors
- Correlation ID for distributed tracing

```go
func LogError(logger *log.Logger, err *APIError, r *http.Request) {
    logger.WithFields(log.Fields{
        "error_code":    err.Code,
        "request_id":    err.RequestID,
        "method":        r.Method,
        "path":          r.URL.Path,
        "client_ip":     r.RemoteAddr,
        "status_code":   err.StatusCode(),
    }).Error(err.Message)
}
```

---

**Bible Standard Compliance**: This specification adheres to the Muster Bible Standard for API consistency, error handling, and observability. All implementations must maintain backward compatibility within major versions.
