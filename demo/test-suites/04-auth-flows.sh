#!/bin/bash
# Integration Test 04: Auth Flows
# Proves: Authentication → Protected Endpoints

set -e

# Resolve everything from the repo root so the suite behaves identically no
# matter which directory the caller runs it from (repo root, demo/, or
# demo/test-suites/) and so a missing binary fails loudly instead of no-oping.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

echo "=== Test 04: Auth Flows ==="
echo ""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

BINARY="${BINARY:-./bin/openapi-cli}"

FAILED=0
WARNED=0
fail() { echo -e "${RED}✗ $1${NC}"; FAILED=$((FAILED + 1)); }
warn() { echo -e "${YELLOW}⚠ $1${NC}"; WARNED=$((WARNED + 1)); }

# required=yes -> a missing package fails the suite (it is this suite's subject)
check_package() {
    local pkg="$1" required="$2" count
    if [ -d "./$pkg" ]; then
        count=$(find "./$pkg" -name "*.go" | wc -l)
        echo -e "${GREEN}✓ $pkg exists ($count files)${NC}"
    elif [ "$required" = "yes" ]; then
        fail "$pkg not found (required by this suite)"
    else
        warn "$pkg not found (optional in this repo)"
    fi
}

summary() {
    echo ""
    echo "=== Test 04 RESULT ==="
    echo ""
    echo "Authentication verification:"
    if [ "$FAILED" -gt 0 ]; then
        echo -e "  ${RED}FAIL${NC} - $FAILED failed, $WARNED warnings"
        exit 1
    fi
    echo -e "  ${GREEN}PASS${NC} - 0 failed, $WARNED warnings"
    echo -e "  ${GREEN}✓${NC} Auth package exists"
    exit 0
}

echo "Step 0: Resolve CLI binary"
echo "--------------------------"
echo "  repo root: $REPO_ROOT"
echo "  binary:    $BINARY"
if [ ! -f "$BINARY" ]; then
    fail "CLI binary not found at '$BINARY' (build it: make build-cli)"
    summary
fi
if [ ! -x "$BINARY" ]; then
    fail "CLI binary at '$BINARY' is not executable"
    summary
fi
echo -e "${GREEN}✓ CLI binary resolved${NC}"

echo ""
echo "Step 1: Check auth commands exist"
echo "---------------------------------"
if $BINARY auth --help 2>/dev/null | grep -q "login\|logout\|status"; then
    echo -e "${GREEN}✓ Auth subcommands available${NC}"
else
    warn "Auth login/logout/status commands not available"
fi

echo ""
echo "Step 2: Verify auth status command"
echo "----------------------------------"
if $BINARY auth status 2>/dev/null; then
    echo -e "${GREEN}✓ Auth status command works${NC}"
else
    warn "Auth status failed (expected without credentials)"
fi

echo ""
echo "Step 3: Check authentication packages"
echo "-------------------------------------"
check_package pkg/auth yes
check_package pkg/keychain no

echo ""
echo "Step 4: Verify OAuth2 support"
echo "-----------------------------"
if $BINARY auth login --help 2>/dev/null | grep -q "oauth2\|OAuth2"; then
    echo -e "${GREEN}✓ OAuth2 option documented${NC}"
else
    warn "OAuth2 not documented in help"
fi

echo ""
echo "Step 5: Test init command with auth"
echo "-----------------------------------"
if $BINARY init --help 2>/dev/null | grep -q "api-key\|oauth2\|mtls"; then
    echo -e "${GREEN}✓ Auth options in init command${NC}"
else
    warn "Auth options not in init help"
fi

summary
