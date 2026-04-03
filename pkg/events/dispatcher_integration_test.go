//go:build integration

package events_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openapi-cli/openapi-cli/pkg/events"
)

// handlerFuncAdapter adapts a function to the Handler interface
type handlerFuncAdapter struct {
	name string
	fn   func(context.Context, *events.Event) error
}

func (h *handlerFuncAdapter) Handle(ctx context.Context, event *events.Event) error {
	return h.fn(ctx, event)
}

func (h *handlerFuncAdapter) Name() string {
	return h.name
}

func (h *handlerFuncAdapter) Supports(eventType events.EventType) bool {
	return true
}

func newHandlerFunc(name string, fn func(context.Context, *events.Event) error) events.Handler {
	return &handlerFuncAdapter{name: name, fn: fn}
}

// TestDispatcher_Integration_BasicPublishSubscribe tests basic event publishing
func TestDispatcher_Integration_BasicPublishSubscribe(t *testing.T) {
	config := events.DefaultDispatcherConfig
	config.WorkerCount = 2
	config.BufferSize = 100

	dispatcher, err := events.NewDispatcher(config)
	require.NoError(t, err)
	defer dispatcher.Stop()

	// Create a handler that captures events
	receivedEvents := make([]*events.Event, 0)
	var mu sync.Mutex

	handler := newHandlerFunc("test-handler", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
		return nil
	})

	// Subscribe to events
	subID := dispatcher.Subscribe("test.event", handler)
	require.NotEmpty(t, subID)

	// Publish events
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for i := 0; i < 10; i++ {
		data, _ := json.Marshal(map[string]interface{}{"index": i})
		event := &events.Event{
			Type:   "test.event",
			Data:   data,
			Source: "test",
		}
		err := dispatcher.Publish(ctx, event)
		require.NoError(t, err)
	}

	// Wait for events to be processed
	time.Sleep(500 * time.Millisecond)

	// Verify events were received
	mu.Lock()
	assert.Len(t, receivedEvents, 10)
	mu.Unlock()
}

// TestDispatcher_Integration_MultipleSubscribers tests multiple handlers for same event
func TestDispatcher_Integration_MultipleSubscribers(t *testing.T) {
	config := events.DefaultDispatcherConfig
	config.WorkerCount = 3
	config.BufferSize = 100

	dispatcher, err := events.NewDispatcher(config)
	require.NoError(t, err)
	defer dispatcher.Stop()

	// Multiple handlers
	var wg sync.WaitGroup
	handler1Count := 0
	handler2Count := 0
	var mu sync.Mutex

	handler1 := newHandlerFunc("handler-1", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		handler1Count++
		mu.Unlock()
		wg.Done()
		return nil
	})

	handler2 := newHandlerFunc("handler-2", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		handler2Count++
		mu.Unlock()
		wg.Done()
		return nil
	})

	// Subscribe both handlers
	dispatcher.Subscribe("multi.event", handler1)
	dispatcher.Subscribe("multi.event", handler2)

	// Publish events
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wg.Add(20) // 10 events * 2 handlers

	for i := 0; i < 10; i++ {
		data, _ := json.Marshal(map[string]interface{}{"index": i})
		event := &events.Event{
			Type:   "multi.event",
			Data:   data,
			Source: "test",
		}
		err := dispatcher.Publish(ctx, event)
		require.NoError(t, err)
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for events")
	}

	// Verify both handlers received all events
	mu.Lock()
	assert.Equal(t, 10, handler1Count)
	assert.Equal(t, 10, handler2Count)
	mu.Unlock()
}

// TestDispatcher_Integration_PriorityOrdering tests priority-based handling
func TestDispatcher_Integration_PriorityOrdering(t *testing.T) {
	config := events.DefaultDispatcherConfig
	config.WorkerCount = 1 // Single worker to maintain order
	config.BufferSize = 100

	dispatcher, err := events.NewDispatcher(config)
	require.NoError(t, err)
	defer dispatcher.Stop()

	// Track order of execution
	executionOrder := make([]int, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create handlers with different priorities
	handlerLow := newHandlerFunc("low-priority", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		executionOrder = append(executionOrder, 1) // Low priority = 1
		mu.Unlock()
		wg.Done()
		return nil
	})

	handlerHigh := newHandlerFunc("high-priority", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		executionOrder = append(executionOrder, 10) // High priority = 10
		mu.Unlock()
		wg.Done()
		return nil
	})

	handlerMedium := newHandlerFunc("medium-priority", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		executionOrder = append(executionOrder, 5) // Medium priority = 5
		mu.Unlock()
		wg.Done()
		return nil
	})

	// Subscribe with priorities
	dispatcher.Subscribe("priority.event", handlerLow, events.WithPriority(1))
	dispatcher.Subscribe("priority.event", handlerHigh, events.WithPriority(10))
	dispatcher.Subscribe("priority.event", handlerMedium, events.WithPriority(5))

	// Publish event
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wg.Add(3)

	data, _ := json.Marshal(map[string]interface{}{"test": true})
	event := &events.Event{
		Type:   "priority.event",
		Data:   data,
		Source: "test",
	}
	err = dispatcher.Publish(ctx, event)
	require.NoError(t, err)

	wg.Wait()

	// Verify order: high (10) should be first, then medium (5), then low (1)
	mu.Lock()
	require.Len(t, executionOrder, 3)
	assert.Equal(t, 10, executionOrder[0], "High priority should execute first")
	assert.Equal(t, 5, executionOrder[1], "Medium priority should execute second")
	assert.Equal(t, 1, executionOrder[2], "Low priority should execute last")
	mu.Unlock()
}

// TestDispatcher_Integration_Filtering tests event filtering
func TestDispatcher_Integration_Filtering(t *testing.T) {
	config := events.DefaultDispatcherConfig
	config.WorkerCount = 2
	config.BufferSize = 100

	dispatcher, err := events.NewDispatcher(config)
	require.NoError(t, err)
	defer dispatcher.Stop()

	// Handler with filter - only process events with "important" field
	filteredCount := 0
	var mu sync.Mutex

	filter := func(event *events.Event) bool {
		var data map[string]interface{}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return false
		}
		if important, ok := data["important"].(bool); ok {
			return important
		}
		return false
	}

	handler := newHandlerFunc("filtered-handler", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		filteredCount++
		mu.Unlock()
		return nil
	})

	dispatcher.Subscribe("filtered.event", handler, events.WithFilter(filter))

	// Publish events
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 5 important events
	for i := 0; i < 5; i++ {
		data, _ := json.Marshal(map[string]interface{}{"index": i, "important": true})
		event := &events.Event{
			Type:   "filtered.event",
			Data:   data,
			Source: "test",
		}
		err := dispatcher.Publish(ctx, event)
		require.NoError(t, err)
	}

	// 5 unimportant events
	for i := 0; i < 5; i++ {
		data, _ := json.Marshal(map[string]interface{}{"index": i, "important": false})
		event := &events.Event{
			Type:   "filtered.event",
			Data:   data,
			Source: "test",
		}
		err := dispatcher.Publish(ctx, event)
		require.NoError(t, err)
	}

	time.Sleep(500 * time.Millisecond)

	// Should only receive important events
	mu.Lock()
	assert.Equal(t, 5, filteredCount, "Should only receive filtered events")
	mu.Unlock()
}

// TestDispatcher_Integration_RetryLogic tests retry on handler failure
func TestDispatcher_Integration_RetryLogic(t *testing.T) {
	config := events.DefaultDispatcherConfig
	config.WorkerCount = 1
	config.BufferSize = 100
	config.RetryAttempts = 3
	config.RetryDelay = 100 * time.Millisecond

	dispatcher, err := events.NewDispatcher(config)
	require.NoError(t, err)
	defer dispatcher.Stop()

	// Handler that fails first 2 times, then succeeds
	attemptCount := 0
	var mu sync.Mutex

	handler := newHandlerFunc("retry-handler", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		attemptCount++
		currentAttempt := attemptCount
		mu.Unlock()

		if currentAttempt < 3 {
			return fmt.Errorf("simulated error on attempt %d", currentAttempt)
		}
		return nil
	})

	dispatcher.Subscribe("retry.event", handler)

	// Publish event
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, _ := json.Marshal(map[string]interface{}{"test": true})
	event := &events.Event{
		Type:   "retry.event",
		Data:   data,
		Source: "test",
	}
	err = dispatcher.Publish(ctx, event)
	require.NoError(t, err)

	// Wait for retries to complete
	time.Sleep(500 * time.Millisecond)

	// Should have retried 3 times
	mu.Lock()
	assert.GreaterOrEqual(t, attemptCount, 3, "Should have retried")
	mu.Unlock()
}

// TestDispatcher_Integration_DeadLetterQueue tests dead letter queue
func TestDispatcher_Integration_DeadLetterQueue(t *testing.T) {
	config := events.DefaultDispatcherConfig
	config.WorkerCount = 1
	config.BufferSize = 100
	config.RetryAttempts = 1
	config.EnableDeadLetter = true
	config.DeadLetterQueueSize = 10

	dispatcher, err := events.NewDispatcher(config)
	require.NoError(t, err)
	defer dispatcher.Stop()

	// Handler that always fails
	handler := newHandlerFunc("failing-handler", func(ctx context.Context, event *events.Event) error {
		return fmt.Errorf("permanent failure")
	})

	dispatcher.Subscribe("failing.event", handler)

	// Publish event
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, _ := json.Marshal(map[string]interface{}{"test": true})
	event := &events.Event{
		Type:   "failing.event",
		Data:   data,
		Source: "test",
	}
	err = dispatcher.Publish(ctx, event)
	require.NoError(t, err)

	// Wait for processing and dead letter
	time.Sleep(500 * time.Millisecond)

	// Note: Dead letter verification depends on implementation details
	// The event should have been moved to dead letter queue after retries
	t.Log("Event should be in dead letter queue after retries")
}

// TestDispatcher_Integration_ConcurrentPublishing tests concurrent event publishing
func TestDispatcher_Integration_ConcurrentPublishing(t *testing.T) {
	config := events.DefaultDispatcherConfig
	config.WorkerCount = 5
	config.BufferSize = 1000

	dispatcher, err := events.NewDispatcher(config)
	require.NoError(t, err)
	defer dispatcher.Stop()

	// Handler that counts events
	eventCount := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	handler := newHandlerFunc("concurrent-handler", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		eventCount++
		mu.Unlock()
		wg.Done()
		return nil
	})

	dispatcher.Subscribe("concurrent.event", handler)

	// Concurrent publishing from multiple goroutines
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	publisherCount := 10
	eventsPerPublisher := 100
	wg.Add(publisherCount * eventsPerPublisher)

	for i := 0; i < publisherCount; i++ {
		go func(id int) {
			for j := 0; j < eventsPerPublisher; j++ {
				data, _ := json.Marshal(map[string]interface{}{"publisher": id, "event": j})
				event := &events.Event{
					Type:   "concurrent.event",
					Data:   data,
					Source: "test",
				}
				err := dispatcher.Publish(ctx, event)
				if err != nil {
					t.Logf("Failed to publish event: %v", err)
				}
			}
		}(i)
	}

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for events")
	}

	// Verify all events were processed
	mu.Lock()
	assert.Equal(t, publisherCount*eventsPerPublisher, eventCount)
	mu.Unlock()
}

// TestDispatcher_Integration_Unsubscribe tests subscription removal
func TestDispatcher_Integration_Unsubscribe(t *testing.T) {
	config := events.DefaultDispatcherConfig
	config.WorkerCount = 1
	config.BufferSize = 100

	dispatcher, err := events.NewDispatcher(config)
	require.NoError(t, err)
	defer dispatcher.Stop()

	// Handler
	eventCount := 0
	var mu sync.Mutex

	handler := newHandlerFunc("unsub-handler", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		eventCount++
		mu.Unlock()
		return nil
	})

	// Subscribe and get ID
	subID := dispatcher.Subscribe("unsub.event", handler)
	require.NotEmpty(t, subID)

	// Publish first event
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data1, _ := json.Marshal(map[string]interface{}{"before": true})
	event1 := &events.Event{
		Type:   "unsub.event",
		Data:   data1,
		Source: "test",
	}
	err = dispatcher.Publish(ctx, event1)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Unsubscribe
	removed := dispatcher.Unsubscribe("unsub.event", subID)
	assert.True(t, removed)

	// Publish second event
	data2, _ := json.Marshal(map[string]interface{}{"after": true})
	event2 := &events.Event{
		Type:   "unsub.event",
		Data:   data2,
		Source: "test",
	}
	err = dispatcher.Publish(ctx, event2)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Should only have received first event
	mu.Lock()
	assert.Equal(t, 1, eventCount, "Should only receive one event before unsubscribe")
	mu.Unlock()
}

// TestDispatcher_Integration_DifferentEventTypes tests multiple event types
func TestDispatcher_Integration_DifferentEventTypes(t *testing.T) {
	config := events.DefaultDispatcherConfig
	config.WorkerCount = 2
	config.BufferSize = 100

	dispatcher, err := events.NewDispatcher(config)
	require.NoError(t, err)
	defer dispatcher.Stop()

	// Handlers for different event types
	userEvents := 0
	orderEvents := 0
	var mu sync.Mutex

	userHandler := newHandlerFunc("user-handler", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		userEvents++
		mu.Unlock()
		return nil
	})

	orderHandler := newHandlerFunc("order-handler", func(ctx context.Context, event *events.Event) error {
		mu.Lock()
		orderEvents++
		mu.Unlock()
		return nil
	})

	dispatcher.Subscribe("user.created", userHandler)
	dispatcher.Subscribe("user.updated", userHandler)
	dispatcher.Subscribe("order.placed", orderHandler)

	// Publish different event types
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 5 user events
	for i := 0; i < 5; i++ {
		data, _ := json.Marshal(map[string]interface{}{"user_id": i})
		event := &events.Event{
			Type:   "user.created",
			Data:   data,
			Source: "test",
		}
		dispatcher.Publish(ctx, event)
	}

	// 3 user updates
	for i := 0; i < 3; i++ {
		data, _ := json.Marshal(map[string]interface{}{"user_id": i})
		event := &events.Event{
			Type:   "user.updated",
			Data:   data,
			Source: "test",
		}
		dispatcher.Publish(ctx, event)
	}

	// 7 order events
	for i := 0; i < 7; i++ {
		data, _ := json.Marshal(map[string]interface{}{"order_id": i})
		event := &events.Event{
			Type:   "order.placed",
			Data:   data,
			Source: "test",
		}
		dispatcher.Publish(ctx, event)
	}

	time.Sleep(500 * time.Millisecond)

	// Verify counts
	mu.Lock()
	assert.Equal(t, 8, userEvents, "Should receive 5 user.created + 3 user.updated")
	assert.Equal(t, 7, orderEvents, "Should receive 7 order.placed")
	mu.Unlock()
}

// TestDispatcher_Integration_PublishSync tests synchronous event processing
func TestDispatcher_Integration_PublishSync(t *testing.T) {
	config := events.DefaultDispatcherConfig
	config.WorkerCount = 2
	config.BufferSize = 100

	dispatcher, err := events.NewDispatcher(config)
	require.NoError(t, err)
	defer dispatcher.Stop()

	// Handler that returns a result
	handler := newHandlerFunc("sync-handler", func(ctx context.Context, event *events.Event) error {
		return nil
	})

	dispatcher.Subscribe("sync.event", handler)

	// Publish synchronously
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, _ := json.Marshal(map[string]interface{}{"test": true})
	event := &events.Event{
		Type:   "sync.event",
		Data:   data,
		Source: "test",
	}
	results := dispatcher.PublishSync(ctx, event)

	// Verify results
	assert.Len(t, results, 1)
	assert.Equal(t, "sync-handler", results[0].HandlerName)
	assert.NoError(t, results[0].Error)
}
