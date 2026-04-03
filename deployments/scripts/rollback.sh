#!/bin/bash
# rollback.sh - Emergency rollback script for Muster deployments
# Reference: specs/OPS-DEPLOYMENT.md, OPS-BACKUP-RECOVERY.md

set -euo pipefail

# Configuration
ENVIRONMENT="${ENVIRONMENT:-staging}"
VERSION="${VERSION:-}"
NAMESPACE="${NAMESPACE:-muster}"
FORCE="${FORCE:-false}"
DB_BACKUP_ID="${DB_BACKUP_ID:-}"
DRY_RUN="${DRY_RUN:-false}"

# Paths
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_FILE="${LOG_FILE:-/var/log/muster/rollbacks/$(date +%Y%m%d-%H%M%S)-${ENVIRONMENT}.log}"

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

# Get current deployment state
get_current_state() {
    log "Capturing current deployment state..."
    
    local state_file="/tmp/muster-rollback-state-${ENVIRONMENT}-$(date +%s).json"
    
    cat > "$state_file" << EOF
{
    "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
    "environment": "$ENVIRONMENT",
    "namespace": "$NAMESPACE",
    "deployments": $(kubectl get deployments -n "$NAMESPACE" -o json 2>/dev/null || echo '{}'),
    "services": $(kubectl get services -n "$NAMESPACE" -o json 2>/dev/null || echo '{}'),
    "pods": $(kubectl get pods -n "$NAMESPACE" -o json 2>/dev/null || echo '{}'),
    "replicasets": $(kubectl get replicasets -n "$NAMESPACE" -o json 2>/dev/null || echo '{}')
}
EOF
    
    log "Current state captured: $state_file"
    echo "$state_file"
}

# Rollback Kubernetes deployment
rollback_kubernetes() {
    log "Rolling back Kubernetes deployments..."
    
    local deployments
    deployments=$(kubectl get deployments -n "$NAMESPACE" -o name 2>/dev/null || true)
    
    if [[ -z "$deployments" ]]; then
        warn "No deployments found in namespace $NAMESPACE"
        return 0
    fi
    
    for deployment in $deployments; do
        log "Rolling back $deployment..."
        
        # Get previous revision
        local prev_revision
        prev_revision=$(kubectl rollout history "$deployment" -n "$NAMESPACE" | tail -n 2 | head -n 1 | awk '{print $1}')
        
        if [[ -n "$prev_revision" ]]; then
            log "Reverting $deployment to revision $prev_revision"
            
            if [[ "$DRY_RUN" == "true" ]]; then
                log "DRY RUN: Would execute: kubectl rollout undo $deployment -n $NAMESPACE"
            else
                kubectl rollout undo "$deployment" -n "$NAMESPACE"
            fi
        else
            warn "No previous revision found for $deployment"
        fi
    done
    
    log "Kubernetes rollback initiated"
}

# Wait for rollback to complete
wait_for_rollback() {
    log "Waiting for rollback to complete..."
    
    local deployments
    deployments=$(kubectl get deployments -n "$NAMESPACE" -o name 2>/dev/null || true)
    
    for deployment in $deployments; do
        log "Waiting for $deployment rollback..."
        
        if [[ "$DRY_RUN" == "true" ]]; then
            log "DRY RUN: Would wait for rollout status"
        else
            if ! kubectl rollout status "$deployment" -n "$NAMESPACE" --timeout=5m; then
                error "Rollback failed for $deployment"
            fi
        fi
    done
    
    log "Rollback completed successfully"
}

# Rollback database if backup provided
rollback_database() {
    if [[ -z "$DB_BACKUP_ID" ]]; then
        warn "No database backup ID provided, skipping database rollback"
        return 0
    fi
    
    log "Rolling back database to backup: $DB_BACKUP_ID"
    
    if [[ ! -f "$SCRIPT_DIR/../backup/restore-database.sh" ]]; then
        error "Database restore script not found"
    fi
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log "DRY RUN: Would execute restore with backup ID: $DB_BACKUP_ID"
    else
        bash "$SCRIPT_DIR/../backup/restore-database.sh" \
            --backup-id "$DB_BACKUP_ID" \
            --target-db "muster-${ENVIRONMENT}" \
            --confirm
        
        log "Database rollback completed"
    fi
}

# Verify rollback success
verify_rollback() {
    log "Verifying rollback..."
    
    # Check pod health
    local not_running
    not_running=$(kubectl get pods -n "$NAMESPACE" --field-selector=status.phase!=Running -o name 2>/dev/null || true)
    
    if [[ -n "$not_running" ]]; then
        warn "Some pods are not running after rollback:\n$not_running"
        
        if [[ "$FORCE" != "true" ]]; then
            error "Rollback verification failed"
        fi
    fi
    
    # Check health endpoints
    local health_url="http://muster.${NAMESPACE}.svc.cluster.local/health"
    if curl -sf "$health_url" > /dev/null 2>&1; then
        log "Health check passed"
    else
        warn "Health check failed after rollback"
    fi
    
    log "Rollback verification complete"
}

# Notify team of rollback
notify_rollback() {
    local reason="${1:-Manual rollback initiated}"
    local duration="${2:-0}"
    
    log "Sending rollback notifications..."
    
    # Log notification
    cat >> "$LOG_FILE" << EOF

========================================
ROLLBACK NOTIFICATION
========================================
Timestamp: $(date -u +%Y-%m-%dT%H:%M:%SZ)
Environment: $ENVIRONMENT
Namespace: $NAMESPACE
Reason: $reason
Duration: ${duration}s
Status: COMPLETED
========================================

EOF
    
    # If slack webhook configured
    if [[ -n "${SLACK_WEBHOOK_URL:-}" ]]; then
        local payload
        payload=$(cat << EOF
{
    "text": "🚨 ROLLBACK EXECUTED - Muster",
    "attachments": [
        {
            "color": "danger",
            "fields": [
                {"title": "Environment", "value": "$ENVIRONMENT", "short": true},
                {"title": "Duration", "value": "${duration}s", "short": true},
                {"title": "Reason", "value": "$reason", "short": false}
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
    
    log "Notifications sent"
}

# Generate rollback report
generate_report() {
    local start_time="$1"
    local end_time="$2"
    local reason="$3"
    
    local duration=$((end_time - start_time))
    local report_file="/var/log/muster/reports/rollback-${ENVIRONMENT}-$(date +%Y%m%d-%H%M%S).md"
    
    mkdir -p "$(dirname "$report_file")"
    
    cat > "$report_file" << EOF
# Muster Rollback Report

**Environment**: $ENVIRONMENT  
**Namespace**: $NAMESPACE  
**Timestamp**: $(date -u +%Y-%m-%dT%H:%M:%SZ)  
**Duration**: ${duration}s  
**Reason**: $reason

## Summary

- Rollback initiated: $(date -d @$start_time '+%Y-%m-%d %H:%M:%S UTC')
- Rollback completed: $(date -d @$end_time '+%Y-%m-%d %H:%M:%S UTC')
- Status: COMPLETED

## Actions Taken

1. Captured pre-rollback state
2. Executed Kubernetes rollback
3. Waited for pods to stabilize
EOF

    if [[ -n "$DB_BACKUP_ID" ]]; then
        echo "4. Restored database from backup: $DB_BACKUP_ID" >> "$report_file"
    fi

    cat >> "$report_file" << EOF
4. Verified rollback success
5. Sent notifications

## Current State

\`\`\`bash
$(kubectl get pods -n "$NAMESPACE" 2>/dev/null || echo "Unable to get pod status")
\`\`\`

## Logs

Full logs available at: $LOG_FILE

## Next Steps

1. Investigate root cause of deployment failure
2. Review and update deployment procedures if needed
3. Monitor system for stability
4. Schedule post-mortem if required
EOF

    log "Rollback report generated: $report_file"
    echo "$report_file"
}

# Main rollback execution
main() {
    local start_time
    start_time=$(date +%s)
    local rollback_reason="Manual rollback initiated"
    
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
            --backup-id)
                DB_BACKUP_ID="$2"
                shift 2
                ;;
            --reason)
                rollback_reason="$2"
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
    
    # Validate environment
    case "$ENVIRONMENT" in
        dev|staging|prod)
            ;;
        *)
            error "Invalid environment: $ENVIRONMENT (must be dev, staging, or prod)"
            ;;
    esac
    
    log "================================================"
    log "Muster Emergency Rollback"
    log "Environment: $ENVIRONMENT"
    log "Namespace: $NAMESPACE"
    log "Reason: $rollback_reason"
    log "================================================"
    
    if [[ "$ENVIRONMENT" == "prod" && "$FORCE" != "true" ]]; then
        warn "Rolling back PRODUCTION deployment!"
        warn "Add --force to proceed"
        exit 1
    fi
    
    # Execute rollback
    local state_file
    state_file=$(get_current_state)
    
    rollback_kubernetes
    wait_for_rollback
    rollback_database
    verify_rollback
    
    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    notify_rollback "$rollback_reason" "$duration"
    
    local report_file
    report_file=$(generate_report "$start_time" "$end_time" "$rollback_reason")
    
    log "================================================"
    log "Rollback completed successfully!"
    log "Duration: ${duration}s"
    log "Log file: $LOG_FILE"
    log "Report: $report_file"
    log "State file: $state_file"
    log "================================================"
}

usage() {
    cat << EOF
Muster Emergency Rollback Script

Usage: $0 [OPTIONS]

Options:
    -e, --environment ENV     Target environment (dev|staging|prod) [default: staging]
    -n, --namespace NS        Kubernetes namespace [default: muster]
    --backup-id ID            Database backup ID to restore (optional)
    --reason TEXT             Reason for rollback
    --dry-run                 Show what would be rolled back without applying
    --force                   Force rollback on production without confirmation
    -h, --help                Show this help message

Examples:
    # Rollback to previous version
    $0 -e staging
    
    # Rollback with database restore
    $0 -e prod --backup-id backup-20260402-140000 --reason "Critical bug"
    
    # Dry run
    $0 -e prod --dry-run

EOF
}

# Run main
main "$@"
