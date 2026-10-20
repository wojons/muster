#!/bin/bash
# Leg B: real-user credential round-trip through the CLI (DF-052 / QA-MUSTER-18-R2 reproof)
# Scratch credential values only; both removed at the end.
set -u
cd /home/kara/wojons/muster
CLI=./bin/openapi-cli
echo "=== 0) inkwell fixture health ==="
curl -s -o /dev/null -w 'health=%{http_code}\n' http://localhost:9803/health
echo "=== 0b) remove prior-turn residue (dogfood19) ==="
$CLI auth remove dogfood19 2>&1; echo "rc=$?"
echo "=== 1) auth add df052 (bearer reader-token) ==="
/usr/bin/time -f 'wall=%es' $CLI auth add --name df052 --type bearer --value reader-token 2>&1; echo "rc=$?"
echo "=== 2) auth list (metadata) ==="
$CLI auth list 2>&1; echo "rc=$?"
echo "=== 3) NEGATIVE CONTROL: /me without --auth ==="
$CLI request GET http://localhost:9803/me -o json 2>&1 | tail -4; echo "rc=${PIPESTATUS[0]}"
echo "=== 4) ROUND-TRIP PROOF: /me --auth df052 (value lives only in the keyring) ==="
$CLI request GET http://localhost:9803/me --auth df052 -o json 2>&1 | tail -8; echo "rc=${PIPESTATUS[0]}"
echo "=== 5) WRONG-VALUE CONTROL: same route, bogus token ==="
$CLI auth add --name df052bad --type bearer --value not-a-real-token >/dev/null 2>&1; echo "add rc=$?"
$CLI request GET http://localhost:9803/me --auth df052bad -o json 2>&1 | tail -4; echo "rc=${PIPESTATUS[0]}"
echo "=== 6) CLEANUP ==="
$CLI auth remove df052 2>&1; echo "rc=$?"
$CLI auth remove df052bad 2>&1; echo "rc=$?"
echo "--- auth list after cleanup ---"
$CLI auth list 2>&1
echo "--- credentials.json after cleanup ---"
cat /home/kara/.config/muster/credentials.json
