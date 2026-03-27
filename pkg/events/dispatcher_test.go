package events

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLogger is a simple logger for testing.
type testLogger struct {
	mu       sync.Mutex
	messages []string
}

func (l *testLogger) Error(msg string, fields ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, "ERROR: "+msg)
}

func (l *testLogger) Info(msg string, fields ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, "INFO: "+msg)
}

func (l *testLogger) Debug(msg string, fields ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.messages = append(l.messages, "DEBUG: "+msg)
}

// testHandler is a simple handler for testing.
type testHandler struct {
	name       string
	eventTypes []EventType
	handler    func(context.Context, *Event) error
}

func (h *testHandler) Handle(ctx context.Context, event *Event) error {
	if h.handler != nil {
		return h.handler(ctx, event)
	}
	return nil
}

func (h *testHandler) Name() string {
	return h.name
}

func (h *testHandler) Supports(eventType EventType) bool {
	for _, et := range h.eventTypes {
		if et == eventType {
			return true
		}
	}
	return false
}

func TestNewDispatcher(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := DefaultDispatcherConfig
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		require.NotNil(t, dispatcher)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()
	})

	t.Run("invalid buffer size", func(t *testing.T) {
		config := DispatcherConfig{
			BufferSize:  0,
			WorkerCount: 1,
		}
		dispatcher, err := NewDispatcher(config)
		assert.Error(t, err)
		assert.Nil(t, dispatcher)
		assert.Contains(t, err.Error(), "buffer_size must be >= 1")
	})

	t.Run("invalid worker count", func(t *testing.T) {
		config := DispatcherConfig{
			BufferSize:  100,
			WorkerCount: 0,
		}
		dispatcher, err := NewDispatcher(config)
		assert.Error(t, err)
		assert.Nil(t, dispatcher)
		assert.Contains(t, err.Error(), "worker_count must be >= 1")
	})

	t.Run("invalid max queue size", func(t *testing.T) {
		config := DispatcherConfig{
			BufferSize:   100,
			WorkerCount:  1,
			MaxQueueSize: 0,
		}
		dispatcher, err := NewDispatcher(config)
		assert.Error(t, err)
		assert.Nil(t, dispatcher)
		assert.Contains(t, err.Error(), "max_queue_size must be >= 1")
	})

	t.Run("with logger option", func(t *testing.T) {
		logger := &testLogger{}
		config := DefaultDispatcherConfig
		dispatcher, err := NewDispatcher(config, WithLogger(logger))
		require.NoError(t, err)
		require.NotNil(t, dispatcher)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()
	})
}

func TestDispatcher_Subscribe(t *testing.T) {
	config := DefaultDispatcherConfig
	dispatcher, err := NewDispatcher(config)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dispatcher.Stop())
	}()

	handler := &testHandler{
		name:       "test-handler",
		eventTypes: []EventType{EventTypeCustom},
	}

	subscriptionID := dispatcher.Subscribe(EventTypeCustom, handler)
	assert.NotEmpty(t, subscriptionID)
	assert.Contains(t, subscriptionID, "sub_")
}

func TestDispatcher_Unsubscribe(t *testing.T) {
	config := DefaultDispatcherConfig
	dispatcher, err := NewDispatcher(config)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dispatcher.Stop())
	}()

	handler := &testHandler{
		name:       "test-handler",
		eventTypes: []EventType{EventTypeCustom},
	}

	subscriptionID := dispatcher.Subscribe(EventTypeCustom, handler)
	assert.NotEmpty(t, subscriptionID)

	removed := dispatcher.Unsubscribe(EventTypeCustom, subscriptionID)
	assert.True(t, removed)

	removedAgain := dispatcher.Unsubscribe(EventTypeCustom, subscriptionID)
	assert.False(t, removedAgain)
}

func TestDispatcher_Publish(t *testing.T) {
	t.Run("async publish", func(t *testing.T) {
		config := DefaultDispatcherConfig
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()

		var receivedEvents []*Event
		var mu sync.Mutex

		handler := &testHandler{
			name:       "test-handler",
			eventTypes: []EventType{EventTypeCustom},
			handler: func(ctx context.Context, event *Event) error {
				mu.Lock()
				defer mu.Unlock()
				receivedEvents = append(receivedEvents, event)
				return nil
			},
		}

		dispatcher.Subscribe(EventTypeCustom, handler)

		event := &Event{
			Type:      EventTypeCustom,
			Source:    "test",
			Timestamp: time.Now().UTC(),
		}

		ctx := context.Background()
		err = dispatcher.Publish(ctx, event)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		assert.Len(t, receivedEvents, 1)
		assert.Equal(t, EventTypeCustom, receivedEvents[0].Type)
	})

	t.Run("publish with empty ID", func(t *testing.T) {
		config := DefaultDispatcherConfig
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()

		var receivedID string
		var mu sync.Mutex
		handler := &testHandler{
			name:       "test-handler",
			eventTypes: []EventType{EventTypeCustom},
			handler: func(ctx context.Context, event *Event) error {
				mu.Lock()
				defer mu.Unlock()
				receivedID = event.ID
				return nil
			},
		}

		dispatcher.Subscribe(EventTypeCustom, handler)

		event := &Event{
			Type:   EventTypeCustom,
			Source: "test",
		}

		ctx := context.Background()
		err = dispatcher.Publish(ctx, event)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)

		mu.Lock()
		defer mu.Unlock()
		assert.NotEmpty(t, receivedID)
	})

	t.Run("publish with context cancellation", func(t *testing.T) {
		config := DispatcherConfig{
			BufferSize:   1,
			WorkerCount:  1,
			MaxQueueSize: 10,
		}
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()

		event1 := &Event{
			Type:   EventTypeCustom,
			Source: "test1",
		}

		ctx1 := context.Background()
		err = dispatcher.Publish(ctx1, event1)
		require.NoError(t, err)

		time.Sleep(50 * time.Millisecond)

		event2 := &Event{
			Type:   EventTypeCustom,
			Source: "test2",
		}

		ctx2, cancel := context.WithCancel(context.Background())
		cancel()

		err = dispatcher.Publish(ctx2, event2)
		if err != nil {
			assert.Equal(t, context.Canceled, err)
		}
	})
}

func TestDispatcher_PublishSync(t *testing.T) {
	t.Run("sync publish", func(t *testing.T) {
		config := DefaultDispatcherConfig
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()

		var callCount int
		handler := &testHandler{
			name:       "test-handler",
			eventTypes: []EventType{EventTypeCustom},
			handler: func(ctx context.Context, event *Event) error {
				callCount++
				return nil
			},
		}

		dispatcher.Subscribe(EventTypeCustom, handler)

		event := &Event{
			Type:   EventTypeCustom,
			Source: "test",
		}

		ctx := context.Background()
		results := dispatcher.PublishSync(ctx, event)

		assert.Len(t, results, 1)
		assert.True(t, results[0].IsSuccess())
		assert.Equal(t, "test-handler", results[0].HandlerName)
		assert.Equal(t, 1, callCount)
	})

	t.Run("sync publish with multiple handlers", func(t *testing.T) {
		config := DefaultDispatcherConfig
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()

		var callCount1, callCount2 int

		handler1 := &testHandler{
			name:       "handler-1",
			eventTypes: []EventType{EventTypeCustom},
			handler: func(ctx context.Context, event *Event) error {
				callCount1++
				return nil
			},
		}

		handler2 := &testHandler{
			name:       "handler-2",
			eventTypes: []EventType{EventTypeCustom},
			handler: func(ctx context.Context, event *Event) error {
				callCount2++
				return nil
			},
		}

		dispatcher.Subscribe(EventTypeCustom, handler1)
		dispatcher.Subscribe(EventTypeCustom, handler2)

		event := &Event{
			Type:   EventTypeCustom,
			Source: "test",
		}

		ctx := context.Background()
		results := dispatcher.PublishSync(ctx, event)

		assert.Len(t, results, 2)
		assert.Equal(t, 1, callCount1)
		assert.Equal(t, 1, callCount2)
	})
}

func TestDispatcher_Priority(t *testing.T) {
	config := DefaultDispatcherConfig
	dispatcher, err := NewDispatcher(config)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dispatcher.Stop())
	}()

	executionOrder := []string{}
	var mu sync.Mutex

	handler1 := &testHandler{
		name:       "low-priority",
		eventTypes: []EventType{EventTypeCustom},
		handler: func(ctx context.Context, event *Event) error {
			mu.Lock()
			defer mu.Unlock()
			executionOrder = append(executionOrder, "low")
			return nil
		},
	}

	handler2 := &testHandler{
		name:       "high-priority",
		eventTypes: []EventType{EventTypeCustom},
		handler: func(ctx context.Context, event *Event) error {
			mu.Lock()
			defer mu.Unlock()
			executionOrder = append(executionOrder, "high")
			return nil
		},
	}

	dispatcher.Subscribe(EventTypeCustom, handler1, WithPriority(1))
	dispatcher.Subscribe(EventTypeCustom, handler2, WithPriority(10))

	event := &Event{
		Type:   EventTypeCustom,
		Source: "test",
	}

	ctx := context.Background()
	dispatcher.PublishSync(ctx, event)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, executionOrder, 2)
	assert.Equal(t, "high", executionOrder[0])
	assert.Equal(t, "low", executionOrder[1])
}

func TestDispatcher_Filter(t *testing.T) {
	config := DefaultDispatcherConfig
	dispatcher, err := NewDispatcher(config)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dispatcher.Stop())
	}()

	var filteredCallCount, unfilteredCallCount int

	filteredHandler := &testHandler{
		name:       "filtered-handler",
		eventTypes: []EventType{EventTypeCustom},
		handler: func(ctx context.Context, event *Event) error {
			filteredCallCount++
			return nil
		},
	}

	unfilteredHandler := &testHandler{
		name:       "unfiltered-handler",
		eventTypes: []EventType{EventTypeCustom},
		handler: func(ctx context.Context, event *Event) error {
			unfilteredCallCount++
			return nil
		},
	}

	dispatcher.Subscribe(EventTypeCustom, filteredHandler, WithFilter(func(event *Event) bool {
		return event.Source == "allowed"
	}))

	dispatcher.Subscribe(EventTypeCustom, unfilteredHandler)

	event1 := &Event{
		Type:   EventTypeCustom,
		Source: "allowed",
	}

	event2 := &Event{
		Type:   EventTypeCustom,
		Source: "blocked",
	}

	ctx := context.Background()
	dispatcher.PublishSync(ctx, event1)
	dispatcher.PublishSync(ctx, event2)

	assert.Equal(t, 1, filteredCallCount, "filtered handler should only be called for allowed source")
	assert.Equal(t, 2, unfilteredCallCount, "unfiltered handler should be called for all events")
}

func TestDispatcher_Once(t *testing.T) {
	config := DefaultDispatcherConfig
	dispatcher, err := NewDispatcher(config)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dispatcher.Stop())
	}()

	var callCount int

	handler := &testHandler{
		name:       "once-handler",
		eventTypes: []EventType{EventTypeCustom},
		handler: func(ctx context.Context, event *Event) error {
			callCount++
			return nil
		},
	}

	dispatcher.Subscribe(EventTypeCustom, handler, Once())

	event1 := &Event{
		Type:   EventTypeCustom,
		Source: "test",
	}

	event2 := &Event{
		Type:   EventTypeCustom,
		Source: "test",
	}

	ctx := context.Background()
	dispatcher.PublishSync(ctx, event1)
	dispatcher.PublishSync(ctx, event2)

	assert.Equal(t, 1, callCount, "handler with Once() should only be called once")
}

func TestDispatcher_Retry(t *testing.T) {
	t.Run("successful execution no retry", func(t *testing.T) {
		config := DispatcherConfig{
			BufferSize:    100,
			WorkerCount:   1,
			MaxQueueSize:  10000,
			RetryAttempts: 3,
			RetryDelay:    time.Millisecond * 10,
		}
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()

		var callCount int
		handler := &testHandler{
			name:       "test-handler",
			eventTypes: []EventType{EventTypeCustom},
			handler: func(ctx context.Context, event *Event) error {
				callCount++
				return nil
			},
		}

		dispatcher.Subscribe(EventTypeCustom, handler)

		event := &Event{
			Type:   EventTypeCustom,
			Source: "test",
		}

		ctx := context.Background()
		results := dispatcher.PublishSync(ctx, event)

		assert.Len(t, results, 1)
		assert.True(t, results[0].IsSuccess())
		assert.Equal(t, 1, callCount)
	})

	t.Run("retry on failure then success", func(t *testing.T) {
		config := DispatcherConfig{
			BufferSize:    100,
			WorkerCount:   1,
			MaxQueueSize:  10000,
			RetryAttempts: 3,
			RetryDelay:    time.Millisecond * 10,
		}
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()

		var callCount int
		handler := &testHandler{
			name:       "test-handler",
			eventTypes: []EventType{EventTypeCustom},
			handler: func(ctx context.Context, event *Event) error {
				callCount++
				if callCount < 3 {
					return assert.AnError
				}
				return nil
			},
		}

		dispatcher.Subscribe(EventTypeCustom, handler)

		event := &Event{
			Type:   EventTypeCustom,
			Source: "test",
		}

		ctx := context.Background()
		results := dispatcher.PublishSync(ctx, event)

		assert.Len(t, results, 1)
		assert.True(t, results[0].IsSuccess())
		assert.Equal(t, 3, callCount)
	})

	t.Run("retry exhausted goes to dead letter", func(t *testing.T) {
		config := DispatcherConfig{
			BufferSize:          100,
			WorkerCount:         1,
			MaxQueueSize:        10000,
			RetryAttempts:       2,
			RetryDelay:          time.Millisecond * 10,
			EnableDeadLetter:    true,
			DeadLetterQueueSize: 10,
		}
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()

		var callCount int
		handler := &testHandler{
			name:       "test-handler",
			eventTypes: []EventType{EventTypeCustom},
			handler: func(ctx context.Context, event *Event) error {
				callCount++
				return assert.AnError
			},
		}

		dispatcher.Subscribe(EventTypeCustom, handler)

		event := &Event{
			Type:   EventTypeCustom,
			Source: "test",
		}

		ctx := context.Background()
		results := dispatcher.PublishSync(ctx, event)

		assert.Len(t, results, 1)
		assert.Error(t, results[0].Error)
		assert.Equal(t, 3, callCount)
	})
}

func TestDispatcher_DeadLetterQueue(t *testing.T) {
	t.Run("dead letter enabled", func(t *testing.T) {
		config := DispatcherConfig{
			BufferSize:          100,
			WorkerCount:         1,
			MaxQueueSize:        10000,
			RetryAttempts:       1,
			RetryDelay:          time.Millisecond * 10,
			EnableDeadLetter:    true,
			DeadLetterQueueSize: 10,
		}
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()

		handler := &testHandler{
			name:       "failing-handler",
			eventTypes: []EventType{EventTypeCustom},
			handler: func(ctx context.Context, event *Event) error {
				return assert.AnError
			},
		}

		dispatcher.Subscribe(EventTypeCustom, handler)

		event := &Event{
			Type:   EventTypeCustom,
			Source: "test",
		}

		ctx := context.Background()
		dispatcher.PublishSync(ctx, event)

		time.Sleep(100 * time.Millisecond)
	})

	t.Run("dead letter disabled", func(t *testing.T) {
		config := DispatcherConfig{
			BufferSize:       100,
			WorkerCount:      1,
			MaxQueueSize:     10000,
			RetryAttempts:    1,
			RetryDelay:       time.Millisecond * 10,
			EnableDeadLetter: false,
		}
		dispatcher, err := NewDispatcher(config)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, dispatcher.Stop())
		}()

		handler := &testHandler{
			name:       "failing-handler",
			eventTypes: []EventType{EventTypeCustom},
			handler: func(ctx context.Context, event *Event) error {
				return assert.AnError
			},
		}

		dispatcher.Subscribe(EventTypeCustom, handler)

		event := &Event{
			Type:   EventTypeCustom,
			Source: "test",
		}

		ctx := context.Background()
		results := dispatcher.PublishSync(ctx, event)

		assert.Len(t, results, 1)
		assert.Error(t, results[0].Error)
	})
}

func TestDispatcher_Stop(t *testing.T) {
	config := DefaultDispatcherConfig
	dispatcher, err := NewDispatcher(config)
	require.NoError(t, err)

	err = dispatcher.Stop()
	require.NoError(t, err)
}

func TestDispatcher_Integration_WithRegistry(t *testing.T) {
	config := DefaultDispatcherConfig
	dispatcher, err := NewDispatcher(config)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, dispatcher.Stop())
	}()

	logger := &testLogger{}
	registry := NewHandlerRegistry(logger)

	var handledEvents []*Event
	var mu sync.Mutex

	testHandlerImpl := &testHandler{
		name:       "registry-handler",
		eventTypes: []EventType{EventTypeInventoryCreated},
		handler: func(ctx context.Context, event *Event) error {
			mu.Lock()
			defer mu.Unlock()
			handledEvents = append(handledEvents, event)
			return nil
		},
	}

	handlerInfo := &HandlerInfo{
		Name:       "registry-handler",
		EventTypes: []EventType{EventTypeInventoryCreated},
		Handler:    testHandlerImpl,
	}

	err = registry.Register(handlerInfo)
	require.NoError(t, err)

	dispatcher.Subscribe(EventTypeInventoryCreated, testHandlerImpl)

	data := map[string]interface{}{
		"inventory_id": "inv-123",
		"name":         "Test Inventory",
	}

	event, err := NewEvent(EventTypeInventoryCreated, "test", data)
	require.NoError(t, err)

	ctx := context.Background()
	err = dispatcher.Publish(ctx, event)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	assert.Len(t, handledEvents, 1)
	assert.Equal(t, EventTypeInventoryCreated, handledEvents[0].Type)

	var eventData map[string]interface{}
	err = json.Unmarshal(handledEvents[0].Data, &eventData)
	require.NoError(t, err)
	assert.Equal(t, "inv-123", eventData["inventory_id"])
}

func TestDispatcherConfig_Validate(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := DefaultDispatcherConfig
		err := config.Validate()
		assert.NoError(t, err)
	})

	t.Run("all invalid fields", func(t *testing.T) {
		config := DispatcherConfig{
			BufferSize:          0,
			WorkerCount:         0,
			MaxQueueSize:        0,
			EnableDeadLetter:    true,
			DeadLetterQueueSize: 0,
		}
		err := config.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "buffer_size must be >= 1")
	})
}
