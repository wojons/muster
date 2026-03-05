# Command Reference

Complete reference for all OpenAPI CLI commands.

## Global Options

All commands support these global flags:

| Flag | Description | Default |
|------|-------------|---------|
| `-h, --help` | Show help | - |
| `-v, --version` | Show version | - |
| `-c, --config <path>` | Configuration file | `openapi-cli.yaml` |
| `-d, --debug` | Enable debug mode | `false` |
| `-q, --quiet` | Suppress output | `false` |
| `--profile <name>` | Use configuration profile | default |

## Specification Commands

### `spec validate`

Validate an OpenAPI specification.

```bash
openapi-cli spec validate <spec-path>
```

**Options**:
- `--strict` - Enable strict validation (default: false)
- `--format <format>` - Output format (text, json) (default: text)

**Examples**:
```bash
# Validate spec
openapi-cli spec validate openapi.yaml

# Strict validation
openapi-cli spec validate openapi.yaml --strict

# JSON output
openapi-cli spec validate openapi.json --format json
```

### `spec info`

Display information about an OpenAPI specification.

```bash
openapi-cli spec info <spec-path>
```

**Examples**:
```bash
openapi-cli spec info openapi.yaml
# Output:
# Title: PetStore API
# Version: 1.0.0
# Description: Sample PetStore API
# Endpoints: 20
```

### `spec convert`

Convert between different specification formats.

```bash
openapi-cli spec convert <input> <output> [options]
```

**Options**:
- `--from <format>` - Input format (yaml, json) (default: auto-detect)
- `--to <format>` - Output format (yaml, json) (default: yaml)

**Examples**:
```bash
openapi-cli spec convert openapi.json openapi.yaml
openapi-cli spec convert openapi.yaml openapi.json --to json
```

## CLI Generation Commands

### `init`

Initialize CLI from OpenAPI specification.

```bash
openapi-cli init <spec-path> [options]
```

**Options**:
- `--output <dir>` - Output directory (default: `./`)
- `--config <file>` - Config file name (default: `openapi-cli.yaml`)

**Examples**:
```bash
openapi-cli init petstore.json
openapi-cli init api.yaml --config my-config.yaml
```

### `inventory`

List API resources from specification.

```bash
openapi-cli inventory list <spec-path> [options]
```

**Options**:
- `--tag <tag>` - Filter by tag
- `--method <method>` - Filter by HTTP method
- `--format <format>` - Output format (table, json) (default: table)

**Examples**:
```bash
openapi-cli inventory list petstore.json
openapi-cli inventory list api.yaml --tag pets
openapi-cli inventory list api.yaml --method GET --format json
```

## API Interaction Commands

### `api request`

Make an API request to an endpoint.

```bash
openapi-cli api request [method] <path> [options]
```

**Options**:
- `--spec <path>` - OpenAPI spec file (default: from config)
- `--body <data>` - Request body (JSON string)
- `--header <name=value>` - Add request header
- `--query <name=value>` - Add query parameter
- `--output <format>` - Response format (json, pretty)

**Examples**:
```bash
openapi-cli api request GET /pet/1 --spec petstore.json
openapi-cli api request POST /pet --body '{"name":"Fluffy"}'
openapi-cli api request GET /pet/findByStatus?status=available
```

### `api batch`

Make multiple API requests in parallel.

```bash
openapi-cli api batch <batch-file>
```

**Batch file format**:
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

**Examples**:
```bash
openapi-cli api batch requests.json
```

## Documentation Commands

### `docs generate`

Generate API documentation from OpenAPI spec.

```bash
openapi-cli docs generate <spec-path> [options]
```

**Options**:
- `--output <dir>` - Output directory (default: `./docs`)
- `--format <format>` - Output format (markdown, html) (default: markdown)
- `--theme <theme>` - Documentation theme

**Examples**:
```bash
openapi-cli docs generate openapi.yaml
openapi-cli docs generate openapi.yaml --output ./site --format html
```

### `docs serve`

Serve documentation locally.

```bash
openapi-cli docs serve [options]
```

**Options**:
- `--port <port>` - Port number (default: 8080)
- `--dir <dir>` - Documentation directory (default: `./docs`)

**Examples**:
```bash
openapi-cli docs serve
openapi-cli docs serve --port 3000
```

## MCP Server Commands

### `mcp-server`

Run Model Context Protocol server.

```bash
openapi-cli mcp-server [options]
```

**Options**:
- `--spec <path>` - OpenAPI spec file
- `--port <port>` - Port for HTTP transport
- `--stdio` - Use stdio transport (default)

**Examples**:
```bash
openapi-cli mcp-server --spec openapi.json
openapi-cli mcp-server --spec openapi.json --stdio
```

## WASM Commands

### `wasm run`

Run a WASM module.

```bash
openapi-cli wasm run <module.wasm> [options]
```

**Options**:
- `--function <name>` - Function to call
- `--arg <value>` - Function argument
- `--memory <size>` - Memory limit (default: 128MB)
- `--timeout <duration>` - Execution timeout (default: 30s)

**Examples**:
```bash
openapi-cli wasm run add.wasm --function add --arg 5 --arg 10
openapi-cli wasm run process.wasm --function process --arg '{"input":"value"}'
```

### `wasm compile`

Compile code to WASM.

```bash
openapi-cli wasm compile <input> <output> [options]
```

**Options**:
- `--language <lang>` - Input language (go, rust, c++)
- `--optimize` - Enable optimization

**Examples**:
```bash
openapi-cli wasm compile add.go add.wasm --optimize
```

## DSL Commands

### `dsl run`

Run a Starlark DSL script.

```bash
openapi-cli dsl run <script.star> [options]
```

**Options**:
- `--spec <path>` - OpenAPI spec file
- `--var <name=value>` - Define variable
- `--timeout <duration>` - Execution timeout

**Examples**:
```bash
openapi-cli dsl run script.star --spec openapi.yaml
openapi-cli dsl run script.star --var API_KEY=secret
```

### `dsl test`

Test a DSL script.

```bash
openapi-cli dsl test <script.star>
```

**Examples**:
```bash
openapi-cli dsl test automation.star
```

## Daemon Commands

### `daemon start`

Start daemon in background.

```bash
openapi-cli daemon start [options]
```

**Options**:
- `--socket <path>` - Unix socket path (default: `/tmp/openapi-cli.sock`)
- `--pid-file <path>` - PID file path (default: `/var/run/openapi-cli.pid`)

**Examples**:
```bash
openapi-cli daemon start
openapi-cli daemon start --socket /tmp/my-daemon.sock
```

### `daemon stop`

Stop daemon.

```bash
openapi-cli daemon stop [options]
```

**Options**:
- `--force` - Force kill

**Examples**:
```bash
openapi-cli daemon stop
openapi-cli daemon stop --force
```

### `daemon status`

Check daemon status.

```bash
openapi-cli daemon status
```

### `daemon restart`

Restart daemon.

```bash
openapi-cli daemon restart
```

## Configuration Commands

### `config show`

Show configuration.

```bash
openapi-cli config show [section]
```

**Examples**:
```bash
openapi-cli config show
openapi-cli config show http
openapi-cli config show --resolved
```

### `config set`

Set configuration value.

```bash
openapi-cli config set <key> <value>
```

**Examples**:
```bash
openapi-cli config set http.timeout 60s
openapi-cli config set spec.base_url https://api.example.com/v1
```

### `config unset`

Unset configuration value.

```bash
openapi-cli config unset <key>
```

**Examples**:
```bash
openapi-cli config unset http.timeout
```

### `config validate`

Validate configuration.

```bash
openapi-cli config validate
```

## Authentication Commands

### `auth login`

Login to OAuth2 provider.

```bash
openapi-cli auth login <provider> [options]
```

**Options**:
- `--client-id <id>` - OAuth2 client ID
- `--client-secret <secret>` - OAuth2 client secret
- `--scopes <scopes>` - OAuth2 scopes

**Examples**:
```bash
openapi-cli auth login google --client-id YOUR_CLIENT_ID
```

### `auth logout`

Logout and remove credentials.

```bash
openapi-cli auth logout [provider]
```

**Examples**:
```bash
openapi-cli auth logout
openapi-cli auth logout google
```

### `auth status`

Show authentication status.

```bash
openapi-cli auth status
```

### `credentials store`

Store credentials securely.

```bash
openapi-cli credentials store <name> [options]
```

**Options**:
- `--header <name>` - HTTP header name
- `--value <value>` - Credential value

**Examples**:
```bash
openapi-cli credentials store myapi --header X-API-Key --value secret
```

### `credentials list`

List stored credentials.

```bash
openapi-cli credentials list
```

### `credentials remove`

Remove stored credentials.

```bash
openapi-cli credentials remove <name>
```

## Cache Commands

### `cache clear`

Clear cache.

```bash
openapi-cli cache clear
```

### `cache stats`

Show cache statistics.

```bash
openapi-cli cache stats
```

## Utility Commands

### `doctor`

Run system diagnostics.

```bash
openapi-cli doctor
```

**Checks**:
- Configuration validity
- File system permissions
- Network connectivity
- Dependency versions

### `version`

Show version information.

```bash
openapi-cli version
```

### `help`

Show help for commands.

```bash
openapi-cli help
openapi-cli help <command>
```

**Examples**:
```bash
openapi-cli help
openapi-cli help init
openapi-cli help "api request"
```

## Output Format Options

### JSON Output

```bash
openapi-cli api request GET /pet/1 --output json
```

### Pretty Output

```bash
openapi-cli api request GET /pet/1 --output pretty
```

### Table Output

```bash
openapi-cli inventory list api.yaml --format table
```

### Quiet Mode

```bash
openapi-cli api request GET /pet/1 --quiet
```

## Debug Mode

Enable debug output:

```bash
openapi-cli --debug api request GET /pet/1
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Generic error |
| 2 | Invalid command-line arguments |
| 3 | Command or file not found |
| 4 | API request error |
| 5 | Authentication error |
| 6 | Validation error |
| 7 | Timeout |
| 8 | Out of memory |
| 127 | Interrupted by user |

## Command Aliases

Create custom aliases in configuration:

```yaml
aliases:
  list-pets: inventory list
  get-pet: api request GET
  create-pet: api request POST
```

## Next Steps

- [Installation Guide](installation.md) - Get started
- [Configuration Guide](configuration.md) - Customize setup
- [API Documentation](../api/README.md) - API reference
