// Package audit provides security audit logging with tamper-resistant storage,
// credential redaction, and chain of custody for compliance requirements.
package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Level represents the severity level of an audit event.
type Level string

const (
	LevelDebug    Level = "debug"
	LevelInfo     Level = "info"
	LevelWarning  Level = "warning"
	LevelError    Level = "error"
	LevelCritical Level = "critical"
)

// Category represents the category of an audit event.
type Category string

const (
	CategoryAuth     Category = "authentication"
	CategoryAccess   Category = "authorization"
	CategoryData     Category = "data_access"
	CategorySystem   Category = "system"
	CategorySecurity Category = "security"
	CategoryAdmin    Category = "administrative"
)

// Config holds configuration for the audit logger.
type Config struct {
	Enabled         bool          `json:"enabled"`
	StorageType     string        `json:"storage_type"`
	RetentionDays   int           `json:"retention_days"`
	BufferSize      int           `json:"buffer_size"`
	FlushInterval   time.Duration `json:"flush_interval"`
	EncryptionKey   string        `json:"encryption_key,omitempty"`
	HashAlgorithm   string        `json:"hash_algorithm"`
	IncludeBody     bool          `json:"include_body"`
	SensitiveFields []string      `json:"sensitive_fields"`
}

// DefaultConfig returns the default audit configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:       true,
		StorageType:   "file",
		RetentionDays: 365,
		BufferSize:    1000,
		FlushInterval: time.Second * 5,
		HashAlgorithm: "sha256",
		SensitiveFields: []string{
			"password", "token", "secret", "api_key", "apikey",
			"client_secret", "credit_card", "cc_number", "ssn", "social_security",
		},
	}
}

// Validate ensures the configuration is valid and secure.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	// Ensure sensitive fields list includes mandatory fields
	mandatory := []string{"password", "token", "secret", "api_key"}
	for _, field := range mandatory {
		found := false
		for _, sf := range c.SensitiveFields {
			if strings.EqualFold(sf, field) {
				found = true
				break
			}
		}
		if !found {
			c.SensitiveFields = append(c.SensitiveFields, field)
		}
	}

	// Ensure IncludeBody=false for production (check via env or explicit setting)
	if c.IncludeBody && os.Getenv("ENVIRONMENT") == "production" {
		return fmt.Errorf("include_body cannot be true in production environment")
	}

	if c.BufferSize < 1 {
		c.BufferSize = 1000
	}

	if c.FlushInterval == 0 {
		c.FlushInterval = time.Second * 5
	}

	return nil
}

// Event represents a single audit event.
type Event struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	Level        Level                  `json:"level"`
	Category     Category               `json:"category"`
	Action       string                 `json:"action"`
	Actor        Actor                  `json:"actor"`
	Resource     Resource               `json:"resource"`
	Result       Result                 `json:"result"`
	Context      map[string]interface{} `json:"context,omitempty"`
	Changes      []Change               `json:"changes,omitempty"`
	Hash         string                 `json:"hash"`
	PreviousHash string                 `json:"previous_hash,omitempty"`
}

// Actor represents the entity that performed the action.
type Actor struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// Resource represents the resource being acted upon.
type Resource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

// Result represents the outcome of the action.
type Result struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Details string `json:"details,omitempty"`
}

// Change represents a field change in the audit event.
type Change struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value,omitempty"`
	NewValue interface{} `json:"new_value,omitempty"`
}

// Filter provides criteria for querying audit events.
type Filter struct {
	StartTime    time.Time
	EndTime      time.Time
	Categories   []Category
	Levels       []Level
	ActorID      string
	ResourceType string
	Action       string
	Success      *bool
}

// Storage defines the interface for audit event storage backends.
type Storage interface {
	Write(event *Event) error
	Query(filter Filter) ([]*Event, error)
	Close() error
}

// RedactionRule defines a pattern for redacting sensitive data.
type RedactionRule struct {
	Pattern     string         `json:"pattern"`
	Replacement string         `json:"replacement"`
	FieldPaths  []string       `json:"field_paths"`
	Mandatory   bool           `json:"mandatory"`
	compiled    *regexp.Regexp `json:"-"`
}

// Redactor applies redaction rules to audit events.
type Redactor struct {
	rules []RedactionRule
	mu    sync.RWMutex
}

// DefaultRedactionRules returns the mandatory redaction rules.
func DefaultRedactionRules() []RedactionRule {
	return []RedactionRule{
		{
			Pattern:     `(?i)(password|passwd|pwd)`,
			Replacement: "[REDACTED]",
			FieldPaths:  []string{"password", "passwd", "pwd"},
			Mandatory:   true,
		},
		{
			Pattern:     `(?i)(token|access_token|refresh_token|id_token)`,
			Replacement: "[REDACTED]",
			FieldPaths:  []string{"token", "access_token", "refresh_token", "id_token"},
			Mandatory:   true,
		},
		{
			Pattern:     `(?i)(secret|api_key|apikey|client_secret)`,
			Replacement: "[REDACTED]",
			FieldPaths:  []string{"secret", "api_key", "apikey", "client_secret"},
			Mandatory:   true,
		},
		{
			Pattern:     `(?i)(credit_card|cc_number|card_number)`,
			Replacement: "[REDACTED]",
			FieldPaths:  []string{"credit_card", "cc_number", "card_number"},
			Mandatory:   true,
		},
		{
			Pattern:     `(?i)(ssn|social_security)`,
			Replacement: "[REDACTED]",
			FieldPaths:  []string{"ssn", "social_security"},
			Mandatory:   true,
		},
	}
}

// NewRedactor creates a new redactor with default rules.
func NewRedactor() *Redactor {
	rules := DefaultRedactionRules()

	// Compile regex patterns
	for i := range rules {
		re, err := regexp.Compile(rules[i].Pattern)
		if err != nil {
			// Fallback to case-insensitive literal match
			re = regexp.MustCompile(regexp.QuoteMeta(rules[i].Pattern))
		}
		rules[i].compiled = re
	}

	return &Redactor{
		rules: rules,
	}
}

// RedactEvent applies redaction rules to an audit event.
func (r *Redactor) RedactEvent(event *Event) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Redact context fields
	if event.Context != nil {
		r.redactMap(event.Context)
	}

	// Redact changes
	for i := range event.Changes {
		if r.isSensitiveField(event.Changes[i].Field) {
			event.Changes[i].OldValue = "[REDACTED]"
			event.Changes[i].NewValue = "[REDACTED]"
		}
	}

	return nil
}

// redactMap recursively redacts sensitive fields in a map.
func (r *Redactor) redactMap(m map[string]interface{}) {
	for key, value := range m {
		if r.isSensitiveField(key) {
			m[key] = "[REDACTED]"
		} else if nestedMap, ok := value.(map[string]interface{}); ok {
			r.redactMap(nestedMap)
		} else if nestedSlice, ok := value.([]interface{}); ok {
			for _, item := range nestedSlice {
				if itemMap, ok := item.(map[string]interface{}); ok {
					r.redactMap(itemMap)
				}
			}
		}
	}
}

// isSensitiveField checks if a field name matches any sensitive field pattern.
func (r *Redactor) isSensitiveField(field string) bool {
	fieldLower := strings.ToLower(field)
	for _, rule := range r.rules {
		for _, path := range rule.FieldPaths {
			if strings.EqualFold(fieldLower, path) {
				return true
			}
		}
	}
	return false
}

// Logger provides secure audit logging with credential redaction.
type Logger struct {
	config   Config
	storage  Storage
	redactor *Redactor
	buffer   []*Event
	mu       sync.Mutex
	lastHash string
	done     chan struct{}
	wg       sync.WaitGroup
	logger   zerolog.Logger
}

// NewLogger creates a new audit logger with the given configuration and storage.
func NewLogger(config Config, storage Storage) (*Logger, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid audit config: %w", err)
	}

	al := &Logger{
		config:   config,
		storage:  storage,
		redactor: NewRedactor(),
		buffer:   make([]*Event, 0, config.BufferSize),
		done:     make(chan struct{}),
		logger:   zerolog.New(os.Stderr).With().Timestamp().Logger(),
	}

	// Only start flush loop if logger is enabled and has a valid interval
	if config.Enabled && config.FlushInterval > 0 {
		al.wg.Add(1)
		go al.flushLoop()
	}

	return al, nil
}

// Log records an audit event synchronously.
func (al *Logger) Log(ctx context.Context, event *Event) error {
	if !al.config.Enabled {
		return nil
	}

	event.ID = al.generateID()
	event.Timestamp = time.Now().UTC()
	event.Hash = al.calculateHash(event)
	event.PreviousHash = al.lastHash
	al.lastHash = event.Hash

	// Apply credential redaction
	if err := al.redactor.RedactEvent(event); err != nil {
		al.logger.Error().Err(err).Msg("failed to redact audit event, proceeding with manual redaction")
		al.manualRedact(event)
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

// LogAsync records an audit event asynchronously.
func (al *Logger) LogAsync(ctx context.Context, event *Event) {
	if !al.config.Enabled {
		return
	}

	go func() {
		if err := al.Log(ctx, event); err != nil {
			al.logger.Error().Err(err).Msg("async audit log failed")
		}
	}()
}

// generateID generates a unique event ID.
func (al *Logger) generateID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), generateRandomString(8))
}

// generateRandomString generates a random string of the given length.
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// calculateHash computes the hash for chain of custody.
func (al *Logger) calculateHash(event *Event) string {
	data, _ := json.Marshal(struct {
		ID           string    `json:"id"`
		Timestamp    time.Time `json:"timestamp"`
		Level        string    `json:"level"`
		Category     string    `json:"category"`
		Action       string    `json:"action"`
		ActorID      string    `json:"actor_id"`
		PreviousHash string    `json:"previous_hash"`
	}{
		ID:           event.ID,
		Timestamp:    event.Timestamp,
		Level:        string(event.Level),
		Category:     string(event.Category),
		Action:       event.Action,
		ActorID:      event.Actor.ID,
		PreviousHash: event.PreviousHash,
	})

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// manualRedact performs manual redaction of known sensitive fields.
func (al *Logger) manualRedact(event *Event) {
	if event.Context != nil {
		sensitiveFields := []string{"password", "token", "secret", "api_key", "apikey", "client_secret"}
		for _, field := range sensitiveFields {
			if _, ok := event.Context[field]; ok {
				event.Context[field] = "[REDACTED]"
			}
		}
	}
}

// Flush writes all buffered events to storage.
func (al *Logger) Flush() error {
	al.mu.Lock()
	events := make([]*Event, len(al.buffer))
	copy(events, al.buffer)
	al.buffer = al.buffer[:0]
	al.mu.Unlock()

	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		if err := al.storage.Write(event); err != nil {
			// Re-add failed events to buffer for retry
			al.mu.Lock()
			al.buffer = append([]*Event{event}, al.buffer...)
			al.mu.Unlock()
			return fmt.Errorf("failed to write audit event: %w", err)
		}
	}

	return nil
}

// Query retrieves audit events matching the filter.
func (al *Logger) Query(filter Filter) ([]*Event, error) {
	return al.storage.Query(filter)
}

// flushLoop periodically flushes the buffer.
func (al *Logger) flushLoop() {
	defer al.wg.Done()

	ticker := time.NewTicker(al.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := al.Flush(); err != nil {
				al.logger.Error().Err(err).Msg("audit flush failed")
			}
		case <-al.done:
			if err := al.Flush(); err != nil {
				al.logger.Error().Err(err).Msg("final audit flush failed")
			}
			return
		}
	}
}

// Close shuts down the audit logger and flushes remaining events.
func (al *Logger) Close() error {
	if al.config.Enabled && al.config.FlushInterval > 0 {
		close(al.done)
		al.wg.Wait()
	}
	return al.storage.Close()
}

// FileStorage implements Storage for file-based audit logging.
type FileStorage struct {
	writer io.WriteCloser
	mu     sync.Mutex
}

// NewFileStorage creates a new file-based storage.
func NewFileStorage(filename string) (*FileStorage, error) {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit file: %w", err)
	}

	return &FileStorage{writer: f}, nil
}

// Write writes an event to the file.
func (fs *FileStorage) Write(event *Event) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}

	_, err = fs.writer.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}

	return nil
}

// Query is not implemented for file storage.
func (fs *FileStorage) Query(filter Filter) ([]*Event, error) {
	return nil, fmt.Errorf("query not supported for file storage")
}

// Close closes the file.
func (fs *FileStorage) Close() error {
	return fs.writer.Close()
}

// DatabaseStorage implements Storage for database-based audit logging.
type DatabaseStorage struct {
	db *sql.DB
}

// NewDatabaseStorage creates a new database-based storage.
func NewDatabaseStorage(db *sql.DB) (*DatabaseStorage, error) {
	return &DatabaseStorage{db: db}, nil
}

// Write writes an event to the database.
func (ds *DatabaseStorage) Write(event *Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := json.Marshal(event.Context)
	if err != nil {
		data = []byte("{}")
	}

	_, err = ds.db.ExecContext(ctx,
		`INSERT INTO audit (action, entity_type, entity_id, user_id, data, ip_address, user_agent, created_at) 
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.Action,
		event.Resource.Type,
		event.Resource.ID,
		event.Actor.ID,
		data,
		event.Actor.IPAddress,
		event.Actor.UserAgent,
		event.Timestamp,
	)

	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}

	return nil
}

// Query retrieves events from the database.
func (ds *DatabaseStorage) Query(filter Filter) ([]*Event, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	query := `SELECT action, entity_type, entity_id, user_id, data, ip_address, user_agent, created_at 
	          FROM audit WHERE 1=1`
	var args []interface{}

	if !filter.StartTime.IsZero() {
		query += " AND created_at >= ?"
		args = append(args, filter.StartTime)
	}
	if !filter.EndTime.IsZero() {
		query += " AND created_at <= ?"
		args = append(args, filter.EndTime)
	}
	if filter.ActorID != "" {
		query += " AND user_id = ?"
		args = append(args, filter.ActorID)
	}
	if filter.ResourceType != "" {
		query += " AND entity_type = ?"
		args = append(args, filter.ResourceType)
	}
	if filter.Action != "" {
		query += " AND action = ?"
		args = append(args, filter.Action)
	}

	query += " ORDER BY created_at DESC LIMIT 1000"

	rows, err := ds.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit events: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var event Event
		var data []byte
		err := rows.Scan(
			&event.Action,
			&event.Resource.Type,
			&event.Resource.ID,
			&event.Actor.ID,
			&data,
			&event.Actor.IPAddress,
			&event.Actor.UserAgent,
			&event.Timestamp,
		)
		if err != nil {
			continue
		}

		if len(data) > 0 {
			json.Unmarshal(data, &event.Context)
		}

		events = append(events, &event)
	}

	return events, nil
}

// Close is a no-op for database storage (connection managed externally).
func (ds *DatabaseStorage) Close() error {
	return nil
}
