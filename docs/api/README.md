# API Documentation

Complete API reference for OpenAPI CLI APIs and protocols.

## Quick Links

- [REST API](rest-api.md) - OpenAPI CLI REST interfaces
- [MCP Protocol](mcp-protocol.md) - Model Context Protocol
- [Reference Card](reference-card.md) - Quick reference

## Overview

OpenAPI CLI provides several APIs for integration:

- **REST API** - For HTTP-based integration
- **MCP** - For AI assistant integration
- **CLI Commands** - For shell scripts and automation
- **WASM Modules** - For custom logic execution
- **Starlark DSL** - For scripting and automation

## REST API

The OpenAPI CLI REST API provides programmatic access to all functionality:

### Base URL

Default: `http://localhost:8080`

Configurable via:

```yaml
mcp:
  enabled: true
  port: 8080
  host: 0.0.0.0
```

### Authentication

Supports:
- API Key
- OAuth2
- mTLS

### Endpoints

#### `GET /`

API information

**Response**:
```json
{
  "name": "openapi-cli",
  "version": "1.0.0",
  "endpoints": ["/spec", "/commands", "/api"]
}
```

#### `POST /spec/validate`

Validate an OpenAPI specification

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

#### `GET /commands`

List available commands

**Response**:
```json
{
  "commands": [
    {
      "name": "init",
      "description": "Initialize CLI from spec",
      "usage": "openapi-cli init <spec-path>"
    }
  ]
}
```

#### `POST /api/request`

Make an API request

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
  "headers": {},
  "body": {
    "id": 1,
    "name": "Fluffy"
  }
}
```

## Model Context Protocol (MCP)

OpenAPI CLI implements the Model Context Protocol for integration with AI assistants like Claude.

### Capabilities

```yaml
capabilities:
  roots:
    listChanged: true
  sampling: {}
  tools:
    listChanged: true
```

### Tools

All API endpoints are exposed as MCP tools with auto-generated descriptions.

Example tool definitions are generated from your OpenAPI spec:

```json
{
  "name": "pet_findId",
  "description": "Find pet by ID",
  "inputSchema": {
    "type": "object",
    "properties": {
      "petId": {
        "type": "integer",
        "description": "ID of pet to return"
      }
    },
    "required": ["petId"]
  }
}
```

### Resource Support

Resources represent OpenAPI endpoints with metadata for structured data access:

```json
{
  "uri": "mcp://openapi-cli/openapi.yaml#/paths//pet/~/get",
  "name": "Pet API",
  "description": "Pet management endpoints",
  "mimeType": "application/json"
}
```

### Running MCP Server

```bash
openapi-cli mcp-server --spec openapi.yaml
```

For integration with Claude Desktop:

```json
{
  "mcpServers": {
    "openapi-cli": {
      "command": "openapi-cli",
      "args": ["mcp-server", "--spec", "/path/to/openapi.json"]
    }
  }
}
```

## Command-Line Interface

### Interactive Mode

```bash
openapi-cli interactive

# Interactive shell
> pet find-id --petId 1
> store order --total 100
> exit
```

### Command Output Formats

**JSON**:
```bash
openapi-cli api request GET /pet/1 --output json
```

**Table**:
```bash
openapi-cli inventory list --format table
```

**YAML**:
```bash
openapi-cli config show --format yaml
```

### Error Handling

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Generic error |
| 2 | Invalid arguments |
| 3 | Not found |
| 4 | API error |
| 5 | Auth error |
| 6 | Validation error |
| 7 | Timeout |
| 8 | Out of memory |
| 127 | Interrupted |

## WASM Integration

### Running WASM Modules

```bash
openapi-cli wasm run module.wasm --function add --arg 5 --arg 10
```

### WASM Capabilities

Configurable sandbox options:

```yaml
wasm:
  allowed_domains:
    - example.com
  sandbox:
    filesystem: false
    network: true
  max_memory: 128MB
  max_cpu: 1
```

## Starlark DSL

### Running Scripts

```bash
openapi-cli dsl run script.star --spec openapi.yaml
```

### Script Example

```python
#!/usr/bin/env openapi-cli dsl

# Make API request
pet = api.pet.find_id(pet_id=1)
print(f"Found pet: {pet['name']}")

# Update status
pet['status'] = 'sold'
api.pet.update(id=1, body=pet)
```

## Authentication APIs

### OAuth2 Flows

- **Authorization Code**: Web applications
- **PKCE**: Mobile and single-page apps
- **Client Credentials**: Service-to-service
- **Device Code**: Headless devices

### Token Management

```bash
# Login
openapi-cli auth login --provider google --client-id YOUR_CLIENT_ID

# Check status
openapi-cli auth status

# Logout
openapi-cli auth logout
```

### Credential Storage

Credentials are stored securely:
- macOS: Keychain
- Linux: Secret Service (libsecret)
- Windows: Credential Manager
- Fallback: Encrypted file

```bash
openapi-cli credentials store myapi --header X-API-Key --value secret
openapi-cli credentials list
```

## Configuration API

### Get Configuration

```bash
openapi-cli config show
openapi-cli config show --resolved
```

### Set Values

```bash
openapi-cli config set http.timeout 60s
openapi-cli config set spec.base_url https://api.example.com/v1
```

### Environment Variables

All config values can be overridden:

```bash
export MUSTER_SPEC_PATH=/path/to/openapi.yaml
export MUSTER_HTTP_TIMEOUT=60s
export MUSTER_LOG_LEVEL=debug
```

## Error Responses

### Format

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

| Code | Description |
|------|-------------|
| INVALID_SPEC | Invalid OpenAPI specification |
| AUTH_FAILED | Authentication failed |
| TIMEOUT | Request timeout |
| NOT_FOUND | Resource not found |
| VALIDATION_ERROR | Input validation failed |
| SERVER_ERROR | Internal server error |

## Rate Limiting

Configure rate limits for API protection:

```yaml
rate_limit:
  enabled: true
  requests_per_second: 10
  burst: 100
  algorithm: token_bucket
```

## Caching

### Cache Configuration

```yaml
cache:
  enabled: true
  ttl: 3600s
  location: ~/.cache/openapi-cli
  max_size: 1GB
```

### Cache Management

```bash
openapi-cli cache clear
openapi-cli cache stats
```

## Logging

### Log Configuration

```yaml
log:
  level: info
  format: json
  output: stderr
```

### Structured Logging

```json
{
  "level": "info",
  "timestamp": "2026-03-02T10:00:00Z",
  "message": "Making API request",
  "method": "GET",
  "url": "https://api.example.com/pet/1"
}
```

## Daemon API

### Unix Socket Interface

```bash
# Start daemon
openapi-cli daemon start --socket /tmp/openapi-cli.sock

# Make request via Unix socket
curl --unix-socket /tmp/openapi-cli.sock http://localhost/pet/find-id?petId=1
```

### HTTP Interface

```bash
# Start with HTTP server
openapi-cli daemon start --type http --port 8080

# Make request
curl http://localhost:8080/pet/find-id?petId=1
```

## Versioning

API versioning follows semantic versioning (`MAJOR.MINOR.PATCH`):

- **MAJOR**: Breaking changes
- **MINOR**: New features (backwards compatible)
- **PATCH**: Bug fixes

Check version:

```bash
openapi-cli --version
# openapi-cli 1.0.0
```

## Related Documentation

- [REST API Reference](rest-api.md) - Detailed endpoint documentation
- [MCP Protocol](mcp-protocol.md) - Model Context Protocol details
- [Command Reference](../guides/commands.md) - Complete CLI command list
- [Architecture Overview](../architecture/overview.md) - System design

## Support

- **GitHub Issues**: [Report bugs](https://github.com/openapi-cli/openapi-cli/issues)
- **Discussions**: [Ask questions](https://github.com/openapi-cli/openapi-cli/discussions)
- **Documentation**: [Full docs](https://openapi-cli.dev/docs/)
