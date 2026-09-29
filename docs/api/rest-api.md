# Daemon HTTP API Reference

Reference for the HTTP surface served by `openapi-daemon` (source: `cmd/daemon/`, `pkg/daemon/`).

> **Note on this document's history.** The previous revision of this file documented a ~20-endpoint
> REST API (a TCP base URL, spec/command/config/cache/auth/webhook endpoints, API-key auth, CORS and
> rate-limit configuration blocks). None of it existed. That revision has been replaced by the surface
> actually registered in `pkg/daemon/` and `cmd/daemon/`; the removed rows are listed under
> [Not implemented](#not-implemented).

## Overview

`openapi-daemon` is a single-user daemon. It is **not** a REST service, and it is not a TCP listener:
it binds an **HTTP/1.1 server to a Unix domain socket** (`net.Listen("unix", ...)`,
`pkg/daemon/proxy.go:397`) and serves one of three handler sets:

| Mode | Invocation | What the socket serves |
|------|-----------|------------------------|
| Status only | `openapi-daemon start` | A canned handler: root info + the health routes (`cmd/daemon/main.go:309`) |
| Proxy with local health | `openapi-daemon start --backend-url <url>` | Local `/health*` routes, everything else reverse-proxied (`cmd/daemon/main.go:342`) |
| Pure proxy | `openapi-daemon proxy --backend-url <url>` | Every path, `/health` included, is forwarded to the backend (`cmd/daemon/main.go:354`) |

`openapi-daemon proxy` without `--backend-url` exits 1 with
`Error: handler is required; use --backend-url to specify the backend URL` (`cmd/daemon/main.go:365`).

There is no `--port`/`--host` flag and no TCP listener anywhere in `cmd/` or `pkg/daemon/` — the only
listener call in the daemon package is the Unix-socket one cited above.

## Transport

| Property | Value | Source |
|----------|-------|--------|
| Transport | Unix domain socket (HTTP/1.1 over it) | `pkg/daemon/proxy.go:397` |
| Default socket path | `/tmp/openapi-cli.sock` | `cmd/daemon/main.go:24` |
| Socket file mode | `0600` (owner only) | `pkg/daemon/proxy.go:403` |
| Default PID file | `<socket-path>.pid` | `cmd/daemon/main.go:110` |
| Server read / write timeout | `10s` / `10s` | `pkg/daemon/proxy.go:412` |
| Backend client timeout (proxy mode) | `30s` | `pkg/daemon/proxy.go:525` |

Requests are made with `curl --unix-socket`:

```bash
curl --unix-socket /tmp/openapi-cli.sock http://localhost/health
```

The `http://localhost/...` here is a placeholder host for curl's benefit — nothing is listening on a
TCP port; the socket path is what selects the server.

## Authentication

The daemon package contains a token middleware and a token-file path, but the shipped CLI does **not**
install the middleware. Both facts, with sources:

| Item | Detail | Source |
|------|--------|--------|
| Header shape (if middleware installed) | `Authorization: Bearer <token>` (prefix literal at the cited line) | `pkg/daemon/token_manager.go:44` |
| Token file path | `<socket-path>.token` (derived by the library; never written or printed by the CLI) | `pkg/daemon/proxy.go:71` |
| Health routes exempt from auth | `/health`, `/health/live`, `/health/ready`, `/health/startup` | `pkg/daemon/proxy.go:368`, `pkg/daemon/proxy.go:373` |
| Middleware | `withTokenAuth` | `pkg/daemon/proxy.go:328` |
| Only caller of the middleware | `NewAuthenticatedProxyServer` — no non-test caller in the repo | `pkg/daemon/proxy.go:310` |
| Token registration | `TokenManager.AddToken` is called only from tests | `pkg/daemon/token_manager.go:22` |

**Measured:** the running daemon answered a non-health path with no `Authorization` header present at
all (see the `/not-a-route` probe below, and the `/hello.txt` probe in proxy mode). It also never
writes the `.token` file, and it no longer claims to: the `Token file:` line was removed from the
`start`, `proxy` and `status` banners (DF-047) because it sent operators hunting for a credential
file that never exists. After `start`, only the socket and PID file exist on disk:

```text
$ ls -l /tmp/muster-daemon.sock*
srw------- 1 kara kara 0 Sep 27 00:57 /tmp/muster-daemon.sock
-rw-r--r-- 1 kara kara 8 Sep 27 00:57 /tmp/muster-daemon.sock.pid
```

So, as shipped: the socket is protected by its `0600` permissions and by the fact that it is a
local-only Unix socket. The bearer token and the `.token` file are library capabilities, not a
requirement of the CLI daemon. If the middleware is ever wired in, its refusal payloads are
`{"error":"Unauthorized","message":"Missing authorization header"}` and
`{"error":"Unauthorized","message":"Invalid or expired token"}` with status 401
(`pkg/daemon/proxy.go:345`, `pkg/daemon/proxy.go:357`).

## Endpoint reference

Method is listed as "any" where the handler is registered with `mux.HandleFunc(path, ...)` and does not
check `r.Method` — a `POST` to such a route behaves exactly like a `GET` (verified for `/health`).

### Status-only mode (`openapi-daemon start`)

| Path | Method | Registered at | Response body |
|------|--------|---------------|---------------|
| `/` | any | `cmd/daemon/main.go:311` (also `pkg/daemon/proxy.go:479`) | `{"status":"ok","message":"Muster daemon is running"}` |
| `/health` | any | `cmd/daemon/main.go:316` (also `pkg/daemon/proxy.go:485`) | `{"status":"healthy"}` |
| `/health/live` | any | `cmd/daemon/main.go:321` (also `pkg/daemon/proxy.go:491`) | `{"status":"alive","timestamp":"<RFC3339 UTC>"}` |
| `/health/ready` | any | `cmd/daemon/main.go:326` (also `pkg/daemon/proxy.go:497`) | `{"status":"ready","timestamp":"<RFC3339 UTC>"}` |
| `/health/startup` | any | `pkg/daemon/proxy.go:503` (library handler only — see below) | `{"status":"started","timestamp":"<RFC3339 UTC>"}` |
| every other path | any | the `/` entry above; `/` is a `net/http` subtree match | `{"status":"ok","message":"Muster daemon is running"}` |

Two behaviours worth knowing:

* **`/health/startup` is not registered by the CLI handler.** `cmd/daemon/main.go:309`
  (`createDaemonHandler`) registers `/`, `/health`, `/health/live` and `/health/ready` — no
  `/health/startup`. The route exists in the library's default handler
  (`pkg/daemon/proxy.go:503`), which the CLI does not use for `start`. The request therefore falls to
  the `/` catch-all and returns the root payload, as the transcript below shows.
* **The `/` catch-all means a missing route is not a 404.** Any unknown path returns HTTP 200 with the
  root payload. Verified with `/not-a-route`.

### Proxy modes (`--backend-url`)

| Path | Mode | Behaviour |
|------|------|-----------|
| `/health`, `/health/live`, `/health/ready` | `start --backend-url` | Served locally by the daemon (`cmd/daemon/main.go:342`, `:348`) so health checks keep working while the backend is down |
| `/health/startup`, `/health/` | `start --backend-url` | Also handled locally (the `/health/` subtree at `cmd/daemon/main.go:349` captures them) but the CLI handler registers no `/health/startup`, so both answer with the root payload |
| every other path | `start --backend-url` | Forwarded to the backend (`pkg/daemon/proxy.go:569`) |
| every path, `/health` included | `proxy --backend-url` | Forwarded to the backend — `proxy` composes no local routes (`cmd/daemon/main.go:354`) |

Measured for the second row (backend log shows neither request reached the backend):

```text
$ curl -sS --unix-socket /tmp/muster-daemon-p5.sock http://localhost/health/startup
{"status":"ok","message":"Muster daemon is running"}

$ curl -sS --unix-socket /tmp/muster-daemon-p5.sock http://localhost/health/
{"status":"ok","message":"Muster daemon is running"}
```

Proxy behaviour, from `pkg/daemon/proxy.go` (`ProxyHandler.ServeHTTP` at `:569`): the path is
sanitised (`:670`, rejecting traversal and control characters), the target keeps the backend's scheme
and host (`:608`), the query string is forwarded (`:592`), hop-by-hop headers are dropped (`:714`) and
`X-Forwarded-For` is added (`:637`). A backend that cannot be reached yields 502
(`:645`); an empty or rejected path yields 400 (`:580`). The transcript below exercises the 200 and
404 passthrough cases; the 502/400 paths are source-derived, not exercised here.

## Verified transcript

Built in this worktree with `CGO_ENABLED=1 go build -o bin/openapi-daemon ./cmd/daemon`.
Every line below was executed; the command is printed above its output. Re-run on 2026-09-29 after
DF-047 removed the `Token file:` banner line — every other line reproduced verbatim.

**Mode A — `start` (no backend):**

```text
$ ./bin/openapi-daemon start --socket-path /tmp/muster-daemon.sock
Daemon started successfully
PID file: /tmp/muster-daemon.sock.pid
Socket: /tmp/muster-daemon.sock

$ curl -sS --unix-socket /tmp/muster-daemon.sock http://localhost/health
{"status":"healthy"}

$ curl -sS --unix-socket /tmp/muster-daemon.sock http://localhost/
{"status":"ok","message":"Muster daemon is running"}

$ curl -sS --unix-socket /tmp/muster-daemon.sock http://localhost/health/live
{"status":"alive","timestamp":"2026-09-27T05:57:38Z"}

$ curl -sS --unix-socket /tmp/muster-daemon.sock http://localhost/health/ready
{"status":"ready","timestamp":"2026-09-27T05:57:38Z"}

$ curl -sS --unix-socket /tmp/muster-daemon.sock http://localhost/health/startup
{"status":"ok","message":"Muster daemon is running"}

$ curl -sS --unix-socket /tmp/muster-daemon.sock -X POST http://localhost/health
{"status":"healthy"}

$ curl -sS --unix-socket /tmp/muster-daemon.sock -o /dev/null -w '%{http_code}\n' http://localhost/not-a-route
200

$ curl -sS --unix-socket /tmp/muster-daemon.sock http://localhost/not-a-route
{"status":"ok","message":"Muster daemon is running"}

$ ./bin/openapi-daemon stop --socket-path /tmp/muster-daemon.sock
Daemon stopped successfully
```

**Mode B — `start --backend-url` in front of a local file server:**

```text
$ ./bin/openapi-daemon start --socket-path /tmp/muster-daemon-proxy.sock --backend-url http://127.0.0.1:18989
Proxying requests to: http://127.0.0.1:18989
Daemon started successfully
PID file: /tmp/muster-daemon-proxy.sock.pid
Socket: /tmp/muster-daemon-proxy.sock

$ curl -sS --unix-socket /tmp/muster-daemon-proxy.sock http://localhost/health
{"status":"healthy"}

$ curl -sS --unix-socket /tmp/muster-daemon-proxy.sock http://localhost/hello.txt
hello from the backend

$ curl -sS --unix-socket /tmp/muster-daemon-proxy.sock -o /dev/null -w '%{http_code}\n' http://localhost/missing.txt
404

$ cat /tmp/doc7-final-backend.log
Serving HTTP on 127.0.0.1 port 18989 (http://127.0.0.1:18989/) ...
127.0.0.1 - - [27/Sep/2026 00:57:40] "GET /hello.txt HTTP/1.1" 200 -
127.0.0.1 - - [27/Sep/2026 00:57:40] code 404, message File not found
127.0.0.1 - - [27/Sep/2026 00:57:40] "GET /missing.txt HTTP/1.1" 404 -

$ ./bin/openapi-daemon stop --socket-path /tmp/muster-daemon-proxy.sock
Daemon stopped successfully
```

The backend log is the proof that `/health` was answered locally: the `/health` request never reached
the backend, while `/hello.txt` and `/missing.txt` did (with the backend's own status codes).

## Configuration

Everything the binary accepts, from its own `--help` (Go's `flag` package accepts both `-flag` and
`--flag`):

```text
$ ./bin/openapi-daemon --help
Usage: ./bin/openapi-daemon [start|stop|status|restart|reload|proxy] [flags]

Commands:
  start
  stop
  status
  restart
  reload
  proxy

Flags:
  -backend-url string
    	Backend URL for proxy mode
  -foreground start
    	Run start in the foreground instead of daemonizing (default: daemonize)
  -pid-file string
    	Path for PID file (default: /tmp/openapi-cli.sock.pid)
  -socket-path string
    	Path for Unix domain socket (default "/tmp/openapi-cli.sock")
  -version
    	Print version information and exit
  -wasm-plugin string
    	Path to WASM plugin for response transformation
```

| Flag | Default | Meaning | Source |
|------|---------|---------|--------|
| `--socket-path` | `/tmp/openapi-cli.sock` | Unix socket to bind | `cmd/daemon/main.go:24`, `:75` |
| `--backend-url` | empty | Enables proxy mode; required by `proxy`, optional for `start` | `cmd/daemon/main.go:74` |
| `--pid-file` | `<socket-path>.pid` | PID file used by `status`/`stop`/`restart` | `cmd/daemon/main.go:76`, `:110` |
| `--foreground` | `false` | Run `start` without spawning the detached child | `cmd/daemon/main.go:79` |
| `--version` | `false` | Print version and exit | `cmd/daemon/main.go:78` |
| `--wasm-plugin` | empty | Declared and parsed but never read — no code path consumes it (`cmd/daemon/main.go:39`, `:77`); accepted for compatibility only | — |

`start` daemonizes by default: it re-executes itself with `MUSTER_DAEMON_CHILD=1` and returns once the
socket and PID file are live (`cmd/daemon/main.go:228`, `:294`). `restart` and `reload` are the same
action (`cmd/daemon/main.go:139`), and `restart` stops the old daemon before spawning the new one
(`cmd/daemon/main.go:470`). Set `MUSTER_DAEMON_CHILD=1` to keep `start` in the foreground instead of
using `--foreground`.

There are **no** YAML configuration keys for this surface. No `mcp:`/`rest:`/`auth:`/`rate_limit:`/
`cors:` block configures the daemon — it takes flags only. `docs/api/mcp-protocol.md` covers the MCP
server, which is a different binary (`openapi-mcp`), not this socket.

## Not implemented

The previous revision of this file invented the following surface. It is recorded here so a reader
knows the removals were deliberate, not an accidental deletion of real documentation. None of these
routes are registered anywhere in the repository:

* **Transport / config it claimed:** a TCP base URL on port 8080, an API-key request header, an
  `auth: {type: apikey}` / `auth: {type: oauth2}` block, an `mcp:` block with `port`/`host`, `rest:`
  and `rate_limit:` blocks, CORS configuration, and WebSocket event streaming.
* **Routes it claimed:** `/spec/*` (validate, info, convert), `/commands` and `/commands/execute`,
  `/api/request` and `/api/batch`, `/docs/generate` and `/docs/*`, `/config` (GET and PUT) and
  `/config/validate`, `/cache/stats` and `DELETE /cache`, `/auth/status`, `/auth/login`,
  `/auth/logout`, `POST /webhooks`, and the `/events` WebSocket.
* **Behaviour it claimed:** a structured `{"error":{"code":...,"message":...,"details":...}}` error
  envelope with a code table, `X-RateLimit-*` headers, and webhook events
  (`api.request`, `api.response`, `api.error`, `spec.loaded`, `command.executed`).

None of these exist. The one thing to understand about probing this daemon with that map in hand: the
`/` catch-all answers *every* unregistered path with HTTP 200 and the root payload, so a request to
one of the routes above looks like a success until you read the body. Check the body, or check the
route table above.

## Related documentation

* [MCP Protocol](mcp-protocol.md) — the `openapi-mcp` server (a separate binary and transport)
* [API Overview](README.md) — index of the API documents
* [Reference Card](reference-card.md) — condensed CLI/daemon reference
