package audit

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// MemoryStorage implements AuditStorage using an in-memory slice.
// This is useful for testing and scenarios where persistence is not required.
// It is thread-safe and maintains all events in memory until Close() is called.
type MemoryStorage struct {
	events []*AuditEvent
	mu     sync.RWMutex
	closed bool
}

// NewMemoryStorage creates a new in-memory audit storage.
func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{
		events: make([]*AuditEvent, 0),
	}
}

// Write adds an audit event to memory storage.
// Returns an error if the storage has been closed.
func (m *MemoryStorage) Write(event *AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return fmt.Errorf("storage is closed")
	}

	// Store a copy to prevent external modification
	m.events = append(m.events, event.Clone())
	return nil
}

// Query retrieves audit events matching the filter criteria.
// Returns events in chronological order (oldest first).
func (m *MemoryStorage) Query(filter AuditFilter) ([]*AuditEvent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed {
		return nil, fmt.Errorf("storage is closed")
	}

	var results []*AuditEvent

	for _, event := range m.events {
		if m.matchesFilter(event, filter) {
			results = append(results, event.Clone())
		}
	}

	return results, nil
}

// matchesFilter checks if an event matches the given filter criteria.
func (m *MemoryStorage) matchesFilter(event *AuditEvent, filter AuditFilter) bool {
	// Time range check
	if !filter.StartTime.IsZero() && event.Timestamp.Before(filter.StartTime) {
		return false
	}
	if !filter.EndTime.IsZero() && event.Timestamp.After(filter.EndTime) {
		return false
	}

	// Category check
	if len(filter.Categories) > 0 {
		found := false
		for _, cat := range filter.Categories {
			if event.Category == cat {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Level check
	if len(filter.Levels) > 0 {
		found := false
		for _, level := range filter.Levels {
			if event.Level == level {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Actor ID check
	if filter.ActorID != "" && event.Actor.ID != filter.ActorID {
		return false
	}

	// Resource type check
	if filter.ResourceType != "" && event.Resource.Type != filter.ResourceType {
		return false
	}

	// Action check
	if filter.Action != "" && event.Action != filter.Action {
		return false
	}

	// Success check
	if filter.Success != nil && event.Result.Success != *filter.Success {
		return false
	}

	return true
}

// Close marks the storage as closed and clears all events.
// This allows the memory to be garbage collected.
func (m *MemoryStorage) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	m.events = nil
	return nil
}

// GetAllEvents returns all events in storage (for testing only).
func (m *MemoryStorage) GetAllEvents() []*AuditEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*AuditEvent, len(m.events))
	for i, event := range m.events {
		result[i] = event.Clone()
	}
	return result
}

// FileStorage implements AuditStorage using a JSON Lines file.
// Each line in the file is a JSON-encoded AuditEvent.
// This provides durable persistence suitable for production use.
type FileStorage struct {
	path     string
	file     *os.File
	writer   *bufio.Writer
	mu       sync.Mutex
	closed   bool
	maxSize  int64
	maxFiles int
}

// FileStorageOptions configures the behavior of FileStorage.
type FileStorageOptions struct {
	// MaxSize is the maximum size of a log file before rotation (default: 100MB)
	MaxSize int64
	// MaxFiles is the maximum number of rotated files to keep (default: 10)
	MaxFiles int
}

// DefaultFileStorageOptions returns sensible defaults.
func DefaultFileStorageOptions() FileStorageOptions {
	return FileStorageOptions{
		MaxSize:  100 * 1024 * 1024, // 100MB
		MaxFiles: 10,
	}
}

// NewFileStorage creates a new file-based audit storage.
// The file is created if it doesn't exist, or appended to if it does.
func NewFileStorage(path string, opts FileStorageOptions) (*FileStorage, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Open file for append (create if doesn't exist)
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return &FileStorage{
		path:     path,
		file:     file,
		writer:   bufio.NewWriter(file),
		maxSize:  opts.MaxSize,
		maxFiles: opts.MaxFiles,
	}, nil
}

// Write persists an audit event to the log file.
// Events are buffered for efficiency and flushed periodically.
func (f *FileStorage) Write(event *AuditEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return fmt.Errorf("storage is closed")
	}

	// Check if rotation is needed
	if f.maxSize > 0 {
		stat, err := f.file.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat file: %w", err)
		}
		if stat.Size() >= f.maxSize {
			if err := f.rotate(); err != nil {
				return fmt.Errorf("failed to rotate file: %w", err)
			}
		}
	}

	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Write to file (JSON Lines format)
	if _, err := f.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}
	if err := f.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Flush immediately for durability
	if err := f.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	// Sync to disk for durability
	if err := f.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}

	return nil
}

// rotate moves the current log file to a backup and creates a new one.
func (f *FileStorage) rotate() error {
	// Close current file
	if err := f.writer.Flush(); err != nil {
		return err
	}
	if err := f.file.Close(); err != nil {
		return err
	}

	// Rotate existing files
	for i := f.maxFiles - 1; i > 0; i-- {
		oldPath := fmt.Sprintf("%s.%d", f.path, i)
		newPath := fmt.Sprintf("%s.%d", f.path, i+1)

		// Remove oldest if it exists
		if i == f.maxFiles-1 {
			_ = os.Remove(newPath)
		}

		// Move file if it exists
		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Rename(oldPath, newPath); err != nil {
				return err
			}
		}
	}

	// Move current file to .1
	if err := os.Rename(f.path, f.path+".1"); err != nil {
		return err
	}

	// Open new file
	file, err := os.OpenFile(f.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	f.file = file
	f.writer = bufio.NewWriter(file)

	return nil
}

// Query retrieves audit events matching the filter criteria.
// This reads from all log files including rotated ones.
func (f *FileStorage) Query(filter AuditFilter) ([]*AuditEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil, fmt.Errorf("storage is closed")
	}

	// Flush any pending writes before querying
	if err := f.writer.Flush(); err != nil {
		return nil, fmt.Errorf("failed to flush: %w", err)
	}

	var results []*AuditEvent

	// Query current and rotated files
	files := []string{f.path}
	for i := 1; i <= f.maxFiles; i++ {
		rotatedPath := fmt.Sprintf("%s.%d", f.path, i)
		if _, err := os.Stat(rotatedPath); err == nil {
			files = append(files, rotatedPath)
		}
	}

	for _, filePath := range files {
		events, err := f.queryFile(filePath, filter)
		if err != nil {
			return nil, fmt.Errorf("failed to query %s: %w", filePath, err)
		}
		results = append(results, events...)
	}

	return results, nil
}

// queryFile reads and filters events from a single file.
func (f *FileStorage) queryFile(filePath string, filter AuditFilter) ([]*AuditEvent, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var results []*AuditEvent
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var event AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			// Skip malformed lines but continue
			continue
		}

		if f.matchesFilter(&event, filter) {
			results = append(results, &event)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// matchesFilter checks if an event matches the filter criteria.
func (f *FileStorage) matchesFilter(event *AuditEvent, filter AuditFilter) bool {
	// Time range check
	if !filter.StartTime.IsZero() && event.Timestamp.Before(filter.StartTime) {
		return false
	}
	if !filter.EndTime.IsZero() && event.Timestamp.After(filter.EndTime) {
		return false
	}

	// Category check
	if len(filter.Categories) > 0 {
		found := false
		for _, cat := range filter.Categories {
			if event.Category == cat {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Level check
	if len(filter.Levels) > 0 {
		found := false
		for _, level := range filter.Levels {
			if event.Level == level {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Actor ID check
	if filter.ActorID != "" && event.Actor.ID != filter.ActorID {
		return false
	}

	// Resource type check
	if filter.ResourceType != "" && event.Resource.Type != filter.ResourceType {
		return false
	}

	// Action check
	if filter.Action != "" && event.Action != filter.Action {
		return false
	}

	// Success check
	if filter.Success != nil && event.Result.Success != *filter.Success {
		return false
	}

	return true
}

// Close cleanly shuts down the file storage, flushing any pending writes.
func (f *FileStorage) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return nil
	}

	f.closed = true

	if err := f.writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush on close: %w", err)
	}

	if err := f.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	return nil
}

// Rotate manually triggers log rotation (for testing/admin use).
func (f *FileStorage) Rotate() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.closed {
		return fmt.Errorf("storage is closed")
	}

	return f.rotate()
}
