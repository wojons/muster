package audit

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStorage is a test implementation of AuditStorage
type mockStorage struct {
	events   []*AuditEvent
	mu       sync.Mutex
	closed   bool
	writeErr error
	queryErr error
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		events: make([]*AuditEvent, 0),
	}
}

func (m *mockStorage) Write(event *AuditEvent) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

func (m *mockStorage) Query(filter AuditFilter) ([]*AuditEvent, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	eventsCopy := make([]*AuditEvent, len(m.events))
	copy(eventsCopy, m.events)
	return eventsCopy, nil
}

func (m *mockStorage) Close() error {
	m.closed = true
	return nil
}

func (m *mockStorage) GetEvents() []*AuditEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*AuditEvent, len(m.events))
	copy(result, m.events)
	return result
}

func TestNewAuditLogger(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 10
	config.FlushInterval = time.Second

	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)

	assert.NotNil(t, logger)
	assert.NotNil(t, logger.buffer)
	assert.NotNil(t, logger.done)
	assert.NotNil(t, logger.redactor)
	assert.Equal(t, 10, cap(logger.buffer))
	assert.Equal(t, config, logger.config)
	assert.Equal(t, storage, logger.storage)

	// Clean up
	err := logger.Close()
	require.NoError(t, err)
}

func TestAuditLogger_Log_Disabled(t *testing.T) {
	config := DefaultAuditConfig
	config.Enabled = false

	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	event := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "test-action",
		Actor:    AuditActor{ID: "user-1", Type: "user"},
		Resource: AuditResource{ID: "res-1", Type: "file"},
		Result:   AuditResult{Success: true},
	}

	err := logger.Log(event)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(storage.GetEvents()))
}

func TestAuditLogger_Log_Success(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 10
	config.FlushInterval = time.Hour // Long interval to prevent auto-flush

	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	event := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "test-action",
		Actor:    AuditActor{ID: "user-1", Type: "user"},
		Resource: AuditResource{ID: "res-1", Type: "file"},
		Result:   AuditResult{Success: true},
	}

	err := logger.Log(event)
	assert.NoError(t, err)

	// Event should be buffered, not written yet
	assert.Equal(t, 0, len(storage.GetEvents()))

	// Flush manually
	err = logger.Flush()
	assert.NoError(t, err)

	// Now event should be written
	events := storage.GetEvents()
	assert.Equal(t, 1, len(events))
	assert.NotEmpty(t, events[0].ID)
	assert.NotZero(t, events[0].Timestamp)
	assert.NotEmpty(t, events[0].Hash)
	assert.Equal(t, AuditLevelInfo, events[0].Level)
	assert.Equal(t, "test-action", events[0].Action)
}

func TestAuditLogger_Log_TriggersFlush(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 2 // Small buffer to trigger flush

	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	// Log 2 events (should trigger flush)
	for i := 0; i < 2; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "test-action",
			Actor:    AuditActor{ID: "user-1", Type: "user"},
			Resource: AuditResource{ID: "res-1", Type: "file"},
			Result:   AuditResult{Success: true},
		}
		err := logger.Log(event)
		require.NoError(t, err)
	}

	// Buffer should have triggered flush
	events := storage.GetEvents()
	assert.Equal(t, 2, len(events))
}

func TestAuditLogger_Log_HashChain(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 10
	config.FlushInterval = time.Hour

	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	// Log multiple events
	for i := 0; i < 3; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "test-action",
			Actor:    AuditActor{ID: "user-1", Type: "user"},
			Resource: AuditResource{ID: "res-1", Type: "file"},
			Result:   AuditResult{Success: true},
		}
		err := logger.Log(event)
		require.NoError(t, err)
	}

	// Flush and check hash chain
	err := logger.Flush()
	require.NoError(t, err)

	events := storage.GetEvents()
	require.Equal(t, 3, len(events))

	// First event should have empty previous hash
	assert.Empty(t, events[0].PreviousHash)

	// Subsequent events should reference previous hash
	assert.Equal(t, events[0].Hash, events[1].PreviousHash)
	assert.Equal(t, events[1].Hash, events[2].PreviousHash)
}

func TestAuditLogger_Log_SensitiveDataRedaction(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 1

	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	event := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategoryAuth,
		Action:   "login",
		Actor:    AuditActor{ID: "user-1", Type: "user"},
		Resource: AuditResource{ID: "api", Type: "endpoint"},
		Result:   AuditResult{Success: true},
		Context: AuditContext{
			Metadata: map[string]string{
				"password": "secret123",
				"token":    "abc123",
			},
		},
	}

	err := logger.Log(event)
	require.NoError(t, err)

	events := storage.GetEvents()
	require.Equal(t, 1, len(events))

	// Sensitive fields should be redacted
	assert.Equal(t, "[REDACTED]", events[0].Context.Metadata["password"])
	assert.Equal(t, "[REDACTED]", events[0].Context.Metadata["token"])
}

func TestAuditLogger_LogAsync(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 1

	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	event := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "test-action",
		Actor:    AuditActor{ID: "user-1", Type: "user"},
		Resource: AuditResource{ID: "res-1", Type: "file"},
		Result:   AuditResult{Success: true},
	}

	logger.LogAsync(event)

	// Wait for async operation
	time.Sleep(100 * time.Millisecond)

	// Should be flushed due to buffer size
	events := storage.GetEvents()
	assert.Equal(t, 1, len(events))
}

func TestAuditLogger_Flush_EmptyBuffer(t *testing.T) {
	config := DefaultAuditConfig
	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	err := logger.Flush()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(storage.GetEvents()))
}

func TestAuditLogger_Flush_WriteError(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 10

	storage := newMockStorage()
	storage.writeErr = errors.New("write failed")

	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	event := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "test-action",
		Actor:    AuditActor{ID: "user-1", Type: "user"},
		Resource: AuditResource{ID: "res-1", Type: "file"},
		Result:   AuditResult{Success: true},
	}

	err := logger.Log(event)
	require.NoError(t, err)

	err = logger.Flush()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

func TestAuditLogger_Query(t *testing.T) {
	config := DefaultAuditConfig
	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	// Pre-populate storage
	event1 := &AuditEvent{
		Level:    AuditLevelInfo,
		Category: AuditCategorySystem,
		Action:   "action-1",
		Actor:    AuditActor{ID: "user-1", Type: "user"},
		Resource: AuditResource{ID: "res-1", Type: "file"},
		Result:   AuditResult{Success: true},
	}
	err := storage.Write(event1)
	require.NoError(t, err)

	filter := AuditFilter{
		Action: "action-1",
	}

	events, err := logger.Query(filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(events))
}

func TestAuditLogger_Query_Error(t *testing.T) {
	config := DefaultAuditConfig
	storage := newMockStorage()
	storage.queryErr = errors.New("query failed")

	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	events, err := logger.Query(AuditFilter{})
	assert.Error(t, err)
	assert.Nil(t, events)
}

func TestAuditLogger_Close(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 10

	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)

	// Log some events
	for i := 0; i < 3; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "test-action",
			Actor:    AuditActor{ID: "user-1", Type: "user"},
			Resource: AuditResource{ID: "res-1", Type: "file"},
			Result:   AuditResult{Success: true},
		}
		err := logger.Log(event)
		require.NoError(t, err)
	}

	// Close should flush remaining events
	err := logger.Close()
	assert.NoError(t, err)
	assert.True(t, storage.closed)
	assert.Equal(t, 3, len(storage.GetEvents()))
}

func TestAuditLogger_ConcurrentAccess(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 100

	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				event := &AuditEvent{
					Level:    AuditLevelInfo,
					Category: AuditCategorySystem,
					Action:   "concurrent-action",
					Actor:    AuditActor{ID: "user-1", Type: "user"},
					Resource: AuditResource{ID: "res-1", Type: "file"},
					Result:   AuditResult{Success: true},
				}
				err := logger.Log(event)
				require.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Flush all events
	err := logger.Flush()
	require.NoError(t, err)

	// Should have 100 events
	events := storage.GetEvents()
	assert.Equal(t, 100, len(events))
}

func TestAuditLogger_CalculateHash(t *testing.T) {
	config := DefaultAuditConfig
	storage := newMockStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		err := logger.Close()
		require.NoError(t, err)
	}()

	event := &AuditEvent{
		ID:        "test-id",
		Level:     AuditLevelInfo,
		Category:  AuditCategorySystem,
		Action:    "test-action",
		Actor:     AuditActor{ID: "user-1", Type: "user"},
		Resource:  AuditResource{ID: "res-1", Type: "file"},
		Result:    AuditResult{Success: true},
		Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	hash1 := logger.calculateHash(event)
	hash2 := logger.calculateHash(event)

	// Same input should produce same hash
	assert.Equal(t, hash1, hash2)
	assert.NotEmpty(t, hash1)
	assert.Equal(t, 64, len(hash1)) // SHA256 hex is 64 chars
}

func TestAuditFilter_Empty(t *testing.T) {
	filter := AuditFilter{}
	assert.Zero(t, filter.StartTime)
	assert.Zero(t, filter.EndTime)
	assert.Nil(t, filter.Categories)
	assert.Nil(t, filter.Levels)
	assert.Empty(t, filter.ActorID)
}
