# PROTO-010: HTTP/2 and HTTP/3 client transport support for Muster

**Row:** PROTO-010 (P1, complexity 2) — INVESTIGATE, no code
**Date:** 2026-09-26
**Status:** Investigation complete. **No product code changed by this row.**
**Design authority:** [specs/PROTOCOLS.md](../../specs/PROTOCOLS.md) (§1 adapter seam, §2 support matrix, §3 stage contract, §4 cross-cutting contract, §6 gates)
**Feeds:** PROTO-012 (SPEC: matrix rows + gates), PROTO-014 (h2), PROTO-015 (h3), PROTO-017 (tests)
**Trigger:** Bane, 2026-09-26 — *"We are going to want this webdav all versions to have support for muster this means muster will need tasks for getting http2 and http3 client support also."*

**Method.** Every number below is from execution on this host, not estimation. The probes are throwaway Go programs in a scratch module (`/tmp/p010`, `module p010` with
`replace github.com/wojons/muster => /home/kara/worktrees/muster-PROTO-010`), so the worktree stayed clean; every HTTP server used was in-process on loopback (127.0.0.1), matching the network-free rule in `specs/PROTOCOLS.md` §4.6. Environment: `go1.26.6 linux/amd64`, `Linux 7.0.0-31-generic x86_64`. Probe sources, the runner, and the full raw transcript are listed in Appendix A/B.

---

## 0. Verdict in one screen

| Question (from the row) | Answer | Basis |
|---|---|---|
| (a) Are h2/h3 transport variants under the existing client, or a new adapter in the matrix? | **Transport variants of the existing HTTP-family path.** They attach at stage 4 only. They cannot be new adapters for the same protocol, and a new `Protocol` constant for a version would duplicate the operation model for no discovery gain. | `pkg/protocol/registry.go:33-35` (one Adapter per `Protocol`), §1.3/§1.4 of the spec, §2 below |
| (b) Which library pairs with the stdlib without forking request semantics? | **h2: none** (stdlib already negotiates it — measured HTTP/2.0 on the current client). **h3: `github.com/quic-go/quic-go` v0.63.0**, whose `http3.Transport` **is** an `http.RoundTripper` — measured to drive GET/PUT/PROPFIND/DELETE unchanged through `pkg/client`. | §3, §4; transcript `h2probe`, `h3probe` |
| (c) How is ALPN/alt-svc negotiation surfaced so a caller sees the version used? | **`http.Response.Proto` is the only surface, and today it is unreliable**: it is truthful on the bare path but **the response cache forges it to `HTTP/1.1` and drops every header except `Content-Type` for 2xx GETs** (`pkg/client/cache.go:110-126`). There is no Alt-Svc consumer anywhere (stdlib or quic-go client). | §6; transcript `wrapperprobe`, `seamprobe`, GOROOT grep |
| (d) Does the unary/streaming contract in PROTOCOLS.md §3 apply unchanged? | **Yes.** Streaming response bodies, request bodies and deadlines behave identically over h2 and h3 (measured: first chunk at 2–3 ms while total was 364 ms; 1 MiB PUT intact both ways; context deadline → `CodeTimeout`). No Executor signature changes. | §7; transcript `streamprobe`, `h3probe` |

Two findings are **not** about h2/h3 at all and should reach the board as their own rows (§9): the cache round-tripper's forged `Proto`/dropped headers, and `SetHTTPClient` silently discarding `WithCache`/`WithRateLimiter`.

---

## 1. The adapter seam, located (AC1)

### 1.1 The seam as declared

`specs/PROTOCOLS.md` §1.2 (lines 39-61) defines four decoupled stages: **1 Discover/Ingest (per protocol) → 2 Operation model (neutral) → 3 Command + MCP tool generation (shared) → 4 Transport Executor (per protocol)**. §1.3 (lines 63-169) freezes the interface signatures; §1.4 (lines 184-194) says which existing packages each stage reuses; §3.5 (lines 253-257) is the stage-4 contract.

The seam is **real code today**, not only spec text:

| Seam element | Spec anchor | Code anchor |
|---|---|---|
| Neutral operation model (`Operation`, `Parameter`, `RequestBodyProperty`, `OperationKind`) | §1.3 (lines 84-140) | `pkg/protocol/protocol.go:27-83` |
| `Discoverer` (stage 1) | §1.3 (lines 142-148) | `pkg/protocol/protocol.go:88-91` |
| `Executor` (stage 4) — the transport boundary | §1.3 (lines 150-161) | `pkg/protocol/protocol.go:97-104` |
| `Adapter` = `Protocol() + Discoverer + Executor` | §1.3 (lines 163-168) | `pkg/protocol/protocol.go:107-111` |
| `Registry` (one adapter per protocol family) | §1.3 (lines 173-182) | `pkg/protocol/registry.go:13-53` |
| HTTP-family stage-4 reuse ("compose `pkg/client.Client`") | §1.4 line 194 (Transport row), §3.5 lines 255-257 | `pkg/client/client.go`, `pkg/protocol/{jsonrpc,graphql}/client.go` |

**Seam state caveat (measured).** `pkg/protocol/` contains `grpc/`, `graphql/`, `jsonrpc/` **and no `openapi/` adapter directory**; `ProtocolOpenAPI` (`pkg/protocol/protocol.go:19`) has **no non-test referent**, and no non-test code calls `protocol.NewRegistry()`. So the neutral model and three adapters exist, while the OpenAPI refit and registry wiring that §6 assigns to order 0 ("part of PROTO-002's seam landing", spec line 381) are still open. The OpenAPI HTTP path a WebDAV client would build on runs today through the pre-seam chain `pkg/request.Builder` → `pkg/client` (`internal/builtin/request.go:148-167`).

### 1.2 Where a transport attaches

For HTTP-family adapters, the established convention is a **shared `*client.Client` injected by option**:

- `pkg/protocol/jsonrpc/client.go:76` (`httpClient *client.Client` field) + `:89` (`WithHTTPClient(c *client.Client) Option`) — its own doc comment cites §3.1: *"adapters must not open their own HTTP clients for discovery; callers should pass the process-wide client here."* Default construction at `:133` (`client.NewClient()`).
- `pkg/protocol/graphql/client.go:122`, `:140` (`WithHTTPClient`), `:188` — the same shape.
- `pkg/client/client.go:99-112` (`NewClient`), `:315-316` (`HTTPClient() *http.Client`), `:320-322` (`SetHTTPClient(*http.Client)`).
- Wrapper options that hang off the transport: `WithRateLimiter` (`pkg/client/ratelimit.go:85-97`), `WithCache` (`pkg/client/cache.go:142-154`).

**A transport therefore attaches by choosing `http.Client.Transport` on the one shared client** — an `http.RoundTripper` value. Two attachment points exist:

1. `Client.SetHTTPClient(&http.Client{Transport: …})` (`pkg/client/client.go:320`) — works today, but see the measured ordering hazard in §4.2.
2. A new `ClientOption` (e.g. `WithTransport(rt http.RoundTripper)`) — additive, participates in the same option pipeline as `WithRateLimiter`/`WithCache`, and is the attachment point PROTO-014 should add. This is a **proposal**, not existing code.

Because the transport is carried on `*http.Client`, one injected client gives **every** HTTP-family adapter the same version behaviour — no per-adapter code, no second call path.

### 1.3 Why h2/h3 must not be new entries in the *adapter* sense

`Registry.Register` rejects a second adapter for an existing `Protocol` (`pkg/protocol/registry.go:33-35`, `errors.CodeValidation "adapter for %q already registered"`), and `Adapter(p)` returns `CodeNotFound` otherwise (`:45-53`). Registering "HTTP/2" and "HTTP/3" as adapters would require new `Protocol` constants, which in turn requires:

- a `Discoverer` per version — but a version produces **no** new operations (discovery is identical; only `Operation.Source` transport ground truth exists already), so each version would re-emit a duplicate copy of every operation;
- the same discovery to run three times for one document, with three credential/rate-limit compositions for one endpoint;
- new subcommand namespaces per §5.1/§5.2 ("the protocol is a SUBCOMMAND NAMESPACE"), i.e. `openapi-cli http2 …`, `openapi-cli http3 …` — a surface no operator asked for.

The seam's own structure points the other way: stage 4 is explicitly replaceable per protocol (§1.2 line 37: *"stage 4 (executor) is replaceable per protocol"*), and stage 3 is explicitly *shared, not per-protocol* (line 44). **A version belongs to stage 4.** Cost of the alternative is measured in the row above; benefit is zero.

---

## 2. What "HTTP/2 client support" and "HTTP/3 client support" mean concretely (AC2)

### 2.1 Measured baseline: Muster already speaks HTTP/2, by accident

`pkg/client.NewClient` builds `&http.Client{Timeout: 30 * time.Second}` with **no `Transport`** (`pkg/client/client.go:101-103`). A nil `Transport` means `http.Client` uses `http.DefaultTransport`, whose `ForceAttemptHTTP2` is `true`. Measured against a loopback TLS server advertising `[h2 http/1.1]`:

```
C1 muster client, NIL transport (client.go:101-103)        client resp.Proto=HTTP/2.0   server saw=HTTP/2.0   err=<nil>
C2 http.Transport{TLSClientConfig}  (no ForceAttemptHTTP2) client resp.Proto=HTTP/1.1   server saw=HTTP/1.1   err=<nil>
C3 http.Transport{ForceAttemptHTTP2:true}                  client resp.Proto=HTTP/2.0   server saw=HTTP/2.0   err=<nil>
C4 muster SetHTTPClient(&h2 transport) (client.go:320)     client resp.Proto=HTTP/2.0   server saw=HTTP/2.0   err=<nil>
C6 control: ALPN NextProtos=[http/1.1]                     client resp.Proto=HTTP/1.1   server saw=HTTP/1.1   err=<nil>
B. http.DefaultTransport.ForceAttemptHTTP2 = true
D. recordingRT inner-transport invocations during C1 = 2  (nil Transport => http.DefaultTransport)
```

Two things follow. **(i)** h2 today is not a decision, it is the accidental consequence of never setting a transport: `grep -rn "http.Transport" --include=*.go . | grep -v _test | wc -l` → **0** in non-test code, and 5 further `&http.Client{…}` constructions exist outside `pkg/client` (§4.1). **(ii)** The moment anyone adds a hand-built transport — for TLS options, a proxy, a custom dialer, an h3 fallback wrapper — **h2 disappears silently** (C2: HTTP/1.1 on a server that offered h2). That is the exact failure mode PROTO-014 exists to prevent, and it needs a test that reads the version from the response, per PROTO-017's anti-gaming clause.

### 2.2 "HTTP/2 client support" concretely

**No struct, interface or signature changes.** The work is configuration plus observability:

| Layer | Change | Kind |
|---|---|---|
| `pkg/client` | new `ClientOption` `WithTransport(rt http.RoundTripper)` (or equivalent) that installs a transport **inside** the option pipeline so wrapper options compose | additive, new option |
| `pkg/client` | the default transport becomes explicit: `&http.Transport{ForceAttemptHTTP2: true, …}` derived from `http.DefaultTransport`, replacing reliance on the nil-transport path | behaviour-preserving |
| `pkg/config` | one knob for the version policy (e.g. `request.http_version: auto\|2\|1.1`) beside the existing `Cache`/`RateLimit` config (`pkg/config/types.go:26-27`) | additive field |
| CLI/executor | surface the version actually used: `resp.Proto` is not printed anywhere today (§6.2) | additive output |
| tests | assert `resp.Proto` **and** the server-side `r.Proto` on an in-process `httptest` h2 server, plus a control that an http/1.1-only server is reported as HTTP/1.1 | new tests |

Optional, and free: `golang.org/x/net/http2` is **already compiled into the CLI binary** (`go mod why golang.org/x/net/http2` → `pkg/protocol/grpc → google.golang.org/grpc → grpc/internal/transport → golang.org/x/net/http2`), so if a row ever needs `http2.ConfigureTransports` for finer h2 control, it adds **zero** new modules.

### 2.3 "HTTP/3 client support" concretely

h3 is a **new stage-4 transport implementation**, attached the same way:

| Layer | Change | Kind |
|---|---|---|
| dependency | `github.com/quic-go/quic-go` becomes a direct require (plus `qpack`, pulled transitively) | new dependency |
| transport | `&http3.Transport{TLSClientConfig: …, QUICConfig: …}` assigned to `http.Client.Transport` — it satisfies `http.RoundTripper` (compile-time assertion verified: `var _ http.RoundTripper = (*http3.Transport)(nil)`) | new value, no new interface |
| fallback | because version selection **is** transport selection (§6.3), "try h3, fall back to h2" is a wrapper `RoundTripper` the caller supplies; neither the stdlib nor quic-go provides one | new (small) code |
| config | `request.http_version: 3` (opt-in, per PROTO-015's "opt-in, HTTP/2 retained as fallback") | additive field |
| tests | in-process `http3.Server` on a UDP loopback socket is the h3 analogue of `httptest` — measured working, network-free | new tests |

Measured end-to-end through the **unmodified** `pkg/client` (scratch module imports the worktree; only `SetHTTPClient` differs from the h2 run):

```
H3-1 GET through muster client (h3 transport)   resp.Proto=HTTP/3.0  200 OK  server saw=HTTP/3.0  body h3-ok:GET:0
H3-2 PUT with body (WebDAV write)               resp.Proto=HTTP/3.0  200 OK  server saw=HTTP/3.0  body h3-ok:PUT:12
H3-3 PROPFIND (WebDAV verb, no body)            resp.Proto=HTTP/3.0  200 OK  server saw=HTTP/3.0
H3-4 DELETE (WebDAV verb)                       resp.Proto=HTTP/3.0  200 OK  server saw=HTTP/3.0
H3-5 GET /slow with 300ms ctx deadline          ERR: context deadline exceeded   (Wire trace proves the request was sent)
D. http3.Transport implements http.RoundTripper: *http3.Transport
```

### 2.4 What does *not* change

- No new `protocol.Protocol` constant, no new `Discoverer`, no change to `Operation`/`Parameter`/`RequestBodyProperty`, no change to `Executor.Execute` (§1.3 stays byte-identical).
- No new output format (§4.4 rule) and no adapter writing to stdout.
- Request semantics are the transport's, not ours: 1 MiB PUT bodies arrive intact (`received=1048576`) over both h2 and h3.

---

## 3. Dependencies and cost (AC3)

### 3.1 HTTP/2 — no new module

| Fact | Measurement |
|---|---|
| h2 negotiated with zero new modules | `C1`/`C3`/`C4` all `HTTP/2.0` using only the stdlib |
| stdlib carries h2 itself | `$GOROOT/src/net/http/h2_bundle.go` present; ALPN observed on the wire |
| even the *explicit* h2 package is already linked | `go mod why golang.org/x/net/http2` → `pkg/protocol/grpc → google.golang.org/grpc → …/internal/transport → golang.org/x/net/http2`; `go list -deps ./cmd/openapi-cli` includes `golang.org/x/net/http2` |
| plaintext h2 is **not** available | cleartext request to an h2c-capable server → `resp.Proto=HTTP/1.1`, server saw `HTTP/1.1` (`3a`) |

The last row matters for bunkerd: **over cleartext there is no h2** (stdlib does no h2c upgrade or prior-knowledge h2c), and **h3 cannot exist without TLS 1.3** (§3.2). A plaintext WebDAV surface is HTTP/1.1-only. That is a wire fact PROTO-012 must put in the negotiation contract.

### 3.2 HTTP/3 — `quic-go`, measured

| Measurement | v0.59.1 (in module cache) | v0.63.0 (latest) |
|---|---|---|
| `go` directive (min toolchain) | `go 1.24` | `go 1.26.0` (repo is `go 1.26.6` → satisfied) |
| modules added to Muster's graph (`go list -m all`: 102 → N) | 102 → **114** (**+12**) | 102 → **112** (**+10**) |
| of which compiled into the binary | **2** — `github.com/quic-go/quic-go`, `github.com/quic-go/qpack v0.6.0` (all others are test/tool-only: testify family, `go.uber.org/mock`, `gcassert`, `x/tools`, `x/mod`, `x/term`, `go-spew`, `go-difflib`, `go-ossfuzz-seeds`) | same 2 runtime, 8 test/tool |
| existing `x/*` versions moved | **none** — MVS keeps Muster's newer pins: `x/net v0.58.0` (quic-go asks v0.43.0/v0.56.0), `x/crypto v0.57.0` (asks v0.41.0/v0.54.0), `x/sys v0.48.0`, `x/sync v0.23.0` | same, verified by `go list -m` |
| real h3 round trip against Muster's graph | HTTP/3.0 on GET/PUT/PROPFIND/DELETE | HTTP/3.0, identical results |
| source size in `GOMODCACHE` | quic-go 4.5 MB (dir listing, `du -sh`) | quic-go **3,389,018 B**, qpack **205,979 B** (3.6 MB total) |
| binary weight (minimal `net/http` program, same toolchain) | — | +**312,019 B** default build; +**143,360 B** with `-ldflags="-s -w"` |
| cross-compile | — | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build` → **rc=0**, 12,168,935 B (pure Go, no CGO) |
| licence | MIT (`LICENSE`, "Copyright (c) 2016 the quic-go authors & Google, Inc.") | MIT |
| vulnerability scan (`govulncheck`) | — | **0 reachable**; 1 module-level advisory in a package the h3 path does not call |
| build time in the repo | not measured (see Open questions) | — |

**Recommendation for PROTO-015:** pin **v0.63.0** — the only measured option that adds fewer graph entries (+10 vs +12), is the current release, and whose `go 1.26.0` floor the repo's `go 1.26.6` already meets. Adding it does **not** perturb any existing dependency version, which is the property that keeps the supply-chain decision small.

### 3.3 Why there is no stdlib/x-net escape hatch (measured)

- `$GOROOT/src/net/http/` has **no** HTTP/3: grep for `HTTP/3|http3` finds only prose in `internal/httpcommon` ("used on the wire for HTTP/2 and HTTP/3 requests") — the encoder is shared, the h3 transport is not present.
- `golang.org/x/net@v0.58.0` — already in Muster's graph — ships `http3/`, but it is a **pre-public-API shim**: the package contains one file with three *unexported* functions (`registerServer`, `registerTransport`, `shallowStructCopy`) wired by `//go:linkname` to `net/http_test`, with the comment *"This is a temporary measure pending this package having a public API"*. The real implementation sits in `internal/http3` (`RegisterTransport(tr *http.Transport, opts TransportOpts) error`), which is **importable only from within `x/net`**. There is no client `RoundTripper`, and no exported symbol an external consumer can call.

So: **h3 requires a QUIC implementation from outside the standard library**, and quic-go is the only mature Go option measured here.

---

## 4. Composition with `pkg/client` (AC4)

### 4.1 Call-site census — one shared client, not a parallel stack

Non-test constructors of an HTTP client in the tree (measured by grep):

| Via `pkg/client.NewClient` | Direct `&http.Client{…}` (no `pkg/client`) |
|---|---|
| `internal/builtin/discover.go:56` | `pkg/request/builder.go:278` (`newHTTPClient`, `MUSTER_HTTP_TIMEOUT`) |
| `internal/builtin/protocol.go:96` | `pkg/completion/dynamic.go:156` |
| `internal/builtin/graphql.go:141` | `pkg/auth/token_manager.go:29` |
| `internal/builtin/request.go:166` (the CLI `request` path) | `pkg/auth/oauth2_flow.go:90` |
| `cmd/mcp-server/main.go:231` | `pkg/daemon/proxy.go:524,535` (proxy **server** side) |
| `pkg/protocol/jsonrpc/client.go:133` | |
| `pkg/protocol/graphql/client.go:188` | |

Seven sites compose `pkg/client`; five build a bare `http.Client`. A transport policy applied only to `pkg/client` therefore reaches the operation-execution path (CLI request verb, MCP adapters) but **not** discovery-time fetches that use `pkg/request`/`pkg/auth`, nor the daemon proxy. PROTO-012 should decide whether that is acceptable (the h2/h3 requirement is about consumption of the WebDAV surface, so the seven `pkg/client` sites are the requirement) and say so explicitly rather than implying a global policy.

### 4.2 Measured hazard — `SetHTTPClient` ordering

`SetHTTPClient` replaces the whole `*http.Client` (`pkg/client/client.go:320-322`), which **throws away whatever the `ClientOption`s installed**:

```
1a wrappers only              : 2 GETs -> wire hits=1  transport=*client.CacheRoundTripper
1b options THEN SetHTTPClient : 2 GETs -> wire hits=2  transport=*http.Transport       (cache lost; both GETs hit the wire)
1c SetHTTPClient THEN option  : 2 GETs -> wire hits=1  transport=*client.CacheRoundTripper
```

Both `WithCache` (`pkg/client/cache.go:148-152`) and `WithRateLimiter` (`pkg/client/ratelimit.go:91-95`) run **at option-application time**, wrapping whatever `Transport` exists then — `nil` on a fresh client, so they capture `http.DefaultTransport` (see below). This is why PROTO-014 should attach the transport **as an option** (one pipeline, deterministic order) rather than by having callers call `SetHTTPClient` after `NewClient(opts…)`.

### 4.3 Measured hazard — the cache wrapper forges the response

`CacheRoundTripper` does not return the wire response for 2xx GETs: it reads the body and rebuilds the response (`pkg/client/cache.go:76-95`, `buildCachedResponse` at `:110-126`), setting `Proto: req.Proto`, `ProtoMajor: req.ProtoMajor` and a **fresh `Header` containing only `Content-Type`**. Measured with the wire as the oracle (server counts `r.Proto` of every request it actually served):

```
variant                                     verb  req#  resp.Proto   X-Server   X-Other       body   wire(server-seen)
V0 none (bare NewClient+WithTimeout)        GET   1     HTTP/2.0     HTTP/2.0   must-survive  {...}  HTTP/2.0
V1 WithRateLimiter only                     GET   1     HTTP/2.0     HTTP/2.0   must-survive  {...}  HTTP/2.0
V2 WithCache only                           GET   1     HTTP/1.1     <absent>   <absent>      {...}  HTTP/2.0     <- wire AND client disagree
V2 WithCache only                           POST  1     HTTP/2.0     HTTP/2.0   must-survive  {...}  HTTP/2.0,HTTP/2.0
V3 WithRateLimiter + WithCache (request.go:158,164) GET 2 HTTP/1.1   <absent>   <absent>      {...}  HTTP/2.0,HTTP/2.0   <- 2nd GET served from cache
```

Consequences, all measured:

1. On a **cache miss** as well as a hit, a 2xx GET reports `resp.Proto == "HTTP/1.1"` even when the wire used HTTP/2.0 — the version the row wants to expose is, on this path, a lie.
2. Every response header except `Content-Type` is dropped — including the `Alt-Svc` header an h3-capable server would send (§6.1).
3. Non-GET verbs pass through untouched (`cache.go:56-58`), so the lie is verb-dependent: the same client tells the truth for POST and untruth for GET.
4. `WithCache` is enabled by config on the CLI request path (`internal/builtin/request.go:160-165`), so this is a live production shape, not a test-only path. The same file already carries a NOTE that the cache is created per request (`:162-163`).

PROTO-014/017 cannot satisfy "a caller can see which version was actually used" while this stands: either the wrapper preserves `Proto`/headers, or the transport records the negotiated version where the wrapper cannot overwrite it. This finding is filed as a candidate row in §9, **not** changed here.

### 4.4 The `http.DefaultTransport` capture

Both wrappers substitute the *process-global* `http.DefaultTransport` when `Transport == nil` (`cache.go:38`, `:149-151`; `ratelimit.go:35`, `:92-94`). That is what makes an in-process probe need to inject trust into the global (§Appendix A), and it is also why an h3 transport must be installed as a real value rather than by mutating globals.

---

## 5. The rows `specs/PROTOCOLS.md` would gain, and the gates they imply (AC5)

The matrix today is §2 lines 211-226 with **14 rows** (grep `^| [0-9]* | \*\*` → 14), the maintenance rule at line 228, and the gate table §6 lines 379-389 (orders 0-8) with the rules at line 391. `grep -n "HTTP/1\|HTTP/2\|HTTP/3\|ALPN" specs/PROTOCOLS.md` → **no matches**: the document has no transport-version dimension and no negotiation contract at all. PROTO-012 fills that gap; this section states what it should contain, grounded in the measurements above.

### 5.1 Proposed matrix rows (proposal — PROTO-012 decides wording and order)

Rows are appended to keep the existing numbering stable, and follow the current column shape `| # | Protocol | Verdict | Order | Reason |`:

| # | Protocol | Verdict | Order | Reason (measured basis) |
|---|---|---|---|---|
| 15 | **HTTP/2 client transport (h2)** | ADOPT — no new dependency | 9 | Already negotiated today via `http.DefaultTransport` (`C1` = HTTP/2.0, zero modules added). The row is configuration + observability, not protocol code; `golang.org/x/net/http2` is already linked (`go mod why`). Gate must prove the version is *observed*, because a hand-built transport silently drops to HTTP/1.1 (`C2`) and a plaintext URL is HTTP/1.1 regardless (`3a`). |
| 16 | **HTTP/3 client transport (QUIC/h3)** | ADOPT — opt-in, one new dependency | 10 | No stdlib h3 and no importable `x/net` h3 (§3.3). `github.com/quic-go/quic-go` measured: `http3.Transport` **is** an `http.RoundTripper`, +10 graph entries of which 2 compile, +3.6 MB module cache, +312,019 B binary, MIT, 0 reachable vulns, `CGO_ENABLED=0` cross-compiles. Opt-in with h2 retained: version selection is transport selection and QUIC is UDP-only (§6.3), so a fallback must be explicit. |
| 17 | **WebDAV (RFC 4918) as an HTTP application profile** | ADOPT or explicitly out-of-scope — **decision required** | 11 | `grep -ri "webdav\|propfind" specs/ docs/ .memory-bank/` → **nothing** in the repo. Bane's directive makes Muster a WebDAV **consumer**, so the matrix needs either a WebDAV row (methods `PROPFIND/PROPPATCH/MKCOL/COPY/MOVE/LOCK/UNLOCK` with a manifest as stage-1 discovery input, on the HTTP-family stage-4 path) or an explicit statement that WebDAV is reached through the generic HTTP path with a hand-written operation manifest. Choosing "nothing" leaves the directive unimplementable in the seam. |

**Rows *not* proposed:** no new `Protocol` constants, and therefore no adapter rows, for HTTP versions (§1.3). The `Protocol` enum (`pkg/protocol/protocol.go:18-25`) stays as it is.

### 5.2 Stage-contract changes implied (exact clause-level edits for PROTO-012)

| Spec location | Change implied | Why (measured) |
|---|---|---|
| §1.2 line 37 | state that a protocol **version** is a stage-4 property, not a stage-1/2 family | §1.3; `registry.go:33-35` |
| §1.4 line 194 (Transport row) | add: "HTTP-family adapters inherit the HTTP version from the shared `pkg/client.Client`; changing the version MUST NOT create a second client or call path" | The `WithHTTPClient` convention already carries this (`jsonrpc/client.go:89`, `graphql/client.go:140`) |
| §3.5 lines 255-257 | add the transport-selection clause: version is chosen by the transport installed on the shared client; adapters MUST NOT negotiate versions themselves | `C2`/`C4`, §6.3 |
| new §4.7 "Transport version negotiation & observability" | (a) `resp.Proto` is the observable version; (b) a downgrade is a reported fact, not a silent success; (c) **the response cache MUST NOT synthesize `Proto`/drop headers** (current `cache.go:110-126` violates a), so h2/h3 acceptance depends on that fix; (d) Alt-Svc is not consumed by anything — h3 discovery is out of band | `wrapperprobe` V2/V3; `seamprobe` 2; GOROOT grep for `Alt-Svc` → 0 hits |
| §4.6 lines 337-349 | add the h3 in-process pattern: `http3.Server` on a UDP loopback socket + `http3.Transport` is the h3 analogue of `httptest`; QUIC mandates TLS 1.3 (`3c` → `CRYPTO_ERROR 0x150`) so the test needs a cert, not a bare listener | `h3probe` (all cases) |
| §6 gate table (after order 8, line 389) | two gate rows, orders 9 and 10 (below) | §5.3 |
| §7 lines 393-405 | one numbered `demo/demos/NN-*.sh` for h2/h3; `demo/demos/` currently holds **10** scripts (`01`–`10`), and §7 pre-assigns `11-grpc-unary.sh`/`12-grpc-streaming.sh`/`13-jsonrpc.sh`, so the h2/h3 script takes the next free ordinal in that queue | `ls demo/demos/` → 10 files |

### 5.3 Gate definitions the two rows imply (must be runnable and network-free, §4.6/§6)

- **Order 9 — h2:** in-process `httptest` TLS server, `EnableHTTP2 = true`, ALPN `[h2, http/1.1]`, loopback only. Assert **both** the client's `resp.Proto` and the server-observed `r.Proto` equal `HTTP/2.0` for GET and for one WebDAV verb; assert a 1 MiB PUT arrives intact; and run the **anti-gaming control** PROTO-017 requires — the same client against a server offering only `http/1.1` must still work and must report `HTTP/1.1` (reading the version from the response, never from configuration). A negative cell guards the measured trap: a hand-built `&http.Transport{}` without `ForceAttemptHTTP2` must *fail* the h2 assertion (it really returns HTTP/1.1).
- **Order 10 — h3:** in-process `http3.Server` on UDP loopback with an in-process self-signed cert + `http3.Transport`. Assert `HTTP/3.0` on both sides for GET/PUT/PROPFIND/DELETE; assert the context deadline maps to `CodeTimeout` (§4.2/§4.3); assert the two negative cells measured here — an h2 transport against an h3-only endpoint fails, and an h3 transport against a TCP-only endpoint fails — which is the *proof* that a fallback is the caller's responsibility and cannot be assumed; and assert that a UDP-blocked environment produces a **reported** fallback, not a silent HTTP/1.1.
- **Gate wiring note:** the guard's test command is `go test -short -count=1 $(go list ./... | grep -v …)` with `test_mode: full` (`.gitreins/config.yaml`), and per AGENTS.md (lines 505-509) **a `go.mod` change forces the full suite** as a safety net. PROTO-015 therefore pays a full-suite run on the committing tick — worth naming in its brief.

---

## 6. Negotiation and observability (row ask (c))

### 6.1 ALPN (h2) versus Alt-Svc (h3) — measured

- **h2 is negotiated in-band by ALPN** inside the TLS handshake and reported by the stdlib as `resp.Proto`. Measured truthfulness on the bare path: client `HTTP/2.0` and server `HTTP/2.0` agree (`C1`/`C3`/`C4`), and the ALPN-suppressed control reports HTTP/1.1 on both sides (`C6`).
- **h3 is not negotiated at all.** Version selection is the choice of transport (§6.3). Discovery of an h3-capable peer happens only out of band, via `Alt-Svc`; measured: a server sending `Alt-Svc: h3=":443"; ma=86400` reaches the caller as a literal header value, `resp.Proto=HTTP/2.0`, and **nothing acts on it** — `grep -rn "Alt-Svc\|AltSvc" $GOROOT/src/net/http/*.go` → **0 hits**, and in quic-go `Alt-Svc` appears only in `http3/server.go` (`SetQUICHeaders`, server side) plus tests. Neither side of the stack ships an Alt-Svc cache.

### 6.2 What a caller can see today

| Surface | h2 | h3 | Notes |
|---|---|---|---|
| `http.Response.Proto` in library code | truthful | truthful | **except** through `WithCache`, where it is forged to `HTTP/1.1` for 2xx GETs (§4.3) |
| CLI `request -v` output | **nothing** | **nothing** | prints `Request:`, `Status:`, masked credential, response headers (`internal/builtin/request.go:195-213`) — `resp.Proto` appears nowhere |
| MCP tool result | nothing | nothing | the payload is the decoded body (§4.4) |
| `Alt-Svc` passthrough | raw header only | raw header only | no consumer |

So the "which version did we actually use" requirement is **not satisfiable today on any user-facing path**: the library value exists but is unreliable under cache, and no CLI/MCP surface prints it. PROTO-014 owns fixing that; PROTO-017 owns asserting it from the response, not the configuration.

### 6.3 Version selection is transport selection (measured, both directions)

```
X-1 h2 transport -> h3-only server   ERR: dial tcp 127.0.0.1:PORT: connection refused
X-2 h3 transport -> h2-only server   ERR: timeout: no recent network activity
```

An `http.Transport` will not fall back to QUIC, and an `http3.Transport` will not fall back to TCP. There is no cross-version negotiation inside either transport and none in the stdlib. Therefore PROTO-015's "HTTP/2 retained as the fallback" is a **deliberate composable `RoundTripper`** (attempt h3, on connection-layer failure retry over h2 and report that a fallback happened), not a configuration flag. UDP blocking cannot be simulated by a test that merely sets an option — the measured negative cells above are the honest way to exercise the fallback path.

---

## 7. Does the unary/streaming contract in PROTOCOLS.md §3 apply unchanged? (row ask (d))

**Yes — unchanged.** §3.5 (`specs/PROTOCOLS.md:253-257`) requires an `Executor` that runs through `pkg/client`; §4.5 defines the bounded-transcript envelope for the streaming `OperationKind`s. Neither interface sees the HTTP version, because the version lives entirely below `http.Client`. Measured on both transports, same client, same `Do` call:

```
proto  ttfb   total   chunks   body
h2     3ms    364ms   3/3      {"n":1}\n{"n":2}\n{"n":3}
h3     2ms    364ms   3/3      {"n":1}\n{"n":2}\n{"n":3}
h2     PUT 1MiB -> 200 OK (echo len=received=1048576) in 5ms  resp.Proto=HTTP/2.0
h3     PUT 1MiB -> 200 OK (echo len=received=1048576) in 7ms  resp.Proto=HTTP/3.0
h2     4 sequential GETs (reuse): 476µs 153µs 96µs  96µs
h3     4 sequential GETs (reuse): 224µs 156µs 119µs 121µs
```

- **Bodies stream incrementally on both** — the first chunk arrived at 2–3 ms while the response took 364 ms, so nothing buffers the body behind the transport. The `Execute` return contract (decoded payload / bounded transcript) is unaffected.
- **Deadlines are honored** — the h3 case returned `context deadline exceeded` on a 300 ms context against a 1.5 s handler, i.e. §4.3's timeout rule maps cleanly to `CodeTimeout`.
- **No h3-specific streaming kind** is introduced. `http3.Transport` exposes `EnableExtendedConnect` (`http3/transport.go:28`, quic-go v0.63.0) for WebSocket-over-h3, but there is **no** corresponding client capability in `net/http`, so this is not a route to the §4.5 stream kinds and should not be presented as one.
- **h3 is not a throughput argument on this link.** Loopback reuse timings are within noise of each other (µs scale), and the two transports differ by ~2 ms on a 1 MiB PUT in-process. Consistent with PROTO-015's note that h3's value here is connection-establishment behaviour, not throughput; **this investigation did not measure a WAN link and does not claim one** (see Open questions).

---

## 8. The consumer relationship: bunker's WebDAV surface (AC6)

The seam's HTTP-family path is what lets Muster consume a WebDAV surface at every HTTP version, and this transport work is the enabling layer for it. Stated precisely:

- **The transport is what makes the consumption possible.** Measured: with `http.DefaultTransport` the client speaks h2 over TLS; with `http3.Transport` installed on the *same* `pkg/client.Client` it speaks h3; over cleartext it is HTTP/1.1 (no h2c). So "Muster supports every version bunkerd's WebDAV surface can negotiate" = "one `*client.Client` whose transport is chosen per version", not a new adapter (§1.3, §4).
- **Cross-repo contract:** the server half of this agreement is **bunker BFS-004** ("WebDAV surface + per-version capability negotiation"), named as the interface this spec must agree with by PROTO-012's own board row (`.coding-hermes/board/tasks.jsonl:141`). The wire facts both sides must agree on are the ones measured here: ALPN `h2`/`http/1.1` in-band; **h3 requires TLS 1.3** (`3c` → `CRYPTO_ERROR 0x150`); **h3 is UDP-only**, so a middlebox or firewall that drops UDP removes it entirely (`X-1`); plaintext surfaces are HTTP/1.1-only (`3a`); and `Alt-Svc` is the only advertisement channel and is not auto-consumed by any Go client (`seamprobe` 2).
- **Transport support alone does not deliver WebDAV consumption.** Measured against a loopback server, using the built CLI (`/tmp/openapi-cli`, 36,692,729 B, from `cmd/openapi-cli`):

```
--- PROPFIND with -b body ---   server saw: proto=HTTP/1.1 method=PROPFIND path=/dav bodyLen=0 body=""
--- PUT with -b body (control) -- server saw: proto=HTTP/1.1 method=PUT      path=/dav/f bodyLen=7 body="{\"a\":1}"
--- MKCOL with -b body ---      server saw: proto=HTTP/1.1 method=MKCOL    path=/dav/d bodyLen=0 body=""
--- request GET -v ---          Request: GET http://127.0.0.1:18099/x
                                Status: 200 200 OK
                                Date: … / Content-Length: 11 / Content-Type: application/json   (no version line)
```

  WebDAV verbs **are** expressible (`openapi-cli request PROPFIND <url>` reaches the wire — `method := strings.ToUpper(args[0])`, `internal/builtin/request.go:50`), but a request **body** is attached only for `POST`/`PUT`/`PATCH` (`request.go:144-146`), so the standard `PROPFIND` with a `<propfind>` body sends **zero bytes** — measured `bodyLen=0`. Resource-typed WebDAV calls and `LOCK` bodies are therefore silently incomplete today. That gap is not a transport concern but it belongs to the same consumer story; §9 lists it as a candidate row.

---

## 9. Findings outside the h2/h3 transport question (candidate rows — none filed by this row)

These were discovered while measuring; they are reported here because they bear directly on the acceptance criteria of PROTO-014/015/017. **This row filed nothing and changed nothing** (AC7).

1. **`CacheRoundTripper` forges `Proto` and drops response headers** for 2xx GETs (`pkg/client/cache.go:76-95`, `:110-126`). Measured: `resp.Proto=HTTP/1.1` and `X-Server-Proto`/`X-Other-Header` absent while the wire was HTTP/2.0, on both miss and hit; POST is unaffected. Directly defeats "the caller can see which version was used" and hides `Alt-Svc`.
2. **`SetHTTPClient` discards the option-installed wrapper chain** when called after `NewClient(opts…)` (`pkg/client/client.go:320-322`; measured 1a/1b/1c: 1 → 2 wire requests, `*CacheRoundTripper` → `*http.Transport`). Any h2/h3 transport injection must respect or fix this ordering.
3. **The CLI drops request bodies for WebDAV verbs** (`internal/builtin/request.go:144-146`: body only for POST/PUT/PATCH; measured `PROPFIND`/`MKCOL` bodyLen=0 while `PUT` bodyLen=7).
4. **No user-facing surface reports the negotiated HTTP version** (`internal/builtin/request.go:195-213`; no `resp.Proto` anywhere in product code).

---

## 10. Open questions (honest unknowns)

1. **Build-time and CI-time cost of adding quic-go** was **not measured** (only graph size, module bytes and binary bytes were). PROTO-015 should measure `go build ./...` wall time and the guard's full-suite run before pinning.
2. **WAN behaviour was not measured.** Every number above is loopback in-process. PROTO-015 explicitly must not assume h3 is faster; this investigation can neither confirm nor refute a real-network advantage (0-RTT vs h2 connection reuse) — the measured link is `127.0.0.1` with no loss, the case where QUIC's loss resilience helps least.
3. **Which quic-go release the maintainers will keep supported** (`v0.59.1` vs `v0.63.0`) is a supply-chain policy call for PROTO-015; both were measured to build against Muster's pins and to complete a real h3 round trip. v0.63.0 adds fewer graph entries.
4. **Whether bunker BFS-004 will advertise h3 via `Alt-Svc`, and on which port** is on the bunker side and unknowable from this repo; Muster cannot consume Alt-Svc without an explicit (small) client-side cache, and nothing in either stack provides one.
5. **Whether Muster should reach WebDAV through a dedicated operation manifest or the generic HTTP path** is the decision PROTO-012/013 must make (row 17 in §5.1). This row measured the current capabilities; it did not decide the modelling.
6. **The exact number in the demo script queue** the h2/h3 script will take depends on the order PROTO-012 assigns relative to the protocol scripts §7 already plans (11-13); `demo/demos/` holds 10 scripts today.
7. **`govulncheck` on the *repo's* full module graph after the dependency lands** was not run; the scan reported here is on the scratch module carrying Muster's pins plus quic-go v0.63.0 (0 reachable, 1 module-level advisory in an uncalled package).

---

## Appendix A — probe inventory and exact commands

Scratch module `/tmp/p010` (`module p010`, `generate`/`replace github.com/wojons/muster => /home/kara/worktrees/muster-PROTO-010`; `go list -m all` = 102 modules before quic-go). Nothing in the worktree was modified (`git status --short` → empty).

| Probe | What it establishes |
|---|---|
| `h2probe/` | which HTTP version each client construction negotiates (C1–C6); that a nil `Transport` really reaches `http.DefaultTransport` (recording `RoundTripper`, 2 invocations) |
| `wrapperprobe/` | the cache/rate-limit wrappers versus negotiated version and headers, with the **server-side wire count as oracle** (V0–V3) |
| `seamprobe/` | `SetHTTPClient` ordering (1a/1b/1c), `Alt-Svc` passthrough (2), cleartext/h2c (3a), TLS-1.3 requirement for QUIC (3c) |
| `h3probe/` | real HTTP/3 round trips (GET/PUT/PROPFIND/DELETE) through the unmodified `pkg/client`; context deadline; both cross-version failures; `RoundTripper` assertion |
| `streamprobe/` | incremental response streaming, 1 MiB PUT integrity, sequential connection reuse, h2 vs h3 |
| `webdavecho/` | the CLI experiment in §8 (logs `r.Proto`, method, body length) |
| `/tmp/sz/` | binary-weight pair (control vs one h3 import) |
| `/tmp/h3only/`, `/tmp/q63/` | standalone quic-go module-graph counts |

```bash
# module graph and versions
cd /tmp/p010 && go list -m all | wc -l                      # 102 baseline / 112 with quic-go v0.63.0
go list -m github.com/quic-go/quic-go github.com/quic-go/qpack golang.org/x/net golang.org/x/crypto
go mod why golang.org/x/net/http2                            # why x/net/http2 is already linked

# probes (transcript in Appendix B)
bash /tmp/p010/run-all-probes.sh > /tmp/p010/probe-transcript.txt 2>&1

# costs
cd /tmp/sz && go build -o bin-a ./a/ && go build -o bin-b ./b/     # 5,221,253 -> 5,533,272 B
govulncheck ./b/                                                  # 0 reachable
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o bin-arm64 ./h3probe/   # rc=0

# CLI experiment (§8)
cd /home/kara/worktrees/muster-PROTO-010 && go build -o /tmp/openapi-cli ./cmd/openapi-cli
/tmp/p010/bin-echo &                       # echo server on 127.0.0.1:18099
/tmp/openapi-cli request PROPFIND http://127.0.0.1:18099/dav -b '{"prop":1}' -o json
/tmp/openapi-cli request PUT      http://127.0.0.1:18099/dav/f -b '{"a":1}'  -o json
/tmp/openapi-cli request MKCOL    http://127.0.0.1:18099/dav/d -b 'x'        -o json
/tmp/openapi-cli request GET      http://127.0.0.1:18099/x -v

# negative source checks
grep -rn "http.Transport" --include=*.go . | grep -v _test | wc -l   # 0 in product code
grep -n "HTTP/1\|HTTP/2\|HTTP/3\|ALPN" specs/PROTOCOLS.md            # no matches
grep -rn "Alt-Svc\|AltSvc" $GOROOT/src/net/http/*.go                 # no matches
```

## Appendix B — raw transcript (verbatim)

```
################################################################
# PROTO-010 probe transcript
# host: Linux 7.0.0-31-generic x86_64 / go version go1.26.6 linux/amd64
# module: /tmp/p010 (scratch module, replace => /home/kara/worktrees/muster-PROTO-010)
# date: 2026-09-26T07:52:53-05:00
################################################################

=== h2probe: which HTTP version does each client construction negotiate? ===
A. loopback httptest server: https://127.0.0.1:40473   EnableHTTP2=true
A. server ALPN NextProtos = [h2 http/1.1]
B. http.DefaultTransport.ForceAttemptHTTP2 = true
B. clone of DefaultTransport keeps ForceAttemptHTTP2 = true

C1 muster client, NIL transport (client.go:101-103)        client resp.Proto=HTTP/2.0   server saw=HTTP/2.0   err=<nil>   [DefaultTransport is used]
C2 http.Transport{TLSClientConfig}  (no ForceAttemptHTTP2) client resp.Proto=HTTP/1.1   server saw=HTTP/1.1   err=<nil>   [hand-built transport]
C3 http.Transport{ForceAttemptHTTP2:true}                  client resp.Proto=HTTP/2.0   server saw=HTTP/2.0   err=<nil>   [opt-in flag]
C4 muster SetHTTPClient(&h2 transport) (client.go:320)     client resp.Proto=HTTP/2.0   server saw=HTTP/2.0   err=<nil>   [injection seam]
C5 muster + WithRateLimiter + WithCache wrappers           client resp.Proto=HTTP/1.1   server saw=           err=<nil>   [config path]
C6 control: ALPN NextProtos=[http/1.1]                     client resp.Proto=HTTP/1.1   server saw=HTTP/1.1   err=<nil>   [forbids h2]

D. recordingRT inner-transport invocations during C1 = 2  (nil Transport => http.DefaultTransport)
D. after NewClient+WithTimeout, HTTPClient().Transport = <nil> (nil)
D. after WithRateLimiter+WithCache, HTTPClient().Transport = *client.CacheRoundTripper (inner transport unexported)
```
*(C5 here is the earlier 2-way run of the same configuration that §4.3's `wrapperprobe` measures with the wire oracle; the cache wrapper rebuilds the response, hence the empty server header.)*

```
=== wrapperprobe: cache/ratelimit wrappers vs negotiated version + headers ===
server: https://127.0.0.1:38215  ALPN=[h2 http/1.1]
http.DefaultTransport swapped for a trusted clone (ForceAttemptHTTP2=true)

variant                                              verb  req#  resp.Proto   X-Server     X-Other      body   wire(server-seen)
V0 none (bare NewClient+WithTimeout)                 GET   1     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0
V0 none (bare NewClient+WithTimeout)                 POST  1     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0,HTTP/2.0
V0 none (bare NewClient+WithTimeout)                 GET   2     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0,HTTP/2.0,HTTP/2.0
V0 none (bare NewClient+WithTimeout)                 POST  2     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0,HTTP/2.0,HTTP/2.0,HTTP/2.0
   transport: <nil>

V1 WithRateLimiter(1000,1000) only                   GET   1     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0
V1 WithRateLimiter(1000,1000) only                   POST  1     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0,HTTP/2.0
V1 WithRateLimiter(1000,1000) only                   GET   2     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0,HTTP/2.0,HTTP/2.0
V1 WithRateLimiter(1000,1000) only                   POST  2     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0,HTTP/2.0,HTTP/2.0,HTTP/2.0
   transport: *client.RateLimitRoundTripper

V2 WithCache(memory,1m) only                         GET   1     HTTP/1.1     <absent>     <absent>     {"ok":true} HTTP/2.0
V2 WithCache(memory,1m) only                         POST  1     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0,HTTP/2.0
V2 WithCache(memory,1m) only                         GET   2     HTTP/1.1     <absent>     <absent>     {"ok":true} HTTP/2.0,HTTP/2.0
V2 WithCache(memory,1m) only                         POST  2     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0,HTTP/2.0,HTTP/2.0
   transport: *client.CacheRoundTripper

V3 WithRateLimiter + WithCache (request.go:158,164)  GET   1     HTTP/1.1     <absent>     <absent>     {"ok":true} HTTP/2.0
V3 WithRateLimiter + WithCache (request.go:158,164)  POST  1     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0,HTTP/2.0
V3 WithRateLimiter + WithCache (request.go:158,164)  GET   2     HTTP/1.1     <absent>     <absent>     {"ok":true} HTTP/2.0,HTTP/2.0
V3 WithRateLimiter + WithCache (request.go:158,164)  POST  2     HTTP/2.0     HTTP/2.0     must-survive {"ok":true} HTTP/2.0,HTTP/2.0,HTTP/2.0
   transport: *client.CacheRoundTripper

=== seamprobe: SetHTTPClient ordering, Alt-Svc, h2c, TLS1.3 requirement ===
1a wrappers only            : 2 GETs -> wire hits=1  transport=*client.CacheRoundTripper
1b options THEN SetHTTPClient: 2 GETs -> wire hits=2  transport=*http.Transport  (cache+ratelimit still in force? false)
1c SetHTTPClient THEN option : 2 GETs -> wire hits=1  transport=*client.CacheRoundTripper
2 Alt-Svc: server sent "h3=\":443\"; ma=86400" -> caller sees "h3=\":443\"; ma=86400" ; resp.Proto=HTTP/2.0 ; anything acting on it? no (see GOROOT grep)
3a cleartext http:// against an h2c-capable server: resp.Proto=HTTP/1.1 serverSaw=HTTP/1.1 err=<nil>
3b h3 default TLS config: resp.Proto=HTTP/3.0 err=<nil>
3c h3 with MaxVersion=TLS1.2 (QUIC mandates TLS1.3): resp.Proto=<nil> err=Get "https://127.0.0.1:56253/x": CRYPTO_ERROR 0x150 (local): tls: no supported versions sa...
3d negotiated TLS version on the working h3 conn: TLS 1.3 (QUIC mandates it; see 3c failure with 1.2 cap)

=== h3probe: real HTTP/3 round trips through muster pkg/client (quic-go v0.63.0) ===
A. HTTP/3 server serving on https://127.0.0.1:60024 (UDP, loopback, in-process)

case                                                       resp.Proto   status     X-Server   body       wire(server-seen)
H3-1 GET through muster client (h3 transport)              HTTP/3.0     200 OK     HTTP/3.0   h3-ok:GET:0 HTTP/3.0 GET /x
H3-2 PUT with body (WebDAV write)                          HTTP/3.0     200 OK     HTTP/3.0   h3-ok:PUT:12 HTTP/3.0 GET /x | HTTP/3.0 PUT /f
H3-3 PROPFIND (WebDAV verb, no body)                       HTTP/3.0     200 OK     HTTP/3.0   h3-ok:PROPFIND:0 HTTP/3.0 GET /x | HTTP/3.0 PUT /f | HTTP/3.0 PROPFIND /d
H3-4 DELETE (WebDAV verb)                                  HTTP/3.0     200 OK     HTTP/3.0   h3-ok:DELETE:0 HTTP/3.0 GET /x | HTTP/3.0 PUT /f | HTTP/3.0 PROPFIND /d | HTTP/3.0 DELETE /f
H3-5 GET /slow with 300ms ctx deadline                     -            -          -          ERR: Get "https://127.0.0.1:60024/slow": context deadline exceede... HTTP/3.0 GET /x | HTTP/3.0 PUT /f | HTTP/3.0 PROPFIND /d | HTTP/3.0 DELETE /f | HTTP/3.0 GET /slow
X-1 h2 transport -> h3-only server (expect failure)        -            -          -          ERR: Get "https://127.0.0.1:60024/x": dial tcp 127.0.0.1:60024: c... HTTP/3.0 GET /x | HTTP/3.0 PUT /f | HTTP/3.0 PROPFIND /d | HTTP/3.0 DELETE /f | HTTP/3.0 GET /slow
X-2 h3 transport -> h2-only server (expect failure)        -            -          -          ERR: Get "https://127.0.0.1:41283/x": timeout: no recent net... 

D. http3.Transport implements http.RoundTripper: *http3.Transport

=== streamprobe: streaming bodies, 1 MiB PUT, connection reuse ===
proto  ttfb       total          chunks     first-chunk-before-last-sent?
h2     3ms        364ms          3/3        first="{\"n\":1}" all="{\"n\":1}\n{\"n\":2}\n{\"n\":3}"
h3     2ms        364ms          3/3        first="{\"n\":1}" all="{\"n\":1}\n{\"n\":2}\n{\"n\":3}"
h2     PUT 1MiB body -> 200 OK (echo len=received=1048576) in 5ms  resp.Proto=HTTP/2.0
h3     PUT 1MiB body -> 200 OK (echo len=received=1048576) in 7ms  resp.Proto=HTTP/3.0
h2     4 sequential GETs (reuse): 476µs 153µs 96µs 96µs 
h3     4 sequential GETs (reuse): 224µs 156µs 119µs 121µs
```
