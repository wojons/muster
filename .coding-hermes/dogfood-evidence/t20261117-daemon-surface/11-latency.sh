#!/bin/bash
# dogfood: where do the ~60ms go on a proxied request?
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
SOCK=$D/p4.sock
FMT='connect=%{time_connect} ttfb=%{time_starttransfer} total=%{time_total} code=%{http_code}\n'
echo "=== 8 proxied GETs, per-call timing ==="
for i in $(seq 1 8); do
  curl -s -o /dev/null -w "$FMT" --unix-socket "$SOCK" http://localhost/api/users
done
echo "=== 8 direct GETs to the same backend ==="
for i in $(seq 1 8); do
  curl -s -o /dev/null -w "$FMT" http://127.0.0.1:19103/api/users
done
echo "=== 4 proxied health (local handler) ==="
for i in $(seq 1 4); do
  curl -s -o /dev/null -w "$FMT" --unix-socket "$SOCK" http://localhost/health
done
