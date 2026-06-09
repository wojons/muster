// axiom:trace work_item=audit-logging-01 spec=migrations/00005_audit.sql,specs/EVENTS-AUDIT.md plan=phase-1/task-1/step-1-3 impl=internal/audit/database_storage.go test=internal/audit/database_storage_test.go

package audit

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDatabaseStorage creates a DatabaseStorage backed by an in-memory SQLite database.
func newTestDatabaseStorage(t *testing.T) *DatabaseStorage {
	t.Helper()
	store, err := NewDatabaseStorage(DefaultDatabaseStorageConfig())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestDatabaseStorage_New(t *testing.T) {
	store := newTestDatabaseStorage(t)
	require.NotNil(t, store)
	assert.False(t, store.closed)
	assert.NotNil(t, store.db)
}

func TestDatabaseStorage_New_InvalidDSN(t *testing.T) {
	_, err := NewDatabaseStorage(DatabaseStorageConfig{
		DSN:       "/nonexistent/path/audit.db",
		MigrateDB: false,
	})
	// Should fail because the parent directory doesn't exist
	assert.Error(t, err)
}

func TestDatabaseStorage_Write(t *testing.T) {
	store := newTestDatabaseStorage(t)
	event := createTestEvent("test-action", AuditLevelInfo, AuditCategorySystem)

	err := store.Write(event)
	require.NoError(t, err)

	// Query back to verify
	results, err := store.Query(AuditFilter{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, event.Action, results[0].Action)
	assert.Equal(t, event.Level, results[0].Level)
	assert.Equal(t, event.Category, results[0].Category)
}

func TestDatabaseStorage_Write_Closed(t *testing.T) {
	store := newTestDatabaseStorage(t)
	require.NoError(t, store.Close())

	event := createTestEvent("test", AuditLevelInfo, AuditCategorySystem)
	err := store.Write(event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestDatabaseStorage_Query_TimeRange(t *testing.T) {
	store := newTestDatabaseStorage(t)
	now := time.Now()

	oldEvent := createTestEvent("old-action", AuditLevelInfo, AuditCategorySystem)
	oldEvent.Timestamp = now.Add(-2 * time.Hour)
	require.NoError(t, store.Write(oldEvent))

	newEvent := createTestEvent("new-action", AuditLevelInfo, AuditCategorySystem)
	newEvent.Timestamp = now
	require.NoError(t, store.Write(newEvent))

	// Query for recent events only
	filter := AuditFilter{
		StartTime: now.Add(-30 * time.Minute),
		EndTime:   now.Add(30 * time.Minute),
	}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "new-action", results[0].Action)
}

func TestDatabaseStorage_Query_Category(t *testing.T) {
	store := newTestDatabaseStorage(t)

	authEvent := createTestEvent("login", AuditLevelInfo, AuditCategoryAuth)
	require.NoError(t, store.Write(authEvent))

	systemEvent := createTestEvent("restart", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, store.Write(systemEvent))

	filter := AuditFilter{
		Categories: []AuditCategory{AuditCategoryAuth},
	}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, AuditCategoryAuth, results[0].Category)
}

func TestDatabaseStorage_Query_Level(t *testing.T) {
	store := newTestDatabaseStorage(t)

	infoEvent := createTestEvent("info-action", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, store.Write(infoEvent))

	errorEvent := createTestEvent("error-action", AuditLevelError, AuditCategorySystem)
	require.NoError(t, store.Write(errorEvent))

	filter := AuditFilter{
		Levels: []AuditLevel{AuditLevelError},
	}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, AuditLevelError, results[0].Level)
}

func TestDatabaseStorage_Query_ActorID(t *testing.T) {
	store := newTestDatabaseStorage(t)

	event1 := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
	event1.Actor.ID = "actor-1"
	require.NoError(t, store.Write(event1))

	event2 := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
	event2.Actor.ID = "actor-2"
	require.NoError(t, store.Write(event2))

	filter := AuditFilter{
		ActorID: "actor-1",
	}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "actor-1", results[0].Actor.ID)
}

func TestDatabaseStorage_Query_ResourceType(t *testing.T) {
	store := newTestDatabaseStorage(t)

	event1 := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
	event1.Resource.Type = "user"
	require.NoError(t, store.Write(event1))

	event2 := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
	event2.Resource.Type = "file"
	require.NoError(t, store.Write(event2))

	filter := AuditFilter{
		ResourceType: "user",
	}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "user", results[0].Resource.Type)
}

func TestDatabaseStorage_Query_Action(t *testing.T) {
	store := newTestDatabaseStorage(t)

	createEvent := createTestEvent("create", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, store.Write(createEvent))

	deleteEvent := createTestEvent("delete", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, store.Write(deleteEvent))

	filter := AuditFilter{
		Action: "create",
	}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "create", results[0].Action)
}

func TestDatabaseStorage_Query_Success(t *testing.T) {
	store := newTestDatabaseStorage(t)

	successEvent := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
	successEvent.Result.Success = true
	require.NoError(t, store.Write(successEvent))

	failureEvent := createTestEvent("action", AuditLevelError, AuditCategorySystem)
	failureEvent.Result.Success = false
	require.NoError(t, store.Write(failureEvent))

	success := true
	filter := AuditFilter{
		Success: &success,
	}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].Result.Success)
}

func TestDatabaseStorage_Query_CombinedFilters(t *testing.T) {
	store := newTestDatabaseStorage(t)
	now := time.Now()

	event1 := createTestEvent("login", AuditLevelInfo, AuditCategoryAuth)
	event1.Timestamp = now
	event1.Actor.ID = "user-1"
	event1.Result.Success = true
	require.NoError(t, store.Write(event1))

	event2 := createTestEvent("login", AuditLevelError, AuditCategoryAuth)
	event2.Timestamp = now
	event2.Actor.ID = "user-2"
	event2.Result.Success = false
	require.NoError(t, store.Write(event2))

	event3 := createTestEvent("logout", AuditLevelInfo, AuditCategoryAuth)
	event3.Timestamp = now
	event3.Actor.ID = "user-1"
	event3.Result.Success = true
	require.NoError(t, store.Write(event3))

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
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "login", results[0].Action)
	assert.Equal(t, "user-1", results[0].Actor.ID)
}

func TestDatabaseStorage_Query_EmptyStorage(t *testing.T) {
	store := newTestDatabaseStorage(t)
	results, err := store.Query(AuditFilter{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestDatabaseStorage_Query_Closed(t *testing.T) {
	store := newTestDatabaseStorage(t)
	require.NoError(t, store.Close())

	_, err := store.Query(AuditFilter{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestDatabaseStorage_Close(t *testing.T) {
	store := newTestDatabaseStorage(t)

	event := createTestEvent("test", AuditLevelInfo, AuditCategorySystem)
	require.NoError(t, store.Write(event))

	err := store.Close()
	require.NoError(t, err)
	assert.True(t, store.closed)

	// Close again should be safe
	err = store.Close()
	require.NoError(t, err)
}

func TestDatabaseStorage_Query_Pagination_Basic(t *testing.T) {
	store := newTestDatabaseStorage(t)

	for i := 0; i < 10; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = string(rune('a' + i))
		require.NoError(t, store.Write(event))
	}

	// Page 1 with page size 3
	filter := AuditFilter{Page: 1, PageSize: 3}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Page 2
	filter.Page = 2
	results, err = store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Page 4 (last partial page)
	filter.Page = 4
	results, err = store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestDatabaseStorage_Query_Pagination_EmptyPage(t *testing.T) {
	store := newTestDatabaseStorage(t)

	for i := 0; i < 5; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		require.NoError(t, store.Write(event))
	}

	filter := AuditFilter{Page: 10, PageSize: 5}
	results, err := store.Query(filter)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestDatabaseStorage_Query_Sort_Ascending(t *testing.T) {
	store := newTestDatabaseStorage(t)
	now := time.Now()

	// Add events in reverse order so ascending sort reorders them
	for i := 4; i >= 0; i-- {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = fmt.Sprintf("evt-%d", i)
		event.Timestamp = now.Add(time.Duration(i) * time.Hour)
		require.NoError(t, store.Write(event))
	}

	filter := AuditFilter{SortByTime: "asc"}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 5)
	// Oldest first — evt-0 (oldest) should be first, evt-4 (newest) last
	assert.Equal(t, "evt-0", results[0].ID)
	assert.Equal(t, "evt-1", results[1].ID)
	assert.Equal(t, "evt-4", results[4].ID)
}

func TestDatabaseStorage_Query_Sort_Descending(t *testing.T) {
	store := newTestDatabaseStorage(t)
	now := time.Now()

	for i := 0; i < 5; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = fmt.Sprintf("evt-%d", i)
		event.Timestamp = now.Add(time.Duration(i) * time.Hour)
		require.NoError(t, store.Write(event))
	}

	filter := AuditFilter{SortByTime: "desc"}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 5)
	// Newest first — evt-4 (newest) should be first, evt-0 (oldest) last
	assert.Equal(t, "evt-4", results[0].ID)
	assert.Equal(t, "evt-3", results[1].ID)
	assert.Equal(t, "evt-0", results[4].ID)
}

func TestDatabaseStorage_Query_Pagination_WithSort(t *testing.T) {
	store := newTestDatabaseStorage(t)
	now := time.Now()

	for i := 0; i < 10; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		event.ID = fmt.Sprintf("evt-%d", i)
		event.Timestamp = now.Add(time.Duration(i) * time.Hour)
		require.NoError(t, store.Write(event))
	}

	filter := AuditFilter{
		Page:       1,
		PageSize:   3,
		SortByTime: "desc",
	}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)
	// Newest 3 events first
	assert.Equal(t, "evt-9", results[0].ID)
	assert.Equal(t, "evt-8", results[1].ID)
	assert.Equal(t, "evt-7", results[2].ID)
}

func TestDatabaseStorage_Write_Concurrent(t *testing.T) {
	store := newTestDatabaseStorage(t)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func() {
			event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
			_ = store.Write(event)
			done <- true
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	results, err := store.Query(AuditFilter{})
	require.NoError(t, err)
	assert.Len(t, results, 10)
}

func TestDatabaseStorage_Query_Pagination_DefaultPage(t *testing.T) {
	store := newTestDatabaseStorage(t)

	for i := 0; i < 5; i++ {
		event := createTestEvent("action", AuditLevelInfo, AuditCategorySystem)
		require.NoError(t, store.Write(event))
	}

	// PageSize without Page should default to 1
	filter := AuditFilter{PageSize: 3}
	results, err := store.Query(filter)
	require.NoError(t, err)
	require.Len(t, results, 3)
}

func TestDatabaseStorage_Write_PreservesFullEvent(t *testing.T) {
	store := newTestDatabaseStorage(t)

	event := &AuditEvent{
		ID:           "evt-custom-id",
		Level:        AuditLevelCritical,
		Category:     AuditCategorySecurity,
		Action:       "security_alert",
		Actor:        AuditActor{ID: "system", Type: "service", Name: "Security Monitor", IPAddress: "10.0.0.1"},
		Resource:     AuditResource{ID: "host-42", Type: "server", Name: "prod-web-01"},
		Result:       AuditResult{Success: false, Code: "CRIT-001", Message: "Intrusion detected", Details: "Port scan from 203.0.113.0/24"},
		Context:      AuditContext{RequestID: "req-sec-1", TraceID: "trace-sec-1"},
		Changes:      []AuditChange{{Field: "firewall_rule", OldValue: "allow_all", NewValue: "deny_all"}},
		Hash:         "abc123hash",
		PreviousHash: "prevhash",
	}
	event.Timestamp = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	require.NoError(t, store.Write(event))

	results, err := store.Query(AuditFilter{})
	require.NoError(t, err)
	require.Len(t, results, 1)

	stored := results[0]
	assert.Equal(t, event.ID, stored.ID)
	assert.Equal(t, event.Action, stored.Action)
	assert.Equal(t, event.Level, stored.Level)
	assert.Equal(t, event.Category, stored.Category)
	assert.Equal(t, event.Actor, stored.Actor)
	assert.Equal(t, event.Resource, stored.Resource)
	assert.Equal(t, event.Result, stored.Result)
	assert.Equal(t, event.Context.RequestID, stored.Context.RequestID)
	assert.Equal(t, event.Context.TraceID, stored.Context.TraceID)
	assert.Equal(t, event.Changes, stored.Changes)
	assert.Equal(t, event.Hash, stored.Hash)
	assert.Equal(t, event.PreviousHash, stored.PreviousHash)
}
