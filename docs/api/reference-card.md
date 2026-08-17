# OpenAPI CLI Reference Card

Quick reference for OpenAPI CLI commands and options.

## Global Options

| Flag | Description |
|------|-------------|
| `-h, --help` | Show help |
| `-v, --version` | Show version |

## Discovery

### `discover`
```bash
openapi-cli discover <url>
```
Fetch a remote OpenAPI 3.x spec and display operations grouped by tag.

## CLI Generation

### `generate`
```bash
openapi-cli generate <spec-path-or-url> [flags]
```
Options: `-u, --base-url <url>`

### `init` (help topic only — not yet implemented)
```bash
openapi-cli init   # prints help text; does nothing
```

### Go library: `pkg/generator` (operation extraction)

Use the generator as a library to build typed command trees from an OpenAPI 3.x
spec without the CLI:

```go
import (
    "github.com/getkin/kin-openapi/openapi3"
    "github.com/wojons/muster/pkg/generator"
)

gen, err := generator.NewGenerator(generator.Config{
    AppName:        "my-cli",
    BaseURL:        "https://api.example.com", // override spec servers[].url
    DefaultFormat:  "json",
    OperationStyle: generator.StyleFlat,
})
if err != nil { /* invalid config */ }

commands, err := gen.GenerateCommands(spec) // *openapi3.T → []*cobra.Command
```

`Config` fields: `AppName`, `AppDescription`, `Version`, `BaseURL`,
`NamingStrategy`, `OperationStyle`, `DefaultFormat`, `SupportedFormats`,
`CommandPrefix`, `ExcludeTags`, `IncludeOnlyTags`, `CustomMappings`
(`map[string]string` of operationId → command name).

Notes:
- Signatures: `NewGenerator(config generator.Config) (*generator.Generator, error)` and
  `(*generator.Generator).GenerateCommands(spec *openapi3.T) ([]*cobra.Command, error)`.
- Operations without an `operationId` use a fallback name derived from
  method + path; the fallback routes through the same path/BaseURL builder as
  operationId ops.
- Generated commands carry a `--base-url` flag (persistent on the resource
  command in the live tree, per-command in the persisted store) that overrides
  `MUSTER_BASE_URL` and the spec's `servers[].url`.

## Inventory

### `inventory`
```bash
openapi-cli inventory [command]
```
Subcommands: `add` (`--host <url> --file <path>`), `list`, `remove` (`--host <url>`)

## API Interaction

### `request`
```bash
openapi-cli request <method> <url> [flags]
```
Options: `-b, --body <json>`, `-H, --header <k=v>` (repeatable), `-o, --output <json|yaml|table>`, `-p, --param <k=v>` (repeatable), `--timeout <duration>`, `-v, --verbose`, `--wasm-plugin <path>`

## DSL

### `dsl run`
```bash
openapi-cli dsl run <file.star>
```
Execute a Starlark script with `http_get`, `http_post`, `http_put`, `http_delete` builtins.

## Daemon

### `daemon`
```bash
openapi-cli daemon [command]
```
Subcommands: `start` (`--socket <path>`, `--backend-url <url>`), `status` (`--socket <path>`), `stop` (`--socket <path>`)

## Configuration

### `config`
```bash
openapi-cli config [command]
```
Subcommands: `profile` (`list`, `show`, `switch`, `delete`), `show`

## Authentication

### `auth`
```bash
openapi-cli auth [command]
```
Subcommands: `add` (`--name <n> --type <api-key|bearer|basic> --value <v>`), `list`, `remove <name>`, `mtls` (`generate`, `configure`, `info`)

## Audit

### `audit`
```bash
openapi-cli audit [command]
```
Subcommands: `log` (`--filter <text>`, `--limit <n>`), `info`, `show` (`--limit <n>`), `status`

## Tenant

### `tenant`
```bash
openapi-cli tenant [command]
```
Subcommands: `current`, `list`, `set <id>`, `show`, `switch <id>`

## Webhook

### `webhook`
```bash
openapi-cli webhook [command]
```
Subcommands: `create` (`--name <n> --url <u> --events <list>`), `delete <id>`, `list`, `subscribe <url> <event>`, `test <id>`

## WASM Modules

### `modules run`
```bash
openapi-cli modules run <file.wasm> [args...] [flags]
```
Options: `--fn <name>` (default: `_start`)

## Utilities

### `version`
```bash
openapi-cli version [flags]
```
Options: `--output` (JSON format)

### `completion`
```bash
openapi-cli completion [bash|zsh|fish|powershell]
```

### `help`
```bash
openapi-cli help [command]
```

## MCP Server

The MCP server is a separate binary (`openapi-mcp`), built with `make build`:
```bash
./bin/openapi-mcp --help
```

## Output Formats

- `-o table` - Table format (default)
- `-o json` - JSON output
- `-o yaml` - YAML output

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MUSTER_SPEC_PATH` | Spec file path | `openapi.json` |
| `MUSTER_BASE_URL` | API base URL | From spec |
| `MUSTER_HTTP_TIMEOUT` | Request timeout | `30s` |
| `MUSTER_OUTPUT_FORMAT` | Output format | `table` |
| `MUSTER_LOG_LEVEL` | Logging level | `info` |
| `MUSTER_PROFILE` | Profile name | `default` |
| `MUSTER_AUTH_APIKEY_VALUE` | API key value | - |

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

### MCP server
```bash
./bin/openapi-mcp --spec openapi.json
```

### With environment variables
```bash
export API_KEY=secret
openapi-cli request GET https://api.example.com/pet/1 -H "X-API-Key: $API_KEY"
```

### Generate docs
(Not yet implemented — use `discover` to explore specs and `generate` to create CLI commands.)

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
| Debug info | `openapi-cli audit log` for recent events |

## Links

- [Full Documentation](../guides/) - All guides
- [Installation](../guides/installation.md) - Install guide
- [Quickstart](../guides/quickstart.md) - Get started
- [Configuration](../guides/configuration.md) - Config options
- [API Reference](../api/) - API docs
