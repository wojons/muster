package perf

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(tb testing.TB) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		tb.Fatalf("Failed to open test DB: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS inventories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url TEXT NOT NULL,
			name TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_name ON inventories(name);
	`)
	if err != nil {
		tb.Fatalf("Failed to create table: %v", err)
	}

	return db
}

func BenchmarkDBQuery(b *testing.B) {
	db := setupTestDB(b)
	defer db.Close()

	for i := 0; i < 10000; i++ {
		db.Exec("INSERT INTO inventories (url, name) VALUES (?, ?)",
			fmt.Sprintf("https://example%d.com", i),
			fmt.Sprintf("inventory%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()

		var id int
		err := db.QueryRow(
			"SELECT id FROM inventories WHERE name = ?",
			"inventory5000",
		).Scan(&id)

		elapsed := time.Since(start)

		if err != nil {
			b.Fatalf("Query failed: %v", err)
		}

		if elapsed > 1*time.Millisecond {
			b.Errorf("Query too slow: %v (target: <1ms indexed)", elapsed)
		}
	}
}

func BenchmarkDBWrite(b *testing.B) {
	db := setupTestDB(b)
	defer db.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()

		tx, err := db.Begin()
		if err != nil {
			b.Fatal(err)
		}

		_, err = tx.Exec(
			"INSERT INTO inventories (url, name) VALUES (?, ?)",
			"https://example.com",
			fmt.Sprintf("inventory%d", i),
		)
		if err != nil {
			tx.Rollback()
			b.Fatalf("Insert failed: %v", err)
		}
		tx.Commit()

		elapsed := time.Since(start)

		if elapsed > 5*time.Millisecond {
			b.Errorf("Write too slow: %v (target: <5ms)", elapsed)
		}
	}
}

func TestDBQueryPerformance(t *testing.T) {
	const targetLatency = 1 * time.Millisecond

	db := setupTestDB(t)
	defer db.Close()

	for i := 0; i < 10000; i++ {
		db.Exec("INSERT INTO inventories (url, name) VALUES (?, ?)",
			fmt.Sprintf("https://example%d.com", i),
			fmt.Sprintf("inventory%d", i))
	}

	start := time.Now()

	var id int
	err := db.QueryRow(
		"SELECT id FROM inventories WHERE name = ?",
		"inventory5000",
	).Scan(&id)

	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if elapsed > targetLatency {
		t.Errorf("Query too slow: %v (target: <%v)", elapsed, targetLatency)
	}

	t.Logf("DB query completed in %v", elapsed)
}
