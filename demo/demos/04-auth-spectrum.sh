#!/bin/bash
# Demo 04: Auth spectrum — Inkwell
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }
echo "═══ Demo 04: Auth Spectrum — Inkwell ═══"
HOST=http://localhost:9803
curl -sf "$HOST/health" > /dev/null || fail "Inkwell not running"

echo -n "No token: "
[ "$(curl -s -o /dev/null -w '%{http_code}' $HOST/posts)" = "200" ] || fail "GET"
[ "$(curl -s -o /dev/null -w '%{http_code}' -X POST $HOST/posts -H 'Content-Type: application/json' -d '{}')" = "401" ] || fail "POST 401"
pass "read OK, write 401"

echo -n "Reader: "
[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer reader-token' $HOST/posts)" = "200" ] || fail "read"
[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer reader-token' -X POST $HOST/posts -H 'Content-Type: application/json' -d '{}')" = "403" ] || fail "write"
pass "can read, cannot write (403)"

echo -n "Author: "
[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer author-token' -X POST $HOST/posts -H 'Content-Type: application/json' -d '{"title":"Demo"}')" = "201" ] || fail "create"
pass "can create (201)"

echo -n "Editor: "
DID=$(curl -sf -H "Authorization: Bearer editor-token" "$HOST/posts?status=draft" | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['id'])")
[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer editor-token' -X PATCH $HOST/posts/$DID -H 'Content-Type: application/json' -d '{"status":"review"}')" = "200" ] || fail "review"
pass "can review drafts"

echo -n "Admin: "
[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer admin-token' -X PATCH $HOST/posts/$DID -H 'Content-Type: application/json' -d '{"status":"published"}')" = "200" ] || fail "publish"
pass "can publish"
pass "Auth spectrum complete"
