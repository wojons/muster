# Staging Environment Deployment Playbook

**Environment**: Staging  
**Purpose**: Pre-production testing, QA validation  
**Frequency**: Per release, after dev approval  
**Downtime**: Rolling update (< 2 minutes)

## Prerequisites

- [ ] Access to staging Kubernetes cluster
- [ ] kubectl configured with staging context
- [ ] Tagged release (e.g., `v1.2.3`)
- [ ] CI/CD pipeline green
- [ ] Database migration plan reviewed

## Deployment Steps

### 1. Announce Deployment

```bash
# Send Slack notification
# Post in #deployments channel
```

**Message Template**:
```
🚀 Deploying Muster to STAGING
Version: v1.2.3
Start: 2026-04-02 14:00 UTC
Expected Duration: 5 minutes
Changelog: [link]
```

### 2. Run Full Test Suite

```bash
# Ensure all tests pass
cd /Users/lexykwaii/Code/muster
make test
make test-integration
make test-e2e
```

### 3. Create Pre-Deployment Backup

```bash
# Backup database
ENVIRONMENT=staging ./deployments/backup/backup-database.sh

# Verify backup
./deployments/backup/verify-backup.sh
```

### 4. Deploy to Staging

```bash
# Using the deployment script
ENVIRONMENT=staging ./deployments/scripts/deploy-muster.sh \
    --environment staging \
    --version v1.2.3 \
    --namespace muster-staging

# Or with dry-run first
ENVIRONMENT=staging ./deployments/scripts/deploy-muster.sh \
    --environment staging \
    --version v1.2.3 \
    --dry-run
```

### 5. Wait for Rollout

```bash
# Monitor rollout
kubectl rollout status deployment/muster -n muster-staging --timeout=5m

# Check pods
kubectl get pods -n muster-staging -w
```

### 6. Run Smoke Tests

```bash
# Health check
curl -sf https://muster-staging.example.com/health

# API smoke test
curl -sf https://muster-staging.example.com/api/v1/status

# MCP health check
curl -sf https://muster-staging.example.com/mcp/health
```

### 7. Run Integration Tests

```bash
# Run staging-specific tests
ENVIRONMENT=staging make test-staging

# Or manual verification
curl -sf https://muster-staging.example.com/api/v1/commands
```

## Verification Checklist

### Health Checks

- [ ] /health returns 200
- [ ] /ready returns 200
- [ ] /metrics endpoint accessible
- [ ] Database connection successful

### Functional Tests

- [ ] Can list APIs
- [ ] Can execute sample command
- [ ] Authentication working
- [ ] Rate limiting functioning

### Monitoring

- [ ] Metrics visible in Grafana
- [ ] No new errors in logs
- [ ] Error rate < 0.1%
- [ ] Response time < 200ms (p95)

## Troubleshooting

### Rollback Procedure

```bash
# Immediate rollback
kubectl rollout undo deployment/muster -n muster-staging

# Or use script
ENVIRONMENT=staging ./deployments/scripts/deploy-muster.sh --rollback

# Verify rollback
kubectl rollout status deployment/muster -n muster-staging
```

### Common Issues

#### Pod stuck in Pending

```bash
kubectl describe pod -n muster-staging <pod-name>
kubectl get events -n muster-staging --field-selector type=Warning
```

#### Image pull errors

```bash
# Check image exists
docker pull registry.example.com/muster:v1.2.3

# Check image pull secrets
kubectl get secret -n muster-staging
```

#### Database connection failures

```bash
# Check secret
kubectl get secret muster-secrets -n muster-staging -o jsonpath='{.data.database-url}' | base64 -d

# Check connectivity
kubectl exec -n muster-staging -it <pod-name> -- sh
curl -sf http://staging-db:5432/health
```

## Post-Deployment Tasks

1. [ ] Notify team deployment complete
2. [ ] Update deployment tracker
3. [ ] Run automated smoke tests
4. [ ] Verify monitoring dashboards
5. [ ] Mark Jira tickets as "In QA"

## Notification Template

**Success**:
```
✅ Muster v1.2.3 deployed to STAGING
Duration: 3 minutes
Status: All systems green
URL: https://muster-staging.example.com
Smoke tests: PASSED
```

**Rollback**:
```
⚠️ Muster staging deployment ROLLED BACK
Version: v1.2.3
Reason: [explain why]
Current version: v1.2.2
```

## Related Documentation

- [OPS-DEPLOYMENT.md](../../specs/OPS-DEPLOYMENT.md)
- [../environments/staging/](../environments/staging/)
- [Rollback Procedures](../scripts/rollback.sh)

## Emergency Contacts

- **On-call Engineer**: [phone/slack]
- **Platform Team**: #platform-team
- **Incidents**: #incidents
