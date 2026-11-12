// Package audit provides audit logging types for the Muster application.
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
//
// ID, Hash and PreviousHash make the persisted trail tamper-evident
// (specs/EVENTS-AUDIT.md): PreviousHash is the Hash of the record written
// immediately before this one, and Hash is the digest of this record's canonical
// preimage (see eventHashPreimage). Both are assigned by the sink write path and
// never trusted from the caller — a producer-supplied Hash is overwritten, so no
// emitter can forge a link in the chain.
type AuditEvent struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	EventType    string                 `json:"event_type"`
	User         string                 `json:"user"`
	Level        AuditLevel             `json:"level"`
	Category     AuditCategory          `json:"category"`
	Action       string                 `json:"action"`
	Actor        AuditActor             `json:"actor"`
	Resource     AuditResource          `json:"resource"`
	Result       AuditResult            `json:"result"`
	Context      AuditContext           `json:"context"`
	Details      map[string]interface{} `json:"details,omitempty"`
	Error        string                 `json:"error,omitempty"`
	Hash         string                 `json:"hash"`
	PreviousHash string                 `json:"previous_hash,omitempty"`
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
		MaxSize:       10 * 1024 * 1024,
		MaxBackups:    5,
		Compress:      false,
		RetentionDays: 365,
	}
}

// AuditLogger is the audit logging engine.
var (
	// ErrBufferFull distinguishes an audit event rejected because the
	// non-blocking buffer is saturated from other logging failures.
	ErrBufferFull = errors.New("audit: event buffer full")
)

type AuditLogger struct {
	config    AuditConfig
	storage   AuditStorage
	events    chan *AuditEvent
	done      chan struct{}
	wg        sync.WaitGroup
	metrics   Metrics
	closeOnce sync.Once
	closeErr  error
}

// AuditStorage is the interface for audit event persistence.
// Metrics is the optional observability sink for audit health. It is kept as a
// small interface so audit does not depend on the monitoring package.
type Metrics interface {
	RecordAuditEventDropped(reason string)
	SetAuditBufferUtilization(ratio float64)
}

var defaultMetrics struct {
	sync.RWMutex
	metrics Metrics
}

// SetMetrics installs the process-wide audit metrics sink. Monitoring calls this
// when it creates the registry served by /metrics; audit remains independent of
// the monitoring package and therefore cannot create an import cycle.
func SetMetrics(metrics Metrics) {
	defaultMetrics.Lock()
	defaultMetrics.metrics = metrics
	defaultMetrics.Unlock()
}

func metricsSink() Metrics {
	defaultMetrics.RLock()
	defer defaultMetrics.RUnlock()
	return defaultMetrics.metrics
}

type AuditStorage interface {
	Write(event *AuditEvent) error
	Close() error
}

func (l *AuditLogger) updateBufferUtilization() {
	if l.metrics == nil || cap(l.events) == 0 {
		if l.metrics != nil {
			l.metrics.SetAuditBufferUtilization(0)
		}
		return
	}
	l.metrics.SetAuditBufferUtilization(float64(len(l.events)) / float64(cap(l.events)))
}

func (l *AuditLogger) recordDrop(reason string) {
	if l.metrics != nil {
		l.metrics.RecordAuditEventDropped(reason)
	}
}

func (l *AuditLogger) write(event *AuditEvent) {
	if l.storage == nil {
		return
	}
	if err := l.storage.Write(event); err != nil {
		l.recordDrop("write_error")
	}
}

// FileStorage writes audit events to a file as JSON lines.
//
// It is also the hash-chain write point: every record leaving this writer has a
// redacted body, a fresh ID, a PreviousHash pointing at the record on disk
// before it, and a Hash covering both. lastHash is seeded from the tail of the
// file at open time, so a short-lived writer (the management-plane emitter opens
// one per event) continues the chain instead of restarting it.
type FileStorage struct {
	mu            sync.Mutex
	file          *os.File
	path          string
	lastHash      string
	hashAlgorithm string
}

// FileStorageOptions configures file-based audit storage.
type FileStorageOptions struct {
	MaxSize       int64
	MaxBackups    int
	Compress      bool
	RetentionDays int
}

// NewFileStorage creates a new file-based audit storage using the default
// sha256 chain algorithm.
func NewFileStorage(path string, opts FileStorageOptions) (*FileStorage, error) {
	return NewFileStorageWithHashAlgorithm(path, opts, HashAlgorithmSHA256)
}

// NewFileStorageWithHashAlgorithm creates a file-based audit storage whose audit
// chain uses algorithm (AuditConfig.HashAlgorithm; unknown values fall back to
// sha256).
func NewFileStorageWithHashAlgorithm(path string, opts FileStorageOptions, algorithm string) (*FileStorage, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &FileStorage{
		file:          f,
		path:          path,
		lastHash:      lastSinkHash(path),
		hashAlgorithm: normalizeHashAlgorithm(algorithm),
	}, nil
}

func (fs *FileStorage) Write(event *AuditEvent) error {
	if event == nil {
		return fmt.Errorf("audit: cannot write a nil event")
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Mandatory transformations, applied to every persisted record: identity,
	// timestamp, and non-disableable redaction of sensitive fields.
	prepareEvent(event)

	// Retention may have rotated the sink (rename) since this descriptor was
	// opened. Re-open and re-seed in that case, otherwise the record would be
	// appended to the unlinked inode and silently lost.
	if err := fs.reopenIfRotated(); err != nil {
		return err
	}

	// Seed from the sink when nothing has been written through this instance
	// yet: another writer may have appended between open and now.
	if fs.lastHash == "" {
		fs.lastHash = lastSinkHash(fs.path)
	}
	event.PreviousHash = fs.lastHash
	hash := computeEventHash(fs.hashAlgorithm, event)
	event.Hash = hash

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if _, err := fs.file.Write(data); err != nil {
		// The record never reached the sink; drop the provisional link so the
		// next event chains from the last hash that is actually persisted.
		event.Hash = ""
		event.PreviousHash = ""
		return err
	}

	fs.lastHash = hash
	return nil
}

func (fs *FileStorage) Close() error {
	return fs.file.Close()
}

// reopenIfRotated re-opens the sink when the path now points at a different
// file than the one this writer holds open (retention rotates by renaming a
// rewritten file into place). The chain is re-seeded from the new tail so the
// next record links to whatever is actually persisted.
func (fs *FileStorage) reopenIfRotated() error {
	pathInfo, err := os.Stat(fs.path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if fdInfo, statErr := fs.file.Stat(); statErr == nil && os.SameFile(pathInfo, fdInfo) {
			return nil
		}
	}
	f, openErr := os.OpenFile(fs.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if openErr != nil {
		return openErr
	}
	_ = fs.file.Close()
	fs.file = f
	fs.lastHash = ""
	return nil
}

var _ AuditStorage = (*FileStorage)(nil)

// NewAuditLogger creates a new audit logger and starts a background
// goroutine that consumes events from the channel and writes them to storage.
func NewAuditLogger(config AuditConfig, storage AuditStorage) *AuditLogger {
	return NewAuditLoggerWithMetrics(config, storage, metricsSink())
}

// NewAuditLoggerWithMetrics creates an audit logger with an optional metrics
// sink. The sink is injected to avoid an import cycle with monitoring.
func NewAuditLoggerWithMetrics(config AuditConfig, storage AuditStorage, metrics Metrics) *AuditLogger {
	bufferSize := config.BufferSize
	if bufferSize < 1 {
		bufferSize = 1
	}
	l := &AuditLogger{
		config:  config,
		storage: storage,
		events:  make(chan *AuditEvent, bufferSize),
		done:    make(chan struct{}),
		metrics: metrics,
	}
	l.updateBufferUtilization()
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
			l.write(event)
			l.updateBufferUtilization()
		case <-l.done:
			// Drain remaining events before shutting down.
			for {
				select {
				case event := <-l.events:
					l.write(event)
					l.updateBufferUtilization()
				default:
					l.updateBufferUtilization()
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
		l.updateBufferUtilization()
		return nil
	default:
		l.recordDrop("buffer_full")
		l.updateBufferUtilization()
		return ErrBufferFull
	}
}

// Close shuts down the audit logger, waits for the consumer goroutine
// to drain remaining events, then closes the storage.
func (l *AuditLogger) Close() error {
	l.closeOnce.Do(func() {
		close(l.done)
		l.wg.Wait()
		if l.storage != nil {
			l.closeErr = l.storage.Close()
		}
	})
	return l.closeErr
}

var _ = io.Discard
