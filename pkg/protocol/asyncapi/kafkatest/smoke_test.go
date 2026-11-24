package kafkatest

import (
	"context"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

// smoke: a plain kafka-go client produces and fetches against the stub.
func TestStubSmokeClientRoundTrip(t *testing.T) {
	b := Start(t, 2)
	b.CreateTopic("smoke")

	// direct kafka-go produce (kafka-go is the reference client)
	writer := &kafka.Writer{Addr: kafka.TCP(b.Addr()), Topic: "smoke", Balancer: &kafka.RoundRobin{}}
	for i := 0; i < 5; i++ {
		if err := writer.WriteMessages(context.Background(), kafka.Message{Key: []byte("k"), Value: []byte("v")}); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	_ = writer.Close()

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{b.Addr()}, Topic: "smoke", Partition: 0, MaxWait: 200 * time.Millisecond,
	})
	defer r.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n := 0
	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			break
		}
		n++
		t.Logf("got p%d o%d key=%s val=%s", m.Partition, m.Offset, m.Key, m.Value)
	}
	if n == 0 {
		t.Fatalf("no messages fetched from the stub")
	}
	_ = b.NextOffset("smoke", 0)
	_ = b.Records("smoke", 0)
}
