## Active Criteria — Demo Projects

> These 5 dummy systems live in `demo/services/`. Each is a self-contained Flask app with seed data.
> Run: `./demo/run-all.sh` — builds Muster, starts all 5, runs 10 demos, prints pass/fail, cleans up.

### AC-D01: Task Forge — Project Management Platform
**What:** Full CRUD for projects, tasks, comments, milestones. Tags with many-to-many, PATCH partial updates, search, filters, pagination.
**Port:** 9801
**Seed data:** "Acme Corp Website Redesign" — 2 projects, 3 milestones, 15 tasks across 3 users, 40 comments
**Demo script:** `demo/demos/02-crud-lifecycle.sh` — create project → create tasks → list → filter by tag → update status → delete
**Verification:** `curl http://localhost:9801/health` → 200, `curl http://localhost:9801/openapi.json` → valid spec with 14+ paths

### AC-D02: ByteStash — Cloud File Storage
**What:** Folders, files, multipart upload, binary download, trash (soft delete with recover/permanent delete), sharing links
**Port:** 9802
**Seed data:** "Marketing Assets" (8 files), "Engineering" (5 binaries), "HR" (3 docs) — organized in folders
**Demo script:** `demo/demos/03-file-workflow.sh` — upload file → list folder → download → verify content → trash → recover → permanent delete
**Verification:** Upload an actual binary, download it, `diff` confirms byte-identical

### AC-D03: Inkwell — Blog / CMS Platform
**What:** Posts, comments, 4 auth roles (reader/author/editor/admin). Draft → review → publish workflow. Comment moderation queue. Role-gated endpoints.
**Port:** 9803
**Seed data:** 5 authors, 12 posts at various stages (draft/review/published), threaded comments, pre-set Bearer tokens per role
**Demo script:** `demo/demos/04-auth-spectrum.sh` — hit as reader (can only read published), author (can create/edit own), editor (can review), admin (can publish/delete). Verify 401/403/200 per role.

### AC-D04: Pulse — Metrics & Analytics Dashboard
**What:** Series, data points, dashboards, alerts. Bulk insert, range queries, 4 aggregation types (avg/min/max/sum), time-window filters.
**Port:** 9804
**Seed data:** "CPU Usage", "Request Latency", "Error Rate" — 50 data points each over 24h. 2 dashboards, 3 alert definitions.
**Demo script:** `demo/demos/05-analytics-pipeline.sh` — bulk insert 10 points → range query → avg aggregation → create alert → list dashboards

### AC-D05: ChainLink — Workflow Automation Engine
**What:** Workflows → steps → executions → webhooks. Multi-step chained execution, async job polling, callback URLs, retry logic.
**Port:** 9805
**Seed data:** "User Onboarding" (4-step: create account → send email → provision resources → notify admin), "Data Pipeline" (3-step: ingest → transform → export). Pre-registered webhook callbacks.
**Demo script:** `demo/demos/06-workflow-orchestration.sh` — create workflow → trigger execution → poll status → verify all steps completed

### AC-D06: Demo Scripts — 10 Self-Documenting Shell Scripts
**What:** Each script is 20-50 lines, prints what it does, shows actual output, ends with `✓ PASS` or `✗ FAIL`. Uses `set -e`, colored output, clear error messages.
| # | Script | What it shows |
|---|--------|--------------|
| 01 | `demo/demos/01-discover-all.sh` | Discover all 5 systems at once, display 70+ endpoints |
| 02 | `demo/demos/02-crud-lifecycle.sh` | Full create→read→update→delete against Task Forge |
| 03 | `demo/demos/03-file-workflow.sh` | Upload/download/trash/recover via ByteStash |
| 04 | `demo/demos/04-auth-spectrum.sh` | Reader→author→editor→admin against Inkwell |
| 05 | `demo/demos/05-analytics-pipeline.sh` | Bulk insert, range query, aggregation via Pulse |
| 06 | `demo/demos/06-workflow-orchestration.sh` | Trigger ChainLink workflow, poll until done |
| 07 | `demo/demos/07-daemon-proxy.sh` | Start daemon, proxy all 5 systems through Unix socket |
| 08 | `demo/demos/08-mcp-ai-agent.sh` | Start MCP server, tools/list shows 70+ tools, tools/call one live |
| 09 | `demo/demos/09-wasm-transform.sh` | Load WASM plugin, see `"wasm": true` in every response |
| 10 | `demo/demos/10-dsl-chaining.sh` | Starlark script: create task → get project → add comment → mark done |

### AC-D07: `run-all.sh` — One Command, Full Demo
**What:** `cd demo && ./run-all.sh` does everything. Builds Muster binaries, starts all 5 services, waits for health checks, runs all 10 demo scripts, prints summary (`N passed, M failed`), cleans up all processes on exit (trap EXIT). Supports `--keep-running` to leave services up for manual exploration.
**Verification:** Exit code = number of failures. Re-running produces identical results.

### AC-D08: Demo WASM Module
**What:** `demo/wasm/response-transform.wasm` — a compiled WASM module (wazero) that intercepts responses and injects `"wasm": true`. Source at `demo/wasm/transform.go`. Used by demo 09.
**Verification:** `./bin/openapi-cli request GET http://localhost:9801/tasks --wasm-plugin demo/wasm/response-transform.wasm` → response contains `"wasm": true`

### AC-D09: README.md — Quickstart Guide
**What:** `demo/README.md` — 30-second quickstart: prerequisites (Go, Python3, Flask), one command to run, what you'll see. Links to individual demo scripts for exploring.
