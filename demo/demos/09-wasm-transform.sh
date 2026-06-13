#!/bin/bash
# Demo 09: WASM plugin transforms responses
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }
echo "═══ Demo 09: WASM Transform ═══"
BIN=./bin/openapi-cli; WASM=demo/wasm/response-transform.wasm
[ -x "$BIN" ] || fail "$BIN"; [ -f "$WASM" ] || fail "$WASM"
curl -sf http://localhost:9801/health > /dev/null || fail "Task Forge"

echo -n "With WASM... "
OUT=$($BIN request GET http://localhost:9801/tasks --wasm-plugin "$WASM" 2>/dev/null)
echo "$OUT" | grep -q '"wasm":true' || fail "wasm not found"
pass "wasm=True injected"

echo -n "Without WASM... "
OUT=$($BIN request GET http://localhost:9801/tasks 2>/dev/null)
echo "$OUT" | grep -qv '"wasm":true' || fail "wasm present without plugin"
pass "no wasm without plugin"
pass "WASM complete"
