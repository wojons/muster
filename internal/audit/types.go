package audit

import "time"

// AuditEvent represents a single audit log entry with comprehensive
// information about the action, actor, resource, and outcome.
// This is a placeholder type that will be fully defined in AUDIT-001 Step 5.
type AuditEvent struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Action    string                 `json:"action"`
	Actor     string                 `json:"actor"`
	Resource  string                 `json:"resource"`
	Outcome   string                 `json:"outcome"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}
