package mcp

import (
	"encoding/json"
	"testing"
)

func TestJSONRPCRequest(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":1,"method":"test","params":{"foo":"bar"}}`)
	var req JSONRPCRequest
	err := json.Unmarshal(data, &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if req.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", req.JSONRPC)
	}
	if req.ID != 1.0 {
		t.Errorf("expected id 1, got %v", req.ID)
	}
	if req.Method != "test" {
		t.Errorf("expected method test, got %s", req.Method)
	}
	if string(req.Params) != `{"foo":"bar"}` {
		t.Errorf("expected params foo:bar, got %s", string(req.Params))
	}
}

func TestJSONRPCResponse(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":1,"result":{"status":"ok"}}`)
	var resp JSONRPCResponse
	err := json.Unmarshal(data, &resp)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", resp.JSONRPC)
	}
	if resp.ID != 1.0 {
		t.Errorf("expected id 1, got %v", resp.ID)
	}
	if string(resp.Result) != `{"status":"ok"}` {
		t.Errorf("expected result status:ok, got %s", string(resp.Result))
	}
}

func TestJSONRPCResponseWithError(t *testing.T) {
	data := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"Invalid request"}}`)
	var resp JSONRPCResponse
	err := json.Unmarshal(data, &resp)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("expected error code -32600, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Invalid request" {
		t.Errorf("expected error message 'Invalid request', got %s", resp.Error.Message)
	}
}

func TestInitializeRequest(t *testing.T) {
	data := []byte(`{
		"protocolVersion":"2024-11-05",
		"capabilities":{},
		"clientInfo":{"name":"test","version":"1.0.0"}
	}`)
	var req InitializeRequest
	err := json.Unmarshal(data, &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if req.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version 2024-11-05, got %s", req.ProtocolVersion)
	}
	if req.ClientInfo.Name != "test" {
		t.Errorf("expected client name test, got %s", req.ClientInfo.Name)
	}
}

func TestInitializeResult(t *testing.T) {
	data := []byte(`{
		"protocolVersion":"2024-11-05",
		"capabilities":{"tools":{},"resources":{}},
		"serverInfo":{"name":"server","version":"1.0.0"}
	}`)
	var result InitializeResult
	err := json.Unmarshal(data, &result)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version 2024-11-05, got %s", result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "server" {
		t.Errorf("expected server name server, got %s", result.ServerInfo.Name)
	}
}

func TestListToolsRequest(t *testing.T) {
	jsonStr := `{"cursor":"test-cursor"}`
	var req ListToolsRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if req.Cursor == nil {
		t.Fatal("expected cursor, got nil")
	}
	if *req.Cursor != "test-cursor" {
		t.Errorf("expected cursor test-cursor, got %s", *req.Cursor)
	}
}

func TestListToolsRequestEmpty(t *testing.T) {
	jsonStr := `{}`
	var req ListToolsRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if req.Cursor != nil {
		t.Error("expected nil cursor, got non-nil")
	}
}

func TestListToolsResult(t *testing.T) {
	result := ListToolsResult{
		Tools: []Tool{
			{
				Name:        "test-tool",
				Description: "A test tool",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
		NextCursor: "next-page",
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var unmarshaled ListToolsResult
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(unmarshaled.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(unmarshaled.Tools))
	}
	if unmarshaled.Tools[0].Name != "test-tool" {
		t.Errorf("expected tool name test-tool, got %s", unmarshaled.Tools[0].Name)
	}
	if unmarshaled.NextCursor != "next-page" {
		t.Errorf("expected next cursor next-page, got %s", unmarshaled.NextCursor)
	}
}

func TestCallToolRequest(t *testing.T) {
	jsonStr := `{"name":"test-tool","arguments":{"param1":"value1"}}`
	var req CallToolRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if req.Name != "test-tool" {
		t.Errorf("expected name test-tool, got %s", req.Name)
	}
	if req.Arguments["param1"] != "value1" {
		t.Errorf("expected param1 value1, got %v", req.Arguments["param1"])
	}
}

func TestCallToolResult(t *testing.T) {
	result := CallToolResult{
		Content: []ToolContent{
			{Type: "text", Text: "result"},
		},
		IsError: false,
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var unmarshaled CallToolResult
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(unmarshaled.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(unmarshaled.Content))
	}
	if unmarshaled.Content[0].Type != "text" {
		t.Errorf("expected type text, got %s", unmarshaled.Content[0].Type)
	}
	if unmarshaled.IsError {
		t.Error("expected no error, got isError true")
	}
}

func TestListResourcesRequest(t *testing.T) {
	jsonStr := `{"cursor":"resource-cursor"}`
	var req ListResourcesRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if req.Cursor == nil {
		t.Fatal("expected cursor, got nil")
	}
	if *req.Cursor != "resource-cursor" {
		t.Errorf("expected cursor resource-cursor, got %s", *req.Cursor)
	}
}

func TestListResourcesResult(t *testing.T) {
	result := ListResourcesResult{
		Resources: []Resource{
			{
				URI:         "file:///test",
				Name:        "Test Resource",
				Description: "A test resource",
				MimeType:    "text/plain",
			},
		},
		NextCursor: "next-resources",
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var unmarshaled ListResourcesResult
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(unmarshaled.Resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(unmarshaled.Resources))
	}
	if unmarshaled.Resources[0].URI != "file:///test" {
		t.Errorf("expected uri file:///test, got %s", unmarshaled.Resources[0].URI)
	}
}

func TestReadResourceRequest(t *testing.T) {
	jsonStr := `{"uri":"file:///path/to/file"}`
	var req ReadResourceRequest
	err := json.Unmarshal([]byte(jsonStr), &req)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if req.URI != "file:///path/to/file" {
		t.Errorf("expected uri file:///path/to/file, got %s", req.URI)
	}
}

func TestReadResourceResult(t *testing.T) {
	result := ReadResourceResult{
		Contents: []ResourceContents{
			{
				URI:      "file:///test",
				MimeType: "text/plain",
				Text:     "Hello, World!",
			},
		},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var unmarshaled ReadResourceResult
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(unmarshaled.Contents) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(unmarshaled.Contents))
	}
	if unmarshaled.Contents[0].Text != "Hello, World!" {
		t.Errorf("expected text 'Hello, World!', got %s", unmarshaled.Contents[0].Text)
	}
}

func TestErrorCodes(t *testing.T) {
	tests := []struct {
		code        ErrorCodes
		expectedInt int
	}{
		{CodeParseError, -32700},
		{CodeInvalidRequest, -32600},
		{CodeMethodNotFound, -32601},
		{CodeInvalidParams, -32602},
		{CodeInternalError, -32603},
		{CodeServerError, -32000},
	}
	for _, tt := range tests {
		if int(tt.code) != tt.expectedInt {
			t.Errorf("expected %d, got %d", tt.expectedInt, int(tt.code))
		}
	}
}

func TestImplementation(t *testing.T) {
	impl := Implementation{
		Name:    "test",
		Version: "1.0.0",
	}
	data, err := json.Marshal(impl)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var unmarshaled Implementation
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if unmarshaled.Name != "test" {
		t.Errorf("expected name test, got %s", unmarshaled.Name)
	}
	if unmarshaled.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", unmarshaled.Version)
	}
}
