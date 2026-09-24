#!/bin/bash
#
# Tagged-release upgrade verification (local, .git present)
#
# Addresses QA-MUSTER-4. The QA battery ships the repo tree to its agent as a
# tar-sync that strips .git, so the agent-side upgrade cell always sees an
# empty tag list and prints `upgrade N/A "no release tags in repo"` — even
# though origin carries v0.1.0 and v0.1.1. That N/A is a harness artifact, not
# a verified upgrade. This script closes the gap LOCALLY, where .git exists:
# it checks out the previous release tag in a throwaway clone, builds and
# installs that release, then reinstalls the current HEAD build over it and
# verifies the binary's reported version moved forward and the CLI still runs.
#
# What this does NOT claim: it never performs the QA battery's remote run and
# it does not depend on .git being present in a tar-synced artifact — it runs
# against the real repository history on a machine that has it. The harness
# itself is out of this repo's scope; see docs/guides/upgrade-verification.md
# for how the battery's upgrade cell should consume these semantics.
#
# Usage: scripts/verify-upgrade-path.sh [PREV_TAG] [HEAD_REF]
#   PREV_TAG   previous release tag (default: second-newest tag by version)
#   HEAD_REF   commit/ref to upgrade to (default: HEAD of the current tree)
# Environment:
#   MUSTER_UPGRADE_WORK   use this directory as the scratch area (kept on exit)
#   MUSTER_UPGRADE_KEEP=1 keep the default scratch directory after the run
# Exit codes: 0 = upgrade path verified, 1 = verification failed, 2 = skipped
#   (prerequisites absent, e.g. fewer than two release tags — reported, never
#   treated as a pass).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

fail() { echo "UPGRADE FAIL: $*" >&2; exit 1; }
step() { echo "[step $1] $2"; }

# ── prerequisites ────────────────────────────────────────────────────────────
git -C "$REPO_ROOT" rev-parse --git-dir >/dev/null 2>&1 \
  || fail "$REPO_ROOT is not a git checkout — tag-based upgrade verification needs .git (a tar-synced tree without .git can never run this; that limitation is the QA-MUSTER-4 finding itself)"

tag_count="$(git -C "$REPO_ROOT" tag -l | grep -c . || true)"
if [ "${tag_count:-0}" -lt 2 ]; then
  echo "UPGRADE N/A: $tag_count release tag(s) in repo — upgrade verification needs a previous release and a current one. Not counted as a pass." >&2
  exit 2
fi

prev_tag="${1:-$(git -C "$REPO_ROOT" tag -l | sort -V | tail -2 | head -1)}"
head_ref="${2:-HEAD}"
head_sha="$(git -C "$REPO_ROOT" rev-parse "${head_ref}^{commit}")" \
  || fail "HEAD ref $head_ref not found in repo"
prev_sha="$(git -C "$REPO_ROOT" rev-parse "${prev_tag}^{commit}" 2>/dev/null)" \
  || fail "previous release tag '$prev_tag' not found in repo (tags: $(git -C "$REPO_ROOT" tag -l | sort -V | tr '\n' ' '))"
prev_describe="$(git -C "$REPO_ROOT" describe --tags --always "$prev_tag")"
head_describe="$(git -C "$REPO_ROOT" describe --tags --always "$head_sha")"

if [ "$prev_sha" = "$head_sha" ]; then
  echo "UPGRADE N/A: HEAD ($head_describe) is already the previous release ($prev_tag) — nothing to upgrade. Not counted as a pass." >&2
  exit 2
fi

work="${MUSTER_UPGRADE_WORK:-}"
cleanup() {
  if [ -z "$work_set" ] && [ "${MUSTER_UPGRADE_KEEP:-0}" != "1" ]; then
    rm -rf "$work"
  fi
}
if [ -n "$work" ]; then work_set=1; mkdir -p "$work"; else work_set=""; work="$(mktemp -d /tmp/muster-upgrade-XXXXXX)"; fi
trap cleanup EXIT

echo "== Muster tagged-release upgrade verification =="
echo "previous release: $prev_describe @ $(git -C "$REPO_ROOT" rev-parse --short "$prev_sha")"
echo "current HEAD:     $(git -C "$REPO_ROOT" rev-parse --short "$head_sha") ($head_describe)"

# ── 1. materialize the previous release WITHOUT the current tree's state ────
# A clone (not `git checkout` in place, not `git archive`) gives the previous
# release its own module cache state and working tree, so the upgrade test
# starts from a real v0.1.x install and not from HEAD's files. No network:
# --local clones from this checkout's object store.
step 1 "materialize previous release $prev_tag into a scratch clone"
if ! git clone --quiet --no-hardlinks --shared "$REPO_ROOT" "$work/prev"; then
  git clone --quiet --no-hardlinks "$REPO_ROOT" "$work/prev"
fi
git -C "$work/prev" checkout --quiet --detach "$prev_sha"
echo "  scratch clone at $work/prev ($(git -C "$work/prev" describe --tags --always))"

# ── 2. build + install the previous release ─────────────────────────────────
# Stamp the version via ldflags into the internal/version package path the
# RELEASE'S OWN go.mod declares — historical tags may carry a different
# module path than HEAD (v0.1.0's Makefile stamps the pre-rename
# github.com/openapi-cli/openapi-cli/internal/version path while its go.mod
# already declares github.com/wojons/muster), so a hardcoded current-module
# -X target would silently miss and the binary would report "dev (unknown)".
mod_path="$(awk '$1 == "module" { print $2; exit }' "$work/prev/go.mod")" \
  || fail "read module path from the previous release's go.mod"
prev_ver_pkg="${mod_path}/internal/version"
build_prev="$work/prev/bin/openapi-cli"
prev_ver="$prev_describe"
prev_commit="$(git -C "$work/prev" rev-parse --short HEAD)"
step 2 "build + install previous release $prev_ver ($prev_commit, module $mod_path)"
( cd "$work/prev" && CGO_ENABLED="${CGO_ENABLED:-1}" go build \
    -ldflags "-X $prev_ver_pkg.Version=$prev_ver -X $prev_ver_pkg.Commit=$prev_commit" \
    -o "$build_prev" ./cmd/openapi-cli ) \
  || fail "previous release $prev_tag would not build (see output above; this is what the QA battery's upgrade cell must report instead of N/A)"
prev_reported="$("$build_prev" version)" \
  || fail "previous-release binary failed its own 'version' command: $prev_reported"
# internal/version.Short() prints "<Version> (<Commit>)"; the version command
# adds no binary-name prefix on tagged releases (the "--version" wrapper path
# prints one, the bare `version` subcommand does not).
if [ "$prev_reported" != "$prev_ver ($prev_commit)" ]; then
  fail "previous-release binary reports '$prev_reported' but version stamping targeted $prev_ver_pkg — the tag's version-stamping surface drifted; verify against its own Makefile before trusting this leg"
fi

# ── 3. install HEAD over the previous release ────────────────────────────────
step 3 "reinstall current HEAD over the previous install (upgrade step)"
head_commit="$(git -C "$REPO_ROOT" rev-parse --short "$head_sha")"
head_mod_path="$(awk '$1 == "module" { print $2; exit }' "$REPO_ROOT/go.mod")" \
  || fail "read module path from HEAD's go.mod"
head_ver_pkg="${head_mod_path}/internal/version"
build_head="$work/bin/openapi-cli"
mkdir -p "$work/bin"
cp "$build_prev" "$build_head"   # the previous release is now "installed"
( cd "$REPO_ROOT" && CGO_ENABLED="${CGO_ENABLED:-1}" go build \
    -ldflags "-X $head_ver_pkg.Version=$head_describe -X $head_ver_pkg.Commit=$head_commit" \
    -o "$build_head" ./cmd/openapi-cli ) \
  || fail "current HEAD would not build over the previous install (upgrade to HEAD failed)"

# ── 4. verify the upgrade took ───────────────────────────────────────────────
step 4 "verify version moved and the upgraded binary works"
before_ver="$("$build_prev" version)" \
  || fail "previous-release binary failed its own 'version' command: $before_ver"
after_ver="$("$build_head" version)" \
  || fail "upgraded binary failed its own 'version' command: $after_ver"
case "$after_ver" in
  *"$prev_ver ("*|*"$prev_describe ("*) fail "upgraded binary still reports the previous version: '$after_ver' (expected $head_describe)";;
esac
case "$after_ver" in
  *"$head_describe ("*|"openapi-cli $head_describe ("*) ;;
  *) fail "upgraded binary reports unexpected version '$after_ver' (expected $head_describe)";;
esac
after_cfg="$("$build_head" config show 2>&1)" \
  || fail "upgraded binary failed a basic command (config show): $after_cfg"

echo "  previous install reported: $before_ver"
echo "  upgraded binary reports:   $after_ver"
echo "  config show after upgrade: $(echo "$after_cfg" | head -1)"
echo "UPGRADE OK: $prev_describe → HEAD ($head_describe) reinstall verified locally — the QA battery's tar-sync upgrade N/A is a .git-stripping artifact, not a broken upgrade path. Remote-battery rerun still required for the live cell; not claimed here."