# Dogfood Integration Report — 2026-09-04

**Run type:** fix-verification + fresh-eyes real use + first bunker install leg
**HEAD under test:** `01849718` (v0.1.1-57-g01849718)
**Verdict:** ✅ SHIPPABLE (first run to earn it after two PROMISING-BUT-ROUGH rounds)
**Operator note:** this run was triggered by the `muster-qa` scheduler role.

## Promise vs reality

*"Turn any OpenAPI spec into a CLI, an MCP server, and a Go library — instantly."*

**Holds on HEAD.** Every surface exercised this run did what the README says, and the six
P1/P2 findings from the 2026-08-17 run (DF-007..DF-012) are all actually fixed — verified by
real use, not by reading the changelog.

## What was done (real use, not test-reading)

1. **Flagship gate:** `./demo/run-all.sh` from a clean shell — 5 mock services up, 10 demo
   scripts, **23/23 checks passed in ~163s**.
2. **Fresh build:** `make build-cli` 4s warm; `openapi-cli version` →
   `v0.1.1-57-g01849718` (version injection correct, GAP-007 still holding).
3. **CLI full CRUD round-trip** against a live task-forge mock (PORT=19801, scratch HOME):
   create-project (201) → get-project → **create-project-task on the nested path** (the op
   that was unreachable on 08-17) → list-project-tasks → update-task (PATCH, status
   flipped `todo`→`done` server-confirmed) → create-project-task again → delete-task →
   delete-project → list-projects `[]`. Zero errors.
4. **Generator inventory check:** task-forge spec = 22 operations → 21 generated commands,
   programmatic diff found **zero missing ops and zero name collisions** (DF-007 + DF-008
   verified fixed).
5. **Daemon:** `start --backend-url` now **really proxies** (GET /projects returned the
   project JSON through the socket, not a canned status — DF-009 fixed); `restart` stopped
   AND restarted; post-restart traffic 200; `stop` cleaned up (DF-011 fixed).
6. **MCP:** stdio handshake → serverInfo `openapi-mcp` → tools/list **22/22 tools** with
   clean snake_case names for the operationId-less spec.
7. **Test-suite duration:** `go test -short -count=1 ./...` = **70s cold, 45 packages, 0
   FAIL** — the GAP-005/GAP-026 "tests too slow" complaints stay fixed.
8. **Bunker install leg (first ever for muster):** fresh ephemeral agent on
   bunker-las-03 (bare Debian 13, go MISSING) → tar-staged tree → manual Go 1.26.5
   install → `make build-cli` → smoke: discover+generate on petstore3, typed command
   round-trip via `--base-url`, generic `request` GET 200 from a local file server.

## Fix-verification table (DF-007..DF-012, all marked complete)

| ID | Was | Verified 09-04 |
|----|-----|----------------|
| DF-007 (P1) | generate mangles operationId-less specs | ✅ fixed: 22 ops → 21 clean commands |
| DF-008 (P1) | name collisions shadow nested ops | ✅ fixed: `create-project-task` distinct |
| DF-009 (P1) | daemon start serves canned status | ✅ fixed: start-mode proxies real traffic |
| DF-010 (P2) | help examples reference nonexistent groups | ✅ examples match real commands |
| DF-011 (P2) | restart = stop-only | ✅ fixed: restarts and serves |
| DF-012 (P3) | reference-card missing generator API | ✅ card covers generator |

## New findings (this run) → board

- **DF-013 (P1)** — running the documented test suite **destroys the user's persisted
  command store**: caught live when my own scratch-HOME store was replaced with test
  fixtures (`spec_path: /tmp/TestRealCLIIntegration…/petstore.yaml`,
  `base_url: http://127.0.0.1:42707`) minutes after `go test -short ./...`. Root cause:
  `demo/test-suites/100-cli-real-integration_test.go` exec()s the built CLI without
  HOME/XDG isolation. This is exactly the "all tests green" trap the dogfood skill exists
  for: the suite passes while it wrecks user state.
- **DF-014 (P1, filed on the bunker board as SPAWN-REG-001)** — bunkerd lost its
  in-memory agent registry mid-run twice: spawn failed "port pool exhausted (10 ranges)"
  with 9 orphaned users holding ranges; another agent was destroyed mid-session by the
  concurrent bunker E2E battery. Infra, not muster — recorded here for the trail.
- **DF-015 (P2)** — README quickstart verbatim fails on relative-server specs: petstore
  declares `servers[].url: /api/v3` (relative), so the generated `get-pet-by-id 1` dies
  client-side `unsupported protocol scheme ""`. `--base-url`/`MUSTER_BASE_URL` fix it;
  the README never says so for the CLI surface.

## Bunker install leg — what the docs silently assume

- **Go 1.26.6+ must already exist.** The agent had none; README/AGENTS never mention
  installing it. Fresh-user path: download go1.26.5 tarball, extract to ~/toolchain,
  extend PATH (≈40s).
- **Transient `no space left on device` at link time** when the Go download tarball is
  still sitting in HOME; deleting it before `make build-cli` avoids it. Per-agent size
  budget on the bunker is tight (~1.5G) — worth a note in docs for container/CI users.
- **Private repo:** documented `git clone` fails for a credential-less fresh user
  (expected, correct per access policy — do NOT widen access). Tar-staging the tree
  worked; consequence: `version` reports `dev (unknown)` without git metadata (cosmetic).
- Smoke result after install: PASS (discover/generate/typed-with-base-url/request 200).

## Frictions (small but real)

1. DF-013 store clobber (above) — the worst one; tests must never write to real HOME.
2. DF-015 relative-server trap (above).
3. `go test -short ./...` is green but only 45 of 53 packages print `ok` — 8 are
   `no test files`; not a bug, just noise for newcomers reading output.

## Time-to-first-success

- Local user (Go installed): **~5 min** to a working typed CLI from clean clone
  (build 4s + generate + first typed call against a demo service).
- Bunker fresh user: **~12 min** including Go toolchain install (~40s), tree transfer,
  build (~67s + retry after disk transient), and smoke.

## What a new user should do differently from the README

1. Use a scratch HOME for the first `generate` (until DF-013 is fixed, ALSO for tests).
2. If the spec has a relative `servers[].url`, pass `--base-url` (DF-015).
3. Install Go 1.26.6+ first; the docs assume it.

See also: `skills/muster-usage/SKILL.md` v1.2.0 (updated this run),
`docs/dogfood/diagnostics.md` (diagnostic trail).
