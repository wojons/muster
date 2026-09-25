package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/protocol"
)

// JSONRPCRegistry implements handlers.CommandRegistry (and handlers.ToolLister)
// for JSON-RPC operations discovered from an OpenRPC document. It is the
// stage-3b surface: tools derive names and input schemas from the neutral
// protocol.Operation model exactly the way pkg/mcp's OpenAPI registry derives
// them from openapi.Operation, and execution flows through the adapter's
// protocol.Executor implementation.
type JSONRPCRegistry struct {
	operations []*protocol.Operation
	executor   protocol.Executor
}

// compile-time pins: the registry satisfies the MCP registry interfaces the
// same way pkg/mcp.OpenAPIRegistry does.
var (
	_ handlers.CommandRegistry = (*JSONRPCRegistry)(nil)
	_ handlers.ToolLister      = (*JSONRPCRegistry)(nil)
)

// NewJSONRPCRegistry builds the MCP registry for the given operations,
// executing through executor (normally the same *Client that discovered
// them, per specs/PROTOCOLS.md §3.4: shared generation pipeline, adapter's
// own executor).
func NewJSONRPCRegistry(ops []*protocol.Operation, executor protocol.Executor) (*JSONRPCRegistry, error) {
	if len(ops) == 0 {
		return nil, errors.New(errors.CodeValidation, "jsonrpc: no operations to register as MCP tools")
	}
	if executor == nil {
		return nil, errors.New(errors.CodeValidation, "jsonrpc: executor is required for MCP tool registration")
	}
	if err := uniqueIDs(ops); err != nil {
		return nil, err
	}
	return &JSONRPCRegistry{operations: ops, executor: executor}, nil
}

// ListCommands implements handlers.CommandRegistry.
func (r *JSONRPCRegistry) ListCommands() []handlers.Command {
	commands := make([]handlers.Command, 0, len(r.operations))
	for _, op := range r.operations {
		commands = append(commands, handlers.Command{
			Name:        op.ID,
			Description: operationDescription(op),
			Flags:       parametersToFlags(op),
		})
	}
	return commands
}

// GetCommand implements handlers.CommandRegistry.
func (r *JSONRPCRegistry) GetCommand(name string) *handlers.Command {
	for _, op := range r.operations {
		if op.ID == name {
			return &handlers.Command{
				Name:        op.ID,
				Description: operationDescription(op),
				Flags:       parametersToFlags(op),
			}
		}
	}
	return nil
}

// ExecuteCommand implements handlers.CommandRegistry by delegating to the
// adapter executor (stage 4).
func (r *JSONRPCRegistry) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return r.executor.Execute(ctx, name, params)
}

// AddCommand is unsupported: JSON-RPC-derived tools are read-only.
func (r *JSONRPCRegistry) AddCommand(cmd handlers.Command) error {
	return errors.New(errors.CodeValidation, "jsonrpc registry is read-only")
}

// RemoveCommand is unsupported: JSON-RPC-derived tools are read-only.
func (r *JSONRPCRegistry) RemoveCommand(name string) error {
	return errors.New(errors.CodeValidation, "jsonrpc registry is read-only")
}

// UpdateCommand is unsupported: JSON-RPC-derived tools are read-only.
func (r *JSONRPCRegistry) UpdateCommand(name string, cmd handlers.Command) error {
	return errors.New(errors.CodeValidation, "jsonrpc registry is read-only")
}

// ListTools implements handlers.ToolLister: tools with populated input
// schemas built from the neutral model (same shape pkg/mcp's OpenAPI
// ToolLister produces — {"type":"object","properties":{...},"required":[...]}).
func (r *JSONRPCRegistry) ListTools() []handlers.Tool {
	tools := make([]handlers.Tool, 0, len(r.operations))
	for _, op := range r.operations {
		properties := make(map[string]interface{}, len(op.Parameters))
		required := make([]string, 0, len(op.Parameters))

		for _, param := range op.Parameters {
			propMap := map[string]interface{}{
				"type": jsonSchemaType(param.Type),
			}
			if param.Description != "" {
				propMap["description"] = param.Description
			}
			properties[param.Name] = propMap
			if param.Required {
				required = append(required, param.Name)
			}
		}

		schema := map[string]interface{}{
			"type":       "object",
			"properties": properties,
		}
		if len(required) > 0 {
			schema["required"] = required
		}

		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			schemaJSON = json.RawMessage(`{"type":"object","properties":{}}`)
		}

		tools = append(tools, handlers.Tool{
			Name:        op.ID,
			Description: operationDescription(op),
			InputSchema: schemaJSON,
		})
	}
	return tools
}

// ServerHandlerRegistrar is the minimal surface needed to register JSON-RPC
// tools on an MCP server (satisfied by *mcp.Server; kept as an interface so
// this package does not depend on the server implementation and tests can
// supply a stub).
type ServerHandlerRegistrar interface {
	RegisterHandler(handlers.Handler)
}

// MCPHandlers builds the tools/list and tools/call handlers for the given
// operations. tools/list serves input schemas from the neutral model;
// tools/call executes through the adapter executor. Both MCP registration
// surfaces (RegisterWithServer for *mcp.Server, direct handlers.Registry
// wiring in tests and embedders) share this one construction path.
func MCPHandlers(ops []*protocol.Operation, executor protocol.Executor) (list handlers.Handler, call handlers.Handler, err error) {
	registry, err := NewJSONRPCRegistry(ops, executor)
	if err != nil {
		return nil, nil, err
	}
	return handlers.NewListToolsHandler(registry), handlers.NewCallToolHandler(executor), nil
}

// RegisterWithServer wires the discovered JSON-RPC operations into an MCP
// server: tools/list (via a handlers.ToolLister registry) and tools/call
// (via the adapter executor). This mirrors pkg/mcp's
// ToolRegistration.RegisterHandlers shape.
func RegisterWithServer(server ServerHandlerRegistrar, ops []*protocol.Operation, executor protocol.Executor) error {
	list, call, err := MCPHandlers(ops, executor)
	if err != nil {
		return err
	}
	server.RegisterHandler(list)
	server.RegisterHandler(call)
	return nil
}

// operationDescription picks the tool description the same way the OpenAPI
// path does: description, else summary, else a method-line fallback.
func operationDescription(op *protocol.Operation) string {
	if op.Description != "" {
		return op.Description
	}
	if op.Summary != "" {
		return op.Summary
	}
	return fmt.Sprintf("JSON-RPC method %s", op.ID)
}

// parametersToFlags converts neutral parameters to MCP command flags
// (matching pkg/mcp.convertParameters' shape for the OpenAPI path).
func parametersToFlags(op *protocol.Operation) []handlers.Flag {
	flags := make([]handlers.Flag, 0, len(op.Parameters))
	for _, param := range op.Parameters {
		flagType := param.Type
		if flagType == "" {
			flagType = "string"
		}
		flags = append(flags, handlers.Flag{
			Name:        param.Name,
			Type:        flagType,
			Description: param.Description,
			Required:    param.Required,
		})
	}
	return flags
}

// jsonSchemaType maps the neutral type vocabulary onto JSON-schema types
// (pkg/mcp uses the same vocabulary, so this is mostly identity with a
// fallback default).
func jsonSchemaType(t string) string {
	if t == "" {
		return "string"
	}
	return t
}
