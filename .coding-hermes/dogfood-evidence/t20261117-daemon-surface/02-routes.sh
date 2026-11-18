#!/bin/bash
# dogfood: documented route table, exactly as docs/guides/daemon-operations.md states it
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
SOCK=$D/openapi-cli.sock
probe() { # method path label -> prints code + body
  local m="$1" p="$2" label="$3"
  local out="$D/probe.$$.out"
  local code
  code=$(curl -s -o "$out" -w '%{http_code}' -X "$m" --unix-socket "$SOCK" "http://localhost$p")
  printf '%-6s %-18s -> %s  body=%s\n' "$m" "$p" "$code" "$(head -c 300 "$out")"
  rm -f "$out"
}
echo "=== socket routes ==="
probe GET /health
probe HEAD /health
probe POST /health
probe GET /health/live
probe GET /health/ready
probe GET /
probe GET /health/startup
probe GET /metrics
probe GET /health/
probe GET /no/such/path
echo "=== metrics listener 127.0.0.1:19110 ==="
MCODE=$(curl -s -o $D/metrics.text -w '%{http_code}' http://127.0.0.1:19110/metrics)
echo "GET :19110/metrics -> $MCODE bytes=$(wc -c < $D/metrics.text) lines=$(wc -l < $D/metrics.text)"
head -12 $D/metrics.text
echo "... families:"
grep -c '^# HELP' $D/metrics.text
echo "=== metrics listener other path ==="
curl -s -o $D/m4.out -w 'GET :19110/health -> %{http_code} body=%{size_download}\n' http://127.0.0.1:19110/health
cat $D/m4.out; echo
echo "=== default metrics addr 9110 (docs claim default) ==="
curl -s -o /dev/null -w 'GET 127.0.0.1:9110/metrics -> %{http_code}\n' --max-time 3 http://127.0.0.1:9110/metrics
