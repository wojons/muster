#!/bin/bash
# Demo 10: Starlark DSL workflow chaining
# NOTE: DSL subcommand pending CLI wiring (internal/builtin/root.go line 61)
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
echo "═══ Demo 10: DSL Chaining ═══"
echo -e "${YELLOW}  ⚠ DSL subcommand not yet wired to CLI.${NC}"
echo -e "${YELLOW}    Package exists at pkg/dsl/ — pending cobra command in internal/builtin/${NC}"
echo -e "${YELLOW}    Tracked as gap in .hermes/acceptance-criteria.md${NC}"
exit 0
