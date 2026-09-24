# Command Reference

Complete reference for all OpenAPI CLI commands.

## Global Options

All commands support these global flags:

| Flag | Description | Default |
|------|-------------|---------|
| `-h, --help` | Show help | - |
| `-v, --version` | Show version | - |

## Discovery Commands

### `discover`

Fetch an OpenAPI 3.x specification from a URL and display all operations
grouped by tag, with method, path, and operationId. No commands are persisted.

```bash
openapi-cli discover <url>
```

**Examples**:
```bash
openapi-cli discover https://petstore3.swagger.io/api/v3/openapi.json
openapi-cli discover http://localhost:9800/openapi.json
```

## CLI Generation Commands

### `generate`

Generate CLI commands dynamically from an OpenAPI 3.0 or 3.1 specification.
The generator parses the spec and creates Cobra commands for each API operation.
Accepts either a local file path or a remote URL (http/https).

Commands are persisted to storage and loaded on subsequent CLI invocations.

```bash
openapi-cli generate [<spec-path-or-url>] [flags]
```

**Options**:
- `-u, --base-url <url>` - Base URL for the API (e.g., `https://api.example.com`)
- `--list` - List the current persisted command store (spec path, hash, generated at, command count)
- `--clear` - Remove the persisted command store

**Examples**:
```bash
openapi-cli generate petstore.json
openapi-cli generate https://petstore3.swagger.io/api/v3/openapi.json
openapi-cli generate openapi.yaml --base-url https://api.example.com/v1
openapi-cli generate --list
openapi-cli generate --clear
```

**Overwrite warning**: If you run `generate` with a different spec than the one
currently persisted, the CLI prints a warning to stderr before overwriting the
store. Running `generate` with the same spec (same hash) does not warn.

### `init` (help topic)

`init` exists as a help topic only — it is **not yet implemented** as a runnable
command. Use `generate` to create CLI commands from a spec.

```bash
openapi-cli init   # prints help text only; does nothing
```

## Inventory Commands

### `inventory`

Manage OpenAPI spec inventories. Each inventory maps a remote host URL to a
local inventory file.

```bash
openapi-cli inventory [command]
```

**Subcommands**:

| Command | Description |
|---------|-------------|
| `add` | Add an inventory (`--host <url> --file <path>`) |
| `list` | List configured inventories |
| `remove` | Remove an inventory (`--host <url>`) |

**Examples**:
```bash
openapi-cli inventory add --host https://api.example.com --file ./inventory.json
openapi-cli inventory list
openapi-cli inventory remove --host https://api.example.com
```

## API Interaction Commands

### `request`

Send HTTP requests with support for various methods, headers, and output formats.

```bash
openapi-cli request <method> <url> [flags]
```

**Options**:
- `-b, --body <string>` - Request body for POST/PUT requests
- `-H, --header <strings>` - Add HTTP header (format: `'Key: Value'`; repeatable)
- `-o, --output <format>` - Output format (`json`, `yaml`, `table`; default: `table`)
- `-p, --param <strings>` - Add query parameter (format: `'key=value'`; repeatable)
- `--timeout <duration>` - Request timeout (default: `30s`)
- `-v, --verbose` - Verbose output
- `--wasm-plugin <path>` - Path to WASM plugin for response transformation

**Examples**:
```bash
openapi-cli request GET https://petstore.swagger.io/v2/pet/1
openapi-cli request POST https://api.example.com/pet --body '{"name":"Fluffy"}'
openapi-cli request GET https://api.example.com/pet/findByStatus --param status=available
openapi-cli request GET https://api.example.com/pet/1 --output json
```

### Typed and generated commands (`--body`/`-b`)

Generated commands (created by `generate`) support a raw JSON body flag on
operations that take a JSON request body (POST/PUT/PATCH methods). The flag
is added only when the operation has a JSON request body; if the spec itself
declares a property or parameter named `body`, that spec-declared flag wins
and the raw flag is not added.

The value is a raw JSON string (quote it in your shell). It is validated as
JSON and sent verbatim as the request body with `Content-Type:
application/json` — it is neither wrapped nor merged with other flags. When
`--body` is set it overrides the individual per-property body flags and
satisfies all required body properties in one shot (you do not need to pass
each required property flag). The same flag and semantics apply to commands
reloaded from the persisted command store. File input (e.g. `@file`) is not
supported — only inline JSON.

**Examples**:
```bash
openapi-cli myapi pet create --body '{"name":"Fluffy"}'
openapi-cli myapi pet update 42 --body '{"name":"Fluffy","status":"sold"}'
```

The command path is `<app-name> <resource> <operation>`; the app name comes
from the spec used with `generate`. Run `openapi-cli <app-name> <resource>
<operation> --help` to see the flags of a generated command, including
`--body` when the operation accepts a JSON body.

## DSL Commands

### `dsl run`

Execute a `.star` Starlark script with built-in HTTP functions:
`http_get(url)`, `http_post(url, body)`, `http_put(url, body)`, `http_delete(url)`.

```bash
openapi-cli dsl run <file.star> [flags]
```

**Examples**:
```bash
openapi-cli dsl run demo/scripts/workflow.star
```

## Daemon Commands

### `daemon`

Manage the daemon process for background API proxying and persistent connections.

```bash
openapi-cli daemon [command]
```

**Subcommands**:

| Command | Description |
|---------|-------------|
| `start` | Start the daemon (`--socket <path>`, `--backend-url <url>`) |
| `status` | Show daemon status (`--socket <path>`) |
| `stop` | Stop the daemon and clean up resources (`--socket <path>`) |

**Examples**:
```bash
openapi-cli daemon start
openapi-cli daemon start --socket /tmp/my-daemon.sock
openapi-cli daemon start --backend-url https://api.example.com
openapi-cli daemon status
openapi-cli daemon stop
```

## Configuration Commands

### `config`

View and modify OpenAPI CLI configuration.

```bash
openapi-cli config [command]
```

**Subcommands**:

| Command | Description |
|---------|-------------|
| `profile` | Manage configuration profiles (`list`, `show`, `switch`, `delete`) |
| `show` | Show current configuration |

**Examples**:
```bash
openapi-cli config show
openapi-cli config profile list
openapi-cli config profile switch production
```

## Authentication Commands

### `auth`

Manage authentication credentials stored in the system keychain.
Credentials are keyed by name and stored with a type (`api-key`, `bearer`, `basic`).
Secrets live in the OS keychain; metadata is kept in config.

```bash
openapi-cli auth [command]
```

**Subcommands**:

| Command | Description |
|---------|-------------|
| `add` | Store a credential (`--name <name> --type <type> --value <value>`) |
| `list` | List stored credentials |
| `remove` | Remove a credential (`<name>`) |
| `mtls` | Manage mutual TLS credentials (`generate`, `configure`, `info`) |

**Examples**:
```bash
openapi-cli auth add --name myapi --type api-key --value sk-abc123
openapi-cli auth list
openapi-cli auth remove myapi
openapi-cli auth mtls generate
```

## Audit Commands

### `audit`

View audit log events recorded by the CLI, daemon, and MCP server.
Events are persisted as JSON lines in the audit log file.

```bash
openapi-cli audit [command]
```

**Subcommands**:

| Command | Description |
|---------|-------------|
| `log` | Show recent audit events (`--filter <text>`, `--limit <n>`) |
| `info` | Show audit configuration and statistics |
| `show` | Show recent audit events (`--limit <n>`) |
| `status` | Show audit logging status |

**Examples**:
```bash
openapi-cli audit log --limit 20
openapi-cli audit log --filter "error"
openapi-cli audit status
```

## Tenant Commands

### `tenant`

Manage the multi-tenant context for this CLI. Each tenant maps to a named
config profile with isolated settings, quotas, and audit records. The active
tenant persists to disk.

```bash
openapi-cli tenant [command]
```

**Subcommands**:

| Command | Description |
|---------|-------------|
| `current` | Show the active tenant with name, tier, and timestamp |
| `list` | List known tenants |
| `set` | Set the active tenant (`<tenant-id>`) |
| `show` | Show current tenant |
| `switch` | Switch current tenant (`<tenant-id>`) |

**Examples**:
```bash
openapi-cli tenant list
openapi-cli tenant set production
openapi-cli tenant current
```

## Webhook Commands

### `webhook`

Manage webhook subscriptions for API events. Subscriptions are stored in the
local config directory and used by the daemon to deliver event notifications.

```bash
openapi-cli webhook [command]
```

**Subcommands**:

| Command | Description |
|---------|-------------|
| `create` | Create a webhook subscription (`--name <n> --url <u> --events <list>`) |
| `delete` | Remove a webhook subscription (`<id>`) |
| `list` | List webhook subscriptions |
| `subscribe` | Subscribe to an API event (`<url> <event-type>`) |
| `test` | Test connectivity to a webhook URL (`<id>`) |

**Examples**:
```bash
openapi-cli webhook create --name "my-hook" --url https://example.com/cb --events "api.request,api.error"
openapi-cli webhook list
openapi-cli webhook test wh_abc123
```

## WASM Module Commands

### `modules`

Manage and execute WebAssembly modules. Modules run in the wazero sandbox
(pure Go, no CGO) with resource limits enforced by the execution sandbox.

```bash
openapi-cli modules [command]
```

**Subcommands**:

| Command | Description |
|---------|-------------|
| `run` | Load and execute a WASM module (`<file.wasm> [args...] --fn <name>`) |

**Examples**:
```bash
openapi-cli modules run add.wasm 5 10 --fn add
```

## Utility Commands

### `version`

Print detailed version information about the OpenAPI CLI.

```bash
openapi-cli version [flags]
```

**Options**:
- `--output` - Output in JSON format

**Examples**:
```bash
openapi-cli version
openapi-cli version --output
```

### `completion`

Generate shell completion scripts for bash, zsh, fish, or PowerShell.

```bash
openapi-cli completion [bash|zsh|fish|powershell]
```

**Examples**:
```bash
openapi-cli completion bash > /etc/bash_completion.d/openapi-cli
openapi-cli completion zsh > "${fpath[1]}/_openapi-cli"
openapi-cli completion fish > ~/.config/fish/completions/openapi-cli.fish
```

### `help`

Show help for any command.

```bash
openapi-cli help [command]
```

## MCP Server

The MCP server is a separate binary (`openapi-mcp`), not a subcommand of
`openapi-cli`. Build it with `make build` and run it directly:

```bash
./bin/openapi-mcp --help
```

## Output Formats

The `request` command and every generated command (e.g. `get-pet` after
generating from a spec) accept `-o, --output`:

| Format | Behavior |
|--------|----------|
| `table` (default) | Human-readable table: arrays become rows with key headers; single objects become key/value rows |
| `json` | Pretty-printed JSON (2-space indent) |
| `yaml` | YAML rendering |

```bash
openapi-cli request GET https://petstore3.swagger.io/api/v3/pet/findByStatus?status=available -o json
openapi-cli get-pet 83165 -o yaml
```

Generated commands additionally accept `--raw` (`-r`) to dump the response
body verbatim, bypassing formatting:

```bash
openapi-cli get-pet 83165 --raw
```

Notes:

- Formatting applies to successful JSON responses. A non-JSON or empty body is
  printed as-is regardless of `--output`.
- On HTTP errors the response body is still printed, followed by an
  `HTTP error: <status>` message (exit code 4, see Exit Codes below).
- `--output`/`--raw` are provided by the command generator; commands reloaded
  from the persisted store currently print the raw body, do not apply
  `--output` formatting, and forward `--raw` as a `?raw=true` query parameter
  (loader gaps, tracked on the project board).

## Exit Codes

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

## Config File Locations

- **Global**: `~/.config/openapi-cli/config.yaml`
- **Project**: `./openapi-cli.yaml`

## Common Workflows

### Discover and generate from a spec
```bash
openapi-cli discover https://petstore3.swagger.io/api/v3/openapi.json
openapi-cli generate https://petstore3.swagger.io/api/v3/openapi.json
```

### Quick API test
```bash
openapi-cli request GET https://petstore.swagger.io/v2/pet/1
```

### Run as daemon
```bash
openapi-cli daemon start --socket /tmp/openapi-cli.sock
curl --unix-socket /tmp/openapi-cli.sock http://localhost/pet/1
openapi-cli daemon stop
```

### With environment variables
```bash
export API_KEY=secret
openapi-cli request GET https://api.example.com/pet/1 -H "X-API-Key: $API_KEY"
```

## Quick Config Examples

### Minimal config
```yaml
openapi-cli:
  spec:
    path: openapi.yaml
```

### With auth
```yaml
openapi-cli:
  spec:
    base_url: https://api.example.com/v1
  auth:
    type: apikey
    apikey:
      value: ${API_KEY}
```

### Full config
```yaml
openapi-cli:
  spec:
    path: ./api/openapi.yaml
    base_url: https://api.example.com/v1
  http:
    timeout: 30s
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
```

## Troubleshooting

| Issue | Solution |
|-------|----------|
| Command not found | `export PATH=$HOME/.local/bin:$PATH` |
| Permission denied | `chmod +x /usr/local/bin/openapi-cli` |
| Unknown command | Run `openapi-cli --help` for the real command list |
| 401 errors | `openapi-cli auth add --name myapi --type api-key --value <key>` |
| Timeout | `openapi-cli request ... --timeout 60s` |
| Debug info | Check `openapi-cli audit log` for recent events |

## Links

- [Full Documentation](../guides/) - All guides
- [Installation](../guides/installation.md) - Install guide
- [Quickstart](../guides/quickstart.md) - Get started
- [Configuration](../guides/configuration.md) - Config options
- [API Reference](../api/) - API docs
