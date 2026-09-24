# Tagged-Release Upgrade Verification

**Addresses QA-MUSTER-4** — the 2026-09-10 QA battery cycle recorded
`upgrade N/A "no release tags in repo"` for this repository even though origin
carries `v0.1.0` and `v0.1.1`. That N/A was a **harness artifact**, not a
verified upgrade: the battery ships the repo tree to its agent as a tar-sync
that strips `.git` (and re-inits a synthetic single-commit repo), so
`git tag` on the agent is always empty and the upgrade cell has nothing to
check out. The upgrade path was recorded as *not applicable* when the honest
verdict was *untested*.

This document and `scripts/verify-upgrade-path.sh` close that gap **locally**,
where `.git` exists. Preserve the original QA observations as recorded: the
`.vfs/graph.db` truncation (INFO — local untracked tool cache, gitignored) and
the disconnect fail-fast (OK — rc=1 with a dead proxy, no hang) are separate
findings and are not re-litigated here.

## What the script proves (and what it does not)

`scripts/verify-upgrade-path.sh` proves a **tagged release → current HEAD
upgrade** end to end, on a machine that has the real repository history:

1. **Materialize the previous release** in a throwaway `git clone` of the
   checkout (`--local --no-hardlinks --shared`, no network) and detach it to
   the previous tag (second-newest tag by version by default). A clone, not an
   in-place checkout, so the upgrade test starts from the release's own files
   and module cache state — never from HEAD's tree.
2. **Build + install that release**, with the version/commit stamped via
   `-ldflags -X` into `internal/version` — using the module path declared in
   *that tag's own* `go.mod`.
3. **Reinstall the current HEAD build over the previous install** — the
   upgrade step itself.
4. **Verify the upgrade took**: the previously installed binary reports the
   previous version, the reinstalled binary reports the HEAD version, and the
   upgraded binary still executes a basic command (`config show`).

What it does **not** claim: it does not perform the QA battery's remote run,
and it does not depend on `.git` surviving inside a tar-synced artifact. A
remote-battery rerun is still required before the harness's `upgrade` cell can
go from N/A to a live verdict — do not cite this script as that rerun.

Exit codes: `0` verified · `1` failed (the failure text is the evidence) ·
`2` skipped because there is genuinely nothing to upgrade (fewer than two
tags, or HEAD *is* the previous release). Skips are reported as N/A-like
honesty, never as a pass.

```bash
scripts/verify-upgrade-path.sh                          # previous tag → HEAD
scripts/verify-upgrade-path.sh v0.1.1                   # explicit previous tag
scripts/verify-upgrade-path.sh v0.1.0 v0.1.1            # explicit both ends
MUSTER_UPGRADE_KEEP=1 scripts/verify-upgrade-path.sh    # keep /tmp scratch for inspection
MUSTER_UPGRADE_WORK=/srv/scratch/upgrade scripts/verify-upgrade-path.sh
```

The scratch area defaults to a `mktemp -d` under `/tmp` and is removed on
exit unless `MUSTER_UPGRADE_KEEP=1` or a custom `MUSTER_UPGRADE_WORK` is set.

## Why the version stamp reads each tag's own go.mod

`go build -ldflags "-X <pkg>.Version=..."` only takes effect when `<pkg>` is
the import path the **built tree** declares. Historical tags can drift from
HEAD's module layout: at `v0.1.0` the Makefile still stamped the pre-rename
path `github.com/openapi-cli/openapi-cli/internal/version`, while its
`go.mod` already declared `github.com/wojons/muster`. The script therefore
reads `module` from the checked-out tag's `go.mod` and targets
`<that-module>/internal/version`, then asserts the produced binary actually
reports the stamped string before the upgrade leg runs. A silent miss would
print `dev (unknown)` and make the upgrade look verified when no version
metadata ever landed — exactly the class of false pass this task exists to
kill.

## Wiring it into the QA battery (out-of-repo harness, instructions only)

The battery script lives outside this repository
(`~/.hermes/scripts/bunker-qa.sh` in the harness deployment). Its upgrade
cell already knows how to install a previous release and reinstall HEAD —
what it lacks on the agent is history. Two honest options, both preserving
the current corruption/disconnect cell semantics:

1. **Host-side pre-flight (recommended, no agent changes)** — before the
   sync, run the equivalent of this script on the host where `.git` exists
   and bake the result into the cell as a pre-verified input, the same way
   the battery already bakes host-detected tag coordinates into the generated
   script. The agent-side cell then reports
   `upgrade OK (host pre-flight: prev-tag → HEAD reinstall verified)` instead
   of N/A, and still FAILs loudly if its own leg breaks.
2. **Ship the previous tag's tree** to the agent (`git archive` over the
   existing sync channel — history is not needed to *build*, only to *find*
   the tag), install it, reinstall HEAD, and have the cell grade the version
   transition. This is the battery's existing `ship_prev_tag_tree` pattern.

In both cases the cell must keep printing N/A **only** when the repo truly
has fewer than two release tags; a stripped `.git` with tags known to exist
on origin is a FAIL/UNTESTED verdict with the reason, never a silent N/A.

## Related QA-MUSTER-4 observations (unchanged, preserved)

- **chaos-corruption**: truncation of `./.vfs/graph/graph.db` — local
  UNTRACKED tool cache (gitignored; `git ls-files .vfs` is empty), not
  shipped repo content. INFO.
- **chaos-disconnect**: fails fast with rc=1 on a dead proxy, no hang. OK.
- **fresh-install** (adjacent QA-MUSTER-1): the CGO seccomp dependency
  (`pkg-config`/`libseccomp-dev`) is documented in
  `docs/guides/installation.md` prerequisites — separate row.