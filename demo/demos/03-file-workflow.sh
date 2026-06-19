#!/bin/bash
# Demo 03: File workflow against ByteStash
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }

echo "═══ Demo 03: File Workflow — ByteStash ═══"
HOST=http://localhost:9802
curl -sf "$HOST/health" > /dev/null || fail "ByteStash not running"

echo "upload" > /tmp/muster-demo.txt
echo -n "Uploading... "
UPLOAD=$(curl -sf -X POST "$HOST/uploads" -H "Content-Type: application/json" -d '{"filename":"demo.txt","folder_id":""}')
UPLOAD_ID=$(echo "$UPLOAD" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
curl -sf -X PUT "$HOST/uploads/$UPLOAD_ID/parts/1" --data-binary @/tmp/muster-demo.txt > /dev/null
curl -sf -X POST "$HOST/uploads/$UPLOAD_ID/complete" > /dev/null
FID=$(curl -sf "$HOST/uploads/$UPLOAD_ID" | python3 -c "import sys,json; print(json.load(sys.stdin).get('file_id',''))")
[ -n "$FID" ] || fail "upload"
pass "uploaded (id=$FID)"

echo -n "Download verify... "
[ "$(curl -sf $HOST/files/$FID/download)" = "upload" ] || fail "content mismatch"
pass "content verified"

echo -n "Trash... "
curl -sf -X DELETE "$HOST/files/$FID" > /dev/null
TC=$(curl -sf "$HOST/trash" | python3 -c "import sys,json; print(len(json.load(sys.stdin)))")
[ "$TC" -gt 0 ] || fail "trash empty"
pass "in trash"

echo -n "Recover... "
TID=$(curl -sf "$HOST/trash" | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['id'])")
curl -sf -X POST "$HOST/trash/$TID/recover" > /dev/null
curl -sf "$HOST/files/$FID" > /dev/null || fail "recover"
pass "recovered"
rm -f /tmp/muster-demo.txt
pass "File workflow complete"
