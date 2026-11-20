# Daemon Operations Guide

Operational reference for the Muster background daemon's HTTP surface: health
endpoints, metrics, socket access, load-balancer / systemd probes, and
Prometheus scraping.

Every route, status code, and response body below is reproducible against a
daemon built from this tree: [Verification log](#verification-log) rebuilds the
binary, starts it against the backends used here, and prints the exact bodies
quoted. If a claim and that log disagree, the log wins. The daemon ships two
ways: the
standalone `openapi-daemon` binary (subcommands `start`, `stop`, `status`,
`restart`, `reload`, `proxy`; socket flag `--socket-path`) and the CLI's
`openapi-cli daemon <subcommand>` wrapper (flag `--socket`). The HTTP surface is
the same — the wrapper execs the daemon binary — with one wrapper difference
worth knowing before debugging health: `openapi-cli daemon start --backend-url
URL` spawns `openapi-daemon proxy`, so **health is proxied there**. Only the
standalone `openapi-daemon start --backend-url URL` serves daemon-local,
backend-aware health. Examples here use the standalone form. See
[Proxy vs standalone socket composition](#proxy-vs-standalone-socket-composition).

## TL;DR — route table

| Route | Served by | Method | Status | Response body (`application/json`) |
|---|---|---|---|---|
| `/health` | daemon socket | any (GET/HEAD/POST all answer 200) | `200 OK` | `{"status":"healthy"}` |
| `/health/live` | daemon socket | any | `200 OK` | `{"status":"alive","timestamp":"2006-01-02T15:04:05Z"}` |
| `/health/ready`, no backend | daemon socket | any | `200 OK` | `{"status":"ready","timestamp":"2006-01-02T15:04:05Z"}` |
| `/health/ready`, `start --backend-url` | daemon socket, probing the backend | any | `200 OK` ready, `503 Service Unavailable` not_ready | `{"status":"ready","backend_status":200,"timestamp":"…"}` or a `not_ready` body (see [`GET /health/ready`](#get-healthready)) |
| `/` | daemon socket | any | `200 OK` | `{"status":"ok","message":"Muster daemon is running"}` |
| `/health/startup`, no backend | daemon socket | any | falls through to `/` | root body, `200 OK` |
| `/metrics`, no backend | daemon socket | any | falls through to `/` | root body, `200 OK` (JSON, not Prometheus) |
| `/metrics` | metrics listener, `127.0.0.1:9110` | GET/HEAD | `200 OK` | Prometheus text exposition |
| any other path | metrics listener, `127.0.0.1:9110` | any | `404 Not Found` | `404 page not found` (plain text) |

The "no backend" rows describe `start` without `--backend-url`. With
`--backend-url` the daemon registers exactly three local health paths —
`/health`, `/health/live`, `/health/ready` — and proxies **everything else** on
the socket to the backend, `/health/startup` and `/metrics` included (measured:
both returned the test backend's own body, not the daemon's root JSON).

Two different servers: **`/metrics` is not part of the daemon's socket handler**
— without `--backend-url` that path falls through to the canned root JSON (with
a backend URL it is proxied to the backend). Prometheus
format comes from a separate loopback TCP listener
(`internal/monitoring.MusterMetricsServer`) that the daemon starts itself once
the socket answers: default `127.0.0.1:9110`, override with `--metrics-addr`.
That listener serves `/metrics` and nothing else (no `/health*`, no proxied
path). See "Prometheus scrape configuration" below for the flag, the
loopback-only rule, and the exact families and label sets it exposes.

### Auth exemption

Health endpoints are the one class of routes that bypass the daemon's token
authentication middleware (`pkg/daemon` `withTokenAuth`, which calls
`isHealthEndpoint`):
`/health`, `/health/live`, `/health/ready`, `/health/startup` (a trailing
slash is normalized). Load balancers and supervisors can probe them without
a token. Token auth only exists when the socket is served by the authenticated
proxy (`NewAuthenticatedProxyServer`, i.e. a configured token manager); the
`openapi-daemon` binary serves the plain proxy handler, where no path asks for
`Authorization` at all. In the authenticated shape, every non-health path does.

## Getting a socket

The daemon serves HTTP over a Unix domain socket, not TCP. Default socket:
`/tmp/openapi-cli.sock` (override with `--socket-path`; PID file defaults to
`<socket>.pid`).

```bash
# Start (daemonizes by default; --foreground keeps it attached)
openapi-daemon start

# Or run the proxy directly
openapi-daemon proxy --backend-url http://127.0.0.1:8080

# All examples below use curl's --unix-socket
curl --unix-socket /tmp/openapi-cli.sock http://localhost/health
```

The socket is created with mode `0600` (owner-only). Probes must run as the
same user as the daemon, or you must grant access via group/ACL on the path.

## Endpoint reference

All bodies below are exact, byte-for-byte, from a live daemon. Timestamps
are RFC 3339 UTC.

### `GET /health`

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"healthy"}
```

- `200` as long as the daemon serves this path itself and the process can answer
  on its socket (the standalone and `start --backend-url` shapes; under `proxy`
  the path is forwarded to the backend — see [Proxy vs
  standalone](#proxy-vs-standalone-socket-composition)). The local handler never
  consults the backend, the audit sink, the metrics listener, or any other
  subsystem, so it cannot report a degraded state: a 200 here proves the daemon
  process can serve requests on its socket, and nothing more. A backend outage
  leaves it at 200 (measured: 200 in 0.2–2 ms while `/health/ready` was
  blocked on a hung backend).
- No timestamp field.
- The generic "is the daemon up" probe, and the right one for restart
  decisions. systemd should use this (or `/health/live`), never `/health/ready`
  — see [Semantics](#semantics-for-lb-and-systemd). It is not an LB health
  check: it stays 200 while the backend behind the daemon is down.

### `GET /health/live`

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"alive","timestamp":"2026-09-27T06:38:07Z"}
```

- Liveness, and the same unconditional 200 as `/health` wherever the daemon
  serves it locally (`start` with or without `--backend-url`): the only thing it
  proves is that the process can serve the socket. It never probes the backend.
  Under `proxy` it is forwarded like any other path. The difference from
  `/health` is cosmetic — the body carries a timestamp.

### `GET /health/ready`

Behaviour depends on how the socket is served. There are three shapes, and only
the middle one is a real readiness contract.

**1. `start` without `--backend-url`** — no backend exists to probe, so the
canned handler answers. This is a liveness answer under a readiness name; do not
put it behind an LB.

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"ready","timestamp":"2026-09-27T06:38:07Z"}
```

**2. `start --backend-url <url>`** — real readiness. Every request probes
`<url>/health` with a 2 s client budget and reports one of three outcomes
(`cmd/daemon/main.go`, `backendReadinessHandler`):

| Backend probe result | Status | Body |
|---|---|---|
| reachable, HTTP status < 500 | `200 OK` | `{"status":"ready","backend_status":<code>,"timestamp":"…"}` |
| reachable, HTTP status ≥ 500 | `503 Service Unavailable` | `{"status":"not_ready","reason":"backend_unhealthy","backend_status":<code>,"timestamp":"…"}` |
| unreachable, refused, or no answer within 2 s | `503 Service Unavailable` | `{"status":"not_ready","reason":"backend_unreachable","error":"<probe error>","timestamp":"…"}` |

Measured bodies, byte-for-byte (timestamps are RFC 3339 UTC):

```json
{"status":"ready","backend_status":200,"timestamp":"2026-11-20T14:28:21Z"}
```

```json
{"status":"not_ready","reason":"backend_unhealthy","backend_status":503,"timestamp":"2026-11-20T14:28:21Z"}
```

```json
{"status":"not_ready","reason":"backend_unreachable","error":"Get \"http://127.0.0.1:1/health\": dial tcp 127.0.0.1:1: connect: connection refused","timestamp":"2026-11-20T14:27:46Z"}
```

Against a backend that accepts the connection and never answers, the probe
returns the same `backend_unreachable` shape after the full budget, with
`error` ending `context deadline exceeded (Client.Timeout exceeded while
awaiting headers)`. Measured 2.001–2.008 s across nine consecutive calls (three
runs of three) while the host was busy: the 2 s client timeout is the floor an
LB sees, and a hung backend makes every readiness probe cost that much.

The classification rule is worth stating plainly because it is asymmetric:
**anything below 500 counts as ready.** A backend answering `404` on `/health`
produces `{"status":"ready","backend_status":404,…}` with HTTP 200 — the probe
checks reachability and non-5xx, not the backend's opinion of the path.

**3. `proxy` subcommand** — no local composition at all, so `/health/ready` is
forwarded to the backend and the status/body are the backend's (see
[`proxy` subcommand](#proxy-subcommand)).

A 503 from `/health/ready` is a statement about the backend, never about the
daemon process: `/health` and `/health/live` keep answering 200 in that same
state (measured — both 200 while readiness spent 2 s and returned 503).

### `GET /` (root info)

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"ok","message":"Muster daemon is running"}
```

- In the standalone shape (no `--backend-url`) also answers `/health/startup`
  and `/metrics` on the socket (unregistered paths fall through to `/` in Go's
  `http.ServeMux`). With `--backend-url` those two paths are proxied to the
  backend instead, because only `/health`, `/health/live` and `/health/ready`
  are registered locally.

### Method behavior

Handlers do not dispatch on method. GET, HEAD, and POST all return `200`
with the same body (HEAD omits the body). Don't rely on method-based
rejection; it doesn't exist.

### Response when things are wrong

- `/health/ready` with `--backend-url` and a dead backend:
  `503 Service Unavailable`, `application/json`,
  `{"status":"not_ready","reason":"backend_unreachable","error":"…"}`. The
  daemon process is fine; the backend behind it is not.
- `/health/ready` with `--backend-url` and a backend answering 5xx:
  `503 Service Unavailable`, `{"status":"not_ready","reason":"backend_unhealthy","backend_status":<code>,…}`.
- Proxied (non-health) paths with a dead backend: `502 Bad Gateway`,
  `text/plain`, body `Failed to execute proxy request`. In `start` mode
  `/health` and `/health/live` stay local and unaffected; `/health/ready` turns
  the same outage into the 503 above. In `proxy` mode the health paths are
  proxied too, so they fail with whatever the backend/proxy chain returns.
- Authenticated proxy shape only: a non-health path without a valid token gets
  `401 Unauthorized` with `{"error":"Unauthorized","message":"…"}`.

`/health` and `/health/live` are the two endpoints that never return a non-200
**while the daemon serves them locally** (`start`, with or without
`--backend-url`): their handlers are unconditional and consult nothing. Any
non-200 you see there in those shapes means the process itself is broken or gone
— treat it as down and restart. (In `proxy` mode they are forwarded to the
backend, so their status is the backend's; a non-200 there is still not evidence
about the daemon process.) A non-200 from `/health/ready` means the opposite:
leave the process alone and look at the backend.

## Proxy vs standalone socket composition

The daemon has two serving shapes, and the health/metrics composition
differs between them — this is the main operational gotcha.

### Standalone (no backend)

`openapi-daemon start` without `--backend-url` serves the canned handler on
the socket: `/` plus the three health routes. Nothing else exists.

### `start` + `--backend-url`

The socket handler is composed: `/health`, `/health/live` and `/health/ready`
are handled **locally by the daemon**, everything else is reverse-proxied to the
backend. `/health/ready` is the local handler that probes the backend, so a
backend outage shows up as a 503 on that one path while `/health` and
`/health/live` keep answering 200.

```bash
openapi-daemon start --backend-url http://127.0.0.1:8080
curl --unix-socket /tmp/openapi-cli.sock http://localhost/health
# -> local canned 200, even if the backend is down
curl --unix-socket /tmp/openapi-cli.sock http://localhost/health/ready
# -> 200 when the backend answers /health below 500, 503 not_ready otherwise
curl --unix-socket /tmp/openapi-cli.sock http://localhost/some/api
# -> proxied to the backend; 502 if it's down
```

### `proxy` subcommand

**The health endpoints are proxied to the backend too.** The standalone
`proxy` subcommand does not compose the local canned health handler —
`/health`, `/health/live`, `/health/ready` are forwarded to the backend like
any other path (measured: a test backend answering every path with
`{"status":"healthy"}` produced that body on all three, and a file server as
backend returned its own 404 for `/health`). There is no daemon-side readiness
check in this shape: whatever your backend says about `/health/ready` is what
the socket returns.

Operational consequence: with `proxy` mode, a "200 from `/health`" proves
your **backend** is up, not the daemon. If you front `proxy` mode with an
LB, either probe the daemon process via systemd/socket liveness instead, or
accept that the probe semantics are the backend's. Prefer `start
--backend-url` when you need daemon-local health with a real backend behind
it.

## Semantics for LB and systemd

### Which endpoint for which consumer

| Consumer | Use | Why |
|---|---|---|
| systemd `ExecStartPost` / wait-for-start | `curl --unix-socket ... /health` | process is up once the socket answers |
| systemd `ExecCondition` / watchdog script | `/health/live` | restart-on-fail semantics; when served locally it never returns non-200 because of the backend (not usable as a daemon check under `proxy`) |
| Load balancer pool membership | `/health/ready` (via a local relay), with `start --backend-url` | real readiness: 503 not_ready when the backend is down or unhealthy; **do not** use it in standalone mode or `proxy` mode, where it is not a daemon-side check |
| Kubernetes (if socket is exposed via a sidecar) | live = `/health/live`, ready = `/health/ready` | convention mapping; same caveat — readiness is only meaningful with `--backend-url` |

### What a 503 means: pool membership vs a daemon failure

The two failure modes look similar from an LB and require opposite actions:

| Observation | Meaning | Action |
|---|---|---|
| `/health/ready` → `503`, body `reason=backend_unreachable` or `backend_unhealthy`, while `/health` and `/health/live` still answer `200` | the daemon process is healthy; the backend behind it is down, 5xx, or slower than the 2 s probe budget | take the node **out of the LB pool**; fix the backend. Do **not** restart the daemon — a restart changes nothing about the backend and costs you the socket |
| `/health` or `/health/live` → non-200, or no answer at all (connection refused / socket missing / relay timeout) | the daemon process itself is gone or wedged | restart the unit / re-establish the relay |

Inspect the reason without parsing prose:

```bash
curl -s --unix-socket /tmp/openapi-cli.sock http://localhost/health/ready \
  | jq -r '"\(.status) reason=\(.reason // "-") backend=\(.backend_status // "-")"'
# not_ready reason=backend_unhealthy backend=503
# not_ready reason=backend_unreachable backend=-
# ready     reason=-                  backend=200
```

A probe on `/health/ready` is not free when the backend is hung: it costs the
full 2 s budget (measured 2.001–2.008 s). Size the LB's `timeout` above 2 s so
a hung backend produces a clean 503 instead of an LB-side timeout, and keep
`/health/live` on a shorter budget for restart decisions.


### systemd example

Because the surface is socket-only, systemd checks need either a curl shim
or `ExecStartPost` polling. A minimal unit fragment:

```ini
[Unit]
Description=Muster daemon
After=network.target

[Service]
Type=notify
ExecStart=/usr/local/bin/openapi-daemon start --foreground
# Liveness: restart the unit when the socket stops answering.
# A tiny curl loop doubles as the watchdog hook.
ExecStartPost=/bin/sh -c 'for i in $(seq 1 20); do \
  curl -fsS --unix-socket /tmp/openapi-cli.sock http://localhost/health && exit 0; \
  sleep 0.5; done; exit 1'
Restart=on-failure
RestartSec=2

[Install]
WantedBy=multi-user.target
```

`-f` makes curl fail on non-2xx, which is what you want: any non-200 means
the process is unhealthy or gone. `Restart=on-failure` then provides the
restart action.

### Load balancer example

An LB cannot dial a Unix socket directly. The common patterns:

1. **TCP relay** (e.g. `systemd-socket-proxyd`, nginx `stream`, or socat)
   exposing the socket on localhost TCP, then point the LB health check at
   `/health/ready` through the relay.
2. **Probe the backend directly** if the daemon is `proxy` mode (health is
   backend-served there anyway — see above).

nginx TCP relay fragment:

```nginx
stream {
    upstream muster_daemon {
        server unix:/tmp/openapi-cli.sock;
    }
    server {
        listen 127.0.0.1:8081;
        proxy_pass muster_daemon;
    }
}
```

LB health check then targets `http://<host>:8081/health/ready`. Treat the
outcomes differently: `200` in rotation; `503` out of rotation but leave the
daemon running (the backend behind it is down — see
[What a 503 means](#what-a-503-means-pool-membership-vs-a-daemon-failure));
connection refused / relay timeout out of rotation **and** restart the daemon.
Keep the check's timeout above 2 s, since that is the readiness probe's own
budget.

## Prometheus scrape configuration

### The metrics listener

- The daemon **socket** does not expose Prometheus format — its `/metrics`
  path falls through to the root JSON info handler (standalone) or is proxied to
  the backend (`start --backend-url`).
- Prometheus format is served by `internal/monitoring.MusterMetricsServer`,
  which the daemon starts itself once the socket answers so the first scrape
  already reports the truth. Construction:
  `monitoring.NewMusterMetricsServer(metricsAddr, metricsRecorder.Muster())` —
  `cmd/daemon/main.go:294-305`.
- Default address `127.0.0.1:9110` — `internal/monitoring/muster.go:495`
  (`monitoring.DefaultMetricsAddr`). It is the `--metrics-addr` flag default
  (`cmd/daemon/main.go:115`) and appears in `--help`
  (`cmd/daemon/main.go:261`).
- Path `/metrics` only (`internal/monitoring/muster.go:536-537`); every other
  path on that port answers `404 page not found`.
- The address must be loopback — `127.0.0.0/8`, `::1`, or `localhost` —
  enforced at startup by `ValidateLoopbackAddr`
  (`internal/monitoring/muster.go:500-522`, called from
  `cmd/daemon/main.go:154-156`). `--metrics-addr 0.0.0.0:9110` exits with
  `Error: metrics addr "0.0.0.0:9110": 0.0.0.0 is not a loopback address; the
  metrics listener binds loopback only`.
- A busy port is a WARNING, not a fatal: the daemon keeps serving the socket
  and prints `[WARN] metrics listener not started on …`
  (`cmd/daemon/main.go:299-302`). On success it prints
  `Metrics: http://127.0.0.1:9110/metrics` (`cmd/daemon/main.go:303`).
- Listener timeouts are `ReadHeaderTimeout: 5s`, `ReadTimeout: 10s`,
  `WriteTimeout: 10s` (`internal/monitoring/muster.go:544-549`), so keep
  `scrape_timeout` at or below 10s.
- The legacy `internal/monitoring.MetricsServer`
  (`internal/monitoring/metrics.go:292-325`, which serves the DEFAULT
  registry plus `/health*`) is retained as a library and is **not**
  constructed by any binary. Do not point a scrape at it.

### Metric families served on 127.0.0.1:9110

One explicit registry per daemon process, built by
`monitoring.NewMusterMetrics` (`internal/monitoring/muster.go:102-232`) and
served with `promhttp.HandlerFor` (`internal/monitoring/muster.go:264-268`).
Only these instruments are registered (`internal/monitoring/muster.go:194-205`),
so the scrape contains no `go_*` or `process_*` collectors.

| Metric | Type | Labels | Defined at |
|---|---|---|---|
| `muster_build_info` | gauge (always 1) | `version`, `commit` | `internal/monitoring/muster.go:111-118` (set to 1 at `:209`) |
| `muster_daemon_up` | gauge | `tenant_id` | `internal/monitoring/muster.go:119-126` (`MarkUp` `:279-284`, `MarkDown` `:289-294`) |
| `muster_daemon_proxy_requests_total` | counter | `endpoint`, `method`, `status_class`, `tenant_id` | `internal/monitoring/muster.go:127-134` |
| `muster_daemon_proxy_duration_seconds` | histogram | `endpoint`, `method` | `internal/monitoring/muster.go:135-143` |
| `muster_audit_events_written_total` | counter | `event_type`, `category` | `internal/monitoring/muster.go:144-151` |
| `muster_audit_events_dropped_total` | counter | `reason` (`buffer_full`, `write_error`) | `internal/monitoring/muster.go:152-159` |
| `muster_audit_buffer_utilization_ratio` | gauge | — | `internal/monitoring/muster.go:160-164` |
| `muster_mcp_tool_executions_total` | counter | `tool`, `status`, `error_class` + const `product="muster"` | `internal/monitoring/muster.go:165-174` |
| `muster_mcp_tool_duration_seconds` | histogram | `tool`, `status` + const `product="muster"` | `internal/monitoring/muster.go:175-185` |
| `muster_mcp_active_sessions` | gauge | const `product="muster"` | `internal/monitoring/muster.go:186-191` |

Label values are bounded by construction: `status_class` is `1xx`…`5xx` or
`unknown` (`StatusClass`, `internal/monitoring/muster.go:394-409`);
`endpoint` collapses opaque path segments to `{id}`
(`NormalizeEndpoint`, `internal/monitoring/muster.go:415-430`);
`error_class` is one of `none`/`auth`/`upstream`/`validation`/`internal`
(`NormalizeMCPErrorClass`, `internal/monitoring/muster.go:330-337`); and
`tenant_id` is `MUSTER_TENANT_ID` when set, otherwise `default`
(`cmd/daemon/main.go:63-68`, `internal/monitoring/muster.go:28`).

### Series present on a freshly started daemon

`NewMusterMetrics` seeds the series that must be visible before anything has
been recorded (`internal/monitoring/muster.go:207-229`). Verified against a
daemon started from this tree (`--backend-url` pointing at a local upstream,
then three proxied requests). Histogram bucket lines are elided — all twelve
`le` buckets sit at 0:

```
muster_build_info{commit="unknown",version="dev"} 1
muster_daemon_up{tenant_id="default"} 1
muster_daemon_proxy_requests_total{endpoint="/",method="none",status_class="none",tenant_id="default"} 0
muster_daemon_proxy_duration_seconds_count{endpoint="/",method="none"} 0
muster_audit_events_written_total{category="none",event_type="none"} 0
muster_audit_events_dropped_total{reason="buffer_full"} 0
muster_audit_events_dropped_total{reason="write_error"} 0
muster_audit_buffer_utilization_ratio 0
muster_mcp_active_sessions{product="muster"} 0
```

### Observed label values under real traffic

The same daemon after three proxied requests (upstream unreachable, so every
call is a 5xx) and with `MUSTER_AUDIT_ENABLED=1`:

```
muster_daemon_proxy_requests_total{endpoint="/",method="GET",status_class="5xx",tenant_id="default"} 1
muster_daemon_proxy_requests_total{endpoint="/",method="none",status_class="none",tenant_id="default"} 0
muster_daemon_proxy_requests_total{endpoint="/missing.txt",method="GET",status_class="5xx",tenant_id="default"} 1
muster_daemon_proxy_duration_seconds_count{endpoint="/missing.txt",method="GET"} 1
muster_audit_events_written_total{category="none",event_type="none"} 0
muster_audit_events_written_total{category="system",event_type="command.execute"} 3
```

Two details worth knowing when writing queries: the seed series keeps its
zero value alongside the real ones (so an unfiltered `sum()` is still correct),
and `endpoint` is the sanitized request path — `{id}` substitution applies only
to opaque identifier segments, so a static path like `/missing.txt` is
reported verbatim.

`daemon_up` starts at `0` and is raised to `1` once the socket answers, so a
scrape during startup reports "not answering yet" rather than a lie. The
`none`/`{id}` label values never come from real traffic, so a seed series is
unambiguous in a dashboard. `muster_mcp_tool_executions_total` and
`muster_mcp_tool_duration_seconds` are registered but have no child series
until something records a tool call — a fresh daemon does not expose them.

### Who records what

- **Proxy traffic:** `pkg/daemon` observes one point per proxied request
  through `SetMetrics`/`RecordDaemonProxyRequest`
  (`pkg/daemon/proxy.go:579-592`; deferred call at `pkg/daemon/proxy.go:716-726`),
  landing on `muster_daemon_proxy_requests_total` (with `status_class`) and
  `muster_daemon_proxy_duration_seconds`.
- **Audit health:** the daemon builds `audit.NewHookWithMetrics(...)` on the
  same recorder (`cmd/daemon/main.go:198-206`), so
  `muster_audit_events_written_total`, `muster_audit_events_dropped_total` and
  `muster_audit_buffer_utilization_ratio` are live — but audit is off unless
  `MUSTER_AUDIT_ENABLED=1` (`internal/audit/hooks.go:218-229`), so a default
  install exposes only the seed series.
- **MCP families:** the three `muster_mcp_*` instruments live on the same
  registry, but the daemon never records MCP tool calls. Their real producer
  is the stdio `mcp-server`, which publishes a registry scoped to just those
  families (`MusterMetrics.MCPRegistry`, `internal/monitoring/muster.go:253-260`)
  into a node_exporter textfile (`cmd/mcp-server/main.go:253-262`; default
  path `/var/lib/node_exporter/textfile/muster_mcp.prom`, override with
  `MUSTER_MCP_METRICS_PATH` or `MUSTER_TEXTFILE_DIR` —
  `internal/monitoring/textfile.go:16-31`).

### Families that are NOT on this port

The process-scoped families declared with `promauto` in
`internal/monitoring/metrics.go:25-105` register on the DEFAULT prometheus
registry, not on the daemon's explicit one, so they never appear at
`127.0.0.1:9110`: `muster_cli_command_executions_total`,
`muster_cli_command_duration_seconds`, `muster_cli_command_errors_total`,
`muster_mcp_active_connections`, `muster_mcp_requests_total`,
`muster_mcp_request_duration_seconds`,
`muster_mcp_tool_executions_total{tool_name,status}` (the legacy `tool_name`
spelling), `muster_daemon_active_socket_connections`, and
`muster_daemon_proxy_requests_total{endpoint,method,tenant_id}` (the legacy
spelling without `status_class`). Two of those names collide with served
families under a different label set; whatever a scrape of `:9110` returns
carries the label sets in the table above.

### prometheus.yml

```yaml
scrape_configs:
  - job_name: "muster-daemon"
    scrape_interval: 15s
    scrape_timeout: 10s       # listener WriteTimeout is 10s; do not exceed
    metrics_path: /metrics
    static_configs:
      - targets: ["127.0.0.1:9110"]
        labels:
          service: "muster-daemon"
```

Notes for operators:

- The listener binds loopback only (enforced at startup), so Prometheus must
  run on the same host or reach it through an SSH tunnel / socket relay.
- `--metrics-addr` has no environment-variable equivalent; put it on the unit's
  `ExecStart` line if you need a different port.
- Running several daemons on one host? Give each its own `--metrics-addr`; the
  second bind fails with the warning above and that daemon simply has no
  metrics endpoint.
- `/health*` is on the Unix socket only; the metrics port answers `404` for it.

### Verifying a scrape

```bash
# Families, label sets and values:
curl -fsS http://127.0.0.1:9110/metrics | grep -E '^muster_'

# Build identity of the running binary:
curl -fsS http://127.0.0.1:9110/metrics | grep '^muster_build_info'

# What the socket answers on the same path (root JSON, not Prometheus):
curl --unix-socket /tmp/openapi-cli.sock http://localhost/metrics

# Anything other than /metrics is not served on the metrics port:
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:9110/health   # 404

# Prometheus accepts the target (in the Prometheus web UI or):
promtool check config /etc/prometheus/prometheus.yml
```

## Troubleshooting quick reference

| Symptom | Likely cause | Fix |
|---|---|---|
| `/health` 200 but API calls 502 | backend down (proxy paths are remote) | restart backend; `/health` and `/health/live` are daemon-local in `start` mode, and `/health/ready` should be 503 in this state |
| `/health/ready` 503 `backend_unreachable` / `backend_unhealthy` | backend down, 5xx, or slower than the 2 s probe | fix the backend; do not restart the daemon (its `/health` is still 200) |
| `/health` returns backend's HTML/404 | you're on `proxy` mode — health is proxied there | switch to `start --backend-url` for daemon-local health |
| `curl --unix-socket` fails to connect | socket path wrong, or wrong user (socket is 0600) | check `openapi-daemon status`; run curl as daemon user |
| `/metrics` returns JSON not Prometheus text | you scraped the Unix socket (standalone mode); that path falls through to `/` | scrape `http://127.0.0.1:9110/metrics` instead of the socket |
| `/metrics` returns the backend's body on the socket | `start --backend-url` — only the three health paths are local | use `--metrics-addr` / `http://127.0.0.1:9110/metrics` |
| `curl 127.0.0.1:9110/metrics` → connection refused | the listener never bound (port already taken) or the daemon is not running in `start`/`proxy` mode | look for `[WARN] metrics listener not started` in the startup log; free the port or pass a different `--metrics-addr` |
| daemon exits with `is not a loopback address` | `--metrics-addr` named a non-loopback host | use `127.0.0.1:<port>`, `[::1]:<port>`, or `localhost:<port>` |
| `404` when probing `/health` on the metrics port | the metrics listener serves `/metrics` only | probe `/health` on the Unix socket |
| LB marks node down though daemon runs | LB can't reach a Unix socket | add a TCP relay; with `--backend-url` a 503 there means the backend, not the daemon |
| systemd unit flaps at start | daemonize delay before socket exists | poll socket in `ExecStartPost` (see unit above) |

## Verification log

Reproduce any claim in this document:

```bash
go build -o /tmp/muster-daemon ./cmd/daemon

WORK=$(mktemp -d)

# ---- Case A: backend refusing connections (nothing on port 1) --------------
/tmp/muster-daemon start --foreground \
  --socket-path "$WORK/a.sock" --metrics-addr 127.0.0.1:19121 \
  --backend-url http://127.0.0.1:1/ & A=$!
for i in $(seq 1 50); do [ -S "$WORK/a.sock" ] && break; sleep 0.1; done

curl -s -w ' [%{http_code}]\n' --unix-socket "$WORK/a.sock" http://localhost/health
# {"status":"healthy"} [200]                       <- daemon is alive
curl -s -w ' [%{http_code}]\n' --unix-socket "$WORK/a.sock" http://localhost/health/live
# {"status":"alive","timestamp":"..."} [200]
curl -s -w ' [%{http_code}]\n' --unix-socket "$WORK/a.sock" http://localhost/health/ready
# {"status":"not_ready","reason":"backend_unreachable","error":"Get \"http://127.0.0.1:1/health\": dial tcp 127.0.0.1:1: connect: connection refused","timestamp":"..."} [503]
curl -s --unix-socket "$WORK/a.sock" http://localhost/health/ready \
  | jq -e '.status == "not_ready" and .reason == "backend_unreachable" and (.error | length) > 0'
# true                                             <- machine-checkable shape

# ---- Case B: backend answering 5xx ----------------------------------------
python3 - <<'PY' & B=$!
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(503)
        self.send_header("Content-Length", "0")
        self.end_headers()
    def log_message(self, *a):
        pass
HTTPServer(("127.0.0.1", 19302), H).serve_forever()
PY
for i in $(seq 1 50); do curl -s -o /dev/null http://127.0.0.1:19302/health && break; sleep 0.1; done
/tmp/muster-daemon start --foreground \
  --socket-path "$WORK/b.sock" --metrics-addr 127.0.0.1:19122 \
  --backend-url http://127.0.0.1:19302 & D=$!
for i in $(seq 1 50); do [ -S "$WORK/b.sock" ] && break; sleep 0.1; done
curl -s -w ' [%{http_code}]\n' --unix-socket "$WORK/b.sock" http://localhost/health/ready
# {"status":"not_ready","reason":"backend_unhealthy","backend_status":503,"timestamp":"..."} [503]

# ---- Case C: backend that accepts and never answers ------------------------
# The probe spends its whole 2 s budget and reports backend_unreachable.
python3 - <<'PY' & C=$!
import socket, time
s = socket.socket()
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(("127.0.0.1", 19303)); s.listen(8)
held = []
while True:
    conn, _ = s.accept()
    held.append(conn)   # accepted, never read, never answered
    time.sleep(3600)
PY
for i in $(seq 1 50); do (exec 3<>/dev/tcp/127.0.0.1/19303) 2>/dev/null && { exec 3<&-; break; }; sleep 0.1; done
/tmp/muster-daemon start --foreground \
  --socket-path "$WORK/c.sock" --metrics-addr 127.0.0.1:19123 \
  --backend-url http://127.0.0.1:19303 & E=$!
for i in $(seq 1 50); do [ -S "$WORK/c.sock" ] && break; sleep 0.1; done
for i in 1 2 3; do
  curl -s -o /dev/null -w 'ready %{http_code} %{time_total}s\n' \
    --unix-socket "$WORK/c.sock" http://localhost/health/ready
done
# ready 503 2.004s      (the 2 s probe budget; 2.001-2.008 s observed)
# ready 503 2.005s
# ready 503 2.008s
curl -s -o /dev/null -w 'health %{http_code} %{time_total}s\n' \
  --unix-socket "$WORK/c.sock" http://localhost/health
# health 200 0.002s      <- liveness stays instant (0.0002-0.002 s observed)
curl -s --unix-socket "$WORK/c.sock" http://localhost/health/ready | jq -r .error
# Get "http://127.0.0.1:19303/health": context deadline exceeded (Client.Timeout exceeded while awaiting headers)

# ---- Case D: healthy backend ----------------------------------------------
python3 - <<'PY' & F=$!
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        body = b'{"status":"healthy"}'
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *a):
        pass
HTTPServer(("127.0.0.1", 19301), H).serve_forever()
PY
for i in $(seq 1 50); do curl -s -o /dev/null http://127.0.0.1:19301/health && break; sleep 0.1; done
/tmp/muster-daemon start --foreground \
  --socket-path "$WORK/d.sock" --metrics-addr 127.0.0.1:19124 \
  --backend-url http://127.0.0.1:19301 & G=$!
for i in $(seq 1 50); do [ -S "$WORK/d.sock" ] && break; sleep 0.1; done
curl -s -w ' [%{http_code}]\n' --unix-socket "$WORK/d.sock" http://localhost/health/ready
# {"status":"ready","backend_status":200,"timestamp":"..."} [200]
curl -s -w ' [%{http_code}]\n' --unix-socket "$WORK/d.sock" http://localhost/metrics
# {"status":"healthy"} [200]                      <- /metrics is PROXIED in this shape

# ---- Standalone shape: no backend -----------------------------------------
/tmp/muster-daemon start --foreground \
  --socket-path "$WORK/s.sock" --metrics-addr 127.0.0.1:19125 & S=$!
for i in $(seq 1 50); do [ -S "$WORK/s.sock" ] && break; sleep 0.1; done
curl -s -w ' [%{http_code}]\n' --unix-socket "$WORK/s.sock" http://localhost/health/ready
# {"status":"ready","timestamp":"..."} [200]      <- liveness under a readiness name
curl -s -w ' [%{http_code}]\n' --unix-socket "$WORK/s.sock" http://localhost/health/startup
# {"status":"ok","message":"Muster daemon is running"} [200]
curl -s -w ' [%{http_code}]\n' --unix-socket "$WORK/s.sock" http://localhost/metrics
# {"status":"ok","message":"Muster daemon is running"} [200]  <- JSON, not Prometheus
curl -fsS http://127.0.0.1:19125/metrics | grep -E '^muster_'  # the seed series
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:19125/health   # 404

kill "$A" "$B" "$C" "$D" "$E" "$F" "$G" "$S" 2>/dev/null
```

The Prometheus-side claims (families, label sets, seed series, loopback-only
binding) are reproduced by the listener checks in
[Verifying a scrape](#verifying-a-scrape).