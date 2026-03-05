# Repository Implementations

Complete implementation details for all repository types.

## Query Definitions

```go
// queries.go defines SQL queries for all database drivers.
package sql

import "fmt"

// QuerySet contains SQL queries for a specific driver.
type QuerySet struct {
	// Inventory queries
	InventoryCreate      string
	InventoryGetByID     string
	InventoryGetByAlias  string
	InventoryGetByModule string
	InventoryUpdate      string
	InventoryDelete      string
	InventoryList        string
	InventoryCount       string
	InventorySearch      string
	
	// Alias queries
	AliasCreate            string
	AliasGetByID           string
	AliasGetByName         string
	AliasGetByInventory    string
	AliasGetByModule       string
	AliasUpdate            string
	AliasDelete            string
	AliasDeleteByInventory string
	
	// Module queries
	ModuleCreate           string
	ModuleGetByID          string
	ModuleGetByPath        string
	ModuleGetByInventory   string
	ModuleUpdate           string
	ModuleDelete           string
	ModuleGetOrphans       string
	ModuleCountByInventory string
	
	// Spec queries
	SpecCreate         string
	SpecGetByID        string
	SpecGetByAlias     string
	SpecGetByModule    string
	SpecUpdate         string
	SpecDelete         string
	SpecDeleteByAlias  string
	SpecDeleteByModule string
	
	// Fingerprint queries
	FingerprintCreate       string
	FingerprintGetByID      string
	FingerprintGetBySpec    string
	FingerprintGetByHash    string
	FingerprintUpdate       string
	FingerprintDelete       string
	FingerprintDeleteBySpec string
	FingerprintExists       string
	
	// Audit queries
	AuditCreate      string
	AuditGetByID     string
	AuditGetByEntity string
	AuditList        string
	AuditCount       string
	AuditCleanup     string
	
	// RateLimit queries
	RateLimitCreate      string
	RateLimitGetByID     string
	RateLimitGetByKey    string
	RateLimitUpdate      string
	RateLimitDelete      string
	RateLimitDeleteByKey string
	RateLimitCleanup     string
	RateLimitIncrement   string
}

// querySets holds queries for each supported driver.
var querySets = map[Driver]*QuerySet{
	DriverSQLite:     sqliteQueries,
	DriverPostgreSQL: postgresQueries,
	DriverMySQL:      mysqlQueries,
}

// GetQuerySet returns the query set for the specified driver.
func GetQuerySet(driver Driver) (*QuerySet, error) {
	qs, ok := querySets[driver]
	if !ok {
		return nil, fmt.Errorf("unsupported driver: %s", driver)
	}
	return qs, nil
}

// now returns the current time (helper for mocking in tests).
func now() time.Time {
	return time.Now().UTC()
}
```

## Inventory Repository Implementation

```go
// inventory_repository.go implements InventoryRepository for SQL databases.
package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	
	"muster/internal/repository"
)

// sqlInventoryRepository implements repository.InventoryRepository.
type sqlInventoryRepository struct {
	*sqlRepository
	queries *QuerySet
}

// NewInventoryRepository creates a new InventoryRepository.
func NewInventoryRepository(repo *sqlRepository) repository.InventoryRepository {
	return &sqlInventoryRepository{
		sqlRepository: repo,
		queries:       querySets[repo.driver],
	}
}

// Create creates a new inventory.
func (r *sqlInventoryRepository) Create(ctx context.Context, inv *repository.Inventory) error {
	query := r.queries.InventoryCreate
	
	result, err := r.exec(ctx, query, inv.Name, inv.Description, inv.Path)
	if err != nil {
		return fmt.Errorf("failed to create inventory: %w", mapError(err))
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		if r.driver == DriverPostgreSQL {
			return nil
		}
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	
	inv.ID = id
	inv.CreatedAt = now()
	inv.UpdatedAt = inv.CreatedAt
	
	return nil
}

// GetByID retrieves an inventory by ID.
func (r *sqlInventoryRepository) GetByID(ctx context.Context, id int64) (*repository.Inventory, error) {
	query := r.queries.InventoryGetByID
	
	inv := &repository.Inventory{}
	row := r.queryRow(ctx, query, id)
	
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
		return nil, fmt.Errorf("failed to get inventory: %w", mapError(err))
	}
	
	return inv, nil
}

// GetByAlias retrieves an inventory by its alias name.
func (r *sqlInventoryRepository) GetByAlias(ctx context.Context, alias string) (*repository.Inventory, error) {
	query := r.queries.InventoryGetByAlias
	
	inv := &repository.Inventory{}
	row := r.queryRow(ctx, query, alias)
	
	err := row.Scan(
		&inv.ID,
		&inv.Name,
		&inv.Description,
		&inv.Path,
		&inv.CreatedAt,
		&inv.UpdatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "inventory", Key: "alias", Value: alias}
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory by alias: %w", mapError(err))
	}
	
	return inv, nil
}

// GetByModule retrieves inventories containing a module.
func (r *sqlInventoryRepository) GetByModule(ctx context.Context, moduleID int64) ([]*repository.Inventory, error) {
	query := r.queries.InventoryGetByModule
	
	rows, err := r.query(ctx, query, moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get inventories by module: %w", mapError(err))
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
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating inventories: %w", err)
	}
	
	return inventories, nil
}

// Update updates an existing inventory.
func (r *sqlInventoryRepository) Update(ctx context.Context, inv *repository.Inventory) error {
	query := r.queries.InventoryUpdate
	
	_, err := r.exec(ctx, query, inv.Name, inv.Description, inv.Path, inv.ID)
	if err != nil {
		return fmt.Errorf("failed to update inventory: %w", mapError(err))
	}
	
	inv.UpdatedAt = now()
	return nil
}

// Delete deletes an inventory by ID.
func (r *sqlInventoryRepository) Delete(ctx context.Context, id int64) error {
	query := r.queries.InventoryDelete
	
	result, err := r.exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete inventory: %w", mapError(err))
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return repository.ErrNotFound{Entity: "inventory", ID: id}
	}
	
	return nil
}

// List retrieves a paginated list of inventories.
func (r *sqlInventoryRepository) List(ctx context.Context, opts repository.ListOptions) ([]*repository.Inventory, error) {
	query := buildListQuery(r.queries.InventoryList, opts, r.driver)
	
	rows, err := r.query(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list inventories: %w", mapError(err))
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
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating inventories: %w", err)
	}
	
	return inventories, nil
}

// Count returns the total count of inventories matching the options.
func (r *sqlInventoryRepository) Count(ctx context.Context, opts repository.ListOptions) (int64, error) {
	query := r.queries.InventoryCount
	
	var count int64
	err := r.queryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count inventories: %w", mapError(err))
	}
	
	return count, nil
}

// Search searches inventories by name or description.
func (r *sqlInventoryRepository) Search(ctx context.Context, searchQuery string, opts repository.ListOptions) ([]*repository.Inventory, error) {
	query := buildListQuery(r.queries.InventorySearch, opts, r.driver)
	
	searchPattern := "%" + searchQuery + "%"
	rows, err := r.query(ctx, query, searchPattern, searchPattern, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to search inventories: %w", mapError(err))
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
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating inventories: %w", err)
	}
	
	return inventories, nil
}

// buildListQuery builds a list query with pagination and ordering.
func buildListQuery(base string, opts repository.ListOptions, driver Driver) string {
	query := base
	
	// Add ORDER BY
	if opts.OrderBy != "" {
		order := opts.Order
		if order != "ASC" && order != "DESC" {
			order = "ASC"
		}
		query = fmt.Sprintf("%s ORDER BY %s %s", query, opts.OrderBy, order)
	}
	
	// Add LIMIT
	if opts.Limit > 0 {
		query = fmt.Sprintf("%s LIMIT %d", query, opts.Limit)
		
		// Add OFFSET
		if opts.Offset > 0 {
			query = fmt.Sprintf("%s OFFSET %d", query, opts.Offset)
		}
	}
	
	return query
}
```

## Alias Repository Implementation

```go
// alias_repository.go implements AliasRepository for SQL databases.
package sql

import (
	"context"
	"database/sql"
	"fmt"
	
	"muster/internal/repository"
)

// sqlAliasRepository implements repository.AliasRepository.
type sqlAliasRepository struct {
	*sqlRepository
	queries *QuerySet
}

// NewAliasRepository creates a new AliasRepository.
func NewAliasRepository(repo *sqlRepository) repository.AliasRepository {
	return &sqlAliasRepository{
		sqlRepository: repo,
		queries:       querySets[repo.driver],
	}
}

// Create creates a new alias.
func (r *sqlAliasRepository) Create(ctx context.Context, alias *repository.Alias) error {
	query := r.queries.AliasCreate
	
	result, err := r.exec(ctx, query, alias.Name, alias.Description, alias.InventoryID, alias.ModuleID)
	if err != nil {
		return fmt.Errorf("failed to create alias: %w", mapError(err))
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		if r.driver == DriverPostgreSQL {
			return nil
		}
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	
	alias.ID = id
	alias.CreatedAt = now()
	alias.UpdatedAt = alias.CreatedAt
	
	return nil
}

// GetByID retrieves an alias by ID.
func (r *sqlAliasRepository) GetByID(ctx context.Context, id int64) (*repository.Alias, error) {
	query := r.queries.AliasGetByID
	
	alias := &repository.Alias{}
	row := r.queryRow(ctx, query, id)
	
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
		return nil, fmt.Errorf("failed to get alias: %w", mapError(err))
	}
	
	return alias, nil
}

// GetByName retrieves an alias by its name.
func (r *sqlAliasRepository) GetByName(ctx context.Context, name string) (*repository.Alias, error) {
	query := r.queries.AliasGetByName
	
	alias := &repository.Alias{}
	row := r.queryRow(ctx, query, name)
	
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
		return nil, repository.ErrNotFound{Entity: "alias", Key: "name", Value: name}
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get alias by name: %w", mapError(err))
	}
	
	return alias, nil
}

// GetByInventory retrieves all aliases for an inventory.
func (r *sqlAliasRepository) GetByInventory(ctx context.Context, inventoryID int64) ([]*repository.Alias, error) {
	query := r.queries.AliasGetByInventory
	
	rows, err := r.query(ctx, query, inventoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to get aliases by inventory: %w", mapError(err))
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
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating aliases: %w", err)
	}
	
	return aliases, nil
}

// GetByModule retrieves all aliases for a module.
func (r *sqlAliasRepository) GetByModule(ctx context.Context, moduleID int64) ([]*repository.Alias, error) {
	query := r.queries.AliasGetByModule
	
	rows, err := r.query(ctx, query, moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get aliases by module: %w", mapError(err))
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
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating aliases: %w", err)
	}
	
	return aliases, nil
}

// Update updates an existing alias.
func (r *sqlAliasRepository) Update(ctx context.Context, alias *repository.Alias) error {
	query := r.queries.AliasUpdate
	
	_, err := r.exec(ctx, query, alias.Name, alias.Description, alias.ModuleID, alias.ID)
	if err != nil {
		return fmt.Errorf("failed to update alias: %w", mapError(err))
	}
	
	alias.UpdatedAt = now()
	return nil
}

// Delete deletes an alias by ID.
func (r *sqlAliasRepository) Delete(ctx context.Context, id int64) error {
	query := r.queries.AliasDelete
	
	result, err := r.exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete alias: %w", mapError(err))
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return repository.ErrNotFound{Entity: "alias", ID: id}
	}
	
	return nil
}

// DeleteByInventory deletes all aliases for an inventory.
func (r *sqlAliasRepository) DeleteByInventory(ctx context.Context, inventoryID int64) error {
	query := r.queries.AliasDeleteByInventory
	
	_, err := r.exec(ctx, query, inventoryID)
	if err != nil {
		return fmt.Errorf("failed to delete aliases by inventory: %w", mapError(err))
	}
	
	return nil
}
```

## Module Repository Implementation

```go
// module_repository.go implements ModuleRepository for SQL databases.
package sql

import (
	"context"
	"database/sql"
	"fmt"
	
	"muster/internal/repository"
)

// sqlModuleRepository implements repository.ModuleRepository.
type sqlModuleRepository struct {
	*sqlRepository
	queries *QuerySet
}

// NewModuleRepository creates a new ModuleRepository.
func NewModuleRepository(repo *sqlRepository) repository.ModuleRepository {
	return &sqlModuleRepository{
		sqlRepository: repo,
		queries:       querySets[repo.driver],
	}
}

// Create creates a new module.
func (r *sqlModuleRepository) Create(ctx context.Context, mod *repository.Module) error {
	query := r.queries.ModuleCreate
	
	result, err := r.exec(ctx, query, mod.Name, mod.Path, mod.Language, mod.InventoryID)
	if err != nil {
		return fmt.Errorf("failed to create module: %w", mapError(err))
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		if r.driver == DriverPostgreSQL {
			return nil
		}
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	
	mod.ID = id
	mod.CreatedAt = now()
	mod.UpdatedAt = mod.CreatedAt
	
	return nil
}

// GetByID retrieves a module by ID.
func (r *sqlModuleRepository) GetByID(ctx context.Context, id int64) (*repository.Module, error) {
	query := r.queries.ModuleGetByID
	
	mod := &repository.Module{}
	row := r.queryRow(ctx, query, id)
	
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
		return nil, fmt.Errorf("failed to get module: %w", mapError(err))
	}
	
	return mod, nil
}

// GetByPath retrieves a module by its filesystem path.
func (r *sqlModuleRepository) GetByPath(ctx context.Context, path string) (*repository.Module, error) {
	query := r.queries.ModuleGetByPath
	
	mod := &repository.Module{}
	row := r.queryRow(ctx, query, path)
	
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
		return nil, repository.ErrNotFound{Entity: "module", Key: "path", Value: path}
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get module by path: %w", mapError(err))
	}
	
	return mod, nil
}

// GetByInventory retrieves all modules for an inventory.
func (r *sqlModuleRepository) GetByInventory(ctx context.Context, inventoryID int64) ([]*repository.Module, error) {
	query := r.queries.ModuleGetByInventory
	
	rows, err := r.query(ctx, query, inventoryID)
	if err != nil {
		return nil, fmt.Errorf("failed to get modules by inventory: %w", mapError(err))
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
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating modules: %w", err)
	}
	
	return modules, nil
}

// Update updates an existing module.
func (r *sqlModuleRepository) Update(ctx context.Context, mod *repository.Module) error {
	query := r.queries.ModuleUpdate
	
	_, err := r.exec(ctx, query, mod.Name, mod.Path, mod.Language, mod.ID)
	if err != nil {
		return fmt.Errorf("failed to update module: %w", mapError(err))
	}
	
	mod.UpdatedAt = now()
	return nil
}

// Delete deletes a module by ID.
func (r *sqlModuleRepository) Delete(ctx context.Context, id int64) error {
	query := r.queries.ModuleDelete
	
	result, err := r.exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete module: %w", mapError(err))
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return repository.ErrNotFound{Entity: "module", ID: id}
	}
	
	return nil
}

// GetOrphans retrieves modules not associated with any inventory.
func (r *sqlModuleRepository) GetOrphans(ctx context.Context) ([]*repository.Module, error) {
	query := r.queries.ModuleGetOrphans
	
	rows, err := r.query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get orphan modules: %w", mapError(err))
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
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating modules: %w", err)
	}
	
	return modules, nil
}

// CountByInventory counts modules in an inventory.
func (r *sqlModuleRepository) CountByInventory(ctx context.Context, inventoryID int64) (int64, error) {
	query := r.queries.ModuleCountByInventory
	
	var count int64
	err := r.queryRow(ctx, query, inventoryID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count modules: %w", mapError(err))
	}
	
	return count, nil
}
```

## Spec Repository Implementation

```go
// spec_repository.go implements SpecRepository for SQL databases.
package sql

import (
	"context"
	"database/sql"
	"fmt"
	
	"muster/internal/repository"
)

// sqlSpecRepository implements repository.SpecRepository.
type sqlSpecRepository struct {
	*sqlRepository
	queries *QuerySet
}

// NewSpecRepository creates a new SpecRepository.
func NewSpecRepository(repo *sqlRepository) repository.SpecRepository {
	return &sqlSpecRepository{
		sqlRepository: repo,
		queries:       querySets[repo.driver],
	}
}

// Create creates a new spec.
func (r *sqlSpecRepository) Create(ctx context.Context, spec *repository.Spec) error {
	query := r.queries.SpecCreate
	
	result, err := r.exec(ctx, query, spec.Name, spec.Content, spec.AliasID, spec.ModuleID)
	if err != nil {
		return fmt.Errorf("failed to create spec: %w", mapError(err))
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		if r.driver == DriverPostgreSQL {
			return nil
		}
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	
	spec.ID = id
	spec.CreatedAt = now()
	spec.UpdatedAt = spec.CreatedAt
	
	return nil
}

// GetByID retrieves a spec by ID.
func (r *sqlSpecRepository) GetByID(ctx context.Context, id int64) (*repository.Spec, error) {
	query := r.queries.SpecGetByID
	
	spec := &repository.Spec{}
	row := r.queryRow(ctx, query, id)
	
	err := row.Scan(
		&spec.ID,
		&spec.Name,
		&spec.Content,
		&spec.AliasID,
		&spec.ModuleID,
		&spec.CreatedAt,
		&spec.UpdatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "spec", ID: id}
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get spec: %w", mapError(err))
	}
	
	return spec, nil
}

// GetByAlias retrieves all specs for an alias.
func (r *sqlSpecRepository) GetByAlias(ctx context.Context, aliasID int64) ([]*repository.Spec, error) {
	query := r.queries.SpecGetByAlias
	
	rows, err := r.query(ctx, query, aliasID)
	if err != nil {
		return nil, fmt.Errorf("failed to get specs by alias: %w", mapError(err))
	}
	defer rows.Close()
	
	var specs []*repository.Spec
	for rows.Next() {
		spec := &repository.Spec{}
		err := rows.Scan(
			&spec.ID,
			&spec.Name,
			&spec.Content,
			&spec.AliasID,
			&spec.ModuleID,
			&spec.CreatedAt,
			&spec.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan spec: %w", err)
		}
		specs = append(specs, spec)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating specs: %w", err)
	}
	
	return specs, nil
}

// GetByModule retrieves all specs for a module.
func (r *sqlSpecRepository) GetByModule(ctx context.Context, moduleID int64) ([]*repository.Spec, error) {
	query := r.queries.SpecGetByModule
	
	rows, err := r.query(ctx, query, moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get specs by module: %w", mapError(err))
	}
	defer rows.Close()
	
	var specs []*repository.Spec
	for rows.Next() {
		spec := &repository.Spec{}
		err := rows.Scan(
			&spec.ID,
			&spec.Name,
			&spec.Content,
			&spec.AliasID,
			&spec.ModuleID,
			&spec.CreatedAt,
			&spec.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan spec: %w", err)
		}
		specs = append(specs, spec)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating specs: %w", err)
	}
	
	return specs, nil
}

// Update updates an existing spec.
func (r *sqlSpecRepository) Update(ctx context.Context, spec *repository.Spec) error {
	query := r.queries.SpecUpdate
	
	_, err := r.exec(ctx, query, spec.Name, spec.Content, spec.ID)
	if err != nil {
		return fmt.Errorf("failed to update spec: %w", mapError(err))
	}
	
	spec.UpdatedAt = now()
	return nil
}

// Delete deletes a spec by ID.
func (r *sqlSpecRepository) Delete(ctx context.Context, id int64) error {
	query := r.queries.SpecDelete
	
	result, err := r.exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete spec: %w", mapError(err))
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return repository.ErrNotFound{Entity: "spec", ID: id}
	}
	
	return nil
}

// DeleteByAlias deletes all specs for an alias.
func (r *sqlSpecRepository) DeleteByAlias(ctx context.Context, aliasID int64) error {
	query := r.queries.SpecDeleteByAlias
	
	_, err := r.exec(ctx, query, aliasID)
	if err != nil {
		return fmt.Errorf("failed to delete specs by alias: %w", mapError(err))
	}
	
	return nil
}

// DeleteByModule deletes all specs for a module.
func (r *sqlSpecRepository) DeleteByModule(ctx context.Context, moduleID int64) error {
	query := r.queries.SpecDeleteByModule
	
	_, err := r.exec(ctx, query, moduleID)
	if err != nil {
		return fmt.Errorf("failed to delete specs by module: %w", mapError(err))
	}
	
	return nil
}
```

## Fingerprint Repository Implementation

```go
// fingerprint_repository.go implements FingerprintRepository for SQL databases.
package sql

import (
	"context"
	"database/sql"
	"fmt"
	
	"muster/internal/repository"
)

// sqlFingerprintRepository implements repository.FingerprintRepository.
type sqlFingerprintRepository struct {
	*sqlRepository
	queries *QuerySet
}

// NewFingerprintRepository creates a new FingerprintRepository.
func NewFingerprintRepository(repo *sqlRepository) repository.FingerprintRepository {
	return &sqlFingerprintRepository{
		sqlRepository: repo,
		queries:       querySets[repo.driver],
	}
}

// Create creates a new fingerprint.
func (r *sqlFingerprintRepository) Create(ctx context.Context, fp *repository.Fingerprint) error {
	query := r.queries.FingerprintCreate
	
	result, err := r.exec(ctx, query, fp.Hash, fp.SpecID, fp.Algorithm)
	if err != nil {
		return fmt.Errorf("failed to create fingerprint: %w", mapError(err))
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		if r.driver == DriverPostgreSQL {
			return nil
		}
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	
	fp.ID = id
	fp.CreatedAt = now()
	
	return nil
}

// GetByID retrieves a fingerprint by ID.
func (r *sqlFingerprintRepository) GetByID(ctx context.Context, id int64) (*repository.Fingerprint, error) {
	query := r.queries.FingerprintGetByID
	
	fp := &repository.Fingerprint{}
	row := r.queryRow(ctx, query, id)
	
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
		return nil, fmt.Errorf("failed to get fingerprint: %w", mapError(err))
	}
	
	return fp, nil
}

// GetBySpec retrieves all fingerprints for a spec.
func (r *sqlFingerprintRepository) GetBySpec(ctx context.Context, specID int64) ([]*repository.Fingerprint, error) {
	query := r.queries.FingerprintGetBySpec
	
	rows, err := r.query(ctx, query, specID)
	if err != nil {
		return nil, fmt.Errorf("failed to get fingerprints by spec: %w", mapError(err))
	}
	defer rows.Close()
	
	var fingerprints []*repository.Fingerprint
	for rows.Next() {
		fp := &repository.Fingerprint{}
		err := rows.Scan(
			&fp.ID,
			&fp.Hash,
			&fp.SpecID,
			&fp.Algorithm,
			&fp.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan fingerprint: %w", err)
		}
		fingerprints = append(fingerprints, fp)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating fingerprints: %w", err)
	}
	
	return fingerprints, nil
}

// GetByHash retrieves a fingerprint by its hash.
func (r *sqlFingerprintRepository) GetByHash(ctx context.Context, hash string) (*repository.Fingerprint, error) {
	query := r.queries.FingerprintGetByHash
	
	fp := &repository.Fingerprint{}
	row := r.queryRow(ctx, query, hash)
	
	err := row.Scan(
		&fp.ID,
		&fp.Hash,
		&fp.SpecID,
		&fp.Algorithm,
		&fp.CreatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "fingerprint", Key: "hash", Value: hash}
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get fingerprint by hash: %w", mapError(err))
	}
	
	return fp, nil
}

// Update updates an existing fingerprint.
func (r *sqlFingerprintRepository) Update(ctx context.Context, fp *repository.Fingerprint) error {
	query := r.queries.FingerprintUpdate
	
	_, err := r.exec(ctx, query, fp.Hash, fp.Algorithm, fp.ID)
	if err != nil {
		return fmt.Errorf("failed to update fingerprint: %w", mapError(err))
	}
	
	return nil
}

// Delete deletes a fingerprint by ID.
func (r *sqlFingerprintRepository) Delete(ctx context.Context, id int64) error {
	query := r.queries.FingerprintDelete
	
	result, err := r.exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete fingerprint: %w", mapError(err))
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return repository.ErrNotFound{Entity: "fingerprint", ID: id}
	}
	
	return nil
}

// DeleteBySpec deletes all fingerprints for a spec.
func (r *sqlFingerprintRepository) DeleteBySpec(ctx context.Context, specID int64) error {
	query := r.queries.FingerprintDeleteBySpec
	
	_, err := r.exec(ctx, query, specID)
	if err != nil {
		return fmt.Errorf("failed to delete fingerprints by spec: %w", mapError(err))
	}
	
	return nil
}

// Exists checks if a fingerprint with the given hash exists.
func (r *sqlFingerprintRepository) Exists(ctx context.Context, hash string) (bool, error) {
	query := r.queries.FingerprintExists
	
	var exists bool
	err := r.queryRow(ctx, query, hash).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check fingerprint existence: %w", mapError(err))
	}
	
	return exists, nil
}
```

## Audit Repository Implementation

```go
// audit_repository.go implements AuditRepository for SQL databases.
package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	
	"muster/internal/repository"
)

// sqlAuditRepository implements repository.AuditRepository.
type sqlAuditRepository struct {
	*sqlRepository
	queries *QuerySet
}

// NewAuditRepository creates a new AuditRepository.
func NewAuditRepository(repo *sqlRepository) repository.AuditRepository {
	return &sqlAuditRepository{
		sqlRepository: repo,
		queries:       querySets[repo.driver],
	}
}

// Create creates a new audit log entry.
func (r *sqlAuditRepository) Create(ctx context.Context, audit *repository.Audit) error {
	query := r.queries.AuditCreate
	
	result, err := r.exec(ctx, query, audit.Action, audit.EntityType, audit.EntityID, audit.UserID, audit.Data)
	if err != nil {
		return fmt.Errorf("failed to create audit log: %w", mapError(err))
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		if r.driver == DriverPostgreSQL {
			return nil
		}
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	
	audit.ID = id
	audit.CreatedAt = now()
	
	return nil
}

// GetByID retrieves an audit log by ID.
func (r *sqlAuditRepository) GetByID(ctx context.Context, id int64) (*repository.Audit, error) {
	query := r.queries.AuditGetByID
	
	audit := &repository.Audit{}
	row := r.queryRow(ctx, query, id)
	
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
		return nil, fmt.Errorf("failed to get audit log: %w", mapError(err))
	}
	
	return audit, nil
}

// GetByEntity retrieves audit logs for a specific entity.
func (r *sqlAuditRepository) GetByEntity(ctx context.Context, entityType string, entityID int64) ([]*repository.Audit, error) {
	query := r.queries.AuditGetByEntity
	
	rows, err := r.query(ctx, query, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("failed to get audit logs by entity: %w", mapError(err))
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
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit logs: %w", err)
	}
	
	return audits, nil
}

// List retrieves a paginated list of audit logs.
func (r *sqlAuditRepository) List(ctx context.Context, opts repository.ListOptions) ([]*repository.Audit, error) {
	query := buildListQuery(r.queries.AuditList, opts, r.driver)
	
	rows, err := r.query(ctx, query, opts.Limit, opts.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list audit logs: %w", mapError(err))
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
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit logs: %w", err)
	}
	
	return audits, nil
}

// Count returns the total count of audit logs.
func (r *sqlAuditRepository) Count(ctx context.Context, opts repository.ListOptions) (int64, error) {
	query := r.queries.AuditCount
	
	var count int64
	err := r.queryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count audit logs: %w", mapError(err))
	}
	
	return count, nil
}

// Cleanup removes audit logs older than the specified time.
func (r *sqlAuditRepository) Cleanup(ctx context.Context, before time.Time) (int64, error) {
	query := r.queries.AuditCleanup
	
	result, err := r.exec(ctx, query, before)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup audit logs: %w", mapError(err))
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	return rowsAffected, nil
}
```

## RateLimit Repository Implementation

```go
// ratelimit_repository.go implements RateLimitRepository for SQL databases.
package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	
	"muster/internal/repository"
)

// sqlRateLimitRepository implements repository.RateLimitRepository.
type sqlRateLimitRepository struct {
	*sqlRepository
	queries *QuerySet
}

// NewRateLimitRepository creates a new RateLimitRepository.
func NewRateLimitRepository(repo *sqlRepository) repository.RateLimitRepository {
	return &sqlRateLimitRepository{
		sqlRepository: repo,
		queries:       querySets[repo.driver],
	}
}

// Create creates a new rate limit entry.
func (r *sqlRateLimitRepository) Create(ctx context.Context, rl *repository.RateLimit) error {
	query := r.queries.RateLimitCreate
	
	result, err := r.exec(ctx, query, rl.Key, rl.Count, rl.Window, rl.ExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to create rate limit: %w", mapError(err))
	}
	
	id, err := result.LastInsertId()
	if err != nil {
		if r.driver == DriverPostgreSQL {
			return nil
		}
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	
	rl.ID = id
	rl.CreatedAt = now()
	
	return nil
}

// GetByID retrieves a rate limit by ID.
func (r *sqlRateLimitRepository) GetByID(ctx context.Context, id int64) (*repository.RateLimit, error) {
	query := r.queries.RateLimitGetByID
	
	rl := &repository.RateLimit{}
	row := r.queryRow(ctx, query, id)
	
	err := row.Scan(
		&rl.ID,
		&rl.Key,
		&rl.Count,
		&rl.Window,
		&rl.ExpiresAt,
		&rl.CreatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "ratelimit", ID: id}
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit: %w", mapError(err))
	}
	
	return rl, nil
}

// GetByKey retrieves a rate limit by its key.
func (r *sqlRateLimitRepository) GetByKey(ctx context.Context, key string) (*repository.RateLimit, error) {
	query := r.queries.RateLimitGetByKey
	
	rl := &repository.RateLimit{}
	row := r.queryRow(ctx, query, key)
	
	err := row.Scan(
		&rl.ID,
		&rl.Key,
		&rl.Count,
		&rl.Window,
		&rl.ExpiresAt,
		&rl.CreatedAt,
	)
	
	if err == sql.ErrNoRows {
		return nil, repository.ErrNotFound{Entity: "ratelimit", Key: "key", Value: key}
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get rate limit by key: %w", mapError(err))
	}
	
	return rl, nil
}

// Update updates an existing rate limit.
func (r *sqlRateLimitRepository) Update(ctx context.Context, rl *repository.RateLimit) error {
	query := r.queries.RateLimitUpdate
	
	_, err := r.exec(ctx, query, rl.Count, rl.Window, rl.ExpiresAt, rl.ID)
	if err != nil {
		return fmt.Errorf("failed to update rate limit: %w", mapError(err))
	}
	
	return nil
}

// Delete deletes a rate limit by ID.
func (r *sqlRateLimitRepository) Delete(ctx context.Context, id int64) error {
	query := r.queries.RateLimitDelete
	
	result, err := r.exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete rate limit: %w", mapError(err))
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	if rowsAffected == 0 {
		return repository.ErrNotFound{Entity: "ratelimit", ID: id}
	}
	
	return nil
}

// DeleteByKey deletes a rate limit by its key.
func (r *sqlRateLimitRepository) DeleteByKey(ctx context.Context, key string) error {
	query := r.queries.RateLimitDeleteByKey
	
	_, err := r.exec(ctx, query, key)
	if err != nil {
		return fmt.Errorf("failed to delete rate limit by key: %w", mapError(err))
	}
	
	return nil
}

// Cleanup removes expired rate limit entries.
func (r *sqlRateLimitRepository) Cleanup(ctx context.Context, before time.Time) (int64, error) {
	query := r.queries.RateLimitCleanup
	
	result, err := r.exec(ctx, query, before)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup rate limits: %w", mapError(err))
	}
	
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	
	return rowsAffected, nil
}

// Increment increments the request count for a key.
func (r *sqlRateLimitRepository) Increment(ctx context.Context, key string) error {
	query := r.queries.RateLimitIncrement
	
	_, err := r.exec(ctx, query, key)
	if err != nil {
		return fmt.Errorf("failed to increment rate limit: %w", mapError(err))
	}
	
	return nil
}
```
