#!/bin/bash
# Demo 10: Starlark DSL workflow chaining
# axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-1
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
echo "═══ Demo 10: DSL Chaining ═══"

# Ensure task-forge service is running
if ! curl -s http://localhost:9801/health > /dev/null 2>&1; then
    echo -e "${YELLOW}  ⚠ Task Forge not running. Starting services...${NC}"
    cd demo && python3 services/task-forge/server.py &
    TASKFORGE_PID=$!
    sleep 2
    if ! curl -s http://localhost:9801/health > /dev/null 2>&1; then
        echo -e "${RED}  ✗ Failed to start Task Forge${NC}"
        exit 1
    fi
    echo -e "${GREEN}  ✓ Task Forge started (PID $TASKFORGE_PID)${NC}"
    trap "kill $TASKFORGE_PID 2>/dev/null; wait $TASKFORGE_PID 2>/dev/null" EXIT
fi

# Build CLI if needed
if [ ! -f bin/openapi-cli ]; then
    echo -e "${YELLOW}  Building openapi-cli...${NC}"
    make build-cli
fi

echo ""
echo "  Running DSL workflow: demo/dsl/workflow.star"
echo ""

# Run the DSL script
./bin/openapi-cli dsl run demo/dsl/workflow.star

echo ""
echo -e "${GREEN}  ✓ DSL workflow executed successfully${NC}"
echo -e "${GREEN}═══ Demo 10: PASS ═══${NC}"
