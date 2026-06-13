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

# Use Task Forge spec
SPEC="http://localhost:9801/openapi.json"
curl -sf "$SPEC" > /dev/null || fail "Task Forge not running"

echo -n "Starting MCP... "
# MCP reads JSON-RPC from stdin, writes to stdout. Pipe request directly.
REQ='{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
RESP=$(echo "$REQ" | timeout 10 $BIN -spec "$SPEC" 2>/dev/null || true)
TC=$(echo "$RESP" | grep -o '"name"' | wc -l)
[ "$TC" -gt 5 ] || fail "expected 5+ tools, got $TC"
pass "$TC tools"

echo -n "tools/call... "
# Use the actual tool name from Task Forge: post_projects_{project_id}_tasks
# Must supply -base-url so the MCP server knows where to send requests
CALL='{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"post_projects_{project_id}_tasks","arguments":{"project_id":"1","title":"MCP-task"}}}'
RESP2=$(echo "$CALL" | timeout 10 $BIN -spec "$SPEC" -base-url http://localhost:9801 2>/dev/null || true)
echo "$RESP2" | grep -q 'MCP-task' || fail "tools/call"
pass "tools/call succeeded"

pass "MCP complete"
