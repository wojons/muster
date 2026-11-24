package asyncapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/wojons/muster/pkg/protocol"
	kafkatest "github.com/wojons/muster/pkg/protocol/asyncapi/kafkatest"
)

// kafkaStub aliases the in-process wire stub for this suite.
type kafkaStub = kafkatest.Broker

// startStub launches the stub broker on a loopback port (cleanup auto).
func startStub(t *testing.T, partitions int) *kafkaStub {
	t.Helper()
	return kafkatest.Start(t, partitions)
}

// testAdapter wires a Client to an in-process stub broker through a
// fixture document whose server host is rewritten to the stub's loopback
// address (the executor reads brokers from Operation.Source when no
// explicit brokers are configured).
func testAdapter(t *testing.T, b *kafkaStub, docName string, opts ...Option) (*Client, []*protocol.Operation) {
	t.Helper()
	data := replaceServerHost(t, loadFixture(t, docName), b.Addr())
	dir := t.TempDir()
	path := filepath.Join(dir, docName)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write temp document: %v", err)
	}
	client := NewClient(opts...)
	ops, err := client.Discover(context.Background(), path)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	return client, ops
}

// replaceServerHost rewrites the fixture's server host so the document
// points at the in-process stub.
func replaceServerHost(t *testing.T, data []byte, addr string) []byte {
	t.Helper()
	return []byte(strings.ReplaceAll(string(data), "broker.example.com:9092", addr))
}

// JSON helpers (kept tiny for test readability).
func jsonUnmarshal(data []byte, v interface{}) error { return json.Unmarshal(data, v) }
func jsonMarshal(v interface{}) ([]byte, error)      { return json.Marshal(v) }

// compact renders a JSON string canonically for comparisons.
func compact(s string) string {
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func itoa(i int) string { return strconv.Itoa(i) }

func hasKey(m map[string]interface{}, k string) bool {
	_, ok := m[k]
	return ok
}

func errorsAs(err error, target interface{}) bool {
	return errors.As(err, target)
}

// seedWriter is the reference-client writer used to place messages before
// a consume: explicit acks keep the wire exchange synchronous and a tiny
// batch flush keeps per-message round trips off the test clock.
func seedWriter(b *kafkaStub, topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(b.Addr()),
		Topic:        topic,
		RequiredAcks: kafka.RequireOne,
		Balancer:     &kafka.RoundRobin{},
		BatchTimeout: 10 * time.Millisecond,
	}
}
