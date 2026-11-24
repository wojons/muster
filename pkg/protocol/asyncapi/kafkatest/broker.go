// Package kafkatest provides an in-process Kafka wire-protocol stub broker
// for tests of the AsyncAPI adapter (PROTO-021 acceptance criterion 3).
//
// kafka-go v0.4.51 ships NO importable in-process broker (measured: the
// module contains no mock broker package), so this package implements the
// minimal loopback server sufficient for kafka-go clients WITHOUT consumer
// groups:
//
//	ApiVersions (18), Metadata (3), ListOffsets (2), Produce (0, acks), Fetch (1)
//
// The stub speaks the real Kafka wire protocol by reusing kafka-go's own
// protocol codec (protocol.ReadRequest / protocol.WriteResponse), so a
// kafka-go client exchanges genuine encoded frames with it over a loopback
// TCP listener. No containers, no external broker, no external network.
//
// TEST-ONLY: this package must NEVER be imported by production code. The
// guard test in this package asserts the production binaries' build closure
// (go list -deps over ./cmd/...) does not contain it.
package kafkatest

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/kafka-go/protocol"
	"github.com/segmentio/kafka-go/protocol/apiversions"
	"github.com/segmentio/kafka-go/protocol/fetch"
	"github.com/segmentio/kafka-go/protocol/listoffsets"
	"github.com/segmentio/kafka-go/protocol/metadata"
	"github.com/segmentio/kafka-go/protocol/produce"
)

// version ranges the stub advertises. They deliberately stop below the
// first flexible-protocol versions (Produce v9 / Fetch v12 / Metadata v9+),
// keeping every exchange in the fixed-width encoding kafka-go's client
// handles natively for these ranges.
var advertisedAPIs = []apiversions.ApiKeyResponse{
	{ApiKey: int16(protocol.Produce), MinVersion: 0, MaxVersion: 7},
	{ApiKey: int16(protocol.Fetch), MinVersion: 0, MaxVersion: 11},
	{ApiKey: int16(protocol.ListOffsets), MinVersion: 1, MaxVersion: 5},
	{ApiKey: int16(protocol.Metadata), MinVersion: 0, MaxVersion: 8},
	{ApiKey: int16(protocol.ApiVersions), MinVersion: 0, MaxVersion: 2},
}

// Record is one message retained by the stub log.
type Record struct {
	Key     []byte
	Value   []byte
	Headers map[string]string
	Time    time.Time
}

// topicLog is one topic's partitioned log.
type topicLog struct {
	name       string
	partitions [][]Record
	nextOffset []int64
	notify     chan struct{}
	notifyMu   sync.Mutex
	notifyOnce sync.Once
}

func (t *topicLog) channel() chan struct{} {
	t.notifyOnce.Do(func() {
		t.notify = make(chan struct{}, 1)
	})
	return t.notify
}

func (t *topicLog) signal() {
	if ch := t.channel(); ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Broker is the in-process stub broker. Construct with Start; always Stop
// via the returned cleanup (Start registers t.Cleanup itself).
type Broker struct {
	ln net.Listener
	t  *testing.T

	mu       sync.Mutex
	topics   map[string]*topicLog
	partNum  int
	failNext int16 // test hook: error code to return on the next produce
	liveC    map[net.Conn]struct{}

	wg     sync.WaitGroup
	closed bool
}

// Start launches the stub broker on a loopback port and registers t.Cleanup
// to stop it. numPartitions is the partition count every topic is created
// with (1 is a valid minimum).
func Start(t *testing.T, numPartitions int) *Broker {
	t.Helper()
	if numPartitions < 1 {
		numPartitions = 1
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("kafkatest: bind loopback listener: %v", err)
	}
	b := &Broker{
		ln:      ln,
		t:       t,
		topics:  map[string]*topicLog{},
		partNum: numPartitions,
		liveC:   map[net.Conn]struct{}{},
	}
	b.wg.Add(1)
	go b.serve()
	t.Cleanup(b.Stop)
	return b
}

// Stop shuts the broker down and waits for its connections to finish.
func (b *Broker) Stop() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	conns := make([]net.Conn, 0, len(b.liveC))
	for c := range b.liveC {
		conns = append(conns, c)
	}
	b.mu.Unlock()

	// Closing the listener stops new dials; closing every live connection
	// unblocks handlers parked in ReadRequest and long-poll fetches, so
	// the wait group actually drains.
	_ = b.ln.Close()
	for _, c := range conns {
		_ = c.Close()
	}

	// Unblock long-poll fetches parked on the notify channels.
	b.mu.Lock()
	for _, t := range b.topics {
		t.signal()
	}
	b.mu.Unlock()

	b.wg.Wait()
}

// Addr returns the broker's "host:port" address for client configuration.
func (b *Broker) Addr() string {
	return b.ln.Addr().String()
}

// RefuseProduceWith arms a sticky test hook: every produce request is
// answered with the given Kafka error code on every partition, and nothing
// is appended, until ClearProduceRefusal is called. kafka-go's writer
// retries retriable codes, so a one-shot refusal would be swallowed by the
// built-in retry — the §6 refusal-mapping injection point needs a sticky
// refusal instead.
func (b *Broker) RefuseProduceWith(code int16) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failNext = code
}

// ClearProduceRefusal drops a sticky refusal armed by RefuseProduceWith.
func (b *Broker) ClearProduceRefusal() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failNext = 0
}

// ensureTopic returns the topic log, creating it with the broker's
// partition count when unknown. The caller must NOT hold b.mu; handlers
// that already hold it use ensureTopicLocked directly.
func (b *Broker) ensureTopic(name string) *topicLog {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ensureTopicLocked(name)
}

func (b *Broker) ensureTopicLocked(name string) *topicLog {
	t, ok := b.topics[name]
	if !ok {
		t = &topicLog{name: name}
		for i := 0; i < b.partNum; i++ {
			t.partitions = append(t.partitions, nil)
			t.nextOffset = append(t.nextOffset, 0)
		}
		b.topics[name] = t
	}
	return t
}

// CreateTopic pre-creates a topic with the broker's partition count. The
// kafka-go Transport caches cluster metadata from an all-topics refresh, so
// topics must exist before clients use them (the stub models a broker with
// auto-creation DISABLED for the metadata path producers consult first).
func (b *Broker) CreateTopic(name string) {
	_ = b.ensureTopic(name)
}

// ProducedAt is one record's (partition, offset) as stamped on the ack.
type ProducedAt struct {
	Partition int32
	Offset    int64
}

// Produce appends records round-robin across the topic's partitions and
// returns the (partition, offset) of each, mirroring what a real produce
// path stamps on the ack.
func (b *Broker) Produce(topic string, records ...Record) []ProducedAt {
	t := b.ensureTopic(topic)
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]ProducedAt, 0, len(records))
	for i, rec := range records {
		p := i % len(t.partitions)
		off := t.nextOffset[p]
		t.partitions[p] = append(t.partitions[p], rec)
		t.nextOffset[p] = off + 1
		out = append(out, ProducedAt{int32(p), off})
	}
	t.signal()
	return out
}

// Records returns a copy of a partition's retained records (test
// introspection).
func (b *Broker) Records(topic string, partition int32) []Record {
	b.mu.Lock()
	defer b.mu.Unlock()
	t, ok := b.topics[topic]
	if !ok || int(partition) >= len(t.partitions) {
		return nil
	}
	out := make([]Record, len(t.partitions[partition]))
	copy(out, t.partitions[partition])
	return out
}

// NextOffset returns a partition's end offset (the high watermark).
func (b *Broker) NextOffset(topic string, partition int32) int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	t, ok := b.topics[topic]
	if !ok || int(partition) >= len(t.nextOffset) {
		return 0
	}
	return t.nextOffset[partition]
}

// ─── wire serving ─────────────────────────────────────────────────────────

func (b *Broker) serve() {
	defer b.wg.Done()
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			_ = conn.Close()
			return
		}
		b.liveC[conn] = struct{}{}
		b.mu.Unlock()
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			defer func() {
				b.mu.Lock()
				delete(b.liveC, conn)
				b.mu.Unlock()
				_ = conn.Close()
			}()
			b.handleConn(conn)
		}()
	}
}

// handleConn serves one client connection until it closes or sends an
// unsupported API.
func (b *Broker) handleConn(conn net.Conn) {
	for {
		apiVersion, correlationID, _, msg, err := protocol.ReadRequest(conn)
		if err != nil {
			return
		}
		switch req := msg.(type) {
		case *apiversions.Request:
			resp := &apiversions.Response{ErrorCode: 0, ApiKeys: advertisedAPIs}
			if err := protocol.WriteResponse(conn, apiVersion, correlationID, resp); err != nil {
				return
			}
		case *metadata.Request:
			if err := protocol.WriteResponse(conn, apiVersion, correlationID, b.metadata(req)); err != nil {
				return
			}
		case *listoffsets.Request:
			if err := protocol.WriteResponse(conn, apiVersion, correlationID, b.listOffsets(req)); err != nil {
				return
			}
		case *produce.Request:
			// Acks=0 carries no response on the Kafka wire: appending the
			// records and staying silent is what the client expects.
			if !req.HasResponse() {
				b.produceWire(req)
				continue
			}
			if err := protocol.WriteResponse(conn, apiVersion, correlationID, b.produceWire(req)); err != nil {
				return
			}
		case *fetch.Request:
			if err := protocol.WriteResponse(conn, apiVersion, correlationID, b.fetch(req)); err != nil {
				return
			}
		default:
			// The client never speaks other APIs without consumer groups;
			// drop the connection so the failure is loud rather than
			// silently wrong.
			return
		}
	}
}

// metadata answers with the single broker (the stub listener address) and
// every known topic partitioned with leader = node 1. Unknown requested
// topics are auto-created.
func (b *Broker) metadata(req *metadata.Request) *metadata.Response {
	println("STUB metadata invoked:", fmt.Sprint(req.TopicNames))
	b.mu.Lock()
	requested := req.TopicNames
	names := requested
	if len(names) == 0 {
		names = make([]string, 0, len(b.topics))
		for n := range b.topics {
			names = append(names, n)
		}
	}
	println("STUB metadata answering with topics:", fmt.Sprint(names))
	host, portStr, _ := strings.Cut(b.ln.Addr().String(), ":")
	port, _ := strconv.Atoi(portStr)
	resp := &metadata.Response{
		Brokers: []metadata.ResponseBroker{{NodeID: 1, Host: host, Port: int32(port)}},
	}
	for _, name := range names {
		t := b.ensureTopicLocked(name)
		partitions := make([]metadata.ResponsePartition, 0, len(t.partitions))
		for i := range t.partitions {
			partitions = append(partitions, metadata.ResponsePartition{
				ErrorCode:      0,
				PartitionIndex: int32(i),
				LeaderID:       1,
				ReplicaNodes:   []int32{1},
				IsrNodes:       []int32{1},
			})
		}
		resp.Topics = append(resp.Topics, metadata.ResponseTopic{
			ErrorCode:  0,
			Name:       name,
			Partitions: partitions,
		})
	}
	b.mu.Unlock()
	return resp
}

// listOffsets answers first (-2) / last (-1) offset queries. Empty
// partitions answer 0 for both (the log starts and ends at 0).
func (b *Broker) listOffsets(req *listoffsets.Request) *listoffsets.Response {
	resp := &listoffsets.Response{}
	for _, t := range req.Topics {
		log := b.ensureTopic(t.Topic)
		b.mu.Lock()
		rt := listoffsets.ResponseTopic{Topic: t.Topic}
		for _, p := range t.Partitions {
			offset := int64(0)
			switch p.Timestamp {
			case -2: // earliest
				if len(log.partitions[p.Partition]) > 0 {
					offset = log.nextOffset[p.Partition] - int64(len(log.partitions[p.Partition]))
				}
			case -1: // latest
				offset = log.nextOffset[p.Partition]
			}
			rt.Partitions = append(rt.Partitions, listoffsets.ResponsePartition{
				Partition: p.Partition,
				ErrorCode: 0,
				Timestamp: -1,
				Offset:    offset,
			})
		}
		b.mu.Unlock()
		resp.Topics = append(resp.Topics, rt)
	}
	return resp
}

// produceWire appends every record and answers per-partition acks. A
// sticky refusal (RefuseProduceWith) answers every request with the armed
// code and appends nothing until cleared.
func (b *Broker) produceWire(req *produce.Request) *produce.Response {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.failNext != 0 {
		code := b.failNext
		return b.produceRefusal(req, code)
	}

	resp := &produce.Response{}
	for _, t := range req.Topics {
		log := b.ensureTopicLocked(t.Topic)
		rt := produce.ResponseTopic{Topic: t.Topic}
		for _, p := range t.Partitions {
			base := log.nextOffset[p.Partition]
			if err := appendRecords(log, p.Partition, p.RecordSet); err != nil {
				rt.Partitions = append(rt.Partitions, produce.ResponsePartition{
					Partition: p.Partition,
					ErrorCode: 87, // InvalidRecord
				})
				continue
			}
			log.signal()
			rt.Partitions = append(rt.Partitions, produce.ResponsePartition{
				Partition:  p.Partition,
				ErrorCode:  0,
				BaseOffset: base,
			})
		}
		resp.Topics = append(resp.Topics, rt)
	}
	return resp
}

// produceRefusal answers every partition with the given error code without
// appending anything (the §6 refusal-mapping injection point).
func (b *Broker) produceRefusal(req *produce.Request, code int16) *produce.Response {
	resp := &produce.Response{}
	for _, t := range req.Topics {
		rt := produce.ResponseTopic{Topic: t.Topic}
		for _, p := range t.Partitions {
			rt.Partitions = append(rt.Partitions, produce.ResponsePartition{
				Partition: p.Partition,
				ErrorCode: code,
			})
		}
		resp.Topics = append(resp.Topics, rt)
	}
	return resp
}

// appendRecords decodes a RecordSet (v1 message sets and v2 record batches
// both decode through the same protocol codec) and appends it to a
// partition log.
func appendRecords(log *topicLog, partition int32, rs protocol.RecordSet) error {
	if int(partition) >= len(log.partitions) {
		return fmt.Errorf("kafkatest: partition %d out of range", partition)
	}
	if rs.Records == nil {
		return nil
	}
	for {
		rec, err := rs.Records.ReadRecord()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var key, value []byte
		if rec.Key != nil {
			key, err = io.ReadAll(rec.Key)
			if err != nil {
				return err
			}
		}
		if rec.Value != nil {
			value, err = io.ReadAll(rec.Value)
			if err != nil {
				return err
			}
		}
		headers := map[string]string{}
		for _, h := range rec.Headers {
			headers[h.Key] = string(h.Value)
		}
		ts := rec.Time
		if ts.IsZero() {
			ts = time.Now()
		}
		log.partitions[partition] = append(log.partitions[partition], Record{
			Key:     key,
			Value:   value,
			Headers: headers,
			Time:    ts,
		})
		log.nextOffset[partition]++
	}
}

// fetch answers one fetch round: the records available at FetchOffset for
// each requested partition, or a long-poll wait (bounded by the request's
// MaxWaitTime, capped at 1s) when a partition has nothing new. The
// high watermark is the partition's next offset.
func (b *Broker) fetch(req *fetch.Request) *fetch.Response {
	wait := time.Duration(req.MaxWaitTime) * time.Millisecond
	if wait <= 0 || wait > time.Second {
		wait = time.Second
	}
	deadline := time.Now().Add(wait)

	for {
		resp := b.fetchOnce(req)
		if resp != nil {
			return resp
		}
		// Nothing available anywhere: long-poll until new data, deadline,
		// or broker shutdown.
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return b.fetchOnceEmpty(req)
		}
		if !b.parkWait(req, remaining) {
			return b.fetchOnceEmpty(req)
		}
	}
}

// parkWait waits up to max for new data on any requested topic's notify
// channel, or broker shutdown; reports whether data may have arrived.
func (b *Broker) parkWait(req *fetch.Request, max time.Duration) bool {
	b.mu.Lock()
	var chans []chan struct{}
	for _, t := range req.Topics {
		if log, ok := b.topics[t.Topic]; ok {
			chans = append(chans, log.channel())
		}
	}
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return false
	}
	timer := time.NewTimer(max)
	defer timer.Stop()
	for _, ch := range chans {
		select {
		case <-ch:
			return true
		case <-timer.C:
			return false
		}
	}
	<-timer.C
	return false
}

// fetchOnce builds the response when at least one requested partition has
// data at its fetch offset, else nil.
func (b *Broker) fetchOnce(req *fetch.Request) *fetch.Response {
	b.mu.Lock()
	defer b.mu.Unlock()
	resp := &fetch.Response{SessionID: req.SessionID}
	anyData := false
	for _, t := range req.Topics {
		log, ok := b.topics[t.Topic]
		if !ok {
			continue
		}
		ft := fetch.ResponseTopic{Topic: t.Topic}
		for _, p := range t.Partitions {
			if int(p.Partition) >= len(log.partitions) {
				continue
			}
			records := availableRecords(log, p.Partition, p.FetchOffset)
			hw := log.nextOffset[p.Partition]
			ft.Partitions = append(ft.Partitions, fetch.ResponsePartition{
				Partition:        p.Partition,
				ErrorCode:        0,
				HighWatermark:    hw,
				LastStableOffset: hw,
				RecordSet:        records,
			})
			if records.Records != nil {
				anyData = true
			}
		}
		resp.Topics = append(resp.Topics, ft)
	}
	if !anyData {
		return nil
	}
	return resp
}

// fetchOnceEmpty answers the deadline round with empty record sets and
// current high watermarks.
func (b *Broker) fetchOnceEmpty(req *fetch.Request) *fetch.Response {
	b.mu.Lock()
	defer b.mu.Unlock()
	resp := &fetch.Response{SessionID: req.SessionID}
	for _, t := range req.Topics {
		log, ok := b.topics[t.Topic]
		ft := fetch.ResponseTopic{Topic: t.Topic}
		if ok {
			for _, p := range t.Partitions {
				if int(p.Partition) >= len(log.partitions) {
					continue
				}
				hw := log.nextOffset[p.Partition]
				ft.Partitions = append(ft.Partitions, fetch.ResponsePartition{
					Partition:        p.Partition,
					ErrorCode:        0,
					HighWatermark:    hw,
					LastStableOffset: hw,
				})
			}
		}
		resp.Topics = append(resp.Topics, ft)
	}
	return resp
}

// availableRecords slices a partition log from the fetch offset, re-encoded
// as a v2 record set, capped at 1000 records per round.
func availableRecords(log *topicLog, partition int32, from int64) protocol.RecordSet {
	logRecords := log.partitions[partition]
	next := log.nextOffset[partition]
	first := next - int64(len(logRecords))
	if from < first {
		from = first
	}
	var out []protocol.Record
	for off := from; off < next && len(out) < 1000; off++ {
		r := logRecords[off-first]
		headers := make([]protocol.Header, 0, len(r.Headers))
		for k, v := range r.Headers {
			headers = append(headers, protocol.Header{Key: k, Value: []byte(v)})
		}
		var key, value protocol.Bytes
		if len(r.Key) > 0 {
			key = protocol.NewBytes(append([]byte(nil), r.Key...))
		}
		if len(r.Value) > 0 {
			value = protocol.NewBytes(append([]byte(nil), r.Value...))
		}
		out = append(out, protocol.Record{
			Offset:  off,
			Time:    r.Time,
			Key:     key,
			Value:   value,
			Headers: headers,
		})
	}
	if len(out) == 0 {
		return protocol.RecordSet{}
	}
	return protocol.RecordSet{
		Version: 2,
		Records: protocol.NewRecordReader(out...),
	}
}
