package repository

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mutecomm/go-sqlcipher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// testDB creates a test database connection
func testDB(t *testing.T) (*sql.DB, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_hipaa_rbac.db")

	db, err := sql.Open("sqlite3", dbPath)
	require.NoError(t, err)

	// Run migrations
	migrationsDir := "../../migrations"
	_, err = NewMigrationManager(db, migrationsDir, zap.NewNop())
	require.NoError(t, err)

	// Create tables manually for test
	createTablesSQL := `
	CREATE TABLE IF NOT EXISTS hipaa_roles (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		description TEXT,
		parent_role_id TEXT REFERENCES hipaa_roles(id),
		minimum_necessary BOOLEAN DEFAULT TRUE,
		created_by TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(tenant_id, name)
	);
	
	CREATE TABLE IF NOT EXISTS hipaa_permissions (
		id TEXT PRIMARY KEY,
		role_id TEXT NOT NULL REFERENCES hipaa_roles(id) ON DELETE CASCADE,
		resource TEXT NOT NULL,
		action TEXT NOT NULL,
		scope TEXT DEFAULT 'default',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(role_id, resource, action, scope)
	);
	
	CREATE TABLE IF NOT EXISTS hipaa_users (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		username TEXT NOT NULL,
		email TEXT NOT NULL,
		employee_id TEXT,
		department TEXT,
		job_title TEXT,
		status TEXT DEFAULT 'active',
		last_login_at TIMESTAMP,
		last_password_change TIMESTAMP,
		failed_login_attempts INTEGER DEFAULT 0,
		account_locked_until TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(tenant_id, username),
		UNIQUE(tenant_id, email)
	);
	
	CREATE TABLE IF NOT EXISTS hipaa_user_roles (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL REFERENCES hipaa_users(id) ON DELETE CASCADE,
		role_id TEXT NOT NULL REFERENCES hipaa_roles(id) ON DELETE CASCADE,
		tenant_id TEXT NOT NULL,
		assigned_by TEXT NOT NULL,
		assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP,
		is_active BOOLEAN DEFAULT TRUE,
		UNIQUE(user_id, role_id, tenant_id)
	);
	
	CREATE TABLE IF NOT EXISTS hipaa_break_glass (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		role_id TEXT NOT NULL,
		reason TEXT NOT NULL,
		justification TEXT NOT NULL,
		authorized_by TEXT,
		authorized_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL,
		reviewed_by TEXT,
		reviewed_at TIMESTAMP,
		review_status TEXT DEFAULT 'pending',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE TABLE IF NOT EXISTS hipaa_phi_resources (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		patient_id TEXT NOT NULL,
		classification TEXT DEFAULT 'normal',
		access_policy_id TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(tenant_id, resource_type, resource_id)
	);
	
	CREATE TABLE IF NOT EXISTS hipaa_access_logs (
		id TEXT PRIMARY KEY,
		tenant_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		role_id TEXT,
		resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL,
		action TEXT NOT NULL,
		result TEXT NOT NULL,
		reason TEXT,
		break_glass_used BOOLEAN DEFAULT FALSE,
		break_glass_id TEXT,
		purpose_of_use TEXT NOT NULL,
		access_type TEXT NOT NULL,
		client_ip TEXT,
		user_agent TEXT,
		session_id TEXT,
		timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`

	_, err = db.Exec(createTablesSQL)
	require.NoError(t, err)

	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(dbPath)
	}

	return db, cleanup
}

func TestHIPAARole_CRUD(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	ctx := context.Background()
	tenantID := "tenant-test-1"

	// Create role
	role := &HIPAARole{
		ID:               "role-test-1",
		TenantID:         tenantID,
		Name:             "Test Physician",
		Type:             RoleTypePhysician,
		Description:      "Test physician role",
		MinimumNecessary: true,
		CreatedBy:        "admin",
	}

	err = store.CreateRole(ctx, role)
	require.NoError(t, err)

	// Get role
	retrieved, err := store.GetRole(ctx, tenantID, role.ID)
	require.NoError(t, err)
	assert.Equal(t, role.ID, retrieved.ID)
	assert.Equal(t, role.Name, retrieved.Name)
	assert.Equal(t, role.Type, retrieved.Type)
	assert.True(t, retrieved.MinimumNecessary)

	// Update role
	role.Description = "Updated physician role"
	err = store.UpdateRole(ctx, role)
	require.NoError(t, err)

	retrieved, err = store.GetRole(ctx, tenantID, role.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated physician role", retrieved.Description)

	// Add permission
	perm := HIPAAPermission{
		Resource: ResourcePHI,
		Action:   "read",
		Scope:    "own_patients",
	}

	err = store.AddPermissionToRole(ctx, role.ID, perm)
	require.NoError(t, err)

	// Get permissions
	permissions, err := store.GetRolePermissions(ctx, role.ID)
	require.NoError(t, err)
	require.Len(t, permissions, 1)
	assert.Equal(t, perm.Resource, permissions[0].Resource)
	assert.Equal(t, perm.Action, permissions[0].Action)

	// List roles
	roles, err := store.ListRoles(ctx, tenantID, nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(roles), 1)

	// Delete role
	err = store.DeleteRole(ctx, tenantID, role.ID)
	require.NoError(t, err)

	_, err = store.GetRole(ctx, tenantID, role.ID)
	assert.ErrorIs(t, err, ErrRoleNotFound)
}

func TestHIPAAUser_CRUD(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	ctx := context.Background()
	tenantID := "tenant-test-2"

	// Create user
	user := &HIPAAUser{
		ID:         "user-test-1",
		TenantID:   tenantID,
		Username:   "dr.smith",
		Email:      "smith@example.com",
		EmployeID:  "1234567890", // Valid NPI format
		Department: "Cardiology",
		JobTitle:   "Attending Physician",
		Status:     "active",
	}

	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Get user
	retrieved, err := store.GetUser(ctx, tenantID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.ID, retrieved.ID)
	assert.Equal(t, user.Username, retrieved.Username)
	assert.Equal(t, user.Email, retrieved.Email)
	assert.Equal(t, "Cardiology", retrieved.Department)

	// Update user
	user.JobTitle = "Senior Attending Physician"
	err = store.UpdateUser(ctx, user)
	require.NoError(t, err)

	retrieved, err = store.GetUser(ctx, tenantID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Senior Attending Physician", retrieved.JobTitle)

	// List users
	users, err := store.ListUsers(ctx, tenantID, nil)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(users), 1)

	// Delete user
	err = store.DeleteUser(ctx, tenantID, user.ID)
	require.NoError(t, err)

	_, err = store.GetUser(ctx, tenantID, user.ID)
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestHIPAAUserRole_Assignment(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	ctx := context.Background()
	tenantID := "tenant-test-3"

	// Create role
	role := &HIPAARole{
		ID:               "role-test-3",
		TenantID:         tenantID,
		Name:             "Test Nurse",
		Type:             RoleTypeNurse,
		MinimumNecessary: true,
		CreatedBy:        "admin",
	}
	err = store.CreateRole(ctx, role)
	require.NoError(t, err)

	// Create user
	user := &HIPAAUser{
		ID:       "user-test-3",
		TenantID: tenantID,
		Username: "nurse.jones",
		Email:    "jones@example.com",
		Status:   "active",
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Assign role to user
	assignment := &HIPAAUserRole{
		UserID:     user.ID,
		RoleID:     role.ID,
		TenantID:   tenantID,
		AssignedBy: "admin",
		IsActive:   true,
	}

	err = store.AssignRoleToUser(ctx, assignment)
	require.NoError(t, err)

	// Get user roles
	roles, err := store.GetUserRoles(ctx, tenantID, user.ID)
	require.NoError(t, err)
	require.Len(t, roles, 1)
	assert.Equal(t, role.ID, roles[0].ID)
	assert.Equal(t, role.Name, roles[0].Name)

	// Get users by role
	users, err := store.GetUsersByRole(ctx, tenantID, role.ID, nil)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, user.ID, users[0].ID)

	// Remove role from user
	err = store.RemoveRoleFromUser(ctx, tenantID, user.ID, role.ID)
	require.NoError(t, err)

	roles, err = store.GetUserRoles(ctx, tenantID, user.ID)
	require.NoError(t, err)
	assert.Empty(t, roles)
}

func TestRBACAuthorizer_Authorize(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	logger := zap.NewNop()
	authorizer, err := NewRBACAuthorizer(store, logger)
	require.NoError(t, err)

	ctx := context.Background()
	tenantID := "tenant-test-4"

	// Create role with permission
	role := &HIPAARole{
		ID:               "role-test-4",
		TenantID:         tenantID,
		Name:             "Test Doctor",
		Type:             RoleTypePhysician,
		MinimumNecessary: true,
		CreatedBy:        "admin",
	}
	err = store.CreateRole(ctx, role)
	require.NoError(t, err)

	// Add permission
	perm := HIPAAPermission{
		Resource: ResourcePHI,
		Action:   "read",
		Scope:    "own_patients",
	}
	err = store.AddPermissionToRole(ctx, role.ID, perm)
	require.NoError(t, err)

	// Create user
	user := &HIPAAUser{
		ID:       "user-test-4",
		TenantID: tenantID,
		Username: "dr.wilson",
		Email:    "wilson@example.com",
		Status:   "active",
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Assign role
	assignment := &HIPAAUserRole{
		UserID:     user.ID,
		RoleID:     role.ID,
		TenantID:   tenantID,
		AssignedBy: "admin",
		IsActive:   true,
	}
	err = store.AssignRoleToUser(ctx, assignment)
	require.NoError(t, err)

	// Authorize - should succeed
	reqPerm := HIPAAPermission{
		Resource: ResourcePHI,
		Action:   "read",
		Scope:    "own_patients",
	}

	err = authorizer.Authorize(ctx, tenantID, user.ID, reqPerm)
	require.NoError(t, err)

	// Authorize - should fail (wrong action)
	wrongPerm := HIPAAPermission{
		Resource: ResourcePHI,
		Action:   "delete",
		Scope:    "own_patients",
	}

	err = authorizer.Authorize(ctx, tenantID, user.ID, wrongPerm)
	assert.ErrorIs(t, err, ErrPermissionDenied)
}

func TestRBACAuthorizer_MinimumNecessary(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	logger := zap.NewNop()
	authorizer, err := NewRBACAuthorizer(store, logger)
	require.NoError(t, err)

	ctx := context.Background()
	tenantID := "tenant-test-5"

	// Create role with minimum necessary enabled
	role := &HIPAARole{
		ID:               "role-test-5",
		TenantID:         tenantID,
		Name:             "Limited Doctor",
		Type:             RoleTypePhysician,
		MinimumNecessary: true,
		CreatedBy:        "admin",
	}
	err = store.CreateRole(ctx, role)
	require.NoError(t, err)

	// Add permission that would violate minimum necessary
	perm := HIPAAPermission{
		Resource: ResourcePHI,
		Action:   "delete",
		Scope:    "all",
	}
	err = store.AddPermissionToRole(ctx, role.ID, perm)
	require.NoError(t, err)

	// Create user
	user := &HIPAAUser{
		ID:       "user-test-5",
		TenantID: tenantID,
		Username: "dr.limited",
		Email:    "limited@example.com",
		Status:   "active",
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Assign role
	assignment := &HIPAAUserRole{
		UserID:     user.ID,
		RoleID:     role.ID,
		TenantID:   tenantID,
		AssignedBy: "admin",
		IsActive:   true,
	}
	err = store.AssignRoleToUser(ctx, assignment)
	require.NoError(t, err)

	// Try to authorize - should fail minimum necessary check
	reqPerm := HIPAAPermission{
		Resource: ResourcePHI,
		Action:   "delete",
		Scope:    "all",
	}

	err = authorizer.Authorize(ctx, tenantID, user.ID, reqPerm)
	assert.ErrorIs(t, err, ErrMinimumNecessary)
}

func TestBreakGlass_Flow(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	logger := zap.NewNop()
	authorizer, err := NewRBACAuthorizer(store, logger)
	require.NoError(t, err)

	ctx := context.Background()
	tenantID := "tenant-test-6"

	// Create emergency role
	role := &HIPAARole{
		ID:               "role-emergency",
		TenantID:         tenantID,
		Name:             "Emergency",
		Type:             RoleTypeEmergency,
		MinimumNecessary: false,
		CreatedBy:        "admin",
	}
	err = store.CreateRole(ctx, role)
	require.NoError(t, err)

	// Create user
	user := &HIPAAUser{
		ID:       "user-test-6",
		TenantID: tenantID,
		Username: "dr.emergency",
		Email:    "emergency@example.com",
		Status:   "active",
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Request break-glass
	reason := "Life-threatening emergency"
	justification := "Patient unconscious, no time for normal authorization"

	auth, err := authorizer.RequestBreakGlass(ctx, tenantID, user.ID, role.ID, reason, justification)
	require.NoError(t, err)
	assert.Equal(t, "pending", auth.ReviewStatus)

	// Approve break-glass
	err = authorizer.ApproveBreakGlass(ctx, tenantID, auth.ID, "compliance.officer")
	require.NoError(t, err)

	// Retrieve and verify
	auth, err = store.GetBreakGlassAuthorization(ctx, tenantID, auth.ID)
	require.NoError(t, err)
	assert.Equal(t, "approved", auth.ReviewStatus)
	assert.Equal(t, "compliance.officer", auth.AuthorizedBy)

	// Authorize with break-glass
	reqPerm := HIPAAPermission{
		Resource: ResourcePHI,
		Action:   "read",
		Scope:    "all",
	}

	err = authorizer.AuthorizeWithBreakGlass(ctx, tenantID, user.ID, auth.ID, reqPerm)
	require.NoError(t, err)

	// Verify access was logged
	opts := &AccessLogOptions{
		BreakGlassOnly: true,
		Limit:          10,
	}
	logs, err := store.GetAccessLogs(ctx, tenantID, opts)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(logs), 1)
	assert.True(t, logs[0].BreakGlassUsed)
	assert.Equal(t, PurposeEmergency, logs[0].PurposeOfUse)
}

func TestAccessLogging(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	ctx := context.Background()
	tenantID := "tenant-test-7"

	// Create user
	user := &HIPAAUser{
		ID:       "user-test-7",
		TenantID: tenantID,
		Username: "dr.logger",
		Email:    "logger@example.com",
		Status:   "active",
	}
	err = store.CreateUser(ctx, user)
	require.NoError(t, err)

	// Log access
	entry := &AccessLogEntry{
		TenantID:     tenantID,
		UserID:       user.ID,
		RoleID:       "role-test",
		ResourceType: ResourcePHI,
		ResourceID:   "patient-123",
		Action:       "read",
		Result:       "allowed",
		PurposeOfUse: PurposeTreatment,
		AccessType:   "view",
		ClientIP:     "192.168.1.100",
		UserAgent:    "Mozilla/5.0",
		SessionID:    "session-abc",
		Timestamp:    time.Now(),
	}

	err = store.LogAccess(ctx, entry)
	require.NoError(t, err)

	// Get access logs
	opts := &AccessLogOptions{
		Limit:   10,
		OrderBy: "timestamp",
		Order:   "DESC",
	}

	logs, err := store.GetAccessLogs(ctx, tenantID, opts)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, user.ID, logs[0].UserID)
	assert.Equal(t, ResourcePHI, logs[0].ResourceType)
	assert.Equal(t, "patient-123", logs[0].ResourceID)
	assert.Equal(t, PurposeTreatment, logs[0].PurposeOfUse)

	// Get user access history
	history, err := store.GetUserAccessHistory(ctx, tenantID, user.ID, opts)
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, user.ID, history[0].UserID)
}

func TestPHIResource_Management(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	ctx := context.Background()
	tenantID := "tenant-test-8"

	// Register PHI resource
	resource := &PHIResource{
		TenantID:       tenantID,
		ResourceType:   "patient_record",
		ResourceID:     "patient-456",
		PatientID:      "patient-456",
		Classification: "sensitive",
	}

	err = store.RegisterPHIResource(ctx, resource)
	require.NoError(t, err)

	// Get PHI resource
	retrieved, err := store.GetPHIResource(ctx, tenantID, resource.ResourceID)
	require.NoError(t, err)
	assert.Equal(t, resource.ResourceType, retrieved.ResourceType)
	assert.Equal(t, "sensitive", retrieved.Classification)

	// Update classification
	err = store.UpdatePHIResourceClassification(ctx, tenantID, resource.ResourceID, "restricted")
	require.NoError(t, err)

	retrieved, err = store.GetPHIResource(ctx, tenantID, resource.ResourceID)
	require.NoError(t, err)
	assert.Equal(t, "restricted", retrieved.Classification)
}

func TestPermission_Validation(t *testing.T) {
	tests := []struct {
		name        string
		permission  HIPAAPermission
		expectError bool
	}{
		{
			name: "valid permission",
			permission: HIPAAPermission{
				Resource: ResourcePHI,
				Action:   "read",
				Scope:    "own_patients",
			},
			expectError: false,
		},
		{
			name: "missing resource",
			permission: HIPAAPermission{
				Action: "read",
				Scope:  "own_patients",
			},
			expectError: true,
		},
		{
			name: "missing action",
			permission: HIPAAPermission{
				Resource: ResourcePHI,
				Scope:    "own_patients",
			},
			expectError: true,
		},
		{
			name: "invalid action",
			permission: HIPAAPermission{
				Resource: ResourcePHI,
				Action:   "invalid_action",
				Scope:    "own_patients",
			},
			expectError: true,
		},
		{
			name: "invalid scope",
			permission: HIPAAPermission{
				Resource: ResourcePHI,
				Action:   "read",
				Scope:    "invalid_scope",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.permission.Validate()
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateNPI(t *testing.T) {
	tests := []struct {
		name        string
		npi         string
		expectError bool
	}{
		{"valid NPI", "1234567897", false}, // Valid NPI with correct check digit
		{"empty NPI", "", true},
		{"too short", "123456789", true},
		{"too long", "12345678901", true},
		{"non-numeric", "123456789A", true},
		{"invalid check digit", "1234567890", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNPI(tt.npi)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateSecureID(t *testing.T) {
	id1 := GenerateSecureID("test")
	// Small delay to ensure different timestamp
	time.Sleep(1 * time.Millisecond)
	id2 := GenerateSecureID("test")

	assert.NotEqual(t, id1, id2, "Generated IDs should be unique")
	assert.Contains(t, id1, "test_", "ID should contain prefix")
	assert.Greater(t, len(id1), 10, "ID should be reasonably long")
}

func TestSQLRBACStore_Close(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLRBACStore(db)
	require.NoError(t, err)

	// Close should not error (doesn't close DB connection)
	err = store.Close()
	assert.NoError(t, err)

	// DB should still be usable
	_, err = db.Exec("SELECT 1")
	assert.NoError(t, err)
}
