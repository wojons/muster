# Dogfood Log — Muster

| Date | Verdict | Promise | Time-to-first-success | Top findings |
|------|---------|---------|----------------------|--------------|
| 2026-08-07 | 🟡 PROMISING-BUT-ROUGH | Turn any OpenAPI spec into 7 surfaces instantly (CLI, MCP, daemon, Go lib, Docker, WASM, Starlark) | ~6 min (fresh `make build` + petstore discover/request) | 1. Daemon start/status/stop control channel broken (DF-001, P1). 2. Guides document nonexistent commands — `spec`, `doctor`, `init --output` (DF-002, P2). 3. `generate` silently overwrites persisted command store; stale commands leak into `--help` (DF-003, P2). |

## 2026-08-07 — Deep run (all 7 surfaces)

**Promise statement:** *"Muster claims that a user can turn any OpenAPI spec into seven working surfaces instantly — CLI (`discover`/`request`), MCP server (every operation = a tool), Unix-socket proxy daemon, Go library, Docker image, WASM plugins, Starlark DSL — no codegen, minutes from spec to usable tooling."*

**What was done (real use, not tests):**
1. Fresh `make build` → 3 binaries; `openapi-cli version` prints `v0.1.0-8-g0d790342` (GAP-007 verified).
2. CLI: full CRUD session against local Task Forge mock (list → create → read → add task → patch → 404/500 error paths) + README quickstart verbatim against public petstore (both work).
3. MCP: raw JSON-RPC handshake over stdio — initialize (serverInfo.name=openapi-mcp, GAP-010 verified), tools/list (22 tools), tools/call post_projects + get_projects (created real project).
4. Daemon: proxy mode serves GET/POST through Unix socket; **start/status/stop control channel broken** (DF-001).
5. Go library: fresh consumer in /tmp with `replace` — parsed spec (pkg/openapi), generated 6 CLI commands (pkg/generator), executed GET/POST via pkg/client. Success.
6. Docker: `docker build` + CLI discover/request + MCP tools/list in container (19 tools).
7. WASM: `modules run hello.wasm --fn hello` → [42]; add.wasm 2+3 → [5].
8. Starlark DSL: 6-step workflow (create project → query → create task → list → delete) all executed; `import` unsupported (documented in diagnostics).

**Findings written to board:** DF-001 (P1), DF-002 (P2), DF-003 (P2), DF-004 (P2), DF-005 (P3), DF-006 (P3) in `.coding-hermes/board/tasks.jsonl`.

**Foreman:** NOT paused (CooldownS=7200, Enabled=true, ticking) — no wake needed; tasks picked up on next tick.
