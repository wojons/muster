package events

import (
	"context"
	"time"
)

// HandlerFunc is a function that handles an event.
// It receives a context and an event, and returns an error if processing fails.
type HandlerFunc func(context.Context, *Event) error

// Handler is the interface for event handlers.
// Implementations must provide methods to handle events, identify themselves,
// and declare which event types they support.
type Handler interface {
	// Handle processes the given event and returns an error if processing fails.
	Handle(ctx context.Context, event *Event) error

	// Name returns the unique name of the handler.
	Name() string

	// Supports returns true if the handler can process the given event type.
	Supports(eventType EventType) bool
}

// HandlerMiddleware is a function that wraps a HandlerFunc to add
// cross-cutting concerns like logging, metrics, timeout, or retry logic.
type HandlerMiddleware func(HandlerFunc) HandlerFunc

// ErrorHandler is a callback function for handling errors that occur
// during event processing. It receives the context, the event that failed,
// and the error that occurred. It can return a modified error or nil to
// suppress the error.
type ErrorHandler func(ctx context.Context, event *Event, err error) error

// HandlerInfo contains metadata and configuration for a registered handler.
// It is used when registering handlers with the HandlerRegistry.
type HandlerInfo struct {
	// Name is the unique identifier for this handler.
	Name string

	// EventTypes lists the event types this handler can process.
	EventTypes []EventType

	// Handler is the actual handler implementation.
	Handler Handler

	// Middleware is a list of middleware functions to wrap the handler.
	Middleware []HandlerMiddleware

	// Timeout is the maximum duration allowed for handler execution.
	// A zero or negative value means no timeout.
	Timeout time.Duration

	// MaxRetries is the number of times to retry failed executions.
	// A zero or negative value means no retries.
	MaxRetries int

	// OnError is an optional callback for custom error handling.
	OnError ErrorHandler
}

// HandlerResult contains the outcome of executing a handler for an event.
type HandlerResult struct {
	// HandlerName is the name of the handler that processed the event.
	HandlerName string

	// EventID is the identifier of the event that was processed.
	EventID string

	// Duration is the time taken to execute the handler.
	Duration time.Duration

	// Error is the error returned by the handler, if any.
	Error error

	// ProcessedAt is the timestamp when the handler completed execution.
	ProcessedAt time.Time
}

// IsSuccess returns true if the handler executed without errors.
func (r *HandlerResult) IsSuccess() bool {
	return r.Error == nil
}

// String returns a string representation of the handler result.
func (r *HandlerResult) String() string {
	if r.IsSuccess() {
		return r.HandlerName + ": success"
	}
	return r.HandlerName + ": " + r.Error.Error()
}

// Validate checks if the HandlerInfo is valid for registration.
// Returns an error if required fields are missing or invalid.
func (info *HandlerInfo) Validate() error {
	if info.Name == "" {
		return &ValidationError{Field: "Name", Message: "handler name is required"}
	}

	if info.Handler == nil {
		return &ValidationError{Field: "Handler", Message: "handler is required"}
	}

	if len(info.EventTypes) == 0 {
		return &ValidationError{Field: "EventTypes", Message: "at least one event type is required"}
	}

	for _, eventType := range info.EventTypes {
		if eventType == "" {
			return &ValidationError{Field: "EventTypes", Message: "event type cannot be empty"}
		}
	}

	return nil
}

// ValidationError represents an error that occurs during handler validation.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}
