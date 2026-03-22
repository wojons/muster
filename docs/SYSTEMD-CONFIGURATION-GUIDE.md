# Systemd Configuration Guide

**Status**: Complete Guide (2026-03-12)
**Purpose**: Configure systemd service units for OpenAPI CLI, daemon, and MCP server
**Scope**: Multi-service setup with security hardening, resource limits, and socket activation

## Overview

This guide provides complete instructions for configuring systemd service units for the OpenAPI CLI suite including the main CLI, daemon service, MCP server, and socket activation.

---

## Service Architecture

### Service Components

```
openapi-cli.service             # Main CLI initialization service
openapi-cli-daemon.service      # Daemon service (HTTP/gRPC API)
openapi-cli.socket              # Socket activation for daemon
openapi-cli-mcp.service         # MCP server service
```

### Service Dependencies

```
openapi-cli.socket (activation)
    ↓
openapi-cli-daemon.service (HTTP/gRPC API)
    ↓
openapi-cli.service (init)
```

---

## Systemd Service Files

### Service File Locations

**System-wide installation**:
```
/usr/local/lib/systemd/system/          # System services
/etc/systemd/system/                   # Custom overrides
/usr/lib/systemd/system/               # Package-provided services
```

**User-level installation**:
```
~/.config/systemd/user/                # User services
```

### Service Files

#### 1. openapi-cli-daemon.service

```ini
[Unit]
Description=OpenAPI CLI Daemon
Documentation=https://docs.openapi-cli.io
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
NotifyAccess=all
User=muster
Group=muster
ExecStart=/usr/bin/openapi-cli daemon --config /etc/openapi-cli/config.yaml
ExecStartPre=/bin/mkdir -p /run/muster
ExecStartPost=/bin/chown -R muster:muster /run/muster
Restart=on-failure
RestartSec=5s

# Security Hardening
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/run/muster /var/lib/openapi-cli /var/log/openapi-cli

# Resource Limits
MemoryLimit=512M
CPUQuota=200%

# Capability filtering
CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_NET_RAW

# Seccomp
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictRealtime=true

[Install]
WantedBy=multi-user.target
```

**Key Settings**:
- `Type=notify`: Service type for systemd-aware daemon
- `NotifyAccess=all`: Allows daemon to notify systemd of state changes
- `Restart=on-failure`: Automatically restart on failure
- `ProtectSystem=strict`: Read-only root filesystem
- `ProtectHome=true`: No access to home directory
- `MemoryLimit=512M`: 512MB memory limit
- `CPUQuota=200%`: 2 CPU cores max
- `CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_NET_RAW`: Minimal Linux capabilities
- `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`: Network family restrictions
- `RestrictRealtime=true`: Disallow real-time scheduling

#### 2. openapi-cli.socket (Socket Activation)

```ini
[Unit]
Description=OpenAPI CLI Socket
Documentation=https://docs.openapi-cli.io

[Socket]
ListenStream=/run/muster/daemon.sock
SocketGroup=muster
SocketMode=0660

[Install]
WantedBy=sockets.target
```

**Key Settings**:
- `ListenStream=/run/muster/daemon.sock`: Unix socket path
- `SocketGroup=muster`: Socket group ownership
- `WantedBy=sockets.target`: Socket activation target

#### 3. openapi-cli-mcp.service

```ini
[Unit]
Description=OpenAPI CLI MCP Server
Documentation=https://docs.openapi-cli.io
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=muster
Group=muster
ExecStart=/usr/bin/openapi-cli-mcp --config /etc/openapi-cli/mcp-config.yaml
Restart=on-failure
RestartSec=5s

# Security Hardening
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/run/muster /var/lib/openapi-cli /var/log/openapi-cli

# Resource Limits
MemoryLimit=256M
CPUQuota=100%

# Capability filtering
CapabilityBoundingSet=CAP_NET_BIND_SERVICE

# Seccomp
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6
RestrictRealtime=true

[Install]
WantedBy=multi-user.target
```

#### 4. openapi-cli.service

```ini
[Unit]
Description=OpenAPI CLI Service
Documentation=https://docs.openapi-cli.io
After=network.target openapi-cli-daemon.service

[Service]
Type=oneshot
RemainAfterExit=yes
User=root
ExecStart=/usr/bin/openapi-cli init --config /etc/openapi-cli/config.yaml
TimeoutStopSec=5

[Install]
WantedBy=multi-user.target
```

**Key Settings**:
- `Type=oneshot`: One-shot service for initialization
- `RemainAfterExit=yes`: Service considered active after exit
- `ExecStart`: Run initialization command at boot

---

## Installation and Setup

### Service Installation

#### Copy Service Files

```bash
# Create systemd directory
sudo mkdir -p /usr/local/lib/systemd/system

# Copy service files
sudo cp packaging/systemd/*.service /usr/local/lib/systemd/system/
sudo cp packaging/systemd/*.socket /usr/local/lib/systemd/system/

# Set permissions
sudo chmod 644 /usr/local/lib/systemd/system/openapi-cli*.service
sudo chmod 644 /usr/local/lib/systemd/system/openapi-cli*.socket

# Reload systemd daemon
sudo systemctl daemon-reload
```

#### Create Service User

```bash
# Create dedicated user for services
sudo adduser --system \
  --no-create-home \
  --home /var/lib/openapi-cli \
  --group \
  --shell /usr/sbin/nologin \
  muster

# Create required directories
sudo mkdir -p /run/muster
sudo mkdir -p /var/lib/openapi-cli
sudo mkdir -p /var/log/openapi-cli
sudo mkdir -p /etc/openapi-cli

# Set ownership and permissions
sudo chown -R muster:muster /run/muster
sudo chown -R muster:muster /var/lib/openapi-cli
sudo chown -R root:muster /etc/openapi-cli

# Set restrictive permissions
sudo chmod 750 /run/muster
sudo chmod 750 /var/lib/openapi-cli
sudo chmod 750 /etc/openapi-cli
```

#### Create Configuration Files

**Daemon configuration** (`/etc/openapi-cli/config.yaml`):
```yaml
cache:
  enabled: true
  max_size_mb: 100

daemon:
  socket_path: /run/muster/daemon.sock
  worker_threads: 4
  request_timeout: 30s

logging:
  level: info
  format: json
  file: /var/log/openapi-cli/openapi-cli.log

auth:
  enable_oauth2: true

security:
  enable_mtls: false
```

**MCP configuration** (`/etc/openapi-cli/mcp-config.yaml`):
```yaml
spec_path: /var/lib/openapi-cli/specs
base_url: ""

logging:
  level: info
  file: /var/log/openapi-cli/mcp.log

tools:
  enabled: true
  timeout: 30s

resources:
  enabled: false

prompts:
  enabled: false
```

### Enable and Start Services

```bash
# Enable all services
sudo systemctl enable openapi-cli.socket
sudo systemctl enable openapi-cli-daemon.service
sudo systemctl enable openapi-cli-mcp.service

# Start all services
sudo systemctl start openapi-cli.socket
sudo systemctl start openapi-cli-daemon.service
sudo systemctl start openapi-cli-mcp.service

# Verify status
sudo systemctl status openapi-cli.socket
sudo systemctl status openapi-cli-daemon.service
sudo systemctl status openapi-cli-mcp.service
```

---

## Service Configuration Options

### Type=notify vs Type=simple

#### Type=notify (Recommended for daemon)

```ini
[Service]
Type=notify
NotifyAccess=all
ExecStart=/usr/bin/openapi-cli daemon
```

**Advantages**:
- Service reports startup completion via `sd_notify()`
- Systemd knows when service is ready
- Proper state tracking
- Better dependency handling

**Requirements**:
- Daemon must use `libsystemd` or socket notification
- Service must send `READY=1` notification

#### Type=simple

```ini
[Service]
Type=simple
ExecStart=/usr/bin/openapi-cli-mcp
```

**Advantages**:
- No notification requirements
- Simpler setup

**Disadvantages**:
- Systemd doesn't know when service is ready
- Dependencies may start prematurely
- No proper state tracking

### Restart Policies

```ini
# Restart on failure with 5s delay
Restart=on-failure
RestartSec=5s

# Restart always (even on success)
Restart=always

# Restart on crash only (non-zero exit)
Restart=on-abnormal

# No restart
Restart=no
```

### ExecStartPre and ExecStartPost

```ini
ExecStartPre=/bin/mkdir -p /run/muster
ExecStartPre=/bin/bash -c 'test -f /etc/openapi-cli/config.yaml || exit 1'

ExecStartPost=/bin/chown -R muster:muster /run/muster
ExecStartPost=/usr/bin/systemctl notify openapi-cli-daemon.service 'READY=1'
```

### Environment Variables

```ini
[Service]
Environment="LOG_LEVEL=info"
EnvironmentFile=/etc/default/openapi-cli
Environment=OPENAPI_CLI_CONFIG=/etc/openapi-cli/config.yaml
Environment=OPENAPI_CLI_CACHE_DIR=/var/lib/openapi-cli/cache
```

---

## Security Hardening

### Minimal Capabilities

```ini
# Allow only necessary capabilities
CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_NET_RAW
```

**Available capabilities**:
- `CAP_NET_BIND_SERVICE`: Bind to privileged ports (<1024)
- `CAP_NET_RAW`: Raw socket access (for custom protocols)
- `CAP_CHOWN`: Change file ownership/directory creation
- `CAP_DAC_OVERRIDE`: Override file permissions

### Restrictive Filesystem Access

```ini
# Read-only root filesystem
ProtectSystem=strict

# No home directory access
ProtectHome=true

# No access to /sys, /proc
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true

# Private /tmp
PrivateTmp=true

# No access to hardware devices
PrivateDevices=true
```

### Network Restrictions

```ini
# Allow only Unix/IPv4/IPv6
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6

# Disable device access
DevicePolicy=closed
DeviceAllow=/dev/null rw
DeviceAllow=/dev/zero rw

# AppArmor/SELinux integration
# AppArmorProfile=openapi-cli-daemon
# SELinuxContext=system_u:object_r:openapi_cli_daemon_exec_t:s0
```

### Resource Limits

```ini
# Memory limit
MemoryLimit=512M

# CPU quota (percentage)
CPUQuota=200%

# File descriptor limit
LimitNOFILE=10000

# Process limit
LimitNPROC=100

# Stack size
LimitSTACK=8M

# Swap limit (requires systemd 236+)
MemorySwapMax=256M
```

### NoNewPrivileges

```ini
# Prevent SETUID/SETGID
NoNewPrivileges=true
```

**Effect**:
- Disallows new privileges for child processes
- Prevents privilege escalation
- Hardens against credential leaks

---

## Socket Activation

### How Socket Activation Works

```
Client Requests → Systemd → Socket → Spawns openapi-cli-daemon.service
                    ↑
                Socket activation
```

**Workflow**:
1. Client connects to `/run/muster/daemon.sock`
2. Systemd creates socket and listens
3. On connection, systemd starts `openapi-cli-daemon.service`
4. Inherits socket from systemd
5. Service runs until no connections

**Advantages**:
- Daemon starts on-demand (faster boot)
- No idle daemon processes
- Automatic scaling
- Better resource utilization

### Enable Socket Activation

```bash
# Stop direct service start
sudo systemctl disable openapi-cli-daemon.service
sudo systemctl stop openapi-cli-daemon.service

# Enable socket activation
sudo systemctl enable openapi-cli.socket
sudo systemctl start openapi-cli.socket

# Verify socket is listening
sudo ss -lp | grep openapi-cli
```

**Output**:
```
unix  LISTEN  0  128  /run/muster/daemon.sock

Users:(("systemd",pid=1,fd=11))
```

### Daemon Socket Inheritance

```go
// OpenAPI CLI daemon must inherit socket
package main

import (
    "os"
    "net"
    "github.com/coreos/go-systemd/daemon"
)

func main() {
    // Check for socket activation
    if passed := os.Getenv("LISTEN_FDS"); passed != "" {
        fd := 3 // systemd passes socket as FD 3
        listener, err := net.FileListener(os.NewFile(uintptr(fd), "daemon.sock"))
        if err != nil {
            log.Fatalf("socket activation: %v", err)
        }
        // Use listener for incoming connections
        defer listener.Close()
    }
}
```

---

## Service Management Commands

### Service Control

```bash
# Start service
sudo systemctl start openapi-cli-daemon.service

# Stop service
sudo systemctl stop openapi-cli-daemon.service

# Restart service
sudo systemctl restart openapi-cli-daemon.service

# Reload service (graceful restart)
sudo systemctl reload openapi-cli-daemon.service

# Enable service (starts on boot)
sudo systemctl enable openapi-cli-daemon.service

# Disable service (doesn't start on boot)
sudo systemctl disable openapi-cli-daemon.service
```

### Service Status

```bash
# Check status
sudo systemctl status openapi-cli-daemon.service

# Check if enabled
systemctl is-enabled openapi-cli-daemon.service

# Check if active
systemctl is-active openapi-cli-daemon.service

# Check failure status
systemctl is-failed openapi-cli-daemon.service

# List all service files
systemctl list-unit-files | grep openapi-cli
```

### Journal Logs

```bash
# View service logs
sudo journalctl -u openapi-cli-daemon.service

# View last 100 lines
sudo journalctl -u openapi-cli-daemon.service -n 100

# Follow logs in real-time
sudo journalctl -u openapi-cli-daemon.service -f

# View logs since boot
sudo journalctl -u openapi-cli-daemon.service --no-pager | tail -100

# View logs from last boot
sudo journalctl -u openapi-cli-daemon.service --no-pager -b 0

# Export logs
sudo journalctl -u openapi-cli-daemon.service --output=cat > openapi-cli-daemon.log
```

### Service Inspection

```bash
# Show service file contents
systemctl cat openapi-cli-daemon.service

# Show service dependencies
systemctl list-dependencies openapi-cli-daemon.service

# Show service properties
systemctl show openapi-cli-daemon.service

# Show process tree
systemctl status openapi-cli-daemon.service --no-pager

# Show resource usage
systemctl show openapi-cli-daemon.service -p CPU
systemctl show openapi-cli-daemon.service -p Memory
```

---

## Configuration Overrides

### Local Overrides (Recommended)

```bash
# Create override directory
sudo mkdir -p /etc/systemd/system/openapi-cli-daemon.service.d/

# Create override file
sudo vim /etc/systemd/system/openapi-cli-daemon.service.d/override.conf
```

**Override file** (`/etc/systemd/system/openapi-cli-daemon.service.d/override.conf`):
```ini
[Service]
Environment="LOG_LEVEL=debug"
MemoryLimit=1G
CPUQuota=300%
```

**Apply changes**:
```bash
sudo systemctl daemon-reload
sudo systemctl restart openapi-cli-daemon.service
```

### Drop-in Configuration

**Multiple override files**:
```ini
# /etc/systemd/system/openapi-cli-daemon.service.d/custom-config.conf
[Service]
Environment="CONFIG_FILE=/etc/openapi-cli/custom.yaml"

# /etc/systemd/system/openapi-cli-daemon.service.d/debug.conf
[Service]
Environment="LOG_LEVEL=debug"

# /etc/systemd/system/openapi-cli-daemon.service.d/resources.conf
[Service]
MemoryLimit=2G
```

**List drop-ins**:
```bash
systemd-analyze cat-config systemd/system/openapi-cli-daemon.service
```

---

## Service Templates

### Multi-Instance Service

```ini
[Unit]
Description=OpenAPI CLI Daemon %i
Documentation=https://docs.openapi-cli.io

[Service]
Type=simple
User=muster
ExecStart=/usr/bin/openapi-cli daemon --instance %i --config /etc/openapi-cli/config-%i.yaml

[Install]
WantedBy=multi-user.target
```

**Start specific instance**:
```bash
sudo systemctl start openapi-cli-daemon@production.service
sudo systemctl start openapi-cli-daemon@staging.service
```

---

## Monitoring and Debugging

### Service Monitoring

```bash
# Show service PIDs
systemctl show openapi-cli-daemon.service -p MainPID

# Show memory usage
systemctl show openapi-cli-daemon.service -p MemoryCurrent

# Show memory limit
systemctl show openapi-cli-daemon.service -p MemoryLimit

# Show CPU usage
systemctl show openapi-cli-daemon.service -p CPUUsageNSec

# Show uptime
systemctl show openapi-cli-daemon.service -p ActiveEnterTimestamp
```

### Service Performance

```bash
# Check service startup time
systemd-analyze time openapi-cli-daemon.service

# Check boot sequence graph
systemd-analyze plot > openapi-cli-daemon.svg

# Check critical-chain
systemd-analyze critical-chain
systemd-analyze critical-chain openapi-cli-daemon.service
```

### Service Debugging

**Add debug logging to service**:
```bash
sudo systemctl edit openapi-cli-daemon.service
```

**Add to override**:
```ini
[Service]
Environment="LOG_LEVEL=debug"
StandardOutput=journal+console
StandardError=journal+console
```

**View binary logs**:
```bash
journalctl --all --full -u openapi-cli-daemon.service
```

---

## Advanced Topics

### Journal Namespace

```ini
[Service]
LogNamespace=openapi-cli
```

**View logs**:
```bash
journalctl --namespace=openapi-cli -u openapi-cli-daemon.service
```

### Slice Management

```ini
[Service]
Slice=system-openapi-cli.slice
```

**Create slice**:
```ini
[Unit]
Description=OpenAPI CLI Slice

[Slice]
MemoryHigh=800M
MemoryMax=1G
CPUWeight=100
```

### Runtime Configuration

```bash
# Allow dynamic service reload
sudo systemctl reload openapi-cli-daemon.service

# Send SIGHUP to reload configuration
sudo kill -HUP $(systemctl show openapi-cli-daemon.service -p MainPID --value)
```

### Service Notify Integration

```go
import (
    "github.com/coreos/go-systemd/daemon"
)

// Send READY=1 notification
daemon.SdNotify(false, "READY=1")

// Send status update
daemon.SdNotify(false, "STATUS=Processing requests")

// Send watchdog ping
daemon.SdNotify(false, "WATCHDOG=1")
```

---

## Troubleshooting

### Service Won't Start

```bash
# Check service logs
sudo journalctl -u openapi-cli-daemon.service -n 50 --no-pager

# Check configuration file syntax
openapi-cli config validate /etc/openapi-cli/config.yaml

# Check socket permissions
ls -la /run/muster/daemon.sock

# Check service user permissions
id muster
sudo -u muster openapi-cli daemon --config /etc/openapi-cli/config.yaml
```

### Socket Activation Not Working

```bash
# Check if socket file exists
ls -la /run/muster/daemon.sock

# Check socket unit status
sudo systemctl status openapi-cli.socket

# Check socket listening
sudo ss -lp | grep openapi-cli

# Verify daemon inherits socket correctly
sudo journalctl -u openapi-cli-daemon.service | grep LISTEN_FDS
```

### Permission Errors

```bash
# Check file permissions
sudo ls -al /etc/openapi-cli/config.yaml
sudo ls -al /run/muster
sudo ls -al /var/lib/openapi-cli

# Check SELinux context (if enabled)
ls -Z /etc/openapi-cli/config.yaml

# Check AppArmor (if enabled)
sudo aa-status | grep openapi-cli

# Relabel SELinux context
sudo restorecon -R /etc/openapi-cli/
sudo restorecon -R /var/lib/openapi-cli/
```

### Resource Limits Exceeded

```bash
# Check current resource usage
systemctl show openapi-cli-daemon.service -p MemoryCurrent
systemctl show openapi-cli-daemon.service -p CPUUsageNSec

# Increase limits
sudo systemctl set-property openapi-cli-daemon.service MemoryLimit=2G
sudo systemctl set-property openapi-cli-daemon.service CPUQuota=300%

# Reset to defaults
sudo systemctl reset-failed openapi-cli-daemon.service
```

---

## Summary

This guide provides complete systemd configuration for:
- ✅ Daemon service with socket activation
- ✅ MCP server service
- ✅ Initialization service
- ✅ Security hardening (capabilities, seccomp, NoNewPrivileges)
- ✅ Resource limits (memory, CPU, file descriptors)
- ✅ Service management (start/stop/restart/enable/disable)
- ✅ Socket activation for on-demand service spawning
- ✅ Journal logging and debugging
- ✅ Configuration overrides and drop-ins
- ✅ Monitoring and performance analysis

**Security Hardening Summary**:
- Minimal capabilities (CAP_NET_BIND_SERVICE only)
- Read-only root filesystem (ProtectSystem=strict)
- No home directory access (ProtectHome=true)
- Private /tmp directory (PrivateTmp=true)
- No hardware device access (PrivateDevices=true)
- Limited network families (RestrictAddressFamilies)
- Restricted resource usage (MemoryLimit, CPUQuota)

**Socket Activation Benefits**:
- On-demand daemon spawning
- Faster boot times
- No idle processes
- Automatic resource scaling
- Better resource utilization

**Next Steps**:
1. Copy service files to `/usr/local/lib/systemd/system/`
2. Create `muster` user and directories
3. Install and configure services
4. Enable and start services
5. Monitor service status and logs
6. Adjust resources and permissions as needed

**Last Updated**: 2026-03-12
