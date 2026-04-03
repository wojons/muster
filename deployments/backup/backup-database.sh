#!/bin/bash
# backup-database.sh - Automated database backup script for Muster
# Reference: specs/OPS-BACKUP-RECOVERY.md

set -euo pipefail

# Configuration
BACKUP_DIR="${BACKUP_DIR:-/var/backups/muster}"
RETENTION_DAYS="${RETENTION_DAYS:-30}"
DB_PATH="${DB_PATH:-/var/lib/muster/muster.db}"
ENABLE_COMPRESSION="${ENABLE_COMPRESSION:-true}"
ENABLE_ENCRYPTION="${ENABLE_ENCRYPTION:-false}"
ENCRYPTION_PASSWORD="${ENCRYPTION_PASSWORD:-}"
CLOUD_PROVIDER="${CLOUD_PROVIDER:-}"  # s3, gcs, azure
CLOUD_BUCKET="${CLOUD_BUCKET:-}"
CLOUD_PREFIX="${CLOUD_PREFIX:-backups}"
NOTIFY_ON_SUCCESS="${NOTIFY_ON_SUCCESS:-false}"
NOTIFY_ON_FAILURE="${NOTIFY_ON_FAILURE:-true}"
SLACK_WEBHOOK="${SLACK_WEBHOOK:-}"
EMAIL_TO="${EMAIL_TO:-}"

# Logging
LOG_FILE="${BACKUP_DIR}/backup.log"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="muster_backup_${TIMESTAMP}.db"
BACKUP_PATH="${BACKUP_DIR}/${BACKUP_FILE}"

# Functions
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"
}

error() {
    log "ERROR: $*"
    notify_failure "$*"
    exit 1
}

notify_success() {
    local message="Backup completed successfully: ${BACKUP_FILE}"
    log "$message"
    
    if [[ "$NOTIFY_ON_SUCCESS" == "true" && -n "$SLACK_WEBHOOK" ]]; then
        curl -s -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"✅ ${message}\",\"channel\":\"#alerts\"}" \
            "$SLACK_WEBHOOK" || true
    fi
    
    if [[ "$NOTIFY_ON_SUCCESS" == "true" && -n "$EMAIL_TO" ]]; then
        echo "$message" | mail -s "Muster Backup Success" "$EMAIL_TO" || true
    fi
}

notify_failure() {
    local message="Backup failed: $*"
    log "$message"
    
    if [[ "$NOTIFY_ON_FAILURE" == "true" && -n "$SLACK_WEBHOOK" ]]; then
        curl -s -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"🔥 ${message}\",\"channel\":\"#alerts\",\"priority\":\"high\"}" \
            "$SLACK_WEBHOOK" || true
    fi
    
    if [[ "$NOTIFY_ON_FAILURE" == "true" && -n "$EMAIL_TO" ]]; then
        echo "$message" | mail -s "Muster Backup FAILED" "$EMAIL_TO" || true
    fi
}

verify_backup() {
    local backup_file="$1"
    log "Verifying backup integrity..."
    
    if [[ ! -f "$backup_file" ]]; then
        error "Backup file not found: $backup_file"
    fi
    
    # Check file size
    local size
    size=$(stat -f%z "$backup_file" 2>/dev/null || stat -c%s "$backup_file")
    if [[ "$size" -lt 1024 ]]; then
        error "Backup file is too small (${size} bytes), may be corrupted"
    fi
    
    # For SQLite, verify header
    if [[ "$ENABLE_ENCRYPTION" != "true" ]]; then
        local header
        header=$(head -c 16 "$backup_file" | cat -v)
        if [[ "$header" != *"SQLite"* ]]; then
            error "Backup file does not have valid SQLite header"
        fi
    fi
    
    # Calculate checksum
    local checksum
    if command -v sha256sum &> /dev/null; then
        checksum=$(sha256sum "$backup_file" | awk '{print $1}')
    else
        checksum=$(shasum -a 256 "$backup_file" | awk '{print $1}')
    fi
    
    log "Backup checksum: ${checksum}"
    echo "$checksum" > "${backup_file}.sha256"
    
    # Test restore to temporary database
    local test_db="${BACKUP_DIR}/.verify_${TIMESTAMP}.db"
    if command -v sqlite3 &> /dev/null && [[ "$ENABLE_ENCRYPTION" != "true" ]]; then
        if sqlite3 "$backup_file" ".tables" > /dev/null 2>&1; then
            log "Backup is valid SQLite database"
        else
            error "Backup file is not a valid SQLite database"
        fi
    fi
    
    rm -f "$test_db"
    log "Backup verification passed"
}

upload_to_cloud() {
    local file="$1"
    local filename
    filename=$(basename "$file")
    
    if [[ -z "$CLOUD_PROVIDER" || -z "$CLOUD_BUCKET" ]]; then
        return 0
    fi
    
    log "Uploading to cloud storage (${CLOUD_PROVIDER})..."
    
    case "$CLOUD_PROVIDER" in
        s3)
            if command -v aws &> /dev/null; then
                aws s3 cp "$file" "s3://${CLOUD_BUCKET}/${CLOUD_PREFIX}/${filename}" \
                    --storage-class STANDARD_IA
                aws s3 cp "${file}.sha256" "s3://${CLOUD_BUCKET}/${CLOUD_PREFIX}/${filename}.sha256" \
                    --storage-class STANDARD_IA
            else
                log "Warning: AWS CLI not found, skipping cloud upload"
            fi
            ;;
        gcs)
            if command -v gsutil &> /dev/null; then
                gsutil cp "$file" "gs://${CLOUD_BUCKET}/${CLOUD_PREFIX}/${filename}"
                gsutil cp "${file}.sha256" "gs://${CLOUD_BUCKET}/${CLOUD_PREFIX}/${filename}.sha256"
            else
                log "Warning: gsutil not found, skipping cloud upload"
            fi
            ;;
        azure)
            if command -v az &> /dev/null; then
                az storage blob upload \
                    --file "$file" \
                    --container-name "$CLOUD_BUCKET" \
                    --name "${CLOUD_PREFIX}/${filename}"
            else
                log "Warning: Azure CLI not found, skipping cloud upload"
            fi
            ;;
        *)
            log "Warning: Unknown cloud provider: $CLOUD_PROVIDER"
            ;;
    esac
    
    log "Cloud upload completed"
}

cleanup_old_backups() {
    log "Cleaning up backups older than ${RETENTION_DAYS} days..."
    
    # Local cleanup
    find "$BACKUP_DIR" -name "muster_backup_*.db*" -mtime +"$RETENTION_DAYS" -type f -delete
    find "$BACKUP_DIR" -name "muster_backup_*.sha256" -mtime +"$RETENTION_DAYS" -type f -delete
    
    # Cloud cleanup
    if [[ -n "$CLOUD_PROVIDER" && -n "$CLOUD_BUCKET" ]]; then
        local cutoff_date
        cutoff_date=$(date -d "-${RETENTION_DAYS} days" +%Y-%m-%d 2>/dev/null || \
                      date -v "-${RETENTION_DAYS}d" +%Y-%m-%d)
        
        case "$CLOUD_PROVIDER" in
            s3)
                aws s3 ls "s3://${CLOUD_BUCKET}/${CLOUD_PREFIX}/" | \
                    awk -v date="$cutoff_date" '$1 < date {print $4}' | \
                    xargs -I {} aws s3 rm "s3://${CLOUD_BUCKET}/${CLOUD_PREFIX}/{}"
                ;;
            gcs)
                gsutil ls "gs://${CLOUD_BUCKET}/${CLOUD_PREFIX}/" | \
                    while read -r obj; do
                        gsutil stat "$obj" | grep -q "$cutoff_date" || gsutil rm "$obj"
                    done
                ;;
        esac
    fi
    
    log "Cleanup completed"
}

# Main backup process
main() {
    log "Starting backup process..."
    
    # Create backup directory
    mkdir -p "$BACKUP_DIR"
    
    # Check if source database exists
    if [[ ! -f "$DB_PATH" ]]; then
        error "Database not found: $DB_PATH"
    fi
    
    log "Source database: $DB_PATH"
    log "Backup destination: $BACKUP_PATH"
    
    # Perform backup using SQLite VACUUM INTO
    log "Creating backup..."
    if command -v sqlite3 &> /dev/null; then
        sqlite3 "$DB_PATH" "VACUUM INTO '${BACKUP_PATH}'"
    else
        cp "$DB_PATH" "$BACKUP_PATH"
    fi
    
    # Compress if enabled
    if [[ "$ENABLE_COMPRESSION" == "true" ]]; then
        log "Compressing backup..."
        gzip "$BACKUP_PATH"
        BACKUP_PATH="${BACKUP_PATH}.gz"
        BACKUP_FILE="${BACKUP_FILE}.gz"
    fi
    
    # Encrypt if enabled
    if [[ "$ENABLE_ENCRYPTION" == "true" ]]; then
        if [[ -z "$ENCRYPTION_PASSWORD" ]]; then
            error "Encryption enabled but no password provided"
        fi
        log "Encrypting backup..."
        
        # Encrypt using openssl
        openssl enc -aes-256-cbc -salt -pbkdf2 \
            -in "$BACKUP_PATH" \
            -out "${BACKUP_PATH}.enc" \
            -pass pass:"$ENCRYPTION_PASSWORD"
        
        rm "$BACKUP_PATH"
        BACKUP_PATH="${BACKUP_PATH}.enc"
        BACKUP_FILE="${BACKUP_FILE}.enc"
    fi
    
    # Verify backup
    verify_backup "$BACKUP_PATH"
    
    # Upload to cloud
    upload_to_cloud "$BACKUP_PATH"
    
    # Cleanup old backups
    cleanup_old_backups
    
    # Update metrics
    local backup_size
    backup_size=$(stat -f%z "$BACKUP_PATH" 2>/dev/null || stat -c%s "$BACKUP_PATH")
    log "Backup size: $backup_size bytes"
    
    # Success notification
    notify_success
    
    log "Backup process completed successfully"
    
    # Output backup info for automation
    echo "BACKUP_FILE=${BACKUP_FILE}"
    echo "BACKUP_PATH=${BACKUP_PATH}"
    echo "BACKUP_SIZE=${backup_size}"
    echo "BACKUP_TIMESTAMP=${TIMESTAMP}"
}

# Run main function
main "$@"
