package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// SQLRBACStore implements RBACStore using SQLite
type SQLRBACStore struct {
	db *sql.DB
}

// NewSQLRBACStore creates a new SQL-based RBAC store
func NewSQLRBACStore(db *sql.DB) (*SQLRBACStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	return &SQLRBACStore{db: db}, nil
}

// CreateRole creates a new role
func (s *SQLRBACStore) CreateRole(ctx context.Context, role *HIPAARole) error {
	query := `
		INSERT INTO hipaa_roles (id, tenant_id, name, type, description, parent_role_id, minimum_necessary, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	if role.CreatedAt.IsZero() {
		role.CreatedAt = now
	}
	if role.UpdatedAt.IsZero() {
		role.UpdatedAt = now
	}

	_, err := s.db.ExecContext(ctx, query,
		role.ID, role.TenantID, role.Name, role.Type, role.Description,
		role.ParentRoleID, role.MinimumNecessary, role.CreatedBy,
		role.CreatedAt, role.UpdatedAt,
	)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrDuplicateRole
		}
		return fmt.Errorf("failed to create role: %w", err)
	}

	return nil
}

// GetRole retrieves a role by ID
func (s *SQLRBACStore) GetRole(ctx context.Context, tenantID, roleID string) (*HIPAARole, error) {
	query := `
		SELECT id, tenant_id, name, type, description, parent_role_id, minimum_necessary, 
		       created_by, created_at, updated_at
		FROM hipaa_roles
		WHERE id = ? AND tenant_id = ?
	`

	role := &HIPAARole{}
	var parentRoleID sql.NullString

	err := s.db.QueryRowContext(ctx, query, roleID, tenantID).Scan(
		&role.ID, &role.TenantID, &role.Name, &role.Type, &role.Description,
		&parentRoleID, &role.MinimumNecessary, &role.CreatedBy,
		&role.CreatedAt, &role.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	if parentRoleID.Valid {
		role.ParentRoleID = &parentRoleID.String
	}

	// Load permissions
	perms, err := s.GetRolePermissions(ctx, roleID)
	if err != nil {
		return nil, fmt.Errorf("failed to load role permissions: %w", err)
	}
	role.Permissions = perms

	return role, nil
}

// UpdateRole updates an existing role
func (s *SQLRBACStore) UpdateRole(ctx context.Context, role *HIPAARole) error {
	query := `
		UPDATE hipaa_roles
		SET name = ?, type = ?, description = ?, parent_role_id = ?, 
		    minimum_necessary = ?, updated_at = ?
		WHERE id = ? AND tenant_id = ?
	`

	role.UpdatedAt = time.Now()

	var parentRoleID interface{}
	if role.ParentRoleID != nil {
		parentRoleID = *role.ParentRoleID
	} else {
		parentRoleID = nil
	}

	result, err := s.db.ExecContext(ctx, query,
		role.Name, role.Type, role.Description, parentRoleID,
		role.MinimumNecessary, role.UpdatedAt, role.ID, role.TenantID,
	)

	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrRoleNotFound
	}

	return nil
}

// DeleteRole deletes a role
func (s *SQLRBACStore) DeleteRole(ctx context.Context, tenantID, roleID string) error {
	query := `DELETE FROM hipaa_roles WHERE id = ? AND tenant_id = ?`

	result, err := s.db.ExecContext(ctx, query, roleID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrRoleNotFound
	}

	return nil
}

// ListRoles lists all roles for a tenant
func (s *SQLRBACStore) ListRoles(ctx context.Context, tenantID string, opts *ListOptions) ([]*HIPAARole, error) {
	query := `
		SELECT id, tenant_id, name, type, description, parent_role_id, minimum_necessary, 
		       created_by, created_at, updated_at
		FROM hipaa_roles
		WHERE tenant_id = ?
	`

	args := []interface{}{tenantID}

	if opts != nil && opts.OrderBy != "" {
		query += fmt.Sprintf(" ORDER BY %s", opts.OrderBy)
		if opts.Order != "" {
			query += fmt.Sprintf(" %s", opts.Order)
		}
	}

	if opts != nil && opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	if opts != nil && opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roles []*HIPAARole
	for rows.Next() {
		role := &HIPAARole{}
		var parentRoleID sql.NullString

		err := rows.Scan(
			&role.ID, &role.TenantID, &role.Name, &role.Type, &role.Description,
			&parentRoleID, &role.MinimumNecessary, &role.CreatedBy,
			&role.CreatedAt, &role.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan role: %w", err)
		}

		if parentRoleID.Valid {
			role.ParentRoleID = &parentRoleID.String
		}

		// Load permissions
		perms, err := s.GetRolePermissions(ctx, role.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load permissions: %w", err)
		}
		role.Permissions = perms

		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating roles: %w", err)
	}

	return roles, nil
}

// AddPermissionToRole adds a permission to a role
func (s *SQLRBACStore) AddPermissionToRole(ctx context.Context, roleID string, permission HIPAAPermission) error {
	if err := permission.Validate(); err != nil {
		return ErrInvalidPermission
	}

	query := `
		INSERT INTO hipaa_permissions (id, role_id, resource, action, scope, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	id := GenerateSecureID("perm")
	_, err := s.db.ExecContext(ctx, query, id, roleID, permission.Resource, permission.Action, permission.Scope, time.Now())

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("permission already exists")
		}
		return fmt.Errorf("failed to add permission: %w", err)
	}

	return nil
}

// RemovePermissionFromRole removes a permission from a role
func (s *SQLRBACStore) RemovePermissionFromRole(ctx context.Context, roleID string, permission HIPAAPermission) error {
	query := `
		DELETE FROM hipaa_permissions
		WHERE role_id = ? AND resource = ? AND action = ? AND scope = ?
	`

	_, err := s.db.ExecContext(ctx, query, roleID, permission.Resource, permission.Action, permission.Scope)
	if err != nil {
		return fmt.Errorf("failed to remove permission: %w", err)
	}

	return nil
}

// GetRolePermissions retrieves all permissions for a role
func (s *SQLRBACStore) GetRolePermissions(ctx context.Context, roleID string) ([]HIPAAPermission, error) {
	query := `
		SELECT resource, action, scope
		FROM hipaa_permissions
		WHERE role_id = ?
	`

	rows, err := s.db.QueryContext(ctx, query, roleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var permissions []HIPAAPermission
	for rows.Next() {
		var perm HIPAAPermission
		err := rows.Scan(&perm.Resource, &perm.Action, &perm.Scope)
		if err != nil {
			return nil, fmt.Errorf("failed to scan permission: %w", err)
		}
		permissions = append(permissions, perm)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating permissions: %w", err)
	}

	return permissions, nil
}

// CreateUser creates a new user
func (s *SQLRBACStore) CreateUser(ctx context.Context, user *HIPAAUser) error {
	query := `
		INSERT INTO hipaa_users (id, tenant_id, username, email, employee_id, department, 
		                         job_title, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}

	var employeeID interface{}
	if user.EmployeID != "" {
		employeeID = user.EmployeID
	} else {
		employeeID = nil
	}

	_, err := s.db.ExecContext(ctx, query,
		user.ID, user.TenantID, user.Username, user.Email, employeeID,
		user.Department, user.JobTitle, user.Status, user.CreatedAt, user.UpdatedAt,
	)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("user already exists (username, email, or employee_id)")
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetUser retrieves a user by ID
func (s *SQLRBACStore) GetUser(ctx context.Context, tenantID, userID string) (*HIPAAUser, error) {
	query := `
		SELECT id, tenant_id, username, email, employee_id, department, job_title, 
		       status, last_login_at, last_password_change, failed_login_attempts, 
		       account_locked_until, created_at, updated_at
		FROM hipaa_users
		WHERE id = ? AND tenant_id = ?
	`

	user := &HIPAAUser{}
	var employeeID, lastLoginAt, lastPasswordChange, accountLockedUntil sql.NullString

	err := s.db.QueryRowContext(ctx, query, userID, tenantID).Scan(
		&user.ID, &user.TenantID, &user.Username, &user.Email, &employeeID,
		&user.Department, &user.JobTitle, &user.Status,
		&lastLoginAt, &lastPasswordChange, &user.FailedLoginAttempts,
		&accountLockedUntil, &user.CreatedAt, &user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if employeeID.Valid {
		user.EmployeID = employeeID.String
	}
	if lastLoginAt.Valid {
		t, _ := time.Parse(time.RFC3339, lastLoginAt.String)
		user.LastLoginAt = &t
	}
	if lastPasswordChange.Valid {
		t, _ := time.Parse(time.RFC3339, lastPasswordChange.String)
		user.LastPasswordChange = &t
	}
	if accountLockedUntil.Valid {
		t, _ := time.Parse(time.RFC3339, accountLockedUntil.String)
		user.AccountLockedUntil = &t
	}

	return user, nil
}

// UpdateUser updates an existing user
func (s *SQLRBACStore) UpdateUser(ctx context.Context, user *HIPAAUser) error {
	query := `
		UPDATE hipaa_users
		SET username = ?, email = ?, employee_id = ?, department = ?, 
		    job_title = ?, status = ?, last_login_at = ?, last_password_change = ?,
		    failed_login_attempts = ?, account_locked_until = ?, updated_at = ?
		WHERE id = ? AND tenant_id = ?
	`

	user.UpdatedAt = time.Now()

	var employeeID, lastLoginAt, lastPasswordChange, accountLockedUntil interface{}
	if user.EmployeID != "" {
		employeeID = user.EmployeID
	}
	if user.LastLoginAt != nil {
		lastLoginAt = user.LastLoginAt.Format(time.RFC3339)
	}
	if user.LastPasswordChange != nil {
		lastPasswordChange = user.LastPasswordChange.Format(time.RFC3339)
	}
	if user.AccountLockedUntil != nil {
		accountLockedUntil = user.AccountLockedUntil.Format(time.RFC3339)
	}

	result, err := s.db.ExecContext(ctx, query,
		user.Username, user.Email, employeeID, user.Department,
		user.JobTitle, user.Status, lastLoginAt, lastPasswordChange,
		user.FailedLoginAttempts, accountLockedUntil, user.UpdatedAt,
		user.ID, user.TenantID,
	)

	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

// DeleteUser deletes a user
func (s *SQLRBACStore) DeleteUser(ctx context.Context, tenantID, userID string) error {
	query := `DELETE FROM hipaa_users WHERE id = ? AND tenant_id = ?`

	result, err := s.db.ExecContext(ctx, query, userID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

// ListUsers lists all users for a tenant
func (s *SQLRBACStore) ListUsers(ctx context.Context, tenantID string, opts *ListOptions) ([]*HIPAAUser, error) {
	query := `
		SELECT id, tenant_id, username, email, employee_id, department, job_title, 
		       status, last_login_at, last_password_change, failed_login_attempts, 
		       account_locked_until, created_at, updated_at
		FROM hipaa_users
		WHERE tenant_id = ?
	`

	args := []interface{}{tenantID}

	if opts != nil && opts.OrderBy != "" {
		query += fmt.Sprintf(" ORDER BY %s", opts.OrderBy)
		if opts.Order != "" {
			query += fmt.Sprintf(" %s", opts.Order)
		}
	}

	if opts != nil && opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	if opts != nil && opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []*HIPAAUser
	for rows.Next() {
		user := &HIPAAUser{}
		var employeeID, lastLoginAt, lastPasswordChange, accountLockedUntil sql.NullString

		err := rows.Scan(
			&user.ID, &user.TenantID, &user.Username, &user.Email, &employeeID,
			&user.Department, &user.JobTitle, &user.Status,
			&lastLoginAt, &lastPasswordChange, &user.FailedLoginAttempts,
			&accountLockedUntil, &user.CreatedAt, &user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if employeeID.Valid {
			user.EmployeID = employeeID.String
		}
		if lastLoginAt.Valid {
			t, _ := time.Parse(time.RFC3339, lastLoginAt.String)
			user.LastLoginAt = &t
		}
		if lastPasswordChange.Valid {
			t, _ := time.Parse(time.RFC3339, lastPasswordChange.String)
			user.LastPasswordChange = &t
		}
		if accountLockedUntil.Valid {
			t, _ := time.Parse(time.RFC3339, accountLockedUntil.String)
			user.AccountLockedUntil = &t
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users: %w", err)
	}

	return users, nil
}

// AssignRoleToUser assigns a role to a user
func (s *SQLRBACStore) AssignRoleToUser(ctx context.Context, assignment *HIPAAUserRole) error {
	query := `
		INSERT INTO hipaa_user_roles (id, user_id, role_id, tenant_id, assigned_by, assigned_at, expires_at, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	id := GenerateSecureID("ur")
	now := time.Now()
	if assignment.AssignedAt.IsZero() {
		assignment.AssignedAt = now
	}

	var expiresAt interface{}
	if assignment.ExpiresAt != nil {
		expiresAt = assignment.ExpiresAt.Format(time.RFC3339)
	} else {
		expiresAt = nil
	}

	_, err := s.db.ExecContext(ctx, query, id, assignment.UserID, assignment.RoleID, assignment.TenantID, assignment.AssignedBy, assignment.AssignedAt, expiresAt, assignment.IsActive)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("role already assigned to user")
		}
		return fmt.Errorf("failed to assign role: %w", err)
	}

	return nil
}

// RemoveRoleFromUser removes a role from a user
func (s *SQLRBACStore) RemoveRoleFromUser(ctx context.Context, tenantID, userID, roleID string) error {
	query := `
		UPDATE hipaa_user_roles
		SET is_active = 0
		WHERE user_id = ? AND role_id = ? AND tenant_id = ?
	`

	_, err := s.db.ExecContext(ctx, query, userID, roleID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to remove role: %w", err)
	}

	return nil
}

// GetUserRoles retrieves all roles for a user
func (s *SQLRBACStore) GetUserRoles(ctx context.Context, tenantID, userID string) ([]*HIPAARole, error) {
	query := `
		SELECT r.id, r.tenant_id, r.name, r.type, r.description, r.parent_role_id, 
		       r.minimum_necessary, r.created_by, r.created_at, r.updated_at
		FROM hipaa_roles r
		INNER JOIN hipaa_user_roles ur ON r.id = ur.role_id
		WHERE ur.user_id = ? AND ur.tenant_id = ? AND ur.is_active = 1
	`

	rows, err := s.db.QueryContext(ctx, query, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var roles []*HIPAARole
	for rows.Next() {
		role := &HIPAARole{}
		var parentRoleID sql.NullString

		err := rows.Scan(
			&role.ID, &role.TenantID, &role.Name, &role.Type, &role.Description,
			&parentRoleID, &role.MinimumNecessary, &role.CreatedBy,
			&role.CreatedAt, &role.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan role: %w", err)
		}

		if parentRoleID.Valid {
			role.ParentRoleID = &parentRoleID.String
		}

		// Load permissions
		perms, err := s.GetRolePermissions(ctx, role.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load permissions: %w", err)
		}
		role.Permissions = perms

		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating roles: %w", err)
	}

	return roles, nil
}

// GetUsersByRole retrieves all users with a specific role
func (s *SQLRBACStore) GetUsersByRole(ctx context.Context, tenantID, roleID string, opts *ListOptions) ([]*HIPAAUser, error) {
	query := `
		SELECT u.id, u.tenant_id, u.username, u.email, u.employee_id, u.department, 
		       u.job_title, u.status, u.last_login_at, u.last_password_change, 
		       u.failed_login_attempts, u.account_locked_until, u.created_at, u.updated_at
		FROM hipaa_users u
		INNER JOIN hipaa_user_roles ur ON u.id = ur.user_id
		WHERE ur.role_id = ? AND ur.tenant_id = ? AND ur.is_active = 1
	`

	args := []interface{}{roleID, tenantID}

	if opts != nil && opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	if opts != nil && opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by role: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []*HIPAAUser
	for rows.Next() {
		user := &HIPAAUser{}
		var employeeID, lastLoginAt, lastPasswordChange, accountLockedUntil sql.NullString

		err := rows.Scan(
			&user.ID, &user.TenantID, &user.Username, &user.Email, &employeeID,
			&user.Department, &user.JobTitle, &user.Status,
			&lastLoginAt, &lastPasswordChange, &user.FailedLoginAttempts,
			&accountLockedUntil, &user.CreatedAt, &user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}

		if employeeID.Valid {
			user.EmployeID = employeeID.String
		}
		if lastLoginAt.Valid {
			t, _ := time.Parse(time.RFC3339, lastLoginAt.String)
			user.LastLoginAt = &t
		}
		if lastPasswordChange.Valid {
			t, _ := time.Parse(time.RFC3339, lastPasswordChange.String)
			user.LastPasswordChange = &t
		}
		if accountLockedUntil.Valid {
			t, _ := time.Parse(time.RFC3339, accountLockedUntil.String)
			user.AccountLockedUntil = &t
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users: %w", err)
	}

	return users, nil
}

// RequestBreakGlass creates a break-glass authorization request
func (s *SQLRBACStore) RequestBreakGlass(ctx context.Context, auth *BreakGlassAuthorization) error {
	query := `
		INSERT INTO hipaa_break_glass (id, user_id, tenant_id, role_id, reason, justification, 
		                                authorized_at, expires_at, review_status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	id := GenerateSecureID("bg")
	auth.ID = id

	_, err := s.db.ExecContext(ctx, query, id, auth.UserID, auth.TenantID, auth.RoleID, auth.Reason, auth.Justification, auth.AuthorizedAt, auth.ExpiresAt, auth.ReviewStatus, auth.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to request break-glass: %w", err)
	}

	return nil
}

// GetBreakGlassAuthorization retrieves a break-glass authorization
func (s *SQLRBACStore) GetBreakGlassAuthorization(ctx context.Context, tenantID, authID string) (*BreakGlassAuthorization, error) {
	query := `
		SELECT id, user_id, tenant_id, role_id, reason, justification, authorized_by, 
		       authorized_at, expires_at, reviewed_by, reviewed_at, review_status, created_at
		FROM hipaa_break_glass
		WHERE id = ? AND tenant_id = ?
	`

	auth := &BreakGlassAuthorization{}
	var reviewedBy sql.NullString
	var reviewedAt sql.NullString

	err := s.db.QueryRowContext(ctx, query, authID, tenantID).Scan(
		&auth.ID, &auth.UserID, &auth.TenantID, &auth.RoleID, &auth.Reason, &auth.Justification,
		&auth.AuthorizedBy, &auth.AuthorizedAt, &auth.ExpiresAt,
		&reviewedBy, &reviewedAt, &auth.ReviewStatus, &auth.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("break-glass authorization not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get break-glass authorization: %w", err)
	}

	if reviewedBy.Valid {
		auth.ReviewedBy = &reviewedBy.String
	}
	if reviewedAt.Valid {
		t, _ := time.Parse(time.RFC3339, reviewedAt.String)
		auth.ReviewedAt = &t
	}

	return auth, nil
}

// ApproveBreakGlass approves a break-glass request
func (s *SQLRBACStore) ApproveBreakGlass(ctx context.Context, tenantID, authID, approvedBy string) error {
	query := `
		UPDATE hipaa_break_glass
		SET authorized_by = ?, authorized_at = ?, review_status = 'approved', reviewed_at = ?
		WHERE id = ? AND tenant_id = ?
	`

	now := time.Now()
	_, err := s.db.ExecContext(ctx, query, approvedBy, now, now, authID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to approve break-glass: %w", err)
	}

	return nil
}

// RejectBreakGlass rejects a break-glass request
func (s *SQLRBACStore) RejectBreakGlass(ctx context.Context, tenantID, authID, rejectedBy, reason string) error {
	query := `
		UPDATE hipaa_break_glass
		SET reviewed_by = ?, reviewed_at = ?, review_status = 'rejected', reason = ?
		WHERE id = ? AND tenant_id = ?
	`

	now := time.Now()
	_, err := s.db.ExecContext(ctx, query, rejectedBy, now, reason, authID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to reject break-glass: %w", err)
	}

	return nil
}

// ExpireBreakGlass expires a break-glass authorization
func (s *SQLRBACStore) ExpireBreakGlass(ctx context.Context, tenantID, authID string) error {
	query := `
		UPDATE hipaa_break_glass
		SET expires_at = ?
		WHERE id = ? AND tenant_id = ?
	`

	now := time.Now()
	_, err := s.db.ExecContext(ctx, query, now, authID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to expire break-glass: %w", err)
	}

	return nil
}

// RegisterPHIResource registers a PHI resource
func (s *SQLRBACStore) RegisterPHIResource(ctx context.Context, resource *PHIResource) error {
	query := `
		INSERT INTO hipaa_phi_resources (id, tenant_id, resource_type, resource_id, patient_id, classification, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	id := GenerateSecureID("phi")
	now := time.Now()

	_, err := s.db.ExecContext(ctx, query, id, resource.TenantID, resource.ResourceType, resource.ResourceID, resource.PatientID, resource.Classification, now, now)

	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("PHI resource already exists")
		}
		return fmt.Errorf("failed to register PHI resource: %w", err)
	}

	return nil
}

// GetPHIResource retrieves a PHI resource
func (s *SQLRBACStore) GetPHIResource(ctx context.Context, tenantID, resourceID string) (*PHIResource, error) {
	query := `
		SELECT id, tenant_id, resource_type, resource_id, patient_id, classification, 
		       access_policy_id, created_at, updated_at
		FROM hipaa_phi_resources
		WHERE resource_id = ? AND tenant_id = ?
	`

	resource := &PHIResource{}
	var accessPolicyID sql.NullString

	err := s.db.QueryRowContext(ctx, query, resourceID, tenantID).Scan(
		&resource.ID, &resource.TenantID, &resource.ResourceType, &resource.ResourceID,
		&resource.PatientID, &resource.Classification, &accessPolicyID,
		&resource.CreatedAt, &resource.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("PHI resource not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get PHI resource: %w", err)
	}

	if accessPolicyID.Valid {
		resource.AccessPolicyID = &accessPolicyID.String
	}

	return resource, nil
}

// UpdatePHIResourceClassification updates the classification of a PHI resource
func (s *SQLRBACStore) UpdatePHIResourceClassification(ctx context.Context, tenantID, resourceID, classification string) error {
	query := `
		UPDATE hipaa_phi_resources
		SET classification = ?, updated_at = ?
		WHERE resource_id = ? AND tenant_id = ?
	`

	_, err := s.db.ExecContext(ctx, query, classification, time.Now(), resourceID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to update PHI classification: %w", err)
	}

	return nil
}

// LogAccess logs an access event
func (s *SQLRBACStore) LogAccess(ctx context.Context, entry *AccessLogEntry) error {
	query := `
		INSERT INTO hipaa_access_logs (id, tenant_id, user_id, role_id, resource_type, resource_id, 
		                                action, result, reason, break_glass_used, break_glass_id, 
		                                purpose_of_use, access_type, client_ip, user_agent, session_id, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	id := GenerateSecureID("log")
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	var breakGlassID interface{}
	if entry.BreakGlassID != nil {
		breakGlassID = *entry.BreakGlassID
	} else {
		breakGlassID = nil
	}

	_, err := s.db.ExecContext(ctx, query, id, entry.TenantID, entry.UserID, entry.RoleID, entry.ResourceType, entry.ResourceID, entry.Action, entry.Result, entry.Reason, entry.BreakGlassUsed, breakGlassID, entry.PurposeOfUse, entry.AccessType, entry.ClientIP, entry.UserAgent, entry.SessionID, entry.Timestamp)

	if err != nil {
		return fmt.Errorf("failed to log access: %w", err)
	}

	return nil
}

// GetAccessLogs retrieves access logs with filtering options
func (s *SQLRBACStore) GetAccessLogs(ctx context.Context, tenantID string, opts *AccessLogOptions) ([]*AccessLogEntry, error) {
	query := `
		SELECT id, tenant_id, user_id, role_id, resource_type, resource_id, action, 
		       result, reason, break_glass_used, break_glass_id, purpose_of_use, 
		       access_type, client_ip, user_agent, session_id, timestamp
		FROM hipaa_access_logs
		WHERE tenant_id = ?
	`

	args := []interface{}{tenantID}

	if opts != nil {
		if opts.StartTime != nil {
			query += " AND timestamp >= ?"
			args = append(args, opts.StartTime)
		}
		if opts.EndTime != nil {
			query += " AND timestamp <= ?"
			args = append(args, opts.EndTime)
		}
		if opts.ResourceType != "" {
			query += " AND resource_type = ?"
			args = append(args, opts.ResourceType)
		}
		if opts.Action != "" {
			query += " AND action = ?"
			args = append(args, opts.Action)
		}
		if opts.Result != "" {
			query += " AND result = ?"
			args = append(args, opts.Result)
		}
		if opts.BreakGlassOnly {
			query += " AND break_glass_used = 1"
		}
		if opts.UserID != "" {
			query += " AND user_id = ?"
			args = append(args, opts.UserID)
		}
	}

	if opts != nil && opts.OrderBy != "" {
		query += fmt.Sprintf(" ORDER BY %s", opts.OrderBy)
		if opts.Order != "" {
			query += fmt.Sprintf(" %s", opts.Order)
		}
	} else {
		query += " ORDER BY timestamp DESC"
	}

	if opts != nil && opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	if opts != nil && opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get access logs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var logs []*AccessLogEntry
	for rows.Next() {
		entry := &AccessLogEntry{}
		var breakGlassID sql.NullString

		err := rows.Scan(
			&entry.ID, &entry.TenantID, &entry.UserID, &entry.RoleID, &entry.ResourceType,
			&entry.ResourceID, &entry.Action, &entry.Result, &entry.Reason,
			&entry.BreakGlassUsed, &breakGlassID, &entry.PurposeOfUse, &entry.AccessType,
			&entry.ClientIP, &entry.UserAgent, &entry.SessionID, &entry.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan access log: %w", err)
		}

		if breakGlassID.Valid {
			entry.BreakGlassID = &breakGlassID.String
		}

		logs = append(logs, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating access logs: %w", err)
	}

	return logs, nil
}

// GetUserAccessHistory retrieves access history for a specific user
func (s *SQLRBACStore) GetUserAccessHistory(ctx context.Context, tenantID, userID string, opts *AccessLogOptions) ([]*AccessLogEntry, error) {
	if opts == nil {
		opts = &AccessLogOptions{}
	}
	opts.UserID = userID

	return s.GetAccessLogs(ctx, tenantID, opts)
}

// GetResourceAccessHistory retrieves access history for a specific resource
func (s *SQLRBACStore) GetResourceAccessHistory(ctx context.Context, tenantID, resourceID string, opts *AccessLogOptions) ([]*AccessLogEntry, error) {
	baseQuery := `
		SELECT id, tenant_id, user_id, role_id, resource_type, resource_id, action, 
		       result, reason, break_glass_used, break_glass_id, purpose_of_use, 
		       access_type, client_ip, user_agent, session_id, timestamp
		FROM hipaa_access_logs
		WHERE resource_id = ? AND tenant_id = ?
	`

	// Reuse GetAccessLogs logic by building args appropriately
	// This is a simplified implementation
	args := []interface{}{resourceID, tenantID}

	if opts != nil && opts.Limit > 0 {
		baseQuery += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource access history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var logs []*AccessLogEntry
	for rows.Next() {
		entry := &AccessLogEntry{}
		var breakGlassID sql.NullString

		err := rows.Scan(
			&entry.ID, &entry.TenantID, &entry.UserID, &entry.RoleID, &entry.ResourceType,
			&entry.ResourceID, &entry.Action, &entry.Result, &entry.Reason,
			&entry.BreakGlassUsed, &breakGlassID, &entry.PurposeOfUse, &entry.AccessType,
			&entry.ClientIP, &entry.UserAgent, &entry.SessionID, &entry.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan access log: %w", err)
		}

		if breakGlassID.Valid {
			entry.BreakGlassID = &breakGlassID.String
		}

		logs = append(logs, entry)
	}

	return logs, nil
}

// Close closes the store
func (s *SQLRBACStore) Close() error {
	// Don't close the DB connection as it's managed elsewhere
	return nil
}
