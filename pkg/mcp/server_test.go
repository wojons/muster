package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/mcp/handlers"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// jsonRPCRequestLine returns a single JSON-RPC 2.0 request as a string ending
// with a newline, suitable for writing to server stdin.
func jsonRPCRequestLine(id int, method string, params interface{}) string {
	var rawParams json.RawMessage
	if params != nil {
		rawParams, _ = json.Marshal(params)
	}
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  rawParams,
	}
	data, _ := json.Marshal(req)
	return string(data) + "\n"
}

// readResponse reads one JSON line from the provided reader until newline.
func readResponse(t *testing.T, r io.Reader) []byte {
	t.Helper()
	// Read until newline
	var buf bytes.Buffer
	oneByte := make([]byte, 1)
	for {
		_, err := r.Read(oneByte)
		if err != nil {
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
		}
		buf.Write(oneByte)
		if oneByte[0] == '\n' {
			break
		}
	}
	return buf.Bytes()
}

// parseResponse unmarshals a JSON-RPC response.
func parseResponse(t *testing.T, data []byte) JSONRPCResponse {
	t.Helper()
	var resp JSONRPCResponse
	require.NoError(t, json.Unmarshal(data, &resp))
	return resp
}

// runServerInBackground starts the server in a goroutine and returns a shutdown
// function that stops it. The server reads from stdinPipeR and writes to
// stdoutBuf/stderrBuf.
func runServerInBackground(t *testing.T) (stdin io.WriteCloser, stdoutBuf *bytes.Buffer, stderrBuf *bytes.Buffer, shutdown func()) {
	t.Helper()
	stdinR, stdinW := io.Pipe()
	stdoutBuf = &bytes.Buffer{}
	stderrBuf = &bytes.Buffer{}

	ctx, cancelFn := context.WithCancel(context.Background())
	server := NewServer(stdinR, stdoutBuf, stderrBuf)

	done := make(chan struct{})
	go func() {
		_ = server.Run(ctx)
		close(done)
	}()

	return stdinW, stdoutBuf, stderrBuf, func() {
		cancelFn()
		_ = stdinW.Close()
		_ = stdinR.Close()
		// Wait for the server goroutine to exit
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Log("warning: server goroutine did not exit within timeout")
		}
	}
}

// waitForOutput waits up to the given duration for stdout to have content.
func waitForOutput(t *testing.T, buf *bytes.Buffer, timeout time.Duration) []byte {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if buf.Len() > 0 {
			return buf.Bytes()
		}
		time.Sleep(5 * time.Millisecond)
	}
	return buf.Bytes()
}

// ---------------------------------------------------------------------------
// NewServer
// ---------------------------------------------------------------------------

func TestNewServer(t *testing.T) {
	var buf bytes.Buffer
	s := NewServer(&buf, &buf, &buf)
	assert.NotNil(t, s)
	assert.False(t, s.IsInitialized())
}

func TestRegisterHandler(t *testing.T) {
	// Create a server and register a custom handler, then verify it responds
	stdinR, stdinW := io.Pipe()
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}

	customHandler := handlers.NewInitializeHandler("custom-server", "2.0.0")
	server := NewServer(stdinR, stdoutBuf, stderrBuf)
	// Override the default initialize handler with our custom one
	server.RegisterHandler(customHandler)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = server.Run(ctx)
		close(done)
	}()

	// Send initialize request — should respond with our custom server info
	_, err := stdinW.Write([]byte(jsonRPCRequestLine(1, "initialize", nil)))
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)
	cancel()
	stdinW.Close() // unblock scanner.Scan()
	<-done

	assert.NotEmpty(t, stdoutBuf.String())
	lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
	if len(lines) > 0 {
		resp := parseResponse(t, []byte(lines[0]))
		assert.Nil(t, resp.Error)
	}

	stdinR.Close()
}

// ---------------------------------------------------------------------------
// Shutdown
// ---------------------------------------------------------------------------

func TestShutdown(t *testing.T) {
	var buf bytes.Buffer
	s := NewServer(&buf, &buf, &buf)
	err := s.Shutdown()
	assert.NoError(t, err)
}

func TestShutdown_StopsRun(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}

	server := NewServer(stdinR, stdoutBuf, stderrBuf)

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		_ = server.Run(ctx)
		close(done)
	}()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Shutdown closes the shutdown channel
	err := server.Shutdown()
	assert.NoError(t, err)

	// Close stdin to unblock scanner.Scan() so Run can check the shutdown channel
	stdinW.Close()

	// Run should return shortly after shutdown + stdin close
	select {
	case <-done:
		// OK — Run returned
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}

	stdinR.Close()
}

// ---------------------------------------------------------------------------
// mustMarshal
// ---------------------------------------------------------------------------

func TestMustMarshal(t *testing.T) {
	data := mustMarshal(map[string]string{"key": "value"})
	assert.JSONEq(t, `{"key":"value"}`, string(data))

	// nil: json.Marshal(nil) returns "null", which is valid — used as-is
	data2 := mustMarshal(nil)
	assert.JSONEq(t, `null`, string(data2))

	// channel (not marshalable) should produce fallback "{}"
	data3 := mustMarshal(make(chan int))
	assert.JSONEq(t, `{}`, string(data3))
}

// ---------------------------------------------------------------------------
// JSON-RPC request lifecycle
// ---------------------------------------------------------------------------

func TestServer_InitializeRequest(t *testing.T) {
	stdin, stdoutBuf, stderrBuf, cancel := runServerInBackground(t)
	defer cancel()

	// Send initialize request
	line := jsonRPCRequestLine(1, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
	})
	_, err := stdin.Write([]byte(line))
	require.NoError(t, err)

	// Wait for response
	raw := waitForOutput(t, stdoutBuf, 2*time.Second)
	require.NotEmpty(t, raw, "expected response from initialize")

	resp := parseResponse(t, raw)
	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Equal(t, float64(1), resp.ID)
	assert.Nil(t, resp.Error)
	require.NotNil(t, resp.Result)

	// Verify result structure
	var initResult map[string]interface{}
	err = json.Unmarshal(resp.Result, &initResult)
	require.NoError(t, err)
	assert.Equal(t, "2024-11-05", initResult["protocolVersion"])
	si := initResult["serverInfo"].(map[string]interface{})
	assert.Equal(t, "openapi-cli-mcp", si["name"])
	assert.Equal(t, "1.0.0", si["version"])

	// stderr should be empty
	assert.Empty(t, stderrBuf.String())
}

func TestServer_InitializeRequest_ThenInitializedNotification(t *testing.T) {
	stdin, stdoutBuf, _, cancel := runServerInBackground(t)
	defer cancel()

	// Step 1: initialize
	line1 := jsonRPCRequestLine(1, "initialize", nil)
	_, err := stdin.Write([]byte(line1))
	require.NoError(t, err)

	raw := waitForOutput(t, stdoutBuf, 2*time.Second)
	require.NotEmpty(t, raw)

	resp := parseResponse(t, raw)
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)

	// Step 2: send initialized notification (no response expected)
	line2 := jsonRPCRequestLine(2, "notifications/initialized", nil)
	_, err = stdin.Write([]byte(line2))
	require.NoError(t, err)

	// The initialized notification handler returns (nil, nil) so it produces a
	// response with result:null. Wait for it to arrive.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
		if len(lines) >= 2 {
			lastResp := parseResponse(t, []byte(lines[len(lines)-1]))
			assert.Nil(t, lastResp.Error, "notification should not produce error response")
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for second response line")
}

func TestServer_MethodNotFound(t *testing.T) {
	stdin, stdoutBuf, _, cancel := runServerInBackground(t)
	defer cancel()

	line := jsonRPCRequestLine(1, "unknown/method", nil)
	_, err := stdin.Write([]byte(line))
	require.NoError(t, err)

	raw := waitForOutput(t, stdoutBuf, 2*time.Second)
	require.NotEmpty(t, raw)

	resp := parseResponse(t, raw)
	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Equal(t, float64(1), resp.ID)
	require.NotNil(t, resp.Error)
	assert.Equal(t, -32601, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "Method not found")
}

func TestServer_ParseError(t *testing.T) {
	stdin, stdoutBuf, _, cancel := runServerInBackground(t)
	defer cancel()

	// Send invalid JSON
	_, err := stdin.Write([]byte("{invalid json}\n"))
	require.NoError(t, err)

	raw := waitForOutput(t, stdoutBuf, 2*time.Second)
	require.NotEmpty(t, raw)

	resp := parseResponse(t, raw)
	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Nil(t, resp.ID) // nil ID on parse error
	require.NotNil(t, resp.Error)
	assert.Equal(t, -32700, resp.Error.Code)
	assert.Equal(t, "Parse error", resp.Error.Message)
}

func TestServer_EmptyLineIgnored(t *testing.T) {
	stdin, stdoutBuf, _, cancel := runServerInBackground(t)
	defer cancel()

	// Empty lines (truly empty, just newline) should be ignored
	_, err := stdin.Write([]byte("\n"))
	require.NoError(t, err)

	// Send a valid request after empty line
	line := jsonRPCRequestLine(1, "initialize", nil)
	_, err = stdin.Write([]byte(line))
	require.NoError(t, err)

	raw := waitForOutput(t, stdoutBuf, 2*time.Second)
	require.NotEmpty(t, raw)
	resp := parseResponse(t, raw)
	assert.Nil(t, resp.Error)
}

func TestServer_MultipleRequests(t *testing.T) {
	stdin, stdoutBuf, _, cancel := runServerInBackground(t)
	defer cancel()

	// Send initialize
	_, err := stdin.Write([]byte(jsonRPCRequestLine(1, "initialize", nil)))
	require.NoError(t, err)

	raw1 := waitForOutput(t, stdoutBuf, 2*time.Second)
	require.NotEmpty(t, raw1)
	resp1 := parseResponse(t, raw1)
	assert.Nil(t, resp1.Error)

	// Send unknown method
	_, err = stdin.Write([]byte(jsonRPCRequestLine(2, "unknown/method", nil)))
	require.NoError(t, err)

	// Wait until we have 2+ lines in the buffer
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
		if len(lines) >= 2 {
			lastResp := parseResponse(t, []byte(lines[len(lines)-1]))
			require.NotNil(t, lastResp.Error)
			assert.Equal(t, -32601, lastResp.Error.Code)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for second response")
}

// ---------------------------------------------------------------------------
// Tools/list
// ---------------------------------------------------------------------------

func TestServer_ToolsList(t *testing.T) {
	stdin, stdoutBuf, stderrBuf, cancel := runServerInBackground(t)
	defer cancel()

	// Initialize first (required by some MCP implementations)
	_, err := stdin.Write([]byte(jsonRPCRequestLine(1, "initialize", nil)))
	require.NoError(t, err)
	raw := waitForOutput(t, stdoutBuf, 2*time.Second)
	require.NotEmpty(t, raw)

	// tools/list is registered only if we register it. The default server
	// only has initialize and notifications/initialized. Let's test that
	// tools/list returns method-not-found by default.
	_, err = stdin.Write([]byte(jsonRPCRequestLine(2, "tools/list", nil)))
	require.NoError(t, err)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
		if len(lines) >= 2 {
			lastResp := parseResponse(t, []byte(lines[len(lines)-1]))
			require.NotNil(t, lastResp.Error)
			assert.Equal(t, -32601, lastResp.Error.Code)
			assert.Empty(t, stderrBuf.String())
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for second response")
}

// ---------------------------------------------------------------------------
// Concurrency: multiple requests handled
// ---------------------------------------------------------------------------

func TestServer_ConcurrentRequests(t *testing.T) {
	stdin, stdoutBuf, _, cancel := runServerInBackground(t)
	defer cancel()

	// Fire several requests rapidly
	for i := 0; i < 5; i++ {
		_, err := stdin.Write([]byte(jsonRPCRequestLine(i, "initialize", nil)))
		require.NoError(t, err)
	}

	// Wait for responses
	time.Sleep(500 * time.Millisecond)

	lines := strings.Split(strings.TrimSpace(stdoutBuf.String()), "\n")
	assert.Len(t, lines, 5, "expected 5 response lines for 5 requests")
	for _, line := range lines {
		if line == "" {
			continue
		}
		resp := parseResponse(t, []byte(line))
		assert.Nil(t, resp.Error, "unexpected error in response: %s", resp.Error)
	}
}

// ---------------------------------------------------------------------------
// Edge: context cancellation stops Run
// ---------------------------------------------------------------------------

func TestServer_ContextCancellation(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}

	server := NewServer(stdinR, stdoutBuf, stderrBuf)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		// Run should return when stdin is closed (EOF), not necessarily
		// with context.Canceled, because scanner.Scan() returns false on EOF
		// before the context is checked.
		_ = server.Run(ctx)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	stdinW.Close() // unblock scanner.Scan() via EOF

	select {
	case <-done:
		// Run returned after stdin close
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after stdin close")
	}

	stdinR.Close()
}

// ---------------------------------------------------------------------------
// Edge: scanner error propagation
// ---------------------------------------------------------------------------

func TestServer_ScannerError(t *testing.T) {
	// Create a reader that will return an error
	pipeR, pipeW := io.Pipe()
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}

	server := NewServer(pipeR, stdoutBuf, stderrBuf)

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		err := server.Run(ctx)
		// Scanner error with pipe: when we close the write end, scanner gets EOF
		// which is not an error
		assert.NoError(t, err)
		close(done)
	}()

	// Write something then close
	_, _ = pipeW.Write([]byte(jsonRPCRequestLine(1, "initialize", nil)))
	pipeW.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}

	// Should have received a response
	assert.NotEmpty(t, stdoutBuf.String())
}

// ---------------------------------------------------------------------------
// IsInitialized
// ---------------------------------------------------------------------------

func TestServer_IsInitialized_InitiallyFalse(t *testing.T) {
	var buf bytes.Buffer
	s := NewServer(&buf, &buf, &buf)
	assert.False(t, s.IsInitialized())
}

// ---------------------------------------------------------------------------
// Edge: Write response with flush
// ---------------------------------------------------------------------------

func TestServer_WriteResponse_Flush(t *testing.T) {
	var buf bytes.Buffer
	s := NewServer(&buf, &buf, &buf)

	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  mustMarshal(map[string]string{"ok": "true"}),
	}
	s.writeResponse(resp)

	// Verify output
	output := buf.String()
	assert.Contains(t, output, `"jsonrpc":"2.0"`)
	assert.Contains(t, output, `"result"`)
}

// ---------------------------------------------------------------------------
// Handle request with nil params
// ---------------------------------------------------------------------------

func TestServer_HandleRequest_NilParams(t *testing.T) {
	// Create a minimal server and test handleRequest directly
	// by sending a request with nil params
	stdinR, stdinW := io.Pipe()
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}

	server := NewServer(stdinR, stdoutBuf, stderrBuf)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = server.Run(ctx)
		close(done)
	}()

	// Send initialize with nil params (the handler accepts nil params)
	_, err := stdinW.Write([]byte(jsonRPCRequestLine(1, "initialize", nil)))
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)
	cancel()
	stdinW.Close()
	<-done

	assert.NotEmpty(t, stdoutBuf.String())
	stdinR.Close()
}
