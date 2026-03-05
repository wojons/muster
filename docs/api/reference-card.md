# OpenAPI CLI Reference Card

Quick reference for OpenAPI CLI commands and options.

## Global Options

| Flag | Description |
|------|-------------|
| `-h, --help` | Show help |
| `-v, --version` | Show version |
| `-c, --config <path>` | Config file path |
| `-d, --debug` | Enable debug |
| `-q, --quiet` | Suppress output |
| `--profile <name>` | Configuration profile |

## Specification Commands

### `spec validate`
```bash
openapi-cli spec validate <spec-path> [options]
```
Options: `--strict`, `--format <text|json>`

### `spec info`
```bash
openapi-cli spec info <spec-path>
```

### `spec convert`
```bash
openapi-cli spec convert <input> <output>
```
Options: `--from <yaml|json>`, `--to <yaml|json>`

## CLI Generation

### `init`
```bash
openapi-cli init <spec-path> [options]
```
Options: `--output <dir>`, `--config <file>`

### `inventory list`
```bash
openapi-cli inventory list <spec-path> [options]
```
Options: `--tag <tag>`, `--method <method>`, `--format <table|json>`

## API Interaction

### `api request`
```bash
openapi-cli api request [METHOD] <path> [options]
```
Options: `--spec <path>`, `--body <json>`, `--header <k=v>`, `--query <k=v>`

### `api batch`
```bash
openapi-cli api batch <batch-file>
```

## Documentation

### `docs generate`
```bash
openapi-cli docs generate <spec-path> [options]
```
Options: `--output <dir>`, `--format <markdown|html>`

### `docs serve`
```bash
openapi-cli docs serve [options]
```
Options: `--port <port>`, `--dir <dir>`

## MCP Server

### `mcp-server`
```bash
openapi-cli mcp-server [options]
```
Options: `--spec <path>`, `--port <port>`, `--stdio`

## WASM

### `wasm run`
```bash
openapi-cli wasm run <module.wasm> [options]
```
Options: `--function <name>`, `--arg <value>`, `--memory <size>`, `--timeout <duration>`

### `wasm compile`
```bash
openapi-cli wasm compile <input> <output> [options]
```
Options: `--language <go|rust>`, `--optimize`

## DSL

### `dsl run`
```bash
openapi-cli dsl run <script.star> [options]
```
Options: `--spec <path>`, `--var <name=value>`, `--timeout <duration>`

## Daemon

### `daemon start`
```bash
openapi-cli daemon start [options]
```
Options: `--socket <path>`, `--pid-file <path>`

### `daemon stop`
```bash
openapi-cli daemon stop [options]
```
Options: `--force`

### `daemon status`
```bash
openapi-cli daemon status
```

## Configuration

### `config show`
```bash
openapi-cli config show [section]
```

### `config set`
```bash
openapi-cli config set <key> <value>
```

### `config unset`
```bash
openapi-cli config unset <key>
```

### `config validate`
```bash
openapi-cli config validate
```

## Authentication

### `auth login`
```bash
openapi-cli auth login <provider> [options]
```
Options: `--client-id <id>`, `--client-secret <secret>`, `--scopes <list>`

### `auth logout`
```bash
openapi-cli auth logout [provider]
```

### `auth status`
```bash
openapi-cli auth status
```

### `credentials store`
```bash
openapi-cli credentials store <name> [options]
```
Options: `--header <name>`, `--value <value>`

### `credentials list`
```bash
openapi-cli credentials list
```

### `credentials remove`
```bash
openapi-cli credentials remove <name>
```

## Cache

### `cache clear`
```bash
openapi-cli cache clear
```

### `cache stats`
```bash
openapi-cli cache stats
```

## Utilities

### `doctor`
```bash
openapi-cli doctor
```

### `version`
```bash
openapi-cli version
```

### `help`
```bash
openapi-cli help [command]
```

## Output Formats

- `--output json` - JSON output
- `--output pretty` - Pretty-printed JSON
- `--output table` - Table format
- `--output text` - Plain text
- `--format <format>` - Alias

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

### Initialize from spec
```bash
openapi-cli init openapi.json && openapi-cli --help
```

### Quick API test
```bash
openapi-cli api request GET /pet/1 --spec openapi.json
```

### MCP server
```bash
openapi-cli mcp-server --spec openapi.json
```

### With environment variables
```bash
export API_KEY=secret && openapi-cli api request POST /pet --body '{"name":"Fluffy"}'
```

### Interactive mode
```bash
openapi-cli interactive
```

### Generate docs
```bash
openapi-cli docs generate openapi.yaml --output ./docs
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
| Invalid spec | `openapi-cli spec validate --strict spec.yaml` |
| 401 errors | `openapi-cli auth login --provider your-provider` |
| Timeout | `openapi-cli config set http.timeout 60s` |
| Debug info | `openapi-cli --debug <command>` |
| Full check | `openapi-cli doctor` |

## Links

- [Full Documentation](../guides/) - All guides
- [Installation](../guides/installation.md) - Install guide
- [Quickstart](../guides/quickstart.md) - Get started
- [Configuration](../guides/configuration.md) - Config options
- [API Reference](../api/) - API docs
