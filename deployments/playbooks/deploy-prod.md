# Production Environment Deployment Playbook

**Environment**: Production  
**Purpose**: Live customer traffic  
**Frequency**: Scheduled releases, hotfixes  
**Downtime**: Rolling update (< 1 minute, zero-downtime)

## Prerequisites

- [ ] Access to production Kubernetes cluster
- [ ] kubectl configured with production context
- [ ] Release approved in release management system
- [ ] QA sign-off from staging
- [ ] Database migration plan approved
- [ ] Rollback plan documented
- [ ] Change management ticket approved

## Pre-Deployment Checklist

### 24 Hours Before

- [ ] Create deployment ticket
- [ ] Announce deployment window
- [ ] Review rollback plan
- [ ] Verify staging deployment is stable
- [ ] Check production resource capacity
- [ ] Verify backup systems operational

### 1 Hour Before

- [ ] Verify on-call engineer is available
- [ ] Check monitoring dashboards
- [ ] Confirm deployment team in #deployments channel
- [ ] Pre-stage Docker images
- [ ] Run pre-deployment health check

### Just Before

- [ ] Check error rate baseline
- [ ] Verify database backup completed
- [ ] Set maintenance window (if applicable)
- [ ] Enable heightened monitoring

## Deployment Steps

### 1. Announce Deployment Start

```bash
# Post in #deployments
# Send pagerduty notification
```

**Message Template**:
```
🔴 PRODUCTION DEPLOYMENT STARTING
Service: Muster
Version: v1.2.3
Start: 2026-04-02 14:00 UTC
Expected Duration: 10 minutes
Change Ticket: CHG-12345
On-call: @engineer-name
```

### 2. Create Database Backup

```bash
# Critical: Always backup before production deploy
ENVIRONMENT=prod ./deployments/backup/backup-database.sh

# Verify backup integrity
./deployments/backup/verify-backup.sh
```

### 3. Enable Maintenance Mode (Optional)

```bash
# If breaking changes, enable maintenance
kubectl apply -f k8s/base/maintenance-mode.yaml

# Wait for propagation
sleep 30
```

### 4. Deploy to Production

```bash
# Deploy with strict verification
ENVIRONMENT=prod ./deployments/scripts/deploy-muster.sh \
    --environment prod \
    --version v1.2.3 \
    --namespace muster-prod
```

**What the script does**:
1. Preflight checks
2. Runs tests
3. Creates backup
4. Applies Kubernetes manifests
5. Waits for rollout
6. Verifies deployment
7. Runs post-deployment tests

### 5. Monitor Rollout

```bash
# Watch rollout in real-time
kubectl rollout status deployment/muster -n muster-prod --timeout=10m

# Watch pods
kubectl get pods -n muster-prod -l app=muster -w

# Check for errors
kubectl logs -n muster-prod -l app=muster --tail=100 | grep -i error
```

### 6. Run Health Checks

```bash
# Health endpoint
curl -sf https://muster.example.com/health

# Ready check
curl -sf https://muster.example.com/ready

# Liveness check
curl -sf https://muster.example.com/live
```

### 7. Disable Maintenance Mode

```bash
# If enabled
kubectl delete -f k8s/base/maintenance-mode.yaml
```

## Verification Checklist

### Immediate (0-5 minutes)

- [ ] All pods running (replicas: 3)
- [ ] Health checks passing
- [ ] No crash loops
- [ ] Database migrations successful

### Short-term (5-30 minutes)

- [ ] Traffic flowing normally
- [ ] Error rate < 0.1%
- [ ] Response time P95 < 200ms
- [ ] CPU usage normal
- [ ] Memory usage normal
- [ ] No new alerts firing

### Medium-term (30-60 minutes)

- [ ] All customer workflows working
- [ ] No increase in support tickets
- [ ] Metrics stable
- [ ] Logs show no anomalies

## Monitoring Queries

### Grafana Dashboards

- **Overview**: https://grafana.example.com/d/muster-overview
- **Errors**: https://grafana.example.com/d/muster-errors
- **Performance**: https://grafana.example.com/d/muster-performance

### Useful Commands

```bash
# Check error rate
kubectl logs -n muster-prod -l app=muster --since=5m | grep ERROR | wc -l

# Check response time
curl -w "@curl-format.txt" -sf -o /dev/null https://muster.example.com/health

# Check resource usage
kubectl top pods -n muster-prod

# Check metrics
kubectl exec -n muster-prod -it <pod> -- curl localhost:9090/metrics
```

## Rollback Procedures

### Automatic Rollback Triggers

- Error rate > 1%
- P95 latency > 500ms
- Health checks failing for > 2 minutes
- Database connection failures

### Manual Rollback

```bash
# Emergency rollback
kubectl rollout undo deployment/muster -n muster-prod

# Or use script
ENVIRONMENT=prod ./deployments/scripts/deploy-muster.sh --rollback

# Monitor rollback
kubectl rollout status deployment/muster -n muster-prod
```

### Database Rollback

```bash
# If database migration needed rollback
./deployments/backup/restore-database.sh \
    --backup-id backup-20260402-140000 \
    --target-db muster-prod
```

## Troubleshooting

### High Error Rate

1. Check logs: `kubectl logs -n muster-prod -l app=muster --tail=500`
2. Check metrics: Grafana error dashboard
3. Check database: Connection pool, slow queries
4. Consider rollback if > 5% errors

### Database Issues

```bash
# Check connection pool
kubectl exec -n muster-prod -it <pod> -- sh
psql $DATABASE_URL -c "SELECT count(*) FROM pg_stat_activity;"

# Check migration status
psql $DATABASE_URL -c "SELECT * FROM schema_migrations;"
```

### Performance Degradation

```bash
# Profile CPU
curl -sf http://muster-prod:6060/debug/pprof/cpu > cpu.prof

# Profile memory
curl -sf http://muster-prod:6060/debug/pprof/heap > heap.prof
```

## Post-Deployment Tasks

### Immediate

- [ ] Disable maintenance window
- [ ] Update deployment tracker
- [ ] Close change ticket
- [ ] Send completion notification

### Within 1 Hour

- [ ] Review metrics
- [ ] Check for anomalies
- [ ] Update runbook if issues found
- [ ] Document lessons learned

### Within 24 Hours

- [ ] Monitor customer feedback
- [ ] Review support tickets
- [ ] Generate deployment report
- [ ] Archive deployment logs

## Notification Templates

**Success**:
```
✅ PRODUCTION DEPLOYMENT COMPLETE
Service: Muster
Version: v1.2.3
Duration: 8 minutes
Status: All systems operational
Metrics: Normal
Error Rate: 0.02%
```

**Rollback**:
```
🚨 PRODUCTION ROLLBACK EXECUTED
Service: Muster
From: v1.2.3
To: v1.2.2
Reason: [critical issue description]
Action Required: [if any]
```

## Emergency Contacts

- **On-call Engineer**: [phone/slack]
- **Platform Team**: #platform-team
- **SRE Team**: #sre
- **Escalation**: [manager phone]

## Related Documentation

- [OPS-DEPLOYMENT.md](../../specs/OPS-DEPLOYMENT.md)
- [../environments/prod/](../environments/prod/)
- [Rollback Script](../scripts/rollback.sh)
- [Incident Response](../../docs/incident-response.md)

## Compliance

- SOC 2 Type II: All deployments logged and auditable
- Change Management: Required ticket for all production changes
- Backup/Recovery: Database backed up before every deployment
- Rollback: Documented and tested procedure available
