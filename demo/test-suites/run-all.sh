#!/bin/bash
# Run all integration test suites

set -e

echo "========================================"
echo "Muster - Complete Demo Test Suite"
echo "========================================"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Counters
TOTAL=0
PASSED=0
FAILED=0

run_test() {
    local test_name="$1"
    local test_script="$2"

    TOTAL=$((TOTAL + 1))
    echo "[$TOTAL/$TOTAL_SUITES] Running: $test_name"
    echo "----------------------------------------"

    if bash "$test_script"; then
        PASSED=$((PASSED + 1))
        echo -e "${GREEN}✓ PASSED: $test_name${NC}"
    else
        FAILED=$((FAILED + 1))
        echo -e "${RED}✗ FAILED: $test_name${NC}"
    fi

    echo ""
    echo "========================================"
    echo ""
}

# Count total test suites
TOTAL_SUITES=6

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Run all test suites
run_test "CLI Generation" "$SCRIPT_DIR/01-cli-generation.sh"
run_test "MCP Flow" "$SCRIPT_DIR/02-mcp-flow.sh"
run_test "WASM Execution" "$SCRIPT_DIR/03-wasm-execution.sh"
run_test "Auth Flows" "$SCRIPT_DIR/04-auth-flows.sh"
run_test "Daemon Socket" "$SCRIPT_DIR/05-daemon-socket.sh"
run_test "Full Stack" "$SCRIPT_DIR/06-full-stack.sh"

# Summary
echo "========================================"
echo "Demo Test Suite Summary"
echo "========================================"
echo -e "Total tests:   $TOTAL"
echo -e "${GREEN}Passed:        $PASSED${NC}"
if [ $FAILED -gt 0 ]; then
    echo -e "${RED}Failed:        $FAILED${NC}"
else
    echo -e "Failed:        0"
fi
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    echo ""
    echo "Verified integrations:"
    echo "  • CLI → OpenAPI Parser → Command Generator"
    echo "  • Command Generator → Generated Commands → Execution"
    echo "  • MCP Server → Tool Registration → Tool Calls"
    echo "  • WASM Runtime → Sandbox → Capability Enforcement"
    echo "  • Authentication → Token Management → Protected Endpoints"
    echo "  • Daemon → Unix Socket → HTTP Proxy → Routing"
    echo "  • Full pipeline: Spec → Parse → Generate → Execute → Validate"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
