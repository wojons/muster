# Staging Environment Configuration

**Environment**: Staging  
**Namespace**: muster-staging  
**Domain**: muster-staging.example.com

## Overview

The staging environment is designed for:
- Pre-production testing
- QA validation
- Integration testing
- Performance baseline

## Configuration

### Resource Limits

```yaml
resources:
  requests:
    cpu: 250m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 1Gi
replicas: 2
```

### Database

- **Type**: PostgreSQL (shared RDS)
- **Instance**: staging-db.example.com
- **Backup**: Daily automated
- **Retention**: 7 days

### Features

- Production-like configuration
- Real database (PostgreSQL)
- Authentication enabled
- Monitoring enabled
- Rate limiting enabled

## Deployment

See [../playbooks/deploy-staging.md](../playbooks/deploy-staging.md)

## Access

- **Web UI**: https://muster-staging.example.com
- **API**: https://muster-staging.example.com/api/v1
- **Health**: https://muster-staging.example.com/health

## Quick Commands

```bash
# Check status
kubectl get pods -n muster-staging

# View logs
kubectl logs -n muster-staging -l app=muster --tail=100

# Run smoke tests
curl -sf https://muster-staging.example.com/health
curl -sf https://muster-staging.example.com/api/v1/status
```

## Monitoring

- **Grafana**: https://grafana.example.com/d/muster-staging
- **Prometheus**: https://prometheus-staging.example.com
- **Logs**: https://logs-staging.example.com
