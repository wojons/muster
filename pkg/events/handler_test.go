package events

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockHandler is a test implementation of the Handler interface.
type mockHandler struct {
	name         string
	supportsFunc func(EventType) bool
	handleFunc   func(context.Context, *Event) error
}

func (m *mockHandler) Handle(ctx context.Context, event *Event) error {
	if m.handleFunc != nil {
		return m.handleFunc(ctx, event)
	}
	return nil
}

func (m *mockHandler) Name() string {
	return m.name
}

func (m *mockHandler) Supports(eventType EventType) bool {
	if m.supportsFunc != nil {
		return m.supportsFunc(eventType)
	}
	return true
}

// TestHandlerInterface verifies that Handler interface is properly defined.
func TestHandlerInterface(t *testing.T) {
	ctx := context.Background()
	event := &Event{
		ID:        "test-123",
		Type:      EventTypeRequest,
		Timestamp: time.Now(),
	}

	// Test that mockHandler implements Handler interface.
	var _ Handler = (*mockHandler)(nil)

	handler := &mockHandler{
		name: "test-handler",
		handleFunc: func(ctx context.Context, e *Event) error {
			if e.ID != "test-123" {
				t.Errorf("expected event ID 'test-123', got '%s'", e.ID)
			}
			return nil
		},
		supportsFunc: func(et EventType) bool {
			return et == EventTypeRequest
		},
	}

	// Test Handle method.
	err := handler.Handle(ctx, event)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// Test Name method.
	if handler.Name() != "test-handler" {
		t.Errorf("expected name 'test-handler', got '%s'", handler.Name())
	}

	// Test Supports method.
	if !handler.Supports(EventTypeRequest) {
		t.Error("expected handler to support EventTypeRequest")
	}
	if handler.Supports(EventTypeResponse) {
		t.Error("expected handler to not support EventTypeResponse")
	}
}

// TestHandlerFunc tests the HandlerFunc type.
func TestHandlerFunc(t *testing.T) {
	ctx := context.Background()
	event := &Event{ID: "test-456"}

	called := false
	var handler HandlerFunc = func(ctx context.Context, e *Event) error {
		called = true
		if e.ID != "test-456" {
			t.Errorf("expected event ID 'test-456', got '%s'", e.ID)
		}
		return nil
	}

	err := handler(ctx, event)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !called {
		t.Error("handler function was not called")
	}
}

// TestHandlerMiddleware tests the HandlerMiddleware type.
func TestHandlerMiddleware(t *testing.T) {
	ctx := context.Background()
	event := &Event{ID: "test-789"}

	var callOrder []string

	// Create a base handler.
	baseHandler := HandlerFunc(func(ctx context.Context, e *Event) error {
		callOrder = append(callOrder, "base")
		return nil
	})

	// Create middleware that logs before and after.
	loggingMiddleware := func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, e *Event) error {
			callOrder = append(callOrder, "before")
			err := next(ctx, e)
			callOrder = append(callOrder, "after")
			return err
		}
	}

	// Wrap the handler with middleware.
	wrapped := loggingMiddleware(baseHandler)

	err := wrapped(ctx, event)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	expectedOrder := []string{"before", "base", "after"}
	if len(callOrder) != len(expectedOrder) {
		t.Errorf("expected call order %v, got %v", expectedOrder, callOrder)
	}
	for i, expected := range expectedOrder {
		if callOrder[i] != expected {
			t.Errorf("expected call order[%d] = '%s', got '%s'", i, expected, callOrder[i])
		}
	}
}

// TestHandlerInfo tests the HandlerInfo struct.
func TestHandlerInfo(t *testing.T) {
	handler := &mockHandler{name: "my-handler"}

	info := &HandlerInfo{
		Name:       "my-handler",
		EventTypes: []EventType{EventTypeRequest, EventTypeResponse},
		Handler:    handler,
		Timeout:    5 * time.Second,
		RetryConfig: &RetryConfig{
			MaxRetries: 3,
		},
		OnError: func(ctx context.Context, event *Event, err error) error {
			return nil // Suppress error.
		},
	}

	// Verify fields are set correctly.
	if info.Name != "my-handler" {
		t.Errorf("expected name 'my-handler', got '%s'", info.Name)
	}
	if len(info.EventTypes) != 2 {
		t.Errorf("expected 2 event types, got %d", len(info.EventTypes))
	}
	if info.Handler != handler {
		t.Error("handler reference mismatch")
	}
	if info.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", info.Timeout)
	}
	if info.RetryConfig == nil || info.RetryConfig.MaxRetries != 3 {
		t.Errorf("expected max retries 3")
	}
	if info.OnError == nil {
		t.Error("expected OnError to be set")
	}
}

// TestHandlerInfoValidate tests the HandlerInfo.Validate method.
func TestHandlerInfoValidate(t *testing.T) {
	tests := []struct {
		name    string
		info    *HandlerInfo
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid handler info",
			info: &HandlerInfo{
				Name:       "test-handler",
				EventTypes: []EventType{EventTypeRequest},
				Handler:    &mockHandler{name: "test-handler"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			info: &HandlerInfo{
				Name:       "",
				EventTypes: []EventType{EventTypeRequest},
				Handler:    &mockHandler{},
			},
			wantErr: true,
			errMsg:  "Name: handler name is required",
		},
		{
			name: "missing handler",
			info: &HandlerInfo{
				Name:       "test-handler",
				EventTypes: []EventType{EventTypeRequest},
				Handler:    nil,
			},
			wantErr: true,
			errMsg:  "Handler: handler is required",
		},
		{
			name: "no event types",
			info: &HandlerInfo{
				Name:       "test-handler",
				EventTypes: []EventType{},
				Handler:    &mockHandler{},
			},
			wantErr: true,
			errMsg:  "EventTypes: at least one event type is required",
		},
		{
			name: "empty event type in list",
			info: &HandlerInfo{
				Name:       "test-handler",
				EventTypes: []EventType{EventTypeRequest, ""},
				Handler:    &mockHandler{},
			},
			wantErr: true,
			errMsg:  "EventTypes: event type cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.info.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
					return
				}
				if err.Error() != tt.errMsg {
					t.Errorf("expected error '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestHandlerResult tests the HandlerResult struct and methods.
func TestHandlerResult(t *testing.T) {
	now := time.Now()

	// Test successful result.
	successResult := &HandlerResult{
		HandlerName: "handler-1",
		EventID:     "event-123",
		Duration:    100 * time.Millisecond,
		Error:       nil,
		ProcessedAt: now,
	}

	if !successResult.IsSuccess() {
		t.Error("expected IsSuccess() to return true for nil error")
	}
	if successResult.String() != "handler-1: success" {
		t.Errorf("expected string 'handler-1: success', got '%s'", successResult.String())
	}

	// Test error result.
	errResult := &HandlerResult{
		HandlerName: "handler-2",
		EventID:     "event-456",
		Duration:    200 * time.Millisecond,
		Error:       errors.New("processing failed"),
		ProcessedAt: now,
	}

	if errResult.IsSuccess() {
		t.Error("expected IsSuccess() to return false for non-nil error")
	}
	if errResult.String() != "handler-2: processing failed" {
		t.Errorf("expected string 'handler-2: processing failed', got '%s'", errResult.String())
	}
}

// TestErrorHandler tests the ErrorHandler type.
func TestErrorHandler(t *testing.T) {
	ctx := context.Background()
	event := &Event{ID: "test-event"}
	originalErr := errors.New("original error")

	// Test error handler that suppresses the error.
	suppressHandler := ErrorHandler(func(ctx context.Context, e *Event, err error) error {
		return nil // Suppress.
	})

	result := suppressHandler(ctx, event, originalErr)
	if result != nil {
		t.Errorf("expected nil after suppression, got: %v", result)
	}

	// Test error handler that modifies the error.
	modifyHandler := ErrorHandler(func(ctx context.Context, e *Event, err error) error {
		return errors.New("modified: " + err.Error())
	})

	result = modifyHandler(ctx, event, originalErr)
	if result == nil {
		t.Error("expected modified error, got nil")
	}
	if result.Error() != "modified: original error" {
		t.Errorf("expected 'modified: original error', got '%s'", result.Error())
	}

	// Test error handler that propagates the error.
	propagateHandler := ErrorHandler(func(ctx context.Context, e *Event, err error) error {
		return err // Propagate.
	})

	result = propagateHandler(ctx, event, originalErr)
	if result != originalErr {
		t.Error("expected original error to be propagated")
	}
}

// TestMultipleMiddleware tests chaining multiple middleware.
func TestMultipleMiddleware(t *testing.T) {
	ctx := context.Background()
	event := &Event{ID: "test-multi"}

	var callOrder []string

	// Create base handler.
	base := HandlerFunc(func(ctx context.Context, e *Event) error {
		callOrder = append(callOrder, "base")
		return nil
	})

	// Create middleware 1.
	mw1 := func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, e *Event) error {
			callOrder = append(callOrder, "mw1-before")
			err := next(ctx, e)
			callOrder = append(callOrder, "mw1-after")
			return err
		}
	}

	// Create middleware 2.
	mw2 := func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, e *Event) error {
			callOrder = append(callOrder, "mw2-before")
			err := next(ctx, e)
			callOrder = append(callOrder, "mw2-after")
			return err
		}
	}

	// Chain middleware: mw2(mw1(base))
	wrapped := mw2(mw1(base))

	err := wrapped(ctx, event)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	expectedOrder := []string{"mw2-before", "mw1-before", "base", "mw1-after", "mw2-after"}
	if len(callOrder) != len(expectedOrder) {
		t.Fatalf("expected %d calls, got %d: %v", len(expectedOrder), len(callOrder), callOrder)
	}
	for i, expected := range expectedOrder {
		if callOrder[i] != expected {
			t.Errorf("callOrder[%d]: expected '%s', got '%s'", i, expected, callOrder[i])
		}
	}
}

// TestValidationError tests the ValidationError type.
func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Field:   "TestField",
		Message: "test message",
	}

	expected := "TestField: test message"
	if err.Error() != expected {
		t.Errorf("expected error '%s', got '%s'", expected, err.Error())
	}
}

// TestRetryConfig tests the RetryConfig struct.
func TestRetryConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		config := DefaultRetryConfig()

		if config.MaxRetries != 3 {
			t.Errorf("expected MaxRetries=3, got %d", config.MaxRetries)
		}
		if config.InitialDelay != 100*time.Millisecond {
			t.Errorf("expected InitialDelay=100ms, got %v", config.InitialDelay)
		}
		if config.MaxDelay != 30*time.Second {
			t.Errorf("expected MaxDelay=30s, got %v", config.MaxDelay)
		}
		if !config.Exponential {
			t.Error("expected Exponential=true")
		}
		if config.Multiplier != 2.0 {
			t.Errorf("expected Multiplier=2.0, got %f", config.Multiplier)
		}
		if config.Randomization != 0.5 {
			t.Errorf("expected Randomization=0.5, got %f", config.Randomization)
		}
	})

	t.Run("custom config", func(t *testing.T) {
		config := RetryConfig{
			MaxRetries:    5,
			InitialDelay:  50 * time.Millisecond,
			MaxDelay:      10 * time.Second,
			Exponential:   false,
			Multiplier:    1.5,
			Randomization: 0.2,
		}

		if config.MaxRetries != 5 {
			t.Errorf("expected MaxRetries=5, got %d", config.MaxRetries)
		}
		if config.InitialDelay != 50*time.Millisecond {
			t.Errorf("expected InitialDelay=50ms, got %v", config.InitialDelay)
		}
		if config.MaxDelay != 10*time.Second {
			t.Errorf("expected MaxDelay=10s, got %v", config.MaxDelay)
		}
		if config.Exponential {
			t.Error("expected Exponential=false")
		}
		if config.Multiplier != 1.5 {
			t.Errorf("expected Multiplier=1.5, got %f", config.Multiplier)
		}
		if config.Randomization != 0.2 {
			t.Errorf("expected Randomization=0.2, got %f", config.Randomization)
		}
	})
}

// TestCircuitState tests the CircuitState enum and String method.
func TestCircuitState(t *testing.T) {
	t.Run("circuit closed string", func(t *testing.T) {
		if CircuitClosed.String() != "closed" {
			t.Errorf("expected 'closed', got '%s'", CircuitClosed.String())
		}
	})

	t.Run("circuit open string", func(t *testing.T) {
		if CircuitOpen.String() != "open" {
			t.Errorf("expected 'open', got '%s'", CircuitOpen.String())
		}
	})

	t.Run("circuit half-open string", func(t *testing.T) {
		if CircuitHalfOpen.String() != "half-open" {
			t.Errorf("expected 'half-open', got '%s'", CircuitHalfOpen.String())
		}
	})
}

// TestCircuitBreakerConfig tests the CircuitBreakerConfig struct.
func TestCircuitBreakerConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		config := DefaultCircuitBreakerConfig()

		if config.MaxFailures != 5 {
			t.Errorf("expected MaxFailures=5, got %d", config.MaxFailures)
		}
		if config.Timeout != 30*time.Second {
			t.Errorf("expected Timeout=30s, got %v", config.Timeout)
		}
		if config.HalfOpenMaxReq != 3 {
			t.Errorf("expected HalfOpenMaxReq=3, got %d", config.HalfOpenMaxReq)
		}
	})

	t.Run("custom config", func(t *testing.T) {
		config := CircuitBreakerConfig{
			MaxFailures:    10,
			Timeout:        1 * time.Minute,
			HalfOpenMaxReq: 5,
		}

		if config.MaxFailures != 10 {
			t.Errorf("expected MaxFailures=10, got %d", config.MaxFailures)
		}
		if config.Timeout != 1*time.Minute {
			t.Errorf("expected Timeout=1m, got %v", config.Timeout)
		}
		if config.HalfOpenMaxReq != 5 {
			t.Errorf("expected HalfOpenMaxReq=5, got %d", config.HalfOpenMaxReq)
		}
	})
}

// TestCircuitBreaker tests the CircuitBreaker implementation.
func TestCircuitBreaker(t *testing.T) {
	t.Run("new circuit breaker starts closed", func(t *testing.T) {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
		if cb.State() != CircuitClosed {
			t.Errorf("expected CircuitClosed, got %v", cb.State())
		}
	})

	t.Run("allows requests when closed", func(t *testing.T) {
		cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())
		if !cb.Allow() {
			t.Error("expected Allow() to return true when closed")
		}
	})

	t.Run("opens after max failures", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			MaxFailures:    3,
			Timeout:        100 * time.Millisecond,
			HalfOpenMaxReq: 1,
		})

		cb.RecordFailure()
		cb.RecordFailure()
		assertEqualState(t, cb, CircuitClosed, "should be closed after 2 failures")

		cb.RecordFailure()
		assertEqualState(t, cb, CircuitOpen, "should be open after 3 failures")
	})

	t.Run("transitions to half-open after timeout", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			MaxFailures:    2,
			Timeout:        50 * time.Millisecond,
			HalfOpenMaxReq: 1,
		})

		cb.RecordFailure()
		cb.RecordFailure()
		assertEqualState(t, cb, CircuitOpen, "should be open")

		time.Sleep(60 * time.Millisecond)

		if !cb.Allow() {
			t.Error("expected Allow() to return true after timeout")
		}
		assertEqualState(t, cb, CircuitHalfOpen, "should be half-open after timeout")
	})

	t.Run("closes after successful requests in half-open", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			MaxFailures:    2,
			Timeout:        10 * time.Millisecond,
			HalfOpenMaxReq: 2,
		})

		cb.RecordFailure()
		cb.RecordFailure()
		time.Sleep(20 * time.Millisecond)
		cb.Allow()
		assertEqualState(t, cb, CircuitHalfOpen, "should be half-open")

		cb.RecordSuccess()
		assertEqualState(t, cb, CircuitHalfOpen, "should still be half-open after 1 success")

		cb.RecordSuccess()
		assertEqualState(t, cb, CircuitClosed, "should be closed after 2 successes")
	})

	t.Run("opens immediately on failure in half-open", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			MaxFailures:    2,
			Timeout:        10 * time.Millisecond,
			HalfOpenMaxReq: 2,
		})

		cb.RecordFailure()
		cb.RecordFailure()
		time.Sleep(20 * time.Millisecond)
		cb.Allow()
		assertEqualState(t, cb, CircuitHalfOpen, "should be half-open")

		cb.RecordFailure()
		assertEqualState(t, cb, CircuitOpen, "should open immediately on failure in half-open")
	})

	t.Run("resets to closed state", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			MaxFailures:    2,
			Timeout:        1 * time.Second,
			HalfOpenMaxReq: 1,
		})

		cb.RecordFailure()
		cb.RecordFailure()
		assertEqualState(t, cb, CircuitOpen, "should be open")

		cb.Reset()
		assertEqualState(t, cb, CircuitClosed, "should be closed after reset")
	})

	t.Run("limits requests in half-open state", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			MaxFailures:    2,
			Timeout:        10 * time.Millisecond,
			HalfOpenMaxReq: 2,
		})

		cb.RecordFailure()
		cb.RecordFailure()
		time.Sleep(20 * time.Millisecond)

		if !cb.Allow() {
			t.Error("expected Allow() to return true")
		}
		if !cb.Allow() {
			t.Error("expected Allow() to return true for second request")
		}

		cb.Allow()
		if cb.Allow() {
			t.Error("expected Allow() to return false after max requests")
		}
	})

	t.Run("clears failure count on success", func(t *testing.T) {
		cb := NewCircuitBreaker(CircuitBreakerConfig{
			MaxFailures:    5,
			Timeout:        100 * time.Millisecond,
			HalfOpenMaxReq: 1,
		})

		cb.RecordFailure()
		cb.RecordFailure()
		assertEqual(t, cb.failureCount, 2, "failure count should be 2")

		cb.RecordSuccess()
		assertEqual(t, cb.failureCount, 0, "failure count should be 0 after success")
	})
}

func assertEqualState(t *testing.T, cb *CircuitBreaker, expected CircuitState, msg string) {
	t.Helper()
	if cb.State() != expected {
		t.Errorf("%s: expected %v, got %v", msg, expected, cb.State())
	}
}

func assertEqual(t *testing.T, actual, expected int, msg string) {
	t.Helper()
	if actual != expected {
		t.Errorf("%s: expected %d, got %d", msg, expected, actual)
	}
}
