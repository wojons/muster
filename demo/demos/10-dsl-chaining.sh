#!/bin/bash
# Demo 10: Starlark DSL workflow chaining
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
echo "═══ Demo 10: DSL Chaining ═══"
echo "  Running workflow.star against Task Forge..."

if ./bin/openapi-cli dsl run demo/dsl/workflow.star >/dev/null 2>&1; then
    echo -e "  ${GREEN}✓ DSL chaining: project + tasks + list + update${NC}"
    echo -e "${GREEN}✓ PASS: 10-dsl-chaining.sh${NC}"
else
    echo -e "  ${RED}✗ DSL chaining failed${NC}"
    echo -e "${RED}✗ FAIL: 10-dsl-chaining.sh${NC}"
    exit 1
fi
