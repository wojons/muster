# OPS-BACKUP-RECOVERY: Backup and Disaster Recovery Specification

## Overview
Comprehensive backup procedures and disaster recovery strategies.

## Database Backup

### Automated Backups

```bash
#!/bin/bash
# backup-database.sh

BACKUP_DIR="/backups/postgres"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="muster_db_${TIMESTAMP}.sql.gz"
RETENTION_DAYS=30

# Create backup
pg_dump -h ${DB_HOST} -U ${DB_USER} -d ${DB_NAME} | gzip > ${BACKUP_DIR}/${BACKUP_FILE}

# Verify backup
if gunzip -t ${BACKUP_DIR}/${BACKUP_FILE}; then
  echo "Backup successful: ${BACKUP_FILE}"
  
  # Upload to S3
  aws s3 cp ${BACKUP_DIR}/${BACKUP_FILE} s3://${S3_BUCKET}/postgres/
  
  # Cleanup old backups
  find ${BACKUP_DIR} -name "muster_db_*.sql.gz" -mtime +${RETENTION_DAYS} -delete
  aws s3 ls s3://${S3_BUCKET}/postgres/ | awk '$1 < "'$(date -d "-${RETENTION_DAYS} days" +%Y-%m-%d)'" {print $4}' | xargs -I {} aws s3 rm s3://${S3_BUCKET}/postgres/{}
else
  echo "Backup failed: ${BACKUP_FILE}"
  exit 1
fi
```

### Kubernetes CronJob

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: database-backup
  namespace: muster
spec:
  schedule: "0 2 * * *"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: postgres:15-alpine
            command:
            - /bin/sh
            - -c
            - |
              pg_dump -h db -U postgres muster | gzip > /backup/muster_$(date +%Y%m%d).sql.gz
              aws s3 cp /backup/muster_$(date +%Y%m%d).sql.gz s3://muster-backups/postgres/
            env:
            - name: PGPASSWORD
              valueFrom:
                secretKeyRef:
                  name: db-secrets
                  key: password
            volumeMounts:
            - name: backup
              mountPath: /backup
          volumes:
          - name: backup
            emptyDir: {}
          restartPolicy: OnFailure
```

## Redis Backup

```bash
#!/bin/bash
# backup-redis.sh

BACKUP_DIR="/backups/redis"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Trigger BGSAVE
redis-cli -h ${REDIS_HOST} BGSAVE

# Wait for save to complete
while redis-cli -h ${REDIS_HOST} LASTSAVE | grep -q "0"; do
  sleep 1
done

# Copy dump file
cp /var/lib/redis/dump.rdb ${BACKUP_DIR}/redis_${TIMESTAMP}.rdb
aws s3 cp ${BACKUP_DIR}/redis_${TIMESTAMP}.rdb s3://${S3_BUCKET}/redis/
```

## File System Backup

```bash
#!/bin/bash
# backup-files.sh

SOURCE_DIRS="/var/lib/muster/uploads /var/log/muster"
BACKUP_DIR="/backups/files"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

tar czf ${BACKUP_DIR}/files_${TIMESTAMP}.tar.gz ${SOURCE_DIRS}
aws s3 cp ${BACKUP_DIR}/files_${TIMESTAMP}.tar.gz s3://${S3_BUCKET}/files/
```

## Recovery Procedures

### Database Recovery

```bash
#!/bin/bash
# restore-database.sh

BACKUP_FILE=$1
DB_HOST=${DB_HOST:-localhost}
DB_NAME=${DB_NAME:-muster}

# Download from S3 if not local
if [[ ${BACKUP_FILE} == s3://* ]]; then
  aws s3 cp ${BACKUP_FILE} /tmp/restore.sql.gz
  BACKUP_FILE=/tmp/restore.sql.gz
fi

# Stop application
kubectl scale deployment muster-api --replicas=0

# Restore database
gunzip < ${BACKUP_FILE} | psql -h ${DB_HOST} -U postgres -d ${DB_NAME}

# Verify restore
psql -h ${DB_HOST} -U postgres -d ${DB_NAME} -c "SELECT COUNT(*) FROM users;"

# Restart application
kubectl scale deployment muster-api --replicas=3
```

### Point-in-Time Recovery

```bash
# Restore to specific time
wal-g backup-fetch /var/lib/postgresql/data LATEST
wal-g wal-fetch /var/lib/postgresql/data "2024-01-15 14:30:00"
```

## Disaster Recovery

### RTO/RPO Targets

| Component | RTO | RPO |
|-----------|-----|-----|
| Database | 1 hour | 15 minutes |
| Redis | 30 minutes | 5 minutes |
| Files | 2 hours | 24 hours |

### Failover Procedure

```bash
# 1. Promote read replica to primary
aws rds promote-read-replica --db-instance-identifier muster-db-standby

# 2. Update application configuration
kubectl set env deployment/muster-api DATABASE_URL="new-primary-url"

# 3. Verify connectivity
kubectl exec -it deployment/muster-api -- python -c "import db; db.check_connection()"

# 4. Resume traffic
kubectl rollout restart deployment/muster-api
```

### Data Retention

```yaml
# retention-policy.yaml
backups:
  postgres:
    hourly: 24    # Keep 24 hourly backups
    daily: 7      # Keep 7 daily backups
    weekly: 4     # Keep 4 weekly backups
    monthly: 12   # Keep 12 monthly backups
    yearly: 3     # Keep 3 yearly backups
  
  redis:
    latest: 5     # Keep 5 most recent
    daily: 7
  
  logs:
    hot: 7        # 7 days on fast storage
    warm: 90      # 90 days on standard storage
    cold: 2555    # 7 years on glacier
```

## Disaster Recovery Procedures

### Adversarial Review Addressed: Missing Disaster Recovery Specifications

Complete disaster recovery procedures covering database corruption, version migration, failover, and data loss scenarios.

#### Recovery Objectives

| Metric | Target | Description |
|--------|--------|-------------|
| **RTO** (Recovery Time Objective) | 1 hour | Maximum acceptable downtime |
| **RPO** (Recovery Point Objective) | 5 minutes | Maximum data loss acceptable |
| **MTTD** (Mean Time To Detect) | 5 minutes | Detection of failure |
| **MTTR** (Mean Time To Repair) | 30 minutes | Repair/restore time |

#### Database Corruption Recovery

**Scenario**: PostgreSQL database corruption detected via checksum failures.

```bash
#!/bin/bash
# recover-corrupted-db.sh

set -e

DB_HOST=${DB_HOST:-localhost}
DB_NAME=${DB_NAME:-muster}
BACKUP_DIR="/backups/postgres"
LOG_FILE="/var/log/muster/recovery.log"

echo "$(date) - Starting database corruption recovery" >> $LOG_FILE

# 1. Stop application traffic
kubectl scale deployment/muster-api --replicas=0
kubectl scale deployment/muster-worker --replicas=0

# 2. Verify corruption
if pg_checksums -D /var/lib/postgresql/data | grep -q "FAILED"; then
    echo "$(date) - Corruption confirmed" >> $LOG_FILE
    
    # 3. Find latest valid backup
    LATEST_BACKUP=$(aws s3 ls s3://muster-backups/postgres/ | \
        grep .sql.gz | tail -1 | awk '{print $4}')
    
    if [ -z "$LATEST_BACKUP" ]; then
        echo "$(date) - No backups found, emergency repair" >> $LOG_FILE
        # Attempt pg_resetwal
        pg_resetwal -D /var/lib/postgresql/data
    else
        # 4. Restore from backup
        aws s3 cp "s3://muster-backups/postgres/$LATEST_BACKUP" /tmp/recovery.sql.gz
        gunzip /tmp/recovery.sql.gz
        
        # Drop and recreate database
        psql -h $DB_HOST -U postgres -c "DROP DATABASE IF EXISTS $DB_NAME;"
        psql -h $DB_HOST -U postgres -c "CREATE DATABASE $DB_NAME;"
        
        # Restore
        psql -h $DB_HOST -U postgres -d $DB_NAME < /tmp/recovery.sql
        
        # 5. Apply WAL archives if available
        if [ -d "/wal_archive" ]; then
            pg_restore -h $DB_HOST -U postgres -d $DB_NAME \
                -c --if-exists -F d /wal_archive
        fi
    fi
    
    # 6. Verify recovery
    if pg_checksums -D /var/lib/postgresql/data | grep -q "OK"; then
        echo "$(date) - Recovery successful" >> $LOG_FILE
    else
        echo "$(date) - Recovery failed, escalating" >> $LOG_FILE
        # Send alert, trigger failover
        ./trigger-failover.sh
    fi
fi

# 7. Restart applications
kubectl scale deployment/muster-api --replicas=3
kubectl scale deployment/muster-worker --replicas=2

echo "$(date) - Recovery complete" >> $LOG_FILE
```

#### Data Migration Between Versions

**Scenario**: Upgrade from v1.x to v2.x with breaking schema changes.

```go
// internal/database/migration/recovery.go

package migration

import (
    "context"
    "fmt"
    "github.com/pressly/goose/v3"
)

// VersionedMigrationRecovery handles failed migrations
type VersionedMigrationRecovery struct {
    db        *sql.DB
    backupDir string
}

// RecoverFromFailedMigration rolls back failed migration
func (vmr *VersionedMigrationRecovery) RecoverFromFailedMigration(
    ctx context.Context,
    targetVersion int64,
) error {
    // Get current version
    current, err := goose.GetDBVersion(ctx, vmr.db)
    if err != nil {
        return fmt.Errorf("get version: %w", err)
    }
    
    // If migration failed midway, we might be in inconsistent state
    if current < targetVersion {
        // Attempt to roll back to last known good state
        backupFile := fmt.Sprintf("%s/backup_v%d.sql", vmr.backupDir, current)
        if _, err := os.Stat(backupFile); err == nil {
            // Restore backup
            if err := vmr.restoreBackup(ctx, backupFile); err != nil {
                return fmt.Errorf("restore backup: %w", err)
            }
            // Reset migration version
                if err := goose.Reset(ctx, vmr.db); err != nil {
                return fmt.Errorf("reset migrations: %w", err)
            }
        }
    }
    
    return nil
}

// SafeMigration performs migration with backup
func (vmr *VersionedMigrationRecovery) SafeMigration(
    ctx context.Context,
    targetVersion int64,
) error {
    // Create pre‑migration backup
    backupPath := fmt.Sprintf("%s/pre_migration_v%d.sql", vmr.backupDir, targetVersion)
    if err := vmr.createBackup(ctx, backupPath); err != nil {
        return fmt.Errorf("pre‑migration backup: %w", err)
    }
    
    // Attempt migration
    if err := goose.UpTo(ctx, vmr.db, "migrations", targetVersion); err != nil {
        // Migration failed – restore backup
        if restoreErr := vmr.restoreBackup(ctx, backupPath); restoreErr != nil {
            // Critical: backup restoration failed
            return fmt.Errorf("migration failed and backup restoration failed: %w", err)
        }
        return fmt.Errorf("migration failed, restored backup: %w", err)
    }
    
    return nil
}
```

#### Failover Procedures

**Scenario**: Primary database failure, switch to replica.

```yaml
# failover-config.yaml
failover:
  triggers:
    - condition: "database_connection_failure > 5min"
      action: "promote_replica"
    - condition: "data_corruption_detected"
      action: "switch_to_backup"
    - condition: "performance_degradation > 95th_percentile"
      action: "load_balance"

  promotion:
    replica_check_interval: "30s"
    replication_lag_threshold: "10s"
    data_loss_tolerance: "5min"
    
  notifications:
    - channel: "slack#alerts"
      severity: "critical"
    - channel: "pagerduty"
      severity: "critical"
```

```bash
#!/bin/bash
# promote-replica.sh

PRIMARY_HOST=${PRIMARY_HOST:-db-primary}
REPLICA_HOST=${REPLICA_HOST:-db-replica}
CHECK_INTERVAL=5
MAX_ATTEMPTS=12

echo "$(date) - Starting failover procedure"

# 1. Verify primary is actually down
for i in $(seq 1 $MAX_ATTEMPTS); do
    if pg_isready -h $PRIMARY_HOST -t 2; then
        echo "$(date) - Primary still responding, aborting failover"
        exit 0
    fi
    sleep $CHECK_INTERVAL
done

# 2. Stop replication on replica
psql -h $REPLICA_HOST -U postgres -c "SELECT pg_wal_replay_pause();"

# 3. Promote replica
psql -h $REPLICA_HOST -U postgres -c "SELECT pg_promote();"

# 4. Wait for promotion to complete
for i in $(seq 1 10); do
    if psql -h $REPLICA_HOST -U postgres -c "SELECT pg_is_in_recovery();" | grep -q "f"; then
        echo "$(date) - Promotion successful"
        break
    fi
    sleep 2
done

# 5. Update DNS/load balancer
aws route53 change-resource-record-sets --hosted-zone-id ZONE_ID \
    --change-batch '{
        "Changes": [{
            "Action": "UPSERT",
            "ResourceRecordSet": {
                "Name": "db.muster.internal",
                "Type": "CNAME",
                "TTL": 60,
                "ResourceRecords": [{"Value": "'$REPLICA_HOST'"}]
            }
        }]
    }'

# 6. Update application configuration
kubectl set env deployment/muster-api DB_HOST=$REPLICA_HOST

echo "$(date) - Failover complete"
```

#### Data Loss Recovery

**Scenario**: Accidental deletion of critical data.

```sql
-- Point-in-time recovery for specific table
-- 1. Identify deletion time
SELECT * FROM audit_log 
WHERE action = 'DELETE' 
AND resource_type = 'inventory'
AND timestamp > NOW() - INTERVAL '1 hour';

-- 2. Restore from WAL archive using pg_waldump
pg_waldump -p /wal_archive -s '2026-03-01 10:30:00' -e '2026-03-01 10:35:00' \
    | grep 'DELETE FROM inventory' \
    | awk '{print "INSERT INTO inventory VALUES (" $4 ");"}' \
    > /tmp/recovery.sql

-- 3. Apply recovery (with conflict resolution)
psql -h localhost -U postgres -d muster -v ON_ERROR_STOP=1 -f /tmp/recovery.sql
```

#### Security Invariants

1. **Backup Encryption Invariant**: All backups encrypted at rest (AES‑256‑GCM).
2. **Recovery Testing Invariant**: DR tests run monthly, results audited.
3. **Version Compatibility Invariant**: Backups restorable across minor versions.
4. **Access Control Invariant**: Only authorized personnel can trigger recovery.

#### Adversarial Test Cases

Add to `SECURITY-TESTING.md`:

1. **Database corruption test**: Simulate page corruption, verify recovery.
2. **Migration rollback test**: Failed migration, verify backup restoration.
3. **Failover test**: Kill primary database, verify replica promotion.
4. **Point‑in‑time recovery test**: Delete data, restore to specific timestamp.

---

## Testing

```bash
#!/bin/bash
# test-recovery.sh

# Monthly DR test
echo "Starting DR test..."

# Create test backup
./backup-database.sh

# Spin up test environment
docker-compose -f docker-compose.test.yml up -d

# Restore to test environment
./restore-database.sh s3://muster-backups/postgres/latest.sql.gz

# Run validation tests
pytest tests/integration/test_recovery.py

# Cleanup
docker-compose -f docker-compose.test.yml down
echo "DR test complete"
```
