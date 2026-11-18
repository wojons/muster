#!/bin/bash
# dogfood: the documented single-daemon guard refusal contract
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
SOCK=$D/p4.sock
PID=$D/p4.pid
echo "=== live daemon ==="
"$BIN" status --socket-path "$SOCK" --pid-file "$PID" 2>&1 | head -4
BEFORE_SUM=$(md5sum "$PID" | cut -d' ' -f1); BEFORE_MT=$(stat -c %Y "$PID")
echo "pidfile before: md5=$BEFORE_SUM mtime=$BEFORE_MT content=$(cat "$PID")"
echo "=== second start, daemonized ==="
"$BIN" start --socket-path "$SOCK" --pid-file "$PID" --metrics-addr 127.0.0.1:19114 > "$D/g1.out" 2> "$D/g1.err"
echo "rc=$?"
echo "stdout: $(cat "$D/g1.out")"
echo "stderr: $(cat "$D/g1.err")"
echo "=== second start, --foreground ==="
timeout 10 "$BIN" start --foreground --socket-path "$SOCK" --pid-file "$PID" --metrics-addr 127.0.0.1:19115 > "$D/g2.out" 2> "$D/g2.err"
echo "rc=$?"
echo "stdout: $(cat "$D/g2.out")"
echo "stderr: $(cat "$D/g2.err")"
echo "=== second start via proxy subcommand ==="
timeout 10 "$BIN" proxy --socket-path "$SOCK" --pid-file "$PID" --backend-url http://127.0.0.1:19103 > "$D/g3.out" 2> "$D/g3.err"
echo "rc=$?"
echo "stdout: $(cat "$D/g3.out")"
echo "stderr: $(cat "$D/g3.err")"
AFTER_SUM=$(md5sum "$PID" | cut -d' ' -f1); AFTER_MT=$(stat -c %Y "$PID")
echo "pidfile after:  md5=$AFTER_SUM mtime=$AFTER_MT"
echo "=== listeners on the socket ==="
curl -s -o /dev/null -w 'socket still answers -> %{http_code}\n' --unix-socket "$SOCK" http://localhost/health
echo "=== client token gate when backend token set but no client token ==="
printf 'up\n' > "$D/u2.token"
rm -f "$D/p5.sock" "$D/p5.pid" "$D/p5.sock.lock"
"$BIN" start --socket-path "$D/p5.sock" --pid-file "$D/p5.pid" --metrics-addr 127.0.0.1:19116 \
  --backend-url http://127.0.0.1:19103 --backend-token-file "$D/u2.token" > "$D/g4.out" 2> "$D/g4.err"
echo "rc=$?"; echo "stdout: $(cat "$D/g4.out")"; echo "stderr: $(cat "$D/g4.err")"
echo "=== empty credential file ==="
: > "$D/empty.token"
rm -f "$D/p6.sock" "$D/p6.pid" "$D/p6.sock.lock"
"$BIN" start --socket-path "$D/p6.sock" --pid-file "$D/p6.pid" --metrics-addr 127.0.0.1:19117 \
  --backend-url http://127.0.0.1:19103 --backend-token-file "$D/empty.token" \
  --client-token-file "$D/client.token" > "$D/g5.out" 2> "$D/g5.err"
echo "rc=$?"; echo "stdout: $(cat "$D/g5.out")"; echo "stderr: $(cat "$D/g5.err")"
