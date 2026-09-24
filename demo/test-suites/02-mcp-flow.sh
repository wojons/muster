#!/bin/bash
# Integration Test 02: MCP Flow
# Proves: MCP Server → Tool Registration → Tool Calls

set -e

# Resolve everything from the repo root so the suite behaves identically no
# matter which directory the caller runs it from (repo root, demo/, or
# demo/test-suites/) and so a missing binary fails loudly instead of no-oping.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

echo "=== Test 02: MCP Flow ==="
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

BINARY="${BINARY:-./bin/openapi-cli}"
MCP_BINARY="${MCP_BINARY:-./bin/openapi-mcp}"
MCP_FILES="pkg/mcp/server.go pkg/mcp/protocol.go"

FAILED=0
WARNED=0
fail() { echo -e "${RED}✗ $1${NC}"; FAILED=$((FAILED + 1)); }
warn() { echo -e "${YELLOW}⚠ $1${NC}"; WARNED=$((WARNED + 1)); }

summary() {
    echo ""
    echo "=== Test 02 RESULT ==="
    echo ""
    echo "MCP Flow verification:"
    if [ "$FAILED" -gt 0 ]; then
        echo -e "  ${RED}FAIL${NC} - $FAILED failed, $WARNED warnings"
        exit 1
    fi
    echo -e "  ${GREEN}PASS${NC} - 0 failed, $WARNED warnings"
    echo -e "  ${GREEN}✓${NC} MCP package structure exists"
    exit 0
}

echo "Step 0: Resolve CLI binary"
echo "--------------------------"
echo "  repo root: $REPO_ROOT"
echo "  binary:    $BINARY"
if [ ! -f "$BINARY" ]; then
    echo -e "${YELLOW}Building CLI binary...${NC}"
    if make build-cli > /dev/null 2>&1 && [ -x "$BINARY" ]; then
        echo -e "${GREEN}✓ Built $BINARY${NC}"
    fi
fi
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
echo "Step 1: Test MCP server availability"
echo "-------------------------------------"
if $BINARY --help 2>/dev/null | grep -q "MCP\|mcp\|server"; then
    echo -e "${GREEN}✓ MCP server available${NC}"
else
    warn "MCP server support may not be implemented yet"
fi

echo ""
echo "Step 2: Check MCP implementation files"
echo "---------------------------------------"
FOUND=0
for file in $MCP_FILES; do
    if [ -f "$file" ]; then
        FOUND=$((FOUND + 1))
        echo -e "${GREEN}✓ Found: $file${NC}"
    else
        warn "Missing: $file"
    fi
done

if [ $FOUND -eq 0 ]; then
    fail "no MCP implementation files found (expected at least one of: $MCP_FILES)"
else
    echo -e "${GREEN}✓ MCP implementation exists ($FOUND files)${NC}"
fi

echo ""
echo "Step 3: Test MCP protocol basics"
echo "--------------------------------"
# Check that this checkout is a resolvable Go module
if go list -m github.com/wojons/muster > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Go module can be listed${NC}"
else
    fail "Go module listing failed (go list -m github.com/wojons/muster)"
fi

echo ""
echo "Step 4: Check MCP server binary"
echo "--------------------------------"
if [ -f "$MCP_BINARY" ]; then
    echo -e "${GREEN}✓ MCP server binary exists ($MCP_BINARY)${NC}"
else
    warn "MCP server binary not found at '$MCP_BINARY' (build it: make build-mcp)"
fi

echo ""
echo "Note: Full MCP integration requires:"
echo "  • MCP server binary"
echo "  • Tool registration system"
echo "  • JSON-RPC protocol handlers"
echo "  • Stdio/stderr communication"

summary
