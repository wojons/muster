package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wojons/muster/pkg/repository"
)

// fingerprintRepository implements repository.FingerprintRepository.
type fingerprintRepository struct {
	base *sqlRepository
	qs   *QuerySet
}

// NewFingerprintRepository creates a new FingerprintRepository.
func NewFingerprintRepository(db *sql.DB, driver Driver) (*fingerprintRepository, error) {
	base := NewRepository(db, driver)
	qs, err := GetQuerySet(driver)
	if err != nil {
		return nil, err
	}
	return &fingerprintRepository{base: base, qs: qs}, nil
}

// Create creates a new fingerprint.
func (r *fingerprintRepository) Create(ctx context.Context, fp *repository.Fingerprint) error {
	result, err := r.base.exec(ctx, r.qs.FingerprintCreate, fp.Hash, fp.SpecID, fp.Algorithm)
	if err != nil {
		return fmt.Errorf("failed to create fingerprint: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	fp.ID = id
	fp.CreatedAt = now()
	return nil
}

// GetByID retrieves a fingerprint by ID.
func (r *fingerprintRepository) GetByID(ctx context.Context, id int64) (*repository.Fingerprint, error) {
	fp := &repository.Fingerprint{}
	row := r.base.queryRow(ctx, r.qs.FingerprintGetByID, id)

	err := row.Scan(
		&fp.ID,
		&fp.Hash,
		&fp.SpecID,
		&fp.Algorithm,
		&fp.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "fingerprint", ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get fingerprint: %w", err)
	}

	return fp, nil
}

// GetBySpec retrieves a fingerprint by spec ID.
func (r *fingerprintRepository) GetBySpec(ctx context.Context, specID int64) (*repository.Fingerprint, error) {
	fp := &repository.Fingerprint{}
	row := r.base.queryRow(ctx, r.qs.FingerprintGetBySpec, specID)

	err := row.Scan(
		&fp.ID,
		&fp.Hash,
		&fp.SpecID,
		&fp.Algorithm,
		&fp.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "fingerprint", ID: 0}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get fingerprint by spec: %w", err)
	}

	return fp, nil
}

// GetByHash retrieves a fingerprint by its hash value.
func (r *fingerprintRepository) GetByHash(ctx context.Context, hash string) (*repository.Fingerprint, error) {
	fp := &repository.Fingerprint{}
	row := r.base.queryRow(ctx, r.qs.FingerprintGetByHash, hash)

	err := row.Scan(
		&fp.ID,
		&fp.Hash,
		&fp.SpecID,
		&fp.Algorithm,
		&fp.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "fingerprint", ID: 0}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get fingerprint by hash: %w", err)
	}

	return fp, nil
}

// Update updates an existing fingerprint.
func (r *fingerprintRepository) Update(ctx context.Context, fp *repository.Fingerprint) error {
	_, err := r.base.exec(ctx, r.qs.FingerprintUpdate, fp.Hash, fp.SpecID, fp.Algorithm, fp.ID)
	if err != nil {
		return fmt.Errorf("failed to update fingerprint: %w", err)
	}
	return nil
}

// Delete removes a fingerprint by ID.
func (r *fingerprintRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.base.exec(ctx, r.qs.FingerprintDelete, id)
	if err != nil {
		return fmt.Errorf("failed to delete fingerprint: %w", err)
	}
	return nil
}

// DeleteBySpec removes the fingerprint for a spec.
func (r *fingerprintRepository) DeleteBySpec(ctx context.Context, specID int64) error {
	_, err := r.base.exec(ctx, r.qs.FingerprintDeleteBySpec, specID)
	if err != nil {
		return fmt.Errorf("failed to delete fingerprint by spec: %w", err)
	}
	return nil
}

// Exists checks if a fingerprint exists for the given hash.
func (r *fingerprintRepository) Exists(ctx context.Context, hash string) (bool, error) {
	var exists bool
	err := r.base.queryRow(ctx, r.qs.FingerprintExists, hash).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check fingerprint existence: %w", err)
	}
	return exists, nil
}
