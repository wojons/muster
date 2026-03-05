# OPS-LOGGING: Structured Logging Specification

## Overview
Structured logging for observability and debugging with JSON output.

## Log Levels

| Level | Value | Usage |
|-------|-------|-------|
| DEBUG | 10 | Detailed debugging |
| INFO | 20 | General information |
| WARNING | 30 | Potential issues |
| ERROR | 40 | Error conditions |
| CRITICAL | 50 | System failure |

```python
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger('app')

logger.debug('Debug message')
logger.info('Processing request', extra={'request_id': 'abc123'})
logger.warning('Resource low', extra={'cpu_percent': 85})
logger.error('Database connection failed', extra={'host': 'db-01'})
logger.critical('System shutdown initiated')
```

## JSON Format

```python
import json
import logging
from pythonjsonlogger import jsonlogger

class CustomJsonFormatter(jsonlogger.JsonFormatter):
    def add_fields(self, log_record, record, message_dict):
        super().add_fields(log_record, record, message_dict)
        log_record['timestamp'] = datetime.utcnow().isoformat()
        log_record['level'] = record.levelname
        log_record['logger'] = record.name
        log_record['source'] = f"{record.filename}:{record.lineno}"

formatter = CustomJsonFormatter('%(timestamp)s %(level)s %(name)s %(message)s')
handler = logging.StreamHandler()
handler.setFormatter(formatter)

logger = logging.getLogger('app')
logger.addHandler(handler)
logger.setLevel(logging.INFO)
```

## Contextual Logging

```python
import structlog

structlog.configure(
    processors=[
        structlog.stdlib.filter_by_level,
        structlog.stdlib.add_logger_name,
        structlog.stdlib.add_log_level,
        structlog.stdlib.PositionalArgumentsFormatter(),
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.StackInfoRenderer(),
        structlog.processors.format_exc_info,
        structlog.processors.UnicodeDecoder(),
        structlog.processors.JSONRenderer()
    ],
    context_class=dict,
    logger_factory=structlog.stdlib.LoggerFactory(),
    wrapper_class=structlog.stdlib.BoundLogger,
    cache_logger_on_first_use=True,
)

logger = structlog.get_logger()
logger = logger.bind(request_id='abc123', user_id='user-456')
logger.info('user_action', action='login', ip='192.168.1.1')
```

## Log Rotation

```python
import logging
from logging.handlers import RotatingFileHandler, TimedRotatingFileHandler

# Size-based rotation
file_handler = RotatingFileHandler(
    'app.log',
    maxBytes=10*1024*1024,  # 10MB
    backupCount=5
)

# Time-based rotation (daily)
timed_handler = TimedRotatingFileHandler(
    'app.log',
    when='midnight',
    interval=1,
    backupCount=30
)

logger = logging.getLogger('app')
logger.addHandler(file_handler)
logger.addHandler(timed_handler)
```

## Correlation IDs

```python
import uuid
from contextvars import ContextVar

request_id = ContextVar('request_id', default=None)

class CorrelationIdFilter(logging.Filter):
    def filter(self, record):
        record.correlation_id = request_id.get() or 'unknown'
        return True

logger.addFilter(CorrelationIdFilter())
formatter = logging.Formatter('%(asctime)s [%(correlation_id)s] %(levelname)s: %(message)s')

# Middleware
@app.middleware("http")
async def add_correlation_id(request, call_next):
    request_id.set(str(uuid.uuid4()))
    response = await call_next(request)
    response.headers['X-Request-ID'] = request_id.get()
    return response
```

## Sensitive Data Filtering

```python
import re

class SensitiveDataFilter(logging.Filter):
    PATTERNS = [
        (r'password["\']?\s*[:=]\s*["\']?[^"\']+', 'password": "[REDACTED]"'),
        (r'token["\']?\s*[:=]\s*["\']?[^"\']+', 'token": "[REDACTED]"'),
        (r'api_key["\']?\s*[:=]\s*["\']?[^"\']+', 'api_key": "[REDACTED]"'),
    ]
    
    def filter(self, record):
        if isinstance(record.msg, str):
            for pattern, replacement in self.PATTERNS:
                record.msg = re.sub(pattern, replacement, record.msg, flags=re.IGNORECASE)
        return True

logger.addFilter(SensitiveDataFilter())
```

## Configuration

```yaml
# logging.yaml
version: 1
disable_existing_loggers: false

formatters:
  json:
    class: pythonjsonlogger.jsonlogger.JsonFormatter
    format: '%(asctime)s %(levelname)s %(name)s %(message)s'
  
handlers:
  console:
    class: logging.StreamHandler
    formatter: json
    stream: ext://sys.stdout
  
  file:
    class: logging.handlers.RotatingFileHandler
    formatter: json
    filename: /var/log/app/app.log
    maxBytes: 10485760
    backupCount: 5

loggers:
  app:
    level: INFO
    handlers: [console, file]
    propagate: no
```
