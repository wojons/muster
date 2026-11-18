#!/bin/bash
# dogfood Step 2b: what the daemon costs in time
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
SOCK=$D/p4.sock
echo "=== cold start (socket create + ready), 5 runs, empty state ==="
for i in 1 2 3 4 5; do
  rm -f "$D/c.sock" "$D/c.pid" "$D/c.sock.lock"
  /usr/bin/time -f "  run $i start_wall=%e user=%U sys=%S" "$BIN" start --socket-path "$D/c.sock" --pid-file "$D/c.pid" --metrics-addr 127.0.0.1:19150 > /dev/null 2> "$D/c.err"
  tail -1 "$D/c.err"
  "$BIN" stop --socket-path "$D/c.sock" --pid-file "$D/c.pid" > /dev/null 2>&1
done
echo "=== warm request latency: 200 sequential GETs ==="
echo "--- through the daemon socket (proxy hop + upstream) ---"
time (for i in $(seq 1 200); do curl -s -o /dev/null --unix-socket "$SOCK" http://localhost/api/users; done) 2> "$D/t-sock.txt"
grep real "$D/t-sock.txt"
echo "--- direct to the same backend (control arm) ---"
time (for i in $(seq 1 200); do curl -s -o /dev/null http://127.0.0.1:19103/api/users; done) 2> "$D/t-dir.txt"
grep real "$D/t-dir.txt"
echo "--- health through the daemon (local handler, no upstream) ---"
time (for i in $(seq 1 200); do curl -s -o /dev/null --unix-socket "$SOCK" http://localhost/health; done) 2> "$D/t-health.txt"
grep real "$D/t-health.txt"
echo "=== metrics scrape cost (100 scrapes) ==="
time (for i in $(seq 1 100); do curl -s -o /dev/null http://127.0.0.1:19112/metrics; done) 2> "$D/t-met.txt"
grep real "$D/t-met.txt"
echo "=== CLI command latency: status (n=20) ==="
time (for i in $(seq 1 20); do "$BIN" status --socket-path "$SOCK" --pid-file "$D/p4.pid" > /dev/null; done) 2> "$D/t-status.txt"
grep real "$D/t-status.txt"
