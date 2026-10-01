package builtin

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// graphqlFixtureSchema is the shared PROTO-005 SDL fixture, resolved from this
// package's directory (the adapter's own tests use the same document, so the CLI
// tests exercise the real ingestion path rather than a copy).
const graphqlFixtureSchema = "../../pkg/protocol/graphql/testdata/schema.graphql"

// graphqlFixtureGoldenQuery is the checked-in expected document for the `user`
// operation called with {"id":"u1"}.
const graphqlFixtureGoldenQuery = "../../pkg/protocol/graphql/testdata/golden_user_query.graphql"

// graphqlRecordedCall is one request the CLI test server observed.
type graphqlRecordedCall struct {
	Method      string
	Path        string
	ContentType string
	Body        []byte
}

// graphqlEchoHandler is a minimal loopback GraphQL server for CLI wiring tests:
// an introspection request is answered from the fixture (unless disabled) and a
// data request receives an echo of the operation name, document and variables.
func graphqlEchoHandler(t *testing.T, mu *sync.Mutex, log *[]graphqlRecordedCall, introspectDisabled bool) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		if log != nil {
			mu.Lock()
			*log = append(*log, graphqlRecordedCall{
				Method:      r.Method,
				Path:        r.URL.Path,
				ContentType: r.Header.Get("Content-Type"),
				Body:        body,
			})
			mu.Unlock()
		}

		var req struct {
			Query         string                 `json:"query"`
			Variables     map[string]interface{} `json:"variables"`
			OperationName string                 `json:"operationName"`
		}
		require.NoError(t, json.Unmarshal(body, &req))
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(req.Query, "__schema") {
			if introspectDisabled {
				_, _ = w.Write([]byte(`{"errors":[{"message":"GraphQL introspection is not allowed by Apollo Server, but the query contained __schema","extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`))
				return
			}
			fixture, err := os.ReadFile("../../pkg/protocol/graphql/testdata/introspection.json")
			require.NoError(t, err)
			_, _ = w.Write(fixture)
			return
		}

		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"echo": map[string]interface{}{
					"operationName": req.OperationName,
					"query":         req.Query,
					"variables":     req.Variables,
				},
			},
		}
		out, err := json.Marshal(resp)
		require.NoError(t, err)
		_, _ = w.Write(out)
	}
}

// graphqlEchoRecorder starts the loopback server plus a request log.
func graphqlEchoRecorder(t *testing.T, introspectDisabled bool) (*httptest.Server, func() []graphqlRecordedCall) {
	t.Helper()
	var mu sync.Mutex
	var log []graphqlRecordedCall
	srv := httptest.NewServer(graphqlEchoHandler(t, &mu, &log, introspectDisabled))
	t.Cleanup(srv.Close)
	return srv, func() []graphqlRecordedCall {
		mu.Lock()
		defer mu.Unlock()
		out := make([]graphqlRecordedCall, len(log))
		copy(out, log)
		return out
	}
}

// runProtocolGraphQL runs `openapi-cli protocol graphql ...` and returns stdout.
func runProtocolGraphQL(t *testing.T, args ...string) string {
	t.Helper()
	out := &bytes.Buffer{}
	cmd := newProtocolGraphQLCommand()
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return out.String()
}

// runProtocolGraphQLErr runs the command expecting a failure.
func runProtocolGraphQLErr(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newProtocolGraphQLCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	err := cmd.Execute()
	require.Error(t, err)
	return err
}

func TestProtocolGraphQL_CommandMountedOnProtocolGroup(t *testing.T) {
	group := newProtocolCommand()
	var graphql *cobra.Command
	for _, sub := range group.Commands() {
		if sub.Name() == "graphql" {
			graphql = sub
		}
	}
	require.NotNil(t, graphql, "protocol group must expose the graphql subcommand")
	assert.Contains(t, graphql.Long, "specs/PROTOCOLS.md")
	assert.Contains(t, graphql.Long, "Known limits", "the command documents its own boundaries")
	assert.NotNil(t, graphql.Flags().Lookup(FlagGraphQLSchema))
	assert.NotNil(t, graphql.Flags().Lookup(FlagGraphQLOperation))
	assert.NotNil(t, graphql.Flags().Lookup(FlagGraphQLDocument))
}

func TestProtocolGraphQL_Discovery_PrintsOperationsWithTheirSource(t *testing.T) {
	out := runProtocolGraphQL(t, "--schema", graphqlFixtureSchema)

	var views []struct {
		ID            string `json:"id"`
		Kind          string `json:"kind"`
		OperationType string `json:"operation_type"`
		Field         string `json:"field"`
		FieldType     string `json:"field_type"`
		Parameters    []struct {
			Name        string `json:"name"`
			Type        string `json:"type"`
			GraphQLType string `json:"graphql_type"`
			Required    bool   `json:"required"`
			Enum        []string
		} `json:"parameters"`
		Source map[string]string `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &views))
	require.Len(t, views, 9)

	ids := make([]string, 0, len(views))
	for _, v := range views {
		ids = append(ids, v.ID)
		assert.Equal(t, "unary", v.Kind)
		assert.Equal(t, graphqlFixtureSchema, v.Source["schema_source"], "discovery records where the schema came from")
		assert.Equal(t, "sdl", v.Source["schema_format"])
		assert.NotEmpty(t, v.Field, "discovery records the schema field behind the command")
	}
	assert.Equal(t, []string{"user", "users", "nodes", "search", "ping", "viewer", "createUser", "setRole", "noop"}, ids)
	assert.NotContains(t, ids, "userChanged", "subscriptions are out of scope")

	user := views[0]
	assert.Equal(t, "query", user.OperationType)
	assert.Equal(t, "User", user.FieldType)
	require.Len(t, user.Parameters, 1)
	assert.Equal(t, "id", user.Parameters[0].Name)
	assert.Equal(t, "string", user.Parameters[0].Type)
	assert.Equal(t, "ID!", user.Parameters[0].GraphQLType, "the GraphQL type stays visible next to the neutral one")
	assert.True(t, user.Parameters[0].Required)
}

func TestProtocolGraphQL_Call_ExecutesOverHTTP(t *testing.T) {
	srv, requests := graphqlEchoRecorder(t, false)

	out := runProtocolGraphQL(t,
		"--schema", graphqlFixtureSchema,
		"--endpoint", srv.URL+"/graphql",
		"--operation", "user",
		"--args", `{"id":"u1"}`,
	)

	assert.Contains(t, out, `"operationName": "UserQuery"`)
	assert.Contains(t, out, `"id": "u1"`)

	calls := requests()
	require.Len(t, calls, 1, "one POST per call: no discovery round-trip when the schema is a document")
	assert.Equal(t, http.MethodPost, calls[0].Method)
	assert.Equal(t, "/graphql", calls[0].Path)
	assert.Equal(t, "application/json", calls[0].ContentType)
	assert.Contains(t, string(calls[0].Body), `"query":"query UserQuery($id: ID!)`)
}

func TestProtocolGraphQL_Document_PrintsTheBuiltQueryDocument(t *testing.T) {
	out := runProtocolGraphQL(t,
		"--schema", graphqlFixtureSchema,
		"--operation", "user",
		"--args", `{"id":"u1"}`,
		"--document",
	)

	golden, err := os.ReadFile(graphqlFixtureGoldenQuery)
	require.NoError(t, err)
	// Line endings are normalized before comparing: the golden file is committed
	// with LF, but a Windows checkout (core.autocrlf) hands it back with CRLF
	// while the printed document always uses LF.
	want := strings.ReplaceAll(string(golden), "\r\n", "\n")
	assert.Equal(t, strings.TrimSpace(want), strings.TrimSpace(out),
		"--document prints exactly the document the request builder would send")

	// A field with no arguments needs no variable declaration block.
	out = runProtocolGraphQL(t, "--schema", graphqlFixtureSchema, "--operation", "ping", "--document")
	assert.Equal(t, "query PingQuery {\n  ping\n}", strings.TrimSpace(out))
}

func TestProtocolGraphQL_FieldsFlag_RestrictsTheDerivedSelection(t *testing.T) {
	out := runProtocolGraphQL(t,
		"--schema", graphqlFixtureSchema,
		"--operation", "user",
		"--args", `{"id":"u1"}`,
		"--fields", "id,name",
		"--document",
	)

	assert.Contains(t, out, "    id\n    name\n")
	assert.NotContains(t, out, "email")
}

func TestProtocolGraphQL_IntrospectionTarget_OverLoopback(t *testing.T) {
	srv, requests := graphqlEchoRecorder(t, false)
	target := "graphql://" + strings.TrimPrefix(srv.URL, "http://") + "/graphql"

	// Discovery only: the introspection query supplies both the commands and
	// the endpoint.
	out := runProtocolGraphQL(t, "--schema", target)
	assert.Contains(t, out, `"id": "user"`)
	assert.Contains(t, out, `"schema_format": "introspection-query"`)

	calls := requests()
	require.Len(t, calls, 1)
	assert.Contains(t, string(calls[0].Body), "__schema")

	// Calling reuses the endpoint the introspection target named.
	out = runProtocolGraphQL(t, "--schema", target, "--operation", "users", "--args", `{"limit":5}`)
	assert.Contains(t, out, `"operationName": "UsersQuery"`)
	assert.Contains(t, out, `"limit": 5`)

	calls = requests()
	require.Len(t, calls, 3, "introspection, then one POST per call")
}

func TestProtocolGraphQL_IntrospectionDisabled_SurfacesActionableError(t *testing.T) {
	srv, _ := graphqlEchoRecorder(t, true)
	target := "graphql://" + strings.TrimPrefix(srv.URL, "http://") + "/graphql"

	err := runProtocolGraphQLErr(t, "--schema", target)
	assert.Contains(t, err.Error(), "introspection")
	assert.Contains(t, err.Error(), "SDL", "the operator is told which fallback to use")
}

func TestProtocolGraphQL_MissingSchema_NamesEveryForm(t *testing.T) {
	err := runProtocolGraphQLErr(t, "--operation", "user")
	assert.Contains(t, err.Error(), "--schema is required")
	assert.Contains(t, err.Error(), "schema.graphql")
	assert.Contains(t, err.Error(), "graphql://")
}

func TestProtocolGraphQL_UnknownOperation_NamesTheDiscoveredOnes(t *testing.T) {
	err := runProtocolGraphQLErr(t, "--schema", graphqlFixtureSchema, "--operation", "nope")
	assert.Contains(t, err.Error(), `operation "nope" not found`)
	assert.Contains(t, err.Error(), "discovered: user, users")
}

func TestProtocolGraphQL_CallWithoutEndpoint_Fails(t *testing.T) {
	// An SDL document names no endpoint, so a call must be told where to go.
	err := runProtocolGraphQLErr(t, "--schema", graphqlFixtureSchema, "--operation", "ping")
	assert.Contains(t, err.Error(), "endpoint")
}

func TestProtocolGraphQL_InvalidArgs_FailsBeforeAnyRequest(t *testing.T) {
	srv, requests := graphqlEchoRecorder(t, false)

	err := runProtocolGraphQLErr(t,
		"--schema", graphqlFixtureSchema,
		"--endpoint", srv.URL+"/graphql",
		"--operation", "user",
		"--args", `{"id":`,
	)
	assert.Contains(t, err.Error(), "invalid --args JSON object")
	assert.Empty(t, requests(), "no request may leave the process with unparseable arguments")
}

func TestProtocolGraphQL_ValidationErrors_SurfaceThroughTheCLI(t *testing.T) {
	srv, requests := graphqlEchoRecorder(t, false)

	err := runProtocolGraphQLErr(t,
		"--schema", graphqlFixtureSchema,
		"--endpoint", srv.URL+"/graphql",
		"--operation", "user",
	)
	assert.Contains(t, err.Error(), `missing required argument "id"`)
	assert.Empty(t, requests())

	err = runProtocolGraphQLErr(t,
		"--schema", graphqlFixtureSchema,
		"--endpoint", srv.URL+"/graphql",
		"--operation", "users",
		"--args", `{"role":"SUPERUSER"}`,
	)
	assert.Contains(t, err.Error(), "ADMIN, MEMBER, GUEST")
	assert.Empty(t, requests())
}

func TestProtocolGraphQL_OutputFormats(t *testing.T) {
	srv, _ := graphqlEchoRecorder(t, false)

	raw := runProtocolGraphQL(t,
		"--schema", graphqlFixtureSchema,
		"--endpoint", srv.URL+"/graphql",
		"--operation", "ping",
		"--output", "raw",
	)
	assert.False(t, strings.Contains(raw, "\n  "), "raw output is compact: %q", raw)
	assert.Contains(t, raw, `"echo"`)

	err := runProtocolGraphQLErr(t,
		"--schema", graphqlFixtureSchema,
		"--endpoint", srv.URL+"/graphql",
		"--operation", "ping",
		"--output", "xml",
	)
	assert.Contains(t, err.Error(), "unsupported output format")
}

func TestProtocolGraphQL_SchemaOverHTTPAndFileURL(t *testing.T) {
	sdl, err := os.ReadFile(graphqlFixtureSchema)
	require.NoError(t, err)
	docSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write(sdl)
	}))
	t.Cleanup(docSrv.Close)

	out := runProtocolGraphQL(t, "--schema", docSrv.URL+"/schema.graphql")
	assert.Contains(t, out, `"id": "user"`)
	assert.Contains(t, out, docSrv.URL+"/schema.graphql")

	// file:// URLs resolve to the same document.
	abs, err := filepath.Abs(graphqlFixtureSchema)
	require.NoError(t, err)
	out = runProtocolGraphQL(t, "--schema", "file://"+abs)
	assert.Contains(t, out, `"id": "user"`)
}
