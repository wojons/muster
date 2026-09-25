# Configuration Guide

Complete guide to customizing OpenAPI CLI behavior.

## Configuration Overview

OpenAPI CLI uses a hierarchical configuration system:

1. **Global Config** (`~/.config/openapi-cli/config.yaml`) - System-wide settings
2. **Project Config** (`./openapi-cli.yaml`) - Project-specific settings
3. **Environment Variables** - Override any config value
4. **Command-line Flags** - Highest precedence

## Configuration File Format

By default, OpenAPI CLI looks for `openapi-cli.yaml` in your project (or current directory):

```yaml
openapi-cli:
  # API Specification
  spec:
    path: ./openapi.json
    base_url: https://api.example.com/v1

  # Command Generation
  cli:
    commands:
      # Customize command naming
      naming:
        verb_first: false
        include_path: true
        separator: '-'

  # Request/Response Settings
  http:
    timeout: 30s
    max_retries: 3
    retry_delay: 1s
    verify_ssl: true

  # Output Formatting
  output:
    format: table
    colors: true
    timezone: UTC

  # Authentication
  auth:
    type: apikey
    apikey:
      header: X-API-Key
      # Load from environment for security
      value: ${API_KEY}

  # MCP Server
  mcp:
    enabled: true
    port: 8080

  # Daemon Mode
  daemon:
    enabled: false
    socket: /tmp/openapi-cli.sock

  # WASM Runtime
  wasm:
    enabled: true
    max_memory: 128MB
    max_cpu: 1

  # Caching
  cache:
    enabled: true
    ttl: 3600s
    location: ~/.cache/openapi-cli

  # Logging
  log:
    level: info
    format: json
    output: stderr
```

## Configuration Locations

### Global Configuration

```bash
# Global config location
mkdir -p ~/.config/openapi-cli
cat > ~/.config/openapi-cli/config.yaml << EOF
openapi-cli:
  http:
    timeout: 60s
  output:
    format: json
EOF
```

### Project Configuration

```bash
# Project config location (in your project directory)
cat > openapi-cli.yaml << EOF
openapi-cli:
  spec:
    path: ./api/openapi.yaml
    base_url: https://api.example.com/v1
EOF
```

### Environment Variables

Override any configuration value with environment variables:

```bash
# Format: MUSTER_<SECTION>_<KEY>
export MUSTER_SPEC_PATH=/path/to/openapi.json
export MUSTER_HTTP_TIMEOUT=60s
export MUSTER_OUTPUT_FORMAT=table
export MUSTER_LOG_LEVEL=debug
```

Common environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `MUSTER_SPEC_PATH` | Path to OpenAPI spec file | `openapi.json` |
| `MUSTER_BASE_URL` | API base URL | From spec |
| `MUSTER_HTTP_TIMEOUT` | Request timeout | `30s` |
| `MUSTER_OUTPUT_FORMAT` | Output format (json/table/yaml) | `table` |
| `MUSTER_LOG_LEVEL` | Logging level | `info` |
| `MUSTER_AUTH_APIKEY_VALUE` | API key value | - |
| `MUSTER_DAEMON_SOCKET` | Unix socket path | `/tmp/openapi-cli.sock` |

## Configuration Commands

### View Current Configuration

```bash
# Show all configuration
openapi-cli config show

# Show specific section
openapi-cli config show http

# Show resolved configuration (merged from all sources)
openapi-cli config show --resolved
```

### Set Configuration Values

```bash
# Set a value
openapi-cli config set http.timeout 60s
openapi-cli config set output.format json
openapi-cli config set auth.type oauth2

# Set nested values
openapi-cli config set spec.base_url https://api.example.com/v1
openapi-cli config set auth.oauth2.client_id YOUR_CLIENT_ID
```

### Unset Configuration Values

```bash
# Unset a value (reverts to default)
openapi-cli config unset http.timeout

# Reset entire configuration
openapi-cli config reset
```

### Configuration Validation

```bash
# Validate configuration file
openapi-cli config validate

# Check configuration for syntax errors
openapi-cli config lint
```

## Authentication Configuration

### API Key Authentication

```yaml
auth:
  type: apikey
  apikey:
    header: X-API-Key
    # From environment (recommended for security)
    value: ${API_KEY}
EOF

api_key_from_env_value: ${API_KEY}
```

### OAuth2 Authentication

```yaml
auth:
  type: oauth2
  oauth2:
    client_id: ${OAUTH_CLIENT_ID}
    client_secret: ${OAUTH_CLIENT_SECRET}
    token_url: https://auth.example.com/oauth/token
    scopes:
      - read
      - write
```

### mTLS Authentication

```yaml
auth:
  type: mtls
  mtls:
    cert_path: ${TLS_CERT_PATH}
    key_path: ${TLS_KEY_PATH}
    ca_path: ${TLS_CA_PATH}
```

### Managing Credentials

```bash
# Login to OAuth2 provider
openapi-cli auth login --provider google --client-id YOUR_CLIENT_ID

# Store credentials securely
openapi-cli auth add --name myapi --type api-key --value my-secret

# List stored credentials
openapi-cli auth list

# Remove credentials
openapi-cli auth remove myapi
```

### Using Stored Credentials (`--auth`)

Credentials stored with `auth add` are attached to outgoing requests with
`--auth <name>` — on the `request` command and on generated typed
commands:

```bash
openapi-cli request GET https://api.example.com/tasks --auth myapi
```

The credential type decides the header: `bearer` attaches as
`Authorization: Bearer <secret>`, `api-key` as `X-API-Key: <secret>`,
`basic` as `Authorization: Basic <base64 user:password>`. An explicit
`-H "Header: Value"` with the same name overrides the stored credential.
Secrets are never echoed — verbose output renders the credential masked
(`Authorization: Bearer ***`).

## Output Configuration

### Output Formats

Available output formats: `json`, `yaml`, `table`, `text`

```yaml
output:
  format: table
  colors: true
  pretty: true
```

### Table Formatting

```yaml
output:
  table:
    max_width: 120
    truncate: true
    alignments:
      id: right
      name: left
      price: right
```

### JSON/YAML Formatting

```yaml
output:
  json:
    indent: 2
    sort_keys: true
  yaml:
    indent: 2
    min_line_width: 80
```

## HTTP Configuration

```yaml
http:
  timeout: 30s
  max_retries: 3
  retry_delay: 1s
  verify_ssl: true
  keep_alive: true
  max_idle_conns: 100
  idle_conn_timeout: 90s
```

### Proxy Configuration

```yaml
http:
  proxy:
    http: http://proxy.example.com:8080
    https: http://proxy.example.com:8443
    no_proxy:
      - localhost
      - 127.0.0.1
```

## MCP Server Configuration

```yaml
mcp:
  enabled: true
  port: 8080
  host: 0.0.0.0
  spec_path: ./openapi.json
  capabilities:
    tools: true
    resources: true
    prompts: true
```

Starting the MCP server:

```bash
openapi-cli mcp-server --config openapi-cli.yaml
```

## Daemon Configuration

```yaml
daemon:
  enabled: false
  socket: /tmp/openapi-cli.sock
  pid_file: /var/run/openapi-cli.pid
  log_file: /var/log/openapi-cli.log
  auto_start: false
```

Managing the daemon:

```bash
# Start daemon
openapi-cli daemon start

# Check status
openapi-cli daemon status

# Stop daemon
openapi-cli daemon stop
openapi-cli daemon stop --force  # Force stop with SIGKILL
```

## Cache Configuration

```yaml
cache:
  enabled: true
  ttl: 3600s
  location: ~/.cache/openapi-cli
  max_size: 1GB
  eviction: lru  # lru or ttl
```

Cache management:

```bash
# Clear cache
openapi-cli cache clear

# Show cache stats
openapi-cli cache stats

# Disable for a command
openapi-cli pet get --cache=false
```

## Logging Configuration

```yaml
log:
  level: info  # debug, info, warn, error
  format: json  # json or text
  output: stderr  # stderr, stdout, or path to file
  timestamps: true
  caller: false
```

Structured logging example (JSON format):

```json
{
  "level": "info",
  "timestamp": "2026-03-02T10:00:00Z",
  "message": "Making API request",
  "method": "GET",
  "url": "https://api.example.com/pet/1"
}
```

## Command Aliases

Create custom aliases for common commands:

```yaml
aliases:
  get-pet: pet find-id --petId 1
  all-pets: pet find-status --status available
  create-order: store order
```

Using aliases:

```bash
openapi-cli get-pet
openapi-cli all-pets
```

## Profiles

Create profiles for different environments:

```yaml
profiles:
  development:
    spec:
      base_url: https://dev-api.example.com/v1
    log:
      level: debug

  staging:
    spec:
      base_url: https://staging-api.example.com/v1
    http:
      timeout: 60s

  production:
    spec:
      base_url: https://api.example.com/v1
    http:
      timeout: 30s
      max_retries: 3
```

Using profiles:

The active profile is resolved in this order: `--profile` flag >
`MUSTER_PROFILE` environment variable > the persisted `default_profile`
in your config file > none.

```bash
# Persist a default profile (writes default_profile into your config
# file; the choice survives across commands and restarts)
openapi-cli config profile switch production

# Load a specific profile for one command (global flag, highest priority)
openapi-cli --profile production pet get

# Profile from environment (overrides the persisted default)
export MUSTER_PROFILE=production
openapi-cli pet get
```

Show the active profile and where it came from:

```bash
openapi-cli config show
# ...
# Default Profile: production
# Active Profile: production (source: persisted)
```

## Advanced Configuration

### WASM Runtime Settings

```yaml
wasm:
  enabled: true
  max_memory: 128MB
  max_cpu: 1
  allowed_domains:
    - wasm.example.com
  sandbox:
    filesystem: false
    network: true
  modules:
    - name: http-client
      path: ./wasm/http-client.wasm
```

### Starlark DSL Configuration

```yaml
dsl:
  enabled: true
  stdlib: true
  max_execution_time: 60s
  allowed_imports:
    - http
    - json
    - time
    - io
```

### Rate Limiting

```yaml
rate_limit:
  enabled: true
  requests_per_second: 10
  burst: 100
  algorithm: token_bucket
```

## Configuration Validation

```bash
# Validate configuration
openapi-cli config validate

# Check for best practices
openapi-cli config check

# Show configuration merge hierarchy
openapi-cli config hierarchy
```

## Example Configuration Files

### Minimal Configuration

```yaml
openapi-cli:
  spec:
    path: openapi.yaml
```

### Complete Production Configuration

```yaml
openapi-cli:
  spec:
    path: ./api/openapi.yaml
    base_url: https://api.example.com/v1

  http:
    timeout: 30s
    max_retries: 3
    verify_ssl: true

  auth:
    type: oauth2
    oauth2:
      client_id: ${OAUTH_CLIENT_ID}
      client_secret: ${OAUTH_CLIENT_SECRET}
      token_url: https://auth.example.com/oauth/token

  output:
    format: table
    colors: true

  mcp:
    enabled: true

  log:
    level: info
    format: json

  cache:
    enabled: true
    ttl: 3600s
```

## Next Steps

- [Command Reference](commands.md) - All available commands
- [REST API Authentication](../api/rest-api.md#authentication) - Authentication methods and setup
- [MCP Server Guide](../api/mcp-protocol.md) - MCP protocol details
