#!/bin/bash
# dogfood: daemon bring-up on a scratch socket; time it, then status
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
SOCK=$D/openapi-cli.sock
PID=$D/daemon.pid
rm -f "$SOCK" "$PID" "$SOCK.lock"
echo "=== start (cold, daemonized) ==="
/usr/bin/time -f "start_wall=%e rc=%x" "$BIN" start --socket-path "$SOCK" --pid-file "$PID" --metrics-addr 127.0.0.1:19110 2>&1
echo "=== status ==="
"$BIN" status --socket-path "$SOCK" --pid-file "$PID" 2>&1
echo "rc=$?"
echo "=== files ==="
ls -la "$SOCK" "$PID" "$SOCK.lock" 2>&1
echo "=== listening sockets for pid file ==="
cat "$PID" 2>/dev/null
