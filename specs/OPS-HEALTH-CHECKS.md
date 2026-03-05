# OPS-HEALTH-CHECKS: Health Check Endpoints Specification

## Overview
Kubernetes-compatible health check endpoints for container orchestration.

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health/live` | GET | Liveness probe |
| `/health/ready` | GET | Readiness probe |
| `/health/startup` | GET | Startup probe |

## Implementation

```python
from fastapi import FastAPI, status
from pydantic import BaseModel
from datetime import datetime
import asyncio

app = FastAPI()

class HealthStatus(BaseModel):
    status: str
    timestamp: datetime
    version: str
    checks: dict = {}

# Application state
app_state = {
    "started": False,
    "ready": False,
    "checks": {}
}

async def check_database():
    try:
        # Database health check
        await db.ping()
        return {"status": "healthy", "response_time_ms": 5}
    except Exception as e:
        return {"status": "unhealthy", "error": str(e)}

async def check_cache():
    try:
        # Cache health check
        await cache.ping()
        return {"status": "healthy"}
    except Exception as e:
        return {"status": "unhealthy", "error": str(e)}

@app.on_event("startup")
async def startup_event():
    app_state["started"] = True
    await asyncio.sleep(5)  # Warmup
    app_state["ready"] = True

@app.get("/health/live", status_code=status.HTTP_200_OK)
async def liveness_probe():
    """Liveness probe - Returns 200 if app is running"""
    if not app_state["started"]:
        return HealthStatus(
            status="down",
            timestamp=datetime.utcnow(),
            version="1.0.0"
        )
    
    return HealthStatus(
        status="healthy",
        timestamp=datetime.utcnow(),
        version="1.0.0"
    )

@app.get("/health/ready", status_code=status.HTTP_200_OK)
async def readiness_probe():
    """Readiness probe - Returns 200 if app can serve traffic"""
    if not app_state["ready"]:
        return HealthStatus(
            status="not_ready",
            timestamp=datetime.utcnow(),
            version="1.0.0"
        ), status.HTTP_503_SERVICE_UNAVAILABLE
    
    checks = {
        "database": await check_database(),
        "cache": await check_cache()
    }
    
    all_healthy = all(c["status"] == "healthy" for c in checks.values())
    
    status_code = status.HTTP_200_OK if all_healthy else status.HTTP_503_SERVICE_UNAVAILABLE
    
    return HealthStatus(
        status="ready" if all_healthy else "not_ready",
        timestamp=datetime.utcnow(),
        version="1.0.0",
        checks=checks
    ), status_code

@app.get("/health/startup", status_code=status.HTTP_200_OK)
async def startup_probe():
    """Startup probe - Returns 200 when app has started"""
    if app_state["started"]:
        return {"status": "started", "timestamp": datetime.utcnow().isoformat()}
    return {"status": "starting"}, status.HTTP_503_SERVICE_UNAVAILABLE
```

## Kubernetes Configuration

```yaml
apiVersion: apps/v1
kind: Deployment
spec:
  template:
    spec:
      containers:
      - name: app
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
        startupProbe:
          httpGet:
            path: /health/startup
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          failureThreshold: 30
```

## Response Codes

| Code | Meaning | Action |
|------|---------|--------|
| 200 | Healthy | Continue |
| 503 | Unhealthy | Stop traffic/restart |
