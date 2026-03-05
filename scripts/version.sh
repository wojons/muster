#!/bin/bash
set -e

# Extract version information from git

get_version() {
    git describe --tags --always --dirty 2>/dev/null || echo "dev"
}

get_commit() {
    git rev-parse --short HEAD 2>/dev/null || echo "unknown"
}

get_build_date() {
    date -u +"%Y-%m-%dT%H:%M:%SZ"
}

get_branch() {
    git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown"
}

# Output based on argument
case "${1:-all}" in
    version)
        get_version
        ;;
    commit)
        get_commit
        ;;
    date)
        get_build_date
        ;;
    branch)
        get_branch
        ;;
    json)
        cat <<EOF
{
  "version": "$(get_version)",
  "commit": "$(get_commit)",
  "build_date": "$(get_build_date)",
  "branch": "$(get_branch)"
}
EOF
        ;;
    *)
        echo "VERSION=$(get_version)"
        echo "COMMIT=$(get_commit)"
        echo "BUILD_DATE=$(get_build_date)"
        echo "BRANCH=$(get_branch)"
        ;;
esac
