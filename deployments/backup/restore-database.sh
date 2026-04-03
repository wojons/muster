#!/bin/bash
# restore-database.sh - Database restoration script for Muster
# Reference: specs/OPS-BACKUP-RECOVERY.md

set -euo pipefail

# Configuration
BACKUP_DIR="${BACKUP_DIR:-/var/backups/muster}"
DB_PATH="${DB_PATH:-/var/lib/muster/muster.db}"
ENABLE_VERIFICATION="${ENABLE_VERIFICATION:-true}"
FORCE_RESTORE="${FORCE_RESTORE:-false}"
ENCRYPTION_PASSWORD="${ENCRYPTION_PASSWORD:-}"
NOTIFY_ON_SUCCESS="${NOTIFY_ON_SUCCESS:-true}"
NOTIFY_ON_FAILURE="${NOTIFY_ON_FAILURE:-true}"
SLACK_WEBHOOK="${SLACK_WEBHOOK:-}"
EMAIL_TO="${EMAIL_TO:-}"

# Logging
LOG_FILE="${BACKUP_DIR}/restore.log"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RESTORE_TEMP_DIR="${BACKUP_DIR}/.restore_temp_${TIMESTAMP}"

# Functions
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

error() {
    log "ERROR: $*"
    notify_failure "$*"
    cleanup
    exit 1
}

cleanup() {
    if [[ -d "$RESTORE_TEMP_DIR" ]]; then
        rm -rf "$RESTORE_TEMP_DIR"
    fi
}

notify_success() {
    local message="Database restored successfully from: ${RESTORE_FILE}"
    log "$message"
    
    if [[ "$NOTIFY_ON_SUCCESS" == "true" && -n "$SLACK_WEBHOOK" ]]; then
        curl -s -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"✅ ${message}\",\"channel\":\"#alerts\"}" \
            "$SLACK_WEBHOOK" || true
    fi
    
    if [[ "$NOTIFY_ON_SUCCESS" == "true" && -n "$EMAIL_TO" ]]; then
        echo "$message" | mail -s "Muster Restore Success" "$EMAIL_TO" || true
    fi
}

notify_failure() {
    local message="Restore failed: $*"
    log "$message"
    
    if [[ "$NOTIFY_ON_FAILURE" == "true" && -n "$SLACK_WEBHOOK" ]]; then
        curl -s -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"🔥 ${message}\",\"channel\":\"#alerts\",\"priority\":\"high\"}" \
            "$SLACK_WEBHOOK" || true
    fi
    
    if [[ "$NOTIFY_ON_FAILURE" == "true" && -n "$EMAIL_TO" ]]; then
        echo "$message" | mail -s "Muster Restore FAILED" "$EMAIL_TO" || true
    fi
}

verify_backup() {
    local backup_file="$1"
    
    if [[ ! -f "$backup_file" ]]; then
        error "Backup file not found: $backup_file"
    fi
    
    # Check if compressed
    if [[ "$backup_file" == *.gz ]]; then
        log "Verifying compressed backup..."
        if ! gunzip -t "$backup_file" 2>&1; then
            error "Backup file is corrupted (gzip test failed)"
        fi
    fi
    
    # Check if encrypted
    if [[ "$backup_file" == *.enc ]]; then
        log "Backup is encrypted, will verify after decryption"
        return 0
    fi
    
    # For SQLite, verify header
    local header
    if [[ "$backup_file" == *.gz ]]; then
        header=$(gunzip -c "$backup_file" 2>/dev/null | head -c 16 | cat -v)
    else
        header=$(head -c 16 "$backup_file" | cat -v)
    fi
    
    if [[ "$header" != *"SQLite"* ]]; then
        error "Backup file does not have valid SQLite header"
    fi
    
    log "Backup verification passed"
}

decrypt_backup() {
    local input_file="$1"
    local output_file="$2"
    
    if [[ -z "$ENCRYPTION_PASSWORD" ]]; then
        error "Encryption password required for encrypted backup"
    fi
    
    log "Decrypting backup..."
    openssl enc -aes-256-cbc -d -pbkdf2 \
        -in "$input_file" \
        -out "$output_file" \
        -pass pass:"$ENCRYPTION_PASSWORD" || {
        error "Failed to decrypt backup (wrong password?)"
    }
}

decompress_backup() {
    local input_file="$1"
    local output_file="$2"
    
    log "Decompressing backup..."
    gunzip -c "$input_file" > "$output_file" || {
        error "Failed to decompress backup"
    }
}

list_available_backups() {
    log "Available backups:"
    local i=1
    for backup in "$BACKUP_DIR"/muster_backup_*.db*; do
        if [[ -f "$backup" ]]; then
            local size date
            size=$(du -h "$backup" 2>/dev/null | cut -f1)
            date=$(ls -lh "$backup" 2>/dev/null | awk '{print $6, $7, $8}')
            printf "%2d. %s (%s) - %s\n" "$i" "$(basename "$backup")" "$size" "$date"
            ((i++))
        fi
    done
}

perform_restore() {
    local backup_file="$1"
    
    log "Starting restore process..."
    log "Source: $backup_file"
    log "Target: $DB_PATH"
    
    # Create temp directory
    mkdir -p "$RESTORE_TEMP_DIR"
    
    local processed_file="$backup_file"
    local temp_processed="${RESTORE_TEMP_DIR}/backup_processed.db"
    
    # Decrypt if needed
    if [[ "$processed_file" == *.enc ]]; then
        temp_processed="${temp_processed}.enc"
        decrypt_backup "$processed_file" "$temp_processed"
        processed_file="$temp_processed"
        temp_processed="${RESTORE_TEMP_DIR}/backup_decrypted.db"
    fi
    
    # Decompress if needed
    if [[ "$processed_file" == *.gz ]]; then
        temp_processed="${temp_processed}.gz"
        decompress_backup "$processed_file" "$temp_processed"
        processed_file="$temp_processed"
        temp_processed="${RESTORE_TEMP_DIR}/backup_decompressed.db"
    fi
    
    # Final path for processed file
    if [[ ! -f "$temp_processed" ]]; then
        cp "$processed_file" "$temp_processed"
        processed_file="$temp_processed"
    else
        processed_file="$temp_processed"
    fi
    
    # Verify SQLite file
    if command -v sqlite3 &> /dev/null; then
        log "Verifying database integrity..."
        if ! sqlite3 "$processed_file" "PRAGMA integrity_check;" | grep -q "ok"; then
            error "Backup database integrity check failed"
        fi
        log "Database integrity check passed"
    fi
    
    # Create target directory
    mkdir -p "$(dirname "$DB_PATH")"
    
    # Backup current database before restore
    if [[ -f "$DB_PATH" && "$FORCE_RESTORE" != "true" ]]; then
        local current_backup="${DB_PATH}.pre_restore_${TIMESTAMP}"
        log "Backing up current database to: $current_backup"
        cp "$DB_PATH" "$current_backup"
    fi
    
    # Perform restore
    log "Restoring database..."
    cp "$processed_file" "$DB_PATH"
    
    # Set proper permissions
    chmod 600 "$DB_PATH"
    
    # Cleanup temp directory
    cleanup
    
    log "Restore completed successfully"
    notify_success
}

# Usage
usage() {
    cat << EOF
Muster Database Restore Script

Usage: $0 [OPTIONS] [BACKUP_FILE]

Options:
    -l, --list              List available backups
    -f, --force             Force restore without backup of current DB
    -p, --password PASS     Encryption password
    -d, --db-path PATH      Target database path (default: $DB_PATH)
    -b, --backup-dir DIR    Backup directory (default: $BACKUP_DIR)
    -h, --help              Show this help message

Examples:
    # List available backups
    $0 --list
    
    # Restore latest backup
    $0
    
    # Restore specific backup
    $0 /var/backups/muster/muster_backup_20240326_120000.db.gz
    
    # Restore encrypted backup
    $0 -p mypassword /var/backups/muster/muster_backup_20240326_120000.db.gz.enc

EOF
}

# Parse arguments
RESTORE_FILE=""
LIST_MODE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -l|--list)
            LIST_MODE=true
            shift
            ;;
        -f|--force)
            FORCE_RESTORE=true
            shift
            ;;
        -p|--password)
            ENCRYPTION_PASSWORD="$2"
            shift 2
            ;;
        -d|--db-path)
            DB_PATH="$2"
            shift 2
            ;;
        -b|--backup-dir)
            BACKUP_DIR="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        -*)
            echo "Unknown option: $1"
            usage
            exit 1
            ;;
        *)
            RESTORE_FILE="$1"
            shift
            ;;
    esac
done

# Main
if [[ "$LIST_MODE" == "true" ]]; then
    list_available_backups
    exit 0
fi

# Find latest backup if none specified
if [[ -z "$RESTORE_FILE" ]]; then
    log "No backup file specified, using latest..."
    RESTORE_FILE=$(ls -t "$BACKUP_DIR"/muster_backup_*.db* 2>/dev/null | head -1)
    if [[ -z "$RESTORE_FILE" ]]; then
        error "No backups found in $BACKUP_DIR"
    fi
    log "Using: $(basename "$RESTORE_FILE")"
fi

# Verify backup
if [[ "$ENABLE_VERIFICATION" == "true" ]]; then
    verify_backup "$RESTORE_FILE"
fi

# Perform restore
perform_restore "$RESTORE_FILE"

log "Database restored successfully"
echo "Restored from: $RESTORE_FILE"
echo "Restored to: $DB_PATH"
