#!/bin/bash
# dogfood: the CLI surface the guide tells operators to use — restart, reload, no-subcommand banner, -version
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
SOCK=$D/n.sock
echo "=== no subcommand ==="
"$BIN" > "$D/s1.out" 2> "$D/s1.err"; echo "rc=$?"; head -8 "$D/s1.out"; echo "stderr=$(cat "$D/s1.err")"
echo "=== unknown subcommand ==="
"$BIN" frobnicate > "$D/s2.out" 2> "$D/s2.err"; echo "rc=$?"; head -3 "$D/s2.out"; echo "stderr=$(cat "$D/s2.err")"
echo "=== -version ==="
"$BIN" -version 2>&1 | head -3; echo "rc=$?"
echo "=== reload (guide: runs stop-then-start) ==="
"$BIN" reload --socket-path "$SOCK" --pid-file "$D/n.pid" --metrics-addr 127.0.0.1:19160 --backend-url http://127.0.0.1:19104 > "$D/s3.out" 2> "$D/s3.err"
echo "rc=$?"; cat "$D/s3.out"; cat "$D/s3.err"
echo "  status after reload:"; "$BIN" status --socket-path "$SOCK" --pid-file "$D/n.pid" 2>&1 | head -4
echo "  serving?"; curl -s -o /dev/null -w '  GET /health -> %{http_code}\n' --unix-socket "$SOCK" http://localhost/health
echo "=== restart ==="
"$BIN" restart --socket-path "$SOCK" --pid-file "$D/n.pid" --metrics-addr 127.0.0.1:19160 --backend-url http://127.0.0.1:19104 > "$D/s4.out" 2> "$D/s4.err"
echo "rc=$?"; cat "$D/s4.out"; cat "$D/s4.err"
"$BIN" status --socket-path "$SOCK" --pid-file "$D/n.pid" 2>&1 | head -4
curl -s -o /dev/null -w '  GET /health after restart -> %{http_code}\n' --unix-socket "$SOCK" http://localhost/health
echo "=== restart with nothing running ==="
"$BIN" stop --socket-path "$D/n.sock" --pid-file "$D/n.pid" > /dev/null 2>&1
"$BIN" restart --socket-path "$D/n.sock" --pid-file "$D/n.pid" --metrics-addr 127.0.0.1:19160 --backend-url http://127.0.0.1:19104 > "$D/s5.out" 2> "$D/s5.err"
echo "rc=$?"; cat "$D/s5.out"; cat "$D/s5.err"
"$BIN" stop --socket-path "$D/n.sock" --pid-file "$D/n.pid" 2>&1 | head -2
