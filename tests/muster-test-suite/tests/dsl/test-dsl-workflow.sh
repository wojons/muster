#!/usr/bin/env bash
# Test: DSL script execution — Starlark scripts with HTTP builtins
# Tests: openapi-cli dsl run <file> with API interactions
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
BINARY="${PROJECT_DIR}/bin/openapi-cli"
TOTAL=0
PASSED=0
FAILED=0

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

pass() { PASSED=$((PASSED + 1)); TOTAL=$((TOTAL + 1)); echo -e "  ${GREEN}✓${NC} $1"; }
fail() { FAILED=$((FAILED + 1)); TOTAL=$((TOTAL + 1)); echo -e "  ${RED}✗${NC} $1"; }

check_exit() {
    local desc="$1" expected="$2" actual="$3"
    if [ "$actual" -eq "$expected" ]; then
        pass "$desc"
    else
        fail "$desc (exit: $actual, expected: $expected)"
    fi
}

check_contains() {
    local desc="$1" pattern="$2"
    if echo "$3" | grep -q "$pattern"; then
        pass "$desc"
    else
        fail "$desc (expected: $pattern)"
    fi
}

echo "=== Test: DSL Script Execution ==="

# 1. Check binary exists
if [ ! -x "$BINARY" ]; then
    echo "  Building binary..."
    (cd "$PROJECT_DIR" && go build -o "$BINARY" ./cmd/openapi-cli/)
fi

# 2. Start Task Forge on :9800
cd "$PROJECT_DIR"
PYTHONDONTWRITEBYTECODE=1 ./demo/.venv/bin/python ./demo/crud-service/server.py &
TF_PID=$!
sleep 2

# 3. Test a simple DSL script
OUTPUT=$(mktemp)
cat > /tmp/test-dsl-simple.star << 'SCRIPT'
# Simple Starlark test
result = http_get("http://localhost:9800/tasks")
status = result[0]
body = result[1]
err = result[2]
SCRIPT

"$BINARY" dsl run /tmp/test-dsl-simple.star 2>/dev/null > "$OUTPUT" || true
check_contains "DSL: simple script runs" "status = 200" "$(cat "$OUTPUT")"
check_contains "DSL: response body" "body = " "$(cat "$OUTPUT")"

# 4. Test CRUD lifecycle via DSL
cat > /tmp/test-dsl-crud.star << 'SCRIPT'
# Full CRUD lifecycle
# Create a task
r = http_post("http://localhost:9800/tasks", "application/json", '{"title":"DSL test task","done":false}')
create_status = r[0]
create_body = r[1]

# List tasks
r2 = http_get("http://localhost:9800/tasks")
list_status = r2[0]
SCRIPT

"$BINARY" dsl run /tmp/test-dsl-crud.star 2>/dev/null > "$OUTPUT" || true
check_contains "DSL: POST creates task (201)" "create_status = 201" "$(cat "$OUTPUT")"
check_contains "DSL: GET lists tasks (200)" "list_status = 200" "$(cat "$OUTPUT")"
check_contains "DSL: task title in body" "DSL test task" "$(cat "$OUTPUT")"

# 5. Test error handling: GET bad URL
cat > /tmp/test-dsl-error.star << 'SCRIPT'
r = http_get("http://localhost:19876/nonexistent")
status = r[0]
err = r[2]
SCRIPT

"$BINARY" dsl run /tmp/test-dsl-error.star 2>/dev/null > "$OUTPUT" || true
check_contains "DSL: connection error handled" "status = 0" "$(cat "$OUTPUT")"
check_contains "DSL: error message returned" "err = " "$(cat "$OUTPUT")"

# Cleanup
kill "$TF_PID" 2>/dev/null || true
rm -f "$OUTPUT" /tmp/test-dsl-simple.star /tmp/test-dsl-crud.star /tmp/test-dsl-error.star

echo ""
echo "Results: $PASSED/$TOTAL passed, $FAILED failed"
exit $FAILED
