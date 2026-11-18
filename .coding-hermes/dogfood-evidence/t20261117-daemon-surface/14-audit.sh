#!/bin/bash
# dogfood: does MUSTER_AUDIT_ENABLED=1 give an operator an audit trail of proxied requests?
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
export MUSTER_AUDIT_ENABLED=1
rm -f "$D/a.sock" "$D/a.pid" "$D/a.sock.lock"
"$BIN" start --socket-path "$D/a.sock" --pid-file "$D/a.pid" --metrics-addr 127.0.0.1:19180 \
  --backend-url http://127.0.0.1:19103 2>&1 | head -2
echo "=== three proxied requests ==="
for p in /api/one /api/two /api/missing.txt; do
  curl -s -o /dev/null -w "  $p -> %{http_code}\n" --unix-socket "$D/a.sock" "http://localhost$p"
done
echo "=== audit families after traffic ==="
curl -s http://127.0.0.1:19180/metrics -o "$D/ma.text"
grep -E 'muster_audit' "$D/ma.text"
echo "=== where did the audit events go? ==="
ls -la "$D"/audit* /tmp/muster-audit* 2>/dev/null | head -5
find "$D" -name '*audit*' -newermt '-3 minutes' 2>/dev/null | head -10
echo "=== default audit sink paths mentioned in docs ==="
grep -rn 'audit.*sink\|audit log\|\.jsonl' docs/guides/configuration.md | head -6
"$BIN" stop --socket-path "$D/a.sock" --pid-file "$D/a.pid" > /dev/null 2>&1
unset MUSTER_AUDIT_ENABLED
