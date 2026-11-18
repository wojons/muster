#!/bin/bash
# dogfood: re-measure proxy latency against a TCP_NODELAY single-write backend,
# and against the two-write Python backend, to attribute the 40ms.
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
FMT='ttfb=%{time_starttransfer} total=%{time_total}\n'
rm -f "$D/n.sock" "$D/n.pid" "$D/n.sock.lock"
"$BIN" start --socket-path "$D/n.sock" --pid-file "$D/n.pid" --metrics-addr 127.0.0.1:19160 \
  --backend-url http://127.0.0.1:19104 2>&1 | head -1
echo "=== 6 proxied GETs -> nodelay single-write backend ==="
for i in $(seq 1 6); do curl -s -o /dev/null -w "$FMT" --unix-socket "$D/n.sock" http://localhost/api/users; done
echo "=== 6 direct GETs -> same backend (control) ==="
for i in $(seq 1 6); do curl -s -o /dev/null -w "$FMT" http://127.0.0.1:19104/api/users; done
echo "=== 200 proxied GETs, wall clock ==="
time (for i in $(seq 1 200); do curl -s -o /dev/null --unix-socket "$D/n.sock" http://localhost/api/users; done) 2>&1 | grep real
echo "=== 200 direct GETs, wall clock ==="
time (for i in $(seq 1 200); do curl -s -o /dev/null http://127.0.0.1:19104/api/users; done) 2>&1 | grep real
