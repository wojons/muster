package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/wojons/muster/pkg/repository"
)

// rateLimitRepository implements repository.RateLimitRepository.
type rateLimitRepository struct {
	base *sqlRepository
	qs   *QuerySet
}

// NewRateLimitRepository creates a new RateLimitRepository.
func NewRateLimitRepository(db *sql.DB, driver Driver) (*rateLimitRepository, error) {
	base := NewRepository(db, driver)
	qs, err := GetQuerySet(driver)
	if err != nil {
		return nil, err
	}
	return &rateLimitRepository{base: base, qs: qs}, nil
}

// Create creates a new rate limit entry.
func (r *rateLimitRepository) Create(ctx context.Context, rl *repository.RateLimit) error {
	result, err := r.base.exec(ctx, r.qs.RateLimitCreate, rl.Key, rl.Count, rl.Window, rl.ExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to create rate limit: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	rl.ID = id
	rl.CreatedAt = now()
	return nil
}

// GetByID retrieves a rate limit by ID.
func (r *rateLimitRepository) GetByID(ctx context.Context, id int64) (*repository.RateLimit, error) {
	rl := &repository.RateLimit{}
	row := r.base.queryRow(ctx, r.qs.RateLimitGetByID, id)

	err := row.Scan(
		&rl.ID,
		&rl.Key,
		&rl.Count,
		&rl.Window,
		&rl.ExpiresAt,
		&rl.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "rate_limit", ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit: %w", err)
	}

	return rl, nil
}

// GetByKey retrieves a rate limit by key.
func (r *rateLimitRepository) GetByKey(ctx context.Context, key string) (*repository.RateLimit, error) {
	rl := &repository.RateLimit{}
	row := r.base.queryRow(ctx, r.qs.RateLimitGetByKey, key)

	err := row.Scan(
		&rl.ID,
		&rl.Key,
		&rl.Count,
		&rl.Window,
		&rl.ExpiresAt,
		&rl.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "rate_limit", ID: 0}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit by key: %w", err)
	}

	return rl, nil
}

// Update updates an existing rate limit.
func (r *rateLimitRepository) Update(ctx context.Context, rl *repository.RateLimit) error {
	_, err := r.base.exec(ctx, r.qs.RateLimitUpdate, rl.Key, rl.Count, rl.Window, rl.ExpiresAt, rl.ID)
	if err != nil {
		return fmt.Errorf("failed to update rate limit: %w", err)
	}
	return nil
}

// Delete removes a rate limit by ID.
func (r *rateLimitRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.base.exec(ctx, r.qs.RateLimitDelete, id)
	if err != nil {
		return fmt.Errorf("failed to delete rate limit: %w", err)
	}
	return nil
}

// DeleteByKey removes a rate limit by key.
func (r *rateLimitRepository) DeleteByKey(ctx context.Context, key string) error {
	_, err := r.base.exec(ctx, r.qs.RateLimitDeleteByKey, key)
	if err != nil {
		return fmt.Errorf("failed to delete rate limit by key: %w", err)
	}
	return nil
}

// Cleanup removes expired rate limits.
func (r *rateLimitRepository) Cleanup(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := r.base.exec(ctx, r.qs.RateLimitCleanup, olderThan)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup rate limits: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return deleted, nil
}

// Increment increments the count for a rate limit key.
func (r *rateLimitRepository) Increment(ctx context.Context, key string, window time.Time) error {
	// Default expiration of 1 hour from now
	expiresAt := now().Add(time.Hour)

	_, err := r.base.exec(ctx, r.qs.RateLimitIncrement, key, window, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to increment rate limit: %w", err)
	}
	return nil
}
