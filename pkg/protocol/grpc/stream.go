package grpc

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	legacyproto "github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"google.golang.org/grpc/metadata"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// ─── Bounded output contract (specs/PROTOCOLS.md §4.5) ───────────────────
//
// A streaming invocation NEVER returns a live stream. An unbounded stream
// handed to a caller (a CLI invocation, an MCP tool call, an agent loop) can
// hang the session forever, so every streaming kind returns a StreamTranscript
// envelope whose collection is bounded in two independent ways:
//
//   - MaxMessages caps how many RESPONSE items are collected;
//   - MaxDuration caps the wall clock of the whole call (sending + collecting).
//
// Hitting either bound is a CLEAN stop: the stream is cancelled locally, the
// transcript reports status "truncated" with items_truncated true and the
// trailer names which bound fired. No error is returned — the caller asked for
// a bounded answer and got one.
//
// A CALLER-side cancellation (context cancel or deadline on the context
// passed to Execute/ExecuteMessages) is a different thing: it is a real
// failure, so the transcript reports status "cancelled" and the mapped
// AppError (CodeTimeout, per §4.2/§4.3 — identical to the unary path) comes
// back alongside the partial transcript.
//
// An error that arrives from the server mid-stream terminates the transcript
// with status "error" and the §4.2-mapped AppError; the items collected so far
// are preserved in the returned transcript.

const (
	// DefaultMaxMessages caps how many response items one streaming
	// invocation may collect. Large enough for a real fan-out query, small
	// enough that the printed transcript stays readable.
	DefaultMaxMessages = 1000

	// DefaultMaxDuration caps the wall clock of one streaming invocation
	// (send + collect). It matches the bounded default the JSON-RPC adapter
	// applies to interactive CLI calls.
	DefaultMaxDuration = 30 * time.Second
)

// Transcript status vocabulary (§4.5). "complete" and "truncated" are normal
// endings; "cancelled" and "error" are failures that also carry an AppError.
const (
	StatusComplete  = "complete"
	StatusTruncated = "truncated"
	StatusCancelled = "cancelled"
	StatusError     = "error"
)

// Limit reasons reported in StreamTranscript.Trailer["limit_reason"] when a
// configured bound stopped the stream.
const (
	LimitReasonMessages = "max-messages"
	LimitReasonDuration = "max-duration"
)

// StreamLimits carries the per-invocation bounds of the streaming contract.
// Zero or negative fields select the package defaults: there is deliberately
// no "unbounded" setting, because an unbounded stream is exactly what §4.5
// exists to prevent.
type StreamLimits struct {
	// MaxMessages caps collected response items (and the request stream
	// length for client-streaming/bidi methods).
	MaxMessages int

	// MaxDuration caps the wall clock of the whole invocation.
	MaxDuration time.Duration
}

// normalize applies the documented defaults to unset bounds.
func (l StreamLimits) normalize() StreamLimits {
	if l.MaxMessages <= 0 {
		l.MaxMessages = DefaultMaxMessages
	}
	if l.MaxDuration <= 0 {
		l.MaxDuration = DefaultMaxDuration
	}
	return l
}

// StreamTranscript is the bounded transcript envelope every streaming
// invocation returns (§4.5 rule 1). It is JSON-serializable so the shared
// output rendering (§4.4) consumes it like any other result.
type StreamTranscript struct {
	// Kind is one of "server-stream", "client-stream", "bidi-stream".
	Kind string `json:"kind"`

	// Status is one of the Status* constants.
	Status string `json:"status"`

	// Items holds the decoded response messages (never null in JSON).
	Items []interface{} `json:"items"`

	// ItemsTruncated is true when a bound stopped the collection early
	// (or, for client-streaming, capped the request stream).
	ItemsTruncated bool `json:"items_truncated"`

	// Trailer carries the adapter-defined final metadata: received,
	// messages_sent, max_messages, duration_ms, plus limit_reason,
	// metadata (the gRPC trailer) and error when applicable.
	Trailer map[string]interface{} `json:"trailer"`
}

// newTranscript starts an envelope with the effective limits recorded, so the
// caller can tell which bound applied even on a complete stream.
func newTranscript(kind protocol.OperationKind, limits StreamLimits) *StreamTranscript {
	return &StreamTranscript{
		Kind:   streamKindLabel(kind),
		Status: StatusComplete,
		Items:  []interface{}{},
		Trailer: map[string]interface{}{
			"max_messages": limits.MaxMessages,
			"received":     0,
		},
	}
}

// streamKindLabel renders the §4.5 envelope's kind field.
func streamKindLabel(kind protocol.OperationKind) string {
	switch kind {
	case protocol.KindServerStream:
		return "server-stream"
	case protocol.KindClientStream:
		return "client-stream"
	case protocol.KindBidiStream:
		return "bidi-stream"
	default:
		return "unary"
	}
}

// streamBound records which configured bound ended a stream so the adapter can
// tell its own deliberate stop (a clean truncation) from the caller cancelling
// the context (a real failure). The first bound to fire wins.
type streamBound struct {
	mu     sync.Mutex
	reason string
}

// mark records reason if no bound has fired yet and reports whether this call
// was the one that claimed it.
func (b *streamBound) mark(reason string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.reason != "" {
		return false
	}
	b.reason = reason
	return true
}

// hit returns the bound that fired, or "" when the stream ended on its own.
func (b *streamBound) hit() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.reason
}

// beginStream derives the cancellable context of one streaming invocation and
// arms the wall-clock bound. stop releases both the timer and the stream and
// is safe to call more than once.
func (a *Adapter) beginStream(ctx context.Context) (context.Context, *streamBound, func()) {
	limits := a.limits.normalize()
	streamCtx, cancel := context.WithCancel(ctx)
	bound := &streamBound{}
	timer := time.AfterFunc(limits.MaxDuration, func() {
		// Only the timer that actually claims the bound cancels the stream;
		// a stream that already ended must not be cancelled retroactively
		// (that would relabel a complete stream after the fact).
		if bound.mark(LimitReasonDuration) {
			cancel()
		}
	})
	stop := func() {
		timer.Stop()
		cancel()
	}
	return streamCtx, bound, stop
}

// messageReceiver is the receive side shared by server-streaming and bidi
// streams (grpcdynamic exposes the same shape for both).
type messageReceiver interface {
	RecvMsg() (legacyproto.Message, error)
}

// requestSender is the send side shared by client-streaming and bidi streams.
type requestSender interface {
	SendMsg(m legacyproto.Message) error
}

// streamOutcome is what the collection loop observed when the stream stopped.
type streamOutcome struct {
	// err is non-nil when the stream did not end normally (io.EOF and a
	// claimed bound are normal endings).
	err error

	// decodeFailure distinguishes a response that could not be decoded from
	// a transport failure, so the right §4.2 mapper runs.
	decodeFailure bool
}

// collectBounded drains a response stream into the transcript until the stream
// ends, the item cap is claimed, or the stream context is done. The cap is
// checked before every receive, so a capped stream never blocks on a receive
// the caller no longer wants.
func collectBounded(recv messageReceiver, t *StreamTranscript, limits StreamLimits, bound *streamBound, stop func()) streamOutcome {
	for {
		if len(t.Items) >= limits.MaxMessages {
			// We already hold every item the caller asked for: cancel the
			// stream and stop. This is a clean stop, not a failure. The
			// cancel is unconditional — whoever claimed the bound, a cap
			// reached here must not leave a live stream behind.
			bound.mark(LimitReasonMessages)
			stop()
			return streamOutcome{}
		}
		msg, err := recv.RecvMsg()
		if err != nil {
			if err == io.EOF {
				return streamOutcome{}
			}
			return streamOutcome{err: err}
		}
		item, decErr := decodeResult(msg)
		if decErr != nil {
			return streamOutcome{err: decErr, decodeFailure: true}
		}
		t.Items = append(t.Items, item)
	}
}

// sendRequests writes the request stream, capped at limits.MaxMessages. The cap
// is tested before each send, so a capped stream is never a partial extra
// message, and the SEND side is the only thing the cap stops: a client-streaming
// invocation still closes the send side and collects the server's answer, and a
// bidi invocation keeps collecting. Backpressure is the transport's: SendMsg
// blocks on gRPC flow control when the server is slow instead of buffering the
// stream in memory.
func sendRequests(sender requestSender, msgs []*dynamic.Message, limits StreamLimits, bound *streamBound) (int, error) {
	sent := 0
	for _, m := range msgs {
		if sent >= limits.MaxMessages {
			bound.mark(LimitReasonMessages)
			return sent, nil
		}
		if err := sender.SendMsg(m); err != nil {
			return sent, err
		}
		sent++
	}
	return sent, nil
}

// finishStream stamps the terminal status and trailer of a streaming
// invocation and returns the AppError that must accompany the transcript (nil
// for the two clean endings). callerCtx is the caller's own context: when it is
// done, the stop was a cancellation, not a bound we imposed.
func finishStream(t *StreamTranscript, bound *streamBound, out streamOutcome, callerCtx context.Context, fullMethod string, started time.Time, sent int) error {
	t.Trailer["received"] = len(t.Items)
	t.Trailer["messages_sent"] = sent
	t.Trailer["duration_ms"] = time.Since(started).Milliseconds()

	if reason := bound.hit(); reason != "" {
		t.Status = StatusTruncated
		t.ItemsTruncated = true
		t.Trailer["limit_reason"] = reason
		return nil
	}
	if out.err == nil {
		t.Status = StatusComplete
		return nil
	}
	t.Trailer["error"] = out.err.Error()
	if callerCtx != nil && callerCtx.Err() != nil {
		t.Status = StatusCancelled
		return mapTransportError(out.err, fullMethod)
	}
	t.Status = StatusError
	if out.decodeFailure {
		return mapDecodeError(out.err, fullMethod)
	}
	return mapTransportError(out.err, fullMethod)
}

// recordTrailer copies the gRPC trailer metadata into the envelope. It is only
// meaningful once the stream has ended with an error (io.EOF or a status), so
// callers invoke it exactly then; a locally truncated stream may carry none.
func recordTrailer(t *StreamTranscript, md metadata.MD) {
	if len(md) == 0 {
		return
	}
	t.Trailer["metadata"] = md
}

// buildMessages converts JSON-style seed maps into dynamic request messages.
// An empty seed yields an empty message list, which is legal: the caller closes
// the send side immediately.
func buildMessages(input *desc.MessageDescriptor, opID string, seed []map[string]interface{}) ([]*dynamic.Message, error) {
	msgs := make([]*dynamic.Message, 0, len(seed))
	for i, s := range seed {
		m, err := buildRequest(input, opID, s)
		if err != nil {
			return nil, errors.Wrap(err, errors.CodeValidation,
				fmt.Sprintf("request message %d of %d", i+1, len(seed)))
		}
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// executeServerStream invokes a server-streaming method and returns the bounded
// transcript (§4.5). The single request message comes from the seed (zero or
// one entry); the response side is collected up to MaxMessages within
// MaxDuration.
func (a *Adapter) executeServerStream(ctx context.Context, method *desc.MethodDescriptor, fullMethod, opID string, seed []map[string]interface{}) (interface{}, error) {
	limits := a.limits.normalize()
	if len(seed) > 1 {
		return nil, errors.Errorf(errors.CodeValidation,
			"server-streaming method %s takes exactly one request message, got %d (pass --arg key=value)", fullMethod, len(seed))
	}

	var reqMsg *dynamic.Message
	if len(seed) == 1 {
		var err error
		reqMsg, err = buildRequest(method.GetInputType(), opID, seed[0])
		if err != nil {
			return nil, err
		}
	} else {
		reqMsg = dynamic.NewMessage(method.GetInputType())
	}

	started := time.Now()
	streamCtx, bound, stop := a.beginStream(ctx)
	defer stop()

	stream, err := grpcdynamic.NewStub(a.conn).InvokeRpcServerStream(streamCtx, method, reqMsg)
	if err != nil {
		return nil, mapTransportError(err, fullMethod)
	}

	transcript := newTranscript(protocol.KindServerStream, limits)
	out := collectBounded(stream, transcript, limits, bound, stop)
	if out.err != nil {
		recordTrailer(transcript, stream.Trailer())
	}
	if err := finishStream(transcript, bound, out, ctx, fullMethod, started, 1); err != nil {
		return transcript, err
	}
	return transcript, nil
}

// executeClientStream invokes a client-streaming method: the whole request
// stream is sent, the send side closed, and the single response collected,
// all inside the same bound. A request stream longer than MaxMessages is
// capped (status truncated, items_truncated true) and the response is still
// collected, because the server did answer the capped stream completely.
func (a *Adapter) executeClientStream(ctx context.Context, method *desc.MethodDescriptor, fullMethod, opID string, seed []map[string]interface{}) (interface{}, error) {
	limits := a.limits.normalize()
	msgs, err := buildMessages(method.GetInputType(), opID, seed)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	streamCtx, bound, stop := a.beginStream(ctx)
	defer stop()

	stream, err := grpcdynamic.NewStub(a.conn).InvokeRpcClientStream(streamCtx, method)
	if err != nil {
		return nil, mapTransportError(err, fullMethod)
	}

	transcript := newTranscript(protocol.KindClientStream, limits)
	sent, sendErr := sendRequests(stream, msgs, limits, bound)

	var out streamOutcome
	switch {
	case sendErr != nil:
		// A failure while sending is only clean when one of our bounds
		// stopped the stream; otherwise it is a broken stream.
		if bound.hit() == "" {
			out = streamOutcome{err: sendErr}
		}
	default:
		resp, recvErr := stream.CloseAndReceive()
		switch {
		case recvErr != nil:
			out = streamOutcome{err: recvErr}
		default:
			item, decErr := decodeResult(resp)
			if decErr != nil {
				out = streamOutcome{err: decErr, decodeFailure: true}
			} else {
				transcript.Items = append(transcript.Items, item)
			}
		}
	}
	if out.err != nil {
		recordTrailer(transcript, stream.Trailer())
	}
	if err := finishStream(transcript, bound, out, ctx, fullMethod, started, sent); err != nil {
		return transcript, err
	}
	return transcript, nil
}

// executeBidiStream invokes a bidirectional method. Send and receive run
// concurrently: a server that only produces while being read (the common echo /
// chat shape) would deadlock a send-everything-then-read implementation, and a
// server that is slower than the client applies backpressure through gRPC flow
// control instead of unbounded client buffering. The collector stops the sender
// the moment the bound is claimed or the stream ends.
func (a *Adapter) executeBidiStream(ctx context.Context, method *desc.MethodDescriptor, fullMethod, opID string, seed []map[string]interface{}) (interface{}, error) {
	limits := a.limits.normalize()
	msgs, err := buildMessages(method.GetInputType(), opID, seed)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	streamCtx, bound, stop := a.beginStream(ctx)
	defer stop()

	stream, err := grpcdynamic.NewStub(a.conn).InvokeRpcBidiStream(streamCtx, method)
	if err != nil {
		return nil, mapTransportError(err, fullMethod)
	}

	transcript := newTranscript(protocol.KindBidiStream, limits)

	var sentCount atomic.Int64
	sendDone := make(chan error, 1)
	go func() {
		sent, sendErr := sendRequests(stream, msgs, limits, bound)
		sentCount.Store(int64(sent))
		if sendErr != nil {
			sendDone <- sendErr
			return
		}
		sendDone <- stream.CloseSend()
	}()

	out := collectBounded(stream, transcript, limits, bound, stop)
	// Release a sender still blocked on flow control, then wait for it so no
	// goroutine outlives the invocation.
	stop()
	sendErr := <-sendDone
	if out.err == nil && sendErr != nil && bound.hit() == "" {
		out = streamOutcome{err: sendErr}
	}
	if out.err != nil {
		recordTrailer(transcript, stream.Trailer())
	}
	if err := finishStream(transcript, bound, out, ctx, fullMethod, started, int(sentCount.Load())); err != nil {
		return transcript, err
	}
	return transcript, nil
}
