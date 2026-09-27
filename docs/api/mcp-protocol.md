# MCP Protocol

Model Context Protocol implementation for OpenAPI CLI.

## Overview

OpenAPI CLI implements MCP (Model Context Protocol) to enable seamless integration with AI assistants like Claude, providing structured access to OpenAPI-specified APIs as AI tools and resources.

## Getting Started

### Running the MCP Server

`openapi-mcp` speaks MCP over **stdio**: JSON-RPC 2.0 requests are read from
stdin, responses are written to stdout, and diagnostics go to stderr.

```bash
# Start the MCP server over stdio — the only transport this binary implements
./bin/openapi-mcp --spec openapi.yaml
```

There is **no HTTP/TCP transport flag in this binary**. It registers no
`net/http` listener, and its flag set (see [Command-line
Flags](#command-line-flags)) contains neither a port nor a host option. Passing
one is a flag-parsing failure: the process exits with status 2 and prints
`flag provided but not defined` plus the usage block, before any spec is read.
Any HTTP-facing surface has to come from an MCP host or proxy sitting in front
of the stdio process — the binary itself only speaks stdio.

To drive the server by hand, pipe requests into stdin (see [Manual
Testing](#manual-testing)); to use it from an assistant, point the MCP client at
the binary (see [Claude Desktop Configuration](#claude-desktop-configuration)).

Go's `flag` package prints flags with a single dash, but both spellings are
accepted: `-spec` and `--spec` are equivalent.

### Claude Desktop Configuration

Configure in Claude Desktop settings:

```json
{
  "mcpServers": {
    "openapi-cli": {
      "command": "/path/to/bin/openapi-mcp",
      "args": ["--spec", "/path/to/openapi.yaml"]
    }
  }
}
```

`--base-url` (or the `MUSTER_BASE_URL` env var) is an optional override: by
default the server targets the spec's own `servers[].url`, so `--spec` alone
works whenever the spec declares an absolute server URL.

### Command-line Flags

`./bin/openapi-mcp --help` prints the complete flag set. The table below mirrors
it, with each flag's default and its actual effect taken from the usage string
in `cmd/mcp-server/main.go`:

| Flag | Default | Effect |
| --- | --- | --- |
| `--spec` | `""` (none) | OpenAPI 3.x document to expose as MCP tools — either a local file path or an `http(s)://` URL. With neither `--spec` nor `--openrpc-doc` the server starts and registers no tools. |
| `--base-url` | `""` (none) | Overrides the base URL the generated tools call. The flag wins over the `MUSTER_BASE_URL` env var, which in turn wins over the spec's `servers[].url`. |
| `--openrpc-doc` | `""` (none) | Path or `http(s)://` URL of an OpenRPC 1.x discovery document; the JSON-RPC methods it discovers are registered as MCP tools alongside the OpenAPI tools. Calls go to `--base-url` / `MUSTER_BASE_URL` when set, otherwise to the document's `servers[0].url`. |
| `--audit` | `false` | Enables audit logging, exactly like `MUSTER_AUDIT_ENABLED=1` (or `true`). Events are appended as JSON lines to `$MUSTER_AUDIT_LOG`, or to `<config dir>/muster/audit.jsonl` when that variable is unset (`$XDG_CONFIG_HOME`, or `$HOME/.config` on Unix). |
| `--wasm-plugin` | `""` (none) | Path to a WASM plugin for response transformation. The value is accepted and stored in the server config; no code in `cmd/mcp-server` reads it at this revision, so it does not change behaviour yet. |
| `--log-level` | `"info"` | Log level. Accepted and stored in the server config; not consumed by `cmd/mcp-server` at this revision (diagnostics go to stderr unconditionally). |
| `--log-output` | `"stderr"` | Log output destination. Accepted and stored in the server config; not consumed at this revision. |
| `--version` | `false` | Prints the build version and exits without starting the server, e.g. `v0.1.1-195-gb983fa6d (b983fa6d)`. |

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

`openapi-mcp` has **no authentication flags and reads no config file**. It is
configured only by the flags in [Command-line Flags](#command-line-flags), none
of which carries credential material. On the request side, the tool executor
(`pkg/mcp`) builds each outbound call from the operation's path and the
resolved base URL and sets no authentication header — credentials are therefore
an upstream concern.

Because the server speaks stdio and is started by the MCP client, access
control is a property of how the process is launched:

1. **Process access** — the client that launches the stdio process is the only
   caller; there is no network listener to expose or protect.
2. **Upstream calls** — put credential handling in the layer that calls the
   upstream API (for example a gateway in front of it). There is no server-side
   token flag to set.
3. **Secrets in the environment** — where an option has both a flag and an
   environment form (`--base-url`/`MUSTER_BASE_URL`,
   `--audit`/`MUSTER_AUDIT_ENABLED`), prefer the environment variable so secret
   material stays out of process listings and shell history.

See [Configuration](../guides/configuration.md) for the environment variables
the project as a whole understands.

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
# Full MCP integration test — drive the stdio server with piped requests
printf '%s\n' \
  '{"jsonrpc":"2.0","method":"initialize","id":1}' \
  '{"jsonrpc":"2.0","method":"tools/list","id":2}' \
  | ./bin/openapi-mcp --spec demo/openapi-specs/petstore.yaml
```

One JSON-RPC response line is written to stdout per request. Response ordering
is not guaranteed — handlers run concurrently, so responses can come back in a
different order than the requests went in:

```console
$ printf '%s\n' \
    '{"jsonrpc":"2.0","method":"initialize","id":1}' \
    '{"jsonrpc":"2.0","method":"tools/list","id":2}' \
    | ./bin/openapi-mcp --spec demo/openapi-specs/petstore.yaml 2>/dev/null
{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"openapi-mcp","version":"1.0.0"}}}
{"jsonrpc":"2.0","id":2,"result":{"tools":[{...5 tools...}]}}
```

Diagnostics (`Loaded spec: …`, `Generated N MCP tools`, `Using base URL: …`) go
to stderr, which is why the transcript above redirects it away with `2>/dev/null`
when only the protocol stream is wanted.

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

There is no debug-logging switch in this binary: diagnostics always go to
stderr, and `--log-level` is accepted but not consumed at this revision (see
[Command-line Flags](#command-line-flags)). To inspect a session, capture stderr
and leave stdout as the protocol stream:

```bash
./bin/openapi-mcp --spec openapi.yaml 2>mcp-server.log
# stdout: JSON-RPC responses only
# mcp-server.log: "Loaded spec: …", "Generated N MCP tools", "Using base URL: …"
```

## Related Documentation

- [API Documentation](README.md) - General API reference
- [OpenAPI Spec](https://swagger.io/specification/) - Official specification
- [MCP Standard](https://modelcontextprotocol.io/) - MCP protocol specification
- [Command Reference](../guides/commands.md) - CLI commands
