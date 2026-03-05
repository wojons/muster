# Epic 1: Inventory System

## Overview
Build the inventory management system that downloads, caches, and manages API configurations from remote HTTPS endpoints. This is the foundation of the entire tool.

## Business Value
- Enables zero-config setup - just point to inventory URL
- Automatic updates without user intervention
- Works offline with cached versions
- Supports multiple environments via aliases
- Allows "last known good" version fallback

## Acceptance Criteria
1. Fetch inventory from configurable HTTPS URL
2. Parse and validate inventory JSON structure
3. Cache inventory locally with HTTP caching headers
4. Support multiple version storage
5. Implement "last known good" marker
6. Support aliases (prod, staging, etc.)
7. Background refresh capability
8. Offline mode using cached data
9. CLI version compatibility checking
10. Clear error messages for invalid/failed inventory

## Technical Requirements
- HTTPS mandatory (TLS 1.2+)
- Support ETag, Cache-Control, Last-Modified headers
- Content-addressable storage for versions
- JSON Schema validation for inventory format
- Background update checking
- Configurable cache limits (size, TTL)

## Inventory Schema
```json
{
  "version": "1.2.3",
  "min_cli_version": "0.5.0",
  "max_cli_version": "1.x",
  "openapi": {
    "source": "embedded" | "url",
    "content": "..." | "https://...",
    "sha256": "..."
  },
  "modules": {
    "processors": [
      {"name": "transform", "url": "...", "sha256": "...", "signature": "..."}
    ]
  },
  "config": {
    "rate_limiting": {"requests_per_minute": 100},
    "retry_policy": {"max_attempts": 3, "backoff": "exponential"}
  },
  "auth": {
    "inventory_auth": {"type": "bearer", "token_env": "INV_TOKEN"},
    "api_auth": {"type": "oauth2", "flow": "client_credentials"}
  }
}
```

## Dependencies
- None (foundational epic)

## Stories
- Story 1.1: HTTPS fetching with TLS verification
- Story 1.2: HTTP caching implementation
- Story 1.3: Local cache storage
- Story 1.4: Version management
- Story 1.5: Alias system
- Story 1.6: "Known good" marker
- Story 1.7: Offline mode
- Story 1.8: CLI version checking
- Story 1.9: Inventory validation
- Story 1.10: Background refresh

## Estimation
- **Story Points**: 21
- **Duration**: 3-4 weeks

## Risks
- Inventory server downtime (mitigated by caching)
- Large inventory files (mitigated by compression/limits)
- Version conflicts (handled by semver checking)

## Notes
- Default inventory path: `/_inventory.json` (configurable)
- Cache location: `~/.cache/openapi-cli/inventory/`
- Consider using If-None-Match for efficient updates
