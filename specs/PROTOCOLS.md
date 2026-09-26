# Protocol Adapters: Design Authority

**Status**: 🚧 DRAFT — design only, no protocol code landed  
**Version**: 1.1.0  
**Last Updated**: 2026-09-26  
**Component**: Architecture (all protocols)  
**Addresses**: PROTO-001 (board: protocol support roadmap PROTO-001..008), PROTO-012 (HTTP/2 + HTTP/3 client transports — §2 rows 15–17, §4.7, §6 orders 9–11)  
**Delta history**: 1.1.0 — HTTP transport version dimension added by PROTO-012 from PROTO-010's measurements (`docs/investigation/PROTO-010-http2-http3-client.md`); see [implementation/PROTO-012-http2-http3-transport.md](implementation/PROTO-012-http2-http3-transport.md) for the reasoning, the measurement log and the open decisions.  
**Related specs**: [CANONICAL-INTERFACES.md](CANONICAL-INTERFACES.md), [CANONICAL-INTERFACES.md §5 inventory](CANONICAL-INTERFACES.md), [MCP-PROTOCOL.md](MCP-PROTOCOL.md), [ERROR-HANDLING.md](implementation/ERROR-HANDLING.md), [RATE-LIMITING.md](RATE-LIMITING.md), [RELIABILITY-RETRY.md](RELIABILITY-RETRY.md), [RELIABILITY-TIMEOUT.md](RELIABILITY-TIMEOUT.md), [CLI-COMMAND-STRUCTURE.md](CLI-COMMAND-STRUCTURE.md), [TOOL-REGISTRATION.md](TOOL-REGISTRATION.md)

## Executive Summary

Muster today is an OpenAPI-3.x-only engine: `pkg/openapi` parses OpenAPI 3.x documents (kin-openapi v0.148.0), `pkg/generator` turns them into cobra commands, `pkg/mcp` turns them into MCP tools, and `pkg/client` executes HTTP. gRPC has since landed as the first non-OpenAPI adapter (`pkg/protocol/grpc`, PROTO-002 unary + PROTO-003 streaming), and `google.golang.org/protobuf` is a **direct** dependency of it (go.mod carries no `// indirect` marker; `go list -m` → `direct=true`; `go mod why` → `pkg/protocol/grpc` → `encoding/protojson`). GraphQL, JSON-RPC (client-side), AsyncAPI and SOAP client code are still absent.

This spec makes **OpenAPI the first adapter, not the engine**. It defines:

1. A **pipeline contract** every protocol adapter implements: *discover/ingest → operation model → command + MCP tool generation → transport executor* (§1, §3).
2. A **protocol support matrix** with a written verdict and reason per protocol, plus a deferral set (§2).
3. The **cross-cutting contract** all adapters must satisfy: auth mapping, error mapping to `pkg/errors.AppError`, timeout/retry/rate-limit reuse, output rendering, MCP agent safety, and network-free testing (§4).
4. The **naming / back-compat decision** for binaries and subcommands once `openapi-cli` becomes a misnomer (§5).
5. The **implementation order**, per-protocol acceptance gates, and the first protocol gate the `demo/run-all.sh` end-to-end gate will exercise (§6, §7).
6. The **HTTP transport version dimension** — HTTP/1.1 / HTTP/2 / HTTP/3 as stage-4 transport variants, not adapters (§1.2, §1.4, §3.5, rows 15–17), with the negotiation & observability contract that makes the version a reported fact rather than a silent downgrade (§4.7 N1–N8), gates at orders 9–11 (§6) and their demo legs (§7). Added in v1.1.0 from PROTO-010's measurements; reasoning in [implementation/PROTO-012-http2-http3-transport.md](implementation/PROTO-012-http2-http3-transport.md).

**Governing rule**: no protocol implementation row may start before its adapter satisfies this spec's pipeline and cross-cutting contracts on paper. Specs are the design authority BEFORE the wave.

## 1. The Adapter Seam

### 1.1 Why OpenAPI must stop being the engine

Today the OpenAPI types ARE the engine's types:

- `pkg/openapi.Operation` carries HTTP-only fields (`Path`, `Method`, `ServerURL`) and every stage downstream (`pkg/generator`, `pkg/mcp`, `internal/builtin/generate.go`, the e2e suites) consumes them directly.
- `pkg/generator.Generator.GenerateCommands(spec *openapi3.T)` and `pkg/mcp.Converter.ConvertToTools(doc *openapi3.T)` accept a kin-openapi `*openapi3.T` — the OpenAPI document itself — as their input contract.

Adding gRPC this way would mean faking an `openapi3.T` for a `.proto` file or duplicating the generator/MCP pipeline for a second protocol. Both are dead ends. The fix is a protocol-neutral **operation model** in the middle, with OpenAPI as the first producer into it.

### 1.2 The four pipeline stages

Every protocol adapter fills the same four stages. Stages are decoupled: an adapter can implement stage 1 (discovery) and stage 2 (model mapping) for spec-driven protocols without any transport, and stage 4 (executor) is replaceable per protocol.

```
┌─────────────────┐   ┌─────────────────┐   ┌──────────────────────────┐   ┌────────────────────┐
│ 1. Discover /   │   │ 2. Operation    │   │ 3. Command + MCP tool    │   │ 4. Transport       │
│    Ingest       │──▶│    Model        │──▶│    generation            │──▶│    Executor        │
│ (per protocol)  │   │ (protocol-      │   │ (pkg/generator, pkg/mcp  │   │ (per protocol)     │
│                 │   │  neutral)       │   │  — SHARED, not per-      │   │                    │
│                 │   │                 │   │   protocol)              │   │                    │
└─────────────────┘   └─────────────────┘   └──────────────────────────┘   └────────────────────┘
```

**Stage 1 — Discover / Ingest** (per protocol):

| Protocol | Discovery input |
|---|---|
| OpenAPI 3.x | spec file/URL (existing `internal/builtin/discover.go` fetch path) |
| Swagger/OpenAPI 2.0 | spec file/URL, converted (see §2) |
| gRPC | `.proto` source, compiled `FileDescriptorSet`, or server reflection (PROTO-002). A descriptor set is a source of the **union** of the operations declared by every file it contains; a set built without `protoc --include_imports` is closed from the protobuf runtime's well-known types before linking, and otherwise fails naming the missing file and the remedy (PROTO-016, §3.1) |
| JSON-RPC 2.0 | OpenRPC document; bare JSON-RPC endpoint with a hand-written manifest otherwise |
| GraphQL | introspection query result or SDL document |
| AsyncAPI 3.x | AsyncAPI document (channels/bindings) |
| SOAP | WSDL 1.1 or 2.0 document |

**Stage 2 — Operation model** (protocol-neutral; new, shared). **Stage 3 — Generation** (existing shared packages, extended). **Stage 4 — Transport executor** (per protocol).

**A protocol VERSION is a stage-4 property, not a protocol family.** HTTP/1.1, HTTP/2 and HTTP/3 are versions of one protocol: they produce no new operations, so they add no `Discoverer`, no `Protocol` constant and no adapter — `Registry.Register` rejects a second adapter for one `Protocol` (`pkg/protocol/registry.go:33-35`) and a version would re-emit a duplicate copy of every operation for no discovery gain. A version is selected by the transport installed on the shared `pkg/client.Client` (§4.7), which is also why version support never creates a second client or a second call path. The HTTP-family rows that add a version dimension are §2 rows 15–17.

### 1.3 Exact interface signatures (the contract)

These are the **canonical** signatures (rule: match exactly; see CANONICAL-INTERFACES.md usage rules). New types live in a new package `pkg/protocol` so the existing packages stay untouched; adapters implement the interfaces below. Method sets and signatures are frozen here — implementations MUST match exactly.

```go
// Package protocol defines the protocol-neutral operation model and the
// adapter seam every protocol backend implements.
package protocol

// Protocol identifies a supported protocol family.
type Protocol string

const (
    ProtocolOpenAPI  Protocol = "openapi"   // OpenAPI 3.x (first adapter)
    ProtocolGRPC     Protocol = "grpc"      // gRPC unary + streaming
    ProtocolJSONRPC  Protocol = "jsonrpc"   // JSON-RPC 2.0 / OpenRPC
    ProtocolGraphQL  Protocol = "graphql"   // GraphQL queries/mutations/subscriptions
    ProtocolAsyncAPI Protocol = "asyncapi"  // event-driven (Kafka/AMQP/MQTT/NATS/WebSocket)
    ProtocolSOAP     Protocol = "soap"      // SOAP 1.1/2.0 via WSDL
)

// OperationKind classifies an operation's interaction shape. MCP agent-safety
// rules (§4.5) key off this value.
type OperationKind int

const (
    KindUnary       OperationKind = iota // single request → single response (unbounded latency is the server's problem)
    KindServerStream                   // single request → response stream
    KindClientStream                   // request stream → single response
    KindBidiStream                     // request stream ↔ response stream
    KindEventSource                    // long-lived subscription/event source (no natural end)
)

// Parameter is the protocol-neutral input parameter. It generalizes
// pkg/openapi.Parameter (fields In/Name/Type/Required/Enum/Default/Example/
// Description) — the OpenAPI adapter maps 1:1 onto it.
type Parameter struct {
    Name        string
    In          string   // adapter-defined location: "path", "query", "header", "message-field", "graphql-variable", "payload", ...
    Type        string   // JSON-schema-style type string, same vocabulary pkg/mcp uses today
    Required    bool
    Enum        []interface{}
    Default     interface{}
    Example     interface{}
    Description string
}

// RequestBodyProperty generalizes pkg/openapi.RequestBodyProperty for
// message/request payloads.
type RequestBodyProperty struct {
    Name        string
    Type        string
    Description string
    Required    bool
    Enum        []interface{}
    Default     interface{}
}

// Operation is the protocol-neutral operation every adapter produces.
// It is the input to stage 3 and carries NO transport-specific fields:
// transport details live in the adapter's Executor, keyed by Op.ID.
type Operation struct {
    ID              string   // stable, unique per document; used as command/tool name seed
    Protocol        Protocol
    Kind            OperationKind
    Tags            []string
    Summary         string
    Description     string
    Parameters      []Parameter
    RequestBody     []RequestBodyProperty
    HasMultiPart    bool                   // false for all non-HTTP protocols today; kept for parity
    MultipartFields []RequestBodyProperty

    // Source records where the executor finds transport ground truth.
    // Adapters stamp it; stage-3 consumers MUST NOT interpret it.
    Source map[string]string // e.g. {"path": "/v1/pets", "method": "GET", "server": "https://..."} for OpenAPI;
                             // {"full_method": "/pkg.Service/Method", "descriptor_index": "3"} for gRPC
}

// Discoverer is stage 1: produce protocol-neutral operations from a
// protocol-specific discovery source. URI may be a file path, URL, or
// protocol-specific handle (e.g. reflection target).
type Discoverer interface {
    // Discover ingests the discovery source and returns operations.
    Discover(ctx context.Context, uri string) ([]*Operation, error)
}

// Executor is stage 4: run one operation and return the result as
// JSON-serializable data (the same shape the HTTP path feeds to output
// rendering today, see §4.4). Adapters own their transport, auth wiring,
// and retry/rate-limit integration (§4.2–4.4) behind this method.
type Executor interface {
    // Execute runs the named operation with the given arguments.
    // args keys match Operation.Parameters[].Name and RequestBody fields.
    // Returns the decoded response payload (map[string]interface{},
    // []interface{}, or scalar). For streaming kinds, Execute returns the
    // BOUNDED transcript per §4.5 — it never leaves a stream open.
    Execute(ctx context.Context, opID string, args map[string]interface{}) (interface{}, error)
}

// Adapter is the unit of registration. One Adapter per protocol family.
type Adapter interface {
    Protocol() Protocol
    Discoverer
    Executor
}
```

**Registry and wiring (additive):**

```go
// Registry resolves operations and executors by protocol. The default
// OpenAPI adapter is registered at startup; new adapters register alongside
// it and never change its behavior.
type Registry struct { /* ... */ }

func NewRegistry() *Registry
func (r *Registry) Register(a Adapter) error          // duplicate Protocol → error
func (r *Registry) Adapter(p Protocol) (Adapter, error) // unknown → AppError CodeNotFound
```

### 1.4 Which existing packages each stage reuses

Stage reuse table — **no new generation pipeline**:

| Stage | Reuses | How |
|---|---|---|
| 1 Discover | `pkg/openapi` (OpenAPI adapter only), `internal/builtin/discover.go` fetch helper (`fetchSpec(url)`), `pkg/client` for HTTP fetch | OpenAPI adapter wraps `openapi.Parser.Parse`/`ExtractOperations` and maps `openapi.Operation` → `protocol.Operation` (below). New adapters add their own Discoverer impl in their own package. |
| 2 Model | `pkg/openapi` types as the mapping source | `openapi.Operation{ID, Tags, Path, Method, Summary, Description, Parameters, RequestBody, ServerURL}` → `protocol.Operation` with `Source{"path","method","server"}`; `openapi.Parameter` → `protocol.Parameter` field-for-field. |
| 3 CLI commands | `pkg/generator` (existing `Config`, `NewGenerator`, `GenerateCommands`) | Extended to accept `[]*protocol.Operation` via a new method; the existing `GenerateCommands(spec *openapi3.T)` stays as-is (§5.2). |
| 3 MCP tools | `pkg/mcp` (`OpenAPITool`, `ToolRegistry`, `Converter.ConvertToTools`) | Same pattern: existing OpenAPI entry points preserved, new entry point accepts the neutral model. |
| 4 Transport | `pkg/client` (HTTP-family adapters incl. SOAP/JSON-RPC-over-HTTP/GraphQL-over-HTTP), `pkg/auth` (`TokenManager`, `Token`), `pkg/rate` (`NewTokenBucket`), `pkg/retry` (`Retrier`), `pkg/request.Builder` | Non-HTTP transports (gRPC, Kafka/AMQP/MQTT/NATS) get their own dialers but reuse the same auth/rate/retry composition rules (§4.2–4.4). HTTP-family adapters inherit the **HTTP version** from the shared `pkg/client.Client`: changing the version MUST NOT create a second client, a second call path, or a new adapter (§4.7) — the version is transport selection, not discovery. |

### 1.5 Back-compat guarantee for the seam

The existing call chain is **frozen**:

- `openapi.Parser.Parse(ctx, data, opts) (*ParseResult, error)` — unchanged.
- `openapi.Parser.ExtractOperations(spec *openapi3.T) ([]*Operation, error)` — unchanged.
- `generator.NewGenerator(config Config) (*Generator, error)` and `Generator.GenerateCommands(spec *openapi3.T) ([]*cobra.Command, error)` — unchanged.
- `mcp.Converter.ConvertToTools(doc *openapi3.T) ([]OpenAPITool, map[string]ExecutionConfig, error)` — unchanged.

Callers verified in-tree that depend on this: `internal/builtin/discover.go`, `internal/builtin/generate.go`, `pkg/mcp/openapi_integration.go`, `test/e2e/*`, `test/functional/workflows/*`, `test/production/*`. None of them change in this wave. Adding fields to `openapi.Operation`/`openapi.Parameter` is allowed (keyed literals only — the tree contains unkeyed literals in tests, so field additions must be appended and keyed literals used in new code); removing or renaming fields is forbidden.

## 2. Protocol Support Matrix

Verdicts: **ADOPT** (build, in the order of §6), **DEFER** (re-evaluate later, written reason), **CONVERT** (support via conversion into an adopted adapter, not a native one). Each verdict has a reason grounded in what exists in the tree today.

| # | Protocol | Verdict | Order | Reason |
|---|---|---|---|---|
| 1 | **OpenAPI 3.x** | ADOPT (exists; refit as adapter) | 0 — already done, refit is part of PROTO-002's seam work | The current engine. Becomes the first `Adapter` implementation: `pkg/openapi` stays the parser; `pkg/generator`, `pkg/mcp`, `pkg/client` stay the shared stages. No behavior change. |
| 2 | **gRPC — unary** | ADOPT | 1 | The original operator request. Strong Go ecosystem (`google.golang.org/grpc` v1.85 line, `github.com/bufbuild/protocompile` v0.14.1, `github.com/jhump/protoreflect` v1.18.1). Descriptor-set/reflection discovery is deterministic and testable in-process via `bufconn`. Unary first: it maps 1:1 onto the existing request→response command shape. |
| 3 | **gRPC — streaming (all 4 kinds)** | ADOPT — **landed (PROTO-003)** | 2 | `pkg/protocol/grpc/stream.go` + `openapi-cli grpc call` implement the bounded transcript of §4.5 for server-streaming, client-streaming and bidi (unary is PROTO-002). Bounds default to 1000 messages / 30 s; see the implementation note at the end of §4.5 for the exact status vocabulary and the one part of the gate still open (MCP opt-in for client/bidi). |
| 4 | **JSON-RPC 2.0 + OpenRPC** | ADOPT | 3 | Cheapest second protocol: the MCP server (`pkg/mcp/server.go`) already implements a JSON-RPC 2.0 codec for the server side; the client side reuses `pkg/client` over HTTP transports, plus `github.com/sourcegraph/jsonrpc2` v0.2.3 where a general client is needed. OpenRPC gives OpenAPI-grade discovery. Proves the seam with a second, non-protobuf adapter. |
| 5 | **GraphQL — queries/mutations** | ADOPT | 4 | Operator named it. Single-endpoint HTTP POST maps cleanly onto `pkg/client` + `pkg/request.Builder`; introspection/SDL parsing via `github.com/vektah/gqlparser/v2` v2.5.58 gives typed operations without codegen. |
| 6 | **GraphQL — subscriptions** | ADOPT, gated behind queries/mutations | 5 | Requires the event/subscription contract from the streaming/event work (same agent-safety rules as gRPC streaming + AsyncAPI). Not started until its gate. |
| 7 | **AsyncAPI 3.x — event-driven (Kafka/AMQP/MQTT/NATS/WebSocket)** | ADOPT, spec-gate first | 6 | Highest design risk: long-lived consumers, no request/response shape, per-broker client libs (`github.com/segmentio/kafka-go` v0.4.51, `github.com/rabbitmq/amqp091-go` v1.15.0, `github.com/eclipse/paho.mqtt.golang` v1.5.1, `github.com/nats-io/nats.go` v1.54.0, `github.com/gorilla/websocket` v1.5.3). PROTO-007 lands a spec gate BEFORE implementation, then one transport end-to-end (in-process test brokers where available). |
| 8 | **SOAP 1.1 / 2.0 via WSDL** | ADOPT (lowest priority of the adopted set) | 7 | Real-world demand exists and HTTP mapping is trivial (`pkg/client` + `pkg/request.Builder` build the envelope; WSDL parsing via `github.com/hooklift/gowsdl` v0.5.0 model or a light custom parser). Fault mapping is well-specified → maps to `CodeBadRequest`/`CodeInternal`. Last because demand is niche and envelope XML handling adds a serialization dependency. |
| 9 | **gRPC-Web / Connect (connectrpc.com/connect)** | DEFER | — | Browser-oriented dialects of gRPC. The CLI and MCP server are native clients — neither has a browser sandbox that forces the HTTP-framing workaround gRPC-Web exists for, and Connect's framing is only needed when a Connect-protocol server is the only exposure available. Every gRPC backend reachable via Connect today also speaks native gRPC (or plain JSON over HTTP, which the JSON-RPC/GraphQL adapters' HTTP layer already covers). The `connectrpc.com/connect` Go module (v1.21.0, verified available) makes a later native-Connect adapter cheap if a Connect-only backend shows up; keep the door open in the matrix rather than building ahead of demand. |
| 10 | **Swagger/OpenAPI 2.0** | CONVERT | 8 (parallel, non-blocking) | kin-openapi v0.148.0 already ships `openapi2conv` in the module cache (verified in GOMODCACHE). Conversion 2.0→3.x feeds the EXISTING OpenAPI adapter — no new adapter needed. PROTO-006 first measures current failure behavior of `Parser.Parse` on 2.0 documents, then adds the convert path. |
| 11 | **Thrift** | DEFER | — | Binary RPC with IDL but a fragmented ecosystem (thrift compiler, per-language codegen); far smaller CLI/MCP audience than gRPC; no in-tree dependency to build on. Re-evaluate only if gRPC adapter adoption shows demand for a second binary IDL. |
| 12 | **Cap'n Proto** | DEFER | — | No standard IDL-based discovery story usable for command generation (schema language exists, but no reflection/discovery convention comparable to gRPC reflection or OpenRPC); object-capability model conflicts with the stateless CLI command shape. |
| 13 | **FlatBuffers** | DEFER | — | A wire format, not an RPC protocol — no operation discovery surface of its own; it appears inside other protocols (e.g. gRPC with FlatBuffers payloads) which the gRPC adapter already covers at the transport level. |
| 14 | **tRPC** | DEFER | — | TypeScript-centric; its contract surface is a TS router, not a machine-readable discovery document. Clients can already be exposed through the JSON-RPC adapter if a tRPC server enables it. Re-evaluate if a discovery-document convention emerges. |
| 15 | **HTTP/2 client transport (h2)** | ADOPT — **no new dependency** (configuration + observability, not protocol code) | 9 | Already negotiated today, by accident: `pkg/client.NewClient` builds `&http.Client{Timeout: 30 * time.Second}` with a nil `Transport` (`pkg/client/client.go:101-103`), so requests land on `http.DefaultTransport`, whose `ForceAttemptHTTP2` is `true` — measured HTTP/2.0 on client and server (PROTO-010 `C1`, re-measured at PROTO-012), zero modules added; `golang.org/x/net/http2` is already in the CLI build closure (516 packages). The row exists because the negotiation is accidental: a hand-built `&http.Transport{}` drops to HTTP/1.1 against a server that offers h2 (PROTO-010 `C2`, re-measured), and there are **0** non-test `http.Transport` constructions in the tree today — the first one anyone adds silently removes h2. Cleartext is HTTP/1.1 regardless (no h2c). Gate: order 9; the version is read from the wire, never from configuration. |
| 16 | **HTTP/3 client transport (QUIC/h3)** | ADOPT — opt-in, **one new dependency** (`github.com/quic-go/quic-go` v0.63.0) | 10 | No stdlib HTTP/3 and no importable `x/net` HTTP/3 (`golang.org/x/net@v0.58.0`'s `http3/` is a `//go:linkname` shim to `net/http_test` with three unexported functions and no client `RoundTripper`). `http3.Transport` **is** an `http.RoundTripper` and was measured driving GET/PUT/PROPFIND/DELETE unchanged through the unmodified `pkg/client`. Measured cost at PROTO-012: module graph 173 → 178 (+5, of which exactly **2** compile — `quic-go`, `qpack`), binary +306,802 B default / **+143,360 B** stripped, module cache 3,594,997 B, MIT, `govulncheck` 0 reachable, `CGO_ENABLED=0` cross-compiles; no existing dependency version moves. Opt-in with h2 retained: **version selection is transport selection** and neither transport falls back to the other (measured both directions), so any fallback is explicit client code that reports itself (§4.7). |
| 17 | **WebDAV (RFC 4918) as an HTTP application profile** | ADOPT — as an **application profile**, not an adapter: it reuses rows 15/16 and the HTTP-family stage-4 path | 11 | Muster is the named WebDAV **consumer** of bunker's surface (bunker BFS-004), and the repo has no WebDAV content at all (`grep -ril "webdav\|propfind"` → 0 in `specs/`, 0 in `.memory-bank/`, 0 in any `.go` file, 0 in any markdown outside the two PROTO-010/011 investigation docs). RFC 4918's method set is a closed vocabulary with no discovery document, so stage 1 for this profile is a **built-in operation manifest** (a discovery document would be a second protocol engine; a hand-written per-endpoint manifest remains the fallback for non-RFC extensions). Measured gap it must close: `openapi-cli request PROPFIND <url> -b <propfind>` reaches the wire with **bodyLen=0** (`internal/builtin/request.go:144-146` attaches a body only for `POST`/`PUT`/`PATCH`) while `PUT` sends its 7 bytes — LOCK/PROPFIND bodies are silently dropped today. Gate: order 11. |

**Matrix maintenance rule**: adding a protocol row to this table requires (a) a written verdict + reason, (b) a board row with acceptance criteria, and (c) — if ADOPT — a gate definition per §6 before implementation starts.

## 3. Stage-by-Stage Contract

### 3.1 Discover / Ingest (stage 1)

- Input is a URI (file path, URL, or protocol-specific handle). HTTP(S) fetches go through the existing `pkg/client.Client` (proxy, TLS, tracing) — adapters do not open their own HTTP clients for discovery.
- Discovery MUST be read-only and side-effect free (no registration, no writes to the target service beyond what discovery inherently requires, e.g. a gRPC reflection RPC or a GraphQL introspection query).
- Output is `[]*protocol.Operation` with unique `ID`s within the adapter. IDs seed command names (§3.3) and MCP tool names (§3.4); adapters MUST produce IDs that survive the naming normalization in `pkg/generator` (`normalizeName`, `normalizeOperationID`).
- **A descriptor set must be self-contained** (PROTO-016, decision D5). `desc.CreateFileDescriptors` resolves imports only from the supplied set, so a set built without `protoc --include_imports` used to fail with a library-level error and no remedy. The gRPC adapter now (a) closes a set by synthesizing the missing imports the protobuf runtime registry knows — the well-known types, which makes the descriptor-set path symmetric with the `.proto`-source path, where `protoparse` carries them built in — and (b) otherwise fails EARLY with `CodeValidation`, before any operation is produced, naming every missing file and the remedy (`protoc --include_imports`, or supplying the missing files as `.proto` sources / an import-path URI of the form `dir1:dir2=a.proto,b.proto`).
- **A descriptor set is a union source** (PROTO-016, decision D6). Discovery returns the union of the operations declared by *every* file in the set, including files the operator did not name; to restrict discovery to one file's services, pass that file's `.proto` source instead.
- **A schema source is a second, non-operation producer** (PROTO-016, §3.2.1). Stage 1 has two producer shapes: adapters that produce `[]*protocol.Operation`, and *schema ingesters* that produce a `*protocol.SchemaSet` type graph. A `.proto` with no `service` declares no operations — that is correct, not a failure — and is still fully described by the schema surface (`openapi-cli grpc schema <uri>`).

### 3.2 Operation model (stage 2)

- Mapping is per-adapter and lives in the adapter's package (e.g. future `pkg/grpc` maps proto methods → `protocol.Operation`; `pkg/openapi` gains one mapping file). The neutral types (`protocol.Operation`, `protocol.Parameter`, `protocol.RequestBodyProperty`) are the ONLY cross-adapter contract.
- `Operation.Source` carries transport ground truth (paths, methods, descriptor indices). Stage-3 consumers never read `Source`; only the adapter's Executor does. This is the same ground-truth-on-the-command principle the generator already applies with its `muster/*` cobra annotations.
- **`RequestBodyProperty` carries depth-1 fidelity facts, additively** (PROTO-016, decision D4). A flat name/type pair could not express what a descriptor graph knows, and the loss was agent-facing: `pkg/mcp` builds MCP input schemas from the flat list. The additive fields are `Ref` (fully-qualified name of the referenced message type), `Format` (JSON form of a well-known type), `ItemType`/`ItemRef` (element of a repeated field), `Repeated`, `Map`, `MapKeyType`/`MapValueType`/`MapValueRef`, and `Oneof` (membership in a user-declared oneof group; a proto3 `optional` field's synthetic oneof is NOT reported). `Enum` — which existed but was never populated — now carries enum value **names**. An adapter with nothing to say leaves them zero, so this is additive for every existing consumer.

#### 3.2.1 The schema model (stage 2, second producer) — PROTO-016, decisions D1/D3

The operation model is operation-shaped and stays that way: a `.proto` with no `service` declares no operations and must not grow pseudo-operations. A schema surface answers a different question and gets its own neutral types, in `pkg/protocol`:

| Type | Shape |
|---|---|
| `protocol.SchemaSet` | `Types map[string]SchemaType` keyed by fully-qualified name, plus `Extensions []SchemaField` and `Source map[string]string` (the same `descriptors`/`host` vocabulary operations are stamped with) |
| `protocol.SchemaType` | `Name`, `Kind` (`message` \| `enum`), `Description`, `Fields []SchemaField`, `EnumValues []string` |
| `protocol.SchemaField` | the same depth-1 facts `RequestBodyProperty` carries, in the graph's own terms |

Rules:

- **FQN references, not recursion.** `Operation`'s shape is frozen for existing consumers, and `RequestBodyProperty` carries a `Ref` (an FQN), never a nested tree. A consumer resolves a `Ref` through a `SchemaSet`; `SchemaField.Ref` follows the same rule.
- **Producer.** The gRPC adapter's schema ingester reuses the *existing* loaders unchanged (`.proto` source, descriptor set, reflection) and walks `GetMessageTypes()`/`GetEnumTypes()` plus the files' extensions in addition to the services-only operation walk. No new dependency, no `protoc`, no second parser.
- **Optional consumer wiring.** `pkg/mcp.NewGRPCRegistry` gained the additive option `WithSchemaSet(*protocol.SchemaSet)`: with a set supplied, a nested field resolves through `$defs`; **without one, a nested field renders exactly as before** (`{"type":"object"}`), so no existing caller's schemas change silently.
- **No Executor.** A schema set is rendered, validated or emitted — never called. There is deliberately no `Executor` for it, which is why it is not a matrix row (§2's maintenance rule): it is not a protocol.
- **Not a new `Protocol` constant.** The set is a protobuf descriptor artifact of the same family as the gRPC adapter, so no matrix event is triggered.

### 3.3 Command generation (stage 3a — `pkg/generator`)

- New method on `Generator` (additive): `GenerateCommandsFromOps(config-validated generator, ops []*protocol.Operation) ([]*cobra.Command, error)` — exact signature to be finalized in the implementing row, but it MUST (a) accept the neutral model, (b) reuse the existing `buildOperationCommand`/naming/flag machinery unchanged, and (c) stamp the existing `muster/*` annotations (values sourced from `Operation.Source` where applicable).
- Flag generation derives from `Parameters`/`RequestBody` exactly as it does today from `openapi.Parameter`/`RequestBodyProperty`. `HasMultiPart` remains OpenAPI-only.

### 3.4 MCP tool generation (stage 3b — `pkg/mcp`)

- Tool names and JSON input schemas derive from the neutral model through the same conversion pipeline `Converter.ConvertToTools` uses today. New entry point (additive) accepts `[]*protocol.Operation`; existing `ConvertToTools(doc *openapi3.T)` is unchanged and remains the OpenAPI path.
- Registration flows through the existing `ToolRegistry`/`handlers.Registry` (`pkg/mcp/handlers/types.go`) unchanged.

### 3.5 Transport executor (stage 4)

- One Executor per adapter implementing `protocol.Executor.Execute(ctx, opID, args) (interface{}, error)`.
- HTTP-family transports (SOAP, JSON-RPC-over-HTTP, GraphQL) MUST compose `pkg/client.Client` (retry via `WithRetryConfig(retry.RetryConfig)`), `pkg/request.Builder` for request assembly, and `pkg/response.Handler` where the response is plain HTTP.
- **Version selection is the transport's, never the adapter's.** An HTTP-family adapter MUST NOT negotiate an HTTP version, set ALPN preferences, or retry across versions on its own; it executes through the shared `pkg/client.Client`, whose `http.RoundTripper` carries the version policy and reports the version actually used per §4.7. This is the same rule as §3.1's "adapters do not open their own HTTP clients", one layer down.
- Non-HTTP transports (gRPC, brokers) own their dialers but MUST apply the same cross-cutting composition (§4).

## 4. Cross-Cutting Contract (every adapter MUST satisfy)

### 4.1 Auth + metadata/header mapping

- Credential acquisition and storage are shared: `pkg/auth.TokenManager` (`NewTokenManager(storage TokenStorage, tokenURL, clientID, clientSecret string)`) and `pkg/auth.Token` (`AccessToken/RefreshToken/TokenType/ExpiresAt`, `IsExpired()` with its 60-second grace) are THE source of bearer/OAuth tokens for every protocol. Keychain-backed `TokenStorage` implementations already exist per platform.
- Per-protocol mapping (each adapter implements; documented in the adapter's spec/README):

| Protocol | Auth surface | Metadata mapping |
|---|---|---|
| OpenAPI (today) | `Authorization`/`api_key` headers per spec security schemes | headers via `pkg/request.Builder.WithHeader` |
| gRPC | OAuth2 token → `authorization: Bearer <token>` metadata key | `metadata.AppendToOutgoingContext`; mTLS via credentials config |
| JSON-RPC/GraphQL over HTTP | as OpenAPI (header-based) | HTTP headers |
| SOAP | WS-Security header or HTTP auth, adapter-configurable | SOAP envelope headers |
| AsyncAPI | per-broker SASL/TLS config | broker-specific connection options |

- Rule: an adapter NEVER invents a token store. It requests credentials from `pkg/auth` and maps them into its transport's native metadata/header channel.

### 4.2 Error mapping → `pkg/errors.AppError`

Every adapter error surfaces as an `*errors.AppError` (so `pkg/cli.ErrorFormatter` and MCP error rendering keep working unchanged). Existing codes (from `pkg/errors/errors.go`): `CodeBadRequest`, `CodeNotFound`, `CodeValidation`, `CodeGeneric`, `CodeDatabase`, `CodeUnauthorized`, `CodeInternal`, `CodeTimeout`, `CodeRateLimit`.

| Source | → AppError code | Notes |
|---|---|---|
| Any transport timeout (context deadline, gRPC `DeadlineExceeded`, HTTP timeout) | `CodeTimeout` | wrap with `errors.Wrap(err, errors.CodeTimeout, …)` |
| Rate limiter refusal / gRPC `ResourceExhausted` / HTTP 429 / broker quota errors | `CodeRateLimit` | preserve server `Retry-After`/retry hints in `WithDetail("retry_after", …)` |
| Auth failure (401, gRPC `Unauthenticated`, SASL failure) | `CodeUnauthorized` | refresh via `pkg/auth` may be attempted first by the adapter |
| Permission failure (403, gRPC `PermissionDenied`) | `CodeUnauthorized` | distinct from unauthenticated; same code, different detail |
| Malformed request rejected by server (400, gRPC `InvalidArgument`) | `CodeBadRequest` | |
| Target not found (404, gRPC `NotFound`) | `CodeNotFound` | |
| Schema/argument validation failures at the client | `CodeValidation` | before any bytes hit the wire |
| Protocol-internal decode/serialization failures | `CodeInternal` | includes protobuf/JSON/XML decode errors |
| A **required** HTTP transport version could not be used (client-side, after the wire attempt — §4.7 N5) | `CodeProtocolMismatch` (new constant) | details `requested`, `observed`, `reason`; never used for a version the policy did not require, and never as a substitute for a fallback that succeeded |
| Everything else | `CodeGeneric` | default; adapter MAY add `WithDetail("protocol", …)` |

Adding new codes is additive-safe today: the only `switch` over error codes in the tree is HTTP-status-based in `pkg/client/client.go` (`switch statusCode`), and `pkg/cli.ErrorFormatter.Format` prints the code string without enumerating it. New protocol codes (if ever needed) MUST be added to `pkg/errors` as new `Code` constants — never by string-matching in adapters.

Rule: adapters map errors AT THE TRANSPORT BOUNDARY (one mapping function per adapter, table-driven where possible), and MUST wrap the original error so `errors.IsError(err, code)` works and the cause chain is preserved for `--verbose` output.

### 4.3 Timeout / retry / rate-limit reuse

- **Timeouts**: every `Execute` call takes `ctx` and honors its deadline. Adapters translate the context deadline into their transport's native deadline mechanism (gRPC context deadline; HTTP `http.Request.WithContext`; broker operation timeouts). No adapter sets its own default timeout that overrides the caller's.
- **Retry**: HTTP-family adapters reuse `pkg/retry.Retrier` (`NewRetrier(config RetryConfig)`, `Execute(ctx, fn func(ctx) (interface{}, error))`) through `pkg/client`'s existing retry wiring (`client.WithRetryConfig`). gRPC gains retries via the transport's own retry policy or a `Retrier` wrapper around unary calls (idempotency-gated: only `KindUnary` ops default-retry; streaming ops never auto-retry — a partially-consumed stream cannot be replayed). Broker publishers follow the gRPC rule; consumers are out of retry scope.
- **Rate limit**: `pkg/rate.NewTokenBucket(ratePerSec, burst int)` (wrapping `pkg/auth/ratelimit.TokenBucket` with `Allow()`/`AllowN(n)`) is THE limiter. HTTP-family adapters continue through `pkg/client.RateLimitRoundTripper`. Non-HTTP adapters wrap their `Execute` with the same `TokenBucket.AllowN(1)` before dial. Rate-limit rejections surface as `CodeRateLimit` per §4.2.
- Composition order is fixed: rate limit → auth → timeout/deadline → single attempt (retries loop the whole order, matching `pkg/client`'s existing `doWithRetry` semantics).

### 4.4 Output rendering

- `Execute` returns JSON-serializable data (`map[string]interface{}`, `[]interface{}`, scalars). Rendering is shared, not per-adapter: the existing formats (`table`, `json`, `yaml` — `generator.Config.SupportedFormats`, `printTableOutput` tabwriter path, and `internal/builtin/request.go` `-o` handling) render whatever `Execute` returns.
- Streaming ops return the bounded transcript (§4.5) — rendering applies to the transcript object, not a live stream.
- A **schema document** (`*protocol.SchemaSet`, §3.2.1) is another payload for the same path: the adapter never prints it, the output layer formats it (the `grpc schema` verb renders text by default and the set as JSON with `-o json`).
- Rule: adding a protocol NEVER adds a new output format, and an adapter never writes to stdout directly.

### 4.5 MCP agent safety (unbounded / streaming / bidi operations)

MCP tool calls are agent-facing: a tool that never returns traps the agent loop. Rules:

1. **Every MCP-exposed operation returns, or is bounded.** `KindUnary` ops are naturally bounded. The other kinds get a transcript envelope:
   ```json
   {
     "kind": "server-stream",
     "status": "complete | cancelled | truncated",
     "items": [ ... up to maxItems events ... ],
     "items_truncated": false,
     "trailer": { ... adapter-defined final metadata ... }
   }
   ```
2. **Boundaries are configured, not unlimited**: max items, max wall-clock, max bytes (per-operation defaults; MCP layer enforces hard caps). Hitting a boundary sets `status: "truncated"`, `items_truncated: true` — the agent is told, never left hanging.
3. **`KindBidi` / `KindClientStream` ops are NOT auto-exposed as MCP tools.** They require an explicit per-operation opt-in (config allow-list) because they cannot be modeled as a single JSON args→result call. CLI exposure is fine (interactive flags), MCP exposure is deliberate only.
4. **`KindEventSource` ops (GraphQL subscriptions, AsyncAPI consumers) are never long-lived inside an MCP tool call.** The MCP tool performs a bounded collect: subscribe, collect up to the caps, unsubscribe, return the transcript. Long-lived consumption is a daemon/DSL use case (`pkg/dsl`, daemon), not an MCP tool.
5. **Errors inside streams** terminate the transcript with `status` reflecting the cause and map to AppError codes per §4.2; the tool result is still a well-formed transcript, not a hung call.

**Implementation note — gRPC adapter (PROTO-003, `pkg/protocol/grpc/stream.go`)**

- Rule 1's envelope is `StreamTranscript`: `kind` (`server-stream` | `client-stream` | `bidi-stream`), `status`, `items` (always a JSON array), `items_truncated`, and `trailer` carrying `received`, `messages_sent`, `max_messages`, `duration_ms`, plus — when applicable — `limit_reason` (`max-messages` | `max-duration`), `metadata` (the gRPC trailer metadata) and `error`.
- Bounds and defaults: `--max-messages` 1000 and `--timeout` 30s, settable per invocation (CLI flags) and per adapter (`Adapter.WithStreamLimits`). A non-positive value selects the default: **there is deliberately no unbounded mode**.
- Status vocabulary as implemented: `complete`; `truncated` (a bound fired — clean stop, no error, `items_truncated: true`); `cancelled` (the CALLER's context was cancelled or its deadline passed — mapped to `CodeTimeout` per §4.2, partial items preserved); `error` (the server failed mid-stream — §4.2-coded error returned alongside the partial transcript, rule 5).
- Client-streaming: the cap truncates the REQUEST stream, and the server's answer to that capped stream is still collected and reported.
- Backpressure: requests are sent sequentially under transport flow control (no client-side buffering of the stream), and bidi send/collect run concurrently so a server that only answers while being read cannot deadlock the client. Streaming calls are never auto-retried (§4.3).
- **Open part of the gate**: rule 3's MCP opt-in for `KindClientStream`/`KindBidi` ops is not implemented yet — `pkg/mcp.GRPCRegistry` still lists every discovered operation as a tool (`TestGRPCRegistry_StreamingToolListed`). Server-streaming is safe to expose today because §4.5 bounds its result; the client-stream/bidi opt-in remains open work.

### 4.6 Network-free per-protocol test strategy (CI MUST NOT need external services)

Every adapter lands with, at minimum:

1. **In-process protocol servers** — real protocol, no real network:
   - gRPC: server on `bufconn` (`google.golang.org/grpc/test/bufconn` — listener over an in-memory buffer; dialer `grpc.WithContextDialer`). This is the canonical Go in-process gRPC test pattern.
   - HTTP-family (SOAP, JSON-RPC, GraphQL): `net/http/httptest.Server` bound to `127.0.0.1` (loopback only, no external network).
   - HTTP/3: an in-process `http3.Server` on a **UDP loopback** socket plus an `http3.Transport` is the h3 analogue of `httptest`. QUIC mandates TLS 1.3, so the fixture needs a real (self-signed) certificate — a bare listener cannot work — and the **negative cells** (an h2 transport against an h3-only endpoint, and the reverse) belong to the pattern, because nothing in either transport falls back to the other (§4.7 N6).
   - WebSocket: `httptest` server doing the upgrade locally.
   - AsyncAPI transports: in-process brokers where Go natives exist (e.g. in-memory NATS server `github.com/nats-io/nats-server/v2` `natsserver.Run`, embedded Kafka via testcontainers is CI-heavy — the AsyncAPI gate (PROTO-007) decides per-transport; loopback brokers are the default).
2. **One conformance comparison per protocol against a reference client** — proves our client speaks the same wire protocol as the ecosystem tool, not just that our client agrees with our server:
   - gRPC: `github.com/fullstorydev/grpcurl` v1.9.4 used as a LIBRARY (`github.com/fullstorydev/grpcurl` package APIs) or as a binary in tests that run it against the same in-process `bufconn`-served listener via a local port, comparing its result to our executor's result for the same call. (Network-free: the server is in-process; grpcurl connects over loopback.)
   - SOAP/JSON-RPC/GraphQL: golden-file comparison of our request bytes against a reference implementation's request bytes (e.g. recorded curl/known-good client output), plus response-byte decode comparison.
3. **The three-test rule from AGENTS.md** applies per adapter: unit (mapping functions), integration (adapter + shared stages wired in-process), e2e (`demo/test-suites/` scenario over loopback).

### 4.7 HTTP transport version negotiation & observability (HTTP/1.1 / HTTP/2 / HTTP/3)

Applies to the HTTP-family adapters (matrix rows 1, 4, 5, 8, 15, 16, 17). Non-HTTP transports are unaffected. The reasoning, the measurement log and the open decisions live in [implementation/PROTO-012-http2-http3-transport.md](implementation/PROTO-012-http2-http3-transport.md); this section is the binding contract.

**N1 — Truth source.** The negotiated HTTP version is the version the response was actually carried on, recorded from the response returned by the **base `http.RoundTripper`** — the one that performs the exchange — at the moment it returns, before any wrapper may rebuild the response. `http.Response.Proto` as seen through `pkg/client` is **not** a truth source today: the response cache rebuilds 2xx GET responses with `Proto: req.Proto` and a header set holding only `Content-Type` (`pkg/client/cache.go:110-126`, called at `:66`/`:94`), and the rate limiter synthesizes a local `429` the same way (`pkg/client/ratelimit.go:50-64`). `req.Proto` is `"HTTP/1.1"` for an outgoing request by construction (measured), so both sites fabricate `HTTP/1.1` regardless of what the network did; the cache's forgery is also verb-dependent (non-GET verbs pass through untouched, `cache.go:56-58`). Product surfaces MUST read the recorded value, never the wrapper-built response. DF-041 must still be fixed for response-object and header fidelity — no rule in this section may be weakened to depend on it.

**N2 — Attachment.** A version is selected by installing a transport on the one shared client: `WithTransport(rt http.RoundTripper)` applies the base transport **inside** the option pipeline, and the version recorder wraps that base, i.e. below `WithRateLimiter`/`WithCache`, so every exchange that reaches the network is recorded. Callers MUST NOT change the version via `Client.SetHTTPClient`: it replaces the whole `*http.Client` and discards the wrapper chain the options installed (measured: `WithCache`+`WithRateLimiter` then `SetHTTPClient` takes wire hits from 1 → 2 and turns `*client.CacheRoundTripper` back into `*http.Transport`; defect row DF-042).

**N3 — Reported record.** Every HTTP-family execution reports (library: the recorder; CLI: the verbose diagnostic surface; MCP: the error details of N5):

| Field | Values | Meaning |
|---|---|---|
| `observed` | `HTTP/1.1` \| `HTTP/2.0` \| `HTTP/3.0` \| `""` | the wire version; **empty** when no exchange took place |
| `source` | `wire` \| `cache` \| `local` | where the response came from |
| `requested` | `auto` \| `1.1` \| `2` \| `3` | the policy in force (N4) |
| `satisfied` | bool | the policy was met (`auto` is satisfied by any `observed` value from the wire) |
| `fallback` | `{from, to, reason}` or absent | present only when a preferred/required version was attempted and another was used |

`observed` is never guessed: a cache-served response reports `source: cache` plus the version stored with the cache entry, or `""` when the entry carries none — **never a default of `HTTP/1.1`**; a locally refused request reports `source: local` with `observed: ""`. A synthesizing wrapper MUST NOT stamp `req.Proto` as an observed version.

**N4 — Policy.** `request.http_version` (config; the `--http-version` flag overrides) takes `auto` (**default** — today's behaviour), `1.1`, `2`, `3`. `auto` never fails on version. `1.1`/`2` REQUIRE the named version. `3` prefers HTTP/3 and MAY fall back to HTTP/2 — the fallback is always reported.

**N5 — A version that cannot be used is a reported fact, never a silent downgrade.**

| Server offers | Policy | Outcome |
|---|---|---|
| `http/1.1` only | `auto` (default) | succeeds; `observed=HTTP/1.1`, `satisfied=true`; the fact is reported — older clients keep working, and the caller is told which version carried the request |
| `http/1.1` only | `2` or `3` | FAILS: `CodeProtocolMismatch` with `requested`, `observed`, `reason="server-offered-http/1.1"` |
| h2 over TLS | `auto` / `2` | succeeds; `observed=HTTP/2.0` |
| h3 reachable | `3` | succeeds; `observed=HTTP/3.0` |
| h3 unreachable (UDP dropped, no QUIC, peer TCP-only) | `3` | succeeds over h2 with `fallback{from:HTTP/3.0, to:HTTP/2.0, reason:<connection-layer class>}` — the fallback is explicit client code and its occurrence is the report |
| plaintext `http://` | any | `observed=HTTP/1.1` by construction (no h2c in the stdlib; QUIC requires TLS 1.3); policy `2`/`3` FAILS with `reason="plaintext-endpoint"` |

Classification: a **QUIC connection-layer failure** (dial refused, handshake timeout, `CRYPTO_ERROR`) is a fallback reason; an **HTTP status** is never one. `fallback` means another version carried the request; `CodeProtocolMismatch` means no permitted version could — the two are never interchanged.

**N6 — No negotiation inside a transport, and Alt-Svc is out of band.** Neither `http.Transport` nor `http3.Transport` falls back to the other (measured in both directions: `dial tcp …: connection refused` and `timeout: no recent network activity`), and **nothing consumes `Alt-Svc`** — neither the stdlib nor quic-go's client (which only sets it server-side). Cross-version behaviour is therefore client code (N4/N5), and h3 selection is explicit configuration, never automatic discovery.

**N7 — Surfaces.** Library: the recorder. CLI: an `HTTP: <version>` line on **stderr**, alongside the existing `Request:`/`Status:` diagnostics (`internal/builtin/request.go:195-213`) — never stdout, whose payload fidelity is frozen (§4.4, §5.3) — plus the `--http-version` flag. MCP: the result payload stays the decoded body (§4.4); a required-version failure is an `AppError` carrying the N3 record, so an agent never receives a silent downgrade.

**N8 — Error code.** `CodeProtocolMismatch`, a new additive `pkg/errors` `Code` constant, means "a required HTTP transport version could not be used" and carries `requested`/`observed`/`reason`; it is mapped at the transport boundary like every other adapter error (§4.2) and is never used for a version the policy did not require.

## 5. Naming & Back-Compat

### 5.1 The misnomer problem

`openapi-cli`, `openapi-mcp`, `openapi-daemon` (Makefile `BINARY_NAME`/`CLI_BINARY`/`MCP_BINARY`/`DAEMON_BINARY`) name the binaries after ONE protocol. After the second protocol lands, the names lie. Decision (binding for PROTO-002 onward):

- **Keep the binary names.** `openapi-cli` stays the binary name for v1. Renaming binaries breaks every doc, script, packaging file (`debian/`, `systemd/`, `Dockerfile`, `demo/run-all.sh` builds `bin/openapi-cli`), and user muscle memory for zero functional gain. The binary is the product; the protocol is a subcommand concern.
- **The protocol is a SUBCOMMAND NAMESPACE, not a binary.** OpenAPI operations stay at the root exactly as today (root verbs: `discover`, `generate`, `request`, `auth`, `config`, `daemon`, `dsl`, `modules`, `webhook`, `tenant`, `audit`, `inventory`, `version`, `completion`, `init`). New protocols mount under a protocol group: `openapi-cli grpc <api> <method> …`, `openapi-cli graphql <endpoint> <query> …`, etc. This keeps the OpenAPI UX byte-for-byte compatible and makes "which protocol" explicit where it is new.
- **Binary rename (to `muster` etc.) is a separate, future decision** — tracked on the board if ever filed, NOT bundled into protocol work. When it happens, old names become permanent aliases (see alias policy below).
- **A protocol group's verb set grows additively** and the verb name is free until taken. The `grpc` group today holds three verbs — `discover <uri>`, `schema <uri>` (PROTO-016: the type graph, including for a source with no service) and `call <service-method>` — all mounted under the existing group with no new binary. A new *capability* of the same protocol is a new verb in the group, not a new group and not a change to a frozen signature (`Discoverer.Discover` is frozen at §1.3, which is why types travel through `schema`, not through `discover --types`).

### 5.2 Alias policy

- Existing commands never move and never gain a conflicting alias: the root `openapi-*` verbs are frozen.
- New protocol groups are additive subcommands; they cannot shadow existing verbs (no `openapi-cli grpc` exists today — verified against `internal/builtin/root.go`).
- If/when a binary rename ever happens: old name → symlink or wrapper alias kept for at least one major version, with a deprecation notice on `--help`, and `openapi-cli X` forwarding to the new binary with identical args and exit codes.

### 5.3 What must keep working (frozen surface)

- All `pkg/generator`/`pkg/openapi`/`pkg/mcp` signatures listed in §1.5.
- Root command set in `internal/builtin/root.go` (`NewRootCommandWithAudit`) and every existing verb's flags/args semantics.
- `demo/run-all.sh` checks (builds `bin/openapi-cli`, `bin/openapi-mcp`, `bin/openapi-daemon`; runs `demo/demos/[0-9]*.sh`).
- Docs paths: `README.md` (`openapi-cli` usage lines), `docs/DOCS-CLI-REFERENCE.md`, `docs/DOCS-MCP-SETUP.md`, `docs/DOCS-OPENAPI-QUICKSTART.md` — OpenAPI examples keep running verbatim.
- MCP tool names produced from OpenAPI specs — byte-identical (agents have them memorized; renaming breaks recorded sessions).

## 6. Implementation Order & Gates

Each ADOPT row lands behind a gate. A gate is a concrete, runnable check — green gate is the acceptance criterion for the corresponding board row; the demo gate (§7) additionally exercises the protocol end-to-end through the real binaries.

| Order | Protocol (board row) | Gate definition (must be runnable, network-free per §4.6) |
|---|---|---|
| 0 | OpenAPI refit as Adapter (part of PROTO-002's seam landing) | Existing full suite green + `demo/run-all.sh` green + a new unit test proving `openapi → protocol.Operation → GenerateCommandsFromOps` produces byte-identical commands to today's path on the petstore demo spec |
| 1 | gRPC unary (PROTO-002) | In-process: `.proto` with ≥2 unary methods served over `bufconn`; (a) discover via descriptor-set finds all methods; (b) discover via reflection on the in-process server finds the same set; (c) invoke each method via CLI flag-parsed args; (d) MCP tool call returns the same payload; (e) error mapping: injected `InvalidArgument` → `CodeBadRequest`, `DeadlineExceeded` → `CodeTimeout`; (f) grpcurl conformance: same invocation returns same result |
| 2 | gRPC streaming (PROTO-003) | Gate of 1 green + all four stream kinds exercised over `bufconn` with the §4.5 transcript envelope; truncation proof (cap < stream length → `items_truncated: true`); MCP exposure of bidi requires explicit opt-in config (negative test: without opt-in, bidi op is absent from tool list) |
| 2a | **Descriptor-set closure (PROTO-016, G-CLOSURE)** — a gate of the gRPC adapter, not a new protocol | `schema_nodeps.pb` (bare `--descriptor_set_out` over a file importing a well-known type; failed with exit 1 before PROTO-016) discovers the same operation set as `schema_all.pb`; source ↔ `--include_imports` set ↔ bare set parity; a set whose import is NOT a well-known type (`main_nodeps.pb`) fails with `CodeValidation` naming the missing file **and** the remedy and returns **0** operations; no-import and `--include_imports` sets unchanged. Runnable: `go test -short ./pkg/protocol/grpc/` |
| 2b | **The `.proto` schema surface (PROTO-016, G-SCHEMA; test row PROTO-018)** — also a gate of the gRPC adapter | The service-less fixture yields **0** operations **and** **4** types in one test (the two surfaces are distinct); depth-1 fidelity against the measured baseline (nested `Ref` resolves in the set, `repeated` item type + ref, map key/value, **2** enum value names where the baseline was **0**, the two oneof members reported as one group, a well-known type mapped to its JSON form); the MCP input schema for a nested/enum input is not a bare `"type":"object"` and carries the enum values (with no schema set supplied, the previous rendering is retained); the three discovery inputs (source, `--include_imports` set, reflection over `bufconn`) agree; negative controls keep `VALIDATION_ERROR`/exit 1. Runnable: `go test -short ./pkg/protocol/grpc/ ./pkg/mcp/ ./internal/builtin/` |
| 3 | JSON-RPC 2.0 / OpenRPC (PROTO-004) | OpenRPC doc → operations → commands/tools; invoke over `httptest` loopback server; JSON-RPC error object → AppError code mapping table test; conformance vs a reference JSON-RPC client |
| 4 | GraphQL queries/mutations (PROTO-005) | Introspection AND SDL ingestion paths each produce operations; invoke against in-process `httptest` GraphQL server; error-path mapping (`errors` array in response); conformance vs reference client query bytes |
| 5 | GraphQL subscriptions | Gate of 4 green + bounded-collect subscription over in-process WS server per §4.5(4) |
| 6 | AsyncAPI 3.x (PROTO-007) | **Spec gate first**: AsyncAPI ingestion spec lands and is reviewed BEFORE implementation; then ONE transport end-to-end over an in-process broker with §4.5(4) bounded-collect semantics; remaining transports follow one at a time |
| 7 | SOAP/WSDL (PROTO-008) | WSDL 1.1 doc → operations; envelope bytes golden-compared against reference; SOAP Fault → AppError mapping test; loopback `httptest` server round-trip |
| 8 | OpenAPI 2.0 conversion (PROTO-006) | Measure-and-document current `Parser.Parse` behavior on 2.0 docs (named in PROTO-006 evidence) → then `openapi2conv` path with a conversion parity test (2.0 doc converted → operations equal to hand-upgraded 3.x doc's operations) |
| 9 | HTTP/2 client transport (PROTO-014) | In-process `httptest` TLS server, `EnableHTTP2 = true`, ALPN offering `h2`, loopback only. (a) the version recorded per §4.7 N1 — **not** read from a wrapper-built response — equals `HTTP/2.0` on both sides for GET and for `PROPFIND`; (b) a 1 MiB PUT arrives intact (server-observed length 1048576); (c) **reporting control**: a server offering only `http/1.1` still serves the request and the recorded version is `HTTP/1.1` with `satisfied: true` under `auto` — the version comes from the wire, never from configuration; (d) **negative cell**: a hand-built `&http.Transport{}` without `ForceAttemptHTTP2` records `HTTP/1.1`, so the assertion can actually fail (measured — this is the anti-gaming cell); (e) **policy cell**: `--http-version 2` against the h1.1-only server fails with `CodeProtocolMismatch` carrying `requested`/`observed`/`reason`; (f) **cache cell**: with `WithCache` enabled, the recorded version for a 2xx GET is still the wire version (today that path's `resp.Proto` is `HTTP/1.1` — DF-041 — so the cell proves the contract does not depend on it). |
| 10 | HTTP/3 client transport (PROTO-015) | In-process `http3.Server` on a **UDP loopback** socket with an in-process self-signed certificate (QUIC mandates TLS 1.3). (a) `HTTP/3.0` recorded on both sides for GET/PUT/PROPFIND/DELETE; (b) context deadline → `CodeTimeout`; (c) **negative cells (measured)**: an h2 transport against an h3-only endpoint fails (dial refused) and an h3 transport against a TCP-only endpoint fails (`timeout: no recent network activity`) — the proof that no cross-version fallback exists inside either transport; (d) **fallback cell**: policy `3` with QUIC unreachable completes over h2 **and reports** `fallback{from,to,reason}` classified as a connection-layer failure, never as a silent success; (e) a TLS-1.2-capped config fails with `CRYPTO_ERROR 0x150` rather than hanging; (f) a plaintext URL can never satisfy policy `3`. |
| 11 | WebDAV application profile (matrix row 17) | In-process `httptest` WebDAV fixture. A `PROPFIND` carrying a `<propfind>` body reaches the wire with a **non-zero** body (today measured `bodyLen=0`, `internal/builtin/request.go:144-146` attaches a body only for POST/PUT/PATCH), likewise a `LOCK` body; a `Depth` header is sent when requested; operations are produced from the built-in manifest with stable IDs; a peer extension refusal surfaces with the peer's own vocabulary preserved in the error details (cross-repo contract, bunker BFS-004). |

Rules: gates run in CI as Go tests (`go test -short ./...` covers them via the standard suite; no special harness). No gate, no implementation. A gate that cannot run network-free fails review.

## 7. Demo Plan (first protocol gate)

`./demo/run-all.sh` is the real end-to-end gate (AGENTS.md; it builds `bin/openapi-cli`, `bin/openapi-mcp`, `bin/openapi-daemon`, starts `demo/services/` (Python services: `bytestash`, `chainlink`, `inkwell`, `pulse`, `task-forge`), seeds data, then runs `demo/demos/[0-9]*.sh` counting pass/fail).

**First protocol gate: gRPC unary (order 1).**

When PROTO-002 lands, the demo gains:

- A gRPC demo service added to the demo stack — a small Go binary in `demo/services/` serving a `.proto` with ≥2 unary methods over a local port (in-process for tests, real listener for the demo gate).
- A new numbered demo script `demo/demos/11-grpc-unary.sh` (follows the existing `[0-9]*.sh` convention `run-all.sh` globs automatically) that: discovers methods from a descriptor set → invokes method 1 via `openapi-cli grpc …` → invokes method 2 via the MCP tool path → asserts both payloads and exercises one injected error mapped to the expected `AppError` code.
- `demo/run-all.sh` picks it up with zero script changes (the glob handles it).

**Landed since (PROTO-016): `demo/demos/11-proto-schema.sh`.** The gRPC unary demo leg above is still missing, so the next free number was taken by the `.proto` support demo instead of `11-grpc-unary.sh`: it serves gates 2a/2b of §6 and runs entirely off committed fixtures (`pkg/protocol/grpc/testdata/`) — no network, no `protoc` at run time. It asserts: `grpc discover` on a service-less `.proto` → 0 operations while `grpc schema` on the same file → 4 types; `grpc discover` on a bare `--descriptor_set_out` set equals the `--include_imports` result; an unresolvable-dependency set exits non-zero naming the file and the remedy; and `grpc schema -o json` carries FQN refs, well-known-type JSON forms and enum values. `12-grpc-unary.sh` (or a re-numbered unary leg) remains owed.

**Later**: each adopted protocol adds one numbered demo script in implementation order (`12-grpc-streaming.sh`, `13-jsonrpc.sh`, …), so `run-all.sh` output shows the protocol matrix growing check by check. The `make demo` integration target stays as-is (informational, not a green gate).

**Transport-version gate (order 9/10, PROTO-012's demo-plan entry).** A gate's demo script takes ordinal `11 + (order − 1)` on an **append-only** queue — landed scripts are never renumbered, and the ordinal is re-derived at landing time from `ls demo/demos/` (10 scripts today). Orders 4–8 therefore own 14–18, and this transport's scripts are:

- `demo/demos/19-http2-transport.sh` (order 9): start a loopback TLS fixture offering `h2`; run one GET and one `PROPFIND` asserting the **reported** version is `HTTP/2.0`; then against an `http/1.1`-only fixture assert the request still succeeds and the report says `HTTP/1.1` (a reporting leg, not a failure leg).
- `demo/demos/20-http3-transport.sh` (order 10): the same fixture over UDP/h3 asserting `HTTP/3.0`, plus the reported-fallback leg of gate 10(d).
- `demo/demos/21-webdav-surface.sh` (order 11): the WebDAV profile gate of §6 row 11.

Two scripts for orders 9/10, not one: order 9's gate must be demonstrable at the moment h2 lands, and a combined script would make it depend on order 10's code. **Fixture ownership:** each script builds and starts its own small Go fixture from `demo/services/transport-fixture/` on a loopback port with a self-signed certificate and tears it down on exit. `run-all.sh` starts services from an **explicit list** (`demo/run-all.sh:43-47`), and the demo stack's Python services cannot serve h2/h3 (the stdlib server is HTTP/1.1-only), so owning the fixture inside the script keeps `run-all.sh` unchanged — the `demos/*.sh` glob picks the new legs up as §7 intends.

## 8. Out of Scope for This Spec

- No protocol implementation code, no new dependencies added to `go.mod` by this spec itself. The third-party modules named in §2 are verified-available recommendations for the implementing rows; the one exception is the HTTP/3 pin (`github.com/quic-go/quic-go` **v0.63.0**), which v1.1.0 **decides** (§2 row 16, §4.7) and PROTO-015 lands in `go.mod` — measured cost and alternatives are in [implementation/PROTO-012-http2-http3-transport.md](implementation/PROTO-012-http2-http3-transport.md) §4.
- No binary renaming (§5.1 decision: keep current names).
- No changes to board files, `.coding-hermes`, or `.gitreins` state from this spec.

**Ruled out — `.proto` as the description format of an HTTP surface (PROTO-013 §4, measured by PROTO-011 §5b).** Describing WebDAV (or any HTTP surface) in `.proto` was considered and **declined**, not deferred: (i) there is no WebDAV code in this repo to describe — Muster is the *consumer* side, an HTTP request/version-negotiation question (PROTO-010/012/014/015), not a protobuf one; (ii) `.proto` has no HTTP verb/path vocabulary of its own — the only mechanism is the `google.api.http` option family, and the in-tree parser cannot resolve `google/api/annotations.proto` on this box (measured: exit 1, `not found (searched [.])`), with nothing in the repo shipping or fetching the googleapis include tree; (iii) even with the includes present the extractor reads services and never method options, so such a file still yields 0 operations; and (iv) the format this repo already ingests for HTTP — OpenAPI — is the right carrier. protobuf is a payload encoding there, which the gRPC adapter covers for gRPC backends. This note belongs in PROTO-012's spec when that row is written (per PROTO-013 §4); it is recorded here so the ruling is readable until then.