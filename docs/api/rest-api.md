# REST API Reference

Complete REST API reference for OpenAPI CLI.

## Overview

OpenAPI CLI provides a REST API for programmatic access to all CLI functionality through HTTP endpoints. The REST API is useful for:
- Building custom integrations
- Automating workflows via webhooks
- Creating custom UIs
- Service-to-service communication

## Base URL

Default: `http://localhost:8080`

Configurable via configuration:

```yaml
mcp:
  enabled: true
  port: 8080
  host: 0.0.0.0
```

## Authentication

The REST API supports multiple authentication methods.

### API Key Authentication

```bash
curl -H "X-API-Key: your-secret-key" http://localhost:8080/spec/validate
```

Configuration:

```yaml
auth:
  type: apikey
  apikey:
    header: X-API-Key
    value: ${API_KEY}
```

### OAuth2 Bearer Token

```bash
curl -H "Authorization: Bearer <token>" http://localhost:8080/spec/validate
```

Configuration:

```yaml
auth:
  type: oauth2
  oauth2:
    token_url: https://auth.example.com/oauth/token
    scopes:
      - read
      - write
```

## API Endpoints

### Root Endpoint

Get API information.

```http
GET /
```

**Response**:
```json
{
  "name": "openapi-cli",
  "version": "1.0.0",
  "endpoints": ["/spec", "/commands", "/api"],
  "documentation": "/docs"
}
```

## Specification Endpoints

### Validate OpenAPI Spec

Validate an OpenAPI specification.

```http
POST /spec/validate
```

**Request Body**:
```json
{
  "spec": "<OpenAPI spec content>",
  "strict": false
}
```

**Response**:
```json
{
  "valid": true,
  "errors": []
}
```

**Example**:
```bash
curl -X POST http://localhost:8080/spec/validate \
  -H "Content-Type: application/json" \
  -d '{"spec":"openapi: 3.0.3\ninfo:\n  title: Test\npaths: {}"}'
```

### Get Spec Info

Get information about an OpenAPI specification.

```http
GET /spec/info?spec=<spec-path>
```

**Response**:
```json
{
  "title": "PetStore API",
  "version": "1.0.0",
  "description": "Sample PetStore API",
  "endpoints": 20,
  "paths": {
    "/pet": ["GET", "POST"],
    "/pet/{petId}": ["GET", "PUT", "DELETE"]
  }
}
```

### Convert Spec Format

Convert between OpenAPI formats.

```http
POST /spec/convert
```

**Request Body**:
```json
{
  "spec": "<spec content>",
  "fromFormat": "json",
  "toFormat": "yaml"
}
```

**Response**:
```json
{
  "spec": "<converted spec>",
  "format": "yaml"
}
```

## CLI Command Endpoints

### List Commands

List all available CLI commands.

```http
GET /commands
```

**Response**:
```json
{
  "commands": [
    {
      "name": "init",
      "description": "Initialize CLI from spec",
      "usage": "openapi-cli init <spec-path>",
      "aliases": ["init"]
    },
    {
      "name": "inventory list",
      "description": "List API resources",
      "usage": "openapi-cli inventory list <spec-path>"
    }
  ]
}
```

### Execute Command

Execute a CLI command via REST API.

```http
POST /commands/execute
```

**Request Body**:
```json
{
  "command": "init",
  "args": ["petstore.json"],
  "options": {
    "output": "./config",
    "verbose": true
  }
}
```

**Response**:
```json
{
  "success": true,
  "exitCode": 0,
  "stdout": "Configuration initialized",
  "stderr": ""
}
```

**Example**:
```bash
curl -X POST http://localhost:8080/commands/execute \
  -H "Content-Type: application/json" \
  -d '{
    "command": "inventory list",
    "args": ["petstore.json"],
    "options": {"format": "json"}
  }'
```

## API Request Endpoints

### Make API Request

Make a request to the target API.

```http
POST /api/request
```

**Request Body**:
```json
{
  "method": "GET",
  "path": "/pet/1",
  "baseUrl": "https://api.example.com/v1",
  "headers": {
    "Authorization": "Bearer token"
  },
  "query": {
    "fields": "id,name"
  }
}
```

**Response**:
```json
{
  "status": 200,
  "headers": {
    "Content-Type": "application/json"
  },
  "body": {
    "id": 1,
    "name": "Fluffy"
  }
}
```

### Batch Requests

Make multiple API requests in parallel.

```http
POST /api/batch
```

**Request Body**:
```json
{
  "requests": [
    {
      "method": "GET",
      "path": "/pet/1"
    },
    {
      "method": "GET",
      "path": "/pet/2"
    }
  ]
}
```

**Response**:
```json
{
  "results": [
    {
      "status": 200,
      "body": {"id": 1, "name": "Fluffy"}
    },
    {
      "status": 200,
      "body": {"id": 2, "name": "Rex"}
    }
  ]
}
```

## Documentation Endpoints

### Generate Documentation

Generate API documentation.

```http
POST /docs/generate
```

**Request Body**:
```json
{
  "spec": "<OpenAPI spec>",
  "format": "markdown",
  "output": "/path/to/docs"
}
```

**Response**:
```json
{
  "success": true,
  "files": [
    "/path/to/docs/index.md",
    "/path/to/docs/api.md"
  ]
}
```

### Serve Documentation

Serve documentation files.

```http
GET /docs/{path}
```

**Example**:
```bash
curl http://localhost:8080/docs/index.md
```

## Configuration Endpoints

### Get Configuration

Get current configuration.

```http
GET /config
```

**Response**:
```json
{
  "spec": {
    "path": "openapi.json",
    "baseUrl": "https://api.example.com/v1"
  },
  "http": {
    "timeout": "30s",
    "verifySsl": true
  },
  "output": {
    "format": "table"
  }
}
```

### Update Configuration

Update configuration values.

```http
PUT /config
```

**Request Body**:
```json
{
  "http": {
    "timeout": "60s"
  },
  "output": {
    "format": "json"
  }
}
```

**Response**:
```json
{
  "success": true
}
```

### Validate Configuration

Validate current configuration.

```http
POST /config/validate
```

**Response**:
```json
{
  "valid": true,
  "errors": []
}
```

## Cache Endpoints

### Get Cache Stats

Get cache statistics.

```http
GET /cache/stats
```

**Response**:
```json
{
  "hits": 150,
  "misses": 30,
  "size": "100MB",
  "items": 500
}
```

### Clear Cache

Clear the cache.

```http
DELETE /cache
```

**Response**:
```json
{
  "success": true,
  "cleared": 500
}
```

## Authentication Endpoints

### Get Auth Status

Check authentication status.

```http
GET /auth/status
```

**Response**:
```json
{
  "authenticated": true,
  "provider": "oauth2",
  "expires": "2026-03-03T10:00:00Z"
}
```

### Login

Authenticate with OAuth2.

```http
POST /auth/login
```

**Request Body**:
```json
{
  "provider": "google",
  "clientId": "your-client-id",
  "scopes": ["read", "write"]
}
```

**Response**:
```json
{
  "success": true,
  "token": "access_token"
}
```

### Logout

Logout and clear credentials.

```http
POST /auth/logout
```

**Response**:
```json
{
  "success": true
}
```

## Health Endpoint

Check service health.

```http
GET /health
```

**Response**:
```json
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "3600s"
}
```

## Error Responses

All endpoints return errors in this format:

```json
{
  "error": {
    "code": "INVALID_SPEC",
    "message": "OpenAPI specification is invalid",
    "details": {
      "line": 10,
      "column": 5,
      "error": "Missing required field"
    }
  }
}
```

### Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| INVALID_SPEC | 400 | Invalid OpenAPI specification |
| AUTH_FAILED | 401 | Authentication failed |
| NOT_FOUND | 404 | Resource not found |
| VALIDATION_ERROR | 422 | Request validation failed |
| TIMEOUT | 408 | Request timeout |
| SERVER_ERROR | 500 | Internal server error |

## Rate Limiting

The REST API includes rate limiting to prevent abuse:

**Default Configuration**:
```yaml
rate_limit:
  enabled: true
  requests_per_second: 10
  burst: 100
  algorithm: token_bucket
```

### Rate Limit Headers

```http
HTTP/1.1 429 Too Many Requests
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1648867200
```

### Rate Limit Response

```json
{
  "error": {
    "code": "RATE_LIMIT_EXCEEDED",
    "message": "Rate limit exceeded. Please retry later."
  }
}
```

## CORS Support

Configure CORS for cross-origin requests:

```yaml
mcp:
  cors:
    enabled: true
    allowed_origins:
      - "http://localhost:3000"
      - "https://yourdomain.com"
    allowed_methods:
      - GET
      - POST
      - PUT
      - DELETE
    allowed_headers:
      - Content-Type
      - Authorization
    max_age: 3600
```

## Webhooks

Configure webhooks for event notifications.

### Register Webhook

```http
POST /webhooks
```

**Request Body**:
```json
{
  "url": "https://yourdomain.com/webhook",
  "events": [
    "api.request",
    "api.error"
  ],
  "secret": "webhook-secret"
}
```

**Response**:
```json
{
  "webhookId": "wh_12345",
  "status": "active"
}
```

### Webhook Events

| Event | Description |
|-------|-------------|
| `api.request` | API request made |
| `api.response` | API response received |
| `api.error` | API error occurred |
| `spec.loaded` | Specification loaded |
| `command.executed` | CLI command executed |

### Webhook Payload

```json
{
  "event": "api.request",
  "timestamp": "2026-03-02T10:00:00Z",
  "data": {
    "method": "GET",
    "path": "/pet/1",
    "headers": {}
  }
}
```

## WebSocket Support

Real-time event streaming via WebSocket.

### Connect to WebSocket

```javascript
const ws = new WebSocket('ws://localhost:8080/events');

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event:', data.event, data);
};

// Subscribe to events
ws.send(JSON.stringify({
  action: 'subscribe',
  events: ['api.request', 'api.response']
}));
```

## Examples

### Example 1: Validate Spec

```bash
# Validate OpenAPI spec
curl -X POST http://localhost:8080/spec/validate \
  -H "Content-Type: application/json" \
  -d @spec.json
```

### Example 2: Get Inventory

```bash
# List API resources
curl -X POST http://localhost:8080/commands/execute \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret" \
  -d '{
    "command": "inventory list",
    "args": ["petstore.json"],
    "options": {"format": "json"}
  }'
```

### Example 3: Make API Request

```bash
# Make a single API request
curl -X POST http://localhost:8080/api/request \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret" \
  -d '{
    "method": "GET",
    "path": "/pet/1",
    "baseUrl": "https://petstore.swagger.io/v2"
  }'
```

### Example 4: Batch Requests

```bash
# Make multiple API requests
curl -X POST http://localhost:8080/api/batch \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret" \
  -d '{
    "requests": [
      {"method": "GET", "path": "/pet/1"},
      {"method": "GET", "path": "/pet/2"},
      {"method": "GET", "path": "/pet/3"}
    ]
  }'
```

### Example 5: Get Configuration

```bash
# Get current configuration
curl http://localhost:8080/config \
  -H "X-API-Key: your-secret"
```

### Example 6: Update Configuration

```bash
# Update timeout setting
curl -X PUT http://localhost:8080/config \
  -H "Content-Type: application/json" \
  -H "X-API-Key: your-secret" \
  -d '{"http": {"timeout": "60s"}}'
```

### Example 7: Health Check

```bash
# Check service health
curl http://localhost:8080/health
```

### Example 8: Clear Cache

```bash
# Clear cache
curl -X DELETE http://localhost:8080/cache \
  -H "X-API-Key: your-secret"
```

## Configuration

### Enable REST API

```yaml
mcp:
  enabled: true
  port: 8080
  host: 0.0.0.0

rest:
  enabled: true
  cors:
    enabled: true
  rate_limit:
    enabled: true
    requests_per_second: 10
```

### Authentication Settings

```yaml
auth:
  type: apikey
  apikey:
    header: X-API-Key
    value: ${API_KEY}
```

## Security

### Recommendations

1. **Use HTTPS** in production
2. **Enable rate limiting** to prevent abuse
3. **Use strong secrets** for API keys
4. **Validate all inputs** before processing
5. **Log all requests** for audit trails
6. **Enable CORS** only for trusted origins

### Best Practices

- Store secrets in environment variables
- Use short-lived tokens
- Rotate credentials regularly
- Audit webhook endpoints
- Monitor for suspicious activity

## Testing

### Manual Testing

```bash
# Health check
curl http://localhost:8080/health

# Validate spec
curl -X POST http://localhost:8080/spec/validate -d @spec.json

# Make request
curl -X POST http://localhost:8080/commands/execute \
  -d '{"command": "inventory list", "args": ["petstore.json"]}'
```

### Automated Testing with curl

```bash
#!/bin/bash

# Test suite
echo "Testing REST API..."

# Health check
curl -f http://localhost:8080/health || exit 1

# Validate spec
curl -f -X POST http://localhost:8080/spec/validate \
  -d @valid_spec.json || exit 1

# Make request
curl -f -X POST http://localhost:8080/commands/execute \
  -H "X-API-Key: $API_KEY" \
  -d '{"command": "version"}' || exit 1

echo "All tests passed!"
```

## Troubleshooting

### Connection Refused

**Issue**: `curl: (7) Failed to connect to localhost port 8080`

**Solution**: Ensure the REST API is enabled and running:

```bash
openapi-cli config set mcp.enabled true
openapi-cli config set mcp.port 8080
openapi-cli mcp-server
```

### Authentication Failed

**Issue**: `{"error":{"code":"AUTH_FAILED"}}`

**Solution**: Verify your API key:

```bash
# Set API key
export API_KEY=your-secret

# Make request
curl -H "X-API-Key: $API_KEY" http://localhost:8080/health
```

### Rate Limit Exceeded

**Issue**: `{"error":{"code":"RATE_LIMIT_EXCEEDED"}}`

**Solution**: Wait and retry, or adjust rate limits:

```yaml
rate_limit:
  requests_per_second: 20
  burst: 200
```

## Related Documentation

- [Configuration Guide](../guides/configuration.md) - API configuration
- [Authentication](../guides/configuration.md#authentication-configuration) - Auth setup
- [Command Reference](../guides/commands.md) - CLI commands
- [MCP Protocol](mcp-protocol.md) - Model Context Protocol
- [API Overview](README.md) - General API information
