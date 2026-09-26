# PROTO-012: HTTP/2 + HTTP/3 Client Transports — Implementation Specification

**Status**: ✅ SPEC — design authority for the transport-version delta; no product code changed by this row
**Version**: 1.0.0
**Date**: 2026-09-26
**Component**: Architecture / transport (stage 4)
**Addresses**: PROTO-012 (board: HTTP/2 + HTTP/3 client transport support)
**Extends**: [specs/PROTOCOLS.md](../PROTOCOLS.md) — this spec IS the delta that lands there (matrix rows §2, stage clauses §1.2/§1.4/§3.5, error row §4.2, in-process pattern §4.6, new §4.7, gate rows §6, demo-plan entry §7). It is not a second protocol document.
**Depends on**: PROTO-010 (evidence, merged `b8134f5d`), PROTO-001 (specs/PROTOCOLS.md, merged `88e14672`)
**Feeds**: PROTO-014 (h2 execution), PROTO-015 (h3 execution), PROTO-017 (tests + demo legs)
**Cross-repo**: bunker board row **BFS-004** (WebDAV surface + per-version capability negotiation) — the server half of the agreement this spec fixes; see §7.

---

## 0. Decisions in one screen

| # | Question | Decision | Basis |
|---|---|---|---|
| D1 | Are h2/h3 new adapters in the matrix? | **No — they are transport variants attaching at stage 4 only.** No new `Protocol` constant, no new `Discoverer`, no new subcommand namespace. Adapter count is unchanged (3 packages today: `pkg/protocol/{grpc,jsonrpc,graphql}`) and `Operation`/`Parameter`/`Executor` stay byte-identical. | `pkg/protocol/registry.go:33-35` rejects a second adapter per protocol (`CodeValidation`); a version produces no new operations; PROTO-010 §1.3/§2 (verdict (a)) |
| D2 | Which adoption class for each? | Matrix rows **15 (h2): ADOPT — no new dependency** (configuration + observability) and **16 (h3): ADOPT — opt-in, one new dependency**. Plus row **17: WebDAV (RFC 4918) as an HTTP application profile** (ADOPT — a profile over rows 15/16, not an adapter). | §1 (before/after delta); measured cost §4 |
| D3 | How does a caller learn the negotiated version? | A **`ProtocolRecorder` fed at the base `http.RoundTripper`** — the version a wire exchange actually used — not `http.Response.Proto` through the wrapper chain, which is **forged today on two paths**. A response with no wire exchange reports `source: cache`/`local` and no version; never a default of `HTTP/1.1`. | §3; measured: PROTO-010 `V2`/`V3`, re-verified at HEAD (cache.go:110-126, ratelimit.go:50-64) |
| D4 | What does a caller see when the server offers only HTTP/1.1? | Under the default policy `auto`: **success + a reported fact** (`observed=HTTP/1.1`, `satisfied=true`). Under a *required* policy (`1.1`/`2`/`3`): a hard, coded failure — `CodeProtocolMismatch` (new constant) with `requested`/`observed`/`reason`. **A downgrade is never silent in either mode.** | §3.4/§3.5 (N4/N5 table) |
| D5 | Does HTTP/2 need a new module? | **No.** The stdlib negotiates it today because `pkg/client.NewClient` leaves `Transport` nil → `http.DefaultTransport` (`ForceAttemptHTTP2: true`); measured HTTP/2.0 on both sides at HEAD; `golang.org/x/net/http2` is already in the CLI build closure (516 packages). | §4.1, M1/M2/M10 |
| D6 | Does HTTP/3, and at what cost? | **Yes — `github.com/quic-go/quic-go` v0.63.0.** In-repo module graph 173 → 178 (**+5**, of which exactly **2** compile: `quic-go`, `qpack`); binary **+306,802 B** default / **+143,360 B** stripped; module cache 3,594,997 B; MIT; 0 reachable vulns; `CGO_ENABLED=0` cross-compiles. | §4.2 |
| D7 | Does the unary/streaming contract change? | **No.** No `Executor` signature change, no new `OperationKind`, no new output format. Bodies stream, deadlines map to `CodeTimeout`, 1 MiB PUTs arrive intact on both transports. | PROTO-010 §7 (`P8`), spec §6.3 |
| D8 | Does anything about WebDAV get decided here? | **Yes, the matrix question PROTO-010 left open**: WebDAV is an HTTP application profile reached through rows 15/16, with **stage 1 = a built-in operation manifest** (the RFC 4918 method set is a closed vocabulary with no discovery document). Its transport half is rows 15/16; its own gate is order 11. | §1 rows 17, §5.3; M12 |

**Design authority**: [`specs/PROTOCOLS.md`](../PROTOCOLS.md). Where this spec and that document disagree, PROTOCOLS.md wins and this file is the defect.

---

## 1. Matrix delta (before / after)

This is the explicit before/after the row asks for. "Before" is HEAD `0765b0c0` with `specs/PROTOCOLS.md` v1.0.0; "after" is that document with this spec's §2/§6/§7/§1.2/§1.4/§3.5/§4.2/§4.6/§4.7 edits applied (applied in this row — see the committed `specs/PROTOCOLS.md` diff).

| Dimension | Before (measured, M11/M15/M16) | After |
|---|---|---|
| `§2` matrix rows | **14** (#1–#14) | **17** (#15, #16, #17 appended; #1–#14 untouched, numbering stable) |
| HTTP-version content in PROTOCOLS.md | **none** — `grep -n "HTTP/1\|HTTP/2\|HTTP/3\|ALPN" specs/PROTOCOLS.md` → 0 matches | `§1.2`, `§1.4`, `§3.5`, new `§4.7`, `§4.6` pattern, rows 15/16 |
| `Protocol` constants | 6 (`openapi, grpc, jsonrpc, graphql, asyncapi, soap`) | **6 — unchanged** (a version is not a family, D1) |
| Adapter implementations | 3 packages (`grpc`, `jsonrpc`, `graphql`); OpenAPI refit still open | **3 — unchanged**; no adapter added or re-registered |
| Registry semantics | `Register` rejects a duplicate `Protocol()` | **unchanged** |
| `§6` gate rows | **9** (orders 0–8) | **12** (orders 9 = h2, 10 = h3, 11 = WebDAV profile) |
| `§7` demo scripts | 10 files in `demo/demos/`; scripts 11–13 pre-declared | derived ordinals **19** (h2), **20** (h3), **21** (WebDAV) by the append-only rule in §6 |
| `§4.2` error codes | 9 codes | **+1 additively**: `CodeProtocolMismatch` |
| `Executor` / `Operation` / `Parameter` / MCP tool names | frozen | **frozen** (no signature or output-format change) |

### 1.1 Why this is a transport variant and not an adapter (AC1)

PROTO-010's verdict (a), restated as the binding rule this spec lands in `§1.2`:

- **One Adapter per Protocol family.** `Registry.Register` returns `errors.CodeValidation "protocol: adapter for %q already registered"` for a second adapter on the same `Protocol` (`pkg/protocol/registry.go:33-35`). Registering "HTTP/2"/"HTTP/3" therefore requires new `Protocol` constants.
- **A version produces zero new operations.** Discovery is identical across versions — `Operation.Source` already carries the transport ground truth (`{"path","method","server"}`). Three version-adapters would each re-emit a duplicate copy of every operation, run discovery three times for one document, and compose credentials/rate limits three times for one endpoint.
- **Stage 4 is the replaceable stage** (`PROTOCOLS.md` §1.2: "stage 4 (executor) is replaceable per protocol"), and stage 3 is explicitly shared. A version belongs to stage 4 by construction.
- **The attachment point already exists as a convention**: a shared `*client.Client` injected by option — `pkg/protocol/jsonrpc/client.go:76,89` and `pkg/protocol/graphql/client.go:122,140` (`WithHTTPClient`). One injected client gives every HTTP-family adapter the same version behaviour with no per-adapter code.
- **A version adds no user surface.** New `Protocol` constants would force new subcommand namespaces (`openapi-cli http2 …`, `openapi-cli http3 …`) under `§5.1`'s "the protocol is a SUBCOMMAND NAMESPACE" rule — a surface no operator asked for. The version is a **flag/config value on existing verbs** (`§3.6`).

### 1.2 Where a version DOES attach

Two attachment points exist; this spec picks the first and forbids the second:

1. **A new `ClientOption` `WithTransport(rt http.RoundTripper)`** — additive, participates in the same option pipeline as `WithRateLimiter` (`pkg/client/ratelimit.go:85`) and `WithCache` (`pkg/client/cache.go:142`). This is the attachment point PROTO-014 adds.
2. `Client.SetHTTPClient` (`pkg/client/client.go:320-322`) — **forbidden for version selection.** It replaces the whole `*http.Client` and discards the wrapper chain the options installed: measured 1a/1b/1c, `WithCache` + `WithRateLimiter` first and `SetHTTPClient` second takes wire hits from 1 → 2 and turns `*client.CacheRoundTripper` back into `*http.Transport` (PROTO-010 §4.2, defect row DF-042).

### 1.3 The three matrix rows, verbatim (appended to `§2`)

| # | Protocol | Verdict | Order | Reason |
|---|---|---|---|---|
| 15 | **HTTP/2 client transport (h2)** | ADOPT — **no new dependency** (configuration + observability, not protocol code) | 9 | Already negotiated today, by accident: `pkg/client.NewClient` builds `&http.Client{Timeout: 30 * time.Second}` with a nil `Transport` (`pkg/client/client.go:101-103`), so requests land on `http.DefaultTransport`, whose `ForceAttemptHTTP2` is `true` — measured HTTP/2.0 on client and server at HEAD, zero modules added. `golang.org/x/net/http2` is already in the CLI build closure (516 packages), so even explicit h2 control adds nothing. The row exists because the negotiation is *accidental*: a hand-built `&http.Transport{}` drops to HTTP/1.1 against a server that offers h2 (measured), and 0 non-test `http.Transport` constructions exist in the tree today — the first one anyone adds silently removes h2. Cleartext is HTTP/1.1 regardless (no h2c). Gate: order 9; the version is read from the wire, never from configuration. |
| 16 | **HTTP/3 client transport (QUIC/h3)** | ADOPT — opt-in, **one new dependency** (`github.com/quic-go/quic-go` v0.63.0) | 10 | No stdlib HTTP/3 and no importable `x/net` HTTP/3 (`golang.org/x/net@v0.58.0`'s `http3/` is a `//go:linkname` shim to `net/http_test` with three unexported functions and no client `RoundTripper`). `http3.Transport` **is** an `http.RoundTripper` and was measured driving GET/PUT/PROPFIND/DELETE unchanged through the unmodified `pkg/client`. Measured cost at PROTO-012: module graph 173 → 178 (+5, of which exactly 2 compile — `quic-go`, `qpack`), binary +306,802 B default / +143,360 B stripped, module cache 3,594,997 B, MIT, `govulncheck` 0 reachable, `CGO_ENABLED=0` cross-compiles. Opt-in with h2 retained: **version selection is transport selection** and neither transport falls back to the other (measured both directions), so any fallback is explicit client code that reports itself (§4.7 N4/N5). |
| 17 | **WebDAV (RFC 4918) as an HTTP application profile** | ADOPT — as an **application profile**, not an adapter: it reuses rows 15/16 and the HTTP-family stage-4 path | 11 | Muster is the named WebDAV **consumer** of bunker's surface (BFS-004), and the repo has no WebDAV content at all: `grep -ril "webdav\|propfind"` → 0 in `specs/`, 0 in `.memory-bank/`, 0 in any `.go` file, 0 in any markdown outside the two PROTO-010/011 investigation docs. The RFC 4918 method set is a closed vocabulary with no discovery document, so stage 1 for this profile is a **built-in operation manifest** (decided here — inventing a discovery document would be a second protocol engine; a hand-written per-endpoint manifest remains the fallback for non-RFC extensions). Measured gap it must close: `openapi-cli request PROPFIND <url> -b <propfind>` reaches the wire with **bodyLen=0** because `internal/builtin/request.go:144-146` attaches a body only for `POST`/`PUT`/`PATCH` (verified at HEAD), while `PUT` sends its 7 bytes — LOCK/PROPFIND bodies are silently dropped today. Gate: order 11. |

**Rows NOT added.** No adapter rows and no `Protocol` constants for HTTP versions (D1). HTTP/1.1 gets no row: it is not something Muster adopts, it is what remains when nothing else negotiates — the behaviour the negotiation contract must *report* (§3).

**Matrix maintenance rule compliance** (`PROTOCOLS.md` §2): each row above carries (a) a written verdict + reason, (b) a board row with acceptance criteria (PROTO-014, PROTO-015, and the WebDAV profile row), (c) a gate definition per §6 (§5 of this spec).

---

## 2. Stage-by-stage contract (what changes at each stage)

| Stage | Change | Detail |
|---|---|---|
| **1 Discover / Ingest** | **none** for HTTP versions; **one new input kind** for the WebDAV profile | A version has no discovery input. Row 17's profile adds a built-in operation manifest as a stage-1 source (no document, no network). |
| **2 Operation model** | **none** | `protocol.Operation`/`Parameter`/`RequestBodyProperty` unchanged. **Decided: the version is NOT a field on `Operation` and NOT a key in `Operation.Source`.** A per-operation version would duplicate a client-wide transport policy, break "one client, one call path" (§1.2), and let stage-3 consumers read a value they must not interpret (§3.2's ground-truth rule). |
| **3a Command generation** | **none** | No new subcommand namespace (§1.1); version policy is a flag on existing verbs. |
| **3b MCP tool generation** | **none** | Tool names and JSON input schemas are byte-identical (§5.3 frozen surface). |
| **4 Transport executor** | **the only stage that changes** | (a) new `ClientOption WithTransport(rt http.RoundTripper)`; (b) the default transport becomes explicit (`&http.Transport{ForceAttemptHTTP2: true, …}` derived from `http.DefaultTransport`) instead of relying on the nil-transport accident; (c) `ProtocolRecorder` installed at the base transport (§3.2); (d) `request.http_version` policy + `--http-version` flag; (e) the h3 transport (`http3.Transport`) and the opt-in h3→h2 fallback wrapper; (f) mismatch reporting per §3.4/§3.5. |
| **Cross-cutting §4** | one new code, one new section, one new test pattern | `§4.2` gains `CodeProtocolMismatch`; new `§4.7` (negotiation & observability); `§4.6` gains the h3 in-process pattern. |

### 2.1 Exact clauses landed in PROTOCOLS.md

Applied in this commit; reproduced here so the spec is self-contained.

1. **`§1.2` (after the four-stage diagram, line ~37)** — new paragraph:

   > **A protocol VERSION is a stage-4 property, not a protocol family.** HTTP/1.1, HTTP/2 and HTTP/3 are versions of one protocol: they produce no new operations, so they add no `Discoverer`, no `Protocol` constant and no adapter. A version is selected by the transport installed on the shared `pkg/client.Client` (§4.7), which is also why version support never creates a second client or a second call path.

2. **`§1.4` Transport row** — appended sentence:

   > HTTP-family adapters inherit the HTTP version from the shared `pkg/client.Client`; changing the version MUST NOT create a second client, a second call path, or a new adapter (§4.7). The version is transport selection, not discovery.

3. **`§3.5` Transport executor** — new bullet:

   > **Version selection is the transport's, never the adapter's.** An HTTP-family adapter MUST NOT negotiate a version, set ALPN preferences, or retry across versions on its own; it executes through the shared `pkg/client.Client` whose `http.RoundTripper` carries the policy and reports the observed version per §4.7.

4. **`§4.2` error table** — new row:

   > | A required HTTP transport version could not be used (client-side, after the wire attempt) | `CodeProtocolMismatch` (new) | details `requested`, `observed`, `reason`; never used for a version the policy did not require |

5. **`§4.6`** — new bullet under the HTTP-family pattern:

   > HTTP/3: an in-process `http3.Server` on a **UDP loopback** socket plus an `http3.Transport` is the h3 analogue of `httptest`. QUIC mandates TLS 1.3, so the fixture needs a real (self-signed) certificate — a bare listener cannot work — and the negative cells (an h2 transport against an h3-only endpoint, and the reverse) are part of the pattern, because nothing in either transport falls back.

6. **`§4.7`** — new section, reproduced in full in §3.6 below.

7. **`§6`** — three gate rows, §5 below. **`§7`** — demo-plan entry, §6 below.

---

## 3. Negotiation & observability contract (`§4.7` in PROTOCOLS.md)

### 3.1 The problem, measured

`http.Response.Proto` is the only surface that could carry the negotiated version, and **on two paths it is a lie** — the design must not depend on it.

| Site | What it does | Measured |
|---|---|---|
| `pkg/client/cache.go:110-126` (`buildCachedResponse`, called at `:66` on a miss and `:94` on a hit) | rebuilds the response from cached bytes, setting `Proto: req.Proto`, `ProtoMajor: req.ProtoMajor` and a fresh `Header` holding only `Content-Type` | PROTO-010 `V2`/`V3`: 2xx GET → `resp.Proto="HTTP/1.1"` and `X-Server-Proto`/`X-Other` absent **while the wire was HTTP/2.0**, on miss *and* hit; POST unaffected. Live shape: `internal/builtin/request.go:158,164` enables exactly `WithRateLimiter`+`WithCache`. |
| `pkg/client/ratelimit.go:50-64` | synthesizes a local `429` without contacting anything, also stamping `Proto: req.Proto` | **re-measured at PROTO-012**: `rate=1/burst=1`, 2nd call → `resp.Proto="HTTP/1.1"`, `ProtoMajor=1`, `Header={}`, body `"rate limit exceeded\n"`, no wire hit. Same at `rate=0/burst=0` (`pkg/rate/tokenbucket.go:18-20` clamps `burst<=0` to `1`). |

And `req.Proto` is `"HTTP/1.1"` for an outgoing request **by construction** — measured at PROTO-012 with a recording `RoundTripper` below `http.Client`: `req.Proto="HTTP/1.1"`, `req.ProtoMajor=1`. So both synthetic sites fabricate `HTTP/1.1` regardless of what the network did.

Two further measured facts close the door on "just read the response":

- **Nothing prints it.** `resp.Proto` appears in product code only at those two synthesizing sites (`M13`); the CLI's verbose path (`internal/builtin/request.go:195-213`) prints `Request:`, `Status:`, the masked credential and the response headers — no version line.
- **The forgery is verb-dependent.** Non-GET verbs pass through the cache untouched (`cache.go:56-58`), so the same client tells the truth for POST and lies for GET.

**Decided:** DF-041 (response cache forging `Proto`/dropping headers) remains required for response-object and header fidelity, and is **not** a dependency of this contract — no rule below may be weakened to rely on it. PROTO-012 additionally found that DF-041's stated scope (the cache) does not cover the rate-limiter's synthetic 429, which fabricates the same value by the same mechanism; recommendation in §8 (open decision O5).

### 3.2 N1 — Truth source and attachment

> **N1.** The negotiated HTTP version is the version the response was actually carried on, recorded from the response returned by the **base `http.RoundTripper`** — the one that performs the exchange — at the moment it returns, before any wrapper may rebuild the response.

Concretely, the option pipeline for an HTTP-family call is ordered `WithTransport` (base) → `WithProtocolRecorder` → `WithRateLimiter` → `WithCache` (outermost, nearest the caller). The recorder sits **below** the wrappers, so it observes every exchange that reaches the network and cannot be overwritten by one that does not. Product surfaces read the recorder, never `resp.Proto` from the wrapper chain.

### 3.3 N2/N3 — What is reported

> **N3.** Every HTTP-family execution reports this record:

| Field | Values | Meaning |
|---|---|---|
| `observed` | `HTTP/1.1` \| `HTTP/2.0` \| `HTTP/3.0` \| `""` | the wire version; **empty** when no exchange took place |
| `source` | `wire` \| `cache` \| `local` | where the response came from |
| `requested` | `auto` \| `1.1` \| `2` \| `3` | the policy in force (§3.4) |
| `satisfied` | bool | the policy was met (`auto` is satisfied by any `observed` value from the wire) |
| `fallback` | object or absent | `{from, to, reason}` — present only when a preferred/required version was attempted and another was used |

Rules:

- **`observed` is never guessed.** A cache-served response reports `source: cache` and the version stored with the cache entry, or `""` when the entry carries none (an entry written before this change) — **never a default of `HTTP/1.1`**. An entry produced by a local refusal reports `source: local` with `observed: ""`.
- **A locally refused request has no negotiated version.** The existing 429 path (rate limiter) and any future synthesizing wrapper MUST NOT stamp `req.Proto` as an observed version.
- **The record is per exchange, and lookups are explicit.** `ProtocolRecorder` keeps the last exchange and a monotonically increasing sequence with a `For(method, url)` lookup; CLI/MCP invocations issue one request per client instance (`internal/builtin/request.go` builds the client per request), so "the last wire exchange of this client" is the correct value on the product surfaces.

> **N7.** Surfaces: **library** — the recorder; **CLI** — a `HTTP: <version>` diagnostic line on stderr next to the existing `Request:`/`Status:` lines (stderr, never stdout: payload fidelity is frozen by §4.4/§5.3), a `--http-version` flag for the policy, and the coded error below; **MCP** — the tool result payload stays the decoded body (§4.4), and a required-version failure is an `AppError` carrying the record, so an agent never receives a silent downgrade.

### 3.4 N4/N5 — Policy, and what a caller sees per server capability

> **N4.** `request.http_version` (config; `--http-version` flag overrides) takes `auto` (**default** — today's behaviour), `1.1`, `2`, `3`. `auto` never fails on version. `1.1`/`2` REQUIRE the named version. `3` prefers HTTP/3 and MAY fall back to HTTP/2 — always reported.

> **N5. A version that cannot be used is a reported fact, never a silent downgrade.**

| Server offers | Policy | Outcome |
|---|---|---|
| `http/1.1` only | `auto` (default) | **succeeds**; `observed=HTTP/1.1`, `satisfied=true`; the fact is reported on the diagnostic surface and available from the recorder. This is what "older clients must work" means for the consumer side. |
| `http/1.1` only | `2` or `3` | **FAILS**: `CodeProtocolMismatch`, details `requested`, `observed`, `reason="server-offered-http/1.1"` |
| h2 over TLS | `auto` / `2` | succeeds; `observed=HTTP/2.0` |
| h3 reachable | `3` | succeeds; `observed=HTTP/3.0` |
| h3 unreachable (UDP dropped, no QUIC, peer TCP-only) | `3` | **succeeds over h2 with `fallback{from:HTTP/3.0, to:HTTP/2.0, reason:<connection-layer class>}`** — the fallback is client code, and its occurrence is the report |
| plaintext `http://` | any | `observed=HTTP/1.1` by construction (no h2c in the stdlib; QUIC requires TLS 1.3); policy `2`/`3` FAILS with `reason="plaintext-endpoint"` |

Classification rules that make the report trustworthy:

- A **QUIC connection-layer failure** (dial refused / handshake timeout / `CRYPTO_ERROR`) is a fallback reason (measured: `X-1` `dial tcp …: connection refused`, `X-2` `timeout: no recent network activity`). An **HTTP status** is never a fallback reason.
- A fallback MUST NOT be reported as success-with-no-signal, and an unsatisfied required policy MUST NOT be reported as a fallback: `fallback` means "another version carried the request", `CodeProtocolMismatch` means "no permitted version could".

### 3.5 N6/N8 — Negation of automatic cross-version behaviour

> **N6.** No version negotiation exists inside a transport: an `http.Transport` will not fall back to QUIC and an `http3.Transport` will not fall back to TCP (measured both directions), and **nothing consumes `Alt-Svc`** — neither the stdlib (`grep -rn "Alt-Svc\|AltSvc" $GOROOT/src/net/http/*.go` → 0 hits) nor quic-go's client (it only sets it server-side). H3 discovery is therefore **explicit configuration**, never automatic.

> **N8.** `CodeProtocolMismatch` is a new additive `pkg/errors` constant used only for "a required HTTP transport version could not be used", mapped at the transport boundary like every other adapter error (§4.2). It is not used for a version the policy did not require.

### 3.6 The section as it lands in PROTOCOLS.md

`§4.7 HTTP transport version negotiation & observability (HTTP/1.1 / HTTP/2 / HTTP/3)` carries N1–N8 with the tables above, scoped to HTTP-family adapters (matrix rows 1, 4, 5, 8, 15, 16, 17); non-HTTP transports are unaffected.

---

## 4. Dependencies and measured cost (AC3)

### 4.1 HTTP/2 — no new module

| Fact | Measurement |
|---|---|
| h2 negotiated with zero new modules | muster client, nil `Transport`, in-process `httptest` TLS server with `EnableHTTP2`: client `resp.Proto=HTTP/2.0`, server `r.Proto=HTTP/2.0` (**M1**, re-measured at HEAD 2026-09-26) |
| the negotiation is accidental | same server, hand-built `&http.Transport{TLSClientConfig: …}` (no `ForceAttemptHTTP2`): client `HTTP/1.1`, server `HTTP/1.1` (**M2**) |
| platform default | `http.DefaultTransport.ForceAttemptHTTP2 = true` (PROTO-010 `B`) |
| 0 non-test `http.Transport` constructions | `grep -rn "http.Transport" --include=*.go . \| grep -v _test \| wc -l` → **0** (**M14**) |
| even the explicit h2 package is already linked | `golang.org/x/net/http2` ∈ `go list -deps ./cmd/openapi-cli` (516-package closure, **M10**); `go mod why golang.org/x/net/http2` → `pkg/protocol/grpc → google.golang.org/grpc → grpc/internal/transport → golang.org/x/net/http2` |
| plaintext is HTTP/1.1-only | cleartext request to an h2c-capable server → `HTTP/1.1` both sides (PROTO-010 `3a`) |

**Decision:** the default transport becomes explicit (`&http.Transport{ForceAttemptHTTP2: true, …}` derived from `http.DefaultTransport`) so h2 stops being an accident, and **the version policy gains observability** — because the measured failure mode is silent (M2), and no surface prints the version today (M13).

### 4.2 HTTP/3 — `quic-go` v0.63.0, measured

PROTO-010 measured v0.59.1 and v0.63.0 in its probe harness; PROTO-012 **re-measured the transferable numbers against this repository's own module graph** and pins the release.

| Measurement | Value (PROTO-012, in-repo) | PROTO-010 harness (context differs) |
|---|---|---|
| module graph, `go list -m all` | **173 → 178 (+5)** — added: `quic-go`, `qpack`, `go-ossfuzz-seeds`, `gcassert`, `go.uber.org/mock`; unchanged with an `http3`-importing package present (**M5**) | 102 → 112 (**+10**) for v0.63.0; 102 → 114 (**+12**) for v0.59.1 |
| of which compiled into the binary | **exactly 2** — `github.com/quic-go/quic-go`, `github.com/quic-go/qpack` (`go list -deps -f '{{if .Module}}{{.Module.Path}}{{end}}'` on a package importing `http3`) (**M6**) | same 2 runtime, 8 test/tool |
| binary weight | control 5,229,212 B → h3 5,536,014 B = **+306,802 B** default; stripped 3,588,361 B → 3,731,721 B = **+143,360 B** (**M7**) | +312,019 B default / +143,360 B stripped (the stripped figures agree exactly; the default figure differs by the control program's own content) |
| module cache bytes | `quic-go@v0.63.0` 3,389,018 B + `qpack@v0.6.0` 205,979 B = **3,594,997 B** (**M8**) | 3.6 MB total — agrees |
| existing `x/*` versions moved | **none** — `x/net v0.58.0`, `x/crypto v0.57.0`, `x/sys v0.48.0`, `x/sync v0.23.0` identical before/after (**M9**) | same, verified |
| toolchain floor | quic-go `go 1.26.0` ≤ repo `go 1.26.6` (**P7**) | same |
| h3 round trip | via PROTO-010 `H3-1..H3-4`: GET/PUT/PROPFIND/DELETE all `HTTP/3.0` client + server; 1 MiB PUT `received=1048576` (**P3**) | — |
| licence | MIT (**P7**) | MIT |
| vulnerability scan | PROTO-010: **0 reachable**, 1 module-level advisory in a package the h3 path does not call (**P7**) | — |
| cross-compile | PROTO-010: `CGO_ENABLED=0 GOOS=linux GOARCH=arm64` rc=0 (12,168,935 B, pure Go) | — |
| build/CI wall time | **NOT MEASURED** — carried to §8 O1 | not measured |

**Why the two graph deltas differ (honest reconciliation).** PROTO-010 counted in a scratch module whose *dependency* graph was pruned: modules Muster already carries for its own tests/tools (testify, `x/tools`, `x/mod`, `x/term`, `go-spew`, `go-difflib`) were absent from its baseline, so quic-go's test/tool requirements looked like additions (+10). In Muster's own main-module graph they are already present, and the same requirement set adds only **+5**. The transferable fact is the pair: **a five-module graph increase of which exactly two compile**. PROTO-015 re-measures in the real tree after the `require` lands (expected 178, §8 O6).

**Decision (pin):** `github.com/quic-go/quic-go` **v0.63.0**. Reasons: the only measured option that adds fewer graph entries than v0.59.1 (+10 vs +12 in the common harness; +5 in-repo), it is the current release, its `go 1.26.0` floor is already satisfied by the repo's `go 1.26.6`, it perturbs **no** existing dependency version (M9), it is MIT, and its measured reachable-vulnerability count is 0. PROTO-010 open question 3 is thereby **closed**, not carried.

### 4.3 What does not change

No new `protocol.Protocol` constant; no `Discoverer`; no change to `Operation`/`Parameter`/`RequestBodyProperty`; `Executor.Execute` stays byte-identical; no new output format (§4.4); no adapter writing to stdout. Request semantics are the transport's: measured bodies stream incrementally (first chunk at 2–3 ms while the response took 364 ms), 1 MiB PUTs arrive intact on both transports, and a context deadline maps to `CodeTimeout` (PROTO-010 §7, `P8`).

---

## 5. Gates (AC4) — the rows landed in `§6`

Must be runnable and network-free per §4.6; green gate = the acceptance criterion for the corresponding board row.

| Order | Protocol (board row) | Gate definition |
|---|---|---|
| 9 | HTTP/2 client transport (PROTO-014) | In-process `httptest` TLS server, `EnableHTTP2 = true`, ALPN offering `h2`, loopback only. (a) the version recorded per §4.7 N1 — **not** read from a wrapper-built response — equals `HTTP/2.0` on both sides for GET and for `PROPFIND`; (b) a 1 MiB PUT arrives intact (server-observed length 1048576); (c) **reporting control**: a server offering only `http/1.1` still serves the request and the recorded version is `HTTP/1.1` with `satisfied: true` under `auto` — the version comes from the wire, never from configuration; (d) **negative cell**: a hand-built `&http.Transport{}` without `ForceAttemptHTTP2` records `HTTP/1.1`, so the assertion can fail (measured M2 — this is the anti-gaming cell PROTO-017 requires); (e) **policy cell**: `--http-version 2` against the h1.1-only server fails with `CodeProtocolMismatch` carrying `requested`/`observed`/`reason`; (f) **cache cell**: with `WithCache` enabled, the recorded version for a 2xx GET is still the wire version (today that path's `resp.Proto` is `HTTP/1.1` — DF-041 — so the cell proves the contract does not depend on it). |
| 10 | HTTP/3 client transport (PROTO-015) | In-process `http3.Server` on a **UDP loopback** socket with an in-process self-signed certificate (QUIC mandates TLS 1.3 — a bare listener cannot work). (a) `HTTP/3.0` recorded on both sides for GET/PUT/PROPFIND/DELETE; (b) context deadline → `CodeTimeout`; (c) **negative cells (measured)**: an h2 transport against an h3-only endpoint fails (dial refused) and an h3 transport against a TCP-only endpoint fails (`timeout: no recent network activity`) — the proof that no cross-version fallback exists inside either transport; (d) **fallback cell**: policy `3` with QUIC unreachable completes over h2 **and reports** `fallback{from,to,reason}` classified as a connection-layer failure, never as a silent success; (e) a TLS-1.2-capped config fails with `CRYPTO_ERROR 0x150` rather than hanging; (f) a plaintext URL can never satisfy policy `3`. |
| 11 | WebDAV application profile (matrix row 17) | In-process `httptest` WebDAV fixture. A `PROPFIND` with a `<propfind>` body reaches the wire with a **non-zero** body (today measured `bodyLen=0`, `internal/builtin/request.go:144-146`), likewise a `LOCK` body; a `Depth` header is sent when requested; operations are produced from the built-in manifest with stable IDs; an extension refusal from the peer surfaces with the peer's own vocabulary preserved in the error details (§7). |

**Gate wiring note (carried into PROTO-015's brief, not a hidden cost):** the guard's test command is `go test -short -count=1` over the filtered package list with `test_mode: full` (`.gitreins/config.yaml`), and per `AGENTS.md` **a `go.mod` change forces the full suite** as a safety net — so the h3 row pays a full-suite run on its committing tick, on top of any h2 change-triggered run.

---

## 6. Demo-plan delta (AC4) — the entry landed in `§7`

`./demo/run-all.sh` is the real end-to-end gate; it globs `demo/demos/[0-9]*.sh`, currently **10** scripts (01–10), and `§7` pre-declares 11 (gRPC unary), 12 (gRPC streaming), 13 (JSON-RPC) for orders 1–3.

- **Ordinal rule (decided):** a gate's demo script takes ordinal `11 + (order − 1)` on the append-only queue — scripts are **never renumbered** once landed, and the ordinal is re-derived at landing time from `ls demo/demos/`. Orders 4–8 own 14–18; therefore **this transport's scripts are `19-http2-transport.sh` (order 9), `20-http3-transport.sh` (order 10)** and row 17's is `21-webdav-surface.sh` (order 11). Two scripts, not one: order 9's gate must be demonstrable at the moment h2 lands, and one combined script would make it depend on order 10's code.
- **Content — `19-http2-transport.sh`:** start a loopback TLS fixture offering `h2`; run one GET and one PROPFIND asserting the *reported* version is `HTTP/2.0`; then against an `http/1.1`-only fixture assert the request succeeds and the report says `HTTP/1.1` (the reporting leg, not a failure leg).
- **Content — `20-http3-transport.sh`:** the same fixture over UDP/h3, asserting `HTTP/3.0`; plus the reported-fallback leg.
- **Fixture ownership (decided):** each demo script builds and starts **its own** Go fixture from `demo/services/transport-fixture/` on a loopback port with a self-signed cert, and tears it down on exit. `run-all.sh` starts services from an **explicit list** (`demo/run-all.sh:43-47`) and seeds per `server.py`; a Python h2/h3 service is not available cheaply (the stdlib server is HTTP/1.1-only), so owning the fixture inside the script keeps `run-all.sh` unchanged — only the `demos/*.sh` glob picks the new legs up.
- **Later**: `run-all.sh` output then shows the transport matrix growing check by check, as §7 intends.

---

## 7. Cross-repo contract: bunker BFS-004 (AC5)

**The named dependency.** The server half of this agreement is bunker board row **BFS-004** — *"SPEC: the WebDAV surface — the standard protocol we implement faithfully PLUS our extension layer, with per-version capability negotiation so an older HTTP/1.1 client still works"* (bunker `.coding-hermes/board/tasks.jsonl`, source PRD). Its own text names Muster as a consumer and cites *"PRD row set PROTO-012/014/015 depends on this surface agreeing to serve h2 and h3 clients."* The dependency is **bidirectional on the wire facts below** and must not be assumed by either side.

**The consumer relationship, stated precisely.** The transport is what makes the consumption possible: with `http.DefaultTransport` the client speaks h2 over TLS; with `http3.Transport` installed on the *same* `pkg/client.Client` it speaks h3; over cleartext it is HTTP/1.1. So "Muster supports every version bunkerd's WebDAV surface can negotiate" = **one `*client.Client` whose transport is chosen per version** — not a new adapter, not a second call path (§1).

**The wire shape both specs must agree on** (all measured):

| # | Wire fact | Measurement | Consequence for the agreement |
|---|---|---|---|
| W1 | h2 is negotiated in-band by ALPN; the client reports what it observed | server ALPN offering `h2` → `HTTP/2.0` both sides (M1); ALPN restricted to `http/1.1` → `HTTP/1.1` both sides (`C6`) | Bunkerd's TLS listener MUST advertise `h2` in ALPN, or a correct client truthfully reports `HTTP/1.1` — the client cannot invent h2 |
| W2 | h3 requires TLS 1.3 and is UDP-only | `3c` → `CRYPTO_ERROR 0x150` with a TLS-1.2 cap; `X-1` dial refused | The h3 service is a distinct UDP endpoint; UDP-dropping middleboxes remove h3 entirely — hence policy `3`'s reported fallback |
| W3 | plaintext surfaces are HTTP/1.1-only | `3a` → `HTTP/1.1` both sides | "Older clients keep working" on a plaintext surface costs nothing; it also means an h2/h3 requirement cannot be met there (reported, §3.4) |
| W4 | `Alt-Svc` is the only h3 advertisement channel and **nothing consumes it** | `grep -rn "Alt-Svc\|AltSvc" $GOROOT/src/net/http/*.go` → 0 hits; quic-go sets it server-side only | Bunkerd MAY advertise Alt-Svc (stock-client behaviour, harmless), but **Muster's h3 support does not depend on it**: h3 is explicit configuration. Muster MUST NOT hardcode an Alt-Svc port, and bunker need not rely on auto-discovery for Muster |
| W5 | capability negotiation is bunker's vocabulary to define | bunker BFS-004: per-version capability negotiation, with a missing extension reported as a structured `capability_unavailable` naming what is absent — never silently different behaviour | **Muster's half:** a structured refusal is surfaced as an `AppError` mapped by **HTTP status first** (§4.2: 4xx → `CodeBadRequest`/`CodeNotFound`/`CodeUnauthorized`, 5xx → `CodeGeneric`), with bunker's own vocabulary preserved verbatim in the details (`capability`, `reason`) — never flattened into prose and never dropped. The distinction "not supported here" vs "not in this version yet" MUST survive into the details |
| W6 | the same request body must reach the wire at every version | 1 MiB PUT intact over h2 and h3 (`P3`; PROTO-010 §7) | Version choice MUST NOT change request/response semantics — the client contract of §4 (unary/streaming) is version-independent |

**Open cross-repo item (named, not invented):** whether BFS-004 will advertise h3 via `Alt-Svc` and on which port is unknowable from this repo (PROTO-010 §10.4). Because of W4 the answer does not block Muster: the operator selects h3 per profile/config. BFS-004 should still state its advertised h3 endpoint so an operator has one place to read it.

---

## 8. Open decisions (each with a recommendation and the reason)

Nothing in §0–§7 is undecided; these are the residual questions, carried explicitly rather than silently assumed.

| # | Open decision | Recommendation | Reason |
|---|---|---|---|
| O1 | quic-go's **build- and CI-wall-time** cost is unmeasured (only graph, cache bytes and binary bytes are) | PROTO-015 measures `go build ./...` wall time and the guard's full-suite run *before* declaring the pin final | A `go.mod` change forces the full suite (AGENTS.md), so the cost is paid on the committing tick whether or not it is measured — measure, don't discover |
| O2 | **WAN behaviour** is unmeasured; every number here is loopback | Do not claim h3 is faster anywhere in docs or demo text; if a performance claim is ever wanted, measure on a lossy link first | Loopback with no loss is the case where QUIC's loss resilience helps least; h3's honest value here is connection establishment, not throughput |
| O3 | Which quic-go release the maintainers keep supported | **Closed by this spec**: pin v0.63.0 (§4.2) | Fewer graph entries than v0.59.1 in both harnesses, current release, toolchain floor satisfied, no existing dependency perturbed |
| O4 | Recorder concurrency shape — per-client last-exchange + `For(method,url)` vs a context-carried per-request value | Per-client recorder **for now**; revisit only if a long-lived client multiplexes versions concurrently | The CLI/MCP build one client per request (`internal/builtin/request.go`), so per-client is sufficient today; a context-carried shape would touch every HTTP-family call site for no current gain |
| O5 | The rate limiter's synthetic `429` fabricates `Proto="HTTP/1.1"` (measured) and DF-041's scope covers only the cache | **Extend DF-041's scope** to the synthetic-response path rather than filing a sibling row | Same defect class, same mechanism (`Proto: req.Proto`), same falsified signal — one fix, one test ("no synthesized response may claim a wire version") |
| O6 | The in-repo absolute graph figure after the `require` lands | PROTO-015 re-measures `go list -m all` in the real worktree after adding the dependency; expected **178** (173 + 5) | PROTO-012 measured on a module-graph-faithful copy of `go.mod`/`go.sum`; the real tree is the authority |
| O7 | Which WebDAV operations get typed commands vs the generic `request` path | Typed commands generated from the built-in manifest for the RFC 4918 method set; the generic verb remains for non-RFC extensions | The standard set is closed and worth typing; extensions are per-peer and belong to the generic path — the row that implements row 17 owns the manifest detail |

**Not open, decided here:** the pin (§4.2 O3), the fixture-ownership and demo ordinals (§6), the truth source (§3.2), the policy default (`auto`, §3.4), the WebDAV matrix verdict and its stage-1 shape (§1.3/§2), and the fallback semantics (§3.4).

---

## 9. Out of scope (AC6)

- **No product code changes.** This row's deliverable is documentation: this file plus the `specs/PROTOCOLS.md` delta it specifies. No Go file, no `go.mod`/`go.sum` change, no test, no demo script.
- No board, `.coding-hermes`, or `.gitreins` state touched; no new board rows filed (DF-041's scope recommendation in O5 is a proposal to the board, not a filing).
- No implementation of `WithTransport`, the recorder, the policy, the h3 transport, or the fallback wrapper — those are PROTO-014/015.
- No binary renaming (§5.1), no new output format (§4.4), no changes to the frozen call chain (§1.5).

---

## Appendix A — Measurement log (PROTO-012, re-measured at HEAD `0765b0c0`)

Host: `Linux 7.0.0-31-generic x86_64`, `go1.26.6 linux/amd64`, 2026-09-26. Probes were throwaway programs in a scratch module outside the worktree (`replace github.com/wojons/muster => …/muster-PROTO-012`); every server was in-process on loopback, per §4.6. The worktree stayed clean (`git status --short` empty before the doc commit).

| ID | Fact | Evidence |
|---|---|---|
| M1 | h2 today, no new module | client `resp.Proto=HTTP/2.0` + server `r.Proto=HTTP/2.0`, server ALPN `NextProtos=[h2]`, `EnableHTTP2=true` |
| M2 | h2 silently lost by a hand-built transport | same server, `&http.Transport{TLSClientConfig:…}` → `HTTP/1.1` client and server |
| M3 | outgoing `req.Proto` is `"HTTP/1.1"` | recording `RoundTripper` below `http.Client`: `req.Proto="HTTP/1.1" req.ProtoMajor=1` |
| M4 | the rate limiter fabricates a version with no exchange | `rate=1/burst=1` call#2 and `rate=0/burst=0` call#2 → `resp.Proto="HTTP/1.1"`, `ProtoMajor=1`, `Header={}`, `429`, body `"rate limit exceeded\n"`, no wire hit (`pkg/rate/tokenbucket.go:18-20` clamps `burst<=0`→1) |
| M5 | in-repo module graph delta | `go list -m all`: 173 → 178 with `quic-go v0.63.0` required; added = `quic-go`, `qpack`, `go-ossfuzz-seeds`, `gcassert`, `go.uber.org/mock` |
| M6 | compiled modules in an h3 binary | `go list -deps -f '{{if .Module}}{{.Module.Path}}{{end}}'` on an `http3`-importing package → exactly `github.com/quic-go/quic-go`, `github.com/quic-go/qpack` |
| M7 | binary weight | 5,229,212 → 5,536,014 B (+306,802) default; 3,588,361 → 3,731,721 B (+143,360) with `-ldflags="-s -w"` (same toolchain, same host) |
| M8 | module cache bytes | `du -sb`: `quic-go@v0.63.0` 3,389,018 B, `qpack@v0.6.0` 205,979 B |
| M9 | no existing dependency moved | `go list -m`: `x/net v0.58.0`, `x/crypto v0.57.0`, `x/sys v0.48.0`, `x/sync v0.23.0` identical before/after the `require` |
| M10 | explicit h2 package already linked | `golang.org/x/net/http2` ∈ `go list -deps ./cmd/openapi-cli` (516 packages); `go mod why golang.org/x/net/http2` → `pkg/protocol/grpc → grpc → internal/transport` |
| M11 | PROTOCOLS.md has no version dimension | §2 = 14 rows; `grep -n "HTTP/1\|HTTP/2\|HTTP/3\|ALPN"` → 0; §6 = orders 0–8; `demo/demos/` = 10 files |
| M12 | no WebDAV content in the tree | `grep -ril "webdav\|propfind"`: `specs/` 0, `.memory-bank/` 0, `*.go` 0, `*.md` outside `docs/investigation/` 0 |
| M13 | the version is unobservable today | `resp.Proto` in product code only at `pkg/client/cache.go:119` and `pkg/client/ratelimit.go:56` (both `Proto: req.Proto`); no `resp.Proto` in `internal/builtin/request.go`'s verbose path (`:195-213`) |
| M14 | no hand-built transport in product code | `grep -rn "http.Transport" --include=*.go . \| grep -v _test \| wc -l` → 0 |
| M15 | one adapter per protocol is enforced | `pkg/protocol/registry.go:33-35` (`CodeValidation`), `:45-53` (`CodeNotFound`) |
| M16 | the seam's own wiring is still open | `ProtocolOpenAPI` has no non-test referent; `protocol.NewRegistry()` has no non-test caller (grep, both empty) |

**Quoted from PROTO-010** (not re-measured here; basis in `docs/investigation/PROTO-010-http2-http3-client.md`): `P1` cleartext has no h2c, `P2` QUIC mandates TLS 1.3 (`CRYPTO_ERROR 0x150`), `P3` h3 round trips GET/PUT/PROPFIND/DELETE + 1 MiB PUT intact, `P4` no cross-version fallback (`X-1`/`X-2`), `P5` no Alt-Svc consumer, `P6` harness graph deltas (102→112 / 102→114), `P7` quic-go toolchain floor/licence/vuln scan/cross-compile, `P8` streaming + deadline behaviour unchanged, `P9` CLI WebDAV body gap (`bodyLen=0` vs `bodyLen=7`), `P10` cache forgery (`cache.go:110-126`, verified at HEAD).

## Appendix B — Apply checklist for the PROTOCOLS.md delta

| Location | Edit |
|---|---|
| `§1.2` (after the four-stage diagram) | new paragraph: a version is a stage-4 property (§2.1 clause 1) |
| `§1.4` Transport row | appended sentence: versions are inherited from the shared client (§2.1 clause 2) |
| `§3.5` | new bullet: version selection is the transport's (§2.1 clause 3) |
| `§4.2` table | new row: `CodeProtocolMismatch` (§2.1 clause 4) |
| `§4.6` | new bullet: the h3 in-process pattern (§2.1 clause 5) |
| new `§4.7` | N1–N8 as drafted in §3 |
| `§2` matrix | rows 15/16/17 (§1.3) |
| `§6` gate table | orders 9/10/11 (§5) |
| `§7` demo plan | the ordinal rule + scripts 19/20/21 (§6) |
| header | version bump + `Last Updated`; `Addresses`/`Related specs` gain this spec |
