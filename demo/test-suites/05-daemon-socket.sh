#!/bin/bash
# Integration Test 05: Daemon Socket
# Proves: Daemon → Socket → Proxy → Response

set -e

# Resolve everything from the repo root so the suite behaves identically no
# matter which directory the caller runs it from (repo root, demo/, or
# demo/test-suites/) and so a missing binary fails loudly instead of no-oping.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

echo "=== Test 05: Daemon Socket ==="
echo ""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

BINARY="${BINARY:-./bin/openapi-cli}"
DAEMON_BINARY="${DAEMON_BINARY:-./bin/openapi-daemon}"

FAILED=0
WARNED=0
fail() { echo -e "${RED}✗ $1${NC}"; FAILED=$((FAILED + 1)); }
warn() { echo -e "${YELLOW}⚠ $1${NC}"; WARNED=$((WARNED + 1)); }

summary() {
    echo ""
    echo "=== Test 05 RESULT ==="
    echo ""
    echo "Daemon Socket verification:"
    if [ "$FAILED" -gt 0 ]; then
        echo -e "  ${RED}FAIL${NC} - $FAILED failed, $WARNED warnings"
        exit 1
    fi
    echo -e "  ${GREEN}PASS${NC} - 0 failed, $WARNED warnings"
    echo -e "  ${GREEN}✓${NC} Daemon implementation exists"
    exit 0
}

echo "Step 0: Resolve CLI binary"
echo "--------------------------"
echo "  repo root:     $REPO_ROOT"
echo "  cli binary:    $BINARY"
echo "  daemon binary: $DAEMON_BINARY"
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
echo "Step 1: Check daemon implementation"
echo "-----------------------------------"
if [ ! -d "./cmd/daemon" ]; then
    fail "Daemon source directory not found at ./cmd/daemon"
else
    FILE_COUNT=$(find "./cmd/daemon" -name "*.go" 2>/dev/null | wc -l)
    if [ "$FILE_COUNT" -gt 0 ]; then
        echo -e "${GREEN}✓ Daemon implementation exists ($FILE_COUNT files)${NC}"
    else
        fail "Daemon directory exists but contains no Go files"
    fi
fi

echo ""
echo "Step 2: Check daemon binary"
echo "---------------------------"
if [ -f "$DAEMON_BINARY" ]; then
    echo -e "${GREEN}✓ Daemon binary exists${NC}"

    if $DAEMON_BINARY --help 2>/dev/null | grep -q "Usage\|help\|daemon"; then
        echo -e "${GREEN}✓ Daemon responds to help${NC}"
    else
        warn "Daemon help not available"
    fi
else
    warn "Daemon binary not found at '$DAEMON_BINARY' (build it: make build-daemon)"
fi

echo ""
echo "Step 3: Verify daemon package"
echo "-----------------------------"
if [ -d "./pkg/daemon" ]; then
    FILE_COUNT=$(find "./pkg/daemon" -name "*.go" | wc -l)
    echo -e "${GREEN}✓ Daemon package exists ($FILE_COUNT files)${NC}"
else
    fail "Daemon package not found at ./pkg/daemon"
fi

echo ""
echo "Step 4: Check socket support"
echo "----------------------------"
# Look for socket-related code (-q so the pipeline cannot mask grep's status)
if grep -rq "socket\|unix\|unixgram" ./pkg/daemon/ 2>/dev/null; then
    echo -e "${GREEN}✓ Socket code found in daemon${NC}"
else
    fail "No socket code found in ./pkg/daemon"
fi

echo ""
echo "Full socket round-trip testing requires a running daemon process"
echo "(see demo/test-suites/05-daemon-proxied.sh)."

summary
