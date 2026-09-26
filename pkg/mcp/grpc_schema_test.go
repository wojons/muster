package mcp

import (
	"context"
	"encoding/json"
	"testing"

	grpcadapter "github.com/wojons/muster/pkg/protocol/grpc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file is PROTO-013's G-SCHEMA gate item 3 — the agent-facing regression
// carried forward from PROTO-011 §10.4: MCP input schemas are built from the
// neutral model, so a nested or enum-typed field used to reach an agent as a
// bare "object" with no properties, or a string with no enum array.
//
// The fixture is the real rich .proto, discovered through the real adapter: the
// point is that the two surfaces compose, not that a hand-built operation can be
// rendered.

// schemaFixturePath is the committed rich fixture (nested, repeated, map, enum,
// well-known type, oneof, optional + one service).
const schemaFixturePath = "../../pkg/protocol/grpc/testdata/schema.proto"

// richGRPCRegistry discovers the fixture and registers it, optionally with the
// schema set supplied.
func richGRPCRegistry(t *testing.T, withSchemaSet bool) (*GRPCRegistry, map[string]interface{}) {
	t.Helper()
	adapter := grpcadapter.NewAdapter()
	t.Cleanup(func() { _ = adapter.Close() })

	ops, err := adapter.Discover(context.Background(), schemaFixturePath)
	require.NoError(t, err, "discover fixture")
	require.Len(t, ops, 1)

	opts := []GRPCRegistryOption{}
	if withSchemaSet {
		set, err := adapter.Schema(context.Background(), schemaFixturePath)
		require.NoError(t, err, "schema fixture")
		opts = append(opts, WithSchemaSet(set))
	}
	registry, err := NewGRPCRegistry(ops, &fakeGRPCExecutor{}, opts...)
	require.NoError(t, err)

	tools := registry.ListTools()
	require.Len(t, tools, 1)
	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(tools[0].InputSchema, &schema), "input schema must be JSON: %s", tools[0].InputSchema)
	return registry, schema
}

// TestGRPCRegistry_InputSchema_NestedFieldIsNotABareObject asserts the nested
// field resolves to a real sub-schema rather than {"type":"object"} with no
// properties, and that the referenced type's own fields are present.
func TestGRPCRegistry_InputSchema_NestedFieldIsNotABareObject(t *testing.T) {
	_, schema := richGRPCRegistry(t, true)

	props := propertiesOf(t, schema)
	inner, ok := props["inner"].(map[string]interface{})
	require.True(t, ok, "field inner must be a schema object: %#v", props["inner"])

	if inner["type"] == "object" && inner["properties"] == nil {
		t.Fatalf("nested field rendered as a bare object with no properties: %#v", inner)
	}
	ref, _ := inner["$ref"].(string)
	require.Contains(t, ref, "#/$defs/schema.Inner", "nested field must reference the type: %#v", inner)

	defs, ok := schema["$defs"].(map[string]interface{})
	require.True(t, ok, "a resolved reference requires $defs: %#v", schema["$defs"])
	def, ok := defs["schema.Inner"].(map[string]interface{})
	require.True(t, ok, "$defs must hold schema.Inner: %#v", defs)
	defProps, ok := def["properties"].(map[string]interface{})
	require.True(t, ok, "the referenced type must carry its properties: %#v", def)
	assert.Equal(t, "string", defProps["a"].(map[string]interface{})["type"], "schema.Inner.a must be a string field")
}

// TestGRPCRegistry_InputSchema_EnumCarriesValues is the measured baseline flip:
// the enum field used to be {"type":"string"} with no enum array.
func TestGRPCRegistry_InputSchema_EnumCarriesValues(t *testing.T) {
	_, schema := richGRPCRegistry(t, true)

	color, ok := propertiesOf(t, schema)["color"].(map[string]interface{})
	require.True(t, ok, "field color must be a schema object")
	values, ok := color["enum"].([]interface{})
	require.True(t, ok, "an enum field must carry its values: %#v", color)
	require.Len(t, values, 2)
	assert.Equal(t, "COLOR_UNSPECIFIED", values[0])
	assert.Equal(t, "RED", values[1])
}

// TestGRPCRegistry_InputSchema_Depth1FactsReachTheAgent covers the rest of the
// depth-1 facts on the agent-facing surface: array items, map value type, the
// well-known type's JSON form, and the oneof group's mutual exclusion.
func TestGRPCRegistry_InputSchema_Depth1FactsReachTheAgent(t *testing.T) {
	_, schema := richGRPCRegistry(t, true)
	props := propertiesOf(t, schema)

	list, ok := props["list"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "array", list["type"])
	items, ok := list["items"].(map[string]interface{})
	require.True(t, ok, "a repeated field must carry its item schema: %#v", list)
	assert.Contains(t, items["$ref"], "#/$defs/schema.Inner")

	attrs, ok := props["attrs"].(map[string]interface{})
	require.True(t, ok)
	additional, ok := attrs["additionalProperties"].(map[string]interface{})
	require.True(t, ok, "a map field must carry its value schema: %#v", attrs)
	assert.Equal(t, "string", additional["type"])

	created, ok := props["created"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "string", created["type"], "a Timestamp field is a JSON string, not an object")
	assert.Equal(t, "date-time", created["format"])

	clauses, ok := schema["allOf"].([]interface{})
	require.True(t, ok, "two oneof members must be expressible as a constraint: %#v", schema["allOf"])
	assert.Len(t, clauses, 1, "one pair of members in the group yields one exclusion clause")
	clause := clauses[0].(map[string]interface{})
	not, ok := clause["not"].(map[string]interface{})
	require.True(t, ok)
	members, ok := not["required"].([]interface{})
	require.True(t, ok)
	assert.ElementsMatch(t, []interface{}{"x", "y"}, members)
}

// TestGRPCRegistry_InputSchema_WithoutSetKeepsPreviousRendering pins the
// back-compat half of the S1 decision: with NO schema set supplied, a nested
// field renders exactly as it did before PROTO-016 — a bare {"type":"object"} —
// so no existing caller's schemas change silently.
func TestGRPCRegistry_InputSchema_WithoutSetKeepsPreviousRendering(t *testing.T) {
	_, schema := richGRPCRegistry(t, false)

	inner, ok := propertiesOf(t, schema)["inner"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "object", inner["type"])
	assert.Nil(t, inner["properties"], "no set supplied means no resolved sub-schema")
	assert.Nil(t, schema["$defs"], "no set supplied means no $defs")
	// The self-contained depth-1 facts are still carried: they need no graph.
	assert.Equal(t, "date-time", propertiesOf(t, schema)["created"].(map[string]interface{})["format"])
}

// propertiesOf returns a JSON schema's top-level properties map.
func propertiesOf(t *testing.T, schema map[string]interface{}) map[string]interface{} {
	t.Helper()
	props, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok, "schema must carry properties: %#v", schema)
	return props
}
