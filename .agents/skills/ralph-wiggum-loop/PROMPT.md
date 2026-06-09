# Ralph Wiggum Loop - Build Phase

You are in a Ralph Wiggum Loop build phase. Your task is to implement the next pending item from the TODO.md file.

## Your Mission

1. Read `TODO.md` (ROOT directory) to find the next uncompleted task (marked with `- [ ]` or `- [ ] Step N:`)
2. Implement the task completely
3. Mark the task as complete by changing `- [ ]` to `- [x]` in TODO.md
4. Commit your changes with a descriptive message

**IMPORTANT**: Use the ROOT `TODO.md` file, NOT `.agents/skills/ralph-wiggum-loop/TODO.md` (which is old/archived).

## Implementation Guidelines

- Follow the code style in AGENTS.md
- Write unit tests for new code
- Run `go test ./...` and `make lint` before marking complete
- Update documentation if needed

## Task Priority Order

Tasks are organized by priority in the ROOT TODO.md:

**CRITICAL (Do First - This Week)**:
1. SEC-ADV-011: Fix SHA-256 calculation bug (pkg/inventory/manager.go:255-257)
2. Audit log race condition (internal/audit/logger.go:80-81)
3. Header injection no CRLF validation (pkg/request/builder.go:58-60)
4. Path traversal in inventory cache (pkg/inventory/cache.go:92-101)
5. Command injection in keychain (pkg/auth/keychain.go:40-48)
6. Symlink TOCTOU race (pkg/dsl/builtins.go:585-592)

**HIGH (Week 2-3)**:
7. Add monitoring/metrics tests (internal/monitoring/metrics.go)
8. Add command store loader tests (pkg/cmdstore/loader.go)
9. Add MCP handler tests (pkg/mcp/handlers/)

**MEDIUM (Month 1-2)**:
10. Circuit breaker implementation
11. SQLite encryption at rest
12. Master key derivation architectural fix

Always work on the first CRITICAL task, then HIGH, then MEDIUM.

## Completion Criteria

A task is complete when:
- Code compiles without errors
- Unit tests pass
- Integration with existing code is verified
- TODO.md is updated with `- [x]` marker
- Changes are committed

## Current Focus

Read TODO.md and implement the first `- [ ]` task you find. Work systematically and thoroughly.
