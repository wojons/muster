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
}

// NewGRPCRegistry creates a registry from neutral operations and the gRPC
// adapter's executor. Operations must carry unique IDs (the adapter
// guarantees this per discovery document).
func NewGRPCRegistry(ops []*protocol.Operation, executor protocol.Executor) (*GRPCRegistry, error) {
	if executor == nil {
		return nil, errors.New(errors.CodeValidation, "gRPC registry requires a non-nil executor")
	}
	r := &GRPCRegistry{
		operations: ops,
		executor:   executor,
		schemas:    make(map[string]json.RawMessage, len(ops)),
		byID:       make(map[string]*protocol.Operation, len(ops)),
	}
	for _, op := range ops {
		if _, dup := r.byID[op.ID]; dup {
			return nil, errors.Errorf(errors.CodeValidation, "duplicate gRPC operation ID %q", op.ID)
		}
		r.byID[op.ID] = op
		r.schemas[op.ID] = grpcInputSchema(op)
	}
	return r, nil
}

// grpcInputSchema builds the MCP JSON input schema from the neutral model:
// gRPC message fields are RequestBodyProperty entries, so every field
// becomes a schema property (same conversion pipeline shape
// Converter.ConvertToTools uses for OpenAPI request bodies).
func grpcInputSchema(op *protocol.Operation) json.RawMessage {
	properties := make(map[string]interface{}, len(op.RequestBody))
	required := make([]string, 0, len(op.RequestBody))
	for _, field := range op.RequestBody {
		prop := map[string]interface{}{
			"type": bodyPropTypeToJSONSchema(field.Type),
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
		properties[field.Name] = prop
		if field.Required {
			required = append(required, field.Name)
		}
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		// Deterministic input can only fail on unsupported values; keep the
		// tool registrable with a permissive schema rather than dropping it.
		return json.RawMessage(`{"type":"object"}`)
	}
	return raw
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
