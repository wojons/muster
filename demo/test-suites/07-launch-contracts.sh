#!/bin/bash
# 07-launch-contracts.sh — verify the documented launch contract of every demo
# service (DF-044): interpreter availability, default port, port precedence
# (argv[1] > PORT environment variable > service default), loud failure with
# usage on an unusable port argument, and the env-based launch shape that
# demo/run-all.sh depends on.
#
# Contracts under test (mirrors the table in demo/README.md):
#   demo/services/task-forge/server.py   default 9801  no auth
#   demo/services/bytestash/server.py    default 9802  no auth
#   demo/services/inkwell/server.py      default 9803  Bearer token
#   demo/services/pulse/server.py        default 9804  no auth
#   demo/services/chainlink/server.py    default 9805  no auth
#   demo/auth-service/server.py          default 9803  Bearer demo-token-12345
#   demo/crud-service/server.py          default 9800  no auth
#   demo/spec-provider/server.py         default 9801  no auth
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT" || exit 1

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
PASS=0; FAIL=0; WARN=0
pass() { echo -e "${GREEN}  ✓${NC} $1"; PASS=$((PASS+1)); }
warn() { echo -e "${YELLOW}  ⚠${NC} $1"; WARN=$((WARN+1)); }
fail() { echo -e "${RED}  ✗${NC} $1"; FAIL=$((FAIL+1)); }
header() { echo ""; echo -e "${YELLOW}── $1 ──${NC}"; }

# ── Interpreter ──────────────────────────────────────────────
# Flask lives in the repo venv when present (run-all.sh has the same rule);
# DEMO_PYTHON overrides both for hosts that keep Flask elsewhere.
if [ -x "$REPO_ROOT/.venv/bin/python3" ]; then
  PYTHON="$REPO_ROOT/.venv/bin/python3"
else
  PYTHON="$(command -v python3 || true)"
fi
PYTHON="${DEMO_PYTHON:-$PYTHON}"

# ── Service contract table (server.py : default port : name : auth) ──
SERVICES=(
  "demo/services/task-forge/server.py:9801:Task Forge:none"
  "demo/services/bytestash/server.py:9802:ByteStash:none"
  "demo/services/inkwell/server.py:9803:Inkwell:bearer"
  "demo/services/pulse/server.py:9804:Pulse:none"
  "demo/services/chainlink/server.py:9805:ChainLink:none"
)
EXTRA_SERVICES=(
  "demo/auth-service/server.py:9803:Auth Service:bearer"
  "demo/crud-service/server.py:9800:CRUD Service:none"
  "demo/spec-provider/server.py:9801:Spec Provider:none"
)

port_free() { ! curl -s -m 1 "http://localhost:$1/health" >/dev/null 2>&1; }

wait_health() { # wait_health <port> <tries>
  for _ in $(seq 1 "${2:-20}"); do
    if curl -s -m 1 "http://localhost:$1/health" >/dev/null 2>&1; then return 0; fi
    sleep 0.25
  done
  return 1
}

start_service() { # start_service <server.py> <mode: argv|env> <port> <logfile>
  # argv mode:  "$PYTHON" <script> <port>     (explicit positional port)
  # env mode:   PORT=<port> "$PYTHON" <script> (the run-all.sh launch shape)
  # The launcher never combines argv and PORT so each cell exercises exactly
  # one resolution path.
  local mode="$2"
  if [ "$mode" = "argv" ]; then
    "$PYTHON" "$1" "$3" >"$4" 2>&1 &
  else
    PORT="$3" "$PYTHON" "$1" >"$4" 2>&1 &
  fi
  echo $!
}

PIDS=()
stop_pid() { kill "$1" 2>/dev/null; wait "$1" 2>/dev/null; }
cleanup() {
  for pid in "${PIDS[@]:-}"; do [ -n "$pid" ] && stop_pid "$pid"; done
}
trap cleanup EXIT

echo "========================================"
echo "Muster Demo — Service Launch Contracts"
echo "========================================"

# ── 1. Interpreter & Flask availability ──────────────────────
header "Interpreter"
if [ -z "$PYTHON" ] || [ ! -x "$PYTHON" ]; then
  fail "no python interpreter found (install Python 3 or create the repo .venv)"
  echo ""; echo -e "${RED}Failed: $FAIL (cannot continue without an interpreter)${NC}"
  exit 1
fi
if "$PYTHON" -m flask --version >/dev/null 2>&1; then
  FLASK_VER="$("$PYTHON" -m flask --version 2>/dev/null | awk '/Flask/{print $2}')"
  pass "$PYTHON ready (Python $("$PYTHON" --version 2>&1 | awk '{print $2}'), Flask $FLASK_VER)"
else
  fail "flask not importable via $PYTHON — the demo services cannot start"
  echo "    Fix: create the repo venv (python3 -m venv .venv && .venv/bin/pip install flask)"
  echo "    or point DEMO_PYTHON at an interpreter that has Flask:"
  echo "    DEMO_PYTHON=/path/to/python bash demo/test-suites/07-launch-contracts.sh"
  echo ""; echo -e "${RED}Failed: $FAIL (cannot exercise service contracts without Flask)${NC}"
  exit 1
fi

# ── 2. Documented contract table (demo/README.md) ────────────
header "Documented contract (demo/README.md)"
if [ ! -f demo/README.md ]; then
  fail "demo/README.md missing"
else
  # README table rows use the directory names (kebab-case) with the port in
  # a trailing comment, e.g. "│   ├── task-forge/  # Project management (port 9801)".
  for entry in "${SERVICES[@]}" "${EXTRA_SERVICES[@]}"; do
    IFS=: read -r script port name auth <<< "$entry"
    dir="$(dirname "$script")"
    if grep -q "$dir" demo/README.md && grep -q "port $port" demo/README.md; then
      pass "README documents $dir (port $port)"
    else
      fail "README does not document $dir (port $port)"
    fi
  done
  if grep -q "argv\[1\]" demo/README.md && grep -q "PORT" demo/README.md; then
    pass "README documents port resolution (argv[1] > PORT env > default)"
  else
    fail "README does not document the port resolution order"
  fi
fi

# ── 3. Default-port launch (no env, no argv) ─────────────────
header "Default port binds (no PORT, no argv)"
for entry in "${SERVICES[@]}"; do
  IFS=: read -r script port name auth <<< "$entry"
  if ! port_free "$port"; then
    fail "$name: documented default port $port is already in use — skipping (is run-all.sh --keep-running active?)"
    continue
  fi
  log="$(mktemp /tmp/df044-launch-XXXXXX.log)"
  pid="$(start_service "$script" argv "$port" "$log")"
  PIDS+=("$pid")
  if wait_health "$port"; then
    pass "$name serves /health on documented default :$port (default == argv value here)"
  else
    fail "$name did not serve /health on documented default :$port (log: $log)"
  fi
  stop_pid "$pid"; PIDS=("${PIDS[@]/$pid/}")
done

# ── 4. PORT environment launch (the run-all.sh shape) ────────
header "PORT environment honored (run-all.sh launch shape)"
entry="${SERVICES[0]}"
IFS=: read -r script port name auth <<< "$entry"
if port_free 9902 && port_free 9801; then
  log="$(mktemp /tmp/df044-launch-XXXXXX.log)"
  pid="$(start_service "$script" env 9902 "$log")"
  PIDS+=("$pid")
  if wait_health 9902 && ! wait_health 9801 1; then
    pass "$name honors PORT=9902 (run-all.sh's env-based launch preserved)"
  elif wait_health 9902; then
    warn "$name bound :9902 but default-port silence unverified"
  else
    fail "$name did not bind PORT=9902"
  fi
  stop_pid "$pid"; PIDS=("${PIDS[@]/$pid/}")
else
  fail "test ports 9902/9801 busy — cannot exercise env launch"
fi

# ── 5. Explicit argv port wins (precedence: argv > env) ──────
header "Explicit argv port (argv > PORT env > default)"
for entry in "${SERVICES[@]}" "${EXTRA_SERVICES[@]}"; do
  IFS=: read -r script port name auth <<< "$entry"
  argv_port=$((port + 100))
  if ! port_free "$argv_port"; then
    fail "$name: test port $argv_port busy — cannot exercise argv launch"
    continue
  fi
  log="$(mktemp /tmp/df044-launch-XXXXXX.log)"
  pid="$(start_service "$script" argv "$argv_port" "$log")"
  PIDS+=("$pid")
  if wait_health "$argv_port" 12 && ! wait_health 9801 1; then
    pass "$name binds explicit argv port :$argv_port (not default :$port)"
  elif wait_health "$argv_port" 12; then
    warn "$name bound :$argv_port but default-port silence unverified"
  else
    fail "$name ignored explicit argv port $argv_port (log: $log)"
  fi
  stop_pid "$pid"; PIDS=("${PIDS[@]/$pid/}")
done

# ── 6. Documented auth behavior (README auth column) ─────────
header "Documented auth behavior"
for entry in "${SERVICES[@]}" "${EXTRA_SERVICES[@]}"; do
  IFS=: read -r script port name auth <<< "$entry"
  [ "$auth" = "none" ] && continue
  dir="$(dirname "$script")"
  if ! grep -q "Bearer" demo/README.md; then
    fail "README lost the Bearer auth documentation ($name)"
    continue
  fi
  probe_port=$((port + 200))
  if ! port_free "$probe_port"; then
    fail "$name: auth probe port $probe_port busy"
    continue
  fi
  log="$(mktemp /tmp/df044-launch-XXXXXX.log)"
  pid="$(start_service "$script" argv "$probe_port" "$log")"
  PIDS+=("$pid")
  wait_health "$probe_port" 12
  code="$(curl -s -o /dev/null -w '%{http_code}' -m 3 -X POST "http://localhost:$probe_port/posts" 2>/dev/null)"
  if [ "$name" = "Inkwell" ]; then
    if [ "$code" = "401" ]; then
      pass "Inkwell rejects an unauthenticated write (POST /posts -> 401)"
    else
      fail "Inkwell unauthenticated write returned $code, want 401 (log: $log)"
    fi
  fi
  if [ "$name" = "Auth Service" ]; then
    code_tasks="$(curl -s -o /dev/null -w '%{http_code}' -m 3 "http://localhost:$probe_port/tasks" 2>/dev/null)"
    code_ok="$(curl -s -o /dev/null -w '%{http_code}' -m 3 -H "Authorization: Bearer demo-token-12345" "http://localhost:$probe_port/tasks" 2>/dev/null)"
    if [ "$code_tasks" = "401" ] && [ "$code_ok" = "200" ]; then
      pass "Auth Service: /tasks -> 401 without token, 200 with documented token"
    else
      fail "Auth Service auth arms got $code_tasks/$code_ok, want 401/200 (log: $log)"
    fi
  fi
  stop_pid "$pid"; PIDS=("${PIDS[@]/$pid/}")
done

# ── 7. Loud failure on an unusable port argument ─────────────
header "Bad argv fails loudly (usage + exit code 2)"
for entry in "${SERVICES[@]}" "${EXTRA_SERVICES[@]}"; do
  IFS=: read -r script port name auth <<< "$entry"
  err="$(mktemp /tmp/df044-launch-XXXXXX.err)"
  "$PYTHON" "$script" not-a-port > /dev/null 2> "$err"
  rc=$?
  if [ "$rc" -eq 2 ] && grep -q "usage" "$err"; then
    pass "$name rejects 'not-a-port' (exit 2, usage printed)"
    rm -f "$err"
  elif [ "$rc" -ne 0 ]; then
    fail "$name failed loudly but wrong shape (rc=$rc, want exit 2 + usage; err kept: $err)"
  else
    fail "$name silently ignored 'not-a-port' and started on its default (rc=0)"
    rm -f "$err"
  fi
done
err="$(mktemp /tmp/df044-launch-XXXXXX.err)"
"$PYTHON" "${SERVICES[0]%%:*}" 9901 extra-arg > /dev/null 2> "$err"
rc=$?
if [ "$rc" -eq 2 ] && grep -q "usage" "$err"; then
  pass "extra argv rejected (exit 2, usage printed)"
  rm -f "$err"
else
  fail "extra argv not rejected (rc=$rc, want exit 2 + usage; err kept: $err)"
fi

# ── Summary ──────────────────────────────────────────────────
echo ""
echo "========================================"
echo "Launch Contract Summary"
echo "========================================"
echo "  Passed: $PASS"
[ "$WARN" -gt 0 ] && echo -e "  ${YELLOW}Warnings: $WARN${NC}"
if [ "$FAIL" -gt 0 ]; then
  echo -e "  ${RED}Failed: $FAIL${NC}"
  exit 1
fi
echo -e "  ${GREEN}All $PASS launch-contract checks passed!${NC}"
exit 0
