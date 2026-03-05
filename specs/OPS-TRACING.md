# OPS-TRACING: Distributed Tracing Specification

## Overview
OpenTelemetry-based distributed tracing for request flow analysis.

## OpenTelemetry Setup

```python
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource, SERVICE_NAME

resource = Resource(attributes={
    SERVICE_NAME: "muster-api"
})

provider = TracerProvider(resource=resource)
processor = BatchSpanProcessor(OTLPSpanExporter(endpoint="otel-collector:4317"))
provider.add_span_processor(processor)
trace.set_tracer_provider(provider)

tracer = trace.get_tracer("muster.tracer")
```

## Span Creation

### Basic Span

```python
with tracer.start_as_current_span("process-request") as span:
    span.set_attribute("http.method", "GET")
    span.set_attribute("http.url", "/api/v1/users")
    # ... process request ...
```

### Nested Spans

```python
with tracer.start_as_current_span("parent-operation") as parent:
    parent.set_attribute("operation.type", "user-creation")
    
    with tracer.start_as_current_span("validate-input") as child1:
        child1.set_attribute("validation.duration_ms", 45)
    
    with tracer.start_as_current_span("database-insert") as child2:
        child2.set_attribute("db.table", "users")
        child2.set_attribute("db.rows_affected", 1)
```

### Async Spans

```python
async def process_request():
    with tracer.start_as_current_span("async-operation") as span:
        span.set_attribute("async", True)
        await some_async_work()
```

## Span Attributes

```python
span.set_attribute("user.id", "12345")
span.set_attribute("user.tier", "premium")
span.set_attribute("request.size_bytes", 1024)
span.set_attribute("cache.hit", True)
span.set_attribute("db.query_time_ms", 25.5)
```

## Span Events

```python
span.add_event("cache-miss", {
    "cache.key": "user:12345",
    "cache.ttl": 300
})

span.add_event("database-query-start")
# ... execute query ...
span.add_event("database-query-end", {
    "db.duration_ms": 45
})
```

## Span Status

```python
from opentelemetry.trace import Status, StatusCode

span.set_status(Status(StatusCode.OK))
span.set_status(Status(StatusCode.ERROR, "Database connection timeout"))
```

## Error Recording

```python
import traceback

try:
    risky_operation()
except Exception as e:
    span.set_status(Status(StatusCode.ERROR, str(e)))
    span.record_exception(e)
    span.set_attribute("error.type", type(e).__name__)
    span.set_attribute("error.stacktrace", traceback.format_exc())
```

## Instrumentation

### Flask

```python
from opentelemetry.instrumentation.flask import FlaskInstrumentor

FlaskInstrumentor().instrument_app(app)
```

### FastAPI

```python
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor

FastAPIInstrumentor.instrument_app(app)
```

### Database

```python
from opentelemetry.instrumentation.sqlalchemy import SQLAlchemyInstrumentor

SQLAlchemyInstrumentor().instrument()
```

### HTTP Client

```python
from opentelemetry.instrumentation.requests import RequestsInstrumentor

RequestsInstrumentor().instrument()
```

## Exporters

### Jaeger

```python
from opentelemetry.exporter.jaeger.thrift import JaegerExporter

jaeger_exporter = JaegerExporter(
    agent_host_name="jaeger-agent",
    agent_port=6831,
)
```

### Zipkin

```python
from opentelemetry.exporter.zipkin.json import ZipkinExporter

zipkin_exporter = ZipkinExporter(
    endpoint="http://zipkin:9411/api/v2/spans"
)
```

### OTLP

```python
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter

otlp_exporter = OTLPSpanExporter(
    endpoint="http://otel-collector:4318/v1/traces"
)
```

## Context Propagation

```python
from opentelemetry.propagate import inject, extract

# Inject context into headers
headers = {}
inject(headers)

# Extract context from headers
context = extract(headers)
with tracer.start_as_current_span("process", context=context):
    pass
```
