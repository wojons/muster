#!/bin/bash
# dogfood: 502 shape, proxy-mode health composition, metrics addr rules
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
echo "=== dead backend (start --backend-url pointing nowhere) ==="
rm -f "$D/p7.sock" "$D/p7.pid" "$D/p7.sock.lock"
"$BIN" start --socket-path "$D/p7.sock" --pid-file "$D/p7.pid" --metrics-addr 127.0.0.1:19118 \
  --backend-url http://127.0.0.1:19999 2>&1 | head -2
for path in /health /health/live /health/ready / /some/path; do
  curl -s -o "$D/dead.out" -w "  $path -> %{http_code} ctype=%{content_type} " --unix-socket "$D/p7.sock" "http://localhost$path"
  echo "body=$(cat "$D/dead.out")"
done
echo "=== metrics after a 5xx ==="
curl -s http://127.0.0.1:19118/metrics -o "$D/m7.text"
grep -E 'proxy_requests_total|proxy_duration_seconds_count' "$D/m7.text"
echo "=== proxy mode: health should be forwarded to the backend ==="
printf '{"from":"backend","path":"REQ"}\n' > /dev/null
timeout 6 "$BIN" proxy --socket-path "$D/p8.sock" --pid-file "$D/p8.pid" --backend-url http://127.0.0.1:19103 > "$D/pm.out" 2> "$D/pm.err" &
PMPID=$!
sleep 1.5
for path in /health /health/live /health/ready /; do
  curl -s -o "$D/pm.o" -w "  proxy-mode $path -> %{http_code} " --unix-socket "$D/p8.sock" "http://localhost$path"
  echo "body=$(head -c 120 "$D/pm.o")"
done
echo "proxy stdout: $(head -3 "$D/pm.out")"
wait $PMPID 2>/dev/null
echo "=== metrics addr rules ==="
rm -f "$D/p9.sock" "$D/p9.pid" "$D/p9.sock.lock"
"$BIN" start --socket-path "$D/p9.sock" --pid-file "$D/p9.pid" --metrics-addr 0.0.0.0:19119 > "$D/ma.out" 2> "$D/ma.err"
echo "non-loopback rc=$? err=$(cat "$D/ma.err")"
echo "=== busy metrics port -> WARN, daemon still serves ==="
rm -f "$D/p10.sock" "$D/p10.pid" "$D/p10.sock.lock"
"$BIN" start --socket-path "$D/p10.sock" --pid-file "$D/p10.pid" --metrics-addr 127.0.0.1:19103 > "$D/mb.out" 2> "$D/mb.err"
echo "rc=$? stdout=$(cat "$D/mb.out") stderr=$(cat "$D/mb.err")"
curl -s -o /dev/null -w '  daemon socket after busy metrics port -> %{http_code}\n' --unix-socket "$D/p10.sock" http://localhost/health
