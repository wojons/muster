package graphql_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
	"github.com/wojons/muster/pkg/protocol/graphql"
)

// ── stage 1: live introspection, over loopback only ──────────────────────

func TestDiscover_LiveIntrospection_OverLoopback(t *testing.T) {
	srv := newTestServer(t)

	client := newClient("")
	ops, err := client.Discover(context.Background(), srv.introspectionTarget())
	require.NoError(t, err)
	require.NotEmpty(t, ops)

	require.NotNil(t, client.Schema())
	assert.Equal(t, graphql.FormatIntrospectionQuery, client.Schema().SourceFormat)
	assert.Equal(t, srv.introspectionTarget(), client.Schema().Source)
	for _, op := range ops {
		assert.Equal(t, graphql.FormatIntrospectionQuery, op.Source[graphql.SourceSchemaFormat])
		assert.Equal(t, srv.URL+"/graphql", op.Source[graphql.SourceEndpoint],
			"the introspection target is recorded as the operation's endpoint")
	}

	// Exactly one request crossed the wire, and it was the introspection query
	// posted as JSON GraphQL-over-HTTP.
	calls := srv.requests()
	require.Len(t, calls, 1)
	assert.Equal(t, http.MethodPost, calls[0].Method)
	assert.Equal(t, "/graphql", calls[0].Path)
	assert.Equal(t, graphql.ContentTypeJSON, calls[0].ContentType)

	var body struct {
		Query         string                 `json:"query"`
		Variables     map[string]interface{} `json:"variables"`
		OperationName string                 `json:"operationName"`
	}
	require.NoError(t, json.Unmarshal(calls[0].Body, &body))
	assert.Contains(t, body.Query, "__schema", "the standard introspection query is what gets posted")
	assert.Equal(t, "IntrospectionQuery", body.OperationName)
	assert.Equal(t, map[string]interface{}{}, body.Variables)
}

func TestDiscover_IntrospectionDisabled_NamesTheSDLFallback(t *testing.T) {
	srv := newTestServer(t, withIntrospectionDisabled())

	client := newClient("")
	_, err := client.Discover(context.Background(), srv.introspectionTarget())

	require.Error(t, err, "a server that refuses introspection must never yield an empty command set")
	assert.True(t, errors.IsError(err, errors.CodeBadRequest), "want BAD_REQUEST, got %v", err)
	assert.Contains(t, err.Error(), "introspection", "the error names what the server refused")
	assert.Contains(t, err.Error(), "SDL", "the error names the SDL fallback")
	assert.Contains(t, err.Error(), "--schema", "the error shows the next step")
}

func TestDiscover_IntrospectionDisabled_RecordedDocumentAlsoDetected(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/introspection-disabled.json"
	body := `{"errors":[{"message":"introspection is disabled","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	client := newClient("")
	_, err := client.Discover(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SDL")
}

func TestDiscover_SDLOverHTTP(t *testing.T) {
	sdl, err := os.ReadFile(sdlFixturePath)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method, "a document URL is fetched, not posted")
		_, _ = w.Write(sdl)
	}))
	t.Cleanup(srv.Close)

	ops, schema := discoverFrom(t, srv.URL+"/schema.graphql")
	assert.Len(t, ops, 9)
	assert.Equal(t, graphql.FormatSDL, schema.SourceFormat)
	assert.Equal(t, srv.URL+"/schema.graphql", schema.Source)
}

func TestDiscover_DocumentURLRejected_NamesTheIntrospectionForm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	client := newClient("")
	_, err := client.Discover(context.Background(), srv.URL+"/schema.graphql")
	require.Error(t, err)
	assert.Contains(t, err.Error(), graphql.IntrospectionScheme,
		"a failed document fetch tells the operator how to introspect the endpoint instead")
}

func TestDiscover_JSONWithoutSchema_FailsLoud(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/other.json"
	require.NoError(t, os.WriteFile(path, []byte(`{"hello":"world"}`), 0o600))

	client := newClient("")
	_, err := client.Discover(context.Background(), path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no __schema object")
}

// ── stage 4: execution ───────────────────────────────────────────────────

func TestExecute_QueryRoundTrip_OverLoopback(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))

	result, err := client.Execute(context.Background(), "user", map[string]interface{}{"id": "u1"})
	require.NoError(t, err)

	// The in-process server echoes what it received; the payload proves the
	// document, the variables object and the operation name all arrived intact.
	echo, ok := result.(map[string]interface{})["echo"].(map[string]interface{})
	require.True(t, ok, "unexpected payload shape %T", result)
	assert.Equal(t, "UserQuery", echo["operationName"])
	assert.Equal(t, map[string]interface{}{"id": "u1"}, echo["variables"])

	document, _ := echo["query"].(string)
	assert.Contains(t, document, "query UserQuery($id: ID!) {")
	assert.Contains(t, document, "user(id: $id) {")
	assert.NotContains(t, document, "u1", "the argument value travels as a variable, never as query text")

	// One POST, one endpoint: no discovery round-trip was needed (the schema
	// came from the SDL file).
	calls := srv.requests()
	require.Len(t, calls, 1)
	assert.Equal(t, http.MethodPost, calls[0].Method)
}

func TestExecute_MutationWithInputObjectAndEnum(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))

	result, err := client.Execute(context.Background(), "createUser", map[string]interface{}{
		"input": map[string]interface{}{
			"name":    "Ada",
			"role":    "ADMIN",
			"profile": map[string]interface{}{"bio": "first programmer"},
		},
	})
	require.NoError(t, err)

	echo := result.(map[string]interface{})["echo"].(map[string]interface{})
	assert.Equal(t, "CreateUserMutation", echo["operationName"])

	variables := echo["variables"].(map[string]interface{})
	input := variables["input"].(map[string]interface{})
	assert.Equal(t, "Ada", input["name"])
	assert.Equal(t, "ADMIN", input["role"], "the enum value travels as its declared name")
	assert.Equal(t, map[string]interface{}{"bio": "first programmer"}, input["profile"])

	document := echo["query"].(string)
	assert.Contains(t, document, "mutation CreateUserMutation($input: CreateUserInput!) {")
	assert.Contains(t, document, "createUser(input: $input) {")
}

func TestExecute_InputObjectDefaultAppliesServerSide(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))

	// `role` has a default (MEMBER) in the schema; omitting it leaves the
	// decision to the server rather than guessing client-side.
	result, err := client.Execute(context.Background(), "createUser", map[string]interface{}{
		"input": map[string]interface{}{"name": "Ada"},
	})
	require.NoError(t, err)

	echo := result.(map[string]interface{})["echo"].(map[string]interface{})
	variables := echo["variables"].(map[string]interface{})
	input := variables["input"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{"name": "Ada"}, input)
	assert.NotContains(t, input, "role")
}

func TestExecute_EnumArgumentRoundTrip(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))

	result, err := client.Execute(context.Background(), "users", map[string]interface{}{"role": "GUEST", "limit": 5})
	require.NoError(t, err)

	echo := result.(map[string]interface{})["echo"].(map[string]interface{})
	variables := echo["variables"].(map[string]interface{})
	assert.Equal(t, "GUEST", variables["role"])
	assert.EqualValues(t, 5, variables["limit"])
}

func TestExecute_UnknownOperation_FailsWithDiscoveredList(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))

	_, err := client.Execute(context.Background(), "nope", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation))
	assert.Contains(t, err.Error(), "discovered: user, users, nodes, search, ping, viewer, createUser, setRole, noop")
	assert.Empty(t, srv.requests(), "no request may leave the process for an unknown operation")
}

func TestExecute_WithoutSchema_FailsWithActionableError(t *testing.T) {
	client := newClient("http://127.0.0.1:1/graphql")
	_, err := client.Execute(context.Background(), "user", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no schema is loaded")
}

func TestExecute_SchemaLoadedLazily_IntrospectionCountedOnce(t *testing.T) {
	srv := newTestServer(t)

	client := newClient(srv.URL+"/graphql", graphql.WithSchema(srv.introspectionTarget()))
	_, err := client.Execute(context.Background(), "ping", nil)
	require.NoError(t, err)
	_, err = client.Execute(context.Background(), "ping", nil)
	require.NoError(t, err)

	calls := srv.requests()
	require.Len(t, calls, 3, "one introspection plus one POST per call — the schema is ingested once")
	assert.Contains(t, string(calls[0].Body), "__schema")
	assert.NotContains(t, string(calls[1].Body), "__schema")
}

// ── stage 4: error mapping ───────────────────────────────────────────────

func TestExecute_GraphQLErrorMapping(t *testing.T) {
	cases := []struct {
		code string
		want errors.Code
	}{
		{"UNAUTHENTICATED", errors.CodeUnauthorized},
		{"FORBIDDEN", errors.CodeUnauthorized},
		{"BAD_USER_INPUT", errors.CodeValidation},
		{"GRAPHQL_PARSE_FAILED", errors.CodeBadRequest},
		{"GRAPHQL_VALIDATION_FAILED", errors.CodeBadRequest},
		{"NOT_FOUND", errors.CodeNotFound},
		{"PERSISTED_QUERY_NOT_FOUND", errors.CodeNotFound},
		{"INTERNAL_SERVER_ERROR", errors.CodeInternal},
		{"TOO_MANY_REQUESTS", errors.CodeRateLimit},
		{"SOMETHING_ELSE", errors.CodeGeneric},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			srv := newTestServer(t, withErrorOnly(tc.code, "the server said no"))
			client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))

			_, err := client.Execute(context.Background(), "user", map[string]interface{}{"id": "u1"})
			require.Error(t, err)
			assert.True(t, errors.IsError(err, tc.want), "want %s, got %v", tc.want, err)
			assert.Contains(t, err.Error(), "the server said no", "the server's message is preserved")

			appErr, ok := err.(*errors.AppError)
			require.True(t, ok, "every adapter error is an AppError, got %T", err)
			assert.Equal(t, string(protocol.ProtocolGraphQL), appErr.Details()["protocol"])
			assert.Equal(t, tc.code, appErr.Details()["graphql_code"])
			assert.Equal(t, "user", appErr.Details()["path"])
		})
	}
}

func TestExecute_PartialSuccess_ReturnsDataAndErrors(t *testing.T) {
	srv := newTestServer(t, withPartial())
	client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))

	result, err := client.Execute(context.Background(), "user", map[string]interface{}{"id": "u1"})
	require.NoError(t, err, "data AND errors is partial success, not a failed call")

	partial, ok := result.(*graphql.PartialResult)
	require.True(t, ok, "want *PartialResult, got %T", result)
	assert.NotNil(t, partial.Data)
	require.Len(t, partial.Errors, 1)
	assert.Equal(t, "profile is unavailable", partial.Errors[0].Message)
	assert.Equal(t, "user.profile", partial.Errors[0].PathString())
	assert.Equal(t, "INTERNAL_SERVER_ERROR", partial.Errors[0].ExtensionCode())

	// The envelope is JSON-serializable, so the shared output layer renders it.
	encoded, err := json.Marshal(partial)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"errors"`)
	assert.Contains(t, string(encoded), `"data"`)
}

func TestExecute_HTTPStatusMapping(t *testing.T) {
	cases := []struct {
		status int
		want   errors.Code
	}{
		{http.StatusBadRequest, errors.CodeBadRequest},
		{http.StatusUnauthorized, errors.CodeUnauthorized},
		{http.StatusForbidden, errors.CodeUnauthorized},
		{http.StatusNotFound, errors.CodeNotFound},
		{http.StatusRequestTimeout, errors.CodeTimeout},
		{http.StatusTooManyRequests, errors.CodeRateLimit},
		{http.StatusInternalServerError, errors.CodeInternal},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			srv := newTestServer(t, withStatus(tc.status))
			client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))

			_, err := client.Execute(context.Background(), "ping", nil)
			require.Error(t, err)
			assert.True(t, errors.IsError(err, tc.want), "want %s, got %v", tc.want, err)

			if tc.status == http.StatusTooManyRequests {
				appErr, ok := err.(*errors.AppError)
				require.True(t, ok)
				assert.Equal(t, "7", appErr.Details()["retry_after"], "the server's retry hint is preserved")
			}
		})
	}
}

func TestExecute_Timeout(t *testing.T) {
	srv := newTestServer(t, withSlow(2*time.Second))
	client := newClient(srv.URL+"/graphql",
		graphql.WithSchema(sdlFixturePath),
		graphql.WithTimeout(50*time.Millisecond),
	)

	start := time.Now()
	_, err := client.Execute(context.Background(), "ping", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeTimeout), "want TIMEOUT, got %v", err)
	assert.Less(t, time.Since(start), time.Second, "the client's own timeout bounds the call")
}

func TestExecute_CallerDeadlineWins(t *testing.T) {
	srv := newTestServer(t, withSlow(2*time.Second))
	client := newClient(srv.URL+"/graphql",
		graphql.WithSchema(sdlFixturePath),
		graphql.WithTimeout(30*time.Second),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.Execute(ctx, "ping", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeTimeout), "got %v", err)
	assert.Less(t, time.Since(start), time.Second, "the caller's deadline is honored, not the adapter's default")
}

func TestExecute_MalformedResponse_IsInternalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)

	client := newClient(srv.URL, graphql.WithSchema(sdlFixturePath))
	_, err := client.Execute(context.Background(), "ping", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeInternal), "got %v", err)
}

func TestExecute_EmptyEnvelope_IsInternalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":null}`))
	}))
	t.Cleanup(srv.Close)

	client := newClient(srv.URL, graphql.WithSchema(sdlFixturePath))
	_, err := client.Execute(context.Background(), "ping", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "neither data nor errors")
}

// ── cross-cutting contract ───────────────────────────────────────────────

// denyLimiter is a rate limiter that always refuses.
type denyLimiter struct{}

// Allow implements the limiter surface the adapter composes.
func (denyLimiter) Allow() bool { return false }

func TestExecute_RateLimitRefusal_MapsToRateLimit(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(srv.URL+"/graphql",
		graphql.WithSchema(sdlFixturePath),
		graphql.WithRateLimiter(denyLimiter{}),
	)

	_, err := client.Execute(context.Background(), "ping", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeRateLimit), "got %v", err)
	assert.Empty(t, srv.requests(), "a refused call never reaches the network")
}

func TestClient_IsAProtocolAdapter(t *testing.T) {
	var adapter protocol.Adapter = newClient("")

	assert.Equal(t, protocol.ProtocolGraphQL, adapter.Protocol())

	registry := protocol.NewRegistry()
	require.NoError(t, registry.Register(adapter))

	got, err := registry.Adapter(protocol.ProtocolGraphQL)
	require.NoError(t, err)
	assert.Same(t, adapter, got)

	// A second adapter for the same family is refused, per the seam contract.
	require.Error(t, registry.Register(newClient("")))
}

func TestClient_UsesTheInjectedHTTPClient(t *testing.T) {
	srv := newTestServer(t)

	// specs/PROTOCOLS.md §3.1: adapters do not open their own HTTP clients for
	// discovery or execution. A client carrying a marker header proves both
	// paths flow through the injected pkg/client.
	shared := client.NewClient(client.WithHeader("X-Muster-Test", "injected"))
	adapter := graphql.NewClient(srv.URL+"/graphql",
		graphql.WithHTTPClient(shared),
		graphql.WithSchema(sdlFixturePath),
	)

	_, err := adapter.LoadSchema(context.Background(), sdlFixturePath)
	require.NoError(t, err)
	_, err = adapter.Execute(context.Background(), "ping", nil)
	require.NoError(t, err)

	calls := srv.requests()
	require.Len(t, calls, 1)
	assert.Equal(t, "injected", calls[0].Headers.Get("X-Muster-Test"))

	// WithHTTPClient(nil) leaves the default client in place rather than
	// clearing it.
	fallback := graphql.NewClient(srv.URL+"/graphql", graphql.WithHTTPClient(nil), graphql.WithSchema(sdlFixturePath))
	_, err = fallback.Execute(context.Background(), "ping", nil)
	require.NoError(t, err)
}

func TestExecute_MultipleErrors_KeepEveryMessage(t *testing.T) {
	srv := newTestServer(t, withErrorList("first failure", "second failure"))
	client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))

	_, err := client.Execute(context.Background(), "ping", nil)
	require.Error(t, err)

	appErr, ok := err.(*errors.AppError)
	require.True(t, ok, "got %T", err)
	assert.Contains(t, err.Error(), "first failure", "the first error classifies the failure")
	messages, ok := appErr.Details()["errors"].([]string)
	require.True(t, ok, "the whole errors array must be preserved, got %T", appErr.Details()["errors"])
	assert.Equal(t, []string{"first failure", "second failure"}, messages)
}

func TestExecute_NilArgumentsAreAccepted(t *testing.T) {
	srv := newTestServer(t)
	client := newClient(srv.URL+"/graphql", graphql.WithSchema(sdlFixturePath))

	// A caller that passes no argument map at all is the same as an empty one.
	result, err := client.Execute(context.Background(), "ping", nil)
	require.NoError(t, err)
	echo := result.(map[string]interface{})["echo"].(map[string]interface{})
	assert.Equal(t, map[string]interface{}{}, echo["variables"])
}

func TestExecute_NilOperationID_IsAValidationError(t *testing.T) {
	client := newClient("", graphql.WithSchema(sdlFixturePath))
	_, err := client.Execute(context.Background(), "   ", nil)
	require.Error(t, err)
	assert.True(t, errors.IsError(err, errors.CodeValidation), "got %v", err)
}
