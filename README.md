# Muster

> Turn any OpenAPI spec into a CLI, an MCP server, and a Go library. Instantly.

Muster reads an OpenAPI specification and generates executable commands — no codegen, no boilerplate, no SDK install. Point it at a spec URL and go.

## What you get

| Surface | Binary / Import | For |
|---------|----------------|-----|
| **CLI** | `openapi-cli` | Humans — `openapi-cli request GET /users` |
| **MCP Server** | `openapi-mcp` | AI agents — Claude Desktop, Cursor, any MCP client |
| **Daemon** | `openapi-daemon` | Multi-client — Unix socket proxy, shared state |
| **Go library** | `github.com/wojons/muster/pkg/...` | Other Go programs — import the parser, generator, client |
| **Docker** | `docker run` | Containerized deployment |
| **WASM plugins** | `--wasm-plugin` flag | Sandboxed response transformation (wazero runtime) |
| **Starlark DSL** | `dsl run script.star` | Multi-step API workflows with http_get/post/put/delete builtins |

Three deployment modes, one codebase. CLI for humans, MCP for AI agents, daemon for infrastructure. WASM for extensibility, Starlark for scripting. Import the Go packages directly for programmatic use.

## Quick start

```bash
# Build
git clone https://github.com/wojons/muster.git
cd muster
go build ./...

# Point at any OpenAPI spec
./bin/openapi-cli discover https://petstore3.swagger.io/api/v3/openapi.json

# Execute operations
./bin/openapi-cli request GET https://petstore3.swagger.io/api/v3/pet/findByStatus --status available

# Run the full demo (5 mock services, 10 scripts, all 7 surfaces)
./demo/run-all.sh
```

## Demo

The demo spins up 5 mock services and exercises every surface:

| Service | Port | What it provides |
|---------|------|-----------------|
| Task Forge | 9801 | Project management — CRUD, nested resources, tags, search |
| ByteStash | 9802 | File storage — upload, download, folders, trash/recover |
| Inkwell | 9803 | Blog/CMS — 4 auth roles, draft→publish workflow |
| Pulse | 9804 | Metrics engine — bulk insert, range queries, aggregations |
| ChainLink | 9805 | Workflow automation — multi-step chains, webhooks, polling |

```bash
cd demo && ./run-all.sh
# Builds binaries → starts 5 services → runs 10 demos → prints 23 pass/fail → cleans up
```

## CLI

```bash
# Discover what an API can do
openapi-cli discover https://api.example.com/openapi.json

# Make requests
openapi-cli request GET https://api.example.com/users
openapi-cli request POST https://api.example.com/users -H "Content-Type: application/json" -b '{"name":"Alice"}'

# Auth
export MUSTER_AUTH_TOKEN="your-token"
openapi-cli request GET https://api.example.com/admin --header "Authorization: Bearer $MUSTER_AUTH_TOKEN"

# WASM transformation
openapi-cli request GET https://api.example.com/data --wasm-plugin ./transform.wasm

# Starlark DSL
openapi-cli dsl run workflow.star
```

## MCP Server

Add to your Claude Desktop config or any MCP client:

```json
{
  "mcpServers": {
    "my-api": {
      "command": "openapi-mcp",
      "args": ["--spec", "https://api.example.com/openapi.json"]
    }
  }
}
```

Every OpenAPI operation becomes an MCP tool. `tools/list` shows them all. `tools/call` executes them. The AI never knows it's talking to a REST API.

## Daemon

```bash
# Start daemon
openapi-daemon proxy --backend-url https://api.example.com --socket-path /tmp/muster.sock

# Talk to it via curl
curl --unix-socket /tmp/muster.sock http://localhost/users
curl --unix-socket /tmp/muster.sock -X POST http://localhost/users -H "Content-Type: application/json" -d '{"name":"Alice"}'
```

## Go library

```go
import (
    "github.com/wojons/muster/pkg/openapi"
    "github.com/wojons/muster/pkg/client"
)

// Parse any OpenAPI spec
doc, _ := openapi.Parse(specBytes, opts)

// Use the HTTP client with auth, rate limiting, caching
c := client.New(client.WithAuth(token), client.WithRateLimit(100))
```

26 public packages under `pkg/`. The same packages the CLI uses.

## Build from source

```bash
git clone https://github.com/wojons/muster.git
cd muster

# Build all three binaries
go build -o bin/openapi-cli ./cmd/openapi-cli
go build -o bin/openapi-mcp ./cmd/mcp-server
go build -o bin/openapi-daemon ./cmd/daemon

# Run tests
go test ./...

# Run the demo
./demo/run-all.sh
```

Requires Go 1.23+, Python 3 (for demo services), Flask (`pip install flask`).

## Project structure

```
cmd/openapi-cli/     # CLI entry point
cmd/mcp-server/      # MCP server entry point
cmd/daemon/          # Daemon entry point
pkg/                 # 26 public packages (importable)
  openapi/           # OpenAPI spec parsing
  mcp/               # MCP protocol implementation
  client/            # HTTP client with auth, rate limiting, retry
  wasm/              # WASM runtime (wazero)
  dsl/               # Starlark DSL interpreter
  config/            # Configuration management
  daemon/            # Unix socket proxy
  cache/             # Multi-tier caching
  rate/              # Token bucket rate limiter
  auth/              # OAuth2, mTLS, bearer tokens
  ...                # 16 more packages
internal/            # Private utilities
demo/                # Self-contained demo (5 services, 10 scripts)
specs/               # 270+ specification files
```

## License

MIT — see [LICENSE](LICENSE)
