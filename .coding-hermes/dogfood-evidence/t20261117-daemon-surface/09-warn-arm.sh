#!/bin/bash
# dogfood: is the documented "[WARN] metrics listener not started" observable in each deploy shape?
# The busy port is 127.0.0.1:19103, already held by a live echo backend.
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
BUSY=19103
echo "occupant check: $(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:$BUSY/)"
echo "=== ARM A: foreground, busy metrics port ==="
timeout 4 "$BIN" start --foreground --socket-path "$D/f1.sock" --pid-file "$D/f1.pid" --metrics-addr 127.0.0.1:$BUSY > "$D/f1.out" 2> "$D/f1.err"
echo "rc=$? (124 = still running at timeout, expected)"
echo "--- stdout ---"; cat "$D/f1.out"
echo "--- stderr ---"; cat "$D/f1.err"
echo "=== ARM B: daemonized (the default), busy metrics port ==="
rm -f "$D/f2.sock" "$D/f2.pid" "$D/f2.sock.lock"
"$BIN" start --socket-path "$D/f2.sock" --pid-file "$D/f2.pid" --metrics-addr 127.0.0.1:$BUSY > "$D/f2.out" 2> "$D/f2.err"
echo "rc=$?"
echo "--- stdout ---"; cat "$D/f2.out"
echo "--- stderr ---"; cat "$D/f2.err"
DPID=$(cat "$D/f2.pid" 2>/dev/null)
echo "daemon pid=$DPID ; its stdio targets:"
ls -l /proc/$DPID/fd/1 /proc/$DPID/fd/2 2>/dev/null
echo "any daemon log file on disk?"
ls -la /var/log/openapi-cli /tmp/muster*.log 2>/dev/null | head -4
echo "=========== EOF ===========" > /dev/null
echo "=== stop the daemonized arm ==="
"$BIN" stop --socket-path "$D/f2.sock" --pid-file "$D/f2.pid" 2>&1
