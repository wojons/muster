# Quick Start Guide

Get up and running with **openapi-cli** in 5 minutes.

**openapi-cli** is a unified CLI tool that:
- Dynamically generates CLI commands from OpenAPI specifications
- Serves those commands via MCP protocol to Claude Desktop
- Runs as a daemon with Unix socket proxy
- Supports authentication (OAuth2, API keys, mTLS)
- Executes WASM plugins for custom logic

## Installation

### macOS
```bash
brew install openapi-cli/openapi-cli/openapi-cli
```

### Linux
```bash
curl -fsSL https://openapi-cli.io/install.sh | bash
```

### Build from source
```bash
git clone https://github.com/openapi-cli/openapi-cli
cd openapi-cli
make build
sudo make install
```

## First Command

After installation, verify the CLI is working:

```bash
openapi-cli --version
```

## Quick Start: Generate Commands from OpenAPI Spec

### Step 1: Get an OpenAPI Specification

Download or create an OpenAPI 3.0/3.1 spec (YAML or JSON format).

**Example: Using Petstore API**
```bash
# Download sample spec
curl -o petstore.yaml https://petstore.swagger.io/v2/swagger.json
```

### Step 2: Generate CLI Commands

Generate CLI commands from the OpenAPI spec:

```bash
# Generate commands with base URL
openapi-cli generate petstore.yaml --base-url https://petstore.swagger.io

# Output:
# ✓ Parsed spec (version: 1.0.0)
# ✓ Generated 7 command groups
# ✓ Commands persisted to storage
# ✓ Registered command: pets
# ✓ Registered command: store
# ✓ Registered command: user
#
# ✓ Success! Commands registered and persisted
# Run 'openapi-cli --help' to see available commands
```

Commands are persisted to storage and loaded automatically on subsequent CLI invocations.

### Step 3: Execute Generated Commands

```bash
# List pets
openapi-cli pets list-pets

# Create a new pet
openapi-cli pets add-pet --name "Buddy" --status available

# Get pet by ID
openapi-cli pets get-pet-by-id --id 123

# Update pet
openapi-cli pets update-pet --id 123 --name "Buddy Updated"

# Delete pet
openapi-cli pets delete-pet --id 123
```

## Authentication

Configure authentication before making API requests:

### OAuth2 Authentication
```bash
# Login with OAuth2
openapi-cli auth login --oauth2

# The tool will guide you through the OAuth2 flow in your browser

# Check authentication status
openapi-cli auth status
```

### API Key Authentication
```bash
# Configure with API key
openapi-cli auth config set --api-key YOUR_API_KEY
```

### Basic Authentication
```bash
# Login with username/password
openapi-cli auth login --username admin --password secret
```

### Logout
```bash
# Clear credentials
openapi-cli auth logout
```

## Inventory Management

Manage API inventory from external hosts:

### Fetch Inventory
```bash
# Fetch inventory from API host
openapi-cli inventory fetch --host https://api.example.com

# Fetch and save as YAML
openapi-cli inventory fetch --host https://api.example.com --output yaml
```

### List Cached Inventories
```bash
# View all cached API inventories
openapi-cli inventory list

# Specify cache directory
openapi-cli inventory list --cache-dir ~/.cache/openapi-cli/inventory
```

### Validate Inventory
```bash
# Validate inventory schema
openapi-cli inventory validate --host https://api.example.com

# Validate with verbose output
openapi-cli inventory validate --host https://api.example.com --verbose
```

## Configuration

Manage configuration settings:

### View Current Configuration
```bash
openapi-cli config get
```

### Set Configuration Values
```bash
# Set output format
openapi-cli config set --output json

# Set timeout
openapi-cli config set --timeout 30s

# Set debug mode
openapi-cli config set --debug
```

### Reset to Defaults
```bash
openapi-cli config reset
```

## Output Formats

Control response output format (supported formats: `table`, `json`, `yaml`, `csv`):

```bash
# Table format (default, human-readable)
openapi-cli pets list-pets

# JSON format
openapi-cli pets list-pets --output json

# YAML format
openapi-cli pets list-pets --output yaml

# CSV format
openapi-cli pets list-pets --output csv
```

## Common Examples

### Example 1: Complete Workflow

```bash
# 1. Download OpenAPI spec
curl -o myapi.yaml https://api.example.com/openapi.yaml

# 2. Generate CLI commands
openapi-cli generate myapi.yaml --base-url https://api.example.com

# 3. Authenticate
openapi-cli auth login --oauth2

# 4. List resources
openapi-cli items list-items

# 5. Get specific item
openapi-cli items get-item --id 123 --output json
```

### Example 2: Debug Mode

```bash
# Enable debug for detailed logging
openapi-cli config set --debug

# Run command with debug output
openapi-cli pets list-pets --verbose
```

### Example 3: Batch Processing

```bash
# List and process as JSON
openapi-cli users list-users --output json > users.json

# Process with jq
cat users.json | jq '.[].email'
```

### Example 4: Multiple APIs

```bash
# Generate commands for API 1
openapi-cli generate api1.yaml --base-url https://api1.example.com

# Generate commands for API 2
openapi-cli generate api2.yaml --base-url https://api2.example.com

# Both APIs available in same CLI
openapi-cli --help  # Shows commands from both APIs
```

## Advanced Features

### Daemon Mode

Run as background daemon with Unix socket proxy:

```bash
# Start daemon
openapi-cli daemon start

# Stop daemon
openapi-cli daemon stop

# Check daemon status
openapi-cli daemon status
```

### MCP Server Integration

Serve commands via MCP protocol to Claude Desktop:

```bash
# Start MCP server
openapi-cli mcp-server

# Configure Claude Desktop with tool:
# {
#   "command": "openapi-cli mcp-server",
#   "args": []
# }
```

### WASM Modules

Load WASM plugins for custom logic:

```bash
# List loaded modules
openapi-cli modules list

# Load WASM module
openapi-cli modules load --module-path /path/to/module.wasm

# Execute WASM function
openapi-cli modules execute --function transformData --input '{"key": "value"}'
```

## Next Steps

- [CLI Reference](DOCS-CLI-REFERENCE.md) - Complete command reference
- [Configuration](DOCS-CONFIGURATION.md) - Configuration options and profiles
- [MCP Integration](DOCS-MCP-SETUP.md) - Set up MCP server for Claude Desktop
- [WASM Plugins](WASM-RUNTIME.md) - WASM module development guide
- [Authentication](OAUTH2-IMPLEMENTATION.md) - Authentication flows and security

## Troubleshooting

### Commands Not Found After Generate

```bash
# Commands are persisted and loaded automatically
# If not showing, check storage:
ls ~/.local/share/openapi-cli/commands.json

# Re-run generate if needed
openapi-cli generate spec.yaml --base-url https://api.example.com
```

### Authentication Errors

```bash
# Check auth status
openapi-cli auth status

# Clear credentials and re-authenticate
openapi-cli auth logout
openapi-cli auth login --oauth2
```

### Connection Errors

```bash
# Enable debug mode
openapi-cli config set --debug

# Verify connectivity
curl -v https://api.example.com
```

## Support

- GitHub: https://github.com/openapi-cli/openapi-cli
- Documentation: https://docs.openapi-cli.io
- Issues: https://github.com/openapi-cli/openapi-cli/issues
