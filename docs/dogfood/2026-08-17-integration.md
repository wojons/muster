# Muster Dogfood — 2026-08-17 Deep Integration Run

> Real use, not tests: a fresh build, a scratch HOME, two live Task Forge mock
> backends (9801/9811), and every documented surface driven like a user would —
> CLI, MCP, daemon, Go library, WASM, Starlark. Verdict: **🟡 PROMISING-BUT-ROUGH**.
> Companion: [diagnostics.md](diagnostics.md) (how it's built, root causes),
> [SKILL.md](../../skills/muster-usage/SKILL.md) (quick reference).

**Promise tested:** *"Turn any OpenAPI spec into a CLI, an MCP server, and a Go
library. Instantly."* — README quick start: `discover` → `generate` → typed
subcommands → `request`.

**Environment:** Go 1.26.6, fresh `make build` (all 3 binaries), isolated
`HOME=/tmp/dogfood-muster/home` (config + command store), task-forge Flask
mocks on :9801 and :9811 (same spec, different backend), petstore.yaml for the
operationId comparison. No destructive commands; mock data only.

---

## 1. What worked (verified live, with evidence)

| # | Surface | Result |
|---|---------|--------|
| 1 | `openapi-cli version` | `v0.1.0-117-g57b90f21` — real version, not `dev` |
| 2 | `openapi-cli init` | Creates `~/.config/muster/config.yaml`, exit 0 (GAP-029) |
| 3 | `discover` local spec | `Base URL: http://localhost:9801` — spec URL, not file path (GAP-016) |
| 4 | `generate` (petstore, has operationIds) | Correct paths: `list-pets → GET /pets`, `get-pet → GET /pets/{petId}` |
| 5 | Typed `create-project --name X` | 201, project created (real backend data) |
| 6 | Typed `get-project 1` | 200, project JSON (path param is POSITIONAL) |
| 7 | `--base-url` override | Project created on :9811 while :9801 untouched (GAP-023) |
| 8 | MCP tools/list + tools/call | 22/22 tools, clean snake_case names even without operationIds; full CRUD round-trip; 404s surface as tool errors |
| 9 | Daemon lifecycle | `--version` real; `--help` exit 0 + subcommands + fixed pid-file default; `start` detaches in 0.054s; `status` shows Running/PID; `stop` cleans socket+pid+token files |
| 10 | Daemon `proxy` mode | Real proxying: GET /projects → backend data, POST → 201 + project created |
| 11 | WASM | `modules run hello.wasm` auto-selects sole export → `[42]` (GAP-014) |
| 12 | Starlark DSL | 6-step workflow (create project → 2 tasks → list → update) all real 200/201s |
| 13 | Go library consumer | /tmp consumer: parsed spec, generated 6 groups/22 ops, executed generated cobra tree (created project), direct pkg/client POST → 201 |
| 14 | `go test -short ./...` | 0.36s warm (46 pkgs ok) — GAP-026 holds |
| 15 | Generic `request` | `GET /tasks` → 200 `[]` (baseline sanity) |

**Time-to-first-success:** ~4 min from `make build` to first real request
(`version` + `init` + `discover` + `create-project` against the local mock).
Library integration (scratch consumer building something real): ~15 min total,
~10 of which was API discovery (see DF-012).

---

## 2. What broke (all reproduced live)

### DF-007 — `generate` on an operationId-less spec mangles names, paths, and base URLs (P1)
Trigger: `demo/services/task-forge/openapi.json` has **zero operationIds**
(`grep -c operationId` = 0; petstore.yaml has 5). The fallback naming path
produces:

```
list-projects  GET  /projectss        ← doubled-s → 404 (Flask) on :9801
list-tasks     GET  /taskss           ← doubled-s → 404
list-tags      GET  /tagss            ← doubled-s → 404
list-health    GET  /healths          ← doubled-s → 404
delete-mileston  DELETE /milestons/{milestonId}   ← truncated ("milestone"→"mileston") → 404
```

…while the same run produced correct `create-project POST /projects` (201) and
`get-project GET /projects/{projectId}` (200). The broken commands also default
to `https://api.example.com` instead of the spec's `servers[].url`: in the
generated `commands.json`, 9 of 22 commands carry `BaseURL:
https://api.example.com` (top-level `base_url` = api.example.com), 13 carry the
spec's `http://localhost:9801`. Evidence: `commands.json` dump + live 404s +
`request GET /tasks` working (proving the server side is fine).

The demo's own flagship mock spec triggers this — the demo's "spec → CLI" story
breaks on its own fixture. Specs with operationIds are unaffected (petstore
verified).

### DF-008 — Name collisions shadow operations (P1)
`create-project` ×3 (Create project / Create milestone / Create task in
project — only POST /projects reachable), `get-project` ×2, `get-task` ×2.
Worse: `commands.json` stores `create-task = POST /tasks`, but the runtime
`create-task` executes `POST /tasks/{task_id}/comments` (add-comment; help
shows `--author/--body`). **"Create task" is unreachable via typed commands.**
Same collisions reproduced in the library-generated cobra tree (root cause in
`pkg/generator`'s `getCommandName`, shared by CLI and library).

### DF-009 — `openapi-daemon start --backend-url` does not proxy (P1)
`start --backend-url http://localhost:9801` prints *"Proxying requests to:
http://localhost:9801"* then serves `{"status":"ok","message":"Muster daemon
is running"}` for **every** path (GET and POST, verified via
`curl --unix-socket`). `proxy` mode (foreground) proxies correctly. README
documents the start+curl workflow — users get a canned "it works" response
with zero backend traffic.

### DF-010 — Generated help lies about groups and hides positional args (P2)
`create-project --help` Example: `openapi-cli projects create-project` →
`Error: unknown command "projects"`. `get-project --help` shows no Args info;
the only hint a positional exists is the runtime error *"expected 1 arguments,
got 0"* (I first tried `--project-id`, which doesn't exist).

### DF-011 — `openapi-daemon restart` = stop-only (P2)
"Daemon stopped; restart must be initiated externally (this process cannot
re-spawn itself)" — status after restart: Running:false. Listed in `--help` as
a first-class subcommand.

### DF-012 — `pkg/generator` undocumented in the API reference (P3)
`docs/api/reference-card.md` has no generator entry although the integration
guide points there for "operation extraction". Had to read source to learn
`NewGenerator(Config)` + `GenerateCommands(doc)`.

---

## 3. Working integration recipe (the "right way" as of 2026-08-17)

```bash
# 1. Build + isolate
make build && export HOME=/tmp/scratch-home
# 2. Backend
PORT=9801 .venv/bin/python3 demo/services/task-forge/server.py &
# 3. CLI happy path (spec WITH operationIds — or use `request`/MCP for others)
./bin/openapi-cli generate demo/openapi-specs/petstore.yaml
./bin/openapi-cli create-project --name Demo --description x   # typed, POST /projects
./bin/openapi-cli get-project 1                                # positional path param
./bin/openapi-cli list-projects --base-url http://localhost:9811   # env switch, no regen
# 4. MCP (full coverage regardless of operationIds)
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}' \
'{"jsonrpc":"2.0","method":"notifications/initialized"}' \
'{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"post_projects","arguments":{"name":"X"}}}' \
| ./bin/openapi-mcp -spec demo/services/task-forge/openapi.json -base-url http://localhost:9801
# 5. Daemon: use proxy mode for real traffic; start/status/stop for lifecycle
./bin/openapi-daemon proxy --backend-url http://localhost:9801 --socket-path /tmp/m.sock &
curl --unix-socket /tmp/m.sock http://localhost/projects
# 6. Go library: /tmp/dogfood-muster/consumer pattern (go.mod replace + pkg/openapi|generator|client)
# 7. Verify: go test -short ./...  (0.36s warm)
```

## 4. Errors hit and their fixes (user's-eye table)

| Error seen | Cause | Fix / workaround |
|---|---|---|
| Flask 404 on `list-projects` after generate | doubled-s path `/projectss` (DF-007) | operationIds in spec, or generic `request`, or MCP |
| `Error: unknown flag: --project-id` | path params are positional, undocumented (DF-010) | `get-project 1` |
| `create-task` creates a COMMENT not a task | name collision shadowing (DF-008) | MCP `post_tasks`, or `request POST /tasks` |
| `curl --unix-socket` returns canned status JSON | `start` mode doesn't proxy (DF-009) | use `proxy` mode |
| `restart` leaves daemon stopped | stop-only implementation (DF-011) | `start` again after `restart` |
| `unsupported protocol scheme ""` (MCP) | spec has relative `servers[].url` | pass `-base-url` |
| petstore 500 on findByStatus | swagger.io server-side | `GET /pet/1` or local mocks |
| `Syntax error: ")" unexpected` at repo root | stale cross-compiled binaries (fixed GAP-028) | use `bin/` |

**What I'd tell the maintainer to fix first (1 hour budget):** DF-007 — the
no-operationId fallback in `pkg/generator` (it's the demo's own spec that
triggers it); then DF-008 (collisions); then DF-009 (daemon start proxy).
