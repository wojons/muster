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
| **Protocol adapters** | `pkg/protocol` | gRPC, JSON-RPC/OpenRPC, and GraphQL surfaces — see [`specs/PROTOCOLS.md`](specs/PROTOCOLS.md) |

Three deployment modes, one codebase. CLI for humans, MCP for AI agents, daemon for infrastructure. WASM for extensibility, Starlark for scripting. Import the Go packages directly for programmatic use.

> 📖 New here? The [Integration Guide](docs/integration-guide.md) has a runnable example for every surface — CLI, MCP, daemon, Go library, Docker, WASM, and Starlark — with real commands and outputs.

## Prerequisites

Building from source needs a Go toolchain **and** a C compiler. Muster's SQLite
support is a CGO package (`github.com/mattn/go-sqlite3`), and `make` builds with
`CGO_ENABLED=1` (its default), so a `gcc`/`clang` on `PATH` is not optional.

| Requirement | Notes |
|---|---|
| **Go 1.26.6+** | the version in [`go.mod`](go.mod) — older toolchains fail with a hard toolchain error. Check with `go version`; install from https://go.dev/dl/ |
| **C compiler** (`gcc` or `clang`) | required for CGO (SQLite). Without one, `make build-cli` stops with `cgo: C compiler "gcc" not found` (reported from `runtime/cgo`) |
| **make**, **git**, **curl** | build driver, cloning the repo, fetching specs over http(s) |

Debian 13 / Ubuntu — one line:

```bash
sudo apt-get update && sudo apt-get install -y golang gcc make git curl
```

`golang` from the distro archive is often **older than `go.mod`'s minimum**: run
`go version` and compare it against the `go` directive in [`go.mod`](go.mod)
(currently `1.26.6`) before building. If it is behind, install the toolchain
from https://go.dev/dl/ instead.

## Quick start

> **Note:** This repository is **PRIVATE** and is not published to the public Go proxy.
> **First**, configure GOPRIVATE and authenticate with GitHub:
> ```bash
> go env -w GOPRIVATE=github.com/wojons/*
> ```
> Then `go get github.com/wojons/muster@v0.1.0` resolves from the private repo.

```bash
# Build
git clone https://github.com/wojons/muster.git
cd muster
make build-cli

# Point at any OpenAPI spec
./bin/openapi-cli discover https://petstore3.swagger.io/api/v3/openapi.json

# Turn the spec into typed subcommands (the core "spec → CLI" step)
./bin/openapi-cli generate https://petstore3.swagger.io/api/v3/openapi.json

# Typed commands are now available on the next invocation, e.g.:
./bin/openapi-cli get-pet-by-id 1

# Or execute ad-hoc operations with the generic request command.
# Note: the public petstore3.swagger.io demo server returns HTTP 500 on most
# petstore endpoints — including findByStatus AND GET /pet/1 — server-side,
# not a Muster bug. For a reliable request example, use the demo's local mock
# services: ./demo/run-all.sh (below) starts them, then e.g.:
./bin/openapi-cli request GET http://localhost:9801/tasks -o json

# Point generated commands at another environment without regenerating:
# ./bin/openapi-cli get-pet-by-id 1 --base-url https://staging.example.com
# (or export MUSTER_BASE_URL=https://staging.example.com)
#
# Specs whose servers[].url is RELATIVE (e.g. "/api/v3") resolve against the
# spec fetch URL when generated over http(s). For local-file specs with a
# relative server, pass the base explicitly with --base-url (or
# MUSTER_BASE_URL) — running such a command without one fails with a hint
# naming those two options instead of a bare http error.

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

## Testing

```bash
make test-short           # Quick pass: all tests in -short mode — skips the long-running stability soak
make test-integration     # Integration-tagged tests (-tags=integration -run Integration) — proves components connect
make test-e2e             # Builds the binaries, then runs the end-to-end suite in test/e2e
make validate             # Full validation gate: unit tests + lint + vet + integration tests + security audit
./demo/run-all.sh         # The real end-to-end gate: builds, starts the 5 demo services, runs the 10 demo scripts, 23+ checks
```

> **Note:** bare `go test ./...` runs the full suite, which includes a 30-minute
> daemon stability soak test (`test/stability`) that exceeds Go's 10-minute
> per-package timeout and fails with "panic: test timed out". Use
> `make test-short` for a quick pass.

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
# Or attach a stored credential created via 'openapi-cli auth add':
openapi-cli request GET https://api.example.com/admin --auth myapi

# WASM transformation
openapi-cli request GET https://api.example.com/data --wasm-plugin ./transform.wasm

# Starlark DSL
openapi-cli dsl run workflow.star
```

## MCP Server

**`openapi-mcp` is the canonical (and only) MCP server binary** — built by `make build-mcp` from `cmd/mcp-server`. No other MCP binary is produced by the build; if a stray `mcp-server` or `openapi-cli-mcp-server` appears in `bin/`, it is a stale artifact from an ad-hoc build — delete it and rebuild.

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

Base URL resolution (DF-MUSTER-1): `openapi-mcp` targets the spec's own
`servers[].url` by default — `--spec` alone is enough when the spec declares
an absolute server URL, and every call would otherwise fail with
`unsupported protocol scheme ""`. `--base-url` or the `MUSTER_BASE_URL` env
var overrides the spec's server when you need to retarget (staging, local
mock, etc.).

Every OpenAPI operation becomes an MCP tool. `tools/list` shows them all. `tools/call` executes them. The AI never knows it's talking to a REST API.

## Daemon

```bash
# Start daemon (daemonizes by default: returns to the shell once the socket is up)
openapi-daemon start --socket-path /tmp/muster.sock

# Check status / stop it
openapi-daemon status --socket-path /tmp/muster.sock
openapi-daemon stop --socket-path /tmp/muster.sock

# Or run as a foreground proxy (useful for debugging)
openapi-daemon proxy --backend-url https://api.example.com --socket-path /tmp/muster.sock
openapi-daemon start --foreground --socket-path /tmp/muster.sock

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
parser := openapi.NewParser()
doc, _ := parser.Parse(ctx, specBytes, openapi.DefaultParseOptions())

// Use the HTTP client with base URL, retry, and default headers
c := client.NewClient(client.WithBaseURL("https://api.example.com"), client.WithTimeout(30*time.Second))
```

28 public packages under `pkg/`. The same packages the CLI uses. Runnable end-to-end example: [docs/integration-guide.md](docs/integration-guide.md#4-go-library-githubcomwojonsmusterpkg).

## Build from source

> **Note:** The repo is **PRIVATE** — clone requires GitHub credentials (see [Quick start](#quick-start) for GOPRIVATE setup).
> For local development without credentials, use a `replace` directive in your `go.mod`:
> ```go
> replace github.com/wojons/muster => /path/to/local/checkout
> ```

```bash
git clone https://github.com/wojons/muster.git
cd muster

# Build all three binaries
go build -o bin/openapi-cli ./cmd/openapi-cli
go build -o bin/openapi-mcp ./cmd/mcp-server
go build -o bin/openapi-daemon ./cmd/daemon

# Run tests
go test ./...
# NOTE: the full suite includes a 30-minute daemon stability soak test
# (test/stability), which exceeds Go's 10-minute per-package timeout and
# fails with "panic: test timed out". Use the short suite for a quick pass:
go test -short ./...
# (or: make test-short)

# Run the demo
./demo/run-all.sh
```

Requires Go 1.26.6+, Python 3 (for demo services), Flask (`pip install flask`).
Also requires a C compiler for CGO (SQLite) — see [Prerequisites](#prerequisites).

> **Linux build note:** default builds (`go build ./...`, `go test ./...`,
> all `make` targets) are dependency-free. The hardened CGO seccomp path is
> opt-in behind the `seccomp` build tag and requires `pkg-config` +
> `libseccomp-dev`:
> ```bash
> sudo apt-get install -y pkg-config libseccomp-dev   # Debian/Ubuntu
> # RHEL/CentOS/Fedora: sudo dnf install -y pkgconf-pkg-config libseccomp-devel
> go build -tags seccomp ./...
> ```
> Without the tag, `pkg/platform` compiles a pure-Go stub
> (`seccomp_stub.go`) that returns "seccomp support not compiled in" errors
> at runtime instead of enforcing filters.

## Project structure

```
cmd/openapi-cli/     # CLI entry point
cmd/mcp-server/      # MCP server entry point
cmd/daemon/          # Daemon entry point
pkg/                 # 28 public packages (importable)
  openapi/           # OpenAPI spec parsing
  mcp/               # MCP protocol implementation
  protocol/          # gRPC, JSON-RPC/OpenRPC, and GraphQL adapters
  client/            # HTTP client with auth, rate limiting, retry
  wasm/              # WASM runtime (wazero)
  dsl/               # Starlark DSL interpreter
  config/            # Configuration management
  daemon/            # Unix socket proxy
  cache/             # Multi-tier caching
  rate/              # Token bucket rate limiter
  auth/              # OAuth2, mTLS, bearer tokens
  ...                # 12 more packages
internal/            # Private utilities
demo/                # Self-contained demo (5 services, 10 scripts)
specs/               # 270+ specification files
```

## License

MIT — see [LICENSE](LICENSE)
