package events

import "context"

// Handler processes events dispatched by the Dispatcher.
// Handlers are registered for specific event types.
type Handler interface {
	// Handle processes an event. Returns an error if processing fails.
	Handle(ctx context.Context, event *Event) error

	// Name returns a human-readable name for this handler.
	Name() string

	// Supports returns true if this handler can process events of the given type.
	Supports(eventType EventType) bool
}
