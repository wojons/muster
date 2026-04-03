# Docker Image Hardening Documentation

## Overview

This document describes the security hardening measures implemented for Muster Docker images following industry best practices and security standards.

## Security Measures Implemented

### 1. Base Image Security

- **Distroless Base Image**: Uses `gcr.io/distroless/static:nonroot` which contains only the application and its runtime dependencies
  - No shell, package manager, or unnecessary tools
  - Minimal attack surface
  - Non-root user (65532:65532)

- **Image Digest Pinning**: All base images use SHA256 digests for reproducible builds
  - Prevents supply chain attacks via compromised tags
  - Ensures build reproducibility

### 2. Build Security

- **Multi-stage builds**: Separates build tools from runtime image
- **Non-root build user**: Build process runs as unprivileged user
- **Layer caching**: Dependencies cached separately from source code
- **Security scanning**: Integrated Trivy vulnerability scanning during build
- **SBOM generation**: Software Bill of Materials generated for supply chain transparency
- **Secret scanning**: Automated detection of exposed secrets in binaries

### 3. Runtime Security

- **Non-root execution**: Container runs as user 65532:65532
- **Read-only root filesystem**: `--read-only` flag prevents runtime modifications
- **No new privileges**: `--security-opt no-new-privileges:true` prevents privilege escalation
- **Capability dropping**: All capabilities dropped except NET_BIND_SERVICE (if needed)
- **Seccomp profile**: Custom seccomp profile limiting available syscalls
- **AppArmor profile**: Uses docker-default profile
- **Resource limits**: CPU and memory limits enforced
- **Network isolation**: Custom bridge network with encryption

### 4. Security Labels

OCI-compliant labels added to image:
- `security.scan-date`: When security scan was performed
- `security.base-image`: Base image used
- `security.non-root`: Indicates non-root execution
- `security.read-only-rootfs`: Indicates read-only filesystem
- `security.no-new-privileges`: Indicates no-new-privileges flag
- `security.distroless`: Indicates distroless base
- `security.sbom-generated`: Indicates SBOM availability

Compliance labels:
- `compliance.gdpr`: GDPR compliance status
- `compliance.soc2`: SOC 2 compliance status
- `compliance.fips-140-2`: FIPS 140-2 compliance status

### 5. Docker Compose Security

The `docker-compose.security.yml` provides:
- Resource limits (CPU: 1.0, Memory: 512MB)
- Read-only root filesystem
- Capability dropping
- Security options (no-new-privileges, seccomp, apparmor)
- Health checks
- Logging configuration
- Tmpfs with noexec/nosuid
- Volume mounts with read-only option

### 6. Secrets Management

- `.dockerignore` excludes:
  - All `.env` files
  - SSH keys and GPG directories
  - Cloud provider credentials
  - Kubernetes configs
  - Build artifacts and test files
  - Development files

## Building the Hardened Image

```bash
# Build with security scanning
docker build \
  --build-arg VERSION=$(git describe --tags) \
  --build-arg GIT_COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  -t muster:secure \
  .

# Run with security options
docker run \
  --read-only \
  --user 65532:65532 \
  --security-opt no-new-privileges:true \
  --security-opt seccomp=deployments/seccomp-profile.json \
  --cap-drop ALL \
  --cap-add NET_BIND_SERVICE \
  --memory 512m \
  --cpus 1.0 \
  muster:secure
```

## Using Docker Compose

```bash
# Deploy with security hardening
docker-compose -f deployments/docker-compose.security.yml up -d

# Verify security settings
docker inspect muster-muster-1 --format '{{json .HostConfig.SecurityOpt}}'
docker inspect muster-muster-1 --format '{{json .Config.Labels}}'
```

## Security Verification

### Check Image Security Labels

```bash
docker inspect muster:latest --format '
Image: {{.Config.Image}}
Non-root: {{index .Config.Labels "security.non-root"}}
Read-only: {{index .Config.Labels "security.read-only-rootfs"}}
No-new-privs: {{index .Config.Labels "security.no-new-privileges"}}
Distroless: {{index .Config.Labels "security.distroless"}}
'
```

### Verify Non-root Execution

```bash
docker run --rm muster:latest id
# Expected: uid=65532 gid=65532
```

### Verify Read-Only Filesystem

```bash
docker run --rm --read-only muster:latest touch /test
# Expected: touch: cannot touch '/test': Read-only file system
```

### Scan for Vulnerabilities

```bash
# Using Trivy
trivy image muster:latest

# Using Grype
grype muster:latest
```

## Security Checklist

- [ ] Image uses distroless base
- [ ] Image digests are pinned
- [ ] No secrets in image
- [ ] .dockerignore configured
- [ ] Non-root user configured
- [ ] Read-only filesystem enforced
- [ ] No-new-privileges set
- [ ] Capabilities dropped
- [ ] Seccomp profile applied
- [ ] Resource limits set
- [ ] Health checks configured
- [ ] Security labels present
- [ ] SBOM generated
- [ ] Vulnerability scan passing

## Compliance

This hardening implementation addresses:

- **CIS Docker Benchmark**: Sections 4.1, 4.3, 4.6, 4.7, 4.9
- **NIST 800-190**: Container security requirements
- **GDPR Article 32**: Security of processing
- **SOC 2 Type II**: CC6.1 (logical access security)

## References

- [Docker Security Best Practices](https://docs.docker.com/develop/security-best-practices/)
- [Distroless Images](https://github.com/GoogleContainerTools/distroless)
- [CIS Docker Benchmark](https://www.cisecurity.org/benchmark/docker)
- [OCI Image Specification](https://github.com/opencontainers/image-spec)
