package events

import (
	"encoding/json"
	"sync"
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

// RetryConfig defines retry behavior for event handlers.
type RetryConfig struct {
	MaxRetries    int           `json:"max_retries"`
	InitialDelay  time.Duration `json:"initial_delay"`
	MaxDelay      time.Duration `json:"max_delay"`
	Exponential   bool          `json:"exponential"`
	Multiplier    float64       `json:"multiplier"`
	Randomization float64       `json:"randomization"`
}

// DefaultRetryConfig provides sensible defaults for retry behavior.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  100 * time.Millisecond,
		MaxDelay:      30 * time.Second,
		Exponential:   true,
		Multiplier:    2.0,
		Randomization: 0.5,
	}
}

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed - normal operation, requests flow through
	CircuitClosed CircuitState = iota
	// CircuitOpen - circuit is tripped, requests fail immediately
	CircuitOpen
	// CircuitHalfOpen - testing if service recovered
	CircuitHalfOpen
)

// String returns a string representation of the circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig defines circuit breaker behavior.
type CircuitBreakerConfig struct {
	MaxFailures    int           `json:"max_failures"`
	Timeout        time.Duration `json:"timeout"`
	HalfOpenMaxReq int           `json:"half_open_max_requests"`
}

// DefaultCircuitBreakerConfig provides sensible defaults.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxFailures:    5,
		Timeout:        30 * time.Second,
		HalfOpenMaxReq: 3,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
// It prevents cascading failures by failing fast when a service is unhealthy.
type CircuitBreaker struct {
	mu sync.RWMutex

	state           CircuitState
	failureCount    int
	successCount    int
	lastFailureTime time.Time
	halfOpenReqs    int

	config CircuitBreakerConfig
}

// NewCircuitBreaker creates a new circuit breaker with the given config.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:  CircuitClosed,
		config: config,
	}
}

// Allow checks if a request should be allowed through.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailureTime) > cb.config.Timeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenReqs = 0
			cb.successCount = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		if cb.halfOpenReqs < cb.config.HalfOpenMaxReq {
			cb.halfOpenReqs++
			return true
		}
		return false
	}
	return false
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0

	if cb.state == CircuitHalfOpen {
		cb.successCount++
		cb.halfOpenReqs++
		if cb.successCount >= cb.config.HalfOpenMaxReq {
			cb.state = CircuitClosed
			cb.successCount = 0
			cb.halfOpenReqs = 0
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.state == CircuitHalfOpen {
		cb.state = CircuitOpen
		cb.halfOpenReqs = 0
		cb.successCount = 0
	} else if cb.failureCount >= cb.config.MaxFailures {
		cb.state = CircuitOpen
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenReqs = 0
}
