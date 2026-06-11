#!/usr/bin/env bash
# test-metrics-engine.sh — Bulk insert, range query, aggregate, dashboard CRUD
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-4
set -euo pipefail

# ── Configuration ───────────────────────────────────────────────
SERVICE_BASE_URL="${SERVICE_BASE_URL:-http://localhost:9804}"
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

assert_float_compare() {
    # Compare two floats with label. Usage: assert_float_compare <expected> <actual> <label>
    # Uses python3 for float comparison with absolute tolerance 0.001
    local expected="$1" actual="$2" label="$3"
    if python3 -c "exit(0 if abs(${actual} - ${expected}) < 0.001 else 1)" 2>/dev/null; then
        green "  PASS: $label"
        PASSED=$((PASSED + 1))
    else
        red "  FAIL: $label — expected ~$expected, got $actual"
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
echo "  Test: metrics-engine bulk/query/aggregate/dashboard"
echo "============================================"

check_deps

# ── Step 1: Health check ────────────────────────────────────────
echo "--- Health Check ---"
HEALTH_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/health")
assert_status 200 "$HEALTH_CODE" "GET /health returns 200"

# ── Step 2: Bulk insert data points ──────────────────────────
echo "--- Bulk Insert ---"
INSERT_BODY=$(python3 -c "
import json
points=[]
ts=0
for v in range(10, 60, 10):
    points.append({'name':'cpu_temp','value':float(v),'timestamp':'2024-01-01T00:{:02d}:00Z'.format(v)})
    points.append({'name':'mem_usage','value':float(v+5),'timestamp':'2024-01-01T00:{:02d}:00Z'.format(v)})
print(json.dumps({'points':points}))
")

INSERT_RESP=$(curl -s -w '\n%{http_code}' \
    -X POST -H 'Content-Type: application/json' \
    -d "$INSERT_BODY" "${SERVICE_BASE_URL}/metrics")
INSERT_CODE=$(echo "$INSERT_RESP" | tail -1)
INSERT_BODY_ONLY=$(echo "$INSERT_RESP" | sed '$d')
assert_status 201 "$INSERT_CODE" "POST /metrics (bulk insert) returns 201"
INSERTED=$(echo "$INSERT_BODY_ONLY" | jq -r '.inserted // 0')
if [ "$INSERTED" -ge 8 ]; then
    green "  PASS: Inserted $INSERTED points (>=8)"
    PASSED=$((PASSED + 1))
else
    red "  FAIL: Expected >=8 points inserted, got $INSERTED"
    FAILED=$((FAILED + 1))
fi

# ── Step 3: Range query ─────────────────────────────────────────
echo "--- Range Query ---"
RANGE_RESP=$(curl -s "${SERVICE_BASE_URL}/metrics/cpu_temp?from=2024-01-01T00:10:00Z&to=2024-01-01T00:40:00Z")
COUNT=$(echo "$RANGE_RESP" | jq -r '.count // 0')
if [ "$COUNT" -ge 3 ]; then
    green "  PASS: Range query returned $COUNT points (>=3)"
    PASSED=$((PASSED + 1))
else
    red "  FAIL: Expected >=3 points, got $COUNT"
    FAILED=$((FAILED + 1))
fi

# ── Step 4: Aggregation (avg) ───────────────────────────────────
echo "--- Aggregate (avg) ---"
AGG_RESP=$(curl -s "${SERVICE_BASE_URL}/metrics/cpu_temp/aggregate?fn=avg&interval=30m")
AGG_DATA=$(echo "$AGG_RESP" | jq -r '.data | length // 0')
if [ "$AGG_DATA" -ge 2 ]; then
    green "  PASS: Aggregate avg returned $AGG_DATA buckets (>=2)"
    PASSED=$((PASSED + 1))
else
    red "  FAIL: Expected >=2 buckets, got $AGG_DATA"
    FAILED=$((FAILED + 1))
fi

# Verify specific values: first bucket should be ~20 (avg of 10+20+30/3)
# Actually our points are at timestamps 00:10, 00:20, 00:30 — and interval is 30m
# So buckets are [0, 30), [30, 60). First bucket has 10, 20, 30 → avg=20
AGG_FIRST_VALUE=$(echo "$AGG_RESP" | jq -r '.data[0].value // 0')
# First 30m bucket [00:00, 00:30): points at 00:10 (10) and 00:20 (20) → avg = 15.0
assert_float_compare 15.0 "$AGG_FIRST_VALUE" "Agg avg first bucket ~15.0"

# ── Step 5: Aggregation (min) ──────────────────────────────────
echo "--- Aggregate (min) ---"
MIN_RESP=$(curl -s "${SERVICE_BASE_URL}/metrics/mem_usage/aggregate?fn=min&interval=30m")
MIN_FIRST=$(echo "$MIN_RESP" | jq -r '.data[0].value // 0')
# First bucket should be 15 (min of 15, 25)
assert_float_compare 15.0 "$MIN_FIRST" "Agg min first bucket ~15.0"

# ── Step 6: Aggregation (max) ──────────────────────────────────
echo "--- Aggregate (max) ---"
MAX_RESP=$(curl -s "${SERVICE_BASE_URL}/metrics/mem_usage/aggregate?fn=max&interval=30m")
MAX_FIRST=$(echo "$MAX_RESP" | jq -r '.data[0].value // 0')
# First 30m bucket [00:00, 00:30): mem_usage at 00:10 (15) and 00:20 (25) → max = 25.0
assert_float_compare 25.0 "$MAX_FIRST" "Agg max first bucket ~25.0"

# ── Step 7: Dashboard CRUD ──────────────────────────────────────
echo "--- Dashboard CRUD ---"

# Create
DASHBOARD_BODY='{"name":"Test Dashboard","description":"A test dashboard","panels":[{"type":"line","metric":"cpu_temp"}]}'
CREATE_DASH_RESP=$(curl -s -w '\n%{http_code}' -X POST -H 'Content-Type: application/json' -d "$DASHBOARD_BODY" "${SERVICE_BASE_URL}/dashboards")
CREATE_DASH_CODE=$(echo "$CREATE_DASH_RESP" | tail -1)
CREATE_DASH_BODY=$(echo "$CREATE_DASH_RESP" | sed '$d')
assert_status 201 "$CREATE_DASH_CODE" "POST /dashboards returns 201"
DASH_ID=$(echo "$CREATE_DASH_BODY" | jq -r '.id // ""')

if [ -z "$DASH_ID" ] || [ "$DASH_ID" = "null" ]; then
    red "  FAIL: Could not extract dashboard ID"
    FAILED=$((FAILED + 1))
else
    # List dashboards
    LIST_DASH_RESP=$(curl -s "${SERVICE_BASE_URL}/dashboards")
    assert_contains "Test Dashboard" "$LIST_DASH_RESP" "Dashboard appears in list"

    # Get dashboard
    GET_DASH_RESP=$(curl -s "${SERVICE_BASE_URL}/dashboards/${DASH_ID}")
    assert_contains "Test Dashboard" "$GET_DASH_RESP" "Get dashboard by ID"

    # Update dashboard
    UPDATE_BODY='{"name":"Updated Dashboard","description":"Updated","panels":[{"type":"bar","metric":"mem_usage"}]}'
    UPDATE_DASH_RESP=$(curl -s -w '\n%{http_code}' -X PUT -H 'Content-Type: application/json' -d "$UPDATE_BODY" "${SERVICE_BASE_URL}/dashboards/${DASH_ID}")
    UPDATE_DASH_CODE=$(echo "$UPDATE_DASH_RESP" | tail -1)
    UPDATE_DASH_BODY=$(echo "$UPDATE_DASH_RESP" | sed '$d')
    assert_status 200 "$UPDATE_DASH_CODE" "PUT /dashboards/${DASH_ID} returns 200"
    assert_contains "Updated Dashboard" "$UPDATE_DASH_BODY" "Dashboard name updated"

    # Delete dashboard
    DELETE_DASH_CODE=$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "${SERVICE_BASE_URL}/dashboards/${DASH_ID}")
    assert_status 200 "$DELETE_DASH_CODE" "DELETE /dashboards/${DASH_ID} returns 200"

    # Verify deleted
    NOTFOUND_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${SERVICE_BASE_URL}/dashboards/${DASH_ID}")
    assert_status 404 "$NOTFOUND_CODE" "GET deleted dashboard returns 404"
fi

# ── Summary ─────────────────────────────────────────────────────
echo ""
echo "============================================"
echo "  Results: $PASSED passed, $FAILED failed"
echo "============================================"

if [ "$FAILED" -gt 0 ]; then
    exit 1
fi
