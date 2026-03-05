# Quickstart Guide

Get up and running with OpenAPI CLI in 5 minutes.

## What You'll Learn

This quickstart will show you how to:
1. Verify your installation
2. Load an OpenAPI specification  
3. Generate CLI commands automatically
4. Make your first API request
5. Run as an MCP server

## Step 1: Verify Installation

```bash
openapi-cli --version
# Expected: openapi-cli 1.0.0

openapi-cli doctor
# Runs system checks and confirms all components are working
```

## Step 2: Load an OpenAPI Specification

Download our example PetStore spec or use your own:

```bash
# Save example spec
curl -L https://raw.githubusercontent.com/swagger-api/swagger-petstore/master/openapi.json -o petstore.json

# Verify spec loads
openapi-cli spec validate petstore.json
# ✓ Specification is valid
```

**What's happening**: OpenAPI CLI parses and validates the OpenAPI specification, ensuring it's compliant with the specification.

## Step 3: List Available API Resources

```bash
openapi-cli inventory list petstore.json

Available Resources:
  /pet                              [POST] Add a new pet
  /pet/{petId}                      [GET]    Find pet by ID
  /pet/findByStatus                [GET]    Finds pets by status
  /pet/{petId}                      [DELETE] Delete pet
  /store/inventory                 [GET]    Returns pet inventories
  /store/order/{orderId}           [GET]    Find purchase order
  /user                             [POST]   Create user
  /user/{username}                 [GET]    Get user by username
  ...
```

**What's happening**: The tool parses the OpenAPI specification and lists all available endpoints with their HTTP methods.

## Step 4: Generate CLI Commands

OpenAPI CLI automatically generates CLI commands from your API specification:

```bash
# Initialize CLI with spec
openapi-cli init petstore.json

# See generated commands
openapi-cli --help
# Available Commands:
#   pet       Pet operations
#   store     Store operations
#   user      User operations

# Explore pet commands
openapi-cli pet --help
# Available Commands:
#   add         Add a new pet to the store
#   find-id     Find pet by ID
#   find-status Find pets by status
#   update      Update an existing pet
#   delete      Delete a pet
```

**What's happening**: OpenAPI CLI dynamically generates CLI commands that mirror the structure of your API, making it intuitive to interact with.

## Step 5: Make Your First API Request

```bash
# Find a pet by ID
openapi-cli pet find-id --petId 1

# Response:
# {
#   "id": 1,
#   "name": "Fluffy",
#   "status": "available",
#   "category": {
#     "id": 0,
#     "name": "string"
#   },
#   "tags": [
#     {
#       "id": 0,
#   ...
}

# Find pets by status
openapi-cli pet find-status --status available

# Add a new pet
openapi-cli pet add --name Fluffy --status available --category.name Dog

# Get formatted output
openapi-cli pet find-id --petId 1 --format table
# ID  Name    Status     Category
# --  ----    ------     --------
# 1   Fluffy  available  Dog
```

**What's happening**: Commands automatically build requests, handle parameters, format responses, and display results in your preferred format.

## Step 6: Run as MCP Server

For AI assistant integration, run as MCP server:

```bash
openapi-cli mcp-server

# This starts the MCP server over stdio for protocol communication
# The server will:
# - List all available tools (API endpoints)
# - Execute tool calls (API requests)
# - Handle resource access
```

**Integration Example**:

```javascript
// Claude Desktop configuration: ~/Library/Application Support/Claude/claude_desktop_config.json
{
  "mcpServers": {
    "openapi-cli": {
      "command": "openapi-cli",
      "args": ["mcp-server", "--spec", "/path/to/petstore.json"]
    }
  }
}
```

## Step 7: Generate API Documentation

```bash
# Generate comprehensive documentation from spec
openapi-cli docs generate petstore.json --output ./docs

# Output includes:
# docs/index.html            # Full documentation
# docs/api/                  # API reference
# docs/schemas/              # Data models
```

Open `docs/index.html` in your browser to explore interactive API documentation with Swagger UI.

## Common Workflows

### Interactive Prompt Mode

```bash
openapi-cli interactive

# You'll enter an interactive shell:
> pet find-id --petId 1
> store inventory --format table
> user create --username testuser --email test@example.com
> exit
```

### Use Custom Configurations

```bash
openapi-cli config set base-url https://petstore.swagger.io/v2
openapi-cli config set timeout 30
openapi-cli config set output-format json

# Check configuration
openapi-cli config show
```

### Build Automation with Starlark

```bash
# Create a script file: api-test.star
#!/usr/bin/env openapi-cli dsl

# Find pet
pet = api.pet.find_id(pet_id=1)
print(f"Found pet: {pet['name']}")

# Update pet
pet['status'] = 'sold'
api.pet.update(id=1, body=pet)

# Run script
openapi-cli dsl run api-test.star --spec petstore.json
```

### Run as Daemon with Socket

```bash
# Start daemon
openapi-cli daemon start --socket /tmp/openapi-cli.sock

# Use Unix socket proxy
curl --unix-socket /tmp/openapi-cli.sock http://localhost/pet/find-id?petId=1

# Stop daemon
openapi-cli daemon stop
```

## Next Steps

- [Configuration Guide](configuration.md) - Customize your setup
- [Command Reference](commands.md) - Explore all commands
- [API Documentation](../api/README.md) - Complete API reference
- [Starlark DSL Guide](../guides/starlark-dsl.md) - Advanced scripting

## Common Questions

**Q: Can I use multiple specs at once?**

A: Yes! Use the `--spec` flag or load from a working directory:

```bash
openapi-cli pet add --spec ./specs/petstore.json --name Fluffy
openapi-cli user get --spec ./specs/users.json --username testuser
```

**Q: How do I handle authentication?**

A: Configure auth settings:

```bash
# API Key
openapi-cli config set auth.apikey.header X-API-Key
openapi-cli config set auth.apikey.value my-secret-key

# OAuth2
openapi-cli auth login --provider google --client-id YOUR_CLIENT_ID

# mTLS
openapi-cli auth mtls setup --cert client.crt --key client.key
```

**Q: Can I save common commands as aliases?**

A: Yes, create aliases in your config:

```yaml
aliases:
  get-pet: pet find-id --petId 1
  all-pets: pet find-status --status available
  create-order: store order
```

## Need Help?

- Run `openapi-cli --help` or `openapi-cli <command> --help`
- Check the [Guides Index](../README.md) for more tutorials
- Visit [GitHub Discussions](https://github.com/openapi-cli/openapi-cli/discussions)
