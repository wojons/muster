#!/bin/bash
# Demo 07: Daemon proxy through Unix socket
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }
echo "═══ Demo 07: Daemon Proxy ═══"
BIN=./bin/openapi-daemon; SOCK=/tmp/muster-demo-daemon.sock
[ -x "$BIN" ] || fail "$BIN not found"

echo -n "Starting daemon... "
$BIN proxy --backend-url http://localhost:9801 --socket-path "$SOCK" &>/dev/null &
DPID=$!; sleep 2
curl -sf --unix-socket "$SOCK" http://localhost/health > /dev/null || fail "health"
pass "daemon on $SOCK"

echo -n "Proxy GET... "
curl -sf --unix-socket "$SOCK" http://localhost/tasks | python3 -c "import sys,json; d=json.load(sys.stdin); exit(0 if isinstance(d,list) else 1)" || fail "proxy"
pass "task list via proxy"

echo -n "Proxy POST... "
curl -sf --unix-socket "$SOCK" -X POST http://localhost/projects/1/tasks -H "Content-Type: application/json" -d '{"title":"via daemon"}' | python3 -c "import sys,json; d=json.load(sys.stdin); exit(0 if d.get('id') else 1)" || fail "create"
pass "created via proxy"

kill $DPID 2>/dev/null; rm -f "$SOCK"
pass "Daemon proxy complete"
