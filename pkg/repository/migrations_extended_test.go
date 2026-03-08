package repository

import (
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMigrationManager_CreateMigration_EmptyName(t *testing.T) {
	sm, err := NewSQLiteManager(&SQLiteConfig{Path: ":memory:"}, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	dir := t.TempDir()
	mm, err := NewMigrationManager(sm.GetDB(), dir, nil)
	require.NoError(t, err)

	path, err := mm.CreateMigration("")
	require.Error(t, err)
	require.Empty(t, path)
}

func TestMigrationManager_New_NilDB(t *testing.T) {
	_, err := NewMigrationManager(nil, t.TempDir(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "database connection is required")
}

func TestMigrationManager_WithLogger(t *testing.T) {
	sm, err := NewSQLiteManager(&SQLiteConfig{Path: ":memory:"}, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	logger := zap.NewNop()
	dir := t.TempDir()

	mm, err := NewMigrationManager(sm.GetDB(), dir, logger)
	require.NoError(t, err)

	// Create a migration file first
	_, err = mm.CreateMigration("migration_for_logger_test")
	require.NoError(t, err)

	err = mm.Validate()
	require.NoError(t, err)
}

func TestMigrationManager_Status_Detailed(t *testing.T) {
	sm, err := NewSQLiteManager(&SQLiteConfig{Path: ":memory:"}, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	dir := t.TempDir()

	mm, err := NewMigrationManager(sm.GetDB(), dir, nil)
	require.NoError(t, err)

	// Create a migration file first
	_, err = mm.CreateMigration("migration_for_status_test")
	require.NoError(t, err)

	status, err := mm.Status()
	require.NoError(t, err)
	require.NotNil(t, status)
	require.GreaterOrEqual(t, status.Current, int64(0))
}

func TestMigrationManager_Validate_Detailed(t *testing.T) {
	sm, err := NewSQLiteManager(&SQLiteConfig{Path: ":memory:"}, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	dir := t.TempDir()

	mm, err := NewMigrationManager(sm.GetDB(), dir, nil)
	require.NoError(t, err)

	// Create a migration file first
	_, err = mm.CreateMigration("migration_for_validate_test")
	require.NoError(t, err)

	err = mm.Validate()
	require.NoError(t, err)
}

func TestMigrationManager_MigrationFileCreation(t *testing.T) {
	sm, err := NewSQLiteManager(&SQLiteConfig{Path: ":memory:"}, nil)
	require.NoError(t, err)
	defer func() { _ = sm.Close() }()

	dir := t.TempDir()

	mm, err := NewMigrationManager(sm.GetDB(), dir, nil)
	require.NoError(t, err)

	path, err := mm.CreateMigration("test_migration")
	require.NoError(t, err)
	require.NotEmpty(t, path)

	// Verify file exists
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NotNil(t, info)
	require.True(t, info.Mode().IsRegular())
}
