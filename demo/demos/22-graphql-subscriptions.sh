#!/bin/bash
# Demo 22: GraphQL subscriptions — bounded collect over an in-process
# graphql-transport-ws server (PROTO-022).
#
# Gate this script serves: specs/PROTOCOLS.md §6 order 5 (board row PROTO-022).
#
# Fixture ownership: this script starts its own in-process WebSocket GraphQL
# fixture (pkg/protocol/graphql/wstest compiled via `go run` of a tiny main)
# on a loopback port — no external service. The fixture speaks the
# graphql-transport-ws subprotocol: connection_init/ack, subscribe, next,
# complete. Three bounded collects run against it:
#   1. a complete collect (the server finishes its own stream),
#   2. a max-messages truncation (the cap stops the stream mid-flight),
#   3. a max-duration truncation (the wall clock stops it).
# Each transcript must report the collected messages, the truncation marker
# when a bound fired, and a clean unsubscribe.
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }

echo "═══════════════════════════════════════════"
echo " Demo 22: GraphQL subscriptions (order 5)"
echo "═══════════════════════════════════════════"

BIN=./bin/openapi-cli
[ -x "$BIN" ] || fail "$BIN not found — run 'go build -o bin/openapi-cli ./cmd/openapi-cli' first"

WORK=$(mktemp -d)
FIXTURE_PID=""
LOG="$WORK/fixture.log"
cleanup() {
  if [ -n "$FIXTURE_PID" ]; then kill "$FIXTURE_PID" 2>/dev/null || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# ── Schema fixture (SDL on disk; ingestion is the real adapter path) ───────
cat > "$WORK/schema.graphql" <<'EOF'
"A demo GraphQL schema with a subscription root."
type Query {
  ready: String
}

"A tick event streamed by the subscription."
type Tick {
  seq: Int
  msg: String
}

type Subscription {
  tick(count: Int): Tick
}
EOF

# ── WS fixture server (loopback, in-process Go) ─────────────────────────────
cat > "$WORK/wstestfixture.go" <<'EOF'
// Command wstestfixture runs the pkg/protocol/graphql/wstest in-process
// graphql-transport-ws server with its knobs from the environment. It prints
// READY <ws-url> once listening and one "UNSUBSCRIBE n" line per client
// complete frame — the clean-unsubscribe evidence the demo asserts on.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wojons/muster/pkg/protocol/graphql/wstest"
)

func main() {
	srv, err := wstest.Start()
	if err != nil {
		fmt.Println("fixture:", err)
		os.Exit(1)
	}
	if v := os.Getenv("EVENTS"); v != "" {
		n := 0
		if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
			fmt.Println("fixture: bad EVENTS:", err)
			os.Exit(9)
		}
		srv.Events = n
	}
	if v := os.Getenv("DELAY_MS"); v != "" {
		var ms int
		if _, err := fmt.Sscanf(v, "%d", &ms); err != nil {
			fmt.Println("fixture: bad DELAY_MS:", err)
			os.Exit(9)
		}
		srv.Delay = time.Duration(ms) * time.Millisecond
	}
	fmt.Println("READY " + srv.URL())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	last := 0
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			if n := srv.Unsubscribes(); n != last {
				fmt.Printf("UNSUBSCRIBE %d\n", n)
				last = n
			}
		}
	}
}
EOF

# ── Fixture lifecycle helper ────────────────────────────────────────────────
# The fixture is COMPILED once and run as a bare binary: `go run`'s child
# process survives a kill of the go run wrapper (an orphaned server with the
# PREVIOUS leg's knobs is exactly the failure mode that fakes a wrong
# transcript), so the kill must land on the server process itself.
echo -n "  building wstest fixture... "
go build -o "$WORK/wstestfixture" "$WORK/wstestfixture.go" || fail "fixture build failed"
echo "ok"

FIXTURE_URL=""
HTTP_URL=""
start_fixture() {
  # Truncate BEFORE spawning: the child's own redirect happens at exec, so a
  # grep issued right after `$!` could otherwise read the PREVIOUS leg's
  # READY line and point the CLI at a dead port.
  : > "$LOG"
  EVENTS="$1" DELAY_MS="${2:-0}" "$WORK/wstestfixture" >> "$LOG" 2>&1 &
  FIXTURE_PID=$!
  for i in $(seq 1 100); do
    if grep -q '^READY ' "$LOG" 2>/dev/null; then
      FIXTURE_URL="$(sed -n 's/^READY //p' "$LOG" | head -1)"
      HTTP_URL="${FIXTURE_URL/ws:\/\//http://}"
      return 0
    fi
    if ! kill -0 "$FIXTURE_PID" 2>/dev/null; then
      fail "fixture died before READY: $(cat "$LOG")"
    fi
    sleep 0.1
  done
  fail "fixture never became ready"
}

stop_fixture() {
  if [ -n "$FIXTURE_PID" ]; then
    kill "$FIXTURE_PID" 2>/dev/null || true
    wait "$FIXTURE_PID" 2>/dev/null || true
    FIXTURE_PID=""
  fi
}

# assert_transcript <name> <file> <grep-patterns...>
assert_transcript() {
  local name="$1" file="$2"; shift 2
  for pattern in "$@"; do
    if ! grep -q "$pattern" "$file"; then
      fail "$name: transcript is missing $pattern — got: $(cat "$file")"
    fi
  done
  pass "$name"
}

# wait_unsubscribe waits for the fixture's 50 ms ticker to report the client
# complete frame (the transcript and the log line land within ~100 ms of each
# other; a single grep would race the ticker).
wait_unsubscribe() {
  for _ in $(seq 1 50); do
    if grep -q '^UNSUBSCRIBE 1$' "$LOG" 2>/dev/null; then return 0; fi
    sleep 0.1
  done
  fail "the fixture never saw the client unsubscribe: $(cat "$LOG")"
}

# ── Leg 1: complete collect — every event, clean unsubscribe ────────────────
start_fixture 3 0
"$BIN" protocol graphql --schema "$WORK/schema.graphql" --endpoint "$HTTP_URL" \
  --operation tick --args '{"count":3}' -o json > "$WORK/run1.json" \
  || fail "leg 1: the CLI failed: $(cat "$WORK/run1.json")"
assert_transcript "leg 1: complete collect (3 events, unsubscribed, no truncation)" "$WORK/run1.json" \
  '"kind": "subscription"' '"status": "complete"' '"received": 3' '"unsubscribed": true' '"seq": 2'
wait_unsubscribe
stop_fixture

# ── Leg 2: max-messages truncation — the cap stops the stream ───────────────
start_fixture 50 0
"$BIN" protocol graphql --schema "$WORK/schema.graphql" --endpoint "$HTTP_URL" \
  --operation tick --max-messages 5 -o json > "$WORK/run2.json" \
  || fail "leg 2: the CLI failed: $(cat "$WORK/run2.json")"
assert_transcript "leg 2: cap truncation (5 events, limit_reason max-messages, clean stop — no error)" "$WORK/run2.json" \
  '"status": "truncated"' '"items_truncated": true' '"limit_reason": "max-messages"' '"received": 5' '"unsubscribed": true'
wait_unsubscribe
stop_fixture

# ── Leg 3: max-duration truncation — the wall clock stops the stream ────────
start_fixture 100000 25
"$BIN" protocol graphql --schema "$WORK/schema.graphql" --endpoint "$HTTP_URL" \
  --operation tick --timeout 150ms -o json > "$WORK/run3.json" \
  || fail "leg 3: the CLI failed: $(cat "$WORK/run3.json")"
assert_transcript "leg 3: wall-clock truncation (limit_reason max-duration, partial items kept)" "$WORK/run3.json" \
  '"status": "truncated"' '"items_truncated": true' '"limit_reason": "max-duration"' '"unsubscribed": true'
wait_unsubscribe
stop_fixture

echo ""
echo -e "${GREEN}GraphQL subscriptions demo: bounded collect, both truncation markers, clean unsubscribe — all proven.${NC}"
