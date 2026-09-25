# Quickstart Guide

Get up and running with OpenAPI CLI in 5 minutes.

## What You'll Learn

This quickstart will show you how to:
1. Verify your installation
2. Explore a remote OpenAPI specification
3. Generate CLI commands from a spec
4. Make your first API request
5. Run as a daemon or MCP server

> **Not built yet?** See the [installation guide's prerequisites](installation.md#prerequisites)
> — a Go toolchain and a C compiler (CGO, for SQLite) are required before
> `make build-cli` produces `bin/openapi-cli`.

## Step 1: Verify Installation

```bash
openapi-cli --version
# Expected: openapi-cli v0.1.0-... (version info)

openapi-cli version
# Prints detailed version information
```

## Step 2: Explore an OpenAPI Specification

Download our example PetStore spec or use your own:

```bash
# Save example spec
curl -L https://raw.githubusercontent.com/swagger-api/swagger-petstore/master/openapi.json -o petstore.json

# Discover API operations from a remote spec
openapi-cli discover https://petstore3.swagger.io/api/v3/openapi.json
# Output: all operations grouped by tag (method, path, operationId)
```

**What's happening**: OpenAPI CLI fetches the spec from the URL, parses it, and
displays every endpoint with its HTTP method, path, and operationId. No commands
are persisted — this is a read-only exploration.

## Step 3: Generate CLI Commands

OpenAPI CLI can generate CLI commands from your API specification:

```bash
# Generate commands from a local spec file
openapi-cli generate petstore.json

# Or from a remote URL
openapi-cli generate https://petstore3.swagger.io/api/v3/openapi.json

# With a custom base URL
openapi-cli generate openapi.yaml --base-url https://api.example.com/v1
```

After generation, the spec's operations become available as CLI commands
(persisted to storage and loaded on subsequent invocations).

**What's happening**: The generator parses the OpenAPI spec and creates Cobra
commands for each API operation. These are persisted so they survive restarts.

**Managing the store**: Use `openapi-cli generate --list` to inspect the current
store (spec path, hash, generated at, command count) and `openapi-cli generate --clear`
to remove it. If you generate from a different spec, the CLI warns before
overwriting the existing store.

## Step 4: Make Your First API Request

```bash
# Make a direct HTTP request
openapi-cli request GET https://petstore.swagger.io/v2/pet/1

# With query parameters
openapi-cli request GET https://petstore.swagger.io/v2/pet/findByStatus --param status=available

# POST with a JSON body
openapi-cli request POST https://petstore.swagger.io/v2/pet --body '{"name":"Fluffy","status":"available"}'

# JSON output
openapi-cli request GET https://petstore.swagger.io/v2/pet/1 --output json

# Add headers
openapi-cli request GET https://api.example.com/pet/1 -H "Authorization: Bearer token123"
```

**What's happening**: The `request` command is a general-purpose HTTP client
with support for headers, query parameters, body data, and multiple output
formats (table, json, yaml).

## Step 5: Run as a Daemon

Start the daemon for background API proxying and persistent connections:

```bash
# Start daemon with a Unix socket
openapi-cli daemon start --socket /tmp/openapi-cli.sock

# Make requests through the socket
curl --unix-socket /tmp/openapi-cli.sock http://localhost/pet/1

# Check daemon status
openapi-cli daemon status

# Stop daemon
openapi-cli daemon stop
```

## Step 6: Run as MCP Server

The MCP server is a separate binary (`openapi-mcp`), built alongside the CLI:

```bash
# Build all binaries
make build

# Run the MCP server
./bin/openapi-mcp --help
```

**Integration Example** (Claude Desktop configuration):

```json
{
  "mcpServers": {
    "openapi-cli": {
      "command": "/path/to/bin/openapi-mcp",
      "args": ["--spec", "/path/to/petstore.json"]
    }
  }
}
```

## Common Workflows

### Use Custom Configurations

```bash
openapi-cli config show
openapi-cli config profile list
openapi-cli config profile switch production
```

### Manage Authentication

```bash
# Store an API key in the OS keychain
openapi-cli auth add --name myapi --type api-key --value sk-abc123

# List stored credentials
openapi-cli auth list

# Remove a credential
openapi-cli auth remove myapi
```

### Build Automation with Starlark

```bash
# Create a script file: api-test.star
# Uses builtins: http_get, http_post, http_put, http_delete

# Run script
openapi-cli dsl run api-test.star
```

### Manage Spec Inventories

```bash
# Add an inventory mapping a host to a local file
openapi-cli inventory add --host https://api.example.com --file ./inventory.json

# List configured inventories
openapi-cli inventory list

# Remove an inventory
openapi-cli inventory remove --host https://api.example.com
```

## Next Steps

- [Configuration Guide](configuration.md) - Customize your setup
- [Command Reference](commands.md) - Explore all commands
- [API Documentation](../api/README.md) - Complete API reference
- [Starlark DSL Guide](../guides/starlark-dsl.md) - Advanced scripting

## Common Questions

**Q: Can I use multiple specs at once?**

A: Yes! Generate commands from each spec, or use `request` directly:

```bash
openapi-cli generate ./specs/petstore.json
openapi-cli generate ./specs/users.json
openapi-cli request GET https://petstore.swagger.io/v2/pet/1
openapi-cli request GET https://users.example.com/api/user/testuser
```

**Q: How do I handle authentication?**

A: Store credentials in the OS keychain:

```bash
# API Key
openapi-cli auth add --name myapi --type api-key --value my-secret-key

# Bearer token
openapi-cli auth add --name myapi --type bearer --value "token123"

# mTLS
openapi-cli auth mtls generate
openapi-cli auth mtls configure
```

**Q: Can I save common commands as aliases?**

A: Use your shell's alias mechanism:

```bash
alias get-pet='openapi-cli request GET https://petstore.swagger.io/v2/pet/1'
alias all-pets='openapi-cli request GET https://petstore.swagger.io/v2/pet/findByStatus --param status=available'
```

## Need Help?

- Run `openapi-cli --help` or `openapi-cli <command> --help`
- Check the [Guides Index](../README.md) for more tutorials
- Visit [GitHub Discussions](https://github.com/wojons/muster/discussions)
