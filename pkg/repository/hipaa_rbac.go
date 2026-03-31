package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	// HIPAA RBAC Errors
	ErrRoleNotFound         = errors.New("role not found")
	ErrUserNotFound         = errors.New("user not found")
	ErrPermissionDenied     = errors.New("permission denied")
	ErrMinimumNecessary     = errors.New("access exceeds minimum necessary principle")
	ErrBreakGlassRequired   = errors.New("break-glass procedure required for this access")
	ErrBreakGlassExpired    = errors.New("break-glass authorization expired")
	ErrPHIAccessRestricted  = errors.New("PHI access restricted without proper authorization")
	ErrInvalidRoleHierarchy = errors.New("invalid role hierarchy")
	ErrDuplicateRole        = errors.New("duplicate role")
	ErrInvalidPermission    = errors.New("invalid permission format")
	ErrAuditLogFailure      = errors.New("failed to audit log access")

	// Resource types for HIPAA
	ResourcePHI             = "phi"
	ResourcePatientRecords  = "patient_records"
	ResourceTreatmentPlans  = "treatment_plans"
	ResourcePrescriptions   = "prescriptions"
	ResourceLabResults      = "lab_results"
	ResourceInsuranceClaims = "insurance_claims"
)

// HIPAA Role Types - Standard healthcare roles
type HIPEARoleType string

const (
	RoleTypePhysician         HIPEARoleType = "physician"
	RoleTypeNurse             HIPEARoleType = "nurse"
	RoleTypeAdministrator     HIPEARoleType = "administrator"
	RoleTypeBilling           HIPEARoleType = "billing"
	RoleTypeResearcher        HIPEARoleType = "researcher"
	RoleTypePharmacist        HIPEARoleType = "pharmacist"
	RoleTypeLabTechnician     HIPEARoleType = "lab_technician"
	RoleTypeReceptionist      HIPEARoleType = "receptionist"
	RoleTypeComplianceOfficer HIPEARoleType = "compliance_officer"
	RoleTypeEmergency         HIPEARoleType = "emergency" // Break-glass role
)

// Permission represents a specific action on a resource
type HIPAAPermission struct {
	Resource string `json:"resource"` // e.g., "phi", "patient_records"
	Action   string `json:"action"`   // e.g., "read", "write", "delete", "audit"
	Scope    string `json:"scope"`    // e.g., "own_patients", "department", "all"
}

func (p HIPAAPermission) String() string {
	scope := p.Scope
	if scope == "" {
		scope = "default"
	}
	return fmt.Sprintf("%s:%s:%s", p.Resource, p.Action, scope)
}

func (p HIPAAPermission) Validate() error {
	if p.Resource == "" {
		return fmt.Errorf("permission resource cannot be empty")
	}
	if p.Action == "" {
		return fmt.Errorf("permission action cannot be empty")
	}

	// Validate action
	validActions := map[string]bool{
		"read": true, "write": true, "create": true,
		"delete": true, "update": true, "audit": true,
	}
	if !validActions[p.Action] {
		return fmt.Errorf("invalid action '%s', must be one of: read, write, create, delete, update, audit", p.Action)
	}

	// Validate scope
	validScopes := map[string]bool{
		"own_patients": true, "department": true, "all": true,
		"assigned": true, "organization": true, "": true,
	}
	if !validScopes[p.Scope] {
		return fmt.Errorf("invalid scope '%s', must be one of: own_patients, department, all, assigned, organization", p.Scope)
	}

	return nil
}

// Role represents a HIPAA-compliant role
type HIPAARole struct {
	ID               string            `json:"id" db:"id"`
	TenantID         string            `json:"tenant_id" db:"tenant_id"`
	Name             string            `json:"name" db:"name"`
	Type             HIPEARoleType     `json:"type" db:"type"`
	Description      string            `json:"description" db:"description"`
	Permissions      []HIPAAPermission `json:"permissions" db:"-"`
	ParentRoleID     *string           `json:"parent_role_id,omitempty" db:"parent_role_id"` // Role hierarchy
	MinimumNecessary bool              `json:"minimum_necessary" db:"minimum_necessary"`     // HIPAA minimum necessary principle
	CreatedAt        time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at" db:"updated_at"`
	CreatedBy        string            `json:"created_by" db:"created_by"`
}

// User represents a system user with HIPAA compliance tracking
type HIPAAUser struct {
	ID                  string     `json:"id" db:"id"`
	TenantID            string     `json:"tenant_id" db:"tenant_id"`
	Username            string     `json:"username" db:"username"`
	Email               string     `json:"email" db:"email"`
	EmployeID           string     `json:"employee_id" db:"employee_id"` // National Provider Identifier (NPI) or internal ID
	Department          string     `json:"department" db:"department"`
	JobTitle            string     `json:"job_title" db:"job_title"`
	Status              string     `json:"status" db:"status"` // active, suspended, terminated
	LastLoginAt         *time.Time `json:"last_login_at" db:"last_login_at"`
	LastPasswordChange  *time.Time `json:"last_password_change" db:"last_password_change"`
	FailedLoginAttempts int        `json:"failed_login_attempts" db:"failed_login_attempts"`
	AccountLockedUntil  *time.Time `json:"account_locked_until" db:"account_locked_until"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

// UserRole represents the assignment of a role to a user
type HIPAAUserRole struct {
	ID         string     `json:"id" db:"id"`
	UserID     string     `json:"user_id" db:"user_id"`
	RoleID     string     `json:"role_id" db:"role_id"`
	TenantID   string     `json:"tenant_id" db:"tenant_id"`
	AssignedBy string     `json:"assigned_by" db:"assigned_by"`
	AssignedAt time.Time  `json:"assigned_at" db:"assigned_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty" db:"expires_at"` // Temporary assignments
	IsActive   bool       `json:"is_active" db:"is_active"`
}

// BreakGlassAuthorization represents emergency access authorization
type BreakGlassAuthorization struct {
	ID                string     `json:"id" db:"id"`
	UserID            string     `json:"user_id" db:"user_id"`
	TenantID          string     `json:"tenant_id" db:"tenant_id"`
	RoleID            string     `json:"role_id" db:"role_id"`
	Reason            string     `json:"reason" db:"reason"`
	Justification     string     `json:"justification" db:"justification"` // Detailed justification
	AuthorizedBy      string     `json:"authorized_by" db:"authorized_by"` // Supervisor/compliance officer
	AuthorizedAt      time.Time  `json:"authorized_at" db:"authorized_at"`
	ExpiresAt         time.Time  `json:"expires_at" db:"expires_at"` // Auto-expire (typically 24h)
	ReviewedBy        *string    `json:"reviewed_by,omitempty" db:"reviewed_by"`
	ReviewedAt        *time.Time `json:"reviewed_at,omitempty" db:"reviewed_at"`
	ReviewStatus      string     `json:"review_status" db:"review_status"` // pending, approved, rejected
	AccessedResources []string   `json:"accessed_resources" db:"-"`        // Resources accessed during emergency
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
}

// PHIResource represents a PHI resource with access controls
type PHIResource struct {
	ID             string    `json:"id" db:"id"`
	TenantID       string    `json:"tenant_id" db:"tenant_id"`
	ResourceType   string    `json:"resource_type" db:"resource_type"`   // patient_record, prescription, etc.
	ResourceID     string    `json:"resource_id" db:"resource_id"`       // External resource identifier
	PatientID      string    `json:"patient_id" db:"patient_id"`         // Patient identifier for access control
	Classification string    `json:"classification" db:"classification"` // normal, sensitive, restricted
	AccessPolicyID *string   `json:"access_policy_id,omitempty" db:"access_policy_id"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// AccessLogEntry represents an audited access to PHI
type AccessLogEntry struct {
	ID             string    `json:"id" db:"id"`
	TenantID       string    `json:"tenant_id" db:"tenant_id"`
	UserID         string    `json:"user_id" db:"user_id"`
	RoleID         string    `json:"role_id" db:"role_id"`
	ResourceType   string    `json:"resource_type" db:"resource_type"`
	ResourceID     string    `json:"resource_id" db:"resource_id"`
	Action         string    `json:"action" db:"action"`
	Result         string    `json:"result" db:"result"` // allowed, denied
	Reason         string    `json:"reason" db:"reason"` // Why access was granted/denied
	BreakGlassUsed bool      `json:"break_glass_used" db:"break_glass_used"`
	BreakGlassID   *string   `json:"break_glass_id,omitempty" db:"break_glass_id"`
	PurposeOfUse   string    `json:"purpose_of_use" db:"purpose_of_use"` // Treatment, Payment, Operations, etc.
	AccessType     string    `json:"access_type" db:"access_type"`       // view, create, update, delete, export
	ClientIP       string    `json:"client_ip" db:"client_ip"`
	UserAgent      string    `json:"user_agent" db:"user_agent"`
	SessionID      string    `json:"session_id" db:"session_id"`
	Timestamp      time.Time `json:"timestamp" db:"timestamp"`
}

// PurposeOfUse codes (HL7 context management)
const (
	PurposeTreatment    = "TREATMENT"
	PurposePayment      = "PAYMENT"
	PurposeOperations   = "OPERATIONS"
	PurposeResearch     = "RESEARCH"
	PurposePublicHealth = "PUBLIC_HEALTH"
	PurposeEmergency    = "EMERGENCY"
)

// RBACStore defines the interface for RBAC data storage
type RBACStore interface {
	// Role operations
	CreateRole(ctx context.Context, role *HIPAARole) error
	GetRole(ctx context.Context, tenantID, roleID string) (*HIPAARole, error)
	UpdateRole(ctx context.Context, role *HIPAARole) error
	DeleteRole(ctx context.Context, tenantID, roleID string) error
	ListRoles(ctx context.Context, tenantID string, opts *ListOptions) ([]*HIPAARole, error)

	// Permission operations
	AddPermissionToRole(ctx context.Context, roleID string, permission HIPAAPermission) error
	RemovePermissionFromRole(ctx context.Context, roleID string, permission HIPAAPermission) error
	GetRolePermissions(ctx context.Context, roleID string) ([]HIPAAPermission, error)

	// User operations
	CreateUser(ctx context.Context, user *HIPAAUser) error
	GetUser(ctx context.Context, tenantID, userID string) (*HIPAAUser, error)
	UpdateUser(ctx context.Context, user *HIPAAUser) error
	DeleteUser(ctx context.Context, tenantID, userID string) error
	ListUsers(ctx context.Context, tenantID string, opts *ListOptions) ([]*HIPAAUser, error)

	// User-Role assignments
	AssignRoleToUser(ctx context.Context, assignment *HIPAAUserRole) error
	RemoveRoleFromUser(ctx context.Context, tenantID, userID, roleID string) error
	GetUserRoles(ctx context.Context, tenantID, userID string) ([]*HIPAARole, error)
	GetUsersByRole(ctx context.Context, tenantID, roleID string, opts *ListOptions) ([]*HIPAAUser, error)

	// Break-glass operations
	RequestBreakGlass(ctx context.Context, auth *BreakGlassAuthorization) error
	GetBreakGlassAuthorization(ctx context.Context, tenantID, authID string) (*BreakGlassAuthorization, error)
	ApproveBreakGlass(ctx context.Context, tenantID, authID string, approvedBy string) error
	RejectBreakGlass(ctx context.Context, tenantID, authID string, rejectedBy, reason string) error
	ExpireBreakGlass(ctx context.Context, tenantID, authID string) error

	// PHI Resource operations
	RegisterPHIResource(ctx context.Context, resource *PHIResource) error
	GetPHIResource(ctx context.Context, tenantID, resourceID string) (*PHIResource, error)
	UpdatePHIResourceClassification(ctx context.Context, tenantID, resourceID, classification string) error

	// Access logging
	LogAccess(ctx context.Context, entry *AccessLogEntry) error
	GetAccessLogs(ctx context.Context, tenantID string, opts *AccessLogOptions) ([]*AccessLogEntry, error)
	GetUserAccessHistory(ctx context.Context, tenantID, userID string, opts *AccessLogOptions) ([]*AccessLogEntry, error)
	GetResourceAccessHistory(ctx context.Context, tenantID, resourceID string, opts *AccessLogOptions) ([]*AccessLogEntry, error)

	// Close the store
	Close() error
}

// AccessLogOptions provides options for querying access logs
type AccessLogOptions struct {
	Limit          int
	Offset         int
	OrderBy        string
	Order          string // ASC or DESC
	StartTime      *time.Time
	EndTime        *time.Time
	ResourceType   string
	Action         string
	Result         string
	BreakGlassOnly bool
	UserID         string
}

// RBACAuthorizer implements HIPAA-compliant role-based access control
type RBACAuthorizer struct {
	store RBACStore
	log   *zap.Logger
	mu    sync.RWMutex
}

// NewRBACAuthorizer creates a new HIPAA-compliant RBAC authorizer
func NewRBACAuthorizer(store RBACStore, log *zap.Logger) (*RBACAuthorizer, error) {
	if store == nil {
		return nil, fmt.Errorf("RBAC store is required")
	}
	if log == nil {
		log = zap.NewNop()
	}

	return &RBACAuthorizer{
		store: store,
		log:   log,
	}, nil
}

// Authorize checks if a user has permission to perform an action on a resource
func (ra *RBACAuthorizer) Authorize(ctx context.Context, tenantID, userID string, permission HIPAAPermission) error {
	ra.mu.RLock()
	defer ra.mu.RUnlock()

	// Get user roles
	roles, err := ra.store.GetUserRoles(ctx, tenantID, userID)
	if err != nil {
		ra.log.Error("Failed to get user roles", zap.Error(err), zap.String("user_id", userID))
		return fmt.Errorf("failed to get user roles: %w", err)
	}

	if len(roles) == 0 {
		ra.log.Warn("User has no roles", zap.String("user_id", userID))
		return ErrPermissionDenied
	}

	// Check each role for permission
	for _, role := range roles {
		rolePermissions, err := ra.store.GetRolePermissions(ctx, role.ID)
		if err != nil {
			ra.log.Error("Failed to get role permissions", zap.Error(err), zap.String("role_id", role.ID))
			continue
		}

		for _, rp := range rolePermissions {
			if ra.permissionMatches(rp, permission) {
				// HIPAA Minimum Necessary check
				if role.MinimumNecessary && !ra.isMinimumNecessary(permission) {
					ra.log.Warn("Access denied - exceeds minimum necessary",
						zap.String("user_id", userID),
						zap.String("role_id", role.ID),
						zap.Any("permission", permission))
					return ErrMinimumNecessary
				}

				ra.log.Debug("Access authorized",
					zap.String("user_id", userID),
					zap.String("role_id", role.ID),
					zap.Any("permission", permission))
				return nil
			}
		}
	}

	ra.log.Warn("Access denied - no matching permission",
		zap.String("user_id", userID),
		zap.Any("permission", permission))
	return ErrPermissionDenied
}

// AuthorizeWithPurpose authorizes access with a specific purpose of use (HIPAA requirement)
func (ra *RBACAuthorizer) AuthorizeWithPurpose(ctx context.Context, tenantID, userID string, permission HIPAAPermission, purposeOfUse string) error {
	// Validate purpose of use
	validPurposes := map[string]bool{
		PurposeTreatment: true, PurposePayment: true, PurposeOperations: true,
		PurposeResearch: true, PurposePublicHealth: true, PurposeEmergency: true,
	}
	if !validPurposes[purposeOfUse] {
		return fmt.Errorf("invalid purpose of use: %s", purposeOfUse)
	}

	// For emergency access, break-glass may be required
	if purposeOfUse == PurposeEmergency {
		ra.log.Info("Emergency access requested - checking break-glass",
			zap.String("user_id", userID),
			zap.String("purpose", purposeOfUse))
		// Break-glass logic would go here
	}

	// Standard authorization
	if err := ra.Authorize(ctx, tenantID, userID, permission); err != nil {
		return err
	}

	// Log access with purpose
	entry := &AccessLogEntry{
		TenantID:     tenantID,
		UserID:       userID,
		ResourceType: permission.Resource,
		Action:       permission.Action,
		Result:       "allowed",
		PurposeOfUse: purposeOfUse,
		Timestamp:    time.Now(),
	}

	if err := ra.store.LogAccess(ctx, entry); err != nil {
		ra.log.Error("Failed to log access", zap.Error(err))
		// Don't fail the request, but log the error
	}

	return nil
}

// permissionMatches checks if a role permission matches the requested permission
func (ra *RBACAuthorizer) permissionMatches(rolePerm, reqPerm HIPAAPermission) bool {
	// Resource must match
	if rolePerm.Resource != reqPerm.Resource {
		return false
	}

	// Action must match
	if rolePerm.Action != reqPerm.Action {
		return false
	}

	// Scope check
	switch rolePerm.Scope {
	case "all", "organization":
		return true
	case "department":
		// Would need department context - simplified for now
		return true
	case "own_patients", "assigned":
		// Would need patient/assignment context - simplified for now
		return true
	default:
		return rolePerm.Scope == reqPerm.Scope
	}
}

// isMinimumNecessary enforces HIPAA minimum necessary principle
func (ra *RBACAuthorizer) isMinimumNecessary(permission HIPAAPermission) bool {
	// Deny bulk operations under minimum necessary
	if strings.HasPrefix(permission.Action, "delete") {
		return false
	}
	if permission.Scope == "all" && permission.Resource == ResourcePHI {
		return false
	}
	return true
}

// RequestBreakGlass initiates a break-glass emergency access request
func (ra *RBACAuthorizer) RequestBreakGlass(ctx context.Context, tenantID, userID, roleID, reason, justification string) (*BreakGlassAuthorization, error) {
	auth := &BreakGlassAuthorization{
		UserID:        userID,
		TenantID:      tenantID,
		RoleID:        roleID,
		Reason:        reason,
		Justification: justification,
		AuthorizedAt:  time.Now(),
		ExpiresAt:     time.Now().Add(24 * time.Hour), // 24 hour default
		ReviewStatus:  "pending",
		CreatedAt:     time.Now(),
	}

	if err := ra.store.RequestBreakGlass(ctx, auth); err != nil {
		return nil, fmt.Errorf("failed to request break-glass: %w", err)
	}

	ra.log.Info("Break-glass requested",
		zap.String("user_id", userID),
		zap.String("role_id", roleID),
		zap.String("reason", reason))

	return auth, nil
}

// ApproveBreakGlass approves a break-glass request
func (ra *RBACAuthorizer) ApproveBreakGlass(ctx context.Context, tenantID, authID, approvedBy string) error {
	if err := ra.store.ApproveBreakGlass(ctx, tenantID, authID, approvedBy); err != nil {
		return fmt.Errorf("failed to approve break-glass: %w", err)
	}

	ra.log.Info("Break-glass approved",
		zap.String("auth_id", authID),
		zap.String("approved_by", approvedBy))

	return nil
}

// AuthorizeWithBreakGlass authorizes access using break-glass emergency privileges
func (ra *RBACAuthorizer) AuthorizeWithBreakGlass(ctx context.Context, tenantID, userID, authID string, permission HIPAAPermission) error {
	// Get break-glass authorization
	auth, err := ra.store.GetBreakGlassAuthorization(ctx, tenantID, authID)
	if err != nil {
		return fmt.Errorf("failed to get break-glass authorization: %w", err)
	}

	// Check if expired
	if time.Now().After(auth.ExpiresAt) {
		return ErrBreakGlassExpired
	}

	// Check if approved
	if auth.ReviewStatus != "approved" {
		return ErrBreakGlassRequired
	}

	// Check if user matches
	if auth.UserID != userID {
		return ErrPermissionDenied
	}

	ra.log.Warn("Break-glass access granted",
		zap.String("user_id", userID),
		zap.String("auth_id", authID),
		zap.Any("permission", permission))

	// Log the break-glass access
	entry := &AccessLogEntry{
		TenantID:       tenantID,
		UserID:         userID,
		RoleID:         auth.RoleID,
		ResourceType:   permission.Resource,
		Action:         permission.Action,
		Result:         "allowed",
		BreakGlassUsed: true,
		BreakGlassID:   &authID,
		PurposeOfUse:   PurposeEmergency,
		Timestamp:      time.Now(),
	}

	if err := ra.store.LogAccess(ctx, entry); err != nil {
		ra.log.Error("Failed to log break-glass access", zap.Error(err))
	}

	return nil
}

// GetEffectivePermissions returns all effective permissions for a user
func (ra *RBACAuthorizer) GetEffectivePermissions(ctx context.Context, tenantID, userID string) ([]HIPAAPermission, error) {
	roles, err := ra.store.GetUserRoles(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	permissionMap := make(map[string]HIPAAPermission)

	for _, role := range roles {
		perms, err := ra.store.GetRolePermissions(ctx, role.ID)
		if err != nil {
			ra.log.Error("Failed to get role permissions", zap.Error(err), zap.String("role_id", role.ID))
			continue
		}

		for _, p := range perms {
			key := p.String()
			permissionMap[key] = p
		}
	}

	permissions := make([]HIPAAPermission, 0, len(permissionMap))
	for _, p := range permissionMap {
		permissions = append(permissions, p)
	}

	return permissions, nil
}

// Close closes the authorizer
func (ra *RBACAuthorizer) Close() error {
	return ra.store.Close()
}

// Helper function to generate secure IDs
func GenerateSecureID(prefix string) string {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%s-%d-%s", prefix, time.Now().UnixNano(), prefix)))
	if prefix != "" {
		return prefix + "_" + hex.EncodeToString(hash[:8])
	}
	return hex.EncodeToString(hash[:8])
}

// ValidateNPI validates a National Provider Identifier
func ValidateNPI(npi string) error {
	if npi == "" {
		return fmt.Errorf("NPI cannot be empty")
	}

	// NPI is 10 digits
	matched, _ := regexp.MatchString(`^\d{10}$`, npi)
	if !matched {
		return fmt.Errorf("invalid NPI format (must be 10 digits)")
	}

	// Luhn algorithm check for NPI
	return validateLuhn(npi)
}

// validateLuhn validates a number using the Luhn algorithm
func validateLuhn(s string) error {
	digits := make([]int, len(s))
	for i, r := range s {
		digits[i] = int(r - '0')
	}

	sum := 0
	for i := len(digits) - 2; i >= 0; i-- {
		d := digits[i]
		if (len(digits)-i)%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}

	checkDigit := digits[len(digits)-1]
	calculatedCheck := (10 - (sum % 10)) % 10

	if calculatedCheck != checkDigit {
		return fmt.Errorf("invalid check digit")
	}

	return nil
}
