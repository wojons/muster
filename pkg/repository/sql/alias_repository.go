package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wojons/muster/pkg/repository"
)

// aliasRepository implements repository.AliasRepository.
type aliasRepository struct {
	base *sqlRepository
	qs   *QuerySet
}

// NewAliasRepository creates a new AliasRepository.
func NewAliasRepository(db *sql.DB, driver Driver) (*aliasRepository, error) {
	base := NewRepository(db, driver)
	qs, err := GetQuerySet(driver)
	if err != nil {
		return nil, err
	}
	return &aliasRepository{base: base, qs: qs}, nil
}

// Create creates a new alias.
func (r *aliasRepository) Create(ctx context.Context, alias *repository.Alias) error {
	result, err := r.base.exec(ctx, r.qs.AliasCreate, alias.Name, alias.Description, alias.InventoryID, alias.ModuleID)
	if err != nil {
		return fmt.Errorf("failed to create alias: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	alias.ID = id
	alias.CreatedAt = now()
	alias.UpdatedAt = alias.CreatedAt
	return nil
}

// GetByID retrieves an alias by ID.
func (r *aliasRepository) GetByID(ctx context.Context, id int64) (*repository.Alias, error) {
	alias := &repository.Alias{}
	row := r.base.queryRow(ctx, r.qs.AliasGetByID, id)

	err := row.Scan(
		&alias.ID,
		&alias.Name,
		&alias.Description,
		&alias.InventoryID,
		&alias.ModuleID,
		&alias.CreatedAt,
		&alias.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "alias", ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get alias: %w", err)
	}

	return alias, nil
}

// GetByName retrieves an alias by name.
func (r *aliasRepository) GetByName(ctx context.Context, name string) (*repository.Alias, error) {
	alias := &repository.Alias{}
	row := r.base.queryRow(ctx, r.qs.AliasGetByName, name)

	err := row.Scan(
		&alias.ID,
		&alias.Name,
		&alias.Description,
		&alias.InventoryID,
		&alias.ModuleID,
		&alias.CreatedAt,
		&alias.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "alias", ID: 0}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get alias by name: %w", err)
	}

	return alias, nil
}

// GetByInventory retrieves all aliases for an inventory.
func (r *aliasRepository) GetByInventory(ctx context.Context, inventoryID int64, opts repository.ListOptions) ([]*repository.Alias, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	rows, err := r.base.query(ctx, r.qs.AliasGetByInventory, inventoryID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list aliases: %w", err)
	}
	defer rows.Close()

	var aliases []*repository.Alias
	for rows.Next() {
		alias := &repository.Alias{}
		err := rows.Scan(
			&alias.ID,
			&alias.Name,
			&alias.Description,
			&alias.InventoryID,
			&alias.ModuleID,
			&alias.CreatedAt,
			&alias.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan alias: %w", err)
		}
		aliases = append(aliases, alias)
	}

	return aliases, rows.Err()
}

// GetByModule retrieves all aliases for a module.
func (r *aliasRepository) GetByModule(ctx context.Context, moduleID int64, opts repository.ListOptions) ([]*repository.Alias, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	rows, err := r.base.query(ctx, r.qs.AliasGetByModule, moduleID, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list aliases by module: %w", err)
	}
	defer rows.Close()

	var aliases []*repository.Alias
	for rows.Next() {
		alias := &repository.Alias{}
		err := rows.Scan(
			&alias.ID,
			&alias.Name,
			&alias.Description,
			&alias.InventoryID,
			&alias.ModuleID,
			&alias.CreatedAt,
			&alias.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan alias: %w", err)
		}
		aliases = append(aliases, alias)
	}

	return aliases, rows.Err()
}

// Update updates an existing alias.
func (r *aliasRepository) Update(ctx context.Context, alias *repository.Alias) error {
	alias.UpdatedAt = now()
	_, err := r.base.exec(ctx, r.qs.AliasUpdate, alias.Name, alias.Description, alias.InventoryID, alias.ModuleID, alias.UpdatedAt, alias.ID)
	if err != nil {
		return fmt.Errorf("failed to update alias: %w", err)
	}
	return nil
}

// Delete removes an alias by ID.
func (r *aliasRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.base.exec(ctx, r.qs.AliasDelete, id)
	if err != nil {
		return fmt.Errorf("failed to delete alias: %w", err)
	}
	return nil
}

// DeleteByInventory removes all aliases for an inventory.
func (r *aliasRepository) DeleteByInventory(ctx context.Context, inventoryID int64) error {
	_, err := r.base.exec(ctx, r.qs.AliasDeleteByInventory, inventoryID)
	if err != nil {
		return fmt.Errorf("failed to delete aliases by inventory: %w", err)
	}
	return nil
}
