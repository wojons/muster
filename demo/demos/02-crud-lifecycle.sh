#!/bin/bash
# Demo 02: Full CRUD lifecycle against Task Forge
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }

echo "═══════════════════════════════════════════"
echo " Demo 02: CRUD Lifecycle — Task Forge"
echo "═══════════════════════════════════════════"

BIN=./bin/openapi-cli
HOST=http://localhost:9801
curl -sf "$HOST/health" > /dev/null || fail "Task Forge not running on :9801"

# Create project
echo -n "Creating project... "
RESP=$($BIN request POST "$HOST/projects" -H "Content-Type: application/json" -b '{"name":"Demo Project","description":"Created by Muster demo"}')
PID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
[ -n "$PID" ] || fail "project create"
pass "project created (id=$PID)"

# Create 3 tasks
for title in "Set up repository" "Write documentation" "Add CI pipeline"; do
  echo -n "Creating: $title... "
  $BIN request POST "$HOST/projects/$PID/tasks" -H "Content-Type: application/json" -b "{\"title\":\"$title\"}" | python3 -c "import sys,json; d=json.load(sys.stdin); exit(0 if d.get('id') else 1)"
done
pass "3 tasks created"

# List tasks
echo -n "Listing... "
COUNT=$($BIN request GET "$HOST/projects/$PID/tasks" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
[ "$COUNT" -eq 3 ] || fail "expected 3, got $COUNT"
pass "$COUNT tasks listed"

# Update
TID1=$($BIN request GET "$HOST/projects/$PID/tasks" | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['id'])")
echo -n "Updating to done... "
$BIN request PATCH "$HOST/tasks/$TID1" -H "Content-Type: application/json" -b '{"status":"done"}' | python3 -c "import sys,json; d=json.load(sys.stdin); exit(0 if d.get('status')=='done' else 1)"
pass "task $TID1 → done"

# Delete
echo -n "Deleting... "
$BIN request DELETE "$HOST/tasks/$TID1" | python3 -c "import sys,json; d=json.load(sys.stdin); exit(0 if d.get('deleted') else 1)"
pass "deleted"

# Verify 404
CODE=$(curl -s -o /dev/null -w "%{http_code}" "$HOST/tasks/$TID1")
[ "$CODE" = "404" ] || fail "expected 404, got $CODE"
pass "deletion confirmed (404)"
echo ""
pass "CRUD lifecycle complete"
