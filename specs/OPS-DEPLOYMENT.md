# OPS-DEPLOYMENT: Deployment Guide and Manifests

## Overview
Complete deployment configurations for Kubernetes, Docker Compose, and systemd.

## Kubernetes Manifests

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: muster
  labels:
    name: muster
---
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: muster-config
  namespace: muster
data:
  APP_ENV: "production"
  LOG_LEVEL: "info"
  METRICS_PORT: "9090"
---
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: muster-secrets
  namespace: muster
type: Opaque
stringData:
  DATABASE_URL: "postgresql://user:pass@db:5432/muster"
  REDIS_URL: "redis://redis:6379"
  SECRET_KEY: "${SECRET_KEY}"
---
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: muster-api
  namespace: muster
spec:
  replicas: 3
  selector:
    matchLabels:
      app: muster-api
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  template:
    metadata:
      labels:
        app: muster-api
    spec:
      containers:
      - name: api
        image: muster/api:latest
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        envFrom:
        - configMapRef:
            name: muster-config
        - secretRef:
            name: muster-secrets
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
---
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: muster-api
  namespace: muster
spec:
  selector:
    app: muster-api
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
  type: ClusterIP
---
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: muster-api-hpa
  namespace: muster
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: muster-api
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

## Docker Compose

```yaml
# docker-compose.yml
version: '3.8'

services:
  api:
    build: .
    ports:
      - "8080:8080"
      - "9090:9090"
    environment:
      - APP_ENV=production
      - DATABASE_URL=postgresql://postgres:postgres@db:5432/muster
      - REDIS_URL=redis://redis:6379
      - LOG_LEVEL=info
    depends_on:
      db:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health/ready"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s
    deploy:
      replicas: 2
      resources:
        limits:
          cpus: '0.5'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 256M
    networks:
      - muster-network

  db:
    image: postgres:15-alpine
    environment:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: muster
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - muster-network

  redis:
    image: redis:7-alpine
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - muster-network

  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./ssl:/etc/nginx/ssl:ro
    depends_on:
      - api
    networks:
      - muster-network

volumes:
  postgres_data:

networks:
  muster-network:
    driver: bridge
```

## Systemd Service

```ini
# /etc/systemd/system/muster.service
[Unit]
Description=Muster API Service
After=network.target
Wants=network.target

[Service]
Type=notify
User=muster
Group=muster
WorkingDirectory=/opt/muster
Environment="APP_ENV=production"
Environment="LOG_LEVEL=info"
EnvironmentFile=/etc/muster/env
ExecStart=/opt/muster/venv/bin/python -m muster.server
ExecReload=/bin/kill -HUP $MAINPID
KillMode=mixed
KillSignal=SIGTERM
TimeoutStopSec=30
Restart=on-failure
RestartSec=5

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/muster

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
```

```bash
# Setup commands
sudo useradd -r -s /bin/false muster
sudo mkdir -p /opt/muster /var/log/muster /etc/muster
sudo chown -R muster:muster /opt/muster /var/log/muster
sudo cp muster.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable muster
sudo systemctl start muster
sudo systemctl status muster
```

## Deployment Commands

```bash
# Kubernetes
kubectl apply -f k8s/
kubectl rollout status deployment/muster-api -n muster
kubectl get pods -n muster

# Docker Compose
docker-compose up -d
docker-compose ps
docker-compose logs -f

# Rollback
kubectl rollout undo deployment/muster-api -n muster
docker-compose down && docker-compose up -d
```
