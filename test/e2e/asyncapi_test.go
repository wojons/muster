package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	kafkatest "github.com/wojons/muster/pkg/protocol/asyncapi/kafkatest"
)

// asyncapiDocFor writes an AsyncAPI 3.x fixture whose server host points
// at the in-process stub, and returns the file path.
func asyncapiDocFor(t *testing.T, addr string) string {
	t.Helper()
	doc := fmt.Sprintf(`{
		"asyncapi": "3.0.0",
		"info": {"title": "E2E AsyncAPI", "version": "1.0.0"},
		"servers": {"prod": {"host": "%s", "pathname": "", "protocol": "kafka"}},
		"channels": {
			"signup": {"address": "users-signedup", "messages": {"UserSignedUp": {"payload": {"type": "object", "properties": {"userId": {"type": "string"}}, "required": ["userId"]}}}},
			"orders": {"address": "orders-{orderId}", "parameters": {"orderId": {"schema": {"type": "string"}, "required": true}}, "messages": {"OrderEvent": {"payload": {"type": "object", "properties": {"orderId": {"type": "string"}}, "required": ["orderId"]}}}}
		},
		"operations": {
			"notifySignup": {"action": "send", "channel": {"$ref": "#/channels/signup"}, "messages": [{"$ref": "#/channels/signup/messages/UserSignedUp"}], "operationId": "sendUserSignedUp_v1"},
			"watchOrders": {"action": "receive", "channel": {"$ref": "#/channels/orders"}, "messages": [{"$ref": "#/channels/orders/messages/OrderEvent"}]}
		}
	}`, addr)
	dir := t.TempDir()
	path := filepath.Join(dir, "asyncapi-e2e.json")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatalf("write document: %v", err)
	}
	return path
}

func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !execErrAs(err, &ee) {
			t.Fatalf("run %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return out.String(), code
}

func execErrAs(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

// TestAsyncAPIEndToEndOverRealBinary exercises the real CLI through the
// PROTO-021 acceptance path: document → operations → Kafka publish and
// bounded consume with distinct exit codes, all against the in-process
// wire stub (no external broker, no containers).
func TestAsyncAPIEndToEndOverRealBinary(t *testing.T) {
	skipLong(t)

	b := kafkatest.Start(t, 2)
	b.CreateTopic("orders-a1")
	b.CreateTopic("users-signedup")
	doc := asyncapiDocFor(t, b.Addr())

	// 1. Discovery through the real binary.
	out, code := runCLI(t, "protocol", "asyncapi", "list", "--doc", doc)
	if code != 0 {
		t.Fatalf("list exited %d: %s", code, out)
	}
	if !strings.Contains(out, "sendUserSignedUp_v1") || !strings.Contains(out, "receive-orders-OrderEvent") {
		t.Fatalf("discovery output incomplete: %s", out)
	}

	// 2. Publish through the real binary → ack JSON, record on the stub.
	out, code = runCLI(t, "protocol", "asyncapi", "publish",
		"--doc", doc, "--operation", "sendUserSignedUp_v1",
		"--args", `{"userId":"e2e-1","_key":"e2e-1"}`)
	if code != 0 {
		t.Fatalf("publish exited %d: %s", code, out)
	}
	if !strings.Contains(out, `"topic": "users-signedup"`) {
		t.Fatalf("publish ack missing topic: %s", out)
	}
	produced := len(b.Records("users-signedup", 0)) + len(b.Records("users-signedup", 1))
	if produced != 1 {
		t.Fatalf("expected one produced record, got %d", produced)
	}

	// 3. Seed a consumed backlog through kafka-go directly (reference
	// client), then drained consume → exit 0.
	w := &kafka.Writer{Addr: kafka.TCP(b.Addr()), Topic: "orders-a1", RequiredAcks: kafka.RequireOne, Balancer: &kafka.RoundRobin{}, BatchTimeout: 10 * time.Millisecond}
	msgs := make([]kafka.Message, 0, 6)
	for i := 0; i < 6; i++ {
		msgs = append(msgs, kafka.Message{Value: []byte(fmt.Sprintf(`{"orderId":"o%d"}`, i))})
	}
	if err := w.WriteMessages(t.Context(), msgs...); err != nil {
		t.Fatalf("reference-client seed: %v", err)
	}
	_ = w.Close()

	out, code = runCLI(t, "protocol", "asyncapi", "consume",
		"--doc", doc, "--operation", "receive-orders-OrderEvent",
		"--args", `{"orderId":"a1"}`, "--max-messages", "100", "--timeout", "20s")
	if code != 0 {
		t.Fatalf("drained consume must exit 0, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"kind":"event-source"`) || !strings.Contains(out, `"status":"complete"`) {
		t.Fatalf("drained transcript envelope missing: %s", tailOf(out))
	}

	// 4. max-messages cap → distinct exit code 4.
	out, code = runCLI(t, "protocol", "asyncapi", "consume",
		"--doc", doc, "--operation", "receive-orders-OrderEvent",
		"--args", `{"orderId":"a1"}`, "--max-messages", "3", "--timeout", "20s")
	if code != 4 {
		t.Fatalf("max-messages cap must exit 4, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"limit_reason":"max-items"`) {
		t.Fatalf("capped transcript envelope missing limit_reason: %s", tailOf(out))
	}

	// 5. Timed out (backlog bigger than the window) → distinct exit code 3.
	// Seed a batch far larger than a 150ms collect can drain.
	msgs2 := make([]kafka.Message, 0, 40000)
	for i := 0; i < 40000; i++ {
		msgs2 = append(msgs2, kafka.Message{Value: []byte(fmt.Sprintf(`{"orderId":"b%d"}`, i))})
	}
	bRef := &kafka.Writer{Addr: kafka.TCP(b.Addr()), Topic: "orders-a1", RequiredAcks: kafka.RequireOne, Balancer: &kafka.RoundRobin{}, BatchTimeout: 10 * time.Millisecond}
	if err := bRef.WriteMessages(t.Context(), msgs2...); err != nil {
		t.Fatalf("big seed: %v", err)
	}
	_ = bRef.Close()

	out, code = runCLI(t, "protocol", "asyncapi", "consume",
		"--doc", doc, "--operation", "receive-orders-OrderEvent",
		"--args", `{"orderId":"a1"}`, "--max-messages", "1000000", "--timeout", "150ms")
	if code != 3 {
		t.Fatalf("timed-out consume must exit 3, got %d: %s", code, tailOf(out))
	}
	if !strings.Contains(out, `"limit_reason":"max-duration"`) {
		t.Fatalf("timed-out transcript envelope missing limit_reason: %s", tailOf(out))
	}
}

func tailOf(s string) string {
	if i := strings.LastIndexByte(s, '\n'); i >= 0 && i < len(s)-1 {
		s = s[max(0, strings.LastIndexByte(s[:i], '\n')+1):]
	}
	return s
}
