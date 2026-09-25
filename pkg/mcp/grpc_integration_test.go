package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	apperrors "github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/protocol"
)

// Type aliases keeping the test file's references short.
type (
	handlersCommand         = handlers.Command
	handlersCommandRegistry = handlers.CommandRegistry
	handlersToolLister      = handlers.ToolLister
)

// fakeGRPCExecutor records calls and returns canned results.
type fakeGRPCExecutor struct {
	lastOpID string
	lastArgs map[string]interface{}
	result   interface{}
	err      error
}

func (f *fakeGRPCExecutor) Execute(_ context.Context, opID string, args map[string]interface{}) (interface{}, error) {
	f.lastOpID = opID
	f.lastArgs = args
	return f.result, f.err
}

// sampleGRPCOps builds a neutral operation set like the gRPC adapter emits.
func sampleGRPCOps() []*protocol.Operation {
	return []*protocol.Operation{
		{
			ID:       "kv.KeyValue-Get",
			Protocol: protocol.ProtocolGRPC,
			Kind:     protocol.KindUnary,
			Summary:  "Invoke kv.KeyValue-Get",
			RequestBody: []protocol.RequestBodyProperty{
				{Name: "key", Type: "string", Required: true},
			},
			Source: map[string]string{"full_method": "/kv.KeyValue/Get"},
		},
		{
			ID:       "kv.KeyValue-Set",
			Protocol: protocol.ProtocolGRPC,
			Kind:     protocol.KindUnary,
			RequestBody: []protocol.RequestBodyProperty{
				{Name: "key", Type: "string", Required: true},
				{Name: "value", Type: "string"},
			},
			Source: map[string]string{"full_method": "/kv.KeyValue/Set"},
		},
		{
			ID:       "kv.KeyValue-Watch",
			Protocol: protocol.ProtocolGRPC,
			Kind:     protocol.KindServerStream,
			RequestBody: []protocol.RequestBodyProperty{
				{Name: "prefix", Type: "string"},
			},
			Source: map[string]string{"full_method": "/kv.KeyValue/Watch"},
		},
	}
}

// TestGRPCRegistry_ImplementsCommandRegistry pins the handlers contract.
func TestGRPCRegistry_ImplementsCommandRegistry(t *testing.T) {
	var _ handlersCommandRegistry = (*GRPCRegistry)(nil)
	var _ handlersToolLister = (*GRPCRegistry)(nil)
}

// TestGRPCRegistry_ListTools: tools derive from the neutral model with
// input schemas from message fields.
func TestGRPCRegistry_ListTools(t *testing.T) {
	exec := &fakeGRPCExecutor{}
	r, err := NewGRPCRegistry(sampleGRPCOps(), exec)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	tools := r.ListTools()
	if len(tools) != 3 {
		t.Fatalf("tools = %d, want 3", len(tools))
	}
	// Deterministic order.
	if tools[0].Name != "kv.KeyValue-Get" || tools[1].Name != "kv.KeyValue-Set" || tools[2].Name != "kv.KeyValue-Watch" {
		t.Fatalf("tool order = %s,%s,%s", tools[0].Name, tools[1].Name, tools[2].Name)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(tools[0].InputSchema, &schema); err != nil {
		t.Fatalf("Get schema not JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Fatalf("Get schema type = %v", schema["type"])
	}
	props, _ := schema["properties"].(map[string]interface{})
	if _, ok := props["key"]; !ok {
		t.Fatalf("Get schema missing key property: %v", props)
	}
	req, _ := schema["required"].([]interface{})
	if len(req) != 1 || req[0] != "key" {
		t.Fatalf("Get schema required = %v, want [key]", req)
	}
}

// TestGRPCRegistry_Execute_DelegatesToAdapter: tool call → executor with the
// opID and args intact.
func TestGRPCRegistry_Execute_DelegatesToAdapter(t *testing.T) {
	exec := &fakeGRPCExecutor{result: map[string]interface{}{"ok": true}}
	r, err := NewGRPCRegistry(sampleGRPCOps(), exec)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	res, err := r.Execute(context.Background(), "kv.KeyValue-Set", map[string]interface{}{
		"key": "k", "value": "v",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if exec.lastOpID != "kv.KeyValue-Set" {
		t.Fatalf("executor got opID %q", exec.lastOpID)
	}
	if exec.lastArgs["key"] != "k" || exec.lastArgs["value"] != "v" {
		t.Fatalf("executor got args %v", exec.lastArgs)
	}
	m, _ := res.(map[string]interface{})
	if m["ok"] != true {
		t.Fatalf("result = %#v", res)
	}
}

// TestGRPCRegistry_Execute_RequiredValidation: missing required arg fails
// client-side with CodeValidation (§4.2 — before any bytes hit the wire).
func TestGRPCRegistry_Execute_RequiredValidation(t *testing.T) {
	exec := &fakeGRPCExecutor{}
	r, err := NewGRPCRegistry(sampleGRPCOps(), exec)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	_, err = r.Execute(context.Background(), "kv.KeyValue-Get", map[string]interface{}{})
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Fatalf("missing-required error = %v, want CodeValidation", err)
	}
	if exec.lastOpID != "" {
		t.Fatal("executor must not be called when validation fails")
	}
}

// TestGRPCRegistry_Execute_UnknownTool: unknown tool → CodeNotFound.
func TestGRPCRegistry_Execute_UnknownTool(t *testing.T) {
	exec := &fakeGRPCExecutor{}
	r, err := NewGRPCRegistry(sampleGRPCOps(), exec)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	_, err = r.Execute(context.Background(), "kv.Nope-Missing", nil)
	if !apperrors.IsError(err, apperrors.CodeNotFound) {
		t.Fatalf("unknown tool error = %v, want CodeNotFound", err)
	}
}

// TestGRPCRegistry_StreamingToolListed: streaming ops are LISTED (mapped at
// discovery) — the rejection happens at the adapter's Execute (PROTO-003).
// The registry must not filter them; the adapter owns that boundary.
func TestGRPCRegistry_StreamingToolListed(t *testing.T) {
	exec := &fakeGRPCExecutor{}
	r, err := NewGRPCRegistry(sampleGRPCOps(), exec)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	found := false
	for _, tool := range r.ListTools() {
		if tool.Name == "kv.KeyValue-Watch" {
			found = true
		}
	}
	if !found {
		t.Fatal("streaming op missing from tool list — discovery must map it (rejection is Execute's job)")
	}
}

// TestGRPCRegistry_DuplicateIDs: duplicate operation IDs are rejected.
func TestGRPCRegistry_DuplicateIDs(t *testing.T) {
	ops := sampleGRPCOps()
	ops = append(ops, ops[0])
	_, err := NewGRPCRegistry(ops, &fakeGRPCExecutor{})
	if err == nil {
		t.Fatal("duplicate IDs must error")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate error = %v", err)
	}
}

// TestGRPCRegistry_ReadOnly: mutation verbs refuse (matches OpenAPI registry).
func TestGRPCRegistry_ReadOnly(t *testing.T) {
	r, err := NewGRPCRegistry(sampleGRPCOps(), &fakeGRPCExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.AddCommand(handlersCommand{}); err == nil {
		t.Fatal("AddCommand must refuse")
	}
	if err := r.RemoveCommand("x"); err == nil {
		t.Fatal("RemoveCommand must refuse")
	}
	if err := r.UpdateCommand("x", handlersCommand{}); err == nil {
		t.Fatal("UpdateCommand must refuse")
	}
}

// TestGRPCRegistry_NilExecutor: nil executor is a validation error.
func TestGRPCRegistry_NilExecutor(t *testing.T) {
	if _, err := NewGRPCRegistry(sampleGRPCOps(), nil); err == nil {
		t.Fatal("nil executor must error")
	}
}

// TestGRPCRegistry_ListCommands: command view carries flags from schema.
func TestGRPCRegistry_ListCommands(t *testing.T) {
	r, err := NewGRPCRegistry(sampleGRPCOps(), &fakeGRPCExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	cmds := r.ListCommands()
	if len(cmds) != 3 {
		t.Fatalf("commands = %d, want 3", len(cmds))
	}
	var getCmd *handlersCommand
	for i := range cmds {
		if cmds[i].Name == "kv.KeyValue-Get" {
			getCmd = &cmds[i]
		}
	}
	if getCmd == nil {
		t.Fatal("Get command missing")
	}
	if len(getCmd.Flags) != 1 || getCmd.Flags[0].Name != "key" {
		t.Fatalf("Get flags = %+v", getCmd.Flags)
	}
}
