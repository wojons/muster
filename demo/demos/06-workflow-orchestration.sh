#!/bin/bash
# Demo 06: Workflow orchestration — ChainLink
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }
echo "═══ Demo 06: Workflow — ChainLink ═══"
HOST=http://localhost:9805
curl -sf "$HOST/health" > /dev/null || fail "ChainLink not running"

echo -n "List workflows... "
WF=$(curl -sf "$HOST/workflows" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
[ "$WF" -ge 1 ] || fail "workflows"
pass "$WF workflow(s)"

echo -n "Execute... "
EXEC=$(curl -sf -X POST "$HOST/workflows/1/execute")
EID=$(echo "$EXEC" | python3 -c "import sys,json; print(json.load(sys.stdin)['execution_id'])")
pass "execution $EID"

echo -n "Polling"; for i in $(seq 1 15); do sleep 1; S=$(curl -sf "$HOST/executions/$EID" | python3 -c "import sys,json; print(json.load(sys.stdin)['status'])"); echo -n " $S"; [ "$S" = "done" ] && break; done; echo ""
STEPS=$(curl -sf "$HOST/executions/$EID" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('step_results',[])))")
[ "$STEPS" -gt 0 ] || fail "steps"
pass "all $STEPS steps complete"
pass "Workflow complete"
