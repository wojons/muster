#!/bin/bash
# Demo 19: HTTP/2 client transport — h2 on the existing HTTP path, with the
# negotiated version observable.
#
# Gate this script serves: specs/PROTOCOLS.md §6 order 9 (board row PROTO-014).
#
# Fixture ownership: this script builds and starts its own Go fixture
# (demo/services/transport-fixture) on loopback with a self-signed certificate
# and tears it down on exit, so run-all.sh's service list stays unchanged.
#
# Every version assertion is checked twice: the server echoes the version it
# saw, and the CLI reports the version it used on stderr. A client claim that
# the server does not agree with is a forgery, not a measurement.
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }

echo "═══════════════════════════════════════════"
echo " Demo 19: HTTP/2 client transport (order 9)"
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

# ── Fixture: h2-capable and http/1.1-only loopback TLS endpoints ─────────────
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
URL_H1=$(awk '/^H1 /{print $2}' "$WORK/fixture.out")
{ [ -n "$CERT" ] && [ -n "$URL_H2" ] && [ -n "$URL_H1" ]; } || fail "fixture did not report CERT/H2/H1"
# The CLI's default transport verifies the peer, so it must trust the fixture's
# self-signed certificate.
export SSL_CERT_FILE="$CERT"
echo "  fixture: h2=$URL_H2 http/1.1-only=$URL_H1"

# ── 1. GET over h2, reported as HTTP/2.0 ────────────────────────────────────
BODY=$("$BIN" request GET "$URL_H2/probe" -v 2>"$WORK/get.err") || fail "GET over the h2 fixture failed"
case "$BODY" in
  *'"server_proto":"HTTP/2.0"'*) ;;
  *) fail "the server did not see HTTP/2.0: $BODY" ;;
esac
grep -q 'HTTP: HTTP/2.0 (wire' "$WORK/get.err" || fail "the CLI must report HTTP/2.0 on stderr: $(cat "$WORK/get.err")"
pass "GET → server saw HTTP/2.0; CLI reported HTTP/2.0 (wire, requested auto)"

# ── 2. PROPFIND, a WebDAV verb, is carried over h2 too ──────────────────────
# Body contract: openapi-cli request attaches -b/--body for ANY method
# (PROTO-019 removed the POST/PUT/PATCH allow-list; request.go), so a WebDAV
# body would travel to the wire here too. This leg stays body-less on purpose —
# the transport cells prove the verb is carried, not the body plumbing (the
# body-over-PROPFIND cell lives in demo 20 and the unit suite; matrix row 17 /
# gate order 11 owns the full WebDAV profile).
BODY=$("$BIN" request PROPFIND "$URL_H2/dav" -v 2>"$WORK/propfind.err") || fail "PROPFIND over the h2 fixture failed"
case "$BODY" in
  *'"server_proto":"HTTP/2.0"'*) ;;
  *) fail "PROPFIND did not reach the server over h2: $BODY" ;;
esac
grep -q 'HTTP: HTTP/2.0 (wire' "$WORK/propfind.err" || fail "PROPFIND must be reported as HTTP/2.0: $(cat "$WORK/propfind.err")"
pass "PROPFIND → server saw HTTP/2.0; CLI reported HTTP/2.0 (wire)"

# ── 3. A PUT body arrives intact over h2 ────────────────────────────────────
head -c 4096 /dev/zero | tr '\0' 'x' >"$WORK/blob.txt"
BLOB=$(cat "$WORK/blob.txt")
BODY=$("$BIN" request PUT "$URL_H2/blob" -b "$BLOB" -v 2>"$WORK/put.err") || fail "PUT over the h2 fixture failed"
case "$BODY" in
  *'"server_proto":"HTTP/2.0"'*) ;;
  *) fail "the PUT must be carried over h2: $BODY" ;;
esac
case "$BODY" in
  *'"body_len":4096'*) ;;
  *) fail "the 4096-byte PUT body must arrive intact over h2: $BODY" ;;
esac
pass "PUT 4096 B → server saw HTTP/2.0 and the whole body (body_len=4096)"

# ── 4. Reporting leg: an HTTP/1.1-only peer is served AND reported as 1.1 ───
BODY=$("$BIN" request GET "$URL_H1/probe" -v 2>"$WORK/h1.err") || fail "GET over the http/1.1-only fixture failed"
case "$BODY" in
  *'"server_proto":"HTTP/1.1"'*) ;;
  *) fail "the http/1.1-only fixture did not serve HTTP/1.1: $BODY" ;;
esac
grep -q 'HTTP: HTTP/1.1 (wire' "$WORK/h1.err" || fail "the report must say HTTP/1.1: $(cat "$WORK/h1.err")"
pass "HTTP/1.1-only peer → served, and reported as HTTP/1.1 (the version comes from the wire)"

# ── 5. Policy leg: a required version the peer cannot provide is a hard fail ─
set +e
OUT=$("$BIN" request GET "$URL_H1/probe" --http-version 2 -v 2>&1)
RC=$?
set -e
[ "$RC" -ne 0 ] || fail "--http-version 2 must fail against an HTTP/1.1-only peer"
case "$OUT" in
  *PROTOCOL_MISMATCH*) ;;
  *) fail "the failure must carry PROTOCOL_MISMATCH: $OUT" ;;
esac
case "$OUT" in
  *server-offered-http/1.1*) ;;
  *) fail "the failure must name the version the peer offered: $OUT" ;;
esac
pass "--http-version 2 vs an HTTP/1.1-only peer → exit $RC, PROTOCOL_MISMATCH (server-offered-http/1.1)"

# ── 6. Policy leg: 1.1 is forced even against an h2-capable peer ────────────
BODY=$("$BIN" request GET "$URL_H2/probe" --http-version 1.1 -v 2>"$WORK/force11.err") || fail "--http-version 1.1 failed against the h2 peer"
case "$BODY" in
  *'"server_proto":"HTTP/1.1"'*) ;;
  *) fail "policy 1.1 must speak HTTP/1.1 even to an h2-capable peer: $BODY" ;;
esac
grep -q 'HTTP: HTTP/1.1 (wire, requested 1.1)' "$WORK/force11.err" || fail "the report must say HTTP/1.1 with the policy in force: $(cat "$WORK/force11.err")"
pass "--http-version 1.1 vs the h2-capable peer → served over HTTP/1.1 (version selection is transport selection)"

# ── 7. Policy leg: an unreachable HTTP/3 is reported, not silent ────────────
# The h2 fixture is TCP-only, so there is no QUIC on its port: policy "3" must
# complete over HTTP/2 *and* say so on the diagnostic surface. (PROTO-014
# asserted here that --http-version 3 failed outright, because HTTP/3 did not
# exist yet; what replaced it is the reported fallback.)
set +e
OUT=$("$BIN" request GET "$URL_H2/probe" --http-version 3 -v 2>&1)
RC=$?
set -e
[ "$RC" -eq 0 ] || fail "--http-version 3 must complete over HTTP/2 when QUIC is unreachable: $OUT"
case "$OUT" in
  *'HTTP: HTTP/2.0 (wire, requested 3, fallback HTTP/3.0→HTTP/2.0:'*) ;;
  *) fail "the fallback must be reported on stderr: $OUT" ;;
esac
case "$OUT" in
  *'"server_proto":"HTTP/2.0"'*) ;;
  *) fail "the h2 endpoint must have carried the request: $OUT" ;;
esac
pass "--http-version 3 vs a TCP-only peer → exit $RC, served over HTTP/2.0, fallback HTTP/3.0→HTTP/2.0 reported"

# ── 8. An unrecognised policy value is rejected, never silently 'auto' ──────
set +e
OUT=$("$BIN" request GET "$URL_H2/probe" --http-version 2.0 2>&1)
RC=$?
set -e
[ "$RC" -ne 0 ] || fail "--http-version 2.0 must be rejected"
case "$OUT" in
  *VALIDATION_ERROR*) ;;
  *) fail "an unknown version must be a validation error: $OUT" ;;
esac
pass "--http-version 2.0 → exit $RC, VALIDATION_ERROR naming the accepted set"

echo ""
pass "Demo 19 complete: h2 negotiation, policy enforcement and version reporting verified end to end"
