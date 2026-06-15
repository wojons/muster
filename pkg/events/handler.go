package events

import "context"

// Handler defines the interface that event handlers must implement.
type Handler interface {
	// Handle processes an event and returns an error if processing fails.
	Handle(ctx context.Context, event *Event) error

	// Name returns a human-readable name for the handler.
	Name() string

	// Supports returns true if the handler can process the given event type.
	Supports(eventType EventType) bool
}
