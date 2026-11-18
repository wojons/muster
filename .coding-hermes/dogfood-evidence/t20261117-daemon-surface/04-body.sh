#!/bin/bash
# dogfood: does the daemon proxy forward request bodies? measure bytes at the backend
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
SOCK=$D/p3.sock
PID=$D/p3.pid
rm -f "$SOCK" "$PID" "$SOCK.lock"
"$BIN" start --socket-path "$SOCK" --pid-file "$PID" --metrics-addr 127.0.0.1:19112 \
  --backend-url http://127.0.0.1:19102 2>&1 | head -3

send() { # label method path body-arg
  local label="$1" m="$2" p="$3" body="$4"
  local code
  if [ -n "$body" ]; then
    code=$(curl -s -o "$D/r3.out" -w '%{http_code}' -X "$m" --data-binary "$body" \
      --unix-socket "$SOCK" "http://localhost$p")
  else
    code=$(curl -s -o "$D/r3.out" -w '%{http_code}' -X "$m" --unix-socket "$SOCK" "http://localhost$p")
  fi
  printf '%-30s %-6s %-16s -> %s  %s\n' "$label" "$m" "$p" "$code" "$(head -c 320 "$D/r3.out")"
}

echo "=== direct to backend (control arm) ==="
curl -s -o "$D/d3.out" -X POST --data-binary '{"direct":true}' http://127.0.0.1:19102/direct >/dev/null
head -c 400 "$D/d3.out"; echo
echo "=== through daemon socket ==="
send "POST json" POST /api/items '{"name":"x","n":1}'
send "PUT  json" PUT /api/items/7 '{"name":"y"}'
send "POST empty" POST /api/ping ''
send "GET  no body" GET /api/users ''
echo "=== raw request from client side (what curl sends) ==="
curl -s -o /dev/null -w 'client-side direct POST -> %{http_code} content_type=%{content_type}\n' -X POST --data-binary '{"z":1}' http://127.0.0.1:19102/x
