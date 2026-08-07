# Muster Dogfood Integration Report — 2026-08-07

> What a real user actually experiences using Muster, surface by surface. Every
> command below was executed during the dogfood run and produced the quoted
> output. This is the "did the docs lie?" record — see
> [diagnostics.md](diagnostics.md) for how it works under the hood and the
> [skills/muster-usage](../../skills/muster-usage/SKILL.md) skill for the
> agent-facing quick reference.

**Verdict: 🟡 PROMISING-BUT-ROUGH.** The core promise holds — 6 of 7 surfaces
deliver real value with the documented commands. The daemon lifecycle
(start/status/stop) is broken (P1), and the guides document commands that
don't exist (P2). Fix those and this is shippable.

**Time-to-first-success: ~6 minutes** (fresh build + discover + first request).
**Friction count: 7** (see Findings).

---

## Setup that works (all verified 2026-08-07)

```bash
cd muster
make build                      # ~1-2 min; builds bin/openapi-cli, bin/openapi-mcp, bin/openapi-daemon
./bin/openapi-cli version       # v0.1.0-8-g0d790342 (0d790342) — version injection works post-GAP-007
```

Requires Go 1.23+ (module targets 1.26). `bin/` is gitignored — always
`make build` on a fresh clone.

For deterministic offline use, start a demo service (they're real Flask APIs
with OpenAPI specs — perfect backends for trying Muster):

```bash
PORT=9801 .venv/bin/python3 demo/services/task-forge/server.py &
curl localhost:9801/health     # {"projects_count":0,"status":"healthy"}
```

## 1. CLI — WORKS (the star surface)

```bash
./bin/openapi-cli discover http://localhost:9801/openapi.json
# API: Task Forge API (v1.0.0) / Base URL: http://localhost:9801 / 22 operations grouped by tag

./bin/openapi-cli request GET http://localhost:9801/projects -o json          # []
./bin/openapi-cli request POST http://localhost:9801/projects \
  -b '{"name":"Dogfood Project","description":"created via openapi-cli","tags":["dogfood"]}' -o json
# 201 → {"id":"1","name":"Dogfood Project",...}
./bin/openapi-cli request GET http://localhost:9801/projects/1 -o json        # read back
./bin/openapi-cli request PATCH http://localhost:9801/projects/1 -b '{"name":"Renamed"}' -o json
./bin/openapi-cli request DELETE http://localhost:9801/tasks/1 -o json
```

Flags (all verified): `-b` body, `-H 'Key: Value'` header, `-o json|yaml|table`,
`-p key=value` query param, `--timeout`, `--wasm-plugin`, `-v` verbose.
README quickstart commands work verbatim against the public petstore too:

```bash
./bin/openapi-cli discover https://petstore3.swagger.io/api/v3/openapi.json
./bin/openapi-cli request GET https://petstore3.swagger.io/api/v3/pet/1 -o json
```

Error path: 404 prints server body + `Error: request failed with status 404`
(exit 1) — but then dumps the full flag usage block (noisy; see DF-006).

## 2. MCP server — WORKS (the AI-agent surface)

Every spec operation becomes an MCP tool. Drive it over stdio (this is the
exact conversation a client has):

```bash
curl -s http://localhost:9801/openapi.json -o /tmp/taskforge.json
printf '%s\n' \
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}' \
'{"jsonrpc":"2.0","method":"notifications/initialized"}' \
'{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
| ./bin/openapi-mcp -spec /tmp/taskforge.json -base-url http://localhost:9801
# initialize → serverInfo: {"name":"openapi-mcp","version":"1.0.0"}   (GAP-010 verified)
# tools/list → 22 tools: get_health, get_projects, post_projects, get_tasks_{task_id}_comments, ...

# Create a resource via tools/call (the "agent does a real task" path):
printf '%s\n' \
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}' \
'{"jsonrpc":"2.0","method":"notifications/initialized"}' \
'{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"post_projects","arguments":{"name":"MCP-Created","description":"by agent"}}}' \
| ./bin/openapi-mcp -spec /tmp/taskforge.json -base-url http://localhost:9801
# → {"id":"3","name":"MCP-Created",...}
```

**Pitfall (documented in the guide, confirmed):** `-base-url` is REQUIRED when
the spec declares a relative `servers[].url` (petstore declares `/api/v3`) —
without it calls fail with `unsupported protocol scheme ""`.

**Pitfall found:** pipelining multiple requests in one stdin burst can return
responses out of order (server processes them concurrently). Real MCP clients
send one request, await the response, then continue — do the same in scripts.

## 3. Daemon — PARTIALLY WORKS (lifecycle broken → DF-001)

Proxy mode works:

```bash
./bin/openapi-daemon proxy --backend-url http://localhost:9801 --socket-path /tmp/muster.sock &
curl --unix-socket /tmp/muster.sock http://localhost/projects       # JSON list
curl --unix-socket /tmp/muster.sock -X POST http://localhost/projects \
  -H "Content-Type: application/json" -d '{"name":"Proxy-Created"}' # 201
```

**But the documented lifecycle lies:** after `start` (or `proxy`), `status`
reports `Running: false` with zero start time while the daemon actively
serves; `stop` prints `Daemon is not running` and the daemon KEEPS serving.
No PID/token file is ever written despite `start` printing their paths.
**Workaround:** kill the process directly (`pkill -f openapi-daemon`). Do not
run `status`/`stop` expecting them to work — they don't until DF-001 is fixed.

## 4. Go library — WORKS (verified with a fresh consumer)

The most valuable surface. Consumer pattern (guide §4, verified):

```bash
mkdir -p /tmp/mylib && cd /tmp/mylib
go mod init mylib
go mod edit -replace github.com/wojons/muster=/path/to/muster   # repo is PRIVATE — see DF-004
go mod tidy && go build -o tool . && ./tool
```

Working API surface (verified by compiling against it):

```go
parser := openapi.NewParser()
result, err := parser.Parse(ctx, specBytes, openapi.DefaultParseOptions())  // result.Document, result.Version

gen, _ := generator.NewGenerator(generator.Config{AppName: "x", BaseURL: "http://localhost:9801"})
cmds, err := gen.GenerateCommands(result.Document)                          // []*cobra.Command

c := client.NewClient(client.WithBaseURL(base), client.WithTimeout(15*time.Second),
    client.WithRetryConfig(retry.DefaultRetryConfig()), client.WithHeader("Accept", "application/json"))
resp, err := c.Get(ctx, "/projects")
resp, err = c.Post(ctx, "/projects", "application/json", bytes.NewReader(payload))
```

Real output from the dogfood consumer: parsed Task Forge spec, generated 6
command groups (`projects`, `tasks`, `comments`, `milestones`, `tags`, …),
`GET /projects → 200`, `POST /projects → 201`.

**Pitfall:** the integration guide lists `pkg/generator` as "operation
extraction" — there is no `generator.ExtractOperations`; the real API is
`NewGenerator(Config)` + `GenerateCommands(doc)`. (Rolled into DF-002.)

## 5. Docker — WORKS

```bash
docker build -t muster:dogfood .        # multi-stage; ~2-3 min
docker run --rm muster:dogfood discover https://petstore3.swagger.io/api/v3/openapi.json
docker run --rm muster:dogfood request GET https://petstore3.swagger.io/api/v3/pet/1 -o json
# MCP in a container:
printf '%s\n' '<initialize>' '<notifications/initialized>' '<tools/list>' | \
docker run --rm -i -v "$PWD/petstore.json:/spec.json:ro" --entrypoint openapi-mcp \
  muster:dogfood -spec /spec.json     # → serverInfo openapi-mcp, 19 tools
```

## 6. WASM plugins — WORKS

```bash
./bin/openapi-cli modules run demo/wasm-modules/hello.wasm --fn hello   # Output: [42]
./bin/openapi-cli modules run testdata/wasm-modules/add.wasm --fn add 2 3  # Output: [5]
```

`--wasm-plugin` on request loads and executes a module; the repo's
`transform.wasm` is a stub that returns an empty body (documented) — a real
plugin must write transformed JSON back into module memory.

## 7. Starlark DSL — WORKS (with quirks)

```python
# workflow.star — the 4 builtins return (status_code, body) tuples
code, body = http_post("http://localhost:9801/projects", '{"name":"DSL Project"}')
print("POST ->", code)                    # 201
code2, body2 = http_get("http://localhost:9801/projects?q=DSL")   # 200
code3, body3 = http_delete("http://localhost:9801/projects/6")    # 200
```

```bash
./bin/openapi-cli dsl run workflow.star
```

**Quirks (all observed):** `import` is NOT supported (parse error — keep
scripts to the 4 HTTP builtins + core expressions); `print()` output goes to
structured `INF` log lines; at script end the interpreter dumps every variable
binding (noisy for long scripts). See DF-006.

---

## Findings (board: `.coding-hermes/board/tasks.jsonl`)

| ID | Sev | Finding |
|----|-----|---------|
| DF-001 | P1 | Daemon start/status/stop control channel broken — lies about running state, can't stop |
| DF-002 | P2 | commands.md/quickstart.md document nonexistent commands (`spec`, `doctor`, `init --output`, `inventory list <spec>`) |
| DF-003 | P2 | `generate` silently overwrites persisted command store; stale commands (e.g. `list-pets`) leak into `--help` |
| DF-004 | P2 | README implies public consumability; repo is private — `go get @v0.1.0`/clone need credentials |
| DF-005 | P3 | `--version` undefined on openapi-mcp / openapi-daemon |
| DF-006 | P3 | Error UX: usage dump on HTTP errors, DSL variable dump, demo services return HTML 500 on bad input |
