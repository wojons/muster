#!/usr/bin/env bash
# test-file-vault.sh — File upload, list, download for file-vault service
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-2
set -euo pipefail

# ── Configuration ───────────────────────────────────────────────
SERVICE_BASE_URL="${SERVICE_BASE_URL:-http://localhost:9802}"
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

rand_str() {
    # Generate a random 8-char alphanumeric string for deterministic-seeming names
    python3 -c "import random; random.seed(${1:-42}); print(''.join(random.choices('abcdefghijklmnopqrstuvwxyz0123456789', k=8)))"
}

# ── Main ────────────────────────────────────────────────────────
echo "============================================"
echo "  Test: file-vault CLI upload/list/download"
echo "============================================"

check_deps

# ── Step 1: Health check ────────────────────────────────────────
echo "--- Health Check ---"
HEALTH_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/health")
assert_status 200 "$HEALTH_CODE" "GET /health returns 200"

# ── Step 2: Create a temp file with deterministic content ───────
echo "--- Create Temp File ---"
SEED=9042
TEST_FILENAME="test-file-$(rand_str $SEED).txt"
TEST_CONTENT="Hello from integration test! Seed=${SEED}"
TMPFILE="/tmp/${TEST_FILENAME}"
echo "$TEST_CONTENT" > "$TMPFILE"
green "  INFO: Created temp file: $TMPFILE"

# ── Step 3: Upload the file via multipart POST ──────────────────
echo "--- Upload File ---"
UPLOAD_RESP=$(curl -s -w '\n%{http_code}' \
    -F "file=@${TMPFILE}" \
    "${SERVICE_BASE_URL}/files")
UPLOAD_CODE=$(echo "$UPLOAD_RESP" | tail -1)
UPLOAD_BODY=$(echo "$UPLOAD_RESP" | sed '$d')
assert_status 201 "$UPLOAD_CODE" "POST /files (upload) returns 201"
FILE_ID=$(echo "$UPLOAD_BODY" | jq -r '.id // ""')
UPLOAD_NAME=$(echo "$UPLOAD_BODY" | jq -r '.name // ""')
assert_contains "$TEST_FILENAME" "$UPLOAD_NAME" "Uploaded filename matches"

if [ -z "$FILE_ID" ] || [ "$FILE_ID" = "null" ]; then
    red "  FAIL: Could not extract file ID — skipping remaining tests"
    rm -f "$TMPFILE"
    exit 1
fi

# ── Step 4: List files and verify our file appears ──────────────
echo "--- List Files ---"
LIST_RESP=$(curl -s "${SERVICE_BASE_URL}/files")
LIST_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/files")
assert_status 200 "$LIST_CODE" "GET /files returns 200"
assert_contains "$TEST_FILENAME" "$LIST_RESP" "File $TEST_FILENAME appears in list"

# ── Step 5: Download the file and verify content ────────────────
echo "--- Download File ---"
DOWNLOAD_RESP=$(curl -s -w '\n%{http_code}' "${SERVICE_BASE_URL}/files/${FILE_ID}/download")
DOWNLOAD_CODE=$(echo "$DOWNLOAD_RESP" | tail -1)
DOWNLOAD_BODY=$(echo "$DOWNLOAD_RESP" | sed '$d')
assert_status 200 "$DOWNLOAD_CODE" "GET /files/${FILE_ID}/download returns 200"
assert_contains "$TEST_CONTENT" "$DOWNLOAD_BODY" "Downloaded content matches"

# ── Step 6: Download nonexistent file → 404 ─────────────────────
echo "--- Download Missing File ---"
MISSING_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/files/99999/download")
assert_status 404 "$MISSING_CODE" "GET /files/99999/download returns 404"

# ── Cleanup ─────────────────────────────────────────────────────
rm -f "$TMPFILE"

# ── Summary ─────────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
