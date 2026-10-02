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

Requires Go 1.26.6+ (the module targets go 1.26.6). A C compiler
(`gcc`/`clang`) is also required — SQLite support is a CGO package and
`make build` runs with `CGO_ENABLED=1` (see
[docs/guides/installation.md](guides/installation.md#prerequisites)). `bin/` is
gitignored; `make
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
build — if you find an `mcp-server` binary in `bin/`, it is a stale artifact
from an ad-hoc build; delete it and rebuild.

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
transformation.

Full start → run → stop sequence (verified):

```bash
# Terminal 1 — start the proxy (socket + pid file land in /tmp)
./bin/openapi-daemon proxy --backend-url https://petstore3.swagger.io --socket-path /tmp/muster.sock
# [INFO] proxy server started socket=/tmp/muster.sock
# Proxy daemon started successfully
# Socket: /tmp/muster.sock

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

- The socket is the only control channel, and the daemon writes no token file:
  `pkg/daemon`'s token middleware is not installed by the shipped CLI, and the
  `Token file:` hint was removed from the banners (DF-047). Keep the socket
  directory private to the daemon user — the socket is created `0600`.
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
(JSON pass-through rules, timeouts) is documented in `pkg/wasm`.

A plugin is executed as a WASI **command** (`pkg/wasm/module.go`,
`runWASMModule`): the response body is written to the module's stdin and
whatever the module writes to **stdout** replaces the response body.
`demo/wasm-modules/transform.wat` is a real command filter — it reads stdin to
EOF and wraps the body in a deterministic JSON envelope, so the hook rewrites a
live response:

```bash
./bin/openapi-cli request GET http://localhost:9801/tasks \
  --wasm-plugin demo/wasm-modules/transform.wasm
# {"wasm":true,"payload":[]}
```

A module that exports only a helper function and defines no `_start` is
instantiated successfully but executes nothing, so the interceptor replaces the
body with an EMPTY one and still exits 0 — the exact failure
`demo/demos/09-wasm-transform.sh` now fails on. Empty stdin deliberately
produces empty stdout, and an empty response body is passed through untouched
before the module is called.

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

## 8. Bring your own live API — the proven recipe

This is the end-to-end recipe for pointing Muster at **any OpenAPI 3.x
service you control** — a served `openapi.json`, a bundled spec file, or
both. Every command below was executed against a live bearer-auth HTTP
server on 2026-09-27 (HEAD `8efdb446`, CLI v0.1.1-223-g8efdb446) from a
clean `HOME`. Substitute your own host, spec path, and credential — no
example secret here is real.

### 8.1 Discovery: served URL or bundled spec file

`discover` reads an OpenAPI 3.x spec from a local file path or an
http/https URL and lists every operation, grouped by tag. Nothing is
persisted.

```bash
./bin/openapi-cli discover https://api.example.com/openapi.json
./bin/openapi-cli discover /path/to/bundled.yaml
```

Both forms were verified against a live server serving its spec at
`/openapi.json` **and** against the same document saved to a local file:
identical operation listing. Use whichever your service provides — no
separate "bundled spec" flag or command exists.

### 8.2 Generate typed commands and pin the base URL

```bash
./bin/openapi-cli generate https://api.example.com/openapi.json \
    --base-url https://api.example.com
# ✓ Success! Commands registered and persisted
```

- The spec's `servers[].url` is used when you omit `--base-url`; if the
  spec declares a **relative** server URL (e.g. `/api/v3`), you must pass
  `--base-url` (or set `MUSTER_BASE_URL`) or every call fails with the
  actionable `unsupported protocol scheme ""` hint (DF-015).
- Generating from a local file without an absolute `servers[].url` also
  needs `--base-url`.
- `--base-url` is **sticky per invocation, overridable per call**: the
  flag wins, then the `MUSTER_BASE_URL` env var, then the stored
  generate-time URL.
- Inspect or reset the persisted store with `generate --list` and
  `generate --clear` (or just `--list` / `--clear` with no spec).

### 8.3 Store a credential once

```bash
./bin/openapi-cli auth add --name myapi --type bearer
# Secret value: (prompted if --value is omitted)
# Stored credential "myapi" (type: bearer)
```

- Types: `api-key`, `bearer`, `basic` (`auth add --help`).
- The secret lives in the OS keychain; only metadata is kept in config.
  `auth list` shows names and types, never values; `auth remove <name>`
  deletes one.
- Wire format at request time: `bearer` → `Authorization: Bearer …`,
  `api-key` → `X-API-Key: …`, `basic` → `Authorization: Basic …`.

### 8.4 Attach auth per request

Stored credentials attach with `--auth NAME` — on generated typed
commands (DF-023) and on `request` alike:

```bash
./bin/openapi-cli list-things --auth myapi                    # 200
./bin/openapi-cli get-thing t1 --auth myapi                   # 200
./bin/openapi-cli request GET https://api.example.com/things --auth myapi
```

Precedence on generated commands and on `request`: an explicit `-H
'Authorization: …'` header **wins over** the stored credential.

### 8.5 Environment-variable path (script-friendly)

| Env var | Effect |
|---------|--------|
| `MUSTER_BASE_URL` | Substitute for `--base-url` on generated commands, `request`, and the MCP server's `--base-url` |
| `MUSTER_AUTH_TOKEN` | Attached as `Authorization: Bearer <token>` on generated (persisted) commands when `--auth` is not used |
| `MUSTER_PROFILE` | Selects a config profile (the `--profile` flag overrides it) |
| `MUSTER_HTTP_TIMEOUT` | Per-request timeout for generated calls, in seconds (default 30) |

```bash
export MUSTER_BASE_URL=https://api.example.com
export MUSTER_AUTH_TOKEN=<your-token>
./bin/openapi-cli list-things      # no flags needed — 200
```

The error paths behave predictably: a request with no credential gets the
upstream `401` as a typed `unauthenticated` error (exit 1); an unknown
resource gets `404 NOT_FOUND`.

### 8.6 Caveats: what changed since the first proof (2026-10-02)

The 2026-09-27 proof listed four open gaps (DF-036/037/038/039). All four
have since landed, and the consensus harness run (§8.7) verified each fix
live on 2026-10-02 (HEAD `2176a340`, CLI v0.1.1-353-g2176a340):

- **MCP auth (DF-036, fixed):** `openapi-mcp` now honors
  `MUSTER_AUTH_TOKEN` (and `-auth-token-file`) and attaches it as
  `Authorization: Bearer <token>` on every tools/call. Verified: a
  bearer-protected `getMetrics` call returned `200` JSON through MCP.
- **Starlark headers (DF-037, fixed):** `http_get/post/put/delete` accept
  an optional headers dict — `http_get(url, headers)` /
  `http_post(url, body, headers)`. Verified: an authed DSL script drove
  `GET /health` (200), `GET /sessions` (200) and `POST /sessions` (201)
  against a bearer-protected server. Note: the dialect has no `while`
  loops and globals are single-assignment — put logic in a `def main()`.
- **MCP error structure (DF-038, fixed):** tool errors come back as
  structured content with `isError: true` instead of undifferentiated
  flat text.
- **Credential echo (DF-039, fixed):** `create-session --output json`
  masks the session-scoped key (`cs_sk_***`).

Still true: MCP tool names are camelCase derived from operationIds
(`listSessions`, `getMetrics` — not snake_case), and tool results are
flat JSON text inside `content[0].text`. Review table output before
sharing it.

### 8.7 Worked example: the full chain against a live harness (consensus, 2026-10-02)

This is the proof run behind §8.6. The harness was a real agent-runtime
API (the consensus server) serving its own OpenAPI 3.1 document at
`/openapi.json`, with bearer auth on everything except `/api/v1/health`.
Every command below was executed and returned the status shown — the
same shape works for any OpenAPI 3.x service you control.

```bash
# 0. Serve the harness (any OpenAPI 3.x server works; consensus shown)
consensus serve --port 9814 --db-url sqlite:///tmp/consensus.db
# Boot prints a first-admin key (cs_ak_...). Save it — it is stored hashed.
# If session creation later returns MISSING_LLM_CONFIG, restart with an
# LLM key exported (any value; muster's chain never invokes the LLM):
#   DEEPSEEK_API_KEY=<value> consensus serve --port 9814 --db-url sqlite:///tmp/consensus.db

# 1. Discover — read-only operation listing from the served spec
./bin/openapi-cli discover http://127.0.0.1:9814/openapi.json
# → operations grouped by tag (sessions, tasks, memory, auth, ...), exit 0

# 2. Generate typed commands, pinned to the live base URL
./bin/openapi-cli generate http://127.0.0.1:9814/openapi.json \
    -u http://127.0.0.1:9814
# ✓ Success! Commands registered and persisted (78 commands from 66 paths)
./bin/openapi-cli generate --list          # spec URL + hash + count

# 3. Store the credential once
./bin/openapi-cli auth add --name harness --type bearer --value <cs_ak_...>

# 4. Call through the CLI (typed generated commands)
./bin/openapi-cli health-check --base-url http://127.0.0.1:9814   # 200, no auth
./bin/openapi-cli list-sessions --base-url http://127.0.0.1:9814 --auth harness   # 200
./bin/openapi-cli get-metrics   --base-url http://127.0.0.1:9814 --auth harness   # 200

# 5. Full write path via CLI: create session → task → message
./bin/openapi-cli create-session --agent_name muster-e2e \
    --goal "muster integration proof" \
    --base-url http://127.0.0.1:9814 --auth harness --output json
# → 201 {"id": "...", "status": "booting", "api_key": "cs_sk_***"}
SID=<id from step 5>
./bin/openapi-cli create-task $SID --title integration-proof \
    --description "task created via muster CLI" \
    --base-url http://127.0.0.1:9814 --auth harness --output json   # 200
./bin/openapi-cli send-message $SID --content "proof message" \
    --base-url http://127.0.0.1:9814 --auth harness --output json
# → {"session": "...", "status": "message_received"}

# 6. MCP leg: 78 tools over stdio, bearer auth, live tool calls
curl -s http://127.0.0.1:9814/openapi.json -o /tmp/harness.json
printf '%s\n' \
 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"m","version":"1"}}}' \
 '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"listSessions","arguments":{}}}' \
| MUSTER_AUTH_TOKEN=<cs_ak_...> ./bin/openapi-mcp \
    -spec /tmp/harness.json -base-url http://127.0.0.1:9814 -log-level error
# → tools/list: 78 tools; tools/call listSessions: 200 JSON of the sessions
#   created in steps 5 AND 7 (cross-leg proof through one server)

# 7. Starlark DSL leg: authed workflow in a script
cat > /tmp/proof.star <<'EOF'
BASE = "http://127.0.0.1:9814"
AUTH = {"Authorization": "Bearer <cs_ak_...>"}
def main():
    s1, _ = http_get(BASE + "/api/v1/health")
    s2, _ = http_get(BASE + "/api/v1/sessions", AUTH)
    s3, b3 = http_post(BASE + "/api/v1/sessions",
        '{"agent_name":"muster-dsl","goal":"dsl leg"}', AUTH)
    print(s1, s2, s3)   # 200 200 201
main()
EOF
./bin/openapi-cli dsl run /tmp/proof.star
```

Cross-leg proof: the session created through the CLI (step 5) and the one
created through the DSL (step 7) both appear in the MCP `listSessions`
result (step 6) — three independent surfaces, one live server, one
spec.

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
