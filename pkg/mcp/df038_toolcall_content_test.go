package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/mcp/handlers"
)

// DF-038. pkg/mcp is the code that maps an upstream HTTP response onto an MCP
// tools/call result. These tests drive the production executor
// (NewOpenAPIExecutor -> handlers.NewCallToolHandler, exactly what
// cmd/mcp-server registers) and pin the wire shape of the result.

// df038Configs builds a single-tool execution config pointing at url.
func df038Configs(name, url string) map[string]ExecutionConfig {
	return map[string]ExecutionConfig{
		name: {
			BaseURL:     url,
			Method:      http.MethodGet,
			Path:        "/health",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}
}

// df038Call drives one tools/call through the production handler and returns
// the marshaled result as parsed JSON.
func df038Call(t *testing.T, executor interface{}, name string) map[string]interface{} {
	t.Helper()

	handler := handlers.NewCallToolHandler(executor)
	result, err := handler.Handle(context.Background(),
		json.RawMessage(`{"name":"`+name+`","arguments":{}}`))
	require.NoError(t, err)

	raw, err := json.Marshal(result)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	return parsed
}

// df038Text returns the first text content block of a tool result.
func df038Text(t *testing.T, parsed map[string]interface{}) string {
	t.Helper()

	blocks, ok := parsed["content"].([]interface{})
	require.True(t, ok, "content must be a JSON array, got %#v", parsed["content"])
	require.NotEmpty(t, blocks)
	block, ok := blocks[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "text", block["type"])
	text, ok := block["text"].(string)
	require.True(t, ok)
	return text
}

// df038ErrorMarker extracts structuredContent.error.
func df038ErrorMarker(t *testing.T, parsed map[string]interface{}) map[string]interface{} {
	t.Helper()

	structured, ok := parsed["structuredContent"].(map[string]interface{})
	require.True(t, ok, "result must carry structuredContent, got %#v", parsed)
	marker, ok := structured["error"].(map[string]interface{})
	require.True(t, ok, "structuredContent must carry an error marker, got %#v", structured)
	return marker
}

// df038UnauthorizedServer serves a JSON 401 with no credential check, recording
// whether an Authorization header arrived.
func df038UnauthorizedServer(t *testing.T, sawAuth *string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized","hint":"missing token"}`))
	}))
}

// TestDF038_ToolsCall_401WithoutCredential_NamesCredentialMechanism is the
// board row's reproduction: an agent calling a tool against a 401 without the
// credential configured must receive the credential mechanism it was expected
// from, plus a client-vs-upstream marker — not just the upstream body.
func TestDF038_ToolsCall_401WithoutCredential_NamesCredentialMechanism(t *testing.T) {
	var sawAuth string
	srv := df038UnauthorizedServer(t, &sawAuth)
	defer srv.Close()

	executor := NewOpenAPIExecutor(df038Configs("healthCheck", srv.URL))
	parsed := df038Call(t, executor, "healthCheck")

	require.Empty(t, sawAuth, "fixture must exercise the unauthenticated path")
	require.Equal(t, true, parsed["isError"], "isError:true must still be set")

	text := df038Text(t, parsed)
	require.Contains(t, text, "401")
	require.Contains(t, text, "MUSTER_AUTH_TOKEN",
		"the failure text must name the credential mechanism")
	require.Contains(t, text, "--auth-token-file")

	marker := df038ErrorMarker(t, parsed)
	require.Equal(t, "client_error", marker["class"],
		"a missing credential is client-side wiring, not an upstream API error")
	require.Equal(t, "missing_credential", marker["code"])
	require.Equal(t, float64(401), marker["status"])
	require.Contains(t, marker["credentialSource"], "MUSTER_AUTH_TOKEN",
		"the marker must name where the credential was expected from")

	upstream, ok := marker["upstreamBody"].(map[string]interface{})
	require.True(t, ok, "the upstream JSON body must stay structured, got %#v", marker["upstreamBody"])
	require.Equal(t, "unauthorized", upstream["error"])
	require.Equal(t, "missing token", upstream["hint"])
}

// TestDF038_ToolsCall_401WithCredential_ReportsRejectedCredential covers the
// configured-credential case: still client-side (auth wiring), but reported
// with the source it came from — and never the credential value.
func TestDF038_ToolsCall_401WithCredential_ReportsRejectedCredential(t *testing.T) {
	const token = "df038-fixture-credential"
	var sawAuth string
	srv := df038UnauthorizedServer(t, &sawAuth)
	defer srv.Close()

	executor := NewOpenAPIExecutor(
		df038Configs("healthCheck", srv.URL),
		WithBearerToken(token),
		WithCredentialSource("the --auth-token-file flag (/tmp/df038.key)"),
	)
	parsed := df038Call(t, executor, "healthCheck")

	require.Equal(t, "Bearer "+token, sawAuth, "the configured credential must still be sent")
	require.Equal(t, true, parsed["isError"])

	text := df038Text(t, parsed)
	require.Contains(t, text, "--auth-token-file", "the text must name the credential source")
	require.NotContains(t, text, token, "the credential value must never be echoed")

	marker := df038ErrorMarker(t, parsed)
	require.Equal(t, "client_error", marker["class"])
	require.Equal(t, "credential_rejected", marker["code"])
	require.Equal(t, float64(401), marker["status"])
	require.Contains(t, marker["credentialSource"], "--auth-token-file")
	require.NotContains(t, marker["credentialSource"], token)
}

// TestDF038_ToolsCall_ClientVsUpstream_Markers pins the classification across
// the failure shapes an agent can hit.
func TestDF038_ToolsCall_ClientVsUpstream_Markers(t *testing.T) {
	t.Run("unresolvable operation is client-side", func(t *testing.T) {
		executor := NewOpenAPIExecutor(df038Configs("healthCheck", "http://127.0.0.1:1"))
		parsed := df038Call(t, executor, "no-such-tool")

		require.Equal(t, true, parsed["isError"])
		marker := df038ErrorMarker(t, parsed)
		require.Equal(t, "client_error", marker["class"])
		require.Equal(t, "unknown_operation", marker["code"])
		require.Contains(t, df038Text(t, parsed), "no-such-tool")
	})

	t.Run("invalid arguments are client-side", func(t *testing.T) {
		configs := map[string]ExecutionConfig{
			"createPet": {
				BaseURL:     "http://127.0.0.1:1",
				Method:      http.MethodPost,
				Path:        "/pets",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
			},
		}
		executor := NewOpenAPIExecutor(configs)
		handler := handlers.NewCallToolHandler(executor)
		result, err := handler.Handle(context.Background(),
			json.RawMessage(`{"name":"createPet","arguments":{}}`))
		require.NoError(t, err)
		raw, err := json.Marshal(result)
		require.NoError(t, err)
		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(raw, &parsed))

		require.Equal(t, true, parsed["isError"])
		marker := df038ErrorMarker(t, parsed)
		require.Equal(t, "client_error", marker["class"])
		require.Equal(t, "invalid_arguments", marker["code"])
	})

	t.Run("upstream 5xx is an upstream error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
		}))
		defer srv.Close()

		executor := NewOpenAPIExecutor(df038Configs("healthCheck", srv.URL))
		parsed := df038Call(t, executor, "healthCheck")

		require.Equal(t, true, parsed["isError"])
		marker := df038ErrorMarker(t, parsed)
		require.Equal(t, "upstream_error", marker["class"])
		require.Equal(t, "upstream_http_error", marker["code"])
		require.Equal(t, float64(500), marker["status"])
		require.Contains(t, df038Text(t, parsed), "boom")
	})

	t.Run("unreachable upstream is an upstream error", func(t *testing.T) {
		// Port 1 refuses connections: a transport failure, not a client-side
		// argument problem.
		executor := NewOpenAPIExecutor(df038Configs("healthCheck", "http://127.0.0.1:1"))
		parsed := df038Call(t, executor, "healthCheck")

		require.Equal(t, true, parsed["isError"])
		marker := df038ErrorMarker(t, parsed)
		require.Equal(t, "upstream_error", marker["class"])
		require.Equal(t, "upstream_unreachable", marker["code"])
	})
}

// TestDF038_ToolsCall_UpstreamError_NonJSONBodyStaysInText pins that a
// non-JSON upstream error body is not advertised as an upstreamBody object: the
// marker carries no null-valued structure, and the body remains in the text.
func TestDF038_ToolsCall_UpstreamError_NonJSONBodyStaysInText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("<html>bad gateway</html>"))
	}))
	defer srv.Close()

	executor := NewOpenAPIExecutor(df038Configs("healthCheck", srv.URL))
	parsed := df038Call(t, executor, "healthCheck")

	marker := df038ErrorMarker(t, parsed)
	require.Equal(t, "upstream_error", marker["class"])
	require.Equal(t, "upstream_http_error", marker["code"])
	require.Equal(t, float64(502), marker["status"])
	require.NotContains(t, marker, "upstreamBody",
		"a non-JSON body must not be advertised as structured upstream content")
	require.Contains(t, df038Text(t, parsed), "bad gateway")
}

// TestDF038_ToolsCall_JSONUpstream_PreservesStructuredContent covers the
// response half: a JSON upstream body is preserved as structured content
// instead of one flat text blob, while the text block and isError semantics
// stay unchanged.
func TestDF038_ToolsCall_JSONUpstream_PreservesStructuredContent(t *testing.T) {
	payload := `{"status":"healthy","checks":{"db":true,"cache":true},"uptime":42}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	executor := NewOpenAPIExecutor(df038Configs("healthCheck", srv.URL))
	parsed := df038Call(t, executor, "healthCheck")

	require.NotContains(t, parsed, "isError", "success must not set isError")

	structured, ok := parsed["structuredContent"].(map[string]interface{})
	require.True(t, ok, "upstream JSON must be preserved as structured content, got %#v", parsed)
	require.Equal(t, "healthy", structured["status"])
	require.Equal(t, float64(42), structured["uptime"])
	checks, ok := structured["checks"].(map[string]interface{})
	require.True(t, ok, "nested JSON must stay structured, got %#v", structured["checks"])
	require.Equal(t, true, checks["db"])

	// The text block is still present and still carries the upstream JSON.
	var textPayload map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(df038Text(t, parsed)), &textPayload))
	require.Equal(t, structured, textPayload)
}

// TestDF038_ToolsCall_JSONArrayUpstream_KeptStructured covers a top-level JSON
// array (DF-016): it must not be flattened into a text-only result.
func TestDF038_ToolsCall_JSONArrayUpstream_KeptStructured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1},{"id":2}]`))
	}))
	defer srv.Close()

	executor := NewOpenAPIExecutor(df038Configs("healthCheck", srv.URL))
	parsed := df038Call(t, executor, "healthCheck")

	structured, ok := parsed["structuredContent"].(map[string]interface{})
	require.True(t, ok, "JSON array must stay structured, got %#v", parsed)
	items, ok := structured["result"].([]interface{})
	require.True(t, ok, "non-object JSON is carried under result, got %#v", structured)
	require.Len(t, items, 2)
}

// TestDF038_ToolsCall_NonJSONUpstream_TextFallback covers a non-JSON upstream
// body: text content carries it (as before) and no structured content is
// advertised, so the distinction between "was JSON" and "was text" survives.
func TestDF038_ToolsCall_NonJSONUpstream_TextFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("OK - all systems nominal"))
	}))
	defer srv.Close()

	executor := NewOpenAPIExecutor(df038Configs("healthCheck", srv.URL))
	parsed := df038Call(t, executor, "healthCheck")

	require.NotContains(t, parsed, "isError")
	require.NotContains(t, parsed, "structuredContent",
		"a non-JSON upstream body must not be advertised as structured content")

	text := df038Text(t, parsed)
	require.Contains(t, text, "OK - all systems nominal",
		"the raw body must remain available in the text content block")
}

// TestDF038_ToolsCall_WireShape covers the JSON-RPC wire shape end to end: the
// result object answering tools/call must keep content[] + isError and carry
// the marker, exactly as an MCP client receives it.
func TestDF038_ToolsCall_WireShape(t *testing.T) {
	var sawAuth string
	srv := df038UnauthorizedServer(t, &sawAuth)
	defer srv.Close()

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(stdinR, stdoutW, &writeCounter{})
	s.RegisterHandler(handlers.NewCallToolHandler(
		NewOpenAPIExecutor(df038Configs("healthCheck", srv.URL))))

	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"healthCheck","arguments":{}}`),
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)
	_, err = stdinW.Write(append(data, '\n'))
	require.NoError(t, err)
	require.NoError(t, stdinW.Close())

	var resp JSONRPCResponse
	require.NoError(t, json.NewDecoder(stdoutR).Decode(&resp))
	require.Nil(t, resp.Error)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Result, &result))
	require.Equal(t, true, result["isError"])
	blocks, ok := result["content"].([]interface{})
	require.True(t, ok, "content must stay an array, got %#v", result["content"])
	require.NotEmpty(t, blocks)

	marker := df038ErrorMarker(t, result)
	require.Equal(t, "client_error", marker["class"])
	require.Equal(t, "missing_credential", marker["code"])

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
}

// writeCounter discards server diagnostics while staying io.Writer compatible.
type writeCounter struct{ n int }

func (w *writeCounter) Write(p []byte) (int, error) {
	w.n += len(p)
	return len(p), nil
}
