package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/protocol"
)

// GRPCRegistry implements handlers.CommandRegistry (and the ToolLister and
// call-executor surfaces) for gRPC operations discovered through the
// protocol-neutral model (specs/PROTOCOLS.md §3.4). Registration flows
// through the existing handlers.Registry: NewListToolsHandler(registry) +
// NewCallToolHandler(registry) exactly like the OpenAPI path.
//
// Execution delegates to the gRPC adapter's protocol.Executor, which owns
// transport, auth metadata, deadline handling, and the PROTO-003 streaming
// rejection — this registry only adapts the neutral model to MCP shapes.
type GRPCRegistry struct {
	operations []*protocol.Operation
	executor   protocol.Executor
	schemas    map[string]json.RawMessage
	byID       map[string]*protocol.Operation
	// schemaSet is the OPTIONAL type graph discovered alongside the
	// operations. When present, a nested-field reference (Ref) resolves to a
	// real sub-schema in $defs; when absent, a nested field renders exactly
	// as it did before PROTO-016.
	schemaSet *protocol.SchemaSet
}

// GRPCRegistryOption configures optional registry behaviour additively.
type GRPCRegistryOption func(*GRPCRegistry)

// WithSchemaSet supplies the discovered type graph so nested field references
// resolve. It is deliberately optional: the two-argument constructor keeps its
// previous behaviour (a nested field is a bare {"type":"object"}), so no
// existing caller's schemas change silently.
func WithSchemaSet(set *protocol.SchemaSet) GRPCRegistryOption {
	return func(r *GRPCRegistry) { r.schemaSet = set }
}

// NewGRPCRegistry creates a registry from neutral operations and the gRPC
// adapter's executor. Operations must carry unique IDs (the adapter
// guarantees this per discovery document). Options are additive.
func NewGRPCRegistry(ops []*protocol.Operation, executor protocol.Executor, opts ...GRPCRegistryOption) (*GRPCRegistry, error) {
	if executor == nil {
		return nil, errors.New(errors.CodeValidation, "gRPC registry requires a non-nil executor")
	}
	r := &GRPCRegistry{
		operations: ops,
		executor:   executor,
		schemas:    make(map[string]json.RawMessage, len(ops)),
		byID:       make(map[string]*protocol.Operation, len(ops)),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	for _, op := range ops {
		if _, dup := r.byID[op.ID]; dup {
			return nil, errors.Errorf(errors.CodeValidation, "duplicate gRPC operation ID %q", op.ID)
		}
		r.byID[op.ID] = op
		r.schemas[op.ID] = grpcInputSchema(op, r.schemaSet)
	}
	return r, nil
}

// grpcInputSchema builds the MCP JSON input schema from the neutral model:
// gRPC message fields are RequestBodyProperty entries, so every field becomes a
// schema property (same conversion pipeline shape Converter.ConvertToTools uses
// for OpenAPI request bodies).
//
// The depth-1 facts the neutral model now carries are rendered rather than
// dropped: an enum field carries its value names, a repeated field carries its
// item schema, a map field carries additionalProperties, a well-known type
// carries its JSON form, and oneof members carry their mutual exclusion. A
// nested message field is referenced by FQN when a schema set is supplied
// (resolving through $defs) and stays the previous bare
// {"type":"object"} when one is not.
func grpcInputSchema(op *protocol.Operation, set *protocol.SchemaSet) json.RawMessage {
	properties := make(map[string]interface{}, len(op.RequestBody))
	required := make([]string, 0, len(op.RequestBody))
	groups := make(map[string][]string)
	defs := make(map[string]interface{})
	visiting := make(map[string]bool)

	for _, field := range op.RequestBody {
		properties[field.Name] = grpcPropertySchema(field, set, defs, visiting)
		if field.Required {
			required = append(required, field.Name)
		}
		if field.Oneof != "" {
			groups[field.Oneof] = append(groups[field.Oneof], field.Name)
		}
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	// A oneof is "at most one member set": expressible in JSON Schema, without
	// forcing the group to be present at all.
	if clauses := oneofExclusionClauses(groups); len(clauses) > 0 {
		schema["allOf"] = clauses
	}
	if len(defs) > 0 {
		schema["$defs"] = defs
	}

	raw, err := json.Marshal(schema)
	if err != nil {
		// Deterministic input can only fail on unsupported values; keep the
		// tool registrable with a permissive schema rather than dropping it.
		return json.RawMessage(`{"type":"object"}`)
	}
	return raw
}

// grpcPropertySchema renders one request-body property.
func grpcPropertySchema(field protocol.RequestBodyProperty, set *protocol.SchemaSet, defs map[string]interface{}, visiting map[string]bool) map[string]interface{} {
	prop := map[string]interface{}{
		"type": bodyPropTypeToJSONSchema(field.Type),
	}
	if field.Format != "" {
		prop["format"] = field.Format
	}
	if field.Description != "" {
		prop["description"] = field.Description
	}
	if len(field.Enum) > 0 {
		prop["enum"] = field.Enum
	}
	if field.Default != nil {
		prop["default"] = field.Default
	}
	switch {
	case field.Map:
		prop["type"] = "object"
		prop["additionalProperties"] = grpcTypeSchema(field.MapValueType, field.MapValueRef, set, defs, visiting)
	case field.Repeated:
		prop["type"] = "array"
		prop["items"] = grpcTypeSchema(field.ItemType, field.ItemRef, set, defs, visiting)
	case field.Ref != "":
		if _, ok := set.Type(field.Ref); ok {
			prop = map[string]interface{}{"$ref": "#/$defs/" + field.Ref}
			fillSchemaDef(field.Ref, set, defs, visiting)
		}
	}
	return prop
}

// grpcTypeSchema renders the schema of a type by JSON type name plus an optional
// FQN reference (a nested message resolves through $defs when the set has it).
func grpcTypeSchema(jsonType, ref string, set *protocol.SchemaSet, defs map[string]interface{}, visiting map[string]bool) map[string]interface{} {
	if ref != "" {
		if _, ok := set.Type(ref); ok {
			fillSchemaDef(ref, set, defs, visiting)
			return map[string]interface{}{"$ref": "#/$defs/" + ref}
		}
	}
	return map[string]interface{}{"type": bodyPropTypeToJSONSchema(jsonType)}
}

// fillSchemaDef adds a referenced type's JSON Schema to $defs, once, and is
// recursion-safe: the slot is claimed before the type is walked, so a
// self-referential message terminates (its own reference is a legal $ref).
func fillSchemaDef(fqn string, set *protocol.SchemaSet, defs map[string]interface{}, visiting map[string]bool) {
	if _, done := defs[fqn]; done || visiting[fqn] {
		return
	}
	typ, ok := set.Type(fqn)
	if !ok {
		return
	}
	if typ.Kind == protocol.SchemaTypeEnum {
		defs[fqn] = map[string]interface{}{
			"type": "string",
			"enum": typ.EnumValues,
		}
		return
	}
	visiting[fqn] = true
	defer delete(visiting, fqn)

	properties := make(map[string]interface{}, len(typ.Fields))
	var required []string
	groups := make(map[string][]string)
	for _, f := range typ.Fields {
		properties[f.Name] = grpcSchemaFieldProperty(f, set, defs, visiting)
		if f.Required {
			required = append(required, f.Name)
		}
		if f.Oneof != "" {
			groups[f.Oneof] = append(groups[f.Oneof], f.Name)
		}
	}
	def := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		def["required"] = required
	}
	if clauses := oneofExclusionClauses(groups); len(clauses) > 0 {
		def["allOf"] = clauses
	}
	defs[fqn] = def
}

// grpcSchemaFieldProperty renders a SchemaField (a type's field in the graph).
func grpcSchemaFieldProperty(field protocol.SchemaField, set *protocol.SchemaSet, defs map[string]interface{}, visiting map[string]bool) map[string]interface{} {
	switch {
	case field.Map:
		prop := map[string]interface{}{
			"type":                 "object",
			"additionalProperties": grpcTypeSchema(field.MapValueType, field.MapValueRef, set, defs, visiting),
		}
		return prop
	case field.Repeated:
		return map[string]interface{}{
			"type":  "array",
			"items": grpcTypeSchema(field.ItemType, field.ItemRef, set, defs, visiting),
		}
	default:
		prop := grpcTypeSchema(field.Type, field.Ref, set, defs, visiting)
		if field.Format != "" {
			prop["format"] = field.Format
		}
		return prop
	}
}

// oneofExclusionClauses expresses each oneof group's wire rule (at most one
// member present) as JSON Schema clauses: for every member pair, the two must
// not both be present. Groups are emitted in sorted order for stable schemas.
func oneofExclusionClauses(groups map[string][]string) []interface{} {
	if len(groups) == 0 {
		return nil
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)

	var clauses []interface{}
	for _, name := range names {
		members := groups[name]
		sort.Strings(members)
		for i := 0; i < len(members); i++ {
			for j := i + 1; j < len(members); j++ {
				clauses = append(clauses, map[string]interface{}{
					"not": map[string]interface{}{"required": []string{members[i], members[j]}},
				})
			}
		}
	}
	return clauses
}

// grpcToolDescription renders the tool description for one operation.
func grpcToolDescription(op *protocol.Operation) string {
	if op.Description != "" {
		return op.Description
	}
	if op.Summary != "" {
		return op.Summary
	}
	return fmt.Sprintf("Invoke gRPC method %s", op.Source[SourceFullMethodKey])
}

// SourceFullMethodKey mirrors pkg/protocol/grpc's SourceFullMethod key. It
// is redeclared here (not imported) because pkg/protocol.Operation.Source is
// documented as opaque to stage-3 consumers; only this description fallback
// reads it, and importing the adapter package from pkg/mcp would invert the
// seam's dependency direction.
const SourceFullMethodKey = "full_method"

// ListCommands returns the operations as MCP commands.
func (r *GRPCRegistry) ListCommands() []handlers.Command {
	commands := make([]handlers.Command, 0, len(r.operations))
	for _, op := range r.sortedOps() {
		var flags []handlers.Flag
		var schema map[string]interface{}
		if err := json.Unmarshal(r.schemas[op.ID], &schema); err == nil {
			if props, ok := schema["properties"].(map[string]interface{}); ok {
				for propName, propSchema := range props {
					if propMap, ok := propSchema.(map[string]interface{}); ok {
						flags = append(flags, handlers.Flag{
							Name:        propName,
							Type:        getString(propMap, "type", "string"),
							Description: getString(propMap, "description", ""),
							Required:    isRequired(schema, propName),
						})
					}
				}
			}
		}
		sort.Slice(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
		commands = append(commands, handlers.Command{
			Name:        op.ID,
			Description: grpcToolDescription(op),
			Flags:       flags,
		})
	}
	return commands
}

// ListTools returns the operations as MCP tools with input schemas.
func (r *GRPCRegistry) ListTools() []handlers.Tool {
	tools := make([]handlers.Tool, 0, len(r.operations))
	for _, op := range r.sortedOps() {
		tools = append(tools, handlers.Tool{
			Name:        op.ID,
			Description: grpcToolDescription(op),
			InputSchema: r.schemas[op.ID],
		})
	}
	return tools
}

// GetCommand returns one command by name, or nil.
func (r *GRPCRegistry) GetCommand(name string) *handlers.Command {
	op, ok := r.byID[name]
	if !ok {
		return nil
	}
	return &handlers.Command{
		Name:        op.ID,
		Description: grpcToolDescription(op),
	}
}

// Execute runs one tool call: schema validation client-side, then the gRPC
// adapter's Executor (which enforces the §4.2 error mapping and the
// PROTO-003 streaming boundary).
func (r *GRPCRegistry) Execute(ctx context.Context, name string, args map[string]interface{}) (interface{}, error) {
	schema, ok := r.schemas[name]
	if !ok {
		return nil, errors.Errorf(errors.CodeNotFound, "gRPC tool %q not found", name)
	}
	if err := validateGRPCArgs(schema, args); err != nil {
		return nil, err
	}
	return r.executor.Execute(ctx, name, args)
}

// ExecuteCommand satisfies handlers.CommandRegistry.
func (r *GRPCRegistry) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return r.Execute(ctx, name, params)
}

// AddCommand is unsupported: the registry is read-only (matches the OpenAPI
// registry's read-only contract).
func (r *GRPCRegistry) AddCommand(cmd handlers.Command) error {
	return fmt.Errorf("grpc registry is read-only")
}

// RemoveCommand is unsupported: the registry is read-only.
func (r *GRPCRegistry) RemoveCommand(name string) error {
	return fmt.Errorf("grpc registry is read-only")
}

// UpdateCommand is unsupported: the registry is read-only.
func (r *GRPCRegistry) UpdateCommand(name string, cmd handlers.Command) error {
	return fmt.Errorf("grpc registry is read-only")
}

// sortedOps returns operations in stable ID order (deterministic tool lists).
func (r *GRPCRegistry) sortedOps() []*protocol.Operation {
	ops := make([]*protocol.Operation, len(r.operations))
	copy(ops, r.operations)
	sort.Slice(ops, func(i, j int) bool { return ops[i].ID < ops[j].ID })
	return ops
}

// validateGRPCArgs enforces required properties from the input schema before
// any bytes hit the wire (§4.2: client-side validation is CodeValidation).
// Type checking stays lenient (JSON numbers arrive as float64); the adapter's
// message conversion is the strict gate.
func validateGRPCArgs(schemaJSON json.RawMessage, args map[string]interface{}) error {
	if len(schemaJSON) == 0 {
		return nil
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(schemaJSON, &schema); err != nil {
		return nil
	}
	required, _ := schema["required"].([]interface{})
	for _, req := range required {
		name, _ := req.(string)
		if name == "" {
			continue
		}
		if v, present := args[name]; !present || v == nil {
			return errors.Errorf(errors.CodeValidation, "missing required argument %q", name)
		}
	}
	return nil
}
