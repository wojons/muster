#!/usr/bin/env bash
# validate-compose.sh — parse-validate every tracked docker-compose file.
#
# Why: `docker compose config` is the only cheap gate that catches schema
# errors (QA-MUSTER-024: a healthcheck.test vector without a CMD/CMD-SHELL/
# NONE prefix aborted `config` with rc=15, so the documented hardened
# deployment path in deployments/DOCKER-SECURITY.md could never start).
# A malformed compose file must fail here, not on the operator's host.
#
# Usage:
#   ./scripts/validate-compose.sh            # validate every tracked file
#   ./scripts/validate-compose.sh -f FILE    # validate one file
#
# Exit codes: 0 all files parse, 1 at least one file failed, 0 with a loud
# SKIP when the docker CLI/plugin is not installed (operators on hosts
# without docker must still be able to run the rest of `make validate`).
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

if ! command -v docker >/dev/null 2>&1; then
    echo "SKIP: docker CLI not found; cannot validate compose files on this host" >&2
    exit 0
fi

if ! docker compose version >/dev/null 2>&1; then
    echo "SKIP: docker compose plugin not available; cannot validate compose files" >&2
    exit 0
fi

files=()
if [ "$#" -gt 0 ] && [ "$1" = "-f" ]; then
    shift
    files=("$@")
else
    # Tracked files only: an untracked scratch compose file is not a
    # deliverable and must not fail the gate.
    while IFS= read -r f; do
        [ -n "$f" ] && files+=("$f")
    done < <(git ls-files | grep -Ei '(^|/)(docker-)?compose(\.[^/]*)?\.ya?ml$')
fi

if [ "${#files[@]}" -eq 0 ]; then
    echo "no compose files tracked; nothing to validate"
    exit 0
fi

fail=0
for f in "${files[@]}"; do
    out=$(docker compose -f "$f" config 2>&1)
    rc=$?
    if [ "$rc" -eq 0 ]; then
        echo "PASS  $f"
    else
        echo "FAIL  $f (rc=$rc)"
        printf '%s\n' "$out" | tail -5 | sed 's/^/      /'
        fail=1
    fi
done

if [ "$fail" -ne 0 ]; then
    echo "compose validation FAILED" >&2
    exit 1
fi
echo "all ${#files[@]} compose file(s) valid"
exit 0
