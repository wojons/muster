---
name: muster-usage
description: >-
  How to USE Muster (github.com/wojons/muster) — the dynamic OpenAPI-to-tooling
  system: CLI, MCP server, daemon, Go library, Docker, WASM, Starlark DSL.
  Load this skill before building on, demoing, testing, or dogfooding Muster.
  Verified by real use on 2026-08-07, 2026-08-17, and 2026-09-04 (see docs/dogfood/).
version: 1.2.0
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
**FIXED (verify window 2026-09-04, HEAD 01849718):** generate on operationId-less specs
(e.g. `demo/services/task-forge/openapi.json`) now works — 22 ops → 21 unique commands,
no mangled paths, no api.example.com defaults (DF-007), and nested same-verb ops get
distinct names (`create-project-task` ≠ `create-project`, DF-008). Verified by full CRUD
round-trip against a live task-forge mock. The 08-17 guidance below is kept for history:

Typed commands (after `generate`): path params are **positional args** (`get-project 1`), body fields are flags (`create-project --name x`), every command has `--base-url` + `MUSTER_BASE_URL` env override (GAP-023). Isolate runs with a scratch `HOME` — config lives at `$HOME/.config/muster/`, command store at `$HOME/.local/share/openapi-cli/commands.json`.

**NEW 09-04 pitfall (DF-013):** running the test suite (`go test -short ./...`) with your
real HOME **overwrites the persisted command store with test fixtures** — the e2e test in
`demo/test-suites/100-cli-real-integration_test.go` exec()s the CLI without HOME isolation.
Always set a scratch HOME before running tests, or re-`generate` afterward.

**Relative-server specs (DF-015):** if the spec's `servers[].url` is RELATIVE (petstore:
`/api/v3`), generated typed commands die client-side with `unsupported protocol scheme ""`.
Pass `--base-url https://petstore3.swagger.io/api/v3` (or export `MUSTER_BASE_URL`) — this
is not optional for petstore-style specs.

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

### Daemon — lifecycle + proxying ALL WORK (verified 2026-09-04, HEAD 01849718)
```bash
./bin/openapi-daemon start --backend-url http://localhost:9801 --socket-path /tmp/m.sock   # detaches <1s, and REALLY proxies now (DF-009 fixed)
./bin/openapi-daemon status --socket-path /tmp/m.sock    # Running: true, PID, start time
./bin/openapi-daemon restart --socket-path /tmp/m.sock --backend-url http://localhost:9801  # stops AND restarts (DF-011 fixed)
./bin/openapi-daemon stop --socket-path /tmp/m.sock      # stops + removes socket/pid/token files
curl --unix-socket /tmp/m.sock http://localhost/projects              # real backend traffic through start mode
```
09-04 verification: start-mode proxied live task-forge traffic (GET/POST 200 with real
payloads), restart came back serving, stop cleaned up. The 08-17 canned-response and
stop-only-restart behaviors are gone.

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
2. Isolate CLI runs: `export HOME=/tmp/scratch-home` (config + command store both live under HOME). **Also isolate before running the test suite** — tests clobber the real store (DF-013).
3. `-base-url` for MCP with relative server URLs (petstore `/api/v3`); `--base-url`/`MUSTER_BASE_URL` for typed CLI commands on those specs (DF-015).
4. Repo is private: use `replace` for Go consumers, GOPRIVATE for go get.
5. OperationId-less specs generate cleanly since the DF-007/DF-008 fixes (verified 09-04); MCP remains the most complete typed surface (22/22 tools).
6. Generated `--help` Examples reference non-existent `projects`/`tasks` groups (DF-010); path params are positional and undocumented.
7. DSL: no `import`; 4 builtins only (`http_get/post/put/delete` returning `(code, body)`).
8. Public petstore endpoints `findByStatus`/`store/inventory` return 500 server-side (swagger.io issue, not Muster) — use `GET /pet/1`, `GET /store/order/1`, or local demo services.
9. Daemon start/restart/proxy/stop all verified working 09-04 (DF-009/DF-011 fixed).
10. Bunker fresh-install leg (09-04): docs assume Go 1.26.6+ is already installed; a bare Debian agent needs a manual Go toolchain install first. Also mind the per-agent size budget: deleting the downloaded Go tarball before `make build-cli` avoids a transient "no space left on device" at link time.

## References

- docs/integration-guide.md — runnable examples for all 7 surfaces (mostly trustworthy; §1 examples use petstore which has operationIds)
- docs/dogfood/2026-09-04-integration.md — 2026-09-04 fix-verification + bunker install-leg record (this run)
- docs/dogfood/2026-08-17-integration.md — 2026-08-17 deep-run record
- docs/dogfood/2026-08-07-integration.md — 2026-08-07 deep-run record
- docs/dogfood/diagnostics.md — root causes, error table, "right way"
- Board: `.coding-hermes/board/tasks.jsonl` (canonical JSONL; open findings DF-013/DF-015 from the 2026-09-04 run; DF-007..012 verified fixed)
