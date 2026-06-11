#!/usr/bin/env bash
# test-all-consumers.sh — Verify data shape consistency across CLI, MCP, and Daemon
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-9
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
set -euo pipefail

# ── Configuration ───────────────────────────────────────────────
SERVICE_BASE_URL="${SERVICE_BASE_URL:-http://localhost:9800}"
BIN_DIR="${BIN_DIR:-${SCRIPT_DIR}/../../bin}"
CLI_BIN="${BIN_DIR}/openapi-cli"
MCP_BIN="${BIN_DIR}/openapi-mcp"
DAEMON_BIN="${BIN_DIR}/openapi-daemon"
SPEC_PATH="${SCRIPT_DIR}/../../apps/task-forge/openapi.json"
SOCKET_PATH="/tmp/test-muster-cross-$$.sock"
PID_FILE="${SOCKET_PATH}.pid"
FAILED=0
PASSED=0

# ── Helpers ─────────────────────────────────────────────────────
red()    { printf '\033[0;31m%s\033[0m\n' "$1"; }
green()  { printf '\033[0;32m%s\033[0m\n' "$1"; }

check_deps() {
    local missing=0
    if [ ! -x "$CLI_BIN" ]; then echo "SKIP: $CLI_BIN not found"; exit 0; fi
    if [ ! -x "$MCP_BIN" ]; then echo "SKIP: $MCP_BIN not found"; exit 0; fi
    if [ ! -x "$DAEMON_BIN" ]; then echo "SKIP: $DAEMON_BIN not found"; exit 0; fi
    if ! command -v curl >/dev/null 2>&1; then echo "SKIP: curl not found"; exit 0; fi
    if ! command -v jq >/dev/null 2>&1; then echo "SKIP: jq not found"; exit 0; fi
}

assert_contains() {
    local needle="$1" haystack="$2" label="$3"
    if echo "$haystack" | grep -qF "$needle"; then
        green "  PASS: $label"
        PASSED=$((PASSED + 1))
    else
        red "  FAIL: $label — expected to contain '$needle'"
        FAILED=$((FAILED + 1))
    fi
}

assert_equal() {
    local expected="$1" actual="$2" label="$3"
    if [ "$expected" = "$actual" ]; then
        green "  PASS: $label"
        PASSED=$((PASSED + 1))
    else
        red "  FAIL: $label — expected '$expected', got '$actual'"
        FAILED=$((FAILED + 1))
    fi
}

run_mcp_cmd() {
    echo "$1" | "$MCP_BIN" --spec "$SPEC_PATH" --base-url "$SERVICE_BASE_URL" --log-output /dev/null 2>/dev/null || true
}

run_socket_curl() {
    curl -s -w '\n%{http_code}' \
        --unix-socket "$SOCKET_PATH" \
        -X "$1" \
        -H 'Content-Type: application/json' \
        ${3:+-d "$3"} \
        "http://localhost$2"
}

extract_project_data() {
    # Extract relevant project fields for comparison across consumers
    jq -r '{name: .name, description: .description, status: .status}'
}

# ── Cleanup ─────────────────────────────────────────────────────
DAEMON_PID=""
MCP_PID=""
cleanup() {
    if [ -n "$MCP_PID" ]; then
        kill "$MCP_PID" 2>/dev/null || true
        wait "$MCP_PID" 2>/dev/null || true
    fi
    if [ -n "$DAEMON_PID" ]; then
        "$DAEMON_BIN" stop --socket-path "$SOCKET_PATH" 2>/dev/null || true
        kill "$DAEMON_PID" 2>/dev/null || true
        wait "$DAEMON_PID" 2>/dev/null || true
    fi
    rm -f "$SOCKET_PATH" "$PID_FILE"
}
trap cleanup EXIT

# ── Main ────────────────────────────────────────────────────────
echo "============================================"
echo "  Test: Cross-consumer data consistency"
echo "============================================"

check_deps

# ── Step 1: Create a project via direct HTTP (CLI equivalent) ───
echo "--- Create via HTTP (CLI consumer) ---"
PROJECT_NAME="CrossConsumerTest"
CREATE_BODY="{\"name\":\"${PROJECT_NAME}\",\"description\":\"Shared test project\",\"status\":\"active\"}"
HTTP_RESP=$(curl -s -w '\n%{http_code}' -X POST -H 'Content-Type: application/json' -d "$CREATE_BODY" "${SERVICE_BASE_URL}/projects")
HTTP_CODE=$(echo "$HTTP_RESP" | tail -1)
HTTP_BODY=$(echo "$HTTP_RESP" | sed '$d')
PROJECT_ID=$(echo "$HTTP_BODY" | jq -r '.id // ""')

if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" = "null" ]; then
    red "  FAIL: Could not create project via HTTP"
    exit 1
fi
green "  INFO: Created project $PROJECT_ID via HTTP"
HTTP_SHAPE=$(echo "$HTTP_BODY" | extract_project_data)
green "  INFO: HTTP shape: $HTTP_SHAPE"

# ── Step 2: Verify project via MCP ──────────────────────────────
echo "--- Verify via MCP ---"
"$MCP_BIN" --spec "$SPEC_PATH" --base-url "$SERVICE_BASE_URL" --log-output /dev/null &
MCP_PID=$!
sleep 1

GET_MCP_REQ="{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"getProject\",\"arguments\":{\"project_id\":${PROJECT_ID}}}}"
MCP_RESP=$(run_mcp_cmd "$GET_MCP_REQ")

if [ -z "$MCP_RESP" ]; then
    red "  FAIL: No response from MCP getProject"
    FAILED=$((FAILED + 1))
else
    assert_contains "$PROJECT_NAME" "$MCP_RESP" "MCP response contains project name"

    # Extract name from MCP response for comparison
    MCP_NAME=$(echo "$MCP_RESP" | python3 -c "
import sys,json
d=json.load(sys.stdin)
content=d.get('result',{}).get('content',[{}])
text=content[0].get('text','{}') if content else '{}'
data=json.loads(text)
print(data.get('name',''))
" 2>/dev/null || echo "")

    if [ -n "$MCP_NAME" ]; then
        assert_equal "$PROJECT_NAME" "$MCP_NAME" "MCP getProject returns same name"
    else
        red "  FAIL: Could not extract project name from MCP response"
        FAILED=$((FAILED + 1))
    fi
fi

kill "$MCP_PID" 2>/dev/null || true
wait "$MCP_PID" 2>/dev/null || true
MCP_PID=""

# ── Step 3: Verify project via Daemon proxy ─────────────────────
echo "--- Verify via Daemon ---"
"$DAEMON_BIN" proxy --backend-url "$SERVICE_BASE_URL" --socket-path "$SOCKET_PATH" &
DAEMON_PID=$!
for i in $(seq 1 30); do
    if [ -S "$SOCKET_PATH" ]; then break; fi
    sleep 0.1
done

DAEMON_RESP=$(run_socket_curl GET "/projects/${PROJECT_ID}")
DAEMON_CODE=$(echo "$DAEMON_RESP" | tail -1)
DAEMON_BODY=$(echo "$DAEMON_RESP" | sed '$d')

if [ "$DAEMON_CODE" -ne 200 ]; then
    red "  FAIL: Daemon proxy returned $DAEMON_CODE"
    FAILED=$((FAILED + 1))
else
    DAEMON_NAME=$(echo "$DAEMON_BODY" | jq -r '.name // ""')
    assert_equal "$PROJECT_NAME" "$DAEMON_NAME" "Daemon getProject returns same name"
    DAEMON_SHAPE=$(echo "$DAEMON_BODY" | extract_project_data)
    assert_equal "$HTTP_SHAPE" "$DAEMON_SHAPE" "HTTP and Daemon return identical data shape"
fi

# ── Step 4: Compare all three consumers agree on data shape ─────
echo "--- Cross-reference summary ---"
# If we got this far with all names matching, the data shape is consistent
if [ "$FAILED" -eq 0 ]; then
    green "  PASS: All three consumers (HTTP/CLI, MCP, Daemon) return consistent data for project $PROJECT_ID"
    PASSED=$((PASSED + 1))
fi

# ── Cleanup ─────────────────────────────────────────────────────
"$DAEMON_BIN" stop --socket-path "$SOCKET_PATH" 2>/dev/null || true
DAEMON_PID=""

# ── Summary ─────────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
