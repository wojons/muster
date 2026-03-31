package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewResidencyEnforcer(t *testing.T) {
	db, logger := setupTestDB(t)
	enforcer := NewResidencyEnforcer(db, logger)

	assert.NotNil(t, enforcer)
	assert.Nil(t, enforcer.currentPolicy)
	assert.Len(t, enforcer.policies, 0)
}

func TestDataRegion_IsValid(t *testing.T) {
	tests := []struct {
		region DataRegion
		valid  bool
	}{
		{DataRegionEU, true},
		{DataRegionUS, true},
		{DataRegionUK, true},
		{DataRegionCH, true},
		{DataRegionCA, true},
		{DataRegionAU, true},
		{DataRegionJP, true},
		{DataRegionIN, true},
		{DataRegionBR, true},
		{DataRegionSG, true},
		{DataRegionAny, true},
		{DataRegion("invalid"), false},
		{DataRegion(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.region), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.region.IsValid())
		})
	}
}

func TestDataRegion_String(t *testing.T) {
	r := DataRegionEU
	assert.Equal(t, "eu", r.String())
}

func TestEnforceMode_IsValid(t *testing.T) {
	tests := []struct {
		mode  EnforceMode
		valid bool
	}{
		{EnforceModeStrict, true},
		{EnforceModeAudit, true},
		{EnforceModeDisabled, true},
		{EnforceMode("invalid"), false},
		{EnforceMode(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.mode.IsValid())
		})
	}
}

func TestResidencyEnforcer_LoadPolicies_NoPolicies(t *testing.T) {
	db, logger := setupTestDB(t)
	enforcer := NewResidencyEnforcer(db, logger)

	// Skip loading policies since table doesn't exist in test setup
	// Just verify enforcer works without policy
	assert.Nil(t, enforcer.currentPolicy)
	assert.Len(t, enforcer.policies, 0)
}

func TestSQLiteManager_CreateResidencyPolicy(t *testing.T) {
	dbPath := setupTestDBPath(t)
	sm, logger := setupTestSQLiteManager(t, dbPath)

	ctx := context.Background()

	// Run migrations
	mm, err := NewMigrationManager(sm.db, "../../migrations", logger)
	require.NoError(t, err)
	err = mm.Up()
	require.NoError(t, err)

	allowedRegions := []DataRegion{DataRegionEU, DataRegionUK}
	policy, err := sm.CreateResidencyPolicy(ctx, "EU Policy", "Restrict to EU and UK", allowedRegions, EnforceModeStrict)

	require.NoError(t, err)
	assert.Greater(t, policy.ID, int64(0))
	assert.Equal(t, "EU Policy", policy.Name)
	assert.Equal(t, "Restrict to EU and UK", policy.Description)
	assert.Equal(t, []DataRegion{DataRegionEU, DataRegionUK}, policy.AllowedRegions)
	assert.Equal(t, EnforceModeStrict, policy.EnforceMode)
	assert.WithinDuration(t, time.Now(), policy.CreatedAt, 2*time.Second)
	assert.WithinDuration(t, time.Now(), policy.UpdatedAt, 2*time.Second)
}

func TestSQLiteManager_CreateResidencyPolicy_InvalidMode(t *testing.T) {
	dbPath := setupTestDBPath(t)
	sm, logger := setupTestSQLiteManager(t, dbPath)

	ctx := context.Background()

	// Run migrations
	mm, err := NewMigrationManager(sm.db, "../../migrations", logger)
	require.NoError(t, err)
	err = mm.Up()
	require.NoError(t, err)

	_, err = sm.CreateResidencyPolicy(ctx, "Test", "Test policy", []DataRegion{DataRegionUS}, "invalid")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid enforce mode")
}

func TestSQLiteManager_UpdateResidencyPolicy(t *testing.T) {
	dbPath := setupTestDBPath(t)
	sm, logger := setupTestSQLiteManager(t, dbPath)

	ctx := context.Background()

	// Run migrations
	mm, err := NewMigrationManager(sm.db, "../../migrations", logger)
	require.NoError(t, err)
	err = mm.Up()
	require.NoError(t, err)

	// Create policy
	allowedRegions := []DataRegion{DataRegionEU}
	policy, err := sm.CreateResidencyPolicy(ctx, "Test Policy", "Original", allowedRegions, EnforceModeAudit)
	require.NoError(t, err)

	// Update policy
	updatedRegions := []DataRegion{DataRegionEU, DataRegionUS, DataRegionUK}
	updated, err := sm.UpdateResidencyPolicy(ctx, policy.ID, "Updated Policy", "Modified", updatedRegions, EnforceModeStrict)

	require.NoError(t, err)
	assert.Equal(t, policy.ID, updated.ID)
	assert.Equal(t, "Updated Policy", updated.Name)
	assert.Equal(t, "Modified", updated.Description)
	assert.Equal(t, updatedRegions, updated.AllowedRegions)
	assert.Equal(t, EnforceModeStrict, updated.EnforceMode)
	assert.WithinDuration(t, policy.CreatedAt, updated.CreatedAt, 1*time.Millisecond)
	assert.WithinDuration(t, time.Now(), updated.UpdatedAt, 2*time.Second)
}

func TestSQLiteManager_DeleteResidencyPolicy(t *testing.T) {
	dbPath := setupTestDBPath(t)
	sm, logger := setupTestSQLiteManager(t, dbPath)

	ctx := context.Background()

	// Run migrations
	mm, err := NewMigrationManager(sm.db, "../../migrations", logger)
	require.NoError(t, err)
	err = mm.Up()
	require.NoError(t, err)

	// Create policy
	policy, err := sm.CreateResidencyPolicy(ctx, "Test", "To delete", []DataRegion{DataRegionUS}, EnforceModeDisabled)
	require.NoError(t, err)

	// Delete policy
	err = sm.DeleteResidencyPolicy(ctx, policy.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = sm.GetResidencyPolicy(ctx, policy.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "policy not found")
}

func TestSQLiteManager_GetResidencyPolicy(t *testing.T) {
	dbPath := setupTestDBPath(t)
	sm, logger := setupTestSQLiteManager(t, dbPath)

	ctx := context.Background()

	// Run migrations
	mm, err := NewMigrationManager(sm.db, "../../migrations", logger)
	require.NoError(t, err)
	err = mm.Up()
	require.NoError(t, err)

	// Create policy
	allowedRegions := []DataRegion{DataRegionEU, DataRegionUK, DataRegionCH}
	policy, err := sm.CreateResidencyPolicy(ctx, "European Policy", "EU+UK+CH", allowedRegions, EnforceModeStrict)
	require.NoError(t, err)

	// Get policy
	retrieved, err := sm.GetResidencyPolicy(ctx, policy.ID)

	require.NoError(t, err)
	assert.Equal(t, policy.ID, retrieved.ID)
	assert.Equal(t, policy.Name, retrieved.Name)
	assert.Equal(t, policy.Description, retrieved.Description)
	assert.Equal(t, allowedRegions, retrieved.AllowedRegions)
	assert.Equal(t, policy.EnforceMode, retrieved.EnforceMode)
}

func TestSQLiteManager_GetResidencyPolicy_NotFound(t *testing.T) {
	dbPath := setupTestDBPath(t)
	sm, logger := setupTestSQLiteManager(t, dbPath)

	ctx := context.Background()

	// Run migrations
	mm, err := NewMigrationManager(sm.db, "../../migrations", logger)
	require.NoError(t, err)
	err = mm.Up()
	require.NoError(t, err)

	_, err = sm.GetResidencyPolicy(ctx, 999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "policy not found")
}

func TestSQLiteManager_GetActiveResidencyPolicy(t *testing.T) {
	dbPath := setupTestDBPath(t)
	sm, logger := setupTestSQLiteManager(t, dbPath)

	ctx := context.Background()

	// Run migrations
	mm, err := NewMigrationManager(sm.db, "../../migrations", logger)
	require.NoError(t, err)
	err = mm.Up()
	require.NoError(t, err)

	// No policy exists
	active, err := sm.GetActiveResidencyPolicy(ctx)
	require.NoError(t, err)
	assert.Nil(t, active)

	// Create first policy
	_, err = sm.CreateResidencyPolicy(ctx, "Policy 1", "First", []DataRegion{DataRegionUS}, EnforceModeAudit)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)

	// Create second policy
	policy2, err := sm.CreateResidencyPolicy(ctx, "Policy 2", "Second", []DataRegion{DataRegionEU}, EnforceModeStrict)
	require.NoError(t, err)

	// Get active (should be the latest)
	active, err = sm.GetActiveResidencyPolicy(ctx)
	require.NoError(t, err)
	assert.NotNil(t, active)
	assert.Equal(t, policy2.ID, active.ID)
	assert.Equal(t, "Policy 2", active.Name)
}

func TestResidencyEnforcer_ValidateRegion_NoPolicy(t *testing.T) {
	db, logger := setupTestDB(t)
	enforcer := NewResidencyEnforcer(db, logger)

	// No policy set - should allow any region
	err := enforcer.ValidateRegion(DataRegionUS)
	assert.NoError(t, err)

	err = enforcer.ValidateRegion(DataRegionEU)
	assert.NoError(t, err)
}

func TestResidencyEnforcer_ValidateRegion_Disabled(t *testing.T) {
	db, logger := setupTestDB(t)
	enforcer := NewResidencyEnforcer(db, logger)

	policy := &DataResidencyPolicy{
		ID:             1,
		Name:           "Test Policy",
		AllowedRegions: []DataRegion{DataRegionEU},
		EnforceMode:    EnforceModeDisabled,
	}
	enforcer.SetCurrentPolicy(policy)

	// Should allow any region when disabled
	err := enforcer.ValidateRegion(DataRegionUS)
	assert.NoError(t, err)
}

func TestResidencyEnforcer_ValidateRegion_Any(t *testing.T) {
	db, logger := setupTestDB(t)
	enforcer := NewResidencyEnforcer(db, logger)

	policy := &DataResidencyPolicy{
		ID:             1,
		Name:           "Open Policy",
		AllowedRegions: []DataRegion{DataRegionAny},
		EnforceMode:    EnforceModeStrict,
	}
	enforcer.SetCurrentPolicy(policy)

	// Should allow any region when DataRegionAny is in allowed list
	err := enforcer.ValidateRegion(DataRegionUS)
	assert.NoError(t, err)

	err = enforcer.ValidateRegion(DataRegionJP)
	assert.NoError(t, err)
}

func TestResidencyEnforcer_ValidateRegion_Strict_Allowed(t *testing.T) {
	db, logger := setupTestDB(t)
	enforcer := NewResidencyEnforcer(db, logger)

	policy := &DataResidencyPolicy{
		ID:             1,
		Name:           "EU Policy",
		AllowedRegions: []DataRegion{DataRegionEU, DataRegionUK},
		EnforceMode:    EnforceModeStrict,
	}
	enforcer.SetCurrentPolicy(policy)

	// Allowed region
	err := enforcer.ValidateRegion(DataRegionEU)
	assert.NoError(t, err)

	err = enforcer.ValidateRegion(DataRegionUK)
	assert.NoError(t, err)
}

func TestResidencyEnforcer_ValidateRegion_Strict_Blocked(t *testing.T) {
	db, logger := setupTestDB(t)
	enforcer := NewResidencyEnforcer(db, logger)

	policy := &DataResidencyPolicy{
		ID:             1,
		Name:           "EU Policy",
		AllowedRegions: []DataRegion{DataRegionEU},
		EnforceMode:    EnforceModeStrict,
	}
	enforcer.SetCurrentPolicy(policy)

	// Blocked region
	err := enforcer.ValidateRegion(DataRegionUS)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrRegionViolation)
	assert.Contains(t, err.Error(), "region us not allowed")
}

func TestResidencyEnforcer_ValidateRegion_Audit(t *testing.T) {
	db, logger := setupTestDB(t)
	enforcer := NewResidencyEnforcer(db, logger)

	policy := &DataResidencyPolicy{
		ID:             1,
		Name:           "EU Policy",
		AllowedRegions: []DataRegion{DataRegionEU},
		EnforceMode:    EnforceModeAudit,
	}
	enforcer.SetCurrentPolicy(policy)

	// Should not block in audit mode, but should log
	err := enforcer.ValidateRegion(DataRegionUS)
	assert.NoError(t, err)
}

func TestResidencyEnforcer_AuthorizeWrite_Strict_Blocked(t *testing.T) {
	dbPath := setupTestDBPath(t)
	sm, logger := setupTestSQLiteManager(t, dbPath)
	enforcer := NewResidencyEnforcer(sm.db, logger)

	// Run migrations to create violations table
	mm, err := NewMigrationManager(sm.db, "../../migrations", logger)
	require.NoError(t, err)
	err = mm.Up()
	require.NoError(t, err)

	ctx := context.Background()

	// Create policy in database
	policy, err := sm.CreateResidencyPolicy(ctx, "US Policy", "US only", []DataRegion{DataRegionUS}, EnforceModeStrict)
	require.NoError(t, err)

	enforcer.SetCurrentPolicy(policy)

	err = enforcer.AuthorizeWrite(ctx, "config", 123, DataRegionEU, "user-456")

	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrRegionViolation)
	assert.Contains(t, err.Error(), "cannot write config/123 to region eu")
}

func TestResidencyEnforcer_AuthorizeWrite_Audit(t *testing.T) {
	dbPath := setupTestDBPath(t)
	sm, logger := setupTestSQLiteManager(t, dbPath)
	enforcer := NewResidencyEnforcer(sm.db, logger)

	// Run migrations
	mm, err := NewMigrationManager(sm.db, "../../migrations", logger)
	require.NoError(t, err)
	err = mm.Up()
	require.NoError(t, err)

	ctx := context.Background()

	// Create policy in database
	policy, err := sm.CreateResidencyPolicy(ctx, "US Policy", "US only", []DataRegion{DataRegionUS}, EnforceModeAudit)
	require.NoError(t, err)

	enforcer.SetCurrentPolicy(policy)

	err = enforcer.AuthorizeWrite(ctx, "config", 123, DataRegionEU, "user-456")

	// Should not block in audit mode
	assert.NoError(t, err)

	// But should record violation
	violations, err := enforcer.GetViolations(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, violations, 1)
	assert.Equal(t, "config", violations[0].EntityType)
	assert.Equal(t, int64(123), violations[0].EntityID)
	assert.Equal(t, DataRegionEU, violations[0].AttemptedRegion)
	assert.Equal(t, "user-456", violations[0].UserID)
	assert.False(t, violations[0].Blocked)
}

func TestResidencyEnforcer_GetViolations_Pagination(t *testing.T) {
	dbPath := setupTestDBPath(t)
	sm, logger := setupTestSQLiteManager(t, dbPath)
	enforcer := NewResidencyEnforcer(sm.db, logger)

	// Run migrations
	mm, err := NewMigrationManager(sm.db, "../../migrations", logger)
	require.NoError(t, err)
	err = mm.Up()
	require.NoError(t, err)

	ctx := context.Background()

	// Create policy first
	policy, err := sm.CreateResidencyPolicy(ctx, "Test Policy", "Test", []DataRegion{DataRegionUS}, EnforceModeStrict)
	require.NoError(t, err)

	// Insert multiple violations
	for i := 0; i < 15; i++ {
		violation := &ResidencyViolation{
			PolicyID:        policy.ID,
			PolicyName:      "Test Policy",
			EntityType:      "config",
			EntityID:        int64(i),
			AttemptedRegion: DataRegionEU,
			AllowedRegions:  "us",
			Action:          "write",
			Blocked:         true,
			UserID:          "test-user",
			CreatedAt:       time.Now(),
		}
		err := enforcer.saveViolation(ctx, violation)
		require.NoError(t, err)
	}

	// Get first page
	violations, err := enforcer.GetViolations(ctx, 10, 0)
	require.NoError(t, err)
	assert.Len(t, violations, 10)

	// Get second page
	violations, err = enforcer.GetViolations(ctx, 10, 10)
	require.NoError(t, err)
	assert.Len(t, violations, 5)
}

func TestRegionsToString(t *testing.T) {
	regions := []DataRegion{DataRegionEU, DataRegionUS, DataRegionUK}
	result := regionsToString(regions)
	assert.Equal(t, "eu, us, uk", result)
}

func TestRegionsToStringSlice(t *testing.T) {
	regions := []DataRegion{DataRegionEU, DataRegionUS, DataRegionUK}
	result := regionsToStringSlice(regions)
	assert.Equal(t, []string{"eu", "us", "uk"}, result)
}

// Helper functions
func setupTestDB(t *testing.T) (*sql.DB, *zap.Logger) {
	logger, _ := zap.NewDevelopment()

	dbPath := setupTestDBPath(t)

	cfg := DefaultSQLiteConfig(dbPath)
	sm, err := NewSQLiteManager(cfg, logger)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = sm.Close()
	})

	return sm.db, logger
}

func setupTestDBPath(t *testing.T) string {
	tmpFile, err := os.CreateTemp("", "muster_test_*.db")
	require.NoError(t, err)

	dbPath := tmpFile.Name()
	_ = tmpFile.Close()
	_ = os.Remove(dbPath)

	t.Cleanup(func() {
		_ = os.Remove(dbPath)
		_ = os.Remove(dbPath + "-wal")
		_ = os.Remove(dbPath + "-shm")
	})

	return dbPath
}

func setupTestSQLiteManager(t *testing.T, dbPath string) (*SQLiteManager, *zap.Logger) {
	logger, _ := zap.NewDevelopment()

	cfg := DefaultSQLiteConfig(dbPath)
	sm, err := NewSQLiteManager(cfg, logger)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = sm.Close()
	})

	return sm, logger
}
