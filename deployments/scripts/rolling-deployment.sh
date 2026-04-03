#!/bin/bash
# rolling-deployment.sh - Advanced rolling deployment with canary and health gates
# Implements zero-downtime deployment strategy with automatic rollback
# Reference: specs/OPS-DEPLOYMENT.md

set -euo pipefail

# Configuration
ENVIRONMENT="${ENVIRONMENT:-staging}"
VERSION="${VERSION:-latest}"
NAMESPACE="${NAMESPACE:-muster}"
STRATEGY="${STRATEGY:-rolling}"  # rolling, canary, blue-green
CANARY_PERCENTAGE="${CANARY_PERCENTAGE:-10}"
CANARY_DURATION="${CANARY_DURATION:-300}"  # 5 minutes
HEALTH_CHECK_INTERVAL="${HEALTH_CHECK_INTERVAL:-10}"
MAX_HEALTH_CHECK_ATTEMPTS="${MAX_HEALTH_CHECK_ATTEMPTS:-30}"
ERROR_THRESHOLD="${ERROR_THRESHOLD:-1.0}"  # Error rate percentage
LATENCY_THRESHOLD="${LATENCY_THRESHOLD:-500}"  # P95 latency in ms
DRY_RUN="${DRY_RUN:-false}"
FORCE="${FORCE:-false}"

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_DIR="$SCRIPT_DIR"
LOG_FILE="${LOG_FILE:-/var/log/muster/deployments/$(date +%Y%m%d-%H%M%S)-${ENVIRONMENT}.log}"

# State tracking
DEPLOYMENT_START_TIME=""
CANARY_START_TIME=""
ROLLBACK_TRIGGERED=false
DEPLOYMENT_SUCCESSFUL=false

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
    exit 1
}

info() {
    local msg="[$(date '+%Y-%m-%d %H:%M:%S')] INFO: $*"
    echo -e "${BLUE}${msg}${NC}"
    echo "$msg" >> "$LOG_FILE"
}

# Deployment phases
phase_start() {
    local phase="$1"
    log "=== Phase: $phase ==="
    echo "{\"phase\":\"$phase\",\"status\":\"started\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" >> "$LOG_FILE.deployment.json"
}

phase_complete() {
    local phase="$1"
    log "=== Phase Complete: $phase ==="
    echo "{\"phase\":\"$phase\",\"status\":\"completed\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" >> "$LOG_FILE.deployment.json"
}

phase_failed() {
    local phase="$1"
    local reason="$2"
    log "=== Phase Failed: $phase - $reason ==="
    echo "{\"phase\":\"$phase\",\"status\":\"failed\",\"reason\":\"$reason\",\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}" >> "$LOG_FILE.deployment.json"
}

# Health check functions
get_deployment_metrics() {
    local deployment="$1"
    local namespace="$2"
    
    # Get pod metrics
    local pods
    pods=$(kubectl get pods -n "$namespace" -l "app.kubernetes.io/name=$deployment" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true)
    
    local ready_count=0
    local total_count=0
    local error_count=0
    
    for pod in $pods; do
        total_count=$((total_count + 1))
        local status
        status=$(kubectl get pod "$pod" -n "$namespace" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")
        
        if [[ "$status" == "Running" ]]; then
            local ready
            ready=$(kubectl get pod "$pod" -n "$namespace" -o jsonpath='{.status.containerStatuses[0].ready}' 2>/dev/null || echo "false")
            if [[ "$ready" == "true" ]]; then
                ready_count=$((ready_count + 1))
            fi
        elif [[ "$status" == "Failed" || "$status" == "Error" || "$status" == "CrashLoopBackOff" ]]; then
            error_count=$((error_count + 1))
        fi
    done
    
    # Calculate error rate
    local error_rate=0
    if [[ $total_count -gt 0 ]]; then
        error_rate=$(echo "scale=2; ($error_count / $total_count) * 100" | bc 2>/dev/null || echo "0")
    fi
    
    echo "{\"ready\":$ready_count,\"total\":$total_count,\"errors\":$error_count,\"error_rate\":$error_rate}"
}

wait_for_healthy_pods() {
    local deployment="$1"
    local namespace="$2"
    local expected_replicas="$3"
    local max_attempts="${4:-$MAX_HEALTH_CHECK_ATTEMPTS}"
    
    log "Waiting for $expected_replicas healthy pods..."
    
    local attempt=0
    while [[ $attempt -lt $max_attempts ]]; do
        local metrics
        metrics=$(get_deployment_metrics "$deployment" "$namespace")
        local ready
        ready=$(echo "$metrics" | jq -r '.ready')
        local error_rate
        error_rate=$(echo "$metrics" | jq -r '.error_rate')
        
        info "Health check $((attempt + 1))/$max_attempts: $ready/$expected_replicas ready, error rate: ${error_rate}%"
        
        # Check if all pods are ready
        if [[ $ready -ge $expected_replicas ]]; then
            # Verify error rate is within threshold
            if (( $(echo "$error_rate < $ERROR_THRESHOLD" | bc -l) )); then
                log "All pods healthy (ready: $ready, error rate: ${error_rate}%)"
                return 0
            else
                warn "Error rate ${error_rate}% exceeds threshold ${ERROR_THRESHOLD}%"
                return 1
            fi
        fi
        
        # Check if too many errors
        if (( $(echo "$error_rate >= $ERROR_THRESHOLD" | bc -l) )); then
            error "Error rate ${error_rate}% exceeds threshold ${ERROR_THRESHOLD}%"
            return 1
        fi
        
        attempt=$((attempt + 1))
        sleep "$HEALTH_CHECK_INTERVAL"
    done
    
    error "Health check timeout after $max_attempts attempts"
    return 1
}

# Rolling deployment strategy
rolling_deployment() {
    phase_start "rolling_deployment"
    
    local deployment="${1:-openapi-cli}"
    local image_tag="${2:-$VERSION}"
    
    log "Starting rolling deployment for $deployment with image tag $image_tag"
    
    # Get current replica count
    local current_replicas
    current_replicas=$(kubectl get deployment "$deployment" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "3")
    
    log "Current replicas: $current_replicas"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY RUN: Would patch deployment with new image"
        kubectl set image deployment/"$deployment" "openapi-cli=ghcr.io/openapi-cli/openapi-cli:${image_tag}" -n "$NAMESPACE" --dry-run=client
        phase_complete "rolling_deployment"
        return 0
    fi
    
    # Update image
    log "Updating deployment image to $image_tag"
    kubectl set image deployment/"$deployment" "openapi-cli=ghcr.io/openapi-cli/openapi-cli:${image_tag}" -n "$NAMESPACE"
    
    # Wait for rollout
    log "Waiting for rollout to complete..."
    if ! kubectl rollout status deployment/"$deployment" -n "$NAMESPACE" --timeout=10m; then
        phase_failed "rolling_deployment" "rollout failed"
        return 1
    fi
    
    # Health check
    if ! wait_for_healthy_pods "$deployment" "$NAMESPACE" "$current_replicas"; then
        phase_failed "rolling_deployment" "health check failed"
        return 1
    fi
    
    phase_complete "rolling_deployment"
    return 0
}

# Canary deployment strategy
canary_deployment() {
    phase_start "canary_deployment"
    
    local deployment="${1:-openapi-cli}"
    local image_tag="${2:-$VERSION}"
    
    log "Starting canary deployment for $deployment with image tag $image_tag"
    log "Canary percentage: $CANARY_PERCENTAGE%, duration: ${CANARY_DURATION}s"
    
    # Get current replica count
    local total_replicas
    total_replicas=$(kubectl get deployment "$deployment" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "3")
    
    local canary_replicas=$(( (total_replicas * CANARY_PERCENTAGE + 99) / 100 ))
    if [[ $canary_replicas -lt 1 ]]; then
        canary_replicas=1
    fi
    
    local stable_replicas=$((total_replicas - canary_replicas))
    
    log "Total replicas: $total_replicas, Canary: $canary_replicas, Stable: $stable_replicas"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY RUN: Would create canary deployment with $canary_replicas replicas"
        phase_complete "canary_deployment"
        return 0
    fi
    
    # Step 1: Scale down stable deployment
    log "Scaling stable deployment to $stable_replicas replicas"
    kubectl scale deployment "$deployment" --replicas="$stable_replicas" -n "$NAMESPACE"
    kubectl wait --for=jsonpath='{.status.readyReplicas}'="$stable_replicas" deployment/"$deployment" -n "$NAMESPACE" --timeout=5m
    
    # Step 2: Create canary deployment
    log "Creating canary deployment with $canary_replicas replicas"
    
    # Export current deployment YAML and modify for canary
    kubectl get deployment "$deployment" -n "$NAMESPACE" -o yaml | \
        sed "s/name: $deployment/name: ${deployment}-canary/" | \
        sed "s/app.kubernetes.io\/name: $deployment/app.kubernetes.io\/name: ${deployment}-canary/" | \
        sed "s/:$deployment/:${deployment}-canary/" | \
        kubectl apply -f -
    
    # Patch canary with new image and replicas
    kubectl set image deployment/"${deployment}-canary" "openapi-cli=ghcr.io/openapi-cli/openapi-cli:${image_tag}" -n "$NAMESPACE"
    kubectl scale deployment "${deployment}-canary" --replicas="$canary_replicas" -n "$NAMESPACE"
    
    # Wait for canary to be ready
    log "Waiting for canary pods to be ready..."
    if ! kubectl wait --for=jsonpath='{.status.readyReplicas}'="$canary_replicas" deployment/"${deployment}-canary" -n "$NAMESPACE" --timeout=5m; then
        phase_failed "canary_deployment" "canary failed to become ready"
        cleanup_canary "$deployment"
        return 1
    fi
    
    # Step 3: Monitor canary for CANARY_DURATION
    CANARY_START_TIME=$(date +%s)
    log "Monitoring canary for $CANARY_DURATION seconds..."
    
    local elapsed=0
    while [[ $elapsed -lt $CANARY_DURATION ]]; do
        local metrics
        metrics=$(get_deployment_metrics "${deployment}-canary" "$NAMESPACE")
        local error_rate
        error_rate=$(echo "$metrics" | jq -r '.error_rate')
        local ready
        ready=$(echo "$metrics" | jq -r '.ready')
        
        info "Canary monitoring: ${elapsed}s/${CANARY_DURATION}s - $ready/$canary_replicas ready, error rate: ${error_rate}%"
        
        # Check health gates
        if (( $(echo "$error_rate >= $ERROR_THRESHOLD" | bc -l) )); then
            warn "Canary error rate ${error_rate}% exceeds threshold ${ERROR_THRESHOLD}%"
            phase_failed "canary_deployment" "error rate exceeded threshold"
            cleanup_canary "$deployment"
            return 1
        fi
        
        if [[ $ready -lt $canary_replicas ]]; then
            warn "Canary has fewer ready pods than expected"
            phase_failed "canary_deployment" "insufficient ready pods"
            cleanup_canary "$deployment"
            return 1
        fi
        
        sleep "$HEALTH_CHECK_INTERVAL"
        elapsed=$(($(date +%s) - CANARY_START_TIME))
    done
    
    log "Canary monitoring complete - promoting to full deployment"
    
    # Step 4: Promote canary to full deployment
    log "Promoting canary to full deployment..."
    
    # Update original deployment with new image
    kubectl set image deployment/"$deployment" "openapi-cli=ghcr.io/openapi-cli/openapi-cli:${image_tag}" -n "$NAMESPACE"
    kubectl scale deployment "$deployment" --replicas="$total_replicas" -n "$NAMESPACE"
    
    # Wait for original deployment
    if ! kubectl rollout status deployment/"$deployment" -n "$NAMESPACE" --timeout=10m; then
        phase_failed "canary_deployment" "promotion failed"
        return 1
    fi
    
    # Health check
    if ! wait_for_healthy_pods "$deployment" "$NAMESPACE" "$total_replicas"; then
        phase_failed "canary_deployment" "promotion health check failed"
        return 1
    fi
    
    # Step 5: Clean up canary
    cleanup_canary "$deployment"
    
    phase_complete "canary_deployment"
    return 0
}

cleanup_canary() {
    local deployment="$1"
    log "Cleaning up canary deployment..."
    kubectl delete deployment "${deployment}-canary" -n "$NAMESPACE" --ignore-not-found=true
    kubectl delete service "${deployment}-canary" -n "$NAMESPACE" --ignore-not-found=true
}

# Blue/Green deployment strategy
blue_green_deployment() {
    phase_start "blue_green_deployment"
    
    local deployment="${1:-openapi-cli}"
    local image_tag="${2:-$VERSION}"
    
    log "Starting blue/green deployment for $deployment with image tag $image_tag"
    
    # Determine current color (blue or green)
    local current_color
    if kubectl get deployment "${deployment}-blue" -n "$NAMESPACE" -o name > /dev/null 2>&1; then
        current_color="blue"
    elif kubectl get deployment "${deployment}-green" -n "$NAMESPACE" -o name > /dev/null 2>&1; then
        current_color="green"
    else
        # Default to blue
        current_color="blue"
    fi
    
    local new_color
    if [[ "$current_color" == "blue" ]]; then
        new_color="green"
    else
        new_color="blue"
    fi
    
    log "Current color: $current_color, New color: $new_color"
    
    # Get replica count
    local replicas
    if [[ "$current_color" == "blue" ]]; then
        replicas=$(kubectl get deployment "${deployment}-blue" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "3")
    else
        replicas=$(kubectl get deployment "${deployment}-green" -n "$NAMESPACE" -o jsonpath='{.spec.replicas}' 2>/dev/null || echo "3")
    fi
    
    log "Target replicas: $replicas"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY RUN: Would create $new_color deployment with $replicas replicas"
        phase_complete "blue_green_deployment"
        return 0
    fi
    
    # Create new color deployment
    log "Creating $new_color deployment..."
    
    # Get base deployment template
    local base_deployment
    if [[ "$current_color" == "blue" ]]; then
        base_deployment="${deployment}-blue"
    else
        base_deployment="${deployment}-green"
    fi
    
    # Export and modify deployment
    kubectl get deployment "$base_deployment" -n "$NAMESPACE" -o yaml 2>/dev/null | \
        sed "s/name: $base_deployment/name: ${deployment}-${new_color}/" | \
        sed "s/app.kubernetes.io\/name: ${deployment}-${current_color}/app.kubernetes.io\/name: ${deployment}-${new_color}/" | \
        kubectl apply -f -
    
    # Set new image and scale
    kubectl set image deployment/"${deployment}-${new_color}" "openapi-cli=ghcr.io/openapi-cli/openapi-cli:${image_tag}" -n "$NAMESPACE"
    kubectl scale deployment "${deployment}-${new_color}" --replicas="$replicas" -n "$NAMESPACE"
    
    # Wait for new deployment to be ready
    log "Waiting for $new_color deployment to be ready..."
    if ! kubectl wait --for=jsonpath='{.status.readyReplicas}'="$replicas" deployment/"${deployment}-${new_color}" -n "$NAMESPACE" --timeout=5m; then
        phase_failed "blue_green_deployment" "new deployment failed to become ready"
        kubectl delete deployment "${deployment}-${new_color}" -n "$NAMESPACE" --ignore-not-found=true
        return 1
    fi
    
    # Health check on new deployment
    if ! wait_for_healthy_pods "${deployment}-${new_color}" "$NAMESPACE" "$replicas"; then
        phase_failed "blue_green_deployment" "new deployment health check failed"
        kubectl delete deployment "${deployment}-${new_color}" -n "$NAMESPACE" --ignore-not-found=true
        return 1
    fi
    
    # Switch service to new color
    log "Switching service to $new_color..."
    kubectl patch service "$deployment" -n "$NAMESPACE" --type='json' -p="[{'op': 'replace', 'path': '/spec/selector/app.kubernetes.io~1name', 'value':'${deployment}-${new_color}'}]"
    
    # Wait for service to propagate
    log "Waiting for service propagation..."
    sleep 10
    
    # Verify service endpoints
    local endpoints
    endpoints=$(kubectl get endpoints "$deployment" -n "$NAMESPACE" -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null || true)
    if [[ -z "$endpoints" ]]; then
        warn "No service endpoints found after switch"
        # Rollback service
        kubectl patch service "$deployment" -n "$NAMESPACE" --type='json' -p="[{'op': 'replace', 'path': '/spec/selector/app.kubernetes.io~1name', 'value':'${deployment}-${current_color}'}]"
        kubectl delete deployment "${deployment}-${new_color}" -n "$NAMESPACE" --ignore-not-found=true
        phase_failed "blue_green_deployment" "service switch failed"
        return 1
    fi
    
    # Monitor new deployment
    log "Monitoring $new_color deployment for stability..."
    if ! wait_for_healthy_pods "${deployment}-${new_color}" "$NAMESPACE" "$replicas" 6; then
        warn "New deployment unstable - rolling back service"
        kubectl patch service "$deployment" -n "$NAMESPACE" --type='json' -p="[{'op': 'replace', 'path': '/spec/selector/app.kubernetes.io~1name', 'value':'${deployment}-${current_color}'}]"
        kubectl delete deployment "${deployment}-${new_color}" -n "$NAMESPACE" --ignore-not-found=true
        phase_failed "blue_green_deployment" "new deployment unstable"
        return 1
    fi
    
    # Scale down old deployment
    log "Scaling down $current_color deployment..."
    kubectl scale deployment "${deployment}-${current_color}" --replicas=0 -n "$NAMESPACE"
    
    # Keep old deployment for quick rollback (optional: can delete after some time)
    log "Blue/green deployment complete. Old deployment scaled to 0 for quick rollback if needed."
    
    phase_complete "blue_green_deployment"
    return 0
}

# Automatic rollback
auto_rollback() {
    local deployment="$1"
    
    warn "Initiating automatic rollback for $deployment..."
    ROLLBACK_TRIGGERED=true
    
    case "$STRATEGY" in
        canary)
            cleanup_canary "$deployment"
            log "Canary cleaned up, stable deployment unchanged"
            ;;
        blue-green)
            # Service should already be pointing to old color
            log "Service pointing to previous color, new deployment scaled to 0"
            kubectl scale deployment "${deployment}-green" --replicas=0 -n "$NAMESPACE" --ignore-not-found=true
            kubectl scale deployment "${deployment}-blue" --replicas=0 -n "$NAMESPACE" --ignore-not-found=true
            ;;
        rolling)
            log "Rolling back deployment..."
            kubectl rollout undo deployment/"$deployment" -n "$NAMESPACE"
            kubectl rollout status deployment/"$deployment" -n "$NAMESPACE" --timeout=5m
            ;;
    esac
    
    log "Rollback complete"
}

# Main deployment execution
main() {
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
            -s|--strategy)
                STRATEGY="$2"
                shift 2
                ;;
            --canary-percentage)
                CANARY_PERCENTAGE="$2"
                shift 2
                ;;
            --canary-duration)
                CANARY_DURATION="$2"
                shift 2
                ;;
            --dry-run)
                DRY_RUN="true"
                shift
                ;;
            --force)
                FORCE="true"
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
    
    # Validate strategy
    case "$STRATEGY" in
        rolling|canary|blue-green)
            ;;
        *)
            error "Invalid strategy: $STRATEGY (must be rolling, canary, or blue-green)"
            ;;
    esac
    
    # Validate environment
    case "$ENVIRONMENT" in
        dev|staging|prod)
            ;;
        *)
            error "Invalid environment: $ENVIRONMENT (must be dev, staging, or prod)"
            ;;
    esac
    
    DEPLOYMENT_START_TIME=$(date +%s)
    
    log "================================================"
    log "Muster Rolling Deployment"
    log "Environment: $ENVIRONMENT"
    log "Version: $VERSION"
    log "Strategy: $STRATEGY"
    log "================================================"
    
    # Execute deployment based on strategy
    local deployment="openapi-cli"
    
    case "$STRATEGY" in
        rolling)
            if ! rolling_deployment "$deployment" "$VERSION"; then
                if [[ "$FORCE" != "true" ]]; then
                    auto_rollback "$deployment"
                    exit 1
                else
                    warn "Deployment failed but --force specified, continuing"
                fi
            fi
            ;;
        canary)
            if ! canary_deployment "$deployment" "$VERSION"; then
                if [[ "$FORCE" != "true" ]]; then
                    exit 1
                else
                    warn "Canary failed but --force specified, continuing"
                fi
            fi
            ;;
        blue-green)
            if ! blue_green_deployment "$deployment" "$VERSION"; then
                if [[ "$FORCE" != "true" ]]; then
                    exit 1
                else
                    warn "Blue/green failed but --force specified, continuing"
                fi
            fi
            ;;
    esac
    
    DEPLOYMENT_SUCCESSFUL=true
    local duration=$(($(date +%s) - DEPLOYMENT_START_TIME))
    
    log "================================================"
    log "Deployment completed successfully!"
    log "Duration: ${duration}s"
    log "Log file: $LOG_FILE"
    log "================================================"
}

usage() {
    cat << EOF
Muster Rolling Deployment Script

Usage: $0 [OPTIONS]

Options:
    -e, --environment ENV       Target environment (dev|staging|prod) [default: staging]
    -v, --version VERSION       Version to deploy [default: latest]
    -n, --namespace NS          Kubernetes namespace [default: muster]
    -s, --strategy STRATEGY     Deployment strategy (rolling|canary|blue-green) [default: rolling]
    --canary-percentage PCT     Canary percentage for canary strategy [default: 10]
    --canary-duration SEC       Canary monitoring duration in seconds [default: 300]
    --dry-run                   Show what would be deployed without applying
    --force                     Force deployment even if health checks fail
    -h, --help                  Show this help message

Examples:
    # Rolling deployment to staging
    $0 -e staging -v v1.2.3

    # Canary deployment (10% traffic for 5 minutes)
    $0 -e prod -v v1.2.3 --strategy canary --canary-percentage 10 --canary-duration 300

    # Blue/Green deployment
    $0 -e prod -v v1.2.3 --strategy blue-green

    # Dry run
    $0 -e prod -v v1.2.3 --dry-run

EOF
}

# Run main
main "$@"
