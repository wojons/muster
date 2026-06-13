# Muster Demo

See Muster in action — 5 dummy products, 10 demo walkthroughs, one command.

## Quickstart

```bash
# Prerequisites
pip install flask        # Python 3.10+
go version               # Go 1.21+

# Run everything
cd demo
./run-all.sh
```

**What happens:** Builds Muster → starts 5 demo services → seeds data → runs 10 demos → prints pass/fail → cleans up.

## What You'll See

```
═══════════════════════════════════════════
 Building Muster
═══════════════════════════════════════════
  ✓ openapi-cli
  ✓ openapi-mcp
  ✓ openapi-daemon

═══════════════════════════════════════════
 Starting Demo Services
═══════════════════════════════════════════
  ✓ Task Forge ready      (:9801)
  ✓ ByteStash ready       (:9802)
  ✓ Inkwell ready         (:9803)
  ✓ Pulse ready           (:9804)
  ✓ ChainLink ready       (:9805)

═══════════════════════════════════════════
 Running Demos
═══════════════════════════════════════════
  ✓ 01-discover-all.sh
  ✓ 02-crud-lifecycle.sh
  ✓ 03-file-workflow.sh
  ✓ 04-auth-spectrum.sh
  ✓ 05-analytics-pipeline.sh
  ✓ 06-workflow-orchestration.sh
  ✓ 07-daemon-proxy.sh
  ✓ 08-mcp-ai-agent.sh
  ✓ 09-wasm-transform.sh
  ✓ 10-dsl-chaining.sh
```

## Keep Services Running

```bash
./run-all.sh --keep-running   # services stay up after demos
```

Then explore manually:
```bash
./bin/openapi-cli discover http://localhost:9801/openapi.json
./bin/openapi-cli request GET http://localhost:9801/tasks
curl http://localhost:9803/health
```

## Demo Services

| Service | Port | Description |
|---------|------|-------------|
| **Task Forge** | 9801 | Project management — projects, tasks, comments, milestones, tags |
| **ByteStash** | 9802 | Cloud file storage — upload, download, trash, recover, share |
| **Inkwell** | 9803 | Blog/CMS — 4 auth roles, draft→publish workflow, comment moderation |
| **Pulse** | 9804 | Metrics dashboard — time-series, aggregations, alerts |
| **ChainLink** | 9805 | Workflow engine — multi-step execution, async polling, webhooks |

Each service is self-contained (`python3 server.py`), serves its own OpenAPI spec at `/openapi.json`, and seeds realistic data on startup.

## Demo Scripts

| # | Script | Shows |
|---|--------|-------|
| 01 | `discover-all` | Discover 70+ endpoints across all 5 services |
| 02 | `crud-lifecycle` | Create→read→update→delete against Task Forge |
| 03 | `file-workflow` | Upload, download, trash, recover via ByteStash |
| 04 | `auth-spectrum` | Reader→author→editor→admin role gates on Inkwell |
| 05 | `analytics` | Bulk insert, range query, aggregation via Pulse |
| 06 | `workflow` | Trigger workflow, poll execution, verify steps on ChainLink |
| 07 | `daemon-proxy` | Unix socket proxy for all services |
| 08 | `mcp-ai-agent` | MCP server — 70+ tools, tools/call live |
| 09 | `wasm-transform` | WASM plugin injects `wasm:true` into responses |
| 10 | `dsl-chaining` | Starlark script chains API calls |

Each script is ~30 lines, self-documenting, with colored ✓ PASS / ✗ FAIL output.

## Running Individual Demos

```bash
# Start services manually
PORT=9801 python3 demo/services/task-forge/server.py &

# Run one demo
bash demo/demos/02-crud-lifecycle.sh
```

## Architecture

```
          ┌──────────────────────────────┐
          │          Muster              │
          │  ┌────────┐  ┌───────────┐   │
          │  │  CLI   │  │ MCP Server│   │
          │  └───┬────┘  └─────┬─────┘   │
          │      │              │         │
          │  ┌───┴──────────────┴───┐    │
          │  │    Daemon Proxy      │    │
          │  └──────────┬───────────┘    │
          └─────────────┼────────────────┘
                        │
    ┌───────────────────┼───────────────────┐
    │       5 Demo Services                  │
    │  ┌─────────┐ ┌─────────┐ ┌──────────┐ │
    │  │Task     │ │ByteStash│ │ Inkwell  │ │
    │  │Forge    │ │ :9802   │ │ :9803    │ │
    │  │:9801    │ │  files  │ │ blog/CMS │ │
    │  └─────────┘ └─────────┘ └──────────┘ │
    │  ┌─────────┐ ┌──────────┐             │
    │  │ Pulse   │ │ChainLink │             │
    │  │ :9804   │ │ :9805    │             │
    │  │ metrics │ │ workflow │             │
    │  └─────────┘ └──────────┘             │
    └───────────────────────────────────────┘
```
