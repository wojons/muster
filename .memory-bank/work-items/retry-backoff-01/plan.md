# Plan: Retry/Backoff Mechanism

**Work Item**: retry-backoff-01  
**Status**: Not started  
**Goal**: Verify spec conformance of existing retry implementation, close any gaps, document coverage.

## Mission

The `pkg/retry/` package and `pkg/client/` integration are already implemented. This work item: (1) cross-references `specs/RELIABILITY-RETRY.md` against the implementation to confirm full coverage, (2) runs the full test suite, (3) documents any gaps found, (4) enables default retry on the HTTP client if justified.

## AC → Verification Mapping

| Acceptance Criteria | Verification |
|---|---|
| AC1: RetryConfig with all required fields exists | Code review — `pkg/retry/config.go` |
| AC2: 4 backoff strategies implemented | Code review — `pkg/retry/backoff.go` |
| AC3: 4 jitter types implemented | Code review — `pkg/retry/jitter.go` |
| AC4: Retrier with context-aware Execute | Code review — `pkg/retry/retrier.go` |
| AC5: Error classification working | Code review — `pkg/retry/errors.go` |
| AC6: Client integration working | Code review — `pkg/client/client.go:87-89,133-134,198,329-340` |
| AC7: All tests pass | `go test -v -count=1 ./pkg/retry/... ./pkg/client/...` |

## Phases

### Phase 1: Spec conformance audit
**Status**: Not started

- **Step 1.1**: Read `specs/RELIABILITY-RETRY.md` and catalog all spec requirements  
  *Verification*: Spec inventory created  
  *Evidence path*: verification.md

- **Step 1.2**: Map each spec requirement to implementation location  
  *Verification*: Table with spec→impl mapping  
  *Evidence path*: verification.md

- **Step 1.3**: Identify any gaps between spec and implementation  
  *Verification*: If gaps found, list as subtasks  
  *Evidence path*: verification.md

### Phase 2: Test suite verification
**Status**: Not started

- **Step 2.1**: Run retry package tests  
  *Verification*: `go test -v -count=1 ./pkg/retry/...`  
  *Evidence path*: test output

- **Step 2.2**: Run client retry tests  
  *Verification*: `go test -v -count=1 -run Retry ./pkg/client/...`  
  *Evidence path*: test output

- **Step 2.3**: Run race detector on retry-sensitive tests  
  *Verification*: `go test -race -count=1 -run Retry ./pkg/client/...`  
  *Evidence path*: test output

### Phase 3: Close gaps (if any found) + document
**Status**: Not started

- **Step 3.1**: Fix any spec gaps found in Phase 1  
  *Verification*: Re-run audit — zero gaps  
  *Rollback*: Revert individual file changes

- **Step 3.2**: Add trace markers and document spec coverage in verification.md  
  *Verification*: `grep 'axiom:trace' pkg/retry/*.go`  
  *Evidence path*: grep output
