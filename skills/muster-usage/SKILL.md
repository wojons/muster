---
name: muster-usage
description: >-
  How to USE Muster (github.com/wojons/muster) — the dynamic OpenAPI-to-tooling
  system: CLI, MCP server, daemon, Go library, Docker, WASM, Starlark DSL.
  Load this skill before building on, demoing, testing, or dogfooding Muster.
  Verified by real use on 2026-08-07 (see docs/dogfood/2026-08-07-integration.md).
version: 1.0.0
category: software-development
---

# Muster Usage — Quick Reference for Agents

Muster turns any OpenAPI 3.x spec into seven surfaces from one Go codebase:
CLI, MCP server, Unix-socket proxy daemon, Go library, Docker image, WASM
plugins, Starlark DSL. **No codegen** — it parses the spec at runtime.

## Entry points

| Surface | How to start | Verified? |
|---------|--------------|-----------|
| CLI | `make build` → `bin/openapi-cli` | ✅ |
| MCP server | `bin/openapi-mcp -spec <file> -base-url <base>` (stdio JSON-RPC) | ✅ |
| Daemon | `bin/openapi-daemon proxy --backend-url <url> --socket-path <path>` | ⚠️ proxy only |
| Go library | `import "github.com/wojons/muster/pkg/{openapi,generator,client}"` | ✅ |
| Docker | `docker build -t muster:latest .` (ENTRYPOINT=openapi-cli) | ✅ |
| WASM | `openapi-cli modules run <file.wasm> --fn <name> [args]` | ✅ |
| Starlark | `openapi-cli dsl run script.star` | ✅ |

## Build

```bash
make build        # bin/openapi-cli, bin/openapi-mcp, bin/openapi-daemon (bin/ is gitignored — always build)
```

## Core workflows (all proven in the 2026-08-07 dogfood run)

### CLI (most-used surface)
```bash
./bin/openapi-cli discover <spec-url-or-file>                 # list all operations
./bin/openapi-cli request GET <base>/projects -o json         # -o json|yaml|table
./bin/openapi-cli request POST <base>/projects -b '{"name":"x"}' -o json
./bin/openapi-cli request GET <base>/projects -p q=term       # query params
./bin/openapi-cli request GET <base>/pets/1 -H "Authorization: Bearer $TOKEN"
```
Real command list (from `--help`): `audit auth completion config daemon
discover dsl generate help inventory modules request tenant version webhook`.
**Do NOT use** `spec`, `doctor`, `init --output`, `inventory list <spec>` —
documented in guides but nonexistent (DF-002).

### MCP (agent surface)
Pipe JSON-RPC lines into `openapi-mcp`; each spec op becomes a snake_case
tool (`POST /projects` → `post_projects`):
```bash
printf '%s\n' \
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}' \
'{"jsonrpc":"2.0","method":"notifications/initialized"}' \
'{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
| ./bin/openapi-mcp -spec /tmp/spec.json -base-url http://localhost:9801
```
- `-base-url` is REQUIRED when the spec has a relative `servers[].url` (e.g. petstore's `/api/v3`); without it calls fail `unsupported protocol scheme ""`.
- Send one request at a time (pipelined bursts can return out of order).

### Go library (consumer pattern — repo is PRIVATE)
```bash
go mod init mylib
go mod edit -replace github.com/wojons/muster=/path/to/muster   # required: repo not fetchable publicly (DF-004)
```
```go
parser := openapi.NewParser()
result, _ := parser.Parse(ctx, specBytes, openapi.DefaultParseOptions()) // .Document, .Version
gen, _ := generator.NewGenerator(generator.Config{AppName: "x", BaseURL: "http://host"})
cmds, _ := gen.GenerateCommands(result.Document)                         // []*cobra.Command
c := client.NewClient(client.WithBaseURL("http://host"), client.WithTimeout(15*time.Second),
    client.WithRetryConfig(retry.DefaultRetryConfig()), client.WithHeader("Accept", "application/json"))
resp, _ := c.Get(ctx, "/projects")                                       // also Post/Put/Delete
```
**Pitfall:** there is NO `generator.ExtractOperations`; the API is
`NewGenerator(Config)` + `GenerateCommands(doc)`.

### Daemon — KNOWN BROKEN lifecycle (DF-001)
`proxy` foreground mode works (curl through the socket). `start`/`status`/
`stop` LIE: status always says not-running, stop never stops. Kill with
`pkill -f openapi-daemon`. Don't report the daemon as "working" beyond proxy
mode until DF-001 lands.

### Demo services (offline, deterministic backends)
```bash
PORT=9801 .venv/bin/python3 demo/services/task-forge/server.py &   # + bytestash 9802, inkwell 9803, pulse 9804, chainlink 9805
curl localhost:9801/openapi.json   # their own OpenAPI specs
./demo/run-all.sh                  # full 7-surface demo, 23 checks
```

## Pitfalls checklist

1. `make build` first — `bin/` is gitignored; stale binaries mislead.
2. `-base-url` for MCP with relative server URLs.
3. Repo is private: use `replace` for Go consumers, GOPRIVATE for go get.
4. `generate` overwrites the ONE persisted command store
   (`~/.local/share/openapi-cli/commands.json`) — stale commands leak into
   `--help` (that's where `list-pets` came from). Re-generate per spec.
5. DSL: no `import`; 4 builtins only (`http_get/post/put/delete` returning
   `(code, body)`); output = binding dump at end, prints go to INF logs.
6. `openapi-mcp`/`openapi-daemon` have no `--version` (DF-005); only
   `openapi-cli version`.
7. Public petstore endpoints `findByStatus`/`store/inventory` return 500
   server-side (swagger.io issue, not Muster) — use `GET /pet/1`,
   `GET /store/order/1`, or the local demo services for deterministic runs.

## References

- docs/integration-guide.md — trustworthy, runnable examples for all 7 surfaces
- docs/dogfood/2026-08-07-integration.md — the verified real-use record
- docs/dogfood/diagnostics.md — root causes, error table, "right way"
- Board: `.coding-hermes/board/tasks.jsonl` (canonical JSONL; tasks DF-001..DF-006 from the 2026-08-07 dogfood run)
