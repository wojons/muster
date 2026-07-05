package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/wojons/muster/pkg/mcp/handlers"
)

func TestNewServer(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	s := NewServer(stdin, stdout, stderr)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.stdin != stdin {
		t.Error("stdin not set")
	}
	if s.stdout != stdout {
		t.Error("stdout not set")
	}
	if s.stderr != stderr {
		t.Error("stderr not set")
	}
	if s.handlers == nil {
		t.Fatal("handlers registry is nil")
	}
	if s.shutdownChan == nil {
		t.Fatal("shutdownChan is nil")
	}
	if s.concurrencySem == nil {
		t.Fatal("concurrencySem is nil")
	}
	// Verify initialize handler is registered
	if h := s.handlers.Get("initialize"); h == nil {
		t.Error("initialize handler not registered")
	}
	// Verify initialized handler is registered
	if h := s.handlers.Get("notifications/initialized"); h == nil {
		t.Error("notifications/initialized handler not registered")
	}
}

func TestServer_IsInitialized(t *testing.T) {
	s := NewServer(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	if s.IsInitialized() {
		t.Error("IsInitialized should return false by default")
	}
}

func TestServer_RegisterHandler(t *testing.T) {
	s := NewServer(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})

	mock := &mockHandler{method: "test/method"}
	s.RegisterHandler(mock)

	if h := s.handlers.Get("test/method"); h == nil {
		t.Error("handler should be registered after RegisterHandler")
	}
}

func TestServer_Shutdown(t *testing.T) {
	s := NewServer(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	err := s.Shutdown()
	if err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}
	// Verify channel was closed
	select {
	case <-s.shutdownChan:
		// Expected — channel closed
	default:
		t.Error("shutdownChan should be closed after Shutdown")
	}
}

func TestServer_Run_InitializeRequest(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(stdinR, stdoutW, stderr)

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Write an initialize request
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{}`),
	}
	data, _ := json.Marshal(req)
	stdinW.Write(data)
	stdinW.Write([]byte("\n"))

	// Close stdin to terminate scanner
	stdinW.Close()

	// Read response from stdout
	var resp JSONRPCResponse
	decoder := json.NewDecoder(stdoutR)
	if err := decoder.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want \"2.0\"", resp.JSONRPC)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}
	if resp.Result == nil {
		t.Error("result should not be nil for initialize")
	}

	// Wait for Run to finish
	select {
	case err := <-done:
		if err != nil && err != io.EOF && err.Error() != "context canceled" {
			t.Logf("Run returned: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestServer_Run_MethodNotFound(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(stdinR, stdoutW, stderr)

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "nonexistent/method",
	}
	data, _ := json.Marshal(req)
	stdinW.Write(data)
	stdinW.Write([]byte("\n"))
	stdinW.Close()

	var resp JSONRPCResponse
	decoder := json.NewDecoder(stdoutR)
	if err := decoder.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
	if resp.Error.Message != "Method not found" {
		t.Errorf("error message = %q, want %q", resp.Error.Message, "Method not found")
	}

	<-done
}

func TestServer_Run_ParseError(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(stdinR, stdoutW, stderr)

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Write malformed JSON
	stdinW.Write([]byte("not valid json\n"))
	stdinW.Close()

	var resp JSONRPCResponse
	decoder := json.NewDecoder(stdoutR)
	if err := decoder.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected parse error for malformed JSON")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("error code = %d, want -32700", resp.Error.Code)
	}

	<-done
}

func TestServer_Run_ContextCancel(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())

	s := NewServer(stdinR, stdoutW, stderr)

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Cancel context to trigger exit
	cancel()
	stdinW.Close()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Logf("Run returned: %v (expected context.Canceled)", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
	stdoutR.Close()
}

func TestServer_Run_Shutdown(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx := context.Background()

	s := NewServer(stdinR, stdoutW, stderr)

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Trigger shutdown
	s.Shutdown()
	stdinW.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Logf("Run returned: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after shutdown")
	}
	stdoutR.Close()
}

func TestServer_Run_EmptyLine(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(stdinR, stdoutW, stderr)

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	// Write some empty lines, then a valid request
	stdinW.Write([]byte("\n"))
	stdinW.Write([]byte("\n"))
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{}`),
	}
	data, _ := json.Marshal(req)
	stdinW.Write(data)
	stdinW.Write([]byte("\n"))
	stdinW.Close()

	// Read just the response to the valid request
	var resp JSONRPCResponse
	decoder := json.NewDecoder(stdoutR)
	if err := decoder.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error for initialize: %s", resp.Error.Message)
	}

	<-done
}

func TestServer_Run_ToolsListRegistered(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(stdinR, stdoutW, stderr)
	// Register tools/list handler
	s.RegisterHandler(handlers.NewListToolsHandler(&emptyRegistry{}))

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}
	data, _ := json.Marshal(req)
	stdinW.Write(data)
	stdinW.Write([]byte("\n"))
	stdinW.Close()

	var resp JSONRPCResponse
	decoder := json.NewDecoder(stdoutR)
	if err := decoder.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error for tools/list: code=%d msg=%s", resp.Error.Code, resp.Error.Message)
	}
	if resp.Result == nil {
		t.Error("result should not be nil for tools/list")
	}

	<-done
}

func TestServer_Run_MCPErrorFromHandler(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(stdinR, stdoutW, stderr)
	s.RegisterHandler(&mcpErrorHandler{})

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "test/mcp-error",
	}
	data, _ := json.Marshal(req)
	stdinW.Write(data)
	stdinW.Write([]byte("\n"))
	stdinW.Close()

	var resp JSONRPCResponse
	decoder := json.NewDecoder(stdoutR)
	if err := decoder.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected MCP error")
	}
	if resp.Error.Code != -32001 {
		t.Errorf("error code = %d, want -32001", resp.Error.Code)
	}

	<-done
}

func TestServer_Run_HandlerReturnsError(t *testing.T) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	stderr := &bytes.Buffer{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := NewServer(stdinR, stdoutW, stderr)
	s.RegisterHandler(&genericErrorHandler{})

	done := make(chan error, 1)
	go func() {
		done <- s.Run(ctx)
	}()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      4,
		Method:  "test/generic-error",
	}
	data, _ := json.Marshal(req)
	stdinW.Write(data)
	stdinW.Write([]byte("\n"))
	stdinW.Close()

	var resp JSONRPCResponse
	decoder := json.NewDecoder(stdoutR)
	if err := decoder.Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error from handler")
	}
	if resp.Error.Code != -32000 {
		t.Errorf("error code = %d, want -32000", resp.Error.Code)
	}

	<-done
}

func TestMustMarshal(t *testing.T) {
	t.Run("valid struct", func(t *testing.T) {
		result := mustMarshal(map[string]string{"key": "value"})
		if string(result) != `{"key":"value"}` {
			t.Errorf("mustMarshal = %s, want {\"key\":\"value\"}", string(result))
		}
	})

	t.Run("nil", func(t *testing.T) {
		result := mustMarshal(nil)
		if string(result) != "null" {
			t.Errorf("mustMarshal(nil) = %s, want null", string(result))
		}
	})
}

func TestJSONRPCResponseTypes(t *testing.T) {
	// Verify JSON-RPC 2.0 response serialization
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Error: &JSONRPCError{
			Code:    -32601,
			Message: "Method not found",
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal JSONRPCResponse: %v", err)
	}
	if !bytes.Contains(data, []byte(`"jsonrpc":"2.0"`)) {
		t.Error("response missing jsonrpc field")
	}
	if !bytes.Contains(data, []byte(`"code":-32601`)) {
		t.Error("response missing error code")
	}
}

func TestJSONRPCRequestTypes(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      nil,
		Method:  "notifications/initialized",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal notification: %v", err)
	}
	var back JSONRPCRequest
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if back.Method != "notifications/initialized" {
		t.Errorf("method = %q", back.Method)
	}
}

// Mock implementations

type mockHandler struct {
	method string
}

func (m *mockHandler) Method() string { return m.method }
func (m *mockHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return "ok", nil
}

type mcpErrorHandler struct{}

func (m *mcpErrorHandler) Method() string { return "test/mcp-error" }
func (m *mcpErrorHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return nil, &handlers.MCPError{Code: -32001, Message: "test MCP error"}
}

type genericErrorHandler struct{}

func (g *genericErrorHandler) Method() string { return "test/generic-error" }
func (g *genericErrorHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return nil, io.EOF
}

// emptyRegistry implements handlers.CommandRegistry with no commands
type emptyRegistry struct{}

func (e *emptyRegistry) ListCommands() []handlers.Command         { return nil }
func (e *emptyRegistry) GetCommand(name string) *handlers.Command { return nil }
func (e *emptyRegistry) ExecuteCommand(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	return nil, nil
}
func (e *emptyRegistry) AddCommand(cmd handlers.Command) error                 { return nil }
func (e *emptyRegistry) RemoveCommand(name string) error                       { return nil }
func (e *emptyRegistry) UpdateCommand(name string, cmd handlers.Command) error { return nil }
