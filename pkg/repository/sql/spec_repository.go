package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/openapi-cli/openapi-cli/pkg/repository"
)

// specRepository implements repository.SpecRepository.
type specRepository struct {
	base *sqlRepository
	qs   *QuerySet
}

// NewSpecRepository creates a new SpecRepository.
func NewSpecRepository(db *sql.DB, driver Driver) (*specRepository, error) {
	base := NewRepository(db, driver)
	qs, err := GetQuerySet(driver)
	if err != nil {
		return nil, err
	}
	return &specRepository{base: base, qs: qs}, nil
}

// Create creates a new spec.
func (r *specRepository) Create(ctx context.Context, spec *repository.Spec) error {
	result, err := r.base.exec(ctx, r.qs.SpecCreate, spec.Name, spec.Content, spec.Fingerprint)
	if err != nil {
		return fmt.Errorf("failed to create spec: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	spec.ID = id
	spec.CreatedAt = now()
	spec.UpdatedAt = spec.CreatedAt
	return nil
}

// GetByID retrieves a spec by ID.
func (r *specRepository) GetByID(ctx context.Context, id int64) (*repository.Spec, error) {
	spec := &repository.Spec{}
	row := r.base.queryRow(ctx, r.qs.SpecGetByID, id)

	err := row.Scan(
		&spec.ID,
		&spec.Name,
		&spec.Content,
		&spec.Fingerprint,
		&spec.CreatedAt,
		&spec.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "spec", ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get spec: %w", err)
	}

	return spec, nil
}

// GetByAlias retrieves specs for an alias.
func (r *specRepository) GetByAlias(ctx context.Context, aliasID int64, opts repository.ListOptions) ([]*repository.Spec, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	rows, err := r.base.query(ctx, r.qs.SpecGetByAlias, aliasID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list specs by alias: %w", err)
	}
	defer rows.Close()

	var specs []*repository.Spec
	for rows.Next() {
		spec := &repository.Spec{}
		err := rows.Scan(
			&spec.ID,
			&spec.Name,
			&spec.Content,
			&spec.Fingerprint,
			&spec.CreatedAt,
			&spec.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan spec: %w", err)
		}
		specs = append(specs, spec)
	}

	return specs, rows.Err()
}

// GetByModule retrieves specs for a module.
func (r *specRepository) GetByModule(ctx context.Context, moduleID int64, opts repository.ListOptions) ([]*repository.Spec, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	rows, err := r.base.query(ctx, r.qs.SpecGetByModule, moduleID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list specs by module: %w", err)
	}
	defer rows.Close()

	var specs []*repository.Spec
	for rows.Next() {
		spec := &repository.Spec{}
		err := rows.Scan(
			&spec.ID,
			&spec.Name,
			&spec.Content,
			&spec.Fingerprint,
			&spec.CreatedAt,
			&spec.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan spec: %w", err)
		}
		specs = append(specs, spec)
	}

	return specs, rows.Err()
}

// Update updates an existing spec.
func (r *specRepository) Update(ctx context.Context, spec *repository.Spec) error {
	spec.UpdatedAt = now()
	_, err := r.base.exec(ctx, r.qs.SpecUpdate, spec.Name, spec.Content, spec.Fingerprint, spec.UpdatedAt, spec.ID)
	if err != nil {
		return fmt.Errorf("failed to update spec: %w", err)
	}
	return nil
}

// Delete removes a spec by ID.
func (r *specRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.base.exec(ctx, r.qs.SpecDelete, id)
	if err != nil {
		return fmt.Errorf("failed to delete spec: %w", err)
	}
	return nil
}

// DeleteByAlias removes all specs for an alias.
func (r *specRepository) DeleteByAlias(ctx context.Context, aliasID int64) error {
	_, err := r.base.exec(ctx, r.qs.SpecDeleteByAlias, aliasID)
	if err != nil {
		return fmt.Errorf("failed to delete specs by alias: %w", err)
	}
	return nil
}

// DeleteByModule removes all specs for a module.
func (r *specRepository) DeleteByModule(ctx context.Context, moduleID int64) error {
	_, err := r.base.exec(ctx, r.qs.SpecDeleteByModule, moduleID)
	if err != nil {
		return fmt.Errorf("failed to delete specs by module: %w", err)
	}
	return nil
}
