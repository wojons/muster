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
├── auth-service/           # Bearer-token acceptance mock (port 9803)
├── crud-service/           # CRUD acceptance mock (port 9800)
├── spec-provider/          # Serves a Petstore spec for discover tests (port 9801)
└── dsl/
    └── workflow.star        # Starlark DSL example script
```

## Service launch contracts

Every demo service is a single `server.py` that resolves its listen port in
this order (first match wins):

1. `argv[1]` — an explicit positional port (e.g. `server.py 9910`)
2. `PORT` environment variable — how `run-all.sh` starts services (`PORT=$PORT python3 server.py`)
3. the service's documented default port (table below)

An invalid or extra argument is rejected loudly: exit code 2 plus a
`usage:` line on stderr. Every service also serves `GET /health`
(`200 {"status":"healthy",...}`) for readiness checks.

| Service | Path | Default port | Auth |
|---|---|---|---|
| Task Forge | `demo/services/task-forge/server.py` | 9801 | none |
| ByteStash | `demo/services/bytestash/server.py` | 9802 | none |
| Inkwell | `demo/services/inkwell/server.py` | 9803 | Bearer token (`reader-token`, `author-token`, `editor-token`, `admin-token` — role-gated) |
| Pulse | `demo/services/pulse/server.py` | 9804 | none |
| ChainLink | `demo/services/chainlink/server.py` | 9805 | none |
| Auth Service | `demo/auth-service/server.py` | 9803 | Bearer `demo-token-12345` (`POST /token` mints it) |
| CRUD Service | `demo/crud-service/server.py` | 9800 | none |
| Spec Provider | `demo/spec-provider/server.py` | 9801 | none |

`demo/services/transport-fixture/` is a Go program, not a Flask service: the
HTTP/2 and HTTP/3 transport demos build and run it (`go run ./demo/services/transport-fixture
[-cert-dir DIR]`); it picks its own loopback ports and prints them as `KEY value`
lines followed by `READY`. It takes no port argument.

The launch-contract suite verifies all of this live:

```bash
bash demo/test-suites/07-launch-contracts.sh
```

## Prerequisites

- Go 1.23+
- Python 3 with Flask. Flask is only guaranteed inside the repo virtualenv —
  `run-all.sh` uses `.venv/bin/python3` when present and falls back to `python3`
  on `PATH`. A bare system Python usually lacks Flask (`ModuleNotFoundError: flask`).
- Flask: `pip install flask` (or create the repo venv: `python3 -m venv .venv && .venv/bin/pip install flask`)
- `demo/test-suites/07-launch-contracts.sh` accepts `DEMO_PYTHON=/path/to/python`
  to test against an interpreter that has Flask installed.

## Expected output

```
Passed: 23
All 23 checks passed! 🎉
```
