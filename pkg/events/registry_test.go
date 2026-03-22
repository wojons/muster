package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registryMockLogger is a test double for the Logger interface.
type registryMockLogger struct {
	infos  []string
	errors []string
	debugs []string
}

func (m *registryMockLogger) Info(msg string, fields ...interface{}) {
	m.infos = append(m.infos, msg)
}

func (m *registryMockLogger) Error(msg string, fields ...interface{}) {
	m.errors = append(m.errors, msg)
}

func (m *registryMockLogger) Debug(msg string, fields ...interface{}) {
	m.debugs = append(m.debugs, msg)
}

// registryMockHandler is a test double for the Handler interface.
type registryMockHandler struct {
	name         string
	supportsFunc func(EventType) bool
	handleFunc   func(context.Context, *Event) error
}

func (m *registryMockHandler) Handle(ctx context.Context, event *Event) error {
	if m.handleFunc != nil {
		return m.handleFunc(ctx, event)
	}
	return nil
}

func (m *registryMockHandler) Name() string {
	return m.name
}

func (m *registryMockHandler) Supports(eventType EventType) bool {
	if m.supportsFunc != nil {
		return m.supportsFunc(eventType)
	}
	return true
}

func TestNewHandlerRegistry(t *testing.T) {
	t.Run("creates registry with logger", func(t *testing.T) {
		logger := &registryMockLogger{}
		registry := NewHandlerRegistry(logger)

		assert.NotNil(t, registry)
		assert.NotNil(t, registry.handlers)
		assert.NotNil(t, registry.byType)
		assert.NotNil(t, registry.middlewares)
		assert.Equal(t, logger, registry.logger)
	})

	t.Run("creates registry without logger", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		assert.NotNil(t, registry)
		assert.Nil(t, registry.logger)
	})
}

func TestHandlerRegistry_Register(t *testing.T) {
	t.Run("registers handler successfully", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		handler := &registryMockHandler{name: "test-handler"}

		info := &HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
		}

		err := registry.Register(info)
		require.NoError(t, err)
		assert.True(t, registry.IsRegistered("test-handler"))
	})

	t.Run("returns error for nil info", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		err := registry.Register(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "handler info is required")
	})

	t.Run("returns error for invalid info", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		info := &HandlerInfo{
			Name:       "",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    &mockHandler{},
		}

		err := registry.Register(info)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid handler info")
	})

	t.Run("returns error for duplicate name", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		handler := &mockHandler{name: "test-handler"}

		info := &HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
		}

		err := registry.Register(info)
		require.NoError(t, err)

		// Try to register again with same name
		err = registry.Register(info)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("logs registration when logger provided", func(t *testing.T) {
		logger := &registryMockLogger{}
		registry := NewHandlerRegistry(logger)
		handler := &mockHandler{name: "test-handler"}

		info := &HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
		}

		err := registry.Register(info)
		require.NoError(t, err)
		assert.Len(t, logger.infos, 1)
		assert.Contains(t, logger.infos[0], "handler registered")
	})

	t.Run("registers handler with multiple event types", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		handler := &mockHandler{name: "test-handler"}

		info := &HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest, EventTypeResponse},
			Handler:    handler,
		}

		err := registry.Register(info)
		require.NoError(t, err)

		handlers := registry.GetHandlers(EventTypeRequest)
		assert.Len(t, handlers, 1)

		handlers = registry.GetHandlers(EventTypeResponse)
		assert.Len(t, handlers, 1)
	})
}

func TestHandlerRegistry_Unregister(t *testing.T) {
	t.Run("unregisters handler successfully", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		handler := &mockHandler{name: "test-handler"}

		info := &HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
		}

		err := registry.Register(info)
		require.NoError(t, err)
		assert.True(t, registry.IsRegistered("test-handler"))

		removed := registry.Unregister("test-handler")
		assert.True(t, removed)
		assert.False(t, registry.IsRegistered("test-handler"))
	})

	t.Run("returns false for non-existent handler", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		removed := registry.Unregister("non-existent")
		assert.False(t, removed)
	})

	t.Run("removes handler from event type index", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		handler := &mockHandler{name: "test-handler"}

		info := &HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
		}

		err := registry.Register(info)
		require.NoError(t, err)

		registry.Unregister("test-handler")

		handlers := registry.GetHandlers(EventTypeRequest)
		assert.Len(t, handlers, 0)
	})

	t.Run("logs unregistration when logger provided", func(t *testing.T) {
		logger := &registryMockLogger{}
		registry := NewHandlerRegistry(logger)
		handler := &mockHandler{name: "test-handler"}

		info := &HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
		}

		err := registry.Register(info)
		require.NoError(t, err)

		registry.Unregister("test-handler")
		assert.Len(t, logger.infos, 2) // One for register, one for unregister
	})
}

func TestHandlerRegistry_GetHandlers(t *testing.T) {
	t.Run("returns handlers for event type", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		handler1 := &mockHandler{name: "handler1"}
		handler2 := &mockHandler{name: "handler2"}

		err := registry.Register(&HandlerInfo{
			Name:       "handler1",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler1,
		})
		require.NoError(t, err)

		err = registry.Register(&HandlerInfo{
			Name:       "handler2",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler2,
		})
		require.NoError(t, err)

		handlers := registry.GetHandlers(EventTypeRequest)
		assert.Len(t, handlers, 2)
	})

	t.Run("returns empty slice for unknown event type", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		handlers := registry.GetHandlers(EventTypeCustom)
		assert.NotNil(t, handlers)
		assert.Len(t, handlers, 0)
	})

	t.Run("returns only handlers for specific event type", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		handler1 := &mockHandler{name: "handler1"}
		handler2 := &mockHandler{name: "handler2"}

		err := registry.Register(&HandlerInfo{
			Name:       "handler1",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler1,
		})
		require.NoError(t, err)

		err = registry.Register(&HandlerInfo{
			Name:       "handler2",
			EventTypes: []EventType{EventTypeResponse},
			Handler:    handler2,
		})
		require.NoError(t, err)

		handlers := registry.GetHandlers(EventTypeRequest)
		assert.Len(t, handlers, 1)
		assert.Equal(t, "handler1", handlers[0].Name)
	})
}

func TestHandlerRegistry_Execute(t *testing.T) {
	t.Run("executes handlers for event type", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		executed := false

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				executed = true
				return nil
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		results := registry.Execute(context.Background(), event)
		assert.Len(t, results, 1)
		assert.True(t, executed)
		assert.True(t, results[0].IsSuccess())
	})

	t.Run("returns error result when handler fails", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		expectedErr := errors.New("handler error")

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				return expectedErr
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		results := registry.Execute(context.Background(), event)
		assert.Len(t, results, 1)
		assert.False(t, results[0].IsSuccess())
		assert.Equal(t, expectedErr, results[0].Error)
	})

	t.Run("executes multiple handlers", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		executionCount := 0

		handler1 := &mockHandler{
			name: "handler1",
			handleFunc: func(ctx context.Context, event *Event) error {
				executionCount++
				return nil
			},
		}

		handler2 := &mockHandler{
			name: "handler2",
			handleFunc: func(ctx context.Context, event *Event) error {
				executionCount++
				return nil
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "handler1",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler1,
		})
		require.NoError(t, err)

		err = registry.Register(&HandlerInfo{
			Name:       "handler2",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler2,
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		results := registry.Execute(context.Background(), event)
		assert.Len(t, results, 2)
		assert.Equal(t, 2, executionCount)
	})

	t.Run("logs execution failure", func(t *testing.T) {
		logger := &registryMockLogger{}
		registry := NewHandlerRegistry(logger)
		expectedErr := errors.New("handler error")

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				return expectedErr
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		registry.Execute(context.Background(), event)
		assert.Len(t, logger.errors, 1)
		assert.Contains(t, logger.errors[0], "handler execution failed")
	})

	t.Run("calls OnError callback when handler fails", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		expectedErr := errors.New("handler error")
		onErrorCalled := false

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				return expectedErr
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
			OnError: func(ctx context.Context, event *Event, err error) error {
				onErrorCalled = true
				assert.Equal(t, expectedErr, err)
				return nil // Suppress the error
			},
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		results := registry.Execute(context.Background(), event)
		assert.True(t, onErrorCalled)
		assert.True(t, results[0].IsSuccess()) // Error was suppressed
	})
}

func TestHandlerRegistry_Timeout(t *testing.T) {
	t.Run("applies timeout to handler", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				select {
				case <-time.After(100 * time.Millisecond):
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
			Timeout:    10 * time.Millisecond,
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		results := registry.Execute(context.Background(), event)
		assert.Len(t, results, 1)
		assert.False(t, results[0].IsSuccess())
		assert.Equal(t, context.DeadlineExceeded, results[0].Error)
	})

	t.Run("no timeout when zero", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		executed := false

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				executed = true
				return nil
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
			Timeout:    0,
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		results := registry.Execute(context.Background(), event)
		assert.True(t, executed)
		assert.True(t, results[0].IsSuccess())
	})
}

func TestHandlerRegistry_Retry(t *testing.T) {
	t.Run("retries on failure", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		attemptCount := 0

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				attemptCount++
				if attemptCount < 3 {
					return errors.New("temporary error")
				}
				return nil
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
			MaxRetries: 3,
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		results := registry.Execute(context.Background(), event)
		assert.True(t, results[0].IsSuccess())
		assert.Equal(t, 3, attemptCount)
	})

	t.Run("returns error after max retries exceeded", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		expectedErr := errors.New("persistent error")
		attemptCount := 0

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				attemptCount++
				return expectedErr
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
			MaxRetries: 2,
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		results := registry.Execute(context.Background(), event)
		assert.False(t, results[0].IsSuccess())
		assert.Equal(t, expectedErr, results[0].Error)
		assert.Equal(t, 3, attemptCount) // Initial + 2 retries
	})

	t.Run("no retry when zero", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		attemptCount := 0

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				attemptCount++
				return errors.New("error")
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
			MaxRetries: 0,
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		results := registry.Execute(context.Background(), event)
		assert.False(t, results[0].IsSuccess())
		assert.Equal(t, 1, attemptCount) // Only initial attempt
	})
}

func TestHandlerRegistry_Middleware(t *testing.T) {
	t.Run("applies middleware to handler", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		middlewareCalled := false

		middleware := func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, event *Event) error {
				middlewareCalled = true
				return next(ctx, event)
			}
		}

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				return nil
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
			Middleware: []HandlerMiddleware{middleware},
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		registry.Execute(context.Background(), event)
		assert.True(t, middlewareCalled)
	})

	t.Run("applies registry middleware to all handlers", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		middlewareCallCount := 0

		middleware := func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, event *Event) error {
				middlewareCallCount++
				return next(ctx, event)
			}
		}

		registry.Use(middleware)

		handler1 := &mockHandler{
			name: "handler1",
			handleFunc: func(ctx context.Context, event *Event) error {
				return nil
			},
		}

		handler2 := &mockHandler{
			name: "handler2",
			handleFunc: func(ctx context.Context, event *Event) error {
				return nil
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "handler1",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler1,
		})
		require.NoError(t, err)

		err = registry.Register(&HandlerInfo{
			Name:       "handler2",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler2,
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		registry.Execute(context.Background(), event)
		assert.Equal(t, 2, middlewareCallCount)
	})

	t.Run("middleware chain executes in correct order", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		order := []string{}

		middleware1 := func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, event *Event) error {
				order = append(order, "before1")
				err := next(ctx, event)
				order = append(order, "after1")
				return err
			}
		}

		middleware2 := func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, event *Event) error {
				order = append(order, "before2")
				err := next(ctx, event)
				order = append(order, "after2")
				return err
			}
		}

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				order = append(order, "handler")
				return nil
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
			Middleware: []HandlerMiddleware{middleware1, middleware2},
		})
		require.NoError(t, err)

		event, err := NewEvent(EventTypeRequest, "test-source", nil)
		require.NoError(t, err)

		registry.Execute(context.Background(), event)

		// Expected order: before1 -> before2 -> handler -> after2 -> after1
		expected := []string{"before1", "before2", "handler", "after2", "after1"}
		assert.Equal(t, expected, order)
	})
}

func TestHandlerRegistry_List(t *testing.T) {
	t.Run("returns empty list when no handlers", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		names := registry.List()
		assert.NotNil(t, names)
		assert.Len(t, names, 0)
	})

	t.Run("returns all registered handler names", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		err := registry.Register(&HandlerInfo{
			Name:       "handler1",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    &mockHandler{name: "handler1"},
		})
		require.NoError(t, err)

		err = registry.Register(&HandlerInfo{
			Name:       "handler2",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    &mockHandler{name: "handler2"},
		})
		require.NoError(t, err)

		names := registry.List()
		assert.Len(t, names, 2)
		assert.Contains(t, names, "handler1")
		assert.Contains(t, names, "handler2")
	})
}

func TestHandlerRegistry_Count(t *testing.T) {
	t.Run("returns zero when empty", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		assert.Equal(t, 0, registry.Count())
	})

	t.Run("returns correct count", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		err := registry.Register(&HandlerInfo{
			Name:       "handler1",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    &mockHandler{name: "handler1"},
		})
		require.NoError(t, err)

		assert.Equal(t, 1, registry.Count())

		err = registry.Register(&HandlerInfo{
			Name:       "handler2",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    &mockHandler{name: "handler2"},
		})
		require.NoError(t, err)

		assert.Equal(t, 2, registry.Count())

		registry.Unregister("handler1")
		assert.Equal(t, 1, registry.Count())
	})
}

func TestHandlerRegistry_IsRegistered(t *testing.T) {
	t.Run("returns false for unregistered handler", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		assert.False(t, registry.IsRegistered("non-existent"))
	})

	t.Run("returns true for registered handler", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    &mockHandler{name: "test-handler"},
		})
		require.NoError(t, err)

		assert.True(t, registry.IsRegistered("test-handler"))
	})
}

func TestHandlerRegistry_Concurrency(t *testing.T) {
	t.Run("handles concurrent registrations", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)

		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func(idx int) {
				handler := &registryMockHandler{name: fmt.Sprintf("handler%d", idx)}
				_ = registry.Register(&HandlerInfo{
					Name:       fmt.Sprintf("handler%d", idx),
					EventTypes: []EventType{EventTypeRequest},
					Handler:    handler,
				})
				done <- true
			}(i)
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		assert.Equal(t, 10, registry.Count())
	})

	t.Run("handles concurrent executions", func(t *testing.T) {
		registry := NewHandlerRegistry(nil)
		executionCount := 0
		var mu sync.Mutex

		handler := &mockHandler{
			name: "test-handler",
			handleFunc: func(ctx context.Context, event *Event) error {
				mu.Lock()
				executionCount++
				mu.Unlock()
				return nil
			},
		}

		err := registry.Register(&HandlerInfo{
			Name:       "test-handler",
			EventTypes: []EventType{EventTypeRequest},
			Handler:    handler,
		})
		require.NoError(t, err)

		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				event, _ := NewEvent(EventTypeRequest, "test-source", nil)
				registry.Execute(context.Background(), event)
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}

		assert.Equal(t, 10, executionCount)
	})
}
