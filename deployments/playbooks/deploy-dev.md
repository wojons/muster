# Development Environment Deployment Playbook

**Environment**: Development  
**Purpose**: Local development, feature testing  
**Frequency**: As needed  
**Downtime**: None (rolling updates)

## Prerequisites

- [ ] Kubernetes cluster access (minikube/kind/k3d)
- [ ] kubectl configured
- [ ] Docker running
- [ ] Go 1.25+ installed

## Deployment Steps

### 1. Build Local Binary

```bash
cd /Users/lexykwaii/Code/muster
make build
```

### 2. Run Unit Tests

```bash
make test-unit
```

### 3. Start Local Environment

```bash
# Using minikube
minikube start
kubectl config use-context minikube

# Or using kind
kind create cluster --name muster-dev
kubectl config use-context kind-muster-dev
```

### 4. Deploy to Development

```bash
# Using the deployment script
ENVIRONMENT=dev ./deployments/scripts/deploy-muster.sh \
    --environment dev \
    --version latest \
    --skip-tests

# Or manually apply manifests
kubectl apply -k k8s/overlays/dev/
```

### 5. Verify Deployment

```bash
# Check pods
kubectl get pods -n muster

# Check services
kubectl get svc -n muster

# Check logs
kubectl logs -n muster -l app=muster --tail=100

# Port forward for local testing
kubectl port-forward -n muster svc/muster 8080:8080
```

### 6. Run Integration Tests

```bash
make test-integration
```

## Troubleshooting

### Pod not starting

```bash
kubectl describe pod -n muster <pod-name>
kubectl logs -n muster <pod-name> --previous
```

### Service unreachable

```bash
kubectl get endpoints -n muster
kubectl port-forward -n muster svc/muster 8080:8080
```

### Database issues

```bash
kubectl exec -n muster -it <pod-name> -- sh
# Check database
sqlite3 /var/lib/muster/muster.db ".tables"
```

## Rollback

```bash
# If deployment failed
kubectl rollout undo deployment/muster -n muster

# Or use script
./deployments/scripts/deploy-muster.sh --environment dev --rollback
```

## Clean Up

```bash
# Delete deployment
kubectl delete -k k8s/overlays/dev/

# Stop minikube
minikube stop
```

## Notes

- Development environment uses ephemeral storage
- No backups are created automatically
- Debug logging is enabled by default
- Resource limits are relaxed
- Hot-reload is available via `make dev`

## Post-Deployment Checklist

- [ ] All pods running
- [ ] Health endpoint responding
- [ ] Can execute sample commands
- [ ] Logs show no errors
- [ ] Can connect to database

## Related Documentation

- [OPS-DEPLOYMENT.md](../../specs/OPS-DEPLOYMENT.md)
- [README.md](../README.md)
- [../environments/dev/](../environments/dev/)
