package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBinaryExecution_MUSTERAuthTokenAuthenticatesEveryToolCall(t *testing.T) {
	const token = "df-036-regression-token"

	var authenticatedCalls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"missing bearer credential"}`))
			return
		}
		authenticatedCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(api.Close)

	specPath := filepath.Join(t.TempDir(), "auth-api.yaml")
	spec := fmt.Sprintf(`openapi: 3.0.0
info:
  title: Authenticated API
  version: 1.0.0
servers:
  - url: %s
paths:
  /sessions:
    get:
      operationId: listSessions
      responses:
        '200':
          description: OK
    post:
      operationId: createSession
      responses:
        '200':
          description: OK
`, api.URL)
	require.NoError(t, os.WriteFile(specPath, []byte(spec), 0o600))

	binaryPath, err := buildMCPServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(binaryPath) })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, binaryPath, "--spec", specPath)
	cmd.Env = append(os.Environ(), "MUSTER_AUTH_TOKEN="+token)

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})

	reader := bufio.NewReader(stdout)
	callTool := func(id int, name string) map[string]interface{} {
		t.Helper()
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      name,
				"arguments": map[string]interface{}{},
			},
		}
		frame, marshalErr := json.Marshal(request)
		require.NoError(t, marshalErr)
		_, err = stdin.Write(append(frame, '\n'))
		require.NoError(t, err)

		line, readErr := reader.ReadBytes('\n')
		require.NoError(t, readErr, "mcp stderr: %s", stderr.String())
		var response map[string]interface{}
		require.NoError(t, json.Unmarshal(line, &response))
		return response
	}

	for id, tool := range []string{"listSessions", "createSession"} {
		response := callTool(id+1, tool)
		require.Nil(t, response["error"], "tools/call %s returned JSON-RPC error: %#v", tool, response["error"])
		result, ok := response["result"].(map[string]interface{})
		require.True(t, ok, "tools/call %s result has type %T", tool, response["result"])
		require.NotEqual(t, true, result["isError"], "tools/call %s failed: %#v", tool, result["content"])
	}

	require.Equal(t, int32(2), authenticatedCalls.Load())
	require.False(t, strings.Contains(stderr.String(), token), "credential must not be written to stderr")
}
