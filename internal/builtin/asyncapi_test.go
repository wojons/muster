package builtin

import (
	"bytes"
	stderrors "errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/spf13/cobra"
	kafkatest "github.com/wojons/muster/pkg/protocol/asyncapi/kafkatest"
)

// asyncapiFixtureDoc builds an AsyncAPI 3.x document pointing its server
// host at the in-process stub, writes it to a temp file and returns the
// path.
func asyncapiFixtureDoc(t *testing.T, addr string) string {
	t.Helper()
	doc := fmt.Sprintf(`{
		"asyncapi": "3.0.0",
		"info": {"title": "CLI Fixtures", "version": "1.0.0"},
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
	path := fmt.Sprintf("%s/asyncapi-cli-test.json", t.TempDir())
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatalf("write document: %v", err)
	}
	return path
}

// findSubCommand looks up one direct subcommand by name.
func findSubCommand(t *testing.T, cmd *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, sub := range cmd.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	t.Fatalf("subcommand %q not found under %s", name, cmd.Name())
	return nil
}

// execAsyncAPI runs the asyncapi group with args and returns (stdout,
// stderr, error).
func execAsyncAPI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	cmd := newProtocolAsyncAPICommand()
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(t.Context())
	return out.String(), errBuf.String(), err
}

// TestAsyncAPICommandRegistered: the protocol group exposes the asyncapi
// namespace with its three verbs.
func TestAsyncAPICommandRegistered(t *testing.T) {
	root := newProtocolCommand()
	asyncCmd := findSubCommand(t, root, "asyncapi")
	for _, name := range []string{"list", "publish", "consume"} {
		if findSubCommand(t, asyncCmd, name) == nil {
			t.Fatalf("protocol asyncapi is missing the %q subcommand", name)
		}
	}
}

// TestAsyncAPIList: discovery renders the neutral operations.
func TestAsyncAPIList(t *testing.T) {
	doc := asyncapiFixtureDoc(t, "127.0.0.1:9092") // discovery never dials
	out, _, err := execAsyncAPI(t, "list", "--doc", doc)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "sendUserSignedUp_v1") || !strings.Contains(out, "receive-orders-OrderEvent") {
		t.Fatalf("list output missing operations: %s", out)
	}
	if !strings.Contains(out, `"kind": "event-source"`) {
		t.Fatalf("receive operation must render as event-source: %s", out)
	}
}

// TestAsyncAPIPublishProducesWithKeyAndAck: the publish verb sends one
// message and renders the broker ack.
func TestAsyncAPIPublishProducesWithKeyAndAck(t *testing.T) {
	stub := kafkatest.Start(t, 2)
	stub.CreateTopic("users-signedup")
	doc := asyncapiFixtureDoc(t, stub.Addr())
	out, _, err := execAsyncAPI(t,
		"publish", "--doc", doc, "--operation", "sendUserSignedUp_v1",
		"--args", `{"userId":"u-9","_key":"u-9"}`)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if !strings.Contains(out, `"topic": "users-signedup"`) || !strings.Contains(out, `"ack": "leader"`) {
		t.Fatalf("ack not rendered: %s", out)
	}
	total := 0
	for p := int32(0); p < 2; p++ {
		total += len(stub.Records("users-signedup", p))
	}
	if total != 1 {
		t.Fatalf("expected exactly one produced record, got %d", total)
	}
}

// TestAsyncAPIConsumeDrainedNDJSON: the drained ending is exit 0 with
// NDJSON item lines plus the final envelope line.
func TestAsyncAPIConsumeDrainedNDJSON(t *testing.T) {
	stub := kafkatest.Start(t, 1)
	stub.CreateTopic("orders-a1")
	doc := asyncapiFixtureDoc(t, stub.Addr())

	w := &kafka.Writer{Addr: kafka.TCP(stub.Addr()), Topic: "orders-a1", RequiredAcks: kafka.RequireOne, BatchTimeout: 10 * time.Millisecond}
	if err := w.WriteMessages(t.Context(),
		kafka.Message{Value: []byte(`{"orderId":"o1"}`)},
		kafka.Message{Value: []byte(`{"orderId":"o2"}`)},
	); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = w.Close()

	out, _, err := execAsyncAPI(t,
		"consume", "--doc", doc, "--operation", "receive-orders-OrderEvent",
		"--max-messages", "100", "--timeout", "10s",
		"--args", `{"orderId":"a1"}`)
	if err != nil {
		t.Fatalf("drained consume must exit 0: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 { // 2 items + the envelope line
		t.Fatalf("expected 3 NDJSON lines, got %d: %s", len(lines), out)
	}
	last := lines[len(lines)-1]
	if !strings.Contains(last, `"status":"complete"`) || !strings.Contains(last, `"kind":"event-source"`) {
		t.Fatalf("final envelope line wrong: %s", last)
	}
	if !strings.Contains(lines[0], `"payload":{"orderId":"o1"}`) {
		t.Fatalf("item line wrong: %s", lines[0])
	}
}

// TestAsyncAPIConsumeCapExitCode: the max-messages cap maps to its
// distinct exit code through the command's RunE (the exitCoder is unwound
// by the real binary's main, verified end-to-end in test/e2e).
func TestAsyncAPIConsumeCapExitCode(t *testing.T) {
	stub := kafkatest.Start(t, 1)
	stub.CreateTopic("orders-a1")
	doc := asyncapiFixtureDoc(t, stub.Addr())

	w := &kafka.Writer{Addr: kafka.TCP(stub.Addr()), Topic: "orders-a1", RequiredAcks: kafka.RequireOne, BatchTimeout: 10 * time.Millisecond}
	msgs := make([]kafka.Message, 0, 30)
	for i := 0; i < 30; i++ {
		msgs = append(msgs, kafka.Message{Value: []byte(`{"orderId":"o` + fmt.Sprint(i) + `"}`)})
	}
	if err := w.WriteMessages(t.Context(), msgs...); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_ = w.Close()

	out, _, err := execAsyncAPI(t,
		"consume", "--doc", doc, "--operation", "receive-orders-OrderEvent",
		"--max-messages", "5", "--timeout", "10s",
		"--args", `{"orderId":"a1"}`)
	if err == nil {
		t.Fatalf("max-messages cap must produce a non-zero exit error")
	}
	var ec interface{ ExitCode() int }
	if !stderrors.As(err, &ec) {
		t.Fatalf("expected an exit-carrying error, got %T: %v", err, err)
	}
	if ec.ExitCode() != exitCodeMaxItemsCap {
		t.Fatalf("expected exit code %d, got %d", exitCodeMaxItemsCap, ec.ExitCode())
	}
	if count := strings.Count(strings.TrimSpace(out), "\n"); count != 5 { // 5 items + envelope
		t.Fatalf("cap consume must print 5 item lines plus the envelope, got %d newlines: %s", count, out)
	}
	if !strings.Contains(strings.TrimSpace(out), `"limit_reason":"max-items"`) {
		t.Fatalf("envelope must carry the max-items limit reason: %s", out)
	}
}
