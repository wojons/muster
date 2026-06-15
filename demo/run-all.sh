#!/bin/bash
# run-all.sh — Build, start, demo, cleanup. One command to see Muster in action.
set -euo pipefail
cd "$(dirname "$0")/.."

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; CYAN='\033[0;36m'; NC='\033[0m'
PASS=0; FAIL=0; KEEP=0; PIDS=()

pass() { echo -e "${GREEN}  ✓${NC} $1"; PASS=$((PASS+1)); }
fail_msg() { echo -e "${RED}  ✗${NC} $1"; FAIL=$((FAIL+1)); }
banner() { echo ""; echo -e "${CYAN}═══════════════════════════════════════════${NC}"; echo -e "${CYAN} $1${NC}"; echo -e "${CYAN}═══════════════════════════════════════════${NC}"; }

cleanup() {
  echo ""; echo -e "${YELLOW}Cleaning up...${NC}"
  for pid in "${PIDS[@]}"; do kill "$pid" 2>/dev/null || true; done
  rm -f /tmp/muster-demo-daemon.sock
}
trap cleanup EXIT

[[ "$*" == *"--keep-running"* ]] && KEEP=1

# ── Build ────────────────────────────────────────────────────
banner "Building Muster"
echo -n "  openapi-cli... "
go build -o bin/openapi-cli ./cmd/openapi-cli 2>/dev/null && pass "openapi-cli" || { fail_msg "openapi-cli build failed"; exit 1; }
echo -n "  openapi-mcp... "
go build -o bin/openapi-mcp ./cmd/mcp-server 2>/dev/null && pass "openapi-mcp" || { fail_msg "openapi-mcp build failed"; exit 1; }
echo -n "  openapi-daemon... "
go build -o bin/openapi-daemon ./cmd/daemon 2>/dev/null && pass "openapi-daemon" || { fail_msg "openapi-daemon build failed"; exit 1; }

# ── Start Services ───────────────────────────────────────────
banner "Starting Demo Services"

SERVICES=(
  "demo/services/task-forge/server.py:9801:Task Forge"
  "demo/services/bytestash/server.py:9802:ByteStash"
  "demo/services/inkwell/server.py:9803:Inkwell"
  "demo/services/pulse/server.py:9804:Pulse"
  "demo/services/chainlink/server.py:9805:ChainLink"
)

for entry in "${SERVICES[@]}"; do
  SCRIPT="${entry%%:*}"; REST="${entry#*:}"; PORT="${REST%%:*}"; NAME="${REST#*:}"
  echo -n "  $NAME (:$PORT)... "
  PORT=$PORT python3.11 "$SCRIPT" &>/dev/null &
  PIDS+=($!)
  # Wait for health
  for i in $(seq 1 20); do
    if curl -sf "http://localhost:$PORT/health" > /dev/null 2>&1; then
      pass "$NAME ready"; break
    fi
    [ $i -eq 20 ] && { fail_msg "$NAME failed to start"; exit 1; }
    sleep 0.5
  done
done

# ── Seed Data ────────────────────────────────────────────────
banner "Seeding Data"
for dir in demo/services/*/; do
  name=$(basename "$dir")
  [ -f "$dir/seed.py" ] || continue
  echo -n "  $name... "
  python3.11 "$dir/seed.py" > /dev/null 2>&1 && pass "$name seeded" || fail_msg "$name seed failed"
done

# ── Run Demos ────────────────────────────────────────────────
banner "Running Demos"

for script in demo/demos/[0-9]*.sh; do
  name=$(basename "$script")
  echo -n "  $name... "
  if bash "$script" &>/dev/null; then
    pass "$name"
  else
    fail_msg "$name"
  fi
done

# ── Summary ──────────────────────────────────────────────────
banner "Results"
echo "  ${GREEN}Passed: $PASS${NC}"
[ $FAIL -gt 0 ] && echo "  ${RED}Failed: $FAIL${NC}"
[ $FAIL -eq 0 ] && echo -e "  ${GREEN}All $PASS checks passed! 🎉${NC}"

if [ $KEEP -eq 1 ]; then
  echo ""
  echo -e "${YELLOW}Services still running (--keep-running). Press Ctrl+C to stop.${NC}"
  trap - EXIT; sleep infinity
fi

exit $FAIL
