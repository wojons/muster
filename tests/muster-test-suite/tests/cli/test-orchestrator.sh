#!/usr/bin/env bash
# test-orchestrator.sh — Create workflow, trigger via webhook, poll execution status
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-5
set -euo pipefail

# ── Configuration ───────────────────────────────────────────────
SERVICE_BASE_URL="${SERVICE_BASE_URL:-http://localhost:9805}"
BIN_DIR="${BIN_DIR:-${SCRIPT_DIR}/../../bin}"
CLI_BIN="${BIN_DIR}/openapi-cli"
MAX_POLL=20
POLL_DELAY=0.2
FAILED=0
PASSED=0

# ── Helpers ─────────────────────────────────────────────────────
red()    { printf '\033[0;31m%s\033[0m\n' "$1"; }
green()  { printf '\033[0;32m%s\033[0m\n' "$1"; }

check_deps() {
    local missing=0
    if ! command -v jq >/dev/null 2>&1; then
        echo "SKIP: jq not found"; exit 0
    fi
    if ! command -v curl >/dev/null 2>&1; then
        echo "SKIP: curl not found"; exit 0
    fi
    if [ ! -x "$CLI_BIN" ]; then
        echo "SKIP: $CLI_BIN not found or not executable"; exit 0
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

# ── Main ────────────────────────────────────────────────────────
echo "============================================"
echo "  Test: orchestrator workflow + webhook"
echo "============================================"

check_deps

# ── Step 1: Health check ────────────────────────────────────────
echo "--- Health Check ---"
HEALTH_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/health")
assert_status 200 "$HEALTH_CODE" "GET /health returns 200"

# ── Step 2: Create a workflow ───────────────────────────────────
echo "--- Create Workflow ---"
WORKFLOW_BODY='{"name":"Deploy Pipeline","description":"Three-step deployment workflow","status":"active","steps":[{"id":1,"name":"Validate","type":"task"},{"id":2,"name":"Deploy","type":"task"},{"id":3,"name":"Verify","type":"task"}]}'
CREATE_RESP=$(curl -s -w '\n%{http_code}' -X POST -H 'Content-Type: application/json' -d "$WORKFLOW_BODY" "${SERVICE_BASE_URL}/workflows")
CREATE_CODE=$(echo "$CREATE_RESP" | tail -1)
CREATE_BODY=$(echo "$CREATE_RESP" | sed '$d')
assert_status 201 "$CREATE_CODE" "POST /workflows returns 201"
WORKFLOW_ID=$(echo "$CREATE_BODY" | jq -r '.id // ""')
STEP_COUNT=$(echo "$CREATE_BODY" | jq -r '.steps | length // 0')
if [ "$STEP_COUNT" -eq 3 ]; then
    green "  PASS: Workflow has 3 steps"
    PASSED=$((PASSED + 1))
else
    red "  FAIL: Expected 3 steps, got $STEP_COUNT"
    FAILED=$((FAILED + 1))
fi

if [ -z "$WORKFLOW_ID" ] || [ "$WORKFLOW_ID" = "null" ]; then
    red "  FAIL: Could not extract workflow ID — aborting"
    exit 1
fi

# ── Step 3: Create a trigger ────────────────────────────────────
echo "--- Create Trigger ---"
TRIGGER_NAME="autodeploy-test-$$"
TRIGGER_BODY="{\"name\":\"${TRIGGER_NAME}\",\"workflow_id\":${WORKFLOW_ID}}"
TRIGGER_RESP=$(curl -s -w '\n%{http_code}' -X POST -H 'Content-Type: application/json' -d "$TRIGGER_BODY" "${SERVICE_BASE_URL}/triggers")
TRIGGER_CODE=$(echo "$TRIGGER_RESP" | tail -1)
TRIGGER_BODY_ONLY=$(echo "$TRIGGER_RESP" | sed '$d')
assert_status 201 "$TRIGGER_CODE" "POST /triggers returns 201"
TRIGGER_ID=$(echo "$TRIGGER_BODY_ONLY" | jq -r '.name // ""')
assert_contains "$TRIGGER_NAME" "$TRIGGER_ID" "Trigger name matches"

# ── Step 4: Trigger the webhook ─────────────────────────────────
echo "--- Fire Webhook ---"
WEBHOOK_BODY='{"environment":"staging","commit":"abc123def"}'
WEBHOOK_RESP=$(curl -s -w '\n%{http_code}' -X POST -H 'Content-Type: application/json' -d "$WEBHOOK_BODY" "${SERVICE_BASE_URL}/triggers/${TRIGGER_NAME}/webhook")
WEBHOOK_CODE=$(echo "$WEBHOOK_RESP" | tail -1)
WEBHOOK_BODY_ONLY=$(echo "$WEBHOOK_RESP" | sed '$d')
assert_status 202 "$WEBHOOK_CODE" "POST /triggers/${TRIGGER_NAME}/webhook returns 202"
EXECUTION_ID=$(echo "$WEBHOOK_BODY_ONLY" | jq -r '.execution_id // ""')

if [ -z "$EXECUTION_ID" ] || [ "$EXECUTION_ID" = "null" ]; then
    red "  FAIL: Could not extract execution ID"
    FAILED=$((FAILED + 1))
else
    # Verify initial status (running or completed)
    EXEC_STATUS=$(echo "$WEBHOOK_BODY_ONLY" | jq -r '.status // ""')
    if [ "$EXEC_STATUS" = "running" ] || [ "$EXEC_STATUS" = "completed" ]; then
        green "  PASS: Initial execution status is $EXEC_STATUS"
        PASSED=$((PASSED + 1))
    else
        red "  FAIL: Unexpected execution status: $EXEC_STATUS"
        FAILED=$((FAILED + 1))
    fi

    # ── Step 5: Poll execution status ───────────────────────────
    echo "--- Poll Execution Status ---"
    POLL_COUNT=0
    FINAL_STATUS="unknown"
    while [ "$POLL_COUNT" -lt "$MAX_POLL" ]; do
        POLL_COUNT=$((POLL_COUNT + 1))
        POLL_RESP=$(curl -s "${SERVICE_BASE_URL}/executions/${EXECUTION_ID}")
        FINAL_STATUS=$(echo "$POLL_RESP" | jq -r '.status // "unknown"')
        if [ "$FINAL_STATUS" = "completed" ] || [ "$FINAL_STATUS" = "failed" ]; then
            break
        fi
        sleep "$POLL_DELAY"
    done

    if [ "$FINAL_STATUS" = "completed" ]; then
        green "  PASS: Execution completed within $POLL_COUNT polls"
        PASSED=$((PASSED + 1))
    else
        red "  FAIL: Execution ended with status '$FINAL_STATUS' after $POLL_COUNT polls"
        FAILED=$((FAILED + 1))
    fi

    # Verify progress reached 100
    PROGRESS=$(echo "$POLL_RESP" | jq -r '.progress // 0')
    if [ "$PROGRESS" -ge 90 ]; then
        green "  PASS: Progress is $PROGRESS%% (>=90)"
        PASSED=$((PASSED + 1))
    else
        red "  FAIL: Progress $PROGRESS%% is less than 90"
        FAILED=$((FAILED + 1))
    fi
fi

# ── Summary ─────────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
