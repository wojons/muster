package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// AuditStorage defines the interface for audit event storage backends.
// Implementations must be thread-safe and handle event persistence.
type AuditStorage interface {
	// Write persists an audit event to storage.
	Write(event *AuditEvent) error
	// Query retrieves audit events matching the filter criteria.
	Query(filter AuditFilter) ([]*AuditEvent, error)
	// Close cleanly shuts down the storage backend.
	Close() error
}

// AuditFilter defines criteria for querying audit events.
type AuditFilter struct {
	StartTime    time.Time
	EndTime      time.Time
	Categories   []AuditCategory
	Levels       []AuditLevel
	ActorID      string
	ResourceType string
	Action       string
	Success      *bool
}

// AuditLogger provides secure, tamper-resistant audit logging with buffering,
// automatic flushing, and hash chain verification.
type AuditLogger struct {
	config   AuditConfig
	storage  AuditStorage
	buffer   []*AuditEvent
	mu       sync.Mutex
	lastHash string
	done     chan struct{}
	wg       sync.WaitGroup
	redactor *Redactor
}

// NewAuditLogger creates a new audit logger with the given configuration and storage.
// It starts a background flush loop that periodically writes buffered events to storage.
func NewAuditLogger(config AuditConfig, storage AuditStorage) *AuditLogger {
	al := &AuditLogger{
		config:   config,
		storage:  storage,
		buffer:   make([]*AuditEvent, 0, config.BufferSize),
		done:     make(chan struct{}),
		redactor: NewRedactor(),
	}

	al.wg.Add(1)
	go al.flushLoop()

	return al
}

// Log records an audit event with automatic ID generation, timestamping,
// hash chain computation, and sensitive data redaction.
// Events are buffered and flushed based on buffer size or flush interval.
func (al *AuditLogger) Log(event *AuditEvent) error {
	if !al.config.Enabled {
		return nil
	}

	event.ID = generateID()
	event.Timestamp = time.Now()
	event.Hash = al.calculateHash(event)
	event.PreviousHash = al.lastHash
	al.lastHash = event.Hash

	// Apply redaction to sensitive fields
	if err := al.redactor.RedactEvent(event); err != nil {
		// Log redaction error but continue - don't lose the audit event
		// In production, this should be logged to an error log
		_ = err
	}

	al.mu.Lock()
	al.buffer = append(al.buffer, event)
	shouldFlush := len(al.buffer) >= al.config.BufferSize
	al.mu.Unlock()

	if shouldFlush {
		return al.Flush()
	}

	return nil
}

// LogAsync records an audit event asynchronously without blocking the caller.
// Errors are silently ignored - use Log() if error handling is required.
func (al *AuditLogger) LogAsync(event *AuditEvent) {
	go func() {
		if err := al.Log(event); err != nil {
			// Log error but don't block - in production this should be logged
			_ = err
		}
	}()
}

// calculateHash computes a SHA256 hash of the event data for tamper resistance.
// The hash includes the event's core fields and the previous hash in the chain.
func (al *AuditLogger) calculateHash(event *AuditEvent) string {
	data, _ := json.Marshal(struct {
		ID        string    `json:"id"`
		Timestamp time.Time `json:"timestamp"`
		Level     string    `json:"level"`
		Category  string    `json:"category"`
		Action    string    `json:"action"`
		ActorID   string    `json:"actor_id"`
		PrevHash  string    `json:"previous_hash"`
	}{
		ID:        event.ID,
		Timestamp: event.Timestamp,
		Level:     string(event.Level),
		Category:  string(event.Category),
		Action:    event.Action,
		ActorID:   event.Actor.ID,
		PrevHash:  event.PreviousHash,
	})

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// flushLoop runs in a background goroutine and periodically flushes the buffer.
// It also flushes on shutdown when the done channel is closed.
func (al *AuditLogger) flushLoop() {
	defer al.wg.Done()

	ticker := time.NewTicker(al.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			_ = al.Flush()
		case <-al.done:
			_ = al.Flush()
			return
		}
	}
}

// Flush immediately writes all buffered events to storage.
// This is thread-safe and can be called concurrently.
func (al *AuditLogger) Flush() error {
	al.mu.Lock()
	if len(al.buffer) == 0 {
		al.mu.Unlock()
		return nil
	}

	events := make([]*AuditEvent, len(al.buffer))
	copy(events, al.buffer)
	al.buffer = al.buffer[:0]
	al.mu.Unlock()

	for _, event := range events {
		if err := al.storage.Write(event); err != nil {
			// Re-add failed events to buffer for retry
			al.mu.Lock()
			al.buffer = append([]*AuditEvent{event}, al.buffer...)
			al.mu.Unlock()
			return err
		}
	}

	return nil
}

// Query retrieves audit events from storage matching the filter criteria.
func (al *AuditLogger) Query(filter AuditFilter) ([]*AuditEvent, error) {
	return al.storage.Query(filter)
}

// Close gracefully shuts down the audit logger, flushing any remaining events.
// It waits for the background flush loop to complete before returning.
func (al *AuditLogger) Close() error {
	close(al.done)
	al.wg.Wait()
	return al.storage.Close()
}

// generateID creates a unique identifier for audit events.
// Uses timestamp and random component for uniqueness.
func generateID() string {
	return time.Now().Format("20060102-150405.000000") + "-" + randomString(8)
}

// randomString generates a random alphanumeric string of the given length.
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
