# MCP Protocol

Model Context Protocol implementation for OpenAPI CLI.

## Overview

OpenAPI CLI implements MCP (Model Context Protocol) to enable seamless integration with AI assistants like Claude, providing structured access to OpenAPI-specified APIs as AI tools and resources.

## Getting Started

### Running the MCP Server

```bash
# Start MCP server with stdio transport (default)
./bin/openapi-mcp --spec openapi.yaml

# Start with HTTP transport for debugging
./bin/openapi-mcp --spec openapi.yaml --port 8080
```

### Claude Desktop Configuration

Configure in Claude Desktop settings:

```json
{
  "mcpServers": {
    "openapi-cli": {
      "command": "/path/to/bin/openapi-mcp",
      "args": [
        "--spec", "/path/to/openapi.yaml",
        "--base-url", "https://api.example.com/v1"
      ]
    }
  }
}
```

### Manual Testing

Test MCP server manually:

```bash
{
  "jsonrpc": "2.0",
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "roots": {},
      "sampling": {},
      "tools": {}
    },
    "clientInfo": {
      "name": "test-client",
      "version": "1.0.0"
    }
  },
  "id": 1
} | ./bin/openapi-mcp --spec openapi.yaml
```

## Server Capabilities

The MCP server advertises these capabilities:

```json
{
  "protocolVersion": "2024-11-05",
  "capabilities": {
    "roots": {
      "listChanged": true
    },
    "sampling": {},
    "tools": {
      "listChanged": true
    }
  },
  "serverInfo": {
    "name": "openapi-cli",
    "version": "1.0.0"
  }
}
```

## Tools (Methods)

### Tool Generation from OpenAPI Spec

OpenAPI CLI automatically generates MCP tools from your OpenAPI specification. Each endpoint becomes a tool.

#### Tool Naming Convention

Tools are named using the pattern: `<tag>_<operationId>` or `<path>_<method>` if no operationId.

Examples:
- `pet_find` - GET /pet
- `pet_add` - POST /pet
- `store_inventory` - GET /store/inventory

#### Tool Schema

Each tool provides full JSON Schema for input validation:

```json
{
  "name": "pet_findById",
  "description": "Find pet by ID. Returns a single pet.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "petId": {
        "type": "integer",
        "description": "ID of pet to return",
        "minimum": 1,
        "required": true
      }
    },
    "additionalProperties": false,
    "required": ["petId"]
  }
}
```

### `tools/list`

List all available tools (API endpoints).

**Request**:
```json
{
  "jsonrpc": "2.0",
  "method": "tools/list",
  "id": 1
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      {
        "name": "pet_add",
        "description": "Add a new pet to the store",
        "inputSchema": {
          "type": "object",
          "properties": {
            "name": {"type": "string"},
            "status": {"type": "string", "enum": ["available", "pending", "sold"]}
          }
        }
      },
      {
        "name": "pet_findById",
        "description": "Find pet by ID",
        "inputSchema": {
          "type": "object",
          "properties": {
            "petId": {"type": "integer"}
          }
        }
      }
    ]
  }
}
```

### `tools/call`

Execute an API endpoint tool.

**Request**:
```json
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "id": 1,
  "params": {
    "name": "pet_findById",
    "arguments": {
      "petId": 1
    }
  }
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [
      {
        "type": "resource",
        "resource": {
          "uri": "mcp://openapi-cli/pet/1"
        }
      },
      {
        "type": "text",
        "text": "Found pet with ID 1"
      }
    ]
  }
}
```

**Error Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32603,
    "message": "Pet not found",
    "data": {
      "statusCode": 404,
      "details": {}
    }
  }
}
```

## Resources (Data Access)

### Resource URIs

Resources are identified using URIs following the pattern:

```
mcp://openapi-cli/<path>#<fragment>
```

Examples:
- `mcp://openapi-cli/paths//pet/~1{petId}/get` - GET /pet/{petId}
- `mcp://openapi-cli/components/schemas/Pet` - Pet schema

### `resources/list`

List available OpenAPI specification resources.

**Request**:
```json
{
  "jsonrpc": "2.0",
  "method": "resources/list",
  "id": 1
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "resources": [
      {
        "uri": "mcp://openapi-cli/openapi.yaml",
        "name": "OpenAPI Specification",
        "description": "Complete OpenAPI 3.0 specification",
        "mimeType": "application/x-yaml"
      },
      {
        "uri": "mcp://openapi-cli/paths//pet/~1{petId}/get",
        "name": "Pet: Find by ID",
        "description": "GET /pet/{petId} endpoint",
        "mimeType": "text/plain"
      }
    ]
  }
}
```

### `resources/read`

Read a resource (OpenAPI specification or metadata).

**Request**:
```json
{
  "jsonrpc": "2.0",
  "method": "resources/read",
  "params": {
    "uri": "mcp://openapi-cli/openapi.yaml"
  },
  "id": 1
}
```

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "contents": [
      {
        "uri": "mcp://openapi-cli/openapi.yaml",
        "mimeType": "application/x-yaml",
        "text": "openapi: 3.0.3\ninfo:\n  title: PetStore API\n..."
      }
    ]
  }
}
```

### `resources/list_changed`

Check for specification changes since last request.

**Response**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "resources": []  // Empty list means no changes
  }
}
```

## Authentication

### MCP Auth Flow

The MCP server supports authentication via configuration:

```bash
./bin/openapi-mcp --spec openapi.yaml \
  --auth.type oauth2 \
  --auth.token your-access-token
```

Config file approach:

```yaml
mcp:
  auth:
    type: oauth2
    token: ${OAUTH_TOKEN}
```

## Error Handling

### Standard Errors

| Code | Message |
|------|--------|
| -32700 | Parse error |
| -32600 | Invalid Request |
| -32601 | Method not found |
| -32602 | Invalid params |
| -32603 | Internal error |
| -32604 | Not found |

### Application-Specific Errors

**Tool Error Example**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32603,
    "message": "API request failed",
    "data": {
      "httpStatus": 500,
      "httpMessage": "Internal Server Error",
      "details": {
        "error": "Database connection failed"
      }
    }
  }
}
```

## Lifecycle Methods

### `initialize`

Initialize MCP connection:

```json
{
  "jsonrpc": "2.0",
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {
      "name": "claude",
      "version": "1.0.0"
    }
  },
  "id": 1
}
```

### `initialized`

Acknowledge initialization:

```json
{
  "jsonrpc": "2.0",
  "method": "initialized"
}
```

### `shutdown`

Graceful shutdown:

```json
{
  "jsonrpc": "2.0",
  "method": "shutdown",
  "id": 1
}
```

## Advanced Configuration

### Custom Tool Generation

Control tool naming and descriptions:

```yaml
mcp:
  tools:
    naming:
      style: operation_id  # operation_id or path_method
      prefix: ""          # Optional prefix
    descriptions:
      include_parameters: true
      include_response: true
```

### Resource Filtering

Control which resources are exposed:

```yaml
mcp:
  resources:
    include:
      - paths**
      - components/schemas**
    exclude:
      - paths/**/internal/**
```

### Rate Limiting

Prevent API abuse through MCP:

```yaml
mcp:
  rate_limit:
    enabled: true
    requests_per_minute: 60
    burst: 10
```

## Examples

### Example 1: Complete Workflow

```bash
# Start MCP server
./bin/openapi-mcp --spec openapi.yaml

# Client workflow:
# 1. Initialize
{
  "jsonrpc": "2.0",
  "method": "initialize",
  "params": {"protocolVersion": "2024-11-05"},
  "id": 1
}

# 2. List tools
{
  "jsonrpc": "2.0",
  "method": "tools/list",
  "id": 2
}

# 3. Call tool
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "pet_findById",
    "arguments": {"petId": 1}
  },
  "id": 3
}
```

### Example 2: Error Handling

```bash
# Invalid tool name
{
  "jsonrpc": "2.0",
  "method": "tools/call",
  "params": {
    "name": "invalid_tool",
    "arguments": {}
  },
  "id": 1
}

# Response:
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32601,
    "message": "Tool 'invalid_tool' not found"
  }
}
```

### Example 3: Resource Reading

```bash
# Read OpenAPI spec
{
  "jsonrpc": "2.0",
  "method": "resources/read",
  "params": {
    "uri": "mcp://openapi-cli/openapi.yaml"
  },
  "id": 1
}

# Response:
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "contents": [
      {
        "uri": "mcp://openapi-cli/openapi.yaml",
        "mimeType": "application/x-yaml",
        "text": "openapi: 3.0.3\n..."
      }
    ]
  }
}
```

## Security Considerations

1. **Authentication**: Always use proper authentication
2. **Environment Variables**: Store secrets in environment variables
3. **Rate Limiting**: Enable for production deployments
4. **Input Validation**: Schema validation is enforced automatically
5. **Sanitization**: Remove sensitive fields from responses

## Testing

### Unit Testing Tools

```python
# Test: tools/list
{
  "jsonrpc": "2.0",
  "method": "tools/list",
  "id": 1
}

# Verify response contains expected tools
```

### Integration Testing

```bash
# Full MCP integration test
./bin/openapi-mcp --spec test-openapi.yaml --port 8080

# Send requests via HTTP or stdio
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"tools/list","id":1}'
```

## Troubleshooting

### Common Issues

**Issue**: "Specification not found"

**Solution**: Check spec path and file permissions:

```bash
./bin/openapi-mcp --spec /full/path/to/openapi.yaml
```

**Issue**: "Tool 'xxx' not found"

**Solution**: Verify the tool name matches the operation or path:

```bash
openapi-cli discover https://petstore3.swagger.io/api/v3/openapi.json  # See available operations
```

### Debug Mode

Enable debug logging:

```bash
./bin/openapi-mcp --spec openapi.yaml
```

## Related Documentation

- [API Documentation](README.md) - General API reference
- [OpenAPI Spec](https://swagger.io/specification/) - Official specification
- [MCP Standard](https://modelcontextprotocol.io/) - MCP protocol specification
- [Command Reference](../guides/commands.md) - CLI commands
