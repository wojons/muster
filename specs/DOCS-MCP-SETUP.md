# MCP Setup Guide for OpenAPI CLI

Configure OpenAPI CLI as an MCP (Model Context Protocol) server for Claude Desktop, enabling AI assistants to interact with any OpenAPI-specified API.

## What is MCP?

Model Context Protocol (MCP) allows Claude Desktop to interact with external tools and data sources. OpenAPI CLI provides an MCP server that generates tools dynamically from any OpenAPI specification file.

## Claude Desktop Configuration

### Step 1: Locate Config File

Find your Claude Desktop configuration:

- **macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows**: `%APPDATA%\Claude\claude_desktop_config.json`
- **Linux**: `~/.config/Claude/claude_desktop_config.json`

### Step 2: Add OpenAPI CLI MCP Server

Edit the configuration file and add the OpenAPI CLI MCP server:

```json
{
  "mcpServers": {
    "openapi-cli": {
      "command": "muster-mcp",
      "args": ["--spec", "/path/to/your/openapi-spec.yaml", "--base-url", "https://api.example.com"]
    }
  }
}
```

**Important Fields**:
- **command**: The OpenAPI CLI MCP server binary (built as `muster-mcp`)
- **--spec**: Path to your OpenAPI 3.0 specification file
- **--base-url**: Base URL for API operations (overrides spec's server URL)

### Step 3: Restart Claude Desktop

Quit and reopen Claude Desktop to load the new MCP server.

### Step 4: Verify Connection

In Claude Desktop, you should see OpenAPI CLI tools available in your OpenAPI spec. Try asking:

> "What tools are available for working with the Petstore API?"

## Configuration Options

### Example: Petstore API

```json
{
  "mcpServers": {
    "petstore": {
      "command": "muster-mcp",
      "args": [
        "--spec", "/Users/yourname/petstore-openapi.yaml",
        "--base-url", "https://petstore.swagger.io/v2"
      ]
    }
  }
}
```

### Example: GitHub API with Custom Spec

```json
{
  "mcpServers": {
    "github": {
      "command": "muster-mcp",
      "args": [
        "--spec", "/Users/yourname/github-openapi.yaml",
        "--base-url", "https://api.github.com",
        "--log-level", "debug"
      ]
    }
  }
}
```

### Multiple API Servers

You can run multiple MCP servers for different APIs:

```json
{
  "mcpServers": {
    "petstore-api": {
      "command": "muster-mcp",
      "args": ["--spec", "/path/to/petstore.yaml"]
    },
    "banking-api": {
      "command": "muster-mcp",
      "args": ["--spec", "/path/to/banking.yaml", "--base-url", "https://api.bank.com"]
    }
  }
}
```

## Command-Line Options

The OpenAPI CLI MCP server supports these flags:

- `--spec <path>` (required) - Path to OpenAPI 3.0 specification file
- `--base-url <url>` - Base URL for API operations (overrides spec's server URLs)
- `--log-level <level>` - Log level: `debug`, `info`, `warn`, `error` (default: `info`)
- `--log-output <output>` - Log output destination: `stdout`, `stderr`, `none` (default: `stderr`)

## Available MCP Tools

The OpenAPI CLI MCP server automatically generates tools from your OpenAPI specification:

- Tools are named: `{operationId}` from your spec (e.g., `listPets`, `createPet`, `updatePet`)
- Tool parameters: Dynamically defined from spec's operation parameters
- Tool responses: Formatted JSON responses from API calls

**Example** (Petstore OpenAPI spec):
- `listPets` - List all pets (GET /pets)
- `createPet` - Create a new pet (POST /pets)
- `getPetById` - Get pet by ID (GET /pets/{id})
- `updatePet` - Update a pet (PUT /pets/{id})
- `deletePet` - Delete a pet (DELETE /pets/{id})

## OpenAPI Specification Requirements

Your OpenAPI spec must:

1. **Use OpenAPI 3.0** (JSON or YAML format)
2. **Define operationId** for each operation (used as tool name)
3. **Specify request schemas** for body parameters
4. **Include response schemas** for proper response formatting

**Minimal Example** (`petstore.yaml`):

```yaml
openapi: 3.0.0
info:
  title: Petstore API
  version: 1.0.0
servers:
  - url: https://petstore.swagger.io/v2
paths:
  /pets:
    get:
      operationId: listPets
      summary: List all pets
      responses:
        '200':
          description: A list of pets
          content:
            application/json:
              schema:
                type: array
                items:
                  type: object
                  properties:
                    id:
                      type: integer
                    name:
                      type: string
    post:
      operationId: createPet
      summary: Create a new pet
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
                tag:
                  type: string
      responses:
        '201':
          description: Pet created
```

## Troubleshooting

### MCP Server Not Connecting

**Symptom**: Claude doesn't show OpenAPI CLI tools

**Solutions**:
1. Verify MCP server binary exists: `which muster-mcp`
   - If not found, build from source: `make build-mcp`
2. Check config file syntax with `jq` or JSON validator
3. Check Claude Desktop logs for errors
   - macOS: `~/Library/Logs/Claude/`
   - Windows: `%APPDATA%\Claude\Logs\`
   - Linux: `~/.local/share/claude/logs/`

### Tools Not Showing

**Symptom**: MCP server connects but no tools appear

**Solutions**:
1. Verify OpenAPI spec file exists and is valid
2. Check spec has `operationId` defined for all operations
3. Verify MCP server logs: Run `muster-mcp --spec ./spec.yaml` in terminal to test
4. Check spec parses correctly: Try running `openapi-cli spec validate ./spec.yaml`

### Authentication Errors

**Symptom**: API calls return 401/403 errors

**Solutions**:
1. Verify API key is configured if required
2. Check base-url is correct
3. Enable debug logging: `--log-level debug`
4. Check API documentation for authentication requirements

### Permission Denied

**Symptom**: Cannot execute muster-mcp binary

**Solutions**:
1. Verify binary has execute permissions: `chmod +x muster-mcp`
2. Ensure binary is in a directory accessible to Claude Desktop
3. Try using full path to binary in Claude config

### Spec Parsing Errors

**Symptom**: MCP server exits with spec loading error

**Solutions**:
1. Validate spec: `openapi-cli spec validate ./spec.yaml`
2. Check spec is valid OpenAPI 3.0 format
3. Verify all required fields are present (paths, operationId, schemas)
4. Test spec independently: `openapi-cli spec show ./spec.yaml`

## Building from Source

If you don't have the binary, build from source:

```bash
# Clone repository
git clone https://github.com/openapi-cli/openapi-cli.git
cd openapi-cli

# Build MCP server binary
make build-mcp

# Binary will be at: ./bin/muster-mcp
```

## Advanced Configuration

### Environment Variables (Optional)

The MCP server supports optional environment variables:

```bash
export OPENAPI_LOG_LEVEL=debug      # Set default log level
export OPENAPI_TIMEOUT=30s          # Request timeout
export OPENAPI_MAX_REDIRECTS=5      # Max HTTP redirects
```

### Profile Support

Configuration profiles are not currently supported for MCP server mode. Use Claude Desktop's multiple server configuration instead.

### Debug Mode

Enable debug logging:

```json
{
  "mcpServers": {
    "openapi-cli": {
      "command": "muster-mcp",
      "args": [
        "--spec", "/path/to/spec.yaml",
        "--base-url", "https://api.example.com",
        "--log-level", "debug"
      ]
    }
  }
}
```

## Examples

See the `demo/openapi-specs/` directory for example OpenAPI specifications:

- `petstore.yaml` - Pet API example
- `banking.yaml` - Banking API with auth
- `streaming.yaml` - Real-time API example

## Additional Resources

- **MCP Protocol**: [Model Context Protocol Specification](https://modelcontextprotocol.io)
- **OpenAPI Specification**: [OpenAPI 3.0 Documentation](https://swagger.io/specification/)
- **OpenAPI CLI Documentation**: [GitHub](https://github.com/openapi-cli/openapi-cli)
