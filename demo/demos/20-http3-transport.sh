#!/bin/bash
# Demo 20: HTTP/3 (QUIC) client transport — h3 when the peer speaks it, and a
# *reported* fallback to h2 when it does not.
#
# Gate this script serves: specs/PROTOCOLS.md §6 order 10 (board row PROTO-015).
#
# Fixture ownership: like demo 19, this script builds and starts its own Go
# fixture (demo/services/transport-fixture) on loopback with a self-signed
# certificate and tears it down on exit, so run-all.sh's service list stays
# unchanged. The fixture's H3 endpoint is UDP-only; its TCP endpoints have no
# QUIC listener beside them, which is what makes the fallback leg meaningful.
#
# Every version assertion is checked twice: the server echoes the version it
# saw, and the CLI reports the version it used on stderr. A client claim that
# the server does not agree with is a forgery, not a measurement.
#
# No performance claim is made here. Measured on this box, loopback HTTP/3 is
# not faster than HTTP/2 (see the PROTO-015 report); what is asserted is
# correctness — the version actually used, and the fallback when there is one.
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }

echo "═══════════════════════════════════════════"
echo " Demo 20: HTTP/3 (QUIC) client transport (order 10)"
echo "═══════════════════════════════════════════"

BIN=./bin/openapi-cli
[ -x "$BIN" ] || fail "$BIN not found — run 'go build -o bin/openapi-cli ./cmd/openapi-cli' first"

WORK=$(mktemp -d)
FIXTURE_PID=""
cleanup() {
  if [ -n "$FIXTURE_PID" ]; then kill "$FIXTURE_PID" 2>/dev/null || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# ── Fixture: an HTTP/3 (UDP) endpoint plus TCP-only TLS endpoints ────────────
go build -o "$WORK/transport-fixture" ./demo/services/transport-fixture || fail "fixture build failed"
"$WORK/transport-fixture" -cert-dir "$WORK" >"$WORK/fixture.out" 2>"$WORK/fixture.err" &
FIXTURE_PID=$!

for _ in $(seq 1 50); do
  grep -q '^READY$' "$WORK/fixture.out" 2>/dev/null && break
  kill -0 "$FIXTURE_PID" 2>/dev/null || fail "fixture exited early: $(cat "$WORK/fixture.err")"
  sleep 0.1
done
grep -q '^READY$' "$WORK/fixture.out" || fail "fixture never became ready: $(cat "$WORK/fixture.err")"

CERT=$(awk '/^CERT /{print $2}' "$WORK/fixture.out")
URL_H2=$(awk '/^H2 /{print $2}' "$WORK/fixture.out")
URL_H3=$(awk '/^H3 /{print $2}' "$WORK/fixture.out")
{ [ -n "$CERT" ] && [ -n "$URL_H2" ] && [ -n "$URL_H3" ]; } || fail "fixture did not report CERT/H2/H3"
# QUIC mandates TLS 1.3, so the h3 client verifies the peer's certificate too.
export SSL_CERT_FILE="$CERT"
echo "  fixture: h3(udp)=$URL_H3 h2(tcp-only)=$URL_H2"

# ── 1. GET over HTTP/3, reported as HTTP/3.0 and with no fallback ───────────
BODY=$("$BIN" request GET "$URL_H3/probe" --http-version 3 -v 2>"$WORK/get.err") || fail "GET over the h3 fixture failed"
case "$BODY" in
  *'"server_proto":"HTTP/3.0"'*) ;;
  *) fail "the server did not see HTTP/3.0: $BODY" ;;
esac
grep -q 'HTTP: HTTP/3.0 (wire, requested 3)' "$WORK/get.err" || fail "the CLI must report HTTP/3.0 on stderr: $(cat "$WORK/get.err")"
grep -q 'fallback' "$WORK/get.err" && fail "nothing may fall back when the peer speaks HTTP/3: $(cat "$WORK/get.err")"
pass "GET → server saw HTTP/3.0; CLI reported HTTP/3.0 (wire, requested 3), no fallback"

# ── 2. A WebDAV verb travels over QUIC too ─────────────────────────────────
BODY=$("$BIN" request PROPFIND "$URL_H3/dav" -b '<propfind/>' --http-version 3 -v 2>"$WORK/propfind.err") || fail "PROPFIND over the h3 fixture failed"
case "$BODY" in
  *'"server_proto":"HTTP/3.0"'*) ;;
  *) fail "PROPFIND did not reach the server over HTTP/3: $BODY" ;;
esac
grep -q 'HTTP: HTTP/3.0 (wire, requested 3)' "$WORK/propfind.err" || fail "PROPFIND must be reported as HTTP/3.0: $(cat "$WORK/propfind.err")"
# The body contract: openapi-cli request attaches -b/--body for ANY method
# (PROTO-019 removed the POST/PUT/PATCH allow-list; request.go), so the 11-byte
# WebDAV PROPFIND body travels to the wire and the fixture's byte count is the
# oracle. (Matrix row 17 / gate order 11 owns the full WebDAV profile.)
case "$BODY" in
  *'"body_len":11'*) ;;
  *) fail "the 11-byte PROPFIND body must arrive intact over HTTP/3: $(echo "$BODY" | head -c 200)" ;;
esac
pass "PROPFIND over HTTP/3 → server saw HTTP/3.0 and the whole body (body_len=11: bodies attach for any method — PROTO-019)"

# ── 3. A PUT body arrives intact over HTTP/3 ───────────────────────────────
head -c 4096 /dev/zero | tr '\0' 'x' >"$WORK/blob.txt"
BLOB=$(cat "$WORK/blob.txt")
BODY=$("$BIN" request PUT "$URL_H3/blob" -b "$BLOB" --http-version 3 -v 2>"$WORK/put.err") || fail "PUT over the h3 fixture failed"
case "$BODY" in
  *'"server_proto":"HTTP/3.0"'*) ;;
  *) fail "the PUT must be carried over HTTP/3: $BODY" ;;
esac
case "$BODY" in
  *'"body_len":4096'*) ;;
  *) fail "the 4096-byte PUT body must arrive intact over HTTP/3: $BODY" ;;
esac
grep -q 'HTTP: HTTP/3.0 (wire' "$WORK/put.err" || fail "the PUT report must say HTTP/3.0: $(cat "$WORK/put.err")"
pass "PUT 4096 B → server saw HTTP/3.0 and the whole body (body_len=4096)"

# ── 4. The reported fallback: QUIC unreachable, HTTP/2 carries it, and says so
set +e
OUT=$("$BIN" request GET "$URL_H2/probe" --http-version 3 -v 2>&1)
RC=$?
set -e
[ "$RC" -eq 0 ] || fail "--http-version 3 must still serve over HTTP/2 when QUIC is unreachable: $OUT"
case "$OUT" in
  *'HTTP: HTTP/2.0 (wire, requested 3, fallback HTTP/3.0→HTTP/2.0:'*) ;;
  *) fail "the fallback must be reported on stderr: $OUT" ;;
esac
case "$OUT" in
  *'"server_proto":"HTTP/2.0"'*) ;;
  *) fail "the h2-only peer must have carried the request: $OUT" ;;
esac
class=$(printf '%s' "$OUT" | sed -n 's/.*fallback HTTP\/3\.0→HTTP\/2\.0: \([a-z-]*\).*/\1/p' | head -1)
case "$class" in
  connection-refused|handshake-timeout|idle-timeout|crypto-error|version-negotiation|quic-connection-failure) ;;
  *) fail "the fallback reason must be a connection-layer class, got '$class': $OUT" ;;
esac
pass "policy 3 vs a QUIC-less peer → exit $RC, served over HTTP/2.0, fallback HTTP/3.0→HTTP/2.0 ($class) reported"

# ── 5. Negative cell: nothing falls back *inside* a transport ──────────────
# The h3 endpoint has no TCP listener, so an h2-only client cannot reach it —
# the proof that the fallback above is client code, not a transport property.
set +e
OUT=$("$BIN" request GET "$URL_H3/probe" --http-version 2 2>&1)
RC=$?
set -e
[ "$RC" -ne 0 ] || fail "--http-version 2 must not reach a UDP-only endpoint"
case "$OUT" in
  *connection*refused*|*dial*|*timeout*) ;;
  *) fail "the failure must name a connection-layer cause: $OUT" ;;
esac
pass "--http-version 2 vs the h3-only endpoint → exit $RC, no cross-version retry inside the transport"

# ── 6. A cleartext endpoint can satisfy no HTTP/3 policy ───────────────────
# Refused structurally, before anything is dialled (QUIC mandates TLS 1.3 and
# the stdlib has no h2c) — hence the unresolvable target is never contacted.
set +e
OUT=$("$BIN" request GET "http://127.0.0.1:9/probe" --http-version 3 2>&1)
RC=$?
set -e
[ "$RC" -ne 0 ] || fail "--http-version 3 must fail on a cleartext endpoint"
case "$OUT" in
  *PROTOCOL_MISMATCH*) ;;
  *) fail "the failure must be PROTOCOL_MISMATCH: $OUT" ;;
esac
case "$OUT" in
  *plaintext-endpoint*) ;;
  *) fail "the failure must name the plaintext endpoint: $OUT" ;;
esac
pass "--http-version 3 vs an http:// endpoint → exit $RC, PROTOCOL_MISMATCH (plaintext-endpoint)"

echo ""
pass "Demo 20 complete: HTTP/3 round trips, the reported HTTP/3→HTTP/2 fallback, and the negative cells verified end to end"
