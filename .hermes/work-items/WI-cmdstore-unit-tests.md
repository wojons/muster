# WI-cmdstore-unit-tests: Add unit tests for pkg/cmdstore

## Goal
Add comprehensive unit tests for `pkg/cmdstore/store.go` (227 lines) and `pkg/cmdstore/loader.go` (334 lines). These packages currently have 0% unit test coverage. They have integration tests in `pkg/cmdstore/integration_test.go` (397 lines) but no unit tests.

## Context
- The CommandStore type manages command metadata with file-based persistence
- Loader loads commands from the store and creates cobra command objects
- All methods are listed below

## What to test

### store.go — CommandStore
1. `NewCommandStore()` — creates store with empty command list
2. `NewCommandStore(path)` — creates store with file path for persistence
3. `Commands()` — returns stored commands
4. `HasData()` — returns false for empty store, true after save
5. `Load()` — loads single command metadata
6. `Save(data)` — persists data to file
7. `Clear()` — removes persisted file and clears state
8. `UpdateBaseURL(url)` — updates base URL
9. `persistToDisk()` — writes JSON to disk
10. `loadFromDisk()` — reads JSON from disk
11. `LoadCommands(ctx, store)` — convenience wrapper
12. `ReconstructCommand(ctx, meta)` — convenience wrapper

### loader.go — Loader
1. `NewLoader(store, baseURL)` — creates loader
2. `LoadCommands()` — loads all commands from store
3. `GetBaseURL()` — returns base URL
4. `SetHeaders(headers)` — sets headers
5. `reconstructCommand(meta)` — builds cobra.Command from metadata
6. `createRunHandler(meta, baseURL)` — creates run handler
7. `getFlagValue(cmd, flagMeta)` — extracts flag values
8. `executeAndReadResponse(builder, cmd, ctx)` — executes HTTP request

## Requirements
- Use `testing` + `stretchr/testify` (require/assert)
- Use temp directories for file-based tests (`t.TempDir()`)
- Test edge cases: empty store, missing file, corrupted JSON, empty base URL
- `reconstructCommand` and `createRunHandler` return `*cobra.Command` — these can be tested for shape (command.Use, command.Short, flags registered, run function not nil)
- Do NOT require a live HTTP server for unit tests — mock where needed
- Target: >80% coverage for the package

## CRITICAL
- Container has NO Python. DO NOT install anything. Just WRITE Go test files.
- Write `pkg/cmdstore/store_test.go` and `pkg/cmdstore/loader_test.go`
- After writing tests, run `go test -v -count=1 ./pkg/cmdstore/` and fix any failures
- Run `go test -cover ./pkg/cmdstore/` and report the coverage percentage
- Commit with message: `test(cmdstore): add unit tests for store and loader` 
