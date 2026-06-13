#!/bin/bash
# Demo 08: MCP server — tools/list + tools/call
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }
echo "═══ Demo 08: MCP AI Agent ═══"
BIN=./bin/openapi-mcp
[ -x "$BIN" ] || fail "$BIN not found"

SPECS=""; for p in 9801 9802 9803 9804 9805; do curl -sf "http://localhost:$p/health" > /dev/null || fail ":$p down"; SPECS="$SPECS --spec http://localhost:$p/openapi.json"; done

echo -n "Starting MCP... "
$BIN $SPECS &>/dev/null &
MPID=$!; sleep 2

echo -n "tools/list... "
REQ='{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
TC=$(echo "$REQ" | timeout 10 $BIN $SPECS 2>/dev/null | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('result',{}).get('tools',[])))" 2>/dev/null || echo "0")
[ "$TC" -gt 50 ] || fail "expected 50+ tools, got $TC"
pass "$TC tools"

echo -n "tools/call... "
CALL='{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"createTask","arguments":{"title":"MCP-task"}}}'
echo "$CALL" | timeout 10 $BIN $SPECS 2>/dev/null | python3 -c "
import sys,json
d=json.load(sys.stdin)
c=d.get('result',{}).get('content',[{}])
inner=json.loads(c[0].get('text','{}'))
exit(0 if inner.get('id') else 1)
" 2>/dev/null || fail "tools/call"
pass "tools/call succeeded"

kill $MPID 2>/dev/null
pass "MCP complete"
