package main

// DF-036 end-to-end coverage gaps beyond the unit and env-path tests:
//
//   1. The --auth-token-file credential path is proven only at the
//      resolveAuthToken unit level; this file drives it through the real
//      binary and a real tools/call against a bearer-gated upstream.
//   2. The upstream-401 contrast arm: with NO credential configured the same
//      upstream refuses the call, and the auth failure must surface to the
//      MCP client as a tool error — proving the mechanism is load-bearing
//      (mirrors the live-consensus 401 the dogfood run hit).
//   3. Help visibility: an operator wiring the server must be able to
//      discover the auth option without reading source.

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

const e2eAuthToken = "df036-e2e-fixture-token"

// bearerGatedUpstream answers 200 only for the exact configured bearer
// credential and 401 otherwise, counting authorized exchanges.
func bearerGatedUpstream(t *testing.T, token string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var authorized atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+token {
			authorized.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"UNAUTHENTICATED missing API key"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &authorized
}

// authedSpec writes an OpenAPI spec whose servers[] points at srv.URL.
func authedSpec(t *testing.T, srvURL string) string {
	t.Helper()
	spec := fmt.Sprintf(`openapi: 3.0.0
info:
  title: Authed API
  version: 1.0.0
servers:
  - url: %s
paths:
  /api/v1/sessions:
    get:
      operationId: listSessions
      responses:
        '200':
          description: OK
`, srvURL)
	path := filepath.Join(t.TempDir(), "authed-spec.yaml")
	require.NoError(t, os.WriteFile(path, []byte(spec), 0o600))
	return path
}

// driveAuthedBinary starts the mcp-server binary, sends one tools/call frame,
// and returns the decoded response alongside everything the server wrote to
// stderr (for credential-leak assertions).
func driveAuthedBinary(t *testing.T, token string, args ...string) (map[string]interface{}, string) {
	t.Helper()

	binaryPath, err := buildMCPServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(binaryPath) })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, binaryPath, args...)
	if token == "" {
		cmd.Env = append(os.Environ(), "MUSTER_AUTH_TOKEN=")
	} else {
		cmd.Env = append(os.Environ(), "MUSTER_AUTH_TOKEN="+token)
	}

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

	frame, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  map[string]interface{}{"name": "listSessions", "arguments": map[string]interface{}{}},
	})
	require.NoError(t, err)
	_, err = stdin.Write(append(frame, '\n'))
	require.NoError(t, err)

	line, err := bufio.NewReader(stdout).ReadBytes('\n')
	require.NoError(t, err, "mcp stderr: %s", stderr.String())
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(line, &response))
	return response, stderr.String()
}

func toolsCallIsError(t *testing.T, response map[string]interface{}) (bool, string) {
	t.Helper()
	require.Nil(t, response["error"], "unexpected JSON-RPC error: %#v", response["error"])
	result, ok := response["result"].(map[string]interface{})
	require.True(t, ok, "result has type %T", response["result"])
	isError, _ := result["isError"].(bool)
	return isError, toolContentText(t, result["content"])
}

// toolContentText decodes the first text item of a tools/call content array.
func toolContentText(t *testing.T, content interface{}) string {
	t.Helper()
	items, ok := content.([]interface{})
	require.True(t, ok, "content has type %T", content)
	require.NotEmpty(t, items)
	first, ok := items[0].(map[string]interface{})
	require.True(t, ok, "content[0] has type %T", items[0])
	text, _ := first["text"].(string)
	return text
}

// toolResultStatus decodes a JSON-object tool result and returns its status.
func toolResultStatus(t *testing.T, text string) string {
	t.Helper()
	var payload struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal([]byte(text), &payload), "tool result text: %s", text)
	return payload.Status
}

func TestBinaryExecution_AuthTokenFileAuthenticatesToolsCall(t *testing.T) {
	srv, authorized := bearerGatedUpstream(t, e2eAuthToken)
	specPath := authedSpec(t, srv.URL)
	tokenFile := filepath.Join(t.TempDir(), "consensus.key")
	require.NoError(t, os.WriteFile(tokenFile, []byte(e2eAuthToken+"\n"), 0o600))

	response, stderr := driveAuthedBinary(t, e2eAuthToken,
		"--spec", specPath, "--auth-token-file", tokenFile)

	isErr, text := toolsCallIsError(t, response)
	require.False(t, isErr, "tools/call with --auth-token-file must authenticate, got: %s", text)
	require.Equal(t, "ok", toolResultStatus(t, text))
	require.Equal(t, int32(1), authorized.Load(), "exactly one authorized exchange")
	require.False(t, strings.Contains(stderr, e2eAuthToken), "credential must not be written to stderr")
}

func TestBinaryExecution_MissingTokenSurfacesUpstreamAuthFailure(t *testing.T) {
	srv, authorized := bearerGatedUpstream(t, e2eAuthToken)
	specPath := authedSpec(t, srv.URL)

	response, stderr := driveAuthedBinary(t, "", "--spec", specPath)

	isErr, content := toolsCallIsError(t, response)
	require.True(t, isErr, "without credentials the bearer-gated upstream must refuse the call")
	require.Contains(t, content, "401", "the upstream auth failure must surface to the MCP client: %s", content)
	require.Equal(t, int32(0), authorized.Load())
	require.False(t, strings.Contains(stderr, e2eAuthToken))
}

func TestBinaryExecution_HelpExposesAuthOptions(t *testing.T) {
	binaryPath, err := buildMCPServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(binaryPath) })

	out, err := exec.Command(binaryPath, "-h").CombinedOutput()
	require.NoError(t, err)
	require.Contains(t, string(out), "-auth-token-file",
		"mcp-server -h must expose the --auth-token-file option")
}
