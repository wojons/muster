// Package audit provides audit logging types for the Muster application.
package audit

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// Audit level constants.
type AuditLevel string

const (
	AuditLevelInfo    AuditLevel = "info"
	AuditLevelWarning AuditLevel = "warning"
	AuditLevelError   AuditLevel = "error"
)

// Audit category constants.
type AuditCategory string

// Audit event types emitted by the entry-point hook (CLI commands, MCP tool
// calls, authentication). The dotted noun.verb form matches the
// management-plane emitters in internal/builtin (webhook.create, tenant.set, …)
// and is what `audit log --filter` matches against.
const (
	EventTypeCommand = "command.execute"
	EventTypeTool    = "tool.call"
	EventTypeAuth    = "auth.attempt"
)

const (
	AuditCategorySystem AuditCategory = "system"
	AuditCategoryAccess AuditCategory = "access"
	AuditCategoryData   AuditCategory = "data"
	AuditCategoryAuth   AuditCategory = "auth"
)

// AuditActor identifies who performed an action.
// Field tags follow specs/EVENTS-AUDIT.md (snake_case) — the audit sink is a
// machine-readable wire format, so Go field names must never leak into it.
type AuditActor struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

// AuditResource identifies what was acted upon.
type AuditResource struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

// AuditResult captures the outcome of an audited action.
type AuditResult struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
}

// AuditContext holds additional metadata for an audit event.
type AuditContext struct {
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AuditEvent represents a single auditable event.
type AuditEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	EventType string                 `json:"event_type"`
	User      string                 `json:"user"`
	Level     AuditLevel             `json:"level"`
	Category  AuditCategory          `json:"category"`
	Action    string                 `json:"action"`
	Actor     AuditActor             `json:"actor"`
	Resource  AuditResource          `json:"resource"`
	Result    AuditResult            `json:"result"`
	Context   AuditContext           `json:"context"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// AuditConfig holds configuration for audit logging.
type AuditConfig struct {
	Enabled         bool
	LogPath         string
	BufferSize      int
	FlushInterval   time.Duration
	RedactSensitive bool
	StorageType     string
	RetentionDays   int
	HashAlgorithm   string
}

// DefaultFileStorageOptions returns the default file storage options.
func DefaultFileStorageOptions() FileStorageOptions {
	return FileStorageOptions{
		MaxSize:    10 * 1024 * 1024,
		MaxBackups: 5,
		Compress:   false,
	}
}

// AuditLogger is the audit logging engine.
type AuditLogger struct {
	config  AuditConfig
	storage AuditStorage
	events  chan *AuditEvent
	done    chan struct{}
	wg      sync.WaitGroup
}

// AuditStorage is the interface for audit event persistence.
type AuditStorage interface {
	Write(event *AuditEvent) error
	Close() error
}

// FileStorage writes audit events to a file as JSON lines.
type FileStorage struct {
	mu   sync.Mutex
	file *os.File
}

// FileStorageOptions configures file-based audit storage.
type FileStorageOptions struct {
	MaxSize    int64
	MaxBackups int
	Compress   bool
}

// NewFileStorage creates a new file-based audit storage.
func NewFileStorage(path string, opts FileStorageOptions) (*FileStorage, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &FileStorage{file: f}, nil
}

func (fs *FileStorage) Write(event *AuditEvent) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	_, err = fs.file.Write(data)
	return err
}

func (fs *FileStorage) Close() error {
	return fs.file.Close()
}

var _ AuditStorage = (*FileStorage)(nil)

// NewAuditLogger creates a new audit logger and starts a background
// goroutine that consumes events from the channel and writes them to storage.
func NewAuditLogger(config AuditConfig, storage AuditStorage) *AuditLogger {
	l := &AuditLogger{
		config:  config,
		storage: storage,
		events:  make(chan *AuditEvent, config.BufferSize),
		done:    make(chan struct{}),
	}
	l.wg.Add(1)
	go l.consume()
	return l
}

// consume reads events from the channel and writes them to storage.
// It runs until Close() is called.
func (l *AuditLogger) consume() {
	defer l.wg.Done()
	for {
		select {
		case event := <-l.events:
			if l.storage != nil {
				_ = l.storage.Write(event)
			}
		case <-l.done:
			// Drain remaining events before shutting down.
			for {
				select {
				case event := <-l.events:
					if l.storage != nil {
						_ = l.storage.Write(event)
					}
				default:
					return
				}
			}
		}
	}
}

// Log records an audit event.
func (l *AuditLogger) Log(ctx context.Context, event *AuditEvent) error {
	select {
	case l.events <- event:
		return nil
	default:
		return nil
	}
}

// Close shuts down the audit logger, waits for the consumer goroutine
// to drain remaining events, then closes the storage.
func (l *AuditLogger) Close() error {
	close(l.done)
	l.wg.Wait()
	if l.storage != nil {
		return l.storage.Close()
	}
	return nil
}

var _ = io.Discard
