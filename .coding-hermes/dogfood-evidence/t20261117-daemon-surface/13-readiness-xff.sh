#!/bin/bash
# dogfood: (1) readiness probe cost when the backend accepts but never answers,
#           (2) does that probe block other routes, (3) what happens to X-Forwarded-For
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
rm -f "$D/h.sock" "$D/h.pid" "$D/h.sock.lock"
"$BIN" start --socket-path "$D/h.sock" --pid-file "$D/h.pid" --metrics-addr 127.0.0.1:19170 \
  --backend-url http://127.0.0.1:19140 2>&1 | head -1
echo "=== /health/ready against a hung backend (3 probes) ==="
for i in 1 2 3; do
  curl -s -o "$D/h.out" -w "  probe $i -> code=%{http_code} ttfb=%{time_starttransfer} total=%{time_total}\n" \
    --max-time 20 --unix-socket "$D/h.sock" http://localhost/health/ready
  echo "     body=$(head -c 260 "$D/h.out")"
done
echo "=== while readiness is probing, is /health still fast? ==="
curl -s -o /dev/null -w '  /health -> %{http_code} ttfb=%{time_starttransfer}\n' --unix-socket "$D/h.sock" http://localhost/health
echo "=== live backend: X-Forwarded-For handling ==="
rm -f "$D/h2.sock" "$D/h2.pid" "$D/h2.sock.lock"
"$BIN" start --socket-path "$D/h2.sock" --pid-file "$D/h2.pid" --metrics-addr 127.0.0.1:19171 \
  --backend-url http://127.0.0.1:19103 2>&1 | head -1
echo "--- client sends its own X-Forwarded-For ---"
curl -s -H 'X-Forwarded-For: 203.0.113.9' -H 'X-Real-IP: 203.0.113.9' --unix-socket "$D/h2.sock" http://localhost/api/whoami -o "$D/xff1.out"
head -c 420 "$D/xff1.out"; echo
echo "--- client sends no XFF ---"
curl -s --unix-socket "$D/h2.sock" http://localhost/api/whoami -o "$D/xff2.out"
head -c 420 "$D/xff2.out"; echo
echo "=== does the daemon audit record proxied requests? (docs claim a hook on the recorder) ==="
grep -rn 'MUSTER_AUDIT_ENABLED' docs/guides/configuration.md | head -3
