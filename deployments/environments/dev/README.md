# Development Environment Configuration

**Environment**: Development  
**Namespace**: muster-dev  
**Cluster**: minikube/kind/k3d

## Overview

The development environment is designed for:
- Local feature development
- Integration testing
- Rapid iteration
- Debug-friendly settings

## Configuration

### Resource Limits

```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 512Mi
```

### Database

- **Type**: SQLite (file-based)
- **Path**: `/var/lib/muster/muster.db`
- **Backup**: Manual only
- **Retention**: Ephemeral

### Features

- Debug logging enabled
- Hot-reload available
- No authentication required
- Relaxed rate limiting
- Verbose error messages

## Deployment

See [../playbooks/deploy-dev.md](../playbooks/deploy-dev.md)

## Quick Commands

```bash
# Port forward
kubectl port-forward -n muster-dev svc/muster 8080:8080

# View logs
kubectl logs -n muster-dev -l app=muster -f

# Restart deployment
kubectl rollout restart deployment/muster -n muster-dev
```
