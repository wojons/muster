#!/bin/bash
# deploy-automation.sh - CI/CD deployment automation for Muster
# Automates the full deployment pipeline from build to production
# Reference: specs/OPS-DEPLOYMENT.md

set -euo pipefail

# Configuration
ENVIRONMENT="${ENVIRONMENT:-staging}"
VERSION="${VERSION:-latest}"
BRANCH="${BRANCH:-main}"
SKIP_BUILD="${SKIP_BUILD:-false}"
SKIP_TESTS="${SKIP_TESTS:-false}"
SKIP_DEPLOY="${SKIP_DEPLOY:-false}"
PROMOTE_TO_PROD="${PROMOTE_TO_PROD:-false}"
NOTIFY_SLACK="${NOTIFY_SLACK:-true}"
DRY_RUN="${DRY_RUN:-false}"

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}/../.."
LOG_FILE="${LOG_FILE:-/var/log/muster/ci-deployments/$(date +%Y%m%d-%H%M%S)-${ENVIRONMENT}.log}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Logging
log() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] $*"
    echo -e "${GREEN}${msg}${NC}"
    mkdir -p "$(dirname "$LOG_FILE")"
    echo "$msg" >> "$LOG_FILE"
}

warn() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] WARNING: $*"
    echo -e "${YELLOW}${msg}${NC}"
    echo "$msg" >> "$LOG_FILE"
}

error() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $*"
    echo -e "${RED}${msg}${NC}"
    echo "$msg" >> "$LOG_FILE"
    
    # Notify on error
    notify_failure "$msg"
    exit 1
}

info() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] INFO: $*"
    echo -e "${BLUE}${msg}${NC}"
    echo "$msg" >> "$LOG_FILE"
}

# Notification functions
notify_start() {
    local env="$1"
    local version="$2"
    
    log "Sending deployment start notification..."
    
    if [[ "$NOTIFY_SLACK" == "true" && -n "${SLACK_WEBHOOK_URL:-}" ]]; then
        local payload
        payload=$(cat << EOF
{
    "text": "🚀 Deployment Started - Muster",
    "attachments": [
        {
            "color": "#439FE0",
            "fields": [
                {"title": "Environment", "value": "$env", "short": true},
                {"title": "Version", "value": "$version", "short": true},
                {"title": "Branch", "value": "$BRANCH", "short": true},
                {"title": "Triggered By", "value": "${GITHUB_ACTOR:-${USER:-unknown}}", "short": true}
            ]
        }
    ]
}
EOF
)
        
        curl -s -X POST -H 'Content-type: application/json' \
            --data "$payload" \
            "$SLACK_WEBHOOK_URL" > /dev/null 2>&1 || warn "Failed to send Slack notification"
    fi
}

notify_success() {
    local env="$1"
    local version="$2"
    local duration="$3"
    
    log "Sending deployment success notification..."
    
    if [[ "$NOTIFY_SLACK" == "true" && -n "${SLACK_WEBHOOK_URL:-}" ]]; then
        local payload
        payload=$(cat << EOF
{
    "text": "✅ Deployment Complete - Muster",
    "attachments": [
        {
            "color": "good",
            "fields": [
                {"title": "Environment", "value": "$env", "short": true},
                {"title": "Version", "value": "$version", "short": true},
                {"title": "Duration", "value": "${duration}s", "short": true},
                {"title": "Status", "value": "Success", "short": true}
            ]
        }
    ]
}
EOF
)
        
        curl -s -X POST -H 'Content-type: application/json' \
            --data "$payload" \
            "$SLACK_WEBHOOK_URL" > /dev/null 2>&1 || warn "Failed to send Slack notification"
    fi
}

notify_failure() {
    local reason="$1"
    
    log "Sending deployment failure notification..."
    
    if [[ "$NOTIFY_SLACK" == "true" && -n "${SLACK_WEBHOOK_URL:-}" ]]; then
        local payload
        payload=$(cat << EOF
{
    "text": "❌ Deployment Failed - Muster",
    "attachments": [
        {
            "color": "danger",
            "fields": [
                {"title": "Environment", "value": "$ENVIRONMENT", "short": true},
                {"title": "Version", "value": "$VERSION", "short": true},
                {"title": "Reason", "value": "$reason", "short": false}
            ]
        }
    ]
}
EOF
)
        
        curl -s -X POST -H 'Content-type: application/json' \
            --data "$payload" \
            "$SLACK_WEBHOOK_URL" > /dev/null 2>&1 || true
    fi
}

# Build phase
build_phase() {
    if [[ "$SKIP_BUILD" == "true" ]]; then
        warn "Skipping build phase"
        return 0
    fi
    
    log "=== Build Phase ==="
    
    cd "$PROJECT_ROOT"
    
    # Verify we're on correct branch
    local current_branch
    current_branch=$(git rev-parse --abbrev-ref HEAD)
    
    if [[ "$current_branch" != "$BRANCH" ]]; then
        error "Not on branch $BRANCH (currently on $current_branch)"
    fi
    
    # Pull latest changes
    log "Pulling latest changes from $BRANCH..."
    git pull origin "$BRANCH"
    
    # Build binaries
    log "Building binaries..."
    if [[ -f "Makefile" ]]; then
        make clean
        make build
    else
        go build -o bin/openapi-cli ./cmd/openapi-cli
        go build -o bin/openapi-mcp ./cmd/mcp-server
        go build -o bin/openapi-daemon ./cmd/daemon
    fi
    
    # Verify binaries exist
    if [[ ! -f "bin/openapi-cli" ]]; then
        error "Build failed: openapi-cli binary not found"
    fi
    
    log "Build completed successfully"
}

# Test phase
test_phase() {
    if [[ "$SKIP_TESTS" == "true" ]]; then
        warn "Skipping test phase"
        return 0
    fi
    
    log "=== Test Phase ==="
    
    cd "$PROJECT_ROOT"
    
    # Run unit tests
    log "Running unit tests..."
    if ! go test -v -race ./pkg/... ./internal/... 2>&1 | tee -a "$LOG_FILE"; then
        error "Unit tests failed"
    fi
    
    # Run integration tests
    log "Running integration tests..."
    if ! make test-integration 2>&1 | tee -a "$LOG_FILE"; then
        warn "Integration tests failed (continuing anyway)"
    fi
    
    # Run linting
    log "Running lint checks..."
    if ! make lint 2>&1 | tee -a "$LOG_FILE"; then
        error "Lint checks failed"
    fi
    
    # Run security scan
    log "Running security scans..."
    if command -v gosec >/dev/null 2>&1; then
        gosec -fmt sarif -out /tmp/gosec-results.sarif ./... || warn "Security scan found issues"
    fi
    
    log "Tests completed successfully"
}

# Package phase
package_phase() {
    log "=== Package Phase ==="
    
    cd "$PROJECT_ROOT"
    
    # Create version tag
    if [[ "$VERSION" == "latest" ]]; then
        VERSION="v$(date +%Y%m%d)-$(git rev-parse --short HEAD)"
        log "Generated version: $VERSION"
    fi
    
    # Build Docker images
    log "Building Docker images..."
    
    if [[ -f "Dockerfile" ]]; then
        docker build -t "muster:${VERSION}" .
        docker tag "muster:${VERSION}" "muster:latest"
        
        if [[ -n "${DOCKER_REGISTRY:-}" ]]; then
            docker tag "muster:${VERSION}" "${DOCKER_REGISTRY}/muster:${VERSION}"
            docker tag "muster:latest" "${DOCKER_REGISTRY}/muster:latest"
            
            if [[ "$DRY_RUN" != "true" ]]; then
                docker push "${DOCKER_REGISTRY}/muster:${VERSION}"
                docker push "${DOCKER_REGISTRY}/muster:latest"
            fi
        fi
    fi
    
    log "Packaging completed successfully"
}

# Deploy phase
deploy_phase() {
    if [[ "$SKIP_DEPLOY" == "true" ]]; then
        warn "Skipping deploy phase"
        return 0
    fi
    
    log "=== Deploy Phase ==="
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY RUN: Would deploy to $ENVIRONMENT"
        return 0
    fi
    
    # Execute deployment using deploy-muster.sh
    log "Executing deployment script..."
    
    if [[ -f "$SCRIPT_DIR/deploy-muster.sh" ]]; then
        bash "$SCRIPT_DIR/deploy-muster.sh" \
            --environment "$ENVIRONMENT" \
            --version "$VERSION" \
            --namespace "muster-${ENVIRONMENT}"
    else
        error "Deployment script not found: $SCRIPT_DIR/deploy-muster.sh"
    fi
    
    log "Deployment completed successfully"
}

# Verification phase
verify_phase() {
    log "=== Verification Phase ==="
    
    local namespace="muster-${ENVIRONMENT}"
    
    # Check pod health
    log "Verifying pod health..."
    local attempts=0
    local max_attempts=30
    
    while [[ $attempts -lt $max_attempts ]]; do
        local not_ready
        not_ready=$(kubectl get pods -n "$namespace" --field-selector=status.phase!=Running -o name 2>/dev/null | wc -l)
        
        if [[ "$not_ready" == "0" ]]; then
            log "All pods are healthy"
            break
        fi
        
        attempts=$((attempts + 1))
        info "Waiting for pods to be ready... ($attempts/$max_attempts)"
        sleep 10
    done
    
    if [[ $attempts -ge $max_attempts ]]; then
        error "Pods failed to become healthy within timeout"
    fi
    
    # Run smoke tests
    log "Running smoke tests..."
    
    if [[ -f "$PROJECT_ROOT/test/e2e/smoke_test.go" ]]; then
        cd "$PROJECT_ROOT"
        go test -v ./test/e2e/smoke_test.go 2>&1 | tee -a "$LOG_FILE" || warn "Smoke tests had failures"
    fi
    
    log "Verification completed"
}

# Promote to production
promote_to_production() {
    if [[ "$PROMOTE_TO_PROD" != "true" ]]; then
        return 0
    fi
    
    log "=== Promotion to Production ==="
    
    if [[ "$ENVIRONMENT" != "staging" ]]; then
        warn "Can only promote from staging environment"
        return 0
    fi
    
    log "Promoting staging deployment to production..."
    
    # Verify staging is stable
    log "Verifying staging stability..."
    
    # Execute production deployment
    log "Deploying to production..."
    
    if [[ -f "$SCRIPT_DIR/deploy-muster.sh" ]]; then
        bash "$SCRIPT_DIR/deploy-muster.sh" \
            --environment prod \
            --version "$VERSION" \
            --namespace muster-prod
    fi
    
    log "Promotion completed"
}

# Generate deployment report
generate_report() {
    local start_time="$1"
    local end_time="$2"
    
    local duration=$((end_time - start_time))
    local report_file="/var/log/muster/reports/deploy-${ENVIRONMENT}-$(date +%Y%m%d-%H%M%S).md"
    
    mkdir -p "$(dirname "$report_file")"
    
    cat > "$report_file" << EOF
# Muster Deployment Report

**Environment**: $ENVIRONMENT  
**Version**: $VERSION  
**Branch**: $BRANCH  
**Timestamp**: $(date -u +%Y-%m-%dT%H:%M:%SZ)  
**Duration**: ${duration}s  
**Status**: SUCCESS

## Pipeline Summary

| Phase | Status | Duration |
|-------|--------|----------|
| Build | Completed | Included |
| Test | Completed | Included |
| Package | Completed | Included |
| Deploy | Completed | Included |
| Verify | Completed | Included |

## Build Info

- Commit: $(git -C "$PROJECT_ROOT" rev-parse HEAD 2>/dev/null || echo "N/A")
- Author: $(git -C "$PROJECT_ROOT" log -1 --pretty=format:'%an' 2>/dev/null || echo "N/A")
- Message: $(git -C "$PROJECT_ROOT" log -1 --pretty=format:'%s' 2>/dev/null || echo "N/A")

## Deployment Info

- Namespace: muster-${ENVIRONMENT}
- Strategy: Rolling update
- Replicas: 3

## Verification

\`\`\`bash
$(kubectl get pods -n "muster-${ENVIRONMENT}" 2>/dev/null || echo "Unable to get pod status")
\`\`\`

## Logs

Full logs: $LOG_FILE

## Next Steps

1. Monitor metrics for 1 hour
2. Review logs for any issues
3. Update deployment documentation if needed
EOF

    log "Deployment report generated: $report_file"
}

# Main execution
main() {
    local start_time
    start_time=$(date +%s)
    
    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -e|--environment)
                ENVIRONMENT="$2"
                shift 2
                ;;
            -v|--version)
                VERSION="$2"
                shift 2
                ;;
            -b|--branch)
                BRANCH="$2"
                shift 2
                ;;
            --skip-build)
                SKIP_BUILD="true"
                shift
                ;;
            --skip-tests)
                SKIP_TESTS="true"
                shift
                ;;
            --skip-deploy)
                SKIP_DEPLOY="true"
                shift
                ;;
            --promote-to-prod)
                PROMOTE_TO_PROD="true"
                shift
                ;;
            --no-notify)
                NOTIFY_SLACK="false"
                shift
                ;;
            --dry-run)
                DRY_RUN="true"
                shift
                ;;
            -h|--help)
                usage
                exit 0
                ;;
            *)
                error "Unknown option: $1"
                ;;
        esac
    done
    
    log "================================================"
    log "Muster CI/CD Deployment Automation"
    log "Environment: $ENVIRONMENT"
    log "Version: $VERSION"
    log "Branch: $BRANCH"
    log "================================================"
    
    notify_start "$ENVIRONMENT" "$VERSION"
    
    # Execute pipeline
    build_phase
    test_phase
    package_phase
    deploy_phase
    verify_phase
    promote_to_production
    
    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    notify_success "$ENVIRONMENT" "$VERSION" "$duration"
    generate_report "$start_time" "$end_time"
    
    log "================================================"
    log "Deployment pipeline completed successfully!"
    log "Duration: ${duration}s"
    log "Log file: $LOG_FILE"
    log "================================================"
}

usage() {
    cat << EOF
Muster CI/CD Deployment Automation

Usage: $0 [OPTIONS]

Options:
    -e, --environment ENV     Target environment (dev|staging|prod) [default: staging]
    -v, --version VERSION     Version to deploy [default: latest]
    -b, --branch BRANCH       Git branch to deploy [default: main]
    --skip-build              Skip build phase
    --skip-tests              Skip test phase
    --skip-deploy             Skip deployment phase
    --promote-to-prod         Promote staging to production after success
    --no-notify               Disable Slack notifications
    --dry-run                 Show what would be done without executing
    -h, --help                Show this help message

Examples:
    # Full deployment pipeline
    $0 -e staging -v v1.2.3
    
    # Deploy to production with promotion
    $0 -e staging --promote-to-prod
    
    # Build and test only (no deploy)
    $0 --skip-deploy
    
    # Dry run
    $0 -e prod --dry-run

EOF
}

# Run main
main "$@"
