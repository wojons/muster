package asyncapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/protocol"
)

// MCP hard caps (specs/PROTOCOLS.md §4.5(4), PROTO-007 §4): an operation
// input may LOWER a bound below the defaults but never RAISE it past the
// cap. The CLI path configures its own bounds through WithStreamLimits;
// the MCP path clamps here.
const (
	MCPHardCapMaxMessages = DefaultMaxMessages
	MCPHardCapMaxDuration = DefaultMaxDuration
)

// AsyncAPIRegistry is the MCP surface for AsyncAPI operations (stage 3b,
// specs/PROTOCOLS.md §3.4): tool names and input schemas derive from the
// neutral protocol.Operation model, execution flows through the adapter's
// protocol.Executor.
//
// The one adapter-specific rule is §4.5(4): KindEventSource operations are
// exposed as BOUNDED-COLLECT tools — subscribe, collect up to the caps,
// close, return the transcript. A consume tool never outlives its tool
// call.
type AsyncAPIRegistry struct {
	operations []*protocol.Operation
	executor   protocol.Executor
	byID       map[string]*protocol.Operation
}

// compile-time pins: the registry satisfies the same MCP registry
// interfaces pkg/mcp's OpenAPI registry and the GraphQL/JSON-RPC
// registries satisfy.
var (
	_ handlers.CommandRegistry = (*AsyncAPIRegistry)(nil)
	_ handlers.ToolLister      = (*AsyncAPIRegistry)(nil)
)

// NewAsyncAPIRegistry builds the MCP registry for the given operations,
// executing through executor (normally a *Client whose stream limits are
// already clamped to the MCP hard caps — construct it with
// WithStreamLimits(EventLimits{MCPHardCapMaxMessages, MCPHardCapMaxDuration})).
func NewAsyncAPIRegistry(ops []*protocol.Operation, executor protocol.Executor) (*AsyncAPIRegistry, error) {
	if len(ops) == 0 {
		return nil, errors.New(errors.CodeValidation, "asyncapi: no operations to register as MCP tools")
	}
	if executor == nil {
		return nil, errors.New(errors.CodeValidation, "asyncapi: executor is required for MCP tool registration")
	}
	r := &AsyncAPIRegistry{
		operations: ops,
		executor:   executor,
		byID:       make(map[string]*protocol.Operation, len(ops)),
	}
	for _, op := range ops {
		if _, dup := r.byID[op.ID]; dup {
			return nil, errors.Errorf(errors.CodeValidation, "duplicate asyncapi operation ID %q", op.ID)
		}
		r.byID[op.ID] = op
	}
	return r, nil
}

// ListCommands implements handlers.CommandRegistry.
func (r *AsyncAPIRegistry) ListCommands() []handlers.Command {
	commands := make([]handlers.Command, 0, len(r.operations))
	for _, op := range r.operations {
		commands = append(commands, handlers.Command{
			Name:        op.ID,
			Description: operationDescription(op),
			Flags:       operationFlags(op),
		})
	}
	return commands
}

// GetCommand implements handlers.CommandRegistry.
func (r *AsyncAPIRegistry) GetCommand(name string) *handlers.Command {
	op, ok := r.byID[name]
	if !ok {
		return nil
	}
	return &handlers.Command{
		Name:        op.ID,
		Description: operationDescription(op),
		Flags:       operationFlags(op),
	}
}

// ExecuteCommand implements handlers.CommandRegistry by delegating to the
// adapter executor (stage 4). Consume calls carry the bounded-collect
// knobs (max_messages, timeout_seconds) described by the tool schema; they
// are lower bounds only — the executor's limits cap what a call may raise.
func (r *AsyncAPIRegistry) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return r.executor.Execute(ctx, name, params)
}

// AddCommand is unsupported: document-derived tools are read-only.
func (r *AsyncAPIRegistry) AddCommand(handlers.Command) error {
	return errors.New(errors.CodeValidation, "asyncapi registry is read-only")
}

// RemoveCommand is unsupported: document-derived tools are read-only.
func (r *AsyncAPIRegistry) RemoveCommand(string) error {
	return errors.New(errors.CodeValidation, "asyncapi registry is read-only")
}

// UpdateCommand is unsupported: document-derived tools are read-only.
func (r *AsyncAPIRegistry) UpdateCommand(string, handlers.Command) error {
	return errors.New(errors.CodeValidation, "asyncapi registry is read-only")
}

// ListTools implements handlers.ToolLister: one tool per operation with an
// input schema built from the neutral model. Consume tools carry the
// bounded-collect knobs (max_messages, timeout_seconds) and a description
// that names the bound semantics — an agent is never left with a hung
// call.
func (r *AsyncAPIRegistry) ListTools() []handlers.Tool {
	tools := make([]handlers.Tool, 0, len(r.operations))
	for _, op := range r.operations {
		schemaJSON, err := json.Marshal(inputSchema(op))
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

// inputSchema builds the JSON-schema object describing one operation's
// invocation arguments: channel parameters, message headers, payload
// fields (or the whole-message payload argument), and the reserved _key
// argument. Consume operations additionally expose the bounded-collect
// knobs.
func inputSchema(op *protocol.Operation) map[string]interface{} {
	properties := map[string]interface{}{}
	required := []string{}

	for _, p := range op.Parameters {
		prop := map[string]interface{}{"type": schemaTypeForNeutral(p.Type)}
		if len(p.Enum) > 0 {
			prop["enum"] = p.Enum
		}
		if p.Default != nil {
			prop["default"] = p.Default
		}
		if p.Description != "" {
			prop["description"] = p.Description
		}
		properties[p.Name] = prop
		if p.Required {
			required = append(required, p.Name)
		}
	}
	for _, f := range op.RequestBody {
		prop := map[string]interface{}{"type": schemaTypeForNeutral(f.Type)}
		if len(f.Enum) > 0 {
			prop["enum"] = f.Enum
		}
		if f.Default != nil {
			prop["default"] = f.Default
		}
		if f.Description != "" {
			prop["description"] = f.Description
		}
		properties[f.Name] = prop
		if f.Required {
			required = append(required, f.Name)
		}
	}
	if op.Kind == protocol.KindUnary {
		properties[reservedArgKey] = map[string]interface{}{
			"type":        "string",
			"description": "Kafka message key for the produced record (reserved invocation argument, not a payload field)",
		}
	} else if op.Kind == protocol.KindEventSource {
		// Bounded-collect knobs: lower bounds only; the executor clamps at
		// the MCP hard caps (§4.5(4)) — an unset/zero value selects the
		// defaults and the caps cannot be raised.
		properties["max_messages"] = map[string]interface{}{
			"type":        "integer",
			"description": fmt.Sprintf("maximum messages to collect before returning the transcript (default %d, hard cap %d)", DefaultMaxMessages, MCPHardCapMaxMessages),
			"minimum":     1,
			"maximum":     MCPHardCapMaxMessages,
		}
		properties["timeout_seconds"] = map[string]interface{}{
			"type":        "number",
			"description": fmt.Sprintf("maximum wall-clock seconds of the collect (default %d, hard cap %d)", int(DefaultMaxDuration.Seconds()), int(MCPHardCapMaxDuration.Seconds())),
			"minimum":     1,
			"maximum":     int(MCPHardCapMaxDuration.Seconds()),
		}
	}

	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// operationDescription picks the tool description; event-source tools
// state the bounded-collect contract explicitly.
func operationDescription(op *protocol.Operation) string {
	desc := op.Description
	if desc == "" {
		desc = op.Summary
	}
	if op.Kind == protocol.KindEventSource {
		return strings.TrimSpace(desc +
			fmt.Sprintf(" — bounded collect: subscribes, collects up to max_messages within timeout_seconds (default %d / %d), closes the subscription, and returns the transcript; never a long-lived consumer.",
				DefaultMaxMessages, int(DefaultMaxDuration.Seconds())))
	}
	return desc
}

// operationFlags converts the neutral model to MCP command flags.
func operationFlags(op *protocol.Operation) []handlers.Flag {
	flags := make([]handlers.Flag, 0, len(op.Parameters))
	for _, param := range op.Parameters {
		flags = append(flags, handlers.Flag{
			Name:        param.Name,
			Type:        schemaTypeForNeutral(param.Type),
			Description: param.Description,
			Required:    param.Required,
		})
	}
	return flags
}

// schemaTypeForNeutral maps the neutral type vocabulary onto JSON-schema
// types; empty falls back to string.
func schemaTypeForNeutral(t string) string {
	if t == "" {
		return "string"
	}
	return t
}

// ServerHandlerRegistrar mirrors the GraphQL/JSON-RPC registration shape:
// satisfied by *mcp.Server; kept as an interface so this package does not
// depend on the server implementation.
type ServerHandlerRegistrar interface {
	RegisterHandler(handlers.Handler)
}

// MCPHandlers builds the tools/list and tools/call handlers for the given
// operations.
func MCPHandlers(ops []*protocol.Operation, executor protocol.Executor) (list handlers.Handler, call handlers.Handler, err error) {
	registry, err := NewAsyncAPIRegistry(ops, executor)
	if err != nil {
		return nil, nil, err
	}
	return handlers.NewListToolsHandler(registry), handlers.NewCallToolHandler(executor), nil
}

// RegisterWithServer wires the discovered AsyncAPI operations into an MCP
// server: tools/list (via a handlers.ToolLister registry) and tools/call
// (via the adapter executor). Mirrors pkg/protocol/graphql and
// pkg/protocol/jsonrpc registration.
func RegisterWithServer(server ServerHandlerRegistrar, ops []*protocol.Operation, executor protocol.Executor) error {
	list, call, err := MCPHandlers(ops, executor)
	if err != nil {
		return err
	}
	server.RegisterHandler(list)
	server.RegisterHandler(call)
	return nil
}
