#!/bin/bash
# Demo 05: Analytics pipeline — Pulse
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }
echo "═══ Demo 05: Analytics — Pulse ═══"
HOST=http://localhost:9804
curl -sf "$HOST/health" > /dev/null || fail "Pulse not running"

echo -n "Bulk insert 10 points... "
POINTS="["; for i in $(seq 0 9); do TS=$(date -d "-$((90-i*10)) minutes" -u +%Y-%m-%dT%H:%M:%SZ); VAL=$(awk "BEGIN {printf \"%.2f\",50+30*sin($i*0.7)}"); [ $i -gt 0 ] && POINTS+=","; POINTS+="{\"timestamp\":\"$TS\",\"value\":$VAL}"; done; POINTS+="]"
[ "$(curl -s -o /dev/null -w '%{http_code}' -X POST $HOST/series/1/data -H 'Content-Type: application/json' -d "$POINTS")" = "201" ] || fail "insert"
pass "10 points"

echo -n "Range query... "
SINCE=$(date -d "2 hours ago" -u +%Y-%m-%dT%H:%M:%SZ)
COUNT=$(curl -sf "$HOST/series/1/data?since=$SINCE" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
[ "$COUNT" -ge 10 ] || fail "range: $COUNT"
pass "$COUNT points"

echo -n "Aggregation... "
AVG=$(curl -sf "$HOST/series/1/data?aggregate=avg&since=$SINCE" | python3 -c "import sys,json; d=json.load(sys.stdin); v=d.get('value'); exit(0 if v else 1)")
pass "avg=$AVG"

echo -n "Dashboards... "
DB=$(curl -sf "$HOST/dashboards" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
[ "$DB" -ge 1 ] || fail "dashboards"
pass "$DB dashboard(s)"
pass "Analytics complete"
