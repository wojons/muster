# OPS-METRICS: Prometheus Metrics Specification

## Overview
Prometheus metrics exposition and collection for observability.

## Metric Types

### Counter
Monotonically increasing cumulative metrics.

```python
from prometheus_client import Counter

http_requests_total = Counter(
    'http_requests_total',
    'Total HTTP requests',
    ['method', 'endpoint', 'status_code']
)

# Usage
http_requests_total.labels(method='GET', endpoint='/api/v1/users', status_code='200').inc()
```

### Gauge
Single numerical values that can go up or down.

```python
from prometheus_client import Gauge

active_connections = Gauge(
    'active_connections',
    'Number of active connections',
    ['service']
)

# Usage
active_connections.labels(service='api').set(42)
```

### Histogram
Samples observations and counts them in buckets.

```python
from prometheus_client import Histogram

request_duration_seconds = Histogram(
    'request_duration_seconds',
    'Request duration in seconds',
    ['endpoint'],
    buckets=[0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0]
)

# Usage
import time
start = time.time()
# ... handle request ...
request_duration_seconds.labels(endpoint='/api/v1/users').observe(time.time() - start)
```

### Summary
Similar to histogram but calculates configurable quantiles.

```python
from prometheus_client import Summary

request_latency = Summary(
    'request_latency_seconds',
    'Request latency',
    ['method']
)

# Usage
@RequestLatency.time()
def handle_request():
    pass
```

## Collectors

### Custom Collector

```python
from prometheus_client.core import GaugeMetricFamily, REGISTRY

class CustomCollector:
    def collect(self):
        gauge = GaugeMetricFamily(
            'custom_metric',
            'Custom metric description',
            labels=['label1']
        )
        gauge.add_metric(['value1'], 42)
        yield gauge

REGISTRY.register(CustomCollector())
```

### Process Collector

```python
from prometheus_client import ProcessCollector

ProcessCollector(namespace='app')
```

### Platform Collector

```python
from prometheus_client import PlatformCollector

PlatformCollector()
```

## Exposition

### HTTP Endpoint

```python
from prometheus_client import start_http_server

start_http_server(9090)
```

### Flask Integration

```python
from flask import Flask
from prometheus_client import make_wsgi_app
from werkzeug.middleware.dispatcher import DispatcherMiddleware

app = Flask(__name__)
app.wsgi_app = DispatcherMiddleware(app.wsgi_app, {
    '/metrics': make_wsgi_app()
})
```

### FastAPI Integration

```python
from fastapi import FastAPI
from prometheus_client import make_asgi_app

app = FastAPI()
metrics_app = make_asgi_app()
app.mount("/metrics", metrics_app)
```

## Metric Naming

| Type | Suffix | Example |
|------|--------|---------|
| Counter | `_total` | `http_requests_total` |
| Gauge | none | `memory_usage_bytes` |
| Histogram | `_bucket`, `_sum`, `_count` | `request_duration_seconds_bucket` |
| Summary | `_sum`, `_count` | `request_latency_seconds_sum` |

## Pushgateway

```python
from prometheus_client import CollectorRegistry, Gauge, push_to_gateway

registry = CollectorRegistry()
g = Gauge('job_last_success', 'Last successful run', registry=registry)
g.set_to_current_time()
push_to_gateway('pushgateway:9091', job='batch_job', registry=registry)
```

## Best Practices
- Use base units (seconds, bytes)
- Include all relevant labels
- Avoid high cardinality labels
- Use appropriate bucket sizes
- Document all metrics
