# Dogfood evidence — muster daemon HTTP surface

- Run: tick `muster-dogfood-2026-11-17-23-48-14` (dogfood lane for `muster`)
- Tree: `b3e50644` (`v0.1.2-0.20261117104148-b3e506449ffe`), clean, 0/0 vs `origin/main`
- Binaries: `go build -o …/openapi-daemon ./cmd/daemon`, `…/openapi-cli ./cmd/openapi-cli`
- Surface walked: the operator page `docs/guides/daemon-operations.md` + `docs/guides/commands.md` daemon
  section — the only documented surface that changed materially since the previous dogfood record
  (`d3a3f0e9`, 2026-11-14): 74 product files, +7655/−918 lines.
- Rows filed: `QA-MUSTER-052`, `QA-MUSTER-053`, `QA-MUSTER-054`

## What was verified against a live daemon (and held)

| Claim in the docs | Measured |
|---|---|
| `/health` → 200 `{"status":"healthy"}` | 200, exact body |
| `/health/live` → 200 `alive`+timestamp | 200, exact body |
| `/` → 200 `{"status":"ok","message":"Muster daemon is running"}` | 200, exact body |
| `/health/startup`, `/metrics` on the socket fall through to `/` | 200, root body (JSON) |
| socket mode `0600` | `srw-------` |
| metrics listener serves only `/metrics`; other paths 404 | 200 / `404 page not found` |
| metrics families + label sets (8 families, seed series, `{id}` collapsing) | matches the doc's tables byte-for-byte; `endpoint="/api/users/{id}"` observed under traffic |
| `status` prints `Running: true` + PID + real start time | yes, plus the correct version string |
| guard refusal: rc=1, `Error: daemon already running (PID N)`, PID file untouched, socket still serving | yes (daemonized, `--foreground`, and `proxy` subcommand — all three) |
| `--backend-token-file` without `--client-token-file` → refuse | rc=1, explicit message |
| empty credential file → refuse | rc=1, `backend credential file "…" is empty` |
| auth: health exempt, everything else requires the client token | 200 / 401 `{"error":"Unauthorized","message":"Missing authorization header"}` / 401 `…"Invalid or expired token"` |
| upstream credential injected, client credential not forwarded | backend saw `Authorization: Bearer upstream-secret-AAA`; client's own header was replaced |
| dead backend → `502`, `text/plain`, `Failed to execute proxy request` | exact |
| `proxy` subcommand forwards `/health*` to the backend too | yes — backend answered all four paths |
| `--metrics-addr` non-loopback → exact refusal text | exact |
| `reload` / `restart` = stop-then-start; `restart` with nothing running starts fresh | yes, with the documented messages |
| no subcommand / unknown subcommand → banner, rc=0 | yes |
| `-version` | `v0.1.2-0.20261117104148-b3e506449ffe (b3e506449ffe)` |
| `MUSTER_AUDIT_ENABLED=1` → audit families live; events land in `<config dir>/muster/audit.jsonl` | 3 proxied requests → `muster_audit_events_written_total{category="system",event_type="command.execute"} 3`, hash-chained entries with `method`/`path`/`status` |

## What did not hold (the filed rows)

1. **`/health/ready`** — doc says readiness is unconditional and "no known path returns a non-200 … treat it
   as down". Measured: 503 `{"status":"not_ready","reason":"backend_unreachable",…}` with a dead backend, and
   503 after **2.0013 / 2.0020 / 2.0009 s** with a backend that accepts and never answers
   (`readiness-hung-backend.json`). `/health` stayed at 200 in 0.2 ms during those probes, i.e. it does not
   block. → `QA-MUSTER-052`
2. **Metrics bind failure is silent in the default (daemonized) shape** — `warn-armA-foreground.err` has the
   documented `[WARN] metrics listener not started on …`: the claim holds in `--foreground`
   (`warn-armA-foreground.txt` is its stdout).
   `warn-armB-daemonized.txt` / `warn-armB-daemonized.err` are empty and both of the child's stdio fds point at
   `/dev/null` (`cmd/daemon/main.go:448-450`); no log destination exists. → `QA-MUSTER-053`

   *Capture naming: stdout captures are committed as `.txt` because `.gitignore:15` ignores `*.out` — a bare
   `git add <dir>` drops them silently. Both arms' bytes are otherwise unmodified.*
3. **`X-Forwarded-For: "@"`** on every proxied request — `xff-client-absent.json` and
   `xff-client-supplied.json`: a client-supplied XFF is replaced (good) with the Unix socket pseudo-host `@`
   (not an address). → `QA-MUSTER-054`

## Two harness bugs found and fixed in MY probes (recorded so the next fire does not re-file them)

- `backend2.py` read only `Content-Length`, so the daemon's chunked upstream requests looked like **dropped
  bodies** (3/3 "POST body_len=0"). `backend3.py` decodes chunked and the same calls carry the full body
  (`body_len=18`, `4000`, PUT/DELETE too). **The daemon forwards bodies correctly** — the first reading was the
  probe's, not the product's.
- The first latency run showed a constant **~41 ms** per proxied request (`perf-200-socket.oldbackend.txt`
  14.2 s / 200 vs `perf-200-direct.oldbackend.txt` 1.93 s / 200). Cause: the Python echo backend wrote headers
  and body as two small writes with Nagle on, meeting the peer's delayed ACK.
  `12-latency-control.sh` re-ran against a raw `TCP_NODELAY` single-write backend: **200 proxied = 1.700 s vs
  200 direct = 1.734 s**, i.e. the proxy hop is sub-millisecond and there is no perf finding.

## Performance (Step 2b) — no row, and the numbers behind that

- cold start (`start`, daemonizes, socket + PID live): **0.05 s** on 5/5 runs, `user`/`sys` 0.00
- proxied request TTFB: **0.38–0.55 ms** (200 sequential calls 1.700 s, ~8.5 ms/call incl. curl startup)
- direct to the same backend: 1.734 s / 200 — **the daemon adds ≤0.1 ms**
- daemon-local `/health`: **0.18–0.23 ms**
- metrics scrape (38 lines): **~10 ms**
- `openapi-daemon status`: ~7 ms per call (20 calls, 0.148 s)

Nothing here is slow enough that a user would feel it, so no `PERF-n` row was minted: a second row over the
same sub-millisecond hop would devalue the existing `PERF-001`/`PERF-002` findings.

## Board writes

- `boardctl validate -C . --fail-on dangling-dep`: rc=0 before (388 rows) and rc=0 after (391 rows); no
  error line names a filed id. `validate-before.txt` / `validate-after.txt`.
- Minter: `board_next_id.py <canonical origin/main board> QA-MUSTER --count 3` → 052/053/054, each proven free
  on the canonical copy and in the checkout before the append.
- Paired `task_created` events: ids 1123, 1124, 1125.

## Not filed (seen, deliberately)

- 401 rejections (missing/invalid client token) are not counted in `muster_daemon_proxy_requests_total` — auth
  happens before the proxy, and the doc only promises one point per *proxied* request. Recorded here, no row:
  the honest fix is a design decision (an auth-rejection counter), not a drift repair.
- Audit records proxied requests as `event_type=command.execute`, `action=command.daemon.request`. Naming
  quirk inside an otherwise usable hash-chained trail; no behaviour is wrong, so no row.
