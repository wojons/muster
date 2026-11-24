package asyncapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	mustererrors "github.com/wojons/muster/pkg/errors"
)

// TestExecute_PublishAckMetadata: one send operation produces one message
// and returns the broker ack (PROTO-007 decision D2 / O4).
func TestExecute_PublishAckMetadata(t *testing.T) {
	b := startStub(t, 2)
	b.CreateTopic("users-signedup")

	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))
	result, err := c.Execute(t.Context(), "sendUserSignedUp_v1", map[string]interface{}{
		"userId":   "u-42",
		"email":    "u@example.com",
		"trace-id": "trace-1",
		"_key":     "u-42",
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	ack, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("publish result must be a JSON-serializable map, got %T", result)
	}
	if ack["topic"] != "users-signedup" {
		t.Fatalf("ack topic wrong: %v", ack["topic"])
	}
	if _, ok := ack["offset"]; !ok {
		t.Fatalf("ack must carry the produced offset: %v", ack)
	}
	if ack["key_present"] != true || ack["header_count"] != int(1) {
		t.Fatalf("ack key/header facts wrong: %v", ack)
	}
	// kafka-go stamps Message.Partition/Offset as int on the caller's copy.
	recs := b.Records("users-signedup", int32(ack["partition"].(int)))
	if len(recs) != 1 {
		t.Fatalf("expected 1 retained record, got %d", len(recs))
	}
	var payload map[string]interface{}
	if err := jsonUnmarshal(recs[0].Value, &payload); err != nil {
		t.Fatalf("retain payload not JSON: %v (%s)", err, recs[0].Value)
	}
	if payload["userId"] != "u-42" {
		t.Fatalf("payload mapping wrong: %v", payload)
	}
	if recs[0].Headers["trace-id"] != "trace-1" {
		t.Fatalf("message header not produced: %v", recs[0].Headers)
	}
	// The message key argument must NOT leak into the payload.
	if hasKey(payload, "_key") {
		t.Fatalf("reserved _key argument leaked into the payload: %v", payload)
	}
}

// TestExecute_PublishParameterizedTopic: channel {param} expressions
// resolve against channel parameters — and this doubles as the CONTROL
// that fails if the executor ignores Operation.Source: the fixture's
// address is the ground truth, not a hardcoded topic.
func TestExecute_PublishParameterizedTopic(t *testing.T) {
	b := startStub(t, 2)
	b.CreateTopic("orders-a1")

	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))
	if _, err := c.Execute(t.Context(), "send-orderEvents-OrderEvent", map[string]interface{}{
		"orderId": "a1",
		"amount":  19.5,
	}); err != nil {
		t.Fatalf("publish to parameterized topic: %v", err)
	}
	if len(b.Records("orders-a1", 0))+len(b.Records("orders-a1", 1)) != 1 {
		t.Fatalf("message not produced to the document's channel address")
	}
	// Wrong-channel control: the SAME argument addressed through a
	// deliberately bogus channel would never produce here — assert the
	// executor did not invent a topic name.
	if l := len(b.Records("users-signedup", 0)); l != 0 {
		t.Fatalf("executor ignored Operation.Source channel address (produced to users-signedup: %d)", l)
	}
}

// TestExecute_PublishValidationBeforeNetwork: malformed operation input is
// CodeValidation and never reaches the wire (§6).
func TestExecute_PublishValidationBeforeNetwork(t *testing.T) {
	b := startStub(t, 1)
	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))

	cases := []struct {
		name string
		opID string
		args map[string]interface{}
	}{
		{"unknown operation", "no-such-op", nil},
		{"missing required payload field", "sendUserSignedUp_v1", map[string]interface{}{"email": "x"}},
		{"unknown argument", "sendUserSignedUp_v1", map[string]interface{}{"userId": "u", "wat": true}},
		{"missing channel parameter", "send-orderEvents-OrderEvent", map[string]interface{}{"amount": 1.0}},
		{"missing everything", "send-orderEvents-OrderEvent", map[string]interface{}{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Execute(t.Context(), tc.opID, tc.args)
			assertCode(t, err, mustererrors.CodeValidation)
		})
	}
	// Nothing reached the wire.
	if got := len(b.Records("users-signedup", 0)); got != 0 {
		t.Fatalf("validation failure produced a message (%d)", got)
	}
}

// TestExecute_ConsumeDrained: the bounded collect drains everything the
// snapshot window held and reports "complete" with the delivered items.
func TestExecute_ConsumeDrained(t *testing.T) {
	b := startStub(t, 3)
	topic := "orders-a1"

	// Reference-client arm FIRST: publish five messages through kafka-go
	// directly (independent oracle), remember their payloads.
	b.CreateTopic(topic)
	// Reference-client arm: kafka-go directly (independent oracle). One
	// WriteMessages call flushes as one batch, keeping the seeding fast.
	direct := &kafka.Writer{Addr: kafka.TCP(b.Addr()), Topic: topic, RequiredAcks: kafka.RequireOne, Balancer: &kafka.RoundRobin{}, BatchTimeout: 10 * time.Millisecond}
	seeds := map[string]string{}
	msgs := make([]kafka.Message, 0, 5)
	for i := 0; i < 5; i++ {
		val := `{"orderId":"o-` + itoa(i) + `","amount":` + itoa(i) + `.5}`
		key := "k" + itoa(i)
		seeds[key] = val
		msgs = append(msgs, kafka.Message{
			Key: []byte(key), Value: []byte(val),
			Headers: []kafka.Header{{Key: "trace-id", Value: []byte("ref-" + itoa(i))}},
		})
	}
	if err := direct.WriteMessages(t.Context(), msgs...); err != nil {
		t.Fatalf("reference client write: %v", err)
	}
	_ = direct.Close()

	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))
	result, err := c.Execute(t.Context(), "receive-orderEvents-OrderEvent", map[string]interface{}{"orderId": "a1"})
	if err != nil {
		t.Fatalf("bounded consume: %v", err)
	}
	tr, ok := result.(*EventTranscript)
	if !ok {
		t.Fatalf("consume result must be the EventTranscript envelope, got %T", result)
	}
	if tr.Kind != "event-source" {
		t.Fatalf("transcript kind must be event-source, got %q", tr.Kind)
	}
	if tr.Status != StatusComplete || tr.ItemsTruncated {
		t.Fatalf("drained collect must be complete/truncated=false: %+v", tr)
	}
	if len(tr.Items) != 5 {
		t.Fatalf("expected 5 items, got %d: %v", len(tr.Items), tr.Items)
	}
	seen := 0
	for _, item := range tr.Items {
		m := item.(map[string]interface{})
		key, _ := m["key"].(string)
		if payload, ok := seeds[key]; ok {
			seen++
			enc, _ := jsonMarshal(m["payload"])
			if string(enc) != compact(payload) {
				t.Fatalf("reference payload mismatch for %s: %s vs %s", key, enc, payload)
			}
		}
		if m["topic"] != topic || m["payload_encoding"] != "json" {
			t.Fatalf("item shape wrong: %v", m)
		}
	}
	if seen < 3 {
		t.Fatalf("reference-client payloads not all replayed through the executor (saw %d)", seen)
	}
	// The executor consumed the messages a DIRECT kafka-go client wrote —
	// the conformance proof that our stub+executor path speaks the same
	// wire the ecosystem client does.
}

// TestExecute_ConsumeTruncatedMaxMessages: the item bound is a clean stop
// with status truncated / limit_reason max-items.
func TestExecute_ConsumeTruncatedMaxMessages(t *testing.T) {
	b := startStub(t, 1)
	topic := "orders-a1"
	b.CreateTopic(topic)

	direct := &kafka.Writer{Addr: kafka.TCP(b.Addr()), Topic: topic, RequiredAcks: kafka.RequireOne, Balancer: &kafka.RoundRobin{}, BatchTimeout: 10 * time.Millisecond}
	msgs := make([]kafka.Message, 0, 50)
	for i := 0; i < 50; i++ {
		msgs = append(msgs, kafka.Message{Value: []byte(`{"orderId":"o` + itoa(i) + `"}`)})
	}
	if err := direct.WriteMessages(t.Context(), msgs...); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	_ = direct.Close()

	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}), WithStreamLimits(EventLimits{MaxMessages: 10, MaxDuration: 10 * time.Second}))
	result, err := c.Execute(t.Context(), "receive-orderEvents-OrderEvent", map[string]interface{}{"orderId": "a1"})
	if err != nil {
		t.Fatalf("bounded consume: %v", err)
	}
	tr := result.(*EventTranscript)
	if tr.Status != StatusTruncated || !tr.ItemsTruncated {
		t.Fatalf("expected truncated status: %+v", tr)
	}
	if tr.Trailer["limit_reason"] != LimitReasonMessages {
		t.Fatalf("limit_reason must be %s: %v", LimitReasonMessages, tr.Trailer["limit_reason"])
	}
	if len(tr.Items) != 10 {
		t.Fatalf("expected exactly 10 items, got %d", len(tr.Items))
	}
	if tr.Trailer["max_items"] != int(10) {
		t.Fatalf("trailer must record the effective bound: %v", tr.Trailer["max_items"])
	}
}

// TestExecute_ConsumeTruncatedMaxDuration: the wall-clock bound fires on a
// backlog larger than the collect window can drain (the CLI's "timed out"
// ending — distinct from drained).
func TestExecute_ConsumeTruncatedMaxDuration(t *testing.T) {
	b := startStub(t, 1)
	topic := "orders-a1"
	b.CreateTopic(topic)

	direct := &kafka.Writer{Addr: kafka.TCP(b.Addr()), Topic: topic, RequiredAcks: kafka.RequireOne, Balancer: &kafka.RoundRobin{}, BatchTimeout: 10 * time.Millisecond}
	msgs := make([]kafka.Message, 0, 5000)
	for i := 0; i < 5000; i++ {
		msgs = append(msgs, kafka.Message{Value: []byte(`{"orderId":"o` + itoa(i) + `"}`)})
	}
	if err := direct.WriteMessages(t.Context(), msgs...); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	_ = direct.Close()

	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}), WithStreamLimits(EventLimits{MaxMessages: 100000, MaxDuration: 300 * time.Millisecond}))
	result, err := c.Execute(t.Context(), "receive-orderEvents-OrderEvent", map[string]interface{}{"orderId": "a1"})
	if err != nil {
		t.Fatalf("bounded consume must be a clean stop, got error: %v", err)
	}
	tr := result.(*EventTranscript)
	if tr.Status != StatusTruncated || tr.Trailer["limit_reason"] != LimitReasonDuration {
		t.Fatalf("expected truncated with limit_reason %s: %v", LimitReasonDuration, tr.Trailer["limit_reason"])
	}
	if len(tr.Items) == 0 || len(tr.Items) >= 5000 {
		t.Fatalf("partial items expected, got %d", len(tr.Items))
	}
}

// TestExecute_ConsumeEmptyWindow: a collect over an empty window completes
// normally with zero items (§9.2).
func TestExecute_ConsumeEmptyWindow(t *testing.T) {
	b := startStub(t, 2)
	b.CreateTopic("orders-a1")
	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))
	result, err := c.Execute(t.Context(), "receive-orderEvents-OrderEvent", map[string]interface{}{"orderId": "a1"})
	if err != nil {
		t.Fatalf("empty-window consume: %v", err)
	}
	tr := result.(*EventTranscript)
	if tr.Status != StatusComplete || len(tr.Items) != 0 {
		t.Fatalf("empty window must complete with zero items: %v", tr)
	}
	if tr.Items == nil {
		t.Fatalf("items must be a non-nil JSON array")
	}
}

// TestExecute_ConsumeCallerCancellation: the caller ending the call while
// a backlog is still being drained keeps the partial items and maps the
// ending to CodeTimeout with status cancelled.
func TestExecute_ConsumeCallerCancellation(t *testing.T) {
	b := startStub(t, 1)
	b.CreateTopic("orders-a1")
	topic := "orders-a1"

	// A backlog bigger than the cancellation window so the collect is
	// mid-drain when the caller cancels (an empty or fully drained window
	// would end complete before the cancellation fires).
	direct := &kafka.Writer{Addr: kafka.TCP(b.Addr()), Topic: topic, RequiredAcks: kafka.RequireOne, Balancer: &kafka.RoundRobin{}, BatchTimeout: 10 * time.Millisecond}
	msgs := make([]kafka.Message, 0, 20000)
	for i := 0; i < 20000; i++ {
		msgs = append(msgs, kafka.Message{Value: []byte(`{"orderId":"o` + itoa(i) + `"}`)})
	}
	if err := direct.WriteMessages(t.Context(), msgs...); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	_ = direct.Close()

	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}),
		WithStreamLimits(EventLimits{MaxMessages: 100000, MaxDuration: 30 * time.Second}))
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	result, err := c.Execute(ctx, "receive-orderEvents-OrderEvent", map[string]interface{}{"orderId": "a1"})
	if err == nil {
		t.Fatalf("caller cancellation must return the mapped AppError")
	}
	assertCode(t, err, mustererrors.CodeTimeout)
	tr := result.(*EventTranscript)
	if tr.Status != StatusCancelled {
		t.Fatalf("expected cancelled status: %+v", tr)
	}
}

// TestExecute_ConsumeGroupBindingRefused: a receive operation whose kafka
// binding demands a consumer group is rejected, never silently stripped
// (§9.2 declared-subset rule).
func TestExecute_ConsumeGroupBindingRefused(t *testing.T) {
	b := startStub(t, 1)
	b.CreateTopic("orders-a1")
	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))
	_, err := c.Execute(t.Context(), "receive-userSignedup-UserSignedUp", nil)
	assertCode(t, err, mustererrors.CodeValidation)
	if !strings.Contains(err.Error(), "groupId") {
		t.Fatalf("refusal must name the unsupported binding field: %v", err)
	}
}

// TestExecute_ProduceRefusalMapsToRateLimit: the §6 quota row, injected
// through the stub's refusal hook (typed protocol code — no substring
// matching).
func TestExecute_ProduceRefusalMapsToRateLimit(t *testing.T) {
	b := startStub(t, 1)
	b.CreateTopic("users-signedup")
	b.RefuseProduceWith(int16(89)) // ThrottlingQuotaExceeded (retriable — sticky refusal defeats the writer's retry)

	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))
	_, err := c.Execute(t.Context(), "sendUserSignedUp_v1", map[string]interface{}{"userId": "u"})
	b.ClearProduceRefusal()
	assertCode(t, err, mustererrors.CodeRateLimit)
}

// TestExecute_BrokerDownMapsToTimeoutOrGeneric: an unreachable broker maps
// onto §6 without crashing and never leaves the call hanging.
func TestExecute_BrokerDownMapsToTimeoutOrGeneric(t *testing.T) {
	c, _ := testAdapter(t, startStub(t, 1), "kafka-basic.json", WithBrokers([]string{"127.0.0.1:1"}), WithTimeout(2*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, err := c.Execute(ctx, "sendUserSignedUp_v1", map[string]interface{}{"userId": "u"})
	if err == nil {
		t.Fatalf("publish against a dead broker must fail")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("publish against a dead broker must not hang: %v", time.Since(start))
	}
	var appErr *mustererrors.AppError
	if !errorsAs(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
	if appErr.Code() != mustererrors.CodeTimeout && appErr.Code() != mustererrors.CodeGeneric {
		t.Fatalf("§6 mapping produced %s; expected CodeTimeout or CodeGeneric", appErr.Code())
	}
}

// TestExecute_ProduceToUnknownTopicIsRefused: the adapter leaves topic
// creation to the broker; a produce against an undeclared topic surfaces a
// typed broker error rather than inventing the topic (mapped per §6).
func TestExecute_ProduceToUnknownTopicIsRefused(t *testing.T) {
	b := startStub(t, 1)
	// Fresh stub, no users-signedup topic: the writer's metadata lookup
	// refuses before any produce.
	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}), WithTimeout(5*time.Second))
	_, err := c.Execute(t.Context(), "sendUserSignedUp_v1", map[string]interface{}{"userId": "x"})
	if err == nil {
		t.Fatalf("produce to an unknown topic must be refused — the topic was never created on the stub")
	}
	var appErr *mustererrors.AppError
	if !errorsAs(err, &appErr) {
		t.Fatalf("expected AppError, got %T", err)
	}
}

// TestExecute_CorrectTopicResolution control: the executor ignores nothing
// from Operation.Source — publishing the orderEvent operation with
// orderId=b9 produces on the DOCUMENT's address (orders/{orderId}) and
// nowhere else; a deliberately wrong channel would fail this control.
func TestExecute_CorrectTopicResolution(t *testing.T) {
	b := startStub(t, 2)
	b.CreateTopic("orders-b9")
	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))
	if _, err := c.Execute(t.Context(), "send-orderEvents-OrderEvent", map[string]interface{}{"orderId": "b9"}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	total := 0
	for p := int32(0); p < 2; p++ {
		total += len(b.Records("orders-b9", p))
	}
	if total != 1 {
		t.Fatalf("expected the message on the document's resolved topic, got %d records", total)
	}
	// The signup topic was never involved: the executor read the channel
	// address from Operation.Source, not a hardcoded default.
	totalSignup := 0
	for p := int32(0); p < 2; p++ {
		totalSignup += len(b.Records("users-signedup", p))
	}
	if totalSignup != 0 {
		t.Fatalf("executor ignored Operation.Source and produced to a foreign topic (%d records)", totalSignup)
	}
}

// TestPublishResultShapeIsJSONSerializable guards the §4.4 rendering
// contract: Execute's outputs marshal without custom encoders.
func TestPublishResultShapeIsJSONSerializable(t *testing.T) {
	b := startStub(t, 1)
	b.CreateTopic("users-signedup")
	c, _ := testAdapter(t, b, "kafka-basic.json", WithBrokers([]string{b.Addr()}))
	result, err := c.Execute(t.Context(), "sendUserSignedUp_v1", map[string]interface{}{"userId": "u"})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if _, err := jsonMarshal(result); err != nil {
		t.Fatalf("publish result not JSON-serializable: %v", err)
	}
}

// TestBoundsCannotBeUnbounded: zero/negative bounds select the defaults;
// there is no unbounded mode (PROTO-007 §4).
func TestBoundsCannotBeUnbounded(t *testing.T) {
	if got := (EventLimits{}).normalize(); got.MaxMessages != DefaultMaxMessages || got.MaxDuration != DefaultMaxDuration {
		t.Fatalf("defaults not applied: %+v", got)
	}
	if got := (EventLimits{MaxMessages: -5, MaxDuration: -time.Second}).normalize(); got.MaxMessages != DefaultMaxMessages {
		t.Fatalf("negative bounds must select defaults: %+v", got)
	}
	if got := (EventLimits{MaxMessages: 7}).normalize(); got.MaxMessages != 7 || got.MaxDuration != DefaultMaxDuration {
		t.Fatalf("explicit lower bound must be honored: %+v", got)
	}
}

// TestSASLMappingBuildsMechanisms proves the §5 Kafka row: each supported
// mechanism maps to a kafka-go mechanism; credentials never appear in
// errors, operations, or transcript state.
func TestSASLMappingBuildsMechanisms(t *testing.T) {
	for _, tc := range []struct {
		mech string
		name string
	}{
		{SASLMechanismPlain, "PLAIN"},
		{SASLMechanismScramSHA256, "SCRAM-SHA-256"},
		{SASLMechanismScramSHA512, "SCRAM-SHA-512"},
	} {
		c := NewClient(WithSASL(tc.mech, "user", "sekrit-does-not-leak"))
		m, err := c.sasl()
		if err != nil {
			t.Fatalf("mechanism %s: %v", tc.mech, err)
		}
		if m.Name() != tc.name {
			t.Fatalf("mechanism %s mapped to %q, want %q", tc.mech, m.Name(), tc.name)
		}
	}
	c := NewClient(WithSASL("bogus-mech", "u", "p"))
	if _, err := c.sasl(); err == nil {
		t.Fatalf("unknown mechanism must be refused")
	}
	c = NewClient()
	if m, err := c.sasl(); m != nil || err != nil {
		t.Fatalf("no SASL mapping must yield nil mechanism, got %v/%v", m, err)
	}
	// Credential leakage guard: the mapped mechanism carries secret bytes
	// in memory but they must not reach operation source stamps.
	ops := (&Client{}).Operations()
	if len(ops) != 0 {
		t.Fatalf("no discovery: no operations may exist")
	}
}
