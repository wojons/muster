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
| `/metrics` | `monitoring.MetricsServer` (TCP) | GET | `200 OK` | Prometheus text exposition |

Important asymmetry: **`/metrics` is not part of the daemon's socket handler
at all.** The only Prometheus HTTP handler in the tree
(`internal/monitoring.MetricsServer`) is currently not constructed by any
binary — it exists as a library (`NewMetricsServer(addr)`), so nothing
listens on a TCP metrics port unless an operator embeds it. Do not point a
scrape job at the daemon socket expecting Prometheus format; the socket's
`/metrics` path answers with the canned root JSON.

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

### The asymmetry, restated

- The daemon **socket** does not expose Prometheus format — its `/metrics`
  path falls through to the root JSON info handler.
- `internal/monitoring.MetricsServer` (`NewMetricsServer(addr)`) is the
  component that serves `GET /metrics` in Prometheus text format (via
  `promhttp.Handler()`) on a **TCP** address. It also serves `/health`,
  `/health/live`, `/health/ready`, and `/health/startup` (all unconditional
  JSON 200s with a `timestamp` field; `/health` body is
  `{"status":"healthy","timestamp":"..."}`).
- As of this writing **no shipped binary constructs `MetricsServer`**, so
  there is no default TCP metrics port. Embedding it (a one-liner in the
  daemon main) is the intended path; until your build does so, the scrape
  config below has nothing to scrape.

### Metric families defined by the daemon libraries

Registered globally via `promauto` in `internal/monitoring` (namespaces:
`openapi_cli`, `openapi_mcp`, `openapi_daemon`):

| Metric | Type | Labels |
|---|---|---|
| `openapi_cli_command_executions_total` | counter | `command`, `status`, `tenant_id` |
| `openapi_cli_command_duration_seconds` | histogram | `command`, `status`, `tenant_id` |
| `openapi_cli_command_errors_total` | counter | `command`, `error_type`, `tenant_id` |
| `openapi_mcp_active_connections` | gauge | — |
| `openapi_mcp_requests_total` | counter | `method`, `tenant_id` |
| `openapi_mcp_request_duration_seconds` | histogram | `method`, `tenant_id` |
| `openapi_mcp_tool_executions_total` | counter | `tool_name`, `status`, `tenant_id` |
| `openapi_daemon_active_socket_connections` | gauge | — |
| `openapi_daemon_proxy_requests_total` | counter | `endpoint`, `method`, `tenant_id` |
| `openapi_daemon_cache_hits_total` | counter | — |
| `openapi_daemon_cache_misses_total` | counter | — |

Note: because these use `promauto.New*` (the default registry), a
`MetricsServer` scrape returns them **plus** the Go runtime and process
collectors (`go_goroutines`, `process_resident_memory_bytes`, etc.).
Per-request recording goes through `MetricRecorder` methods
(`RecordCommandExecution`, `RecordProxyRequest`, ...); counters stay at
their zero values until those paths execute.

### prometheus.yml

Once your build serves the metrics server (example assumes `:9090` was
chosen when constructing `NewMetricsServer(":9090")`):

```yaml
scrape_configs:
  - job_name: "muster-daemon"
    scrape_interval: 15s
    scrape_timeout: 10s       # server WriteTimeout is 10s; do not exceed
    metrics_path: /metrics
    static_configs:
      - targets: ["127.0.0.1:9090"]
        labels:
          service: "muster-daemon"
```

Notes for operators:

- The metrics server has `ReadTimeout: 5s` / `WriteTimeout: 10s`; keep
  `scrape_timeout` at or below 10s.
- The `/health*` routes on the metrics TCP server can also serve as
  network-reachable probes if you front the metrics port with an LB —
  bodies match the socket ones plus a `timestamp` (and `/health` gains a
  timestamp there).
- If you scrape through a socket relay instead, the target becomes the
  relay's TCP address; everything else is identical.

### Verifying a scrape

```bash
# Confirm exposition format and families:
curl -fsS http://127.0.0.1:9090/metrics | grep -E '^openapi_(cli|mcp|daemon)_'

# Confirm Prometheus accepts the target (in the Prometheus web UI or):
promtool check config /etc/prometheus/prometheus.yml
```

## Troubleshooting quick reference

| Symptom | Likely cause | Fix |
|---|---|---|
| `/health` 200 but API calls 502 | backend down (proxy paths are remote) | restart backend; health is daemon-local in `start` mode |
| `/health` returns backend's HTML/404 | you're on `proxy` mode — health is proxied there | switch to `start --backend-url` for daemon-local health |
| `curl --unix-socket` fails to connect | socket path wrong, or wrong user (socket is 0600) | check `openapi-daemon status`; run curl as daemon user |
| `/metrics` returns JSON not Prometheus text | that's the socket fall-through to `/` | scrape the `MetricsServer` TCP port, not the socket |
| No process listening on metrics port | no binary constructs `MetricsServer` yet | embed it in your build; until then there is no TCP metrics surface |
| LB marks node down though daemon runs | LB can't reach a Unix socket | add a TCP relay or probe the backend |
| systemd unit flaps at start | daemonize delay before socket exists | poll socket in `ExecStartPost` (see unit above) |

## Verification log

Reproduce any claim in this document:

```bash
go build -o /tmp/muster-daemon ./cmd/daemon
/tmp/muster-daemon start --foreground --socket-path /tmp/doc.sock &
curl --unix-socket /tmp/doc.sock http://localhost/health          # {"status":"healthy"}
curl --unix-socket /tmp/doc.sock http://localhost/health/live     # {"status":"alive",...}
curl --unix-socket /tmp/doc.sock http://localhost/health/ready    # {"status":"ready",...}
curl --unix-socket /tmp/doc.sock http://localhost/metrics         # JSON root body, not Prometheus
```