package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// mockHandler — full Handler interface implementation
// ---------------------------------------------------------------------------

type mockHandler struct {
	name     string
	handleFn func(context.Context, *Event) error
	supports map[EventType]bool
}

func (m *mockHandler) Handle(ctx context.Context, event *Event) error {
	if m.handleFn != nil {
		return m.handleFn(ctx, event)
	}
	return nil
}

func (m *mockHandler) Name() string {
	return m.name
}

func (m *mockHandler) Supports(eventType EventType) bool {
	if m.supports != nil {
		return m.supports[eventType]
	}
	return true
}

func requireNoErr(t require.TestingT, err error, msg ...interface{}) {
	require.NoError(t, err, msg...)
}

// ---------------------------------------------------------------------------
// 1. Event_Creation
// ---------------------------------------------------------------------------

func TestEvent_Creation(t *testing.T) {
	t.Parallel()

	ev := &Event{
		Type:   "user.created",
		Data:   []byte(`{"id":42}`),
		Source: "auth-service",
	}
	assert.Equal(t, EventType("user.created"), ev.Type)
	assert.Equal(t, []byte(`{"id":42}`), ev.Data)
	assert.Equal(t, "auth-service", ev.Source)

	// Zero-value event
	var zero Event
	assert.Empty(t, zero.Type)
	assert.Nil(t, zero.Data)
	assert.Empty(t, zero.Source)
}

// ---------------------------------------------------------------------------
// 2. DefaultDispatcherConfig_Values
// ---------------------------------------------------------------------------

func TestDefaultDispatcherConfig_Values(t *testing.T) {
	cfg := DefaultDispatcherConfig
	assert.Equal(t, 4, cfg.WorkerCount)
	assert.Equal(t, 1000, cfg.BufferSize)
	assert.Equal(t, 0, cfg.RetryAttempts)
	assert.Equal(t, 100*time.Millisecond, cfg.RetryDelay)
	assert.False(t, cfg.EnableDeadLetter)
	assert.Equal(t, 100, cfg.DeadLetterQueueSize)
}

// ---------------------------------------------------------------------------
// 3. NewDispatcher_ZeroConfig — uses defaults
// ---------------------------------------------------------------------------

func TestNewDispatcher_ZeroConfig(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{})
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	assert.Equal(t, 4, d.config.WorkerCount)
	assert.Equal(t, 1000, d.config.BufferSize)
	assert.Nil(t, d.deadLetter)
}

// ---------------------------------------------------------------------------
// 4. NewDispatcher_NegativeConfig — negative/invalid values fall back
// ---------------------------------------------------------------------------

func TestNewDispatcher_NegativeConfig(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: -1,
		BufferSize:  -5,
	})
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	assert.Equal(t, 4, d.config.WorkerCount)
	assert.Equal(t, 1000, d.config.BufferSize)
}

// ---------------------------------------------------------------------------
// 5. NewDispatcher_DeadLetterEnabled — DLQ channel created
// ---------------------------------------------------------------------------

func TestNewDispatcher_DeadLetterEnabled(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount:         1,
		BufferSize:          10,
		EnableDeadLetter:    true,
		DeadLetterQueueSize: 5,
	})
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	require.NotNil(t, d.deadLetter)
	assert.Equal(t, 5, cap(d.deadLetter))
}

// ---------------------------------------------------------------------------
// 6. Subscribe_ReturnsNonEmptyID
// ---------------------------------------------------------------------------

func TestSubscribe_ReturnsNonEmptyID(t *testing.T) {
	d, err := NewDispatcher(DefaultDispatcherConfig)
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	h := &mockHandler{name: "sub-test"}
	id := d.Subscribe("test.event", h)
	assert.NotEmpty(t, id)
	assert.Contains(t, string(id), "sub-")
}

// ---------------------------------------------------------------------------
// 7. Unsubscribe_Existing_ReturnsTrue
// ---------------------------------------------------------------------------

func TestUnsubscribe_Existing_ReturnsTrue(t *testing.T) {
	d, err := NewDispatcher(DefaultDispatcherConfig)
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	h := &mockHandler{name: "unsub-test"}
	id := d.Subscribe("test.event", h)

	ok := d.Unsubscribe("test.event", id)
	assert.True(t, ok)

	// Second call must return false
	ok = d.Unsubscribe("test.event", id)
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// 8. Unsubscribe_NonExistent_ReturnsFalse
// ---------------------------------------------------------------------------

func TestUnsubscribe_NonExistent_ReturnsFalse(t *testing.T) {
	d, err := NewDispatcher(DefaultDispatcherConfig)
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	ok := d.Unsubscribe("nonexistent.event", "sub-never-registered")
	assert.False(t, ok)
}

// ---------------------------------------------------------------------------
// 9. Unsubscribe_WrongEventType_ReturnsFalse
// ---------------------------------------------------------------------------

func TestUnsubscribe_WrongEventType_ReturnsFalse(t *testing.T) {
	d, err := NewDispatcher(DefaultDispatcherConfig)
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	h := &mockHandler{name: "type-test"}
	id := d.Subscribe("type.a", h)

	// Try to unsubscribe with a different event type
	ok := d.Unsubscribe("type.b", id)
	assert.False(t, ok)

	// Original subscription is still intact
	ok = d.Unsubscribe("type.a", id)
	assert.True(t, ok)
}

// ---------------------------------------------------------------------------
// 10. Stop_PreventsPublish — publish after stop returns error
// ---------------------------------------------------------------------------

func TestStop_PreventsPublish(t *testing.T) {
	d, err := NewDispatcher(DefaultDispatcherConfig)
	require.NoError(t, err)

	d.Stop()
	assert.True(t, d.stopped)

	ctx := context.Background()
	err = d.Publish(ctx, &Event{Type: "after.stop"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dispatcher is stopped")
}

// ---------------------------------------------------------------------------
// 11. Stop_PreventsPublishSync — PublishSync on stopped dispatcher
// ---------------------------------------------------------------------------

func TestStop_PreventsPublishSync(t *testing.T) {
	d, err := NewDispatcher(DefaultDispatcherConfig)
	require.NoError(t, err)

	d.Stop()

	ctx := context.Background()
	results := d.PublishSync(ctx, &Event{Type: "after.stop"})
	assert.Nil(t, results)
}

// ---------------------------------------------------------------------------
// 12. PublishSync_ReturnsHandlerResults
// ---------------------------------------------------------------------------

func TestPublishSync_ReturnsHandlerResults(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: 2,
		BufferSize:  10,
	})
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	h1 := &mockHandler{
		name: "handler-one",
		handleFn: func(_ context.Context, _ *Event) error {
			return nil
		},
	}
	h2 := &mockHandler{
		name: "handler-two",
		handleFn: func(_ context.Context, _ *Event) error {
			return errors.New("fail from h2")
		},
	}

	d.Subscribe("sync.event", h1)
	d.Subscribe("sync.event", h2)

	ctx := context.Background()
	results := d.PublishSync(ctx, &Event{Type: "sync.event", Data: []byte("ok")})

	require.Len(t, results, 2)

	// Results should be in priority order (both priority 0, stable sort not guaranteed)
	// Collect names so order-independent check
	names := make(map[string]error)
	for _, r := range results {
		names[r.HandlerName] = r.Error
	}
	assert.NoError(t, names["handler-one"])
	assert.Error(t, names["handler-two"])
	assert.Contains(t, names["handler-two"].Error(), "fail from h2")
}

// ---------------------------------------------------------------------------
// 13. WithFilter_FiltersEvents
// ---------------------------------------------------------------------------

func TestWithFilter_FiltersEvents(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: 1,
		BufferSize:  10,
	})
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	var mu sync.Mutex
	count := 0

	h := &mockHandler{
		name: "filtered",
		handleFn: func(_ context.Context, _ *Event) error {
			mu.Lock()
			count++
			mu.Unlock()
			return nil
		},
	}

	// Only accept events where Source == "allowed"
	d.Subscribe("filter.event", h, WithFilter(func(ev *Event) bool {
		return ev.Source == "allowed"
	}))

	ctx := context.Background()

	// Publish events that pass the filter
	for i := 0; i < 3; i++ {
		err := d.Publish(ctx, &Event{Type: "filter.event", Source: "allowed"})
		require.NoError(t, err)
	}
	// Publish events that are blocked by the filter
	for i := 0; i < 2; i++ {
		err := d.Publish(ctx, &Event{Type: "filter.event", Source: "blocked"})
		require.NoError(t, err)
	}

	// Use PublishSync to get deterministic timing — wait for all events to process
	// by publishing a sync sentinel event with the same type
	sentinel := &mockHandler{
		name: "sentinel",
		handleFn: func(_ context.Context, _ *Event) error {
			return nil
		},
	}
	sentinelID := d.Subscribe("filter.event", sentinel)
	t.Cleanup(func() { d.Unsubscribe("filter.event", sentinelID) })

	d.PublishSync(ctx, &Event{Type: "filter.event", Source: "sentinel"})

	mu.Lock()
	assert.Equal(t, 3, count, "handler should only receive events with Source='allowed'")
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// 14. WithFilter_NilFilter — nil filter treated as pass-through
// ---------------------------------------------------------------------------

func TestWithFilter_NilFilterPassThrough(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: 1,
		BufferSize:  10,
	})
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	called := false
	h := &mockHandler{
		name: "nil-filter",
		handleFn: func(_ context.Context, _ *Event) error {
			called = true
			return nil
		},
	}

	// WithFilter(nil) — should default to pass-through
	id := d.Subscribe("no-filter.event", h, WithFilter(nil))
	t.Cleanup(func() { d.Unsubscribe("no-filter.event", id) })

	ctx := context.Background()
	results := d.PublishSync(ctx, &Event{Type: "no-filter.event"})
	require.Len(t, results, 1)
	assert.True(t, called)
}

// ---------------------------------------------------------------------------
// 15. ConcurrentPublish
// ---------------------------------------------------------------------------

func TestConcurrentPublish(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: 1, // Single worker guarantees sequential processing
		BufferSize:  500,
		RetryAttempts: 1,
	})
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	var mu sync.Mutex
	total := 0

	h := &mockHandler{
		name: "concurrent",
		handleFn: func(_ context.Context, _ *Event) error {
			mu.Lock()
			total++
			mu.Unlock()
			return nil
		},
	}

	_ = d.Subscribe("concurrent.event", h)

	ctx := context.Background()
	const publishers = 10
	const eventsPer = 50

	var pubWg sync.WaitGroup
	for i := 0; i < publishers; i++ {
		pubWg.Add(1)
		go func() {
			defer pubWg.Done()
			for j := 0; j < eventsPer; j++ {
				_ = d.Publish(ctx, &Event{Type: "concurrent.event"})
			}
		}()
	}
	pubWg.Wait()

	// With single worker, PublishSync acts as a barrier — all prior
	// events are processed FIFO before the sync event.
	done := make(chan struct{})
	d.Subscribe("__barrier__", &mockHandler{
		name: "sync-barrier",
		handleFn: func(_ context.Context, _ *Event) error {
			close(done)
			return nil
		},
	})
	d.PublishSync(ctx, &Event{Type: "__barrier__"})
	<-done

	mu.Lock()
	assert.Equal(t, publishers*eventsPer, total)
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// 16. RetryOnHandlerFailure
// ---------------------------------------------------------------------------

func TestRetryOnHandlerFailure(t *testing.T) {
	// Configure retries — tightly constrained for speed
	cfg := DefaultDispatcherConfig
	cfg.WorkerCount = 1
	cfg.BufferSize = 10
	cfg.RetryAttempts = 2
	cfg.RetryDelay = time.Millisecond

	d, err := NewDispatcher(cfg)
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	var mu sync.Mutex
	attempts := 0

	h := &mockHandler{
		name: "retry-me",
		handleFn: func(_ context.Context, _ *Event) error {
			mu.Lock()
			attempts++
			count := attempts
			mu.Unlock()
			if count < 3 { // fail first 2 times (RetryAttempts=2 = 3 total calls)
				return errors.New("transient error")
			}
			return nil
		},
	}

	d.Subscribe("retry.event", h)

	// Use PublishSync — blocked until handler completes all retries
	ctx := context.Background()
	results := d.PublishSync(ctx, &Event{Type: "retry.event"})

	require.Len(t, results, 1)
	assert.Equal(t, "retry-me", results[0].HandlerName)
	assert.NoError(t, results[0].Error, "handler should succeed on 3rd attempt")

	mu.Lock()
	assert.Equal(t, 3, attempts, "handler should be called 3 times (1 initial + 2 retries)")
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// 17. Retry_Exhaustion_ReturnsError
// ---------------------------------------------------------------------------

func TestRetry_Exhaustion_ReturnsError(t *testing.T) {
	cfg := DefaultDispatcherConfig
	cfg.WorkerCount = 1
	cfg.BufferSize = 10
	cfg.RetryAttempts = 1
	cfg.RetryDelay = time.Millisecond

	d, err := NewDispatcher(cfg)
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	var mu sync.Mutex
	attempts := 0

	h := &mockHandler{
		name: "always-fail",
		handleFn: func(_ context.Context, _ *Event) error {
			mu.Lock()
			attempts++
			mu.Unlock()
			return errors.New("permanent failure")
		},
	}

	d.Subscribe("retry.exhaust", h)

	ctx := context.Background()
	results := d.PublishSync(ctx, &Event{Type: "retry.exhaust"})

	require.Len(t, results, 1)
	require.Error(t, results[0].Error)
	assert.Contains(t, results[0].Error.Error(), "permanent failure")

	mu.Lock()
	assert.Equal(t, 2, attempts, "handler should be called 2 times (1 initial + 1 retry)")
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// 18. DeadLetterQueue — failed events routed to DLQ
// ---------------------------------------------------------------------------

func TestDeadLetterQueue_RoutesFailedEvents(t *testing.T) {
	cfg := DefaultDispatcherConfig
	cfg.WorkerCount = 1
	cfg.BufferSize = 10
	cfg.RetryAttempts = 0
	cfg.EnableDeadLetter = true
	cfg.DeadLetterQueueSize = 5

	d, err := NewDispatcher(cfg)
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	require.NotNil(t, d.deadLetter)

	h := &mockHandler{
		name: "dlq-target",
		handleFn: func(_ context.Context, _ *Event) error {
			return errors.New("always fail")
		},
	}

	d.Subscribe("dlq.event", h)

	ctx := context.Background()
	_ = d.Publish(ctx, &Event{
		Type:   "dlq.event",
		Data:   []byte("fail-me"),
		Source: "dlq-test",
	})

	// Use PublishSync as a barrier — wait for DLQ event to be processed
	barrier := &mockHandler{name: "dlq-barrier"}
	barrierID := d.Subscribe("dlq.event", barrier)
	t.Cleanup(func() { d.Unsubscribe("dlq.event", barrierID) })
	d.PublishSync(ctx, &Event{Type: "dlq.event"})

	// Dead letter queue should now have at least one event
	select {
	case ev := <-d.deadLetter:
		assert.Equal(t, EventType("dlq.event"), ev.Type)
		assert.Equal(t, "dlq-test", ev.Source)
		assert.Equal(t, []byte("fail-me"), ev.Data)
	default:
		t.Fatal("expected at least one event in the dead-letter queue")
	}
}

// ---------------------------------------------------------------------------
// 19. DeadLetterQueue_Disabled_NoChannel — DLQ channel nil when disabled
// ---------------------------------------------------------------------------

func TestDeadLetterQueue_Disabled_NoChannel(t *testing.T) {
	cfg := DefaultDispatcherConfig
	cfg.WorkerCount = 1
	cfg.BufferSize = 10
	cfg.EnableDeadLetter = false

	d, err := NewDispatcher(cfg)
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	assert.Nil(t, d.deadLetter)
}

// ---------------------------------------------------------------------------
// 20. Supports_Filtering — mockHandler correctly implements Supports
// ---------------------------------------------------------------------------

func TestHandler_SupportsMethod(t *testing.T) {
	t.Parallel()

	h := &mockHandler{
		name: "supports-check",
		supports: map[EventType]bool{
			"type.a": true,
			"type.b": true,
			"type.c": false,
		},
	}

	assert.True(t, h.Supports("type.a"))
	assert.True(t, h.Supports("type.b"))
	assert.False(t, h.Supports("type.c"))
	// Unknown type: not in map, returns false (zero value)
	assert.False(t, h.Supports("type.unknown"))
}

// ---------------------------------------------------------------------------
// 21. Publish_ContextCancellation — cancelled context aborts blocked publish
// ---------------------------------------------------------------------------

func TestPublish_ContextCancellation(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: 1,
		BufferSize:  1,
	})
	require.NoError(t, err)

	unblock := make(chan struct{})
	started := make(chan struct{}, 1)
	blocker := &mockHandler{
		name: "blocker",
		handleFn: func(_ context.Context, _ *Event) error {
			select {
			case started <- struct{}{}:
			default:
			}
			<-unblock
			return nil
		},
	}
	d.Subscribe("cancel.event", blocker)

	ctx := context.Background()

	// Fill channel buffer; worker picks up event and blocks inside handler.
	err = d.Publish(ctx, &Event{Type: "cancel.event"})
	require.NoError(t, err)
	<-started

	// Fill remaining buffer slot.
	err = d.Publish(ctx, &Event{Type: "cancel.event"})
	require.NoError(t, err)

	// Channel is full, worker blocked. A third Publish with cancelled
	// context should yield context.Canceled.
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err = d.Publish(cancelCtx, &Event{Type: "cancel.event"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	close(unblock) // let the blocking handler finish
	d.Stop()
}

// ---------------------------------------------------------------------------
// 22. PublishSync_ContextCancellation — cancelled context with PublishSync
// ---------------------------------------------------------------------------

func TestPublishSync_ContextCancellation(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: 1,
		BufferSize:  1,
	})
	require.NoError(t, err)

	unblock := make(chan struct{})
	started := make(chan struct{}, 1)
	blocker := &mockHandler{
		name: "blocker",
		handleFn: func(_ context.Context, _ *Event) error {
			select {
			case started <- struct{}{}:
			default:
			}
			<-unblock
			return nil
		},
	}
	d.Subscribe("cancel.sync", blocker)

	ctx := context.Background()

	// Fill channel + worker
	err = d.Publish(ctx, &Event{Type: "cancel.sync"})
	require.NoError(t, err)
	<-started
	err = d.Publish(ctx, &Event{Type: "cancel.sync"})
	require.NoError(t, err)

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	results := d.PublishSync(cancelCtx, &Event{Type: "cancel.sync"})
	assert.Nil(t, results, "PublishSync should return nil when context is cancelled")

	close(unblock)
	d.Stop()
}

// ---------------------------------------------------------------------------
// 23. Subscribe_WithFilterAndPriority_Combined
// ---------------------------------------------------------------------------

func TestSubscribe_WithFilterAndPriority_Combined(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: 1,
		BufferSize:  10,
	})
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	var mu sync.Mutex
	var order []string

	hLow := &mockHandler{
		name: "low",
		handleFn: func(_ context.Context, _ *Event) error {
			mu.Lock()
			order = append(order, "low")
			mu.Unlock()
			return nil
		},
	}
	hHigh := &mockHandler{
		name: "high",
		handleFn: func(_ context.Context, _ *Event) error {
			mu.Lock()
			order = append(order, "high")
			mu.Unlock()
			return nil
		},
	}

	// Subscribe low first, then high — but high has higher priority
	d.Subscribe("combined.event", hLow, WithPriority(1))
	d.Subscribe("combined.event", hHigh, WithPriority(10))

	ctx := context.Background()
	results := d.PublishSync(ctx, &Event{Type: "combined.event"})

	require.Len(t, results, 2)
	mu.Lock()
	require.Len(t, order, 2)
	assert.Equal(t, "high", order[0], "high priority handler should run first")
	assert.Equal(t, "low", order[1], "low priority handler should run second")
	mu.Unlock()
}