# Story 1.4: Version Management

## Epic
Epic 1: Inventory System

## Story
As a user, I want to manage multiple versions of the inventory so that I can rollback to working versions or pin to specific releases.

## Acceptance Criteria
- [ ] Store multiple inventory versions locally
- [ ] Support `--version <version>` flag to use specific version
- [ ] Support `--use-latest` flag (default behavior)
- [ ] Support `--use-known-good` flag
- [ ] `mark-good` command to mark current as known good
- [ ] `versions` command to list cached versions
- [ ] Automatic fallback to cached if server down
- [ ] Version pinning via config
- [ ] Show warning if using old version
- [ ] CLI version compatibility checking

## Technical Notes
- Cache structure: `~/.cache/openapi-cli/inventory/<url-hash>/<version>.json`
- Symlinks: `current` → latest, `known-good` → user-marked
- Store version metadata (date fetched, size, etc.)
- Implement semver comparison for compatibility checks

## Estimation
- **Story Points**: 5
- **Duration**: 3-4 days

## Dependencies
- Story 1.1: HTTPS Fetching
- Story 1.3: Local Cache Storage

## Test Cases
1. Fetch and store new version
2. Use specific version with flag
3. Mark version as known good
4. Use known good version when server down
5. List all cached versions
6. Fallback to cached on network error
7. Show compatibility warning
8. Pin version in config
9. Automatic cleanup of old versions (keep last N)
10. Show version info in verbose mode
