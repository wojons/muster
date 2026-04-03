#!/bin/bash
# backup-monitor.sh - Continuous monitoring for backup health
# Reference: specs/OPS-BACKUP-RECOVERY.md

set -euo pipefail

# Configuration
BACKUP_DIR="${BACKUP_DIR:-/var/backups/muster}"
MAX_BACKUP_AGE="${MAX_BACKUP_AGE:-86400}"  # 24 hours in seconds
MIN_BACKUP_SIZE="${MIN_BACKUP_SIZE:-1024}"  # 1KB
METRICS_FILE="${METRICS_FILE:-/var/lib/muster/metrics/backup.prom}"
ALERT_THRESHOLD_FAILED="${ALERT_THRESHOLD_FAILED:-3}"

# Logging
LOG_FILE="${BACKUP_DIR}/monitor.log"

# Functions
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

# Prometheus metrics format output
write_metric() {
    local name="$1"
    local value="$2"
    local labels="${3:-}"
    
    if [[ -n "$labels" ]]; then
        echo "${name}{${labels}} ${value}" >> "$METRICS_FILE.tmp"
    else
        echo "${name} ${value}" >> "$METRICS_FILE.tmp"
    fi
}

# Generate Prometheus metrics
generate_metrics() {
    mkdir -p "$(dirname "$METRICS_FILE")"
    
    # Clear temp file
    > "$METRICS_FILE.tmp"
    
    # Header
    echo "# HELP muster_backup_last_timestamp Unix timestamp of last backup" >> "$METRICS_FILE.tmp"
    echo "# TYPE muster_backup_last_timestamp gauge" >> "$METRICS_FILE.tmp"
    
    # Find latest backup
    local latest_backup
    latest_backup=$(find "$BACKUP_DIR" -name "muster_backup_*.db*" -type f -printf '%T@ %p\n' 2>/dev/null | \
        sort -rn | head -1 | cut -d' ' -f2-)
    
    if [[ -n "$latest_backup" ]]; then
        local timestamp
        timestamp=$(stat -c %Y "$latest_backup" 2>/dev/null || stat -f %m "$latest_backup")
        write_metric "muster_backup_last_timestamp" "$timestamp"
        
        # Calculate age
        local now
        now=$(date +%s)
        local age=$((now - timestamp))
        write_metric "muster_backup_age_seconds" "$age"
        
        # Check if backup is too old
        if [[ $age -gt $MAX_BACKUP_AGE ]]; then
            write_metric "muster_backup_stale" "1"
            log "WARNING: Last backup is ${age}s old (max: ${MAX_BACKUP_AGE}s)"
        else
            write_metric "muster_backup_stale" "0"
        fi
        
        # Backup size
        local size
        size=$(stat -c%s "$latest_backup" 2>/dev/null || stat -f%z "$latest_backup")
        write_metric "muster_backup_last_size_bytes" "$size"
        
        # Check size
        if [[ $size -lt $MIN_BACKUP_SIZE ]]; then
            write_metric "muster_backup_too_small" "1"
            log "WARNING: Last backup is only ${size} bytes (min: ${MIN_BACKUP_SIZE})"
        else
            write_metric "muster_backup_too_small" "0"
        fi
    else
        # No backups found
        write_metric "muster_backup_last_timestamp" "0"
        write_metric "muster_backup_age_seconds" "0"
        write_metric "muster_backup_stale" "1"
        write_metric "muster_backup_last_size_bytes" "0"
        write_metric "muster_backup_too_small" "0"
        log "WARNING: No backups found in $BACKUP_DIR"
    fi
    
    # Total backup count
    local backup_count
    backup_count=$(find "$BACKUP_DIR" -name "muster_backup_*.db*" -type f | wc -l)
    echo "# HELP muster_backup_total_count Total number of backups" >> "$METRICS_FILE.tmp"
    echo "# TYPE muster_backup_total_count gauge" >> "$METRICS_FILE.tmp"
    write_metric "muster_backup_total_count" "$backup_count"
    
    # Total backup size
    local total_size
    total_size=$(find "$BACKUP_DIR" -name "muster_backup_*.db*" -type f -exec stat -c%s {} \; 2>/dev/null | \
        awk '{sum+=$1} END {print sum}' || echo "0")
    echo "# HELP muster_backup_total_size_bytes Total size of all backups" >> "$METRICS_FILE.tmp"
    echo "# TYPE muster_backup_total_size_bytes gauge" >> "$METRICS_FILE.tmp"
    write_metric "muster_backup_total_size_bytes" "${total_size:-0}"
    
    # Check backup success log
    if [[ -f "$BACKUP_DIR/backup.log" ]]; then
        local last_success
        last_success=$(grep "completed successfully" "$BACKUP_DIR/backup.log" | tail -1 | \
            awk '{print $1" "$2}' | tr -d '[]')
        
        if [[ -n "$last_success" ]]; then
            local success_timestamp
            success_timestamp=$(date -d "$last_success" +%s 2>/dev/null || \
                date -j -f "%Y-%m-%d %H:%M:%S" "$last_success" +%s 2>/dev/null)
            
            if [[ -n "${success_timestamp:-}" ]]; then
                local success_age
                success_age=$(($(date +%s) - success_timestamp))
                echo "# HELP muster_backup_last_success_age_seconds Age of last successful backup" >> "$METRICS_FILE.tmp"
                echo "# TYPE muster_backup_last_success_age_seconds gauge" >> "$METRICS_FILE.tmp"
                write_metric "muster_backup_last_success_age_seconds" "$success_age"
            fi
        fi
    fi
    
    # Failed backups in last 24h
    local failed_count=0
    if [[ -f "$BACKUP_DIR/backup.log" ]]; then
        failed_count=$(grep "$(date +%Y-%m-%d)" "$BACKUP_DIR/backup.log" | grep -c "ERROR:" || echo "0")
    fi
    echo "# HELP muster_backup_failures_today Number of failed backups today" >> "$METRICS_FILE.tmp"
    echo "# TYPE muster_backup_failures_today gauge" >> "$METRICS_FILE.tmp"
    write_metric "muster_backup_failures_today" "$failed_count"
    
    # Move temp to final
    mv "$METRICS_FILE.tmp" "$METRICS_FILE"
    
    log "Metrics updated: $METRICS_FILE"
}

# Main
main() {
    log "Starting backup monitoring..."
    generate_metrics
    log "Backup monitoring completed"
}

# Run
main "$@"
