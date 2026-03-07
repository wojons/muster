# CLI Reference

Complete reference for all OpenAPI CLI commands.

## Overview

OpenAPI CLI is a unified tool that generates commands from OpenAPI specifications and serves them via MCP protocol. Unlike traditional CLI tools with fixed commands, OpenAPI CLI dynamically generates command-line interfaces from API specifications.

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--config` | Config file path | `~/.config/openapi-cli/config.yaml` |
| `--profile` | Configuration profile | `default` |
| `--output, -o` | Output format (json, yaml, table) | `table` |
| `--verbose, -v` | Enable verbose logging | `false` |
| `--version` | Show version | - |

## Built-in Commands

### `openapi-cli auth`

Manage authentication for API connections.

#### `openapi-cli auth login [--oauth2] [--username <user>] [--password <pass>] [--token <token>]`

Authenticate with the API server.

Flags:
- `--oauth2` - Use OAuth2 authentication
- `--username` - Username for basic auth
- `--password` - Password for basic auth
- `--token` - Bearer token for authentication

Examples:
```bash
# Login with OAuth2
openapi-cli auth login --oauth2

# Login with username/password
openapi-cli auth login --username admin --password secret

# Login with bearer token
openapi-cli auth login --token eyJhbGciOiJIUzI1NiJ9...
```

#### `openapi-cli auth logout`

Clear stored authentication credentials.

#### `openapi-cli auth status`

Display current authentication status.

### `openapi-cli config`

Manage OpenAPI CLI configuration.

#### `openapi-cli config show`

Show current configuration settings including directories and version.

### `openapi-cli completion [bash|zsh|fish|powershell]`

Generate shell completion scripts for bash, zsh, fish, or PowerShell.

Example:
```bash
# For bash
openapi-cli completion bash > /etc/bash_completion.d/openapi-cli

# For zsh
openapi-cli completion zsh > "${fpath[1]}/_openapi-cli"

# For fish
openapi-cli completion fish > ~/.config/fish/completions/openapi-cli.fish
```

### `openapi-cli daemon`

Control the background daemon process.

#### `openapi-cli daemon start`

Start the daemon service in the background with Unix socket support.

#### `openapi-cli daemon stop`

Stop the running daemon service.

#### `openapi-cli daemon status`

Check if daemon is running and get status information.

### `openapi-cli generate <spec-path>`

Generate CLI commands from an OpenAPI specification file.

Arguments:
- `<spec-path>` - Path to OpenAPI specification file (openapi.yaml/openapi.json)

Example:
```bash
openapi-cli generate https://api.example.com/openapi.yaml
openapi-cli generate ./specs/petstore.yaml
```

This command:
1. Fetches and validates the OpenAPI specification
2. Generates CLI commands for each operation
3. Registers commands with the CLI
4. Loads commands for immediate use

### `openapi-cli help [command]`

Display help information about commands.

### `openapi-cli init`

Initialize OpenAPI CLI configuration and setup default directories.

### `openapi-cli inventory`

Manage specification inventory and caching.

#### `openapi-cli inventory fetch <url>`

Fetch OpenAPI specification from URL and add to inventory.

Arguments:
- `<url>` - URL to OpenAPI specification

#### `openapi-cli inventory list`

List all specifications in the inventory.

#### `openapi-cli inventory validate <spec-path>`

Validate an OpenAPI specification file.

Arguments:
- `<spec-path>` - Path to specification file

### `openapi-cli modules`

Manage WASM modules for extended functionality.

### `openapi-cli request <url> [method]`

Make HTTP requests to APIs directly from CLI.

Arguments:
- `<url>` - API endpoint URL
- `[method]` - HTTP method (GET, POST, PUT, DELETE, etc.)

### `openapi-cli version`

Display OpenAPI CLI version information.

## Generated Commands

After loading an OpenAPI specification, OpenAPI CLI dynamically generates commands for each API operation. Commands are organized by API resources and HTTP methods.

### Example Generated Commands

If you load a Pet Store API specification, you might get:

```bash
# List all pets
openapi-cli pets list

# Get a specific pet
openapi-cli pets get --id 42

# Create a pet
openapi-cli pets create --name "Fluffy" --type "cat"

# Update a pet
openapi-cli pets update --id 42 --name "Fluffy Jr"

# Delete a pet
openapi-cli pets delete --id 42
```

All generated commands support:
- Resource grouping (e.g., `pets`, `users`, `orders`)
- Global flags for configuration
- Automatic argument validation
- Help text from OpenAPI descriptions

### Accessing Generated Commands

Generated commands are loaded into the CLI and available immediately after running `openapi-cli generate` or `openapi-cli inventory fetch`.

To see all available commands (including generated ones):
```bash
openapi-cli --help
```

To see help for a generated command:
```bash
openapi-cli pets --help
openapi-cli pets get --help
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid command/usage |
| 3 | Authentication error |
| 4 | Resource not found |
| 5 | Permission denied |
| 6 | Network error |
| 7 | Validation error |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `OPENAPI_CONFIG` | Config file path |
| `OPENAPI_PROFILE` | Default profile |
| `OPENAPI_DEBUG` | Enable debug logging |

## Default Directory Structure

OpenAPI CLI uses standard XDG Base Directory paths:

| Directory | Location (Linux) | Location (macOS) | Location (Windows) |
|-----------|------------------|------------------|---------------------|
| Config | `~/.config/openapi-cli/` | `~/Library/Application Support/openapi-cli/` | `%APPDATA%\openapi-cli\` |
| Cache | `~/.cache/openapi-cli/` | `~/Library/Caches/openapi-cli/` | `%LOCALAPPDATA%\openapi-cli\` |
| Data | `~/.local/share/openapi-cli/` | `~/Library/Application Support/openapi-cli/` | `%LOCALAPPDATA%\openapi-cli\` |

### Data Directory Contents

The data directory (`~/.local/share/openapi-cli/` on Linux) contains:
- `commands.json` - Persisted generated commands
- `inventory/` - Cached OpenAPI specifications
- `specs/` - Downloaded specification files
- `modules/` - WASM modules

## MCP Server Mode

In addition to CLI commands, OpenAPI CLI can run as an MCP server for AI assistants like Claude Desktop.

### Start MCP Server

```bash
openapi-cli mcp-server
```

The MCP server serves commands via JSON-RPC over stdio. Commands, resources, and prompts are automatically exposed through the MCP protocol.

## Examples

### Complete Workflow

```bash
# 1. Configure OpenAPI CLI
openapi-cli init

# 2. Authenticate with your API
openapi-cli auth login --oauth2

# 3. Fetch an OpenAPI specification
openapi-cli inventory fetch https://api.example.com/openapi.yaml

# 4. Generate commands
openapi-cli generate https://api.example.com/openapi.yaml

# 5. Use generated commands
openapi-cli users list
openapi-cli users get --id 42
openapi-cli orders create --user-id 42 --product-id 15

# 6. Or make direct requests
openapi-cli request https://api.example.com/users/42
```

### Direct Request Mode

```bash
# GET request
openapi-cli request https://api.example.com/pets

# POST request with JSON body
openapi-cli request https://api.example.com/pets POST '{"name":"Fluffy","type":"cat"}'

# PUT request
openapi-cli request https://api.example.com/pets/42 PUT '{"name":"Fluffy Jr"}'

# DELETE request
openapi-cli request https://api.example.com/pets/42 DELETE
```

### Daemon Mode

```bash
# Start daemon
openapi-cli daemon start

# Check status
openapi-cli daemon status

# Stop daemon
openapi-cli daemon stop
```

The daemon runs in the background with a Unix socket, providing:
- Socket-based API proxy
- Persistent connection pooling
- Request routing and caching
- Background task execution

## See Also

- [Quick Start Guide](DOCS-OPENAPI-QUICKSTART.md) - Getting started with OpenAPI CLI
- [Configuration](DOCS-CONFIGURATION.md) - Detailed configuration options
- [Authentication](API-AUTHENTICATION.md) - OAuth2, mTLS, and API key authentication
- [OpenAPI Specification](OPENAPI-PARSER.md) - OpenAPI parsing and command generation
- [MCP Protocol](MCP-PROTOCOL.md) - MCP server integration with Claude Desktop
