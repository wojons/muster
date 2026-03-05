# OPS-MONITORING: Monitoring Setup and Dashboards

## Overview
Complete monitoring stack with Prometheus, Grafana, and alerting.

## Prometheus Configuration

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s
  external_labels:
    cluster: production
    replica: '{{.ExternalURL}}'

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']

rule_files:
  - /etc/prometheus/alerts/*.yml

scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']

  - job_name: 'muster-api'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
            - muster
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        target_label: __address__
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2

  - job_name: 'node-exporter'
    static_configs:
      - targets: ['node-exporter:9100']

  - job_name: 'postgres-exporter'
    static_configs:
      - targets: ['postgres-exporter:9187']
```

## Grafana Dashboard

```json
{
  "dashboard": {
    "title": "Muster API Overview",
    "tags": ["muster", "api"],
    "timezone": "utc",
    "panels": [
      {
        "title": "Request Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(http_requests_total[5m])) by (status_code)",
            "legendFormat": "{{status_code}}"
          }
        ],
        "yaxes": [{"format": "reqps"}]
      },
      {
        "title": "Latency (95th percentile)",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, sum(rate(request_duration_seconds_bucket[5m])) by (le))",
            "legendFormat": "p95"
          }
        ],
        "yaxes": [{"format": "s"}]
      },
      {
        "title": "Error Rate",
        "type": "singlestat",
        "targets": [
          {
            "expr": "sum(rate(http_requests_total{status_code=~\"5..\"}[5m])) / sum(rate(http_requests_total[5m]))"
          }
        ],
        "format": "percentunit"
      },
      {
        "title": "Active Connections",
        "type": "graph",
        "targets": [
          {
            "expr": "active_connections",
            "legendFormat": "Connections"
          }
        ]
      },
      {
        "title": "Database Connections",
        "type": "graph",
        "targets": [
          {
            "expr": "pg_stat_activity_count",
            "legendFormat": "Active"
          }
        ]
      },
      {
        "title": "Memory Usage",
        "type": "graph",
        "targets": [
          {
            "expr": "process_resident_memory_bytes / 1024 / 1024",
            "legendFormat": "MB"
          }
        ],
        "yaxes": [{"format": "mbytes"}]
      }
    ]
  }
}
```

## Key Metrics

| Metric | Description | Alert Threshold |
|--------|-------------|-----------------|
| `http_requests_total` | Total HTTP requests | - |
| `request_duration_seconds` | Request latency | p95 > 500ms |
| `active_connections` | Active connections | > 1000 |
| `process_resident_memory_bytes` | Memory usage | > 512MB |
| `up` | Service health | = 0 |

## Runbooks

### High Error Rate

**Symptoms:** Error rate > 5%

**Checklist:**
1. Check application logs: `kubectl logs -f deployment/muster-api`
2. Verify database connectivity
3. Check external service health
4. Review recent deployments
5. Check resource utilization

**Resolution:**
- Rollback if recent deployment: `kubectl rollout undo deployment/muster-api`
- Scale up if resource constrained
- Restart affected pods

### High Latency

**Symptoms:** p95 latency > 500ms

**Checklist:**
1. Check database query performance
2. Review cache hit rates
3. Analyze slow request traces
4. Check network latency

**Resolution:**
- Optimize slow queries
- Increase cache TTL
- Scale horizontally

### Service Down

**Symptoms:** Health check failures

**Checklist:**
1. Verify pod status: `kubectl get pods`
2. Check events: `kubectl get events`
3. Review container logs
4. Check resource limits

**Resolution:**
- Delete stuck pods
- Increase resource limits
- Check node health

## Monitoring Commands

```bash
# View metrics
curl http://localhost:9090/metrics

# Query Prometheus
kubectl port-forward svc/prometheus 9090:9090
open http://localhost:9090

# Access Grafana
kubectl port-forward svc/grafana 3000:3000
open http://localhost:3000
```
