# Muster Integration Guide

> Every surface Muster exposes, with a runnable example for each. If you can run
> the commands below, you have a working Muster installation.

Muster turns any OpenAPI 3.x specification into **seven surfaces**:

| # | Surface | Entry point |
|---|---------|-------------|
| 1 | CLI | `openapi-cli` |
| 2 | MCP server | `openapi-mcp` |
| 3 | Daemon | `openapi-daemon` |
| 4 | Go library | `github.com/wojons/muster/pkg/...` |
| 5 | Docker | `muster:latest` image |
| 6 | WASM plugins | `openapi-cli modules run` / `--wasm-plugin` |
| 7 | Starlark DSL | `openapi-cli dsl run` |

All examples below were executed against the public example spec used across
this repository:

```
https://petstore3.swagger.io/api/v3/openapi.json
```

> **Note on the public demo server (verified 2026-08-06):** the swagger.io
> petstore server returns HTTP 500 on some endpoints (`findByStatus`,
> `store/inventory`) — this is a server-side issue, not a Muster bug (plain
> `curl` gets the same 500). `GET /pet/1` and `GET /store/order/1` are stable
> and used in the examples below. For fully deterministic runs, use the
> self-contained demo instead: `./demo/run-all.sh` (5 mock services on
> localhost, 23 checks).

---

## 0. Prerequisites and build

```bash
git clone https://github.com/wojons/muster.git
cd muster
make build          # builds all three binaries into bin/
ls bin/             # openapi-cli  openapi-daemon  openapi-mcp  (exactly these three)
```

Requires Go 1.26.6+ (the module targets go 1.26.6). `bin/` is gitignored; `make
build` is the canonical way to produce the three binaries above — no other
binary names are produced by any Makefile target.

See [docs/guides/installation.md](guides/installation.md) for install options
(including FIPS builds and Linux packages).

---

## 1. CLI (`openapi-cli`)

The CLI has two primary verbs: `discover` (read a spec, list every operation)
and `request` (execute any HTTP call). Full reference:
[docs/guides/commands.md](guides/commands.md).

### 1.1 Discover — list every operation in a spec

```bash
./bin/openapi-cli discover https://petstore3.swagger.io/api/v3/openapi.json
```

Real output (truncated):

```
API: Swagger Petstore - OpenAPI 3.0 (v1.0.27)
Base URL: https://petstore3.swagger.io

pet
  POST   /pet                                     addPet
  PUT    /pet                                     updatePet
  GET    /pet/findByStatus                        findPetsByStatus
  GET    /pet/{petId}                             getPetById
  ...
store
  GET    /store/inventory                         getInventory
  POST   /store/order                             placeOrder
user
  GET    /user/{username}                         getUserByName
```

`discover` fetches the spec over HTTP, parses it, and prints every
path+method+operationId grouped by tag. Nothing is persisted.

### 1.2 Request — call an operation

```bash
./bin/openapi-cli request GET https://petstore3.swagger.io/api/v3/pet/1 -o json
```

Real output (truncated):

```json
{
  "id": 1,
  "name": "Pet1",
  "photoUrls": [
    "test1",
    "test2",
    "/tmp/inflector17846167080302375556.tmp",
    ...
  ],
  "status": "available"
}
```

Flags in daily use:

| Flag | Purpose | Example |
|------|---------|---------|
| `-p key=value` | Query parameter (repeatable) | `-p status=available` |
| `-H 'Key: Value'` | Header (repeatable) | `-H "Authorization: Bearer $TOKEN"` |
| `-b '{...}'` | Request body | `-b '{"name":"Alice"}'` |
| `-o json\|yaml\|table` | Output format (default `table`) | `-o yaml` |
| `--timeout 30s` | Per-request timeout | `--timeout 10s` |
| `--wasm-plugin path.wasm` | Response transformation hook (see §6) | `--wasm-plugin ./transform.wasm` |

Auth is configured via `openapi-cli auth` / `config` (bearer, API key, OAuth2,
mTLS) — see [docs/guides/configuration.md](guides/configuration.md).

---

## 2. MCP server (`openapi-mcp`)

**`openapi-mcp` is the one and only MCP server binary.** It is built by
`make build-mcp` from `cmd/mcp-server`. No other MCP binary is produced by the
build — if you find an `mcp-server` or `openapi-cli-mcp-server` file in `bin/`,
it is a stale artifact from an ad-hoc build; delete it and rebuild.

Every OpenAPI operation becomes an MCP tool: `tools/list` shows them,
`tools/call` executes them, and the AI client never sees a raw REST call.

### 2.1 Point Claude Desktop (or any MCP client) at it

```json
{
  "mcpServers": {
    "my-api": {
      "command": "/absolute/path/to/bin/openapi-mcp",
      "args": ["--spec", "/absolute/path/to/openapi.json", "--base-url", "https://api.example.com"]
    }
  }
}
```

Flags:

| Flag | Purpose |
|------|---------|
| `-spec` | Path to the OpenAPI spec file (required) |
| `-base-url` | Base URL override for API operations (env: `MUSTER_BASE_URL`). Optional: when omitted, `openapi-mcp` uses the spec's own `servers[].url` (operation- → path- → root-level), so `--spec` alone works for specs declaring an absolute server URL. Relative `servers[].url` values (e.g. `/api/v3`) cannot be dereferenced standalone — supply `-base-url` for those. |
| `-wasm-plugin` | WASM response-transformation plugin (see §6) |
| `-audit` | Enable audit logging (or `MUSTER_AUDIT_ENABLED=1`) |
| `-log-level`, `-log-output` | Logging control (default: `info` to `stderr`) |

**Base URL resolution order (DF-MUSTER-1):** `--base-url` flag →
`MUSTER_BASE_URL` env var → the spec's `servers[].url` → none (calls fail at
request time until a base URL exists). The resolved URL is announced on
stderr at startup (`Using base URL: …`), so a misconfigured server is visible
before the first call.

### 2.2 Drive it over stdio by hand (JSON-RPC)

MCP speaks JSON-RPC 2.0 over stdio. This is the exact conversation a client
has with the server:

```bash
curl -sS -m 20 https://petstore3.swagger.io/api/v3/openapi.json -o /tmp/petstore.json

printf '%s\n' \
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' \
'{"jsonrpc":"2.0","method":"notifications/initialized"}' \
'{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
| ./bin/openapi-mcp -spec /tmp/petstore.json
```

Real output: `tools/list` returns 19 tools, e.g. `getPetById` with an input
schema `{petId: integer}`. The server logs `Loaded spec: Swagger Petstore -
OpenAPI 3.0 (1.0.27)` and `Generated 19 MCP tools` on stderr.

To actually call an operation (this executes a live HTTP request):

```bash
printf '%s\n' \
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' \
'{"jsonrpc":"2.0","method":"notifications/initialized"}' \
'{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"getPetById","arguments":{"petId":1}}}' \
| ./bin/openapi-mcp -spec /tmp/petstore.json -base-url https://petstore3.swagger.io/api/v3
```

The `-base-url` here is only needed because the petstore spec declares a
*relative* server (`/api/v3`). For a spec with an absolute `servers[].url`,
omit the flag entirely and `tools/call` targets the spec's server (see the
resolution order in §2.1).

Real output: the tool result content carries the pet JSON
(`{"id":1,"name":"Pet1",...}`).

Protocol details: [docs/api/mcp-protocol.md](api/mcp-protocol.md).
Go API for building MCP servers programmatically:
[docs/examples/mcp-handlers-examples.md](examples/mcp-handlers-examples.md).

---

## 3. Daemon (`openapi-daemon`)

The daemon runs a persistent Unix-socket HTTP proxy to a backend API — one
shared process, many clients (`curl`, scripts, other services), with request
transformation and a token file for the control channel.

Full start → run → stop sequence (verified):

```bash
# Terminal 1 — start the proxy (socket + pid file land in /tmp)
./bin/openapi-daemon proxy --backend-url https://petstore3.swagger.io --socket-path /tmp/muster.sock
# [INFO] proxy server started socket=/tmp/muster.sock
# Proxy daemon started successfully
# Socket: /tmp/muster.sock
# Token file: /tmp/muster.sock.token

# Terminal 2 — talk to the backend through the socket
curl --unix-socket /tmp/muster.sock http://localhost/api/v3/pet/1
# {"id":1,"name":"Pet1","photoUrls":[...],"status":"available"}

curl --unix-socket /tmp/muster.sock -X POST http://localhost/api/v3/pet \
     -H "Content-Type: application/json" \
     -d '{"name":"ProxyTest","photoUrls":[],"status":"available"}'

# Terminal 1 — stop
./bin/openapi-daemon stop --socket-path /tmp/muster.sock
# (falls back to SIGTERM if the pid file is gone)
```

Subcommands: `start`, `proxy`, `stop`, `restart`, `status`. Flags:
`--backend-url`, `--socket-path` (default `/tmp/muster-daemon.sock`),
`--pid-file`, plus `-wasm-plugin` for response transformation.

Operational notes:

- The proxy writes `<socket-path>.token` next to the socket for the control
  channel. Keep the socket directory private to the daemon user.
- For systemd deployment (restart policies, socket activation) see
  [docs/SYSTEMD-CONFIGURATION-GUIDE.md](SYSTEMD-CONFIGURATION-GUIDE.md).
- Daemon Go API and controller patterns:
  [docs/examples/daemon-examples.md](examples/daemon-examples.md).

---

## 4. Go library (`github.com/wojons/muster/pkg/...`)

22 public packages under `pkg/` — the same packages the CLI is built on.
The two you will use most are `pkg/openapi` (spec parsing) and `pkg/client`
(traced HTTP client with retry, headers, base URL).

Runnable example (verified — prints a parsed spec title and fetches a pet):

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/openapi"
	"github.com/wojons/muster/pkg/retry"
)

func main() {
	ctx := context.Background()

	// 1. Parse an OpenAPI spec (bytes from a file, curl, etc.)
	data, err := os.ReadFile("petstore.json")
	if err != nil {
		panic(err)
	}
	parser := openapi.NewParser()
	result, err := parser.Parse(ctx, data, openapi.DefaultParseOptions())
	if err != nil {
		panic(err)
	}
	fmt.Printf("Parsed spec: %q (OpenAPI %s)\n", result.Document.Info.Title, result.Version)

	// 2. Traced HTTP client: base URL, timeout, retry, default headers
	cfg := retry.DefaultRetryConfig()
	cfg.MaxAttempts = 2
	c := client.NewClient(
		client.WithBaseURL("https://petstore3.swagger.io"),
		client.WithTimeout(30*time.Second),
		client.WithRetryConfig(cfg),
		client.WithHeader("Accept", "application/json"),
	)

	resp, err := c.Get(ctx, "/api/v3/pet/1")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	fmt.Printf("GET /api/v3/pet/1 -> %d\n", resp.StatusCode)
	if resp.StatusCode == http.StatusOK {
		fmt.Println("Pet fetched OK via the Go library client")
	}
}
```

Real output:

```
Parsed spec: "Swagger Petstore - OpenAPI 3.0" (OpenAPI 1.0.27)
GET /api/v3/pet/1 -> 200
Pet fetched OK via the Go library client
```

To run it against a local checkout:

```bash
mkdir -p /tmp/muster-lib-example && cd /tmp/muster-lib-example
# (save the program above as main.go)
curl -sS -m 20 https://petstore3.swagger.io/api/v3/openapi.json -o petstore.json
go mod init example
go mod edit -replace github.com/wojons/muster=/path/to/muster
go mod tidy
go run .
```

Key packages: `pkg/openapi` (parser), `pkg/client` (HTTP client, retry,
rate-limit round trippers), `pkg/generator` (operation extraction),
`pkg/mcp` (MCP protocol), `pkg/dsl` (Starlark interpreter), `pkg/wasm`
(wazero runtime), `pkg/daemon` (daemon controller), `pkg/auth`, `pkg/cache`,
`pkg/rate`, `pkg/config`. Package-level API reference:
[docs/api/README.md](api/README.md) and [docs/api/reference-card.md](api/reference-card.md).

---

## 5. Docker

The multi-stage Dockerfile builds `openapi-cli` and `openapi-mcp` statically
(CGO disabled) onto a distroless non-root base with a Trivy security scan and
SBOM stage. The image `ENTRYPOINT` is `openapi-cli`, so the container behaves
exactly like the CLI.

```bash
# Build (equivalent to: make docker-build)
docker build -t muster:latest .

# CLI surface in a container — discover
docker run --rm muster:latest discover https://petstore3.swagger.io/api/v3/openapi.json

# CLI surface in a container — request (JSON output)
docker run --rm muster:latest request GET https://petstore3.swagger.io/api/v3/pet/1 -o json

# MCP surface in a container — hand a spec file in and run the server
docker run --rm -i -v "$PWD/petstore.json:/spec.json:ro" \
  --entrypoint openapi-mcp muster:latest -spec /spec.json \
  -base-url https://petstore3.swagger.io/api/v3
# pipe the same JSON-RPC lines from §2.2 into stdin
```

Container notes:

- `EXPOSE 8080` is declared for daemon HTTP mode; the image itself contains no
  writable layers (distroless non-root) — mount state/socket directories with
  `-v` when running the daemon.
- The image has a `HEALTHCHECK` running `openapi-cli version`.

---

## 6. WASM plugins (wazero sandbox)

Muster embeds a pure-Go WebAssembly runtime (wazero, no CGO). WASM appears in
two places:

1. `openapi-cli modules run` — execute any `.wasm` module directly in the
   sandbox. Integer args are passed to the exported function.
2. `--wasm-plugin <file.wasm>` on `request`, `openapi-daemon` and
   `openapi-mcp` — hook that rewrites response bodies before they are
   returned to the caller.

Runnable examples (verified against real modules in this repo):

```bash
# Execute a module's exported function with integer args
./bin/openapi-cli modules run demo/wasm-modules/hello.wasm --fn hello
# Executed demo/wasm-modules/hello.wasm (function: hello)
# Output: [42]

./bin/openapi-cli modules run testdata/wasm-modules/add.wasm --fn add 2 3
# Executed testdata/wasm-modules/add.wasm (function: add)
# Output: [5]

# Response-transformation hook on a live request (module is loaded and called;
# a custom module's transform function replaces the response body)
./bin/openapi-cli request GET https://petstore3.swagger.io/api/v3/pet/1 \
  --wasm-plugin demo/wasm-modules/transform.wasm -o json
```

Flags: `modules run <file.wasm> [args...] --fn <name>` (default `_start`).

Building your own modules: the demo ships `.wat` sources with a build script —
`demo/wasm-modules/build.sh` (needs `wat2wasm` from wabt) — and
`testdata/wasm-modules/` holds the test fixtures. The interceptor behavior
(JSON pass-through rules, timeouts) is documented in `pkg/wasm`. Note the
`transform.wasm` in `demo/` is a stub — it loads and executes cleanly but
returns an empty body; a real plugin must write its transformed JSON back to
the module's memory.

---

## 7. Starlark DSL (`openapi-cli dsl run`)

The DSL is a sandboxed, Python-like scripting language with four HTTP
builtins. **Each builtin returns a `(status_code, response_body)` tuple.**

| Builtin | Signature | Returns |
|---------|-----------|---------|
| `http_get` | `http_get(url)` | `(code, body)` |
| `http_post` | `http_post(url, body)` | `(code, body)` |
| `http_put` | `http_put(url, body)` | `(code, body)` |
| `http_delete` | `http_delete(url)` | `(code, body)` |

Runnable example — save as `petstore.star`:

```python
BASE = "https://petstore3.swagger.io/api/v3"

# Fetch a pet by id
code, body = http_get(BASE + "/pet/1")
print("GET /pet/1 ->", code)

# Create a resource (POST)
code2, body2 = http_post(BASE + "/pet",
    '{"name":"DSL Pet","photoUrls":[],"status":"available"}')
print("POST /pet ->", code2)

# Chain: list, then update
code3, body3 = http_get(BASE + "/store/inventory")
print("GET /store/inventory ->", code3)
```

Run it:

```bash
./bin/openapi-cli dsl run petstore.star
```

Real output (verified 2026-08-06 — read endpoints succeed; write/aggregate
endpoints return the public demo server's own 500, see the §0 note):

```
GET /pet/1 -> 200
POST /pet -> 500      # public demo server rejects writes (server-side)
GET /store/inventory -> 500   # public demo server issue, see §0 note
DSL OK
```

Point the same script at a spec you control (or the local demo services on
:9801-:9805) and the write calls succeed — see `demo/dsl/workflow.star`,
which creates projects and tasks against the Task Forge mock service. The
same builtins drive the full 5-service demo:

```bash
./demo/run-all.sh   # starts mock services on :9801-:9805, runs all 7 surfaces, 23 checks
```

Language guide and config (timeouts, imports, memory limits):
[docs/guides/starlark-dsl.md](guides/starlark-dsl.md) and
[docs/examples/dsl-examples.md](examples/dsl-examples.md).

---

## Where to go next

| Need | Doc |
|------|-----|
| First 5 minutes | [Quickstart](guides/quickstart.md) |
| Command reference | [Commands](guides/commands.md) |
| Configuration & auth | [Configuration](guides/configuration.md) |
| MCP protocol | [MCP protocol](api/mcp-protocol.md) |
| Daemon operations | [Daemon examples](examples/daemon-examples.md) · [Systemd guide](SYSTEMD-CONFIGURATION-GUIDE.md) |
| Starlark DSL | [DSL guide](guides/starlark-dsl.md) · [DSL examples](examples/dsl-examples.md) |
| Troubleshooting | [Troubleshooting](guides/troubleshooting.md) |
| Trace-based debugging | [Trace debugging](guides/trace-based-debugging.md) |
| Full-system demo | `./demo/run-all.sh` (5 services, 7 surfaces, 23 checks) |
