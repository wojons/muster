package asyncapi

import (
	"sort"
	"time"

	"github.com/wojons/muster/pkg/protocol"
)

// ─── Bounded output contract (specs/PROTOCOLS.md §4.5(4), PROTO-007 §4) ──
//
// A KindEventSource invocation NEVER returns a live subscription. The
// bounded collect subscribes, gathers items, unsubscribes/closes, and
// returns the EventTranscript envelope. Hitting a bound is a CLEAN stop
// (status "truncated"); a caller cancellation preserves partial items and
// reports "cancelled"; a broker failure preserves partial items and
// reports "error" alongside the mapped AppError.

// Default bounds per invocation (PROTO-007 §4: 1000 messages / 30 seconds;
// there is deliberately no unbounded setting — missing, zero or negative
// values select the defaults).
const (
	DefaultMaxMessages = 1000
	DefaultMaxDuration = 30 * time.Second
)

// Transcript status vocabulary (same envelope shape as the gRPC
// StreamTranscript; kind is "event-source" for this adapter).
const (
	StatusComplete  = "complete"
	StatusTruncated = "truncated"
	StatusCancelled = "cancelled"
	StatusError     = "error"
)

// Limit reasons reported in EventTranscript.Trailer["limit_reason"] when a
// configured bound stopped the collect.
const (
	LimitReasonMessages = "max-items"    // PROTO-007 §4 envelope vocabulary
	LimitReasonDuration = "max-duration" // PROTO-007 §4 envelope vocabulary
)

// EventLimits carries the per-invocation bounds of the bounded-collect
// contract. Zero or negative fields select the package defaults.
type EventLimits struct {
	MaxMessages int
	MaxDuration time.Duration
}

// normalize applies the documented defaults to unset bounds.
func (l EventLimits) normalize() EventLimits {
	if l.MaxMessages <= 0 {
		l.MaxMessages = DefaultMaxMessages
	}
	if l.MaxDuration <= 0 {
		l.MaxDuration = DefaultMaxDuration
	}
	return l
}

// EventTranscript is the bounded transcript envelope every consume
// invocation returns (PROTO-007 §4). It is JSON-serializable so the shared
// output rendering consumes it like any other result.
type EventTranscript struct {
	Kind           string                 `json:"kind"` // always "event-source"
	Status         string                 `json:"status"`
	Items          []interface{}          `json:"items"` // always a JSON array
	ItemsTruncated bool                   `json:"items_truncated"`
	Trailer        map[string]interface{} `json:"trailer"`
}

// newEventTranscript starts an envelope with the effective bounds recorded,
// so the caller can tell which bound applied even on a complete stream.
func newEventTranscript(limits EventLimits) *EventTranscript {
	return &EventTranscript{
		Kind:   "event-source",
		Status: StatusComplete,
		Items:  []interface{}{},
		Trailer: map[string]interface{}{
			"received":  0,
			"max_items": limits.MaxMessages,
		},
	}
}

// finish stamps the terminal status and trailer of a bounded collect.
//
// drained reports whether every partition was fully consumed within the
// collect window (the natural end of an event-source exchange); a bound
// fired → "truncated"; a broker failure → "error"; a caller cancellation →
// "cancelled". The subscription is closed by the caller before finishing.
func (t *EventTranscript) finish(limits EventLimits, drained bool, bound string, failure error, callerCancelled bool, started time.Time) {
	t.Trailer["received"] = len(t.Items)
	t.Trailer["duration_ms"] = time.Since(started).Milliseconds()

	switch {
	case drained:
		t.Status = StatusComplete
	case bound != "":
		t.Status = StatusTruncated
		t.ItemsTruncated = true
		t.Trailer["limit_reason"] = bound
	case callerCancelled:
		t.Status = StatusCancelled
	case failure != nil:
		t.Status = StatusError
		t.Trailer["error"] = failure.Error()
	default:
		t.Status = StatusComplete
	}
}

// drainTracker reports the drained predicate used by the consume loop.
type drainTracker struct {
	// start and end are the per-partition offset snapshots taken at collect
	// start (end exclusive).
	start     map[int32]int64
	end       map[int32]int64
	delivered map[int32]int64 // highest delivered offset per partition
	partIDs   []int32
}

func newDrainTracker(start, end map[int32]int64) *drainTracker {
	t := &drainTracker{
		start:     start,
		end:       end,
		delivered: map[int32]int64{},
	}
	for p := range end {
		t.partIDs = append(t.partIDs, p)
	}
	sort.Slice(t.partIDs, func(i, j int) bool { return t.partIDs[i] < t.partIDs[j] })
	return t
}

// record notes one delivered message and reports whether the whole topic
// has been drained within the collect window:
//
//   - a partition whose snapshot window was empty (end == start) counts as
//     drained immediately (nothing retained to read);
//   - otherwise it is drained once the highest delivered offset reaches
//     the snapshot end (offsets are start-inclusive, end-exclusive).
//
// Messages published after the snapshot was taken may still be delivered;
// they are appended as items but do not extend the drain target.
func (t *drainTracker) record(partition int32, offset int64) bool {
	if v, ok := t.delivered[partition]; !ok || offset > v {
		t.delivered[partition] = offset
	}
	for _, p := range t.partIDs {
		if t.end[p] == t.start[p] {
			continue // empty window: nothing to drain
		}
		// A partition with no delivered message is NOT drained: the
		// zero-value map lookup would otherwise read offset 0 and satisfy
		// end-1 == 0, marking a partition whose first message is still in
		// flight as fully consumed.
		if d, seen := t.delivered[p]; !seen || d < t.end[p]-1 {
			return false
		}
	}
	return true
}

// drainedAtStart reports whether every partition's snapshot window was
// empty (the collect can complete immediately with zero items — the
// PROTO-007 §9.2 empty-receive-window rule).
func (t *drainTracker) drainedAtStart() bool {
	for _, p := range t.partIDs {
		if t.end[p] != t.start[p] {
			return false
		}
	}
	return true
}

// ensure the streaming-kind label helper keeps parity with the gRPC
// adapter's vocabulary for cross-adapter transcript rendering.
var _ = protocol.KindEventSource
