#!/bin/bash
# dogfood: minimal repro — proxied write bodies arrive empty. 3 repeats + sizes.
D=/home/kara/.hermes/cache/scratch/dogfood-muster-t1
SOCK=$D/p3.sock
BODY='{"name":"x","n":1}'
for i in 1 2 3; do
  code=$(curl -s -o "$D/rep.out" -w '%{http_code}' -X POST --data-binary "$BODY" --unix-socket "$SOCK" http://localhost/api/items)
  printf 'repeat %s POST -> %s %s\n' "$i" "$code" "$(head -c 200 "$D/rep.out")"
done
# large body
BIG=$(head -c 4000 /dev/zero | tr '\0' 'a')
code=$(curl -s -o "$D/big.out" -w '%{http_code}' -X POST --data-binary "$BIG" --unix-socket "$SOCK" http://localhost/api/big)
printf 'large POST (4000B) -> %s %s\n' "$code" "$(head -c 160 "$D/big.out")"
# explicit Content-Length + json content type
code=$(curl -s -o "$D/cl.out" -w '%{http_code}' -X POST -H 'Content-Type: application/json' --data-binary "$BODY" --unix-socket "$SOCK" http://localhost/api/json)
printf 'json CT POST -> %s %s\n' "$code" "$(head -c 220 "$D/cl.out")"
# chunked from client
code=$(printf '%s' "$BODY" | curl -s -o "$D/ch.out" -w '%{http_code}' -X POST -H 'Transfer-Encoding: chunked' -H 'Content-Type: application/json' --data-binary @- --unix-socket "$SOCK" http://localhost/api/chunked)
printf 'client-chunked POST -> %s %s\n' "$code" "$(head -c 220 "$D/ch.out")"
# control: same body direct to backend
code=$(curl -s -o "$D/ctl.out" -w '%{http_code}' -X POST --data-binary "$BODY" http://127.0.0.1:19102/api/items)
printf 'CONTROL direct -> %s %s\n' "$code" "$(head -c 200 "$D/ctl.out")"
