package jsonrpc_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/protocol/jsonrpc"
)

// ── registry unit tests ───────────────────────────────────────────────────

// testOps discovers operations from a temp OpenRPC doc so the registry is
// exercised with exactly what production discovery yields.
func testOps(t *testing.T) []*protocol.Operation {
	t.Helper()
	doc := `{
	  "openrpc": "1.2.6",
	  "methods": [
	    {
	      "name": "add",
	      "summary": "Add two numbers",
	      "params": [
	        {"name": "a", "required": true, "schema": {"type": "integer"}},
	        {"name": "b", "required": true, "schema": {"type": "integer"}}
	      ]
	    },
	    {"name": "ping", "summary": "Ping"}
	  ]
	}`

	c := jsonrpc.NewClient("http://127.0.0.1:1")
	ops, err := c.Discover(context.Background(), writeTempDoc(t, doc))
	require.NoError(t, err)
	return ops
}

// registryExecutor is a scriptable executor for registry tests.
type registryExecutor struct {
	result interface{}
	err    error
	gotID  string
	gotArg map[string]interface{}
}

func (e *registryExecutor) Execute(ctx context.Context, opID string, args map[string]interface{}) (interface{}, error) {
	e.gotID = opID
	e.gotArg = args
	return e.result, e.err
}

func TestJSONRPCRegistry_ListTools_BuildsSchemas(t *testing.T) {
	reg, err := jsonrpc.NewJSONRPCRegistry(testOps(t), &registryExecutor{})
	require.NoError(t, err)

	tools := reg.ListTools()
	require.Len(t, tools, 2)

	byName := map[string]handlers.Tool{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}

	add := byName["add"]
	require.NotNil(t, add)
	assert.Equal(t, "Add two numbers", add.Description)

	var schema struct {
		Type       string                 `json:"type"`
		Properties map[string]interface{} `json:"properties"`
		Required   []string               `json:"required"`
	}
	require.NoError(t, json.Unmarshal(add.InputSchema, &schema))
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "a")
	assert.Contains(t, schema.Properties, "b")
	assert.Equal(t, []string{"a", "b"}, schema.Required)

	ping := byName["ping"]
	require.NotNil(t, ping)
	var pingSchema struct {
		Type       string                 `json:"type"`
		Properties map[string]interface{} `json:"properties"`
		Required   []string               `json:"required"`
	}
	require.NoError(t, json.Unmarshal(ping.InputSchema, &pingSchema))
	assert.Empty(t, pingSchema.Required)
}

func TestJSONRPCRegistry_CommandSurface(t *testing.T) {
	exec := &registryExecutor{result: map[string]interface{}{"sum": float64(3)}}
	reg, err := jsonrpc.NewJSONRPCRegistry(testOps(t), exec)
	require.NoError(t, err)

	cmds := reg.ListCommands()
	require.Len(t, cmds, 2)
	assert.Equal(t, "add", cmds[0].Name)
	require.Len(t, cmds[0].Flags, 2)
	assert.Equal(t, "a", cmds[0].Flags[0].Name)
	assert.True(t, cmds[0].Flags[0].Required)

	got := reg.GetCommand("add")
	require.NotNil(t, got)
	assert.Equal(t, "add", got.Name)
	assert.Nil(t, reg.GetCommand("missing"))

	// ExecuteCommand delegates to the executor (stage 4).
	res, err := reg.ExecuteCommand(context.Background(), "add", map[string]interface{}{"a": float64(1), "b": float64(2)})
	require.NoError(t, err)
	assert.Equal(t, "add", exec.gotID)
	assert.Equal(t, float64(3), res.(map[string]interface{})["sum"])

	// Read-only surface matches the OpenAPI registries' convention.
	require.Error(t, reg.AddCommand(handlers.Command{}))
	require.Error(t, reg.RemoveCommand("add"))
	require.Error(t, reg.UpdateCommand("add", handlers.Command{}))
}

func TestJSONRPCRegistry_RejectsNilExecutor(t *testing.T) {
	_, err := jsonrpc.NewJSONRPCRegistry(testOps(t), nil)
	require.Error(t, err)
}

func TestJSONRPCRegistry_RejectsEmptyOperations(t *testing.T) {
	_, err := jsonrpc.NewJSONRPCRegistry(nil, &registryExecutor{})
	require.Error(t, err)
}

// ── end-to-end MCP wiring: tools/list + tools/call over a real server ─────

func TestRegisterWithServer_ToolsListAndCall(t *testing.T) {
	srv := newRecordingServer(t, nil)
	c := jsonrpc.NewClient(srv.URL)

	doc := `{
	  "openrpc": "1.2.6",
	  "servers": [{"name": "test", "url": "` + srv.URL + `"}],
	  "methods": [
	    {"name": "echo", "params": [{"name": "msg", "schema": {"type": "string"}}]}
	  ]
	}`
	ops, err := c.Discover(context.Background(), writeTempDoc(t, doc))
	require.NoError(t, err)

	// Wire through the shared handler construction (same path the MCP
	// server uses), registered directly on a handlers.Registry.
	list, call, err := jsonrpc.MCPHandlers(ops, c)
	require.NoError(t, err)
	reg := handlers.NewRegistry()
	reg.Register(list)
	reg.Register(call)

	// tools/list over the MCP surface.
	listRes, err := reg.Handle(context.Background(), "tools/list", mustMarshalJSON(map[string]interface{}{}))
	require.NoError(t, err)
	tools, ok := listRes.(*handlers.ListToolsResult)
	require.True(t, ok)
	require.Len(t, tools.Tools, 1)
	assert.Equal(t, "echo", tools.Tools[0].Name)

	// tools/call: the MCP tool call executes the JSON-RPC method end-to-end
	// through the adapter executor and comes back as MCP content text.
	callRes, err := reg.Handle(context.Background(), "tools/call", mustMarshalJSON(map[string]interface{}{
		"name":      "echo",
		"arguments": map[string]interface{}{"msg": "through-mcp"},
	}))
	require.NoError(t, err)
	result, ok := callRes.(*handlers.CallToolResult)
	require.True(t, ok)
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Contains(t, result.Content[0].Text, "through-mcp")
}

func TestRegisterWithServer_ErrorSurfacesAsToolError(t *testing.T) {
	// Server whose methods all return "method not found" (-32601).
	srv := newRecordingServer(t, nil)
	c := jsonrpc.NewClient(srv.URL)

	ops, err := c.Discover(context.Background(), writeTempDoc(t, `{
	  "openrpc": "1.2.6",
	  "methods": [{"name": "nope"}]
	}`))
	require.NoError(t, err)

	list, call, err := jsonrpc.MCPHandlers(ops, c)
	require.NoError(t, err)
	reg := handlers.NewRegistry()
	reg.Register(list)
	reg.Register(call)

	// The discovered method does not exist server-side → JSON-RPC error →
	// AppError CodeNotFound → MCP tool result with isError=true and the
	// AppError code visible in the text (§4.2: error rendering keeps working
	// unchanged).
	callRes, err := reg.Handle(context.Background(), "tools/call", mustMarshalJSON(map[string]interface{}{
		"name":      "nope",
		"arguments": map[string]interface{}{},
	}))
	require.NoError(t, err, "the tool call itself succeeds; the error rides the result")
	result, ok := callRes.(*handlers.CallToolResult)
	require.True(t, ok)
	assert.True(t, result.IsError)
	require.Len(t, result.Content, 1)
	assert.Contains(t, result.Content[0].Text, "NOT_FOUND", "AppError code must survive into the MCP tool result")
}
