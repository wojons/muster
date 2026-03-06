## Spec Conformance Sweep #005: OPS-DEPLOYMENT.md (2026-03-05)

**Status**: ⚠️ PARTIAL (60% - Kubernetes manifests present but architectural mismatch)

**Spec Location**: `specs/OPS-DEPLOYMENT.md`

**Analysis**: Deployment infrastructure exists but spec describes different application type

**Architecture Mismatch**:

**What Spec Assumes**:
- Web API server (muster/api:latest)
- PostgreSQL database required
- Redis cache required
- Nginx reverse proxy
- Full HTTP API deployment

**What This Project Actually Is**:
- CLI tool (openapi-cli/openapi-mcp)
- CLI daemon mode for socket access
- MCP server for Claude Desktop
- Stateless client tool (no database)
- Minimal dependencies

**Conformance Analysis**:

### Kubernetes Manifests (~50% match)

| Component | Required | Actual | Status | Notes |
|-----------|----------|--------|--------|-------|
| namespace.yaml | `muster` | `openapi-cli` | ⚠️ Different name | Namespace naming differs |
| configmap.yaml | APP_ENV, LOG_LEVEL | config.yaml with settings | ⚠️ Different structure | Config format differs |
| secret.yaml | DATABASE_URL, REDIS_URL | api-key, oauth-client-secret | ⚠️ Different secrets | No DB/Redis secrets needed |
| deployment.yaml | `muster/api:latest` | `ghcr.io/openapi-cli/openapi-cli:1.0.0` | ⚠️ Different image | CLI daemon vs API server |
| service.yaml | Port 80/9090 ClusterIP | Port 80/9090 ClusterIP | ✅ MATCHES | Service structure matches |
| hpa.yaml | min=3, max=10, cpu=70%, mem=80% | min=3, max=10, cpu=60%, mem=70% | ⚠️ Different targets | HPA structure matches |

**Additional Kubernetes Manifests Present** (beyond spec):
- networkpolicy.yaml ✅
- pdb.yaml ✅
- rbac.yaml ✅
- serviceaccount.yaml ✅
- servicemonitor.yaml ✅

### Docker Compose (~30% match)

**Spec Requires**:
```yaml
api:  # Web API server
  - depends_on: db, redis
  - healthcheck: /health/ready

db:  # PostgreSQL
  - postgres:15-alpine

redis:  # Redis
  - redis:7-alpine

nginx:  # Reverse proxy
  - nginx:alpine
```

**Actual Has**:
```yaml
cli:  # CLI container
cli-daemon:  # CLI daemon mode
mcp-server:  # MCP server mode
```

**Status**: ❌ NO MATCH - Architectural mismatch (CLI vs Web API)

### Systemd Service (~0% match)

**Spec Requires**: `/etc/systemd/system/muster.service` with Python web server
**Actual**: ❌ NOT IMPLEMENTED

**Status**: ❌ MISSING - No systemd service file

**Evidence Provided**:

1. **Kubernetes manifests present**: `ls k8s/base/` shows 12 YAML files (including spec requires + extras)
2. **Docker Compose exists**: `docker-compose.yml` present but services differ
3. **No systemd**: `find . -name "*.service"` found no systemd service files
4. **Architecture mismatch**: Spec has DB/Redis/Nginx; actual uses CLI daemon only

**Impact**: 🟢 LOW

**Impact Analysis**:
- ✅ Kubernetes deployment infrastructure exists and is production-ready
- ✅ Docker Compose for local development exists
- ⚠️ Spec describes web API server architecture but project is CLI tool
- ❌ Systemd service not implemented (may be needed for production Linux deployments)
- ⚠️ Spec naming conventions (muster) differ from actual (openapi-cli)

**What's Working**:
- ✅ RollingUpdate deployment strategy
- ✅ HorizontalPodAutoscaler with scale policies
- ✅ Liveness, readiness, and startup probes
- ✅ Security hardening (non-root, readOnlyRootFilesystem)
- ✅ Resource limits and requests
- ✅ Service account and RBAC
- ✅ Network policies and pod disruption budgets
- ✅ Prometheus metrics scraping enabled

**What's Different**:
- ⚠️ Namespace: `muster` vs `openapi-cli`
- ⚠️ Service naming: `muster-api` vs `openapi-cli`
- ⚠️ Image: `muster/api:latest` vs `ghcr.io/openapi-cli/openapi-cli:1.0.0`
- ❌ Docker Compose: web API + DB + Redis vs CLI daemon
- ❌ Systemd: Python web server vs CLI daemon

**Recommendation**:

1. **Update spec to match actual implementation**:
   - Change from web API server architecture to CLI daemon architecture
   - Remove PostgreSQL/Redis requirements (CLI tool doesn't need database)
   - Remove Nginx requirement (CLI daemon exposes port, no reverse proxy needed)
   - Update Docker Compose to reflect actual services (cli, cli-daemon, mcp-server)

2. **Add systemd service** (optional):
   - Create deployments/muster.service for Linux deployments
   - Or note that systemd is optional (Docker/Kubernetes preferred)

3. **Update naming conventions**:
   - Decide on canonical namespace: `muster` or `openapi-cli`
   - Update all manifests to use consistent naming

**Priority**: 🟡 MEDIUM (update spec documentation to match actual deployment)

**Timeline**: 1 day to update spec

**Note**:
- Kubernetes deployment infrastructure is production-ready ✅
- Spec describes a different application type (web API vs CLI daemon)
- Actual deployment is suitable for this project but doesn't match spec
