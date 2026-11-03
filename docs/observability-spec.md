# Muster — Observability Spec

Status: audit + design proposal (no service code changed).

This document states what Muster can prove about itself today, and the smallest
set of changes that would let the fleet's Prometheus/Loki stack answer the
questions an operator actually asks at 3am.

---

## 1. What Muster is, and what "healthy" means

Muster is an OpenAPI-to-MCP harness. It ships three binaries:

| Binary | Role | Process shape |
|---|---|---|
| `cmd/daemon` | Long-lived proxy that injects the upstream credential and serves over a **Unix socket** | daemonized, one per user-host, listens on `/tmp/openapi-cli.sock` (`cmd/daemon/main.go:25`) |
| `cmd/mcp-server` | MCP server that exposes OpenAPI operations as MCP tools | **stdio only** — no listener at all (`cmd/mcp-server/main.go:47-55`; no `Listen`/`http.Server` outside `internal/monitoring`) |
| `cmd/openapi-cli` | One-shot CLI: parse a spec, run a command, exit | short-lived, exit code is the report |

### Healthy means, per binary

- **daemon** — the socket exists *and* answers; proxied upstream calls succeed at a
  normal rate; the backend credential has not expired; the audit sink is not
  silently shedding events.
- **mcp-server** — the host answered `initialize`, tool calls are succeeding, and
  the upstream API is not rate-limiting it.
- **openapi-cli** — the invocation exited 0, and, if it failed, its failure is in
  the audit trail.

The last clause is the important one. In a stdio and one-shot design, **the exit
code and the audit record are the entire observability surface**. Nothing else
survives the process.

---

## 2. Current state — what is proven, with citations

### 2.1 A Prometheus client library is present but wired to nothing

`internal/monitoring/metrics.go` defines a real, reasonably designed metric set
(10 metric families, see §2.2) and a `MetricsServer` that serves them at
`/metrics` via `promhttp.Handler()` (`internal/monitoring/metrics.go:246`).

**It is dead code.** No package imports it:

```
$ grep -rn "internal/monitoring" --include=*.go . | grep -v "internal/monitoring/"
(no output)
```

`NewMetricsServer` and `NewMetricRecorder` have zero non-test callers:

```
$ grep -rn "NewMetricsServer\|NewMetricRecorder" --include=*.go . | grep -v "internal/monitoring/"
(no output)
```

Consequences, and they compound:

1. No binary calls `NewMetricsServer`, so `/metrics` is never actually served.
2. Even if it were, the metric vectors are package-level `promauto` vars
   (`metrics.go:24-119`) registered on the **default** registry, while
   `promhttp.Handler()` (`metrics.go:246`) serves that same default registry — so
   the plumbing would line up. The blocker is upstream of that: no recorder is
   ever invoked, so the vectors are always zero / absent.
3. `MetricRecorder.registry` is allocated at `metrics.go:132` and **never used** —
   the recorder writes to the global vars, not to `mr.registry`. This is a latent
   bug that would bite anyone who "just wires it up": they would expect
   isolation and get a shared global registry.

### 2.2 The existing (dead) metric catalogue

Names below are the actual rendered Prometheus names.

| Name | Type | Labels |
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
| `openapi_daemon_cache_hits_total` / `_cache_misses_total` | counter | — |

Problems with this catalogue as-is:

- **Wrong namespace.** Everything is `openapi_*`. The product is `muster` and the
  fleet convention is `<product>_<subsystem>_<thing>_<unit>`. A collector cannot
  select `muster_*` today at all.
- **`tenant_id` as a label is questionable for a CLI binary.** Muster's tenant is a
  locally configured *active tenant* persisted to `tenants.json`
  (`internal/builtin/extended.go:313-326`, `activeTenantID` at `:440`) — i.e. a
  handful of values per install, selected by a local `muster tenant use` command.
  It is **not** a per-request user id. So it is low-cardinality *in principle*
  (bounded by the number of tenants configured on one host). The risk is
  aggregation: it multiplies every series by tenant count and, in a fleet where
  most installs have exactly one tenant, adds a dimension that is constant per
  process. **Recommendation:** keep it, but only on `muster_daemon_*` (the
  long-lived, potentially multi-tenant process), and drop it from the
  `openapi_cli`/`mcp` families where the process has one tenant for its whole
  lifetime. If any future code paths a *remote/request-supplied* tenant into a
  label, that must be capped — see §6.
- **`DaemonCacheHits/CacheMisses` have no cache behind them** in the daemon proxy
  path I could find; they would report zeros forever if wired naively.
- `DaemonProxyRequests` uses an `endpoint` label (`metrics.go:102`). Endpoints come
  from the OpenAPI spec, so cardinality is bounded by spec size, not by traffic —
  acceptable, but it must be a *normalized* path template, never a filled path.

### 2.3 A second, unmentioned HTTP surface: daemon health endpoints

The daemon's proxy handler serves `/health`, `/health/live`, `/health/ready`,
`/health/startup` (`pkg/daemon/proxy.go:502-524`), and these are deliberately
excluded from token auth (`isHealthEndpoint`, `pkg/daemon/proxy.go:385-395`).

They are **hard-coded 200s with no dependency checks**: every handler writes
`StatusOK` and a literal body (`proxy.go:505`, `:511`, `:517`, `:523`). So
`/health/ready` returns `ready` even when the backend credential is missing or the
audit sink is wedged. As a liveness signal over the socket it is honest; as a
readiness signal it is fiction.

This is the **only** existing HTTP surface in the product, and it is a **Unix
socket**, not a TCP port. That single fact drives the entire collection design in
§4.

### 2.4 What is actually durable today: the audit JSONL sink

`internal/audit` provides a tamper-evident (hash-chained) JSONL audit log:
`FileStorage.Write` opens the sink `O_APPEND|O_CREATE` at mode 0600
(`internal/audit/audit_types.go:166`), records carry `id`, `timestamp`,
`event_type`, `user`, `level`, `category`, `action`, `actor{}`, `resource{}`,
`result{success,code}`, `context.metadata{}` (`audit_types.go:26-77`), and each row
links to its predecessor by `previous_hash`/`hash` (`audit_types.go:68-72`).

This is a **real durable signal** and it is the right foundation. But:

- **It is opt-in and off by default.** `DefaultConfig` sets `Enabled` only when
  `MUSTER_AUDIT_ENABLED` is `1`/`true` (`internal/audit/hooks.go:208-218`), and
  `cmd/mcp-server/main.go:53` gates it behind `--audit`. A default install emits
  nothing.
- **It records almost nothing about work.** The only emission sites in the
  binaries are process lifecycle:
  - `cmd/openapi-cli/main.go:138` — logs a command **only on failure**.
  - `cmd/mcp-server/main.go:249,252,255` — `mcp-server-start` ok/fail and
    `mcp-server-run` fail.

  `LogToolCall` and `LogAuth` exist (`internal/audit/hooks.go:122`, `:155`) but
  have **zero non-test callers**:
  ```
  $ grep -rn "LogToolCall\|LogAuth" --include=*.go . | grep -v _test
  ./internal/audit/hooks.go:122:func (h *Hook) LogToolCall(...)
  ./internal/audit/hooks.go:155:func (h *Hook) LogAuth(...)
  ```
  So the single most operationally important event — *did an MCP tool call
  succeed, for which tool, how long did it take* — is **never recorded**. A
  successful MCP tool call leaves no trace anywhere.
- **The sink sheds events silently.** `AuditLogger.Log` is a non-blocking send on
  a buffered channel (`BufferSize: 100`, `hooks.go:214`; channel at
  `audit_types.go:260`) and on a full buffer takes the `default` branch:

  ```go
  // internal/audit/audit_types.go:295-302
  func (l *AuditLogger) Log(ctx context.Context, event *AuditEvent) error {
      select {
      case l.events <- event:
          return nil
      default:
          return nil   // <-- event dropped, caller told "success"
      }
  }
  ```

  Under load — exactly when the signal matters — audit rows vanish and the hash
  chain stays valid, so *nothing downstream can tell they were lost*. The
  consumer also discards write errors (`_ = l.storage.Write(event)`,
  `audit_types.go:276` and `:284`). This is the most serious observability defect in
  the product: **the sink that is supposed to be the record of truth can lie by
  omission, and no counter exists to say it did.**

### 2.5 Logging

Structured logging exists (`pkg/log`, zerolog; `internal/monitoring/metrics.go`
takes a `*zerolog.Logger`). `docs/LOG-AGGREGATION.md` documents shipping JSON logs
to Loki or ELK, and `docs/LOGGING-STANDARDS.md` sets the format. Logs therefore
reach Loki *if* configured — but logs cannot answer "what is the tool-call error
rate" without full-text gymnastics, and a `mcp-server` under stdio has its stderr
captured by whoever spawned it.

### 2.6 Existing docs

`docs/METRICS-STANDARDIZATION.md` prescribes a `muster_` namespace with RED/USE
metrics (`Namespace: "muster"`, lines 36-60) — which **contradicts the shipped
`internal/monitoring` code**, whose namespace is `openapi_cli`/`openapi_mcp`/
`openapi_daemon` (`metrics.go:17-21`). The doc describes a design nobody
implemented. This spec supersedes both for measurement purposes.

---

## 3. Metric catalogue — proposed

Naming: `muster_<subsystem>_<thing>_<unit>`. Subsystems: `daemon`, `mcp`, `cli`,
`audit`, `upstream`. All durations in seconds, all sizes in bytes, all times in
Unix seconds.

### 3.1 The minimal viable set (answers "is it up, is it working, is it failing")

These five answer the three questions that matter and nothing more.

| Metric | Type | Unit | Labels | Question it answers |
|---|---|---|---|---|
| `muster_build_info` | gauge (always 1) | – | `version`, `commit` | What exactly is running on this host? |
| `muster_daemon_up` | gauge | bool | `tenant_id` | Is the daemon answering right now? |
| `muster_daemon_proxy_requests_total` | counter | requests | `endpoint`, `method`, `status_class`, `tenant_id` | Is the daemon doing work, and how much of it is failing? |
| `muster_daemon_proxy_duration_seconds` | histogram | seconds | `endpoint`, `method` | How slow are proxied calls? |
| `muster_audit_events_written_total` | counter | events | `event_type`, `category` | Is the record of truth actually being written? |

`muster_daemon_up` is the one a blackbox probe would substitute for, but emitting
it ourselves matters because the socket is not network-reachable (§4.2).

### 3.2 Depth — the MCP binary (where the work really happens)

| Metric | Type | Unit | Labels | Question it answers |
|---|---|---|---|---|
| `muster_mcp_tool_executions_total` | counter | calls | `tool`, `status`, `error_class` | Which tool is failing, and how often? |
| `muster_mcp_tool_duration_seconds` | histogram | seconds | `tool`, `status` | Is a tool hanging? (p99 tail) |
| `muster_mcp_active_sessions` | gauge | sessions | – | How many clients are attached? |
| `muster_mcp_requests_total` | counter | requests | `method`, `status_class` | MCP protocol-level traffic + errors |
| `muster_mcp_initialize_failures_total` | counter | failures | `reason` (**exact-match enum only**) | Are clients failing to bootstrap? |

`tool` is bounded by the OpenAPI spec's operation count. **`initialize_failures`
must never carry the raw error string as a label** — bucket it into a closed enum
(`auth`, `spec_unparsable`, `upstream_unreachable`, `protocol`).

### 3.3 Depth — upstream credit

| Metric | Type | Unit | Labels | Question it answers |
|---|---|---|---|---|
| `muster_upstream_requests_total` | counter | requests | `status_class`, `tenant_id` | Is the third-party API healthy or rate-limiting us? |
| `muster_upstream_rate_limited_total` | counter | events | `tenant_id` | Are we burning quota on 429s? |
| `muster_upstream_request_duration_seconds` | histogram | seconds | `status_class` | Is the slowness ours or theirs? |

### 3.4 Depth — audit health (the sink that must not lie)

| Metric | Type | Unit | Labels | Question it answers |
|---|---|---|---|---|
| `muster_audit_events_dropped_total` | counter | events | `reason` (`buffer_full`, `write_error`) | **Is the audit trail silently lossy?** |
| `muster_audit_buffer_utilization_ratio` | gauge | ratio | – | How close is the sink to shedding? |
| `muster_audit_sink_bytes` | gauge | bytes | – | Is the audit file growing as expected (or has rotation stalled)? |
| `muster_audit_chain_write_errors_total` | counter | errors | `reason` | Are writes failing against the file system? |

`events_dropped_total` is the metric that does not exist anywhere today and is
worth more than the other fourteen combined, because it is the only way to learn
that §2.4's silent drop is happening.

### 3.5 Short-lived CLI lane

`openapi-cli` must **not** be scraped. A process that exits has no scrape window.
Its right answer is a **completion record**, not an endpoint:

| Metric | Type | Unit | Labels | Question it answers |
|---|---|---|---|---|
| `muster_cli_invocations_total` | counter | invocations | `command`, `status` | How is the CLI being used, and how often does it fail? |
| `muster_cli_duration_seconds` | histogram | seconds | `command` | How slow is a one-shot invocation? |

Emitted by writing a Prometheus textfile (`*.prom`) atomically to a textfile
collector directory, or by a `POST` to the pushgateway, at process exit. Label
`command` from the closed set of CLI subcommands; never the argv tail.

---

## 4. Exposure mechanism

### 4.1 The core problem: there is no TCP surface

Muster's only HTTP listener binds a **Unix socket**
(`pkg/daemon/proxy.go:414`: `net.Listen("unix", p.socketPath)`), at
`/tmp/openapi-cli.sock`. Prometheus cannot scrape a Unix socket. `mcp-server` has
no listener at all. So "add `/metrics` to the existing server" — the normal
answer — does not work here unless the socket is also reachable over TCP.

### 4.2 Design

**Daemon:** add `promhttp.Handler()` to the daemon's existing handler mux
(`createDefaultHandler`, `pkg/daemon/proxy.go:493-494`) at `/metrics`, and **additionally bind a loopback TCP
listener** on a dedicated metrics port (default `127.0.0.1:9110`, override via
`--metrics-addr`), serving only `/metrics` and `/health/*`. Bind loopback only;
the fleet scraper is on the same host or arrives over the tailnet. Exclusion from
token auth follows the existing `isHealthEndpoint` pattern
(`pkg/daemon/proxy.go:385-395`) — `/metrics` must be added to that allowlist for
the socket path, and the TCP listener should serve *only* `/metrics` so a
path-traversal cannot reach the proxy through it.

**MCP server:** it has no server to attach to. Two acceptable options, in order:

1. **Preferred — textfile collector.** Write `muster_mcp_*.prom` at graceful exit
   (and periodically while running) to a node_exporter `textfile` directory. The
   fleet already runs node_exporter per host; this is zero new scrape targets and
   is the textbook answer for a process with no HTTP surface.
2. If a long-lived MCP session must be scrapeable live, add an optional
   `--metrics-addr` loopback listener alongside the stdio loop. This is a larger
   change and should be justified by a real need to observe an *in-flight*
   session.

**CLI:** textfile/pushgateway completion record, as §3.5.

**Do not** use the default `prometheus` registry in production code. The dead
code at `metrics.go:132` shows the trap: a `*prometheus.Registry` field that the
recorder ignores. Create one explicit registry per process, register explicitly,
and serve *that* registry. It makes tests hermetic and prevents double-registration
panics.

---

## 5. Collection path

- **Scrape job** (to be added to `/root/fleet-monitoring/prometheus/prometheus.yml`
  by whoever owns the monitoring stack — *not by this audit*):

  ```yaml
  - job_name: muster
    scrape_interval: 30s
    static_configs:
      - targets: ['127.0.0.1:9110']
        labels:
          host: <hostname>
          product: muster
          service: daemon
  ```

- **textfile collector** for `mcp-server`/`cli`: point node_exporter at a
  directory (e.g. `/var/lib/node_exporter/textfile`) and have the binaries write
  `muster_mcp.prom` / `muster_cli.prom` there atomically (write temp + `rename`).
  Labels `product="muster"` are emitted *inside* the textfile, since the textfile
  collector cannot set them.
- **Push alternative:** the fleet's agents normally `remote_write`. For Muster that
  only helps after a push bridge exists; emitting to a scrape target is simpler and
  keeps the daemon free of a write path to Prometheus.
- **Existing fleet agent behaviour:** not verifiable from this repo — see §7.
- **Loki:** keep logs as-is (`docs/LOG-AGGREGATION.md`); add `trace_id`/`session_id`
  correlation in log lines — as a *log field*, never a metric label.

---

## 6. Deliberately NOT instrumented

- **Per-request user, session, or task ids as labels.** `tenant_id` is acceptable
  only because it is a locally configured, near-constant value (§2.2). Anything
  arriving from a request must not become a label.
- **OpenAPI path parameters.** `endpoint` must be the spec template
  (`/pets/{petId}`), never the filled path. A filled path is unbounded and is the
  classic Prometheus cardinality bomb.
- **Argv tails, prompt text, tool arguments, response bodies, tokens.** These are
  in audit events where they belong; a metric label is an infinite-cardinality
  liability and a leak surface.
- **Timestamp/version-in-name metrics.** No `muster_last_run_20261103`. Timestamps
  belong in the value of a gauge, not the name.
- **Per-tool histograms for very high tool cardinality** — if a spec exposes
  hundreds of operations, prefer a single `muster_mcp_tool_duration_seconds` with
  a bounded `tool` label set and overflow into `tool="other"`.
- **The `openapi_daemon_cache_hits_total`/`cache_misses_total` pair** from dead
  code (§2.2), until a cache actually exists to count.

---

## 7. Verification — how a reviewer proves each metric is live

"Done" means every command below returns data **and** the negative control fires.

**1. The endpoint exists and is ours**

```bash
curl -sS http://127.0.0.1:9110/metrics | grep -c '^muster_'
# expect > 0
```

**2. The minimal set is present** (PromQL)

```promql
# build info — exactly one series, version visible
count(muster_build_info) == 1
# the daemon is up
max(muster_daemon_up) == 1
# it is doing work: request rate over 5m
sum(rate(muster_daemon_proxy_requests_total[5m]))
# and failing in a measurable proportion
sum(rate(muster_daemon_proxy_requests_total{status_class=~"4..|5.."}[5m]))
  / sum(rate(muster_daemon_proxy_requests_total[5m]))
# the audit sink is writing
sum(rate(muster_audit_events_written_total[5m])) > 0
```

**3. Negative controls — the probes that flip.** A metric that cannot go bad is
not evidence. Each row must be provable:

| To prove | Do this | Expect |
|---|---|---|
| `muster_daemon_up` can report down | `kill -STOP` the daemon (or stop it) | `max(muster_daemon_up)` → `0` or series goes stale; Alertmanager `MusterDaemonDown` fires |
| `proxy_requests_total{status_class}` is real | issue a proxied call that returns 500 upstream | `5xx` rate rises on the next scrape |
| `audit_events_dropped_total` is wired | set `BufferSize: 1`, flood events | counter > 0, **and** an audit row is provably missing vs. calls made |
| `muster_mcp_tool_executions_total` is real | call one MCP tool successfully then once with bad params | `status="success"` +1, `status="error"` +1 |
| textfile collection works | write `muster_cli.prom` by hand with a sentinel value | sentinel appears in Prometheus; delete the file → series goes stale within 2 intervals |

**4. The bug this spec is motivated by is fixed**

```bash
# log a known number of events under saturation, then compare
grep -c '"event_type"' /path/to/audit.log
# must equal the number of operations performed
# and: sum(muster_audit_events_dropped_total) must account for the difference
```

---

## 8. Audit verdict

**Before:** `MUSTER IS BLIND.` No scrapeable metric exists. A Prometheus client
library and a `/metrics` handler are committed but unreferenced
(`internal/monitoring/metrics.go` — zero importers). The only HTTP surface is a
Unix socket serving four hard-coded-200 health endpoints
(`pkg/daemon/proxy.go:502-524`). The only durable record is an opt-in, off-by-default
hash-chained JSONL audit log that (a) records process lifecycle but **never a
successful MCP tool call**, and (b) **silently drops events under load while
reporting success** (`internal/audit/audit_types.go:295-302`).

**After this spec is implemented:** five metrics answer "up / working / failing /
is the record of truth real"; a loopback `/metrics` on the daemon, textfile
collection for the stdio and one-shot binaries; one scrape job; a drop counter
that makes the silent loss loud.

**Effort:** the minimal set is small — the handler already exists and only needs
to be reached and populated. The audit drop counter is a few lines and is the
highest-value single change in the document.
