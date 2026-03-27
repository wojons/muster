package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemoryStorage_Write tests writing events to memory storage.
func TestMemoryStorage_Write(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	event := createTestEvent("test-action", AuditLevelInfo, AuditCategorySystem)

	err := storage.Write(event)
	require.NoError(t, err)

	events := storage.GetAllEvents()
	require.Len(t, events, 1)
	assert.Equal(t, event.Action, events[0].Action)
	assert.Equal(t, event.ID, events[0].ID)
}

// TestMemoryStorage_Write_Closed tests writing to closed storage fails.
func TestMemoryStorage_Write_Closed(t *testing.T) {
	storage := NewMemoryStorage()
	_ = storage.Close()

	event := createTestEvent("test-action", AuditLevelInfo, AuditCategorySystem)
	err := storage.Write(event)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestMemoryStorage_Query_TimeRange tests querying by time range.
func TestMemoryStorage_Query_TimeRange(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	now := time.Now()

	// Add events at different times
	oldEvent := createTestEvent("old-action", AuditLevelInfo, AuditCategorySystem)
	oldEvent.Timestamp = now.Add(-2 * time.Hour)
	require.NoError(t, storage.Write(oldEvent))

	newEvent := createTestEvent("new-action", AuditLevelInfo, AuditCategorySystem)
	newEvent.Timestamp = now
	require.NoError(t, storage.Write(newEvent))

	// Query for recent events only
	filter := AuditFilter{
		StartTime: now.Add(-30 * time.Minute),
		EndTime:   now.Add(30 * time.Minute),
	}

	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "new-action", results[0].Action)
}

// TestMemoryStorage_Query_Category tests querying by category.
func TestMemoryStorage_Query_Category(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	authEvent := createTestEvent("login", AuditLevelInfo, AuditCategoryAuth)
	require.NoError(t, storage.Write(authEvent))

	systemEvent := createTestEvent("restart", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, storage.Write(systemEvent))

	filter := AuditFilter{
		Categories: []AuditCategory{AuditCategoryAuth},
	}

	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, AuditCategoryAuth, results[0].Category)
}

// TestMemoryStorage_Query_Level tests querying by level.
func TestMemoryStorage_Query_Level(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	infoEvent := createTestEvent("info-action", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, storage.Write(infoEvent))

	errorEvent := createTestEvent("error-action", AuditLevelError, AuditCategorySystem)
	require.NoError(t, storage.Write(errorEvent))

	filter := AuditFilter{
		Levels: []AuditLevel{AuditLevelError},
	}

	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, AuditLevelError, results[0].Level)
}

// TestMemoryStorage_Query_ActorID tests querying by actor ID.
func TestMemoryStorage_Query_ActorID(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	event1 := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
	event1.Actor.ID = "actor-1"
	require.NoError(t, storage.Write(event1))

	event2 := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
	event2.Actor.ID = "actor-2"
	require.NoError(t, storage.Write(event2))

	filter := AuditFilter{
		ActorID: "actor-1",
	}

	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "actor-1", results[0].Actor.ID)
}

// TestMemoryStorage_Query_ResourceType tests querying by resource type.
func TestMemoryStorage_Query_ResourceType(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	event1 := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
	event1.Resource.Type = "user"
	require.NoError(t, storage.Write(event1))

	event2 := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
	event2.Resource.Type = "file"
	require.NoError(t, storage.Write(event2))

	filter := AuditFilter{
		ResourceType: "user",
	}

	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "user", results[0].Resource.Type)
}

// TestMemoryStorage_Query_Action tests querying by action.
func TestMemoryStorage_Query_Action(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	createEvent := createTestEvent("create", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, storage.Write(createEvent))

	deleteEvent := createTestEvent("delete", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, storage.Write(deleteEvent))

	filter := AuditFilter{
		Action: "create",
	}

	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "create", results[0].Action)
}

// TestMemoryStorage_Query_Success tests querying by success status.
func TestMemoryStorage_Query_Success(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	successEvent := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
	successEvent.Result.Success = true
	require.NoError(t, storage.Write(successEvent))

	failureEvent := createTestEvent("action", AuditLevelError, AuditCategorySystem)
	failureEvent.Result.Success = false
	require.NoError(t, storage.Write(failureEvent))

	success := true
	filter := AuditFilter{
		Success: &success,
	}

	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Result.Success)
}

// TestMemoryStorage_Query_CombinedFilters tests querying with multiple filters.
func TestMemoryStorage_Query_CombinedFilters(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	now := time.Now()

	// Add multiple events
	event1 := createTestEvent("login", AuditLevelInfo, AuditCategoryAuth)
	event1.Timestamp = now
	event1.Actor.ID = "user-1"
	event1.Result.Success = true
	require.NoError(t, storage.Write(event1))

	event2 := createTestEvent("login", AuditLevelError, AuditCategoryAuth)
	event2.Timestamp = now
	event2.Actor.ID = "user-2"
	event2.Result.Success = false
	require.NoError(t, storage.Write(event2))

	event3 := createTestEvent("logout", AuditLevelInfo, AuditCategoryAuth)
	event3.Timestamp = now
	event3.Actor.ID = "user-1"
	event3.Result.Success = true
	require.NoError(t, storage.Write(event3))

	// Query with combined filters
	success := true
	filter := AuditFilter{
		StartTime:  now.Add(-time.Hour),
		EndTime:    now.Add(time.Hour),
		Categories: []AuditCategory{AuditCategoryAuth},
		Levels:     []AuditLevel{AuditLevelInfo},
		ActorID:    "user-1",
		Action:     "login",
		Success:    &success,
	}

	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "login", results[0].Action)
	assert.Equal(t, "user-1", results[0].Actor.ID)
}

// TestMemoryStorage_Query_EmptyStorage tests querying empty storage.
func TestMemoryStorage_Query_EmptyStorage(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	results, err := storage.Query(AuditFilter{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestMemoryStorage_Query_Closed tests querying closed storage fails.
func TestMemoryStorage_Query_Closed(t *testing.T) {
	storage := NewMemoryStorage()
	_ = storage.Close()

	_, err := storage.Query(AuditFilter{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestMemoryStorage_Close tests closing storage clears events.
func TestMemoryStorage_Close(t *testing.T) {
	storage := NewMemoryStorage()

	event := createTestEvent("test", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, storage.Write(event))

	err := storage.Close()
	require.NoError(t, err)

	events := storage.GetAllEvents()
	assert.Empty(t, events)
}

// TestMemoryStorage_ConcurrentAccess tests concurrent write and read operations.
func TestMemoryStorage_ConcurrentAccess(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	done := make(chan bool)

	// Concurrent writes
	for i := 0; i < 10; i++ {
		go func(idx int) {
			event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
			event.ID = string(rune('a' + idx))
			_ = storage.Write(event)
			done <- true
		}(i)
	}

	// Wait for all writes
	for i := 0; i < 10; i++ {
		<-done
	}

	events := storage.GetAllEvents()
	assert.Len(t, events, 10)
}

// TestFileStorage_Write tests writing events to file storage.
func TestFileStorage_Write(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	event := createTestEvent("test-action", AuditLevelInfo, AuditCategorySystem)
	err = storage.Write(event)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(logPath)
	require.NoError(t, err)
}

// TestFileStorage_Write_Closed tests writing to closed storage fails.
func TestFileStorage_Write_Closed(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	_ = storage.Close()

	event := createTestEvent("test", AuditLevelInfo, AuditCategorySystem)
	err = storage.Write(event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestFileStorage_Query tests querying file storage.
func TestFileStorage_Query(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	// Write events
	for i := 0; i < 5; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = string(rune('a' + i))
		err := storage.Write(event)
		require.NoError(t, err)
	}

	// Query all events
	results, err := storage.Query(AuditFilter{})
	require.NoError(t, err)
	assert.Len(t, results, 5)
}

// TestFileStorage_Query_WithFilter tests querying with filters.
func TestFileStorage_Query_WithFilter(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	// Write different types of events
	authEvent := createTestEvent("login", AuditLevelInfo, AuditCategoryAuth)
	authEvent.Actor.ID = "user-1"
	require.NoError(t, storage.Write(authEvent))

	systemEvent := createTestEvent("restart", AuditLevelInfo, AuditCategorySystem)
	systemEvent.Actor.ID = "user-2"
	require.NoError(t, storage.Write(systemEvent))

	// Query for auth events only
	filter := AuditFilter{
		Categories: []AuditCategory{AuditCategoryAuth},
	}

	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, AuditCategoryAuth, results[0].Category)
}

// TestFileStorage_Rotation tests automatic log rotation.
func TestFileStorage_Rotation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	opts := FileStorageOptions{
		MaxSize:  100, // Very small for testing
		MaxFiles: 3,
	}

	storage, err := NewFileStorage(logPath, opts)
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	// Write enough data to trigger rotation
	for i := 0; i < 20; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.Context.Metadata = map[string]string{
			"data": "this is some padding data to make the file larger",
		}
		err := storage.Write(event)
		require.NoError(t, err)
	}

	// Check that rotation occurred
	_, err = os.Stat(logPath + ".1")
	assert.NoError(t, err, "rotated file .1 should exist")
}

// TestFileStorage_Rotate_Manual tests manual rotation.
func TestFileStorage_Rotate_Manual(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	event := createTestEvent("test", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, storage.Write(event))

	err = storage.Rotate()
	require.NoError(t, err)

	// Check rotated file exists
	_, err = os.Stat(logPath + ".1")
	assert.NoError(t, err)

	// Check new file is empty
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, int64(0), info.Size())
}

// TestFileStorage_Rotate_Closed tests rotation on closed storage fails.
func TestFileStorage_Rotate_Closed(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	_ = storage.Close()

	err = storage.Rotate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

// TestFileStorage_Close tests proper cleanup.
func TestFileStorage_Close(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)

	event := createTestEvent("test", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, storage.Write(event))

	err = storage.Close()
	require.NoError(t, err)

	// Verify file exists with content
	content, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

// TestFileStorage_Query_NonExistentFile tests querying non-existent file.
func TestFileStorage_Query_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "nonexistent", "audit.log")

	// Create storage (should create parent directory)
	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	// Query should return empty results
	results, err := storage.Query(AuditFilter{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestFileStorage_MultipleRotatedFiles tests querying across rotated files.
func TestFileStorage_MultipleRotatedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	// Write and rotate multiple times
	for i := 0; i < 3; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = string(rune('a' + i))
		err := storage.Write(event)
		require.NoError(t, err)

		err = storage.Rotate()
		require.NoError(t, err)
	}

	// Query should return events from all files
	results, err := storage.Query(AuditFilter{})
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

// TestFileStorage_CreateDirectory tests automatic directory creation.
func TestFileStorage_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "deeply", "nested", "path", "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	event := createTestEvent("test", AuditLevelInfo, AuditCategorySystem)
	err = storage.Write(event)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(logPath)
	require.NoError(t, err)
}

// TestFileStorage_Permissions tests file permissions.
func TestFileStorage_Permissions(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	// Check file permissions (should be 0600)
	info, err := os.Stat(logPath)
	require.NoError(t, err)
	mode := info.Mode().Perm()
	assert.Equal(t, os.FileMode(0600), mode, "File should have 0600 permissions")
}

// TestMemoryStorage_Query_Pagination_Basic tests basic pagination.
func TestMemoryStorage_Query_Pagination_Basic(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	// Add 10 events
	for i := 0; i < 10; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = string(rune('a' + i))
		require.NoError(t, storage.Write(event))
	}

	// Test page 1 with page size 3
	filter := AuditFilter{
		Page:     1,
		PageSize: 3,
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "a", results[0].ID)
	assert.Equal(t, "c", results[2].ID)

	// Test page 2
	filter.Page = 2
	results, err = storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "d", results[0].ID)
	assert.Equal(t, "f", results[2].ID)

	// Test page 4 (last partial page)
	filter.Page = 4
	results, err = storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "j", results[0].ID)
}

// TestMemoryStorage_Query_Pagination_EmptyPage tests pagination beyond available data.
func TestMemoryStorage_Query_Pagination_EmptyPage(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	// Add 5 events
	for i := 0; i < 5; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		require.NoError(t, storage.Write(event))
	}

	// Test page beyond available data
	filter := AuditFilter{
		Page:     10,
		PageSize: 5,
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestMemoryStorage_Query_Pagination_DefaultPage tests default page handling.
func TestMemoryStorage_Query_Pagination_DefaultPage(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	// Add 5 events
	for i := 0; i < 5; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		require.NoError(t, storage.Write(event))
	}

	// Test with page size but no page (should default to 1)
	filter := AuditFilter{
		PageSize: 3,
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)
}

// TestMemoryStorage_Query_Pagination_NoLimit tests no pagination when PageSize is 0.
func TestMemoryStorage_Query_Pagination_NoLimit(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	// Add 10 events
	for i := 0; i < 10; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		require.NoError(t, storage.Write(event))
	}

	// Test with PageSize = 0 (no pagination)
	filter := AuditFilter{
		Page:     1,
		PageSize: 0,
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 10)
}

// TestMemoryStorage_Query_Sort_Ascending tests ascending sort order.
func TestMemoryStorage_Query_Sort_Ascending(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	now := time.Now()
	// Add events in reverse chronological order
	for i := 4; i >= 0; i-- {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = string(rune('a' + i))
		event.Timestamp = now.Add(time.Duration(i) * time.Hour)
		require.NoError(t, storage.Write(event))
	}

	// Test ascending sort (oldest first)
	filter := AuditFilter{
		SortByTime: "asc",
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 5)
	// Should be in chronological order (oldest first)
	assert.Equal(t, "e", results[0].ID)
	assert.Equal(t, "a", results[4].ID)
}

// TestMemoryStorage_Query_Sort_Descending tests descending sort order.
func TestMemoryStorage_Query_Sort_Descending(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	now := time.Now()
	// Add events in chronological order
	for i := 0; i < 5; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = string(rune('a' + i))
		event.Timestamp = now.Add(time.Duration(i) * time.Hour)
		require.NoError(t, storage.Write(event))
	}

	// Test descending sort (newest first)
	filter := AuditFilter{
		SortByTime: "desc",
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 5)
	// Should be in reverse chronological order (newest first)
	assert.Equal(t, "e", results[0].ID)
	assert.Equal(t, "a", results[4].ID)
}

// TestMemoryStorage_Query_Pagination_WithSort tests pagination with sorting.
func TestMemoryStorage_Query_Pagination_WithSort(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	now := time.Now()
	// Add 10 events
	for i := 0; i < 10; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = string(rune('a' + i))
		event.Timestamp = now.Add(time.Duration(i) * time.Hour)
		require.NoError(t, storage.Write(event))
	}

	// Test descending sort with pagination
	filter := AuditFilter{
		Page:       1,
		PageSize:   3,
		SortByTime: "desc",
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)
	// Should get newest 3 events
	assert.Equal(t, "j", results[0].ID)
	assert.Equal(t, "h", results[2].ID)
}

// TestFileStorage_Query_Pagination tests pagination for file storage.
func TestFileStorage_Query_Pagination(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	// Add 10 events
	for i := 0; i < 10; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = string(rune('a' + i))
		err := storage.Write(event)
		require.NoError(t, err)
	}

	// Test page 1
	filter := AuditFilter{
		Page:     1,
		PageSize: 3,
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "a", results[0].ID)

	// Test page 2
	filter.Page = 2
	results, err = storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "d", results[0].ID)
}

// TestFileStorage_Query_Sort tests sorting for file storage.
func TestFileStorage_Query_Sort(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	now := time.Now()
	// Add 5 events
	for i := 0; i < 5; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = string(rune('a' + i))
		event.Timestamp = now.Add(time.Duration(i) * time.Hour)
		err := storage.Write(event)
		require.NoError(t, err)
	}

	// Test descending sort
	filter := AuditFilter{
		SortByTime: "desc",
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 5)
	assert.Equal(t, "e", results[0].ID)
	assert.Equal(t, "a", results[4].ID)
}

// TestMemoryStorage_Query_Pagination_InvalidPage tests invalid page number handling.
func TestMemoryStorage_Query_Pagination_InvalidPage(t *testing.T) {
	storage := NewMemoryStorage()
	defer func() { require.NoError(t, storage.Close()) }()

	// Add 5 events
	for i := 0; i < 5; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		require.NoError(t, storage.Write(event))
	}

	// Test with page 0 (should default to 1)
	filter := AuditFilter{
		Page:     0,
		PageSize: 3,
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Test with negative page (should default to 1)
	filter.Page = -1
	results, err = storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)
}

// TestFileStorage_Query_Pagination_WithRotation tests pagination across rotated files.
func TestFileStorage_Query_Pagination_WithRotation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "audit.log")

	storage, err := NewFileStorage(logPath, DefaultFileStorageOptions())
	require.NoError(t, err)
	defer func() { require.NoError(t, storage.Close()) }()

	// Add events and rotate
	for i := 0; i < 5; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = string(rune('a' + i))
		err := storage.Write(event)
		require.NoError(t, err)
		if i == 2 {
			err = storage.Rotate()
			require.NoError(t, err)
		}
	}

	// Query with pagination
	filter := AuditFilter{
		Page:     1,
		PageSize: 3,
	}
	results, err := storage.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)
}

// createTestEvent creates a test audit event with the given parameters.
func createTestEvent(action string, level AuditLevel, category AuditCategory) *AuditEvent {
	return &AuditEvent{
		ID:        "test-id",
		Timestamp: time.Now(),
		Level:     level,
		Category:  category,
		Action:    action,
		Actor: AuditActor{
			ID:   "test-actor",
			Type: "user",
			Name: "Test User",
		},
		Resource: AuditResource{
			ID:   "test-resource",
			Type: "resource",
		},
		Result: AuditResult{
			Success: true,
		},
		Context: AuditContext{
			RequestID: "test-request",
			Metadata:  make(map[string]string),
		},
	}
}
