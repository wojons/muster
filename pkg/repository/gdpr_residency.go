package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

var (
	// ErrRegionViolation is returned when a data residency violation occurs
	ErrRegionViolation = fmt.Errorf("data residency violation")

	// ErrRegionNotSupported is returned when a region is not supported
	ErrRegionNotSupported = fmt.Errorf("region not supported")
)

// DataRegion represents a geographic data region
type DataRegion string

const (
	// DataRegionEU - European Union
	DataRegionEU DataRegion = "eu"
	// DataRegionUS - United States
	DataRegionUS DataRegion = "us"
	// DataRegionUK - United Kingdom
	DataRegionUK DataRegion = "uk"
	// DataRegionCH - Switzerland
	DataRegionCH DataRegion = "ch"
	// DataRegionCA - Canada
	DataRegionCA DataRegion = "ca"
	// DataRegionAU - Australia
	DataRegionAU DataRegion = "au"
	// DataRegionJP - Japan
	DataRegionJP DataRegion = "jp"
	// DataRegionIN - India
	DataRegionIN DataRegion = "in"
	// DataRegionBR - Brazil
	DataRegionBR DataRegion = "br"
	// DataRegionSG - Singapore
	DataRegionSG DataRegion = "sg"
	// DataRegionAny - No restriction (default)
	DataRegionAny DataRegion = "any"
)

// IsValid checks if the region is a valid data region
func (r DataRegion) IsValid() bool {
	switch r {
	case DataRegionEU, DataRegionUS, DataRegionUK, DataRegionCH,
		DataRegionCA, DataRegionAU, DataRegionJP, DataRegionIN,
		DataRegionBR, DataRegionSG, DataRegionAny:
		return true
	default:
		return false
	}
}

// String returns the string representation of the region
func (r DataRegion) String() string {
	return string(r)
}

// DataResidencyPolicy represents a GDPR Article 44-49 data residency policy
type DataResidencyPolicy struct {
	ID             int64        `db:"id" json:"id"`
	Name           string       `db:"name" json:"name"`
	Description    string       `db:"description" json:"description"`
	AllowedRegions []DataRegion `db:"-" json:"allowed_regions"`
	EnforceMode    EnforceMode  `db:"enforce_mode" json:"enforce_mode"`
	CreatedAt      time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time    `db:"updated_at" json:"updated_at"`
}

// EnforceMode defines how strictly the policy is enforced
type EnforceMode string

const (
	// EnforceModeStrict - Block all writes to non-compliant regions
	EnforceModeStrict EnforceMode = "strict"
	// EnforceModeAudit - Allow but log violations
	EnforceModeAudit EnforceMode = "audit"
	// EnforceModeDisabled - No enforcement
	EnforceModeDisabled EnforceMode = "disabled"
)

// IsValid checks if the enforce mode is valid
func (m EnforceMode) IsValid() bool {
	switch m {
	case EnforceModeStrict, EnforceModeAudit, EnforceModeDisabled:
		return true
	default:
		return false
	}
}

// ResidencyViolation represents a data residency violation
type ResidencyViolation struct {
	ID              int64      `db:"id" json:"id"`
	PolicyID        int64      `db:"policy_id" json:"policy_id"`
	PolicyName      string     `db:"policy_name" json:"policy_name"`
	EntityType      string     `db:"entity_type" json:"entity_type"`
	EntityID        int64      `db:"entity_id" json:"entity_id"`
	AttemptedRegion DataRegion `db:"attempted_region" json:"attempted_region"`
	AllowedRegions  string     `db:"allowed_regions" json:"allowed_regions"`
	Action          string     `db:"action" json:"action"`
	Blocked         bool       `db:"blocked" json:"blocked"`
	UserID          string     `db:"user_id" json:"user_id"`
	CreatedAt       time.Time  `db:"created_at" json:"created_at"`
}

// ResidencyEnforcer enforces data residency policies
type ResidencyEnforcer struct {
	policies      []*DataResidencyPolicy
	currentPolicy *DataResidencyPolicy
	logger        *zap.Logger
	db            *sql.DB
}

// NewResidencyEnforcer creates a new residency enforcer
func NewResidencyEnforcer(db *sql.DB, logger *zap.Logger) *ResidencyEnforcer {
	return &ResidencyEnforcer{
		policies: make([]*DataResidencyPolicy, 0),
		logger:   logger,
		db:       db,
	}
}

// LoadPolicies loads all policies from the database
func (re *ResidencyEnforcer) LoadPolicies(ctx context.Context) error {
	rows, err := re.db.QueryContext(ctx, `
		SELECT id, name, description, enforce_mode, created_at, updated_at
		FROM data_residency_policies
		ORDER BY created_at DESC
		LIMIT 1
	`)
	if err != nil {
		if err == sql.ErrNoRows {
			re.logger.Info("No data residency policy found, using default (any region)")
			return nil
		}
		return fmt.Errorf("load policies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	re.policies = make([]*DataResidencyPolicy, 0)
	for rows.Next() {
		policy := &DataResidencyPolicy{}
		var regionsStr string

		err := rows.Scan(
			&policy.ID,
			&policy.Name,
			&policy.Description,
			&policy.EnforceMode,
			&policy.CreatedAt,
			&policy.UpdatedAt,
		)
		if err != nil {
			return fmt.Errorf("scan policy: %w", err)
		}

		// Parse allowed regions from comma-separated string
		if regionsStr != "" {
			regions := strings.Split(regionsStr, ",")
			for _, r := range regions {
				policy.AllowedRegions = append(policy.AllowedRegions, DataRegion(strings.TrimSpace(r)))
			}
		} else {
			// Default to any region if not specified
			policy.AllowedRegions = []DataRegion{DataRegionAny}
		}

		re.policies = append(re.policies, policy)
	}

	if len(re.policies) > 0 {
		re.currentPolicy = re.policies[0]
		re.logger.Info("Loaded data residency policy",
			zap.String("name", re.currentPolicy.Name),
			zap.String("enforce_mode", string(re.currentPolicy.EnforceMode)),
			zap.Int("allowed_regions", len(re.currentPolicy.AllowedRegions)),
		)
	}

	return rows.Err()
}

// SetCurrentPolicy sets the active policy
func (re *ResidencyEnforcer) SetCurrentPolicy(policy *DataResidencyPolicy) {
	re.currentPolicy = policy
}

// GetCurrentPolicy returns the current active policy
func (re *ResidencyEnforcer) GetCurrentPolicy() *DataResidencyPolicy {
	return re.currentPolicy
}

// ValidateRegion checks if a region is allowed by the current policy
func (re *ResidencyEnforcer) ValidateRegion(region DataRegion) error {
	if re.currentPolicy == nil {
		// No policy means any region is allowed
		return nil
	}

	if re.currentPolicy.EnforceMode == EnforceModeDisabled {
		return nil
	}

	// Check if region is in allowed list
	allowed := false
	for _, allowedRegion := range re.currentPolicy.AllowedRegions {
		if allowedRegion == DataRegionAny || allowedRegion == region {
			allowed = true
			break
		}
	}

	if !allowed {
		violation := &ResidencyViolation{
			PolicyID:        re.currentPolicy.ID,
			PolicyName:      re.currentPolicy.Name,
			AttemptedRegion: region,
			AllowedRegions:  regionsToString(re.currentPolicy.AllowedRegions),
			Action:          "validate",
			Blocked:         re.currentPolicy.EnforceMode == EnforceModeStrict,
		}

		if re.currentPolicy.EnforceMode == EnforceModeStrict {
			re.logger.Warn("Data residency violation - operation blocked",
				zap.String("policy", re.currentPolicy.Name),
				zap.String("attempted_region", string(region)),
				zap.String("allowed_regions", violation.AllowedRegions),
			)
			return fmt.Errorf("%w: region %s not allowed by policy %s (allowed: %s)",
				ErrRegionViolation, region, re.currentPolicy.Name, violation.AllowedRegions)
		}

		re.logger.Warn("Data residency violation - audit mode",
			zap.String("policy", re.currentPolicy.Name),
			zap.String("attempted_region", string(region)),
			zap.String("allowed_regions", violation.AllowedRegions),
		)
	}

	return nil
}

// AuthorizeWrite checks if a write operation is allowed
func (re *ResidencyEnforcer) AuthorizeWrite(ctx context.Context, entityType string, entityID int64, region DataRegion, userID string) error {
	if re.currentPolicy == nil {
		return nil
	}

	if re.currentPolicy.EnforceMode == EnforceModeDisabled {
		return nil
	}

	allowed := false
	for _, allowedRegion := range re.currentPolicy.AllowedRegions {
		if allowedRegion == DataRegionAny || allowedRegion == region {
			allowed = true
			break
		}
	}

	if !allowed {
		// Log violation
		violation := &ResidencyViolation{
			PolicyID:        re.currentPolicy.ID,
			PolicyName:      re.currentPolicy.Name,
			EntityType:      entityType,
			EntityID:        entityID,
			AttemptedRegion: region,
			AllowedRegions:  regionsToString(re.currentPolicy.AllowedRegions),
			Action:          "write",
			Blocked:         re.currentPolicy.EnforceMode == EnforceModeStrict,
			UserID:          userID,
			CreatedAt:       time.Now(),
		}

		// Save violation to database
		if err := re.saveViolation(ctx, violation); err != nil {
			re.logger.Error("Failed to save residency violation",
				zap.Error(err),
			)
		}

		if re.currentPolicy.EnforceMode == EnforceModeStrict {
			re.logger.Warn("Data residency violation - write blocked",
				zap.String("policy", re.currentPolicy.Name),
				zap.String("entity_type", entityType),
				zap.Int64("entity_id", entityID),
				zap.String("attempted_region", string(region)),
				zap.String("user_id", userID),
			)
			return fmt.Errorf("%w: cannot write %s/%d to region %s (policy: %s, allowed: %s)",
				ErrRegionViolation, entityType, entityID, region, re.currentPolicy.Name, violation.AllowedRegions)
		}

		re.logger.Warn("Data residency violation - write audited",
			zap.String("policy", re.currentPolicy.Name),
			zap.String("entity_type", entityType),
			zap.Int64("entity_id", entityID),
			zap.String("attempted_region", string(region)),
			zap.String("user_id", userID),
		)
	}

	return nil
}

// saveViolation saves a residency violation to the database
func (re *ResidencyEnforcer) saveViolation(ctx context.Context, violation *ResidencyViolation) error {
	_, err := re.db.ExecContext(ctx, `
		INSERT INTO data_residency_violations 
		(policy_id, policy_name, entity_type, entity_id, attempted_region, allowed_regions, action, blocked, user_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		violation.PolicyID,
		violation.PolicyName,
		violation.EntityType,
		violation.EntityID,
		violation.AttemptedRegion,
		violation.AllowedRegions,
		violation.Action,
		violation.Blocked,
		violation.UserID,
		violation.CreatedAt,
	)
	return err
}

// GetViolations retrieves violations from the database
func (re *ResidencyEnforcer) GetViolations(ctx context.Context, limit int, offset int) ([]*ResidencyViolation, error) {
	rows, err := re.db.QueryContext(ctx, `
		SELECT id, policy_id, policy_name, entity_type, entity_id, 
		       attempted_region, allowed_regions, action, blocked, user_id, created_at
		FROM data_residency_violations
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	violations := make([]*ResidencyViolation, 0)
	for rows.Next() {
		v := &ResidencyViolation{}
		err := rows.Scan(
			&v.ID,
			&v.PolicyID,
			&v.PolicyName,
			&v.EntityType,
			&v.EntityID,
			&v.AttemptedRegion,
			&v.AllowedRegions,
			&v.Action,
			&v.Blocked,
			&v.UserID,
			&v.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		violations = append(violations, v)
	}

	return violations, rows.Err()
}

// Helper functions
func regionsToStringSlice(regions []DataRegion) []string {
	result := make([]string, len(regions))
	for i, r := range regions {
		result[i] = string(r)
	}
	return result
}

func regionsToString(regions []DataRegion) string {
	result := regionsToStringSlice(regions)
	return strings.Join(result, ", ")
}

// CreateResidencyPolicy creates a new data residency policy
func (sm *SQLiteManager) CreateResidencyPolicy(ctx context.Context, name, description string, allowedRegions []DataRegion, enforceMode EnforceMode) (*DataResidencyPolicy, error) {
	if !enforceMode.IsValid() {
		return nil, fmt.Errorf("invalid enforce mode: %s", enforceMode)
	}

	regionsStr := regionsToString(allowedRegions)

	now := time.Now()
	policy := &DataResidencyPolicy{
		Name:           name,
		Description:    description,
		AllowedRegions: allowedRegions,
		EnforceMode:    enforceMode,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	result, err := sm.db.ExecContext(ctx, `
		INSERT INTO data_residency_policies 
		(name, description, allowed_regions, enforce_mode, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		policy.Name,
		policy.Description,
		regionsStr,
		policy.EnforceMode,
		policy.CreatedAt,
		policy.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create policy: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get policy ID: %w", err)
	}
	policy.ID = id

	sm.log.Info("Created data residency policy",
		zap.Int64("id", policy.ID),
		zap.String("name", policy.Name),
		zap.String("enforce_mode", string(policy.EnforceMode)),
		zap.Int("allowed_regions", len(policy.AllowedRegions)),
	)

	return policy, nil
}

// UpdateResidencyPolicy updates an existing data residency policy
func (sm *SQLiteManager) UpdateResidencyPolicy(ctx context.Context, id int64, name, description string, allowedRegions []DataRegion, enforceMode EnforceMode) (*DataResidencyPolicy, error) {
	if !enforceMode.IsValid() {
		return nil, fmt.Errorf("invalid enforce mode: %s", enforceMode)
	}

	// Get existing policy to preserve CreatedAt
	existing, err := sm.GetResidencyPolicy(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get existing policy: %w", err)
	}

	regionsStr := regionsToString(allowedRegions)

	now := time.Now()
	policy := &DataResidencyPolicy{
		ID:             id,
		Name:           name,
		Description:    description,
		AllowedRegions: allowedRegions,
		EnforceMode:    enforceMode,
		CreatedAt:      existing.CreatedAt,
		UpdatedAt:      now,
	}

	_, err = sm.db.ExecContext(ctx, `
		UPDATE data_residency_policies 
		SET name = ?, description = ?, allowed_regions = ?, enforce_mode = ?, updated_at = ?
		WHERE id = ?
	`,
		policy.Name,
		policy.Description,
		regionsStr,
		policy.EnforceMode,
		policy.UpdatedAt,
		policy.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("update policy: %w", err)
	}

	sm.log.Info("Updated data residency policy",
		zap.Int64("id", policy.ID),
		zap.String("name", policy.Name),
		zap.String("enforce_mode", string(policy.EnforceMode)),
	)

	return policy, nil
}

// DeleteResidencyPolicy deletes a data residency policy
func (sm *SQLiteManager) DeleteResidencyPolicy(ctx context.Context, id int64) error {
	_, err := sm.db.ExecContext(ctx, `
		DELETE FROM data_residency_policies WHERE id = ?
	`, id)
	if err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}

	sm.log.Info("Deleted data residency policy",
		zap.Int64("id", id),
	)

	return nil
}

// GetResidencyPolicy retrieves a data residency policy by ID
func (sm *SQLiteManager) GetResidencyPolicy(ctx context.Context, id int64) (*DataResidencyPolicy, error) {
	row := sm.db.QueryRowContext(ctx, `
		SELECT id, name, description, allowed_regions, enforce_mode, created_at, updated_at
		FROM data_residency_policies
		WHERE id = ?
	`, id)

	policy := &DataResidencyPolicy{}
	var regionsStr string

	err := row.Scan(
		&policy.ID,
		&policy.Name,
		&policy.Description,
		&regionsStr,
		&policy.EnforceMode,
		&policy.CreatedAt,
		&policy.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("policy not found: %d", id)
		}
		return nil, fmt.Errorf("get policy: %w", err)
	}

	// Parse allowed regions
	if regionsStr != "" {
		regions := strings.Split(regionsStr, ",")
		for _, r := range regions {
			policy.AllowedRegions = append(policy.AllowedRegions, DataRegion(strings.TrimSpace(r)))
		}
	} else {
		policy.AllowedRegions = []DataRegion{DataRegionAny}
	}

	return policy, nil
}

// GetActiveResidencyPolicy retrieves the most recently created policy
func (sm *SQLiteManager) GetActiveResidencyPolicy(ctx context.Context) (*DataResidencyPolicy, error) {
	row := sm.db.QueryRowContext(ctx, `
		SELECT id, name, description, allowed_regions, enforce_mode, created_at, updated_at
		FROM data_residency_policies
		ORDER BY created_at DESC
		LIMIT 1
	`)

	policy := &DataResidencyPolicy{}
	var regionsStr string

	err := row.Scan(
		&policy.ID,
		&policy.Name,
		&policy.Description,
		&regionsStr,
		&policy.EnforceMode,
		&policy.CreatedAt,
		&policy.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No policy exists
		}
		return nil, fmt.Errorf("get active policy: %w", err)
	}

	// Parse allowed regions
	if regionsStr != "" {
		regions := strings.Split(regionsStr, ",")
		for _, r := range regions {
			policy.AllowedRegions = append(policy.AllowedRegions, DataRegion(strings.TrimSpace(r)))
		}
	} else {
		policy.AllowedRegions = []DataRegion{DataRegionAny}
	}

	return policy, nil
}
