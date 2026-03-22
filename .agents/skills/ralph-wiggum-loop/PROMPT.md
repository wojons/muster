# Ralph Wiggum Loop - Build Phase

You are in a Ralph Wiggum Loop build phase. Your task is to implement the next pending item from the TODO.md file.

## Your Mission

1. Read `.agents/skills/ralph-wiggum-loop/TODO.md` to find the next uncompleted task (marked with `- [ ]`)
2. Implement the task completely
3. Mark the task as complete by changing `- [ ]` to `- [x]` in TODO.md
4. Commit your changes with a descriptive message

## Implementation Guidelines

- Follow the code style in AGENTS.md
- Write unit tests for new code
- Run `go test ./...` and `make lint` before marking complete
- Update documentation if needed

## Task Priority Order

Tasks are organized by priority:
1. P0 tasks (AUDIT-001) - BLOCKING PRODUCTION
2. P1 tasks (AUDIT-002, AUDIT-003) - HIGH PRIORITY
3. P2 tasks (AUDIT-004, AUDIT-005) - MEDIUM PRIORITY
4. IMPL tasks (IMPL-001 through IMPL-010) - Implementation

Always work on the first incomplete task in priority order.

## Completion Criteria

A task is complete when:
- Code compiles without errors
- Unit tests pass
- Integration with existing code is verified
- TODO.md is updated with `- [x]` marker
- Changes are committed

## Current Focus

Read TODO.md and implement the first `- [ ]` task you find. Work systematically and thoroughly.
