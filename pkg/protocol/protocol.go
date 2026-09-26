// Package protocol defines the protocol-neutral operation model and the
// adapter seam every protocol backend implements.
//
// This package is the shared source of truth frozen in
// specs/PROTOCOLS.md §1.3: adapters (OpenAPI, gRPC, JSON-RPC, GraphQL, ...)
// implement Discoverer + Executor and register with Registry. Existing
// packages (pkg/generator, pkg/mcp, pkg/client) remain the shared stage-3
// and stage-4 machinery; new protocol adapters add their own packages.
package protocol

import (
	"context"
	"sort"
)

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
// rules (specs/PROTOCOLS.md §4.5) key off this value.
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
//
// The fields below Default carry the DEPTH-1 fidelity facts a descriptor graph
// knows but a flat name/type pair cannot express (PROTO-013 §2.5, decision D4).
// They are ADDITIVE: an adapter with nothing to say leaves them zero, and a
// consumer that ignores them renders what it rendered before.
type RequestBodyProperty struct {
	Name        string
	Type        string
	Description string
	Required    bool
	Enum        []interface{}
	Default     interface{}

	// Ref is the fully-qualified name of the message type this property
	// references; empty for scalars, enums and well-known types (whose wire
	// JSON is not an object of the type). Resolvable in a SchemaSet.
	Ref string
	// Format is the JSON-schema-style format of the property's wire JSON
	// (e.g. "date-time" for google.protobuf.Timestamp); empty when the type
	// has no special JSON form.
	Format string
	// ItemType/ItemRef describe the element of a Repeated property.
	ItemType string
	ItemRef  string
	// Repeated and Map state the cardinality explicitly, so a consumer need
	// not infer it from Type.
	Repeated bool
	Map      bool
	// MapKeyType/MapValueType/MapValueRef describe a map property's key and
	// value types (the JSON object form of a proto map).
	MapKeyType   string
	MapValueType string
	MapValueRef  string
	// Oneof names the oneof group this property is a member of; empty when it
	// is not a member. Properties sharing a group name are mutually exclusive
	// on the wire, which is the constraint a JSON Schema expresses as
	// "not both present".
	Oneof string
}

// SchemaTypeKind classifies a schema declaration's form.
type SchemaTypeKind string

const (
	SchemaTypeMessage SchemaTypeKind = "message" // fields + nested declarations
	SchemaTypeEnum    SchemaTypeKind = "enum"    // value names
)

// SchemaField is one field of a SchemaType: the same depth-1 facts
// RequestBodyProperty carries, in the graph's own terms.
type SchemaField struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // JSON-schema-style wire type ("string","object","array",...)
	Format       string   `json:"format,omitempty"`
	Description  string   `json:"description,omitempty"`
	Required     bool     `json:"required,omitempty"`
	Enum         []string `json:"enum,omitempty"` // value names, for enum-typed fields
	Ref          string   `json:"ref,omitempty"`  // FQN of the referenced message type; empty for scalars
	Repeated     bool     `json:"repeated,omitempty"`
	ItemType     string   `json:"item_type,omitempty"`
	ItemRef      string   `json:"item_ref,omitempty"`
	Map          bool     `json:"map,omitempty"`
	MapKeyType   string   `json:"map_key_type,omitempty"`
	MapValueType string   `json:"map_value_type,omitempty"`
	MapValueRef  string   `json:"map_value_ref,omitempty"`
	Oneof        string   `json:"oneof,omitempty"` // oneof group name; empty when not a member
}

// SchemaType is one named declaration in a SchemaSet.
type SchemaType struct {
	Name        string         `json:"name"` // fully-qualified name
	Kind        SchemaTypeKind `json:"kind"`
	Description string         `json:"description,omitempty"`
	Fields      []SchemaField  `json:"fields,omitempty"`
	EnumValues  []string       `json:"enum_values,omitempty"`
}

// SchemaSet is a protobuf type graph keyed by fully-qualified name — the
// stage-2 output of a schema ingester (PROTO-013 §2.4, decision D1/D3). It is
// deliberately NOT part of Operation: an operation is operation-shaped, while a
// schema set is a type graph that may describe a source with no operations at
// all. There is no Executor for a SchemaSet: its consumers render, validate or
// emit, and never call.
type SchemaSet struct {
	// Types maps a fully-qualified type name ("schema.Outer") to its
	// declaration. Map entries are synthetic nested message types and are
	// named as the descriptor names them ("schema.Outer.AttrsEntry").
	Types map[string]SchemaType `json:"types"`
	// Extensions carries protobuf extension fields declared by the files in
	// the set: extension fields are not members of the extended message, so
	// they are listed here rather than mixed into a type's fields.
	Extensions []SchemaField `json:"extensions,omitempty"`
	// Source records where the set came from, reusing Operation.Source's
	// vocabulary: "descriptors" = proto | descriptorset | reflection, plus
	// "host" for reflection.
	Source map[string]string `json:"source,omitempty"`
}

// Type returns the named declaration and whether it is present.
func (s *SchemaSet) Type(fqn string) (SchemaType, bool) {
	if s == nil || s.Types == nil {
		return SchemaType{}, false
	}
	t, ok := s.Types[fqn]
	return t, ok
}

// Names returns every fully-qualified type name in stable sorted order.
func (s *SchemaSet) Names() []string {
	if s == nil {
		return nil
	}
	names := make([]string, 0, len(s.Types))
	for name := range s.Types {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
// rendering today, see specs/PROTOCOLS.md §4.4). Adapters own their transport,
// auth wiring, and retry/rate-limit integration (§4.2–4.4) behind this method.
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
