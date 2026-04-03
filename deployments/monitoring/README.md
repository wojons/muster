# Monitoring Stack for Muster

Complete monitoring stack with Prometheus, Grafana, and alerting.

## Overview

This directory contains the complete monitoring infrastructure for Muster, including:
- **SLOs (Service Level Objectives)** with defined targets and error budgets
- **Prometheus** configuration for metrics collection
- **Grafana** dashboards for visualization
- **Alert rules** for proactive monitoring

## Quick Start

### Deploy with Docker Compose

```bash
cd deployments/monitoring
docker-compose up -d
```

### Access Dashboards

- **Grafana:** http://localhost:3000 (admin/admin)
- **Prometheus:** http://localhost:9090
- **Alertmanager:** http://localhost:9093

## Directory Structure

```
deployments/monitoring/
├── SLOs.md                                    # SLO definitions
├── docker-compose.yml                         # Docker Compose stack
├── README.md                                  # This file
├── prometheus/
│   ├── prometheus.yml                         # Prometheus config
│   └── rules/
│       ├── slo-alerts.yml                     # SLO alert rules
│       └── custom-metrics.yml                 # Custom metrics rules
└── grafana/
    ├── dashboards/
    │   ├── muster-api-overview.json          # Main API dashboard
    │   ├── muster-mcp-dashboard.json         # MCP dashboard
    │   ├── muster-daemon-dashboard.json      # Daemon dashboard
    │   └── muster-cache-dashboard.json       # Cache dashboard
    └── provisioning/
        ├── dashboards/
        │   └── dashboards.yml                # Dashboard provisioning
        └── datasources/
            └── prometheus.yml                # Datasource provisioning
```

## SLOs Defined

See `SLOs.md` for complete definitions. Key SLOs:

| Category | Target | Measurement Window |
|----------|--------|-------------------|
| API Availability | 99.9% | 30 days |
| API Latency (p95) | < 200ms | 7 days |
| MCP Availability | 99.5% | 30 days |
| Daemon Availability | 99.99% | 30 days |
| Cache Hit Rate | > 95% | 7 days |

## Alerting

### Alert Severity Levels

- **Critical:** Immediate attention required (pages on-call)
- **Warning:** Should be addressed soon
- **Info:** Informational, no immediate action needed

### Key Alerts

1. **MusterHighErrorRate:** Error rate > 0.1%
2. **MusterHighLatency:** p95 latency > 200ms
3. **MusterAPIDown:** API availability checks failing
4. **MusterDaemonDown:** Daemon is not responding
5. **MusterCacheLowHitRate:** Cache hit rate < 95%

## Dashboards

### Muster API Overview

Main dashboard showing:
- SLO compliance (availability, latency, error rate, cache hit rate)
- Request rate by status code
- Latency distribution (p50, p95, p99)
- Resource usage (memory, CPU)
- Error budget burn down

### Runbooks

See `SLOs.md` for detailed runbooks on handling:
- High error rate
- High latency
- Service down
- Cache issues

## Custom Metrics

Muster exposes custom metrics at `/metrics`:

```
# HTTP requests
http_requests_total{status_code="200", method="GET", path="/api/v1/pets"}

# Request duration
request_duration_seconds_bucket{le="0.1", path="/api/v1/pets"}

# MCP tool calls
mcp_tool_calls_total{tool="getPet", status="success"}

# Cache operations
cache_hits_total{cache="memory"}
cache_misses_total{cache="memory"}

# Command store
cmdstore_load_duration_seconds_bucket{le="0.05"}
cmdstore_persist_total{status="success"}

# Auth operations
oauth_token_refresh_total{provider="oidc", status="success"}
auth_operation_duration_seconds_bucket{le="0.05", operation="validate"}
```

## Development

### Adding New Alerts

Edit `prometheus/rules/slo-alerts.yml`:

```yaml
- alert: MusterNewAlert
  expr: your_promql_expression > threshold
  for: 5m
  labels:
    severity: warning
  annotations:
    summary: "Description of the alert"
    description: "Detailed description"
```

### Adding New Dashboards

1. Create dashboard JSON in `grafana/dashboards/`
2. Grafana will auto-load via provisioning

### Testing Alerts

```bash
# Trigger test alert
curl -X POST http://localhost:9093/-/reload

# Check alert status
curl http://localhost:9090/api/v1/alerts
```

## Production Deployment

### Kubernetes

Apply the manifests:

```bash
kubectl apply -f k8s/monitoring/
```

### Configuration

Update `prometheus.yml` for production:
- Configure external labels (cluster, environment)
- Set up remote_write for long-term storage
- Configure alertmanager endpoints

### Security

1. Enable TLS for Prometheus/Grafana
2. Configure authentication
3. Restrict access to monitoring endpoints

## Troubleshooting

### No metrics appearing

1. Check Prometheus targets: http://localhost:9090/targets
2. Verify Muster metrics endpoint: curl http://muster:8080/metrics
3. Check pod annotations for Prometheus discovery

### Alerts not firing

1. Check alert rules: http://localhost:9090/rules
2. Verify Alertmanager is configured
3. Check Alertmanager UI for silences

### Dashboard empty

1. Verify datasource connection in Grafana
2. Check time range (default: last 1 hour)
3. Validate PromQL queries

## References

- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [SRE Book - SLOs](https://sre.google/sre-book/service-level-objectives/)
- [Prometheus Alerting](https://prometheus.io/docs/alerting/latest/overview/)

---

*Last Updated: 2026-04-02*
