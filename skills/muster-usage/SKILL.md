---
name: muster-usage
description: >-
  How to USE Muster (github.com/wojons/muster) — the dynamic OpenAPI-to-tooling
  system: CLI, MCP server, daemon, Go library, Docker, WASM, Starlark DSL.
  Load this skill before building on, demoing, testing, or dogfooding Muster.
  Verified by real use on 2026-08-07 and 2026-08-17 (see docs/dogfood/).
version: 1.1.0
category: software-development
---

# Muster Usage — Quick Reference for Agents

Muster turns any OpenAPI 3.x spec into seven surfaces from one Go codebase:
CLI, MCP server, Unix-socket proxy daemon, Go library, Docker image, WASM
plugins, Starlark DSL. **No codegen** — it parses the spec at runtime.

## Entry points (all verified 2026-08-17)

| Surface | How to start | Status |
|---------|--------------|--------|
| CLI | `make build` → `bin/openapi-cli` | ✅ works |
| MCP server | `bin/openapi-mcp -spec <file> -base-url <base>` (stdio JSON-RPC) | ✅ works, incl. operationId-less specs |
| Daemon | `bin/openapi-daemon start|status|stop|proxy` | ✅ lifecycle works; ⚠️ `start` does NOT proxy (DF-009), `restart` stops only (DF-011) |
| Go library | `import "github.com/wojons/muster/pkg/{openapi,generator,client}"` | ✅ works (consumer verified) |
| Docker | `docker build -t muster:latest .` (ENTRYPOINT=openapi-cli) | ✅ (verified 08-07) |
| WASM | `openapi-cli modules run <file.wasm> [--fn <name>]` | ✅ works; sole export auto-selected (GAP-014) |
| Starlark | `openapi-cli dsl run script.star` | ✅ works (http_get/post/put/delete builtins) |

## Build

```bash
make build        # bin/openapi-cli, bin/openapi-mcp, bin/openapi-daemon (bin/ is gitignored — always build)
go test -short ./...   # ~0.4s warm, ~46s cold-cache; ALL tests must pass
```

## Core workflows (proven in the 2026-08-17 deep run)

### CLI (most-used surface)
```bash
./bin/openapi-cli version                                  # real version (v0.1.0-117-...)
./bin/openapi-cli init                                     # creates ~/.config/muster/config.yaml (GAP-029)
./bin/openapi-cli discover <spec-url-or-file>              # prints Base URL from spec servers[].url (GAP-016)
./bin/openapi-cli generate <spec-file-or-url>              # persist typed commands to commands.json
./bin/openapi-cli request GET http://localhost:9801/projects -o json
./bin/openapi-cli request POST http://localhost:9801/projects -b '{"name":"x"}' -o json
```
Typed commands (after `generate`): path params are **positional args** (`get-project 1`), body fields are flags (`create-project --name x`), every command has `--base-url` + `MUSTER_BASE_URL` env override (GAP-023). Isolate runs with a scratch `HOME` — config lives at `$HOME/.config/muster/`, command store at `$HOME/.local/share/openapi-cli/commands.json`.

**CRITICAL generate pitfall (DF-007):** on a spec with **no operationIds** (e.g. the demo's own `demo/services/task-forge/openapi.json`), generated list commands get doubled-s paths (`/projectss`, `/taskss` → 404), "milestone" ops get truncated (`delete-mileston`, `/milestons/{milestonId}`), and fallback commands default to `https://api.example.com`. Specs WITH operationIds (petstore.yaml) generate correctly. Workaround: use generic `request` for operationId-less specs, or add operationIds to the spec.

**CRITICAL collision pitfall (DF-008):** same-verb ops on nested resources collapse onto one command name (`create-project` shadows Create milestone + Create task in project; `create-task` runs add-comment, so POST /tasks is unreachable). The MCP surface does NOT have this problem — use MCP for full coverage.

### MCP (agent surface) — the most reliable typed surface
Pipe JSON-RPC lines into `openapi-mcp`; each spec op becomes a snake_case tool (`POST /projects` → `post_projects`, `GET /projects/{id}/tasks` → `get_projects_{project_id}_tasks`). 22/22 tools for task-forge, clean names even without operationIds:
```bash
printf '%s\n' \
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}' \
'{"jsonrpc":"2.0","method":"notifications/initialized"}' \
'{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
| ./bin/openapi-mcp -spec demo/services/task-forge/openapi.json -base-url http://localhost:9801
```
- `-base-url` is REQUIRED when the spec has a relative `servers[].url` (e.g. petstore's `/api/v3`); without it calls fail `unsupported protocol scheme ""`.
- CRUD verified: create → get → delete → list round-trip; backend 404s surface as tool errors.

### Daemon — lifecycle NOW WORKS (GAP-018/020/021/022/031)
```bash
./bin/openapi-daemon start --backend-url http://localhost:9801 --socket-path /tmp/m.sock   # detaches <1s
./bin/openapi-daemon status --socket-path /tmp/m.sock    # Running: true, PID, start time
./bin/openapi-daemon stop --socket-path /tmp/m.sock      # stops + removes socket/pid/token files
./bin/openapi-daemon proxy --backend-url <url> --socket-path <path>   # REAL proxying (foreground)
curl --unix-socket /tmp/m.sock http://localhost/projects              # through proxy mode
```
⚠️ **`start` with `--backend-url` prints "Proxying requests to..." but serves a canned `{"status":"ok","message":"Muster daemon is running"}` for every path — it does NOT proxy (DF-009).** Use `proxy` mode for real proxying. `restart` stops the daemon without restarting it (DF-011).

### Go library (consumer pattern — repo is PRIVATE)
```bash
go mod init mylib
go mod edit -replace github.com/wojons/muster=/path/to/muster   # required: repo not fetchable publicly
```
```go
parser := openapi.NewParser()
result, _ := parser.Parse(ctx, specBytes, openapi.DefaultParseOptions()) // .Document (openapi3.T), .Version
gen, _ := generator.NewGenerator(generator.Config{AppName: "x", BaseURL: "http://host", DefaultFormat: "json"})
cmds, _ := gen.GenerateCommands(result.Document)   // []*cobra.Command, grouped by resource (projects, tasks, ...)
// execute: add cmds to a root cobra.Command; each op has --base-url flag + RunE doing the HTTP
c := client.NewClient(client.WithBaseURL("http://host"), client.WithTimeout(10*time.Second))
resp, _ := c.Post(ctx, "/projects", "application/json", bytes.NewReader([]byte(`{...}`))) // Get/Put/Patch/Delete too
```
Verified end-to-end 2026-08-17: /tmp/dogfood-muster/consumer (tfctl) parsed task-forge, generated 6 groups/22 ops, executed the generated tree (created a real project), and used pkg/client directly. NOTE: the generated tree has the SAME name collisions as the CLI (shared root cause in pkg/generator — DF-008).

### Demo services (offline, deterministic backends)
```bash
PORT=9801 .venv/bin/python3 demo/services/task-forge/server.py &   # + bytestash 9802, inkwell 9803, pulse 9804, chainlink 9805
curl localhost:9801/openapi.json   # their own OpenAPI specs (NOTE: no operationIds → DF-007 trigger)
./demo/run-all.sh                  # full 7-surface demo, 23 checks (needs .venv)
```
Mock stores are **in-memory** — data resets on server restart (by design).

## Pitfalls checklist

1. `make build` first — `bin/` is gitignored; stale binaries mislead.
2. Isolate CLI runs: `export HOME=/tmp/scratch-home` (config + command store both live under HOME).
3. `-base-url` for MCP with relative server URLs (petstore `/api/v3`).
4. Repo is private: use `replace` for Go consumers, GOPRIVATE for go get.
5. `generate` on operationId-less specs → broken paths + api.example.com defaults (DF-007); collisions shadow ops (DF-008). Use MCP or `request` for those specs.
6. Generated `--help` Examples reference non-existent `projects`/`tasks` groups (DF-010); path params are positional and undocumented.
7. DSL: no `import`; 4 builtins only (`http_get/post/put/delete` returning `(code, body)`).
8. Public petstore endpoints `findByStatus`/`store/inventory` return 500 server-side (swagger.io issue, not Muster) — use `GET /pet/1`, `GET /store/order/1`, or local demo services.
9. Daemon: `start` doesn't proxy (DF-009); `restart` = stop-only (DF-011).

## References

- docs/integration-guide.md — runnable examples for all 7 surfaces (mostly trustworthy; §1 examples use petstore which has operationIds)
- docs/dogfood/2026-08-17-integration.md — 2026-08-17 deep-run record (this run)
- docs/dogfood/2026-08-07-integration.md — 2026-08-07 deep-run record
- docs/dogfood/diagnostics.md — root causes, error table, "right way"
- Board: `.coding-hermes/board/tasks.jsonl` (canonical JSONL; open findings DF-007..DF-012 from the 2026-08-17 run)
