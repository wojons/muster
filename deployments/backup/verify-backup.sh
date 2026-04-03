#!/bin/bash
# verify-backup.sh - Backup verification and testing script
# Reference: specs/OPS-BACKUP-RECOVERY.md

set -euo pipefail

# Configuration
BACKUP_DIR="${BACKUP_DIR:-/var/backups/muster}"
DB_PATH="${DB_PATH:-/var/lib/muster/muster.db}"
ENCRYPTION_PASSWORD="${ENCRYPTION_PASSWORD:-}"
TEST_DB_PREFIX="test_restore_"
MAX_RESTORE_TIME="${MAX_RESTORE_TIME:-300}"  # 5 minutes
MAX_BACKUP_AGE="${MAX_BACKUP_AGE:-86400}"   # 24 hours
NOTIFY_ON_SUCCESS="${NOTIFY_ON_SUCCESS:-true}"
NOTIFY_ON_FAILURE="${NOTIFY_ON_FAILURE:-true}"
SLACK_WEBHOOK="${SLACK_WEBHOOK:-}"
EMAIL_TO="${EMAIL_TO:-}"

# Logging
LOG_FILE="${BACKUP_DIR}/verify.log"
REPORT_FILE="${BACKUP_DIR}/verify-report-$(date +%Y%m%d_%H%M%S).json"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
TEMP_DIR=""

# Results
declare -A RESULTS
BACKUP_COUNT=0
SUCCESS_COUNT=0
FAILURE_COUNT=0
TOTAL_SIZE=0

# Functions
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

error() {
    log "ERROR: $*"
}

cleanup() {
    if [[ -n "$TEMP_DIR" && -d "$TEMP_DIR" ]]; then
        rm -rf "$TEMP_DIR"
    fi
}

trap cleanup EXIT

notify_success() {
    local message="Backup verification completed: ${SUCCESS_COUNT}/${BACKUP_COUNT} backups verified successfully"
    log "$message"
    
    if [[ "$NOTIFY_ON_SUCCESS" == "true" && -n "$SLACK_WEBHOOK" ]]; then
        curl -s -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"✅ ${message}\",\"channel\":\"#alerts\"}" \
            "$SLACK_WEBHOOK" || true
    fi
}

notify_failure() {
    local message="Backup verification FAILED: ${FAILURE_COUNT}/${BACKUP_COUNT} backups failed"
    log "$message"
    
    if [[ "$NOTIFY_ON_FAILURE" == "true" && -n "$SLACK_WEBHOOK" ]]; then
        curl -s -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"🔥 ${message}\",\"channel\":\"#alerts\",\"priority\":\"high\"}" \
            "$SLACK_WEBHOOK" || true
    fi
}

verify_single_backup() {
    local backup_file="$1"
    local filename
    filename=$(basename "$backup_file")
    
    log "Verifying: $filename"
    
    local result="PASSED"
    local errors=""
    local start_time
    start_time=$(date +%s)
    local processed_file="$backup_file"
    
    # Create temp directory for this backup
    TEMP_DIR=$(mktemp -d)
    
    # Test 1: File exists and is readable
    if [[ ! -r "$backup_file" ]]; then
        result="FAILED"
        errors="File not readable"
    fi
    
    # Test 2: File size is reasonable
    local size
    size=$(stat -c%s "$backup_file" 2>/dev/null || stat -f%z "$backup_file")
    if [[ "$size" -lt 1024 ]]; then
        result="FAILED"
        errors="${errors}; File too small (${size} bytes)"
    fi
    
    TOTAL_SIZE=$((TOTAL_SIZE + size))
    
    # Test 3: Checksum verification
    if [[ -f "${backup_file}.sha256" ]]; then
        log "  Checking checksum..."
        local expected_checksum
        local actual_checksum
        expected_checksum=$(cat "${backup_file}.sha256")
        
        if command -v sha256sum &> /dev/null; then
            actual_checksum=$(sha256sum "$backup_file" | awk '{print $1}')
        else
            actual_checksum=$(shasum -a 256 "$backup_file" | awk '{print $1}')
        fi
        
        if [[ "$expected_checksum" != "$actual_checksum" ]]; then
            result="FAILED"
            errors="${errors}; Checksum mismatch"
        fi
    else
        log "  Warning: No checksum file found"
    fi
    
    # Test 4: Decrypt if encrypted
    if [[ "$backup_file" == *.enc && "$result" == "PASSED" ]]; then
        log "  Decrypting..."
        if [[ -z "$ENCRYPTION_PASSWORD" ]]; then
            result="FAILED"
            errors="${errors}; Encryption password required"
        else
            local decrypted="${TEMP_DIR}/decrypted.db"
            if openssl enc -aes-256-cbc -d -pbkdf2 -in "$backup_file" -out "$decrypted" -pass pass:"$ENCRYPTION_PASSWORD" 2>/dev/null; then
                processed_file="$decrypted"
            else
                result="FAILED"
                errors="${errors}; Decryption failed"
            fi
        fi
    fi
    
    # Test 5: Decompress if compressed
    if [[ "$processed_file" == *.gz && "$result" == "PASSED" ]]; then
        log "  Decompressing..."
        local decompressed="${TEMP_DIR}/decompressed.db"
        if gunzip -c "$processed_file" > "$decompressed" 2>/dev/null; then
            processed_file="$decompressed"
        else
            result="FAILED"
            errors="${errors}; Decompression failed"
        fi
    fi
    
    # Test 6: SQLite integrity check
    if [[ "$result" == "PASSED" ]]; then
        log "  Checking SQLite integrity..."
        
        # Check SQLite header
        local header
        header=$(head -c 16 "$processed_file" | cat -v)
        if [[ "$header" != *"SQLite"* ]]; then
            result="FAILED"
            errors="${errors}; Not a valid SQLite database"
        fi
        
        # Full integrity check
        if command -v sqlite3 &> /dev/null; then
            if ! sqlite3 "$processed_file" "PRAGMA integrity_check;" 2>/dev/null | grep -q "ok"; then
                result="FAILED"
                errors="${errors}; SQLite integrity check failed"
            fi
        fi
    fi
    
    # Test 7: Restore test (if not skipped)
    if [[ "$result" == "PASSED" && "${SKIP_RESTORE_TEST:-}" != "true" ]]; then
        log "  Testing restore..."
        local test_db="${TEMP_DIR}/test_restored.db"
        
        local restore_start
        restore_start=$(date +%s)
        
        cp "$processed_file" "$test_db"
        
        local restore_end
        restore_end=$(date +%s)
        local restore_time=$((restore_end - restore_start))
        
        if [[ $restore_time -gt $MAX_RESTORE_TIME ]]; then
            result="WARNING"
            errors="${errors}; Restore took ${restore_time}s (max: ${MAX_RESTORE_TIME}s)"
        fi
        
        # Verify restored database
        if ! sqlite3 "$test_db" "SELECT count(*) FROM sqlite_master;" > /dev/null 2>&1; then
            result="FAILED"
            errors="${errors}; Restored database not queryable"
        fi
    fi
    
    # Calculate total time
    local end_time
    end_time=$(date +%s)
    local total_time=$((end_time - start_time))
    
    # Store results
    RESULTS["$filename,result"]="$result"
    RESULTS["$filename,size"]="$size"
    RESULTS["$filename,time"]="$total_time"
    RESULTS["$filename,errors"]="$errors"
    
    log "  Result: $result (${total_time}s)"
    
    if [[ "$result" == "FAILED" ]]; then
        ((FAILURE_COUNT++))
        log "  Errors: $errors"
    else
        ((SUCCESS_COUNT++))
    fi
    
    ((BACKUP_COUNT++))
    cleanup
}

generate_report() {
    log "Generating verification report..."
    
    local success_rate=0
    if [[ $BACKUP_COUNT -gt 0 ]]; then
        success_rate=$((SUCCESS_COUNT * 100 / BACKUP_COUNT))
    fi
    
    cat > "$REPORT_FILE" << EOF
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "total_backups": $BACKUP_COUNT,
  "successful": $SUCCESS_COUNT,
  "failed": $FAILURE_COUNT,
  "success_rate": $success_rate,
  "total_size_bytes": $TOTAL_SIZE,
  "results": [
EOF
    
    local first=true
    for key in "${!RESULTS[@]}"; do
        if [[ "$key" == *,result ]]; then
            local filename="${key%,*}"
            local result="${RESULTS[$key]}"
            local size="${RESULTS[$filename,size]}"
            local time="${RESULTS[$filename,time]}"
            local errors="${RESULTS[$filename,errors]}"
            
            if [[ "$first" == "true" ]]; then
                first=false
            else
                echo "," >> "$REPORT_FILE"
            fi
            
            cat >> "$REPORT_FILE" << EOF
    {
      "filename": "$filename",
      "result": "$result",
      "size_bytes": $size,
      "verify_time_seconds": $time,
      "errors": "$errors"
    }
EOF
        fi
    done
    
    cat >> "$REPORT_FILE" << EOF

  ]
}
EOF
    
    log "Report saved to: $REPORT_FILE"
}

# Main verification process
main() {
    log "Starting backup verification process..."
    
    # Check backup directory exists
    if [[ ! -d "$BACKUP_DIR" ]]; then
        error "Backup directory does not exist: $BACKUP_DIR"
        exit 1
    fi
    
    # Find all backups
    local backups=()
    while IFS= read -r -d '' file; do
        backups+=("$file")
    done < <(find "$BACKUP_DIR" -name "muster_backup_*.db*" -type f -print0 2>/dev/null || true)
    
    if [[ ${#backups[@]} -eq 0 ]]; then
        error "No backups found in $BACKUP_DIR"
        exit 1
    fi
    
    log "Found ${#backups[@]} backups to verify"
    
    # Verify each backup
    for backup in "${backups[@]}"; do
        verify_single_backup "$backup"
    done
    
    # Generate report
    generate_report
    
    # Summary
    log "=========================================="
    log "Verification Summary:"
    log "  Total: $BACKUP_COUNT"
    log "  Passed: $SUCCESS_COUNT"
    log "  Failed: $FAILURE_COUNT"
    log "  Total Size: $TOTAL_SIZE bytes"
    log "=========================================="
    
    if [[ $FAILURE_COUNT -gt 0 ]]; then
        notify_failure
        exit 1
    else
        notify_success
        exit 0
    fi
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -d|--backup-dir)
            BACKUP_DIR="$2"
            shift 2
            ;;
        -p|--password)
            ENCRYPTION_PASSWORD="$2"
            shift 2
            ;;
        --skip-restore)
            SKIP_RESTORE_TEST="true"
            shift
            ;;
        --max-restore-time)
            MAX_RESTORE_TIME="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  -d, --backup-dir DIR      Backup directory (default: $BACKUP_DIR)"
            echo "  -p, --password PASS       Encryption password"
            echo "  --skip-restore            Skip restore test (faster)"
            echo "  --max-restore-time SEC    Max restore time in seconds (default: 300)"
            echo "  -h, --help                Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Run main process
main
