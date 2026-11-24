package asyncapi

import (
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/wojons/muster/pkg/mcp/handlers"
	"github.com/wojons/muster/pkg/protocol"
)

// TestMCPRegistryToolListing: every discovered operation surfaces as a
// tool; consume tools describe the bounded-collect contract.
func TestMCPRegistryToolListing(t *testing.T) {
	b := startStub(t, 1)
	c, ops := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}),
		WithStreamLimits(EventLimits{MaxMessages: MCPHardCapMaxMessages, MaxDuration: MCPHardCapMaxDuration}))
	reg, err := NewAsyncAPIRegistry(ops, c)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	tools := reg.ListTools()
	if len(tools) != len(ops) {
		t.Fatalf("tool count %d != operation count %d", len(tools), len(ops))
	}
	byName := map[string]handlers.Tool{}
	for _, tool := range tools {
		byName[tool.Name] = tool
	}
	consume := byName["receive-orderEvents-OrderEvent"]
	if consume.Name == "" {
		t.Fatalf("consume tool missing (tools: %v)", tools)
	}
	if !strings.Contains(consume.Description, "bounded collect") {
		t.Fatalf("consume tool must state the bounded-collect contract: %q", consume.Description)
	}
	var schema map[string]interface{}
	if err := jsonUnmarshal(consume.InputSchema, &schema); err != nil {
		t.Fatalf("consume tool schema not JSON: %v", err)
	}
	props := schema["properties"].(map[string]interface{})
	if _, ok := props["max_messages"]; !ok {
		t.Fatalf("consume tool schema must expose max_messages: %v", schema)
	}
	if _, ok := props["timeout_seconds"]; !ok {
		t.Fatalf("consume tool schema must expose timeout_seconds: %v", schema)
	}
	// Secrets never reach tool schemas: the security scheme references are
	// metadata only.
	raw := string(consume.InputSchema)
	if strings.Contains(raw, "sekrit") || strings.Contains(raw, "password") || strings.Contains(raw, "saslUsername") {
		t.Fatalf("credential material in tool schema: %s", raw)
	}

	publish := byName["sendUserSignedUp_v1"]
	if publish.Name == "" {
		t.Fatalf("publish tool missing")
	}
	if strings.Contains(publish.Description, "bounded collect") {
		t.Fatalf("publish tool must not claim bounded collect: %q", publish.Description)
	}
}

// TestMCPRegistryBoundedCollectExecution: the registry's tools/call runs a
// real bounded collect against the in-process broker and returns the
// transcript (PROTO-007 §4.5(4) — never a live subscription behind the
// tool call).
func TestMCPRegistryBoundedCollectExecution(t *testing.T) {
	b := startStub(t, 1)
	b.CreateTopic("orders-a1")
	topic := "orders-a1"

	direct := seedWriter(b, topic)
	msgs := make([]kafka.Message, 0, 12)
	for i := 0; i < 12; i++ {
		msgs = append(msgs, kafka.Message{Value: []byte(`{"orderId":"o` + itoa(i) + `"}`)})
	}
	if err := direct.WriteMessages(t.Context(), msgs...); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	_ = direct.Close()

	// The MCP-facing executor instance is built with the MCP hard caps —
	// exactly what the mcp-server wiring would construct.
	c, ops := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}),
		WithStreamLimits(EventLimits{MaxMessages: MCPHardCapMaxMessages, MaxDuration: MCPHardCapMaxDuration}))
	reg, err := NewAsyncAPIRegistry(ops, c)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	result, err := reg.ExecuteCommand(t.Context(), "receive-orderEvents-OrderEvent", map[string]interface{}{
		"orderId":         "a1",
		"max_messages":    5,
		"timeout_seconds": 10,
	})
	if err != nil {
		t.Fatalf("bounded collect via MCP registry: %v", err)
	}
	tr, ok := result.(*EventTranscript)
	if !ok {
		t.Fatalf("expected transcript from tools/call, got %T", result)
	}
	if tr.Status != StatusTruncated || tr.Trailer["limit_reason"] != LimitReasonMessages {
		t.Fatalf("expected a truncation by max_messages: %+v", tr)
	}
	if len(tr.Items) != 5 {
		t.Fatalf("bounded collect collected %d, want 5", len(tr.Items))
	}

	// A consume attempt through the registry on the group-bearing op is
	// refused, not hung.
	if _, err := reg.ExecuteCommand(t.Context(), "receive-userSignedup-UserSignedUp", nil); err == nil {
		t.Fatalf("consumer-group refuse did not fire through the registry")
	}
}

// TestMCPRegistryPublishExecution: a send op executes through the same
// registry.
func TestMCPRegistryPublishExecution(t *testing.T) {
	b := startStub(t, 1)
	b.CreateTopic("users-signedup")
	c, ops := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))
	reg, err := NewAsyncAPIRegistry(ops, c)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	result, err := reg.ExecuteCommand(t.Context(), "sendUserSignedUp_v1", map[string]interface{}{"userId": "u"})
	if err != nil {
		t.Fatalf("publish via MCP registry: %v", err)
	}
	if _, ok := result.(map[string]interface{}); !ok {
		t.Fatalf("publish ack shape wrong: %T", result)
	}
}

// TestMCPRegistryRejectsDuplicatesAndNilExecutor.
func TestMCPRegistryRejectsDuplicatesAndNilExecutor(t *testing.T) {
	op := &protocol.Operation{ID: "op", Kind: protocol.KindUnary, Protocol: protocol.ProtocolAsyncAPI}
	if _, err := NewAsyncAPIRegistry([]*protocol.Operation{op}, nil); err == nil {
		t.Fatalf("nil executor must be refused")
	}
	if _, err := NewAsyncAPIRegistry([]*protocol.Operation{op}, NewClient()); err != nil {
		t.Fatalf("valid registry: %v", err)
	}
	dup := &protocol.Operation{ID: "op", Kind: protocol.KindUnary, Protocol: protocol.ProtocolAsyncAPI}
	if _, err := NewAsyncAPIRegistry([]*protocol.Operation{op, dup}, NewClient()); err == nil {
		t.Fatalf("duplicate operation IDs must be refused")
	}
}

// TestMCPHandlerRegistrationShape: the MCPHandlers/RegisterWithServer
// path builds list+call handlers over a fake server registrar (mirrors the
// graphql/jsonrpc wiring contract).
type fakeServer struct {
	handlers []handlers.Handler
}

func (f *fakeServer) RegisterHandler(h handlers.Handler) { f.handlers = append(f.handlers, h) }

func TestMCPHandlerRegistrationShape(t *testing.T) {
	b := startStub(t, 1)
	c, ops := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))
	fake := &fakeServer{}
	if err := RegisterWithServer(fake, ops, c); err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(fake.handlers) != 2 {
		t.Fatalf("expected list+call handlers registered, got %d", len(fake.handlers))
	}
}

// defaultBoundsSanity ties the MCP cap vocabulary to the package defaults.
func TestMCPDefaultsMatchSpec(t *testing.T) {
	if DefaultMaxMessages != 1000 || DefaultMaxDuration != 30*time.Second {
		t.Fatalf("PROTO-007 §4 defaults must be 1000 messages / 30 seconds: %d / %v", DefaultMaxMessages, DefaultMaxDuration)
	}
	if MCPHardCapMaxMessages < DefaultMaxMessages || MCPHardCapMaxDuration < DefaultMaxDuration {
		t.Fatalf("MCP hard caps must not undercut the defaults")
	}
}
