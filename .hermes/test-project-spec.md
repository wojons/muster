# Muster Integration Test Suite (MITS) — Specification

> Written by Hermes. Delivered by Axiom. Stored in the Muster repo so anyone can run it.

## Purpose

A comprehensive, self-contained integration test suite that validates all three Muster consumers (CLI, MCP server, Daemon) against realistic API backends. Running `make test-integration` or `./tests/muster-test-suite/run.sh` gives full end-to-end validation with clear pass/fail output.

## Architecture

```
tests/muster-test-suite/
├── README.md                    # How to run, what to expect
├── run.sh                       # Single command: builds Muster, starts apps, runs tests, cleans up
├── apps/                        # Realistic demo applications (each with OpenAPI spec)
│   ├── task-forge/              # Port 9800 — Project management API
│   │   ├── server.py
│   │   ├── openapi.json
│   │   └── seed.py
│   ├── file-vault/              # Port 9802 — File storage API
│   │   ├── server.py
│   │   ├── openapi.json
│   │   └── seed.py
│   ├── content-hub/             # Port 9803 — Blog/CMS with role-based auth
│   │   ├── server.py
│   │   ├── openapi.json
│   │   └── seed.py
│   ├── metrics-engine/          # Port 9804 — Time-series analytics API
│   │   ├── server.py
│   │   ├── openapi.json
│   │   └── seed.py
│   └── orchestrator/            # Port 9805 — Workflow/chained-call API
│       ├── server.py
│       ├── openapi.json
│       └── seed.py
├── specs/
│   └── all-services.json        # Combined OpenAPI spec referencing all 5 apps
├── tests/
│   ├── cli/                     # Shell-based CLI integration tests
│   │   ├── test-task-forge.sh
│   │   ├── test-file-vault.sh
│   │   ├── test-content-hub.sh
│   │   ├── test-metrics-engine.sh
│   │   └── test-orchestrator.sh
│   ├── mcp/                     # JSON-RPC MCP server tests
│   │   ├── test-tools-list.sh
│   │   └── test-tool-execution.sh
│   ├── daemon/                  # Unix socket proxy tests
│   │   └── test-daemon-proxy.sh
│   └── cross-consistency/       # CLI vs MCP vs Daemon consistency
│       └── test-all-consumers.sh
├── fixtures/
│   ├── hello.txt
│   └── sample-metrics.json
└── expected/                    # Expected output snapshots for deterministic tests
```

## Demo Applications — 5 Real-Feeling APIs

Each demo app demonstrates different API patterns Muster must handle. They replace the current basic `demo/` services with richer, more realistic backends.

### 1. Task Forge (Port 9800) — Project Management API
**Feels like:** GitHub Projects / Linear / Jira lite
**Patterns demonstrated:** Nested resources, query filters, partial updates, pagination, sorting, search
**Resources:** Projects, Tasks, Comments, Milestones, Tags
**Key endpoints:**
- `GET/POST /projects` — list/create projects
- `GET/POST /projects/{id}/tasks` — tasks nested under project
- `GET/PUT/PATCH/DELETE /tasks/{id}` — full CRUD with PATCH partial update
- `GET /tasks?project_id=1&status=open&priority=high&tag=bug&sort=created_at&order=desc&page=2&per_page=10` — rich filtering
- `GET /tasks/search?q=critical+bug` — full-text search
- `POST /tasks/{id}/comments` — nested comments
- `GET/POST /milestones` — milestone CRUD

### 2. File Vault (Port 9802) — File Storage API
**Feels like:** Google Drive / Dropbox API
**Patterns demonstrated:** Multipart upload, binary download, metadata, folder hierarchy, sharing
**Key endpoints:**
- `POST /files` — multipart upload with metadata (filename, folder_id, tags)
- `GET /files/{id}/download` — binary download with Content-Disposition
- `GET /files?folder_id=3&sort=name` — list with filters
- `PATCH /files/{id}` — rename/move (partial update: `{"name":"new.txt","folder_id":7}`)
- `POST /files/{id}/shares` — share with another user
- `POST /files/{id}/trash` / `POST /files/{id}/recover` — soft delete
- `GET /files/search?q=report` — search by name

### 3. Content Hub (Port 9803) — Blog/CMS with Auth
**Feels like:** Medium / WordPress REST API
**Patterns demonstrated:** Role-based auth (reader/author/editor/admin), draft→publish workflow, nested comments, tag filtering
**Resources:** Posts, Comments, Authors
**Key endpoints:**
- `POST /posts` (author+) — create draft `{"title":"...", "content":"...", "tags":["golang"]}`
- `PUT /posts/{id}/publish` (editor+) — transition to published
- `GET /posts?status=published&tag=golang&page=1&per_page=10` — public listing
- `GET/POST /posts/{id}/comments` — nested comments
- `GET /authors/{id}` — author profile with post count
- `GET /posts/search?q=docker+kubernetes` — full-text search
- Auth tokens: `reader-token`, `author-token`, `editor-token`, `admin-token` — each with different permissions

### 4. Metrics Engine (Port 9804) — Time-Series Analytics
**Feels like:** Datadog / Grafana metrics API
**Patterns demonstrated:** Time-series, aggregations (avg/min/max/sum), range queries, bulk insert
**Resources:** Metrics, DataPoints, Dashboards
**Key endpoints:**
- `POST /metrics` — bulk insert: `{"points":[{"name":"cpu","value":85.2,"timestamp":"2024-01-01T00:00:00Z"},...]}`
- `GET /metrics/{name}?from=2024-01-01T00:00:00Z&to=2024-01-02T00:00:00Z` — range query
- `GET /metrics/{name}/aggregate?fn=avg&interval=1h` — aggregation
- `GET /metrics?names=cpu,memory,disk` — multi-series
- `GET /metrics/{name}/latest` — most recent value
- `GET/POST/PUT/DELETE /dashboards` — dashboard CRUD

### 5. Orchestrator (Port 9805) — Workflow/Webhook API
**Feels like:** Zapier / n8n webhook receiver
**Patterns demonstrated:** Webhook triggers, chained API calls, async job polling, callback URLs
**Resources:** Workflows, Executions, Triggers
**Key endpoints:**
- `POST /workflows` — define a multi-step workflow
- `POST /triggers/{name}/webhook` — webhook endpoint that kicks off a workflow
- `GET /executions/{id}` — poll job status/progress
- `GET /executions?workflow_id=3&status=completed` — list execution history
- `POST /callbacks/{id}` — receive async callback results

## Test Coverage Matrix

| Consumer | Task Forge | File Vault | Content Hub | Metrics Engine | Orchestrator |
|----------|-----------|------------|-------------|----------------|--------------|
| CLI      | CRUD + filter + paginate + partial update | Upload + download + list + search | Auth-gated (4 roles) + draft/publish | Bulk insert + range + aggregate | Trigger + poll |
| MCP      | tools/list → all ops, tools/call → real HTTP | tools/list → all ops, tools/call → real HTTP | tools/list → all ops, tools/call → real HTTP | tools/list → all ops, tools/call → real HTTP | tools/list → all ops, tools/call → real HTTP |
| Daemon   | Unix socket proxy CRUD | Unix socket proxy upload | Unix socket proxy + auth pass-through | Unix socket proxy range | Unix socket proxy trigger |

## User Journeys (the "feels real" tests)

### Journey 1: CLI Power User
1. `./bin/openapi-cli discover specs/all-services.json` → lists all 5 APIs and their operations
2. Creates a project "Q4 Launch" in Task Forge
3. Adds tasks: "Write docs" (high), "Fix bug #42" (critical), "Update deps" (low)
4. Filters: `listTasks --project-id 1 --priority critical` → only "Fix bug #42"
5. Uploads design doc to File Vault: `uploadFile --file ./docs/design.pdf --folder-id 1`
6. Searches: `searchFiles --q design` → finds design.pdf
7. Publishes a post in Content Hub: set `MUSTER_AUTH_TOKEN=editor-token`, create post, publish
8. Inserts CPU metrics: `POST /metrics` bulk insert 10 points
9. Queries range: `getMetric cpu --from ... --to ...` → returns 10 points
10. Triggers a workflow that chains task creation + file upload
11. Polls execution until complete

### Journey 2: MCP AI Agent
1. Connect to `openapi-mcp --spec ./specs/all-services.json`
2. `tools/list` → sees operations from all 5 services with typed input schemas
3. `tools/call createTask {"title":"Agent task"}` → HTTP 201
4. `tools/call listTasks {"project_id":1}` → includes agent task
5. `tools/call createPost {"title":"AI wrote this","content":"Generated content"}` → HTTP 201
6. Cross-verify: CLI `listTasks` shows same data as MCP `listTasks`

### Journey 3: Daemon Proxy
1. Start daemon: `./bin/openapi-daemon --spec ./specs/all-services.json --socket /tmp/muster-test.sock`
2. `curl --unix-socket /tmp/muster-test.sock http://localhost/tasks` → same as CLI
3. `curl --unix-socket /tmp/muster-test.sock -X POST -H "Authorization: Bearer editor-token" ...` → auth passes through
4. Binary download through daemon: `curl --unix-socket ... GET /files/1/download` → file bytes match

## Success Criteria for the Test Suite Itself

1. **Single command:** `make test-integration` or `./tests/muster-test-suite/run.sh` does everything
2. **Self-contained:** No external services, no API keys, no internet required
3. **Deterministic:** Seeded data + fixed ports + deterministic test order → same result every run
4. **Fast:** Full suite < 2 minutes
5. **Clear output:** Each test prints `PASS` or `FAIL` with actual vs expected on failure
6. **Cleans up:** Kills all demo services on exit (even on Ctrl+C)
7. **No mocks:** Every test hits a live running Flask service
8. **Go build gate:** Run `go build ./...` before testing, skip tests if build fails

## Implementation Phases (for Axiom)

### Phase A: Enhanced Demo Apps (replaces current demo/)
Build all 5 demo apps with their OpenAPI specs. Each app:
- Flask server with proper error handling, logging, health endpoint
- Complete OpenAPI 3.0 JSON spec (not a stub — every endpoint, every parameter, every response code)
- Seed script that populates realistic test data
- Port assignments: 9800 (Task Forge), 9802 (File Vault), 9803 (Content Hub), 9804 (Metrics Engine), 9805 (Orchestrator)

### Phase B: Combined Spec + Test Scripts
- `specs/all-services.json` — combined OpenAPI spec that references all 5 apps
- Shell test scripts for CLI, MCP, and Daemon consumers
- Each test script is a shell script that starts the relevant apps, runs Muster commands, checks output, exits with status

### Phase C: Runner + README
- `run.sh` — master runner that builds Muster, starts all apps, runs all tests, reports summary
- `README.md` — how to run, prerequisites, expected output, troubleshooting

## Relationship to Existing demo/ Directory

The existing `demo/` directory at the repo root will be replaced by `tests/muster-test-suite/apps/`. The old `demo/crud-service`, `demo/file-service`, `demo/auth-service`, and `demo/spec-provider` are simpler versions — the new apps are richer and self-documenting via their OpenAPI specs.

The existing `demo/run-all.sh` and `demo/stop-all.sh` will be superseded by `tests/muster-test-suite/run.sh`.
