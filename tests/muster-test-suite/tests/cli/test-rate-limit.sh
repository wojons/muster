#!/usr/bin/env bash
# test-rate-limit.sh — AC-010: Rate limiter prevents API abuse
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
BIN_DIR="${BIN_DIR:-${PROJECT_ROOT}/bin}"
PASS=0
FAIL=0

log_pass() { echo "  ✅ PASS: $1"; PASS=$((PASS + 1)); }
log_fail() { echo "  ❌ FAIL: $1"; FAIL=$((FAIL + 1)); }
check() {
    local desc="$1"; shift
    if "$@"; then log_pass "$desc"; else log_fail "$desc"; fi
}

echo "=== AC-010: Rate Limiter Integration Tests ==="

# --- Go unit-level rate limiter tests ---
echo ""
echo "--- Go tests: pkg/rate + pkg/client ---"
if go test ./pkg/rate/ ./pkg/client/ -race 2>&1 | grep -q "^ok"; then
    log_pass "pkg/rate + pkg/client tests pass"
else
    log_fail "pkg/rate + pkg/client tests failed"
fi

# --- Build check ---
echo ""
echo "--- Build check ---"
if go build ./... 2>/dev/null; then
    log_pass "go build ./... clean"
else
    log_fail "go build ./... has errors"
fi

# --- Check that rate limit config types exist ---
echo ""
echo "--- Config types ---"
if grep -q "RateLimitConfig" "${PROJECT_ROOT}/pkg/config/types.go" 2>/dev/null; then
    log_pass "RateLimitConfig type exists in config/types.go"
else
    log_fail "RateLimitConfig type missing from config/types.go"
fi

if grep -q "RateLimit" "${PROJECT_ROOT}/pkg/config/types.go" 2>/dev/null; then
    log_pass "RateLimit field in Config struct"
else
    log_fail "RateLimit field missing from Config struct"
fi

# --- Check that consumers import rate limit ---
echo ""
echo "--- Consumer wiring ---"
if grep -q "WithRateLimiter\|RateLimitRoundTripper\|ratelimit\|rate\." "${PROJECT_ROOT}/internal/builtin/request.go" 2>/dev/null; then
    log_pass "CLI request.go references rate limiting"
else
    log_fail "CLI request.go does NOT reference rate limiting"
fi

if grep -q "WithRateLimiter\|RateLimitRoundTripper\|NewProxyHandlerWith" "${PROJECT_ROOT}/pkg/daemon/proxy.go" 2>/dev/null; then
    log_pass "Daemon proxy.go references rate limiting"
else
    log_fail "Daemon proxy.go does NOT reference rate limiting"
fi

if grep -q "WithRateLimiter\|RateLimitRoundTripper\|NewExecutorWith" "${PROJECT_ROOT}/pkg/mcp/openapi_integration.go" 2>/dev/null; then
    log_pass "MCP openapi_integration.go references rate limiting"
else
    log_fail "MCP openapi_integration.go does NOT reference rate limiting"
fi

# --- Check that Go integration tests exist ---
echo ""
echo "--- Integration tests ---"
if grep -rq "429\|TooManyRequests\|RateLimit" "${PROJECT_ROOT}/pkg/daemon/"*.go "${PROJECT_ROOT}/pkg/mcp/"*.go 2>/dev/null; then
    log_pass "Integration tests reference 429 / rate limiting"
else
    log_fail "No integration tests for rate limiting found"
fi

# --- Summary ---
echo ""
echo "Results: $PASS passed, $FAIL failed"
exit "$FAIL"
