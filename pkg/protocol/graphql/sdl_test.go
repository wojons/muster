package graphql_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/protocol/graphql"
)

// The SDL parser is exercised end to end through the fixture in
// graphql_test.go; these tests pin the parts of the type system that fixture
// does not reach.

func TestParseSDL_BlockStringDescriptions_AreDedented(t *testing.T) {
	const sdl = `"""
Root queries.

Second paragraph.
"""
type Query {
  """
  Fetch a thing.

  Multi-line.
  """
  thing: String @deprecated(reason: "use otherThing")
  otherThing: String
}
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)

	field := schema.Type("Query").Field("thing")
	require.NotNil(t, field)
	assert.Equal(t, "Fetch a thing.\n\nMulti-line.", field.Description)

	ops, err := schema.Operations("")
	require.NoError(t, err)
	require.Len(t, ops, 2)
	assert.Equal(t, "Fetch a thing.", ops[0].Summary, "the summary is the first sentence")
	assert.Equal(t, "Fetch a thing.\n\nMulti-line.", ops[0].Description)
}

func TestParseSDL_ExtendMergesIntoTheDeclaredType(t *testing.T) {
	const sdl = `
type Query { a: String }
extend type Query { b: String, a: String! }
input Filter { a: String }
extend input Filter { b: Int = 3 }
enum Role { ADMIN }
extend enum Role { GUEST }
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)

	query := schema.Type("Query")
	require.NotNil(t, query)
	assert.Equal(t, []string{"a", "b"}, []string{query.Fields[0].Name, query.Fields[1].Name},
		"an extension appends the fields it adds")
	assert.Equal(t, "String!", query.Field("a").Type.String(), "a re-declared field is replaced")

	filter := schema.Type("Filter")
	require.NotNil(t, filter)
	assert.Equal(t, "Int", filter.InputField("b").Type.String())
	require.True(t, filter.InputField("b").HasDefault)

	assert.Equal(t, []string{"ADMIN", "GUEST"}, schema.Type("Role").EnumValues)

	ops, err := schema.Operations("")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, graphql.OperationIDs(ops))
}

func TestParseSDL_DirectiveApplicationsAreParsedThenIgnored(t *testing.T) {
	const sdl = `
directive @tag(name: String = "x", weights: [Int] = [1, 2]) repeatable on FIELD_DEFINITION | OBJECT

type Query @tag(name: "q") {
  a: String @tag(name: "a", weights: [1, 2, 3])
  b: String @deprecated
  c(arg: Int = 1 @tag): String
}
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)

	ops, err := schema.Operations("")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, graphql.OperationIDs(ops))
	arg := graphql.OperationByID(ops, "c").Parameters[0]
	assert.Equal(t, "integer", arg.Type)
	assert.EqualValues(t, 1, arg.Default)
}

func TestParseSDL_SchemaBlockSelectsTheRoots(t *testing.T) {
	const sdl = `
schema { query: Root, mutation: RootMutation }
type Root { ping: String }
type Query { decoy: String }
type RootMutation { poke: String }
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)

	ops, err := schema.Operations("")
	require.NoError(t, err)
	assert.Equal(t, []string{"ping", "poke"}, graphql.OperationIDs(ops),
		"only the declared roots become operations, not the conventionally named decoys")
}

func TestParseSDL_CommentsAndCommasAreInsignificant(t *testing.T) {
	const sdl = `
# a comment
type Query { # trailing comment
  a: String,   # commas are whitespace
  b(limit: Int = 5,): String
}
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)
	assert.Equal(t, "5", schema.Type("Query").Field("b").Arg("limit").DefaultValue)
	assert.Len(t, schema.Type("Query").Fields, 2)
}

func TestParseSDL_SingleLineStringEscapes(t *testing.T) {
	sdl := "type Query {\n  \"quote: \\\" and newline: \\n end\"\n  a: String\n}\n"
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)
	assert.Equal(t, "quote: \" and newline: \n end", schema.Type("Query").Field("a").Description)
}

func TestParseSDL_ValueLiterals(t *testing.T) {
	const sdl = `
type Query {
  a(i: Int = -3, f: Float = 1.5, s: String = "x", b: Boolean = true, n: String = null, e: Color = RED, l: [Int] = [1, 2], o: Filter = { tag: "t", nested: { deep: 2 } }): String
}
enum Color { RED }
input Filter { tag: String, nested: Inner }
input Inner { deep: Int }
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)

	field := schema.Type("Query").Field("a")
	require.NotNil(t, field)

	assert.EqualValues(t, -3, field.Arg("i").Default)
	assert.EqualValues(t, 1.5, field.Arg("f").Default)
	assert.Equal(t, `"x"`, field.Arg("s").DefaultValue)
	assert.Equal(t, true, field.Arg("b").Default)
	assert.Equal(t, "null", field.Arg("n").DefaultValue, "an explicit null default is recorded as such")
	require.True(t, field.Arg("n").HasDefault)
	assert.Nil(t, field.Arg("n").Default)
	assert.Equal(t, "RED", field.Arg("e").DefaultValue, "enum defaults keep the bare spelling")
	assert.Equal(t, "{nested: {deep: 2}, tag: \"t\"}", field.Arg("o").DefaultValue,
		"object defaults are rendered with sorted keys so the spelling is deterministic")
	assert.Equal(t, "array", schema.NeutralType(field.Arg("l").Type), "a list maps to the neutral array type")
	require.True(t, field.Arg("l").Type.IsList())
}

func TestBuilder_SchemaThatNoLongerDescribesTheOperation(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)

	// An operation stamped with a field the schema does not have (a schema
	// changed under a stale operation list) fails as NOT_FOUND, not as a
	// silently empty document.
	ghost := &protocol.Operation{
		ID:       "ghost",
		Protocol: protocol.ProtocolGraphQL,
		Source: map[string]string{
			graphql.SourceField:         "ghost",
			graphql.SourceOperationType: "query",
		},
	}
	_, err := client.Builder().Build(ghost, nil, "")
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeNotFound), "got %v", err)
	assert.Contains(t, err.Error(), "Query.ghost")

	// A subscription-shaped operation is refused: subscriptions are out of scope.
	subscription := &protocol.Operation{
		ID:       "ticks",
		Protocol: protocol.ProtocolGraphQL,
		Source: map[string]string{
			graphql.SourceField:         "ticks",
			graphql.SourceOperationType: "subscription",
		},
	}
	_, err = client.Builder().Build(subscription, nil, "")
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation), "got %v", err)
	assert.Contains(t, err.Error(), `unsupported operation_type "subscription"`)
	assert.Contains(t, err.Error(), "only query and mutation are ingested")
}

func TestBuilder_SelectionAllowList_OnAUnionRootFailsLoud(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath, graphql.WithSelection(graphql.SelectionOptions{Fields: []string{"__typename"}}))
	op := graphql.OperationByID(client.Operations(), "search")
	require.NotNil(t, op)

	_, err := client.Builder().Build(op, nil, "")
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation))
	assert.Contains(t, err.Error(), "union")
}

func TestClient_DiscoverFromFileURL(t *testing.T) {
	client := newClient("")
	ops, err := client.Discover(context.Background(), "file://"+sdlFixturePath)
	require.NoError(t, err)
	assert.Len(t, ops, 9)
	assert.Equal(t, "file://"+sdlFixturePath, client.Schema().Source)
}
