package graphql_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/mcp"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/protocol/graphql"
)

// mcpRegistryFor wires the MCP registry for a fixture schema plus a live client.
func mcpRegistryFor(t *testing.T, srv *testServer) (*graphql.GraphQLRegistry, *graphql.Client) {
	t.Helper()
	_, schema := discoverFrom(t, sdlFixturePath)
	client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))
	ops, err := client.Discover(context.Background(), sdlFixturePath)
	require.NoError(t, err)

	registry, err := graphql.NewGraphQLRegistry(ops, schema, client)
	require.NoError(t, err)
	return registry, client
}

// toolByName returns the named tool, failing when it is absent.
func toolByName(t *testing.T, tools []handlers.Tool, name string) handlers.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("no tool named %q (have %d tools)", name, len(tools))
	return handlers.Tool{}
}

// inputSchema decodes a tool's input schema.
func inputSchema(t *testing.T, tool handlers.Tool) map[string]interface{} {
	t.Helper()
	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(tool.InputSchema, &schema))
	return schema
}

// properties returns an input schema's property map.
func properties(t *testing.T, schema map[string]interface{}) map[string]interface{} {
	t.Helper()
	props, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok, "input schema has no properties object: %v", schema)
	return props
}

// requiredList returns an input schema's required list.
func requiredList(t *testing.T, schema map[string]interface{}) []interface{} {
	t.Helper()
	required, _ := schema["required"].([]interface{})
	return required
}

func TestMCP_ToolsMirrorTheCLIArgumentModel(t *testing.T) {
	srv := newTestServer(t)
	registry, _ := mcpRegistryFor(t, srv)

	tools := registry.ListTools()
	require.Len(t, tools, 9, "one tool per ingested operation")
	assert.Equal(t, []string{"user", "users", "nodes", "search", "ping", "viewer", "createUser", "setRole", "noop"},
		toolNames(tools))

	// A query with a required argument, an optional Int with a default, and an
	// enum: the same argument model the CLI flags express.
	users := inputSchema(t, toolByName(t, tools, "users"))
	usersProps := properties(t, users)
	assert.Empty(t, requiredList(t, users), "both arguments are optional")
	assert.Equal(t, "integer", usersProps["limit"].(map[string]interface{})["type"])
	assert.EqualValues(t, 20, usersProps["limit"].(map[string]interface{})["default"])
	assert.Equal(t, []interface{}{"ADMIN", "MEMBER", "GUEST"}, usersProps["role"].(map[string]interface{})["enum"])

	// An input-object argument carries its nested shape, defaults included.
	createUser := inputSchema(t, toolByName(t, tools, "createUser"))
	assert.Equal(t, []interface{}{"input"}, requiredList(t, createUser))

	input := properties(t, createUser)["input"].(map[string]interface{})
	assert.Equal(t, "object", input["type"])
	inputProps := input["properties"].(map[string]interface{})
	assert.Equal(t, []interface{}{"name"}, input["required"], "only the non-null input field without a default is required")

	assert.Equal(t, "string", inputProps["name"].(map[string]interface{})["type"])
	assert.Equal(t, "MEMBER", inputProps["role"].(map[string]interface{})["default"])
	assert.Equal(t, []interface{}{"ADMIN", "MEMBER", "GUEST"}, inputProps["role"].(map[string]interface{})["enum"])

	tags := inputProps["tags"].(map[string]interface{})
	assert.Equal(t, "array", tags["type"])
	assert.Equal(t, "string", tags["items"].(map[string]interface{})["type"])

	profile := inputProps["profile"].(map[string]interface{})
	assert.Equal(t, "object", profile["type"])
	profileProps := profile["properties"].(map[string]interface{})
	assert.Equal(t, ``, profileProps["website"].(map[string]interface{})["default"], "the nested default is reported as declared")

	// A custom scalar says what it is instead of pretending to be a JSON type.
	viewer := inputSchema(t, toolByName(t, tools, "viewer"))
	assert.Equal(t, "object", viewer["type"])
	assert.Empty(t, properties(t, viewer))
}

func TestMCP_CustomScalarFieldIsDocumentedInTheSchema(t *testing.T) {
	srv := newTestServer(t)
	registry, _ := mcpRegistryFor(t, srv)

	tools := registry.ListTools()
	user := toolByName(t, tools, "user")
	assert.Equal(t, "Fetch one user by ID.", user.Description)

	// The tool for a mutation takes its input object; the DateTime custom
	// scalar is only reachable through a nested field, so look at the schema's
	// description of it.
	_, schema := discoverFrom(t, sdlFixturePath)
	dateTime := schema.Type("DateTime")
	require.NotNil(t, dateTime)
	assert.Equal(t, graphql.KindScalar, dateTime.Kind)
	assert.False(t, dateTime.IsBuiltin, "DateTime is a custom scalar, not a built-in")

	builtin := schema.Type("Int")
	require.NotNil(t, builtin)
	assert.True(t, builtin.IsBuiltin)
}

func TestMCP_CommandsCarryFlags(t *testing.T) {
	srv := newTestServer(t)
	registry, _ := mcpRegistryFor(t, srv)

	commands := registry.ListCommands()
	require.Len(t, commands, 9)

	var user handlers.Command
	for _, cmd := range commands {
		if cmd.Name == "user" {
			user = cmd
		}
	}
	require.Equal(t, "user", user.Name)
	require.Len(t, user.Flags, 1)
	assert.Equal(t, "id", user.Flags[0].Name)
	assert.True(t, user.Flags[0].Required)
	assert.Equal(t, "string", user.Flags[0].Type)

	require.NotNil(t, registry.GetCommand("ping"))
	assert.Nil(t, registry.GetCommand("nope"))
}

func TestMCP_ExecuteCommandUsesTheAdapter(t *testing.T) {
	srv := newTestServer(t)
	registry, _ := mcpRegistryFor(t, srv)

	result, err := registry.ExecuteCommand(context.Background(), "user", map[string]interface{}{"id": "u1"})
	require.NoError(t, err)

	echo := result.(map[string]interface{})["echo"].(map[string]interface{})
	assert.Equal(t, "UserQuery", echo["operationName"])
	assert.Equal(t, map[string]interface{}{"id": "u1"}, echo["variables"])
	assert.Len(t, srv.requests(), 1, "one GraphQL POST per tool call")
}

func TestMCP_ReadOnlyRegistry(t *testing.T) {
	srv := newTestServer(t)
	registry, _ := mcpRegistryFor(t, srv)

	assert.Error(t, registry.AddCommand(handlers.Command{Name: "x"}))
	assert.Error(t, registry.RemoveCommand("user"))
	assert.Error(t, registry.UpdateCommand("user", handlers.Command{Name: "user"}))
}

func TestMCP_Construction_ValidatesInputs(t *testing.T) {
	srv := newTestServer(t)
	ops, schema := discoverFrom(t, sdlFixturePath)

	_, err := graphql.NewGraphQLRegistry(nil, schema, newClient(srv.URL))
	require.Error(t, err)

	_, err = graphql.NewGraphQLRegistry(ops, schema, nil)
	require.Error(t, err)

	registry, err := graphql.NewGraphQLRegistry(ops, nil, newClient(srv.URL))
	require.NoError(t, err, "a registry without the schema still describes the neutral model")
	tools := registry.ListTools()
	require.Len(t, tools, len(ops))
	users := inputSchema(t, toolByName(t, tools, "users"))
	assert.Equal(t, "integer", properties(t, users)["limit"].(map[string]interface{})["type"],
		"the neutral fallback still carries the mapped type")
}

// registryRegistrar adapts the package-level handler registry to the registrar
// surface the adapter registers against. In production that surface is
// *mcp.Server (pinned below); the registry keeps the test able to invoke the
// registered handlers directly.
type registryRegistrar struct{ registry *handlers.Registry }

// RegisterHandler implements graphql.ServerHandlerRegistrar.
func (r registryRegistrar) RegisterHandler(h handlers.Handler) { r.registry.Register(h) }

func TestMCP_RegisterWithServer_ServesListAndCall(t *testing.T) {
	srv := newTestServer(t)
	_, schema := discoverFrom(t, sdlFixturePath)
	client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))
	ops, err := client.Discover(context.Background(), sdlFixturePath)
	require.NoError(t, err)

	registry := handlers.NewRegistry()
	require.NoError(t, graphql.RegisterWithServer(registryRegistrar{registry: registry}, ops, schema, client))

	// tools/list
	listed, err := registry.Handle(context.Background(), "tools/list", json.RawMessage(`{}`))
	require.NoError(t, err)
	listResult, ok := listed.(*handlers.ListToolsResult)
	require.True(t, ok, "got %T", listed)
	require.Len(t, listResult.Tools, 9)
	assert.NotEmpty(t, listResult.Tools[0].InputSchema)

	// tools/call
	called, err := registry.Handle(context.Background(), "tools/call",
		json.RawMessage(`{"name":"user","arguments":{"id":"u1"}}`))
	require.NoError(t, err)
	callResult, ok := called.(*handlers.CallToolResult)
	require.True(t, ok, "got %T", called)
	require.False(t, callResult.IsError)
	require.Len(t, callResult.Content, 1)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(callResult.Content[0].Text), &payload))
	echo := payload["echo"].(map[string]interface{})
	assert.Equal(t, "UserQuery", echo["operationName"])

	// A failing call is reported as a tool error, never as a hung tool.
	called, err = registry.Handle(context.Background(), "tools/call",
		json.RawMessage(`{"name":"user","arguments":{}}`))
	require.NoError(t, err)
	callResult, ok = called.(*handlers.CallToolResult)
	require.True(t, ok)
	assert.True(t, callResult.IsError)
	assert.Contains(t, callResult.Content[0].Text, `missing required argument "id"`)
}

// toolNames returns the tools' names in order.
func toolNames(tools []handlers.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

// compile-time pin: the registry's executor surface matches protocol.Executor.
var _ protocol.Executor = (*graphql.Client)(nil)

// compile-time pin: the production MCP server satisfies the registrar surface
// RegisterWithServer takes.
var _ graphql.ServerHandlerRegistrar = (*mcp.Server)(nil)

func TestMCP_SelfReferentialInputObject_IsDepthCapped(t *testing.T) {
	const sdl = `
type Query { node(filter: Filter): String }
input Filter { name: String, child: Filter }
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)
	ops, err := schema.Operations("")
	require.NoError(t, err)

	registry, err := graphql.NewGraphQLRegistry(ops, schema, &stubExecutor{})
	require.NoError(t, err)

	tools := registry.ListTools()
	require.Len(t, tools, 1)

	// Walking the nested "child" properties must terminate and say so.
	schemaJSON, err := json.Marshal(tools[0].InputSchema)
	require.NoError(t, err)
	assert.Contains(t, string(schemaJSON), "nested below depth")

	nodes := 0
	property, ok := properties(t, inputSchema(t, tools[0]))["filter"].(map[string]interface{})
	require.True(t, ok)
	for {
		nodes++
		require.Less(t, nodes, 10, "the input schema recursion must be bounded")

		props, ok := property["properties"].(map[string]interface{})
		if !ok {
			break
		}
		child, ok := props["child"].(map[string]interface{})
		if !ok {
			break
		}
		property = child
	}
	assert.Equal(t, 3, nodes, "the cap stops before recursing forever")
	assert.Contains(t, property["description"], "nested below depth",
		"the capped level says why it has no properties")
}

// stubExecutor satisfies protocol.Executor for registry-only tests.
type stubExecutor struct{}

// Execute implements protocol.Executor.
func (*stubExecutor) Execute(_ context.Context, opID string, _ map[string]interface{}) (interface{}, error) {
	return map[string]interface{}{"op": opID}, nil
}
