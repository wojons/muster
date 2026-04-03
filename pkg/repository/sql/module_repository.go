package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/openapi-cli/openapi-cli/pkg/repository"
)

// moduleRepository implements repository.ModuleRepository.
type moduleRepository struct {
	base *sqlRepository
	qs   *QuerySet
}

// NewModuleRepository creates a new ModuleRepository.
func NewModuleRepository(db *sql.DB, driver Driver) (*moduleRepository, error) {
	base := NewRepository(db, driver)
	qs, err := GetQuerySet(driver)
	if err != nil {
		return nil, err
	}
	return &moduleRepository{base: base, qs: qs}, nil
}

// Create creates a new module.
func (r *moduleRepository) Create(ctx context.Context, mod *repository.Module) error {
	result, err := r.base.exec(ctx, r.qs.ModuleCreate, mod.Name, mod.Path, mod.Language, mod.InventoryID)
	if err != nil {
		return fmt.Errorf("failed to create module: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	mod.ID = id
	mod.CreatedAt = now()
	mod.UpdatedAt = mod.CreatedAt
	return nil
}

// GetByID retrieves a module by ID.
func (r *moduleRepository) GetByID(ctx context.Context, id int64) (*repository.Module, error) {
	mod := &repository.Module{}
	row := r.base.queryRow(ctx, r.qs.ModuleGetByID, id)

	err := row.Scan(
		&mod.ID,
		&mod.Name,
		&mod.Path,
		&mod.Language,
		&mod.InventoryID,
		&mod.CreatedAt,
		&mod.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "module", ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get module: %w", err)
	}

	return mod, nil
}

// GetByPath retrieves a module by its file path.
func (r *moduleRepository) GetByPath(ctx context.Context, path string) (*repository.Module, error) {
	mod := &repository.Module{}
	row := r.base.queryRow(ctx, r.qs.ModuleGetByPath, path)

	err := row.Scan(
		&mod.ID,
		&mod.Name,
		&mod.Path,
		&mod.Language,
		&mod.InventoryID,
		&mod.CreatedAt,
		&mod.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "module", ID: 0}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get module by path: %w", err)
	}

	return mod, nil
}

// GetByInventory retrieves all modules for an inventory.
func (r *moduleRepository) GetByInventory(ctx context.Context, inventoryID int64, opts repository.ListOptions) ([]*repository.Module, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	rows, err := r.base.query(ctx, r.qs.ModuleGetByInventory, inventoryID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list modules: %w", err)
	}
	defer rows.Close()

	var modules []*repository.Module
	for rows.Next() {
		mod := &repository.Module{}
		err := rows.Scan(
			&mod.ID,
			&mod.Name,
			&mod.Path,
			&mod.Language,
			&mod.InventoryID,
			&mod.CreatedAt,
			&mod.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan module: %w", err)
		}
		modules = append(modules, mod)
	}

	return modules, rows.Err()
}

// Update updates an existing module.
func (r *moduleRepository) Update(ctx context.Context, mod *repository.Module) error {
	mod.UpdatedAt = now()
	_, err := r.base.exec(ctx, r.qs.ModuleUpdate, mod.Name, mod.Path, mod.Language, mod.InventoryID, mod.UpdatedAt, mod.ID)
	if err != nil {
		return fmt.Errorf("failed to update module: %w", err)
	}
	return nil
}

// Delete removes a module by ID.
func (r *moduleRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.base.exec(ctx, r.qs.ModuleDelete, id)
	if err != nil {
		return fmt.Errorf("failed to delete module: %w", err)
	}
	return nil
}

// GetOrphans retrieves modules with no valid inventory.
func (r *moduleRepository) GetOrphans(ctx context.Context) ([]*repository.Module, error) {
	rows, err := r.base.query(ctx, r.qs.ModuleGetOrphans)
	if err != nil {
		return nil, fmt.Errorf("failed to get orphaned modules: %w", err)
	}
	defer rows.Close()

	var modules []*repository.Module
	for rows.Next() {
		mod := &repository.Module{}
		err := rows.Scan(
			&mod.ID,
			&mod.Name,
			&mod.Path,
			&mod.Language,
			&mod.InventoryID,
			&mod.CreatedAt,
			&mod.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan module: %w", err)
		}
		modules = append(modules, mod)
	}

	return modules, rows.Err()
}

// CountByInventory returns the count of modules for an inventory.
func (r *moduleRepository) CountByInventory(ctx context.Context, inventoryID int64) (int64, error) {
	var count int64
	err := r.base.queryRow(ctx, r.qs.ModuleCountByInventory, inventoryID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count modules: %w", err)
	}
	return count, nil
}
