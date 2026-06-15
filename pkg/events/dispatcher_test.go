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

// Compile-time check: mockHandler satisfies Handler.
var _ Handler = (*mockHandler)(nil)

// blockingHandle provides a handler that blocks inside Handle until the
// caller closes the unblock channel. Used to fill dispatcher buffers.
type blockingHandle struct {
	started chan struct{}
	unblock chan struct{}
	name    string
}

func newBlocking(name string) *blockingHandle {
	return &blockingHandle{
		started: make(chan struct{}),
		unblock: make(chan struct{}),
		name:    name,
	}
}

func (b *blockingHandle) handler() *mockHandler {
	return &mockHandler{
		name: b.name,
		handleFn: func(_ context.Context, _ *Event) error {
			close(b.started)
			<-b.unblock
			return nil
		},
	}
}

// ---------------------------------------------------------------------------
// 1. TestNewDispatcher — valid and invalid configs
// ---------------------------------------------------------------------------

func TestNewDispatcher(t *testing.T) {
	t.Parallel()

	t.Run("zero config uses defaults", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{})
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		assert.Equal(t, 4, d.config.WorkerCount)
		assert.Equal(t, 1000, d.config.BufferSize)
		assert.Equal(t, 0, d.config.RetryAttempts)
		assert.Equal(t, 100*time.Millisecond, d.config.RetryDelay)
		assert.False(t, d.config.EnableDeadLetter)
		assert.Equal(t, 100, d.config.DeadLetterQueueSize)
		assert.Nil(t, d.deadLetter)
	})

	t.Run("negative values fall back to defaults", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{
			WorkerCount: -1,
			BufferSize:  -5,
		})
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		assert.Equal(t, 4, d.config.WorkerCount)
		assert.Equal(t, 1000, d.config.BufferSize)
	})

	t.Run("explicit config values are honored", func(t *testing.T) {
		cfg := DispatcherConfig{
			WorkerCount:         8,
			BufferSize:          500,
			RetryAttempts:       3,
			RetryDelay:          50 * time.Millisecond,
			EnableDeadLetter:    true,
			DeadLetterQueueSize: 50,
		}
		d, err := NewDispatcher(cfg)
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		assert.Equal(t, 8, d.config.WorkerCount)
		assert.Equal(t, 500, d.config.BufferSize)
		assert.Equal(t, 3, d.config.RetryAttempts)
		assert.Equal(t, 50*time.Millisecond, d.config.RetryDelay)
		assert.True(t, d.config.EnableDeadLetter)
		assert.Equal(t, 50, d.config.DeadLetterQueueSize)
	})

	t.Run("dead letter enabled creates channel", func(t *testing.T) {
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
	})

	t.Run("dead letter disabled leaves channel nil", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{
			WorkerCount: 1,
			BufferSize:  10,
		})
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		assert.Nil(t, d.deadLetter)
	})

	t.Run("zero workers defaults to one", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{
			WorkerCount: 0,
			BufferSize:  10,
		})
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		assert.Equal(t, 1, d.config.WorkerCount, "WorkerCount=0 should default to 1")
	})
}

// ---------------------------------------------------------------------------
// 2. TestSubscribe — returns SubscriptionID
// ---------------------------------------------------------------------------

func TestSubscribe(t *testing.T) {
	d, err := NewDispatcher(DefaultDispatcherConfig)
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	t.Run("returns non-empty subscription ID", func(t *testing.T) {
		h := &mockHandler{name: "sub-test"}
		id := d.Subscribe("test.event", h)
		assert.NotEmpty(t, id)
	})

	t.Run("returns unique IDs for different subscriptions", func(t *testing.T) {
		h1 := &mockHandler{name: "h1"}
		h2 := &mockHandler{name: "h2"}
		id1 := d.Subscribe("multi.event", h1)
		id2 := d.Subscribe("multi.event", h2)
		assert.NotEqual(t, id1, id2, "subscription IDs should be unique")
	})

	t.Run("supports options", func(t *testing.T) {
		h := &mockHandler{name: "opts-test"}
		id := d.Subscribe("opts.event", h, WithPriority(5), WithFilter(func(ev *Event) bool {
			return true
		}))
		assert.NotEmpty(t, id)
	})

	t.Run("nil handler returns empty ID", func(t *testing.T) {
		id := d.Subscribe("nil.event", nil)
		assert.Empty(t, id, "subscribing nil handler should produce empty ID")
	})
}

// ---------------------------------------------------------------------------
// 3. TestPublishAsync — event delivered to subscribers
// ---------------------------------------------------------------------------

func TestPublishAsync(t *testing.T) {
	t.Run("event delivered to subscribed handler", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{WorkerCount: 1, BufferSize: 10})
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		delivered := make(chan *Event, 1)
		h := &mockHandler{
			name: "async-test",
			handleFn: func(_ context.Context, ev *Event) error {
				delivered <- ev
				return nil
			},
		}
		d.Subscribe("async.event", h)

		ctx := context.Background()
		err = d.Publish(ctx, &Event{
			Type:   "async.event",
			Data:   []byte(`{"key":"value"}`),
			Source: "test-suite",
		})
		require.NoError(t, err)

		// Use sync barrier to guarantee delivery before checking
		d.Subscribe("async.event", &mockHandler{
			name: "barrier",
			handleFn: func(_ context.Context, _ *Event) error {
				return nil
			},
		})
		_ = d.PublishSync(ctx, &Event{Type: "async.event"})

		select {
		case ev := <-delivered:
			assert.Equal(t, EventType("async.event"), ev.Type)
			assert.Equal(t, []byte(`{"key":"value"}`), ev.Data)
			assert.Equal(t, "test-suite", ev.Source)
		default:
			t.Fatal("expected event to be delivered to handler")
		}
	})

	t.Run("only subscribers with matching type receive event", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{WorkerCount: 1, BufferSize: 10})
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		var mu sync.Mutex
		received := []string(nil)

		h := &mockHandler{
			name: "type-a",
			handleFn: func(_ context.Context, _ *Event) error {
				mu.Lock()
				received = append(received, "type-a")
				mu.Unlock()
				return nil
			},
		}
		d.Subscribe("type.a", h)
		d.Subscribe("type.b", &mockHandler{
			name: "type-b",
			handleFn: func(_ context.Context, _ *Event) error {
				mu.Lock()
				received = append(received, "type-b")
				mu.Unlock()
				return nil
			},
		})

		ctx := context.Background()
		_ = d.Publish(ctx, &Event{Type: "type.a"})
		_ = d.PublishSync(ctx, &Event{Type: "type.a"})

		mu.Lock()
		assert.Equal(t, []string{"type-a"}, received, "only type-a handler should receive")
		mu.Unlock()
	})

	t.Run("publish to no subscribers does not panic", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{WorkerCount: 1, BufferSize: 10})
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		ctx := context.Background()
		err = d.Publish(ctx, &Event{Type: "unregistered.event"})
		assert.NoError(t, err, "Publish with no subscribers should not panic or error")
	})

	t.Run("concurrent publishes all delivered", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{
			WorkerCount:   1,
			BufferSize:    500,
			RetryAttempts: 1,
		})
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		var mu sync.Mutex
		total := 0

		h := &mockHandler{
			name: "concurrent-count",
			handleFn: func(_ context.Context, _ *Event) error {
				mu.Lock()
				total++
				mu.Unlock()
				return nil
			},
		}
		d.Subscribe("concurrent.event", h)

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

		// Barrier
		d.Subscribe("concurrent.event", &mockHandler{
			name: "barrier",
			handleFn: func(_ context.Context, _ *Event) error {
				return nil
			},
		})
		_ = d.PublishSync(ctx, &Event{Type: "concurrent.event"})

		mu.Lock()
		assert.Equal(t, publishers*eventsPer, total)
		mu.Unlock()
	})
}

// ---------------------------------------------------------------------------
// 4. TestPublishSync — returns handler results
// ---------------------------------------------------------------------------

func TestPublishSync(t *testing.T) {
	t.Run("returns results for all handlers", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{WorkerCount: 2, BufferSize: 10})
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

		names := make(map[string]error)
		for _, r := range results {
			names[r.HandlerName] = r.Error
		}
		assert.NoError(t, names["handler-one"])
		require.Error(t, names["handler-two"])
		assert.Contains(t, names["handler-two"].Error(), "fail from h2")
	})

	t.Run("returns nil for stopped dispatcher", func(t *testing.T) {
		d, err := NewDispatcher(DefaultDispatcherConfig)
		require.NoError(t, err)
		d.Stop()

		ctx := context.Background()
		results := d.PublishSync(ctx, &Event{Type: "after.stop"})
		assert.Nil(t, results)
	})

	t.Run("returns empty for zero subscribers", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{WorkerCount: 1, BufferSize: 10})
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		ctx := context.Background()
		results := d.PublishSync(ctx, &Event{Type: "orphan.event"})
		assert.Empty(t, results)
	})
}

// ---------------------------------------------------------------------------
// 5. TestUnsubscribe — removes subscriber, false for unknown
// ---------------------------------------------------------------------------

func TestUnsubscribe(t *testing.T) {
	d, err := NewDispatcher(DefaultDispatcherConfig)
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	t.Run("returns true when removing existing subscriber", func(t *testing.T) {
		h := &mockHandler{name: "unsub-test"}
		id := d.Subscribe("test.event", h)

		ok := d.Unsubscribe("test.event", id)
		assert.True(t, ok)
	})

	t.Run("idempotent — second call returns false", func(t *testing.T) {
		h := &mockHandler{name: "unsub-dup"}
		id := d.Subscribe("dup.event", h)

		ok := d.Unsubscribe("dup.event", id)
		assert.True(t, ok)

		ok = d.Unsubscribe("dup.event", id)
		assert.False(t, ok)
	})

	t.Run("returns false for non-existent event type", func(t *testing.T) {
		ok := d.Unsubscribe("nonexistent.event", "sub-never-registered")
		assert.False(t, ok)
	})

	t.Run("returns false for wrong event type", func(t *testing.T) {
		h := &mockHandler{name: "wrong-type"}
		id := d.Subscribe("type.a", h)

		ok := d.Unsubscribe("type.b", id)
		assert.False(t, ok)

		// Original subscription still intact
		ok = d.Unsubscribe("type.a", id)
		assert.True(t, ok)
	})

	t.Run("subscriber no longer receives after unsubscribe", func(t *testing.T) {
		d2, err := NewDispatcher(DispatcherConfig{WorkerCount: 1, BufferSize: 10})
		require.NoError(t, err)
		t.Cleanup(d2.Stop)

		var mu sync.Mutex
		count := 0

		h := &mockHandler{
			name: "unsub-after",
			handleFn: func(_ context.Context, _ *Event) error {
				mu.Lock()
				count++
				mu.Unlock()
				return nil
			},
		}
		id := d2.Subscribe("unsub.event", h)

		ctx := context.Background()
		_ = d2.PublishSync(ctx, &Event{Type: "unsub.event"})

		mu.Lock()
		assert.Equal(t, 1, count)
		mu.Unlock()

		d2.Unsubscribe("unsub.event", id)
		_ = d2.PublishSync(ctx, &Event{Type: "unsub.event"})

		mu.Lock()
		assert.Equal(t, 1, count, "should not receive after unsubscribe")
		mu.Unlock()
	})
}

// ---------------------------------------------------------------------------
// 6. TestStop — idempotent, blocked after stop
// ---------------------------------------------------------------------------

func TestStop(t *testing.T) {
	t.Run("stops dispatcher", func(t *testing.T) {
		d, err := NewDispatcher(DefaultDispatcherConfig)
		require.NoError(t, err)

		d.Stop()
		assert.True(t, d.stopped)
	})

	t.Run("idempotent — multiple calls do not panic", func(t *testing.T) {
		d, err := NewDispatcher(DefaultDispatcherConfig)
		require.NoError(t, err)

		d.Stop()
		d.Stop()
		d.Stop()
		assert.True(t, d.stopped)
	})

	t.Run("Publish returns error after stop", func(t *testing.T) {
		d, err := NewDispatcher(DefaultDispatcherConfig)
		require.NoError(t, err)

		d.Stop()

		ctx := context.Background()
		err = d.Publish(ctx, &Event{Type: "after.stop"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dispatcher is stopped")
	})

	t.Run("PublishSync returns nil after stop", func(t *testing.T) {
		d, err := NewDispatcher(DefaultDispatcherConfig)
		require.NoError(t, err)

		d.Stop()

		ctx := context.Background()
		results := d.PublishSync(ctx, &Event{Type: "after.stop"})
		assert.Nil(t, results)
	})

	t.Run("Subscribe still works after stop", func(t *testing.T) {
		d, err := NewDispatcher(DefaultDispatcherConfig)
		require.NoError(t, err)

		d.Stop()

		id := d.Subscribe("after.stop", &mockHandler{name: "after-stop"})
		assert.NotEmpty(t, id, "subscribe should still work after stop")
	})
}

// ---------------------------------------------------------------------------
// 7. TestRetry — handler retries then succeeds/fails
// ---------------------------------------------------------------------------

func TestRetry(t *testing.T) {
	t.Run("handler succeeds after retries", func(t *testing.T) {
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
			name: "retry-succeed",
			handleFn: func(_ context.Context, _ *Event) error {
				mu.Lock()
				attempts++
				count := attempts
				mu.Unlock()
				if count < 3 { // fail first 2 times (RetryAttempts=2 = up to 3 total)
					return errors.New("transient error")
				}
				return nil
			},
		}

		d.Subscribe("retry.event", h)

		ctx := context.Background()
		results := d.PublishSync(ctx, &Event{Type: "retry.event"})

		require.Len(t, results, 1)
		assert.Equal(t, "retry-succeed", results[0].HandlerName)
		assert.NoError(t, results[0].Error, "handler should succeed on 3rd attempt")

		mu.Lock()
		assert.Equal(t, 3, attempts, "handler should be called 3 times (1 initial + 2 retries)")
		mu.Unlock()
	})

	t.Run("retry exhaustion returns error", func(t *testing.T) {
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
			name: "retry-exhaust",
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
	})

	t.Run("no retry when RetryAttempts is zero", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{
			WorkerCount:   1,
			BufferSize:    10,
			RetryAttempts: 0,
		})
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		attempts := 0

		h := &mockHandler{
			name: "no-retry",
			handleFn: func(_ context.Context, _ *Event) error {
				attempts++
				return errors.New("fail once")
			},
		}
		d.Subscribe("no.retry", h)

		ctx := context.Background()
		results := d.PublishSync(ctx, &Event{Type: "no.retry"})
		require.Len(t, results, 1)
		require.Error(t, results[0].Error)
		assert.Equal(t, 1, attempts, "handler should only be called once with RetryAttempts=0")
	})
}

// ---------------------------------------------------------------------------
// 8. TestDeadLetter — dead letter queue behavior
// ---------------------------------------------------------------------------

func TestDeadLetter(t *testing.T) {
	t.Run("failed events routed to dead letter queue", func(t *testing.T) {
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

		// Barrier
		barrierID := d.Subscribe("dlq.event", &mockHandler{name: "dlq-barrier"})
		t.Cleanup(func() { d.Unsubscribe("dlq.event", barrierID) })
		d.PublishSync(ctx, &Event{Type: "dlq.event"})

		select {
		case ev := <-d.deadLetter:
			assert.Equal(t, EventType("dlq.event"), ev.Type)
			assert.Equal(t, "dlq-test", ev.Source)
			assert.Equal(t, []byte("fail-me"), ev.Data)
		default:
			t.Fatal("expected at least one event in the dead-letter queue")
		}
	})

	t.Run("multiple failed events all reach dead letter", func(t *testing.T) {
		cfg := DefaultDispatcherConfig
		cfg.WorkerCount = 1
		cfg.BufferSize = 10
		cfg.RetryAttempts = 0
		cfg.EnableDeadLetter = true
		cfg.DeadLetterQueueSize = 10

		d, err := NewDispatcher(cfg)
		require.NoError(t, err)
		t.Cleanup(d.Stop)

		h := &mockHandler{
			name: "dlq-multi",
			handleFn: func(_ context.Context, _ *Event) error {
				return errors.New("fail all")
			},
		}
		d.Subscribe("dlq.multi", h)

		ctx := context.Background()
		for i := 0; i < 3; i++ {
			_ = d.Publish(ctx, &Event{Type: "dlq.multi", Data: []byte{byte(i)}})
		}

		barrierID := d.Subscribe("dlq.multi", &mockHandler{name: "barrier"})
		t.Cleanup(func() { d.Unsubscribe("dlq.multi", barrierID) })
		_ = d.PublishSync(ctx, &Event{Type: "dlq.multi"})

		count := 0
		for {
			select {
			case <-d.deadLetter:
				count++
			default:
				goto done
			}
		}
	done:
		assert.Equal(t, 3, count, "all 3 failed events should be in DLQ")
	})

	t.Run("dead letter nil when disabled", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{
			WorkerCount: 1,
			BufferSize:  10,
		})
		require.NoError(t, err)
		t.Cleanup(d.Stop)
		assert.Nil(t, d.deadLetter)
	})
}

// ---------------------------------------------------------------------------
// 9. TestWithPriority — priority sorting
// ---------------------------------------------------------------------------

func TestWithPriority(t *testing.T) {
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
	hMedium := &mockHandler{
		name: "medium",
		handleFn: func(_ context.Context, _ *Event) error {
			mu.Lock()
			order = append(order, "medium")
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

	// Subscribe in low-to-high order; execution should be high-to-low
	d.Subscribe("priority.event", hLow, WithPriority(1))
	d.Subscribe("priority.event", hMedium, WithPriority(5))
	d.Subscribe("priority.event", hHigh, WithPriority(10))

	ctx := context.Background()
	results := d.PublishSync(ctx, &Event{Type: "priority.event"})

	require.Len(t, results, 3)
	mu.Lock()
	require.Len(t, order, 3)
	assert.Equal(t, "high", order[0], "highest priority handler should run first")
	assert.Equal(t, "medium", order[1], "medium priority handler should run second")
	assert.Equal(t, "low", order[2], "lowest priority handler should run last")
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// 10. TestWithFilter — filter option
// ---------------------------------------------------------------------------

func TestWithFilter(t *testing.T) {
	t.Run("filter blocks events that do not match", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{WorkerCount: 1, BufferSize: 10})
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

		d.Subscribe("filter.event", h, WithFilter(func(ev *Event) bool {
			return ev.Source == "allowed"
		}))

		ctx := context.Background()
		for i := 0; i < 3; i++ {
			err := d.Publish(ctx, &Event{Type: "filter.event", Source: "allowed"})
			require.NoError(t, err)
		}
		for i := 0; i < 2; i++ {
			err := d.Publish(ctx, &Event{Type: "filter.event", Source: "blocked"})
			require.NoError(t, err)
		}

		// Barrier
		barrierID := d.Subscribe("filter.event", &mockHandler{name: "barrier"})
		t.Cleanup(func() { d.Unsubscribe("filter.event", barrierID) })
		d.PublishSync(ctx, &Event{Type: "filter.event", Source: "sentinel"})

		mu.Lock()
		assert.Equal(t, 3, count, "handler should only receive events with Source='allowed'")
		mu.Unlock()
	})

	t.Run("nil filter treated as pass-through", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{WorkerCount: 1, BufferSize: 10})
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

		id := d.Subscribe("no-filter.event", h, WithFilter(nil))
		t.Cleanup(func() { d.Unsubscribe("no-filter.event", id) })

		ctx := context.Background()
		results := d.PublishSync(ctx, &Event{Type: "no-filter.event"})
		require.Len(t, results, 1)
		assert.True(t, called)
	})

	t.Run("filter works with priority", func(t *testing.T) {
		d, err := NewDispatcher(DispatcherConfig{WorkerCount: 1, BufferSize: 10})
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

		d.Subscribe("combined.event", hLow, WithPriority(1), WithFilter(func(ev *Event) bool {
			return true
		}))
		d.Subscribe("combined.event", hHigh, WithPriority(10), WithFilter(func(ev *Event) bool {
			return true
		}))

		ctx := context.Background()
		results := d.PublishSync(ctx, &Event{Type: "combined.event"})
		require.Len(t, results, 2)
		mu.Lock()
		require.Len(t, order, 2)
		assert.Equal(t, "high", order[0])
		assert.Equal(t, "low", order[1])
		mu.Unlock()
	})
}

// ---------------------------------------------------------------------------
// 11. TestDefaultConfig — correct defaults
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultDispatcherConfig
	assert.Equal(t, 4, cfg.WorkerCount)
	assert.Equal(t, 1000, cfg.BufferSize)
	assert.Equal(t, 0, cfg.RetryAttempts)
	assert.Equal(t, 100*time.Millisecond, cfg.RetryDelay)
	assert.False(t, cfg.EnableDeadLetter)
	assert.Equal(t, 100, cfg.DeadLetterQueueSize)
}

// ---------------------------------------------------------------------------
// 12. TestEventStruct — Event creation
// ---------------------------------------------------------------------------

func TestEventStruct(t *testing.T) {
	t.Parallel()

	t.Run("event with all fields", func(t *testing.T) {
		ev := &Event{
			Type:   "user.created",
			Data:   []byte(`{"id":42}`),
			Source: "auth-service",
		}
		assert.Equal(t, EventType("user.created"), ev.Type)
		assert.Equal(t, []byte(`{"id":42}`), ev.Data)
		assert.Equal(t, "auth-service", ev.Source)
	})

	t.Run("zero-value event", func(t *testing.T) {
		var zero Event
		assert.Empty(t, zero.Type)
		assert.Nil(t, zero.Data)
		assert.Empty(t, zero.Source)
	})

	t.Run("event with empty data", func(t *testing.T) {
		ev := &Event{
			Type:   "heartbeat",
			Data:   []byte{},
			Source: "system",
		}
		assert.Equal(t, EventType("heartbeat"), ev.Type)
		assert.Empty(t, ev.Data)
		assert.Equal(t, "system", ev.Source)
	})
}

// ---------------------------------------------------------------------------
// Additional: context cancellation tests
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

	// Channel full, worker blocked. Third Publish with cancelled context
	// should yield context.Canceled.
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err = d.Publish(cancelCtx, &Event{Type: "cancel.event"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)

	close(unblock)
	d.Stop()
}

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
// 11. TestGetStats_InitialCounts — GetStats returns zero-valued initial stats
// ---------------------------------------------------------------------------

func TestGetStats_InitialCounts(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: 2,
		BufferSize:  100,
	})
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	stats := d.GetStats()
	assert.NotNil(t, stats, "GetStats must return a non-nil value")

	// All counters start at zero.
	assert.Zero(t, stats.EventsPublished, "initial EventsPublished should be 0")
	assert.Zero(t, stats.EventsFailed, "initial EventsFailed should be 0")
	assert.Zero(t, stats.EventsInFlight, "initial EventsInFlight should be 0")
}

// ---------------------------------------------------------------------------
// 12. TestHandlerSupports_False_SkipsEvent — handler with Supports()=false
//     is skipped by the dispatcher
// ---------------------------------------------------------------------------

func TestHandlerSupports_False_SkipsEvent(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: 1,
		BufferSize:  10,
	})
	require.NoError(t, err)
	t.Cleanup(d.Stop)

	var mu sync.Mutex
	callCount := 0

	h := &mockHandler{
		name: "supports-filter",
		supports: map[EventType]bool{
			"allowed.event": true,
			"blocked.event": false,
		},
		handleFn: func(_ context.Context, _ *Event) error {
			mu.Lock()
			callCount++
			mu.Unlock()
			return nil
		},
	}

	// Subscribe the same handler to both event types.
	d.Subscribe("allowed.event", h)
	d.Subscribe("blocked.event", h)

	ctx := context.Background()

	// Publish to blocked event type — handler must NOT be called.
	resultsBlocked := d.PublishSync(ctx, &Event{Type: "blocked.event"})
	assert.Empty(t, resultsBlocked, "handler with Supports=false should be skipped")

	mu.Lock()
	assert.Equal(t, 0, callCount, "handler should not be called for unsupported type")
	mu.Unlock()

	// Publish to allowed event type — handler MUST be called.
	resultsAllowed := d.PublishSync(ctx, &Event{Type: "allowed.event"})
	require.Len(t, resultsAllowed, 1)
	assert.Equal(t, "supports-filter", resultsAllowed[0].HandlerName)

	mu.Lock()
	assert.Equal(t, 1, callCount, "handler should be called for supported type")
	mu.Unlock()
}

// ---------------------------------------------------------------------------
// 13. TestPublish_BufferOverflow_ReturnsError — full buffer returns error,
//     not a panic
// ---------------------------------------------------------------------------

func TestPublish_BufferOverflow_ReturnsError(t *testing.T) {
	d, err := NewDispatcher(DispatcherConfig{
		WorkerCount: 1,
		BufferSize:  2,
	})
	require.NoError(t, err)

	block := newBlocking("overflow")
	d.Subscribe("overflow.event", block.handler())

	ctx := context.Background()

	// Fill the buffer: first event gets consumed by the worker and blocks
	// inside the handler.
	err = d.Publish(ctx, &Event{Type: "overflow.event"})
	require.NoError(t, err)
	<-block.started

	// Fill the remaining buffer slot (BufferSize=2, one slot occupied by
	// the event being processed, so 1 slot left in the channel).
	err = d.Publish(ctx, &Event{Type: "overflow.event"})
	require.NoError(t, err)

	// Buffer is now full. The next Publish must NOT panic. With a short
	// timeout context, it should return an error or timeout — but never
	// crash.
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = d.Publish(timeoutCtx, &Event{Type: "overflow.event"})
	require.Error(t, err, "publishing to a full buffer should return an error, not panic")

	close(block.unblock)
	d.Stop()
}
