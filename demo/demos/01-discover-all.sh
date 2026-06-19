#!/bin/bash
# Demo 01: Discover all 5 Muster demo services
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }

echo "═══════════════════════════════════════════"
echo " Demo 01: Discover All Services"
echo "═══════════════════════════════════════════"

BIN=./bin/openapi-cli
[ -x "$BIN" ] || fail "$BIN not found — run 'go build -o bin/openapi-cli ./cmd/openapi-cli' first"

SERVICES=(
  "http://localhost:9801/openapi.json|Task Forge"
  "http://localhost:9802/openapi.json|ByteStash"
  "http://localhost:9803/openapi.json|Inkwell"
  "http://localhost:9804/openapi.json|Pulse"
  "http://localhost:9805/openapi.json|ChainLink"
)

TOTAL=0
for entry in "${SERVICES[@]}"; do
  URL="${entry%%|*}"; NAME="${entry##*|}"
  echo ""
  echo "── ${NAME} ──"
  OUT=$("$BIN" discover "$URL" 2>&1) && STATUS=$? || STATUS=$?
  if [ "$STATUS" -ne 0 ]; then
    fail "${NAME} discovery failed: $OUT"
  fi
  COUNT=$(echo "$OUT" | grep -c '^[[:space:]]*[A-Z]\+ ' || true)
  TOTAL=$((TOTAL + COUNT))
  echo "$OUT" | head -8
  echo "  ... ($COUNT operations)"
done

echo ""
pass "Discovered $TOTAL operations across 5 services"
