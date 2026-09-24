> **⚠️ ASPIRATIONAL / NOT YET AVAILABLE** — This guide documents a planned package repository that is not yet operational. The package repository domain (packages.example.com) is a placeholder. For now, install Muster from source using `make build-cli`. See the main [README](../README.md#quick-start) for current installation instructions.

# Linux Installation Guide

**Status**: Framework Documentation created (2026-03-10)
**Platform**: Linux (Debian/Ubuntu, RHEL/CentOS, Alpine, Arch)
**Scope**: Package manager integration and service installation

## Overview

This guide covers installation of Muster (OpenAPI CLI + MCP Server) on Linux platforms including package manager integration and service setup.

## Prerequisites

### System Requirements

**OS**: Linux (Debian/Ubuntu, RHEL/CentOS, Alpine, Arch Linux)
**Architecture**: amd64, arm64, armv7
**Memory**: 512MB minimum (1GB recommended)
**Go**: Version 1.25.0 or higher (for building from source)
**Disk**: 100MB for binary + 50MB runtime

**Dependencies (build from source)**: `pkg-config` + `libseccomp-dev`
(Debian/Ubuntu) — a full-module build (`go build ./...`, `go vet ./...`,
`go test ./...`) compiles the cgo seccomp platform package (`pkg/platform` →
`github.com/seccomp/libseccomp-golang`), whose `#cgo pkg-config: libseccomp`
directive invokes `pkg-config --cflags -- libseccomp`; without these system
packages the build fails while compiling `pkg/platform` with
`github.com/seccomp/libseccomp-golang: exec: "pkg-config": executable file
not found in $PATH` (missing `pkg-config`) or `Package libseccomp was not
found in the pkg-config search path` (missing `libseccomp-dev`, i.e. the
`libseccomp.pc` file). RHEL/CentOS/Fedora equivalent:
`sudo dnf install -y pkgconf-pkg-config libseccomp-devel`.

```bash
# Debian/Ubuntu — required before building/testing from source
sudo apt-get install -y pkg-config libseccomp-dev
```

> **Note:** `make build-cli` alone does not import the seccomp package, but
> tests (`go test ./...`) and full-module builds always do — install both
> packages before contributing or running CI-equivalent builds. The SQLite
> driver (`github.com/mattn/go-sqlite3`) bundles its own SQLite amalgamation,
> so building from source does **not** require `libsqlite3-dev` (and a
> binary/package install requires neither).

### Pre-Installation Verification

```bash
# Check OS distribution
cat /etc/os-release

# Check architecture
uname -m

# Check available memory
free -h

# Check disk space
df -h /

# Check for SQLite development libraries (Ubuntu/Debian)
dpkg -l | grep libsqlite3-dev

# Check for SQLite development libraries (RHEL/CentOS)
rpm -q sqlite-devel

# Verify Go version (if building from source)
go version
```

## Package Installation

### Debian/Ubuntu

#### Using APT Repository

```bash
# Add OpenAPI CLI repository
curl -fsSL https://packages.example.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/openapi-cli-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/openapi-cli-archive-keyring.gpg] https://packages.example.com/ $(. /etc/os-release && echo $VERSION_CODENAME) main" | sudo tee /etc/apt/sources.list.d/openapi-cli.list

# Install
sudo apt update
sudo apt install openapi-cli openapi-cli-mcp

# Enable and start service
sudo systemctl enable --now openapi-cli
```

#### Manual Installation (Binary)

```bash
# Download latest binary
wget https://github.com/wojons/muster/releases/download/v1.0.0/openapi-cli-linux-amd64
chmod +x openapi-cli-linux-amd64
sudo mv openapi-cli-linux-amd64 /usr/local/bin/openapi-cli

# Install systemd service
sudo mkdir -p /usr/local/lib/systemd/system
sudo cp systemd/openapi-cli.service /usr/local/lib/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now openapi-cli
```

### RHEL/CentOS

#### Using DNF/YUM Repository

```bash
# Add OpenAPI CLI repository
sudo rpm --import https://packages.example.com/gpg
sudo tee /etc/yum.repos.d/openapi-cli.repo <<'EOF'
[openapi-cli]
name=OpenAPI CLI Repository
baseurl=https://packages.example.com/packages/$releasever/$basearch/
enabled=1
gpgcheck=0
EOF

# Install
sudo dnf install openapi-cli openapi-cli-mcp

# Enable and start service
sudo systemctl enable --now openapi-cli
```

#### Manual Installation (Binary)

```bash
# Download latest binary
sudo dnf install -y wget
wget https://github.com/wojons/muster/releases/download/v1.0.0/openapi-cli-linux-amd64
chmod +x openapi-cli-linux-amd64
sudo mv openapi-cli-linux-amd64 /usr/local/bin/openapi-cli

# Install systemd service
sudo mkdir -p /usr/local/lib/systemd/system
sudo cp systemd/openapi-cli.service /usr/local/lib/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now openapi-cli
```

### Alpine

```bash
# Add OpenAPI CLI repository
sudo wget -O /etc/apk/keys/openapi-cli.rsa.pub https://packages.example.com/alpine/openapi-cli.rsa.pub
sudo echo "https://packages.example.com/alpine/main" | sudo tee -a /etc/apk/repositories

# Install
apk update
apk add openapi-cli openapi-cli-mcp

# Enable and start service (Alpine uses OpenRC)
rc-update add openapi-cli default
rc-service openapi-cli start
```

### Arch

#### Using Pacman (AUR)

```bash
# Install from AUR using yay or paru
yay -S openapi-cli-bin
sudo systemctl enable --now openapi-cli
```

#### Manual Installation (Binary)

```bash
# Download latest binary
wget https://github.com/wojons/muster/releases/download/v1.0.0/openapi-cli-linux-amd64
chmod +x openapi-cli-linux-amd64
sudo mv openapi-cli-linux-amd64 /usr/local/bin/openapi-cli

# Install systemd service
sudo mkdir -p /usr/local/lib/systemd/system
sudo cp systemd/openapi-cli.service /usr/local/lib/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now openapi-cli
```

## Service Configuration

### Systemd Service Setup

The OpenAPI CLI daemon is managed by systemd with full service configuration.

#### Service Files Location
```
/usr/local/lib/systemd/system/openapi-cli.service
/usr/local/lib/systemd/system/openapi-cli-daemon.service
/usr/local/lib/systemd/system/openapi-cli-mcp.service
```

#### Service Installation

```bash
# Install all service files
sudo mkdir -p /usr/local/lib/systemd/system
sudo cp packaging/systemd/*.service /usr/local/lib/systemd/system/
sudo chmod 644 /usr/local/lib/systemd/system/*.service
sudo systemctl daemon-reload

# Enable services
sudo systemctl enable openapi-cli.service
sudo systemctl enable openapi-cli-daemon.service
sudo systemctl enable openapi-cli-mcp.service

# Start services
sudo systemctl start openapi-cli
sudo systemctl start openapi-cli-daemon
sudo systemctl start openapi-cli-mcp
```

#### Service Configuration Files

Place configuration file at:
```
/etc/openapi-cli/config.yaml
```

Example configuration:
```yaml
# OpenAPI CLI Configuration
cache:
  enabled: true
  max_size_mb: 100

daemon:
  socket_path: /run/muster/daemon.sock
  user: muster

logging:
  level: info
  format: json
  file: /var/log/wojons/muster.log
```

### Managing Services

```bash
# Check status
sudo systemctl status openapi-cli

# Restart service
sudo systemctl restart openapi-cli

# View logs
sudo journalctl -u openapi-cli -f

# Enable/disable at boot
sudo systemctl enable openapi-cli
sudo systemctl disable openapi-cli

# Stop service
sudo systemctl stop openapi-cli
```

## Post-Installation Verification

```bash
# Verify binary installation
openapi-cli version
openapi-cli --help

# Verify service status
sudo systemctl status openapi-cli

# Verify socket creation
ls -la /run/muster/daemon.sock

# Verify directory structure
ls -la /var/lib/muster/
ls -la /etc/openapi-cli/

# Test basic functionality
openapi-cli generate --help
# (requires an OpenAPI spec file to generate commands)

# Test MCP server (in separate terminal)
openapi-cli-mcp --version
```

## Configuration

### Basic Configuration

Create configuration directory and file:
```bash
sudo mkdir -p /etc/openapi-cli
sudo vim /etc/openapi-cli/config.yaml
```

Example minimal configuration:
```yaml
cache:
  enabled: true
  max_size_mb: 100

daemon:
  socket_path: /run/muster/daemon.sock

logging:
  level: info
```

### Advanced Configuration

```yaml
cache:
  enabled: true
  max_size_mb: 100
  ttl_minutes: 60

daemon:
  socket_path: /run/muster/daemon.sock
  worker_threads: 4
  request_timeout: 30s

logging:
  level: info
  format: json
  file: /var/log/wojons/muster.log

auth:
  enable_oauth2: true
  oauth2_client_id: "${OAUTH_CLIENT_ID}"
  oauth2_client_secret: "${OAUTH_CLIENT_SECRET}"

security:
  enable_mtls: false
  cert_path: /etc/openapi-cli/certs/client.crt
  key_path: /etc/openapi-cli/certs/client.key
  ca_path: /etc/openapi-cli/certs/ca.crt
```

## Security Setup

### User and Directory Permissions

The daemon runs with limited privileges:

```bash
# Create dedicated user (if not created by package)
sudo adduser --system --no-create-home --home /var/lib/muster --group --shell /usr/sbin/nologin muster

# Create directories with proper permissions
sudo mkdir -p /var/lib/muster /var/run/muster /etc/openapi-cli /var/log/openapi-cli
sudo chown -R muster:muster /var/lib/muster /var/run/muster

# Set restrictive file permissions
sudo chmod 750 /var/lib/muster
sudo chmod 750 /var/run/muster
sudo chmod 750 /etc/openapi-cli
sudo chmod 640 /etc/openapi-cli/config.yaml
```

### AppArmor Profile

Install AppArmor profile for additional security:

```bash
# Install AppArmor profile
sudo cp packaging/apparmor/usr.bin.openapi-cli /etc/apparmor.d/
sudo apparmor_parser -r /etc/apparmor.d/usr.bin.openapi-cli
sudo systemctl restart apparmor
```

### Seccomp Filters

Seccomp BPF filters are applied automatically when using systemd service files that include:

```ini
[Service]
...
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictRealtime=true
RestrictNamespaces=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
```

## Troubleshooting

```bash
# Service status
sudo systemctl status openapi-cli

# Service logs
sudo journalctl -u openapi-cli -n 100

# Application logs
sudo tail -n 100 /var/log/wojons/muster.log

# Socket check
ls -la /run/muster/daemon.sock

# Test configuration
openapi-cli config validate

# Check cache status
openapi-cli cache stats
```

### Common Issues

**Issue**: Service fails to start

**Solution**:
```bash
# Check for permission issues
sudo ls -la /run/muster/
sudo ls -la /var/lib/muster/

# Check configuration file syntax
openapi-cli config validate /etc/openapi-cli/config.yaml

# Review service logs
sudo journalctl -u openapi-cli --no-pager | tail -50
```

**Issue**: Socket connection refused

**Solution**:
```bash
# Check if daemon is running
sudo systemctl status openapi-cli-daemon

# Check socket file exists and has correct permissions
ls -la /run/muster/daemon.sock

# Restart daemon
sudo systemctl restart openapi-cli-daemon
```

**Issue**: Permission denied errors

**Solution**:
```bash
# Ensure user exists
grep muster /etc/passwd

# Ensure directories are owned by muster user
sudo chown -R muster:muster /var/lib/muster /var/run/muster

# Check file permissions
sudo ls -la /etc/openapi-cli/config.yaml
```

**Packages**: Debian, RPM, Alpine, Arch

**Last Updated**: 2026-03-12

