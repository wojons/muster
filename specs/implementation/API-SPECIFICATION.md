# API Specification - Complete HTTP API Definitions

## Base URLs

### Client API (Inventory Server)
```
https://{host}/_inventory.json          # Main inventory endpoint
https://{host}/.well-known/openapi-cli/keys.json  # Emergency key endpoint
https://{host}/_revocation.json         # Revocation list
```

### Bundled Server API (Reference Implementation)
```
https://server.example.com/api/v1/
```

---

## 1. Inventory Endpoint

### GET /_inventory.json

**Description**: Returns the inventory configuration for the CLI/MCP tool.

**Request Headers**:
```
Authorization: Bearer {token}           # Optional: Inventory auth
If-None-Match: {etag}                   # Optional: HTTP caching
Accept: application/json
```

**Query Parameters**:
- `client_version` (string): Client CLI version
- `fingerprint` (string): Client fingerprint
- `user_id` (string): User identifier for dynamic configs

**Response 200 OK**:
```json
{
  "version": "1.2.3",
  "min_cli_version": "1.0.0",
  "max_cli_version": "2.0.0",
  "updated_at": "2026-02-28T15:30:00Z",
  
  "openapi": {
    "source": "url",
    "url": "https://api.example.com/openapi.json",
    "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "signature": "base64signature..."
  },
  
  "modules": {
    "processors": [
      {
        "name": "data-transform",
        "url": "https://cdn.example.com/modules/transform.wasm",
        "sha256": "abc123...",
        "signature": "sig456...",
        "capabilities": ["http"]
      }
    ]
  },
  
  "config": {
    "rate_limiting": {
      "requests_per_minute": 100,
      "burst": 10
    },
    "retry": {
      "max_attempts": 3,
      "backoff": "exponential",
      "initial_delay_ms": 1000
    },
    "fingerprinting": {
      "enabled": true,
      "regenerate_on_ip_change": true,
      "components": ["ip", "hardware", "random"]
    }
  },
  
  "auth": {
    "inventory": {
      "type": "bearer",
      "token_env": "INV_TOKEN",
      "refresh_url": "https://auth.example.com/refresh"
    },
    "api": {
      "type": "oauth2",
      "flow": "client_credentials",
      "token_url": "https://auth.example.com/token",
      "client_id": "client123",
      "scopes": ["api:read", "api:write"]
    }
  },
  
  "signatures": {
    "method": "ecdsa-p256",
    "key_id": "key-2024-03",
    "signature": "base64..."
  }
}
```

**Response 304 Not Modified**:
```
HTTP/1.1 304 Not Modified
ETag: "abc123"
Cache-Control: max-age=3600
```

**Response 401 Unauthorized**:
```json
{
  "error": "unauthorized",
  "message": "Invalid or missing inventory authentication",
  "code": "INV_AUTH_FAILED"
}
```

**Response 403 Forbidden**:
```json
{
  "error": "forbidden",
  "message": "Access denied for this client version",
  "code": "VERSION_NOT_ALLOWED",
  "min_version": "1.5.0",
  "current_version": "1.2.0"
}
```

---

## 2. Revocation Endpoint

### GET /_revocation.json

**Description**: Returns list of revoked inventories, modules, and fingerprints.

**Request Headers**:
```
If-None-Match: {etag}
Accept: application/json
```

**Response 200 OK**:
```json
{
  "updated_at": "2026-02-28T15:30:00Z",
  "revocations": {
    "inventories": [
      {
        "host": "api.example.com",
        "version": "1.0.0",
        "reason": "compromised_signing_key",
        "severity": "critical",
        "revoked_at": "2026-02-28T14:00:00Z"
      }
    ],
    "modules": [
      {
        "sha256": "def456...",
        "reason": "malicious_code",
        "severity": "critical",
        "revoked_at": "2026-02-28T13:00:00Z"
      }
    ],
    "fingerprints": [
      {
        "fingerprint": "fp_abc123",
        "reason": "suspicious_activity",
        "severity": "high",
        "revoked_at": "2026-02-28T12:00:00Z"
      }
    ]
  },
  "emergency": {
    "enabled": false,
    "message": "Service temporarily disabled",
    "contact": "security@example.com"
  }
}
```

---

## 3. Key Server API

### POST /api/v1/credentials

**Description**: Fetch encrypted credentials from key server.

**Request**:
```json
{
  "client_id": "client_abc123",
  "fingerprint": "fp_def456",
  "credential_types": ["api_token", "signing_key"]
}
```

**Request Headers**:
```
Authorization: Bearer {key_server_token}
Content-Type: application/json
```

**Response 200 OK**:
```json
{
  "credentials": [
    {
      "type": "api_token",
      "encrypted_data": "base64encrypted...",
      "key_id": "key-2024-03",
      "algorithm": "aes-256-gcm",
      "expires_at": "2026-03-01T00:00:00Z"
    }
  ],
  "key_server_public_key": "base64publickey..."
}
```

---

## 4. Webhook Proxy API (Reference Server)

### WebSocket /ws/webhooks

**Description**: Client connection for receiving webhook forwards.

**Connection**:
```javascript
const ws = new WebSocket('wss://server.example.com/ws/webhooks', [], {
  headers: {
    'Authorization': 'Bearer ' + clientToken
  }
});
```

**Message Format (Server → Client)**:
```json
{
  "type": "webhook",
  "webhook_id": "wh_abc123",
  "method": "POST",
  "path": "/webhook/payment",
  "headers": {
    "Content-Type": "application/json",
    "X-Signature": "sig123"
  },
  "body": "base64encoded...",
  "timestamp": "2026-02-28T15:30:00Z"
}
```

**Response Format (Client → Server)**:
```json
{
  "webhook_id": "wh_abc123",
  "status_code": 200,
  "headers": {
    "Content-Type": "application/json"
  },
  "body": "base64encoded..."
}
```

### HTTP POST /webhook/{client-id}

**Description**: External webhook endpoint that forwards to client.

**Request Headers**:
```
Content-Type: application/json
X-Webhook-Signature: sha256=signature
```

**Request Body**: Arbitrary webhook payload

**Response 200 OK**: Proxied client response
**Response 404 Not Found**: Client not connected
**Response 504 Gateway Timeout**: Client didn't respond in time

---

## 5. Bundled Server Admin API

### GET /api/v1/admin/health

**Response 200 OK**:
```json
{
  "status": "healthy",
  "checks": {
    "database": {"status": "healthy", "response_time_ms": 5},
    "storage": {"status": "healthy", "response_time_ms": 12}
  },
  "timestamp": "2026-02-28T15:30:00Z"
}
```

### GET /api/v1/admin/metrics

**Response**: Prometheus metrics text

### POST /api/v1/admin/rotate-keys

**Request**:
```json
{
  "key_type": "signing",
  "grace_period_days": 7
}
```

**Response 202 Accepted**:
```json
{
  "rotation_id": "rot_abc123",
  "status": "in_progress",
  "new_key_id": "key-2024-04"
}
```

---

## 6. Error Response Schema

All errors follow this schema:

```json
{
  "error": "error_code",
  "message": "Human-readable description",
  "code": "MACHINE_READABLE_CODE",
  "details": {
    "field": "additional context"
  },
  "request_id": "req_abc123",
  "timestamp": "2026-02-28T15:30:00Z"
}
```

**Error Codes**:
- `INV_AUTH_FAILED` - Inventory authentication failed
- `VERSION_NOT_ALLOWED` - CLI version not permitted
- `SIGNATURE_INVALID` - Inventory signature verification failed
- `MODULE_NOT_FOUND` - Requested module not found
- `RATE_LIMIT_EXCEEDED` - Rate limit hit
- `CREDENTIAL_EXPIRED` - Credentials expired
- `WEBHOOK_TIMEOUT` - Webhook forward timed out
- `INTERNAL_ERROR` - Internal server error

---

## 7. HTTP Caching

**Inventory Endpoint**:
```
Cache-Control: max-age=3600, must-revalidate
ETag: "abc123"
Last-Modified: Wed, 28 Feb 2026 15:30:00 GMT
Vary: Authorization, Accept-Encoding
```

**Module Assets**:
```
Cache-Control: public, max-age=31536000, immutable
ETag: "sha256:abc123"
```

**Revocation List**:
```
Cache-Control: max-age=60, must-revalidate
ETag: "rev-abc123"
```

---

## 8. Rate Limiting Headers

**Response Headers**:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1709139600
X-RateLimit-Retry-After: 60
```

**Rate Limit Response (429)**:
```json
{
  "error": "rate_limit_exceeded",
  "message": "Rate limit exceeded. Retry after 60 seconds.",
  "code": "RATE_LIMIT_EXCEEDED",
  "retry_after": 60
}
```

---

## 9. Content Types

All API endpoints accept/return:
- `application/json` (default)
- `application/yaml` (if requested via Accept header)
- `application/octet-stream` (for WASM modules)

---

## 10. Security Headers

All responses include:
```
Strict-Transport-Security: max-age=31536000; includeSubDomains
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Content-Security-Policy: default-src 'none'
```

---

## 11. Go HTTP Client Implementation

```go
package api

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/go-resty/resty/v2"
)

// Client wraps the HTTP client with inventory-specific logic
type Client struct {
    httpClient *resty.Client
    baseURL    string
    authToken  string
}

// NewClient creates a new API client
func NewClient(baseURL string) *Client {
    client := resty.New()
    client.SetTimeout(30 * time.Second)
    client.SetRetryCount(3)
    client.SetRetryWaitTime(1 * time.Second)
    client.AddRetryCondition(func(r *resty.Response, err error) bool {
        return r.StatusCode() == http.StatusTooManyRequests ||
               (r.StatusCode() >= 500 && r.StatusCode() < 600)
    })

    return &Client{
        httpClient: client,
        baseURL:    baseURL,
    }
}

// FetchInventory retrieves the inventory configuration
func (c *Client) FetchInventory(ctx context.Context, opts FetchOptions) (*Inventory, error) {
    req := c.httpClient.R().
        SetContext(ctx).
        SetHeader("Accept", "application/json")

    if opts.ETag != "" {
        req.SetHeader("If-None-Match", opts.ETag)
    }

    if c.authToken != "" {
        req.SetHeader("Authorization", "Bearer "+c.authToken)
    }

    if opts.ClientVersion != "" {
        req.SetQueryParam("client_version", opts.ClientVersion)
    }

    resp, err := req.Get(c.baseURL + "/_inventory.json")
    if err != nil {
        return nil, fmt.Errorf("failed to fetch inventory: %w", err)
    }

    if resp.StatusCode() == http.StatusNotModified {
        return nil, ErrNotModified
    }

    if resp.StatusCode() != http.StatusOK {
        return nil, parseError(resp)
    }

    var inv Inventory
    if err := json.Unmarshal(resp.Body(), &inv); err != nil {
        return nil, fmt.Errorf("failed to parse inventory: %w", err)
    }

    inv.ETag = resp.Header().Get("ETag")
    inv.LastModified = resp.Header().Get("Last-Modified")

    return &inv, nil
}

// FetchRevocationList retrieves the revocation list
func (c *Client) FetchRevocationList(ctx context.Context, etag string) (*RevocationList, error) {
    req := c.httpClient.R().
        SetContext(ctx).
        SetHeader("Accept", "application/json")

    if etag != "" {
        req.SetHeader("If-None-Match", etag)
    }

    resp, err := req.Get(c.baseURL + "/_revocation.json")
    if err != nil {
        return nil, fmt.Errorf("failed to fetch revocation list: %w", err)
    }

    if resp.StatusCode() == http.StatusNotModified {
        return nil, ErrNotModified
    }

    if resp.StatusCode() != http.StatusOK {
        return nil, parseError(resp)
    }

    var list RevocationList
    if err := json.Unmarshal(resp.Body(), &list); err != nil {
        return nil, fmt.Errorf("failed to parse revocation list: %w", err)
    }

    return &list, nil
}

// parseError converts HTTP error responses to Go errors
func parseError(resp *resty.Response) error {
    var apiErr APIError
    if err := json.Unmarshal(resp.Body(), &apiErr); err != nil {
        return fmt.Errorf("HTTP %d: %s", resp.StatusCode(), resp.String())
    }

    return &apiErr
}
```

---

## 12. Data Models (Go Structs)

```go
// Inventory represents the main configuration
type Inventory struct {
    Version        string          `json:"version"`
    MinCLIVersion  string          `json:"min_cli_version"`
    MaxCLIVersion  string          `json:"max_cli_version"`
    UpdatedAt      time.Time       `json:"updated_at"`
    OpenAPI        OpenAPISource   `json:"openapi"`
    Modules        ModuleConfig    `json:"modules"`
    Config         SystemConfig    `json:"config"`
    Auth           AuthConfig      `json:"auth"`
    Signatures     Signatures      `json:"signatures"`
    
    // HTTP metadata
    ETag           string          `json:"-"`
    LastModified   string          `json:"-"`
}

// OpenAPISource defines where to get the OpenAPI spec
type OpenAPISource struct {
    Source   string `json:"source"` // "embedded" or "url"
    Content  string `json:"content,omitempty"`
    URL      string `json:"url,omitempty"`
    SHA256   string `json:"sha256"`
    Signature string `json:"signature,omitempty"`
}

// ModuleConfig defines available WASM modules
type ModuleConfig struct {
    Processors []Module `json:"processors"`
    Validators []Module `json:"validators,omitempty"`
}

// Module represents a WASM module
type Module struct {
    Name        string   `json:"name"`
    URL         string   `json:"url"`
    SHA256      string   `json:"sha256"`
    Signature   string   `json:"signature"`
    Capabilities []string `json:"capabilities"`
}

// SystemConfig contains client-side controls
type SystemConfig struct {
    RateLimiting   RateLimitConfig `json:"rate_limiting"`
    Retry          RetryConfig     `json:"retry"`
    Fingerprinting FingerprintConfig `json:"fingerprinting"`
}

// AuthConfig defines authentication requirements
type AuthConfig struct {
    Inventory AuthMethod `json:"inventory"`
    API       AuthMethod `json:"api"`
}

// AuthMethod specifies authentication type and parameters
type AuthMethod struct {
    Type      string            `json:"type"` // "bearer", "oauth2", "mtls", etc.
    Params    map[string]string `json:"-"`    // Type-specific params
}
```

---

## Deterministic Guarantees

This API specification must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic JSON Serialization

All API request/response JSON must be serialized deterministically:

- **Key ordering**: Object keys must be sorted lexicographically (UTF‑8 byte order)
- **Whitespace**: No extra whitespace (compact JSON)
- **Number formatting**: Use Go's `json.Marshal` default (IEEE 754 double precision)
- **Date formatting**: RFC 3339 with nanosecond precision and "Z" suffix for UTC

```go
// internal/determinism/json.go

func MarshalDeterministic(v interface{}) ([]byte, error) {
	// Use standard library with sorted keys option
	return json.Marshal(v)
}
```

### 2. HTTP Header Canonicalization

HTTP headers must be canonicalized identically across platforms:

- **Header name case**: Use `textproto.CanonicalMIMEHeaderKey()`
- **Header value whitespace**: Trim leading/trailing whitespace, collapse internal spaces
- **Multiple values**: Join with comma (RFC 7230)
- **Date headers**: RFC 1123 format (Mon, 02 Jan 2006 15:04:05 GMT)

### 3. URL Parameter Ordering

URL query parameters must be ordered deterministically:

- **Sort by name**, then by value (lexicographic)
- **Percent‑encoding**: Use uppercase hexadecimal digits (A‑F)
- **Reserved characters**: Encode per RFC 3986

```go
// internal/determinism/url.go

func EncodeQueryParams(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(url.QueryEscape(k))
		buf.WriteByte('=')
		buf.WriteString(url.QueryEscape(params[k]))
	}
	return buf.String()
}
```

### 4. Consistent Error Responses

Error responses must have identical structure across all endpoints:

- **Error ID**: Deterministic UUID v5 generated from error details
- **Timestamp**: UTC with nanosecond precision
- **Stack traces**: Only in debug mode, deterministic formatting

### 5. Platform‑Independent Authentication

Authentication headers must be platform‑independent:

- **Bearer tokens**: Case‑sensitive exact string match
- **mTLS certificates**: PEM encoding with deterministic ordering (subject, issuer, serial)
- **OAuth2**: Token introspection must produce identical results across platforms

### 6. Testing Determinism

All API tests must verify deterministic behavior:

```go
//go:build linux || darwin || windows
func TestAPIDeterminism(t *testing.T) {
	// Verify identical JSON serialization across platforms
	// Verify identical HTTP headers across platforms
	// Verify identical URL parameter ordering
}
```

### 7. Integration with SDK Generation

When generating SDKs for 12 languages, API clients must:

- Produce identical HTTP requests (headers, body, URL)
- Parse identical HTTP responses (error mapping, data extraction)
- Handle authentication with same header injection logic

**Verification**: Run `./scripts/validate‑determinism‑api.sh` to confirm cross‑platform consistency.

---

This specification provides complete HTTP API definitions with request/response schemas, Go implementation examples, and data models for Ralph Loop implementation.
