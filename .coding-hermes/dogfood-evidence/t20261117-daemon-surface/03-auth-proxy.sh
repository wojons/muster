#!/bin/bash
# dogfood: authenticated proxy, upstream credential injection, metrics labels, 502 shape
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
BIN=$D/bin/openapi-daemon
SOCK=$D/p2.sock
PID=$D/p2.pid
BACKEND=http://127.0.0.1:19101
printf 'upstream-secret-AAA\n' > "$D/upstream.token"
printf 'client-token-BBB\n' > "$D/client.token"
rm -f "$SOCK" "$PID" "$SOCK.lock"

echo "=== start with backend + tokens ==="
"$BIN" start --socket-path "$SOCK" --pid-file "$PID" --metrics-addr 127.0.0.1:19111 \
  --backend-url "$BACKEND" --backend-token-file "$D/upstream.token" \
  --client-token-file "$D/client.token" 2>&1
echo "rc=$?"

p() { # label method path extra-curl-args...
  local label="$1" m="$2" p="$3"; shift 3
  local code
  code=$(curl -s -o "$D/r.out" -w '%{http_code}' -X "$m" "$@" --unix-socket "$SOCK" "http://localhost$p")
  printf '%-34s %s %-24s -> %s %s\n' "$label" "$m" "$p" "$code" "$(head -c 220 "$D/r.out")"
}

echo "=== no token ==="
p "health (no token)" GET /health
p "proxied (no token)" GET /api/users
echo "=== correct client token ==="
p "health (token)" GET /health -H "Authorization: Bearer client-token-BBB"
p "proxied (token)" GET /api/users -H "Authorization: Bearer client-token-BBB"
p "proxied + id segment" GET /api/users/12345 -H "Authorization: Bearer client-token-BBB"
p "proxied POST (token)" POST /api/items -H "Authorization: Bearer client-token-BBB" -d '{"a":1}'
echo "=== wrong token ==="
p "proxied (wrong token)" GET /api/users -H "Authorization: Bearer nope"
echo "=== metrics after traffic ==="
curl -s -o "$D/m2.text" -w 'metrics -> %{http_code}\n' http://127.0.0.1:19111/metrics
grep -E 'muster_daemon_proxy_requests_total|muster_daemon_proxy_duration_seconds_count|muster_build_info|muster_daemon_up' "$D/m2.text"
cat "$D/p2.pid" 2>/dev/null
