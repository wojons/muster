package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// AuditLevel represents the severity level of an audit event.
type AuditLevel string

const (
	AuditLevelDebug    AuditLevel = "debug"
	AuditLevelInfo     AuditLevel = "info"
	AuditLevelWarning  AuditLevel = "warning"
	AuditLevelError    AuditLevel = "error"
	AuditLevelCritical AuditLevel = "critical"
)

// IsValid returns true if the audit level is one of the defined constants.
func (l AuditLevel) IsValid() bool {
	switch l {
	case AuditLevelDebug, AuditLevelInfo, AuditLevelWarning,
		AuditLevelError, AuditLevelCritical:
		return true
	}
	return false
}

// AuditCategory represents the category of an audit event.
type AuditCategory string

const (
	AuditCategoryAuth     AuditCategory = "authentication"
	AuditCategoryAccess   AuditCategory = "authorization"
	AuditCategoryData     AuditCategory = "data_access"
	AuditCategorySystem   AuditCategory = "system"
	AuditCategorySecurity AuditCategory = "security"
	AuditCategoryAdmin    AuditCategory = "administrative"
)

// IsValid returns true if the audit category is one of the defined constants.
func (c AuditCategory) IsValid() bool {
	switch c {
	case AuditCategoryAuth, AuditCategoryAccess, AuditCategoryData,
		AuditCategorySystem, AuditCategorySecurity, AuditCategoryAdmin:
		return true
	}
	return false
}

// AuditActor represents the entity that performed the action being audited.
type AuditActor struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// Validate checks if the audit actor has required fields.
func (a AuditActor) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("actor ID is required")
	}
	if a.Type == "" {
		return fmt.Errorf("actor type is required")
	}
	return nil
}

// AuditResource represents the resource that was acted upon.
type AuditResource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

// Validate checks if the audit resource has required fields.
func (r AuditResource) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("resource ID is required")
	}
	if r.Type == "" {
		return fmt.Errorf("resource type is required")
	}
	return nil
}

// AuditResult represents the outcome of the audited action.
type AuditResult struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Details string `json:"details,omitempty"`
}

// AuditContext provides additional context for the audit event.
type AuditContext struct {
	RequestID string            `json:"request_id,omitempty"`
	TraceID   string            `json:"trace_id,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// AuditChange represents a field-level change in an audit event.
type AuditChange struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value,omitempty"`
	NewValue interface{} `json:"new_value,omitempty"`
}

// AuditEvent represents a single audit log entry with comprehensive
// information about the action, actor, resource, and outcome.
type AuditEvent struct {
	ID           string        `json:"id"`
	Timestamp    time.Time     `json:"timestamp"`
	Level        AuditLevel    `json:"level"`
	Category     AuditCategory `json:"category"`
	Action       string        `json:"action"`
	Actor        AuditActor    `json:"actor"`
	Resource     AuditResource `json:"resource"`
	Result       AuditResult   `json:"result"`
	Context      AuditContext  `json:"context"`
	Changes      []AuditChange `json:"changes,omitempty"`
	Hash         string        `json:"hash"`
	PreviousHash string        `json:"previous_hash,omitempty"`
}

// Validate checks if the audit event has all required fields.
func (e *AuditEvent) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("event ID is required")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}
	if !e.Level.IsValid() {
		return fmt.Errorf("invalid audit level: %s", e.Level)
	}
	if !e.Category.IsValid() {
		return fmt.Errorf("invalid audit category: %s", e.Category)
	}
	if e.Action == "" {
		return fmt.Errorf("action is required")
	}
	if err := e.Actor.Validate(); err != nil {
		return fmt.Errorf("invalid actor: %w", err)
	}
	if err := e.Resource.Validate(); err != nil {
		return fmt.Errorf("invalid resource: %w", err)
	}
	return nil
}

// ComputeHash calculates the SHA256 hash of the event data.
// This is used for the hash chain to provide tamper resistance.
func (e *AuditEvent) ComputeHash() string {
	// Create a copy without the hash fields for consistent hashing
	eventCopy := *e
	eventCopy.Hash = ""
	eventCopy.PreviousHash = ""

	// Serialize to JSON for consistent hashing
	data, err := json.Marshal(eventCopy)
	if err != nil {
		// If serialization fails, use a fallback
		return ""
	}

	// Append previous hash if it exists
	if e.PreviousHash != "" {
		data = append(data, []byte(e.PreviousHash)...)
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// SetHash computes and sets the hash for this event.
func (e *AuditEvent) SetHash(previousHash string) {
	e.PreviousHash = previousHash
	e.Hash = e.ComputeHash()
}

// String returns a human-readable representation of the audit event.
func (e *AuditEvent) String() string {
	return fmt.Sprintf("AuditEvent{ID: %s, Level: %s, Category: %s, Action: %s, Actor: %s, Resource: %s, Success: %t}",
		e.ID, e.Level, e.Category, e.Action, e.Actor.Name, e.Resource.Name, e.Result.Success)
}

// Clone creates a deep copy of the audit event.
func (e *AuditEvent) Clone() *AuditEvent {
	if e == nil {
		return nil
	}

	clone := &AuditEvent{
		ID:           e.ID,
		Timestamp:    e.Timestamp,
		Level:        e.Level,
		Category:     e.Category,
		Action:       e.Action,
		Actor:        e.Actor,
		Resource:     e.Resource,
		Result:       e.Result,
		Context:      e.Context,
		Hash:         e.Hash,
		PreviousHash: e.PreviousHash,
	}

	// Deep copy changes
	if e.Changes != nil {
		clone.Changes = make([]AuditChange, len(e.Changes))
		copy(clone.Changes, e.Changes)
	}

	// Deep copy context metadata
	if e.Context.Metadata != nil {
		clone.Context.Metadata = make(map[string]string, len(e.Context.Metadata))
		for k, v := range e.Context.Metadata {
			clone.Context.Metadata[k] = v
		}
	}

	return clone
}
