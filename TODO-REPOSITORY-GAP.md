# Database Repository Layer Implementation Gap

**Date**: 2026-03-05
**Spec**: DATABASE-REPOSITORY-IMPLEMENTATIONS.md
**Status**: ❌ NOT IMPLEMENTED - CRITICAL GAP

## Overview

The DATABASE-REPOSITORY-IMPLEMENTATIONS.md spec defines complete SQL repository implementations for all database entities. However, the actual codebase has NO repository implementations - only basic connection management.

## Current State

### What Exists (minimal implementation):
```
pkg/repository/
├── repository.go          # Type definitions only (115 lines)
├── sqlite.go              # Connection manager only (135 lines)
└── repository_test.go     # Basic connection tests (98 lines)
```

**Total**: Only ~350 lines of code (connections and types only)

### What's Spec Required But Missing:
❌ SQL query definitions (~200 lines)
❌ sqlInventoryRepository implementation (~300 lines)
❌ sqlAliasRepository implementation (~250 lines)
❌ sqlModuleRepository implementation (~250 lines)
❌ sqlSpecRepository implementation (~250 lines)
❌ sqlFingerprintRepository implementation (~200 lines)
❌ sqlAuditRepository implementation (~200 lines)
❌ sqlRateLimitRepository implementation (~200 lines)

**Total Missing**: ~1,850 lines of repository implementation

## Critical Gaps Found

### 1. No CRUD Operations for Any Entity

**Entities with no repository implementation**:
- ❌ Inventory (no Create, GetByID, GetByAlias, Update, Delete, List, Search)
- ❌ Alias (no Create, GetByID, GetByName, GetByInventory, Update, Delete)
- ❌ Module (no Create, GetByID, GetByPath, GetByInventory, Update, Delete, GetOrphans)
- ❌ Spec (no Create, GetByID, GetByAlias, GetByModule, Update, Delete)
- ❌ Fingerprint (no Create, GetByID, GetBySpec, GetByHash, Update, Delete, Exists)
- ❌ Audit (no Create, GetByID, GetByEntity, List, Count, Cleanup)
- ❌ RateLimit (no Create, GetByID, GetByKey, Update, Delete, Cleanup, Increment)

**Impact**: System cannot persist or retrieve ANY data

### 2. Schema Mismatch

**Spec Expects** (pkg/repository/repository.go:70-76):
```go
type Spec struct {
    ID        int64
    Name      string
    Content   string
    AliasID   int64     // ✅ Expected foreign key
    ModuleID  int64     // ✅ Expected foreign key
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

**Migration Has** (migrations/00003_modules_aliases_specs.sql:34-40):
```sql
CREATE TABLE IF NOT EXISTS specs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    fingerprint VARCHAR(255),  -- ❌ Unexpected field
    -- Missing: alias_id, module_id
    ...
);
```

**Impact**: Spec's repository code won't compile against current schema

### 3. No Query Layer

**Spec Defines**:
- Driver-specific query sets (SQLite, PostgreSQL, MySQL)
- Parameterized queries with proper error handling
- Pagination, search, and counting operations
- Transaction support

**Current State**:
- No query definitions anywhere
- No SQL abstraction layer
- No query building utilities

### 4. No Repository Initialization Functions

**Spec Defines**:
- `NewInventoryRepository(sqlRepository) repository.InventoryRepository`
- `NewAliasRepository(sqlRepository) repository.AliasRepository`
- `NewModuleRepository(sqlRepository) repository.ModuleRepository`
- (plus 4 more for other entities)

**Current State**:
- No repository factory functions
- No way to create repository instances

## Database Migrations Exist But Are Not Used

**Present migrations**:
- ✅ migrations/00001_configs.sql
- ✅ migrations/00002_inventories.sql
- ✅ migrations/00003_modules_aliases_specs.sql
- ✅ migrations/00004_fingerprints.sql
- ✅ migrations/00005_audit.sql
- ✅ migrations/00006_rate_limits.sql

**Problem**: No code to call these migrations or use the tables they create

## What This Breaks

1. **Inventory Management**: Cannot create, read, update, or delete inventories
2. **Module Management**: Cannot track modules or their relationships
3. **Spec Management**: Cannot store or retrieve specs
4. **Audit Logging**: No audit trail of operations
5. **Rate Limiting**: Cannot enforce rate limits
6. **Fingerprinting**: Cannot track content hashes
7. **Any Data Persistence**: No ability to persist any data to database

## Impact Assessment

**Priority**: 🔴 CRITICAL (Blocker)

**Why it's Critical**:
1. System cannot function without data persistence
2. All database entities are currently unusable
3. Inventory, Module, Spec management completely broken
4. 1600+ lines of spec implementation completely missing

**Severity**: HIGH - Core functionality non-existent

**Recovery Time**: 2-3 days to implement complete repository layer with tests

## Required Implementation

### Files to Create (~1,850 lines of code)

1. **pkg/repository/sql/queries.go** (~200 lines)
   - QuerySet struct for all SQL queries
   - Driver-specific query sets (sqliteQueries, postgresQueries, mysqlQueries)
   - GetQuerySet function

2. **pkg/repository/sql/inventory_repository.go** (~300 lines)
   - sqlInventoryRepository struct
   - Create, GetByID, GetByAlias, GetByModule, Update, Delete, List, Count, Search methods

3. **pkg/repository/sql/alias_repository.go** (~250 lines)
   - sqlAliasRepository struct
   - Create, GetByID, GetByName, GetByInventory, GetByModule, Update, Delete, DeleteByInventory methods

4. **pkg/repository/sql/module_repository.go** (~250 lines)
   - sqlModuleRepository struct
   - Create, GetByID, GetByPath, GetByInventory, Update, Delete, GetOrphans, CountByInventory methods

5. **pkg/repository/sql/spec_repository.go** (~250 lines)
   - sqlSpecRepository struct
   - Create, GetByID, GetByAlias, GetByModule, Update, Delete, DeleteByAlias, DeleteByModule methods

6. **pkg/repository/sql/fingerprint_repository.go** (~200 lines)
   - sqlFingerprintRepository struct
   - Create, GetByID, GetBySpec, GetByHash, Update, Delete, DeleteBySpec, Exists methods

7. **pkg/repository/sql/audit_repository.go** (~200 lines)
   - sqlAuditRepository struct
   - Create, GetByID, GetByEntity, List, Count, Cleanup methods

8. **pkg/repository/sql/ratelimit_repository.go** (~200 lines)
   - sqlRateLimitRepository struct
   - Create, GetByID, GetByKey, Update, Delete, DeleteByKey, Cleanup, Increment methods

### Files to Fix (Schema Incompatibility)

**Option A**: Update Type Definitions (Recommended)
- Modify `pkg/repository/repository.go` Spec type to match migration schema
- Add `Fingerprint string` field
- Remove `AliasID, ModuleID` fields (or clarify relationships in migrations)

**Option B**: Update Migrations
- Modify `migrations/00003_modules_aliases_specs.sql` spec table
- Add `alias_id, module_id` columns
- Remove `fingerprint` column
- Add foreign key constraints

### Tests to Add (~500+ lines)

- `pkg/repository/sql/inventory_repository_test.go`
- `pkg/repository/sql/alias_repository_test.go`
- `pkg/repository/sql/module_repository_test.go`
- `pkg/repository/sql/spec_repository_test.go`
- `pkg/repository/sql/fingerprint_repository_test.go`
- `pkg/repository/sql/audit_repository_test.go`
- `pkg/repository/sql/ratelimit_repository_test.go`

## Implementation Tasks

1. **Fix Schema Incompatibility** (2 hours)
   - Decide on Option A or B
   - Update either type definitions or migrations
   - Verify consistency across all entity types

2. **Create SQL Query Layer** (2 hours)
   - Implement `pkg/repository/sql/queries.go`
   - Define all CRUD queries for 7 entity types
   - Add query validation

3. **Implement Repository Layer** (12 hours)
   - Create 8 repository implementation files
   - Follow spec exactly (copy/paste is acceptable)
   - Add error handling and context support

4. **Add Repository Factories** (2 hours)
   - Create NewInventoryRepository, NewAliasRepository, etc.
   - Initialize queries and connection sharing
   - Add dependency injection support

5. **Write Comprehensive Tests** (8 hours)
   - Unit tests for each repository
   - Integration tests with in-memory database
   - Test all CRUD operations
   - Test edge cases and error scenarios

6. **Validate End-to-End** (2 hours)
   - Create test that inserts data via repository
   - Verify data persists correctly
   - Test relationships between entities

**Total Estimated Time**: 28 hours (3-4 days)

## Notes

- Spec provides complete working code (1600+ lines)
- Can copy/paste from spec to accelerate implementation
- Must verify schema compatibility before writing code
- Database migrations already exist - just need code to use them

## Next Steps

1. Fix schema incompatibility (choose Option A or B)
2. Create `pkg/repository/sql/` directory
3. Implement queries.go with all SQL queries
4. Implement 8 repository files following spec
5. Add comprehensive tests
6. Validate end-to-end functionality
