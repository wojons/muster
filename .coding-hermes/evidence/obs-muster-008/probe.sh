#!/usr/bin/env bash
# OBS-MUSTER-008 live .prom proof: drive the real mcp-server over stdio,
# one run against an unreachable upstream (class=upstream), one against a
# 401 stub (class=auth), plus an unknown-tool call (status=error + audit).
set -u
D=/home/kara/.hermes/cache/scratch/obs008-prom
cd "$D"

REQ='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"listThing","arguments":{}}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"no_such_tool","arguments":{}}}'

python3 stub_401.py & STUB=$!
sleep 1

printf '%s\n' "$REQ" | MUSTER_MCP_METRICS_PATH="$D/runA.prom" \
  ./mcp-server --spec "$D/spec.json" --base-url http://127.0.0.1:9 --audit \
  > runA.out 2> runA.err
echo "runA exit=$?"

printf '%s\n' "$REQ" | MUSTER_MCP_METRICS_PATH="$D/runB.prom" \
  ./mcp-server --spec "$D/spec.json" --base-url http://127.0.0.1:40181 --audit \
  > runB.out 2> runB.err
echo "runB exit=$?"

kill $STUB 2>/dev/null
echo "=== runA .prom ==="; cat runA.prom 2>/dev/null
echo "=== runB .prom ==="; cat runB.prom 2>/dev/null
echo "=== tool.call audit lines ==="
grep -h 'tool.call' runA.err runA.out runB.err runB.out 2>/dev/null | head -8
echo "=== runA tools/call responses (truncated) ==="
grep tools/call runA.out 2>/dev/null | head -2 | cut -c1-400
