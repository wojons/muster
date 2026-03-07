# Spec Conformance Sweep #200 - PLATFORM-LINUX.md

**Date**: 2026-03-06
**Spec**: specs/PLATFORM-LINUX.md (2,919 lines)
**Category**: Platform-Specific Implementation - Linux
**Sweep Trigger**: Idle-Time Spec Conformance Sweep (no pending tasks)
**Total Requirements**: 22 major categories
**Implementation Status**: 0/22 implemented (0% conformance)

---

## Executive Summary

**CRITICAL GAP IDENTIFIED**: Complete absence of Linux platform implementation

This specification requires comprehensive Linux-specific integration across multiple domains:
- System integration (systemd, init.d, desktop)
- Package managers (Debian, RPM, Alpine, Arch)
- Keychain integration (Secret Service, GNOME Keyring, KWallet)
- File system (XDG directories, SELinux, permissions)
- Network integration (NetworkManager, proxy detection)
- Security hardening (AppArmor, Seccomp)

**Impact**: Linux distribution and native integration completely blocked
**Estimated Effort**: 6-9 weeks for full implementation
**Priority**: HIGH (blocks native Linux deployment)

---

## Detailed Analysis

### Section 1: System Integration ❌ NOT IMPLEMENTED

**Required Files (7, 0 implemented):**

| Requirement | File | Status | Evidence |
|-------------|------|--------|----------|
| systemd service | `/etc/systemd/system/muster-daemon.service` | ❌ Missing | No systemd files in project |
| systemd socket | `/etc/systemd/system/muster-daemon.socket` | ❌ Missing | No systemd files in project |
| systemd user service | `/etc/systemd/user/muster-daemon.service` | ❌ Missing | No systemd files in project |
| init.d script | `/etc/init.d/muster-daemon` | ❌ Missing | No init.d files in project |
| desktop entry | `/usr/share/applications/muster.desktop` | ❌ Missing | No desktop files in project |
| mime type | `/usr/share/mime/packages/muster.xml` | ❌ Missing | No mime files in project |

**Impact**: System cannot be installed as native Linux service

---

### Section 2: Package Managers ❌ NOT IMPLEMENTED

**Required Packaging (4 formats, 0 implemented):**

| Format | Location | Components Missing |
|--------|----------|-------------------|
| Debian | `packaging/deb/` | control, preinst, postinst, prerm, postrm, build-deb.sh |
| RPM | `packaging/rpm/` | muster.spec, build-rpm.sh |
| Alpine | `APKBUILD` | APKBUILD, pre/post install scripts |
| Arch | `PKGBUILD` | PKGBUILD, install scripts |

**Impact**: No Linux package distribution possible

---

### Section 3: Keychain Integration ❌ NOT IMPLEMENTED

**Required Implementations (3 backends, 0 implemented):**

| Backend | File | Status | Dependencies |
|---------|------|--------|--------------|
| Secret Service | `keyring_secret_service.go` | ❌ Missing | github.com/godbus/dbus/v5 |
| GNOME Keyring | `keyring_gnome.go` | ❌ Missing | secret-tool CLI |
| KWallet | `keyring_kwallet.go` | ❌ Missing | github.com/godbus/dbus/v5 |

**Impact**: No secure credential storage on Linux

**Current Alternative**: 
- Basic file-based credential storage in pkg/auth/
- No integration with platform keyring services

---

### Section 4: File System ❌ NOT IMPLEMENTED

**Required Files (3, 0 implemented):**

| Feature | File | Status |
|---------|------|--------|
| XDG Directories | `xdg.go` | ❌ Missing |
| Permissions | `permissions.go` | ❌ Missing |
| SELinux Contexts | `selinux.go` | ❌ Missing |

**Spec Requirements**:
- XDG Base Directory Specification compliance
- Secure file permissions (0600 files, 0700 directories)
- SELinux context management and policy installation
- Secure temp file creation and cleanup

**Current Alternative**:
- Basic filesystem operations in pkg/config/
- No XDG compliance
- No SELinux support

---

### Section 5: Network ❌ NOT IMPLEMENTED

**Required Files (2, 0 implemented):**

| Feature | File | Status | Dependencies |
|---------|------|--------|--------------|
| NetworkManager | `networkmanager.go` | ❌ Missing | github.com/godbus/dbus/v5 |
| Proxy Detection | `proxy.go` | ❌ None | None |

**Spec Requirements**:
- NetworkManager D-Bus integration for network state monitoring
- Proxy detection from environment (HTTP_PROXY, etc.)
- gsettings integration for GNOME proxy settings
- PAC file parsing support
- NO_PROXY list handling

**Current Alternative**:
- Basic HTTP client in pkg/client/
- No proxy support
- No network change detection

---

### Section 6: Security ❌ NOT IMPLEMENTED

**Required Files (2, 0 implemented):**

| Feature | File | Status | Dependencies |
|---------|------|--------|--------------|
| AppArmor | `packaging/apparmor/usr.bin.muster` | ❌ Missing | AppArmor userspace |
| Seccomp | `seccomp.go` | ❌ Missing | github.com/seccomp/libseccomp-golang |

**Spec Requirements**:
- AppArmor profile with:
  - File access restrictions
  - Capability denial
  - D-Bus rules for keyrings
  - Network access control
  - Syscall filtering
  
- Seccomp BPF filters with 3 profiles:
  - Minimal (essential syscalls only)
  - Standard (network operations)
  - Relaxed (WASM execution)

**Current Alternative**:
- No containerization support
- No syscall filtering
- No mandatory access control

---

## Root Cause Analysis

### Platform Directory Structure

```
internal/platform/
├── linux/          ❌ DOES NOT EXIST (should contain 8 Go files)
├── darwin/          ❌ Unknown if exists
├── windows/        ❌ Unknown if exists
└── common.go        ❌ Unknown if exists
```

**Test Result**: `internal/platform/` exists but is EMPTY (only 2 entries: . and ..)

### Why This Matters

**Business Impact**:
1. **Package Distribution**: Cannot create .deb, .rpm, .apk, or .arch packages
2. **System Integration**: Cannot run as systemd service with socket activation
3. **Desktop Integration**: No menu entries or file association
4. **Security Hardening**: No AppArmor/SELinux profiles for production
5. **Platform Integration**: No NetworkManager, proxy, or keychain support
6. **Linux Compliance**: Not XDG-compliant, no Fedora/Ubuntu packaging

**User Experience Impact**:
1. Binary-only distribution (no package managers)
2. Manual installation required
3. No system service support
4. No desktop integration
5. Reduced security on Linux

---

## Implementation Roadmap

### Phase 1: Platform Infrastructure (2-3 weeks) - PRIORITY 1

**Week 1-2: Core Platform Files**
```go
// internal/platform/linux/xdg.go (300 lines)
- XDGPaths struct with ConfigHome, CacheHome, DataHome, StateHome, RuntimeDir
- DefaultXDGPaths() with fallback logic
- MusterPaths() returning application-specific paths
- EnsureAll(), ConfigFile(), CacheFile(), DataFile(), StateFile(), RuntimeFile()
- SocketPath(), PidFile()

// internal/platform/linux/permissions.go (200 lines)
- SecureFileMode (0600), SecureDirMode (0700)
- SecureFile() with O_EXCL and permission fixing
- SecureMkdirAll() with umask handling
- IsSecure(), FixPermissions(), GetOwner(), IsOwnedByCurrentUser()
- SecureTempFile(), RemoveSecure()

// internal/platform/linux/secret_service.go (400 lines)
- SecretServiceKeyring struct implementing auth.Keyring
- NewSecretServiceKeyring() with D-Bus session connection
- Get(), Set(), Delete() methods
- openSession(), getDefaultCollection()
- handlePrompt() for UI interaction

// internal/platform/linux/gnome_keyring.go (200 lines)
- GnomeKeyring struct with secret-tool CLI wrapper
- IsAvailable() check
- Get(), Set(), Delete() using secret-tool
- Error handling for CLI fallbacks

// internal/platform/linux/kwallet.go (300 lines)
- KWalletKeyring struct with D-Bus connection
- NewKWalletKeyring() with wallet opening
- Get(), Set(), Delete() with folder management
- isCLIAvailable() fallback
```

**Week 3: Tests & Integration**
- Unit tests for all 5 files (goal: 80%+ coverage)
- Integration tests with real D-Bus services
- Platform detection tests
- Keyring backend selection logic

**Deliverables**:
- ✅ 5 platform implementation files (1,400 lines)
- ✅ Comprehensive test suite (600+ lines)
- ✅ Platform dependency validation

---

### Phase 2: Network & Security (1-2 weeks) - PRIORITY 2

**Week 1: Network Integration**
```go
// internal/platform/linux/networkmanager.go (500 lines)
- NetworkManager struct with D-Bus system connection
- NewNetworkManager() with signal subscription
- GetConnectivity(), GetActiveConnections()
- GetProxySettings() returning ProxySettings
- WatchStateChanges() with context cancellation
- NetworkState enum and ActiveConnection struct

// internal/platform/linux/proxy.go (300 lines)
- GetProxyFromEnvironment() with fallback chain
- GetNoProxyHosts(), ShouldUseProxy()
- CreateProxyTransport() with NO_PROXY support
- getProxyFromNetrc(), parsePAC()
- ConfigureProxyFromGsettings() for GNOME
- execCommand() helper
```

**Week 2: Security Hardening**
```go
// internal/platform/linux/seccomp.go (800 lines)
- SeccompProfile enum (Minimal, Standard, Relaxed)
- ApplySeccompFilter() with profile selection
- applyMinimalProfile() (essential syscalls only)
- applyStandardProfile() (network operations)
- applyRelaxedProfile() (WASM execution)
- addSyscalls() helper
- CheckSeccompSupport(), GenerateSeccompProfile()
- SeccompProfileForWASM() for containerization
```

**Deliverables**:
- ✅ Network integration with NetworkManager and proxy support
- ✅ Seccomp BPF filtering for sandboxing
- ✅ Network change detection
- ✅ Comprehensive tests

---

### Phase 3: Packaging (2-3 weeks) - MEDIUM PRIORITY

**Week 1-2: Debian & RPM Packaging**

Debian:
```bash
# packaging/deb/control
Package: muster
Section: utils
Priority: optional
Architecture: amd64
Depends: libc6 (>= 2.31), ca-certificates
Recommends: gnome-keyring | kwallet-cli

# packaging/deb/preinst
#!/bin/sh
set -e
if ! id -u muster >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /bin/false muster
fi

# packaging/deb/postinst
#!/bin/sh
set -e
mkdir -p /etc/muster /var/lib/muster /run/muster
chown -R muster:muster /var/lib/muster /run/muster
systemctl daemon-reload
systemctl enable muster-daemon.socket
```

RPM:
```spec
# packaging/rpm/muster.spec
Name:           muster
Version:        1.0.0
Release:        1%{?dist}
Summary:        Modern API Client and Development Tool
License:        MIT
BuildArch:      x86_64
Requires:       ca-certificates

%description
Muster is a powerful CLI tool for API development...

%install
install -D -m 755 muster %{buildroot}%{_bindir}/muster
install -D -m 644 systemd/muster-daemon.service %{buildroot}%{_unitdir}/
```

**Week 3: Alpine & Arch Packaging**

Alpine:
```bash
# packaging/alpine/APKBUILD
pkgname=muster
pkgver=1.0.0
pkgdesc="Modern API Client and Development Tool"
arch="x86_64 aarch64"
depends="ca-certificates"
install="muster.pre-install,muster.post-install"
```

Arch:
```bash
# packaging/arch/PKGBUILD
pkgname=muster
pkgver=1.0.0
pkgrel=1
pkgdesc="Modern API Client and Development Tool"
arch=('x86_64' 'aarch64')
depends=('glibc' 'ca-certificates')
```

**Build Scripts**:
- `scripts/build-deb.sh` (150 lines)
- `scripts/build-rpm.sh` (100 lines)
- `scripts/build-apk.sh` (80 lines)
- `scripts/build-arch.sh` (100 lines)

**Deliverables**:
- ✅ 4 complete packaging systems
- ✅ 4 build automation scripts
- ✅ Package signing infrastructure
- ✅ Package repository setup

---

### Phase 4: System Integration (1 week) - LOW PRIORITY

**Files Required**:

systemd:
```ini
# packaging/systemd/muster-daemon.service
[Unit]
Description=Muster API CLI Daemon
After=network.target
Wants=network.target

[Service]
Type=notify
ExecStart=/usr/bin/muster daemon --config /etc/muster/daemon.yaml
User=muster
Group=muster

[Install]
WantedBy=multi-user.target
```

init.d:
```bash
# packaging/init.d/muster-daemon
#!/bin/sh
### BEGIN INIT INFO
# Provides:          muster-daemon
# Required-Start:    $network $remote_fs
# Required-Stop:     $network $remote_fs
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Muster API CLI Daemon
### END INIT INFO

DAEMON=/usr/bin/muster
start-stop-daemon --start --quiet -m -p $PIDFILE ...
```

Desktop:
```ini
# packaging/desktop/muster.desktop
[Desktop Entry]
Version=1.0
Type=Application
Name=Muster CLI
Exec=muster %F
Icon=muster
Terminal=true
Categories=Development;Network;
```

AppArmor:
```conf
# packaging/apparmor/usr.bin.muster
#include <tunables/global>

/usr/bin/muster {
  #include <abstractions/base>
  
  # Configuration files
  /etc/muster/** r,
  owner @{HOME}/.config/muster/** rwk,
  
  deny capability sys_admin,
  deny capability sys_ptrace,
  deny /etc/shadow r,
  
  # Network access
  network inet stream,
  network inet6 stream,
}
```

**Deliverables**:
- ✅ System service support (systemd + init.d)
- ✅ Desktop integration
- ✅ AppArmor security profile
- ✅ SELinux policy (from spec)

---

## Testing Strategy

### Unit Tests (80%+ coverage goal)
- `internal/platform/linux/*_test.go` for each file
- Mock D-Bus connections for testing
- File system tests with temp directories
- Permission validation tests
- XDG path computation tests

### Integration Tests
- Real systemd service lifecycle tests
- AppArmor profile enforcement tests
- NetworkManager integration tests
- Package installation tests (using Docker containers)
- Keyring backend tests (requires mocking CLI tools)

### Platform Tests
- Test on Ubuntu LTS
- Test on Fedora latest
- Test on Debian stable
- Test on Alpine
- Test on Arch Linux

### Security Tests
- AppArmor profile enforcement
- Seccomp syscall filtering
- File permission verification
- SELinux context application
- Privilege escalation tests

---

## Dependencies Required

### Go Dependencies
```
github.com/godbus/dbus/v5    # D-Bus integration (Secret Service, KWallet, NetworkManager)
github.com/seccomp/libseccomp-golang  # Seccomp BPF filtering
```

### System Dependencies (runtime)
- `systemd` (for system service)
- `apparmor` (for security hardening)
- `libseccomp2` (for syscall filtering)
- `secret-tool` (for GNOME Keyring backend)
- `kwalletcli` (for KWallet CLI fallback)
- `NetworkManager` (for network state)

### Build Dependencies
- `dpkg-deb` (Debian package building)
- `rpmbuild` (RPM package building)
- `abuild` (Alpine package building)
- `makepkg` (Arch package building)

---

## Risk Assessment

### HIGH RISKS

**D-Bus Integration Complexity**
- Risk: D-Bus integration can be fragile across Linux distributions
- Mitigation: Provide fallback CLI tools for all backends, extensive testing

**Package Maintenance Burden**
- Risk: Maintaining 4 package formats creates high ongoing maintenance
- Mitigation: Automated build scripts, CI/CD packaging pipeline

**Security Profile Updates**
- Risk: Seccomp/AppArmor profiles need updates as code evolves
- Mitigation: Security review in pull requests, automated profile validation

### MEDIUM RISKS

**Distribution Compatibility**
- Risk: Features may not work across all Linux distributions
- Mitigation: Test on major distributions, fallback mechanisms

**Keyring Backend Availability**
- Risk: All 3 backends (Secret Service, GNOME, KWallet) may not be installed
- Mitigation: Runtime detection with fallback to file storage

### LOW RISKS

**Desktop Integration**
- Risk: Desktop integration rarely used by CLI tools
- Mitigation: Consider optional/deprecated feature

---

## Alternatives Considered

### Option 1: Complete Implementation (RECOMMENDED)
- **Effort**: 6-9 weeks
- **Benefit**: Full Linux native integration
- **Use Case**: Production Linux deployments

### Option 2: Minimal Implementation Only
- **Effort**: 2-3 weeks (just XDG, permissions, basic keyring)
- **Benefit**: Core Linux functionality without packaging
- **Use Case**: Binary-only distributions

### Option 3: External Package Scripts Only
- **Effort**: 1-2 weeks (just packaging without platform code)
- **Benefit**: Package manager support without platform integration
- **Use Case**: Teams managing their own infrastructure

### Option 4: Defer to Phase 2+
- **Effort**: 0 weeks (not implement now)
- **Benefit**: Focus on core features first
- **Use Case**: Initial release with binary-only distribution

---

## Recommendations

### Immediate Actions (This Phase)

1. **Priority 1**: Create `internal/platform/linux/` directory with core files (xdg, permissions)
   - Rationale: Foundation for all other platform work
   - Time: 1-2 days

2. **Priority 2**: Implement keychain integration (Secret Service, GNOME, KWallet)
   - Rationale: Security requirement, cannot defer
   - Time: 2-3 days

3. **Priority 3**: Document binary-only Linux installation guide
   - Rationale: Provide path for users awaiting full implementation
   - Time: 1-2 days

### Short-term Actions (Next Phase)

4. Implement NetworkManager and proxy support
5. Choose packaging approach (Option 1-4 above)
6. Create systemd/init.d integration
7. Add AppArmor/Seccomp profiles

### Long-term Actions (Future Phases)

8. Desktop integration (optional)
9. SELinux policy creation
10. Package repository setup
11. Signed packages
12. Continuous integration for packaging

---

## Conclusion

**Spec Complexity**: EXTREME - This is one of the most complex specs in repository (2,919 lines)

**Implementation Gap**: COMPLETE - 0/22 requirements met (0% conformance)

**Strategic Fit**: HIGH - Blocks native Linux distribution and deployment

**Recommended Path**:
1. **Option 4** (Defer full implementation to Phase 2+)
2. **Document** current binary-only Linux support
3. **Focus** on core CLI/MCP features first

**Timeline**:
- **Phase 2+**: Full 6-9 week implementation
- **Now**: Document current state, create installation guides

**Decision Required**: Should this be Phase 2+ work or must be in Phase 1?

---

## Evidence

### Directory Structure Check
```bash
$ ls -la internal/platform/
total 0
drwxr-xr-x@  2 lexykwaii  staff   64 Mar  2 19:57 .
drwxr-xr-x@ 16 lexykwaii  staff  512 Mar  3 03:08 ..
```

**Conclusion**: `internal/platform/linux/` directory DOES NOT EXIST

### File Search for Linux Components
```bash
$ find . -name "*linux*" -type f | grep -E "(xdg|keyring|networkmanager|proxy)" | head -10
(No results)
```

**Conclusion**: Zero Linux-specific platform files present

### Go Dependencies Check
```bash
$ cat go.mod | grep -E "(dbus|seccomp)"
(No results)
```

**Conclusion**: No required dependencies for Linux platform integration

---

**Report Completion Date**: 2026-03-06
**Next Sweep**: Random spec from specs/ directory
**Sweep Count**: #200 (Idle-Time Spec Conformance Sweep)
