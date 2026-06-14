package repository

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Compile-time interface check: *SQLiteStore satisfies Store
// ---------------------------------------------------------------------------

var _ Store = (*SQLiteStore)(nil)

// ---------------------------------------------------------------------------
// NewSQLiteStore
// ---------------------------------------------------------------------------

func TestNewSQLiteStore_Valid(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(\":memory:\") returned unexpected error: %v", err)
	}
	defer s.Close()

	if s.DB() == nil {
		t.Fatal("DB() returned nil after NewSQLiteStore(\":memory:\")")
	}
}

func TestNewSQLiteStore_InvalidPath(t *testing.T) {
	// A path whose parent directory does not exist — SQLite cannot create a file there.
	_, err := NewSQLiteStore("/nonexistent-xyz-abc/some.db")
	if err == nil {
		t.Fatal("NewSQLiteStore with non-existent directory expected error, got nil")
	}
}

func TestNewSQLiteStore_DBPing(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(\":memory:\") failed: %v", err)
	}
	defer s.Close()

	if err := s.DB().Ping(); err != nil {
		t.Fatalf("DB().Ping() after NewSQLiteStore failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SQLiteStore.Close
// ---------------------------------------------------------------------------

func TestSQLiteStore_Close(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(\":memory:\") failed: %v", err)
	}

	// First close should succeed.
	if err := s.Close(); err != nil {
		t.Fatalf("first Close() returned error: %v", err)
	}

	// Second close on the same store — db is now nil so it should return nil.
	if err := s.Close(); err != nil {
		t.Fatalf("second Close() (after db already closed) returned error: %v", err)
	}
}

func TestSQLiteStore_Close_NilDB(t *testing.T) {
	s := &SQLiteStore{db: nil}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() with nil db returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SQLiteStore.DB
// ---------------------------------------------------------------------------

func TestSQLiteStore_DB(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(\":memory:\") failed: %v", err)
	}
	defer s.Close()

	db := s.DB()
	if db == nil {
		t.Fatal("DB() returned nil")
	}

	// Pointer identity — DB() must return the same *sql.DB held internally.
	if s.db != db {
		t.Fatal("DB() returned a different *sql.DB instance than the internal field")
	}
}

// ---------------------------------------------------------------------------
// SQLiteStore.Exec + QueryRow (full round-trip: CREATE, INSERT, SELECT)
// ---------------------------------------------------------------------------

func TestSQLiteStore_Exec_CreateTableInsertSelect(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(\":memory:\") failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	// Create table.
	_, err = s.Exec(ctx, `CREATE TABLE IF NOT EXISTS test_exec (
		id   INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("Exec CREATE TABLE failed: %v", err)
	}

	// Insert a row.
	res, err := s.Exec(ctx, "INSERT INTO test_exec (name) VALUES (?)", "hello")
	if err != nil {
		t.Fatalf("Exec INSERT failed: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() failed: %v", err)
	}
	if id != 1 {
		t.Fatalf("expected LastInsertId = 1, got %d", id)
	}

	// Read it back via QueryRow.
	var name string
	err = s.QueryRow(ctx, "SELECT name FROM test_exec WHERE id = ?", id).Scan(&name)
	if err != nil {
		t.Fatalf("QueryRow Scan failed: %v", err)
	}
	if name != "hello" {
		t.Fatalf("expected name = \"hello\", got %q", name)
	}
}

// ---------------------------------------------------------------------------
// SQLiteStore.Query (INSERT multiple, SELECT rows)
// ---------------------------------------------------------------------------

func TestSQLiteStore_Query(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(\":memory:\") failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	_, err = s.Exec(ctx, `CREATE TABLE IF NOT EXISTS test_query (
		id    INTEGER PRIMARY KEY AUTOINCREMENT,
		label TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("Exec CREATE TABLE failed: %v", err)
	}

	for _, lbl := range []string{"alpha", "beta", "gamma"} {
		_, err = s.Exec(ctx, "INSERT INTO test_query (label) VALUES (?)", lbl)
		if err != nil {
			t.Fatalf("Exec INSERT %q failed: %v", lbl, err)
		}
	}

	rows, err := s.Query(ctx, "SELECT id, label FROM test_query ORDER BY id")
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var id int64
		var label string
		if err := rows.Scan(&id, &label); err != nil {
			t.Fatalf("rows.Scan failed: %v", err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err() returned error: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 rows, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// SQLiteStore.QueryRow (INSERT single, SELECT single)
// ---------------------------------------------------------------------------

func TestSQLiteStore_QueryRow(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(\":memory:\") failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()

	_, err = s.Exec(ctx, `CREATE TABLE IF NOT EXISTS test_qrow (
		id  INTEGER PRIMARY KEY AUTOINCREMENT,
		val TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("Exec CREATE TABLE failed: %v", err)
	}

	_, err = s.Exec(ctx, "INSERT INTO test_qrow (val) VALUES (?)", "single-row")
	if err != nil {
		t.Fatalf("Exec INSERT failed: %v", err)
	}

	row := s.QueryRow(ctx, "SELECT val FROM test_qrow WHERE id = ?", 1)
	if row == nil {
		t.Fatal("QueryRow returned nil *sql.Row")
	}

	var val string
	if err := row.Scan(&val); err != nil {
		t.Fatalf("QueryRow Scan failed: %v", err)
	}
	if val != "single-row" {
		t.Fatalf("expected val = \"single-row\", got %q", val)
	}
}

// ---------------------------------------------------------------------------
// Cancelled context propagation
// ---------------------------------------------------------------------------

func TestSQLiteStore_Exec_CancelledContext(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(\":memory:\") failed: %v", err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // context is already cancelled

	_, err = s.Exec(ctx, "SELECT 1")
	if err == nil {
		t.Fatal("Exec with cancelled context expected error, got nil")
	}
}

func TestSQLiteStore_Query_CancelledContext(t *testing.T) {
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(\":memory:\") failed: %v", err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // context is already cancelled

	_, err = s.Query(ctx, "SELECT 1")
	if err == nil {
		t.Fatal("Query with cancelled context expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ErrNotFound
// ---------------------------------------------------------------------------

func TestErrNotFound_Error(t *testing.T) {
	e := ErrNotFound{Entity: "user", ID: 42}
	msg := e.Error()
	want := "user with id 42 not found"
	if msg != want {
		t.Fatalf("ErrNotFound.Error() = %q, want %q", msg, want)
	}
}

// ---------------------------------------------------------------------------
// ListOptions
// ---------------------------------------------------------------------------

func TestListOptions(t *testing.T) {
	// Zero value.
	var opts ListOptions
	if opts.Limit != 0 {
		t.Errorf("zero-value ListOptions.Limit = %d, want 0", opts.Limit)
	}
	if opts.Offset != 0 {
		t.Errorf("zero-value ListOptions.Offset = %d, want 0", opts.Offset)
	}

	// Populated value.
	opts = ListOptions{Limit: 25, Offset: 10}
	if opts.Limit != 25 {
		t.Errorf("ListOptions.Limit = %d, want 25", opts.Limit)
	}
	if opts.Offset != 10 {
		t.Errorf("ListOptions.Offset = %d, want 10", opts.Offset)
	}
}

// ---------------------------------------------------------------------------
// Store interface satisfaction (compile-time + test-time)
// ---------------------------------------------------------------------------

func TestStoreInterface_CompileCheck(t *testing.T) {
	// Ensure *SQLiteStore can be assigned to Store at run time.
	var s Store = (*SQLiteStore)(nil)
	_ = s
}

// ---------------------------------------------------------------------------
// Alias struct field verification
// ---------------------------------------------------------------------------

func TestAlias_Struct(t *testing.T) {
	now := time.Now()

	a := Alias{
		ID:          1,
		Name:        "my-alias",
		Description: "test alias description",
		InventoryID: 100,
		ModuleID:    200,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if a.ID != 1 {
		t.Errorf("Alias.ID = %d, want 1", a.ID)
	}
	if a.Name != "my-alias" {
		t.Errorf("Alias.Name = %q, want \"my-alias\"", a.Name)
	}
	if a.Description != "test alias description" {
		t.Errorf("Alias.Description = %q, want \"test alias description\"", a.Description)
	}
	if a.InventoryID != 100 {
		t.Errorf("Alias.InventoryID = %d, want 100", a.InventoryID)
	}
	if a.ModuleID != 200 {
		t.Errorf("Alias.ModuleID = %d, want 200", a.ModuleID)
	}
	if !a.CreatedAt.Equal(now) {
		t.Errorf("Alias.CreatedAt does not match assigned value")
	}
	if !a.UpdatedAt.Equal(now) {
		t.Errorf("Alias.UpdatedAt does not match assigned value")
	}
}

// ---------------------------------------------------------------------------
// Benchmark
// ---------------------------------------------------------------------------

func BenchmarkNewSQLiteStore(b *testing.B) {
	for i := 0; i < b.N; i++ {
		s, err := NewSQLiteStore(":memory:")
		if err != nil {
			b.Fatalf("NewSQLiteStore(\":memory:\") failed: %v", err)
		}
		_ = s.Close()
	}
}
