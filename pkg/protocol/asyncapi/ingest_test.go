package asyncapi

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	mustererrors "github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(fixture(t, name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func parseFixture(t *testing.T, name string) *Document {
	t.Helper()
	doc, err := ParseDocument(fixture(t, name), loadFixture(t, name))
	if err != nil {
		t.Fatalf("ParseDocument(%s): %v", name, err)
	}
	return doc
}

// opByID finds an operation by neutral ID.
func opByID(t *testing.T, doc *Document, id string) *protocol.Operation {
	t.Helper()
	for _, op := range doc.Ops {
		if op.ID == id {
			return op
		}
	}
	t.Fatalf("operation %q not found in parsed document", id)
	return nil
}

func assertCode(t *testing.T, err error, want mustererrors.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", want)
	}
	var appErr *mustererrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	t.Helper()
	if appErr.Code() != want {
		t.Fatalf("expected code %s, got %s: %s", want, appErr.Code(), appErr.Message())
	}
}

// TestParse_BasicKafkaDocument walks the full happy-path fixture: operation
// expansion, kinds, synthesized IDs, field mapping and binding preservation.
func TestParse_BasicKafkaDocument(t *testing.T) {
	doc := parseFixture(t, "kafka-basic.json")

	if doc.AsyncAPI != "3.0.0" || doc.Title != "Fleet Events" || doc.Version != "1.2.0" {
		t.Fatalf("document facts wrong: %+v", doc)
	}
	if len(doc.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(doc.Servers))
	}

	// Six expansions: 2 (notifyOrder) + 1 (notifySignup) + 1 (watchOrders)
	// + 1 (watchSignups) + ... = 5 operations (one per message).
	if len(doc.Ops) != 5 {
		t.Fatalf("expected 5 operations, got %d: %v", len(doc.Ops), ids(doc.Ops))
	}
	got := map[string]bool{}
	for _, op := range doc.Ops {
		got[op.ID] = true
	}
	wantIDs := []string{
		"send-orderEvents-OrderEvent",
		"send-orderEvents-AuditTrail",
		"sendUserSignedUp_v1",
		"receive-orderEvents-OrderEvent",
		"receive-userSignedup-UserSignedUp",
	}
	for _, id := range wantIDs {
		if !got[id] {
			t.Fatalf("missing operation id %q (have %v)", id, ids(doc.Ops))
		}
	}

	// Kinds (PROTO-007 decision D2).
	if op := opByID(t, doc, "sendUserSignedUp_v1"); op.Kind != protocol.KindUnary {
		t.Fatalf("send action must map to KindUnary, got %d", op.Kind)
	}
	if op := opByID(t, doc, "receive-orderEvents-OrderEvent"); op.Kind != protocol.KindEventSource {
		t.Fatalf("receive action must map to KindEventSource, got %d", op.Kind)
	}

	// Multi-message expansion retains identities.
	multi := opByID(t, doc, "send-orderEvents-AuditTrail")
	if multi.Source[SourceOperationID] != "notifyOrder" ||
		multi.Source[SourceMessageID] != "AuditTrail" ||
		multi.Source[SourceChannelID] != "orderEvents" {
		t.Fatalf("multi-message expansion lost source identity: %v", multi.Source)
	}

	// Channel params and address (including {param} templates).
	order := opByID(t, doc, "send-orderEvents-OrderEvent")
	if order.Source[SourceChannelAddress] != "orders-{orderId}" {
		t.Fatalf("channel address not preserved verbatim: %q", order.Source[SourceChannelAddress])
	}
	var orderParam *protocol.Parameter
	for i := range order.Parameters {
		if order.Parameters[i].Name == "orderId" && order.Parameters[i].In == InChannel {
			orderParam = &order.Parameters[i]
		}
	}
	if orderParam == nil {
		t.Fatalf("channel parameter orderId not mapped: %+v", order.Parameters)
	}
	if orderParam.Type != "string" || orderParam.Description != "order identifier" || !orderParam.Required {
		t.Fatalf("channel parameter mapping wrong: %+v", orderParam)
	}

	// Payload depth-1 fields + required.
	var fields []protocol.RequestBodyProperty
	for _, f := range order.RequestBody {
		fields = append(fields, f)
	}
	if len(fields) != 2 || fields[0].Name != "amount" || fields[1].Name != "orderId" {
		t.Fatalf("payload fields wrong (sorted order expected): %+v", fields)
	}
	if fields[1].Required != true || fields[0].Required != false {
		t.Fatalf("required flags wrong: %+v", fields)
	}

	// Opaque payload becomes the whole-object argument.
	audit := opByID(t, doc, "send-orderEvents-AuditTrail")
	if len(audit.RequestBody) != 1 || audit.RequestBody[0].Name != reservedArgPayload {
		t.Fatalf("opaque payload must produce one whole-object argument: %+v", audit.RequestBody)
	}
	if audit.Source[SourcePayloadRef] == "" {
		t.Fatalf("AuditTrail payload $ref not preserved in source")
	}

	// Message headers as message-header parameters.
	signup := opByID(t, doc, "sendUserSignedUp_v1")
	var traceID *protocol.Parameter
	for i := range signup.Parameters {
		if signup.Parameters[i].Name == "trace-id" {
			traceID = &signup.Parameters[i]
		}
	}
	if traceID == nil || traceID.In != InMessageHeader || !traceID.Required || traceID.Type != "string" {
		t.Fatalf("header parameter mapping wrong: %+v", signup.Parameters)
	}

	// Tags, summary, security, bindings, document fallback.
	if len(signup.Tags) != 1 || signup.Tags[0] != "users" || signup.Summary != "Publish a signup event" {
		t.Fatalf("tags/summary wrong: %+v", signup)
	}
	var security []map[string]interface{}
	if err := json.Unmarshal([]byte(signup.Source[SourceSecurity]), &security); err != nil {
		t.Fatalf("security source is not canonical JSON: %v", err)
	}
	if len(security) != 1 {
		if _, hasScram := security[0]["saslScram"]; !hasScram {
			t.Fatalf("security requirement not preserved: %s", signup.Source[SourceSecurity])
		}
	}
	if signup.Source[SourceOpBinding] == "" || !containsJSONKey(signup.Source[SourceOpBinding], "kafka") {
		t.Fatalf("operation binding not preserved as canonical JSON: %q", signup.Source[SourceOpBinding])
	}
	if signup.Source[SourceChannelBinding] == "" || !containsJSONKey(signup.Source[SourceChannelBinding], "kafka") {
		t.Fatalf("channel binding not preserved: %q", signup.Source[SourceChannelBinding])
	}
	if signup.Source[SourceMsgBinding] == "" || !containsJSONKey(signup.Source[SourceMsgBinding], "kafka") {
		t.Fatalf("message binding not preserved: %q", signup.Source[SourceMsgBinding])
	}

	// Fallbacks: summary/description fall back to the document context.
	if op := opByID(t, doc, "receive-orderEvents-OrderEvent"); op.Summary != "Fleet Events" {
		t.Fatalf("document-context summary fallback missing: %q", op.Summary)
	}

	// Server selection: NOT stamped by ParseDocument (selection happens in
	// Discover); a single declared server is chosen deterministically at
	// discovery time.
	for _, op := range doc.Ops {
		if op.Source[SourceServerName] != "" || op.Source[SourceServerURL] != "" {
			t.Fatalf("ParseDocument must not select a server (PROTO-007 §2.2): %q", op.ID)
		}
	}
	disc := NewClient()
	discOps, err := disc.Discover(t.Context(), fixture(t, "kafka-basic.json"))
	if err != nil {
		t.Fatalf("Discover(kafka-basic.json): %v", err)
	}
	for _, op := range discOps {
		if op.Source[SourceServerName] != "prod" ||
			op.Source[SourceServerURL] != "broker.example.com:9092" ||
			op.Source[SourceServerProtocol] != "kafka" {
			t.Fatalf("server identity not stamped by Discover: %q", op.ID)
		}
		if op.Source[SourceServerBinding] == "" {
			t.Fatalf("server binding not stamped on %q", op.ID)
		}
	}
}

func ids(ops []*protocol.Operation) []string {
	out := make([]string, 0, len(ops))
	for _, op := range ops {
		out = append(out, op.ID)
	}
	return out
}

func containsJSONKey(raw, key string) bool {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// TestParse_SynthCollisionResolves deterministically: two operations with
// the same (action, channel, message) identity get -2 on the second, and
// the assignment is stable across parses.
func TestParse_SynthCollisionResolves(t *testing.T) {
	doc := parseFixture(t, "kafka-synth-collision.json")
	if len(doc.Ops) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(doc.Ops))
	}
	if doc.Ops[0].ID != "receive-c1-M" || doc.Ops[1].ID != "receive-c1-M-2" {
		t.Fatalf("synthesis collision not resolved deterministically: %s, %s", doc.Ops[0].ID, doc.Ops[1].ID)
	}
	again := parseFixture(t, "kafka-synth-collision.json")
	for i := range doc.Ops {
		if doc.Ops[i].ID != again.Ops[i].ID {
			t.Fatalf("synthesis not stable across parses")
		}
	}
}

// TestParse_Rejections covers the §9.2 failure gates: version, explicit ID
// collisions, unresolved refs, invalid actions, no operations.
func TestParse_Rejections(t *testing.T) {
	t.Run("asyncapi 2.x document", func(t *testing.T) {
		_, err := ParseDocument(fixture(t, "kafka-asyncapi2.json"), loadFixture(t, "kafka-asyncapi2.json"))
		assertCode(t, err, mustererrors.CodeValidation)
	})

	t.Run("duplicate explicit operationId", func(t *testing.T) {
		_, err := ParseDocument(fixture(t, "kafka-dup-id.json"), loadFixture(t, "kafka-dup-id.json"))
		assertCode(t, err, mustererrors.CodeValidation)
	})

	t.Run("unresolved channel ref", func(t *testing.T) {
		_, err := ParseDocument(fixture(t, "kafka-unresolved-ref.json"), loadFixture(t, "kafka-unresolved-ref.json"))
		assertCode(t, err, mustererrors.CodeValidation)
	})

	t.Run("asyncapi 2.x action name", func(t *testing.T) {
		_, err := ParseDocument(fixture(t, "kafka-invalid-action.json"), loadFixture(t, "kafka-invalid-action.json"))
		assertCode(t, err, mustererrors.CodeValidation)
	})

	t.Run("no operations", func(t *testing.T) {
		body := []byte(`{"asyncapi":"3.0.0","info":{"title":"empty"},"servers":{},"channels":{},"operations":{}}`)
		_, err := ParseDocument("inline", body)
		assertCode(t, err, mustererrors.CodeValidation)
	})

	t.Run("external $ref rejected", func(t *testing.T) {
		body := []byte(`{"asyncapi":"3.0.0","servers":{},"channels":{"c":{"address":"t","messages":{"M":{"payload":{"type":"object"}}}}},"operations":{"o":{"action":"send","channel":{"$ref":"https://other/doc#/channels/c"},"messages":[{"$ref":"#/channels/c/messages/M"}]}}}`)
		_, err := ParseDocument("inline", body)
		assertCode(t, err, mustererrors.CodeValidation)
	})

	t.Run("neither JSON nor YAML", func(t *testing.T) {
		_, err := ParseDocument("inline", []byte("}}not a document{{"))
		assertCode(t, err, mustererrors.CodeValidation)
	})
}

// TestParse_YAMLDatasettle: the YAML fixture parses to the same operation
// IDs and kinds as the JSON one.
func TestParse_YAMLDatasettle(t *testing.T) {
	doc := parseFixture(t, "kafka-basic.yaml")
	if len(doc.Ops) != 1 {
		t.Fatalf("expected 1 operation from YAML fixture, got %d (%v)", len(doc.Ops), ids(doc.Ops))
	}
	op := doc.Ops[0]
	if op.ID != "send-userSignedup-UserSignedUp" || op.Kind != protocol.KindUnary {
		t.Fatalf("YAML parse wrong: %+v", op)
	}
	if op.Source[SourceChannelAddress] != "users-signedup" {
		t.Fatalf("YAML channel address wrong: %q", op.Source[SourceChannelAddress])
	}
}

// TestParse_ServerAmbiguity: two servers without explicit selection fail
// validation; a selection option (exercised through Discover in the
// integration suite) is required.
func TestParse_ServerAmbiguity(t *testing.T) {
	// ParseDocument itself never selects (selection happens in Discover),
	// so only Discover can enforce ambiguity.
	client := NewClient()
	_, err := client.Discover(t.Context(), fixture(t, "kafka-multi-server.json"))
	assertCode(t, err, mustererrors.CodeValidation)
}

// TestParse_InlineMessageIdentity: a message declared inline in an
// operation still ingests with a stable derived identity.
func TestParse_InlineMessageIdentity(t *testing.T) {
	body := []byte(`{
		"asyncapi": "3.0.0",
		"servers": {"s": {"host": "b:9092", "protocol": "kafka"}},
		"channels": {"c": {"address": "topic-x", "messages": {}}},
		"operations": {
			"o": {
				"action": "send",
				"channel": {"$ref": "#/channels/c"},
				"messages": [{"payload": {"type": "object", "properties": {"a": {"type": "string"}}}}]
			}
		}
	}`)
	doc, err := ParseDocument("inline", body)
	if err != nil {
		t.Fatalf("inline message must ingest: %v", err)
	}
	if doc.Ops[0].ID != "send-c-inline-message" {
		t.Fatalf("inline message identity wrong: %q", doc.Ops[0].ID)
	}
	if len(doc.Ops[0].RequestBody) != 1 || doc.Ops[0].RequestBody[0].Name != "a" {
		t.Fatalf("inline payload not mapped: %+v", doc.Ops[0].RequestBody)
	}
}
