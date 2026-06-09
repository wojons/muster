// Package sql provides SQL repository implementations.
package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/wojons/muster/pkg/repository"
)

// inventoryRepository implements repository.InventoryRepository.
type inventoryRepository struct {
	base *sqlRepository
	qs   *QuerySet
}

// NewInventoryRepository creates a new InventoryRepository.
func NewInventoryRepository(db *sql.DB, driver Driver) (*inventoryRepository, error) {
	base := NewRepository(db, driver)
	qs, err := GetQuerySet(driver)
	if err != nil {
		return nil, err
	}
	return &inventoryRepository{base: base, qs: qs}, nil
}

// Create creates a new inventory.
func (r *inventoryRepository) Create(ctx context.Context, inv *repository.Inventory) error {
	result, err := r.base.exec(ctx, r.qs.InventoryCreate, inv.Name, inv.Description, inv.Path)
	if err != nil {
		return fmt.Errorf("failed to create inventory: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}

	inv.ID = id
	inv.CreatedAt = now()
	inv.UpdatedAt = inv.CreatedAt
	return nil
}

// GetByID retrieves an inventory by ID.
func (r *inventoryRepository) GetByID(ctx context.Context, id int64) (*repository.Inventory, error) {
	inv := &repository.Inventory{}
	row := r.base.queryRow(ctx, r.qs.InventoryGetByID, id)

	err := row.Scan(
		&inv.ID,
		&inv.Name,
		&inv.Description,
		&inv.Path,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "inventory", ID: id}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory: %w", err)
	}

	return inv, nil
}

// GetByAlias retrieves an inventory by its alias name.
func (r *inventoryRepository) GetByAlias(ctx context.Context, alias string) (*repository.Inventory, error) {
	inv := &repository.Inventory{}
	row := r.base.queryRow(ctx, r.qs.InventoryGetByAlias, alias)

	err := row.Scan(
		&inv.ID,
		&inv.Name,
		&inv.Description,
		&inv.Path,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "inventory", ID: 0}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory by alias: %w", err)
	}

	return inv, nil
}

// GetByModule retrieves an inventory by a module ID.
func (r *inventoryRepository) GetByModule(ctx context.Context, moduleID int64) (*repository.Inventory, error) {
	inv := &repository.Inventory{}
	row := r.base.queryRow(ctx, r.qs.InventoryGetByModule, moduleID)

	err := row.Scan(
		&inv.ID,
		&inv.Name,
		&inv.Description,
		&inv.Path,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "inventory", ID: 0}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory by module: %w", err)
	}

	return inv, nil
}

// Update updates an existing inventory.
func (r *inventoryRepository) Update(ctx context.Context, inv *repository.Inventory) error {
	inv.UpdatedAt = now()
	_, err := r.base.exec(ctx, r.qs.InventoryUpdate, inv.Name, inv.Description, inv.Path, inv.UpdatedAt, inv.ID)
	if err != nil {
		return fmt.Errorf("failed to update inventory: %w", err)
	}
	return nil
}

// Delete removes an inventory by ID.
func (r *inventoryRepository) Delete(ctx context.Context, id int64) error {
	_, err := r.base.exec(ctx, r.qs.InventoryDelete, id)
	if err != nil {
		return fmt.Errorf("failed to delete inventory: %w", err)
	}
	return nil
}

// List retrieves inventories with pagination.
func (r *inventoryRepository) List(ctx context.Context, opts repository.ListOptions) ([]*repository.Inventory, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	rows, err := r.base.query(ctx, r.qs.InventoryList, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list inventories: %w", err)
	}
	defer rows.Close()

	var inventories []*repository.Inventory
	for rows.Next() {
		inv := &repository.Inventory{}
		err := rows.Scan(
			&inv.ID,
			&inv.Name,
			&inv.Description,
			&inv.Path,
			&inv.CreatedAt,
			&inv.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan inventory: %w", err)
		}
		inventories = append(inventories, inv)
	}

	return inventories, rows.Err()
}

// Count returns the total number of inventories.
func (r *inventoryRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.base.queryRow(ctx, r.qs.InventoryCount).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count inventories: %w", err)
	}
	return count, nil
}

// Search searches inventories by name or description.
func (r *inventoryRepository) Search(ctx context.Context, query string, opts repository.ListOptions) ([]*repository.Inventory, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	likeQuery := "%" + query + "%"
	rows, err := r.base.query(ctx, r.qs.InventorySearch, likeQuery, likeQuery, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to search inventories: %w", err)
	}
	defer rows.Close()

	var inventories []*repository.Inventory
	for rows.Next() {
		inv := &repository.Inventory{}
		err := rows.Scan(
			&inv.ID,
			&inv.Name,
			&inv.Description,
			&inv.Path,
			&inv.CreatedAt,
			&inv.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan inventory: %w", err)
		}
		inventories = append(inventories, inv)
	}

	return inventories, rows.Err()
}
