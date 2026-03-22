package events

import (
	"encoding/json"
	"time"
)

// EventType represents the type of an event.
type EventType string

// Event type constants for built-in event types.
const (
	EventTypeInventoryCreated EventType = "inventory.created"
	EventTypeInventoryUpdated EventType = "inventory.updated"
	EventTypeInventoryDeleted EventType = "inventory.deleted"
	EventTypeUserLogin        EventType = "user.login"
	EventTypeUserLogout       EventType = "user.logout"
	EventTypeAuditLog         EventType = "audit.log"
	EventTypeRequest          EventType = "request"
	EventTypeResponse         EventType = "response"
	EventTypeError            EventType = "error"
	EventTypeCustom           EventType = "custom"
)

// Event represents an event in the system.
type Event struct {
	// ID is the unique identifier for this event.
	ID string `json:"id"`

	// Type is the event type.
	Type EventType `json:"type"`

	// Source identifies where the event originated from.
	Source string `json:"source"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// Data contains the event payload as raw JSON.
	Data json.RawMessage `json:"data"`

	// Metadata contains additional event metadata.
	Metadata EventMetadata `json:"metadata"`
}

// EventMetadata contains metadata about an event.
type EventMetadata struct {
	// CorrelationID links related events together.
	CorrelationID string `json:"correlation_id,omitempty"`

	// CausationID identifies the event that caused this event.
	CausationID string `json:"causation_id,omitempty"`

	// UserID identifies the user associated with the event.
	UserID string `json:"user_id,omitempty"`

	// TraceID is used for distributed tracing.
	TraceID string `json:"trace_id,omitempty"`

	// Attributes contains additional custom attributes.
	Attributes map[string]string `json:"attributes,omitempty"`
}

// NewEvent creates a new Event with the given type, source, and data.
// It automatically sets the ID and timestamp.
func NewEvent(eventType EventType, source string, data interface{}) (*Event, error) {
	id, err := GenerateEventID()
	if err != nil {
		return nil, err
	}

	var rawData json.RawMessage
	if data != nil {
		rawData, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}

	return &Event{
		ID:        id,
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now().UTC(),
		Data:      rawData,
		Metadata:  EventMetadata{},
	}, nil
}

// GenerateEventID generates a unique event ID.
// This is a placeholder implementation - in production, use UUID or similar.
func GenerateEventID() (string, error) {
	return time.Now().UTC().Format("20060102-150405.000000000"), nil
}

// UnmarshalData unmarshals the event data into the provided target.
func (e *Event) UnmarshalData(target interface{}) error {
	if len(e.Data) == 0 {
		return nil
	}
	return json.Unmarshal(e.Data, target)
}

// String returns a string representation of the event type.
func (et EventType) String() string {
	return string(et)
}
