# OPS-BACKUP-RECOVERY: Backup and Recovery Procedures

## Overview

This document provides comprehensive backup and disaster recovery procedures for Muster.

## Quick Start

```bash
# Create a backup
./deployments/backup/backup-database.sh

# Restore from latest backup
./deployments/backup/restore-database.sh

# Verify backups
./deployments/backup/verify-backup.sh

# Run backup monitor
./deployments/backup/backup-monitor.sh
```

## Scripts

### backup-database.sh

Automated database backup with compression, encryption, and cloud upload support.

**Features:**
- SQLite VACUUM INTO for consistent backups
- Optional gzip compression
- Optional AES-256 encryption
- Automatic checksum generation
- Cloud storage support (S3, GCS, Azure)
- Retention policy enforcement
- Slack/email notifications

**Environment Variables:**
- `BACKUP_DIR` - Backup directory (default: /var/backups/muster)
- `RETENTION_DAYS` - Days to retain backups (default: 30)
- `DB_PATH` - Path to SQLite database (default: /var/lib/muster/muster.db)
- `ENABLE_COMPRESSION` - Enable gzip compression (default: true)
- `ENABLE_ENCRYPTION` - Enable AES encryption (default: false)
- `ENCRYPTION_PASSWORD` - Encryption password (required if encryption enabled)
- `CLOUD_PROVIDER` - Cloud provider (s3, gcs, azure)
- `CLOUD_BUCKET` - Cloud storage bucket
- `SLACK_WEBHOOK` - Slack webhook URL for notifications

**Usage:**
```bash
# Basic backup
BACKUP_DIR=/backups DB_PATH=/data/muster.db ./backup-database.sh

# Encrypted backup
ENABLE_ENCRYPTION=true ENCRYPTION_PASSWORD=secret ./backup-database.sh

# Backup with S3 upload
CLOUD_PROVIDER=s3 CLOUD_BUCKET=my-bucket ./backup-database.sh
```

### restore-database.sh

Database restoration with integrity verification.

**Features:**
- Automatic decryption
- Automatic decompression
- SQLite integrity check
- Pre-restore backup of current database
- Notification on completion

**Usage:**
```bash
# Restore latest backup
./restore-database.sh

# Restore specific backup
./restore-database.sh /backups/muster_backup_20240326_120000.db.gz

# Restore encrypted backup
ENCRYPTION_PASSWORD=secret ./restore-database.sh -p /backups/backup.enc

# List available backups
./restore-database.sh --list
```

### verify-backup.sh

Comprehensive backup verification and testing.

**Features:**
- Checksum verification
- File size validation
- SQLite integrity check
- Restore test
- Performance testing
- JSON report generation

**Usage:**
```bash
# Verify all backups
./verify-backup.sh

# Skip restore test (faster)
./verify-backup.sh --skip-restore

# Verify with password
ENCRYPTION_PASSWORD=secret ./verify-backup.sh
```

### backup-monitor.sh

Prometheus metrics exporter for backup monitoring.

**Features:**
- Last backup timestamp
- Backup age tracking
- Stale backup detection
- Backup size metrics
- Failed backup counting

**Metrics:**
- `muster_backup_last_timestamp` - Unix timestamp of last backup
- `muster_backup_age_seconds` - Age of last backup
- `muster_backup_stale` - 1 if backup is older than threshold
- `muster_backup_total_count` - Total number of backups
- `muster_backup_failures_today` - Number of failed backups today

## Kubernetes Deployment

### Backup CronJob

```bash
# Apply backup CronJob
kubectl apply -f k8s/base/backup-cronjob.yaml

# View CronJob
kubectl get cronjob -n muster

# View backup jobs
kubectl get jobs -n muster

# View logs
kubectl logs -n muster job/muster-database-backup-xxx
```

### Configuration

**ConfigMap (backup-config):**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: backup-config
  namespace: muster
data:
  cloud-provider: s3
  cloud-bucket: muster-backups
  email-to: ops@muster.io
```

**Secrets (backup-secrets):**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: backup-secrets
  namespace: muster
type: Opaque
stringData:
  encryption-enabled: "true"
  encryption-password: "your-secret-password"
  aws-access-key-id: "AKIA..."
  aws-secret-access-key: "..."
  slack-webhook: "https://hooks.slack.com/services/..."
```

## Docker Build

```bash
# Build backup container
docker build -t muster/backup:latest deployments/backup/

# Run backup
docker run -v /data:/data -v /backups:/backups muster/backup:latest

# Run with environment
docker run \
  -e DB_PATH=/data/muster.db \
  -e BACKUP_DIR=/backups \
  -e ENABLE_COMPRESSION=true \
  -v /data:/data \
  -v /backups:/backups \
  muster/backup:latest
```

## Deployment

See [deploy-muster.sh](deploy-muster.sh) for production deployment procedures.

## Disaster Recovery

### RPO (Recovery Point Objective)
- **Target**: < 24 hours
- **Actual**: Depends on backup schedule (default: daily at 2 AM)

### RTO (Recovery Time Objective)
- **Target**: < 30 minutes
- **Actual**: ~5-10 minutes for database restore

### Recovery Procedures

1. **Database Corruption**
   ```bash
   # Stop application
   kubectl scale deployment muster --replicas=0 -n muster
   
   # Restore from backup
   ./restore-database.sh
   
   # Restart application
   kubectl scale deployment muster --replicas=3 -n muster
   ```

2. **Complete Data Loss**
   ```bash
   # Provision new storage
   kubectl delete pvc muster-data-pvc -n muster
   kubectl apply -f k8s/base/pvc.yaml
   
   # Restore from cloud backup
   aws s3 cp s3://muster-backups/backups/latest.db.gz /tmp/
   ./restore-database.sh /tmp/latest.db.gz
   ```

3. **Point-in-Time Recovery**
   ```bash
   # List available backups
   ./restore-database.sh --list
   
   # Restore specific point in time
   ./restore-database.sh /backups/muster_backup_20240326_020000.db.gz
   ```

## Monitoring

### Prometheus Alerts

```yaml
groups:
  - name: backup_alerts
    rules:
      - alert: BackupStale
        expr: muster_backup_stale == 1
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Backup is stale"
          description: "Last backup is older than 24 hours"
      
      - alert: BackupTooSmall
        expr: muster_backup_too_small == 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Backup file too small"
          description: "Backup file may be corrupted"
      
      - alert: BackupFailures
        expr: muster_backup_failures_today > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Backup failures detected"
          description: "{{ $value }} backup(s) failed today"
```

### Grafana Dashboard

Create panels for:
- Last backup timestamp
- Backup success rate
- Backup size over time
- Cloud upload latency
- Storage usage

## Testing

Run backup/restore tests:

```bash
# Run verification
cd deployments/backup
./verify-backup.sh

# Run unit tests
cd ../../
go test ./pkg/repository/... -run Backup -v
```

## Security

- Backups are encrypted at rest (AES-256-CBC)
- Checksums verify integrity
- Cloud uploads use TLS
- Access to backup files restricted to muster user

## Troubleshooting

### Backup fails with "database is locked"
- Check if database is in use
- Increase timeout in script
- Consider VACUUM INTO (uses online backup)

### Restore fails with "checksum mismatch"
- Backup may be corrupted
- Try older backup
- Check disk space

### Cloud upload fails
- Verify credentials
- Check bucket permissions
- Verify network connectivity
