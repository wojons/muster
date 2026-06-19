# Muster Demo

Self-contained demo that exercises every Muster surface against 5 mock services.

## Run it

```bash
cd demo && ./run-all.sh
```

That's it. It builds all 3 binaries, starts 5 services, runs 10 demo scripts, reports pass/fail, and cleans up.

## What's in here

```
demo/
├── run-all.sh              # One command to run everything
├── services/               # 5 mock Flask APIs
│   ├── task-forge/         # Project management (port 9801)
│   ├── bytestash/          # File storage (port 9802)
│   ├── inkwell/            # Blog/CMS with auth (port 9803)
│   ├── pulse/              # Metrics engine (port 9804)
│   └── chainlink/          # Workflow automation (port 9805)
├── demos/                  # 10 demo scripts
│   ├── 01-discover-all.sh          # Discover all 70+ endpoints
│   ├── 02-crud-lifecycle.sh        # Create → Read → Update → Delete
│   ├── 03-file-workflow.sh         # Upload → Download → Trash → Recover
│   ├── 04-auth-spectrum.sh         # Reader → Author → Editor → Admin
│   ├── 05-analytics-pipeline.sh    # Bulk insert → Range query → Aggregate
│   ├── 06-workflow-orchestration.sh # Trigger → Poll → Verify
│   ├── 07-daemon-proxy.sh          # Unix socket proxy for all services
│   ├── 08-mcp-ai-agent.sh          # MCP tools/list + tools/call
│   ├── 09-wasm-transform.sh        # WASM plugin response injection
│   └── 10-dsl-chaining.sh          # Starlark script: task → update → list
└── dsl/
    └── workflow.star        # Starlark DSL example script
```

## Prerequisites

- Go 1.23+
- Python 3
- Flask: `pip install flask`

## Expected output

```
Passed: 23
All 23 checks passed! 🎉
```
