package grpc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/wojons/muster/pkg/errors"
)

// ─── Bounded output contract tests (PROTO-003, specs/PROTOCOLS.md §4.5) ──
//
// Every test drives the adapter against the in-process dynamic server on a
// bufconn listener — no external host, no real network (§4.6).

// streamTestTimeout bounds every assertion that "the call returned instead of
// hanging". Generous on purpose: it must fail only on a real hang.
const streamTestTimeout = 3 * time.Second

// newStreamAdapter serves the in-process KeyValue service and returns an
// adapter bound to it with the given limits.
func newStreamAdapter(t *testing.T, limits StreamLimits) *Adapter {
	t.Helper()
	return newStreamAdapterWith(t, limits, nil)
}

// newStreamAdapterWith is newStreamAdapter plus a per-method error injection
// map (the mid-stream failure fixture).
func newStreamAdapterWith(t *testing.T, limits StreamLimits, inject map[string]codes.Code) *Adapter {
	t.Helper()
	store := newKVStore()
	lis, stop := serveKV(t, store, inject, nil)
	t.Cleanup(stop)

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc).WithStreamLimits(limits)
	t.Cleanup(func() { _ = adapter.Close() })

	if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
		t.Fatalf("discover: %v", err)
	}
	return adapter
}

// transcriptOf requires the result to be a streaming transcript envelope.
func transcriptOf(t *testing.T, res interface{}) *StreamTranscript {
	t.Helper()
	tr, ok := res.(*StreamTranscript)
	if !ok {
		t.Fatalf("result = %#v, want *StreamTranscript", res)
	}
	return tr
}

// ── server-streaming ────────────────────────────────────────────────────

// TestExecute_ServerStream_Complete: a stream that ends on its own yields
// status complete with every item and a trailer that records the bound used.
func TestExecute_ServerStream_Complete(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{})

	res, err := adapter.Execute(context.Background(), "kv.KeyValue-Watch", map[string]interface{}{"prefix": "k"})
	if err != nil {
		t.Fatalf("Execute(Watch) = %v, want success", err)
	}
	tr := transcriptOf(t, res)
	if tr.Kind != "server-stream" {
		t.Errorf("kind = %q, want server-stream", tr.Kind)
	}
	if tr.Status != StatusComplete {
		t.Errorf("status = %q, want complete", tr.Status)
	}
	if len(tr.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(tr.Items))
	}
	if tr.ItemsTruncated {
		t.Error("items_truncated must be false for a complete stream")
	}
	first, _ := tr.Items[0].(map[string]interface{})
	if first["key"] != "k-0" || first["value"] != "event" {
		t.Errorf("first item = %#v, want key=k-0 value=event", tr.Items[0])
	}
	if tr.Trailer["received"] != 3 {
		t.Errorf("trailer received = %v, want 3", tr.Trailer["received"])
	}
	if tr.Trailer["messages_sent"] != 1 {
		t.Errorf("trailer messages_sent = %v, want 1", tr.Trailer["messages_sent"])
	}
	if tr.Trailer["max_messages"] != DefaultMaxMessages {
		t.Errorf("trailer max_messages = %v, want %d", tr.Trailer["max_messages"], DefaultMaxMessages)
	}
	if _, ok := tr.Trailer["limit_reason"]; ok {
		t.Errorf("limit_reason must be absent for a complete stream, got %v", tr.Trailer["limit_reason"])
	}
}

// TestExecute_ServerStream_MaxMessagesTruncated: the item cap stops a stream
// that would otherwise never end, cleanly (no error) and with an explicit
// truncation marker (§4.5 rule 2).
func TestExecute_ServerStream_MaxMessagesTruncated(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{MaxMessages: 1})

	start := time.Now()
	res, err := adapter.Execute(context.Background(), "kv.KeyValue-Tail", map[string]interface{}{"prefix": "k"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("capped Execute(Tail) = %v, want a clean stop", err)
	}
	if elapsed > streamTestTimeout {
		t.Fatalf("capped stream took %v — the cap must stop it, not the server", elapsed)
	}
	tr := transcriptOf(t, res)
	if tr.Status != StatusTruncated {
		t.Errorf("status = %q, want truncated", tr.Status)
	}
	if !tr.ItemsTruncated {
		t.Error("items_truncated must be true when the cap fires")
	}
	if len(tr.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(tr.Items))
	}
	if tr.Trailer["limit_reason"] != LimitReasonMessages {
		t.Errorf("limit_reason = %v, want %s", tr.Trailer["limit_reason"], LimitReasonMessages)
	}
}

// TestExecute_ServerStream_DurationBound: the wall-clock bound stops an
// endless stream on its own, again with the truncation marker and no error.
func TestExecute_ServerStream_DurationBound(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{MaxMessages: 1000, MaxDuration: 100 * time.Millisecond})

	start := time.Now()
	res, err := adapter.Execute(context.Background(), "kv.KeyValue-Tail", map[string]interface{}{"prefix": "k"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("duration-bounded Execute(Tail) = %v, want a clean stop", err)
	}
	if elapsed > streamTestTimeout {
		t.Fatalf("duration bound took %v — the bound must stop the stream", elapsed)
	}
	tr := transcriptOf(t, res)
	if tr.Status != StatusTruncated {
		t.Errorf("status = %q, want truncated", tr.Status)
	}
	if !tr.ItemsTruncated {
		t.Error("items_truncated must be true when the duration bound fires")
	}
	if tr.Trailer["limit_reason"] != LimitReasonDuration {
		t.Errorf("limit_reason = %v, want %s", tr.Trailer["limit_reason"], LimitReasonDuration)
	}
	if len(tr.Items) != 1 {
		t.Errorf("items = %d, want the single event the server sent", len(tr.Items))
	}
}

// TestExecute_ServerStream_CallerDeadline: the CALLER's context deadline is a
// cancellation, not a bound we imposed — the transcript says so and the mapped
// AppError (CodeTimeout) comes back alongside the partial transcript.
func TestExecute_ServerStream_CallerDeadline(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	res, err := adapter.Execute(ctx, "kv.KeyValue-Tail", map[string]interface{}{"prefix": "k"})
	elapsed := time.Since(start)
	if elapsed > streamTestTimeout {
		t.Fatalf("cancelled stream took %v — cancellation must propagate", elapsed)
	}
	if err == nil {
		t.Fatal("a caller-side deadline must return an error")
	}
	if !apperrors.IsError(err, apperrors.CodeTimeout) {
		t.Fatalf("deadline error = %v, want AppError CodeTimeout", err)
	}
	tr := transcriptOf(t, res)
	if tr.Status != StatusCancelled {
		t.Errorf("status = %q, want cancelled", tr.Status)
	}
	if len(tr.Items) != 1 {
		t.Errorf("items = %d, want the event received before the deadline", len(tr.Items))
	}
}

// TestExecute_ServerStream_MidStreamError: a server that fails after sending
// items terminates the transcript with status error, keeps the partial items,
// and maps the status per §4.2 (§4.5 rule 5).
func TestExecute_ServerStream_MidStreamError(t *testing.T) {
	adapter := newStreamAdapterWith(t, StreamLimits{}, map[string]codes.Code{
		"/kv.KeyValue/Tail": codes.NotFound,
	})

	res, err := adapter.Execute(context.Background(), "kv.KeyValue-Tail", map[string]interface{}{"prefix": "k"})
	if err == nil {
		t.Fatal("a mid-stream server error must be reported")
	}
	if !apperrors.IsError(err, apperrors.CodeNotFound) {
		t.Fatalf("mid-stream error = %v, want AppError CodeNotFound", err)
	}
	tr := transcriptOf(t, res)
	if tr.Status != StatusError {
		t.Errorf("status = %q, want error", tr.Status)
	}
	if len(tr.Items) != 1 {
		t.Errorf("items = %d, want the partial item preserved", len(tr.Items))
	}
	if tr.Trailer["error"] == nil {
		t.Error("trailer must carry the stream error for a partial transcript")
	}
}

// TestExecute_ServerStream_TooManyMessages: a server-streaming method takes
// exactly one request message; ExecuteMessages with more is client-side
// validation before any bytes hit the wire (§4.2).
func TestExecute_ServerStream_TooManyMessages(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{})

	_, err := adapter.ExecuteMessages(context.Background(), "kv.KeyValue-Watch", []map[string]interface{}{
		{"prefix": "a"}, {"prefix": "b"},
	})
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Fatalf("two seeds for a server-streaming method = %v, want AppError CodeValidation", err)
	}
}

// ── client-streaming ────────────────────────────────────────────────────

// TestExecute_ClientStream_RoundTrip: N messages in, one response out, inside
// the same bounded envelope.
func TestExecute_ClientStream_RoundTrip(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{})

	res, err := adapter.ExecuteMessages(context.Background(), "kv.KeyValue-Upload", []map[string]interface{}{
		{"key": "a"}, {"key": "b"}, {"key": "c"},
	})
	if err != nil {
		t.Fatalf("ExecuteMessages(Upload) = %v, want success", err)
	}
	tr := transcriptOf(t, res)
	if tr.Kind != "client-stream" {
		t.Errorf("kind = %q, want client-stream", tr.Kind)
	}
	if tr.Status != StatusComplete {
		t.Errorf("status = %q, want complete", tr.Status)
	}
	if len(tr.Items) != 1 {
		t.Fatalf("items = %d, want the single response", len(tr.Items))
	}
	resp, _ := tr.Items[0].(map[string]interface{})
	if resp["count"] != float64(3) || resp["joined"] != "a,b,c" {
		t.Errorf("response = %#v, want count=3 joined=a,b,c", tr.Items[0])
	}
	if tr.Trailer["messages_sent"] != 3 {
		t.Errorf("trailer messages_sent = %v, want 3", tr.Trailer["messages_sent"])
	}
	if tr.Trailer["received"] != 1 {
		t.Errorf("trailer received = %v, want 1", tr.Trailer["received"])
	}
}

// TestExecute_ClientStream_CappedRequestStream: a request stream longer than
// the cap is stopped at the cap, marked truncated, and the server's answer to
// that capped stream is still collected (never a silent half-answer).
func TestExecute_ClientStream_CappedRequestStream(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{MaxMessages: 2})

	res, err := adapter.ExecuteMessages(context.Background(), "kv.KeyValue-Upload", []map[string]interface{}{
		{"key": "a"}, {"key": "b"}, {"key": "c"},
	})
	if err != nil {
		t.Fatalf("capped ExecuteMessages(Upload) = %v, want a clean stop", err)
	}
	tr := transcriptOf(t, res)
	if tr.Status != StatusTruncated {
		t.Errorf("status = %q, want truncated", tr.Status)
	}
	if !tr.ItemsTruncated {
		t.Error("items_truncated must be true when the request stream was capped")
	}
	if tr.Trailer["limit_reason"] != LimitReasonMessages {
		t.Errorf("limit_reason = %v, want %s", tr.Trailer["limit_reason"], LimitReasonMessages)
	}
	if tr.Trailer["messages_sent"] != 2 {
		t.Errorf("trailer messages_sent = %v, want 2 (the cap)", tr.Trailer["messages_sent"])
	}
	resp, _ := tr.Items[0].(map[string]interface{})
	if resp["joined"] != "a,b" {
		t.Errorf("response = %#v, want the answer to the capped stream (a,b)", tr.Items[0])
	}
}

// TestExecute_ClientStream_ZeroMessages: an empty request stream is legal —
// the client closes the send side immediately.
func TestExecute_ClientStream_ZeroMessages(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{})

	res, err := adapter.ExecuteMessages(context.Background(), "kv.KeyValue-Upload", nil)
	if err != nil {
		t.Fatalf("empty ExecuteMessages(Upload) = %v, want success", err)
	}
	tr := transcriptOf(t, res)
	if tr.Status != StatusComplete {
		t.Errorf("status = %q, want complete", tr.Status)
	}
	if tr.Trailer["messages_sent"] != 0 {
		t.Errorf("trailer messages_sent = %v, want 0", tr.Trailer["messages_sent"])
	}
	if len(tr.Items) != 1 {
		t.Fatalf("items = %d, want the single response", len(tr.Items))
	}
	// The server answered count=0 / joined=""; proto3 JSON omits
	// default-valued scalars, so the decoded response is an empty object.
	// What this test pins is that the empty request stream was accepted.
	resp, _ := tr.Items[0].(map[string]interface{})
	if len(resp) != 0 {
		t.Errorf("response = %#v, want {} (proto3 JSON omits the zero count)", resp)
	}
}

// ── bidirectional ───────────────────────────────────────────────────────

// TestExecute_Bidi_RoundTrip: messages flow both ways concurrently — a server
// that answers only while being read must not deadlock the client.
func TestExecute_Bidi_RoundTrip(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{})

	res, err := adapter.ExecuteMessages(context.Background(), "kv.KeyValue-Chat", []map[string]interface{}{
		{"text": "one"}, {"text": "two"}, {"text": "three"},
	})
	if err != nil {
		t.Fatalf("ExecuteMessages(Chat) = %v, want success", err)
	}
	tr := transcriptOf(t, res)
	if tr.Kind != "bidi-stream" {
		t.Errorf("kind = %q, want bidi-stream", tr.Kind)
	}
	if tr.Status != StatusComplete {
		t.Errorf("status = %q, want complete", tr.Status)
	}
	if len(tr.Items) != 3 {
		t.Fatalf("items = %d, want 3 echoes", len(tr.Items))
	}
	for i, want := range []string{"one", "two", "three"} {
		got, _ := tr.Items[i].(map[string]interface{})
		if got["text"] != want {
			t.Errorf("echo %d = %#v, want text=%s", i, tr.Items[i], want)
		}
	}
	if tr.Trailer["messages_sent"] != 3 {
		t.Errorf("trailer messages_sent = %v, want 3", tr.Trailer["messages_sent"])
	}
}

// TestExecute_Bidi_Cancellation: a caller cancel stops a live bidi stream (the
// server here holds the stream open after echoing), returns promptly, and
// reports the partial round trip.
func TestExecute_Bidi_Cancellation(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(150*time.Millisecond, cancel)
	defer cancel()

	start := time.Now()
	res, err := adapter.ExecuteMessages(ctx, "kv.KeyValue-ChatHold", []map[string]interface{}{
		{"text": "hold"},
	})
	elapsed := time.Since(start)
	if elapsed > streamTestTimeout {
		t.Fatalf("cancelled bidi stream took %v — cancellation must propagate", elapsed)
	}
	if err == nil {
		t.Fatal("a caller cancel must return an error")
	}
	if !apperrors.IsError(err, apperrors.CodeTimeout) {
		t.Fatalf("cancel error = %v, want AppError CodeTimeout", err)
	}
	tr := transcriptOf(t, res)
	if tr.Status != StatusCancelled {
		t.Errorf("status = %q, want cancelled", tr.Status)
	}
	if len(tr.Items) != 1 {
		t.Fatalf("items = %d, want the echo received before the cancel", len(tr.Items))
	}
	got, _ := tr.Items[0].(map[string]interface{})
	if got["text"] != "hold" {
		t.Errorf("echo = %#v, want text=hold", tr.Items[0])
	}
}

// TestExecute_Bidi_ZeroMessages: closing the send side with no messages ends a
// bidi stream cleanly with an empty (never null) item list.
func TestExecute_Bidi_ZeroMessages(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{})

	res, err := adapter.ExecuteMessages(context.Background(), "kv.KeyValue-Chat", nil)
	if err != nil {
		t.Fatalf("empty ExecuteMessages(Chat) = %v, want success", err)
	}
	tr := transcriptOf(t, res)
	if tr.Status != StatusComplete {
		t.Errorf("status = %q, want complete", tr.Status)
	}
	if len(tr.Items) != 0 {
		t.Errorf("items = %#v, want none", tr.Items)
	}
	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("transcript not JSON-serializable: %v", err)
	}
	if !strings.Contains(string(raw), `"items":[]`) {
		t.Errorf("empty items must marshal as [], got %s", raw)
	}
}

// ── envelope + bounds contract ──────────────────────────────────────────

// TestStreamTranscript_EnvelopeShape pins the §4.5 envelope keys and JSON
// types, so a rename cannot silently break transcript consumers.
func TestStreamTranscript_EnvelopeShape(t *testing.T) {
	adapter := newStreamAdapter(t, StreamLimits{})

	res, err := adapter.Execute(context.Background(), "kv.KeyValue-Watch", map[string]interface{}{"prefix": "k"})
	if err != nil {
		t.Fatalf("Execute(Watch) = %v", err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal transcript: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("transcript is not a JSON object: %v", err)
	}
	for _, key := range []string{"kind", "status", "items", "items_truncated", "trailer"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("envelope key %q missing from %s", key, raw)
		}
	}
	if len(decoded) != 5 {
		t.Errorf("envelope keys = %v, want exactly the five §4.5 keys", decoded)
	}
	if _, ok := decoded["items"].([]interface{}); !ok {
		t.Errorf("items must be a JSON array, got %T", decoded["items"])
	}
	if decoded["items_truncated"] != false {
		t.Errorf("items_truncated = %v, want false", decoded["items_truncated"])
	}
	if _, ok := decoded["trailer"].(map[string]interface{}); !ok {
		t.Errorf("trailer must be a JSON object, got %T", decoded["trailer"])
	}
}

// TestStreamLimits_NormalizeDefaults: unset or nonsensical bounds select the
// documented defaults — there is no unbounded mode (§4.5 rule 2).
func TestStreamLimits_NormalizeDefaults(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   StreamLimits
	}{
		{"unset", StreamLimits{}},
		{"zero", StreamLimits{MaxMessages: 0, MaxDuration: 0}},
		{"negative", StreamLimits{MaxMessages: -5, MaxDuration: -time.Second}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.normalize()
			if got.MaxMessages != DefaultMaxMessages {
				t.Errorf("MaxMessages = %d, want %d", got.MaxMessages, DefaultMaxMessages)
			}
			if got.MaxDuration != DefaultMaxDuration {
				t.Errorf("MaxDuration = %v, want %v", got.MaxDuration, DefaultMaxDuration)
			}
		})
	}

	explicit := StreamLimits{MaxMessages: 7, MaxDuration: 2 * time.Second}.normalize()
	if explicit.MaxMessages != 7 || explicit.MaxDuration != 2*time.Second {
		t.Errorf("explicit limits = %+v, want them preserved", explicit)
	}
}

// TestExecute_MessagesSeedOnUnary: ExecuteMessages may seed a unary method with
// one message (or none), but not more than one.
func TestExecute_MessagesSeedOnUnary(t *testing.T) {
	store := newKVStore()
	store.set("greeting", "hello")
	lis, stop := serveKV(t, store, nil, nil)
	defer stop()

	cc := connFor(t, lis)
	adapter := NewAdapter().WithConn(cc)
	defer func() { _ = adapter.Close() }()
	if _, err := adapter.Discover(context.Background(), protoSourceToTemp(t)); err != nil {
		t.Fatalf("discover: %v", err)
	}

	res, err := adapter.ExecuteMessages(context.Background(), "kv.KeyValue-Get", []map[string]interface{}{
		{"key": "greeting"},
	})
	if err != nil {
		t.Fatalf("single-message unary seed = %v, want success", err)
	}
	got, ok := res.(map[string]interface{})
	if !ok || got["value"] != "hello" {
		t.Fatalf("result = %#v, want value=hello", res)
	}

	_, err = adapter.ExecuteMessages(context.Background(), "kv.KeyValue-Get", []map[string]interface{}{
		{"key": "a"}, {"key": "b"},
	})
	if !apperrors.IsError(err, apperrors.CodeValidation) {
		t.Fatalf("two seeds for a unary method = %v, want AppError CodeValidation", err)
	}
}

// ─── caller-stop classification (PROTO-009 race) ─────────────────────────

// expiredDeadlineCtx is a context whose deadline has passed while Err() is still
// nil: the window in which the transport observes the caller's deadline before
// the context package has marked the context done.
type expiredDeadlineCtx struct{ context.Context }

func (expiredDeadlineCtx) Deadline() (time.Time, bool) {
	return time.Now().Add(-time.Second), true
}

// TestFinishStream_DeadlinePassedButNotYetMarked pins how a caller-deadline
// stop is classified when it lands inside that window: the transcript is
// "cancelled", not "error".
//
// TestExecute_ServerStream_CallerDeadline failed on macos-latest for exactly
// this reason — the transport ended the call at the 150ms deadline (items
// intact, CodeTimeout) while callerCtx.Err() was still nil, so the transcript
// was labelled "error".
func TestFinishStream_DeadlinePassedButNotYetMarked(t *testing.T) {
	tr := &StreamTranscript{Trailer: map[string]interface{}{}}
	out := streamOutcome{err: context.DeadlineExceeded}

	err := finishStream(tr, &streamBound{}, out, expiredDeadlineCtx{context.Background()}, "/kv.KeyValue/Tail", time.Now(), 1)

	if tr.Status != StatusCancelled {
		t.Fatalf("status = %q, want cancelled", tr.Status)
	}
	if !apperrors.IsError(err, apperrors.CodeTimeout) {
		t.Fatalf("error = %v, want AppError CodeTimeout", err)
	}
}

// TestFinishStream_LiveDeadlineStaysAnError is the control: a caller deadline
// that has NOT passed must leave a server-reported failure classified as
// "error", so the check above cannot be satisfied by ignoring the status.
func TestFinishStream_LiveDeadlineStaysAnError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	tr := &StreamTranscript{Trailer: map[string]interface{}{}}
	out := streamOutcome{err: status.Error(codes.NotFound, "key not found")}

	err := finishStream(tr, &streamBound{}, out, ctx, "/kv.KeyValue/Tail", time.Now(), 0)

	if tr.Status != StatusError {
		t.Fatalf("status = %q, want error", tr.Status)
	}
	if !apperrors.IsError(err, apperrors.CodeNotFound) {
		t.Fatalf("error = %v, want AppError CodeNotFound", err)
	}
}
