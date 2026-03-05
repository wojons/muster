package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	server := NewServer(stdin, stdout, stderr)
	if server == nil {
		t.Fatal("expected server, got nil")
	}

	if server.IsInitialized() {
		t.Error("expected server not initialized")
	}
}

func TestServerRun(t *testing.T) {
	stdin := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	server := NewServer(stdin, stdout, stderr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	err := <-errChan
	if err != nil && err != context.Canceled {
		t.Errorf("expected no error or context canceled, got %v", err)
	}
}

func TestServerHandleInitialize(t *testing.T) {
	stdin := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}` + "\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	server := NewServer(stdin, stdout, stderr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	output := stdout.String()
	if output == "" {
		t.Fatal("expected output, got empty string")
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("expected no error, got %v", resp.Error)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if result.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version 2024-11-05, got %s", result.ProtocolVersion)
	}
}

func TestServerHandleUnknownMethod(t *testing.T) {
	stdin := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"unknown.method","params":{}}` + "\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	server := NewServer(stdin, stdout, stderr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	output := stdout.String()
	if output == "" {
		t.Fatal("expected output, got empty string")
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error")
	}

	if resp.Error.Code != -32601 {
		t.Errorf("expected error code -32601, got %d", resp.Error.Code)
	}

	if resp.Error.Message != "Method not found" {
		t.Errorf("expected message 'Method not found', got %s", resp.Error.Message)
	}
}

func TestServerHandleInvalidJSON(t *testing.T) {
	stdin := strings.NewReader(`{invalid json}` + "\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	server := NewServer(stdin, stdout, stderr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	output := stdout.String()
	if output == "" {
		t.Fatal("expected output, got empty string")
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error == nil {
		t.Fatal("expected error")
	}

	if resp.Error.Code != -32700 {
		t.Errorf("expected error code -32700, got %d", resp.Error.Code)
	}

	if resp.Error.Message != "Parse error" {
		t.Errorf("expected message 'Parse error', got %s", resp.Error.Message)
	}
}

func TestServerHandleNotifications(t *testing.T) {
	stdin := strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}` + "\n")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	server := NewServer(stdin, stdout, stderr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = server.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	output := stdout.String()
	if output == "" {
		t.Fatal("expected output, got empty string")
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("expected no error, got %v", resp.Error)
	}
}

func TestServerShutdown(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	server := NewServer(stdin, stdout, stderr)

	err := server.Shutdown()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestMustMarshal(t *testing.T) {
	result := mustMarshal(map[string]string{"test": "value"})
	if result == nil {
		t.Fatal("expected marshaled data, got nil")
	}

	var unmarshaled map[string]string
	if err := json.Unmarshal(result, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled["test"] != "value" {
		t.Errorf("expected test value, got %s", unmarshaled["test"])
	}
}

func TestIsInitialized(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	server := NewServer(stdin, stdout, stderr)

	if server.IsInitialized() {
		t.Error("expected server not initialized")
	}
}
