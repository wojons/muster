package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/repository"
)

// auditRepository implements repository.AuditRepository.
type auditRepository struct {
	base *sqlRepository
	qs   *QuerySet
}

// NewAuditRepository creates a new AuditRepository.
func NewAuditRepository(db *sql.DB, driver Driver) (*auditRepository, error) {
	base := NewRepository(db, driver)
	qs, err := GetQuerySet(driver)
	if err != nil {
		return nil, err
	}
	return &auditRepository{base: base, qs: qs}, nil
}

// Create creates a new audit log entry.
func (r *auditRepository) Create(ctx context.Context, audit *repository.Audit) error {
	result, err := r.base.exec(ctx, r.qs.AuditCreate, audit.Action, audit.EntityType, audit.EntityID, audit.UserID, audit.Data)
	if err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	audit.ID = id
	audit.CreatedAt = now()
	return nil
}

// GetByID retrieves an audit log by ID.
func (r *auditRepository) GetByID(ctx context.Context, id int64) (*repository.Audit, error) {
	audit := &repository.Audit{}
	row := r.base.queryRow(ctx, r.qs.AuditGetByID, id)

	err := row.Scan(
		&audit.ID,
		&audit.Action,
		&audit.EntityType,
		&audit.EntityID,
		&audit.UserID,
		&audit.Data,
		&audit.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "audit", ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get audit log: %w", err)
	}

	return audit, nil
}

// GetByEntity retrieves audit logs for a specific entity.
func (r *auditRepository) GetByEntity(ctx context.Context, entityType string, entityID int64, opts repository.ListOptions) ([]*repository.Audit, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	rows, err := r.base.query(ctx, r.qs.AuditGetByEntity, entityType, entityID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer rows.Close()

	var audits []*repository.Audit
	for rows.Next() {
		audit := &repository.Audit{}
		err := rows.Scan(
			&audit.ID,
			&audit.Action,
			&audit.EntityType,
			&audit.EntityID,
			&audit.UserID,
			&audit.Data,
			&audit.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}
		audits = append(audits, audit)
	}

	return audits, rows.Err()
}

// List retrieves all audit logs with pagination.
func (r *auditRepository) List(ctx context.Context, opts repository.ListOptions) ([]*repository.Audit, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	rows, err := r.base.query(ctx, r.qs.AuditList, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", err)
	}
	defer rows.Close()

	var audits []*repository.Audit
	for rows.Next() {
		audit := &repository.Audit{}
		err := rows.Scan(
			&audit.ID,
			&audit.Action,
			&audit.EntityType,
			&audit.EntityID,
			&audit.UserID,
			&audit.Data,
			&audit.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}
		audits = append(audits, audit)
	}

	return audits, rows.Err()
}

// Count returns the total number of audit logs.
func (r *auditRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.base.queryRow(ctx, r.qs.AuditCount).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count audit logs: %w", err)
	}
	return count, nil
}

// Cleanup removes audit logs older than the specified time.
func (r *auditRepository) Cleanup(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := r.base.exec(ctx, r.qs.AuditCleanup, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup audit logs: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return deleted, nil
}
