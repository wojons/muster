package graphql_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol/graphql"
)

// newAdapterFrom discovers a fixture and returns the wired client.
func newAdapterFrom(t *testing.T, uri string, opts ...graphql.Option) *graphql.Client {
	t.Helper()
	client := newClient("", opts...)
	_, err := client.Discover(context.Background(), uri)
	require.NoError(t, err)
	return client
}

// buildFor builds the request for one operation of a fixture schema.
func buildFor(t *testing.T, client *graphql.Client, opID string, args map[string]interface{}) *graphql.Request {
	t.Helper()
	op := graphql.OperationByID(client.Operations(), opID)
	require.NotNil(t, op, "operation %s must exist", opID)
	req, err := client.Builder().Build(op, args, "http://127.0.0.1:1/graphql")
	require.NoError(t, err)
	return req
}

// decodeBody decodes a request body into its wire members.
func decodeBody(t *testing.T, req *graphql.Request) struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
} {
	t.Helper()
	body, err := req.Body()
	require.NoError(t, err)

	var decoded struct {
		Query         string                 `json:"query"`
		Variables     map[string]interface{} `json:"variables"`
		OperationName string                 `json:"operationName"`
	}
	require.NoError(t, json.Unmarshal(body, &decoded), "the body must be a JSON object: %s", string(body))
	return decoded
}

// ── the request builder ──────────────────────────────────────────────────

func TestBuilder_QueryDocument_MatchesGoldenFile(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)
	req := buildFor(t, client, "user", map[string]interface{}{"id": "u1"})

	requireGolden(t, goldenQueryPath, req.Query)
	assert.Equal(t, "UserQuery", req.OperationName)
	assert.Equal(t, "http://127.0.0.1:1/graphql", req.Endpoint)
	assert.Equal(t, map[string]interface{}{"id": "u1"}, req.Variables)
}

func TestBuilder_Body_MatchesGoldenBytes(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)
	req := buildFor(t, client, "user", map[string]interface{}{"id": "u1"})

	body, err := req.Body()
	require.NoError(t, err)
	requireGolden(t, goldenBodyPath, string(body))

	// The body is the GraphQL-over-HTTP shape: query document, variables
	// object, operation name — one POST, no path/method builder.
	var members map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &members))
	require.Len(t, members, 3)
	assert.Contains(t, members, "query")
	assert.Contains(t, members, "variables")
	assert.Contains(t, members, "operationName")
}

func TestBuilder_NoArguments_OmitsVariableDeclaration(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)
	req := buildFor(t, client, "ping", nil)

	assert.Equal(t, "query PingQuery {\n  ping\n}", req.Query,
		"a field with no arguments carries no variable declaration block")
	assert.Equal(t, "PingQuery", req.OperationName)

	decoded := decodeBody(t, req)
	assert.Equal(t, map[string]interface{}{}, decoded.Variables,
		"the variables member is always an object (never null)")
}

func TestBuilder_Mutation_CarriesInputObject(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)
	req := buildFor(t, client, "createUser", map[string]interface{}{
		"input": map[string]interface{}{"name": "Ada", "role": "ADMIN"},
	})

	assert.Equal(t, "mutation CreateUserMutation($input: CreateUserInput!) {\n"+
		"  createUser(input: $input) {\n"+
		"    id\n    name\n    email\n    role\n    tags\n    profile {\n      bio\n      website\n      joinedAt\n    }\n"+
		"  }\n}", req.Query)

	decoded := decodeBody(t, req)
	assert.Equal(t, map[string]interface{}{"input": map[string]interface{}{"name": "Ada", "role": "ADMIN"}}, decoded.Variables)
}

func TestBuilder_OmittedOptionalArguments_AreNotMaterialized(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)

	// `users(limit: Int = 20, role: Role)` called with nothing: the document
	// declares no variables and the server applies `limit`'s default.
	req := buildFor(t, client, "users", nil)
	assert.Equal(t, "query UsersQuery {\n  users {\n    id\n    name\n    email\n    role\n    tags\n    profile {\n      bio\n      website\n      joinedAt\n    }\n  }\n}", req.Query)
	assert.Empty(t, req.Variables)

	// Nested input-field defaults are the server's too.
	req = buildFor(t, client, "createUser", map[string]interface{}{
		"input": map[string]interface{}{"name": "Ada"},
	})
	decoded := decodeBody(t, req)
	assert.Equal(t, map[string]interface{}{"input": map[string]interface{}{"name": "Ada"}}, decoded.Variables,
		"omitted input fields stay omitted so the server applies its declared defaults")
}

func TestBuilder_ValuesAreNeverInterpolatedIntoTheDocument(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)

	benign := buildFor(t, client, "user", map[string]interface{}{"id": "u1"})
	hostile := buildFor(t, client, "user", map[string]interface{}{
		"id": `u1") { id } } mutation Evil { noop } #`,
	})

	assert.Equal(t, benign.Query, hostile.Query,
		"an argument value must never change the document — values travel as variables")
	assert.Equal(t, `u1") { id } } mutation Evil { noop } #`, hostile.Variables["id"])

	body, err := hostile.Body()
	require.NoError(t, err)
	assert.NotContains(t, string(body), "@@", "sanity: the body is still the same 3-member envelope")
	var members map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &members))
	require.Len(t, members, 3)
}

func TestBuilder_ArgumentErrors_AreValidationErrors(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)
	op := graphql.OperationByID(client.Operations(), "user")
	require.NotNil(t, op)

	cases := []struct {
		name     string
		args     map[string]interface{}
		contains string
	}{
		{"missing required", nil, `missing required argument "id"`},
		{"unknown argument", map[string]interface{}{"id": "u1", "nope": 1}, `unknown argument(s) nope`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.Builder().Build(op, tc.args, "")
			require.Error(t, err)
			assert.True(t, errors.IsError(err, errors.CodeValidation), "want VALIDATION_ERROR, got %v", err)
			assert.Contains(t, err.Error(), tc.contains)
		})
	}
}

func TestBuilder_EnumArgument_IsValidatedAgainstTheSchema(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)
	op := graphql.OperationByID(client.Operations(), "users")
	require.NotNil(t, op)

	ok, err := client.Builder().Build(op, map[string]interface{}{"role": "ADMIN"}, "")
	require.NoError(t, err)
	assert.Equal(t, "ADMIN", ok.Variables["role"])

	_, err = client.Builder().Build(op, map[string]interface{}{"role": "SUPERUSER"}, "")
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation), "got %v", err)
	assert.Contains(t, err.Error(), "ADMIN, MEMBER, GUEST", "the error lists the schema's values")
}

func TestBuilder_InputObjectArgument_IsValidatedFieldByField(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)
	op := graphql.OperationByID(client.Operations(), "createUser")
	require.NotNil(t, op)

	_, err := client.Builder().Build(op, map[string]interface{}{"input": map[string]interface{}{"name": "Ada", "nickname": "a"}}, "")
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation))
	assert.Contains(t, err.Error(), `input has no field "nickname"`)
	assert.Contains(t, err.Error(), "name: String!", "the error lists the input object's fields")

	_, err = client.Builder().Build(op, map[string]interface{}{"input": map[string]interface{}{"email": "a@b.c"}}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input.name is required")

	// A nested input object is recursed into.
	req, err := client.Builder().Build(op, map[string]interface{}{
		"input": map[string]interface{}{"name": "Ada", "profile": map[string]interface{}{"bio": "hi", "nope": 1}},
	}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input.profile has no field \"nope\"")
	assert.Nil(t, req)

	// Non-object values are rejected with the path named.
	_, err = client.Builder().Build(op, map[string]interface{}{"input": "ada"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input must be a CreateUserInput object")
}

func TestBuilder_ScalarCoercion_FromCLISpellings(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)
	users := graphql.OperationByID(client.Operations(), "users")
	require.NotNil(t, users)

	req, err := client.Builder().Build(users, map[string]interface{}{"limit": "5"}, "")
	require.NoError(t, err)
	assert.EqualValues(t, 5, req.Variables["limit"], "a numeric string coerces to Int")

	_, err = client.Builder().Build(users, map[string]interface{}{"limit": "5.5"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expects an integer")

	// A list accepts a single value (GraphQL's own coercion rule).
	createUser := graphql.OperationByID(client.Operations(), "createUser")
	req, err = client.Builder().Build(createUser, map[string]interface{}{
		"input": map[string]interface{}{"name": "Ada", "tags": "solo"},
	}, "")
	require.NoError(t, err)
	input, ok := req.Variables["input"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, []interface{}{"solo"}, input["tags"])

	// A non-null argument refuses an explicit null.
	setRole := graphql.OperationByID(client.Operations(), "setRole")
	_, err = client.Builder().Build(setRole, map[string]interface{}{"id": "u1", "role": nil}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-null")
}

func TestBuilder_Selection_IsDerivedFromTheSchema(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)

	// A union-typed field selects __typename: inline fragments are out of scope.
	search := buildFor(t, client, "search", nil)
	assert.Equal(t, "query SearchQuery {\n  search {\n    __typename\n  }\n}", search.Query)

	// An interface-typed field selects the interface's own fields.
	nodes := buildFor(t, client, "nodes", nil)
	assert.Equal(t, "query NodesQuery {\n  nodes {\n    id\n  }\n}", nodes.Query)

	// A depth-1 selection stops at the leaf fields of the root object.
	shallow := newAdapterFrom(t, sdlFixturePath, graphql.WithSelection(graphql.SelectionOptions{MaxDepth: 1}))
	req := buildFor(t, shallow, "viewer", nil)
	assert.Equal(t, "query ViewerQuery {\n  viewer {\n    id\n    name\n  }\n}", req.Query)
}

func TestBuilder_SelectionAllowList_IsPerOperationAndStrict(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath, graphql.WithSelection(graphql.SelectionOptions{Fields: []string{"name", "id"}}))
	req := buildFor(t, client, "user", map[string]interface{}{"id": "u1"})

	assert.Equal(t, "query UserQuery($id: ID!) {\n  user(id: $id) {\n    name\n    id\n  }\n}", req.Query,
		"the allow-list is applied in the caller's order")

	unknown := newAdapterFrom(t, sdlFixturePath, graphql.WithSelection(graphql.SelectionOptions{Fields: []string{"nope"}}))
	op := graphql.OperationByID(unknown.Operations(), "user")
	_, err := unknown.Builder().Build(op, map[string]interface{}{"id": "u1"}, "")
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation))
	assert.Contains(t, err.Error(), "selectable: id, name, email, role, tags, profile")
}

func TestBuilder_MaxFields_CapsEachLevel(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath, graphql.WithSelection(graphql.SelectionOptions{MaxDepth: 2, MaxFields: 2}))
	req := buildFor(t, client, "user", map[string]interface{}{"id": "u1"})

	assert.Equal(t, "query UserQuery($id: ID!) {\n  user(id: $id) {\n    id\n    name\n  }\n}", req.Query)
}

func TestBuilder_FieldsWithRequiredArguments_AreNotSelected(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)

	// `user(id: ID!)` requires an argument, so no derived selection can invoke
	// it: `viewer { id name }` is what the schema allows without values.
	req := buildFor(t, client, "viewer", nil)
	assert.NotContains(t, req.Query, "user", "a field with a required argument cannot be selected argument-free")

	// The operation itself is still invocable — with its argument bound.
	assert.Contains(t, buildFor(t, client, "user", map[string]interface{}{"id": "u1"}).Query, "user(id: $id)")
}

func TestBuilder_EndpointFallback_FromSchemaSource(t *testing.T) {
	srv := newTestServer(t)
	client := newClient("")
	ops, err := client.Discover(context.Background(), srv.introspectionTarget())
	require.NoError(t, err)
	require.Equal(t, "http://"+strings.TrimPrefix(srv.URL, "http://")+"/graphql", client.Endpoint(),
		"a live introspection target becomes the client's default endpoint")

	req, err := client.Builder().Build(graphql.OperationByID(ops, "ping"), nil, "")
	require.NoError(t, err)
	assert.Equal(t, srv.URL+"/graphql", req.Endpoint)

	// An explicit endpoint always wins.
	req, err = client.Builder().Build(graphql.OperationByID(ops, "ping"), nil, "http://elsewhere.example/graphql")
	require.NoError(t, err)
	assert.Equal(t, "http://elsewhere.example/graphql", req.Endpoint)
}

func TestRequest_HTTPRequest_IsOnePostToTheEndpoint(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)
	req := buildFor(t, client, "user", map[string]interface{}{"id": "u1"})

	httpReq, err := req.HTTPRequest(context.Background())
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, httpReq.Method)
	assert.Equal(t, "http://127.0.0.1:1/graphql", httpReq.URL.String())
	assert.Equal(t, graphql.ContentTypeJSON, httpReq.Header.Get("Content-Type"))
	assert.Empty(t, httpReq.URL.Query(), "GraphQL arguments travel in the body, never in the URL")

	body, err := req.Body()
	require.NoError(t, err)

	raw, err := io.ReadAll(httpReq.Body)
	require.NoError(t, err)
	assert.Equal(t, string(body), string(raw), "the assembled HTTP request carries the rendered body verbatim")
}

func TestBuilder_ScalarCoercion_CoversTheBuiltinVocabulary(t *testing.T) {
	const sdl = `
type Query {
  probe(f: Float, b: Boolean, s: String, id: ID, when: Stamp, list: [Int!], nested: [[Int]]): String
}
scalar Stamp
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)
	ops, err := schema.Operations("")
	require.NoError(t, err)
	builder, err := graphql.NewBuilder(schema, graphql.SelectionOptions{})
	require.NoError(t, err)
	probe := graphql.OperationByID(ops, "probe")

	req, err := builder.Build(probe, map[string]interface{}{
		"f":      "1.5",
		"b":      "true",
		"s":      42,
		"id":     7,
		"when":   "2026-09-25T00:00:00Z",
		"list":   []interface{}{1, "2"},
		"nested": []interface{}{[]interface{}{1, 2}},
	}, "")
	require.NoError(t, err)

	assert.EqualValues(t, 1.5, req.Variables["f"])
	assert.Equal(t, true, req.Variables["b"])
	assert.Equal(t, "42", req.Variables["s"], "a number coerces to String as its decimal spelling")
	assert.Equal(t, "7", req.Variables["id"], "ID accepts an Int spelling")
	assert.Equal(t, "2026-09-25T00:00:00Z", req.Variables["when"], "a custom scalar passes through unchanged")
	assert.Equal(t, []interface{}{int64(1), int64(2)}, req.Variables["list"])
	assert.Equal(t, []interface{}{[]interface{}{int64(1), int64(2)}}, req.Variables["nested"])

	// Values that cannot coerce are refused, naming the path.
	_, err = builder.Build(probe, map[string]interface{}{"b": "maybe"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expects true or false")

	_, err = builder.Build(probe, map[string]interface{}{"f": "abc"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expects a number")

	_, err = builder.Build(probe, map[string]interface{}{"s": map[string]interface{}{}}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expects a String")

	// A false boolean is a value, not an omission.
	req, err = builder.Build(probe, map[string]interface{}{"b": false}, "")
	require.NoError(t, err)
	assert.Equal(t, false, req.Variables["b"])
	assert.Contains(t, req.Query, "probe(b: $b)")
}

func TestBuilder_DepthBoundary_UsesTypenameWhenNoLeafFieldExists(t *testing.T) {
	const sdl = `
type Query { root: Level1 }
type Level1 { inner: Level2 }
type Level2 { deeper: Level3 }
type Level3 { name: String }
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)
	ops, err := schema.Operations("")
	require.NoError(t, err)
	builder, err := graphql.NewBuilder(schema, graphql.SelectionOptions{MaxDepth: 2})
	require.NoError(t, err)

	req, err := builder.Build(graphql.OperationByID(ops, "root"), nil, "")
	require.NoError(t, err)

	// Level2 has no leaf field at all (its only field is another object), so the
	// depth boundary needs __typename to keep the document valid.
	assert.Equal(t, "query RootQuery {\n  root {\n    inner {\n      __typename\n    }\n  }\n}", req.Query)

	// Give Level2 a scalar and the boundary selects it instead.
	const withLeaf = `
type Query { root: Level1 }
type Level1 { inner: Level2 }
type Level2 { deeper: Level3, label: String }
type Level3 { name: String }
`
	leafy, err := graphql.ParseSDL(withLeaf)
	require.NoError(t, err)
	leafyOps, err := leafy.Operations("")
	require.NoError(t, err)
	leafyBuilder, err := graphql.NewBuilder(leafy, graphql.SelectionOptions{MaxDepth: 2})
	require.NoError(t, err)
	req, err = leafyBuilder.Build(graphql.OperationByID(leafyOps, "root"), nil, "")
	require.NoError(t, err)
	assert.Equal(t, "query RootQuery {\n  root {\n    inner {\n      label\n    }\n  }\n}", req.Query)
}

func TestBuilder_CyclicSchema_Terminates(t *testing.T) {
	const sdl = `
type Query { node: Node }
type Node { name: String, child: Node }
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)
	ops, err := schema.Operations("")
	require.NoError(t, err)
	builder, err := graphql.NewBuilder(schema, graphql.SelectionOptions{MaxDepth: 5})
	require.NoError(t, err)

	req, err := builder.Build(graphql.OperationByID(ops, "node"), nil, "")
	require.NoError(t, err)
	assert.Contains(t, req.Query, "child {")
	assert.NotContains(t, req.Query, "child {\n      child {", "the cycle is cut, not recursed into")
}

func TestBuilder_RequestBodyForMultipleArguments_KeepsCallerIndependentOrder(t *testing.T) {
	client := newAdapterFrom(t, sdlFixturePath)
	op := graphql.OperationByID(client.Operations(), "users")
	require.NotNil(t, op)

	// Variables are declared in the field's declared argument order, whatever
	// order the caller's map iterates in.
	req, err := client.Builder().Build(op, map[string]interface{}{"role": "MEMBER", "limit": 3}, "")
	require.NoError(t, err)
	assert.Contains(t, req.Query, "users(limit: $limit, role: $role)")

	body, err := req.Body()
	require.NoError(t, err)
	assert.Contains(t, string(body), `"variables":{"limit":3,"role":"MEMBER"}`,
		"the variables object carries both bindings")
}
