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
curl -sf http://localhost:9801/health > /dev/null || fail "Task Forge down"

# Start daemon in background
$BIN proxy --backend-url http://localhost:9801 --socket-path "$SOCK" &
DPID=$!
sleep 2
curl -sf --unix-socket "$SOCK" http://localhost/health > /dev/null || { kill $DPID 2>/dev/null; fail "daemon health"; }
pass "daemon on $SOCK"

echo -n "Proxy GET... "
curl -sf --unix-socket "$SOCK" http://localhost/tasks > /dev/null || fail "proxy GET"
pass "task list via proxy"

echo -n "Proxy POST project... "
curl -sf --unix-socket "$SOCK" -X POST http://localhost/projects -H "Content-Type: application/json" -d '{"name":"Daemon Project"}' > /dev/null || fail "proxy POST project"
pass "project created via proxy"

echo -n "Proxy POST task... "
curl -sf --unix-socket "$SOCK" -X POST http://localhost/projects/1/tasks -H "Content-Type: application/json" -d '{"title":"via daemon"}' > /dev/null || fail "proxy POST task"
pass "task created via proxy"

kill $DPID 2>/dev/null; rm -f "$SOCK"
pass "Daemon proxy complete"
