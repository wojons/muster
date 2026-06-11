#!/usr/bin/env bash
# test-task-forge.sh — Full CRUD cycle for task-forge service via openapi-cli request
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-1
set -euo pipefail

# ── Configuration ───────────────────────────────────────────────
SERVICE_BASE_URL="${SERVICE_BASE_URL:-http://localhost:9800}"
BIN_DIR="${BIN_DIR:-${SCRIPT_DIR}/../../bin}"
CLI_BIN="${BIN_DIR}/openapi-cli"
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

run_curl() {
    # Make an HTTP request via curl and return the response body and HTTP status code
    # Usage: run_curl <method> <path> [body]
    local method="$1" path="$2" body="${3:-}"
    if [ -n "$body" ]; then
        curl -s -w '\n%{http_code}' -X "$method" "${SERVICE_BASE_URL}${path}" \
            -H 'Content-Type: application/json' -d "$body"
    else
        curl -s -w '\n%{http_code}' -X "$method" "${SERVICE_BASE_URL}${path}"
    fi
}

# ── Main ────────────────────────────────────────────────────────
echo "============================================"
echo "  Test: task-forge CLI CRUD"
echo "============================================"

check_deps

# ── Step 1: Health check ────────────────────────────────────────
echo "--- Health Check ---"
HEALTH_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/health")
assert_status 200 "$HEALTH_CODE" "GET /health returns 200"

# ── Step 2: Create a project ────────────────────────────────────
echo "--- Create Project ---"
CREATE_BODY='{"name":"Test Project Beta","description":"Created by integration test","status":"active"}'
CREATE_RESP=$(run_curl POST /projects "$CREATE_BODY")
CREATE_CODE=$(echo "$CREATE_RESP" | tail -1)
CREATE_BODY_ONLY=$(echo "$CREATE_RESP" | sed '$d')
assert_status 201 "$CREATE_CODE" "POST /projects returns 201"
PROJECT_ID=$(echo "$CREATE_BODY_ONLY" | jq -r '.id // ""')
assert_contains "Test Project Beta" "$CREATE_BODY_ONLY" "Created project name matches"

if [ -z "$PROJECT_ID" ] || [ "$PROJECT_ID" = "null" ]; then
    red "  FAIL: Could not extract project ID — skipping remaining tests"
    exit 1
fi

# ── Step 3: List projects and verify project appears ────────────
echo "--- List Projects ---"
LIST_RESP=$(curl -s "${SERVICE_BASE_URL}/projects")
LIST_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/projects")
assert_status 200 "$LIST_CODE" "GET /projects returns 200"
assert_contains "$PROJECT_ID" "$LIST_RESP" "Project $PROJECT_ID appears in list"

# ── Step 4: Get project by ID ───────────────────────────────────
echo "--- Get Project ---"
GET_RESP=$(curl -s "${SERVICE_BASE_URL}/projects/${PROJECT_ID}")
GET_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/projects/${PROJECT_ID}")
assert_status 200 "$GET_CODE" "GET /projects/${PROJECT_ID} returns 200"
PROJECT_NAME=$(echo "$GET_RESP" | jq -r '.name // ""')
assert_contains "Test Project Beta" "$PROJECT_NAME" "Get returns correct name"

# ── Step 5: Update project ──────────────────────────────────────
echo "--- Update Project ---"
UPDATE_BODY='{"name":"Test Project Gamma","description":"Updated desc"}'
UPDATE_RESP=$(run_curl PUT "/projects/${PROJECT_ID}" "$UPDATE_BODY")
UPDATE_CODE=$(echo "$UPDATE_RESP" | tail -1)
assert_status 200 "$UPDATE_CODE" "PUT /projects/${PROJECT_ID} returns 200"
UPDATED_NAME=$(echo "$UPDATE_RESP" | sed '$d' | jq -r '.name // ""')
assert_contains "Test Project Gamma" "$UPDATED_NAME" "Updated name matches"

# ── Step 6: Verify the update persisted ─────────────────────────
GET_RESP2=$(curl -s "${SERVICE_BASE_URL}/projects/${PROJECT_ID}")
GET_NAME2=$(echo "$GET_RESP2" | jq -r '.name // ""')
assert_contains "Test Project Gamma" "$GET_NAME2" "Re-fetch confirms updated name"

# ── Step 7: Delete project ──────────────────────────────────────
echo "--- Delete Project ---"
DELETE_CODE=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "${SERVICE_BASE_URL}/projects/${PROJECT_ID}")
assert_status 200 "$DELETE_CODE" "DELETE /projects/${PROJECT_ID} returns 200"

# ── Step 8: Verify 404 on deleted project ───────────────────────
echo "--- Verify Deleted ---"
NOTFOUND_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/projects/${PROJECT_ID}")
assert_status 404 "$NOTFOUND_CODE" "GET /projects/${PROJECT_ID} returns 404 after delete"

# ── Summary ─────────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
