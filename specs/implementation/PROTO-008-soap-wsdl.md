# PROTO-008: SOAP/WSDL 1.1 + 2.0 Adapter — Implementation Specification

**Status**: ✅ LANDED — implementation + spec (this document is the design authority for the SOAP adapter)
**Version**: 1.0.0
**Date**: 2026-10-07
**Component**: Architecture / protocol adapter (stage 1–4)
**Addresses**: PROTO-008 (board: SOAP/WSDL 1.1 + 2.0 adapter — operation ingestion, envelope builder, fault mapping)
**Related specs**: [specs/PROTOCOLS.md](../PROTOCOLS.md) (design authority), [PROTO-007-asyncapi-ingestion.md](PROTO-007-asyncapi-ingestion.md) (spec-gate pattern)
**Depends on**: PROTO-001 protocol adapter seam and the neutral model in `pkg/protocol`; the HTTP-family transport rules of `PROTOCOLS.md` §3.5
**Feeds**: the demo leg (`demo/demos/21-soap-adapter.sh`) and any future WS-Security row

---

## 0. Decisions in one screen

| # | Question | Decision | Basis |
|---|---|---|---|
| D1 | What does stage 1 ingest? | WSDL 1.1 `<definitions>` and WSDL 2.0 `<description>` documents, parsed with a light `encoding/xml` tree walker (no `gowsdl` dependency). Each bound operation becomes one neutral `protocol.Operation` (KindUnary). | `PROTOCOLS.md` §2 row 8; module-cost verdict below |
| D2 | How are typed message parts represented? | Each input part / input element's depth-1 XSD fields map onto `RequestBodyProperty`: required (minOccurs ≥ 1), repeated (maxOccurs unbounded → `Repeated` + `Type:"array"` + `ItemType`), nillable (noted in the property description; JSON null is always legal). XSD built-in types map onto the JSON-schema vocabulary via a fixed table (§3). | neutral model; gRPC adapter precedent |
| D3 | What envelope shape does stage 4 send? | SOAP 1.1: POST body `text/xml; charset=utf-8` plus an HTTP `SOAPAction: "<action>"` header. SOAP 1.2: POST body `application/soap+xml; charset=utf-8; action="<action>"` — no SOAPAction header. Optional WS-Addressing 1.0 header block (`wsa:Action`, `wsa:To`, `wsa:MessageID`) behind `WithAddressing(true)` / `--addressing`. | acceptance criterion 2 |
| D4 | What does the response render as? | JSON by default (the first non-Fault Body child element, converted: attributes as `@name`, repeated children as arrays); raw XML on demand (`"_output":"xml"` in args). | acceptance criterion 2 |
| D5 | How do faults map? | Table-driven (§5): fault code local part Client/Sender/VersionMismatch/DataEncodingUnknown/MustUnderstand → `CodeBadRequest`; Server/Receiver → `CodeInternal`; anything else → `CodeGeneric`. The raw fault code, version, subcode, actor and detail are preserved as AppError details. Faults are detected inside ANY HTTP status, including 200 OK. | acceptance criterion 3 |
| D6 | What is explicitly OUT OF SCOPE? | WS-Security (UsernameToken, X.509 signing, WS-SecurityPolicy) and MTOM/XOP attachments. Documented here and in `--help`; a WSDL declaring them still ingests, and the limitation is never silently ignored. | acceptance criterion 4 |

**Design authority**: [`specs/PROTOCOLS.md`](../PROTOCOLS.md) wins if wording conflicts. This spec narrows the SOAP row; it does not replace the shared adapter contract.

---

## 1. Problem statement and scope

SOAP/WSDL is the last adopted protocol of the `PROTOCOLS.md` matrix (row 8, order 7 — lowest priority of the adopted set, niche demand, well-specified fault semantics). The adapter is client-side only: ingest a WSDL, build an envelope, parse a response or fault. No SOAP server exists outside test fixtures, and the shared `Discoverer`/`Executor`/generation contracts are unchanged.

## 2. Stage-1 ingestion (WSDL → neutral operations)

The discoverer accepts a local WSDL path or an `http(s)://` URL (file wins on `os.Stat`, so a WSDL can be fetched from a loopback test server without a second discovery code path). Discovery never contacts the service endpoint; malformed documents fail with `CodeValidation` and return no partial operation set.

### 2.1 WSDL 1.1 mapping

| WSDL 1.1 source | Destination | Notes |
|---|---|---|
| `<definitions targetNamespace>` | document namespace | required; missing → `CodeValidation` |
| `<wsdl:message>` parts (`element` / `type` attributes) | resolved to `{uri}Local` QNames against the target namespace | prefix handling in §2.3 |
| `<portType><operation>` | one `protocol.Operation` per operation | ID = `Service-Operation` (gRPC adapter's service-method shape) |
| `<binding><soap:operation soapAction>` | `Source["soap_action"]` | binding-level `style` default, overridden per operation |
| `<service><port><soap:address location>` | `Source["endpoint"]` | the executor's default target |
| inline `<xsd:schema>` in `<types>` | global element declarations + named complexTypes | keyed by resolved QName so cross-references resolve |

### 2.2 WSDL 2.0 mapping

| WSDL 2.0 source | Destination | Notes |
|---|---|---|
| `<description targetNamespace>` | document namespace | required |
| `<interface><operation>` with `input`/`output` `element` refs | one operation per bound interface operation | element-ref parts carry no `name` (part name falls back to the element local part) |
| `<binding wsoap:...><operation wsoap:action>` | `Source["soap_action"]` | both `wsoap:action` and plain `action` attribute spellings accepted |
| `<service><endpoint address>` | `Source["endpoint"]` | version stamps `Source["soap_version"]="1.2"`, selecting the 1.2 envelope |

### 2.3 QName resolution rule

The parser keeps local names and attribute values only, so QName-valued attributes (`element="tns:EchoRequest"`, `type="xsd:string"`) resolve pragmatically, matching what real WSDL documents do:

- `xsd:`/`xs:` → the XML Schema namespace, then reduced to the local name by the type-mapping table;
- no prefix or `tns:`/unknown prefix → the document (or schema) `targetNamespace` — WSDL-owned QNames reference the document's own components;
- protocol prefixes (`soap`, `wsdl`, `wsa`, …) never own data types → target namespace.

WSDL 2.0's `wsoap:action` attribute is namespaced and stored as `uri:local`; the parser accepts both that spelling and a plain `action`.

## 3. XSD type → CLI flag mapping table

Each depth-1 element becomes a request-body property, i.e. one CLI flag / MCP input field. The mapping:

| XSD type (local name) | Neutral `Type` | Flag behavior |
|---|---|---|
| `int`, `integer`, `long`, `short`, `byte`, `unsigned*`, `non(Negative|Positive)Integer` | `integer` | numeric text on the wire |
| `float`, `double`, `decimal` | `number` | |
| `boolean` | `boolean` | rendered `true`/`false` |
| `dateTime`, `date`, `time`, `duration`, `gYear*`, `gMonth*`, `gDay` | `string` | lexical XML form sent unchanged — no implicit JSON date rewrite |
| everything else (`string`, `token`, `base64Binary`, `anyURI`, derived/unknown) | `string` | XML-escaped text |

Cardinality and optionality:

| XSD fact | Neutral model | CLI flag behavior |
|---|---|---|
| `minOccurs` absent or ≥ 1 | `Required: true` | flag is required |
| `minOccurs="0"` | `Required: false` | flag optional; omitted ⇒ element not emitted |
| `maxOccurs="unbounded"` (or numeric > 1) | `Repeated: true`, `Type:"array"`, `ItemType:<item>` | flag accepts a JSON array; each entry renders as one repeated XML element |
| `nillable="true"` | note in the property description | JSON null is always a legal value; `xsi:nil` emission stays manual |

## 4. Envelope builder (stage 4)

### 4.1 Header shape per version

```
SOAP 1.1 (envelope ns http://schemas.xmlsoap.org/soap/envelope/)
  POST {endpoint}
  Content-Type: text/xml; charset=utf-8
  SOAPAction: "http://example.com/echo/Echo"

SOAP 1.2 (envelope ns http://www.w3.org/2003/05/soap-envelope)
  POST {endpoint}
  Content-Type: application/soap+xml; charset=utf-8; action="http://example.com/calc/Add"
```

With `WithAddressing(true)` / `--addressing` a WS-Addressing 1.0 (2005/08) header block is added inside the envelope:

```xml
<wsa:Header xmlns:wsa="http://www.w3.org/2005/08/addressing">
  <wsa:Action>{SOAPAction}</wsa:Action>
  <wsa:To>{endpoint}</wsa:To>
  <wsa:MessageID>urn:uuid:{nano-timestamp}</wsa:MessageID>
</wsa:Header>
```

### 4.2 Body shape

- document style (default): the input element's local name wraps its depth-1 fields in declaration order, and carries the input element's namespace as a default declaration — `<EchoRequest xmlns="http://example.com/echo"><message>hi</message><tags>a</tags><tags>b</tags></EchoRequest>`. A document/literal service whose input element is namespace-qualified refuses an unqualified root with `Invalid namespace ''` (the SOAP envelope namespace is not the payload's), so the declaration is required, not cosmetic.
- rpc style (or a missing input element): the operation name wraps its parts, qualified with the document target namespace.
- children inherit the root's namespace declaration and are emitted unqualified; the neutral request model carries no per-field namespace, so a depth-1 field in a different namespace is not representable.
- omitted optional args are not emitted; array values render one element per entry; object values render each key as a child element (sorted for determinism); scalars are XML-escaped (`& < > " '`).

### 4.3 Response rendering

JSON default: the first non-Fault Body child element converts to a map (attributes under `@name`, repeated children as arrays, text-only elements as strings). `"_output":"xml"` in the args returns the raw response bytes as a string. An empty Body renders `{}` (an accepted one-way call).

## 5. Fault → AppError mapping

Faults are parsed from both wire shapes — 1.1 (`faultcode`/`faultstring`/`faultactor`/`detail`) and 1.2 (`Code/Value` + optional `Subcode`, `Reason/Text`, `Detail`) — and are detected inside any HTTP status, including 200 OK and the 4xx/5xx bodies SOAP 1.2 services use. The mapping (fault code **local part** decides; prefixes differ per version and per service):

| Fault code (local part) | AppError code | Rationale |
|---|---|---|
| `Client` / `Sender` | `CodeBadRequest` | the caller sent it wrong |
| `Server` / `Receiver` | `CodeInternal` | the endpoint failed |
| `VersionMismatch` | `CodeBadRequest` | envelope version refused |
| `DataEncodingUnknown` | `CodeBadRequest` | |
| `MustUnderstand` | `CodeBadRequest` | a required header was not understood |
| anything else | `CodeGeneric` | |

Preserved details on every fault: `fault_code` (raw QName as sent), `fault_version` (`1.1`/`1.2`), `fault_subcode` (1.2), `fault_actor` (1.1), and `fault_detail` (the Detail element's children rendered compactly — never dropped). Transport failures map independently: timeout/deadline → `CodeTimeout`, 429 → `CodeRateLimit` (+ `retry_after`), 401/403 → `CodeUnauthorized`, 404 → `CodeNotFound`, other 5xx → `CodeInternal`, other 4xx → `CodeBadRequest`.

## 6. OUT OF SCOPE — WS-Security and MTOM/attachments

**This adapter does not implement WS-Security or MTOM/XOP.** Explicitly:

- **WS-Security** (WS-SecurityPolicy assertions, `UsernameToken`, `Timestamp` headers, X.509 XML signature/encryption): not built. A WSDL whose binding declares a policy still ingests — the policy is not a wire requirement of discovery — but a service that *requires* the tokens will fault. The adapter sends no security header block; it does not silently claim support.
- **MTOM/XOP binary attachments**: not built. `base64Binary`/`hexBinary` fields are mapped as plain `string` flags whose values travel as inline lexical base64/hex inside the envelope; multipart MIME packaging (`multipart/related`, XOP `Include` elements) is not produced or parsed.

Both are documented in the package doc comment and in `protocol soap --help`. A future row (e.g. PROTO-0xx "SOAP security") would add a header-block plugin seam behind `WithAddressing`; the current header block is intentionally the only extension point.

## 7. Dependency decision

**No new module.** `github.com/hooklift/gowsdl v0.5.0` (the matrix's candidate) is a code *generator* (CLI producing Go packages), not a parse-and-map library — pulling it in for a runtime adapter would add a codegen toolchain plus its module graph for facts a ~600-line `encoding/xml` walker already extracts. Module graph unchanged (0 modules added); `go.mod`/`go.sum` untouched.

## 8. Test strategy and gate evidence

Network-free, per the shared gate rules (`PROTOCOLS.md` §6: "no gate, no implementation"; every gate runs in CI as Go tests):

1. **Unit** — WSDL 1.1 ingestion (typed parts: required / optional / nillable / repeated), WSDL 2.0 ingestion (SOAP 1.2 version stamp, `wsoap:action`), non-WSDL rejection, XSD→JSON type table, cardinality translation.
2. **Golden** — the envelope bytes for a 1.1 document-style call are compared exactly (`TestBuildEnvelope_SOAP11_Golden`), including XML escaping and repeated-element rendering; the 1.2 content-type/action parameter and the WS-Addressing block are asserted verbatim.
3. **Round-trip (in-process httptest)** — a loopback SOAP server fixture: success call (JSON render + repeated element as a JSON array), raw-XML rendering, SOAP 1.1 fault on 500 → `CodeBadRequest` with `fault_code`/`fault_detail` preserved, SOAP 1.2 fault with subcode, fault inside a 200 → `CodeInternal`, unknown operation / no-discovery negatives.
4. **CLI-level** (`internal/builtin/soap_test.go`) — discovery listing, HTTP round-trip through the real `protocol soap` command with the server asserting the SOAPAction header, `--document` (no network), `--addressing`, and root-command wiring.

## 9. Demo and docs

- `demo/demos/21-soap-adapter.sh` — starts an in-process Go SOAP fixture (loopback), runs the built CLI through discovery, a document-style call with a repeated field, and a fault, and asserts the JSON output. Picked up by `demo/run-all.sh`'s `[0-9]*.sh` glob without renumbering.
- Docs: this spec + the package documentation; no separate docs page (the `--help` text carries the operator surface, matching the GraphQL/JSON-RPC adapters).

## Appendix A — References

- [`specs/PROTOCOLS.md`](../PROTOCOLS.md), especially §2 row 8, §3.1–3.5, §4.1–4.4, §6 order 7.
- [SOAP 1.1 spec](https://www.w3.org/TR/2000/NOTE-SOAP-20000508/), [SOAP 1.2 part 1](https://www.w3.org/TR/soap12-part1/), [WSDL 1.1](https://www.w3.org/TR/wsdl), [WSDL 2.0](https://www.w3.org/TR/wsdl20/), [WS-Addressing 1.0](https://www.w3.org/Submission/ws-addressing/).
