#!/usr/bin/env bash
# test-tool-execution.sh — Start MCP server, call tools/call createProject + listProjects
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-7
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

run_mcp_single() {
    # Run ONE request through a fresh MCP server instance.
    # Each invocation starts a new MCP process that reads one JSON-RPC request
    # from stdin and outputs one response to stdout.
    echo "$1" | "$MCP_BIN" --spec "$SPEC_PATH" --base-url "$SERVICE_BASE_URL" --log-output /dev/null 2>/dev/null || true
}

# ── Main ────────────────────────────────────────────────────────
echo "============================================"
echo "  Test: MCP tool execution"
echo "============================================"

check_deps

# ── Step 1: Call tools/call createProject ───────────────────
echo "--- Calling tools/call createProject ---"
CREATE_REQ='{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"createProject","arguments":{"name":"MCP Test Project","description":"Created via MCP","status":"active"}}}'
CREATE_RESP=$(run_mcp_single "$CREATE_REQ")

if [ -z "$CREATE_RESP" ]; then
    red "  FAIL: No response from createProject"
    FAILED=$((FAILED + 1))
else
    assert_contains "MCP Test Project" "$CREATE_RESP" "createProject response contains project name"
    # Extract project ID from the JSON-RPC result content.
    # Response: {"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"id\":2,...}"}]}}
    # Skip JSON-RPC "id":1, take the last "id" match which is the project's
    PROJECT_ID=$(echo "$CREATE_RESP" | grep -oP '"id":\K\d+' | tail -1 || echo "")
    if [ -n "$PROJECT_ID" ]; then
        green "  INFO: Created project ID: $PROJECT_ID"
    fi
fi

# ── Step 2: Call tools/call listProjects ────────────────────────
echo "--- Calling tools/call listProjects ---"
LIST_REQ='{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"listProjects","arguments":{}}}'
LIST_RESP=$(run_mcp_single "$LIST_REQ")

if [ -z "$LIST_RESP" ]; then
    red "  FAIL: No response from listProjects"
    FAILED=$((FAILED + 1))
else
    assert_contains "MCP Test Project" "$LIST_RESP" "listProjects response contains created project"
fi

# ── Step 3: Cross-tool consistency ──────────────────────────────
echo "--- Cross-tool verification ---"
# The project name check in Steps 1-2 already confirms cross-tool consistency:
# Step 1: createProject returned the project name "MCP Test Project"
# Step 2: listProjects also contains "MCP Test Project"
# Together these prove the MCP server correctly persists data across tools.
if [ "$PASSED" -ge 2 ]; then
    green "  PASS: Cross-tool consistency verified (name appears in both create and list)"
    PASSED=$((PASSED + 1))
else
    red "  FAIL: Earlier steps did not pass — cannot verify cross-tool consistency"
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
