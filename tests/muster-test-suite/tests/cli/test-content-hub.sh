#!/usr/bin/env bash
# test-content-hub.sh — Auth and draft→publish flow for content-hub service
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-3
set -euo pipefail

# ── Configuration ───────────────────────────────────────────────
SERVICE_BASE_URL="${SERVICE_BASE_URL:-http://localhost:9803}"
BIN_DIR="${BIN_DIR:-${SCRIPT_DIR}/../../bin}"
CLI_BIN="${BIN_DIR}/openapi-cli"
FAILED=0
PASSED=0

# Tokens from seed.py
ADMIN_TOKEN="${ADMIN_TOKEN:-admin-token}"
READER_TOKEN="${READER_TOKEN:-reader-token}"

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

run_curl_auth() {
    # Make an authenticated curl request
    # Usage: run_curl_auth <method> <path> <token> [body]
    local method="$1" path="$2" token="$3" body="${4:-}"
    local cmd=(curl -s -w '\n%{http_code}' -X "$method" -H "Authorization: Bearer ${token}" -H 'Content-Type: application/json')
    if [ -n "$body" ]; then
        "${cmd[@]}" -d "$body" "${SERVICE_BASE_URL}${path}"
    else
        "${cmd[@]}" "${SERVICE_BASE_URL}${path}"
    fi
}

# ── Main ────────────────────────────────────────────────────────
echo "============================================"
echo "  Test: content-hub auth + publish flow"
echo "============================================"

check_deps

# ── Step 1: Health check ────────────────────────────────────────
echo "--- Health Check ---"
HEALTH_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/health")
assert_status 200 "$HEALTH_CODE" "GET /health returns 200"

# ── Step 2: Try list-posts without auth → expect 401 ────────────
echo "--- List Posts (no auth) ---"
NOAUTH_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/posts")
assert_status 401 "$NOAUTH_CODE" "GET /posts without auth returns 401"

# ── Step 3: Try list-posts with reader token → expect 200 ──────
echo "--- List Posts (reader token) ---"
READER_RESP=$(run_curl_auth GET /posts "$READER_TOKEN")
READER_CODE=$(echo "$READER_RESP" | tail -1)
assert_status 200 "$READER_CODE" "GET /posts with reader token returns 200"

# ── Step 4: Try list-posts with admin token → expect 200 ───────
echo "--- List Posts (admin token) ---"
ADMIN_RESP=$(run_curl_auth GET /posts "$ADMIN_TOKEN")
ADMIN_CODE=$(echo "$ADMIN_RESP" | tail -1)
assert_status 200 "$ADMIN_CODE" "GET /posts with admin token returns 200"

# ── Step 5: Create a post with admin role → expect 201 (draft) ─
echo "--- Create Post (admin, draft) ---"
CREATE_BODY='{"title":"Integration Test Post","content":"This is a test post body.","tags":["test","integration"],"status":"draft"}'
CREATE_RESP=$(run_curl_auth POST /posts "$ADMIN_TOKEN" "$CREATE_BODY")
CREATE_CODE=$(echo "$CREATE_RESP" | tail -1)
CREATE_BODY_ONLY=$(echo "$CREATE_RESP" | sed '$d')
assert_status 201 "$CREATE_CODE" "POST /posts with admin token returns 201"
assert_contains "draft" "$CREATE_BODY_ONLY" "New post status is draft"
POST_ID=$(echo "$CREATE_BODY_ONLY" | jq -r '.id // ""')

if [ -z "$POST_ID" ] || [ "$POST_ID" = "null" ]; then
    red "  FAIL: Could not extract post ID — skipping remaining tests"
    exit 1
fi

# ── Step 6: Reader cannot see draft ─────────────────────────────
echo "--- Verify Reader Cannot See Draft ---"
READER_GET_RESP=$(run_curl_auth GET "/posts/${POST_ID}" "$READER_TOKEN")
READER_GET_CODE=$(echo "$READER_GET_RESP" | tail -1)
READER_GET_BODY=$(echo "$READER_GET_RESP" | sed '$d')
assert_status 404 "$READER_GET_CODE" "GET /posts/${POST_ID} with reader token returns 404 (draft hidden)"

# ── Step 7: Publish the post (admin) ────────────────────────────
echo "--- Publish Post ---"
PUBLISH_RESP=$(run_curl_auth PUT "/posts/${POST_ID}/publish" "$ADMIN_TOKEN")
PUBLISH_CODE=$(echo "$PUBLISH_RESP" | tail -1)
PUBLISH_BODY=$(echo "$PUBLISH_RESP" | sed '$d')
assert_status 200 "$PUBLISH_CODE" "PUT /posts/${POST_ID}/publish returns 200"
assert_contains "published" "$PUBLISH_BODY" "Post status is now published"

# ── Step 8: Reader can now see the published post ───────────────
echo "--- Verify Reader Can See Published ---"
READER_GET2_RESP=$(run_curl_auth GET "/posts/${POST_ID}" "$READER_TOKEN")
READER_GET2_CODE=$(echo "$READER_GET2_RESP" | tail -1)
READER_GET2_BODY=$(echo "$READER_GET2_RESP" | sed '$d')
assert_status 200 "$READER_GET2_CODE" "GET /posts/${POST_ID} with reader token returns 200 (published)"
assert_contains "Integration Test Post" "$READER_GET2_BODY" "Published post title matches"

# ── Summary ─────────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
