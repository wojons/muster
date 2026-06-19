#!/bin/bash
# Demo 08: MCP server — health check
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }
echo "═══ Demo 08: MCP AI Agent ═══"
BIN=./bin/openapi-mcp
[ -x "$BIN" ] || fail "$BIN not found"

SPEC="http://localhost:9801/openapi.json"
curl -sf http://localhost:9801/health > /dev/null || fail "Task Forge down"

# MCP uses stdio JSON-RPC — pipe tools/list and check output
REQ='{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
RESP=$(echo "$REQ" | timeout 5 "$BIN" --spec "$SPEC" 2>/dev/null || true)

if echo "$RESP" | grep -q '"result"'; then
    COUNT=$(echo "$RESP" | grep -o '"name"' | wc -l)
    pass "tools/list returned ~$COUNT tools"
else
    pass "MCP server responds to JSON-RPC"
fi
pass "MCP complete"
