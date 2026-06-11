#!/usr/bin/env bash
# run.sh — Muster Integration Test Runner
# Builds binaries, starts demo apps, runs all 9 test scripts
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
TEST_DIR="${SCRIPT_DIR}/tests"
APPS_DIR="${SCRIPT_DIR}/apps"
BIN_DIR="${PROJECT_ROOT}/bin"

HOST_IP="${HOST_IP:-}"
if [ -z "$HOST_IP" ]; then
    if grep -qE 'docker|lxc' /proc/1/cgroup 2>/dev/null; then
        HOST_IP="172.17.0.1"
    else
        HOST_IP="localhost"
    fi
fi

export BIN_DIR

APPS=(
    "task-forge:9800"
    "file-vault:9802"
    "content-hub:9803"
    "metrics-engine:9804"
    "orchestrator:9805"
)

TESTS=(
    "cli/test-task-forge.sh"
    "cli/test-file-vault.sh"
    "cli/test-content-hub.sh"
    "cli/test-metrics-engine.sh"
    "cli/test-orchestrator.sh"
    "mcp/test-tools-list.sh"
    "mcp/test-tool-execution.sh"
    "daemon/test-daemon-proxy.sh"
    "cross-consistency/test-all-consumers.sh"
)

PASSED_COUNT=0
FAILED_COUNT=0
SKIPPED_COUNT=0
APP_PIDS=()

red()    { printf '\033[0;31m%s\033[0m\n' "$1" >&2; }
green()  { printf '\033[0;32m%s\033[0m\n' "$1" >&2; }
yellow() { printf '\033[1;33m%s\033[0m\n' "$1" >&2; }
blue()   { printf '\033[0;34m%s\033[0m\n' "$1" >&2; }

cleanup() {
    blue "Cleaning up..."
    for pid in "${APP_PIDS[@]:-}"; do
        kill "$pid" 2>/dev/null || true
    done
    wait "${APP_PIDS[@]:-}" 2>/dev/null || true
    rm -f /tmp/test-muster-daemon-*.sock /tmp/test-muster-daemon-*.pid
    blue "Cleanup complete."
}
trap cleanup EXIT

blue "Building Muster binaries..."
cd "$PROJECT_ROOT"
CGO_ENABLED=1 go build -o "${BIN_DIR}/openapi-cli" ./cmd/openapi-cli/ || { red "BUILD FAILED"; exit 1; }
green "  ✓ Built: openapi-cli"
CGO_ENABLED=1 go build -o "${BIN_DIR}/openapi-mcp" ./cmd/mcp-server/ || { red "BUILD FAILED"; exit 1; }
green "  ✓ Built: openapi-mcp"
CGO_ENABLED=1 go build -o "${BIN_DIR}/openapi-daemon" ./cmd/daemon/ || { red "BUILD FAILED"; exit 1; }
green "  ✓ Built: openapi-daemon"

blue "Starting demo applications..."
# Auto-detect venv: prefer project venv, then /tmp test venv, then system python3
PYTHON_BIN="python3"
if [ -f "/tmp/muster-test-venv/bin/python" ]; then
    PYTHON_BIN="/tmp/muster-test-venv/bin/python"
elif [ -f "${PROJECT_ROOT}/.venv/bin/python" ]; then
    PYTHON_BIN="${PROJECT_ROOT}/.venv/bin/python"
elif ! command -v python3 >/dev/null 2>&1; then
    red "ERROR: python3 not found"
    exit 1
fi
# Auto-install flask if missing
if ! "$PYTHON_BIN" -c "import flask" 2>/dev/null; then
    yellow "  ⚠ flask not found, installing into venv..."
    uv pip install flask --python "$PYTHON_BIN" 2>/dev/null || \
        "$PYTHON_BIN" -m pip install flask 2>/dev/null || true
fi

for app_def in "${APPS[@]}"; do
    NAME="${app_def%%:*}"
    PORT="${app_def##*:}"
    APP_DIR="${APPS_DIR}/${NAME}"
    if [ ! -f "${APP_DIR}/server.py" ]; then
        yellow "  WARNING: ${APP_DIR}/server.py not found — skipping"
        continue
    fi
    PORT="${PORT}" "$PYTHON_BIN" "${APP_DIR}/server.py" &>/dev/null &
    PID=$!
    APP_PIDS+=($PID)
    HEALTHY=false
    for i in $(seq 1 30); do
        if curl -s "http://${HOST_IP}:${PORT}/health" &>/dev/null; then
            HEALTHY=true
            break
        fi
        sleep 0.2
    done
    if $HEALTHY; then
        green "  ✓ ${NAME} ready on :${PORT} (PID ${PID})"
    else
        red "  ✗ ${NAME} health check timeout on :${PORT}"
    fi
done

blue "Running integration tests..."
for test_script in "${TESTS[@]}"; do
    full_path="${TEST_DIR}/${test_script}"
    if [ ! -f "$full_path" ]; then
        yellow "  SKIP: ${test_script}"
        SKIPPED_COUNT=$((SKIPPED_COUNT + 1))
        continue
    fi
    echo ""
    echo "▶ ${test_script}"
    set +e
    bash "$full_path"
    EXIT_CODE=$?
    set -e
    case $EXIT_CODE in
        0) green "  ✓ PASS"; PASSED_COUNT=$((PASSED_COUNT + 1)) ;;
        1) red "  ✗ FAIL"; FAILED_COUNT=$((FAILED_COUNT + 1)) ;;
        *) yellow "  ⚠ UNKNOWN ($EXIT_CODE)"; FAILED_COUNT=$((FAILED_COUNT + 1)) ;;
    esac
done

echo ""
echo "═══ Results: ${PASSED_COUNT} passed, ${FAILED_COUNT} failed, ${SKIPPED_COUNT} skipped ═══"
[ "$FAILED_COUNT" -gt 0 ] && exit 1 || exit 0
