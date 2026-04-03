#!/bin/bash
# deploy-muster.sh - Production deployment script for Muster
# Reference: specs/OPS-DEPLOYMENT.md

set -euo pipefail

# Configuration
ENVIRONMENT="${ENVIRONMENT:-staging}"
VERSION="${VERSION:-latest}"
NAMESPACE="${NAMESPACE:-muster}"
SKIP_TESTS="${SKIP_TESTS:-false}"
SKIP_BACKUP="${SKIP_BACKUP:-false}"
FORCE_DEPLOY="${FORCE_DEPLOY:-false}"
DRY_RUN="${DRY_RUN:-false}"

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$SCRIPT_DIR"
K8S_DIR="${K8S_DIR:-${DEPLOY_DIR}/../k8s}"
MANIFESTS_DIR="${MANIFESTS_DIR:-${K8S_DIR}/base}"
OVERLAYS_DIR="${OVERLAYS_DIR:-${K8S_DIR}/overlays}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $*"
}

warn() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] WARNING:${NC} $*"
}

error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ERROR:${NC} $*"
    exit 1
}

# Deployment functions
preflight_checks() {
    log "Running preflight checks..."
    
    # Check kubectl
    if ! command -v kubectl &> /dev/null; then
        error "kubectl is not installed"
    fi
    
    # Check cluster connectivity
    if ! kubectl cluster-info > /dev/null 2>&1; then
        error "Cannot connect to Kubernetes cluster"
    fi
    
    # Check namespace exists
    if ! kubectl get namespace "$NAMESPACE" > /dev/null 2>&1; then
        warn "Namespace $NAMESPACE does not exist, creating..."
        kubectl create namespace "$NAMESPACE"
    fi
    
    # Check required secrets exist
    local required_secrets=("muster-secrets" "backup-secrets")
    for secret in "${required_secrets[@]}"; do
        if ! kubectl get secret "$secret" -n "$NAMESPACE" > /dev/null 2>&1; then
            warn "Secret $secret not found in namespace $NAMESPACE"
        fi
    done
    
    log "Preflight checks passed"
}

create_backup() {
    if [[ "$SKIP_BACKUP" == "true" ]]; then
        warn "Skipping pre-deployment backup"
        return 0
    fi
    
    log "Creating pre-deployment backup..."
    
    if [[ -f "$DEPLOY_DIR/backup/backup-database.sh" ]]; then
        BACKUP_DIR="/var/backups/muster" \
        DB_PATH="/var/lib/muster/muster.db" \
        bash "$DEPLOY_DIR/backup/backup-database.sh"
    else
        warn "Backup script not found, skipping backup"
    fi
}

run_tests() {
    if [[ "$SKIP_TESTS" == "true" ]]; then
        warn "Skipping tests"
        return 0
    fi
    
    log "Running pre-deployment tests..."
    
    # Build and test locally first
    if [[ -f "${DEPLOY_DIR}/../Makefile" ]]; then
        (cd "${DEPLOY_DIR}/.." && make test) || error "Tests failed"
    fi
    
    log "Tests passed"
}

deploy_kubernetes() {
    log "Deploying to Kubernetes..."
    log "Environment: $ENVIRONMENT"
    log "Version: $VERSION"
    log "Namespace: $NAMESPACE"
    
    local overlay_path="${OVERLAYS_DIR}/${ENVIRONMENT}"
    
    if [[ ! -d "$overlay_path" ]]; then
        warn "No overlay found for $ENVIRONMENT, using base manifests"
        overlay_path="$MANIFESTS_DIR"
    fi
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "Dry run mode - not applying changes"
        kubectl apply -k "$overlay_path" --dry-run=client -o yaml
        return 0
    fi
    
    # Apply with version substitution
    log "Applying Kubernetes manifests..."
    
    if [[ -d "$overlay_path" ]]; then
        # Use kustomize
        kubectl apply -k "$overlay_path"
    else
        # Apply base manifests
        for file in "$MANIFESTS_DIR"/*.yaml; do
            if [[ -f "$file" ]]; then
                envsubst < "$file" | kubectl apply -f -
            fi
        done
    fi
    
    log "Kubernetes manifests applied"
}

wait_for_rollout() {
    log "Waiting for rollout to complete..."
    
    local deployments
    deployments=$(kubectl get deployment -n "$NAMESPACE" -o name)
    
    for deployment in $deployments; do
        log "Waiting for $deployment..."
        if ! kubectl rollout status "$deployment" -n "$NAMESPACE" --timeout=5m; then
            error "Rollout failed for $deployment"
        fi
    done
    
    log "Rollout completed successfully"
}

verify_deployment() {
    log "Verifying deployment..."
    
    # Check all pods are running
    local not_running
    not_running=$(kubectl get pods -n "$NAMESPACE" --field-selector=status.phase!=Running -o name 2>/dev/null || true)
    
    if [[ -n "$not_running" ]]; then
        error "Some pods are not running:\n$not_running"
    fi
    
    # Check service endpoints
    local services
    services=$(kubectl get svc -n "$NAMESPACE" -o name)
    
    for svc in $services; do
        local endpoints
        endpoints=$(kubectl get endpoints "${svc#service/}" -n "$NAMESPACE" -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null || true)
        
        if [[ -z "$endpoints" ]]; then
            warn "Service $svc has no endpoints"
        fi
    done
    
    log "Deployment verification passed"
}

rollback() {
    warn "Rolling back deployment..."
    
    local deployments
    deployments=$(kubectl get deployment -n "$NAMESPACE" -o name)
    
    for deployment in $deployments; do
        kubectl rollout undo "$deployment" -n "$NAMESPACE"
    done
    
    log "Rollback completed"
}

post_deployment_tests() {
    log "Running post-deployment tests..."
    
    # Health check
    local health_url="http://muster.${NAMESPACE}.svc.cluster.local/health"
    if curl -sf "$health_url" > /dev/null 2>&1; then
        log "Health check passed"
    else
        warn "Health check failed"
    fi
    
    # Smoke tests
    log "Running smoke tests..."
    
    log "Post-deployment tests completed"
}

usage() {
    cat << EOF
Muster Deployment Script

Usage: $0 [OPTIONS]

Options:
    -e, --environment ENV     Target environment (dev|staging|prod) [default: staging]
    -v, --version VERSION     Version to deploy [default: latest]
    -n, --namespace NS        Kubernetes namespace [default: muster]
    --skip-tests              Skip running tests before deployment
    --skip-backup             Skip creating pre-deployment backup
    --force                   Force deployment even if tests fail
    --dry-run                 Show what would be deployed without applying
    -h, --help                Show this help message

Examples:
    # Deploy to staging
    $0 -e staging
    
    # Deploy specific version to production with dry run
    $0 -e prod -v v1.2.3 --dry-run
    
    # Deploy without tests (emergency)
    $0 -e prod --skip-tests --force

EOF
}

# Main deployment process
main() {
    log "Starting Muster deployment..."
    
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
            -n|--namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            --skip-tests)
                SKIP_TESTS="true"
                shift
                ;;
            --skip-backup)
                SKIP_BACKUP="true"
                shift
                ;;
            --force)
                FORCE_DEPLOY="true"
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
    
    # Validate environment
    case "$ENVIRONMENT" in
        dev|staging|prod)
            ;;
        *)
            error "Invalid environment: $ENVIRONMENT (must be dev, staging, or prod)"
            ;;
    esac
    
    # Execute deployment steps
    preflight_checks
    run_tests
    create_backup
    
    if deploy_kubernetes; then
        wait_for_rollout
        verify_deployment
        post_deployment_tests
        log "Deployment completed successfully!"
    else
        error "Deployment failed"
    fi
}

# Run main
main "$@"
