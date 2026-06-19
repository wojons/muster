# WI-017: Unit test coverage for high-priority untested packages

## Goal
Add unit tests for the highest-impact untested packages. These packages have zero test coverage and are core to Muster's functionality.

## Target packages (in priority order)

### Tier 1 — Core data & operations (MUST test)
1. **pkg/repository** (1,119 lines) — Database repository layer. Methods include CRUD operations for various entities. Write tests using in-memory SQLite (use `:memory:` DSN) to avoid needing external DB.
2. **pkg/repository/sql** (974 lines) — SQL-specific implementations. Test the SQL query builders, migrations, and schema operations.
3. **pkg/cmdstore** (538 lines) — Command storage and management. Test store/retrieve/delete operations.

### Tier 2 — API & protocol (SHOULD test if time)
4. **pkg/openapi** (446 lines) — OpenAPI spec parsing. Test spec loading, validation, path extraction.
5. **pkg/request** (328 lines) — HTTP request building. Test request construction, header setting, body encoding.

## Constraints
- Use Go standard library `testing` package
- Use `github.com/stretchr/testify` if already a dependency
- For DB-dependent code: use SQLite `:memory:` DSN
- Each test file goes in the same package (not `_test` external package unless necessary)
- Target: >50% coverage for each tested package
- Do NOT test D-Bus stubs (internal/platform/linux) — those require real D-Bus
- Do NOT test Redis stub (pkg/cache/redis.go) — it's intentionally stubbed
- Do NOT test exploit code (exploits/wasm)

## Verification
After writing tests, verify they pass:
```bash
go test -v -count=1 ./pkg/repository/ ./pkg/repository/sql/ ./pkg/cmdstore/ ./pkg/openapi/ ./pkg/request/
```

## Project context
- Go module: `github.com/wojons/muster`
- Already uses testify in some packages
- Uses `modernc.org/sqlite` for in-memory SQLite
- Custom error package at `pkg/errors`
