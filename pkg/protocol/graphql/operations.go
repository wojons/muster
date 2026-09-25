package graphql

import (
	"fmt"
	"strings"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// OperationType names the two root operation types this adapter ingests.
// Subscriptions are a third GraphQL root but are deliberately out of scope for
// the bounded first row (specs/PROTOCOLS.md §2 row 6).
type OperationType string

const (
	// OperationQuery is the read root (the schema's query type).
	OperationQuery OperationType = "query"
	// OperationMutation is the write root (the schema's mutation type).
	OperationMutation OperationType = "mutation"
	// OperationSubscription is GraphQL's third root; recognized and skipped.
	OperationSubscription OperationType = "subscription"
)

// Source-stamp keys stamped on every ingested operation. Operation.Source is
// where an adapter records ground truth for its own executor
// (specs/PROTOCOLS.md §3.2); stage-3 consumers never interpret these keys, and
// discovery output renders them so an operator can see exactly what was
// ingested and from where.
const (
	// SourceField is the schema field name the operation was derived from.
	SourceField = "field"
	// SourceOperationType is "query" or "mutation".
	SourceOperationType = "operation_type"
	// SourceFieldType is the field's GraphQL return type ("[User!]!").
	SourceFieldType = "field_type"
	// SourceArgTypes is the field's argument list as "name:Type" pairs in
	// declaration order ("id:ID!,filter:UserFilter") — the GraphQL types behind
	// the neutral parameter types.
	SourceArgTypes = "arg_types"
	// SourceEndpoint is the endpoint an invocation defaults to, when the
	// ingestion source named one (an introspection target or a URL).
	SourceEndpoint = "endpoint"
	// SourceSchemaSource is the discovery URI the schema was ingested from.
	SourceSchemaSource = "schema_source"
	// SourceSchemaFormat is which ingestion path produced it: sdl,
	// introspection-json, or introspection-query.
	SourceSchemaFormat = "schema_format"
)

// FieldCounts summarizes the fields a schema exposes per root operation type.
type FieldCounts struct {
	Queries       int
	Mutations     int
	Subscriptions int
}

// Counts reports how many fields each root operation type exposes.
func (s *Schema) Counts() FieldCounts {
	var c FieldCounts
	if root := s.QueryRoot(); root != nil {
		c.Queries = len(root.Fields)
	}
	if root := s.MutationRoot(); root != nil {
		c.Mutations = len(root.Fields)
	}
	if root := s.SubscriptionRoot(); root != nil {
		c.Subscriptions = len(root.Fields)
	}
	return c
}

// Operations maps the schema's Query fields onto query operations and its
// Mutation fields onto mutation operations, on the protocol-neutral operation
// model (stage 2 mapping, specs/PROTOCOLS.md §3.2). Subscription fields are
// skipped: they are the follow-up row, and a bounded first row ships nothing
// half-supported.
//
// Every field becomes exactly one KindUnary operation (GraphQL queries and
// mutations are single request → single response). endpoint, when non-empty, is
// stamped onto each operation's source so the executor can invoke without the
// caller repeating it (single-endpoint servers make this the normal case).
//
// IDs are the field names. A mutation whose field name is already claimed by a
// query keeps the bare name when it is free and otherwise takes the
// "mutation_<field>" form, so IDs stay unique and deterministic; a remaining
// collision is reported instead of silently overwriting.
func (s *Schema) Operations(endpoint string) ([]*protocol.Operation, error) {
	ops := make([]*protocol.Operation, 0, s.Counts().Queries+s.Counts().Mutations)

	claimed := make(map[string]bool)
	appendRoot := func(root *Type, opType OperationType) error {
		if root == nil {
			return nil
		}
		for i := range root.Fields {
			field := &root.Fields[i]
			id := field.Name
			if opType == OperationMutation && claimed[id] {
				id = string(OperationMutation) + "_" + field.Name
			}
			op, err := s.operation(id, opType, field, endpoint)
			if err != nil {
				return err
			}
			claimed[id] = true
			ops = append(ops, op)
		}
		return nil
	}

	if err := appendRoot(s.QueryRoot(), OperationQuery); err != nil {
		return nil, err
	}
	if err := appendRoot(s.MutationRoot(), OperationMutation); err != nil {
		return nil, err
	}
	if err := uniqueIDs(ops); err != nil {
		return nil, err
	}
	return ops, nil
}

// operation builds one neutral operation from one schema field.
func (s *Schema) operation(id string, opType OperationType, field *Field, endpoint string) (*protocol.Operation, error) {
	params := make([]protocol.Parameter, 0, len(field.Args))
	argTypes := make([]string, 0, len(field.Args))
	for i := range field.Args {
		arg := &field.Args[i]
		params = append(params, protocol.Parameter{
			Name:        arg.Name,
			In:          ParameterLocationVariable,
			Type:        s.NeutralType(arg.Type),
			Required:    arg.Required(),
			Enum:        s.EnumValues(arg.Type),
			Default:     arg.Default,
			Description: argumentDescription(arg),
		})
		argTypes = append(argTypes, fmt.Sprintf("%s:%s", arg.Name, arg.Type.String()))
	}

	source := map[string]string{
		SourceField:         field.Name,
		SourceOperationType: string(opType),
		SourceFieldType:     field.Type.String(),
		SourceSchemaSource:  s.Source,
		SourceSchemaFormat:  s.SourceFormat,
	}
	if len(argTypes) > 0 {
		source[SourceArgTypes] = strings.Join(argTypes, ",")
	}
	if endpoint != "" {
		source[SourceEndpoint] = endpoint
	}

	return &protocol.Operation{
		ID:          id,
		Protocol:    protocol.ProtocolGraphQL,
		Kind:        protocol.KindUnary,
		Tags:        []string{string(opType)},
		Summary:     firstSentence(field.Description),
		Description: field.Description,
		Parameters:  params,
		Source:      source,
	}, nil
}

// ParameterLocationVariable is the protocol.Parameter.In value every GraphQL
// argument carries: the argument is bound as a GraphQL variable, never
// interpolated into the query document.
const ParameterLocationVariable = "graphql-variable"

// argumentDescription keeps the GraphQL type visible to operators once the
// neutral (JSON-schema-ish) type has replaced it, and states the default when
// the schema declares one. The GraphQL type is also stamped verbatim in the
// operation's arg_types source key.
func argumentDescription(arg *InputValue) string {
	parts := make([]string, 0, 2)
	if desc := strings.TrimSpace(arg.Description); desc != "" {
		parts = append(parts, desc)
	}
	typePart := fmt.Sprintf("GraphQL type %s", arg.Type.String())
	if arg.HasDefault {
		typePart += fmt.Sprintf(", default %s", arg.DefaultValue)
	}
	parts = append(parts, typePart)
	return strings.Join(parts, " — ")
}

// uniqueIDs verifies operation IDs are unique within a document
// (specs/PROTOCOLS.md §3.1).
func uniqueIDs(ops []*protocol.Operation) error {
	seen := make(map[string]bool, len(ops))
	for _, op := range ops {
		if seen[op.ID] {
			return errors.New(errors.CodeValidation, fmt.Sprintf("graphql: duplicate operation ID %q in schema", op.ID))
		}
		seen[op.ID] = true
	}
	return nil
}

// OperationByID returns the operation with the given ID, or nil.
func OperationByID(ops []*protocol.Operation, id string) *protocol.Operation {
	for _, op := range ops {
		if op.ID == id {
			return op
		}
	}
	return nil
}

// OperationField resolves the schema field an ingested operation was derived
// from, using the operation's source stamp (the operation type and field name).
// It returns nil when the schema no longer describes the field — the request
// builder turns that into a typed error rather than guessing.
func (s *Schema) OperationField(op *protocol.Operation) *Field {
	if s == nil || op == nil {
		return nil
	}
	var root *Type
	switch OperationType(op.Source[SourceOperationType]) {
	case OperationQuery:
		root = s.QueryRoot()
	case OperationMutation:
		root = s.MutationRoot()
	default:
		return nil
	}
	if root == nil {
		return nil
	}
	name := op.Source[SourceField]
	if name == "" {
		name = op.ID
	}
	return root.Field(name)
}

// OperationIDs returns the operations' IDs in document order.
func OperationIDs(ops []*protocol.Operation) []string {
	ids := make([]string, 0, len(ops))
	for _, op := range ops {
		ids = append(ids, op.ID)
	}
	return ids
}

// FieldName returns the schema field name an ingested operation was derived
// from (the source stamp is the authority; the operation ID falls back to it).
func FieldName(op *protocol.Operation) string {
	if op == nil {
		return ""
	}
	if name := op.Source[SourceField]; name != "" {
		return name
	}
	return op.ID
}
