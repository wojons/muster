# Log Aggregation Setup

Muster supports two log aggregation backends: **ELK Stack** (Elasticsearch, Logstash, Kibana) and **Grafana Loki**.

## Quick Start

### Option 1: ELK Stack

```bash
cd deployments/logging/elk
docker-compose up -d
```

Access Kibana at: http://localhost:5601

### Option 2: Grafana Loki

```bash
cd deployments/logging/loki
docker-compose up -d
```

Access Grafana at: http://localhost:3000 (admin/admin)

## Configuration

### Application Configuration

Add to your `config.yaml`:

```yaml
logging:
  # Output JSON logs for aggregation
  format: json
  
  # Include correlation IDs
  correlation_id_header: X-Correlation-ID
  
  # Log aggregation configuration
  aggregation:
    # Enable log shipping to aggregator
    enabled: true
    
    # Which backend to use: "elk", "loki", or "stdout"
    backend: loki
    
    # Loki configuration
    loki:
      url: http://localhost:3100
      batch_size: 1024
      batch_wait: 1s
      labels:
        service: muster-api
        environment: production
    
    # ELK configuration
    elk:
      logstash:
        host: localhost
        port: 5044
        protocol: tcp
      
    # Log retention settings
    retention:
      hot: 30d      # Keep in fast storage
      warm: 90d     # Keep in slower storage
      cold: 365d    # Archive to cold storage
```

## Architecture

### Log Flow

```
┌─────────────┐
│   Muster    │
│ Application │
└──────┬──────┘
       │ JSON logs
       │ + correlation IDs
       ▼
┌─────────────┐
│   Promtail  │ (Loki)
│  Filebeat   │ (ELK)
└──────┬──────┘
       │ Ship logs
       ▼
┌─────────────┐     ┌─────────────┐
│    Loki     │     │ Elasticsearch│
└──────┬──────┘     └──────┬──────┘
       │                   │
       └─────────┬─────────┘
                 ▼
          ┌─────────────┐
          │   Grafana   │
          │    Kibana   │
          └─────────────┘
```

## Log Retention Policies

### Default Retention

| Tier | Duration | Storage | Access Pattern |
|------|----------|---------|----------------|
| Hot | 0-30 days | SSD/High IOPS | Real-time queries |
| Warm | 30-90 days | Standard disk | Historical analysis |
| Cold | 90+ days | Archive/S3 | Compliance only |

### Retention Configuration

#### Loki

Edit `loki-config.yml`:

```yaml
table_manager:
  retention_deletes_enabled: true
  retention_period: 90d

schema_config:
  configs:
    - from: 2026-01-01
      store: boltdb-shipper
      object_store: filesystem
      schema: v11
      index:
        prefix: index_
        period: 24h
      chunks:
        prefix: chunk_
        period: 168h  # 7 days
```

#### Elasticsearch

Set index lifecycle policy:

```json
{
  "policy": {
    "phases": {
      "hot": {
        "min_age": "0ms",
        "actions": {
          "rollover": {
            "max_age": "1d",
            "max_size": "50gb"
          }
        }
      },
      "warm": {
        "min_age": "30d",
        "actions": {
          "shrink": {
            "number_of_shards": 1
          }
        }
      },
      "delete": {
        "min_age": "90d",
        "actions": {
          "delete": {}
        }
      }
    }
  }
}
```

## Log Queries

### Loki (LogQL)

```logql
# Query by service
{service="muster-api"}

# Query by correlation ID
{service="muster-api"} |= "correlation_id=abc123"

# Query errors
{service="muster-api"} |= "level=error"

# Query by time range
{service="muster-api"} 
  |= "correlation_id=abc123"
  [1h]

# Aggregate error rate
sum by (level) (
  rate(
    {service="muster-api"} 
    |= "level=error" [5m]
  )
)
```

### Kibana (Lucene)

```
service:must* AND level:error
service:muster-api AND correlation_id:abc123
service:muster-api AND @timestamp:[now-1h TO now]
```

## Alerting

### Loki Alerting Rules

```yaml
# loki-alerts.yml
groups:
  - name: muster-alerts
    rules:
      - alert: HighErrorRate
        expr: |
          sum by (service) (
            rate(
              {service="muster-api"} 
              |= "level=error" [5m]
            )
          ) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High error rate in {{ $labels.service }}"
          
      - alert: LogVolumeDrop
        expr: |
          sum by (service) (
            rate(
              {service="muster-api"} [5m]
            )
          ) == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Log volume dropped for {{ $labels.service }}"
```

### Elasticsearch Watcher

```json
{
  "trigger": {
    "schedule": {
      "interval": "5m"
    }
  },
  "input": {
    "search": {
      "request": {
        "search_type": "query_then_fetch",
        "indices": ["muster-logs-*"],
        "body": {
          "query": {
            "bool": {
              "must": [
                {
                  "range": {
                    "@timestamp": {
                      "gte": "now-5m"
                    }
                  }
                },
                {
                  "term": {
                    "level": "error"
                  }
                }
              ]
            }
          }
        }
      }
    }
  },
  "condition": {
    "compare": {
      "ctx.payload.hits.total": {
        "gt": 100
      }
    }
  },
  "actions": {
    "send_email": {
      "email": {
        "to": "alerts@example.com",
        "subject": "High error rate detected"
      }
    }
  }
}
```

## Correlation ID Tracking

Correlation IDs enable request tracing across services:

```go
// In your handler
ctx := log.WithCorrelationID(r.Context(), r.Header.Get("X-Correlation-ID"))

// Automatically propagated to all log entries
logger := log.FromContext(ctx)
logger.Info().
    Str("component", "api").
    Str("operation", "get_user").
    Msg("Processing request")

// Output includes:
// {"correlation_id": "abc-123-def", "component": "api", ...}
```

## Troubleshooting

### Logs not appearing

1. Check Promtail/Filebeat status:
   ```bash
   docker logs promtail
   ```

2. Verify log file permissions:
   ```bash
   ls -la /var/lib/docker/containers/
   ```

3. Check Loki/Elasticsearch connectivity:
   ```bash
   curl http://localhost:3100/ready  # Loki
   curl http://localhost:9200/_cluster/health  # Elasticsearch
   ```

### High memory usage

1. Adjust retention policies
2. Reduce batch sizes in Promtail/Filebeat config
3. Enable compression

### Log shipping delays

1. Check network connectivity
2. Review batch wait times
3. Monitor queue depths

## Security Considerations

- Enable TLS for log shipping
- Use authentication for Loki/Elasticsearch
- Redact sensitive data before shipping (automatic in Muster)
- Restrict log query access
- Rotate API keys regularly
- Audit log access

## Performance Tuning

### Loki

- Increase `batch_size` for higher throughput
- Tune `chunk_target_size` based on log volume
- Use SSD for index storage
- Enable compression

### Elasticsearch

- Increase heap size for large clusters
- Use multiple shards for high volume
- Enable index lifecycle management
- Tune refresh intervals

## References

- [Loki Documentation](https://grafana.com/docs/loki/)
- [ELK Stack Documentation](https://www.elastic.co/guide/index.html)
- [Muster Logging Standards](../LOGGING-STANDARDS.md)
