package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// ─── Bounded-collect subscriptions (specs/PROTOCOLS.md §4.5(4), §2 row 6) ──
//
// A GraphQL subscription is a KindEventSource operation: it has no natural
// end, so — exactly like gRPC streaming (pkg/protocol/grpc/stream.go) and the
// AsyncAPI consume path — an invocation NEVER returns a live subscription. The
// call subscribes, collects items up to the caps, unsubscribes, and returns a
// SubscriptionTranscript envelope. Hitting a bound is a CLEAN stop: the
// client unsubscribes and the transcript reports status "truncated" with
// items_truncated true and a limit_reason naming which bound fired. No error
// is returned — the caller asked for a bounded answer and got one.
//
// A CALLER-side cancellation (context cancel or deadline on the caller's
// context) is a real failure: the transcript reports "cancelled" and the
// mapped AppError (CodeTimeout, mirroring the gRPC streaming path) comes back
// alongside the partial transcript.
//
// A server-sent graphql-transport-ws "error" message, or a failure to decode
// an item, terminates the transcript with status "error" and the §4.2-mapped
// AppError; items collected so far are preserved.
//
// Transport (§4.7 applied to the WS dialer): subscriptions are not an HTTP
// version, so like every non-HTTP transport the WS dialer is its own dialer —
// but it is the ONLY new transport surface: discovery, schema ingestion, the
// unary POST call path and the shared composition order (§4.3: rate limit →
// deadline → single attempt) are untouched. The WS URL is derived from the
// SAME endpoint Execute posts to (http(s):// → ws(s)://), never a second
// endpoint, and the same Client's rate limiter guards the dial.

const (
	// DefaultSubscriptionMaxMessages caps how many events one bounded
	// collect gathers. Same default as the gRPC streaming contract (§4.5:
	// 1000 messages / 30 s).
	DefaultSubscriptionMaxMessages = 1000

	// DefaultSubscriptionDuration caps the wall clock of one bounded collect
	// (dial + subscribe + collect + unsubscribe). Same default as the gRPC
	// streaming contract.
	DefaultSubscriptionDuration = 30 * time.Second

	// WSSubprotocol is the graphql-transport-ws subprotocol this client
	// offers on every dial. Only servers speaking that subprotocol are
	// usable; a server answering without it fails the handshake loudly.
	WSSubprotocol = "graphql-transport-ws"
)

// Subscription transcript status vocabulary — the same words as the gRPC
// StreamTranscript and the AsyncAPI EventTranscript (§4.5).
const (
	SubStatusComplete  = "complete"
	SubStatusTruncated = "truncated"
	SubStatusCancelled = "cancelled"
	SubStatusError     = "error"
)

// Limit reasons reported in SubscriptionTranscript.Trailer["limit_reason"].
// The max-messages spelling mirrors pkg/protocol/grpc's envelope (§4.5
// implementation note), not the AsyncAPI "max-items" one.
const (
	LimitReasonMessages = "max-messages"
	LimitReasonDuration = "max-duration"
)

// SubscriptionLimits carries the per-invocation bounds of the
// bounded-collect contract. Zero or negative fields select the package
// defaults: there is deliberately no "unbounded" setting, because an
// unbounded subscription is exactly what §4.5(4) exists to prevent.
type SubscriptionLimits struct {
	// MaxMessages caps collected events.
	MaxMessages int

	// MaxDuration caps the wall clock of the whole invocation (dial +
	// subscribe + collect + unsubscribe).
	MaxDuration time.Duration
}

// normalize applies the documented defaults to unset bounds.
func (l SubscriptionLimits) normalize() SubscriptionLimits {
	if l.MaxMessages <= 0 {
		l.MaxMessages = DefaultSubscriptionMaxMessages
	}
	if l.MaxDuration <= 0 {
		l.MaxDuration = DefaultSubscriptionDuration
	}
	return l
}

// SubscriptionTranscript is the bounded transcript envelope every
// subscription invocation returns (§4.5 rule 1). It is JSON-serializable so
// the shared output rendering (§4.4) consumes it like any other result.
type SubscriptionTranscript struct {
	// Kind is always "subscription".
	Kind string `json:"kind"`

	// Status is one of the SubStatus* constants.
	Status string `json:"status"`

	// Items holds the decoded event payloads (never null in JSON).
	Items []interface{} `json:"items"`

	// ItemsTruncated is true when a bound stopped the collection early.
	ItemsTruncated bool `json:"items_truncated"`

	// Trailer carries the final metadata: received, max_messages,
	// duration_ms, plus limit_reason and error when applicable.
	Trailer map[string]interface{} `json:"trailer"`
}

// newSubscriptionTranscript starts an envelope with the effective limits
// recorded, so the caller can tell which bound applied even on a complete
// collect.
func newSubscriptionTranscript(limits SubscriptionLimits) *SubscriptionTranscript {
	return &SubscriptionTranscript{
		Kind:   "subscription",
		Status: SubStatusComplete,
		Items:  []interface{}{},
		Trailer: map[string]interface{}{
			"max_messages": limits.MaxMessages,
			"received":     0,
		},
	}
}

// finish stamps the terminal status and trailer of a bounded collect.
//
// bounded names the limit that fired ("" when none did), failure is the
// error that stopped the collect (nil for a natural or bounded end),
// callerEnded reports the caller's own context being done, and
// unsubscribed records whether the client's complete message went out
// (the clean-unsubscribe evidence the transcript reports).
func (t *SubscriptionTranscript) finish(limits SubscriptionLimits, bounded string, failure error, callerEnded bool, unsubscribed bool, started time.Time) error {
	t.Trailer["received"] = len(t.Items)
	t.Trailer["duration_ms"] = time.Since(started).Milliseconds()
	t.Trailer["unsubscribed"] = unsubscribed

	switch {
	case bounded != "":
		t.Status = SubStatusTruncated
		t.ItemsTruncated = true
		t.Trailer["limit_reason"] = bounded
		return nil
	case failure == nil:
		t.Status = SubStatusComplete
		return nil
	}
	t.Trailer["error"] = failure.Error()
	if callerEnded {
		t.Status = SubStatusCancelled
		return errors.Wrap(failure, errors.CodeTimeout, "graphql: subscription collect cancelled")
	}
	t.Status = SubStatusError
	return failure
}

// graphql-transport-ws message frame (server and client share the shape).
type wsFrame struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// graphql-transport-ws message types (protocol 1.0).
const (
	wsTypeConnectionInit  = "connection_init"
	wsTypeConnectionAck   = "connection_ack"
	wsTypeConnectionError = "connection_error"
	wsTypeSubscribe       = "subscribe"
	wsTypeNext            = "next"
	wsTypeError           = "error"
	wsTypeComplete        = "complete"
)

// subscriptionCall is one bounded-collect invocation's live connection state.
type subscriptionCall struct {
	conn     *websocket.Conn
	callCtx  context.Context
	caller   context.Context
	deadline time.Time
}

// write sends one frame before the wall clock runs out.
func (c *subscriptionCall) write(f wsFrame) error {
	_ = c.conn.SetWriteDeadline(c.deadline)
	return c.conn.WriteJSON(f)
}

// Subscribe performs the bounded collect of one subscription operation
// (specs/PROTOCOLS.md §4.5(4)): subscribe, collect up to the caps,
// unsubscribe, return the transcript. Hitting a cap is a clean stop, not an
// error; a caller cancellation or a server-side failure returns the partial
// transcript alongside the mapped AppError.
func (c *Client) Subscribe(ctx context.Context, opID string, args map[string]interface{}, limits SubscriptionLimits) (interface{}, error) {
	if strings.TrimSpace(opID) == "" {
		return nil, errors.New(errors.CodeValidation, "graphql: operation ID is required")
	}
	if err := c.ensureSchema(ctx); err != nil {
		return nil, err
	}

	op := OperationByID(c.ops, opID)
	if op == nil {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: operation %q is not in the discovered schema (discovered: %s)",
			opID, strings.Join(OperationIDs(c.ops), ", ")))
	}
	if op.Kind != protocol.KindEventSource {
		return nil, errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: operation %q is not a subscription (KindEventSource); use Execute for unary operations",
			opID))
	}

	req, err := c.builder.Build(op, args, c.endpoint)
	if err != nil {
		return nil, err
	}
	wsURL, err := WebsocketURL(req.Endpoint)
	if err != nil {
		return nil, err
	}

	limits = limits.normalize()
	started := time.Now()

	// §4.3 composition order: rate limit first.
	if c.rate != nil && !c.rate.Allow() {
		return nil, errors.New(errors.CodeRateLimit, "graphql: local rate limit refused the call")
	}

	// The wall-clock bound is the invocation's own deadline layered over the
	// caller's: when the CALLER's context ends first that is a cancellation
	// (a real failure), never a truncation.
	callCtx, cancel := context.WithTimeout(ctx, limits.MaxDuration)
	defer cancel()

	transcript := newSubscriptionTranscript(limits)

	dialer := &websocket.Dialer{
		Subprotocols:     []string{WSSubprotocol},
		HandshakeTimeout: limits.MaxDuration,
		Proxy:            http.ProxyFromEnvironment,
	}
	conn, resp, dialErr := dialer.DialContext(callCtx, wsURL, nil)
	if dialErr != nil {
		if callerEndedTheCall(ctx) {
			return transcript, errors.Wrap(dialErr, errors.CodeTimeout, "graphql: subscription dial cancelled")
		}
		if callCtx.Err() != nil {
			// Our own wall clock fired before the dial completed.
			transcript.Trailer["received"] = 0
			transcript.Status = SubStatusTruncated
			transcript.ItemsTruncated = true
			transcript.Trailer["limit_reason"] = LimitReasonDuration
			transcript.Trailer["duration_ms"] = time.Since(started).Milliseconds()
			return transcript, nil
		}
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return nil, errors.Wrap(fmt.Errorf("HTTP %s", resp.Status), errors.CodeUnauthorized,
				"graphql: endpoint refused the websocket upgrade")
		}
		detail := "graphql: websocket dial failed"
		if resp != nil {
			detail = fmt.Sprintf("%s (endpoint answered HTTP %s)", detail, resp.Status)
		}
		return nil, errors.Wrap(dialErr, errors.CodeGeneric, detail)
	}
	defer func() { _ = conn.Close() }()

	// The CALLER's own context ends the collect by closing the socket — a
	// blocking read would otherwise ignore the cancellation until the
	// absolute read deadline. Our own wall clock is NOT watched here: it is
	// already enforced by that deadline, and closing early would rob the
	// collector of the clean unsubscribe (the socket must stay open so the
	// complete frame can go out at a truncated stop). Which context ended
	// the collect is classified from the contexts, not the socket error.
	socketClosed := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-socketClosed:
		}
	}()

	call := &subscriptionCall{conn: conn, callCtx: callCtx, caller: ctx, deadline: time.Now().Add(limits.MaxDuration)}

	// Handshake: connection_init → connection_ack.
	if err := call.write(wsFrame{Type: wsTypeConnectionInit, Payload: json.RawMessage(`{}`)}); err != nil {
		return c.subscriptionFailed(transcript, call, limits, err, started)
	}
	frame, err := call.read()
	if err != nil {
		return c.subscriptionFailed(transcript, call, limits, err, started)
	}
	switch frame.Type {
	case wsTypeConnectionAck:
	case wsTypeConnectionError:
		failure := errors.New(errors.CodeGeneric, "graphql: endpoint refused the websocket connection (connection_error)")
		transcript.Trailer["error"] = failure.Error()
		_ = call.unsubscribe("")
		return transcript, failure
	default:
		failure := errors.New(errors.CodeInternal, fmt.Sprintf("graphql: unexpected websocket frame %q during handshake", frame.Type))
		_ = call.unsubscribe("")
		return transcript, failure
	}

	// Subscribe: the operation's derived document travels as the payload,
	// exactly like a unary POST body's query/variables/operationName.
	payload, err := json.Marshal(map[string]interface{}{
		"query":         req.Query,
		"variables":     req.Variables,
		"operationName": req.OperationName,
	})
	if err != nil {
		return c.subscriptionFailed(transcript, call, limits, err, started)
	}
	const subID = "muster-1"
	if err := call.write(wsFrame{ID: subID, Type: wsTypeSubscribe, Payload: payload}); err != nil {
		return c.subscriptionFailed(transcript, call, limits, err, started)
	}

	// Collect loop: the cap is tested BEFORE every read, so a capped
	// subscription never blocks on a read the caller no longer wants —
	// the same rule as the gRPC collector.
	bounded := ""
	var failure error
	for {
		if len(transcript.Items) >= limits.MaxMessages {
			bounded = LimitReasonMessages
			break
		}
		frame, err := call.read()
		if err != nil {
			failure = err
			break
		}
		switch frame.Type {
		case wsTypeNext:
			item, decErr := decodeResponse(frame.Payload)
			if decErr != nil {
				failure = decErr
			} else {
				transcript.Items = append(transcript.Items, item)
			}
		case wsTypeError:
			var errs []GraphQLError
			if uErr := json.Unmarshal(frame.Payload, &errs); uErr != nil {
				failure = errors.Wrap(uErr, errors.CodeInternal, "graphql: decode subscription error frame")
			} else {
				failure = errorsAppError(errs)
			}
		case wsTypeComplete:
			// The server ended the subscription: an unsolicited complete
			// of our id (or a broadcast one) is a natural end.
			if frame.ID == "" || frame.ID == subID {
				goto done
			}
			// A complete for some other subscription id on this shared
			// connection is noise: ignore.
		default:
			// Unknown frame types are forwards-compatible noise: ignore.
		}
		if failure != nil {
			break
		}
	}
done:
	// Unsubscribe + clean close — including on a truncated stop, where the
	// complete frame IS the clean unsubscribe. The server's own complete
	// already ended the protocol exchange; ours is a no-op for it, and the
	// write's success is the transcript's unsubscribed evidence.
	unsubscribed := call.unsubscribe(subID) == nil
	close(socketClosed)

	// Our own wall clock fired (the watcher closed the socket): that is the
	// max-duration bound — a clean stop — unless the CALLER's context ended
	// first, which is a cancellation.
	if bounded == "" && failure != nil && callCtx.Err() != nil && !callerEndedTheCall(ctx) {
		bounded = LimitReasonDuration
		failure = nil
	}

	if bounded != "" {
		return transcript, transcript.finish(limits, bounded, nil, false, unsubscribed, started)
	}
	if failure != nil {
		return transcript, transcript.finish(limits, "", failure, callerEndedTheCall(ctx), unsubscribed, started)
	}
	return transcript, transcript.finish(limits, "", nil, false, unsubscribed, started)
}

// subscriptionFailed closes a call that failed before or during the collect
// setup and reports the transcript with the failure's status. The caller's
// context decides cancelled vs error.
func (c *Client) subscriptionFailed(t *SubscriptionTranscript, call *subscriptionCall, limits SubscriptionLimits, failure error, started time.Time) (interface{}, error) {
	_ = call.unsubscribe("")
	return t, t.finish(limits, "", failure, callerEndedTheCall(call.caller), false, started)
}

// read reads one frame under the invocation's wall clock.
func (c *subscriptionCall) read() (wsFrame, error) {
	_ = c.conn.SetReadDeadline(c.deadline)
	var f wsFrame
	if err := c.conn.ReadJSON(&f); err != nil {
		return f, err
	}
	return f, nil
}

// unsubscribe sends the client's complete frame (the graphql-transport-ws
// unsubscribe) and closes the socket with a normal close code. It never
// blocks past a short grace period: after the collector stops, the server
// may be mid-write, and a linger would only delay the caller.
func (c *subscriptionCall) unsubscribe(id string) error {
	if id != "" {
		_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if err := c.conn.WriteJSON(wsFrame{ID: id, Type: wsTypeComplete}); err != nil {
			_ = c.conn.Close()
			return err
		}
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "unsubscribed"))
	return c.conn.Close()
}

// callerEndedTheCall reports whether the CALLER's own context stopped the
// collect (same rule as the gRPC collector: an expired caller deadline is a
// cancellation even before the context package notices).
func callerEndedTheCall(callerCtx context.Context) bool {
	if callerCtx == nil {
		return false
	}
	if callerCtx.Err() != nil {
		return true
	}
	deadline, ok := callerCtx.Deadline()
	return ok && !time.Now().Before(deadline)
}

// websocketURL derives the subscription transport URL from the SAME endpoint
// the unary path posts to (§4.7: the WS dialer is transport selection on the
// existing endpoint, never a second call path). http→ws, https→wss; an
// explicit ws(s):// endpoint passes through unchanged. Exported as
// WebsocketURL for the acceptance battery's derivation table.
func WebsocketURL(endpoint string) (string, error) {
	e := strings.TrimSpace(endpoint)
	switch {
	case e == "":
		return "", errors.New(errors.CodeValidation,
			"graphql: no endpoint for the subscription; pass --endpoint or discover from an endpoint-naming source")
	case strings.HasPrefix(e, "ws://"), strings.HasPrefix(e, "wss://"):
		return e, nil
	case strings.HasPrefix(e, "http://"):
		return "ws://" + strings.TrimPrefix(e, "http://"), nil
	case strings.HasPrefix(e, "https://"):
		return "wss://" + strings.TrimPrefix(e, "https://"), nil
	default:
		return "", errors.New(errors.CodeValidation, fmt.Sprintf(
			"graphql: endpoint %q has no scheme a websocket dial can derive (expected http(s):// or ws(s)://)", endpoint))
	}
}
