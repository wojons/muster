# DOC-15 — audit retention enforcement + bounded newest reads: evidence

Date: 2026-10-24
Worktree: `wt/DOC-15`
Spec: `specs/EVENTS-AUDIT.md`

## What the finding claimed, and what was true

| Claim | Status before | Fix |
|---|---|---|
| `audit info` prints retention, `specs/EVENTS-AUDIT.md` documents another | INFO printed `90 days`, spec's `DefaultAuditConfig` said `365` | both are 365 now, and that exact value is what the enforcer uses |
| no retention enforcement exists | true: nothing removed an expired record | `PruneRetention` + `audit prune` verb + enforcement at writer open |
| `audit log` reads/unmarshals the whole file before limiting | true: `os.ReadFile` + unmarshal every line, then reverse + cut | tail-block reader returns only the newest `--limit` rows |

## AC (a) — a documented mechanism enforces retention, info == enforced value

Retention rotates expired rows into `<log>.archive-<unixnano>.jsonl` and rewrites
the active log with a re-signed, valid hash chain. Both segments verify
independently; the rotation boundary is an explicit deletion, so no cross-file
continuity is claimed (documented in `specs/EVENTS-AUDIT.md` and
`docs/guides/commands.md`).

Live run (built `./bin/openapi-cli`, scratch sink, hook disabled so state is
observable):

```
--- before:
2026-10-24T00:00:00Z		test	fresh-row	
2025-01-01T00:00:00Z		test	old-row	
--- dry run:
Would prune 1 of 2 records older than 365 days from .../audit.jsonl
--- verb:
Pruned 1 records older than 365 days from .../audit.jsonl
--- after:
2026-10-24T00:00:00Z		test	fresh-row	
--- archives:
.../audit.jsonl.archive-1792837305974790183.jsonl
--- verify active:
1 records, chain intact
--- verify archive:
1 records, chain intact
```

Enforcement also runs when a writer opens the sink (so an idle trail cannot grow
past the window). Same scratch host, hook enabled, one audited command:

```
--- after one audited command:
2026-10-24T10:21:46Z	info	command.execute	command.openapi-cli audit status	kara
2026-10-24T00:00:00Z		test	fresh-row	
--- archives:
.../audit2.jsonl.archive-1792837306054407357.jsonl
--- verify active:
3 records, chain intact
```

`audit info` and the spec agree with the enforced value:

```
Retention:        365 days
```
(`internal/audit.DefaultConfig().RetentionDays`,
`internal/audit.DefaultFileStorageOptions().RetentionDays`, the `audit prune`
default window, and `specs/EVENTS-AUDIT.md`'s `RetentionDays: 365` are the same
number; `TestAuditCommand_InfoRetentionMatchesEnforcement` pins all four.)

## AC (b) — enforcement code exists

```
$ grep -rniE 'func .*(rotate|prune|trim|retention)|os.Truncate' --include=*.go internal/audit cmd internal/builtin | grep -v _test
internal/audit/retention.go:21:func PruneRetention(path string, retentionDays int, algorithm string) (int, error) {
internal/audit/audit_types.go:232:func (fs *FileStorage) reopenIfRotated() error {
```

## AC (c) — `audit log --limit 1` stays bounded

`readNewestAuditEvents` reads 64 KiB blocks backwards from EOF, decodes only the
newest `--limit` lines, and returns them newest-first. `audit log` (unfiltered)
and `audit show` use it; the `--filter` path still scans the file because a
filter can match anywhere.

- `TestAuditCommand_LogLimitStaysBounded`: 10,000-row sink vs 200-row sink,
  `--limit 1`, min of 5 runs each, asserts `large <= 2 * small`.
- `TestReadNewestAuditEvents_AllocationsBounded`: on a multi-megabyte sink the
  bounded read allocates < half the file size while the previous whole-file path
  allocates >= the file size (the non-flaky structural half of the proof).
- `TestAuditCommand_LogLimitNewestOnLargeSink`: 4,000-row sink, `--limit 1`
  returns `actor-3999`, `--limit 150` returns rows 3999..3850 in order — the
  bounded read cannot regress to oldest-first.

## Hash-chain policy (preserved, not silently broken)

- Prune re-signs the archive segment and the retained segment, so
  `audit verify` passes on both.
- `FileStorage.Write` re-opens the sink when the path no longer refers to the
  open descriptor (retention rotates by rename), then re-seeds from the new tail.
  Without this, a long-lived writer appended to the unlinked inode and the record
  vanished — `TestFileStorage_SurvivesRetentionRotation` reproduces exactly that
  (and its RED run failed with "post-rotation write was lost (appended to the
  unlinked inode)").

## Tests added

`internal/audit/retention_test.go`
- `TestPruneRetentionRemovesOldRowsAndPreservesChains`
- `TestPruneRetentionDoesNotRemoveRecentRows`
- `TestPruneRetentionRejectsMalformedRows`
- `TestPruneRetentionArchiveRowsAreNonVacuous`
- `TestFileStorage_SurvivesRetentionRotation`

`internal/builtin/audit_retention_test.go`
- `TestAuditCommand_InfoRetentionMatchesEnforcement`
- `TestAuditCommand_PruneRotatesExpiredRows` (removal + archive + idempotency)
- `TestAuditCommand_PruneDryRunLeavesSinkUntouched` (byte-identical sink)
- `TestAuditCommand_LogLimitNewestOnLargeSink`
- `TestAuditCommand_LogLimitStaysBounded`
- `TestReadNewestAuditEvents_AllocationsBounded`

`internal/audit/audit_test.go` — `TestDefaultConfig` now expects 365.

## RED proofs

| Reverted | Failing test | Observed |
|---|---|---|
| bounded reader → whole-file read | `TestAuditCommand_LogLimitNewestOnLargeSink` | returned oldest rows |
| reopen-on-rotation call | `TestFileStorage_SurvivesRetentionRotation` | "post-rotation write was lost (appended to the unlinked inode)" |

## Gates

```
gofmt -l internal/ cmd/                       -> clean
go build ./...                                -> ok
go vet ./...                                  -> ok
go test -short ./... -count=1 -parallel 4     -> green
  (pkg/daemon TestProxyHandler_CallerCancellationPropagates flaked once under
   load, passed 3/3 in isolation and green on the full rerun; unrelated to audit)
go test ./internal/audit ./internal/builtin   -> ok (7.9s / 0.2s targeted)
make build-cli                                -> ok
./bin/openapi-cli audit info | grep -E '^Retention:[[:space:]]+365 days$' -> matches
gitreins guard                                -> PASS (test mode: full, go_tests passed)
```

## Files changed

- `internal/audit/retention.go` (new), `internal/audit/retention_test.go` (new)
- `internal/audit/audit_types.go` (RetentionDays default, `reopenIfRotated`)
- `internal/audit/hooks.go` (enforce on writer open, 365 default)
- `internal/builtin/extended.go` (`readNewestAuditEvents`, `audit prune`, bounded show/log)
- `internal/builtin/audit_retention_test.go` (new)
- `internal/audit/audit_test.go`, `specs/EVENTS-AUDIT.md`, `docs/guides/commands.md`
