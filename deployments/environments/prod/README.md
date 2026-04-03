# Production Environment Configuration

**Environment**: Production  
**Namespace**: muster-prod  
**Domain**: muster.example.com

## Overview

The production environment serves live customer traffic.

## Configuration

### Resource Limits

```yaml
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 2Gi
replicas: 3
minReplicas: 3
maxReplicas: 10
```

### Database

- **Type**: PostgreSQL (dedicated RDS)
- **Instance**: prod-db.example.com
- **Backup**: Hourly automated + continuous streaming
- **Retention**: 30 days
- **Multi-AZ**: Yes

### Features

- Full production configuration
- High availability
- Auto-scaling enabled
- Comprehensive monitoring
- Security hardening
- DDoS protection

## Deployment

See [../playbooks/deploy-prod.md](../playbooks/deploy-prod.md)

**IMPORTANT**: All production deployments require:
1. Change management ticket
2. QA sign-off
3. Database backup
4. Rollback plan

## Access

- **Web UI**: https://muster.example.com
- **API**: https://muster.example.com/api/v1
- **Health**: https://muster.example.com/health
- **Metrics**: https://muster.example.com/metrics

## Quick Commands

```bash
# Check status
kubectl get pods -n muster-prod

# View logs (last 100 lines)
kubectl logs -n muster-prod -l app=muster --tail=100

# Check HPA status
kubectl get hpa -n muster-prod

# Check resource usage
kubectl top pods -n muster-prod
```

## Monitoring

- **Grafana**: https://grafana.example.com/d/muster-overview
- **PagerDuty**: https://pagerduty.example.com
- **Logs**: https://logs.example.com

## Emergency Procedures

### Immediate Actions

1. Check #incidents channel
2. Verify on-call engineer notified
3. Assess impact
4. Execute rollback if necessary

### Contacts

- **On-call**: [phone]
- **Slack**: #incidents
- **Escalation**: [manager]

## Compliance

- SOC 2 Type II: ✓
- GDPR: ✓
- Change Management: ✓
- Backup/Recovery: ✓
