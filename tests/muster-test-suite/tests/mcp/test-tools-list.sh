#!/usr/bin/env bash
# test-tools-list.sh — Start MCP server, call tools/list, verify expected tool names
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-6
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
set -euo pipefail

# ── Configuration ───────────────────────────────────────────────
SERVICE_BASE_URL="${SERVICE_BASE_URL:-http://localhost:9800}"
BIN_DIR="${BIN_DIR:-${SCRIPT_DIR}/../../bin}"
MCP_BIN="${BIN_DIR}/openapi-mcp"
SPEC_PATH="${SCRIPT_DIR}/../../apps/task-forge/openapi.json"
FAILED=0
PASSED=0

# ── Helpers ─────────────────────────────────────────────────────
red()    { printf '\033[0;31m%s\033[0m\n' "$1"; }
green()  { printf '\033[0;32m%s\033[0m\n' "$1"; }

check_deps() {
    if [ ! -x "$MCP_BIN" ]; then
        echo "SKIP: $MCP_BIN not found or not executable"; exit 0
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

# ── Cleanup ─────────────────────────────────────────────────────
cleanup() {
    if [ -n "${MCP_PID:-}" ]; then
        kill "$MCP_PID" 2>/dev/null || true
        wait "$MCP_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# ── Main ────────────────────────────────────────────────────────
echo "============================================"
echo "  Test: MCP tools/list"
echo "============================================"

check_deps

# ── Step 1: Start MCP server with task-forge spec ───────────────
echo "--- Starting MCP Server ---"
"$MCP_BIN" --spec "$SPEC_PATH" --base-url "$SERVICE_BASE_URL" --log-output /dev/null &
MCP_PID=$!
green "  INFO: MCP server started with PID $MCP_PID"

# Give the server a moment to start
sleep 1

# ── Step 2: Send tools/list request via stdio ───────────────────
echo "--- Calling tools/list ---"
REQUEST='{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'

# Pipe the request into the MCP process, read the response
RESPONSE=$(echo "$REQUEST" | "$MCP_BIN" --spec "$SPEC_PATH" --base-url "$SERVICE_BASE_URL" --log-output /dev/null 2>/dev/null || true)

if [ -z "$RESPONSE" ]; then
    red "  FAIL: No response received from MCP tools/list"
    FAILED=$((FAILED + 1))
else
    # Check that expected tool names are present
    assert_contains "createProject" "$RESPONSE" "tools/list includes createProject"
    assert_contains "listProjects" "$RESPONSE" "tools/list includes listProjects"
    assert_contains "getProject" "$RESPONSE" "tools/list includes getProject"
    assert_contains "updateProject" "$RESPONSE" "tools/list includes updateProject"
    assert_contains "deleteProject" "$RESPONSE" "tools/list includes deleteProject"
    assert_contains "listTasks" "$RESPONSE" "tools/list includes listTasks"
    assert_contains "createTask" "$RESPONSE" "tools/list includes createTask"
    assert_contains "jsonrpc" "$RESPONSE" "Response is valid JSON-RPC"
fi

# ── Step 3: Kill MCP server ─────────────────────────────────────
echo "--- Stopping MCP Server ---"
kill "$MCP_PID" 2>/dev/null || true
wait "$MCP_PID" 2>/dev/null || true
green "  INFO: MCP server stopped"

# ── Summary ─────────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
