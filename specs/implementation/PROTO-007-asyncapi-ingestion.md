# PROTO-007: AsyncAPI 3.x Ingestion — Implementation Specification

**Status**: ✅ SPEC GATE — design authority for AsyncAPI ingestion; no product code changed by this row  
**Version**: 1.0.0  
**Date**: 2026-10-07  
**Component**: Architecture / protocol adapter (stage 1–4)  
**Addresses**: PROTO-007 (board: AsyncAPI 3.x ingestion)  
**Related specs**: [specs/PROTOCOLS.md](../PROTOCOLS.md) (design authority), [specs/CANONICAL-INTERFACES.md](../CANONICAL-INTERFACES.md)  
**Depends on**: PROTO-001 protocol adapter seam and the existing neutral model in `pkg/protocol`; PROTO-003 bounded-stream transcript contract  
**Feeds**: PROTO-007 implementation gate, then one broker-transport implementation at a time

---

## 0. Decisions in one screen

| # | Question | Decision | Basis |
|---|---|---|---|
| D1 | What does stage 1 ingest? | AsyncAPI 3.x documents: servers, channels, channel messages, operations (`send` / `receive`), and protocol bindings. Emit neutral `protocol.Operation` values, not AsyncAPI document objects. | `PROTOCOLS.md` §§1, 3 |
| D2 | How are consumer and producer operations represented? | `receive` becomes `KindEventSource`; `send` becomes `KindUnary` for a bounded publish call with a publish acknowledgement/result. No new operation kind. | `PROTOCOLS.md` §§1.3, 4.5 |
| D3 | Where does transport ground truth live? | In `Operation.Source`: channel address, action, selected server identity, message identity, and separately preserved server/channel/operation/message bindings. Stage-3 consumers do not interpret it. | `PROTOCOLS.md` §§1.3, 3.2 |
| D4 | Which transport is first end-to-end? | **NATS**, using `nats.go v1.54.0`, tested with the in-process NATS server `nats-server/v2` and `natsserver.Run`. Kafka, AMQP, MQTT and WebSocket wait for separate gates. | `PROTOCOLS.md` §§2 row 7, 4.6; comparison in §3 |
| D5 | How do consumer operations behave in MCP? | Bounded collect only: default maximum 1000 items and 30 seconds; always unsubscribe and return a transcript. No long-lived consumer inside an MCP tool call. | `PROTOCOLS.md` §4.5(4), gRPC transcript precedent |
| D6 | Is this a code or dependency change? | No. This deliverable is documentation only. Any transport dependency is considered by its implementation gate and is not added by this spec. | PROTO-007 task scope |

**Design authority**: [`specs/PROTOCOLS.md`](../PROTOCOLS.md) wins if wording conflicts. This spec narrows the AsyncAPI row; it does not replace the shared adapter contract.

---

## 1. Problem statement and scope

AsyncAPI documents describe event-driven APIs, but their channel/message/binding structure is not an HTTP request/response operation. PROTO-007 must ingest that document into Muster’s protocol-neutral operation model and make an event-source call safe for command and MCP use without changing the shared `Discoverer`, `Executor`, or generation contracts. The design separates document interpretation from broker execution, preserves binding facts for the stage-4 executor, and requires the first implementation to prove one complete transport path before adding others.

This is a design gate. It specifies mapping, bounded behavior, auth, errors, test strategy, and acceptance gates; it does not implement the adapter or select dependencies for any later transport beyond the first end-to-end candidate recommendation.

## 2. Stage-1 ingestion and neutral-model mapping

### 2.1 Input and operation expansion

The AsyncAPI discoverer accepts a local AsyncAPI 3.x document URI and returns `[]*protocol.Operation` per `PROTOCOLS.md` §3.1. Document parsing and reference resolution are read-only. Discovery MUST validate the document version and required references before returning operations; malformed or unresolved structures fail with `CodeValidation`, not a partial operation set. Network access to a broker is never part of discovery.

AsyncAPI 3.x `operations` are the primary operation definitions. Each operation references one channel, declares an `action` (`send` or `receive`), and associates one or more messages. Resolve channel and message references before mapping. Expand an operation with multiple message choices into one neutral operation per message so each generated command/tool has a stable payload contract; preserve the originating AsyncAPI operation identity and message identity in `Source`. Do not synthesize operations merely because a channel exists without a referenced operation.

### 2.2 Field mapping

| AsyncAPI 3.x source | `protocol.Operation` destination | Mapping contract |
|---|---|---|
| Document `info.title`, `info.version` | `Summary`/`Description` only when a per-operation value is absent; document provenance in `Source` | Document-wide values are context, not unique operation IDs. Preserve source document identity in string form. |
| Operation `operationId` | `ID` | Prefer the explicit ID. If absent, synthesize a deterministic ID from action + channel identity + message identity. Resolve collisions deterministically and reject any unresolved duplicate IDs. |
| Operation `action: receive` | `KindEventSource` | This is the application consuming messages. MCP uses bounded collect (§4), never a persistent subscription. |
| Operation `action: send` | `KindUnary` | A single publish invocation with a bounded publish result/acknowledgement. No new `KindPublish` is introduced. A later API/model proposal is required if the existing executor result contract cannot express a transport acknowledgement. |
| Operation `tags`, `summary`, `description` | `Tags`, `Summary`, `Description` | Preserve order and text; apply empty-value defaults only where the shared generator requires them. |
| Channel `address` | `Source["channel_address"]` | Preserve the literal address, including parameter expressions. Do not reinterpret it as an HTTP path. |
| Channel address parameters | `Parameters` | Map each parameter by name, type, requiredness and description, using an AsyncAPI/channel-address location such as `In: "channel"`; retain the original parameter name. |
| Message `headers` schema | `Parameters` | Map depth-1 header properties as `In: "message-header"`; preserve required, type, enum, default, example and description where the neutral fields support them. |
| Message `payload` schema | `RequestBody` | Map depth-1 payload properties to `RequestBodyProperty` (name, type, required, enum, default, description). Preserve a payload schema reference as a reference string in source metadata when the current neutral field set cannot represent it. |
| AsyncAPI operation/channel/message names and references | `Source["operation_id"]`, `Source["channel_id"]`, `Source["message_id"]` | Store identifiers as strings. Keep the AsyncAPI identities even when a generated `ID` is synthesized. |
| Operation `reply` / correlation metadata | `Source` | Preserve as canonical JSON strings; no reply-flow behavior is implied by ingestion alone. |
| Selected server identity and server URL/host/protocol | `Source["server_name"]`, `Source["server_url"]`, `Source["server_protocol"]` | Choose a server deterministically from explicit discovery options or the document’s declared default ordering; if ambiguous and no selection was supplied, fail with `CodeValidation` rather than guessing. The chosen identity is runtime input, not a network call during discovery. |
| Server, channel, operation and message bindings | `Source["binding.server"]`, `Source["binding.channel"]`, `Source["binding.operation"]`, `Source["binding.message"]` | Preserve each binding object independently as deterministic canonical JSON. The executor interprets only the binding scopes relevant to its selected transport. Do not flatten away the scope or merge conflicting values silently. |
| Operation security requirement references | `Source["security"]` | Preserve referenced scheme names and requirement scopes for auth configuration and diagnostics; secret values are never copied into the document model. |

`Operation.Protocol` is `protocol.ProtocolAsyncAPI`. `HasMultiPart` and `MultipartFields` remain false/empty. `Operation.Source` is `map[string]string`; any structured source value MUST therefore be encoded as deterministic JSON text. Stage-3 generators and MCP converters use only the neutral fields and MUST NOT interpret `Source`.

### 2.3 Deliberately dropped or deferred information

- Broker topology, consumer groups, offsets, durable subscription state, retries, transactions, acknowledgements, partitions, queue declarations, and delivery guarantees are execution/configuration concerns, not stage-1 operations. Preserve declared binding facts in `Source`; do not imply they have been enacted.
- Schema constructs deeper than the current depth-1 neutral property model are not expanded into recursive trees. Retain schema references and unsupported schema details in source metadata where practical; report unsupported executable payload shapes explicitly rather than silently claiming fidelity.
- AsyncAPI `components` are reference sources, not operations themselves. Unused components, examples, external documentation, contact/license metadata, and arbitrary extensions are not emitted as operations. Unknown extensions may be retained only as provenance; they do not alter execution.
- Reply channels, request/reply correlation, multi-step workflows, and server-side operation semantics are not implemented by this ingestion gate. A producer operation remains a publish call, not a synthesized consumer/reply pair.
- A channel address may contain expressions. Resolution of runtime values is not performed at document-ingest time; address parameters remain explicit neutral inputs and the executor receives the original address template through `Source`.

## 3. One-transport end-to-end choice and dependency assessment

### 3.1 Candidate comparison

The versions below are the candidates named by the design authority. The module-cost column describes the expected cost class, not a measured module-graph delta; the implementation gate MUST record actual `go list -m all` and compiled-package deltas before adding a dependency. No candidate module is added by this specification.

| Candidate | Go module / cost class | Network-free test story | AsyncAPI binding maturity and trade-off |
|---|---|---|---|
| Kafka — `github.com/segmentio/kafka-go v0.4.51` | One Kafka client module plus its transitive graph; likely the broadest dependency and CI footprint of these candidates. Measure compiled closure and binary impact before pinning. | A true in-process Kafka broker is not a lightweight Go-native fixture; Testcontainers would make CI container-dependent/heavy. Loopback broker processes are possible but do not satisfy the default no-external-service test bar as cleanly. | Established Kafka binding for Kafka-specific channel/topic, key, partition and message semantics. Strong protocol fit, but more operational setup and heavier test infrastructure for the first proof. |
| AMQP — `github.com/rabbitmq/amqp091-go v1.15.0` | Small, focused AMQP client module; broker itself is separate and therefore not represented by the Go module delta. | No embedded pure-Go RabbitMQ broker in the required test pattern; container/process fixture is heavier than an embedded server and may be unsuitable for hermetic CI. | Mature AMQP binding surface for exchange, queue, routing and delivery properties. Good candidate after the initial seam is proven; server lifecycle complicates the first gate. |
| MQTT — `github.com/eclipse/paho.mqtt.golang v1.5.1` | Client module with a modest transitive graph; still requires measuring the actual build closure. | In-process Go MQTT brokers are available in the ecosystem, but require selecting and pinning a broker fixture; it is not as directly supplied by the stated reference pattern as embedded NATS. | AsyncAPI MQTT binding covers MQTT-specific QoS, retain and session semantics. Suitable for later constrained deployment, but QoS/session behavior adds test cases to the first adapter proof. |
| NATS — `github.com/nats-io/nats.go v1.54.0` | One client module plus a bounded Go dependency graph; server test dependency is test-only and must not enter production builds. Measure both graphs separately. | Strongest fit: use the in-process server from `github.com/nats-io/nats-server/v2`, started with `natsserver.Run`, and connect the client through its local listener. No external broker or container is required. | NATS binding captures NATS-specific subject/queue/group behavior. The binding and client/server semantics are suitable for a clear first publish/subscribe proof; unsupported NATS-specific details remain explicit source facts until implemented. |
| WebSocket — `github.com/gorilla/websocket v1.5.3` | Small client module and low dependency cost; likely the lightest option. | Excellent loopback test story using `httptest` and a local upgrade endpoint. | WebSocket binding is transport-oriented and does not provide broker semantics such as durable subscription, topic retention or consumer offsets. Useful for direct event channels, but less representative of the broker-oriented AsyncAPI row. |

Binding maturity here means fit and breadth of the protocol-specific binding vocabulary, not a claim that all binding features are executable. The ingestion contract preserves binding scopes and ground truth for all five protocols. Each transport implementation must declare the binding subset it executes and fail clearly for unsupported required features. AsyncAPI specification and binding references: [AsyncAPI 3.0.0 specification](https://www.asyncapi.com/docs/reference/specification/v3.0.0); transport-specific support remains gated by the per-transport implementation tests.

### 3.2 Recommendation: NATS first

Choose **NATS** for the first end-to-end implementation. This decision is based on the primary gate constraint in `PROTOCOLS.md` §4.6: a real protocol exchange must run in-process with no external service. NATS has both a Go client candidate and the specifically identified embedded server API (`natsserver.Run`), so one test process can exercise discovery-to-executor wiring, publish, subscribe, bounds and cleanup without Docker or a separately managed broker. It therefore gives the highest signal on the adapter seam and bounded event-source behavior at the lowest test-operability cost.

This is not a claim that NATS is universally preferable or that it has the smallest measured production binary. Before the implementation pins dependencies, it records the exact module graph, compiled package closure, license/security scan and build/test impact. The NATS server dependency is test-only; it MUST NOT be imported by production packages or leak into the production binary graph.

The other four wait for concrete reasons: Kafka has the heaviest in-process test burden and a large broker lifecycle; AMQP has no equally direct embedded Go broker fixture in the required pattern; MQTT needs a chosen broker fixture and explicit QoS/session coverage; WebSocket is cheap but does not prove broker semantics. Each may proceed only after NATS establishes the neutral-model and executor pattern, with its own binding subset and network-free gate.

## 4. KindEventSource bounded-collect semantics

An AsyncAPI `receive` operation is `KindEventSource`. It MUST implement the `PROTOCOLS.md` §4.5(4) bounded-collect rule: subscribe, collect, unsubscribe, return. It MUST NOT hold a broker consumer, socket, or subscription open after an MCP tool call returns. Long-lived consumption belongs to daemon/DSL execution, not an MCP call.

Use the existing gRPC `StreamTranscript` vocabulary and JSON envelope shape:

```json
{
  "kind": "event-source",
  "status": "complete | cancelled | truncated | error",
  "items": [],
  "items_truncated": false,
  "trailer": {
    "received": 0,
    "max_items": 1000,
    "duration_ms": 0,
    "limit_reason": "max-items | max-duration",
    "message": "",
    "error": ""
  }
}
```

`items` is always a JSON array. Defaults are **1000 messages** and **30 seconds** per invocation. There is no unbounded setting: missing, zero or negative values select the defaults; an explicit configuration may lower a bound, while the MCP hard cap may not be raised by operation input. Stop at the first reached bound, unsubscribe/close before returning, set `status: "truncated"`, `items_truncated: true`, and set `trailer.limit_reason` to `max-items` or `max-duration`. If the caller cancels or its context deadline expires, preserve partial items, set `status: "cancelled"`, clean up the subscription, and return/map the cancellation as `CodeTimeout`. A broker/decode failure preserves partial items, sets `status: "error"`, and returns the mapped `AppError` alongside a well-formed transcript where the caller surface supports both; no stream error may leave the tool waiting.

The publish (`send`) path is one bounded `KindUnary` execution, not a long-lived stream. It returns the broker acknowledgement/metadata in JSON-serializable form. Producer retry is idempotency-gated per `PROTOCOLS.md` §4.3; consumers are not automatically replayed after partial collection.

## 5. Authentication mapping

AsyncAPI broker authentication follows `PROTOCOLS.md` §4.1. Credential acquisition/storage is shared through `pkg/auth`; the adapter maps configured credentials to native broker options. It MUST NOT add an adapter-owned token store, copy secrets into `Operation.Source`, or log credentials.

| Broker family | Adapter mapping responsibility |
|---|---|
| Kafka | Map configured SASL mechanism/credentials and TLS settings into the Kafka dialer/client options. |
| AMQP | Map username/password or supported SASL credentials and TLS settings into the AMQP connection configuration. |
| MQTT | Map broker credentials and TLS settings into the Paho client configuration; never place secret values in channel parameters or tool schemas. |
| NATS | Map configured user/password, token or supported credential flow and TLS settings into the NATS connection options. |
| WebSocket | Map HTTP handshake authentication/headers through shared auth material and TLS configuration; do not invent a separate credential cache. |

The selected AsyncAPI security schemes and their requirement references are preserved as metadata for configuration/diagnostics. A secret is resolved at execution time from shared auth/configuration, then applied only through the selected transport’s native mechanism. Refresh, if supported by the credential type, is delegated to `pkg/auth` before mapping.

## 6. Error mapping

Every transport-boundary failure is wrapped with `pkg/errors.Wrap` and surfaces as `*errors.AppError`, preserving the original cause for `errors.IsError` and verbose diagnostics, per `PROTOCOLS.md` §4.2.

| Broker failure | AppError code | Required behavior |
|---|---|---|
| Broker quota / throttling / publish or consume rate refusal | `CodeRateLimit` | Preserve broker retry hints in details when available. |
| SASL or equivalent authentication failure | `CodeUnauthorized` | If shared auth permits refresh, retry the auth acquisition once according to the shared auth contract; otherwise fail coded. |
| Connection/dial timeout, context deadline | `CodeTimeout` | Honor caller context/deadline; do not replace it with an adapter default. |
| Message payload/header decode or protocol serialization failure | `CodeInternal` | Name the failed decode/serialization phase without including secrets or unbounded payload dumps. |
| Malformed operation input rejected before network activity | `CodeValidation` | Identify the field/operation; no broker call is made. |
| Other broker/transport failures | `CodeGeneric` | Include protocol and operation context as safe details; preserve the underlying cause. |

A single adapter mapping function per transport is table-driven where possible. Classification is by typed transport error/status, not by brittle message substring matching. A timeout must not be mislabeled as a generic broker failure, and quota/auth/decode errors must not be collapsed into one generic code.

## 7. Network-free test strategy and conformance

CI MUST NOT require a publicly reachable broker or an external service. The first end-to-end suite uses the in-process NATS server from `github.com/nats-io/nats-server/v2` via `natsserver.Run`, plus the NATS client. Start the server inside the test, connect only to its local listener, publish/consume test messages, then close subscriptions and stop the server with test cleanup. Tests must not silently fall back to a developer-installed daemon or a shared host broker.

Every adapter follows the three-test rule from `AGENTS.md` and `PROTOCOLS.md` §4.6:

1. **Unit** — deterministic document fixtures exercise AsyncAPI 3.x parsing, references, stable ID derivation, send/receive classification, payload/header/channel-parameter mapping, binding-scope preservation, ambiguity rejection, and unsupported-schema errors.
2. **Integration** — connect the AsyncAPI discoverer, neutral operations, selected transport executor, shared auth/error mapping, and bounded collect against the in-process NATS server. Assert actual publish/consume data, transcript bounds, cleanup/unsubscribe, cancellation, timeout, quota/auth mapping where injectable, and decode failures. Assert the test broker has stopped after each test.
3. **E2E** — invoke the real CLI/MCP path over the in-process broker fixture (or loopback fixture started by the test harness) and prove an AsyncAPI document becomes an operation and returns a bounded transcript/publish result. No external endpoint or pre-seeded host service is allowed.

Include one conformance comparison against a reference client for the selected transport: issue the same publish and subscribe exchange through `nats.go` directly and through Muster’s executor against the same in-process server, then compare the received payload and relevant message metadata. The direct reference-client arm is an independent oracle; comparing only Muster to its own fixture handler is insufficient. The implementation gate should retain a control that uses a deliberately wrong channel address or binding so the test fails if the executor ignores `Operation.Source` and hardcodes a subject.

Later transport gates choose a broker fixture that is genuinely in-process or loopback-only and keep the same three layers and reference-client comparison. A container-dependent test is not accepted as the sole proof of an AsyncAPI transport.

## 8. Implementation order 6 gate definition

`PROTOCOLS.md` §6 order 6 is two gates, in order; the first is this document and the second is implementation. No implementation starts until this spec gate is reviewed and accepted.

| Gate | Acceptance definition |
|---|---|
| **Spec gate — PROTO-007 (this document)** | This reviewed document defines the stage-1 mapping, preserved/dropped fields, producer/consumer kinds, one-transport rationale weighing all five candidates, bounded collect defaults and envelope, shared auth/error mapping, network-free test plan, and the implementation acceptance gate. No Go implementation or dependency change is part of this gate. |
| **Implementation gate — first transport** | One selected transport (NATS) works end-to-end from an AsyncAPI 3.x fixture through `protocol.Operation` and the real executor against an in-process broker. Consumer results obey the §4 bounded transcript and clean up subscriptions; producer publish returns a bounded result. Unit, integration, e2e and independent reference-client conformance checks pass; network-free tests and transport error mapping are proven. Production and test dependency graphs are reported separately. |
| **Implementation gate — remaining transports** | Add Kafka, AMQP, MQTT, and WebSocket one at a time. Each transport documents its executable binding subset, preserves its ground truth, maps auth/errors, has an in-process/loopback test fixture, passes the three-test rule plus a reference-client conformance comparison, and cannot regress the first transport’s gate. Do not batch transports into one implementation wave. |

The spec gate is satisfied by this file only after review. A future code implementation must not claim order 6 green merely because the AsyncAPI document parses: the broker exchange, bounded consumer lifecycle, and conformance proof are all required.

**Demo-plan alignment (`PROTOCOLS.md` §7).** The implementation must add an end-to-end demo leg using the real CLI/MCP entry point and a local NATS fixture, proving document ingestion plus bounded consume/publish behavior. Assign its numbered filename from the append-only demo queue at implementation time; do not reserve or infer an ordinal in this spec. Follow §7's real-binary/demo gate conventions and do not treat a unit-only fixture as the demo. The current `demo/run-all.sh` glob should pick up a correctly numbered script without renumbering earlier demos.

## 9. Non-goals, edge cases, and open questions

### 9.1 Non-goals

- No Go implementation, `go.mod`/`go.sum` edit, dependency installation, or transport code in this spec gate.
- No implementation of all five transports at once; NATS is the first proposed end-to-end transport, and each remaining transport is separate work.
- No durable consumer service, daemon lifecycle, offset management UI, transaction model, exactly-once guarantee, or cross-broker abstraction beyond the neutral operation seam.
- No promise to execute every AsyncAPI binding field or every JSON Schema construct. Unsupported required features must fail explicitly; preserved metadata is not equivalent to implemented semantics.
- No recursive schema graph in `protocol.Operation`, no automatic reply workflow, and no change to shared `Discoverer`, `Executor`, `OperationKind`, CLI, or MCP interfaces in this spec.
- No adapter-owned token store, output format, or direct stdout writes.

### 9.2 Edge cases and required behavior

- Missing/duplicate operation IDs: synthesize deterministic IDs only when absent; reject collisions after normalization rather than produce duplicate tools.
- Unresolved `$ref`, unsupported AsyncAPI version, invalid action, or operation referencing no channel/message: fail discovery with `CodeValidation` and return no partial set.
- Multiple messages: emit one operation per message and retain source operation/channel/message identity.
- Ambiguous server selection: require an explicit server choice or deterministic declared default; never dial during discovery to guess availability.
- Empty receive window: return a valid transcript with `items: []`, `status: "complete"` only when the collect completes normally; a bound-fired empty result is `truncated` and marked accordingly.
- Bound/cancel/error during receive: preserve partial items, set transcript status/trailer consistently, and always close the subscription before return.
- Unknown binding fields: preserve the scoped binding JSON; executor must reject an unsupported field if it is required to honor requested behavior instead of silently ignoring it.
- Secret-bearing security configuration: never serialize credential values into operation metadata, logs, errors, tool schemas, or transcript trailers.

### 9.3 Open questions for implementation review

| # | Question | Recommendation / owner |
|---|---|---|
| O1 | Which AsyncAPI parser/library and exact version? | Implementation row measures module/build cost, AsyncAPI 3.x support, reference resolution and binding fidelity before pinning; do not add it in this spec gate. |
| O2 | What is the exact NATS binding subset for first implementation? | Implementation spec must enumerate supported server/channel/operation/message binding fields and fail unsupported execution requirements explicitly. |
| O3 | Does the current `protocol.Operation` payload model need an additive schema-reference field? | First implementation should preserve schema references in `Source` if depth-1 mapping suffices; propose a separate neutral-model change only with a demonstrated consumer need and compatibility analysis. |
| O4 | How are publish acknowledgements and broker metadata represented across clients? | Start with JSON-serializable acknowledgement metadata returned as the `KindUnary` result; do not introduce a new operation kind without evidence that the executor contract cannot support it. |
| O5 | Are per-operation collect bounds configurable, and where are they stored? | Keep defaults at 1000 messages / 30 seconds and enforce MCP hard caps; implementation may add bounded configuration without permitting an unbounded mode. |
| O6 | How should client-stream/bidi-like AsyncAPI patterns be exposed? | This gate models `receive` as bounded `KindEventSource` and `send` as one `KindUnary` publish. Multi-step and bidirectional workflows remain deferred pending an explicit neutral-model and MCP-safety proposal. |

---

## Appendix A — References

- [`specs/PROTOCOLS.md`](../PROTOCOLS.md), especially §§1.2–1.4, §2 row 7, §§3.1–3.5, §§4.1–4.6, §6 order 6, and §7 demo plan.
- [`specs/CANONICAL-INTERFACES.md`](../CANONICAL-INTERFACES.md).
- [AsyncAPI 3.0.0 Specification](https://www.asyncapi.com/docs/reference/specification/v3.0.0).
