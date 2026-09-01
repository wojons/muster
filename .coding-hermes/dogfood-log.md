# Dogfood Log — Muster

| Date | Verdict | Promise | Time-to-first-success | Top findings |
|------|---------|---------|----------------------|--------------|
| 2026-08-07 | 🟡 PROMISING-BUT-ROUGH | Turn any OpenAPI spec into 7 surfaces instantly (CLI, MCP, daemon, Go lib, Docker, WASM, Starlark) | ~6 min (fresh `make build` + petstore discover/request) | 1. Daemon start/status/stop control channel broken (DF-001, P1). 2. Guides document nonexistent commands — `spec`, `doctor`, `init --output` (DF-002, P2). 3. `generate` silently overwrites persisted command store; stale commands leak into `--help` (DF-003, P2). |
| 2026-08-17 | 🟡 PROMISING-BUT-ROUGH | Turn any OpenAPI spec into a CLI, MCP server, and Go library instantly | ~4 min (build + version/init/discover + first typed create against local mock) | 1. `generate` on operationId-less specs (incl. demo's own task-forge) mangles paths (/projectss, /milestons) + defaults to api.example.com (DF-007, P1). 2. Command name collisions shadow ops — create-task runs add-comment, POST /tasks unreachable (DF-008, P1). 3. daemon `start --backend-url` serves canned status instead of proxying; `restart` = stop-only (DF-009/011, P1/P2). |

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

## 2026-08-17 — Deep run (2nd, post GAP-001..031)

**Promise statement:** *"Muster claims a user can turn any OpenAPI spec into
typed CLI commands, an MCP tool surface, a Unix-socket proxy daemon, a Go
library, WASM plugins and Starlark workflows — instantly, no codegen."*

**What was done (real use, not tests):** fresh `make build`; isolated HOME;
two task-forge mocks (:9801/:9811). CLI: version/init/discover/generate +
typed CRUD (create-project 201, get-project 1 200, --base-url switch to :9811
verified). MCP: initialize + tools/list (22 tools) + full CRUD round-trip +
error surfacing. Daemon: --version/--help/start (0.054s detach)/status/stop
all fixed-verified; `proxy` mode real traffic; start-mode proxy broken.
WASM auto-export-select verified; Starlark 6-step workflow; Go library
consumer (/tmp/dogfood-muster/consumer) parsed + generated + executed the
generated tree + direct pkg/client POST; `go test -short ./...` 0.36s warm.

**Findings written to board:** DF-007 (P1), DF-008 (P1), DF-009 (P1),
DF-010 (P2), DF-011 (P2), DF-012 (P3) in `.coding-hermes/board/tasks.jsonl`.

**Foreman:** NOT woken — fleet.toml operator pin cooldown_s=21600 (6h);
prior stand-in cycles established the convention "never PUT below operator
pin" (GAP-029/030/031 reasoning G5). Foreman is enabled and ticking
(last tick 2026-08-17 12:46); it will pick up DF-007..012 on the next
natural tick.
2026-09-01 | PROMISING-BUT-ROUGH | 5s t2fs | friction 11 | 5 findings
