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

// jsonrpcEchoServer is a minimal JSON-RPC 2.0 echo server for CLI wiring
// tests (loopback only; same codec shapes pkg/mcp serves).
func jsonrpcEchoServer(t *testing.T) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

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

		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
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
