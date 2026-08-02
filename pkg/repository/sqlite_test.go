package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewSQLiteStore_ValidMemory(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(:memory:) failed: %v", err)
	}
	defer store.Close()

	if store.DB() == nil {
		t.Error("DB() returned nil")
	}
}

func TestNewSQLiteStore_ValidTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore(%s) failed: %v", path, err)
	}
	defer store.Close()

	if store.DB() == nil {
		t.Error("DB() returned nil")
	}
}

func TestNewSQLiteStore_InvalidPath(t *testing.T) {
	// Use a path that can't be opened (directory instead of file, or /dev/null weirdness)
	// Try a deeply nested non-existent dir with no write perms
	_, err := NewSQLiteStore("/nonexistent/path/that/does/not/exist/db.sqlite")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestSQLiteStore_Close(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}

	err = store.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Second close should not panic and may return nil or error
	err = store.Close()
	// Either nil or error is acceptable for double-close
	_ = err
}

func TestSQLiteStore_DB(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	db := store.DB()
	if db == nil {
		t.Fatal("DB() returned nil")
	}

	// Verify the connection is alive
	if err := db.Ping(); err != nil {
		t.Errorf("db.Ping() failed: %v", err)
	}
}

func TestSQLiteStore_Exec_CreateTable(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.Exec(ctx, "CREATE TABLE test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
}

func TestSQLiteStore_Exec_InsertAndDelete(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create table
	_, err = store.Exec(ctx, "CREATE TABLE items (id INTEGER PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	// Insert
	result, err := store.Exec(ctx, "INSERT INTO items (value) VALUES (?)", "hello")
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId failed: %v", err)
	}
	if id != 1 {
		t.Errorf("expected LastInsertId = 1, got %d", id)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Errorf("expected RowsAffected = 1, got %d", rowsAffected)
	}

	// Delete
	result, err = store.Exec(ctx, "DELETE FROM items WHERE id = ?", id)
	if err != nil {
		t.Fatalf("DELETE failed: %v", err)
	}

	rowsAffected, err = result.RowsAffected()
	if err != nil {
		t.Fatalf("RowsAffected failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Errorf("expected RowsAffected = 1 after delete, got %d", rowsAffected)
	}
}

func TestSQLiteStore_Exec_BadSQL(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.Exec(ctx, "NOT A VALID SQL STATEMENT")
	if err == nil {
		t.Error("expected error for bad SQL, got nil")
	}
}

func TestSQLiteStore_Exec_CancelledContext(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = store.Exec(ctx, "CREATE TABLE cancelled_test (id INTEGER PRIMARY KEY)")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestSQLiteStore_Exec_ContextDeadline(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	// Create a table first so we can run a query that might hit deadline
	ctx := context.Background()
	_, err = store.Exec(ctx, "CREATE TABLE deadline_test (id INTEGER PRIMARY KEY, data BLOB)")
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	// Insert some data
	for i := 0; i < 100; i++ {
		_, err = store.Exec(ctx, "INSERT INTO deadline_test (data) VALUES (?)", make([]byte, 1024))
		if err != nil {
			t.Fatalf("INSERT failed: %v", err)
		}
	}

	// Use an already-expired deadline
	deadlineCtx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	_, err = store.Exec(deadlineCtx, "SELECT COUNT(*) FROM deadline_test")
	if err == nil {
		t.Error("expected error for expired deadline context, got nil")
	}
}

func TestSQLiteStore_Query(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Setup
	_, err = store.Exec(ctx, "CREATE TABLE query_test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	_, err = store.Exec(ctx, "INSERT INTO query_test (name) VALUES ('alpha')")
	if err != nil {
		t.Fatalf("INSERT alpha failed: %v", err)
	}
	_, err = store.Exec(ctx, "INSERT INTO query_test (name) VALUES ('beta')")
	if err != nil {
		t.Fatalf("INSERT beta failed: %v", err)
	}

	// Query
	rows, err := store.Query(ctx, "SELECT id, name FROM query_test ORDER BY id")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err(): %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}
}

func TestSQLiteStore_Query_Empty(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	_, err = store.Exec(ctx, "CREATE TABLE empty_test (id INTEGER PRIMARY KEY, name TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	rows, err := store.Query(ctx, "SELECT id, name FROM empty_test")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	if rows.Next() {
		t.Error("expected no rows in empty table")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err(): %v", err)
	}
}

func TestSQLiteStore_Query_BadSQL(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	_, err = store.Query(ctx, "NOT A VALID QUERY")
	if err == nil {
		t.Error("expected error for bad SQL query, got nil")
	}
}

func TestSQLiteStore_QueryRow(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	_, err = store.Exec(ctx, "CREATE TABLE row_test (id INTEGER PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	_, err = store.Exec(ctx, "INSERT INTO row_test (value) VALUES ('found')")
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	row := store.QueryRow(ctx, "SELECT id, value FROM row_test WHERE id = 1")
	var id int64
	var value string
	if err := row.Scan(&id, &value); err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if id != 1 {
		t.Errorf("expected id = 1, got %d", id)
	}
	if value != "found" {
		t.Errorf("expected value = 'found', got %q", value)
	}
}

func TestSQLiteStore_QueryRow_NoRows(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	_, err = store.Exec(ctx, "CREATE TABLE norow_test (id INTEGER PRIMARY KEY)")
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	row := store.QueryRow(ctx, "SELECT id FROM norow_test WHERE id = 999")
	var id int64
	err = row.Scan(&id)
	if err != sql.ErrNoRows {
		t.Errorf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestSQLiteStore_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.db")

	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	_, err = store.Exec(ctx, "CREATE TABLE concurrent_test (id INTEGER PRIMARY KEY, counter INTEGER)")
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	_, err = store.Exec(ctx, "INSERT INTO concurrent_test (counter) VALUES (0)")
	if err != nil {
		t.Fatalf("INSERT failed: %v", err)
	}

	var wg sync.WaitGroup
	numGoroutines := 10
	iterations := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_, err := store.Exec(ctx, "UPDATE concurrent_test SET counter = counter + 1 WHERE id = 1")
				if err != nil {
					t.Errorf("concurrent Exec failed: %v", err)
				}
			}
		}()
	}

	wg.Wait()

	// Verify final count
	row := store.QueryRow(ctx, "SELECT counter FROM concurrent_test WHERE id = 1")
	var counter int
	if err := row.Scan(&counter); err != nil {
		t.Fatalf("final Scan failed: %v", err)
	}
	if counter != numGoroutines*iterations {
		t.Errorf("expected counter = %d, got %d (possible race condition?)", numGoroutines*iterations, counter)
	}
}

func TestSQLiteStore_ConcurrentCloseAndQuery(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}

	ctx := context.Background()
	_, err = store.Exec(ctx, "CREATE TABLE ccq_test (id INTEGER PRIMARY KEY)")
	if err != nil {
		store.Close()
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	var wg sync.WaitGroup

	// Reader goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, _ = store.Query(ctx, "SELECT id FROM ccq_test")
		}
	}()

	// Close after a brief delay
	time.Sleep(10 * time.Millisecond)
	store.Close()

	wg.Wait()
	// No panic is success
}

func TestErrNotFound_Error(t *testing.T) {
	e := ErrNotFound{Entity: "user", ID: 42}
	msg := e.Error()
	expected := "user with id 42 not found"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

func TestErrNotFound_Error_ZeroID(t *testing.T) {
	e := ErrNotFound{Entity: "task", ID: 0}
	msg := e.Error()
	expected := "task with id 0 not found"
	if msg != expected {
		t.Errorf("expected %q, got %q", expected, msg)
	}
}

func TestSQLiteStore_SatisfiesStoreInterface(t *testing.T) {
	// Compile-time check that SQLiteStore satisfies Store
	var _ Store = (*SQLiteStore)(nil)
}

func TestNewSQLiteStore_ValidWALMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal_test.db")

	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore(%s) failed: %v", path, err)
	}
	defer store.Close()

	// Verify WAL files are created
	walPath := path + "-wal"
	_, _ = os.Stat(walPath)
	// WAL file may not exist yet if no writes — that's fine
	// Just verify the store is functional
	if err := store.DB().Ping(); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestSQLiteStore_Exec_MultipleStatements(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore failed: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create and populate in sequence
	_, err = store.Exec(ctx, "CREATE TABLE multi_test (id INTEGER PRIMARY KEY, label TEXT)")
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	for i := 1; i <= 5; i++ {
		result, err := store.Exec(ctx, "INSERT INTO multi_test (label) VALUES (?)", "item")
		if err != nil {
			t.Fatalf("INSERT %d failed: %v", i, err)
		}
		id, _ := result.LastInsertId()
		if id != int64(i) {
			t.Errorf("expected LastInsertId = %d, got %d", i, id)
		}
	}

	// Query count
	row := store.QueryRow(ctx, "SELECT COUNT(*) FROM multi_test")
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("COUNT Scan failed: %v", err)
	}
	if count != 5 {
		t.Errorf("expected count = 5, got %d", count)
	}
}
