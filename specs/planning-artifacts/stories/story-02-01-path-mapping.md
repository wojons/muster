# Story 2.1: Path-to-Command Mapping Algorithm

## Epic
Epic 2: Dynamic CLI Generation

## Story
As a user, I want OpenAPI paths converted to CLI commands so that I can interact with the API naturally via command line.

## Acceptance Criteria
- [ ] Convert `/users/{id}` to `users get --id`
- [ ] Convert `/users` to `users list`
- [ ] Support nested resources (e.g., `/users/{id}/posts` → `users posts list --user-id`)
- [ ] Handle HTTP methods as subcommands or flags
- [ ] Preserve path parameter names
- [ ] Support API versioning in paths
- [ ] Allow custom command aliases via spec extensions

## Technical Notes
- Implement path parsing algorithm
- Handle edge cases (trailing slashes, empty paths)
- Support x-cli-name extension for custom names
- Consider operation tags for grouping

## Estimation
- **Story Points**: 5
- **Duration**: 3-4 days

## Dependencies
- Story 1.6: External Reference Resolution

## Test Cases
1. Simple path converts correctly
2. Path with parameter converts correctly
3. Nested resource path converts correctly
4. Multiple parameters handled
5. Different HTTP methods generate different commands
6. Custom alias via extension works
7. Edge cases (root path, trailing slash)
