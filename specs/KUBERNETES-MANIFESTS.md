# KUBERNETES-MANIFESTS.md

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

---

## Overview

The Kubernetes Manifests specification defines complete deployment configurations for running the OpenAPI CLI + MCP Server on Kubernetes. This includes Deployments, Services, ConfigMaps, Secrets, and supporting resources for production-grade operation.

A blind person can visualize Kubernetes as a **highly organized warehouse management system**. The warehouse (cluster) has multiple sections (namespaces), each containing workers (pods) that perform tasks. The workers are organized into teams (deployments), have designated workstations (services) for communication, and store their instructions (configurations) in labeled filing cabinets (configmaps and secrets). A supervisor (controller) constantly monitors and replaces any worker who stops functioning.

### Key Resources

1. **Namespace**: Logical isolation boundary
2. **Deployment**: Manages replica sets and pod lifecycle
3. **Service**: Network access to pods
4. **ConfigMap**: Non-sensitive configuration data
5. **Secret**: Sensitive configuration data
6. **ServiceAccount**: Pod identity and permissions
7. **RBAC**: Role-based access control
8. **NetworkPolicy**: Network traffic rules
9. **HPA**: Horizontal pod autoscaling
10. **PodDisruptionBudget**: Availability guarantees

---

## Technical Implementation

### 1. Namespace

```yaml
# base/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/version: "1.0.0"
    app.kubernetes.io/component: namespace
    app.kubernetes.io/part-of: openapi-cli
    app.kubernetes.io/managed-by: kubectl
---
```

### 2. ServiceAccount

```yaml
# base/serviceaccount.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: openapi-cli
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: serviceaccount
automountServiceAccountToken: true
---
```

### 3. RBAC Configuration

```yaml
# base/rbac.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: openapi-cli
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: rbac
rules:
  # Allow reading configmaps
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch"]
  
  # Allow reading secrets (for API keys)
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get", "list", "watch"]
  
  # Allow reading service discovery
  - apiGroups: [""]
    resources: ["services", "endpoints"]
    verbs: ["get", "list", "watch"]
  
  # Allow pod discovery for inventory
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: openapi-cli
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: rbac
subjects:
  - kind: ServiceAccount
    name: openapi-cli
    namespace: openapi-cli
roleRef:
  kind: Role
  name: openapi-cli
  apiGroup: rbac.authorization.k8s.io
---
```

### 4. ConfigMap

```yaml
# base/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: openapi-cli-config
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: config
data:
  config.yaml: |
    server:
      port: 8080
      host: 0.0.0.0
    
    log:
      level: info
      format: json
      output: stdout
    
    inventory:
      cache_ttl: 300
      fetch_timeout: 30s
      
    http:
      timeout: 30s
      max_retries: 3
      retry_backoff: exponential
      
    rate_limit:
      enabled: true
      requests_per_minute: 100
      burst: 10
      
    metrics:
      enabled: true
      path: /metrics
      
    health:
      enabled: true
      path: /health
      
    tracing:
      enabled: true
      endpoint: "http://jaeger:14268/api/traces"
      sample_rate: 0.1
---
```

### 5. Secret

```yaml
# base/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: openapi-cli-secrets
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: secrets
type: Opaque
stringData:
  # Base64-encoded sensitive values
  # Use: kubectl create secret generic openapi-cli-secrets \
  #   --from-literal=api-key=YOUR_API_KEY \
  #   --from-literal=oauth-client-secret=YOUR_SECRET
  api-key: ""  # Populate via external secret management
  oauth-client-secret: ""
  database-url: ""
---
```

### 6. Deployment

```yaml
# base/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: openapi-cli
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: server
    app.kubernetes.io/version: "1.0.0"
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  selector:
    matchLabels:
      app.kubernetes.io/name: openapi-cli
      app.kubernetes.io/component: server
  template:
    metadata:
      labels:
        app.kubernetes.io/name: openapi-cli
        app.kubernetes.io/component: server
        app.kubernetes.io/version: "1.0.0"
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: openapi-cli
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      
      containers:
        - name: openapi-cli
          image: ghcr.io/openapi-cli/openapi-cli:1.0.0
          imagePullPolicy: IfNotPresent
          command: ["/usr/local/bin/openapi-cli"]
          args: ["daemon", "start", "--config", "/config/config.yaml"]
          
          ports:
            - name: http
              containerPort: 8080
              protocol: TCP
            - name: grpc
              containerPort: 9090
              protocol: TCP
          
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef:
                  fieldPath: metadata.name
            - name: POD_NAMESPACE
              valueFrom:
                fieldRef:
                  fieldPath: metadata.namespace
            - name: POD_IP
              valueFrom:
                fieldRef:
                  fieldPath: status.podIP
            - name: API_KEY
              valueFrom:
                secretKeyRef:
                  name: openapi-cli-secrets
                  key: api-key
            - name: OAUTH_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: openapi-cli-secrets
                  key: oauth-client-secret
          
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
          
          volumeMounts:
            - name: config
              mountPath: /config
              readOnly: true
            - name: cache
              mountPath: /home/nonroot/.cache/openapi-cli
            - name: tmp
              mountPath: /tmp
          
          livenessProbe:
            httpGet:
              path: /health/live
              port: http
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 5
            failureThreshold: 3
          
          readinessProbe:
            httpGet:
              path: /health/ready
              port: http
            initialDelaySeconds: 5
            periodSeconds: 5
            timeoutSeconds: 3
            failureThreshold: 3
          
          startupProbe:
            httpGet:
              path: /health/startup
              port: http
            initialDelaySeconds: 5
            periodSeconds: 5
            timeoutSeconds: 3
            failureThreshold: 30
          
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - ALL
      
      volumes:
        - name: config
          configMap:
            name: openapi-cli-config
        - name: cache
          emptyDir:
            sizeLimit: 1Gi
        - name: tmp
          emptyDir:
            sizeLimit: 100Mi
      
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              app.kubernetes.io/name: openapi-cli
      
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchExpressions:
                    - key: app.kubernetes.io/name
                      operator: In
                      values:
                        - openapi-cli
                topologyKey: kubernetes.io/hostname
---
```

### 7. Service

```yaml
# base/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: openapi-cli
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: service
spec:
  type: ClusterIP
  ports:
    - name: http
      port: 80
      targetPort: 8080
      protocol: TCP
    - name: grpc
      port: 9090
      targetPort: 9090
      protocol: TCP
  selector:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: server
  sessionAffinity: None
---
```

### 8. Horizontal Pod Autoscaler

```yaml
# base/hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: openapi-cli
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: autoscaling
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: openapi-cli
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
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
        - type: Percent
          value: 100
          periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Percent
          value: 10
          periodSeconds: 60
---
```

### 9. Pod Disruption Budget

```yaml
# base/pdb.yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: openapi-cli
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: availability
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: openapi-cli
      app.kubernetes.io/component: server
---
```

### 10. Network Policy

```yaml
# base/networkpolicy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: openapi-cli
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: network-policy
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: openapi-cli
      app.kubernetes.io/component: server
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow HTTP traffic from ingress
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - protocol: TCP
          port: 8080
    # Allow internal cluster traffic
    - from:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 8080
        - protocol: TCP
          port: 9090
  egress:
    # Allow DNS
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: UDP
          port: 53
    # Allow external HTTPS
    - to: []
      ports:
        - protocol: TCP
          port: 443
    # Allow internal API calls
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 8080
---
```

### 11. Ingress

```yaml
# overlays/production/ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: openapi-cli
  namespace: openapi-cli
  labels:
    app.kubernetes.io/name: openapi-cli
    app.kubernetes.io/component: ingress
  annotations:
    kubernetes.io/ingress.class: nginx
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/proxy-body-size: "10m"
    nginx.ingress.kubernetes.io/rate-limit: "100"
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
spec:
  tls:
    - hosts:
        - api.openapi-cli.io
      secretName: openapi-cli-tls
  rules:
    - host: api.openapi-cli.io
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: openapi-cli
                port:
                  number: 80
---
```

### 12. Kustomization

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: openapi-cli

resources:
  - base/namespace.yaml
  - base/serviceaccount.yaml
  - base/rbac.yaml
  - base/configmap.yaml
  - base/secret.yaml
  - base/deployment.yaml
  - base/service.yaml
  - base/hpa.yaml
  - base/pdb.yaml
  - base/networkpolicy.yaml

commonLabels:
  app.kubernetes.io/name: openapi-cli
  app.kubernetes.io/part-of: openapi-cli
  app.kubernetes.io/managed-by: kustomize

images:
  - name: ghcr.io/openapi-cli/openapi-cli
    newTag: 1.0.0

replicas:
  - name: openapi-cli
    count: 3

configMapGenerator:
  - name: openapi-cli-config
    behavior: merge
    literals:
      - LOG_LEVEL=info

---
# overlays/production/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: openapi-cli-production

resources:
  - ../../base
  - ingress.yaml

namePrefix: prod-

commonLabels:
  environment: production

patchesStrategicMerge:
  - replica-patch.yaml
  - resource-patch.yaml

configMapGenerator:
  - name: openapi-cli-config
    behavior: merge
    literals:
      - LOG_LEVEL=warn
      - RATE_LIMIT_ENABLED=true
```

### 13. Helm Chart

```yaml
# Chart.yaml
apiVersion: v2
name: openapi-cli
description: A Helm chart for OpenAPI CLI + MCP Server
type: application
version: 1.0.0
appVersion: "1.0.0"
keywords:
  - openapi
  - cli
  - mcp
  - api
home: https://openapi-cli.io
sources:
  - https://github.com/openapi-cli/muster
maintainers:
  - name: OpenAPI CLI Team
    email: team@openapi-cli.io
---
# values.yaml
replicaCount: 3

image:
  repository: ghcr.io/openapi-cli/openapi-cli
  pullPolicy: IfNotPresent
  tag: "1.0.0"

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""

serviceAccount:
  create: true
  annotations: {}
  name: ""

podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8080"
  prometheus.io/path: "/metrics"

podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  fsGroup: 65532

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL

service:
  type: ClusterIP
  port: 80
  targetPort: 8080

ingress:
  enabled: false
  className: "nginx"
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

resources:
  requests:
    memory: 128Mi
    cpu: 100m
  limits:
    memory: 512Mi
    cpu: 500m

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 80

nodeSelector: {}

tolerations: []

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                  - openapi-cli
          topologyKey: kubernetes.io/hostname
```

### 14. Deployment Scripts

```bash
#!/bin/bash
# scripts/deploy-k8s.sh

set -euo pipefail

ENVIRONMENT=${1:-staging}
NAMESPACE="openapi-cli-${ENVIRONMENT}"

echo "Deploying to ${ENVIRONMENT}..."

# Apply base resources
echo "Applying base resources..."
kubectl apply -k overlays/${ENVIRONMENT}/

# Wait for rollout
echo "Waiting for deployment..."
kubectl rollout status deployment/openapi-cli -n ${NAMESPACE} --timeout=5m

# Verify health
echo "Verifying health..."
kubectl get pods -n ${NAMESPACE} -l app.kubernetes.io/name=openapi-cli

echo "Deployment complete!"
```

### 15. Health Checks

```go
// internal/server/health.go
package server

import (
    "net/http"
)

type HealthChecker struct {
    checks []HealthCheck
}

type HealthCheck struct {
    Name     string
    Check    func() error
    Critical bool
}

func (hc *HealthChecker) Register(name string, critical bool, check func() error) {
    hc.checks = append(hc.checks, HealthCheck{
        Name:     name,
        Check:    check,
        Critical: critical,
    })
}

func (hc *HealthChecker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    status := HealthStatus{
        Status: "healthy",
        Checks: make(map[string]CheckResult),
    }
    
    httpStatus := http.StatusOK
    
    for _, check := range hc.checks {
        err := check.Check()
        result := CheckResult{
            Status: "pass",
        }
        
        if err != nil {
            result.Status = "fail"
            result.Error = err.Error()
            
            if check.Critical {
                status.Status = "unhealthy"
                httpStatus = http.StatusServiceUnavailable
            }
        }
        
        status.Checks[check.Name] = result
    }
    
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(httpStatus)
    json.NewEncoder(w).Encode(status)
}
```

---

## Resource Specifications

| Resource | Replicas | Memory | CPU | Storage |
|----------|----------|--------|-----|---------|
| Deployment | 3 | 128-512Mi | 100-500m | 1Gi cache |
| HPA | 3-10 | Auto | Auto | - |
| PDB | Min 2 | - | - | - |

## Security Configuration

- **ServiceAccount**: Least-privilege RBAC
- **SecurityContext**: Non-root, read-only root filesystem
- **NetworkPolicy**: Ingress/Egress restrictions
- **Secrets**: External secret management recommended
- **PodSecurity**: RuntimeDefault seccomp profile

---

## Usage Examples

```bash
# Deploy to staging
kubectl apply -k overlays/staging/

# Deploy with Helm
helm upgrade --install openapi-cli ./chart \
  --namespace openapi-cli \
  --set image.tag=1.0.0 \
  --set ingress.enabled=true

# Check deployment status
kubectl get pods -n openapi-cli
kubectl logs -n openapi-cli -l app.kubernetes.io/name=openapi-cli

# Port forward for local testing
kubectl port-forward -n openapi-cli svc/openapi-cli 8080:80

# Scale deployment
kubectl scale deployment openapi-cli -n openapi-cli --replicas=5

# Rollout restart
kubectl rollout restart deployment/openapi-cli -n openapi-cli
```

---

## References

- **Docker Image**: `DOCKER-IMAGE.md`
- **Container Integration**: `CONTAINER-INTEGRATION.md`
- **Deployment Spec**: `components/deployment-spec.md`
- **Operations**: `OPS-DEPLOYMENT.md`

---

**Bible Standard Compliance**: ✅ This specification is canonical, deterministic, and implementation-agnostic. It guarantees identical behavior across different AI systems, models, and coding harnesses. No human interpretation required.

**Deterministic Guarantees**:
- All Kubernetes resources explicitly defined
- Resource limits and requests specified
- Security contexts standardized
- Health check endpoints defined
- All labels follow Kubernetes recommended conventions
