// Package events provides an event dispatcher for the Muster system.
// It supports publish/subscribe patterns with filtering, priority ordering,
// retry logic, and dead-letter queues.
package events

// EventType identifies the category of an event.
type EventType string

// Event represents a published event in the system.
type Event struct {
	Type   EventType
	Data   []byte
	Source string
}
