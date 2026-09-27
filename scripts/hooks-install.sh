#!/bin/bash
# Local-only convenience target: once per clone, make git aware of the
# repository's own pre-commit guard (.githooks/pre-commit).
# Untracked-by-design: core.hooksPath must remain a LOCAL setting so the
# fleet's per-worktree/per-clone hook setup is never overridden.

set -euo pipefail

git config core.hooksPath .githooks
echo "core.hooksPath set to .githooks (local config)."
echo "The guard (.githooks/pre-commit) now runs on every commit in this clone."
