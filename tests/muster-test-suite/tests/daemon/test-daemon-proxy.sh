#!/usr/bin/env bash
# test-daemon-proxy.sh — Start daemon proxying task-forge, curl through Unix socket for CRUD
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-8
set -euo pipefail

# ── Configuration ───────────────────────────────────────────────
SERVICE_BASE_URL="${SERVICE_BASE_URL:-http://localhost:9800}"
BIN_DIR="${BIN_DIR:-${SCRIPT_DIR}/../../bin}"
DAEMON_BIN="${BIN_DIR}/openapi-daemon"
SOCKET_PATH="/tmp/test-muster-daemon-$$.sock"
PID_FILE="${SOCKET_PATH}.pid"
FAILED=0
PASSED=0

# ── Helpers ─────────────────────────────────────────────────────
red()    { printf '\033[0;31m%s\033[0m\n' "$1"; }
green()  { printf '\033[0;32m%s\033[0m\n' "$1"; }

check_deps() {
    if [ ! -x "$DAEMON_BIN" ]; then
        echo "SKIP: $DAEMON_BIN not found or not executable"; exit 0
    fi
    if ! command -v curl >/dev/null 2>&1; then
        echo "SKIP: curl not found"; exit 0
    fi
    if ! command -v jq >/dev/null 2>&1; then
        echo "SKIP: jq not found"; exit 0
    fi
}

assert_status() {
    local expected="$1" actual="$2" label="$3"
    if [ "$actual" -eq "$expected" ]; then
        green "  PASS: $label"
        PASSED=$((PASSED + 1))
    else
        red "  FAIL: $label — expected $expected, got $actual"
        FAILED=$((FAILED + 1))
    fi
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

run_socket_curl() {
    # Make an HTTP request through the Unix socket
    # Usage: run_socket_curl <method> <path> [body]
    local method="$1" path="$2" body="${3:-}"
    if [ -n "$body" ]; then
        curl -s -w '\n%{http_code}' \
            --unix-socket "$SOCKET_PATH" \
            -X "$method" \
            -H 'Content-Type: application/json' \
            -d "$body" \
            "http://localhost${path}"
    else
        curl -s -w '\n%{http_code}' \
            --unix-socket "$SOCKET_PATH" \
            -X "$method" \
            "http://localhost${path}"
    fi
}

# ── Cleanup ─────────────────────────────────────────────────────
DAEMON_PID=""
cleanup() {
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
echo "  Test: daemon proxy through Unix socket"
echo "============================================"

check_deps

# ── Step 1: Start daemon ────────────────────────────────────────
echo "--- Starting Daemon ---"
"$DAEMON_BIN" proxy --backend-url "$SERVICE_BASE_URL" --socket-path "$SOCKET_PATH" &
DAEMON_PID=$!
green "  INFO: Daemon started with PID $DAEMON_PID"

# Wait for socket to appear (with timeout)
for i in $(seq 1 30); do
    if [ -S "$SOCKET_PATH" ]; then
        green "  INFO: Socket $SOCKET_PATH is ready"
        break
    fi
    sleep 0.1
done

if [ ! -S "$SOCKET_PATH" ]; then
    red "  FAIL: Socket did not appear after 3s"
    FAILED=$((FAILED + 1))
    exit 1
fi

# ── Step 2: Create a project through daemon proxy ───────────────
echo "--- Daemon Proxy: Create Project ---"
CREATE_BODY='{"name":"Daemon Test Project","description":"Created via daemon proxy","status":"active"}'
CREATE_RESP=$(run_socket_curl POST /projects "$CREATE_BODY")
CREATE_CODE=$(echo "$CREATE_RESP" | tail -1)
CREATE_BODY_ONLY=$(echo "$CREATE_RESP" | sed '$d')
assert_status 201 "$CREATE_CODE" "POST /projects via daemon returns 201"
PROXY_PROJECT_ID=$(echo "$CREATE_BODY_ONLY" | jq -r '.id // ""')
assert_contains "Daemon Test Project" "$CREATE_BODY_ONLY" "Daemon-proxied project name matches"

if [ -z "$PROXY_PROJECT_ID" ] || [ "$PROXY_PROJECT_ID" = "null" ]; then
    red "  FAIL: Could not extract project ID — skipping remaining tests"
    FAILED=$((FAILED + 1))
    exit 1
fi

# ── Step 3: Search for project across paginated pages ───────────
echo "--- Daemon Proxy: Search Created Project ---"
LIST_RESP=$(run_socket_curl GET "/projects")
LIST_CODE=$(echo "$LIST_RESP" | tail -1)
assert_status 200 "$LIST_CODE" "GET /projects via daemon returns 200"

# Search across paginated pages for the created project
FOUND=false
for page in 1 2 3 4; do
    PAGE_RESP=$(run_socket_curl GET "/projects?page=${page}&limit=20")
    PAGE_BODY=$(echo "$PAGE_RESP" | sed '$d')
    if echo "$PAGE_BODY" | grep -qF "Daemon Test Project"; then
        FOUND=true
        break
    fi
done
if $FOUND; then
    green "  PASS: Daemon Test Project found in list across pages"
    PASSED=$((PASSED + 1))
else
    red "  FAIL: Daemon Test Project not found in any paginated page"
    FAILED=$((FAILED + 1))
fi

# ── Step 4: Compare with direct HTTP response ───────────────────
echo "--- Direct HTTP: Compare ---"
DIRECT_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/projects")
assert_status 200 "$DIRECT_CODE" "Direct GET /projects returns 200"

# Search across pages directly
DIRECT_FOUND=false
for page in 1 2 3 4; do
    DIRECT_LIST=$(curl -s "${SERVICE_BASE_URL}/projects?page=${page}&limit=20")
    if echo "$DIRECT_LIST" | grep -qF "Daemon Test Project"; then
        DIRECT_FOUND=true
        break
    fi
done
if $DIRECT_FOUND; then
    green "  PASS: Direct HTTP also returns the project"
    PASSED=$((PASSED + 1))
else
    red "  FAIL: Direct HTTP project not found in any paginated page"
    FAILED=$((FAILED + 1))
fi

# Cross-reference: same ID appears in both
if [ -n "$PROXY_PROJECT_ID" ]; then
    for page in 1 2 3 4; do
        DIRECT_LIST=$(curl -s "${SERVICE_BASE_URL}/projects?page=${page}&limit=20")
        DIRECT_HAS_ID=$(echo "$DIRECT_LIST" | jq -r ".data[] | select(.id == ${PROXY_PROJECT_ID}) | .name" 2>/dev/null)
        if [ -n "$DIRECT_HAS_ID" ]; then break; fi
    done
    if [ "$DIRECT_HAS_ID" = "Daemon Test Project" ]; then
        green "  PASS: Same project ID ${PROXY_PROJECT_ID} visible via daemon and direct HTTP"
        PASSED=$((PASSED + 1))
    else
        red "  FAIL: Project ID ${PROXY_PROJECT_ID} not found in direct HTTP response"
        FAILED=$((FAILED + 1))
    fi
fi

# ── Step 5: Verify a GET by ID through proxy ────────────────────
echo "--- Daemon Proxy: Get Project ---"
GET_RESP=$(run_socket_curl GET "/projects/${PROXY_PROJECT_ID}")
GET_CODE=$(echo "$GET_RESP" | tail -1)
GET_BODY=$(echo "$GET_RESP" | sed '$d')
assert_status 200 "$GET_CODE" "GET /projects/${PROXY_PROJECT_ID} via daemon returns 200"
DAEMON_PROJECT_NAME=$(echo "$GET_BODY" | jq -r '.name // ""')

# Direct HTTP get for comparison
DIRECT_GET=$(curl -s "${SERVICE_BASE_URL}/projects/${PROXY_PROJECT_ID}")
DIRECT_GET_NAME=$(echo "$DIRECT_GET" | jq -r '.name // ""')

if [ "$DAEMON_PROJECT_NAME" = "$DIRECT_GET_NAME" ]; then
    green "  PASS: Daemon and direct HTTP return identical project name: $DAEMON_PROJECT_NAME"
    PASSED=$((PASSED + 1))
else
    red "  FAIL: Daemon returned '$DAEMON_PROJECT_NAME', direct returned '$DIRECT_GET_NAME'"
    FAILED=$((FAILED + 1))
fi

# ── Step 6: Delete through proxy ────────────────────────────────
echo "--- Daemon Proxy: Delete Project ---"
DELETE_CODE=$(run_socket_curl DELETE "/projects/${PROXY_PROJECT_ID}" | tail -1)
assert_status 200 "$DELETE_CODE" "DELETE /projects/${PROXY_PROJECT_ID} via daemon returns 200"

# ── Step 7: Verify 404 after delete through proxy ──────────────
NOTFOUND_CODE=$(run_socket_curl GET "/projects/${PROXY_PROJECT_ID}" | tail -1)
assert_status 404 "$NOTFOUND_CODE" "GET via daemon returns 404 after delete"

# ── Step 8: Stop daemon ─────────────────────────────────────────
echo "--- Stopping Daemon ---"
"$DAEMON_BIN" stop --socket-path "$SOCKET_PATH" 2>/dev/null || true
DAEMON_PID=""
rm -f "$SOCKET_PATH" "$PID_FILE"
sleep 0.5
if [ ! -S "$SOCKET_PATH" ]; then
    green "  PASS: Daemon stopped and socket cleaned up"
    PASSED=$((PASSED + 1))
else
    red "  FAIL: Socket $SOCKET_PATH still exists after stop"
    FAILED=$((FAILED + 1))
fi

# ── Summary ─────────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
