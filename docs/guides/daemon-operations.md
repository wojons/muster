# Daemon Operations Guide

Operational reference for the Muster background daemon's HTTP surface: health
endpoints, metrics, socket access, load-balancer / systemd probes, and
Prometheus scraping.

Every route, status code, and response body below was verified against a
running daemon built from this tree. The daemon ships two ways: the
standalone `openapi-daemon` binary (subcommands `start`, `stop`, `status`,
`restart`, `reload`, `proxy`; socket flag `--socket-path`) and the CLI's
`openapi-cli daemon <subcommand>` wrapper (flag `--socket`). The HTTP
surface is identical; examples here use the standalone form.

## TL;DR — route table

| Route | Served by | Method | Success status | Response body (`application/json`) |
|---|---|---|---|---|
| `/health` | daemon socket | any (GET/HEAD/POST all answer 200) | `200 OK` | `{"status":"healthy"}` |
| `/health/live` | daemon socket | any | `200 OK` | `{"status":"alive","timestamp":"2006-01-02T15:04:05Z"}` |
| `/health/ready` | daemon socket | any | `200 OK` | `{"status":"ready","timestamp":"2006-01-02T15:04:05Z"}` |
| `/` | daemon socket | any | `200 OK` | `{"status":"ok","message":"Muster daemon is running"}` |
| `/health/startup` | daemon socket | any | falls through to `/` | root body, `200 OK` |
| `/metrics` | daemon socket | any | falls through to `/` | root body, `200 OK` (JSON, not Prometheus) |
| `/metrics` | metrics listener, `127.0.0.1:9110` | GET/HEAD | `200 OK` | Prometheus text exposition |
| any other path | metrics listener, `127.0.0.1:9110` | any | `404 Not Found` | `404 page not found` (plain text) |

Two different servers: **`/metrics` is not part of the daemon's socket handler**
— on the socket that path falls through to the canned root JSON. Prometheus
format comes from a separate loopback TCP listener
(`internal/monitoring.MusterMetricsServer`) that the daemon starts itself once
the socket answers: default `127.0.0.1:9110`, override with `--metrics-addr`.
That listener serves `/metrics` and nothing else (no `/health*`, no proxied
path). See "Prometheus scrape configuration" below for the flag, the
loopback-only rule, and the exact families and label sets it exposes.

### Auth exemption

Health endpoints are the one class of routes that bypass the daemon's token
authentication middleware (`pkg/daemon` `withTokenAuth`):
`/health`, `/health/live`, `/health/ready`, `/health/startup` (a trailing
slash is normalized). Load balancers and supervisors can probe them without
a token; every other path on the socket requires `Authorization`.

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

- `200` as long as the process is up and the socket answers. There is
  currently no degraded/down branch: the handler is unconditional.
- No timestamp field.
- The generic "is the daemon up" probe. systemd should use this (or
  `/health/live`) rather than `/health/ready`, since ready is equally
  unconditional today — see [Semantics](#semantics-for-lb-and-systemd).

### `GET /health/live`

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"alive","timestamp":"2026-09-27T06:38:07Z"}
```

- Liveness. Same unconditional-200 caveat as `/health`: the process being
  able to serve the socket is the only thing it proves.

### `GET /health/ready`

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"ready","timestamp":"2026-09-27T06:38:07Z"}
```

- Readiness. Currently identical in behavior to liveness — the handler does
  not consult backend connectivity or any subsystem state. A proxy daemon
  with a dead backend still answers ready/200 (verified: only non-health
  paths return `502` in that state).

### `GET /` (root info)

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"ok","message":"Muster daemon is running"}
```

- Also answers `/health/startup` and `/metrics` on the socket (unregistered
  paths fall through to `/` in Go's `http.ServeMux`).

### Method behavior

Handlers do not dispatch on method. GET, HEAD, and POST all return `200`
with the same body (HEAD omits the body). Don't rely on method-based
rejection; it doesn't exist.

### Response when things are wrong

- Backend unreachable (proxy paths with a dead backend): `502 Bad Gateway`,
  `text/plain`, body `Failed to execute proxy request`. Health paths are
  local and unaffected in `start` mode.
- No known path returns a non-200 today. If you observe a non-200 health
  response, the daemon process itself is broken — treat it as down.

## Proxy vs standalone socket composition

The daemon has two serving shapes, and the health/metrics composition
differs between them — this is the main operational gotcha.

### Standalone (no backend)

`openapi-daemon start` without `--backend-url` serves the canned handler on
the socket: `/` plus the three health routes. Nothing else exists.

### `start` + `--backend-url`

The socket handler is composed: health paths (`/health`, `/health/`) are
handled **locally by the daemon**, everything else is reverse-proxied to the
backend.

```bash
openapi-daemon start --backend-url http://127.0.0.1:8080
curl --unix-socket /tmp/openapi-cli.sock http://localhost/health
# -> local canned 200, even if the backend is down
curl --unix-socket /tmp/openapi-cli.sock http://localhost/some/api
# -> proxied to the backend; 502 if it's down
```

### `proxy` subcommand

**The health endpoints are proxied to the backend too.** The standalone
`proxy` subcommand does not compose the local canned health handler —
`/health`, `/health/live`, `/health/ready` are forwarded to the backend like
any other path (verified: a plain file server as backend returned its own
404 for `/health`).

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
| systemd `ExecCondition` / watchdog script | `/health/live` | restart-on-fail semantics |
| Load balancer pool membership | `/health/ready` via an authenticated local relay, or the backend's own health check | ready is the conventional LB endpoint; note it is not yet backend-aware |
| Kubernetes (if socket is exposed via a sidecar) | live = `/health/live`, ready = `/health/ready` | convention mapping |

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

LB health check then targets `http://<host>:8081/health/ready`, expecting
HTTP 200. Mark the node out of rotation on non-200/connection failure.

## Prometheus scrape configuration

### The metrics listener

- The daemon **socket** does not expose Prometheus format — its `/metrics`
  path falls through to the root JSON info handler.
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
| `/health` 200 but API calls 502 | backend down (proxy paths are remote) | restart backend; health is daemon-local in `start` mode |
| `/health` returns backend's HTML/404 | you're on `proxy` mode — health is proxied there | switch to `start --backend-url` for daemon-local health |
| `curl --unix-socket` fails to connect | socket path wrong, or wrong user (socket is 0600) | check `openapi-daemon status`; run curl as daemon user |
| `/metrics` returns JSON not Prometheus text | you scraped the Unix socket; that path falls through to `/` | scrape `http://127.0.0.1:9110/metrics` instead of the socket |
| `curl 127.0.0.1:9110/metrics` → connection refused | the listener never bound (port already taken) or the daemon is not running in `start`/`proxy` mode | look for `[WARN] metrics listener not started` in the startup log; free the port or pass a different `--metrics-addr` |
| daemon exits with `is not a loopback address` | `--metrics-addr` named a non-loopback host | use `127.0.0.1:<port>`, `[::1]:<port>`, or `localhost:<port>` |
| `404` when probing `/health` on the metrics port | the metrics listener serves `/metrics` only | probe `/health` on the Unix socket |
| LB marks node down though daemon runs | LB can't reach a Unix socket | add a TCP relay or probe the backend |
| systemd unit flaps at start | daemonize delay before socket exists | poll socket in `ExecStartPost` (see unit above) |

## Verification log

Reproduce any claim in this document:

```bash
go build -o /tmp/muster-daemon ./cmd/daemon
/tmp/muster-daemon start --foreground --socket-path /tmp/doc.sock --metrics-addr 127.0.0.1:19110 &
curl --unix-socket /tmp/doc.sock http://localhost/health          # {"status":"healthy"}
curl --unix-socket /tmp/doc.sock http://localhost/health/live     # {"status":"alive",...}
curl --unix-socket /tmp/doc.sock http://localhost/health/ready    # {"status":"ready",...}
curl --unix-socket /tmp/doc.sock http://localhost/metrics         # JSON root body, not Prometheus
curl -fsS http://127.0.0.1:19110/metrics | grep -E '^muster_'    # the 10 families/seed series above
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:19110/health   # 404
```