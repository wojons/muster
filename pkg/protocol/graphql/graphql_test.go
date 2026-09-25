// Package graphql_test is the PROTO-005 acceptance battery: schema ingestion
// from BOTH an SDL document and an introspection response, operation/parameter
// mapping, the single-endpoint request builder, an invoke round-trip against an
// in-process GraphQL server, GraphQL error → AppError mapping, the
// introspection-disabled path, and the MCP tool surface.
//
// Every test is network-free (loopback httptest servers only, or checked-in
// fixtures) per specs/PROTOCOLS.md §4.6.
package graphql_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/protocol/graphql"
)

const (
	sdlFixturePath           = "testdata/schema.graphql"
	introspectionFixturePath = "testdata/introspection.json"
	goldenQueryPath          = "testdata/golden_user_query.graphql"
	goldenBodyPath           = "testdata/golden_user_body.json"
)

// ── in-process GraphQL test server ───────────────────────────────────────

// recordedCall is one request the test server observed.
type recordedCall struct {
	Method      string
	Path        string
	ContentType string
	Body        []byte
	Headers     http.Header
}

// testServer is a loopback GraphQL server. It records every request and answers
// two families of request: an introspection request (the body mentions __schema)
// and a data request.
//
// Data requests get a universal echo payload — the server reports the operation
// name, the verbatim query document and the variables object it received — so a
// test can prove what actually crossed the wire without the server having to
// implement a schema.
type testServer struct {
	*httptest.Server

	mu        sync.Mutex
	calls     []recordedCall
	responses []string

	// behaviour switches
	introspectDisabled bool
	errorOnly          bool
	errorCode          string
	errorMessage       string
	partial            bool
	slow               time.Duration
	statusCode         int
	errorList          []string

	// introspection body to serve (defaults to the checked-in fixture)
	introspectionBody string
}

// serverOption mutates the test server before it starts serving.
type serverOption func(*testServer)

// withIntrospectionDisabled makes introspection requests fail the way a
// production-hardened server does.
func withIntrospectionDisabled() serverOption {
	return func(s *testServer) { s.introspectDisabled = true }
}

// withErrorOnly makes data requests answer with an errors-only response.
func withErrorOnly(code, message string) serverOption {
	return func(s *testServer) {
		s.errorOnly = true
		s.errorCode = code
		s.errorMessage = message
	}
}

// withPartial makes data requests answer with data AND errors (GraphQL partial
// success).
func withPartial() serverOption {
	return func(s *testServer) { s.partial = true }
}

// withSlow makes data requests take d before answering.
func withSlow(d time.Duration) serverOption {
	return func(s *testServer) { s.slow = d }
}

// withStatus makes every data request answer with an HTTP status (no GraphQL
// body), for transport-level mapping tests.
func withStatus(code int) serverOption {
	return func(s *testServer) { s.statusCode = code }
}

// withErrorList makes data requests answer with one error object per message,
// for the multi-error mapping path.
func withErrorList(messages ...string) serverOption {
	return func(s *testServer) { s.errorList = messages }
}

// withIntrospectionBody overrides the introspection response body.
func withIntrospectionBody(body string) serverOption {
	return func(s *testServer) { s.introspectionBody = body }
}

// newTestServer starts the loopback GraphQL server.
func newTestServer(t *testing.T, opts ...serverOption) *testServer {
	t.Helper()

	fixture, err := os.ReadFile(introspectionFixturePath)
	require.NoError(t, err, "introspection fixture must exist")

	s := &testServer{introspectionBody: string(fixture)}
	for _, opt := range opts {
		opt(s)
	}

	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		s.mu.Lock()
		s.calls = append(s.calls, recordedCall{
			Method:      r.Method,
			Path:        r.URL.Path,
			ContentType: r.Header.Get("Content-Type"),
			Body:        body,
			Headers:     r.Header.Clone(),
		})
		introspectDisabled := s.introspectDisabled
		errorOnly := s.errorOnly
		errorCode := s.errorCode
		errorMessage := s.errorMessage
		partial := s.partial
		slow := s.slow
		statusCode := s.statusCode
		errorList := s.errorList
		introspectionBody := s.introspectionBody
		s.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		var req struct {
			Query         string                 `json:"query"`
			Variables     map[string]interface{} `json:"variables"`
			OperationName string                 `json:"operationName"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":[{"message":"request body is not JSON"}]}`))
			return
		}

		if strings.Contains(req.Query, "__schema") {
			if introspectDisabled {
				_, _ = w.Write([]byte(`{"errors":[{"message":"GraphQL introspection is not allowed by Apollo Server, but the query contained __schema","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`))
				return
			}
			_, _ = w.Write([]byte(introspectionBody))
			return
		}

		if slow > 0 {
			time.Sleep(slow)
		}
		if statusCode != 0 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(statusCode)
			_, _ = w.Write([]byte(`{"errors":[{"message":"transport-level failure"}]}`))
			return
		}
		if errorOnly {
			resp := map[string]interface{}{
				"errors": []map[string]interface{}{{
					"message":    errorMessage,
					"path":       []interface{}{"user"},
					"extensions": map[string]interface{}{"code": errorCode},
				}},
			}
			out, _ := json.Marshal(resp)
			_, _ = w.Write(out)
			return
		}
		if len(errorList) > 0 {
			errs := make([]map[string]interface{}, 0, len(errorList))
			for _, message := range errorList {
				errs = append(errs, map[string]interface{}{
					"message":    message,
					"extensions": map[string]interface{}{"code": "INTERNAL_SERVER_ERROR"},
				})
			}
			out, _ := json.Marshal(map[string]interface{}{"errors": errs})
			_, _ = w.Write(out)
			return
		}

		echo := map[string]interface{}{
			"operationName": req.OperationName,
			"query":         req.Query,
			"variables":     req.Variables,
		}
		resp := map[string]interface{}{"data": map[string]interface{}{"echo": echo}}
		if partial {
			resp["errors"] = []map[string]interface{}{{
				"message":    "profile is unavailable",
				"path":       []interface{}{"user", "profile"},
				"extensions": map[string]interface{}{"code": "INTERNAL_SERVER_ERROR"},
			}}
		}
		out, _ := json.Marshal(resp)
		_, _ = w.Write(out)
	}))
	t.Cleanup(s.Close)
	return s
}

// calls returns a copy of the recorded request log.
func (s *testServer) requests() []recordedCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]recordedCall, len(s.calls))
	copy(out, s.calls)
	return out
}

// introspectionTarget returns the discovery URI for this server.
func (s *testServer) introspectionTarget() string {
	return graphql.IntrospectionScheme + strings.TrimPrefix(s.URL, "http://") + "/graphql"
}

// ── helpers ──────────────────────────────────────────────────────────────

// newClient builds an adapter client bound to endpoint with the default HTTP
// client and a short timeout.
func newClient(endpoint string, opts ...graphql.Option) *graphql.Client {
	base := []graphql.Option{graphql.WithTimeout(5 * time.Second)}
	return graphql.NewClient(endpoint, append(base, opts...)...)
}

// discoverFrom ingests a schema source and returns the operations.
func discoverFrom(t *testing.T, uri string) ([]*protocol.Operation, *graphql.Schema) {
	t.Helper()
	client := newClient("")
	ops, err := client.Discover(context.Background(), uri)
	require.NoError(t, err)
	require.NotEmpty(t, ops, "ingestion must produce operations")
	return ops, client.Schema()
}

// paramByName returns the named parameter, failing when it is absent.
func paramByName(t *testing.T, op *protocol.Operation, name string) protocol.Parameter {
	t.Helper()
	for _, p := range op.Parameters {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("operation %s has no parameter %q (has %v)", op.ID, name, paramNames(op))
	return protocol.Parameter{}
}

// paramNames lists an operation's parameter names.
func paramNames(op *protocol.Operation) []string {
	names := make([]string, 0, len(op.Parameters))
	for _, p := range op.Parameters {
		names = append(names, p.Name)
	}
	return names
}

// requireGolden compares got with a checked-in golden file (trailing whitespace
// ignored).
func requireGolden(t *testing.T, path, got string) {
	t.Helper()
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, strings.TrimSpace(string(want)), strings.TrimSpace(got),
		"artifact must match the checked-in golden file %s", path)
}

// ── stage 1/2: ingestion ─────────────────────────────────────────────────

func TestIngest_SDL_ProducesTypedOperations(t *testing.T) {
	ops, schema := discoverFrom(t, sdlFixturePath)

	require.Equal(t, graphql.FormatSDL, schema.SourceFormat)
	require.Equal(t, sdlFixturePath, schema.Source)

	ids := graphql.OperationIDs(ops)
	assert.Equal(t, []string{"user", "users", "nodes", "search", "ping", "viewer", "createUser", "setRole", "noop"}, ids,
		"every Query and Mutation field becomes one command, in declaration order")

	for _, op := range ops {
		assert.Equal(t, protocol.ProtocolGraphQL, op.Protocol)
		assert.Equal(t, protocol.KindUnary, op.Kind)
		assert.Contains(t, []string{"query", "mutation"}, op.Source[graphql.SourceOperationType])
		assert.Equal(t, sdlFixturePath, op.Source[graphql.SourceSchemaSource])
		assert.Equal(t, graphql.FormatSDL, op.Source[graphql.SourceSchemaFormat])
		assert.NotEmpty(t, op.Source[graphql.SourceFieldType], "every operation records its return type")
	}

	// Subscriptions are out of scope: recognized, not ingested.
	assert.NotContains(t, ids, "userChanged")
	assert.Equal(t, 1, schema.Counts().Subscriptions)

	// A field's arguments become flags with the neutral type vocabulary.
	user := graphql.OperationByID(ops, "user")
	require.NotNil(t, user)
	id := paramByName(t, user, "id")
	assert.Equal(t, graphql.ParameterLocationVariable, id.In)
	assert.Equal(t, "string", id.Type, "ID! maps to the neutral string type")
	assert.True(t, id.Required)
	assert.Contains(t, id.Description, "ID!", "the GraphQL type stays visible in the description")
	assert.Equal(t, "Fetch one user by ID.", user.Summary)

	// Non-null list of non-null objects, an enum argument, an Int default.
	users := graphql.OperationByID(ops, "users")
	require.NotNil(t, users)
	assert.Equal(t, "[User!]!", users.Source[graphql.SourceFieldType])
	limit := paramByName(t, users, "limit")
	assert.Equal(t, "integer", limit.Type)
	assert.False(t, limit.Required, "an argument with a default is not required")
	assert.EqualValues(t, 20, limit.Default)
	role := paramByName(t, users, "role")
	assert.Equal(t, "string", role.Type)
	assert.False(t, role.Required)
	assert.Equal(t, []interface{}{"ADMIN", "MEMBER", "GUEST"}, role.Enum)

	// Input objects map to the neutral object type and keep their GraphQL type.
	createUser := graphql.OperationByID(ops, "createUser")
	require.NotNil(t, createUser)
	input := paramByName(t, createUser, "input")
	assert.Equal(t, "object", input.Type)
	assert.True(t, input.Required)
	assert.Equal(t, "input:CreateUserInput!", createUser.Source[graphql.SourceArgTypes])

	// A nullable field with no arguments still becomes a command.
	noop := graphql.OperationByID(ops, "noop")
	require.NotNil(t, noop)
	assert.Empty(t, noop.Parameters)
	assert.Equal(t, "String", noop.Source[graphql.SourceFieldType])
}

func TestIngest_SDL_InputFieldDefaultsAreIngested(t *testing.T) {
	_, schema := discoverFrom(t, sdlFixturePath)

	input := schema.Type("CreateUserInput")
	require.NotNil(t, input)
	require.Equal(t, graphql.KindInputObject, input.Kind)

	role := input.InputField("role")
	require.NotNil(t, role)
	require.True(t, role.HasDefault)
	assert.Equal(t, "MEMBER", role.DefaultValue, "enum defaults keep their unquoted spelling")
	assert.Equal(t, "MEMBER", fmt.Sprint(role.Default))

	tags := input.InputField("tags")
	require.NotNil(t, tags)
	require.True(t, tags.HasDefault)
	assert.Equal(t, "[]", tags.DefaultValue)

	nested := input.InputField("profile")
	require.NotNil(t, nested)
	assert.Equal(t, "ProfileInput", nested.Type.NamedType())

	website := schema.Type("ProfileInput").InputField("website")
	require.NotNil(t, website)
	require.True(t, website.HasDefault)
	assert.Equal(t, `""`, website.DefaultValue)

	// A non-null input field without a default is required.
	assert.True(t, input.InputField("name").Required())
	assert.False(t, input.InputField("email").Required())
}

func TestIngest_IntrospectionJSON_ProducesTypedOperations(t *testing.T) {
	ops, schema := discoverFrom(t, introspectionFixturePath)

	require.Equal(t, graphql.FormatIntrospectionJSON, schema.SourceFormat)
	require.Equal(t, introspectionFixturePath, schema.Source)
	require.Equal(t, "Query", schema.QueryType)
	require.Equal(t, "Mutation", schema.MutationType)
	require.Equal(t, "Subscription", schema.SubscriptionType)

	user := graphql.OperationByID(ops, "user")
	require.NotNil(t, user)
	assert.Equal(t, "id:ID!", user.Source[graphql.SourceArgTypes])

	users := graphql.OperationByID(ops, "users")
	require.NotNil(t, users)
	assert.Equal(t, "limit:Int,role:Role", users.Source[graphql.SourceArgTypes])
	assert.Equal(t, []interface{}{"ADMIN", "MEMBER", "GUEST"}, paramByName(t, users, "role").Enum)

	createUser := graphql.OperationByID(ops, "createUser")
	require.NotNil(t, createUser)
	assert.Equal(t, "mutation", createUser.Source[graphql.SourceOperationType])
	assert.Equal(t, "User!", createUser.Source[graphql.SourceFieldType],
		"the return type keeps its non-null wrapper")
}

// opSnapshot is the comparable view of an ingested operation: everything stage 3
// consumes, with the ingestion-provenance keys (which necessarily differ between
// the two paths) removed.
type opSnapshot struct {
	ID          string
	Kind        protocol.OperationKind
	Tags        []string
	Summary     string
	Description string
	Parameters  []paramSnapshot
	Field       string
	FieldType   string
	OpType      string
	ArgTypes    string
}

// paramSnapshot is the comparable view of one parameter. Default is compared
// through its JSON encoding so the two ingestion paths cannot disagree about a
// value's spelling.
type paramSnapshot struct {
	Name        string
	In          string
	Type        string
	Required    bool
	Enum        []interface{}
	Default     string
	Description string
}

// snapshot normalizes an operation list for comparison.
func snapshot(t *testing.T, ops []*protocol.Operation) []opSnapshot {
	t.Helper()
	out := make([]opSnapshot, 0, len(ops))
	for _, op := range ops {
		snap := opSnapshot{
			ID:          op.ID,
			Kind:        op.Kind,
			Tags:        op.Tags,
			Summary:     op.Summary,
			Description: op.Description,
			Field:       op.Source[graphql.SourceField],
			FieldType:   op.Source[graphql.SourceFieldType],
			OpType:      op.Source[graphql.SourceOperationType],
			ArgTypes:    op.Source[graphql.SourceArgTypes],
		}
		for _, p := range op.Parameters {
			encoded := ""
			if p.Default != nil {
				raw, err := json.Marshal(p.Default)
				require.NoError(t, err)
				encoded = string(raw)
			}
			snap.Parameters = append(snap.Parameters, paramSnapshot{
				Name:        p.Name,
				In:          p.In,
				Type:        p.Type,
				Required:    p.Required,
				Enum:        p.Enum,
				Default:     encoded,
				Description: p.Description,
			})
		}
		out = append(out, snap)
	}
	return out
}

func TestIngest_SDLAndIntrospection_AgreeOnEveryOperation(t *testing.T) {
	fromSDL, sdlSchema := discoverFrom(t, sdlFixturePath)
	fromIntrospection, introspectionSchema := discoverFrom(t, introspectionFixturePath)

	assert.Equal(t, snapshot(t, fromSDL), snapshot(t, fromIntrospection),
		"the SDL path and the introspection path must map the same schema onto identical operations")

	// The provenance stamps are the one intentional difference.
	assert.Equal(t, graphql.FormatSDL, sdlSchema.SourceFormat)
	assert.Equal(t, graphql.FormatIntrospectionJSON, introspectionSchema.SourceFormat)
	for i := range fromSDL {
		assert.Equal(t, sdlFixturePath, fromSDL[i].Source[graphql.SourceSchemaSource])
		assert.Equal(t, introspectionFixturePath, fromIntrospection[i].Source[graphql.SourceSchemaSource])
	}
}

func TestIngest_SDL_UnionAndInterfaceAreRecorded(t *testing.T) {
	_, schema := discoverFrom(t, sdlFixturePath)

	union := schema.Type("SearchResult")
	require.NotNil(t, union)
	assert.Equal(t, graphql.KindUnion, union.Kind)
	assert.Equal(t, []string{"User", "Profile"}, union.PossibleTypes)
	assert.Empty(t, union.Fields, "a union has no fields of its own (inline fragments are out of scope)")

	iface := schema.Type("Node")
	require.NotNil(t, iface)
	assert.Equal(t, graphql.KindInterface, iface.Kind)
	assert.Equal(t, []string{"User"}, iface.PossibleTypes, "implementers are derived from `implements` clauses")
	require.NotNil(t, iface.Field("id"))

	user := schema.Type("User")
	require.NotNil(t, user)
	assert.Equal(t, []string{"Node"}, user.Interfaces)
}

func TestIngest_MutationNameCollision_GetsPrefixedID(t *testing.T) {
	const sdl = `
type Query { ping: String }
type Mutation { ping: String, other: String }
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)

	ops, err := schema.Operations("")
	require.NoError(t, err)
	assert.Equal(t, []string{"ping", "mutation_ping", "other"}, graphql.OperationIDs(ops),
		"a mutation whose field name is already a query command keeps a unique, deterministic ID")
}

func TestIngest_SDLParseErrors_AreValidationErrors(t *testing.T) {
	cases := map[string]string{
		"unterminated braces": "type Query { ping: String",
		"unknown definition":  "wibble Query { ping: String }",
		"missing type":        "type Query { ping String }",
	}
	for name, sdl := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := graphql.ParseSDL(sdl)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "line 1", "a parse error names where it happened")
		})
	}
}

func TestIngest_EmptySchemaSource_FailsLoud(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.graphql")
	require.NoError(t, os.WriteFile(empty, []byte("\n  \n"), 0o600))

	client := newClient("")
	_, err := client.Discover(context.Background(), empty)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestIngest_SubscriptionOnlySchema_FailsWithActionableError(t *testing.T) {
	const sdl = `
type Query { nothing: String }
type Subscription { ticks: String }
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)
	// Drop the query root so the schema exposes subscriptions only.
	schema.QueryType = ""
	ops, err := schema.Operations("")
	require.NoError(t, err)
	require.Empty(t, ops)
	assert.Equal(t, 1, schema.Counts().Subscriptions)
}

func TestIngest_MissingSource_FailsWithActionableError(t *testing.T) {
	client := newClient("")
	_, err := client.Discover(context.Background(), filepath.Join(t.TempDir(), "absent.graphql"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found on disk")
	assert.Contains(t, err.Error(), graphql.IntrospectionScheme, "the error names the introspection form")
}

func TestIngest_IntrospectionResponseShapes(t *testing.T) {
	// A recorded response may be the GraphQL envelope (the fixture), a bare
	// {"__schema": ...} object, or the raw schema object. All three ingest.
	fixture, err := os.ReadFile(introspectionFixturePath)
	require.NoError(t, err)

	var envelope struct {
		Data struct {
			Schema json.RawMessage `json:"__schema"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(fixture, &envelope))
	require.NotEmpty(t, envelope.Data.Schema)

	shapes := map[string]string{
		"envelope":    string(fixture),
		"bare schema": `{"__schema":` + string(envelope.Data.Schema) + `}`,
		"raw object":  string(envelope.Data.Schema),
	}
	for name, body := range shapes {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schema.json")
			require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

			ops, schema := discoverFrom(t, path)
			assert.Len(t, ops, 9)
			assert.Equal(t, "Query", schema.QueryType)
		})
	}
}

func TestIngest_IntrospectionResponseWithoutKinds_StillIngestsNames(t *testing.T) {
	// Some hand-recorded documents omit the "kind" of a named type reference.
	// The named type is still resolvable; the schema's own table is authority.
	body := `{"data":{"__schema":{
	  "queryType":{"name":"Query"},
	  "types":[
	    {"kind":"OBJECT","name":"Query","fields":[
	      {"name":"ping","args":[
	        {"name":"echo","type":{"name":"String","ofType":null},"defaultValue":null}
	      ],"type":{"name":"String","ofType":null}}
	    ]}
	  ]}}}`
	path := filepath.Join(t.TempDir(), "kinds.json")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	ops, _ := discoverFrom(t, path)
	require.Len(t, ops, 1)
	assert.Equal(t, "String", ops[0].Source[graphql.SourceFieldType])
	require.Len(t, ops[0].Parameters, 1)
	assert.Equal(t, "string", ops[0].Parameters[0].Type)
}

func TestIngest_RejectsNonGraphQLJSONAndUnterminatedLists(t *testing.T) {
	dir := t.TempDir()

	notSchema := filepath.Join(dir, "array.json")
	require.NoError(t, os.WriteFile(notSchema, []byte(`["not","a","schema"]`), 0o600))
	_, err := newClient("").Discover(context.Background(), notSchema)
	require.Error(t, err)

	badSDL := filepath.Join(dir, "bad.graphql")
	require.NoError(t, os.WriteFile(badSDL, []byte("type Query { a: [Int }\n"), 0o600))
	_, err = newClient("").Discover(context.Background(), badSDL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 1", "a parse error names its position")
}

func TestIngest_SDL_UndeclaredTypeFallsBackToTheScalarVocabulary(t *testing.T) {
	const sdl = `
type Query {
  a(mystery: MissingThing, many: [MissingThing]): String
}
`
	schema, err := graphql.ParseSDL(sdl)
	require.NoError(t, err)
	ops, err := schema.Operations("")
	require.NoError(t, err)

	field := schema.OperationField(graphql.OperationByID(ops, "a"))
	require.NotNil(t, field)
	assert.Equal(t, "string", schema.NeutralType(field.Arg("mystery").Type))
	assert.Equal(t, "array", schema.NeutralType(field.Arg("many").Type))
}
