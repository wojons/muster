package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/protocol"
)

// maxMCPInputDepth bounds how deep the generated MCP input schema recurses into
// nested input objects (a self-referential input type would otherwise never
// finish).
const maxMCPInputDepth = 3

// GraphQLRegistry is the MCP surface for GraphQL operations (stage 3b,
// specs/PROTOCOLS.md §3.4): tool names, flag lists and JSON input schemas derive
// from the neutral protocol.Operation model — the SAME model the CLI surfaces —
// while execution flows through the adapter's protocol.Executor.
//
// It differs from the JSON-RPC registry in one way: GraphQL arguments can be
// input objects, so the registry keeps the ingested schema to describe nested
// object properties, enum members and declared defaults in the tool input
// schema. The argument MODEL is still the neutral one; the schema only adds
// detail the neutral model cannot carry.
type GraphQLRegistry struct {
	operations []*protocol.Operation
	schema     *Schema
	executor   protocol.Executor
}

// compile-time pins: the registry satisfies the same MCP registry interfaces
// pkg/mcp's OpenAPI registry and pkg/protocol/jsonrpc's registry satisfy.
var (
	_ handlers.CommandRegistry = (*GraphQLRegistry)(nil)
	_ handlers.ToolLister      = (*GraphQLRegistry)(nil)
)

// NewGraphQLRegistry builds the MCP registry for the given operations,
// executing through executor (normally the same *Client that discovered them).
func NewGraphQLRegistry(ops []*protocol.Operation, schema *Schema, executor protocol.Executor) (*GraphQLRegistry, error) {
	if len(ops) == 0 {
		return nil, errors.New(errors.CodeValidation, "graphql: no operations to register as MCP tools")
	}
	if executor == nil {
		return nil, errors.New(errors.CodeValidation, "graphql: executor is required for MCP tool registration")
	}
	if err := uniqueIDs(ops); err != nil {
		return nil, err
	}
	return &GraphQLRegistry{operations: ops, schema: schema, executor: executor}, nil
}

// ListCommands implements handlers.CommandRegistry.
func (r *GraphQLRegistry) ListCommands() []handlers.Command {
	commands := make([]handlers.Command, 0, len(r.operations))
	for _, op := range r.operations {
		commands = append(commands, handlers.Command{
			Name:        op.ID,
			Description: r.operationDescription(op),
			Flags:       parametersToFlags(op),
		})
	}
	return commands
}

// GetCommand implements handlers.CommandRegistry.
func (r *GraphQLRegistry) GetCommand(name string) *handlers.Command {
	for _, op := range r.operations {
		if op.ID == name {
			return &handlers.Command{
				Name:        op.ID,
				Description: r.operationDescription(op),
				Flags:       parametersToFlags(op),
			}
		}
	}
	return nil
}

// ExecuteCommand implements handlers.CommandRegistry by delegating to the
// adapter executor (stage 4).
func (r *GraphQLRegistry) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return r.executor.Execute(ctx, name, params)
}

// AddCommand is unsupported: GraphQL-derived tools are read-only.
func (r *GraphQLRegistry) AddCommand(cmd handlers.Command) error {
	return errors.New(errors.CodeValidation, "graphql registry is read-only")
}

// RemoveCommand is unsupported: GraphQL-derived tools are read-only.
func (r *GraphQLRegistry) RemoveCommand(name string) error {
	return errors.New(errors.CodeValidation, "graphql registry is read-only")
}

// UpdateCommand is unsupported: GraphQL-derived tools are read-only.
func (r *GraphQLRegistry) UpdateCommand(name string, cmd handlers.Command) error {
	return errors.New(errors.CodeValidation, "graphql registry is read-only")
}

// ListTools implements handlers.ToolLister: one tool per operation, with an
// input schema built from the SAME argument model the CLI flags come from.
func (r *GraphQLRegistry) ListTools() []handlers.Tool {
	tools := make([]handlers.Tool, 0, len(r.operations))
	for _, op := range r.operations {
		schemaJSON, err := json.Marshal(r.inputSchema(op))
		if err != nil {
			schemaJSON = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		tools = append(tools, handlers.Tool{
			Name:        op.ID,
			Description: r.operationDescription(op),
			InputSchema: schemaJSON,
		})
	}
	return tools
}

// inputSchema builds the JSON-schema object describing one operation's
// arguments.
func (r *GraphQLRegistry) inputSchema(op *protocol.Operation) map[string]interface{} {
	properties := map[string]interface{}{}
	required := []string{}

	field := r.schema.OperationField(op)
	if field == nil {
		// The neutral operation still describes the arguments, even when the
		// schema behind it is unavailable.
		for _, param := range op.Parameters {
			properties[param.Name] = neutralProperty(param)
			if param.Required {
				required = append(required, param.Name)
			}
		}
		return objectSchema(properties, required)
	}

	for i := range field.Args {
		arg := &field.Args[i]
		properties[arg.Name] = r.propertySchema(arg)
		if arg.Required() {
			required = append(required, arg.Name)
		}
	}
	return objectSchema(properties, required)
}

// propertySchema describes one argument, recursing into input objects and
// enums.
//
// jsonSchemaFor may already have attached a description explaining something
// structural (a custom scalar, a depth cap, an undeclared type); a declared
// argument description or the type fallback never overwrites it.
func (r *GraphQLRegistry) propertySchema(arg *InputValue) map[string]interface{} {
	prop := r.jsonSchemaFor(arg.Type, 1)
	if arg.HasDefault {
		prop["default"] = arg.Default
	}
	if _, described := prop["description"]; !described {
		if desc := strings.TrimSpace(arg.Description); desc != "" {
			prop["description"] = desc
		} else {
			prop["description"] = fmt.Sprintf("GraphQL type %s", arg.Type.String())
		}
	}
	return prop
}

// jsonSchemaFor maps a GraphQL type reference onto a JSON-schema fragment.
// Custom scalars fall back to "string" (the documented fallback) and say so.
func (r *GraphQLRegistry) jsonSchemaFor(ref *TypeRef, depth int) map[string]interface{} {
	if ref == nil {
		return map[string]interface{}{"type": "string"}
	}
	if ref.Kind == KindNonNull {
		return r.jsonSchemaFor(ref.OfType, depth)
	}
	if ref.IsList() {
		return map[string]interface{}{
			"type":  "array",
			"items": r.jsonSchemaFor(ref.ListElement(), depth),
		}
	}

	named := ref.Named()
	if named == nil {
		return map[string]interface{}{"type": "string"}
	}
	t := r.schema.Type(named.Name)
	switch {
	case t == nil:
		return map[string]interface{}{
			"type":        "string",
			"description": fmt.Sprintf("undeclared type %s (passed through as a string)", named.Name),
		}
	case t.IsKind(KindEnum):
		values := make([]interface{}, 0, len(t.EnumValues))
		for _, v := range t.EnumValues {
			values = append(values, v)
		}
		return map[string]interface{}{"type": "string", "enum": values}
	case t.IsKind(KindInputObject):
		if depth >= maxMCPInputDepth {
			return map[string]interface{}{
				"type":        "object",
				"description": fmt.Sprintf("input object %s (nested below depth %d)", t.Name, maxMCPInputDepth),
			}
		}
		properties := map[string]interface{}{}
		required := []string{}
		for i := range t.InputFields {
			in := &t.InputFields[i]
			prop := r.jsonSchemaFor(in.Type, depth+1)
			if in.HasDefault {
				prop["default"] = in.Default
			}
			if _, described := prop["description"]; !described {
				if desc := strings.TrimSpace(in.Description); desc != "" {
					prop["description"] = desc
				} else {
					prop["description"] = fmt.Sprintf("GraphQL type %s", in.Type.String())
				}
			}
			properties[in.Name] = prop
			if in.Required() {
				required = append(required, in.Name)
			}
		}
		schema := objectSchema(properties, required)
		schema["description"] = fmt.Sprintf("input object %s", t.Name)
		return schema
	case t.IsKind(KindScalar):
		if t.IsBuiltin {
			return map[string]interface{}{"type": builtinScalarJSONType(t.Name)}
		}
		return map[string]interface{}{
			"type":        "string",
			"description": fmt.Sprintf("custom scalar %s (no JSON-schema type of its own; sent as a string)", t.Name),
		}
	default:
		return map[string]interface{}{
			"type":        "string",
			"description": fmt.Sprintf("%s is an output-only type", t.Name),
		}
	}
}

// neutralProperty describes an argument from the neutral model alone (the
// fallback when the ingested schema is not available to the registry).
func neutralProperty(param protocol.Parameter) map[string]interface{} {
	prop := map[string]interface{}{"type": jsonSchemaType(param.Type)}
	if len(param.Enum) > 0 {
		prop["enum"] = param.Enum
	}
	if param.Default != nil {
		prop["default"] = param.Default
	}
	if param.Description != "" {
		prop["description"] = param.Description
	}
	return prop
}

// objectSchema assembles an object schema, omitting an empty required list.
func objectSchema(properties map[string]interface{}, required []string) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// ServerHandlerRegistrar is the minimal surface needed to register GraphQL tools
// on an MCP server (satisfied by *mcp.Server; kept as an interface so this
// package does not depend on the server implementation and tests can supply a
// stub).
type ServerHandlerRegistrar interface {
	RegisterHandler(handlers.Handler)
}

// MCPHandlers builds the tools/list and tools/call handlers for the given
// operations. tools/list serves input schemas derived from the neutral model
// (plus the schema's nested detail); tools/call executes through the adapter
// executor. Both MCP registration surfaces share this one construction path.
func MCPHandlers(ops []*protocol.Operation, schema *Schema, executor protocol.Executor) (list handlers.Handler, call handlers.Handler, err error) {
	registry, err := NewGraphQLRegistry(ops, schema, executor)
	if err != nil {
		return nil, nil, err
	}
	return handlers.NewListToolsHandler(registry), handlers.NewCallToolHandler(executor), nil
}

// RegisterWithServer wires the discovered GraphQL operations into an MCP server:
// tools/list (via a handlers.ToolLister registry) and tools/call (via the
// adapter executor). This mirrors pkg/mcp's ToolRegistration.RegisterHandlers
// shape and pkg/protocol/jsonrpc.RegisterWithServer.
func RegisterWithServer(server ServerHandlerRegistrar, ops []*protocol.Operation, schema *Schema, executor protocol.Executor) error {
	list, call, err := MCPHandlers(ops, schema, executor)
	if err != nil {
		return err
	}
	server.RegisterHandler(list)
	server.RegisterHandler(call)
	return nil
}

// operationDescription picks the tool description: the field's description,
// else its summary, else a GraphQL-shaped fallback naming operation type and
// field.
func (r *GraphQLRegistry) operationDescription(op *protocol.Operation) string {
	if op.Description != "" {
		return op.Description
	}
	if op.Summary != "" {
		return op.Summary
	}
	return fmt.Sprintf("GraphQL %s field %s", op.Source[SourceOperationType], FieldName(op))
}

// parametersToFlags converts neutral parameters to MCP command flags (matching
// pkg/mcp's OpenAPI path and the JSON-RPC adapter's shape).
func parametersToFlags(op *protocol.Operation) []handlers.Flag {
	flags := make([]handlers.Flag, 0, len(op.Parameters))
	for _, param := range op.Parameters {
		flags = append(flags, handlers.Flag{
			Name:        param.Name,
			Type:        jsonSchemaType(param.Type),
			Description: param.Description,
			Required:    param.Required,
		})
	}
	return flags
}

// jsonSchemaType maps the neutral type vocabulary onto JSON-schema types.
func jsonSchemaType(t string) string {
	if t == "" {
		return "string"
	}
	return t
}
