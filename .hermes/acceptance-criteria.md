# Acceptance Criteria for Muster

> Written and maintained by Hermes from user conversations.
> Hermes tests these as a real user — spins up fake services, walks full journeys.

**Last run:** 2026-06-09 19:00 CDT
**Run status:** AC-001 ✅ AC-002 ✅ AC-003 ✅ AC-004 ✅. AC-005 delegated to Axiom.
**Summary:** AC-011 ✅. AC-001 ✅. AC-002 ✅. AC-003 ✅. AC-004 ✅ (MCP tools/list + tools/call across 5 apps). AC-005 🔄. AC-006 ⬜. AC-012 ⬜. AC-013 ⬜.
**Key findings:** 7 commits this session. 98 MCP tools across 5 services. MCP handles auth-gated endpoints, missing args, unknown tools gracefully.

## Demo Infrastructure (Fake Apps)

Muster needs things to talk to. These fake services get spun up during testing:

> **Note:** AC-011 is currently building 5 enhanced demo apps in `tests/muster-test-suite/apps/`. The table below shows the TARGET state. Current `demo/` directory has simpler versions (crud-service, file-service, auth-service, spec-provider) that will be replaced.

| Service | Port | Language | What it provides |
|---------|------|----------|-----------------|
| **Task Forge** | 9800 | Python Flask | Project management API — projects, tasks, comments, milestones, tags. Nested resources, query filters, PATCH partial updates, pagination, search |
| **File Vault** | 9802 | Python Flask | File storage API — multipart upload, binary download, folder hierarchy, sharing, soft delete (trash/recover) |
| **Content Hub** | 9803 | Python Flask | Blog/CMS with role-based auth (reader/author/editor/admin) — posts, comments, draft→publish workflow |
| **Metrics Engine** | 9804 | Python Flask | Time-series analytics — bulk insert, range queries, aggregations (avg/min/max/sum), multi-series, dashboard CRUD |
| **Orchestrator** | 9805 | Python Flask | Workflow/webhook API — multi-step chained execution, async job polling, callback URLs |

The spec-provider concept is absorbed into each app's own `/openapi.json` endpoint. A combined spec at `tests/muster-test-suite/specs/all-services.json` references all 5.

## Active Criteria

### AC-001: CLI discovers operations from a live OpenAPI spec
**Journey:** A user starts the spec-provider and crud-service, then points the CLI at the spec URL. The CLI reaches out, fetches the OpenAPI JSON, and displays every operation the API supports — method, path, parameters, response types. The user can see at a glance what the API can do without reading raw JSON.
**Expected at each step:**
  1. Start `crud-service` on :9800 and `spec-provider` on :9801 → `curl http://localhost:9801/openapi.json` returns 200 with valid JSON containing paths for `/tasks` (GET, POST, PUT, DELETE)
  2. `./bin/openapi-cli discover http://localhost:9801/openapi.json` → exits 0
  3. Output lists operations: `GET /tasks` (listTasks), `POST /tasks` (createTask), `GET /tasks/{id}` (getTask), `PUT /tasks/{id}` (updateTask), `DELETE /tasks/{id}` (deleteTask)
  4. Each operation shows its parameters — createTask should show required `title` string and optional `status` field
  5. Run against `file-service` spec → `POST /files` shows `multipart/form-data` content type
**Cross-verify:** `curl http://localhost:9801/openapi.json | jq '.paths | keys'` — CLI output must match the spec exactly
**Edge cases to probe:** Spec with no operations (empty paths) → graceful message, not crash. Invalid spec URL → clear error, not panic.
**Status:** ✅ passed
**Evidence (2026-06-09 17:00):** ✅ `discover http://localhost:9800/openapi.json` fetches spec via HTTP, parses, displays 32 ops with params. ✅ `generate http://...` works with URLs (was broken). ✅ Edge cases: invalid URL, unreachable host, non-OpenAPI response all return clean errors. ✅ `go build ./cmd/openapi-cli/` passes. ✅ `go vet` clean. Committed ce5bd06.

### AC-002: CLI executes full CRUD lifecycle against a live REST API
**Journey:** User discovers a CRUD API spec, then creates a task, lists all tasks, retrieves the specific task, updates its status, deletes it, and verifies it's gone — a complete lifecycle through the CLI. Every step hits the real Flask service and returns real HTTP responses.
**Expected at each step:**
  1. `crud-service` on :9800, `spec-provider` on :9801 — both healthy (`curl localhost:9800/health` → 200)
  2. **Create:** `./bin/openapi-cli request POST http://localhost:9800/tasks -H "Content-Type: application/json" -b '{"title":"Buy groceries","done":false}'` → HTTP 201, JSON body has `id: 1`, `title: "Buy groceries"`, `done: false`
  3. **List:** `./bin/openapi-cli request GET http://localhost:9800/tasks` → HTTP 200, JSON array containing the created task
  4. **Get:** `./bin/openapi-cli request GET http://localhost:9800/tasks/1` → HTTP 200, exact task returned
  5. **Update:** `./bin/openapi-cli request PUT http://localhost:9800/tasks/1 -H "Content-Type: application/json" -b '{"done":true}'` → HTTP 200, `done: true`, `title` unchanged
  6. **Delete:** `./bin/openapi-cli request DELETE http://localhost:9800/tasks/1` → HTTP 200, `{"deleted":true}`
  7. **Verify deletion:** `./bin/openapi-cli request GET http://localhost:9800/tasks/1` → HTTP 404, `{"error":"task not found"}`
**Cross-verify:** Check crud-service logs — all 6 HTTP requests appear with correct methods, paths, and JSON bodies
**Edge cases to probe:** Create with missing required field → clear validation error. Update non-existent ID → 404. Double-delete → 404.
**Status:** 🔄 in progress — Axiom fixing body-as-query-param bug in generated commands
**Evidence (2026-06-09 17:11):** ✅ Full CRUD lifecycle via `request` command: CREATE → 201, LIST → 200 (appears in list), GET → 200, UPDATE → 200, DELETE → 200 (`{"deleted":true}`), VERIFY → 404 (`{"error":"project not found"}`). ❌ Generated commands broken (body fields sent as query params → HTTP 400). Axiom fixing now.

### AC-003: CLI handles different API flavors — file upload, auth, pagination
**Journey:** The CLI isn't just for CRUD. A user uploads a file to a file service, downloads it back, lists all files. Then they hit an auth-gated API without a token and get denied; with the token they succeed. They create 15 tasks and paginate through them — page 2 shows only items 6-10.
**Expected at each step:**
  1. Start `file-service` on :9802, `auth-service` on :9803, `crud-service` on :9800
  2. **File upload:** `./bin/openapi-cli execute uploadFile --file ./testdata/hello.txt` → HTTP 201, response shows `filename: "hello.txt"`, `size: <correct bytes>`
  3. **File download:** `./bin/openapi-cli execute getFile --name hello.txt` → HTTP 200, response body matches original file content
  4. **File list:** `./bin/openapi-cli execute listFiles` → HTTP 200, array includes `hello.txt`
  5. **Auth-gated (no token):** `./bin/openapi-cli execute listTasks --target http://localhost:9803` → HTTP 401, error mentions missing auth
  6. **Auth-gated (with token):** Set `MUSTER_AUTH_TOKEN=test-token-123`, re-run → HTTP 200, data returned. Verify `Authorization: Bearer test-token-123` in auth-service logs
  7. **Pagination:** Create 15 tasks in crud-service, then `execute listTasks --page 2 --limit 5` → HTTP 200, returns page 2 items only (tasks 6-10)
**Cross-verify:** auth-service logs show 401 then 200 with correct Bearer header. crud-service logs show paginated request with query params.
**Edge cases to probe:** Upload 0-byte file → still works (201). Upload huge file → clean error, not crash. Page beyond data range → empty array, not error.
**Status:** pending

### AC-004: MCP server registers tools and executes them — same APIs, different protocol
**Journey:** An AI agent connects to Muster's MCP server via JSON-RPC over stdio. It calls `tools/list` and sees every OpenAPI operation as a tool with typed parameters. It calls `createTask`, which makes a real HTTP POST to the crud-service. It calls `listTasks` and sees the task it just created. Same API, different protocol — the AI agent never knows it's talking to a REST service.
**Expected at each step:**
  1. Start `crud-service` :9800, `spec-provider` :9801
  2. Start MCP server: `./bin/openapi-mcp --spec http://localhost:9801/openapi.json`
  3. Send `tools/list`: `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` → response contains `createTask` (with `title`, `status` params), `listTasks`, `getTask` (with `id` param), `updateTask`, `deleteTask`
  4. Each tool's `inputSchema` matches the OpenAPI spec's parameter definitions
  5. Call `createTask`: `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"createTask","arguments":{"title":"MCP-created","status":"pending"}}}` → valid JSON-RPC response, `content[0].text` contains HTTP 201 body with task JSON
  6. Call `listTasks`: `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"listTasks","arguments":{}}}` → response includes "MCP-created" task
**Cross-verify:** crud-service logs show POST and GET arriving, proving MCP tools → real HTTP
**Edge cases to probe:** `tools/call` with unknown tool name → JSON-RPC error. `tools/call` with wrong param types → clear validation error. Server start with invalid spec URL → clean exit, not crash.
**Status:** pending

### AC-005: Daemon proxies requests through a Unix socket
**Journey:** A user starts the daemon, which listens on a Unix socket. They use curl to send HTTP requests through the socket to the backend — create a task, list tasks, delete a task, all transparently proxied. The daemon is invisible; the user just talks HTTP over a file path instead of a TCP port.
**Expected at each step:**
  1. Start `crud-service` on :9800
  2. Start daemon: `./bin/openapi-daemon --backend http://localhost:9800 --socket /tmp/muster-daemon-test.sock`
  3. `curl --unix-socket /tmp/muster-daemon-test.sock http://localhost/tasks` → HTTP 200, task list JSON (same as hitting :9800 directly)
  4. `curl --unix-socket /tmp/muster-daemon-test.sock -X POST http://localhost/tasks -H "Content-Type: application/json" -d '{"title":"Via daemon","status":"pending"}'` → HTTP 201
  5. `curl --unix-socket /tmp/muster-daemon-test.sock http://localhost/tasks` → new task appears
  6. `curl --unix-socket /tmp/muster-daemon-test.sock -X DELETE http://localhost/tasks/<id>` → HTTP 200
  7. **Health check:** `curl --unix-socket /tmp/muster-daemon-test.sock http://localhost/health` → HTTP 200, `{"status":"ok"}`
**Cross-verify:** crud-service logs show all requests arriving — daemon relayed them transparently
**Edge cases to probe:** Backend down → daemon returns 502. Socket file already exists → daemon cleans it up or errors clearly. Multiple concurrent connections → all succeed.
**Status:** pending

### AC-006: All three consumers work against the same services — consistency check
**Journey:** CLI, MCP, and Daemon all talk to the same live services. A user creates a task via CLI, another via MCP, another via Daemon — all three responses have the same JSON shape. Listing tasks through any consumer shows all three. No consumer drops fields, reorders properties, or changes data types.
**Expected at each step:**
  1. Start all services: crud-service :9800, file-service :9802, auth-service :9803, spec-provider :9801
  2. **Via CLI:** `./bin/openapi-cli execute createTask --data '{"title":"CLI check"}'` → record JSON structure
  3. **Via MCP:** `tools/call createTask {"title":"MCP check"}` → record JSON structure
  4. **Via Daemon:** `curl --unix-socket /tmp/muster-daemon-test.sock -X POST http://localhost/tasks -d '{"title":"Daemon check"}'` → record JSON structure
  5. All three responses must have identical JSON shape: `{"id": "...", "title": "...", "status": "pending"}`
  6. **Via CLI:** `execute listTasks` → all 3 tasks present
  7. **Via MCP:** `tools/call listTasks` → same 3 tasks, same data
  8. **Via Daemon:** `curl --unix-socket ... GET /tasks` → same 3 tasks, same data
**Cross-verify:** All three consumers hit the same crud-service — compare logs to confirm request counts
**Edge cases to probe:** Create with Unicode title → all consumers preserve characters correctly. Create with long title (1000 chars) → all consumers handle it.
**Status:** pending

### AC-012: Test scripts exercise all 3 Muster consumers against the 5 demo apps
**Journey:** With all 5 demo apps running and Muster binaries built, shell-based test scripts walk real user journeys: CLI creates/filters/uploads/publishes, MCP server exposes tools and executes calls, daemon proxies through Unix socket. Each script is a standalone shell script with clear pass/fail output.
**Expected at each step:**
  1. `tests/muster-test-suite/tests/cli/test-task-forge.sh` — creates project, adds tasks, filters by priority, verifies output. Exits 0 on pass.
  2. `tests/muster-test-suite/tests/cli/test-file-vault.sh` — uploads file, downloads it, verifies content match, lists, searches
  3. `tests/muster-test-suite/tests/cli/test-content-hub.sh` — tests all 4 auth roles (reader/author/editor/admin), draft→publish, comments
  4. `tests/muster-test-suite/tests/cli/test-metrics-engine.sh` — bulk insert 10 points, range query, aggregation (avg), latest value
  5. `tests/muster-test-suite/tests/cli/test-orchestrator.sh` — creates workflow, triggers webhook, polls execution until complete
  6. `tests/muster-test-suite/tests/mcp/test-tools-list.sh` — `tools/list` returns all operations from all 5 apps with input schemas
  7. `tests/muster-test-suite/tests/mcp/test-tool-execution.sh` — `tools/call` creates tasks, uploads files, publishes posts; verifies via CLI
  8. `tests/muster-test-suite/tests/daemon/test-daemon-proxy.sh` — curl through Unix socket for CRUD, file download, auth pass-through
  9. `tests/muster-test-suite/tests/cross-consistency/test-all-consumers.sh` — CLI creates task → MCP sees it → Daemon sees it (same data shape)
**Cross-verify:** All test scripts reference specific ports, use deterministic seed data, and produce machine-parseable output
**Edge cases to probe:** Missing binary → skip with clear message. App down → fail with diagnostic. Auth token wrong → verify 401.
**Status:** pending
**Depends on:** AC-011 (apps must exist), AC-001 (CLI must work)
**Axiom work item:** WI-test-suite-phase-b

### AC-013: Single-command runner — `make test-integration` or `./tests/muster-test-suite/run.sh`
**Journey:** A developer runs a single command. It builds Muster binaries, starts all 5 demo apps, runs all test scripts in order, reports results in a clean summary, and cleans up all processes — even on Ctrl+C. No manual steps, no remembering port numbers.
**Expected at each step:**
  1. `./tests/muster-test-suite/run.sh` → builds Muster (`go build ./...`), exits with message if build fails
  2. Starts all 5 apps, waits for health checks to pass
  3. Runs CLI tests (5 scripts), MCP tests (2 scripts), Daemon tests (1 script), Cross-consistency (1 script) = 9 total
  4. Each test prints `✓ PASS: <test name>` or `✗ FAIL: <test name> — <reason>`
  5. Summary: `N passed, M failed, X skipped` with exit code = number of failures
  6. Cleanup: kills all 5 app PIDs on exit (trap EXIT), removes Unix socket file
  7. `README.md` documents prerequisites (Python 3 + Flask, curl, jq), how to run, expected output
**Cross-verify:** Run twice — both produce identical results (deterministic). Kill mid-run → apps are cleaned up.
**Edge cases to probe:** Port already in use → clear error with PID. Muster binary missing → `go build` first. Python not found → clear message.
**Status:** pending
**Depends on:** AC-011 (apps), AC-012 (test scripts)
**Axiom work item:** WI-test-suite-phase-c

## Passed Criteria

### AC-011: Enhanced demo apps (5 real-feeling APIs with OpenAPI specs) ✅
**Verified:** 2026-06-09 14:40 CDT
**Evidence:** All 5 apps start healthy (HTTP 200), valid OpenAPI 3.0.3 specs (53 paths, 99 operations). Content Hub correctly returns 401 without Bearer token. 15 files, 7,197 lines. Committed as `3b49f74`.
**Spec reference:** `.hermes/test-project-spec.md`

<!-- Move verified criteria here when they pass -->

## Backlog

### AC-007: WASM plugin intercepts and transforms responses
**Journey:** A developer builds a WASM module that appends `"wasm": true` to every JSON response. They load it into Muster as middleware. Every API response now includes the WASM watermark. If someone loads a broken module that loops forever, Muster kills it after a timeout instead of hanging.
**Expected at each step:**
  1. Build test WASM module: `response-transform.wasm` that appends `"wasm":true` to JSON bodies
  2. Start Muster with `--wasm-plugin ./testdata/response-transform.wasm`
  3. Execute any CRUD operation → response JSON contains `"wasm": true`
  4. Operations returning non-JSON (404, binary) → WASM doesn't crash, response passes through or gets error wrapper
  5. Load module that loops forever → Muster kills it after timeout, returns error, doesn't crash
**Why deferred:** `pkg/wasm` Wasmtime runtime. Phase 32 review flagged MEDIUM memory limits issue. Core pipeline first.

### AC-008: Starlark DSL scripts compose multiple API calls
**Journey:** A user writes a `.star` script that creates a task, waits a moment, updates it to completed, then lists all tasks — a mini workflow in a few lines of Starlark. They run it like a script and see the output trace.
**Expected at each step:**
  1. Write `testdata/workflow.star` with `create_task`, `update_task`, `list_tasks` calls and print statements
  2. `./bin/openapi-cli run testdata/workflow.star` → exits 0
  3. Output shows: `Created: <id>`, `Updated: completed`, `Total tasks: N`
  4. crud-service logs show POST, PUT, GET in sequence
  5. Task exists with `status: "completed"`
  6. **Error case:** Script calling non-existent operation → Starlark error caught, not Go panic
**Why deferred:** `pkg/dsl` Starlark interpreter. Depends on core CLI execution. Phase 32 verified DNS/SSRF protections.

### AC-009: Caching layer reduces repeat requests
**Journey:** When the same GET request is made twice in quick succession, the second call returns instantly from cache — the backend never sees it. After the cache TTL expires, the next call hits the backend again.
**Expected at each step:**
  1. Start crud-service with request counter in logs
  2. Create task, then `execute getTask --id <id>` twice
  3. First call → crud-service log shows 1 GET. Second call → log still shows only 1 GET (cache hit)
  4. Both responses return identical JSON
  5. Create new task → `execute listTasks` → cache miss (new data). Run again → cache hit
  6. Wait for TTL expiry → re-run → cache miss, backend gets hit again
**Why deferred:** `pkg/cache` multi-tier (memory + Redis). Correctness first.

### AC-010: Rate limiter prevents API abuse
**Journey:** Muster is configured to allow 5 requests per second. A user fires 10 rapid requests — the first 5 succeed, the next 5 get rate-limited. After waiting a second, requests work again.
**Expected at each step:**
  1. Start crud-service with rate limit: 5 req/s
  2. Fire 10 rapid `execute listTasks` in a loop
  3. First 5 → HTTP 200. Next 5 → HTTP 429 or queued
  4. Wait 1 second, fire again → HTTP 200
  5. crud-service never received more than 5 requests in any 1-second window
**Why deferred:** `pkg/rate` token bucket. Depends on having enough operations to hit the limit.
