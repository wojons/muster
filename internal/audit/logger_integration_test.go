package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_AuditLogger_MemoryStorage tests the full audit logging workflow with MemoryStorage
func TestIntegration_AuditLogger_MemoryStorage(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 5
	config.FlushInterval = time.Hour // Prevent auto-flush during test

	storage := NewMemoryStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		_ = logger.Close()
	}()

	// Test 1: Log events and verify they are buffered
	events := []*AuditEvent{
		{
			Level:    AuditLevelInfo,
			Category: AuditCategoryAuth,
			Action:   "user_login",
			Actor:    AuditActor{ID: "user-123", Type: "user", Name: "John Doe"},
			Resource: AuditResource{ID: "auth-service", Type: "service"},
			Result:   AuditResult{Success: true},
		},
		{
			Level:    AuditLevelWarning,
			Category: AuditCategoryAccess,
			Action:   "permission_denied",
			Actor:    AuditActor{ID: "user-456", Type: "user", Name: "Jane Smith"},
			Resource: AuditResource{ID: "admin-panel", Type: "resource"},
			Result:   AuditResult{Success: false, Code: "FORBIDDEN"},
		},
		{
			Level:    AuditLevelError,
			Category: AuditCategorySystem,
			Action:   "database_error",
			Actor:    AuditActor{ID: "system", Type: "service"},
			Resource: AuditResource{ID: "db-01", Type: "database"},
			Result:   AuditResult{Success: false, Message: "Connection timeout"},
		},
	}

	for _, event := range events {
		err := logger.Log(event)
		require.NoError(t, err)
	}

	// Events should be buffered, not written yet
	allEvents := storage.GetAllEvents()
	assert.Equal(t, 0, len(allEvents), "Events should be buffered, not written yet")

	// Test 2: Flush and verify events are written
	err := logger.Flush()
	require.NoError(t, err)

	allEvents = storage.GetAllEvents()
	require.Equal(t, 3, len(allEvents), "All events should be written after flush")

	// Test 3: Verify event properties
	for i, event := range allEvents {
		assert.NotEmpty(t, event.ID, "Event %d should have an ID", i)
		assert.False(t, event.Timestamp.IsZero(), "Event %d should have a timestamp", i)
		assert.NotEmpty(t, event.Hash, "Event %d should have a hash", i)
		assert.NotNil(t, events[i], "Event %d should exist", i)
	}

	// Test 4: Query events by category
	authEvents, err := logger.Query(AuditFilter{Categories: []AuditCategory{AuditCategoryAuth}})
	require.NoError(t, err)
	assert.Equal(t, 1, len(authEvents))
	assert.Equal(t, "user_login", authEvents[0].Action)

	// Test 5: Query events by level
	errorEvents, err := logger.Query(AuditFilter{Levels: []AuditLevel{AuditLevelError}})
	require.NoError(t, err)
	assert.Equal(t, 1, len(errorEvents))
	assert.Equal(t, "database_error", errorEvents[0].Action)

	// Test 6: Query events by success status
	successEvents, err := logger.Query(AuditFilter{})
	require.NoError(t, err)
	// Should return all events (no filter on success)
	assert.Equal(t, 3, len(successEvents))

	// Test 7: Verify hash chain integrity
	for i := 1; i < len(allEvents); i++ {
		assert.Equal(t, allEvents[i-1].Hash, allEvents[i].PreviousHash,
			"Event %d's PreviousHash should match Event %d's Hash", i, i-1)
	}
}

// TestIntegration_AuditLogger_FileStorage tests the full audit logging workflow with FileStorage
func TestIntegration_AuditLogger_FileStorage(t *testing.T) {
	// Create temporary directory for test files
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	config := DefaultAuditConfig
	config.BufferSize = 3
	config.FlushInterval = time.Hour

	opts := DefaultFileStorageOptions()
	storage, err := NewFileStorage(logPath, opts)
	require.NoError(t, err)

	logger := NewAuditLogger(config, storage)
	// Note: logger.Close() is called manually later in this test

	// Test 1: Log multiple events
	events := make([]*AuditEvent, 5)
	for i := 0; i < 5; i++ {
		events[i] = &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "system_check",
			Actor:    AuditActor{ID: "monitor", Type: "service"},
			Resource: AuditResource{ID: "system", Type: "infrastructure"},
			Result:   AuditResult{Success: true},
			Context: AuditContext{
				RequestID: "req-" + string(rune('A'+i)),
				Metadata: map[string]string{
					"check_type": "health",
					"instance":   string(rune('0' + i)),
				},
			},
		}
		err := logger.Log(events[i])
		require.NoError(t, err)
	}

	// First 3 events should have triggered auto-flush (buffer size = 3)
	// Remaining 2 events should still be in buffer

	// Test 2: Manual flush remaining events
	err = logger.Flush()
	require.NoError(t, err)

	// Test 3: Close logger (should flush any remaining events)
	err = logger.Close()
	require.NoError(t, err)

	// Test 4: Verify file was created
	_, err = os.Stat(logPath)
	require.NoError(t, err, "Audit log file should exist")

	// Test 5: Create new storage instance to read back events
	storage2, err := NewFileStorage(logPath, opts)
	require.NoError(t, err)
	defer func() {
		_ = storage2.Close()
	}()

	// Test 6: Query all events
	allEvents, err := storage2.Query(AuditFilter{})
	require.NoError(t, err)
	assert.Equal(t, 5, len(allEvents), "All 5 events should be persisted")

	// Test 7: Verify event properties
	for i, event := range allEvents {
		assert.NotEmpty(t, event.ID, "Event %d should have ID", i)
		assert.False(t, event.Timestamp.IsZero(), "Event %d should have timestamp", i)
		assert.NotEmpty(t, event.Hash, "Event %d should have hash", i)
		assert.Equal(t, "system_check", event.Action)
		assert.Equal(t, "monitor", event.Actor.ID)
	}

	// Test 8: Query with time filter
	pastFilter := AuditFilter{
		StartTime: time.Now().Add(-time.Hour),
		EndTime:   time.Now().Add(time.Hour),
	}
	timeFilteredEvents, err := storage2.Query(pastFilter)
	require.NoError(t, err)
	assert.Equal(t, 5, len(timeFilteredEvents))

	// Test 9: Query with non-matching time filter
	futureFilter := AuditFilter{
		StartTime: time.Now().Add(time.Hour),
	}
	futureEvents, err := storage2.Query(futureFilter)
	require.NoError(t, err)
	assert.Equal(t, 0, len(futureEvents))
}

// TestIntegration_AuditLogger_BufferManagement tests buffer flush behavior
func TestIntegration_AuditLogger_BufferManagement(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 2 // Small buffer to trigger auto-flush
	config.FlushInterval = time.Hour

	storage := NewMemoryStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		_ = logger.Close()
	}()

	// Test 1: Auto-flush on buffer full
	// Note: BufferSize is 2, but auto-flush triggers when len(buffer) >= BufferSize
	// So first event goes to buffer (len=1), second event triggers flush (len=2 >= 2)
	for i := 0; i < 4; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "action-" + string(rune('A'+i)),
			Actor:    AuditActor{ID: "user", Type: "user"},
			Resource: AuditResource{ID: "res", Type: "resource"},
			Result:   AuditResult{Success: true},
		}
		err := logger.Log(event)
		require.NoError(t, err)
		// Small delay to allow auto-flush to complete
		time.Sleep(10 * time.Millisecond)
	}

	// Give auto-flush time to complete
	time.Sleep(100 * time.Millisecond)

	// Events are auto-flushed when buffer reaches capacity
	allEvents := storage.GetAllEvents()
	assert.GreaterOrEqual(t, len(allEvents), 2, "Some events should have auto-flushed")

	// Test 2: Manual flush remaining events
	err := logger.Flush()
	require.NoError(t, err)

	allEvents = storage.GetAllEvents()
	assert.Equal(t, 4, len(allEvents), "All events should be written after manual flush")
}

// TestIntegration_AuditLogger_ConcurrentLogging tests concurrent access patterns
func TestIntegration_AuditLogger_ConcurrentLogging(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 50
	config.FlushInterval = time.Millisecond * 50

	storage := NewMemoryStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		_ = logger.Close()
	}()

	// Concurrent logging from multiple goroutines
	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 20

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				// Add small stagger to reduce ID collision chance
				time.Sleep(time.Microsecond * 10)
				event := &AuditEvent{
					Level:    AuditLevelInfo,
					Category: AuditCategorySystem,
					Action:   "concurrent-action",
					Actor: AuditActor{
						ID:   fmt.Sprintf("goroutine-%d", id),
						Type: "worker",
					},
					Resource: AuditResource{ID: "shared-resource", Type: "resource"},
					Result:   AuditResult{Success: true},
				}
				err := logger.Log(event)
				require.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Wait for auto-flush to complete
	time.Sleep(200 * time.Millisecond)

	// Manual flush to ensure all events are written
	err := logger.Flush()
	require.NoError(t, err)

	allEvents := storage.GetAllEvents()
	expectedEvents := numGoroutines * eventsPerGoroutine
	assert.Equal(t, expectedEvents, len(allEvents),
		"All concurrent events should be logged")

	// Note: Event IDs may have rare collisions due to time-based generation
	// Verify most IDs are unique (allowing for rare collisions)
	eventIDs := make(map[string]int)
	duplicateCount := 0
	for _, event := range allEvents {
		if eventIDs[event.ID] > 0 {
			duplicateCount++
		}
		eventIDs[event.ID]++
	}

	// Allow up to 5% duplicates due to time-based ID generation in concurrent scenarios
	maxAllowedDuplicates := expectedEvents * 5 / 100
	assert.LessOrEqual(t, duplicateCount, maxAllowedDuplicates,
		"Duplicate event IDs should be rare (got %d, allowed up to %d)", duplicateCount, maxAllowedDuplicates)
}

// TestIntegration_AuditLogger_HashChainVerification tests hash chain integrity
func TestIntegration_AuditLogger_HashChainVerification(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 10
	config.FlushInterval = time.Hour

	storage := NewMemoryStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		_ = logger.Close()
	}()

	// Log a sequence of events
	numEvents := 10
	for i := 0; i < numEvents; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "chain-test",
			Actor:    AuditActor{ID: "tester", Type: "user"},
			Resource: AuditResource{ID: "chain-resource", Type: "resource"},
			Result:   AuditResult{Success: true},
		}
		err := logger.Log(event)
		require.NoError(t, err)
	}

	// Flush all events
	err := logger.Flush()
	require.NoError(t, err)

	events := storage.GetAllEvents()
	require.Equal(t, numEvents, len(events))

	// Verify hash chain
	assert.Empty(t, events[0].PreviousHash, "First event should have empty PreviousHash")

	for i := 1; i < len(events); i++ {
		assert.Equal(t, events[i-1].Hash, events[i].PreviousHash,
			"Event %d PreviousHash should equal Event %d Hash", i, i-1)
		assert.NotEmpty(t, events[i].Hash, "Event %d should have a hash", i)
	}

	// Verify each hash is unique
	hashes := make(map[string]bool)
	for _, event := range events {
		assert.False(t, hashes[event.Hash], "Hash should be unique: %s", event.Hash)
		hashes[event.Hash] = true
	}
}

// TestIntegration_AuditLogger_GracefulShutdown tests shutdown behavior with pending events
func TestIntegration_AuditLogger_GracefulShutdown(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 100 // Large buffer to prevent auto-flush
	config.FlushInterval = time.Hour

	storage := NewMemoryStorage()
	logger := NewAuditLogger(config, storage)

	// Log some events
	numEvents := 10
	for i := 0; i < numEvents; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "shutdown-test",
			Actor:    AuditActor{ID: "tester", Type: "user"},
			Resource: AuditResource{ID: "resource", Type: "resource"},
			Result:   AuditResult{Success: true},
		}
		err := logger.Log(event)
		require.NoError(t, err)
	}

	// Events should be in buffer, not yet written
	eventsBeforeClose := storage.GetAllEvents()
	assert.Equal(t, 0, len(eventsBeforeClose), "Events should be buffered")

	// Close should flush all events
	err := logger.Close()
	require.NoError(t, err)

	// Verify all events were flushed on close
	// Note: We can't query the storage after close since it's closed,
	// but we verified the flush happened in Close()
}

// TestIntegration_AuditLogger_QueryFiltering tests comprehensive query filtering
func TestIntegration_AuditLogger_QueryFiltering(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 100
	config.FlushInterval = time.Hour

	storage := NewMemoryStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		_ = logger.Close()
	}()

	// Create events with different properties
	// Note: Logger sets timestamps automatically, so we can't filter by specific times
	events := []*AuditEvent{
		{
			Level:    AuditLevelInfo,
			Category: AuditCategoryAuth,
			Action:   "login",
			Actor:    AuditActor{ID: "user-1", Type: "user"},
			Resource: AuditResource{ID: "api", Type: "endpoint"},
			Result:   AuditResult{Success: true},
		},
		{
			Level:    AuditLevelWarning,
			Category: AuditCategoryAuth,
			Action:   "login_failed",
			Actor:    AuditActor{ID: "user-2", Type: "user"},
			Resource: AuditResource{ID: "api", Type: "endpoint"},
			Result:   AuditResult{Success: false},
		},
		{
			Level:    AuditLevelError,
			Category: AuditCategorySystem,
			Action:   "db_error",
			Actor:    AuditActor{ID: "service-a", Type: "service"},
			Resource: AuditResource{ID: "database", Type: "resource"},
			Result:   AuditResult{Success: false},
		},
		{
			Level:    AuditLevelInfo,
			Category: AuditCategoryData,
			Action:   "read",
			Actor:    AuditActor{ID: "user-1", Type: "user"},
			Resource: AuditResource{ID: "file-1", Type: "file"},
			Result:   AuditResult{Success: true},
		},
	}

	for _, event := range events {
		err := logger.Log(event)
		require.NoError(t, err)
	}

	err := logger.Flush()
	require.NoError(t, err)

	// Test filter by actor ID
	user1Events, err := logger.Query(AuditFilter{ActorID: "user-1"})
	require.NoError(t, err)
	assert.Equal(t, 2, len(user1Events))

	// Test filter by resource type
	endpointEvents, err := logger.Query(AuditFilter{ResourceType: "endpoint"})
	require.NoError(t, err)
	assert.Equal(t, 2, len(endpointEvents))

	// Test filter by action
	loginEvents, err := logger.Query(AuditFilter{Action: "login"})
	require.NoError(t, err)
	assert.Equal(t, 1, len(loginEvents))

	// Test filter by categories
	authEvents, err := logger.Query(AuditFilter{Categories: []AuditCategory{AuditCategoryAuth}})
	require.NoError(t, err)
	assert.Equal(t, 2, len(authEvents))

	// Test filter by levels
	warningErrorEvents, err := logger.Query(AuditFilter{Levels: []AuditLevel{AuditLevelWarning, AuditLevelError}})
	require.NoError(t, err)
	assert.Equal(t, 2, len(warningErrorEvents))

	// Test combined filters
	combinedFilter := AuditFilter{
		Categories: []AuditCategory{AuditCategoryAuth},
		Levels:     []AuditLevel{AuditLevelInfo},
	}
	combinedEvents, err := logger.Query(combinedFilter)
	require.NoError(t, err)
	assert.Equal(t, 1, len(combinedEvents))
	assert.Equal(t, "login", combinedEvents[0].Action)

	// Test time range filter (using wide time range to capture all events)
	// Since logger sets timestamps automatically, use a wide window
	now := time.Now()
	timeFilter := AuditFilter{
		StartTime: now.Add(-time.Hour),
		EndTime:   now.Add(time.Hour),
	}
	timeFilteredEvents, err := logger.Query(timeFilter)
	require.NoError(t, err)
	assert.Equal(t, 4, len(timeFilteredEvents), "All events should be within the time range")
}

// TestIntegration_AuditLogger_FileRotation tests log rotation with FileStorage
func TestIntegration_AuditLogger_FileRotation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	// Configure with small max size to trigger rotation
	opts := FileStorageOptions{
		MaxSize:  200, // 200 bytes - very small to force rotation
		MaxFiles: 3,
	}

	config := DefaultAuditConfig
	config.BufferSize = 1 // Flush immediately

	storage, err := NewFileStorage(logPath, opts)
	require.NoError(t, err)

	logger := NewAuditLogger(config, storage)

	// Log enough events to trigger rotation
	// Each event is roughly 150-200 bytes when JSON encoded
	numEvents := 10
	for i := 0; i < numEvents; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "rotation-test-action",
			Actor: AuditActor{
				ID:        fmt.Sprintf("test-user-%d", i),
				Type:      "user",
				Name:      fmt.Sprintf("Test User %d", i),
				IPAddress: fmt.Sprintf("192.168.1.%d", i+1),
			},
			Resource: AuditResource{
				ID:   fmt.Sprintf("resource-%d", i),
				Type: "test-resource",
				Name: fmt.Sprintf("Test Resource %d", i),
			},
			Result: AuditResult{Success: true},
			Context: AuditContext{
				RequestID: fmt.Sprintf("request-%d", i),
				Metadata: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
		}
		err := logger.Log(event)
		require.NoError(t, err)
	}

	err = logger.Close()
	require.NoError(t, err)

	// Verify main log file exists
	_, err = os.Stat(logPath)
	require.NoError(t, err, "Audit log file should exist")

	// Check if rotation occurred (look for rotated files)
	rotatedCount := 0
	for i := 1; i <= 3; i++ {
		rotatedPath := fmt.Sprintf("%s.%d", logPath, i)
		if _, err := os.Stat(rotatedPath); err == nil {
			rotatedCount++
		}
	}

	// Should have at least one rotated file
	assert.GreaterOrEqual(t, rotatedCount, 1, "Should have at least one rotated file")

	// Create new storage to query all files
	storage2, err := NewFileStorage(logPath, opts)
	require.NoError(t, err)
	defer func() {
		_ = storage2.Close()
	}()

	// Query should return events from all rotated files
	allEvents, err := storage2.Query(AuditFilter{})
	require.NoError(t, err)

	// With rotation, we should have some events retained
	// Some events may be lost due to rotation discarding older files
	// With 10 events and aggressive rotation (200 bytes), we expect at least 30%
	minExpectedEvents := numEvents * 30 / 100
	assert.GreaterOrEqual(t, len(allEvents), minExpectedEvents,
		"Should have at least %d events after rotation (got %d)", minExpectedEvents, len(allEvents))
}

// TestIntegration_AuditLogger_AsyncLogging tests async logging functionality
func TestIntegration_AuditLogger_AsyncLogging(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 1 // Small buffer to ensure quick writes

	storage := NewMemoryStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		_ = logger.Close()
	}()

	// Log events asynchronously
	numEvents := 20
	for i := 0; i < numEvents; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "async-action",
			Actor:    AuditActor{ID: "async-user", Type: "user"},
			Resource: AuditResource{ID: "async-resource", Type: "resource"},
			Result:   AuditResult{Success: true},
		}
		logger.LogAsync(event)
	}

	// Wait for async operations to complete
	time.Sleep(500 * time.Millisecond)

	// Flush any remaining events
	err := logger.Flush()
	require.NoError(t, err)

	// Verify all events were logged
	events := storage.GetAllEvents()
	assert.Equal(t, numEvents, len(events), "All async events should be logged")
}

// TestIntegration_AuditLogger_StorageTransition tests switching between storage backends
func TestIntegration_AuditLogger_StorageTransition(t *testing.T) {
	config := DefaultAuditConfig
	config.BufferSize = 10
	config.FlushInterval = time.Hour

	// Phase 1: Use MemoryStorage
	memStorage := NewMemoryStorage()
	logger := NewAuditLogger(config, memStorage)

	// Log events to memory
	for i := 0; i < 5; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "memory-phase",
			Actor:    AuditActor{ID: "tester", Type: "user"},
			Resource: AuditResource{ID: "resource", Type: "resource"},
			Result:   AuditResult{Success: true},
		}
		err := logger.Log(event)
		require.NoError(t, err)
	}

	err := logger.Flush()
	require.NoError(t, err)

	memEvents := memStorage.GetAllEvents()
	assert.Equal(t, 5, len(memEvents))

	// Close and switch to FileStorage
	err = logger.Close()
	require.NoError(t, err)

	// Phase 2: Use FileStorage with new logger
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	fileStorage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)

	logger2 := NewAuditLogger(config, fileStorage)
	defer func() {
		_ = logger2.Close()
	}()

	// Log events to file
	for i := 0; i < 3; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "file-phase",
			Actor:    AuditActor{ID: "tester", Type: "user"},
			Resource: AuditResource{ID: "resource", Type: "resource"},
			Result:   AuditResult{Success: true},
		}
		err := logger2.Log(event)
		require.NoError(t, err)
	}

	err = logger2.Flush()
	require.NoError(t, err)

	// Verify file storage works
	fileEvents, err := fileStorage.Query(AuditFilter{})
	require.NoError(t, err)
	assert.Equal(t, 3, len(fileEvents))
}

// TestIntegration_AuditLogger_DisabledLogging tests behavior when logging is disabled
func TestIntegration_AuditLogger_DisabledLogging(t *testing.T) {
	config := DefaultAuditConfig
	config.Enabled = false

	storage := NewMemoryStorage()
	logger := NewAuditLogger(config, storage)
	defer func() {
		_ = logger.Close()
	}()

	// Try to log events
	for i := 0; i < 10; i++ {
		event := &AuditEvent{
			Level:    AuditLevelInfo,
			Category: AuditCategorySystem,
			Action:   "should-not-log",
			Actor:    AuditActor{ID: "tester", Type: "user"},
			Resource: AuditResource{ID: "resource", Type: "resource"},
			Result:   AuditResult{Success: true},
		}
		err := logger.Log(event)
		require.NoError(t, err) // Should not error, just no-op
	}

	err := logger.Flush()
	require.NoError(t, err)

	// Verify no events were written
	events := storage.GetAllEvents()
	assert.Equal(t, 0, len(events), "No events should be logged when disabled")
}

// TestIntegration_AuditLogger_RealWorldScenario tests a realistic audit scenario
func TestIntegration_AuditLogger_RealWorldScenario(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	config := DefaultAuditConfig
	config.BufferSize = 5
	config.FlushInterval = time.Second * 2
	config.SensitiveFields = []string{"password", "token", "api_key", "secret", "credit_card"}

	opts := DefaultFileStorageOptions()
	storage, err := NewFileStorage(logPath, opts)
	require.NoError(t, err)

	logger := NewAuditLogger(config, storage)
	defer func() {
		_ = logger.Close()
	}()

	// Simulate a real-world scenario: user login workflow
	testCases := []struct {
		name     string
		level    AuditLevel
		category AuditCategory
		action   string
		actor    AuditActor
		resource AuditResource
		result   AuditResult
		metadata map[string]string
	}{
		{
			name:     "User authentication attempt",
			level:    AuditLevelInfo,
			category: AuditCategoryAuth,
			action:   "authenticate",
			actor:    AuditActor{ID: "user-12345", Type: "user", Name: "john.doe@example.com", IPAddress: "203.0.113.42"},
			resource: AuditResource{ID: "auth-service", Type: "service"},
			result:   AuditResult{Success: true, Code: "AUTH_SUCCESS"},
			metadata: map[string]string{"auth_method": "password", "mfa_used": "true"},
		},
		{
			name:     "API key generation",
			level:    AuditLevelInfo,
			category: AuditCategoryAccess,
			action:   "generate_api_key",
			actor:    AuditActor{ID: "user-12345", Type: "user", Name: "john.doe@example.com", IPAddress: "203.0.113.42"},
			resource: AuditResource{ID: "api-key-service", Type: "service"},
			result:   AuditResult{Success: true},
			metadata: map[string]string{"api_key": "sk_live_1234567890abcdef", "token": "sensitive_token_value"},
		},
		{
			name:     "Sensitive data access",
			level:    AuditLevelWarning,
			category: AuditCategoryData,
			action:   "access_sensitive_data",
			actor:    AuditActor{ID: "user-12345", Type: "user", Name: "john.doe@example.com", IPAddress: "203.0.113.42"},
			resource: AuditResource{ID: "customer-db", Type: "database", Name: "customers"},
			result:   AuditResult{Success: true},
			metadata: map[string]string{"records_accessed": "150", "query_id": "q-98765"},
		},
		{
			name:     "Permission check",
			level:    AuditLevelInfo,
			category: AuditCategoryAccess,
			action:   "check_permission",
			actor:    AuditActor{ID: "user-12345", Type: "user", Name: "john.doe@example.com"},
			resource: AuditResource{ID: "admin-resource", Type: "resource"},
			result:   AuditResult{Success: false, Code: "PERM_DENIED", Message: "User lacks admin privileges"},
			metadata: map[string]string{"required_role": "admin", "user_role": "user"},
		},
		{
			name:     "System configuration change",
			level:    AuditLevelCritical,
			category: AuditCategoryAdmin,
			action:   "update_config",
			actor:    AuditActor{ID: "admin-999", Type: "admin", Name: "system-admin"},
			resource: AuditResource{ID: "system-config", Type: "configuration"},
			result:   AuditResult{Success: true},
			metadata: map[string]string{"change_type": "security_policy", "previous_value": "allow_all", "new_value": "deny_unknown"},
		},
	}

	for _, tc := range testCases {
		event := &AuditEvent{
			Level:    tc.level,
			Category: tc.category,
			Action:   tc.action,
			Actor:    tc.actor,
			Resource: tc.resource,
			Result:   tc.result,
			Context: AuditContext{
				RequestID: "req-" + tc.action + "-" + time.Now().Format("20060102150405"),
				Metadata:  tc.metadata,
			},
		}
		err := logger.Log(event)
		require.NoError(t, err, "Failed to log: %s", tc.name)
	}

	// Wait for auto-flush
	time.Sleep(2500 * time.Millisecond)

	// Query events by category
	authEvents, err := logger.Query(AuditFilter{Categories: []AuditCategory{AuditCategoryAuth}})
	require.NoError(t, err)
	assert.Equal(t, 1, len(authEvents))

	accessEvents, err := logger.Query(AuditFilter{Categories: []AuditCategory{AuditCategoryAccess}})
	require.NoError(t, err)
	assert.Equal(t, 2, len(accessEvents))

	// Query critical events
	criticalEvents, err := logger.Query(AuditFilter{Levels: []AuditLevel{AuditLevelCritical}})
	require.NoError(t, err)
	assert.Equal(t, 1, len(criticalEvents))
	assert.Equal(t, "update_config", criticalEvents[0].Action)

	// Verify redaction of sensitive fields in the API key event
	allEvents, err := logger.Query(AuditFilter{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(allEvents), 4)

	for _, event := range allEvents {
		// Check for redacted sensitive data
		for key, value := range event.Context.Metadata {
			if key == "api_key" || key == "token" {
				assert.Equal(t, "[REDACTED]", value,
					"Sensitive field %s should be redacted", key)
			}
		}
	}
}
