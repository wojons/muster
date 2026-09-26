# PROTO-013 — The `.proto` support surface (implementation spec)

**Task row**: PROTO-013 (SPEC, complexity 2) — the spec row of the board chain PROTO-011 (investigate)
→ **PROTO-013 (this row, spec)** → PROTO-016 (EXEC) → PROTO-018 (TEST).
**Depends on**: PROTO-011 (INVESTIGATE, complete, merged `de77926f`) — **the scope below is derived
from its findings, not assumed in advance.**
**Evidence base**: `docs/investigation/PROTO-011-proto-support.md` (646 lines, measured at HEAD
`4bd1e0dc`), re-verified for this spec at HEAD `0765b0c0` (worktree `muster-PROTO-013`, branch
`wt/PROTO-013`) — transcript in Appendix A.
**Design authority read first**: `specs/PROTOCOLS.md` (410 lines).
**Date**: 2026-09-26.
**Status**: SPEC. **This row changes no product code** (acceptance 6). Every product, doc and board
change named here is assigned to PROTO-016 / PROTO-018 / a named new row in §7.

---

## 0. Scope rule — the scope IS an output of PROTO-011 (acceptance 1)

PROTO-011 classed three readings of Bane's sentence *"Muster might need some support for .proto file
support as well."* This spec covers exactly those three, in that order, and nothing else. Each section
below states a **decision** plus the **measurement** that forces it.

| # | Reading (PROTO-011's class, verbatim) | Section here | This spec's outcome |
|---|---|---|---|
| **a** | General `.proto` **parsing as a schema source** beyond gRPC — **PARTIAL** | §2 | **ADOPTED, scoped and gated.** Extraction is services-only (`pkg/protocol/grpc/ingest.go:92`) and a service-less `.proto` yields **0** operations (measured). Spec'd as four increments S1–S4 against a new stage-2 schema seam, with gate **G-SCHEMA**. |
| **b** | `.proto` as the **description format for the WebDAV surface** — **MISSING, and a mismatch to recommend against** | §4 | **DECLINED in prose**, with the measured reasoning and the place the ruling belongs. Declining is the deliverable (acceptance 3). |
| **c** | **Descriptor-set handling** left by PROTO-002 — **SHIPPED with one gap** | §1 (ships) + §3 (the gap) | **Documented as shipped** (the anti-re-specification list, `file:line` per capability) + **one bounded fix** for the dependency-closure gap, with gate **G-CLOSURE**. |

### 0.1 The failure this row exists to prevent — what this spec deliberately does NOT re-specify

PROTO-013's row text names its own anti-goal ("re-specifying descriptor-set ingestion that already
ships"). The following are therefore **explicitly out of scope, because they are already implemented
and tested in the tree**. They are listed with their evidence so PROTO-016 does not build them again:

| Already shipped — do not re-spec | Evidence |
|---|---|
| `.proto` **source** ingestion, pure Go, no `protoc` invocation | `pkg/protocol/grpc/protosource.go:26-66`; measured working (Appendix A: `schema.proto` → 1 op, WKT import resolved with no include path) |
| Import-path / file-list URI forms (`dir1:dir2=a.proto,b.proto`) | `protosource.go:26-49`; PROTO-011 §4.1 M-C2 |
| Compiled `FileDescriptorSet` ingestion | `ingest.go:203-231` (`loadDescriptorSet` → `decodeDescriptorSet` → `desc.CreateFileDescriptors`) |
| Deterministic descriptor ordering | `ingest.go:228-229` |
| Descriptor-set ↔ source **parity test** + bad-bytes test | `pkg/protocol/grpc/adapter_test.go:81 TestDiscover_DescriptorSetParity`, `:522 TestIngest_BadDescriptorSet` |
| Server reflection (v1 + v1alpha), reflection-service self-filter | `pkg/protocol/grpc/reflect.go:23-110`, `:47-50` |
| Method → `protocol.Operation` mapping, **all four** stream kinds | `ingest.go:46-105`; streaming `pkg/protocol/grpc/stream.go` (477 lines), bounds `stream.go:46-56` (1000 messages / 30 s) |
| proto2 `required` fidelity | `ingest.go:131-133` (`isRequiredByRule`), consumed at `ingest.go:120` |
| The three discovery inputs as a **stated** contract | `specs/PROTOCOLS.md:55` already reads "`.proto` source, compiled `FileDescriptorSet`, or server reflection (PROTO-002)" — the §3.1 sentence this spec adds is the *closure rule*, not the input list |

---

## 1. Reading (c) — what ships today, and the one gap

Classification **SHIPPED with one gap** (PROTO-011 §0/§5c), reproduced behaviourally at HEAD
`0765b0c0` (Appendix A):

| Source form | Measured result |
|---|---|
| `.proto` with 1 service, 1 unary + 1 server-streaming method | **2** operations |
| that file's `FileDescriptorSet`, bare `--descriptor_set_out` (the file imports nothing) | **2** operations |
| a file importing a well-known type, set built with `--include_imports` (2 files in the set) | **1** operation — and **no** phantom operations from the dependency file |
| **the same file, set built WITHOUT `--include_imports`** | **FAILS, exit 1**: `VALIDATION_ERROR: link descriptors from schema_nodeps.pb: no such file: "google/protobuf/timestamp.proto"` |
| a 3-message + 1-enum **service-less** file and its descriptor set | **0** operations (exit 0) — the reading-(a) measurement, §2 |
| a set built with `--include_imports` over a file that imports another file declaring its own service | **2** operations (`dep.Extra-Ping`, `mainpkg.Main-Do`) — union over **every** file in the set |

**The gap is the fourth row and only the fourth row.** It is the first thing an operator hits when
they run bare `protoc --descriptor_set_out` on any file that imports `google/protobuf/*.proto`, it
fails with a library-level error, and **the CLI offers no remedy** (`ingest.go:219` passes the set
straight to `desc.CreateFileDescriptors`; the error text at `ingest.go:220-223` names the file but not
the fix). §3 specifies the bounded fix.

**Adjacent non-gaps, stated so they are not mistaken for defects** (PROTO-011 §5c, §4.5):

- **Emitting a descriptor set is not needed for consumption.** `descriptorSetBytes` (`ingest.go:236-255`)
  has **no** non-test caller (only `testdata_test.go` references it) and no CLI flag exposes it. This
  spec keeps it that way: no work here.
- **Union-over-all-files is contract, not a bug.** The behaviour is principled (all files in the set
  are walked, `ingest.go:224-229`), measured identical on the source path, and it is *documented* by
  §3.3 — not changed.
- **The source path does not have the closure problem at all.** `protoparse` has the well-known types
  built in: `schema.proto` importing `google/protobuf/timestamp.proto` parsed with **no** include path
  on disk (measured). The two ingestion paths are asymmetric, and that asymmetry is the reason the
  §3 fix is honest rather than a special case.

---

## 2. Reading (a) — the general `.proto` schema surface: **ADOPTED** (acceptance 2)

### 2.1 The measured defect this section exists to fix

PROTO-011 measured the following on the rich fixture (`schema.proto`, Appendix B of the
investigation; reproduced here):

- A `.proto` with **3 messages + 1 enum and no service** discovers **0** operations — from source
  *and* from its descriptor set. Exit code is **0** and the output is the single line
  `No operations found.`
- For a file *with* a service, the discovered operation carries a **flat** field list: 9 of 9 fields
  surfaced, **0** nested properties, **0** enum values, **0** oneof groups.
- `protocol.RequestBodyProperty.Enum` (`pkg/protocol/protocol.go:60`) is **never assigned** anywhere
  in the gRPC package: `grep -rn "Enum:" pkg/protocol/grpc/*.go` → **0** hits.

The operative code facts:

- **Extraction is services-only.** `MapFileDescriptor` walks `fd.GetServices()` and nothing else
  (`ingest.go:90-96`, the walk at `:92`); message, enum and extension declarations are structurally
  invisible. A service-less file is therefore a file with no discoverable surface.
- **The model is flat by construction.** `mapMessageFields` (`ingest.go:110-124`) iterates top-level
  fields only, and `protocol.RequestBodyProperty` has no reference to a nested type — a documented
  model limit, not a mapping bug.
- **The flatness is agent-facing today**, not only in a hypothetical schema source: `grpcInputSchema`
  (`pkg/mcp/grpc_integration.go:57-77`) builds MCP JSON input schemas directly from the flat
  `RequestBody` list — it copies `Type`, `Enum` (always empty), `Default`, `Description`. So for a
  nested or enum-typed field an agent is shown `"type": "object"` with no properties, or a string with
  no `enum` array. (PROTO-011 §10.4 left the *behavioural* difference between execution-time
  validation via the real descriptor — `helpers.go:82`, `dynamic.Message.UnmarshalJSON` — and the
  flat MCP schema unmeasured; S1's gate requires that one focused measurement.)

### 2.2 Decision D1 — the schema surface is a **new neutral type**, not a deeper operation model

**Decision.** Add `protocol.SchemaType` / `protocol.SchemaField` (plus an FQN-keyed
`protocol.SchemaSet`) as new types in `pkg/protocol`. Operations gain a **reference** to a type
(`SchemaField.Ref` = fully-qualified name) instead of embedding recursive trees.

**Reason.** `protocol.Operation` is operation-shaped and its `Source` doc already says stage-3
consumers must not interpret transport details (`specs/PROTOCOLS.md:136-140`); §1.5's back-compat
guarantee (`:205`) permits *additive* model change only, and three consumers (`pkg/openapi`,
`pkg/generator`, `pkg/mcp`) read the flat list today. Making `RequestBodyProperty` recursive would
silently change the meaning of every existing consumer's walk. A separate keyed type keeps the
operation contract flat, gives the schema graph a home that needs no Executor, and makes the wiring
question answerable once (S2) rather than per-consumer.

**Rejected alternative** (recorded, not silently dropped): recursion on `RequestBodyProperty`. It is
fewer types, but it forces every existing flat-list consumer to become recursion-aware and gives no
place for the descriptor-graph facts (FQN resolution, enum value names, oneof member sets, WKT
identity) that a schema surface needs.

### 2.3 Decision D2 — operation extraction stays **services-only**; schema extraction is a second walk

**Decision.** `MapFileDescriptor`'s services-only walk (`ingest.go:90-96`) is **correct and stays**.
Operations are RPCs; a `.proto` with no `service` has no operations, and inventing pseudo-operations
from message types would be a lie in the operation model. The schema surface gets its own extraction
that walks `GetMessageTypes()`, `GetEnumTypes()` and the file's extensions in addition to services,
producing a `SchemaSet`.

**Consequence that must be observable** (and is gated): the two surfaces are distinguishable from the
CLI. Today `grpc discover msgs_only.proto` prints `No operations found.` with exit **0**
(`internal/builtin/grpc.go:113-117`) — the same line as a file that parsed fine and has nothing, which
is exactly the ambiguity the operator hit. After S3, the schema verb reports the **type** count for
the same file (measured input: 4 types, 0 operations), so "no RPCs" and "nothing parsed" stop looking
alike.

### 2.4 Decision D3 — the producer/consumer seam and the FQN rule

**Decision.** The schema surface is a **stage-1 producer with a stage-2 output** and a
**non-executing stage-3 consumer**:

- **Producer (stage 1)**: a `SchemaIngester` in the gRPC adapter's package, reusing the *existing*
  loaders unchanged — `loadProtoSources` (`protosource.go:26-66`), `loadDescriptorSet`
  (`ingest.go:203-209`), reflection (`reflect.go:23-110`) — so all three discovery inputs produce
  schema sets exactly as they produce operations. **No new dependency**, no `protoc`, no new parser.
- **Output (stage 2)**: `*protocol.SchemaSet` — a map/registry keyed by fully-qualified type name,
  each `SchemaType` carrying fields, enum values and oneof member sets; each `SchemaField` carrying
  the JSON-schema-style type, cardinality, `Ref` (FQN of the referenced type, empty for scalars),
  map key/value types, WKT identity and description.
- **Carrier stamp**: `SchemaSet.Source map[string]string` reuses the *existing* source vocabulary
  already stamped on operations — `descriptors` = `proto` | `descriptorset` | `reflection`, declared at
  `ingest.go:37` and stamped at `ingest.go:72` (default), `adapter.go:139` (file-based: `proto` or
  `descriptorset`), `adapter.go:126-127` and `reflect.go:66-67` (reflection, which also stamps
  `host`). **No new
  `protocol.Protocol` constant is added**: §2's matrix maintenance rule (`PROTOCOLS.md:228`) makes a
  new protocol value a matrix event, and the schema set is a protobuf descriptor artifact of the same
  family, not a new wire protocol. This also keeps §10.3 of PROTO-011 (protoreflect v1 vs v2) from
  becoming a blocker — the walk uses the same `desc` API the shipped ingest uses.
- **Consumer (stage 3)**: rendering/validation, **never execution**. §4.4 (`PROTOCOLS.md:303-307`)
  already fixes the rule that adapters never print and that formats are shared; a schema document is
  another payload for that same rendering path. There is deliberately **no Executor** for a schema
  set.

**Where this attaches** is stated as a seam table in §5 (acceptance 5).

### 2.5 Decision D4 — depth-1 fidelity is part of this scope, because it is broken today

**Decision.** S1 fixes the four measured fidelity losses on the **operation** path (not only in the
new schema types), because they reach agents today through `grpcInputSchema`:

| Measured loss (PROTO-011 §4.2) | S1 behaviour |
|---|---|
| nested message → `type: object`, **no** sub-fields | `SchemaField.Ref` = FQN of the nested type (resolvable in the `SchemaSet`); the operation schema renders the reference rather than a bare `object` |
| `repeated Inner` → `type: array`, **no** item type | element type carried (scalar type or `Ref`) |
| `map<string,string>` → `type: object`, key/value lost | map key + value types carried |
| enum field → `type: string`, `enum=[]` | enum **value names** populated into `RequestBodyProperty.Enum` (the field exists at `protocol.go:60` and is unused — 0 hits) |
| `oneof` members → two independent fields | oneof membership carried so the group is expressible (and constraining in JSON Schema) |
| `google.protobuf.Timestamp` → `type: object` | WKT mapped to its JSON form (`string` with a format), as the wire JSON actually is |

proto2 `required` (`ingest.go:120`, `:131-133`) is already correct and is **not** re-specified.

**How a nested `Ref` is resolved by a consumer — the one wiring decision D4 forces (and closes).**
Enum values, WKT mapping, item types, map key/value and oneof membership are all depth-1 facts that fit
inside `RequestBodyProperty`, so they need no extra plumbing. A nested `Ref` needs the type it names, so
the consumer takes the schema graph **optionally**: `NewGRPCRegistry` (`pkg/mcp/grpc_integration.go:33`)
gains an additive option (e.g. `WithSchemaSet(*protocol.SchemaSet)`) and resolves `Ref`s through it;
**with no set supplied, a nested field renders exactly as it does today** (`"type":"object"`). The
existing two-argument constructor therefore keeps its behaviour and its callers, in line with §1.5's
additive-only rule (`PROTOCOLS.md:205`), and the richer schema arrives when a set is supplied (which
S3's verb and its wiring provide). This is why S1 is additive in behaviour, not a silent schema change
for an existing caller.

**Why S1 carries no back-compat risk today — and why that window is closing.** §5.3 of the design
authority freezes MCP tool *names* for OpenAPI-derived tools (`PROTOCOLS.md:373`), and a richer
schema is a payload change, not a name change — but the decisive fact is that **no agent sees these
gRPC input schemas yet**: `GRPCRegistry` has **0** non-test references outside
`pkg/mcp/grpc_integration.go`, and `cmd/` contains **0** references to `pkg/protocol/grpc` (the MCP
server registers OpenAPI and JSON-RPC only, `cmd/mcp-server/main.go:23`, `:259` — re-measured at this
HEAD). Fixing the flat schema now is therefore free; doing it after the MCP gRPC registration lands
would change schemas under live agents, which is a different (and much more expensive) decision. That
asymmetry is the reason S1 is sequenced first.

### 2.6 Scope increments S1–S4 (for PROTO-016 / PROTO-018)

| # | Increment | Content | Size/risk |
|---|---|---|---|
| **S1** | Operation-model depth-1 fidelity (D4) | `Ref` + item type + map key/value + enum values + oneof membership + WKT mapping; optional `SchemaSet` on the consumer so `Ref`s resolve (`grpcInputSchema`, `pkg/mcp/grpc_integration.go:57-77`, emits `enum` arrays; no set supplied ⇒ today's rendering, unchanged) | Small code, cross-consumer: touches `protocol` and `pkg/mcp` render path. **Do first** — it is user-visible on today's shipped surface |
| **S2** | `SchemaSet` extraction (D1/D3) | new neutral types + the schema walk (messages/enums/extensions) over the existing loaders; no new dependency | Medium; pure addition, no existing behaviour touched |
| **S3** | `grpc schema <uri>` verb (OD-2) | lists the type graph for any of the three discovery inputs, **including a service-less file**; text + machine-readable JSON (**the JSON output does not exist today** — `grpc discover` prints two text lines per operation, `internal/builtin/grpc.go:119-122`, and registers no format flag; the JSON shape precedent is JSON-RPC's `--doc` path, `internal/builtin/protocol.go:83-84`, marshalled at `:235`) | Small once S2 exists |
| **S4** | JSON Schema document emission | the non-executing consumer: emit `SchemaSet` as a JSON Schema (`$defs`/`$ref` for FQN references, `enum` arrays, `oneOf` for oneof groups) so the surface is *useful* (validation/docs/codegen) | Medium; **open decision OD-1** |

**Explicitly deferred by this spec** (each named with its reason so it is not silently dropped):
non-WKT import resolution (`google/api/annotations.proto` — §4, declined with (b)); real `.proto`
fixtures on disk (**0** exist today; the only fixture is the Go string constant
`testdata_test.go:29`) → **OD-3**; the missing demo leg `demo/demos/11-grpc-unary.sh` promised by
`PROTOCOLS.md:402` (`demo/demos/` holds `01`–`10` only, verified) → PROTO-016/018 follow-up, not new
scope; generated CLI commands from operations (`pkg/generator` → **0** `pkg/protocol` references) and
MCP gRPC wiring (`cmd/` → 0 references to `pkg/protocol/grpc`) → **separate gaps that exist with or
without `.proto` support**, per PROTO-011 §8.4.

### 2.7 Gate G-SCHEMA (acceptance: "its own gate")

Runnable, network-free per `PROTOCOLS.md §4.6`, runs under `go test -short ./...`:

1. **Two surfaces, one file.** The `msgs_only.proto` fixture (3 messages + 1 enum, no service) →
   `grpc discover` yields **0** operations (unchanged, `ingest.go:92` semantics) **and**
   `grpc schema` yields **4** types; assertion on both numbers in one test, proving the surfaces are
   distinct rather than the same walk under two names.
2. **Fidelity, against the measured baseline.** The rich fixture → assert: nested field carries the
   nested type's FQN; `repeated` field carries its element type; map field carries key + value types;
   the enum field carries **2** value names (`COLOR_UNSPECIFIED`, `RED`) where the measured baseline is
   **0**; the two oneof members are reported as one group; `google.protobuf.Timestamp` maps to the
   JSON form of the wire value, not `object`.
3. **Agent-facing regression (carried forward from PROTO-011 §10.4).** One focused test on the MCP
   input schema for a nested/enum input: a nested field must not be rendered as a bare
   `"type":"object"` with no properties, and an enum field must carry its values. This is the test
   PROTO-011 said was "worth one focused test before deciding whether §4.2 is a `.proto`-only
   concern" — S1's acceptance makes it a requirement, and it decides that question in favour of
   "it is a defect on the shipped path".
4. **Source/set/reflection equivalence.** The same fixture discovered as `.proto` source, as an
   `--include_imports` descriptor set, and over reflection (bufconn) produces equal schema sets —
   extending the existing parity pattern (`adapter_test.go:81`) rather than inventing a new one.
5. **Negative controls keep their class.** A syntax error and a missing file still fail with the same
   `VALIDATION_ERROR` class and exit 1 (measured, Appendix A).

---

## 3. Reading (c) — the bounded fix for descriptor sets (acceptance 4)

### 3.1 The contract, measured

`desc.CreateFileDescriptors` is documented to require "all transitive dependencies for every file"
(`jhump/protoreflect@v1.18.1/desc/convert.go:173-175`) and resolves each dependency **only** from the
supplied set, raising the library's `ErrNoSuchFile` on a miss (`convert.go:272-275`, the error type
being `github.com/jhump/protoreflect/internal`.ErrNoSuchFile) — there is **no** fallback
to the Go protobuf runtime registry. Muster passes the set straight through (`ingest.go:219`), so the
caller inherits that contract. Measured at this HEAD (Appendix A): exit **1**, message
`VALIDATION_ERROR: link descriptors from schema_nodeps.pb: no such file: "google/protobuf/timestamp.proto"`.

### 3.2 Decision D5 — resolve-then-remedy, in two branches, with a fail-early path

**Decision.** `decodeDescriptorSet` (`ingest.go:213-231`) gains a **pre-flight closure pass**, and
then one of exactly two outcomes:

1. **Resolvable missing dependency → resolve it silently.** Compute the set's dependency closure from
   the `FileDescriptorProto`s (no linking needed), and for each missing name look it up in
   `protoregistry.GlobalFiles` (the linked-in Go protobuf runtime registry — populated at `init()` by
   the generated `types/known/*` packages: `timestamppb` registers at
   `google.golang.org/protobuf@v1.36.12/types/known/timestamppb/timestamp.pb.go:334`), synthesize its
   `FileDescriptorProto` via `protodesc.ToFileDescriptorProto`, append it to the set, and re-link.
   This makes the descriptor-set path symmetric with the source path, which already resolves the same
   well-known types for free (`protoparse`'s standard imports — measured).

2. **Unresolvable missing dependency → fail early with a remedy.** `CodeValidation`, exit 1, **before**
   any operation is produced, naming every missing file, and naming the fix:
   `protoc --include_imports`, *or* supply the missing file(s) as `.proto` sources / an import-path
   URI (the `dir1:dir2=a.proto` form that already exists, `protosource.go:26-49`).

**This is not a proposal — it was prototyped at this HEAD** (transcript in Appendix A): the
`--include_imports`-less set that fails today was pre-flighted, `google/protobuf/timestamp.proto` was
synthesized from `GlobalFiles`, linking succeeded, and the result was **the same 1 operation**
(`schema.Desc-Do`) as the `--include_imports` set. The negative branch was measured too: a set missing
a *non*-registry dependency (`dep.proto`) is not resolvable, which is exactly the case the fail-early
path exists for. Feasibility therefore rests on the **existing direct dependency**
`google.golang.org/protobuf` (`go.mod:120`) and adds nothing to `go.mod`.

### 3.3 Decision D6 — union-over-all-files is documented, not changed

**Decision.** The measured union behaviour (§1, row 6) becomes an explicit sentence in the stage-1
contract (§5's seam table, §3.1 anchor): *a descriptor set is a discovery source of the union of the
operations declared by every file it contains; to restrict discovery to one file's services, pass that
file's `.proto` source.* **Reason:** the behaviour is principled and already reproducible on the
source path, and a "named-file only" filter would be a semantic change to a shipped adapter for no
operator-reported need. Documenting it costs one sentence; changing it costs a behaviour decision that
belongs to an ADOPT row's owner, not to this spec.

### 3.4 Gate G-CLOSURE

1. **The measured failure becomes a success.** `schema_nodeps.pb` (built with bare
   `--descriptor_set_out` from a file importing a well-known type — the exact input measured to fail)
   now discovers **1** operation, identical to `schema_all.pb` (`--include_imports`). Extend
   `adapter_test.go:81 TestDiscover_DescriptorSetParity` so the parities are source ↔ set-with-imports
   ↔ **set-without-imports**.
2. **The negative branch is loud and early.** A set missing an unresolvable dependency (fixture:
   `main_nodeps.pb`, missing `dep.proto` — measured unresolvable) fails with `CodeValidation`, exit 1,
   an error naming the missing file **and** the remedy, and **zero** operations returned (no partial
   discovery).
3. **No regression on the shipped paths.** Sets with no imports (`kv_nodeps`-shaped, measured 2 ops)
   and `--include_imports` sets are unchanged; `go test -short -count=1 ./pkg/protocol/... ./pkg/mcp/...`
   stays green (baseline: 7 packages `ok`).

---

## 4. Reading (b) — `.proto` as the WebDAV contract: **DECLINED** (acceptance 3)

**Decision: do not build (b). It is not a scope cut — it is a measurement-backed no.**

The measured reasoning, in the order it defeats the proposal:

1. **There is no WebDAV surface in this repo to describe.** A whole-tree `grep -ri webdav` returns hits
   **only** inside `.coding-hermes/board/tasks.jsonl` rows PROTO-010..PROTO-018 — 0 Go files, 0 specs,
   0 docs. And Muster is the **consumer** side (board PROTO-010: talk to a bunkerd WebDAV surface over
   HTTP/1.1, h2, h3); consuming WebDAV is HTTP request construction and version negotiation —
   PROTO-010/012/014/015 territory, not protobuf territory.
2. **`.proto` has no HTTP verb/path vocabulary on its own.** The only mechanism is the
   `google.api.http` option family, and the in-tree parser **cannot resolve**
   `google/api/annotations.proto` on this box — measured, exit 1:
   `import "google/api/annotations.proto" not found (searched [.])`. Nothing in the repo ships or
   fetches the `googleapis` include tree.
3. **Even with the includes present, it would still yield 0 operations.** Extraction is services-only
   (`ingest.go:92`) and never reads method options, so a `.proto`-described REST surface would need a
   `protoc-gen-openapi`/gnostic-style translation — a second toolchain for a problem the repo already
   solves with the OpenAPI adapter (kin-openapi, `PROTOCOLS.md:190-193`, `:213`).
4. **The right description format for an HTTP surface already exists here.** WebDAV endpoints
   (PROPFIND, `DAV:` headers, depth) are HTTP; Muster's HTTP description path is the OpenAPI adapter
   with the whole stage-2/3/4 machinery behind it. protobuf is only a payload encoding in that
   picture, which the gRPC adapter already covers for gRPC backends.

**Where the ruling belongs (so the decline is recorded where it will be read):** as a
**rejected-alternative note in the WebDAV spec (PROTO-012)**, not as a row in `PROTOCOLS.md`'s matrix —
the matrix decides *protocols* (`:211-227`), and (b) is neither a protocol nor a gRPC sub-feature. If
an RFC 4918-level machine-readable contract is ever wanted for the WebDAV surface, the format to reach
for is the one the repo ingests for HTTP (OpenAPI); `.proto` is measured not to be a viable carrier.
This also closes PROTO-011's open question 2 by decision rather than by deferral.

---

## 5. The seam this spec attaches to (acceptance 5)

Per-protocol seam as it stands: §1.2 stage 1 (`PROTOCOLS.md:49-61`), §3.1 contract (`:232-236`),
§3.2 model (`:238-241`). Where each piece of this spec attaches — **existing seam or new, with the
reason**:

| Work item | Attaches to (anchor) | Verdict |
|---|---|---|
| (c) descriptor-set closure fix (§3) | **§3.1 Discover/Ingest** — the gRPC row's "Discovery input" cell (`:55`) plus one bullet in §3.1's list stating the self-containment rule and the remedy. | **EXISTING SEAM.** It is an ingest-contract clarification on an already-adopted adapter; no new seam, no matrix change. |
| (c) union semantics (§3.3) | same cell, §3.1 | **EXISTING SEAM** — one sentence of contract. |
| Machine-readable output for the schema verb (S3) | **§4.4 Output rendering** (`:303-307`) + **§5.1 subcommand policy** (`:353-360`) | **EXISTING SEAM** — protocols mount under their group; adapters never print; the output layer formats. |
| (a) the schema surface (S1–S4) | **NEW SEAM, and not a matrix row**, attached at §3.2, entered from §1.2, rendered per §4.4: (i) §1.2 stage-1 prose gains the second producer shape (operation-producing adapters vs schema-producing ingesters); (ii) **§3.2 gains the schema-model contract** (the neutral type graph produced by an ingest source and consumed by rendering/validation); (iii) §4.4 covers its rendering. | **NEW SEAM REQUIRED.** Reason: the matrix (`:211-227`) is a table of *protocols* keyed by how operations are discovered and executed. A schema source produces **no operations** and has **no Executor**, so it fits neither stage 1's output contract (`[]*protocol.Operation`, `:236`) nor §3.2's operation-shaped model. Adding it as a matrix row would overload `Protocol` (against the §2 maintenance rule, `:228`); hiding it inside the gRPC adapter without a seam would bury a schema surface behind an RPC adapter. `Findings: PROTO-011 §7 reached the same conclusion; this spec makes the attachment points concrete.` |
| (b) WebDAV-as-`.proto` | **No seam — and none should be created.** | Belongs as a rejected-alternative note in PROTO-012 (§4). |

**Exact `PROTOCOLS.md` edits PROTO-016 must apply** (this row writes no product/doc files outside
its own deliverable — acceptance 6):

1. `:12` — stale premise correction: "`google.golang.org/protobuf` is only an indirect dependency" is
   **false** at this HEAD (PROTO-011 §3: `go list -m` → `direct=true`; `go.mod:120` carries no
   `// indirect` marker; `go mod why` → `pkg/protocol/grpc` → `encoding/protojson`). PROTO-011 §9.5
   records the same correction for `:214`'s protocompile claim (in the module graph only: 0
   `github.com/bufbuild/protocompile` packages in the CLI's build closure; what runs is protoreflect's
   vendored copy, 15 packages).
2. §1.2 stage-1 prose + §3.1 bullets — the two contract sentences from §3.2/§3.3 of this spec.
3. §3.2 — the schema-model subsection (D1/D3 types, FQN reference rule, no Executor).
4. §4.4 — one line: a schema document is rendered by the shared output layer like any other payload.
5. §5.1 — the new verb mounts under the existing `grpc` group (no new binary; §5.1's rule is
   authoritative and already generalises).
6. §6 — the two gate definitions of §6 of this spec, appended in §6's own table shape immediately
   after the gRPC streaming row (`:383`), labelled by board row (PROTO-016 / PROTO-018) so they are
   not mistaken for a new protocol's gate.
7. §7 — the demo plan's `demo/demos/11-grpc-unary.sh` (`:399-403`) is still **missing** (`demo/demos/`
   holds `01`–`10`, verified); if PROTO-016 adds a `.proto`/schema demo step it takes the next free
   number and must state which gate it serves.

---

## 6. Gates, in `PROTOCOLS.md §6` shape (for the §6 append in §5 above)

| Order | Work (board row) | Gate definition (runnable, network-free per §4.6, `go test -short`) |
|---|---|---|
| 2a | (c) descriptor-set closure (PROTO-016) | `schema_nodeps.pb` (bare `--descriptor_set_out`, measured failing today with exit 1) discovers the same operation set as `schema_all.pb`; parity test extended at `adapter_test.go:81`; unresolvable-dep set (`main_nodeps.pb`) fails with `CodeValidation` naming file + remedy and returns **0** operations; no-import and `--include_imports` sets unchanged. |
| 2b | (a) schema surface (PROTO-016 / PROTO-018) | service-less fixture: **0** operations **and** **4** types; fidelity assertions against the measured baseline (nested `Ref`, item type, map key/value, **2** enum value names vs **0** today, oneof group, WKT JSON form); MCP input-schema regression for nested/enum input; source ↔ set ↔ reflection equivalence for the schema set; syntax-error and missing-file negative controls keep `VALIDATION_ERROR`/exit 1. |

Both gates run in CI as normal Go tests (`PROTOCOLS.md:391`); neither needs a network, `protoc`, or
`grpcurl` at test time (fixtures are committed bytes — see OD-3).

---

## 7. Board consequences

| Row | Was | Becomes under this spec |
|---|---|---|
| **PROTO-016** (EXEC, was "may be documentation-only") | scope-agnostic | **Not** documentation-only: G-CLOSURE (§3) is real code, and G-SCHEMA S1–S3 are real code. Everything else in the `.proto` story **is** documentation (the shipped-surface table in §0.1 + the §5 doc edits). If OD-1 resolves to "operation client only", PROTO-016 reduces to the doc edits + G-CLOSURE and S1–S3 are dropped — the honest floor of this chain. |
| **PROTO-018** (TEST, "only if PROTO-016 produces code") | conditional | Produces code ⇒ the test row is live: G-CLOSURE + G-SCHEMA are its content. If PROTO-016 lands code only for G-CLOSURE, PROTO-018 closes against G-CLOSURE plus the PROTO-002 pattern and cites this spec for the deferred schema work. |
| **(new, if OD-1 says adopt S4)** | — | a design row for JSON Schema emission (S4), sized separately: PROTO-011 §8 states items 1–2 are close to `complexity=2` while the full schema surface is not, and the whole shipped gRPC surface is **1,517** production lines / **1,891** test lines / **77** test functions (measured `wc -l`), so a re-size is required before S4 is dispatched. |

---

## 8. Open decisions (acceptance 7)

Each carries a recommendation and the reason; nothing here is left implicit.

**OD-1 — Does the (a) schema surface ship in this chain at all, or is the chain reduced to
documentation + the closure fix?**
*Recommendation*: ship **S1–S3**, defer **S4**, and re-size PROTO-016 from complexity 2 to 3.
*Reason*: measured — the flatness is not a hypothetical schema-source concern: MCP input schemas are
built from the flat list today (`pkg/mcp/grpc_integration.go:57-77`), so S1 fixes a live agent-facing
defect, and S2/S3 are the smallest honest way to make a service-less `.proto` useful (0 operations,
4 types measured). S4 is the only piece whose value is speculative (no consumer asked for a JSON
Schema document yet) and the only piece large enough to need its own design row. The owner ruling that
PROTO-011 §10.1 asked for ("which reading Bane meant") is *this* decision; PROTO-011's own breakdown
(§8.1–§8.5) is the shape being recommended.

**OD-2 — Verb surface: a new `grpc schema <uri>` verb, or `grpc discover --types`?**
*Recommendation*: a **new verb**, mounted under the existing `grpc` group (no new binary; §5.1).
*Reason*: §1.3 freezes `Discoverer.Discover(ctx, uri) ([]*protocol.Operation, error)` (`:145-148`) and
§3.1 defines its output as operations (`:236`); overloading `discover` with types would either break a
frozen signature or smuggle a type graph through `Operation.Source`, which §3.2 forbids stage-3
consumers from interpreting (`:241`). The `grpc` group today holds exactly two verbs —
`discover <uri>` (`internal/builtin/grpc.go:90`) and `call <service-method>` (`:159`) — so the name is
free and the placement is already the sanctioned "protocol is a subcommand namespace" policy.

**OD-3 — Fixtures: real `.proto` files on disk, or keep the Go string constant?**
*Recommendation*: add on-disk fixtures under `pkg/protocol/grpc/testdata/` for the gate, **and** keep
the existing string constant (`testdata_test.go:29`, materialised by `protoSourceToTemp`,
`testdata_test.go:395`) for the isolation tests that must not touch the filesystem.
*Reason*: the repo has **0** `.proto` files on disk today (measured) and the gate needs committed
fixture *bytes*: a `--include_imports`-less descriptor set cannot be produced at test time without
`protoc`, and §4.6's network-free rule plus "no external services in CI" argue against a build-time
`protoc` dependency. Committing the three small `.pb` fixtures (measured sizes:
`schema_nodeps.pb` 542 B, `schema_all.pb` 800 B, `main_nodeps.pb` 121 B — all under 1 KB) removes
that dependency entirely; the string constant stays for tests whose whole point is not touching disk.

---

## 9. Evidence index — every number in this spec, and where it was measured

Numbers marked **R** were re-measured for this spec at HEAD `0765b0c0` (Appendix A); the rest are
quoted from PROTO-011's measurements (§/appendix cited). No number in this spec is estimated.

| Number | Value | Source |
|---|---|---|
| Ops from a 3-message + 1-enum service-less `.proto` | **0** (exit 0) | PROTO-011 §4.1; **R** |
| Ops from that file's descriptor set | **0** | PROTO-011 §4.1 |
| Ops from a 1-service / 2-method file | **2** | PROTO-011 §4.1 |
| Ops from the rich fixture (source and `--include_imports` set) | **1** | PROTO-011 §4.1; **R** |
| Bare `--descriptor_set_out` on a WKT-importing file | **exit 1**, `no such file: "google/protobuf/timestamp.proto"` | PROTO-011 §4.3; **R** |
| Fields surfaced / nested props / enum values / oneof groups on the rich message | **9 / 0 / 0 / 0** | PROTO-011 §4.2 |
| `grep -rn "Enum:" pkg/protocol/grpc/*.go` | **0** | PROTO-011 Appendix A; **R** |
| Union set discovers | **2** ops from 2 files | PROTO-011 §4.5; **R** |
| `google/api/annotations.proto` resolution | **fails**, `not found (searched [.])` | PROTO-011 §4.4; **R** |
| Prototype of the §3.2 remedy | synthesize WKT from registry → link OK → **1** op (same as `--include_imports`) | **R** (this spec; Appendix A) |
| Unresolvable missing dep (`dep.proto`) | registry miss → remedy branch required | **R** (this spec; Appendix A) |
| `pkg/generator` references to `pkg/protocol` | **0** | PROTO-011 §2; **R** |
| `.proto` files on disk in the repo | **0** | PROTO-011 §2; **R** |
| `demo/demos/` scripts | `01`–`10`; no `11-grpc-unary.sh` | **R** |
| gRPC surface size (production / test / test functions) | **1,517** / **1,891** / **77** (48 + 19 + 10) | PROTO-011 §2; **R** |
| Baseline suites on the gRPC/protocol surface | 7 packages `ok` (`go test -short -count=1 ./pkg/protocol/... ./pkg/mcp/... ./internal/builtin/`) | PROTO-011 §1 |
| `grpcurl` in the tree | **6** hits, **0** invocations, binary absent | PROTO-011 §2 |
| Negative controls (syntax error; missing file) | `VALIDATION_ERROR`, exit **1** in both cases | PROTO-011 §4.1; **R** |
| protobuf dependency class | **direct** (`go.mod:120`, `go list -m direct=true`) | PROTO-011 §3; PROTOCOLS.md:12 is stale |

---

## Appendix A — re-verification transcript at HEAD `0765b0c0`

Binary `/tmp/muster-probe-013/openapi-cli` built from worktree `muster-PROTO-013` (go1.26.6
linux/amd64); fixtures `/tmp/muster-probe-013/fx/` (identical in content to PROTO-011's Appendix B —
the same `msgs_only.proto`, `schema.proto`, `httpannot.proto`, `dep.proto`/`main.proto` shapes);
descriptor sets built with system `protoc` 3.21.12 (`libprotoc 3.21.12`).

```
$ openapi-cli grpc discover msgs_only.proto      # 3 messages + 1 enum, no service
No operations found.                             rc=0

$ openapi-cli grpc discover msgs_only.pb         # its descriptor set, no --include_imports
No operations found.                             rc=0

$ openapi-cli grpc discover schema.proto         # WKT import, no include path on disk
unary            schema.Desc-Do                  rc=0

$ openapi-cli grpc discover schema_all.pb        # protoc --include_imports
unary            schema.Desc-Do                  rc=0

$ openapi-cli grpc discover schema_nodeps.pb     # protoc WITHOUT --include_imports
Error: VALIDATION_ERROR: link descriptors from schema_nodeps.pb:
  no such file: "google/protobuf/timestamp.proto"   rc=1

$ openapi-cli grpc discover httpannot.proto      # google.api.http annotation shape
Error: VALIDATION_ERROR: parse proto files [httpannot.proto] (import paths []):
  httpannot.proto:3:8: import "google/api/annotations.proto" not found (searched [.]):
  open google/api/annotations.proto: no such file or directory   rc=1

$ openapi-cli grpc discover multi.pb             # main.proto + dep.proto, --include_imports
unary            dep.Extra-Ping
unary            mainpkg.Main-Do                 rc=0

$ openapi-cli grpc discover ".:.=main.proto,dep.proto"   # same union via the source URI form
unary            dep.Extra-Ping
unary            mainpkg.Main-Do                 rc=0

$ openapi-cli grpc discover broken.proto         # negative control: syntax error
Error: VALIDATION_ERROR: parse proto files [broken.proto] (import paths []):
  broken.proto:3:28: syntax error: expecting ';'    rc=1

$ openapi-cli grpc discover /tmp/does-not-exist.proto   # negative control: missing file
Error: VALIDATION_ERROR: parse proto files [/tmp/does-not-exist.proto] (import paths []):
  NOT_FOUND: read proto file /tmp/does-not-exist.proto:
  open /tmp/does-not-exist.proto: no such file or directory   rc=1
```

Remedy prototype (`probe013`, run in a `git archive HEAD` copy at
`/tmp/muster-copy-013-4096036`, so the worktree stays clean — the copy's packages are byte-identical
to this HEAD):

```
$ go run ./probe013 fx/schema_nodeps.pb          # the set measured to fail above
baseline: LINK FAILED -> no such file: "google/protobuf/timestamp.proto"
synthesized from GlobalFiles: [google/protobuf/timestamp.proto] (files in set now 2)
remedy: LINK OK linked_files=2 operations=1
  op=schema.Desc-Do

$ go run ./probe013 fx/main_nodeps.pb            # missing dep is NOT a well-known type
baseline: LINK FAILED -> no such file: "dep.proto"
unresolved dep (needs remedy text/import path): dep.proto
remedy: LINK FAILED -> no such file: "dep.proto"     exit status 1
```

Feasibility facts behind that prototype: `google.golang.org/protobuf/types/known/*` (10 packages
including `timestamppb`) is in the CLI's build closure (`go list -deps ./cmd/openapi-cli`), and the
generated WKT package registers its file in `protoregistry.GlobalFiles` at `init()`
(`types/known/timestamppb/timestamp.pb.go:334`).

**One text-level discrepancy vs PROTO-011's Appendix A, recorded rather than smoothed over:** the
missing-*file* error's trailing segment differs between the two measurement bases —
`read proto file X: no such file or directory` (PROTO-011) vs `read proto file X:
open X: no such file or directory` (here). The error **class** (`VALIDATION_ERROR`) and exit code
(**1**) agree, and nothing in this spec depends on that text. Error strings are explicitly **not**
part of any contract stated here; only classes, exit codes and counts are.

## Appendix B — the fixture inputs this spec's gates assume

`msgs_only.proto` (3 messages + 1 enum, **no service** — the file that yields 0 operations and, after
S2/S3, 4 types):

```proto
syntax = "proto3";
package only;
message Thing { string id = 1; int64 n = 2; }
message Other { string name = 1; }
enum Flavor { FLAVOR_UNSPECIFIED = 0; SWEET = 1; }
```

`schema.proto` (the rich message + one service; imports a well-known type with **no** include path on
disk) and `httpannot.proto` (the `google.api.http` shape (b) would require) are byte-identical to
PROTO-011 Appendix B and are not repeated here.

Descriptor sets assumed by G-CLOSURE, all committed as fixture bytes (OD-3):
`schema_nodeps.pb` (bare `--descriptor_set_out`; the measured failure), `schema_all.pb`
(`--include_imports`; the success baseline), `main_nodeps.pb` (bare; missing `dep.proto`; the
fail-early branch).
