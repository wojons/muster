#!/bin/bash
# dogfood: body forwarding through the daemon proxy, backend decodes chunked correctly
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
SOCK=$D/p4.sock
PID=$D/p4.pid
rm -f "$SOCK" "$PID" "$SOCK.lock"
"$BIN" start --socket-path "$SOCK" --pid-file "$PID" --metrics-addr 127.0.0.1:19113 \
  --backend-url http://127.0.0.1:19103 2>&1 | head -2

show() { head -c 400 "$1"; echo; }
BODY='{"name":"x","n":1}'
echo "=== direct control ==="
curl -s -o "$D/b-ctl.out" -X POST --data-binary "$BODY" -H 'Content-Type: application/json' http://127.0.0.1:19103/api/items
show "$D/b-ctl.out"
echo "=== proxied, client sends Content-Length ==="
curl -s -o "$D/b-px.out" -X POST --data-binary "$BODY" -H 'Content-Type: application/json' --unix-socket "$SOCK" http://localhost/api/items
show "$D/b-px.out"
echo "=== proxied, 4 KB body ==="
BIG=$(head -c 4000 /dev/zero | tr '\0' 'a')
curl -s -o "$D/b-big.out" -X POST --data-binary "$BIG" --unix-socket "$SOCK" http://localhost/api/big
show "$D/b-big.out"
echo "=== proxied, PUT ==="
curl -s -o "$D/b-put.out" -X PUT --data-binary "$BODY" --unix-socket "$SOCK" http://localhost/api/items/7
show "$D/b-put.out"
echo "=== proxied, DELETE with body ==="
curl -s -o "$D/b-del.out" -X DELETE --data-binary "$BODY" --unix-socket "$SOCK" http://localhost/api/items/7
show "$D/b-del.out"
