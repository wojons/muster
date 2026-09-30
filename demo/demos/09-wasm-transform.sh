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

# Baseline: the untransformed body, so the assertions below can prove the
# plugin actually ran (REVIEW-MUSTER-6: a plugin that wrote nothing to stdout
# replaced the real body with an EMPTY one and still exited 0).
BASE=$("$BIN" request GET http://localhost:9801/tasks 2>/dev/null || true)
[ -n "$BASE" ] || fail "baseline request produced an empty body"

echo -n "With WASM... "
OUT=$("$BIN" request GET http://localhost:9801/tasks --wasm-plugin "$WASM" 2>/dev/null || true)

[ -n "$OUT" ] || fail "wasm transform produced EMPTY output (plugin wrote nothing to stdout)"
[ "$OUT" != "$BASE" ] || fail "wasm output is byte-identical to the untransformed body (plugin did not run)"
echo "$OUT" | grep -q '"wasm":true' || fail "transformed body is missing the \"wasm\" marker: $(printf '%s' "$OUT" | head -c 80)"
pass "WASM transform emitted $(printf '%s' "$OUT" | wc -c) bytes (baseline $(printf '%s' "$BASE" | wc -c) bytes)"
pass "WASM complete"
