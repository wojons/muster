# PROTO-011 — `.proto` as a first-class schema source beyond gRPC

**Task row**: PROTO-011 (INVESTIGATE, complexity 2)
**Trigger**: Bane, 2026-09-26 — *"Muster might need some support for .proto file support as well."*
**Tree measured**: worktree `/home/kara/worktrees/muster-PROTO-011`, branch `wt/PROTO-011`,
HEAD `4bd1e0dc5b0517702095250c58c79505b427f00e`
**Date**: 2026-09-26
**Deliverable**: this document. **No product code was changed by this row** (acceptance 7).
**Design authority read first**: `specs/PROTOCOLS.md` (410 lines).

---

## 0. Verdict in one screen

The row exists to class three readings of Bane's sentence **before** any spec is written. Each is
classed below; the sections that follow carry the file:line evidence and the raw measurements.

| # | Reading | Class | One-line reason (evidence in §5) |
|---|---|---|---|
| **a** | General `.proto` **parsing** as a schema source feeding the operation model independently of invoking gRPC | **PARTIAL** | The parser ships and is proven (`pkg/protocol/grpc/protosource.go:26`), but the *only* thing extracted from a descriptor graph is `GetServices()` (`pkg/protocol/grpc/ingest.go:90-96`). Measured: a `.proto` containing 3 messages + 1 enum and **no** `service` yields **0 operations**; so does its descriptor set. There is no schema surface to feed. |
| **b** | `.proto` as a **description format for the WebDAV surface** (so client code is generated from a contract instead of hand-written) | **MISSING — and a mismatch to recommend against** | There is **no WebDAV code in the repo at all** (0 hits outside the PROTO-010..018 board rows themselves). `.proto` has no HTTP verb/path vocabulary; describing HTTP requires the `google.api.http` annotation toolchain, and the parser cannot even resolve `google/api/annotations.proto` on this box (measured). Muster's HTTP surface is already described by the OpenAPI adapter, which is the format WebDAV-shaped work should reuse (§5b). |
| **c** | Descriptor-set handling gaps left by PROTO-002 | **SHIPPED with one real gap** | `.proto` source, self-contained descriptor sets, and reflection all ingest and map through one path (measured, §4.1/§4.3). The gap: a descriptor set **without** transitive dependencies hard-fails with a library-level error and no user-facing remedy (measured, §4.3). Two capabilities exist in code but are unreachable from any binary: emitting a descriptor set (`##descriptorSetBytes`) and a *typed* discovery output. |

Headline for the follow-up spec row: **(a) is where the real work is, (c) is one bounded
ingest-contract fix, and (b) should be answered "no" in prose rather than built.**

One premise in the row is **stale** and is corrected here: `specs/PROTOCOLS.md:12` says protobuf
"is only an indirect dependency". It is **direct** as of this HEAD (§3).

---

## 1. Method — what was measured, and how

Nothing below is estimated. Every count and behaviour in §3–§5 is either a **MEASURED** result
(transcript in Appendix A) or a **CODE-READ** fact cited by `file:line`. The two are labelled where
they could be confused.

Measurement set-up:

* `go build -o /tmp/muster-probe-011/openapi-cli ./cmd/openapi-cli` from the worktree HEAD
  (go1.26.6 linux/amd64) — the real binary, not a test harness.
* Fixtures written to `/tmp/muster-probe-011/fx/` (outside the worktree; Appendix B lists them).
* Descriptor sets produced with `protoc 3.21.12` (system protoc), with and without
  `--include_imports`, to isolate the dependency-closure behaviour.
* For facts that the CLI does not print (per-field mapping into the neutral model), the same
  HEAD was **copied** to `/tmp/muster-copy-011-3363683` and a ~25-line `probe011` main was run
  there against `pkg/protocol/grpc.Adapter.Discover`. The copy exists so the worktree stays clean;
  the package under test is byte-identical to the worktree HEAD.
* Baseline suites confirmed green on this HEAD before drawing conclusions:
  `go test -short -count=1 ./pkg/protocol/... ./pkg/mcp/... ./internal/builtin/` → all 7 packages
  `ok` (grpc 0.475s, jsonrpc 0.931s, graphql 4.065s, mcp 0.031s, builtin 1.198s).

---

## 2. What already exists — the PROTO-002 / PROTO-003 accounting

`specs/PROTOCOLS.md:213-215` carries the gRPC matrix rows, and PROTO-002 (board: `complete`,
commit `b3e3d4da`) plus PROTO-003 (`complete`, commit `b9e896e4`) are in the tree. The `.proto`
surface today, classified:

| Capability | Class | Evidence |
|---|---|---|
| `.proto` source ingestion, pure Go, no `protoc` invocation | **SHIPPED** | `pkg/protocol/grpc/protosource.go:26-66` (`protoparse.Parser`, `Accessor: diskAccessor(...)`, `InferImportPaths: true` at `:56-59`); package doc `pkg/protocol/grpc/ingest.go:10-11` "protoc is never invoked (acceptance criterion)" |
| Import-path / file-list URI forms (`dir1:dir2=a.proto,b.proto`) | **SHIPPED** | `protosource.go:26-49`; measured working with two files (§4.1 M-C2) |
| Compiled `FileDescriptorSet` ingestion | **SHIPPED** | `ingest.go:203-231` (`loadDescriptorSet` → `decodeDescriptorSet` → `desc.CreateFileDescriptors`), reached by the suffix rule at `ingest.go:188-199` |
| Descriptor-set **self-containment** enforcement (missing transitive deps) | **PARTIAL** — fails loudly, no remedy | measured §4.3: `VALIDATION_ERROR: link descriptors from schema_nodeps.pb: no such file: "google/protobuf/timestamp.proto"`. Library contract: `desc/convert.go:176-178` ("must include all transitive dependencies"); failure raised in `createFromSet` at `desc/convert.go:261-294` via `intn.ErrNoSuchFile` |
| Server reflection (`grpc://` / `reflection://`, v1 + v1alpha) | **SHIPPED** | `pkg/protocol/grpc/adapter.go:114-130`, `pkg/protocol/grpc/reflect.go:23-110`; reflection-service self-filter at `reflect.go:47-50` |
| Method → `protocol.Operation` mapping | **SHIPPED** | `ingest.go:46-105`; measured: 1 unary + 1 server-streaming from a 2-method service (§4.1) |
| Rich message → neutral model mapping (nested/repeated/map/enum/WKT/oneof/optional) | **PARTIAL** — flat, one level | `ingest.go:110-124` iterates top-level fields only; measured output for a message exercising all seven shapes in §4.2: nested → `object` with **no** sub-fields, repeated → `array` with **no** item type, map → `object` with **no** value type, enum → `string` with `enum=[]`, oneof members emitted as two independent fields, WKT `Timestamp` → `object` |
| Enum **values** carried into the model | **MISSING** | `protocol.RequestBodyProperty.Enum` (`pkg/protocol/protocol.go:60`) is never assigned in the gRPC package: `grep -rn "Enum:" pkg/protocol/grpc/*.go` → **0** hits (measured) |
| proto2 `required` fidelity | **SHIPPED** | `ingest.go:131-133` (`!IsProto3() && IsRequired()`), consumed at `:120` |
| Unary invocation from the CLI (HTTP/2, TLS/mTLS, metadata, deadline) | **SHIPPED** | `internal/builtin/grpc.go:146-326`; adapter composition `adapter.go:156-254`; auth carrier `helpers.go:23-49` |
| Streaming (all 4 kinds, bounded transcript) | **SHIPPED (PROTO-003)** | `pkg/protocol/grpc/stream.go` (477 lines), bounds at `stream.go:46-57` (1000 msgs / 30 s); CLI bound-flag surface registered at `internal/builtin/grpc.go:314-323` (`--max-messages`, `--timeout`) and documented in the verb's help at `:188-198` |
| Discovery **listing** output (kind, ID, full method) | **SHIPPED** | `internal/builtin/grpc.go:113-124`. Note: text only, and the verb registers **no** format flag (measured on the `schema_nodeps.pb` / `httpannot.proto` error paths, which print the verb's whole flag list: `-h, --help` plus the global `--profile` only) |
| Emitting a descriptor set from a discovery run | **MISSING (dead code)** | `ingest.go:236-255` `descriptorSetBytes` is unexported and has **no** non-test caller (measured: the only references are its own definition and `pkg/protocol/grpc/testdata_test.go:412`). No CLI flag exposes it |
| Generated **CLI commands** from gRPC operations (spec `§3.3`) | **MISSING** | `pkg/generator` contains **0** references to `pkg/protocol` (measured: `grep -rn 'pkg/protocol' pkg/generator/` → 0 lines). `GenerateCommandsFromOps` (`specs/PROTOCOLS.md:245`) does not exist; the gRPC CLI surface is the two hand-written verbs `discover`/`call` |
| MCP tools for gRPC operations (spec `§3.4`) | **PARTIAL — library only, unwired** | `pkg/mcp/grpc_integration.go:23-51` + 10 tests in `pkg/mcp/grpc_integration_test.go`; **all** 14 non-test references to `GRPCRegistry` are inside that one file. `cmd/mcp-server/main.go` imports and registers OpenAPI (`:114`, `:204`) and JSON-RPC (`:123`, `:259`) only — **0** references to `pkg/protocol/grpc` anywhere in `cmd/` |
| grpcurl reference-client conformance (spec `§4.6:347`, gate `§6:382(f)`) | **MISSING** | Whole-repo grep for `grpcurl` → **6** hits (counted with `--exclude-dir=investigation`, i.e. excluding this document), **all prose/comments**: `internal/builtin/grpc.go:39`, `specs/PROTOCOLS.md:347`, `:382`, `pkg/protocol/grpc/ingest.go:234`, plus the PROTO-001 and PROTO-002 board rows. No test invokes it; `command -v grpcurl` → absent on this box. The repo's only wire-conformance test belongs to JSON-RPC (`pkg/protocol/jsonrpc/jsonrpc_test.go:614 TestConformance_WireBytes_Golden`) |
| Demo gate leg (spec `§7:401-403` `demo/demos/11-grpc-unary.sh`) | **MISSING** | `demo/demos/` holds `03-…` through `10-dsl-chaining.sh`; no `11-*`, no `*grpc*` script. `grep -i grpc demo/run-all.sh` → 0 hits |
| On-disk `.proto` fixtures in the repo | **ZERO** | `find . -name '*.proto'` (excluding `.git`) → **0** files. The only fixture is a Go string constant, `pkg/protocol/grpc/testdata_test.go:29` (`testKVSource`), materialised into `t.TempDir()` by `protoSourceToTemp` (`testdata_test.go:394-398`) |

Size of the surface, for scale on any follow-up estimate (measured `wc -l`):
production code **1 517** lines across the 7 non-test files in `pkg/protocol/grpc`;
tests **1 891** lines; gRPC-related test functions **77** (48 in `pkg/protocol/grpc`,
19 in `internal/builtin/grpc_test.go`, 10 in `pkg/mcp/grpc_integration_test.go`).

---

## 3. Dependency facts (acceptance 2) — `google.golang.org/protobuf` is **DIRECT**

Measured two ways (both agree):

* `go list -m -f '{{.Path}} direct={{not .Indirect}}' …`:

  ```
  google.golang.org/protobuf direct=true
  google.golang.org/grpc direct=true
  github.com/jhump/protoreflect direct=true
  github.com/jhump/protoreflect/v2 direct=false
  github.com/golang/protobuf direct=false
  ```
* `go.mod` — the entries carry **no** `// indirect` marker:
  * `go.mod:119` `google.golang.org/grpc v1.83.2`
  * `go.mod:120` `google.golang.org/protobuf v1.36.12`
  * `go.mod:82` `github.com/jhump/protoreflect v1.18.1`
  * and explicitly marked indirect: `go.mod:50` `github.com/golang/protobuf v1.5.4`, `go.mod:51`
    `github.com/jhump/protoreflect/v2 v2.0.0-beta.1`

`go mod why -m google.golang.org/protobuf` → `github.com/wojons/muster/pkg/protocol/grpc` →
`google.golang.org/protobuf/encoding/protojson`: the direct edge is created by the gRPC adapter
itself (`pkg/protocol/grpc/helpers.go:12`), not by a test.

**So the row's premise is wrong in the direction of *less* work**: .proto support does **not**
"likely promote" protobuf from indirect to direct — PROTO-002 already did that. What remains
unpromoted is unrelated:

* `github.com/bufbuild/protocompile v0.14.1` — in the **module graph only**, via
  `github.com/jhump/protoreflect/v2@v2.0.0-beta.1` (`go mod graph` edge). It is **not** in
  `go.mod`, and **0** packages with the module path `github.com/bufbuild/protocompile` are linked
  into `./cmd/openapi-cli`. What the binary actually runs is protoreflect's **vendored** copy,
  i.e. `github.com/jhump/protoreflect/desc/protoparse/internal/protocompile/...` (15 such packages
  in the CLI's dependency closure). `specs/PROTOCOLS.md:214`'s claim that protocompile v0.14.1 is
  part of the gRPC stack is therefore only true in the "module is in the graph" sense.
* Legacy `github.com/golang/protobuf v1.5.4` **is** on a live code path despite being marked
  indirect — `pkg/protocol/grpc/stream.go:11` imports it as `legacyproto`.

Correction owed to the design authority (documentation only, not this row's scope):
`specs/PROTOCOLS.md:12` — "`google.golang.org/protobuf` is only an indirect dependency" — was true
when PROTO-001 was written and is false at this HEAD.

---

## 4. Measurements

### 4.1 How many operations each source shape yields

Via `openapi-cli grpc discover <uri>` (transcript in Appendix A):

| Source | Contents | Operations found |
|---|---|---|
| `kv.proto` | 1 service, `Get` (unary) + `Watch` (server-streaming) | **2** (`kv.KeyValue-Get` unary, `kv.KeyValue-Watch` server-streaming) |
| `msgs_only.proto` | 3 messages + 1 enum, **no service** | **0** — `No operations found.` (exit 0) |
| `msgs_only.pb` | descriptor set of the above (`--include_imports`) | **0** — `No operations found.` (exit 0) |
| `empty.proto` | `syntax` + `package` only | **0** — `No operations found.` (exit 0) |
| `schema.proto` | 1 service whose input message exercises nested/repeated/map/enum/WKT/oneof/optional, importing `google/protobuf/timestamp.proto` | **1** (`schema.Desc-Do`) — the WKT import resolved with **no** protoc include path |
| `schema_all.pb` | same, `--include_imports` (2 files in the set: `schema.proto`, `google/protobuf/timestamp.proto`) | **1** — no phantom operations from the dependency file |
| `main.proto` + `dep.proto` consumed as `.:.=main.proto,dep.proto` | `main.proto` imports `dep.proto`, both declare services | **2** — `dep.Extra-Ping` **and** `mainpkg.Main-Do` |

Negative controls, to prove parsing (not just an empty result) is what runs:

* invalid syntax → `VALIDATION_ERROR: parse proto files [broken.proto] (import paths []): broken.proto:3:24: syntax error: unexpected ';', expecting int literal`
* missing file → `VALIDATION_ERROR: parse proto files [/tmp/does-not-exist.proto] (import paths []): NOT_FOUND: read proto file …: no such file or directory`
* the **root** (OpenAPI) verb on a `.proto` → `parse spec from "kv.proto" (not valid OpenAPI?) …` i.e. no format dispatch exists at the root surface (`internal/builtin/discover.go:33-90` calls `openapi.NewParser().Parse` unconditionally)

### 4.2 What the neutral model actually carries for a rich message

Measured through `Adapter.Discover` on `schema.proto` (the CLI's listing does not print fields, so
this needed the package-level probe):

```
operations=1
op=schema.Desc-Do kind=0 protocol=grpc
  field name=id         type=string   required=false enum=[]
  field name=inner      type=object   required=false enum=[]   <- nested message: no sub-fields
  field name=list       type=array    required=false enum=[]   <- repeated: no item type
  field name=attrs      type=object   required=false enum=[]   <- map: no value type
  field name=color      type=string   required=false enum=[]   <- enum: no values
  field name=created    type=object   required=false enum=[]   <- Timestamp WKT: not special-cased
  field name=x          type=string   required=false enum=[]   <- oneof member, ungrouped
  field name=y          type=integer  required=false enum=[]   <- oneof member, ungrouped
  field name=opt        type=string   required=false enum=[]   <- proto3 optional: optional
```

9 of 9 fields surfaced, **0** nested properties, **0** enum values, **0** oneof groups. The neutral
model (`pkg/protocol/protocol.go:55-62`) is deliberately flat: `RequestBodyProperty` has no
reference to a nested type, so this is a **model** limit, not a mapping bug. `mapMessageFields`
(`ingest.go:110-124`) does exactly what the model allows.

### 4.3 Descriptor sets: the dependency-closure rule

| Set | Built with | Result |
|---|---|---|
| `kv_nodeps.pb` (file has **no** imports) | plain `--descriptor_set_out` | **works** — 2 operations |
| `schema_nodeps.pb` (file imports a WKT) | plain `--descriptor_set_out` | **FAILS**: `VALIDATION_ERROR: link descriptors from schema_nodeps.pb: no such file: "google/protobuf/timestamp.proto"` (exit 1) |
| `schema_all.pb` (same file) | `--include_imports` | **works** — 1 operation |

The rule is the library's, not Muster's: `desc.CreateFileDescriptors` is documented as requiring
"all transitive dependencies for every file" (`jhump/protoreflect@v1.18.1/desc/convert.go:176-178`),
and `createFromSet` resolves each dependency **only** from the supplied set's own map, raising
`intn.ErrNoSuchFile` on a miss (`convert.go:272-275`) — there is **no** fallback to the Go protobuf
runtime registry. Muster passes the set straight through (`ingest.go:215-219`), so the caller
inherits that contract; the error text is accurate but names a file the user did not think they
needed, and the CLI offers no remedy (no `--include-imports`-style helper, no "compile with
protoc --include_imports" hint, no on-disk WKT lookup).

Worth noting in the same breath: the **source** path does **not** have this problem — `protoparse`
has the WKTs built in, which is why `schema.proto` parsed with a `google/protobuf/timestamp.proto`
import and no such file on disk. The two ingestion paths are therefore asymmetric.

### 4.4 Import resolution: what the parser can and cannot resolve

* `google/protobuf/timestamp.proto` — resolved from the parser's built-in standard imports (M-4.1
  above, measured).
* `google/api/annotations.proto` (the annotation file every `.proto`-described HTTP API needs) —
  **not resolvable**: `httpannot.proto:3:8: import "google/api/annotations.proto" not found
  (searched [.]): open google/api/annotations.proto: no such file or directory`.
  So on this box a `.proto` carrying `option (google.api.http) = …` cannot even be parsed without
  first obtaining the `googleapis` proto include tree, and nothing in the repo ships or fetches it.

### 4.5 Descriptor-set discovery is a union over **every** file in the set

`main.proto` importing `dep.proto`, built with `--include_imports`, discovers **both**
`mainpkg.Main-Do` and `dep.Extra-Ping`: `decodeDescriptorSet` returns all files
(`ingest.go:219-230`) and `MapFileDescriptors` walks them all (`ingest.go:99-105`). For a real
`protoc --include_imports` run over a service that imports a third-party library which itself
declares services, those library services become operations. No phantom operations appeared from
the WKT dependency file (it declares none) — the behaviour is principled, but it is a semantic
decision (union, not "the file the user named") that is not stated anywhere in `PROTOCOLS.md`'s
stage-1 contract.

### 4.6 Reachability of the `.proto` surface from the shipped binaries

| Surface | Reachable from | Evidence |
|---|---|---|
| `grpc discover` / `grpc call` (CLI) | `bin/openapi-cli` | registered at `internal/builtin/root.go:109`; verbs `internal/builtin/grpc.go:81-82` |
| OpenAPI discovery (`discover`, root) | `bin/openapi-cli` | `root.go:100` |
| `protocol jsonrpc` / `protocol graphql` | `bin/openapi-cli` | `root.go:108`; `internal/builtin/protocol.go:44-45` |
| gRPC **MCP tools** | **nothing** | `GRPCRegistry` referenced only in `pkg/mcp/grpc_integration.go` + its test; `cmd/mcp-server/main.go` registers OpenAPI (`:114`) and JSON-RPC (`:123`) only |
| GraphQL MCP tools | **nothing** | `pkg/protocol/graphql/mcp.go:304 RegisterWithServer` exists; `cmd/` contains **0** references to `pkg/protocol/graphql` |

---

## 5. Classifying (a), (b), (c)

### 5a. General `.proto` parsing as a schema source, independent of invoking gRPC — **PARTIAL**

What is shipped is *parsing plus RPC extraction*. The parser is complete and pleasant to use: pure
Go, no `protoc`, URI forms for file lists and import paths, WKT imports built in, comments carried
through into descriptions (`ingest.go:172-180`), proto2 `required` respected, one shared mapping
for all three discovery sources.

What does **not** exist is any *schema* surface. Measured: 0 operations from a service-less file and
from its descriptor set (§4.1). Code-read: the extraction is `fd.GetServices()`
(`ingest.go:90-96`), so messages, enums, and extensions are structurally invisible. And even for a
service file, the discovered operation carries a **flat** field list (§4.2) — no nesting, no enum
values, no oneof groups — so "feed the operation model" cannot today express a type graph even if
something walked the message types.

So: if Bane means "let me point Muster at a `.proto` and get a schema/type surface out of it"
(introspection, validation, docs, codegen, or a contract for something else), that is **not
shipped** — not because the parser is missing, but because the model and the extractor are
method-shaped. That is the fraction of PROTO-002 that composed with the existing engine.

### 5b. `.proto` as the description format for the WebDAV surface — **MISSING, and the wrong tool**

Facts, all measured or code-read:

1. **There is no WebDAV surface in this repo.** `grep -ri webdav` over the whole tree returns hits
   **only** inside `.coding-hermes/board/tasks.jsonl` rows PROTO-010..PROTO-018 (the planning rows
   this investigation belongs to). 0 Go files, 0 specs, 0 docs.
2. **Muster is the consumer side** (board PROTO-010: it must talk to a bunkerd WebDAV surface over
   HTTP/1.1, h2, h3). Consuming WebDAV requires *HTTP request construction and version
   negotiation* — which is PROTO-010/012/014/015 territory, not protobuf territory.
3. **`.proto` cannot describe HTTP verbs/paths by itself.** The only mechanism is the
   `google.api.http` option family, and the parser cannot resolve `google/api/annotations.proto`
   on this box (§4.4). Even with the includes present, `openapi-cli grpc discover httpannot.proto`
   would still produce **0 operations**, because the extractor only looks at services and never at
   method options — a `.proto`-described REST surface would need `protoc-gen-openapi`/gnostic-style
   translation, i.e. a second toolchain for a problem the repo already solves.
4. **The repo already has the right description format.** WebDAV endpoints (PROPFIND, `DAV:`
   headers, depth) are HTTP; Muster's HTTP description path is the OpenAPI adapter (kin-openapi
   v0.149.0) with the whole stage-2/3/4 machinery behind it (`specs/PROTOCOLS.md:190-195`,
   `:213`).

Verdict: **do not build (b).** The honest answer to "should the WebDAV surface be described in
`.proto`" is that a contract for the WebDAV surface should be expressed in the format Muster
already ingests for HTTP, and protobuf is only a payload encoding there — which is orthogonal to
Bane's sentence and already covered by the gRPC adapter for gRPC backends.

### 5c. Descriptor-set handling gaps — **SHIPPED, one gap**

Ship-book quality where it matters: three source forms verified end-to-end (source, set,
reflection), the set decoder handles both raw and protoc-built sets, deterministic ordering
(`ingest.go:228-229`), a parity test between proto-sourced and set-sourced discovery
(`pkg/protocol/grpc/adapter_test.go:81 TestDiscover_DescriptorSetParity`), and a bad-bytes test
(`adapter_test.go:522 TestIngest_BadDescriptorSet`).

The gap is **non-self-contained sets** (§4.3). It is real, it is the first thing an operator hits
when they run bare `protoc --descriptor_set_out`, and it is bounded: an ingest-contract decision
(detect the miss early, name the remedy, or resolve missing imports from the parser's standard
imports / a supplied include dir) plus a test. Two adjacent non-gaps to state explicitly so the
follow-up row does not re-spec them: emitting a descriptor set is *not* needed for consumption, and
the union-over-all-files behaviour of §4.5 is a documented-semantics issue, not a defect.

---

## 6. What a general, non-gRPC `.proto` source would need that does not exist today (acceptance 4)

Enumerated as gaps, each tied to the evidence above. This is the list the follow-up spec row should
choose from — **not** a mandate to build all of it.

1. **A schema extractor.** Something that walks `desc.FileDescriptor.GetMessageTypes()/
   GetEnumTypes()/GetExtensions()` in addition to `GetServices()` (`ingest.go:90-96` is the
   services-only walk). Today a service-less file yields 0 operations (§4.1).
2. **A typed output in the neutral model.** `protocol.Operation` is the only cross-adapter contract
   and it is operation-shaped; `RequestBodyProperty` (`pkg/protocol/protocol.go:55-62`) is flat.
   Exposing a schema graph needs either recursive properties or a second neutral type (e.g. a
   schema/type registry keyed by fully-qualified name) — and a decision about how it travels to MCP
   JSON Schema, where nesting/enums/oneof actually matter for agent input validity.
3. **Fidelity the model currently drops** (measured in §4.2, all four): nested message structure,
   repeated item types, map key/value types, enum value sets and names; plus oneof grouping and
   WKT special-casing (Timestamp/Duration/Any/Struct/FieldMask are "well-known" precisely because
   their JSON forms are special). Note `RequestBodyProperty.Enum` exists and is never populated.
4. **A consumer that does not execute.** For operations, stage 3 is where meaning is made, and for
   gRPC it is **absent**: `pkg/generator` has 0 `pkg/protocol` references, so
   `GenerateCommandsFromOps` (`specs/PROTOCOLS.md:245`) does not exist, and the MCP registry is
   unwired (§4.6). A schema source has no Executor at all, so it needs a consumer that renders/
   validates/emits rather than calls — that concept does not exist anywhere in the repo today.
5. **An import-resolution story for non-WKT imports.** `google/api/annotations.proto` fails (§4.4).
   A general path needs either bundled includes, a `--proto-path` for the fetch, or a documented
   requirement — plus the same treatment for the descriptor-set closure gap (§4.3), since both are
   "a file the user did not name is needed".
6. **A machine-readable discovery output.** `grpc discover` prints two text lines per operation
   (`internal/builtin/grpc.go:119-122`) and registers no format flag; the JSON-RPC adapter's
   `--doc` path prints full JSON operation views (`internal/builtin/protocol.go:244-293`). Any
   tooling that consumes `.proto` output will want the JSON shape, not the pretty listing.
7. **Fixtures on disk.** 0 `.proto` files in the repo; the only fixture is a Go string constant
   (`testdata_test.go:29`). A general schema path — and any golden-output test for it — will want
   real files, plus the `demo/` leg that `PROTOCOLS.md §7` already promises and that does not exist
   (`demo/demos/` stops at `10-dsl-chaining.sh`).

---

## 7. The seam (acceptance 6)

`specs/PROTOCOLS.md`'s seam is **per-protocol**: §1.2 stage 1 "Discover/Ingest (per protocol)"
(`:49-61`), §3.1 contract (`:232-236`), §3.2 model (`:238-241`). Where a `.proto` source attaches,
and where it does **not**:

| Work item | Existing seam it attaches to | Verdict |
|---|---|---|
| Descriptor-set closure gap (5c) | **§3.1 Discover/Ingest**, gRPC row's "Discovery input" cell (`specs/PROTOCOLS.md:55`) — plus a sentence in §3.1's bullet list about self-containment/remedy behaviour | **Existing seam. No new seam needed.** It is an ingest-contract clarification on an already-adopted adapter |
| Union-vs-named-file semantics (§4.5) | same cell, §3.1 | **Existing seam** — one sentence of contract |
| JSON discovery output for `grpc discover` | §4.4 Output rendering (`:303-307`) + §5.1 subcommand policy (`:353-360`) | **Existing seam** (rendering rule: adapters never print; the output layer formats) |
| General non-gRPC `.proto` **schema** source (5a) | **None exists** | **NEW SEAM REQUIRED**, and it is not a protocol row. The matrix (`:211-227`) is a table of *protocols*, keyed by how operations are discovered and executed; a schema source produces no operations and has no executor, so it fits neither stage 1's output contract (`[]*protocol.Operation`, `:236`) nor §3.2's model. Justification: adding it as "matrix row 15" would overload `Protocol`, and adding it as a gRPC sub-feature would hide a schema surface behind an RPC adapter. The smallest honest seam is a **stage-2 input** in §3.2 — a neutral schema/type model produced by an ingest source and consumed by *rendering/validation* stage-3 consumers — with the gRPC adapter's existing descriptor loader as its first producer |

(b) has no seam either, and should not get one: `PROTOCOLS.md`'s matrix decides *protocols*; the
WebDAV work is already covered as an HTTP-transport question by PROTO-010/012/014/015. If the
owner still wants a written ruling, it belongs as a **rejected-alternative note** in the WebDAV
spec (`PROTO-012`), not as a matrix row.

---

## 8. Proposed scope for the follow-up spec row (acceptance 5)

**The scope below is an OUTPUT of this investigation, not an input to it.** It is derived from the
classification in §0/§5 and the gap list in §6; if the owner's intended reading differs, the
classification is what should be argued with, and the scope should be re-derived.

Proposed split (rows already exist on the board as PROTO-013 spec → PROTO-016 exec → PROTO-018
test; this investigation recommends their *content*, not their existence):

1. **In scope — document what ships, measurably.** `PROTO-013` should record, in `PROTOCOLS.md`'s
   existing shape: the three discovery inputs (`:55` already says this), the four source-shape
   behaviours measured in §4.1, the union semantics of §4.5, and the descriptor-set
   self-containment rule with its remedy. Then `PROTO-016` is honestly **documentation-only** for
   everything except item 3, and `PROTO-018` closes against the existing PROTO-002 pattern plus the
   missing `demo/demos/11-grpc-unary.sh` leg from `PROTOCOLS.md §7`.
2. **In scope — one bounded code fix.** The descriptor-set closure gap (5c): fail early with a
   remedy that names `protoc --include_imports` (or a configured include dir / the parser's standard
   imports), plus a regression test using a set built without `--include_imports`. This is the only
   *defect-class* item in the whole investigation.
3. **In scope, decision-required — the schema surface (5a).** Either (i) declare `Muster` is an
   *operation* client and `.proto` support means "methods in, commands/tools out" — in which case
   the deliverable is the pinned documentation plus the honest label on the model's flatness
   (§4.2); or (ii) adopt a schema surface, which requires the new stage-2 seam of §7, recursive
   typing, enum/oneof/WKT fidelity, and a non-executing consumer. (ii) is a **design** row with its
   own gate, materially larger than the "document and close" outcome the board anticipates.
4. **Out of scope — and recommended to stay so.** `.proto` as a description format for the WebDAV
   surface (§5b). Behaviour change to the neutral model for gRPC operations (generated CLI
   commands, MCP wiring) — those are separate gaps (§4.6) that exist with or without `.proto`
   support and should be filed on their own merits.
5. **Ordering.** Fix + document (2, then 1) before any schema work (3), because (2) is user-visible
   today and (3) is the only part that can be answered "no" at zero cost.

Cost honesty: items 1–2 are close to the `complexity=2` the board assigned to this chain. Item 3(ii)
is not, and should be re-sized if adopted.

---

## 9. Adjacent findings (found while measuring, not asked for by this row)

Filed as *candidates*, deliberately **not** as board rows from this investigation (an investigation
should not mutate the board). Each is evidence-backed and independently checkable:

1. **`PROTOCOLS.md §6` gate 1 is not fully evidenced for gRPC.** Gate `(f)` (grpcurl conformance,
   `:382`) and §4.6(2)'s "one conformance comparison per protocol against a reference client"
   (`:346-347`) have no implementation: 0 grpcurl invocations in the tree, grpcurl not installed.
   The only reference-client wire test in the repo is JSON-RPC's
   (`pkg/protocol/jsonrpc/jsonrpc_test.go:614`).
2. **Gate `(d)` (MCP tool call returns the same payload) is satisfied only at unit-test level.**
   `pkg/mcp/grpc_integration_test.go` passes; the MCP server binary has no gRPC path (§4.6).
3. **`PROTOCOLS.md §7`'s `demo/demos/11-grpc-unary.sh` does not exist** and `run-all.sh` has no
   gRPC leg, so PROTO-002 acceptance (7) is unmet in the tree.
4. **Stage 3a (`GenerateCommandsFromOps`) is unimplemented for every adapter**, not just gRPC:
   `pkg/generator` has 0 `pkg/protocol` references.
5. **`specs/PROTOCOLS.md:12` is stale** (protobuf dependency class) and `:214` overstates
   protocompile's role (§3).
6. **`.proto` discovery output is text-only** while JSON-RPC's discovery prints JSON (§6.6).
7. **Ops observation, not a `.proto` finding**: this repo's `core.hooksPath` is set to `.githooks`,
   which does **not** exist in the main tree or in this worktree, and is not tracked by git
   (`git ls-files .githooks` → 0; `ls .githooks` → No such file). The pre-commit guard therefore
   never runs on a commit in this configuration — a commit prints no guard banner. This affects the
   evidentiary weight of *any* commit here, including this document's; it was recorded by running
   the guard out-of-band on the staged change set (see the row's completion report).

---

## 10. Open questions — things this investigation could **not** determine

1. **Which reading Bane actually meant.** The sentence is ambiguous between (a)/(b)/(c); this
   document classes all three so the owner can decide, but the *intent* is not knowable from the
   repo. Owner ruling wanted before PROTO-013's scope is fixed for item 3.
2. **Whether the WebDAV surface will ever need a machine-readable contract of its own.** If an RFC
   4918-level description is ever wanted, the format choice (OpenAPI as today vs something else) is
   a `PROTO-012` question; this investigation only measured that `.proto` is not a viable carrier
   (§5b).
3. **Whether protoreflect v1 is the long-term parser.** `github.com/jhump/protoreflect/v2
   v2.0.0-beta.1` and `github.com/bufbuild/protocompile v0.14.1` are already in the module graph;
   the adapter uses v1's `desc/protoparse` with its vendored protocompile copy. Migration cost and
   whether v2 would change any measured behaviour were **not** measured — out of scope here.
4. **Whether the flat neutral model is a defect for the *existing* gRPC path.** For unary calls the
   request is built by `dynamic.Message.UnmarshalJSON` from the real descriptor
   (`helpers.go:75-88`), so agent input is validated against the true schema regardless of the
   neutral model's flatness; MCP input *schemas*, however, are generated from the flat model
   (`pkg/mcp/grpc_integration.go:57+`). I did not measure the behavioural difference between the two
   for a nested/enum input — worth one focused test before deciding whether §4.2 is a `.proto`-only
   concern.
5. **Scale/performance of discovery** on large descriptor graphs (e.g. the googleapis include tree)
   was not measured; the `google/api/annotations.proto` fixture could not be parsed at all (§4.4).

---

## Appendix A — raw measurement transcript

Binary: `/tmp/muster-probe-011/openapi-cli` built from HEAD `4bd1e0dc` (go1.26.6).
Fixtures: `/tmp/muster-probe-011/fx/`.

```
$ openapi-cli grpc discover kv.proto
unary            kv.KeyValue-Get
         /kv.KeyValue/Get
server-streaming kv.KeyValue-Watch
         /kv.KeyValue/Watch
rc=0

$ openapi-cli grpc discover msgs_only.proto
No operations found.
rc=0

$ openapi-cli grpc discover schema.proto
unary            schema.Desc-Do
         /schema.Desc/Do
rc=0

$ openapi-cli grpc discover schema_all.pb          # protoc --include_imports
unary            schema.Desc-Do
         /schema.Desc/Do
rc=0

$ openapi-cli grpc discover schema_nodeps.pb       # protoc WITHOUT --include_imports
Error: VALIDATION_ERROR: link descriptors from schema_nodeps.pb: no such file: "google/protobuf/timestamp.proto"
rc=1

$ openapi-cli grpc discover msgs_only.pb
No operations found.
rc=0

$ openapi-cli grpc discover kv_nodeps.pb           # no imports in the file -> works
unary            kv.KeyValue-Get
         /kv.KeyValue/Get
server-streaming kv.KeyValue-Watch
         /kv.KeyValue/Watch

$ openapi-cli grpc discover multi.pb               # main.proto + dep.proto, --include_imports
unary            dep.Extra-Ping
         /dep.Extra/Ping
unary            mainpkg.Main-Do
         /mainpkg.Main/Do

$ openapi-cli grpc discover ".:.=main.proto,dep.proto"
unary            dep.Extra-Ping
         /dep.Extra/Ping
unary            mainpkg.Main-Do
         /mainpkg.Main/Do

$ openapi-cli grpc discover httpannot.proto        # google.api.http annotations
Error: VALIDATION_ERROR: parse proto files [httpannot.proto] (import paths []):
  httpannot.proto:3:8: import "google/api/annotations.proto" not found (searched [.]):
  open google/api/annotations.proto: no such file or directory

$ openapi-cli grpc discover broken.proto
Error: VALIDATION_ERROR: parse proto files [broken.proto] (import paths []):
  broken.proto:3:24: syntax error: unexpected ';', expecting int literal

$ openapi-cli grpc discover /tmp/does-not-exist.proto
Error: VALIDATION_ERROR: parse proto files [/tmp/does-not-exist.proto] (import paths []):
  NOT_FOUND: read proto file /tmp/does-not-exist.proto: no such file or directory

$ openapi-cli grpc discover empty.proto
No operations found.
rc=0

$ openapi-cli discover kv.proto                    # ROOT (OpenAPI) verb on a .proto
Error: parse spec from "kv.proto" (not valid OpenAPI?): failed to unmarshal data:
  json error: invalid character 's' looking for beginning of value, ...
```

Package-level probe (copy of the same HEAD, in `/tmp/muster-copy-011-3363683`, `probe011`):

```
$ go run ./probe011 /tmp/muster-probe-011/fx/schema.proto
operations=1
op=schema.Desc-Do kind=0 protocol=grpc
  summary="Invoke schema.Desc-Do" tags=[] params=0
  field name=id         type=string   required=false enum=[] desc=""
  field name=inner      type=object   required=false enum=[] desc=""
  field name=list       type=array    required=false enum=[] desc=""
  field name=attrs      type=object   required=false enum=[] desc=""
  field name=color      type=string   required=false enum=[] desc=""
  field name=created    type=object   required=false enum=[] desc=""
  field name=x          type=string   required=false enum=[] desc=""
  field name=y          type=integer  required=false enum=[] desc=""
  field name=opt        type=string   required=false enum=[] desc=""
  source={"descriptors":"proto","full_method":"/schema.Desc/Do","input_type":"schema.Outer",
          "method":"Do","output_type":"schema.Outer","service":"schema.Desc"}

$ go run ./probe011 /tmp/muster-probe-011/fx/msgs_only.proto
source=.../msgs_only.proto operations=0

$ go run ./probe011 /tmp/muster-probe-011/fx/msgs_only.pb
source=.../msgs_only.pb operations=0

$ go run ./probe011 /tmp/muster-probe-011/fx/empty.proto
source=.../empty.proto operations=0

$ go run ./probe011 /tmp/muster-probe-011/fx/kv.proto
source=.../kv.proto operations=2
  op=kv.KeyValue-Get requestfields=1 [key:string req=false]
  op=kv.KeyValue-Watch requestfields=1 [prefix:string req=false]
```

Counts (all measured on this HEAD):

```
find . -name '*.proto' -not -path './.git/*' | wc -l        -> 0
grep -rn 'Enum:' pkg/protocol/grpc/*.go | wc -l             -> 0
grep -rn 'pkg/protocol' pkg/generator/ | wc -l              -> 0
grep -rn 'GRPCRegistry' --include=*.go . | grep -v _test.go | wc -l -> 14 (all in its own file)
grep -rn 'grpc' cmd/mcp-server/*.go | wc -l                 -> 0
ls demo/demos/                                              -> 03..10 (no 11-grpc-unary.sh)
grep -rn 'grpcurl' . --exclude-dir=.git --exclude-dir=investigation | wc -l
                                                            -> 6 (4 spec/board prose, 2 code comments; 0 invocations)
                                                               (the --exclude-dir keeps this document out of its own count)
wc -l pkg/protocol/grpc/{adapter,errors,helpers,ingest,protosource,reflect,stream}.go -> 1517 total
wc -l pkg/protocol/grpc/*_test.go                           -> 1891 total
gRPC test functions: 48 (pkg/protocol/grpc) + 19 (builtin) + 10 (mcp) -> 77
go test -short -count=1 ./pkg/protocol/... ./pkg/mcp/... ./internal/builtin/ -> 7 packages ok
```

Dependency probes:

```
$ go list -m -f '{{.Path}} direct={{not .Indirect}}' google.golang.org/protobuf google.golang.org/grpc \
    github.com/jhump/protoreflect github.com/jhump/protoreflect/v2 github.com/golang/protobuf
google.golang.org/protobuf direct=true
google.golang.org/grpc direct=true
github.com/jhump/protoreflect direct=true
github.com/jhump/protoreflect/v2 direct=false
github.com/golang/protobuf direct=false

$ go mod why -m google.golang.org/protobuf
# google.golang.org/protobuf
github.com/wojons/muster/pkg/protocol/grpc
google.golang.org/protobuf/encoding/protojson

$ go mod graph | grep protocompile
github.com/jhump/protoreflect/v2@v2.0.0-beta.1 github.com/bufbuild/protocompile@v0.14.1

$ go list -deps ./cmd/openapi-cli | grep -c '^github.com/bufbuild/protocompile'  -> 0
$ go list -deps ./cmd/openapi-cli | grep -c 'protoreflect/desc/protoparse/internal/protocompile' -> 15
```

## Appendix B — fixtures used (exact inputs)

`msgs_only.proto` — 3 messages + 1 enum, no service:

```proto
syntax = "proto3";
package only;
message Thing { string id = 1; int64 n = 2; }
message Other { string name = 1; }
enum Flavor { FLAVOR_UNSPECIFIED = 0; SWEET = 1; }
```

`schema.proto` — the rich message (nested, repeated, map, enum, WKT, oneof, optional) plus one service:

```proto
syntax = "proto3";
package schema;
import "google/protobuf/timestamp.proto";
enum Color { COLOR_UNSPECIFIED = 0; RED = 1; }
message Inner { string a = 1; }
message Outer {
  string id = 1;
  Inner inner = 2;
  repeated Inner list = 3;
  map<string,string> attrs = 4;
  Color color = 5;
  google.protobuf.Timestamp created = 6;
  oneof choice { string x = 7; int32 y = 8; }
  optional string opt = 9;
}
service Desc { rpc Do(Outer) returns (Outer); }
```

`httpannot.proto` — the `google.api.http` annotation shape that (b) would require:

```proto
syntax = "proto3";
package webdav;
import "google/api/annotations.proto";
message PutReq { string path = 1; }
message PutResp { bool ok = 1; }
service Files {
  rpc Put(PutReq) returns (PutResp) {
    option (google.api.http) = { put: "/{path}" body: "*" };
  }
}
```

`dep.proto` / `main.proto` — the import relationship used for the union measurement; `kv.proto` —
1 service, 1 unary + 1 server-streaming method; `empty.proto`, `broken.proto` — negative controls.

Descriptor sets: `protoc -I . --descriptor_set_out=<name>.pb [--include_imports] <file>.proto`
(libprotoc 3.21.12); set file counts verified with `protoc --decode=google.protobuf.FileDescriptorSet`.
