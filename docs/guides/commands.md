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
Swagger 2.0 documents are also accepted: they are converted to the
OpenAPI 3.x model on ingest (`openapi-cli discover` prints a
"converted from Swagger/OpenAPI 2.0" note; a document that parses but
declares no operations is refused instead of persisting an empty command
store).
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

## gRPC Commands

The `grpc` group talks to gRPC servers through the protocol adapter seam
described in [specs/PROTOCOLS.md](../../specs/PROTOCOLS.md). One discovery
source model feeds all three verbs; invocation covers all four RPC kinds
(unary, server-streaming, client-streaming, bidirectional). Implementation:
`internal/builtin/grpc.go` (commands) and `pkg/protocol/grpc` (adapter).

Discovery source `<uri>` forms (accepted by `discover`, `schema`, and
`call --source`):

| Form | Meaning |
|------|---------|
| `file.proto` | .proto source, resolved against the current directory |
| `dir1:dir2=file.proto` | .proto file resolved against `dir1` (then cwd); `dir1:dir2` are the import paths for its includes |
| `file.pb` (any non-`.proto` file) | compiled `FileDescriptorSet` (descriptor set) |
| `grpc://host:port` | live server reflection |

Every verb also accepts `-h, --help` and the global `--profile` flag.

### `grpc discover`

Ingest a source and list the operations (RPCs) it exposes: one entry per
operation with its kind, operation ID, and full method path, sorted by ID.
A source with no `service` declaration lists no operations — operations are
RPCs; the types themselves are the `grpc schema` surface.

```bash
openapi-cli grpc discover <uri>
```

**Examples** (using fixtures shipped in this repository, so they are runnable
as soon as the CLI is built or installed):

```bash
openapi-cli grpc discover "pkg/protocol/grpc/testdata:pkg/protocol/grpc/testdata=main.proto"
openapi-cli grpc discover pkg/protocol/grpc/testdata/multi_all.pb
openapi-cli grpc discover grpc://localhost:9000
```

The first prints, for example:

```text
unary            mainpkg.Main-Do
         /mainpkg.Main/Do
```

A descriptor set built without `protoc --include_imports` is not
self-contained: the CLI fails early with a message naming the missing import
and the remedy (rebuild the set with `--include_imports`, or supply the
missing files as `.proto` sources / an import-path URI).

### `grpc schema`

Ingest a source and list the protobuf type graph it declares: one line per
type (kind, fully-qualified name, field or enum-value count) plus a total.
Unlike `discover`, a source with no service is still fully described —
message and enum declarations are the schema surface.

```bash
openapi-cli grpc schema <uri> [flags]
```

**Options**:
- `-o, --output <format>` - `text` (default) prints the type listing; `json` prints the whole schema set as JSON (types keyed by fully-qualified name, each with its fields — type, format, ref, repeated/map details — enum values, oneof group, and source stamps)

**Examples**:

```bash
openapi-cli grpc schema pkg/protocol/grpc/testdata/msgs_only.proto
openapi-cli grpc schema msgs_only.proto -o json
openapi-cli grpc schema grpc://localhost:9000
```

The first prints:

```text
enum     only.Flavor                              2 values
message  only.Other                               1 field
message  only.Thing                               2 fields
message  only.Third                               1 field
4 types
```

### `grpc call`

Invoke one gRPC method end-to-end: unary, server-streaming,
client-streaming, or bidirectional. The operation ID uses the
`service-method` form printed by `grpc discover` (e.g. `kv.KeyValue-Get`).

```bash
openapi-cli grpc call <service-method> [flags]
```

**Options**:
- `--source <uri>` - Discovery source (required; same forms as `grpc discover`). A `grpc://host:port` source also supplies the invocation target
- `--target <host:port>` - Invocation target. Optional when `--source grpc://host:port`; otherwise required — the call errors with `no invocation target` if neither names one
- `--arg <key=value>` - Request message field for unary/server-streaming methods (repeatable; values are strings, `true`/`false` parse as bools)
- `--messages <json|@file|@->` - Request message STREAM for client-streaming/bidi methods: an inline JSON array of objects, `@file.json`, or `@-` for stdin
- `--max-messages <n>` - Streaming bound: stop after this many response items (default 1000; `0` selects the default). There is no unbounded setting
- `--timeout <duration>` - Wall-clock bound for one streaming call, sending included (default 30s; `0` selects the default); also the request deadline for unary calls
- `--token <token>` - Bearer token attached as the standard `authorization` metadata header
- `--header <key:value>` - Raw metadata header (repeatable)
- `-o, --output <format>` - `json` (default, pretty-printed) or `raw` (compact JSON)

**Rules**:
- `--arg` and `--messages` are mutually exclusive (rejected as ambiguous). A method that takes one request message rejects `--messages`.
- On a client-streaming/bidi method, `--arg` seeds a ONE-message request stream and no flags at all sends an empty stream; the CLI prints a stderr note saying which it is doing.

**Examples**:

```bash
# Unary against a .proto source:
openapi-cli grpc call kv.KeyValue-Get --source kv.proto --target localhost:9000 --arg key=demo
# Unary via reflection (the source names the target, no --target needed):
openapi-cli grpc call kv.KeyValue-Get --source grpc://localhost:9000 --arg key=demo
# Server-streaming, capped at 5 items:
openapi-cli grpc call kv.KeyValue-Watch --source kv.proto --target localhost:9000 --arg prefix=k --max-messages 5
# Client-streaming / bidi: the request stream as a JSON array (@file.json and @- also work):
openapi-cli grpc call kv.KeyValue-Upload --source kv.proto --target localhost:9000 --messages '[{"key":"a"},{"key":"b"}]'
```

Every streaming call returns a **bounded transcript envelope**, never a live
stream (the bound flags above; specs/PROTOCOLS.md §4.5):

```json
{
  "kind": "server-stream",
  "status": "complete",
  "items": [],
  "items_truncated": false,
  "trailer": {"received": 3, "messages_sent": 3, "max_messages": 1000,
              "duration_ms": 12, "limit_reason": "", "metadata": {}}
}
```

Hitting `--max-messages` or `--timeout` is a clean stop: status
`"truncated"`, `items_truncated` true, `trailer.limit_reason` naming the
bound, exit code 0. A caller cancellation or deadline ends the stream
immediately with status `"cancelled"` and a non-zero exit; a mid-stream
server error prints the partial transcript, then fails with the mapped error
code.

For the full contract text (cancellation, backpressure, trailer metadata),
run `openapi-cli grpc call --help`.

## Protocol Commands

Protocol families other than OpenAPI and gRPC mount under the `protocol`
group (specs/PROTOCOLS.md §5.1; root OpenAPI verbs stay at the top level).
The GraphQL adapter is covered in the
[GraphQL Adapter guide](graphql-adapter.md); the JSON-RPC adapter is
documented below. Implementation: `internal/builtin/protocol.go`.

### `protocol jsonrpc`

Discover JSON-RPC 2.0 methods from an OpenRPC discovery document and execute
them over HTTP POST. One command, three modes:

| Mode | Flags | Behavior |
|------|-------|----------|
| Discovery | `--doc` | Fetch the OpenRPC document (local file path or http(s) URL) and print each method as a neutral operation (JSON list) |
| Document-backed call | `--doc --method` | Look the method up in the document and execute it against `--endpoint`, defaulting to the document's `servers[0].url` |
| Endpoint-only call | `--endpoint --method` (no `--doc`) | Send the JSON-RPC 2.0 request straight to `--endpoint` — no document fetch, no method lookup; both flags are required in this mode |

```bash
openapi-cli protocol jsonrpc [flags]
```

**Options**:
- `--doc <path-or-url>` - OpenRPC discovery document (file path or http(s) URL); enables discovery and document-backed calls
- `--endpoint <url>` - JSON-RPC endpoint URL (default: the document's `servers[0].url`); required for a call with no `--doc`
- `--method <name>` - Method to execute; omit it (with `--doc`) to only list discovered operations
- `--args <json>` - Method arguments as a JSON object (default `{}`)
- `--timeout <duration>` - Per-call timeout, default 30s (applies only when the call has no other deadline)

Batches are not implemented (optional in JSON-RPC 2.0); every call is a
single request.

**Examples** (verified against a local OpenRPC document naming
`servers[0].url = http://127.0.0.1:9999` and a trivial echo server):

```bash
# Discovery — lists methods with id, kind, parameters, and source stamps:
openapi-cli protocol jsonrpc --doc ./openrpc.json
# Document-backed call — --endpoint defaults to servers[0].url:
openapi-cli protocol jsonrpc --doc ./openrpc.json --method echo --args '{"msg":"hi"}'
# Endpoint-only call — no discovery document involved:
openapi-cli protocol jsonrpc --endpoint http://127.0.0.1:9999 --method echo --args '{"msg":"direct"}'
```

Discovery output is a JSON array; one entry looks like:

```json
[
  {
    "id": "echo",
    "kind": "unary",
    "summary": "Echo the message",
    "parameters": [
      {"name": "msg", "in": "payload", "type": "string", "required": true, "description": ""}
    ],
    "source": {"full_method": "echo", "server": "http://127.0.0.1:9999"}
  }
]
```

Calls print the decoded `result` as pretty-printed JSON
(`{"msg": "hi"}` for the examples above).

Usage errors name the missing flag — for example, an endpoint-only
invocation without `--method` fails with
`--method is required when --doc is omitted (...)`, and a `--doc`-backed
call with an unknown method lists the methods that were discovered.

Run `openapi-cli protocol jsonrpc --help` for the full mode contract.

## API Interaction Commands

### `request`

Send HTTP requests with support for various methods, headers, and output formats.

```bash
openapi-cli request <method> <url> [flags]
```

**Options**:
- `--auth <name>` - Attach a stored credential by name (see `auth add`; bearer → `Authorization: Bearer <secret>`, api-key → `X-API-Key: <secret>`; explicit `-H` with the same header overrides)
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
openapi-cli request GET https://api.example.com/tasks --auth myapi
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
| `profile` | Manage configuration profiles (`create`, `list`, `show`, `switch`, `delete`) |
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

Stored credentials are attached to requests with `--auth <name>` (on the
`request` command and on generated typed commands). `bearer` credentials
attach as `Authorization: Bearer <secret>`, `api-key` credentials as
`X-API-Key: <secret>`. The secret itself is never printed — verbose output
renders it masked (`Authorization: Bearer ***`).

**Examples**:
```bash
openapi-cli auth add --name myapi --type api-key --value sk-abc123
openapi-cli auth list
openapi-cli auth remove myapi
openapi-cli auth mtls generate
openapi-cli request GET https://api.example.com/tasks --auth myapi
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
| `info` | Show audit configuration and statistics (including the enforced `Retention:`) |
| `prune` | Enforce retention: rotate rows older than the window (`--days <n>`, `--dry-run`) |
| `show` | Show recent audit events (`--limit <n>`) |
| `status` | Show audit logging status |

**Retention**: the default window is 365 days (`RetentionDays`), the value
`audit info` reports. Expired rows are rotated into
`<audit log>.archive-<unixnano>.jsonl` and the active log is rewritten with a
valid hash chain; retention is enforced on `audit prune` and whenever a writer
opens the sink. `audit log --limit N` reads only the newest `N` rows off the end
of the file, so its cost does not grow with the size of the log.

**Examples**:
```bash
openapi-cli audit log --limit 20
openapi-cli audit log --filter "error"
openapi-cli audit prune --dry-run
openapi-cli audit prune
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
| 401 errors | `openapi-cli auth add --name myapi --type api-key --value <key>`, then add `--auth myapi` to the request |
| Timeout | `openapi-cli request ... --timeout 60s` |
| Debug info | Check `openapi-cli audit log` for recent events |

## Links

- [Full Documentation](../guides/) - All guides
- [Installation](../guides/installation.md) - Install guide
- [Quickstart](../guides/quickstart.md) - Get started
- [Configuration](../guides/configuration.md) - Config options
- [API Reference](../api/) - API docs
