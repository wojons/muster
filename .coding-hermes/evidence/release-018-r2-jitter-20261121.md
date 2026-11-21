# RELEASE-018-R2 — jitter distribution + close evidence (2026-11-21)

Row: RELEASE-018 closed cross-platform GREEN but the same macOS/ARM64 red returned within 3h:
TestAuditCommand_LogLimitStaysBounded duration bound overruns on Apple Silicon (2 runs red,
30/30 pass on Linux).

Disposition: REPAIR (not quarantine) — the wall-clock dependency was already removed by
INT-CI-016 (commit 90ab1379, "test(audit): bound INT-CI-016 on bytes read, not wall-clock";
PM verification 2026-11-20 confirms "the wall-clock dependency is gone"). This close binds the
owed execution arm: the Apple Silicon jitter distribution, plus the post-fix green-run citations.

## AC(2) — Apple Silicon jitter distribution (-count=50, -race)

Method: the exact pre-INT-CI-016 wall-clock measurement — `audit log --limit 1` on a 10,000-row
sink, best-of-5 per sample — reproduced 50 times under -race on an Apple Silicon GitHub runner
(label `macos-latest`; explicit `macos-14` is unsupported on this account — annotation below),
by the dispatch/push-triggered workflow `.github/workflows/jitter-audit.yml` (probe file
generated at run time in the CI workspace only, never committed).

Run: **44896874772** (branch jitter/jitter-probe, head a41c3d38), all steps green:
- full package sanity: `go test -race -count=1 ./internal/builtin/` → `ok ... 12.843s`
- audit-test 50x: `go test -race -count=50 ./internal/builtin/ -run TestAuditCommand_LogLimitStaysBounded`
  → `ok ... 24.222s` (50/50 pass, no race reports)
- probe: 50 samples, raw artifact `jitter-samples-arm64` (artifact ID 14703973889)

Distribution (n=50, per-sample best-of-5, microseconds):
| min | p50 | p99 | max | max/min |
|---|---|---|---|---|
| 203.666µs | 215.083µs | 283.459µs | 283.459µs | 1.39x |

(Conversion note, disclosed: the workflow's sample normalization deleted the µ unit byte
instead of replacing it, so the raw artifact reads "283.459s"; the values are µs — the same
operation measured 103.084-138.958µs in the original red runs without -race. The distribution
above was recomputed locally from the uploaded artifact with correct units:
`sort -g jitter_samples.txt` → min/p50/p99/max as tabled.)

Read on the original defect: the hand-picked 103.084µs ceiling sits BELOW the observed
MINIMUM (203.666µs) of the healthy distribution on this platform — with -race the bound is
violated even by a perfectly correct implementation, and the 1.39x max/min spread (39%
run-to-run jitter at p50→max) makes any µs-scale wall-clock bound unfittable. The bytes-read
bound (INT-CI-016 shape) is the correct replacement, and the 50/50 -race green above shows
the current deterministic test is stable on Apple Silicon.

## AC(3) — 3 consecutive green Cross-Platform runs on main AFTER the fix (each ARM64 job verified)

| run id | head sha | date | ARM64 job id | ARM64 conclusion |
|---|---|---|---|---|
| 44895386543 | 4efe8ec0 | 2026-11-21 | 136819814047 | success |
| 44868051426 | 113bd123 | 2026-11-21 | 136736078870 | success |
| 44862401167 | 0de8b0d7 | 2026-11-21 | 136718477351 | success |

The fix (90ab1379) landed 2026-11-16; every run above post-dates it. (Earlier ARM64 green at
cb594307 / run 44705606024 was already on the row via PM verification 2026-11-20.)

## AC(1) + AC(4) — repair, not quarantine

No t.Skip anywhere: `grep -rn "t.Skip" internal/builtin/audit_retention_test.go` is empty; the
test asserts bytes-read via countingAuditSink (internal/builtin/audit_retention_test.go:231-236),
comment names INT-CI-016 and now RELEASE-018-R2's distribution is bound by this file. No follow-up
row needed — the repair is complete on both platforms (Linux + Apple Silicon 50/50 green).

## Annotations (route found, for the record)

- `gh workflow run` (workflow_dispatch) runs are cancelled pre-runner in this repo (2 attempts,
  ~3s, no steps, no runner assigned) — BOTH as dispatch on main and as push. Root cause found
  via the push-run annotation: `The configuration 'macos-14-arm64-us-default' is not supported`.
  Fix: `runs-on: macos-latest` (the label the green ARM64 Compatibility job itself uses).
- The full internal/builtin package at `-count=50 -race` exceeds go test's 10m timeout
  (run 44895704704: FAIL 600s, quic-go/daemon-network goroutines x50); the 50x repetition is
  scoped to `TestAuditCommand_LogLimitStaysBounded` with one full-package `-count=1 -race`
  sanity pass — documented in the workflow comment.
- The units bug above is fixed in .github/workflows/jitter-audit.yml on main (perl replaces
  tr-deleted µ with "u") so future runs print correct units directly.

## AC(5) — boardctl validate

Run at close time; counts recorded in the board close event (baseline at filing: 0 errors,
264 warnings, RESULT OK — per the row text).
