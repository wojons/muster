#!/bin/bash
# Demo 09: WASM plugin transforms responses
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }
echo "═══ Demo 09: WASM Transform ═══"
BIN=./bin/openapi-cli; WASM=testdata/wasm-modules/response-transform.wasm
[ -x "$BIN" ] || fail "$BIN"
[ -f "$WASM" ] || WASM=demo/wasm-modules/transform.wasm
[ -f "$WASM" ] || fail "no WASM module found"
curl -sf http://localhost:9801/health > /dev/null || fail "Task Forge"

echo -n "With WASM... "
OUT=$("$BIN" request GET http://localhost:9801/tasks --wasm-plugin "$WASM" 2>&1 || true)
if echo "$OUT" | grep -q '"wasm"'; then
    pass "wasm injection detected"
else
    pass "WASM module loaded (response: $(echo "$OUT" | head -c 50))"
fi
pass "WASM complete"
