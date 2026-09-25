package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jsonrpcRecordedCall is one request a test server observed.
type jsonrpcRecordedCall struct {
	Method      string
	Path        string
	ContentType string
	Body        []byte
}

// jsonrpcEchoHandler is the shared JSON-RPC 2.0 echo behaviour for CLI wiring
// tests (loopback only; same codec shapes pkg/mcp serves). When log is
// non-nil, every request is appended to it under mu.
func jsonrpcEchoHandler(t *testing.T, mu *sync.Mutex, log *[]jsonrpcRecordedCall) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		if log != nil {
			mu.Lock()
			*log = append(*log, jsonrpcRecordedCall{
				Method:      r.Method,
				Path:        r.URL.Path,
				ContentType: r.Header.Get("Content-Type"),
				Body:        body,
			})
			mu.Unlock()
		}

		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      interface{}     `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      req.ID,
		}
		if req.Method == "echo" {
			resp["result"] = req.Params
		} else {
			resp["error"] = map[string]interface{}{"code": -32601, "message": "Method not found"}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// jsonrpcEchoServer is a minimal JSON-RPC 2.0 echo server for CLI wiring
// tests (loopback only; same codec shapes pkg/mcp serves).
func jsonrpcEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(jsonrpcEchoHandler(t, nil, nil))
	t.Cleanup(srv.Close)
	return srv
}

// jsonrpcEchoRecorder is jsonrpcEchoServer plus a request log, so the
// endpoint-only tests can prove the call is a single POST straight to the
// endpoint (no discovery round-trip).
func jsonrpcEchoRecorder(t *testing.T) (*httptest.Server, func() []jsonrpcRecordedCall) {
	t.Helper()
	var mu sync.Mutex
	var log []jsonrpcRecordedCall
	srv := httptest.NewServer(jsonrpcEchoHandler(t, &mu, &log))
	t.Cleanup(srv.Close)
	return srv, func() []jsonrpcRecordedCall {
		mu.Lock()
		defer mu.Unlock()
		out := make([]jsonrpcRecordedCall, len(log))
		copy(out, log)
		return out
	}
}

// writeJSONRPCDoc writes an OpenRPC document naming the given server URL.
func writeJSONRPCDoc(t *testing.T, serverURL string) string {
	t.Helper()
	doc := `{
	  "openrpc": "1.2.6",
	  "servers": [{"name": "test", "url": "` + serverURL + `"}],
	  "methods": [
	    {"name": "echo", "summary": "Echo the message",
	     "params": [{"name": "msg", "required": true, "schema": {"type": "string"}}]}
	  ]
	}`
	path := filepath.Join(t.TempDir(), "openrpc.json")
	require.NoError(t, os.WriteFile(path, []byte(doc), 0o600))
	return path
}

// runProtocolJSONRPC runs `openapi-cli protocol jsonrpc ...` and returns stdout.
func runProtocolJSONRPC(t *testing.T, args ...string) string {
	t.Helper()
	out := &bytes.Buffer{}
	cmd := newProtocolJSONRPCCommand()
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return out.String()
}

func TestProtocolJSONRPC_Discover_PrintsOperations(t *testing.T) {
	srv := jsonrpcEchoServer(t)
	docPath := writeJSONRPCDoc(t, srv.URL)

	out := runProtocolJSONRPC(t, "--doc", docPath)

	assert.Contains(t, out, `"id": "echo"`, "discovered method must be listed")
	assert.Contains(t, out, `"kind": "unary"`)
	assert.Contains(t, out, `"msg"`)
}

func TestProtocolJSONRPC_Call_ExecutesOverHTTP(t *testing.T) {
	srv := jsonrpcEchoServer(t)
	docPath := writeJSONRPCDoc(t, srv.URL)

	out := runProtocolJSONRPC(t,
		"--doc", docPath,
		"--method", "echo",
		"--args", `{"msg":"through-cli"}`,
	)

	assert.Contains(t, out, "through-cli", "echoed argument must come back as the JSON result")
}

func TestProtocolJSONRPC_Call_EndpointFlagOverridesDocument(t *testing.T) {
	srv := jsonrpcEchoServer(t)
	// Document points at a dead endpoint; the flag must override.
	docPath := writeJSONRPCDoc(t, "http://127.0.0.1:1")

	out := runProtocolJSONRPC(t,
		"--doc", docPath,
		"--endpoint", srv.URL,
		"--method", "echo",
		"--args", `{"msg":"override"}`,
	)
	assert.Contains(t, out, "override")
}

// ── endpoint-only (call-only) mode, DF-025 ───────────────────────────────
//
// These tests never provide a discovery document anywhere: the endpoint flag
// plus the method name must be enough. The recorder proves the CLI issues one
// POST carrying a spec-shaped JSON-RPC 2.0 request and nothing else.

func TestProtocolJSONRPC_Call_EndpointOnly_NoDocRequired(t *testing.T) {
	srv, calls := jsonrpcEchoRecorder(t)

	out := runProtocolJSONRPC(t,
		"--endpoint", srv.URL,
		"--method", "echo",
		"--args", `{"msg":"endpoint-only"}`,
	)

	assert.Contains(t, out, "endpoint-only", "endpoint-only call must print the decoded result")

	got := calls()
	require.Len(t, got, 1, "endpoint-only call must be exactly one request (no discovery fetch)")
	assert.Equal(t, http.MethodPost, got[0].Method)
	assert.Equal(t, "application/json", got[0].ContentType)

	var wire struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      interface{}     `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	require.NoError(t, json.Unmarshal(got[0].Body, &wire))
	assert.Equal(t, "2.0", wire.JSONRPC)
	assert.Equal(t, "echo", wire.Method)
	require.NotNil(t, wire.ID, "JSON-RPC 2.0 requests must carry an id")
	var params map[string]string
	require.NoError(t, json.Unmarshal(wire.Params, &params))
	assert.Equal(t, "endpoint-only", params["msg"])
}

func TestProtocolJSONRPC_Call_EndpointOnly_DefaultArgsRideAsEmptyObject(t *testing.T) {
	srv, calls := jsonrpcEchoRecorder(t)

	// --args omitted: the "{}" default must ride the wire as an empty params
	// object (the adapter always sends params; JSON-RPC 2.0 makes it optional).
	out := runProtocolJSONRPC(t, "--endpoint", srv.URL, "--method", "echo")

	require.Len(t, calls(), 1)
	assert.Contains(t, string(calls()[0].Body), `"params":{}`)
	assert.Contains(t, out, "{", "empty params must render as the echoed object")
}

func TestProtocolJSONRPC_EndpointOnly_MissingMethod_Fails(t *testing.T) {
	const endpoint = "http://127.0.0.1:1/rpc"

	cmd := newProtocolJSONRPCCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--endpoint", endpoint})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--method", "usage error must name the missing flag")
	assert.Contains(t, err.Error(), endpoint, "usage error must name the endpoint in play")
}

func TestProtocolJSONRPC_EndpointOnly_MissingEndpoint_Fails(t *testing.T) {
	cmd := newProtocolJSONRPCCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--method", "echo"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--endpoint", "usage error must name the missing flag")
	assert.Contains(t, err.Error(), "--doc", "usage error must state the --doc alternative")
}

func TestProtocolJSONRPC_EndpointOnly_InvalidArgsJSON_Fails(t *testing.T) {
	cmd := newProtocolJSONRPCCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	// A JSON array is not the documented "JSON object" shape.
	cmd.SetArgs([]string{"--endpoint", "http://127.0.0.1:1/rpc", "--method", "echo", "--args", "[1,2]"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--args")
}

func TestProtocolJSONRPC_EndpointOnly_RemoteMethodNotFound_Surfaces(t *testing.T) {
	srv := jsonrpcEchoServer(t)

	cmd := newProtocolJSONRPCCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--endpoint", srv.URL, "--method", "nope"})

	err := cmd.Execute()
	require.Error(t, err)
	// The endpoint answered: no discovery-based "method not found in the
	// discovery document" wording may appear in call-only mode.
	assert.Contains(t, err.Error(), "Method not found")
	assert.NotContains(t, err.Error(), "not found in the discovery document")
}

func TestProtocolJSONRPC_Call_UnknownMethod_FailsWithDiscoveredList(t *testing.T) {
	srv := jsonrpcEchoServer(t)
	docPath := writeJSONRPCDoc(t, srv.URL)

	cmd := newProtocolJSONRPCCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--doc", docPath, "--method", "missing"})
	err := cmd.Execute()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "not found"))
	assert.True(t, strings.Contains(err.Error(), "echo"), "error must name the discovered methods")
}

func TestProtocolJSONRPC_MissingDocFlag_Fails(t *testing.T) {
	cmd := newProtocolJSONRPCCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--doc")
}

// Root-level wiring: the protocol group must be mounted on the frozen root.
func TestProtocolCommand_MountedOnRoot(t *testing.T) {
	root := NewRootCommand(nil)
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "protocol" {
			found = true
			assert.True(t, c.HasSubCommands(), "protocol group must carry protocol members")
			break
		}
	}
	assert.True(t, found, "protocol command must be registered on the root")
}

// Silence the unused-context import in constrained configurations.
var _ = context.Background
