# Story 1.1: HTTPS Fetching with TLS

## Epic
Epic 1: Inventory System

## Story
As a user, I want to fetch inventory from an HTTPS URL with proper TLS verification so that I can securely configure the tool from a remote source.

## Acceptance Criteria
- [ ] Accept `--inventory-url <url>` argument (HTTPS only)
- [ ] Fetch inventory from remote HTTPS URL
- [ ] Verify TLS certificate (can be disabled for dev with warnings)
- [ ] Support custom CA bundles
- [ ] Support mTLS (client certificates) for inventory fetching
- [ ] Show progress during download
- [ ] Handle network errors with retry (3 attempts)
- [ ] Support HTTP caching headers (ETag, If-None-Match)
- [ ] Validate content type (application/json)
- [ ] Clear error messages for TLS/SSL errors

## Technical Notes
- Use axios or node-fetch with proper TLS config
- Implement exponential backoff (1s, 2s, 4s delays)
- Store TLS settings in configuration
- Support environment variables for cert paths:
  - `OPENAPICLI_CA_FILE`
  - `OPENAPICLI_CERT_FILE`
  - `OPENAPICLI_KEY_FILE`

## Estimation
- **Story Points**: 3
- **Duration**: 2-3 days

## Dependencies
- None

## Test Cases
1. Successfully fetch inventory from HTTPS
2. Verify TLS certificate by default
3. Use custom CA bundle
4. Use mTLS (client cert + key)
5. Handle self-signed cert (with flag)
6. Handle TLS handshake failure
7. Handle 404 error
8. Handle network timeout
9. Use cached response (304 Not Modified)
10. Retry on transient errors
