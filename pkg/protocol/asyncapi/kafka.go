package asyncapi

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
	"github.com/wojons/muster/pkg/client"
	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// saslMechanismPlain and friends name the SASL mechanisms the adapter maps
// onto kafka-go (PROTO-007 §5 Kafka row).
const (
	SASLMechanismPlain       = "plain"
	SASLMechanismScramSHA256 = "scram-sha256"
	SASLMechanismScramSHA512 = "scram-sha512"
)

// httpClient is the minimal transport surface the adapter composes for
// document fetches (satisfied by *client.Client per specs/PROTOCOLS.md §3.1).
type httpClient interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

// Client is the AsyncAPI adapter over the Kafka transport. It discovers
// operations from an AsyncAPI 3.x document and executes them end-to-end:
// send → one bounded Kafka produce; receive → a bounded collect
// (subscribe, gather, close, return — never a long-lived subscription).
//
// The zero value is not usable; construct with NewClient.
type Client struct {
	brokers    []string
	httpClient httpClient
	timeout    time.Duration
	limits     EventLimits
	server     string

	// SASL / TLS credential mapping (PROTO-007 §5). Credentials live only
	// here, are applied to kafka-go's dialer/transport at execution time,
	// and are never serialized into operations, logs, transcripts, or tool
	// schemas.
	saslMechanism string
	saslUsername  string
	saslPassword  string
	tlsCfg        *tls.Config

	docMu sync.Mutex
	doc   *Document
	ops   []*protocol.Operation
	byID  map[string]*protocol.Operation
}

// Option configures a Client.
type Option func(*Client)

// WithBrokers pins the broker addresses Execute publishes to and consumes
// from. Without it the selected server's host (Operation.Source
// server_url) is used.
func WithBrokers(brokers []string) Option {
	return func(c *Client) {
		for _, b := range brokers {
			if t := strings.TrimSpace(b); t != "" {
				c.brokers = append(c.brokers, t)
			}
		}
	}
}

// WithHTTPClient supplies the shared HTTP client (pkg/client, with its
// tracing/retry wiring) for document fetches. Adapters never open their own
// HTTP clients (specs/PROTOCOLS.md §3.1).
func WithHTTPClient(hc *client.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithTimeout bounds a publish call whose caller's context carries no
// deadline. The caller's own deadline always wins (§4.3).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// WithStreamLimits sets the bounded-collect bounds for consume calls
// (defaults 1000 messages / 30 seconds; zero or negative values select the
// defaults — there is no unbounded mode).
func WithStreamLimits(limits EventLimits) Option {
	return func(c *Client) { c.limits = limits }
}

// WithServer selects the AsyncAPI server to run against when the document
// declares more than one (§2.2: ambiguous selection fails validation
// rather than guessing).
func WithServer(name string) Option {
	return func(c *Client) { c.server = strings.TrimSpace(name) }
}

// WithSASL maps configured SASL credentials onto the Kafka transport
// (PROTO-007 §5). mechanism is plain, scram-sha256 or scram-sha512. The
// credentials are held only in memory on the client and applied through
// kafka-go's native mechanism at dial time.
func WithSASL(mechanism, username, password string) Option {
	return func(c *Client) {
		c.saslMechanism = strings.ToLower(strings.TrimSpace(mechanism))
		c.saslUsername = username
		c.saslPassword = password
	}
}

// WithTLS maps configured TLS settings onto the Kafka transport.
func WithTLS(cfg *tls.Config) Option {
	return func(c *Client) { c.tlsCfg = cfg }
}

// DefaultTLSConfig builds the TLS settings behind the CLI's --tls flag
// (PROTO-007 §5 Kafka row: mapped TLS settings into the native transport):
// system roots, minimum TLS 1.2, default SNI from the broker address.
func DefaultTLSConfig() (*tls.Config, error) {
	return &tls.Config{MinVersion: tls.VersionTLS12}, nil
}

// NewClient creates an AsyncAPI/Kafka adapter client.
func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient: client.NewClient(),
		timeout:    DefaultMaxDuration,
		limits:     EventLimits{MaxMessages: DefaultMaxMessages, MaxDuration: DefaultMaxDuration},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// Protocol implements protocol.Adapter.
func (c *Client) Protocol() protocol.Protocol { return protocol.ProtocolAsyncAPI }

// Operations returns the operations discovered by the most recent Discover.
func (c *Client) Operations() []*protocol.Operation {
	c.docMu.Lock()
	defer c.docMu.Unlock()
	return c.ops
}

// Document returns the parsed document, or nil before discovery.
func (c *Client) Document() *Document {
	c.docMu.Lock()
	defer c.docMu.Unlock()
	return c.doc
}

// Execute implements protocol.Executor: send operations produce one
// message; receive operations run the bounded collect. args keys match
// Operation.Parameters[].Name (channel + message headers), RequestBody
// fields (payload), and the reserved "_key" argument (Kafka message key).
func (c *Client) Execute(ctx context.Context, opID string, args map[string]interface{}) (interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	c.docMu.Lock()
	op, ok := c.byID[opID]
	c.docMu.Unlock()
	if !ok {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q was not discovered; run discovery first", opID))
	}

	switch op.Kind {
	case protocol.KindUnary:
		return c.Publish(ctx, op, args)
	case protocol.KindEventSource:
		return c.Consume(ctx, op, args)
	default:
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q has kind %d: not executable by this transport", opID, op.Kind))
	}
}

// invocation carries the split invocation arguments of one execution.
type invocation struct {
	topic   string
	key     []byte
	headers []kafka.Header
	payload []byte // encoded JSON value, nil = no payload

	// payloadIsWhole records that the operation's payload was opaque (the
	// reserved whole-object argument carried it).
	payloadIsWhole bool

	// Bounded-collect knob requests (0 = unset). Effective bounds are the
	// configured limits clamped DOWN to these when set (§4.5(4)).
	requestedMaxMessages    int
	requestedTimeoutSeconds int
}

// buildInvocation splits and validates the invocation arguments BEFORE any
// network activity (§6: malformed operation input is CodeValidation and no
// broker call is made).
func buildInvocation(op *protocol.Operation, args map[string]interface{}) (*invocation, error) {
	if args == nil {
		args = map[string]interface{}{}
	}
	inv := &invocation{}

	channelParams := map[string]bool{}
	headerParams := map[string]bool{}
	payloadFields := map[string]bool{}
	opaque := false
	for _, p := range op.Parameters {
		switch p.In {
		case InChannel:
			channelParams[p.Name] = true
		case InMessageHeader:
			headerParams[p.Name] = true
		}
	}
	for _, f := range op.RequestBody {
		if f.Name == reservedArgPayload {
			opaque = true
		}
		payloadFields[f.Name] = true
	}

	// Topic: the channel address with {parameter} expressions resolved.
	topic := op.Source[SourceChannelAddress]
	used := map[string]bool{}
	for name, required := range channelParams {
		v, present := args[name]
		if !present || v == nil {
			if required || strings.Contains(topic, "{"+name+"}") {
				return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
					"asyncapi: operation %q: missing channel parameter %q needed to resolve the topic address", op.ID, name))
			}
			continue
		}
		used[name] = true
		topic = strings.ReplaceAll(topic, "{"+name+"}", valueString(v))
	}
	if rest := remainingExpr(topic); rest != "" {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q: channel address still contains unresolved parameter %s after argument substitution", op.ID, rest))
	}
	if strings.TrimSpace(topic) == "" {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: operation %q resolves to an empty topic address", op.ID))
	}
	inv.topic = topic

	// Message key (reserved argument).
	if kv, present := args[reservedArgKey]; present && kv != nil {
		used[reservedArgKey] = true
		inv.key = []byte(valueString(kv))
	}

	// Bounded-collect knobs (reserved, consume only): they may LOWER the
	// configured bounds, never raise them (§4.5(4)).
	if op.Kind == protocol.KindEventSource {
		used[reservedArgMaxMessages] = true
		used[reservedArgTimeoutSeconds] = true
		if v, present := args[reservedArgMaxMessages]; present && v != nil {
			if n, ok := asPositiveInt(v); ok {
				inv.requestedMaxMessages = n
			} else {
				return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
					"asyncapi: operation %q: %s must be a positive integer", op.ID, reservedArgMaxMessages))
			}
		}
		if v, present := args[reservedArgTimeoutSeconds]; present && v != nil {
			if n, ok := asPositiveSeconds(v); ok {
				inv.requestedTimeoutSeconds = n
			} else {
				return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
					"asyncapi: operation %q: %s must be a positive number of seconds", op.ID, reservedArgTimeoutSeconds))
			}
		}
	}

	// Message headers (In: message-header).
	for name := range headerParams {
		if v, present := args[name]; present && v != nil {
			used[name] = true
			inv.headers = append(inv.headers, kafka.Header{Key: name, Value: []byte(valueString(v))})
		}
	}

	// Payload.
	if opaque {
		if v, present := args[reservedArgPayload]; present {
			used[reservedArgPayload] = true
			enc, err := json.Marshal(v)
			if err != nil {
				return nil, errors.Wrap(err, errors.CodeValidation, fmt.Sprintf(
					"asyncapi: operation %q: %s argument is not JSON-serializable", op.ID, reservedArgPayload))
			}
			inv.payload = enc
			inv.payloadIsWhole = true
		}
		// A nil payload is legal for messages whose schema allows it; the
		// producer sends a nil value.
	} else {
		obj := map[string]interface{}{}
		for _, f := range op.RequestBody {
			if f.Name == reservedArgPayload {
				continue
			}
			v, present := args[f.Name]
			if !present {
				if f.Required {
					return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
						"asyncapi: operation %q: missing required payload field %q", op.ID, f.Name))
				}
				continue
			}
			used[f.Name] = true
			obj[f.Name] = v
		}
		enc, err := json.Marshal(obj)
		if err != nil {
			return nil, errors.Wrap(err, errors.CodeInternal, fmt.Sprintf(
				"asyncapi: operation %q: encode payload", op.ID))
		}
		inv.payload = enc
	}

	// Unknown arguments are rejected before any network activity: the
	// operation's contract is the document's schema, not an open bag.
	for name := range args {
		if !used[name] {
			return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: operation %q: unknown argument %q (declared inputs: channel parameters, message headers, payload fields%s)",
				op.ID, name, payloadHint(opaque)))
		}
	}
	return inv, nil
}

func payloadHint(opaque bool) string {
	if opaque {
		return ", the whole-message payload argument"
	}
	return ""
}

// remainingExpr finds the first "{param}" expression still unresolved in a
// topic address, or "" when none remain.
func remainingExpr(topic string) string {
	start := strings.IndexByte(topic, '{')
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(topic[start:], '}')
	if end < 0 {
		return topic[start:]
	}
	return topic[start : start+end+1]
}

// valueString renders an invocation argument as wire text (JSON for
// structured values, raw text for strings).
func valueString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	}
}

// asPositiveInt coerces a JSON/neutral numeric argument to a positive int.
func asPositiveInt(v interface{}) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, t > 0
	case int32:
		return int(t), t > 0
	case int64:
		return int(t), t > 0
	case float64:
		return int(t), t > 0 && float64(int(t)) == t
	case json.Number:
		var f float64
		if err := json.Unmarshal([]byte(t.String()), &f); err != nil {
			return 0, false
		}
		return asPositiveInt(f)
	default:
		return 0, false
	}
}

// asPositiveSeconds coerces a numeric argument to whole seconds.
func asPositiveSeconds(v interface{}) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), t >= 1
	case int:
		return t, t >= 1
	case int64:
		return int(t), t >= 1
	case json.Number:
		var f float64
		if err := json.Unmarshal([]byte(t.String()), &f); err != nil {
			return 0, false
		}
		return asPositiveSeconds(f)
	default:
		return 0, false
	}
}

// brokersFor resolves the broker addresses of an execution: the client's
// explicit list wins, else the selected server's host stamped at discovery.
func (c *Client) brokersFor(op *protocol.Operation) ([]string, error) {
	if len(c.brokers) > 0 {
		return c.brokers, nil
	}
	if url := op.Source[SourceServerURL]; url != "" {
		return []string{url}, nil
	}
	return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
		"asyncapi: operation %q has no broker address: pass brokers to the client or select a server in the document", op.ID))
}

// sasl builds the kafka-go mechanism from the mapped credentials
// (PROTO-007 §5). Credentials never appear in errors or logs.
func (c *Client) sasl() (sasl.Mechanism, error) {
	switch c.saslMechanism {
	case "":
		return nil, nil
	case SASLMechanismPlain:
		return plain.Mechanism{Username: c.saslUsername, Password: c.saslPassword}, nil
	case SASLMechanismScramSHA256:
		return scram.Mechanism(scram.SHA256, c.saslUsername, c.saslPassword)
	case SASLMechanismScramSHA512:
		return scram.Mechanism(scram.SHA512, c.saslUsername, c.saslPassword)
	default:
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"asyncapi: unsupported SASL mechanism %q (supported: plain, scram-sha256, scram-sha512)", c.saslMechanism))
	}
}

// transport assembles the kafka-go transport carrying SASL/TLS material.
func (c *Client) transport() (sasl.Mechanism, *kafka.Transport, error) {
	mech, err := c.sasl()
	if err != nil {
		return nil, nil, err
	}
	tr := &kafka.Transport{SASL: mech, TLS: c.tlsCfg}
	return mech, tr, nil
}

// Publish executes one send operation: a single bounded produce with
// acknowledgement metadata as the result (PROTO-007 §2.2 decision D2).
func (c *Client) Publish(ctx context.Context, op *protocol.Operation, args map[string]interface{}) (interface{}, error) {
	if err := refuseGroupBinding(op); err != nil {
		return nil, err
	}
	inv, err := buildInvocation(op, args)
	if err != nil {
		return nil, err
	}
	brokers, err := c.brokersFor(op)
	if err != nil {
		return nil, err
	}

	// Composition order (§4.3): rate limit → auth (mapped above) →
	// timeout/deadline → single attempt. Publishes are KindUnary and
	// idempotency-gated: no automatic retries are layered here beyond
	// kafka-go's own leader retry on the writer.
	if _, ok := ctx.Deadline(); !ok && c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	_, transport, err := c.transport()
	if err != nil {
		return nil, err
	}
	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        inv.topic,
		Balancer:     &kafka.RoundRobin{},
		RequiredAcks: kafka.RequireOne,
		Transport:    transport,
		// One message per invocation: flush immediately instead of
		// aggregating over the kafka-go default 1s batch window.
		BatchTimeout: 10 * time.Millisecond,
		// The document decides whether the topic exists; the producer does
		// not paper over a missing topic.
		AllowAutoTopicCreation: false,
	}
	defer func() { _ = writer.Close() }()

	// The writer owns the topic stamp; kafka-go refuses a message that
	// names it as well.
	msg := kafka.Message{
		Key:     inv.key,
		Value:   inv.payload,
		Headers: inv.headers,
		Time:    clock(),
	}
	if err := writer.WriteMessages(ctx, msg); err != nil {
		return nil, mapKafkaError(err, fmt.Sprintf("publish operation %q", op.ID))
	}

	result := map[string]interface{}{
		"topic":         inv.topic,
		"partition":     msg.Partition,
		"offset":        msg.Offset,
		"message_count": 1,
		"key_present":   len(inv.key) > 0,
		"header_count":  len(inv.headers),
		"ack":           "leader",
	}
	return result, nil
}

// refuseGroupBinding enforces the declared binding subset (§9.2): a
// receive operation whose Kafka binding requires consumer-group semantics
// (groupId) is rejected instead of silently ignoring the field.
func refuseGroupBinding(op *protocol.Operation) error {
	for _, key := range []string{SourceOpBinding, SourceChannelBinding, SourceServerBinding} {
		raw := op.Source[key]
		if raw == "" || !strings.Contains(raw, "kafka") {
			continue
		}
		var binding struct {
			Kafka map[string]interface{} `json:"kafka"`
		}
		if err := json.Unmarshal([]byte(raw), &binding); err != nil {
			continue // unparsable binding JSON is preserved verbatim, not enacted
		}
		if _, requiresGroup := binding.Kafka["groupId"]; requiresGroup {
			return errors.New(errors.CodeValidation, fmt.Sprintf(
				"asyncapi: operation %q: its %s binding declares kafka groupId; consumer groups are not part of this transport's executable subset (bounded collect runs without group state; durable group consumption is out of scope per PROTO-007 §2.3)",
				op.ID, key))
		}
	}
	return nil
}

// partitionSnapshot is the per-partition offset state taken at collect
// start: [first, last) is the window the drain rule tracks.
type partitionSnapshot struct {
	first int64
	last  int64
}

// Consume executes one receive operation as a bounded collect
// (PROTO-007 §4): subscribe, collect up to the bounds, close every
// subscription, return the EventTranscript. On the cancelled and error
// endings the partial transcript comes back alongside the mapped AppError.
func (c *Client) Consume(ctx context.Context, op *protocol.Operation, args map[string]interface{}) (interface{}, error) {
	if err := refuseGroupBinding(op); err != nil {
		return nil, err
	}
	// Only channel parameters are inputs of a receive (message headers
	// describe the incoming envelope, not the invocation).
	inv, err := buildInvocation(op, args)
	if err != nil {
		return nil, err
	}
	brokers, err := c.brokersFor(op)
	if err != nil {
		return nil, err
	}

	limits := c.limits.normalize()
	if inv.requestedMaxMessages > 0 && inv.requestedMaxMessages < limits.MaxMessages {
		limits.MaxMessages = inv.requestedMaxMessages
	}
	if inv.requestedTimeoutSeconds > 0 && time.Duration(inv.requestedTimeoutSeconds)*time.Second < limits.MaxDuration {
		limits.MaxDuration = time.Duration(inv.requestedTimeoutSeconds) * time.Second
	}
	transcript := newEventTranscript(limits)

	// Partition + offset discovery: metadata and ListOffsets over one
	// client. Unknown topics surface here as typed broker errors.
	mech, transport, err := c.transport()
	if err != nil {
		return nil, err
	}
	addr := brokers[0]
	kc := &kafka.Client{
		Addr:      kafka.TCP(addr),
		Timeout:   10 * time.Second,
		Transport: transport,
	}
	partitions, err := topicPartitions(ctx, kc, inv.topic)
	if err != nil {
		return nil, mapKafkaError(err, fmt.Sprintf("consume operation %q (metadata)", op.ID))
	}

	offsets, err := offsetSnapshot(ctx, kc, inv.topic, partitions)
	if err != nil {
		return nil, mapKafkaError(err, fmt.Sprintf("consume operation %q (offsets)", op.ID))
	}

	tracker := newDrainTracker(
		snapshotMap(offsets, func(p partitionSnapshot) int64 { return p.first }),
		snapshotMap(offsets, func(p partitionSnapshot) int64 { return p.last }),
	)

	dialer := &kafka.Dialer{
		Timeout:       10 * time.Second,
		TLS:           c.tlsCfg,
		SASLMechanism: mech,
	}
	readers := make([]*kafka.Reader, 0, len(partitions))
	for _, p := range partitions {
		r := kafka.NewReader(kafka.ReaderConfig{
			Brokers:       brokers,
			Topic:         inv.topic,
			Partition:     p,
			Dialer:        dialer,
			MinBytes:      1,
			MaxBytes:      10 << 20,
			MaxWait:       time.Second, // bounded long-poll so closes stay prompt
			QueueCapacity: 64,
		})
		readers = append(readers, r)
	}

	// The collect owns one cancellable context: every ending (bound fired,
	// drained, caller cancel, broker error) cancels it, releasing the
	// readers before the envelope returns (§4: never leave a subscription
	// open behind a returned tool call).
	collectCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type delivered struct {
		item interface{}
		part int32
		off  int64
	}
	itemsCh := make(chan delivered, 256)
	errCh := make(chan error, len(readers))

	var wg sync.WaitGroup
	for i, r := range readers {
		wg.Add(1)
		go func(r *kafka.Reader, p int32) {
			defer wg.Done()
			for {
				m, err := r.FetchMessage(collectCtx)
				if err != nil {
					// collectCtx cancellation is the collect's own stop,
					// not a broker failure.
					if collectCtx.Err() != nil {
						return
					}
					select {
					case errCh <- err:
					case <-collectCtx.Done():
					}
					return
				}
				d := delivered{item: messageItem(m), part: int32(p), off: m.Offset}
				select {
				case itemsCh <- d:
				case <-collectCtx.Done():
					return
				}
			}
		}(r, int32(partitions[i]))
	}
	// Close every reader once the collect stops; never leak a subscription.
	defer func() {
		cancel()
		wg.Wait()
		for _, r := range readers {
			_ = r.Close()
		}
	}()

	started := clock()
	timer := time.NewTimer(limits.MaxDuration)
	defer timer.Stop()

	// Empty snapshot window: the collect completes immediately with zero
	// items (PROTO-007 §9.2 empty-receive-window rule).
	drained := tracker.drainedAtStart()
	var failure error
	bound := ""

loop:
	for {
		switch {
		case drained || failure != nil || bound != "":
			break loop
		case len(transcript.Items) >= limits.MaxMessages:
			bound = LimitReasonMessages
			break loop
		}
		select {
		case d := <-itemsCh:
			transcript.Items = append(transcript.Items, d.item)
			drained = tracker.record(d.part, d.off)
		case err := <-errCh:
			failure = err
			break loop
		case <-timer.C:
			bound = LimitReasonDuration
			break loop
		case <-ctx.Done():
			// The CALLER ended the call: release everything and report the
			// cancellation (mapped CodeTimeout per §4.2/§6).
			cancel()
			transcript.finish(limits, false, "", nil, true, started)
			return transcript, mapKafkaError(ctx.Err(), fmt.Sprintf("consume operation %q", op.ID))
		}
	}

	transcript.finish(limits, drained, bound, failure, false, started)
	if failure != nil {
		return transcript, mapKafkaError(failure, fmt.Sprintf("consume operation %q", op.ID))
	}
	return transcript, nil
}

// messageItem renders one Kafka message as the transcript item: payload
// decoded as JSON when possible, with honest encodings otherwise
// (PROTO-007 §9.2: report unsupported shapes explicitly, never silently).
func messageItem(m kafka.Message) map[string]interface{} {
	item := map[string]interface{}{
		"topic":     m.Topic,
		"partition": m.Partition,
		"offset":    m.Offset,
	}
	if !m.Time.IsZero() {
		item["timestamp_ms"] = m.Time.UnixMilli()
	}
	if len(m.Key) > 0 {
		item["key"] = wireString(m.Key)
	} else {
		item["key"] = nil
	}
	if len(m.Headers) > 0 {
		headers := map[string]string{}
		for _, h := range m.Headers {
			headers[h.Key] = string(h.Value)
		}
		item["headers"] = headers
	}
	switch {
	case len(m.Value) == 0:
		item["payload"] = nil
		item["payload_encoding"] = "empty"
	case utf8.Valid(m.Value):
		var decoded interface{}
		if err := json.Unmarshal(m.Value, &decoded); err == nil {
			item["payload"] = decoded
			item["payload_encoding"] = "json"
		} else {
			item["payload"] = string(m.Value)
			item["payload_encoding"] = "utf8"
		}
	default:
		item["payload"] = base64.StdEncoding.EncodeToString(m.Value)
		item["payload_encoding"] = "base64"
	}
	return item
}

// wireString renders wire bytes for JSON output: text when valid UTF-8,
// base64 otherwise.
func wireString(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// topicPartitions returns the partition IDs of a topic from broker
// metadata. Unknown topics surface as typed broker errors.
func topicPartitions(ctx context.Context, kc *kafka.Client, topic string) ([]int, error) {
	res, err := kc.Metadata(ctx, &kafka.MetadataRequest{Topics: []string{topic}})
	if err != nil {
		return nil, err
	}
	for _, t := range res.Topics {
		if t.Name != topic {
			continue
		}
		if t.Error != nil {
			return nil, t.Error
		}
		ids := make([]int, 0, len(t.Partitions))
		for _, p := range t.Partitions {
			ids = append(ids, p.ID)
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("topic %q has no partitions", topic)
		}
		return ids, nil
	}
	return nil, kafka.Error(kafka.UnknownTopicOrPartition)
}

// offsetSnapshot reads [first, last) for every partition in one
// ListOffsets round (first and last timestamps per partition).
func offsetSnapshot(ctx context.Context, kc *kafka.Client, topic string, partitions []int) (map[int32]partitionSnapshot, error) {
	reqs := make([]kafka.OffsetRequest, 0, len(partitions)*2)
	for _, p := range partitions {
		reqs = append(reqs, kafka.FirstOffsetOf(p), kafka.LastOffsetOf(p))
	}
	res, err := kc.ListOffsets(ctx, &kafka.ListOffsetsRequest{
		Topics: map[string][]kafka.OffsetRequest{topic: reqs},
	})
	if err != nil {
		return nil, err
	}
	out := map[int32]partitionSnapshot{}
	for _, po := range res.Topics[topic] {
		if po.Error != nil {
			return nil, po.Error
		}
		// Each response entry answers ONE timestamp query: kafka-go fills
		// exactly one of the two fields, leaving the other zero. A zero
		// answer is indistinguishable from the correct zero default (empty
		// window starts at 0), so plain non-zero assignment merges both
		// entries correctly — including truncated logs whose real first
		// offset is > 0.
		snap := out[int32(po.Partition)]
		if po.FirstOffset != 0 {
			snap.first = po.FirstOffset
		}
		if po.LastOffset != 0 {
			snap.last = po.LastOffset
		}
		out[int32(po.Partition)] = snap
	}
	return out, nil
}

// snapshotMap projects a snapshot into the per-partition value map the
// drain tracker consumes.
func snapshotMap(m map[int32]partitionSnapshot, pick func(partitionSnapshot) int64) map[int32]int64 {
	out := make(map[int32]int64, len(m))
	for p, s := range m {
		out[p] = pick(s)
	}
	return out
}
