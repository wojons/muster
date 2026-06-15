package events

// EventType represents the type/category of an event.
type EventType string

// Event represents a single event to be dispatched.
type Event struct {
	// Type is the event type used for routing to subscribers.
	Type EventType
	// Data holds the event payload as raw bytes.
	Data []byte
	// Source identifies where the event originated.
	Source string
}
