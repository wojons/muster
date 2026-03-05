package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type TestDB struct {
	DB       *sql.DB
	Database string
}

func NewTestDB(t *testing.T) *TestDB {
	t.Helper()

	ctx := context.Background()

	tmpDir := t.TempDir()

	dbPath := fmt.Sprintf("%s/test_%d.db", tmpDir, time.Now().UnixNano())

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	require.NoError(t, db.PingContext(ctx))

	t.Cleanup(func() {
		db.Close()
	})

	return &TestDB{DB: db, Database: dbPath}
}

func NewTestDBWithTx(t *testing.T) (*sql.DB, *sql.Tx) {
	t.Helper()
	testDB := NewTestDB(t)
	tx, err := testDB.DB.Begin()
	require.NoError(t, err)
	return testDB.DB, tx
}
