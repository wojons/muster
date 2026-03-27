package handlers

import (
	"context"
	"encoding/json"
	"testing"
)

func TestInitializeHandler_Method(t *testing.T) {
	handler := NewInitializeHandler("test-server", "1.0.0")
	method := handler.Method()
	if method != "initialize" {
		t.Errorf("expected method 'initialize', got '%s'", method)
	}
}

func TestInitializeHandler_Handle_ValidRequest(t *testing.T) {
	handler := NewInitializeHandler("muster-mcp", "2.0.0")

	req := InitializeRequest{
		ProtocolVersion: "2024-11-05",
		Capabilities: ClientCapabilities{
			Roots: &RootsCapability{},
		},
		ClientInfo: Implementation{
			Name:    "test-client",
			Version: "1.0.0",
		},
	}
	params, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var initResult InitializeResult
	switch v := result.(type) {
	case InitializeResult:
		initResult = v
	case *InitializeResult:
		initResult = *v
	default:
		t.Fatalf("expected InitializeResult or *InitializeResult, got %T", result)
	}

	if initResult.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version '2024-11-05', got '%s'", initResult.ProtocolVersion)
	}

	if initResult.ServerInfo.Name != "muster-mcp" {
		t.Errorf("expected server name 'muster-mcp', got '%s'", initResult.ServerInfo.Name)
	}

	if initResult.ServerInfo.Version != "2.0.0" {
		t.Errorf("expected server version '2.0.0', got '%s'", initResult.ServerInfo.Version)
	}

	if initResult.Capabilities.Tools == nil {
		t.Error("expected tools capability to be non-nil")
	}

	if initResult.Capabilities.Resources == nil {
		t.Error("expected resources capability to be non-nil")
	}

	if initResult.Capabilities.Prompts == nil {
		t.Error("expected prompts capability to be non-nil")
	}
}

func TestInitializeHandler_Handle_InvalidJSON(t *testing.T) {
	handler := NewInitializeHandler("test-server", "1.0.0")

	ctx := context.Background()
	_, err := handler.Handle(ctx, json.RawMessage("invalid json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	mcpErr, ok := err.(*MCPError)
	if !ok {
		t.Fatal("expected MCPError")
	}

	if mcpErr.Code != -32602 {
		t.Errorf("expected error code -32602, got %d", mcpErr.Code)
	}

	if mcpErr.Message != "Invalid params" {
		t.Errorf("expected error message 'Invalid params', got '%s'", mcpErr.Message)
	}
}

func TestInitializeHandler_Handle_MinimalRequest(t *testing.T) {
	handler := NewInitializeHandler("minimal-server", "0.1.0")

	params := json.RawMessage(`{}`)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var initResult InitializeResult
	switch v := result.(type) {
	case InitializeResult:
		initResult = v
	case *InitializeResult:
		initResult = *v
	default:
		t.Fatalf("expected InitializeResult or *InitializeResult, got %T", result)
	}

	if initResult.ServerInfo.Name != "minimal-server" {
		t.Errorf("expected server name 'minimal-server', got '%s'", initResult.ServerInfo.Name)
	}
}

func TestInitializeHandler_Handle_WithCapabilities(t *testing.T) {
	handler := NewInitializeHandler("full-server", "3.0.0")

	req := InitializeRequest{
		ProtocolVersion: "2024-11-05",
		Capabilities:    ClientCapabilities{},
		ClientInfo: Implementation{
			Name:    "advanced-client",
			Version: "2.5.0",
		},
	}
	params, _ := json.Marshal(req)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var initResult InitializeResult
	switch v := result.(type) {
	case InitializeResult:
		initResult = v
	case *InitializeResult:
		initResult = *v
	default:
		t.Fatalf("expected InitializeResult or *InitializeResult, got %T", result)
	}

	if initResult.Capabilities.Tools == nil {
		t.Error("expected tools capability")
	}
	if initResult.Capabilities.Resources == nil {
		t.Error("expected resources capability")
	}
	if initResult.Capabilities.Prompts == nil {
		t.Error("expected prompts capability")
	}
}

func TestInitializedHandler_Method(t *testing.T) {
	handler := NewInitializedHandler()
	method := handler.Method()
	if method != "notifications/initialized" {
		t.Errorf("expected method 'notifications/initialized', got '%s'", method)
	}
}

func TestInitializedHandler_Handle(t *testing.T) {
	handler := NewInitializedHandler()

	ctx := context.Background()
	result, err := handler.Handle(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestInitializedHandler_Handle_WithParams(t *testing.T) {
	handler := NewInitializedHandler()

	params := json.RawMessage(`{"someField": "value"}`)

	ctx := context.Background()
	result, err := handler.Handle(ctx, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
}

func TestInitializeResult_MarshalJSON(t *testing.T) {
	result := &InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: InitializeCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
			Prompts:   &PromptsCapability{},
		},
		ServerInfo: ServerInfo{
			Name:    "test-server",
			Version: "1.0.0",
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled InitializeResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ProtocolVersion != result.ProtocolVersion {
		t.Errorf("protocol version mismatch: expected %s, got %s", result.ProtocolVersion, unmarshaled.ProtocolVersion)
	}
	if unmarshaled.ServerInfo.Name != result.ServerInfo.Name {
		t.Errorf("server name mismatch: expected %s, got %s", result.ServerInfo.Name, unmarshaled.ServerInfo.Name)
	}
}

func TestInitializeRequest_UnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"protocolVersion": "2024-11-05",
		"capabilities": {},
		"clientInfo": {
			"name": "test-client",
			"version": "1.0.0"
		}
	}`

	var req InitializeRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version '2024-11-05', got '%s'", req.ProtocolVersion)
	}
	if req.ClientInfo.Name != "test-client" {
		t.Errorf("expected client name 'test-client', got '%s'", req.ClientInfo.Name)
	}
	if req.ClientInfo.Version != "1.0.0" {
		t.Errorf("expected client version '1.0.0', got '%s'", req.ClientInfo.Version)
	}
}

func TestServerInfo_MarshalJSON(t *testing.T) {
	info := ServerInfo{
		Name:    "test-server",
		Version: "1.2.3",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	expected := `{"name":"test-server","version":"1.2.3"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestImplementation_MarshalJSON(t *testing.T) {
	impl := Implementation{
		Name:    "test-impl",
		Version: "0.0.1",
	}

	data, err := json.Marshal(impl)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	expected := `{"name":"test-impl","version":"0.0.1"}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestClientCapabilities_MarshalJSON(t *testing.T) {
	caps := ClientCapabilities{
		Roots: &RootsCapability{},
	}

	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	expected := `{"roots":{}}`
	if string(data) != expected {
		t.Errorf("expected %s, got %s", expected, string(data))
	}
}

func TestInitializeCapabilities_MarshalJSON(t *testing.T) {
	caps := InitializeCapabilities{
		Tools:     &ToolsCapability{},
		Resources: &ResourcesCapability{},
		Prompts:   &PromptsCapability{},
	}

	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled InitializeCapabilities
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Tools == nil {
		t.Error("expected tools capability")
	}
	if unmarshaled.Resources == nil {
		t.Error("expected resources capability")
	}
	if unmarshaled.Prompts == nil {
		t.Error("expected prompts capability")
	}
}

func TestNewInitializeHandler_CreatesCorrectInstance(t *testing.T) {
	handler := NewInitializeHandler("my-server", "10.20.30")
	if handler == nil {
		t.Fatal("expected handler instance, got nil")
	}
	if handler.serverName != "my-server" {
		t.Errorf("expected serverName 'my-server', got '%s'", handler.serverName)
	}
	if handler.serverVersion != "10.20.30" {
		t.Errorf("expected serverVersion '10.20.30', got '%s'", handler.serverVersion)
	}
}

func TestNewInitializedHandler_CreatesCorrectInstance(t *testing.T) {
	handler := NewInitializedHandler()
	if handler == nil {
		t.Fatal("expected handler instance, got nil")
	}
}
