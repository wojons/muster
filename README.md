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
| **Go 1.26.10+** (repo developed on **1.26.10**) | the `go` directive in [`go.mod`](go.mod) is `1.26.10`; a toolchain older than that either auto-fetches it (`GOTOOLCHAIN=auto`, the default — needs network) or fails with a hard toolchain error (`GOTOOLCHAIN=local`). Check with `go version`; install from https://go.dev/dl/ |
| **C compiler** (`gcc` or `clang`) | required for CGO (SQLite). Without one, `make build-cli` stops with `cgo: C compiler "gcc" not found` (reported from `runtime/cgo`) |
| **make**, **git**, **curl** | build driver, cloning the repo, fetching specs over http(s) |
| **Python 3 + Flask** | demo only — the 5 mock services are Flask apps. Install the pinned demo dependencies into the repo venv (`python3 -m venv .venv && .venv/bin/pip install -r demo/requirements.txt`); a bare system `pip install flask` is refused on Debian 12/13, Ubuntu 23.04+ (PEP 668). Not needed to build or use the binaries |

Debian 13 / Ubuntu — one line:

```bash
sudo apt-get update && sudo apt-get install -y golang gcc make git curl
```

`golang` from the distro archive is often **older than the repo's toolchain**: run
`go version` and compare it against the `go` directive in [`go.mod`](go.mod)
(currently `1.26.10`; the repo is developed and tested on that release) before
building. If it is behind, install the toolchain
from https://go.dev/dl/ instead.

## Quick start

> **Note:** This repository is **PUBLIC** and is published to the public Go proxy.
> `go get github.com/wojons/muster@v0.1.1` resolves anonymously on a fresh module
> (proxy.golang.org serves v0.1.0 and v0.1.1). No GOPRIVATE or GitHub credentials are
> required for consumption.
>
> Latest release: v0.1.1 (2026-08-31). A v0.2.0 tag is proposed once the Build and
> Cross-Platform CI workflows are green on the release candidate and an owner go-ahead
> is recorded; see the RELEASE-014 readiness row on the muster board.

```bash
# Build
git clone https://github.com/wojons/muster.git
cd muster
make build-cli

# Start the repo-owned deterministic Task Forge API in another terminal.
# From the repository root, install the pinned Flask dependency once:
python3 -m venv .venv && .venv/bin/pip install -r demo/requirements.txt
PORT=9801 .venv/bin/python3 demo/services/task-forge/server.py

# Point at the deterministic local OpenAPI spec served by Task Forge.
./bin/openapi-cli discover http://localhost:9801/openapi.json

# Turn the spec into typed subcommands (the core "spec → CLI" step).
./bin/openapi-cli generate http://localhost:9801/openapi.json

# Typed commands are now available on the next invocation. This lists the
# repo-owned Task Forge tasks and prints a JSON body (an empty list is valid).
./bin/openapi-cli list-tasks -o json

# Or execute the same deterministic operation with the generic request command.
./bin/openapi-cli request GET http://localhost:9801/tasks -o json

# Point generated commands at another environment without regenerating:
# ./bin/openapi-cli list-tasks --base-url https://staging.example.com -o json
# (or export MUSTER_BASE_URL=https://staging.example.com)
#
# Specs whose servers[].url is RELATIVE (e.g. "/api/v3") resolve against the
# spec fetch URL when generated over http(s). For local-file specs with a
# relative server, pass the base explicitly with --base-url (or
# MUSTER_BASE_URL) — running such a command without one fails with a hint
# naming those two options instead of a bare http error.

# Run the full demo (5 mock services, 21 demo scripts, 6 integration suites, 33 checks total).
# The mock services are Flask apps — install the pinned demo dependencies once
# (a bare system `pip install flask` is refused on Debian 12/13, Ubuntu 23.04+):
./demo/run-all.sh
# No venv yet? `./demo/run-all.sh --bootstrap` creates .venv and installs Flask.
# Flask already in another interpreter? Skip the venv: DEMO_PYTHON=/path/to/python3 ./demo/run-all.sh
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
# The 5 mock services are Flask apps — install the pinned demo dependencies first
# (once; a bare system `pip install flask` is refused on Debian 12/13+, Ubuntu 23.04+):
python3 -m venv .venv && .venv/bin/pip install -r demo/requirements.txt

cd demo && ./run-all.sh
# Builds binaries → starts 5 services → runs 21 demos + 6 integration suites (33 checks) → prints the pass/fail count → cleans up
```

`run-all.sh` probes its interpreter for `flask` **before** it builds anything: with
no `.venv` and no Flask on the system `python3`, it exits 1 naming the venv
command above, instead of building and then reporting a service that failed to
start. Already have Flask in another interpreter? Point the demo at it with
`DEMO_PYTHON=/path/to/python3 ./demo/run-all.sh`; if the repo venv does not exist
yet, `./demo/run-all.sh --bootstrap` creates it and installs Flask.

## Testing

```bash
make test-short           # Quick pass: all tests in -short mode — skips the long-running stability soak
make test-integration     # Integration-tagged tests (-tags=integration -run Integration) — proves components connect
make test-e2e             # Builds the binaries, then runs the end-to-end suite in test/e2e
make validate             # Full validation gate: unit tests + lint + vet + integration tests + security audit (see validate note below)
./demo/run-all.sh         # The real end-to-end gate: builds, starts the 5 demo services, runs 21 demo scripts + 6 integration suites (33 checks total)
DEMO_PYTHON=/path/to/python3 ./demo/run-all.sh   # no venv yet? add --bootstrap to create .venv and install Flask
```

> **Note:** the daemon stability soak is opt-in: files under `test/stability` use
> the `stability` build tag, so bare `go test ./...` does not include that soak and
> is safe for the normal suite. Use `make test-short` when you want the repository's
> quicker short-mode validation.

> **`make validate` note:** the target's checks (unit tests, lint, vet, integration
> tests, security audit) are real gates, but its final stage runs the partial
> integration demo, which is informational only — it currently reports 5 of 10
> scenarios mocked (remediation in progress) and is NOT a green pre-commit gate.
> `make validate` still exits 0 because the demo stage is not scored; use
> `./demo/run-all.sh` for the real end-to-end gate.

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

### Webhook subscriptions

The `openapi-cli webhook` commands manage subscription metadata in the local
config directory. Webhook subscriptions are inventory-only: Muster does not
deliver event notifications for them. Use `webhook test <id>` to check that a
configured URL is reachable.

## MCP Server

**`openapi-mcp` is the canonical (and only) MCP server binary** — built by `make build-mcp` from `cmd/mcp-server` (Makefile `MCP_BINARY := openapi-mcp`); `make build-cli` and `make build-daemon` likewise each write exactly one named binary. The real hazard is `bin/` itself: it is **gitignored** (`.gitignore`), so a stale binary left there by an earlier build is invisible to git and silently survives every rebuild. If a binary behaves unlike the source says it should, delete `bin/` (`make clean`) and rebuild before debugging.

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

23 public packages under `pkg/`. The same packages the CLI uses. Runnable end-to-end example: [docs/integration-guide.md](docs/integration-guide.md#4-go-library-githubcomwojonsmusterpkg).

## Docker

Build the image from the repository root:

```bash
docker build -f Dockerfile -t muster:latest .
```

Run any of the three binaries:

```bash
# CLI (the image's default entrypoint)
docker run --rm muster:latest --help
docker run --rm muster:latest discover https://petstore3.swagger.io/api/v3/openapi.json

# MCP server (override the CLI entrypoint)
docker run --rm --entrypoint /usr/local/bin/openapi-mcp muster:latest --help
docker run --rm -p 8080:8080 --entrypoint /usr/local/bin/openapi-mcp muster:latest --spec https://petstore3.swagger.io/api/v3/openapi.json

# Daemon (override the CLI entrypoint)
docker run --rm --entrypoint /usr/local/bin/openapi-daemon -v /tmp/muster.sock:/tmp/muster.sock muster:latest --help
docker run --rm --entrypoint /usr/local/bin/openapi-daemon -v /tmp/muster.sock:/tmp/muster.sock muster:latest start --socket-path /tmp/muster.sock
```

Or use docker-compose with profiles:

```bash
# Start core dependencies (db, redis)
docker compose up -d

# Start specific services
docker compose --profile daemon up -d
docker compose --profile mcp up -d
docker compose --profile cli up -d

# Full stack (api, daemon, nginx, prometheus, grafana)
docker compose --profile prod up -d
```

## Build from source

> **Note:** This repository is **PUBLIC** — `git clone` and `go get github.com/wojons/muster@v0.1.1` work anonymously; no GOPRIVATE or GitHub credentials needed.
> For local development against an unpublished checkout, use a `replace` directive in your `go.mod`:
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

# Version stamps
# Bare `go build` from a git checkout carries VCS metadata via BuildInfo:
#   openapi-cli version -> v0.1.2-0.20261018093809-b5f8c1e5127f (b5f8c1e5127f)
# It prints "dev (unknown)" only when built outside a git checkout or with
# -buildvcs=false. Stamped builds come from the Makefile:
#   make build          # version/commit/branch/date via -ldflags (see Makefile LDFLAGS)
# `openapi-cli version` always prints a non-empty version. Docker images are
# stamped by their build pipeline (ARG VERSION → -ldflags in the Dockerfile).

# Run tests
go test ./...                 # Normal suite; stability soak is build-tagged
go test -short ./...          # Faster short-mode validation
# (or: make test-short)

# Run the demo
./demo/run-all.sh
```

Requires Go 1.26.10+ (repo developed on 1.26.10), Python 3, and Flask (the demo's services are Flask apps).
Also requires a C compiler for CGO (SQLite) — see [Prerequisites](#prerequisites).

Install the pinned demo dependencies into a repo venv — on Debian 12/13 and Ubuntu
23.04+ a bare system `pip install flask` stops with
`error: externally-managed-environment` (PEP 668):

```bash
python3 -m venv .venv && .venv/bin/pip install -r demo/requirements.txt
./demo/run-all.sh     # picks up .venv/bin/python3 automatically
```

`demo/run-all.sh` checks that its interpreter can `import flask` before it builds
or starts anything. If it cannot, the script exits 1 with that exact command and
the real cause, instead of falling back to a system `python3` and failing later
with a service-start error. Already have Flask elsewhere? Set
`DEMO_PYTHON=/path/to/python3` when running the demo. If the repo venv is missing,
`./demo/run-all.sh --bootstrap` creates it and installs Flask automatically;
otherwise use the pinned `demo/requirements.txt` command above. Alternatives to
the venv are `pipx install flask` or
`uv pip install --python .venv/bin/python3 -r demo/requirements.txt`.

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
pkg/                 # 23 public packages (importable)
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
  ...                # 6 more package directories
internal/            # Private utilities
demo/                # Self-contained demo (5 services, 14 scripts)
specs/               # 228 specification files
```

## Limits / known gaps

- **No v0.2.0 release assets yet** — install v0.1.1 via `go get github.com/wojons/muster@v0.1.1` or clone + build.
- **Demo requires Python 3 + Flask in a venv** — `demo/requirements.txt` pins Flask; on Debian 12/13 and Ubuntu 23.04+ a bare `pip install flask` is refused (PEP 668). Use `python3 -m venv .venv && .venv/bin/pip install -r demo/requirements.txt` or `./demo/run-all.sh --bootstrap`.
- **Seccomp hardening is opt-in** — default builds are dependency-free; the hardened path needs `-tags seccomp` plus `pkg-config` + `libseccomp-dev` (see [Build from source](#build-from-source)).
- **`bin/` is gitignored** — stale binaries survive rebuilds. Run `make clean` before debugging unexpected binary behavior.
- **Public petstore3 demo server is unreliable** — some endpoints return HTTP 500 server-side (measured 2026-10-18: `/store/inventory`); use the local mock services (`./demo/run-all.sh`) for deterministic requests.

## License

MIT — see [LICENSE](LICENSE)
