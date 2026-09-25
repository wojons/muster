// Package protocol defines the protocol-neutral operation model and the
// adapter seam every protocol backend implements.
//
// This package is the design authority's frozen contract
// (specs/PROTOCOLS.md §1.3): the signatures below are copied verbatim and
// MUST NOT drift. Adapters (OpenAPI, gRPC, JSON-RPC, …) implement Adapter
// and register with Registry; generation (pkg/generator, pkg/mcp) consumes
// []Operation; the transport lives behind Executor.
package protocol

import "context"

// Protocol identifies a supported protocol family.
type Protocol string

const (
	ProtocolOpenAPI  Protocol = "openapi"  // OpenAPI 3.x (first adapter)
	ProtocolGRPC     Protocol = "grpc"     // gRPC unary + streaming
	ProtocolJSONRPC  Protocol = "jsonrpc"  // JSON-RPC 2.0 / OpenRPC
	ProtocolGraphQL  Protocol = "graphql"  // GraphQL queries/mutations/subscriptions
	ProtocolAsyncAPI Protocol = "asyncapi" // event-driven (Kafka/AMQP/MQTT/NATS/WebSocket)
	ProtocolSOAP     Protocol = "soap"     // SOAP 1.1/2.0 via WSDL
)

// OperationKind classifies an operation's interaction shape. MCP agent-safety
// rules (§4.5) key off this value.
type OperationKind int

const (
	KindUnary        OperationKind = iota // single request → single response (unbounded latency is the server's problem)
	KindServerStream                      // single request → response stream
	KindClientStream                      // request stream → single response
	KindBidiStream                        // request stream ↔ response stream
	KindEventSource                       // long-lived subscription/event source (no natural end)
)

// Parameter is the protocol-neutral input parameter. It generalizes
// pkg/openapi.Parameter (fields In/Name/Type/Required/Enum/Default/Example/
// Description) — the OpenAPI adapter maps 1:1 onto it.
type Parameter struct {
	Name        string
	In          string // adapter-defined location: "path", "query", "header", "message-field", "graphql-variable", "payload", ...
	Type        string // JSON-schema-style type string, same vocabulary pkg/mcp uses today
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
	ID              string // stable, unique per document; used as command/tool name seed
	Protocol        Protocol
	Kind            OperationKind
	Tags            []string
	Summary         string
	Description     string
	Parameters      []Parameter
	RequestBody     []RequestBodyProperty
	HasMultiPart    bool // false for all non-HTTP protocols today; kept for parity
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
