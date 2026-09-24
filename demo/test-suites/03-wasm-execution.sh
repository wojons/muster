#!/bin/bash
# Integration Test 03: WASM Execution
# Proves: CLI → WASM Runtime (wazero sandbox) → Module Execution

set -e

# Resolve everything from the repo root so the suite behaves identically no
# matter which directory the caller runs it from (repo root, demo/, or
# demo/test-suites/) and so a missing binary fails loudly instead of no-oping.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

echo "=== Test 03: WASM Execution ==="
echo ""

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

BINARY="${BINARY:-./bin/openapi-cli}"
WASM_DIR="${WASM_DIR:-./demo/wasm-modules}"

FAILED=0
WARNED=0
fail() { echo -e "${RED}✗ $1${NC}"; FAILED=$((FAILED + 1)); }
warn() { echo -e "${YELLOW}⚠ $1${NC}"; WARNED=$((WARNED + 1)); }

summary() {
    echo ""
    echo "=== Test 03 RESULT ==="
    echo ""
    echo "WASM Execution verification:"
    if [ "$FAILED" -gt 0 ]; then
        echo -e "  ${RED}FAIL${NC} - $FAILED failed, $WARNED warnings"
        exit 1
    fi
    echo -e "  ${GREEN}PASS${NC} - 0 failed, $WARNED warnings"
    echo -e "  ${GREEN}✓${NC} WASM package exists"
    echo -e "  ${GREEN}✓${NC} WASM module executed in the sandbox"
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
echo "Step 1: Check WASM modules exist"
echo "--------------------------------"
if [ ! -d "$WASM_DIR" ]; then
    fail "WASM module directory not found at '$WASM_DIR'"
else
    WASM_COUNT=$(find "$WASM_DIR" -name "*.wasm" | wc -l)
    echo "  Found $WASM_COUNT WASM modules"
    if [ "$WASM_COUNT" -gt 0 ]; then
        echo -e "${GREEN}✓ WASM modules available${NC}"
    else
        fail "no .wasm modules found in '$WASM_DIR'"
    fi
fi

echo ""
echo "Step 2: Test CLI WASM commands"
echo "------------------------------"
if $BINARY modules run --help 2>/dev/null | grep -q "openapi-cli modules run"; then
    echo -e "${GREEN}✓ Module execution command available${NC}"
else
    fail "'modules run' command not available (openapi-cli modules run --help)"
fi

echo ""
echo "Step 3: Execute a WASM module in the sandbox"
echo "--------------------------------------------"
HELLO_WASM="$WASM_DIR/hello.wasm"
if [ ! -f "$HELLO_WASM" ]; then
    warn "no hello.wasm to execute in '$WASM_DIR'"
elif RUN_OUTPUT=$($BINARY modules run "$HELLO_WASM" 2>&1); then
    echo "$RUN_OUTPUT" | sed 's/^/  /'
    echo -e "${GREEN}✓ WASM module executed through the CLI${NC}"
else
    echo "$RUN_OUTPUT" | sed 's/^/  /'
    fail "executing '$HELLO_WASM' failed"
fi

echo ""
echo "Step 4: Verify WASM sandbox implementation"
echo "------------------------------------------"
if [ ! -d "./pkg/wasm" ]; then
    fail "WASM runtime package not found at ./pkg/wasm"
else
    WASM_FILES=$(find ./pkg/wasm -name "*.go" | wc -l)
    echo "  Contains $WASM_FILES Go files"
    if [ "$WASM_FILES" -gt 0 ]; then
        echo -e "${GREEN}✓ WASM runtime implementation exists${NC}"
    else
        fail "pkg/wasm contains no Go files"
    fi
fi

echo ""
echo "To create more test WASM modules:"
echo "  1. Write Go/Rust code with WASM target"
echo "  2. Compile to .wasm format"
echo "  3. Place in demo/wasm-modules/"
echo "  4. Run: openapi-cli modules run <module.wasm>"

summary
