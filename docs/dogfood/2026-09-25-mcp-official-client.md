# Dogfood Integration Report — 2026-09-25 (run 4): the MCP server driven by the OFFICIAL MCP client

**Angle (per skill — prior runs covered the entry surface, management plane,
reload, and protocol seams; none had ever connected a real MCP client):**
drive `openapi-mcp` with the official MCP Python SDK (`mcp` package,
`ClientSession` + stdio transport) — the closest thing to what Claude
Desktop / Cursor actually do — and judge the server against the MCP spec,
not against hand-rolled JSON-RPC.

## What a real integration looks like (working example)

```bash
# terminal 1: any OpenAPI-speaking service
cd demo/services/task-forge && python3 server.py          # serves /openapi.json + REST on :9801

# terminal 2: the client side (what an agent harness runs)
uv venv /tmp/mcp-client && uv pip install --python /tmp/mcp-client/bin/python mcp
cat > /tmp/drive.py <<'PY'
import asyncio
from mcp import ClientSession, StdioServerParameters
from mcp.client.stdio import stdio_client

async def main():
    p = StdioServerParameters(
        command="/path/to/bin/openapi-mcp",
        args=["--spec", "http://127.0.0.1:9801/openapi.json"],
        env={"HOME": "/tmp/df-home", "PATH": "/usr/bin:/bin"},
    )
    async with stdio_client(p) as (r, w):
        async with ClientSession(r, w) as s:
            await asyncio.wait_for(s.initialize(), timeout=30)   # ⚠ see DF-031
            tools = (await s.list_tools()).tools                 # 22 tools
            res = await s.call_tool("post_projects", {"name": "my project"})
            print(res.content[0].text)                           # created row JSON
asyncio.run(main())
PY
python3 /tmp/drive.py
```

Notes for future agents:
- `env=` in `StdioServerParameters` REPLACES the environment — pass PATH.
- The SDK is currently snake_case (`init.server_info`, `t.input_schema`,
  `res.is_error`); older camelCase snippets in the wild will crash.
- Body fields arrive as TOP-LEVEL schema properties (not a `body` object):
  call `patch_tasks_{task_id}` with `{"task_id": N, "priority": "high"}`,
  NOT `{"task_id": N, "body": "{...}"}`. Both compile-time guessable forms
  were tried; only the flat one mutates the row (proved against the mock).

## What passed (the product is real)

| Check | Result |
|---|---|
| initialize via official SDK | OK (server_info.name=openapi-mcp) |
| tools/list | 22 tools, every one carries a JSON-schema inputSchema |
| create → read → nested create → patch → list → delete → 404-after-delete | full round-trip through tools/call, zero protocol errors |
| error paths | missing id → `is_error=True` + `Error: HTTP 404: {"error":"task not found"}`; unknown tool → MCP `is_error` result |
| schema shapes | required body fields present for non-templated ops (post_projects: required=[name]) |
| perf | full session (spawn+init+tools/list) 21–27 ms (n=12, median 23); CLI request 9.6 ms ± 0.5; binary startup 10.7 ms ± 0.6 |

## What failed (findings → board DF-031..035)

1. **DF-031 (P1) — wire violation that CRASHES the official SDK.** After
   `notifications/initialized` the server writes `{"jsonrpc":"2.0","result":null}`
   — no `id`, no `method`: neither a response nor a notification. The
   official SDK throws 7 pydantic validation errors. Raw-probe evidence in
   the board row. Spec: servers MUST NOT reply to notifications. Prior runs
   missed this because hand-rolled probes only sent requests, never the
   notification. Site: pkg/mcp/server.go handleRequest (~112–130).
2. **DF-032 (P2) — `capabilities:{}` while serving 22 tools**; serverInfo
   version hardcoded "1.0.0" vs real v0.1.1-151. Strict clients gate on the
   advertised capability and would hide/refuse every tool.
3. **DF-033 (P2) — 16/22 tool names contain literal `{curly}` path
   templates**, outside the MCP Tool.name conventional charset.
4. **DF-034 (P1) — templated tools omit their path params from
   inputSchema** (get_projects_{project_id}: zero properties). Works when
   the params are passed anyway, but the advertised contract is unusable by
   schema-validating clients (Claude Desktop strict mode, conformance
   suites) for 16 of 22 tools.
5. **DF-035 (P1) — Makefile git-stamp fallback breaks tarball-style
   installs**: in a zero-commit repo `git rev-parse --abbrev-ref HEAD`
   prints `HEAD` AND fails → `BRANCH="HEAD unknown"` → bare word in
   -ldflags → linker prints usage → build dead after the full module
   download (96 s measured fresh). Pure no-.git tree builds fine (3 s).

## Diagnostic trail (how these were found — the right way)

- Official client first → crash INSIDE the SDK's pydantic parsing on the
  server's stray line, but the union error text listed every variant →
  wrote a 40-line raw stdio probe (send initialize, initialized, tools/list;
  dump every stdout line) → the spec-violating line is visible verbatim and
  reproducible without the SDK.
- Schema-vs-behavior split: a "PATCH ignored my body" fail turned out to be
  TWO separate things — my wrong arg shape (body wrapper) and, separately,
  the missing path-param in the schema (DF-034, real). Direct curl against
  the mock isolated the mock from muster; a live create+patch through the
  MCP tool proved the wire path works when called per its actual schema.
- Distinguish "your bug" from "their bug" before filing: the 09-25 skill
  pitfalls (`--help` before filing; EXIT=PIPESTATUS) both applied here.
- Install leg: fresh agent on bunker-mvp (las-01/03/04 unreachable, las-02
  bunkerd crash-looping) — documented path (toolchain bootstrap + make
  build-cli) reproduced DF-035 on the first pass and PASSED on the tarball
  path; discover/generate/typed-suggestion/generic-request all verified on
  the fresh install before destroy.
