#!/bin/bash
# Demo 10: Starlark DSL workflow chaining
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }
echo "═══ Demo 10: DSL Chaining ═══"
BIN=./bin/openapi-cli; [ -x "$BIN" ] || fail "$BIN"
curl -sf http://localhost:9801/health > /dev/null || fail "Task Forge"

cat > /tmp/dsl-demo.star << 'STAR'
def workflow():
    h = "http://localhost:9801"
    p = http_post(h + "/projects", body='{"name":"DSL Demo"}').json()
    print("Project:", p["id"])
    t = http_post(h + "/projects/" + p["id"] + "/tasks", body='{"title":"DSL task"}').json()
    print("Task:", t["id"])
    g = http_get(h + "/tasks/" + t["id"]).json()
    print("Got:", g["title"])
    u = http_put(h + "/tasks/" + t["id"], body='{"title":"DSL task","status":"done"}').json()
    print("Status:", u.get("status"))
    return "OK: " + p["id"] + "/" + t["id"]
r = workflow()
print("RESULT:", r)
STAR

echo -n "Running DSL... "
OUT=$($BIN dsl run /tmp/dsl-demo.star 2>&1 || true)
echo "$OUT" | grep -q "RESULT: OK:" || fail "DSL: $OUT"
pass "DSL workflow complete"
rm -f /tmp/dsl-demo.star
pass "DSL complete"
