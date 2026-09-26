#!/bin/bash
# Demo 11: .proto support — descriptor-set closure + the schema surface
#
# Gate this script serves: PROTO-016 (G-CLOSURE + G-SCHEMA S1–S3), the two gate
# definitions appended to specs/PROTOCOLS.md §6.
#
# Everything here is a committed fixture under pkg/protocol/grpc/testdata: no
# network, no protoc at run time, no fixture generation.
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }

echo "═══════════════════════════════════════════"
echo " Demo 11: .proto Support (closure + schema)"
echo "═══════════════════════════════════════════"

BIN=./bin/openapi-cli
FIX=pkg/protocol/grpc/testdata
[ -x "$BIN" ] || fail "$BIN not found — run 'go build -o bin/openapi-cli ./cmd/openapi-cli' first"

# ── 1. The two surfaces are distinct: no service ⇒ no operations, four types ──
DISCOVER_OUT=$("$BIN" grpc discover "$FIX/msgs_only.proto" 2>&1) || fail "discover of a service-less .proto errored: $DISCOVER_OUT"
echo "$DISCOVER_OUT" | grep -q "No operations found." || fail "a service-less .proto must discover 0 operations: $DISCOVER_OUT"
pass "grpc discover msgs_only.proto → 0 operations (RPC extraction stays services-only)"

SCHEMA_OUT=$("$BIN" grpc schema "$FIX/msgs_only.proto" 2>&1) || fail "schema of a service-less .proto errored: $SCHEMA_OUT"
echo "$SCHEMA_OUT" | grep -q "4 types" || fail "a service-less .proto must still describe 4 types: $SCHEMA_OUT"
echo "$SCHEMA_OUT" | grep -q "only.Thing" || fail "the type listing must name only.Thing: $SCHEMA_OUT"
pass "grpc schema msgs_only.proto → 4 types (the same file, described as a type graph)"

# ── 2. G-CLOSURE: a set without --include_imports is closed, not fatal ───────
NODEPS_OUT=$("$BIN" grpc discover "$FIX/schema_nodeps.pb" 2>&1) || fail "a bare descriptor set must be closed from the runtime registry: $NODEPS_OUT"
echo "$NODEPS_OUT" | grep -q "schema.Desc-Do" || fail "expected schema.Desc-Do: $NODEPS_OUT"
ALLDEPS_OUT=$("$BIN" grpc discover "$FIX/schema_all.pb" 2>&1) || fail "discover of schema_all.pb errored: $ALLDEPS_OUT"
[ "$NODEPS_OUT" = "$ALLDEPS_OUT" ] || fail "without/with --include_imports must discover identically:
  nodeps: $NODEPS_OUT
  all:    $ALLDEPS_OUT"
pass "grpc discover schema_nodeps.pb (no --include_imports) → same operations as schema_all.pb"

# ── 3. G-CLOSURE negative branch: loud, early, with the remedy named ─────────
set +e
LEGIBLE_OUT=$("$BIN" grpc discover "$FIX/main_nodeps.pb" 2>&1)
LEGIBLE_RC=$?
set -e
[ "$LEGIBLE_RC" -ne 0 ] || fail "a set whose import cannot be resolved must fail"
echo "$LEGIBLE_OUT" | grep -q "dep.proto" || fail "the failure must name the missing file: $LEGIBLE_OUT"
echo "$LEGIBLE_OUT" | grep -q -- "--include_imports" || fail "the failure must name the remedy: $LEGIBLE_OUT"
pass "grpc discover main_nodeps.pb → exit $LEGIBLE_RC, naming dep.proto and the remedy"

# ── 4. Depth-1 fidelity reaches the machine-readable output ─────────────────
JSON_OUT=$("$BIN" grpc schema "$FIX/schema.proto" -o json 2>&1) || fail "schema -o json errored: $JSON_OUT"
echo "$JSON_OUT" | grep -q "schema.Inner" || fail "JSON must carry the nested type reference: $JSON_OUT"
echo "$JSON_OUT" | grep -q "date-time" || fail "JSON must map the well-known Timestamp to its JSON form: $JSON_OUT"
echo "$JSON_OUT" | grep -q "COLOR_UNSPECIFIED" || fail "JSON must carry the enum value names: $JSON_OUT"
pass "grpc schema schema.proto -o json → refs, WKT JSON forms and enum values present"

echo ""
pass "Demo 11 complete: closure fix + schema surface verified end to end"
