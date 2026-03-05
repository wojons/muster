# openapi-cli

A Helm chart for OpenAPI CLI + MCP Server.

## Installation

Add this Helm repository:

```bash
helm repo add openapi-cli https://charts.openapi-cli.io
helm repo update
```

Install the chart:

```bash
helm install openapi-cli openapi-cli/openapi-cli --namespace openapi-cli
```

## Configuration

See `values.yaml` for all available configuration options.

### Scaling

```bash
helm upgrade openapi-cli openapi-cli/openapi-cli \
  --set replicaCount=5 \
  --set autoscaling.maxReplicas=20
```

### Ingress

```yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
  hosts:
    - host: api.openapi-cli.io
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: openapi-cli-tls
      hosts:
        - api.openapi-cli.io
```

## Development

Install dependencies:

```bash
helm dependency build
```

Lint the chart:

```bash
helm lint .
```

Template test:

```bash
helm template openapi-cli . --namespace openapi-cli
```

Install locally:

```bash
helm install openapi-cli . --namespace openapi-cli
```

## Testing

The Helm chart includes tests for:
- Pod deployment
- Service availability
- Configuration injection

Run tests:

```bash
helm test openapi-cli --namespace openapi-cli
```

## Metrics and Monitoring

The chart exports Prometheus metrics on port 8080 at `/metrics`. Metrics include:
- Request latency
- Request count by endpoint
- Error rates
- Cache hit/miss ratios

## Security

The chart implements:
- Non-root user (UID 65532)
- Read-only root filesystem
- Dropped all capabilities
- Network policies for ingress/egress
- RBAC with least privileges

## Troubleshooting

Check pod logs:

```bash
kubectl logs -n openapi-cli -l app.kubernetes.io/name=openapi-cli
```

Port forward for local testing:

```bash
kubectl port-forward -n openapi-cli svc/openapi-cli 8080:80
```

## License

Apache License 2.0
