# Platform-Specific Implementation - Linux

Linux-specific implementation details for the Muster CLI tool.

---

## Table of Contents

1. [System Integration](#1-system-integration)
2. [Package Managers](#2-package-managers)
3. [Keychain Integration](#3-keychain-integration)
4. [File System](#4-file-system)
5. [Network](#5-network)
6. [Security](#6-security)

---

## 1. System Integration

### 1.1 systemd Service

Service unit for daemon mode:

```ini
# /etc/systemd/system/muster-daemon.service
[Unit]
Description=Muster API CLI Daemon
Documentation=https://muster.sh/docs/daemon
After=network.target
Wants=network.target

[Service]
Type=notify
ExecStart=/usr/bin/muster daemon --config /etc/muster/daemon.yaml
ExecReload=/bin/kill -HUP $MAINPID
KillMode=mixed
KillSignal=SIGTERM
TimeoutStopSec=30

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/muster /var/run/muster
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
RestrictRealtime=true
RestrictNamespaces=true
LockPersonality=true
MemoryDenyWriteExecute=true
SystemCallFilter=@system-service
SystemCallErrorNumber=EPERM

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

# User and group
User=muster
Group=muster

# Environment
Environment="MUSTER_HOME=/var/lib/muster"
Environment="MUSTER_LOG_LEVEL=info"
EnvironmentFile=-/etc/default/muster

# Notify systemd when ready
NotifyAccess=main

[Install]
WantedBy=multi-user.target
```

Socket activation unit:

```ini
# /etc/systemd/system/muster-daemon.socket
[Unit]
Description=Muster Daemon Socket
Documentation=https://muster.sh/docs/daemon

[Socket]
ListenStream=/run/muster/daemon.sock
SocketMode=0660
SocketUser=muster
SocketGroup=muster

[Install]
WantedBy=sockets.target
```

Systemd target for user-mode daemon:

```ini
# /etc/systemd/user/muster-daemon.service
[Unit]
Description=Muster User Daemon
Documentation=https://muster.sh/docs/daemon

[Service]
Type=simple
ExecStart=/usr/bin/muster daemon --user
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
```

### 1.2 init.d Script (Legacy Systems)

For systems without systemd:

```bash
#!/bin/sh
# /etc/init.d/muster-daemon
### BEGIN INIT INFO
# Provides:          muster-daemon
# Required-Start:    $network $remote_fs
# Required-Stop:     $network $remote_fs
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: Muster API CLI Daemon
# Description:       Provides HTTP/gRPC API for Muster CLI operations
### END INIT INFO

DAEMON=/usr/bin/muster
DAEMON_ARGS="daemon --config /etc/muster/daemon.yaml"
PIDFILE=/var/run/muster-daemon.pid
USER=muster
GROUP=muster

case "$1" in
  start)
    echo "Starting Muster daemon..."
    start-stop-daemon --start --quiet --pidfile $PIDFILE \
      --chuid $USER:$GROUP --exec $DAEMON -- $DAEMON_ARGS
    ;;
  stop)
    echo "Stopping Muster daemon..."
    start-stop-daemon --stop --quiet --pidfile $PIDFILE \
      --exec $DAEMON --retry TERM/30/KILL/5
    ;;
  restart)
    $0 stop
    $0 start
    ;;
  status)
    if [ -f $PIDFILE ]; then
      if kill -0 $(cat $PIDFILE) 2>/dev/null; then
        echo "Muster daemon is running (PID: $(cat $PIDFILE))"
      else
        echo "Muster daemon is not running (stale PID file)"
      fi
    else
      echo "Muster daemon is not running"
    fi
    ;;
  *)
    echo "Usage: $0 {start|stop|restart|status}"
    exit 1
    ;;
esac

exit 0
```

### 1.3 Desktop Files

Desktop entry for GUI integration:

```ini
# /usr/share/applications/muster.desktop
[Desktop Entry]
Version=1.0
Type=Application
Name=Muster CLI
Comment=API Client and Development Tool
Exec=muster %F
Icon=muster
Terminal=true
Categories=Development;Network;
MimeType=application/x-yaml;application/json;
Keywords=api;http;rest;openapi;cli;
StartupNotify=true
X-GNOME-SingleWindow=true
```

Mime type association:

```xml
<!-- /usr/share/mime/packages/muster.xml -->
<?xml version="1.0" encoding="UTF-8"?>
<mime-info xmlns="http://www.freedesktop.org/standards/shared-mime-info">
  <mime-type type="application/x-openapi">
    <comment>OpenAPI Specification</comment>
    <glob pattern="*.openapi.yaml"/>
    <glob pattern="*.openapi.json"/>
    <glob pattern="*openapi*.yaml"/>
    <glob pattern="*openapi*.json"/>
  </mime-type>
</mime-info>
```

---

## 2. Package Managers

### 2.1 Debian Package (.deb)

Package structure:

```
muster_1.0.0_amd64/
├── DEBIAN/
│   ├── control
│   ├── preinst
│   ├── postinst
│   ├── prerm
│   └── postrm
└── usr/
    ├── bin/
    │   └── muster
    ├── share/
    │   ├── applications/
    │   │   └── muster.desktop
    │   ├── doc/
    │   │   └── muster/
    │   │       ├── changelog.gz
    │   │       ├── copyright
    │   │       └── README
    │   ├── man/
    │   │   └── man1/
    │   │       └── muster.1.gz
    │   └── mime/
    │       └── packages/
    │           └── muster.xml
    └── lib/
        └── systemd/
            └── system/
                ├── muster-daemon.service
                └── muster-daemon.socket
```

Control file:

```
# DEBIAN/control
Package: muster
Version: 1.0.0
Section: utils
Priority: optional
Architecture: amd64
Depends: libc6 (>= 2.31), ca-certificates
Recommends: gnome-keyring | kwallet-cli
Suggests: muster-completion
Maintainer: Muster Team <team@muster.sh>
Description: Modern API Client and Development Tool
 Muster is a powerful CLI tool for API development and testing.
 Features OpenAPI integration, authentication management, and
 extensible workflows.
Homepage: https://muster.sh
```

Pre-installation script:

```bash
#!/bin/sh
# DEBIAN/preinst
set -e

case "$1" in
  install|upgrade)
    # Create muster user if it doesn't exist
    if ! id -u muster >/dev/null 2>&1; then
      useradd --system --no-create-home --shell /bin/false muster
    fi
    ;;
esac

exit 0
```

Post-installation script:

```bash
#!/bin/sh
# DEBIAN/postinst
set -e

case "$1" in
  configure)
    # Create directories
    mkdir -p /etc/muster
    mkdir -p /var/lib/muster
    mkdir -p /var/log/muster
    mkdir -p /run/muster
    
    # Set permissions
    chown muster:muster /var/lib/muster
    chown muster:muster /var/log/muster
    chown muster:muster /run/muster
    
    # Update mime database
    update-mime-database /usr/share/mime || true
    
    # Update desktop database
    update-desktop-database /usr/share/applications || true
    
    # Reload systemd
    if [ -d /run/systemd/system ]; then
      systemctl daemon-reload
      systemctl enable muster-daemon.socket
    fi
    ;;
esac

exit 0
```

Build script:

```bash
#!/bin/bash
# scripts/build-deb.sh

set -e

VERSION=${1:-1.0.0}
ARCH=${2:-amd64}
PKG_DIR="muster_${VERSION}_${ARCH}"

# Create package structure
rm -rf "$PKG_DIR"
mkdir -p "$PKG_DIR/DEBIAN"
mkdir -p "$PKG_DIR/usr/bin"
mkdir -p "$PKG_DIR/usr/share/doc/muster"
mkdir -p "$PKG_DIR/etc/muster"

# Copy binary
cp dist/muster-linux-$ARCH "$PKG_DIR/usr/bin/muster"
chmod 755 "$PKG_DIR/usr/bin/muster"

# Copy control file
cat > "$PKG_DIR/DEBIAN/control" <<EOF
Package: muster
Version: $VERSION
Section: utils
Priority: optional
Architecture: $ARCH
Depends: libc6 (>= 2.31), ca-certificates
Maintainer: Muster Team <team@muster.sh>
Description: Modern API Client and Development Tool
EOF

# Copy scripts
cp packaging/deb/preinst "$PKG_DIR/DEBIAN/"
cp packaging/deb/postinst "$PKG_DIR/DEBIAN/"
cp packaging/deb/prerm "$PKG_DIR/DEBIAN/"
cp packaging/deb/postrm "$PKG_DIR/DEBIAN/"
chmod 755 "$PKG_DIR/DEBIAN/"{preinst,postinst,prerm,postrm}

# Build package
dpkg-deb --build "$PKG_DIR"

# Sign package (optional)
# dpkg-sig --sign builder "${PKG_DIR}.deb"

echo "Package built: ${PKG_DIR}.deb"
```

### 2.2 RPM Package

Spec file:

```spec
# muster.spec
Name:           muster
Version:        1.0.0
Release:        1%{?dist}
Summary:        Modern API Client and Development Tool
License:        MIT
URL:            https://muster.sh
Source0:        %{name}-%{version}.tar.gz

BuildArch:      x86_64
Requires:       ca-certificates
Recommends:     gnome-keyring

%description
Muster is a powerful CLI tool for API development and testing.
Features OpenAPI integration, authentication management, and
extensible workflows.

%prep
%setup -q

%build
# Binary is pre-built
cp %{_sourcedir}/muster-linux-amd64 muster

%install
install -D -m 755 muster %{buildroot}%{_bindir}/muster
install -D -m 644 systemd/muster-daemon.service %{buildroot}%{_unitdir}/muster-daemon.service
install -D -m 644 systemd/muster-daemon.socket %{buildroot}%{_unitdir}/muster-daemon.socket
install -D -m 644 desktop/muster.desktop %{buildroot}%{_datadir}/applications/muster.desktop

# Create directories
mkdir -p %{buildroot}%{_sysconfdir}/muster
mkdir -p %{buildroot}%{_sharedstatedir}/muster
mkdir -p %{buildroot}%{_localstatedir}/log/muster
mkdir -p %{buildroot}%{_rundir}/muster

%pre
getent group muster >/dev/null || groupadd -r muster
getent passwd muster >/dev/null || useradd -r -g muster -d %{_sharedstatedir}/muster -s /sbin/nologin muster

%post
%systemd_post muster-daemon.socket
%systemd_post muster-daemon.service

%preun
%systemd_preun muster-daemon.service
%systemd_preun muster-daemon.socket

%postun
%systemd_postun_with_restart muster-daemon.socket

%files
%license LICENSE
%doc README.md
%{_bindir}/muster
%{_unitdir}/muster-daemon.service
%{_unitdir}/muster-daemon.socket
%{_datadir}/applications/muster.desktop
%dir %attr(755, muster, muster) %{_sysconfdir}/muster
%dir %attr(755, muster, muster) %{_sharedstatedir}/muster
%dir %attr(755, muster, muster) %{_localstatedir}/log/muster
%dir %attr(755, muster, muster) %{_rundir}/muster

%changelog
* Mon Jan 15 2024 Muster Team <team@muster.sh> - 1.0.0-1
- Initial release
```

Build script:

```bash
#!/bin/bash
# scripts/build-rpm.sh

set -e

VERSION=${1:-1.0.0}

# Create RPM build structure
mkdir -p ~/rpmbuild/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

# Copy source files
cp dist/muster-linux-amd64 ~/rpmbuild/SOURCES/
cp packaging/rpm/muster.spec ~/rpmbuild/SPECS/

# Create tarball
tar czf ~/rpmbuild/SOURCES/muster-${VERSION}.tar.gz -C . \
  --exclude='.git' --exclude='dist' --exclude='bin' .

# Build RPM
rpmbuild -bb ~/rpmbuild/SPECS/muster.spec

echo "RPM built: ~/rpmbuild/RPMS/x86_64/muster-${VERSION}-1.x86_64.rpm"
```

### 2.3 Alpine Package (APK)

APKBUILD file:

```bash
# APKBUILD
pkgname=muster
pkgver=1.0.0
pkgrel=0
pkgdesc="Modern API Client and Development Tool"
url="https://muster.sh"
arch="x86_64 aarch64"
license="MIT"
depends="ca-certificates"
install="muster.pre-install muster.post-install"
source="muster-$pkgver.tar.gz"
builddir="$srcdir"

build() {
    return 0
}

check() {
    ./muster version
}

package() {
    install -D -m755 muster "$pkgdir"/usr/bin/muster
    install -D -m644 muster-daemon.service "$pkgdir"/usr/lib/systemd/system/muster-daemon.service
    install -D -m644 muster-daemon.socket "$pkgdir"/usr/lib/systemd/system/muster-daemon.socket
    
    install -d -m755 -o muster -g muster "$pkgdir"/var/lib/muster
    install -d -m755 -o muster -g muster "$pkgdir"/var/log/muster
}
```

Pre-install script:

```bash
#!/bin/sh
# muster.pre-install

addgroup -S muster 2>/dev/null
adduser -S -D -H -h /var/lib/muster -s /sbin/nologin -G muster -g muster muster 2>/dev/null

exit 0
```

Post-install script:

```bash
#!/bin/sh
# muster.post-install

# Create necessary directories
mkdir -p /etc/muster
mkdir -p /run/muster

exit 0
```

### 2.4 Arch Linux PKGBUILD

```bash
# PKGBUILD
pkgname=muster
pkgver=1.0.0
pkgrel=1
pkgdesc="Modern API Client and Development Tool"
arch=('x86_64' 'aarch64')
url="https://muster.sh"
license=('MIT')
depends=('glibc' 'ca-certificates')
optdepends=('gnome-keyring: GNOME keyring support'
            'kwallet: KDE wallet support')
source=("$pkgname-$pkgver.tar.gz::https://github.com/anomalyco/muster/releases/download/v$pkgver/muster-linux-amd64.tar.gz")
sha256sums=('SKIP')

package() {
    install -Dm755 muster "$pkgdir/usr/bin/muster"
    install -Dm644 LICENSE "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
    install -Dm644 README.md "$pkgdir/usr/share/doc/$pkgname/README.md"
    install -Dm644 systemd/muster-daemon.service "$pkgdir/usr/lib/systemd/system/muster-daemon.service"
    install -Dm644 systemd/muster-daemon.socket "$pkgdir/usr/lib/systemd/system/muster-daemon.socket"
    install -Dm644 desktop/muster.desktop "$pkgdir/usr/share/applications/muster.desktop"
    
    # Install shell completions
    install -Dm644 completions/muster.bash "$pkgdir/usr/share/bash-completion/completions/muster"
    install -Dm644 completions/muster.zsh "$pkgdir/usr/share/zsh/site-functions/_muster"
    install -Dm644 completions/muster.fish "$pkgdir/usr/share/fish/vendor_completions.d/muster.fish"
    
    # Create user and directories
    install -d -m755 -o 65534 -g 65534 "$pkgdir/var/lib/muster"
    install -d -m755 -o 65534 -g 65534 "$pkgdir/var/log/muster"
    install -d -m755 -o 65534 -g 65534 "$pkgdir/run/muster"
}
```

---

## 3. Keychain Integration

### 3.1 Secret Service D-Bus

Implementation for FreeDesktop Secret Service API:

```go
// internal/platform/linux/keyring_secret_service.go
//go:build linux
// +build linux

package linux

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"

    "github.com/godbus/dbus/v5"
    "github.com/anomalyco/muster/internal/core/auth"
)

const (
    secretServiceBusName      = "org.freedesktop.secrets"
    secretServicePath         = "/org/freedesktop/secrets"
    secretServiceInterface    = "org.freedesktop.secrets.Service"
    secretCollectionInterface = "org.freedesktop.secrets.Collection"
    secretItemInterface       = "org.freedesktop.secrets.Item"
    secretSessionInterface    = "org.freedesktop.secrets.Session"
)

// SecretServiceKeyring implements auth.Keyring for Linux Secret Service
type SecretServiceKeyring struct {
    conn       *dbus.Conn
    service    dbus.BusObject
    collection dbus.BusObject
    session    dbus.BusObject
    sessionPath dbus.ObjectPath
}

// NewSecretServiceKeyring creates a new Secret Service keyring instance
func NewSecretServiceKeyring() (*SecretServiceKeyring, error) {
    conn, err := dbus.ConnectSessionBus()
    if err != nil {
        return nil, fmt.Errorf("failed to connect to session bus: %w", err)
    }

    kr := &SecretServiceKeyring{
        conn:    conn,
        service: conn.Object(secretServiceBusName, secretServicePath),
    }

    // Open session for encryption
    if err := kr.openSession(); err != nil {
        conn.Close()
        return nil, err
    }

    // Get default collection
    if err := kr.getDefaultCollection(); err != nil {
        kr.Close()
        return nil, err
    }

    return kr, nil
}

func (kr *SecretServiceKeyring) openSession() error {
    var sessionPath dbus.ObjectPath
    err := kr.service.Call(secretServiceInterface+".OpenSession", 0, 
        "plain", dbus.MakeVariant("")).Store(&sessionPath)
    if err != nil {
        return fmt.Errorf("failed to open session: %w", err)
    }

    kr.sessionPath = sessionPath
    kr.session = kr.conn.Object(secretServiceBusName, sessionPath)
    return nil
}

func (kr *SecretServiceKeyring) getDefaultCollection() error {
    var collectionPath dbus.ObjectPath
    err := kr.service.Call(secretServiceInterface+".ReadAlias", 0, 
        "default").Store(&collectionPath)
    if err != nil {
        return fmt.Errorf("failed to get default collection: %w", err)
    }

    if collectionPath == "/" {
        // No default collection, use session collection
        collectionPath = "/org/freedesktop/secrets/collection/session"
    }

    kr.collection = kr.conn.Object(secretServiceBusName, collectionPath)
    return nil
}

// Get retrieves a credential from the keyring
func (kr *SecretServiceKeyring) Get(ctx context.Context, key string) (*auth.Credential, error) {
    attributes := map[string]string{
        "service": "muster",
        "key":     key,
    }

    var unlocked []dbus.ObjectPath
    var locked []dbus.ObjectPath
    err := kr.collection.Call(secretCollectionInterface+".SearchItems", 0, 
        attributes).Store(&unlocked, &locked)
    if err != nil {
        return nil, fmt.Errorf("failed to search items: %w", err)
    }

    if len(unlocked) == 0 {
        if len(locked) > 0 {
            return nil, auth.ErrKeyringLocked
        }
        return nil, auth.ErrCredentialNotFound
    }

    // Get first unlocked item
    item := kr.conn.Object(secretServiceBusName, unlocked[0])
    
    var secret []interface{}
    err = item.Call(secretItemInterface+".GetSecret", 0, kr.sessionPath).Store(&secret)
    if err != nil {
        return nil, fmt.Errorf("failed to get secret: %w", err)
    }

    // Decode secret value
    secretValue, ok := secret[2].([]byte)
    if !ok {
        return nil, errors.New("invalid secret format")
    }

    var cred auth.Credential
    if err := json.Unmarshal(secretValue, &cred); err != nil {
        return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
    }

    return &cred, nil
}

// Set stores a credential in the keyring
func (kr *SecretServiceKeyring) Set(ctx context.Context, key string, cred *auth.Credential) error {
    data, err := json.Marshal(cred)
    if err != nil {
        return fmt.Errorf("failed to marshal credential: %w", err)
    }

    secret := []interface{}{
        "",                    // session path (not used with plain encryption)
        make([]byte, 0),       // parameters
        data,                  // value
        "text/plain",          // content type
    }

    properties := map[string]dbus.Variant{
        "org.freedesktop.Secret.Item.Label": dbus.MakeVariant(fmt.Sprintf("muster: %s", key)),
        "org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(map[string]string{
            "service": "muster",
            "key":     key,
        }),
    }

    var itemPath dbus.ObjectPath
    var promptPath dbus.ObjectPath
    err = kr.collection.Call(secretCollectionInterface+".CreateItem", 0,
        properties, secret, true).Store(&itemPath, &promptPath)
    if err != nil {
        return fmt.Errorf("failed to create item: %w", err)
    }

    // Handle prompt if required (for locked collections)
    if promptPath != "/" {
        return kr.handlePrompt(ctx, promptPath)
    }

    return nil
}

// Delete removes a credential from the keyring
func (kr *SecretServiceKeyring) Delete(ctx context.Context, key string) error {
    attributes := map[string]string{
        "service": "muster",
        "key":     key,
    }

    var unlocked []dbus.ObjectPath
    var locked []dbus.ObjectPath
    err := kr.collection.Call(secretCollectionInterface+".SearchItems", 0,
        attributes).Store(&unlocked, &locked)
    if err != nil {
        return fmt.Errorf("failed to search items: %w", err)
    }

    if len(unlocked) == 0 {
        return auth.ErrCredentialNotFound
    }

    // Delete first matching item
    item := kr.conn.Object(secretServiceBusName, unlocked[0])
    err = item.Call(secretItemInterface+".Delete", 0).Store()
    if err != nil {
        return fmt.Errorf("failed to delete item: %w", err)
    }

    return nil
}

func (kr *SecretServiceKeyring) handlePrompt(ctx context.Context, promptPath dbus.ObjectPath) error {
    prompt := kr.conn.Object(secretServiceBusName, promptPath)
    
    // Connect to prompt completed signal
    sigChan := make(chan *dbus.Signal, 1)
    kr.conn.Signal(sigChan)
    defer kr.conn.RemoveSignal(sigChan)

    matchRule := fmt.Sprintf("type='signal',path='%s',interface='%s',member='Completed'",
        promptPath, "org.freedesktop.Secret.Prompt")
    kr.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule)

    // Trigger prompt
    var promptResult dbus.ObjectPath
    err := prompt.Call("org.freedesktop.Secret.Prompt.Prompt", 0, "").Store(&promptResult)
    if err != nil {
        return err
    }

    // Wait for completion
    select {
    case sig := <-sigChan:
        if len(sig.Body) < 2 {
            return errors.New("invalid prompt response")
        }
        dismissed := sig.Body[0].(bool)
        if dismissed {
            return auth.ErrKeyringOperationCancelled
        }
        return nil
    case <-ctx.Done():
        prompt.Call("org.freedesktop.Secret.Prompt.Dismiss", 0)
        return ctx.Err()
    }
}

// Close releases resources
func (kr *SecretServiceKeyring) Close() error {
    if kr.session != nil {
        kr.session.Call(secretSessionInterface+".Close", 0)
    }
    if kr.conn != nil {
        return kr.conn.Close()
    }
    return nil
}
```

### 3.2 GNOME Keyring

GNOME Keyring specific implementation:

```go
// internal/platform/linux/keyring_gnome.go
//go:build linux
// +build linux

package linux

import (
    "context"
    "encoding/json"
    "fmt"
    "os/exec"
    "strings"

    "github.com/anomalyco/muster/internal/core/auth"
)

// GnomeKeyring implements keyring using gnome-keyring-daemon CLI
type GnomeKeyring struct {
    collection string
}

// NewGnomeKeyring creates a GNOME Keyring adapter
func NewGnomeKeyring() *GnomeKeyring {
    return &GnomeKeyring{
        collection: "login", // Use login keyring by default
    }
}

// IsAvailable checks if gnome-keyring-daemon is running
func (k *GnomeKeyring) IsAvailable() bool {
    cmd := exec.Command("gnome-keyring-daemon", "-s")
    err := cmd.Run()
    return err == nil
}

// Get retrieves a credential
func (k *GnomeKeyring) Get(ctx context.Context, key string) (*auth.Credential, error) {
    if !k.IsAvailable() {
        return nil, auth.ErrKeyringNotAvailable
    }

    displayKey := fmt.Sprintf("muster/%s", key)
    
    cmd := exec.CommandContext(ctx, "secret-tool", "lookup",
        "service", "muster",
        "key", key)
    
    output, err := cmd.Output()
    if err != nil {
        if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
            return nil, auth.ErrCredentialNotFound
        }
        return nil, fmt.Errorf("secret-tool lookup failed: %w", err)
    }

    var cred auth.Credential
    if err := json.Unmarshal(output, &cred); err != nil {
        return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
    }

    return &cred, nil
}

// Set stores a credential
func (k *GnomeKeyring) Set(ctx context.Context, key string, cred *auth.Credential) error {
    if !k.IsAvailable() {
        return auth.ErrKeyringNotAvailable
    }

    data, err := json.Marshal(cred)
    if err != nil {
        return fmt.Errorf("failed to marshal credential: %w", err)
    }

    displayKey := fmt.Sprintf("muster/%s", key)
    
    cmd := exec.CommandContext(ctx, "secret-tool", "store",
        "--label="+displayKey,
        "service", "muster",
        "key", key)
    
    cmd.Stdin = strings.NewReader(string(data))
    
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("secret-tool store failed: %w", err)
    }

    return nil
}

// Delete removes a credential
func (k *GnomeKeyring) Delete(ctx context.Context, key string) error {
    if !k.IsAvailable() {
        return auth.ErrKeyringNotAvailable
    }

    cmd := exec.CommandContext(ctx, "secret-tool", "clear",
        "service", "muster",
        "key", key)
    
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("secret-tool clear failed: %w", err)
    }

    return nil
}

// Close is a no-op for GNOME Keyring
func (k *GnomeKeyring) Close() error {
    return nil
}
```

### 3.3 KWallet

KDE KWallet implementation:

```go
// internal/platform/linux/keyring_kwallet.go
//go:build linux
// +build linux

package linux

import (
    "context"
    "encoding/json"
    "fmt"
    "os/exec"

    "github.com/godbus/dbus/v5"
    "github.com/anomalyco/muster/internal/core/auth"
)

const (
    kwalletBusName   = "org.kde.kwalletd5"
    kwalletPath      = "/modules/kwalletd5"
    kwalletInterface = "org.kde.KWallet"
)

// KWalletKeyring implements keyring using KDE KWallet
type KWalletKeyring struct {
    conn       *dbus.Conn
    wallet     dbus.BusObject
    walletId   int32
    appId      string
}

// NewKWalletKeyring creates a new KWallet keyring instance
func NewKWalletKeyring() (*KWalletKeyring, error) {
    conn, err := dbus.ConnectSessionBus()
    if err != nil {
        return nil, fmt.Errorf("failed to connect to session bus: %w", err)
    }

    kw := &KWalletKeyring{
        conn:  conn,
        wallet: conn.Object(kwalletBusName, dbus.ObjectPath(kwalletPath)),
        appId: "muster",
    }

    // Open default wallet
    if err := kw.openWallet(); err != nil {
        conn.Close()
        return nil, err
    }

    return kw, nil
}

func (kw *KWalletKeyring) openWallet() error {
    var walletId int32
    var open bool
    
    err := kw.wallet.Call(kwalletInterface+".open", 0, 
        "kdewallet", 0, kw.appId).Store(&walletId, &open)
    if err != nil {
        return fmt.Errorf("failed to open wallet: %w", err)
    }

    if !open {
        return auth.ErrKeyringLocked
    }

    kw.walletId = walletId
    return nil
}

// Get retrieves a credential
func (kw *KWalletKeyring) Get(ctx context.Context, key string) (*auth.Credential, error) {
    folder := "muster"
    
    // Check if folder exists
    var hasFolder bool
    err := kw.wallet.Call(kwalletInterface+".hasFolder", 0,
        kw.walletId, folder, kw.appId).Store(&hasFolder)
    if err != nil {
        return nil, fmt.Errorf("failed to check folder: %w", err)
    }

    if !hasFolder {
        return nil, auth.ErrCredentialNotFound
    }

    // Read entry
    var value string
    err = kw.wallet.Call(kwalletInterface+".readEntry", 0,
        kw.walletId, folder, key, kw.appId).Store(&value)
    if err != nil {
        return nil, fmt.Errorf("failed to read entry: %w", err)
    }

    if value == "" {
        return nil, auth.ErrCredentialNotFound
    }

    var cred auth.Credential
    if err := json.Unmarshal([]byte(value), &cred); err != nil {
        return nil, fmt.Errorf("failed to unmarshal credential: %w", err)
    }

    return &cred, nil
}

// Set stores a credential
func (kw *KWalletKeyring) Set(ctx context.Context, key string, cred *auth.Credential) error {
    folder := "muster"
    
    // Create folder if it doesn't exist
    var hasFolder bool
    err := kw.wallet.Call(kwalletInterface+".hasFolder", 0,
        kw.walletId, folder, kw.appId).Store(&hasFolder)
    if err != nil {
        return fmt.Errorf("failed to check folder: %w", err)
    }

    if !hasFolder {
        err = kw.wallet.Call(kwalletInterface+".createFolder", 0,
            kw.walletId, folder, kw.appId).Store()
        if err != nil {
            return fmt.Errorf("failed to create folder: %w", err)
        }
    }

    data, err := json.Marshal(cred)
    if err != nil {
        return fmt.Errorf("failed to marshal credential: %w", err)
    }

    err = kw.wallet.Call(kwalletInterface+".writeEntry", 0,
        kw.walletId, folder, key, string(data), kw.appId).Store()
    if err != nil {
        return fmt.Errorf("failed to write entry: %w", err)
    }

    return nil
}

// Delete removes a credential
func (kw *KWalletKeyring) Delete(ctx context.Context, key string) error {
    folder := "muster"
    
    err := kw.wallet.Call(kwalletInterface+".removeEntry", 0,
        kw.walletId, folder, key, kw.appId).Store()
    if err != nil {
        return fmt.Errorf("failed to remove entry: %w", err)
    }

    return nil
}

// Close releases resources
func (kw *KWalletKeyring) Close() error {
    if kw.wallet != nil && kw.walletId != 0 {
        kw.wallet.Call(kwalletInterface+".close", 0, kw.walletId, false, kw.appId)
    }
    if kw.conn != nil {
        return kw.conn.Close()
    }
    return nil
}

// KWalletCLI provides fallback using kwalletcli
func (kw *KWalletKeyring) isCLIAvailable() bool {
    _, err := exec.LookPath("kwalletcli")
    return err == nil
}
```

---

## 4. File System

### 4.1 XDG Directories

XDG Base Directory Specification implementation:

```go
// internal/platform/linux/xdg.go
//go:build linux
// +build linux

package linux

import (
    "os"
    "path/filepath"
)

// XDGPaths holds all XDG directory paths
type XDGPaths struct {
    ConfigHome  string
    CacheHome   string
    DataHome    string
    StateHome   string
    RuntimeDir  string
}

// DefaultXDGPaths returns XDG paths with defaults
func DefaultXDGPaths() *XDGPaths {
    home := os.Getenv("HOME")
    if home == "" {
        home = "/tmp"
    }

    paths := &XDGPaths{
        ConfigHome:  os.Getenv("XDG_CONFIG_HOME"),
        CacheHome:   os.Getenv("XDG_CACHE_HOME"),
        DataHome:    os.Getenv("XDG_DATA_HOME"),
        StateHome:   os.Getenv("XDG_STATE_HOME"),
        RuntimeDir:  os.Getenv("XDG_RUNTIME_DIR"),
    }

    // Apply defaults
    if paths.ConfigHome == "" {
        paths.ConfigHome = filepath.Join(home, ".config")
    }
    if paths.CacheHome == "" {
        paths.CacheHome = filepath.Join(home, ".cache")
    }
    if paths.DataHome == "" {
        paths.DataHome = filepath.Join(home, ".local", "share")
    }
    if paths.StateHome == "" {
        paths.StateHome = filepath.Join(home, ".local", "state")
    }
    if paths.RuntimeDir == "" {
        // Fallback to /tmp for systems without XDG_RUNTIME_DIR
        paths.RuntimeDir = filepath.Join("/tmp", "muster-runtime-"+os.Getenv("USER"))
    }

    return paths
}

// MusterPaths returns muster-specific paths
func (x *XDGPaths) MusterPaths() *MusterPaths {
    return &MusterPaths{
        ConfigDir:   filepath.Join(x.ConfigHome, "muster"),
        CacheDir:    filepath.Join(x.CacheHome, "muster"),
        DataDir:     filepath.Join(x.DataHome, "muster"),
        StateDir:    filepath.Join(x.StateHome, "muster"),
        RuntimeDir:  filepath.Join(x.RuntimeDir, "muster"),
    }
}

// MusterPaths holds muster-specific directory paths
type MusterPaths struct {
    ConfigDir  string
    CacheDir   string
    DataDir    string
    StateDir   string
    RuntimeDir string
}

// EnsureAll creates all muster directories
func (m *MusterPaths) EnsureAll() error {
    dirs := []string{
        m.ConfigDir,
        m.CacheDir,
        m.DataDir,
        m.StateDir,
        m.RuntimeDir,
        filepath.Join(m.ConfigDir, "keys"),
        filepath.Join(m.CacheDir, "inventories"),
        filepath.Join(m.CacheDir, "modules"),
        filepath.Join(m.CacheDir, "specs"),
        filepath.Join(m.DataDir, "known-good"),
        filepath.Join(m.DataDir, "audit"),
    }

    for _, dir := range dirs {
        if err := os.MkdirAll(dir, 0755); err != nil {
            return err
        }
    }

    return nil
}

// ConfigFile returns the path to a config file
func (m *MusterPaths) ConfigFile(name string) string {
    return filepath.Join(m.ConfigDir, name)
}

// CacheFile returns the path to a cache file
func (m *MusterPaths) CacheFile(name string) string {
    return filepath.Join(m.CacheDir, name)
}

// DataFile returns the path to a data file
func (m *MusterPaths) DataFile(name string) string {
    return filepath.Join(m.DataDir, name)
}

// StateFile returns the path to a state file
func (m *MusterPaths) StateFile(name string) string {
    return filepath.Join(m.StateDir, name)
}

// RuntimeFile returns the path to a runtime file
func (m *MusterPaths) RuntimeFile(name string) string {
    return filepath.Join(m.RuntimeDir, name)
}

// SocketPath returns the path to the daemon socket
func (m *MusterPaths) SocketPath() string {
    return filepath.Join(m.RuntimeDir, "daemon.sock")
}

// PidFile returns the path to the PID file
func (m *MusterPaths) PidFile() string {
    return filepath.Join(m.RuntimeDir, "daemon.pid")
}
```

### 4.2 Permissions

File and directory permissions handling:

```go
// internal/platform/linux/permissions.go
//go:build linux
// +build linux

package linux

import (
    "fmt"
    "os"
    "syscall"
)

// SecureFileMode is the recommended permission for sensitive files (0600)
const SecureFileMode os.FileMode = 0600

// SecureDirMode is the recommended permission for sensitive directories (0700)
const SecureDirMode os.FileMode = 0700

// StandardDirMode is the standard permission for regular directories (0755)
const StandardDirMode os.FileMode = 0755

// StandardFileMode is the standard permission for regular files (0644)
const StandardFileMode os.FileMode = 0644

// SecureFile creates a file with restricted permissions (0600)
func SecureFile(path string) (*os.File, error) {
    // Use O_EXCL to prevent race conditions
    f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, SecureFileMode)
    if err != nil {
        return nil, err
    }

    // Ensure permissions are correct (umask might have affected them)
    if err := os.Chmod(path, SecureFileMode); err != nil {
        f.Close()
        os.Remove(path)
        return nil, err
    }

    return f, nil
}

// SecureMkdirAll creates a directory with specified permissions
func SecureMkdirAll(path string, mode os.FileMode) error {
    if err := os.MkdirAll(path, mode); err != nil {
        return err
    }

    // Ensure permissions are correct
    return os.Chmod(path, mode)
}

// IsSecure checks if a file has secure permissions (not world-readable)
func IsSecure(path string) (bool, error) {
    info, err := os.Stat(path)
    if err != nil {
        return false, err
    }

    mode := info.Mode().Perm()
    // Check if world has any permissions
    if mode&0077 != 0 {
        return false, nil
    }

    return true, nil
}

// FixPermissions corrects file permissions to be secure
func FixPermissions(path string, isDir bool) error {
    var mode os.FileMode
    if isDir {
        mode = SecureDirMode
    } else {
        mode = SecureFileMode
    }

    return os.Chmod(path, mode)
}

// GetOwner returns the UID and GID of a file
func GetOwner(path string) (uid, gid int, err error) {
    info, err := os.Stat(path)
    if err != nil {
        return 0, 0, err
    }

    stat, ok := info.Sys().(*syscall.Stat_t)
    if !ok {
        return 0, 0, fmt.Errorf("unable to get file stats")
    }

    return int(stat.Uid), int(stat.Gid), nil
}

// IsOwnedByCurrentUser checks if file is owned by current user
func IsOwnedByCurrentUser(path string) (bool, error) {
    uid, _, err := GetOwner(path)
    if err != nil {
        return false, err
    }

    return uid == os.Getuid(), nil
}

// SecureTempFile creates a temporary file in a secure manner
func SecureTempFile(dir, pattern string) (*os.File, error) {
    if dir == "" {
        dir = os.TempDir()
    }

    // Create with restrictive permissions
    f, err := os.CreateTemp(dir, pattern)
    if err != nil {
        return nil, err
    }

    if err := os.Chmod(f.Name(), SecureFileMode); err != nil {
        f.Close()
        os.Remove(f.Name())
        return nil, err
    }

    return f, nil
}
```

### 4.3 SELinux Considerations

SELinux context handling:

```go
// internal/platform/linux/selinux.go
//go:build linux
// +build linux

package linux

import (
    "os/exec"
    "path/filepath"
    "strings"
)

// SELinuxContext holds security context information
type SELinuxContext struct {
    User  string
    Role  string
    Type  string
    Level string
}

// IsSELinuxEnabled checks if SELinux is enabled on the system
func IsSELinuxEnabled() bool {
    cmd := exec.Command("getenforce")
    output, err := cmd.Output()
    if err != nil {
        return false
    }

    status := strings.TrimSpace(string(output))
    return status == "Enforcing" || status == "Permissive"
}

// GetFileContext retrieves the SELinux context of a file
func GetFileContext(path string) (*SELinuxContext, error) {
    cmd := exec.Command("ls", "-Z", path)
    output, err := cmd.Output()
    if err != nil {
        return nil, err
    }

    // Parse output: system_u:object_r:etc_t:s0 /etc/passwd
    parts := strings.Fields(string(output))
    if len(parts) < 4 {
        return nil, nil // No SELinux context
    }

    contextParts := strings.Split(parts[2], ":")
    if len(contextParts) < 3 {
        return nil, nil
    }

    ctx := &SELinuxContext{
        User: contextParts[0],
        Role: contextParts[1],
        Type: contextParts[2],
    }

    if len(contextParts) > 3 {
        ctx.Level = contextParts[3]
    }

    return ctx, nil
}

// SetFileContext sets the SELinux context for a file
func SetFileContext(path, context string) error {
    cmd := exec.Command("chcon", context, path)
    return cmd.Run()
}

// RestoreCon restores the default SELinux context for a file
func RestoreCon(path string, recursive bool) error {
    args := []string{"-v"}
    if recursive {
        args = append(args, "-R")
    }
    args = append(args, path)

    cmd := exec.Command("restorecon", args...)
    return cmd.Run()
}

// ApplyDaemonContexts applies appropriate SELinux contexts for daemon files
func ApplyDaemonContexts(baseDir string) error {
    if !IsSELinuxEnabled() {
        return nil
    }

    contexts := map[string]string{
        filepath.Join(baseDir, "etc"):       "etc_t",
        filepath.Join(baseDir, "bin/muster"): "bin_t",
        filepath.Join(baseDir, "var/lib"):   "var_lib_t",
        filepath.Join(baseDir, "var/log"):   "var_log_t",
        filepath.Join(baseDir, "run"):       "var_run_t",
    }

    for path, contextType := range contexts {
        context := "system_u:object_r:" + contextType + ":s0"
        if err := SetFileContext(path, context); err != nil {
            return err
        }
    }

    // Restore contexts recursively
    return RestoreCon(baseDir, true)
}

// SELinuxPolicy module for muster
const SELinuxPolicy = `
module muster 1.0;

require {
    type unconfined_t;
    type http_port_t;
    type user_home_t;
    type var_lib_t;
    type var_log_t;
    type var_run_t;
    class tcp_socket { create bind listen accept connect read write };
    class unix_stream_socket { create connect read write };
    class file { read write create unlink open getattr setattr };
    class dir { read write create add_name remove_name search };
    class process { signal };
}

type muster_t;
type muster_exec_t;
type muster_var_lib_t;
type muster_var_log_t;
type muster_var_run_t;

# Domain transition
application_domain(muster_t, muster_exec_t)

# Allow muster daemon to run
allow muster_t self:process signal;

# Network access
allow muster_t http_port_t:tcp_socket { create bind listen accept connect read write };
allow muster_t self:unix_stream_socket { create connect read write };

# File access
allow muster_t muster_var_lib_t:dir { read write create add_name remove_name search };
allow muster_t muster_var_lib_t:file { read write create unlink open getattr setattr };
allow muster_t muster_var_log_t:dir { read write create add_name remove_name search };
allow muster_t muster_var_log_t:file { read write create unlink open getattr setattr };
allow muster_t muster_var_run_t:dir { read write create add_name remove_name search };
allow muster_t muster_var_run_t:file { read write create unlink open getattr setattr };

# Allow reading config from user home (for user daemon)
allow muster_t user_home_t:dir search;
allow muster_t user_home_t:file { read open getattr };

# Logging
logging_send_syslog_msg(muster_t)
`

// InstallSELinuxPolicy installs the SELinux policy module
func InstallSELinuxPolicy() error {
    if !IsSELinuxEnabled() {
        return nil
    }

    // Check if module already exists
    cmd := exec.Command("semodule", "-l")
    output, err := cmd.Output()
    if err != nil {
        return err
    }

    if strings.Contains(string(output), "muster") {
        // Module already installed
        return nil
    }

    // Create temporary policy file
    tmpFile, err := SecureTempFile("", "muster.te")
    if err != nil {
        return err
    }
    defer tmpFile.Close()
    defer RemoveSecure(tmpFile.Name())

    if _, err := tmpFile.WriteString(SELinuxPolicy); err != nil {
        return err
    }
    tmpFile.Close()

    // Compile and install module
    baseName := strings.TrimSuffix(tmpFile.Name(), ".te")
    
    // Compile type enforcement file
    cmd = exec.Command("checkmodule", "-M", "-m", "-o", baseName+".mod", tmpFile.Name())
    if err := cmd.Run(); err != nil {
        return err
    }

    // Create policy package
    cmd = exec.Command("semodule_package", "-o", baseName+".pp", "-m", baseName+".mod")
    if err := cmd.Run(); err != nil {
        return err
    }

    // Install module
    cmd = exec.Command("semodule", "-i", baseName+".pp")
    return cmd.Run()
}

// RemoveSecure securely removes a file
func RemoveSecure(path string) error {
    // Overwrite with zeros before deleting
    // This is a simplified version; consider using shred for sensitive data
    return exec.Command("shred", "-u", "-z", path).Run()
}
```

---

## 5. Network

### 5.1 NetworkManager Integration

Network change detection and proxy configuration:

```go
// internal/platform/linux/networkmanager.go
//go:build linux
// +build linux

package linux

import (
    "context"
    "encoding/json"
    "fmt"
    "net"
    "net/url"
    "strings"

    "github.com/godbus/dbus/v5"
)

const (
    nmBusName        = "org.freedesktop.NetworkManager"
    nmPath           = "/org/freedesktop/NetworkManager"
    nmInterface      = "org.freedesktop.NetworkManager"
    nmSettingsPath   = "/org/freedesktop/NetworkManager/Settings"
    nmSettingsIface  = "org.freedesktop.NetworkManager.Settings"
)

// NetworkManager provides integration with NetworkManager
type NetworkManager struct {
    conn     *dbus.Conn
    nm       dbus.BusObject
    settings dbus.BusObject
    signals  chan *dbus.Signal
}

// NewNetworkManager creates a new NetworkManager instance
func NewNetworkManager() (*NetworkManager, error) {
    conn, err := dbus.ConnectSystemBus()
    if err != nil {
        return nil, fmt.Errorf("failed to connect to system bus: %w", err)
    }

    nm := &NetworkManager{
        conn:     conn,
        nm:       conn.Object(nmBusName, dbus.ObjectPath(nmPath)),
        settings: conn.Object(nmBusName, dbus.ObjectPath(nmSettingsPath)),
        signals:  make(chan *dbus.Signal, 10),
    }

    conn.Signal(nm.signals)

    // Subscribe to state changes
    if err := nm.subscribeToStateChanges(); err != nil {
        conn.Close()
        return nil, err
    }

    return nm, nil
}

func (nm *NetworkManager) subscribeToStateChanges() error {
    // Add match rule for NetworkManager state changes
    matchRule := "type='signal',interface='org.freedesktop.NetworkManager',member='StateChanged'"
    return nm.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err
}

// NetworkState represents the network connectivity state
type NetworkState int

const (
    NetworkStateUnknown      NetworkState = 0
    NetworkStateAsleep       NetworkState = 10
    NetworkStateDisconnected NetworkState = 20
    NetworkStateConnecting   NetworkState = 30
    NetworkStateLocalOnly    NetworkState = 40
    NetworkStateConnectedSite NetworkState = 50
    NetworkStateConnectedGlobal NetworkState = 60
)

// GetConnectivity returns the current network connectivity state
func (nm *NetworkManager) GetConnectivity() (NetworkState, error) {
    var state uint32
    err := nm.nm.Call(nmInterface+".state", 0).Store(&state)
    if err != nil {
        return NetworkStateUnknown, err
    }

    return NetworkState(state), nil
}

// GetActiveConnections returns the currently active connections
func (nm *NetworkManager) GetActiveConnections() ([]ActiveConnection, error) {
    var paths []dbus.ObjectPath
    err := nm.nm.Call(nmInterface+".GetActiveConnections", 0).Store(&paths)
    if err != nil {
        return nil, err
    }

    var connections []ActiveConnection
    for _, path := range paths {
        conn, err := nm.getActiveConnectionDetails(path)
        if err != nil {
            continue
        }
        connections = append(connections, *conn)
    }

    return connections, nil
}

// ActiveConnection represents an active network connection
type ActiveConnection struct {
    ID         string
    UUID       string
    Type       string
    Device     string
    IP4Config  string
    IP6Config  string
    VPN        bool
}

func (nm *NetworkManager) getActiveConnectionDetails(path dbus.ObjectPath) (*ActiveConnection, error) {
    obj := nm.conn.Object(nmBusName, path)
    
    var props map[string]dbus.Variant
    err := obj.Call("org.freedesktop.DBus.Properties.GetAll", 0, 
        "org.freedesktop.NetworkManager.Connection.Active").Store(&props)
    if err != nil {
        return nil, err
    }

    conn := &ActiveConnection{}
    
    if v, ok := props["Id"]; ok {
        conn.ID = v.Value().(string)
    }
    if v, ok := props["Uuid"]; ok {
        conn.UUID = v.Value().(string)
    }
    if v, ok := props["Type"]; ok {
        conn.Type = v.Value().(string)
    }
    if v, ok := props["Vpn"]; ok {
        conn.VPN = v.Value().(bool)
    }

    return conn, nil
}

// GetProxySettings retrieves proxy configuration for the active connection
func (nm *NetworkManager) GetProxySettings() (*ProxySettings, error) {
    // Get proxy method
    method, err := nm.getProxyMethod()
    if err != nil {
        return nil, err
    }

    switch method {
    case "none":
        return &ProxySettings{Mode: ProxyModeDirect}, nil
    case "auto":
        url, err := nm.getPacUrl()
        if err != nil {
            return nil, err
        }
        return &ProxySettings{Mode: ProxyModeAuto, PACURL: url}, nil
    case "manual":
        return nm.getManualProxySettings()
    default:
        return &ProxySettings{Mode: ProxyModeDirect}, nil
    }
}

// ProxyMode represents the proxy configuration mode
type ProxyMode string

const (
    ProxyModeDirect ProxyMode = "direct"
    ProxyModeAuto   ProxyMode = "auto"
    ProxyModeManual ProxyMode = "manual"
)

// ProxySettings holds proxy configuration
type ProxySettings struct {
    Mode       ProxyMode
    HTTPProxy  string
    HTTPSProxy string
    FTPProxy   string
    SOCKSProxy string
    NoProxy    []string
    PACURL     string
}

func (nm *NetworkManager) getProxyMethod() (string, error) {
    var method string
    err := nm.nm.Call(nmInterface+".GetProxyMethod", 0).Store(&method)
    return method, err
}

func (nm *NetworkManager) getPacUrl() (string, error) {
    var url string
    err := nm.nm.Call(nmInterface+".GetProxyPacUrl", 0).Store(&url)
    return url, err
}

func (nm *NetworkManager) getManualProxySettings() (*ProxySettings, error) {
    var config map[string]dbus.Variant
    err := nm.nm.Call(nmInterface+".GetProxyConfiguration", 0).Store(&config)
    if err != nil {
        return nil, err
    }

    settings := &ProxySettings{Mode: ProxyModeManual}

    if v, ok := config["http"]; ok {
        settings.HTTPProxy = v.Value().(string)
    }
    if v, ok := config["https"]; ok {
        settings.HTTPSProxy = v.Value().(string)
    }
    if v, ok := config["ftp"]; ok {
        settings.FTPProxy = v.Value().(string)
    }
    if v, ok := config["socks"]; ok {
        settings.SOCKSProxy = v.Value().(string)
    }
    if v, ok := config["no-proxy"]; ok {
        if noProxy, ok := v.Value().([]string); ok {
            settings.NoProxy = noProxy
        }
    }

    return settings, nil
}

// WatchStateChanges monitors network state changes
func (nm *NetworkManager) WatchStateChanges(ctx context.Context, callback func(NetworkState)) {
    go func() {
        for {
            select {
            case sig := <-nm.signals:
                if sig.Name == nmInterface+".StateChanged" {
                    if len(sig.Body) > 0 {
                        if state, ok := sig.Body[0].(uint32); ok {
                            callback(NetworkState(state))
                        }
                    }
                }
            case <-ctx.Done():
                return
            }
        }
    }()
}

// Close releases resources
func (nm *NetworkManager) Close() error {
    if nm.signals != nil {
        close(nm.signals)
    }
    if nm.conn != nil {
        return nm.conn.Close()
    }
    return nil
}
```

### 5.2 Proxy Detection

Environment-based proxy detection:

```go
// internal/platform/linux/proxy.go
//go:build linux
// +build linux

package linux

import (
    "net/http"
    "net/url"
    "os"
    "strings"
    "syscall"
)

// GetProxyFromEnvironment detects proxy settings from environment variables
func GetProxyFromEnvironment() (*url.URL, error) {
    // Check proxy environment variables in order of precedence
    envVars := []string{
        "muster_PROXY",
        "MUSTER_PROXY",
        "HTTPS_PROXY",
        "https_proxy",
        "HTTP_PROXY",
        "http_proxy",
    }

    for _, envVar := range envVars {
        if proxyURL := os.Getenv(envVar); proxyURL != "" {
            return url.Parse(proxyURL)
        }
    }

    // Check for .netrc style configuration
    if proxy := getProxyFromNetrc(); proxy != "" {
        return url.Parse(proxy)
    }

    // Check for proxy.pac if available
    if pacURL := os.Getenv("AUTOCONFIG_URL"); pacURL != "" {
        return parsePAC(pacURL)
    }

    return nil, nil
}

// GetNoProxyHosts returns the list of hosts that should bypass proxy
func GetNoProxyHosts() []string {
    noProxy := os.Getenv("NO_PROXY")
    if noProxy == "" {
        noProxy = os.Getenv("no_proxy")
    }

    if noProxy == "" {
        return nil
    }

    return strings.Split(noProxy, ",")
}

// ShouldUseProxy checks if a URL should use proxy based on NO_PROXY settings
func ShouldUseProxy(targetURL string, noProxyHosts []string) bool {
    if len(noProxyHosts) == 0 {
        return true
    }

    u, err := url.Parse(targetURL)
    if err != nil {
        return true
    }

    host := u.Hostname()

    for _, pattern := range noProxyHosts {
        pattern = strings.TrimSpace(pattern)
        
        // Exact match
        if host == pattern {
            return false
        }

        // Wildcard suffix match (e.g., .example.com)
        if strings.HasPrefix(pattern, ".") && strings.HasSuffix(host, pattern) {
            return false
        }

        // Wildcard prefix match (e.g., *.example.com)
        if strings.HasPrefix(pattern, "*.") && 
           strings.HasSuffix(host, pattern[1:]) {
            return false
        }

        // CIDR notation (e.g., 192.168.0.0/16)
        if strings.Contains(pattern, "/") {
            if ip := net.ParseIP(host); ip != nil {
                if isIPInCIDR(ip, pattern) {
                    return false
                }
            }
        }
    }

    return true
}

func isIPInCIDR(ip net.IP, cidr string) bool {
    _, ipNet, err := net.ParseCIDR(cidr)
    if err != nil {
        return false
    }
    return ipNet.Contains(ip)
}

// getProxyFromNetrc attempts to read proxy from ~/.netrc
func getProxyFromNetrc() string {
    home := os.Getenv("HOME")
    if home == "" {
        return ""
    }

    netrcPath := home + "/.netrc"
    data, err := os.ReadFile(netrcPath)
    if err != nil {
        return ""
    }

    // Parse netrc for proxy entry
    lines := strings.Split(string(data), "\n")
    for _, line := range lines {
        if strings.Contains(line, "proxy") {
            // Simple parsing - extract URL after proxy keyword
            parts := strings.Fields(line)
            for i, part := range parts {
                if part == "proxy" && i+1 < len(parts) {
                    return parts[i+1]
                }
            }
        }
    }

    return ""
}

// parsePAC parses and evaluates PAC (Proxy Auto-Config) files
func parsePAC(pacURL string) (*url.URL, error) {
    // This is a simplified implementation
    // Full PAC support requires JavaScript evaluation
    // Consider using a library like github.com/jackwakefield/gopac

    // For now, return the PAC URL as the proxy
    return url.Parse(pacURL)
}

// CreateProxyTransport creates an HTTP transport with proxy support
func CreateProxyTransport() *http.Transport {
    transport := &http.Transport{
        Proxy: func(req *http.Request) (*url.URL, error) {
            // Check NO_PROXY first
            noProxyHosts := GetNoProxyHosts()
            if !ShouldUseProxy(req.URL.String(), noProxyHosts) {
                return nil, nil
            }

            // Get proxy from environment
            return GetProxyFromEnvironment()
        },
    }

    return transport
}

// ConfigureProxyFromGsettings reads proxy settings from gsettings (GNOME)
func ConfigureProxyFromGsettings() (*ProxySettings, error) {
    mode, err := execCommand("gsettings", "get", "org.gnome.system.proxy", "mode")
    if err != nil {
        return nil, err
    }

    mode = strings.Trim(mode, "'\"")

    settings := &ProxySettings{}

    switch mode {
    case "none":
        settings.Mode = ProxyModeDirect
    case "auto":
        settings.Mode = ProxyModeAuto
        pacURL, _ := execCommand("gsettings", "get", "org.gnome.system.proxy", "autoconfig-url")
        settings.PACURL = strings.Trim(pacURL, "'\"")
    case "manual":
        settings.Mode = ProxyModeManual
        settings.HTTPProxy = getGnomeProxy("http")
        settings.HTTPSProxy = getGnomeProxy("https")
        settings.FTPProxy = getGnomeProxy("ftp")
        settings.SOCKSProxy = getGnomeProxy("socks")
        
        noProxy, _ := execCommand("gsettings", "get", "org.gnome.system.proxy", "ignore-hosts")
        noProxy = strings.Trim(noProxy, "[]'")
        settings.NoProxy = strings.Split(noProxy, ",")
    }

    return settings, nil
}

func getGnomeProxy(scheme string) string {
    host, _ := execCommand("gsettings", "get", "org.gnome.system.proxy."+scheme, "host")
    port, _ := execCommand("gsettings", "get", "org.gnome.system.proxy."+scheme, "port")
    
    host = strings.Trim(host, "'\"")
    port = strings.Trim(port, "'")

    if host == "" || host == "null" {
        return ""
    }

    return scheme + "://" + host + ":" + port
}

func execCommand(name string, args ...string) (string, error) {
    cmd := exec.Command(name, args...)
    output, err := cmd.Output()
    return strings.TrimSpace(string(output)), err
}
```

---

## 6. Security

### 6.1 AppArmor Profile

AppArmor profile for the daemon:

```conf
# /etc/apparmor.d/usr.bin.muster
#include <tunables/global>

/usr/bin/muster {
  #include <abstractions/base>
  #include <abstractions/nameservice>
  #include <abstractions/ssl_certs>
  #include <abstractions/user-tmp>

  # Binary
  /usr/bin/muster mr,

  # Configuration files
  /etc/muster/** r,
  owner @{HOME}/.config/muster/** rwk,

  # Data directories
  owner @{HOME}/.local/share/muster/** rwk,
  owner @{HOME}/.cache/muster/** rwk,
  owner @{HOME}/.local/state/muster/** rwk,

  # Runtime directories
  owner @{run}/user/[0-9]*/muster/** rwk,
  owner /tmp/muster-*/** rw,

  # System runtime (for system daemon)
  /run/muster/** rwk,
  /var/lib/muster/** rwk,
  /var/log/muster/** rw,

  # Network access
  network inet stream,
  network inet6 stream,
  network unix stream,

  # DNS resolution
  /etc/resolv.conf r,
  /etc/nsswitch.conf r,
  /etc/host.conf r,
  /etc/hosts r,

  # CA certificates
  /etc/ssl/certs/** r,
  /etc/pki/tls/certs/** r,
  /usr/share/ca-certificates/** r,
  /usr/local/share/ca-certificates/** r,

  # System information
  /etc/os-release r,
  /etc/machine-id r,
  /var/lib/dbus/machine-id r,

  # Time
  /etc/localtime r,
  /usr/share/zoneinfo/** r,

  # D-Bus (for keyring integration)
  dbus (send)
       bus=session
       path=/org/freedesktop/secrets
       interface=org.freedesktop.secrets.*
       peer=(name=org.freedesktop.secrets),

  dbus (send)
       bus=session
       path=/modules/kwalletd5
       interface=org.kde.KWallet.*
       peer=(name=org.kde.kwalletd5),

  # Deny dangerous operations
  deny capability sys_admin,
  deny capability sys_ptrace,
  deny capability sys_rawio,
  deny capability sys_module,
  deny capability sys_time,
  deny capability mknod,
  deny capability net_admin,

  # Deny access to sensitive files
  deny /etc/shadow r,
  deny /etc/shadow- r,
  deny /etc/gshadow r,
  deny /etc/gshadow- r,
  deny /etc/passwd w,
  deny /etc/group w,

  # Deny kernel manipulation
  deny /proc/sys/** w,
  deny /sys/** w,

  # Allow reading proc for self
  /proc/@{pid}/** r,
  /proc/@{pid}/fd/ r,
  /proc/@{pid}/fd/* r,
  /proc/@{pid}/maps r,
  /proc/@{pid}/mounts r,
  /proc/@{pid}/mountinfo r,

  # Deny ptrace
  deny ptrace,

  # Deny mount operations
  deny mount,
  deny remount,
  deny umount,
}
```

Profile installation script:

```bash
#!/bin/bash
# scripts/install-apparmor.sh

set -e

PROFILE_SRC="packaging/apparmor/usr.bin.muster"
PROFILE_DEST="/etc/apparmor.d/usr.bin.muster"

echo "Installing AppArmor profile for muster..."

# Copy profile
sudo cp "$PROFILE_SRC" "$PROFILE_DEST"

# Set permissions
sudo chmod 644 "$PROFILE_DEST"

# Load profile
sudo apparmor_parser -r "$PROFILE_DEST"

# Enable profile
sudo aa-enforce /usr/bin/muster

echo "AppArmor profile installed and enabled"
```

### 6.2 Seccomp Filters

Seccomp BPF filter for syscall filtering:

```go
// internal/platform/linux/seccomp.go
//go:build linux
// +build linux

package linux

import (
    "fmt"
    "os"
    "os/exec"
    "runtime"
    "syscall"

    "github.com/seccomp/libseccomp-golang"
)

// SeccompProfile defines the allowed syscalls
type SeccompProfile int

const (
    // SeccompProfileMinimal allows only essential syscalls
    SeccompProfileMinimal SeccompProfile = iota
    // SeccompProfileStandard allows common syscalls for network operations
    SeccompProfileStandard
    // SeccompProfileRelaxed allows most syscalls (for WASM execution)
    SeccompProfileRelaxed
)

// ApplySeccompFilter applies a seccomp filter to the current process
func ApplySeccompFilter(profile SeccompProfile) error {
    // Ensure we're on Linux
    if runtime.GOOS != "linux" {
        return fmt.Errorf("seccomp is only supported on Linux")
    }

    // Create filter
    filter, err := seccomp.NewFilter(seccomp.ActErrno.SetReturnCode(int16(syscall.EPERM)))
    if err != nil {
        return fmt.Errorf("failed to create seccomp filter: %w", err)
    }
    defer filter.Release()

    // Add allowed syscalls based on profile
    switch profile {
    case SeccompProfileMinimal:
        if err := applyMinimalProfile(filter); err != nil {
            return err
        }
    case SeccompProfileStandard:
        if err := applyStandardProfile(filter); err != nil {
            return err
        }
    case SeccompProfileRelaxed:
        if err := applyRelaxedProfile(filter); err != nil {
            return err
        }
    }

    // Allow exiting
    if err := filter.AddRule(seccomp.Syscall(syscall.SYS_EXIT_GROUP), seccomp.ActAllow); err != nil {
        return err
    }

    // Load the filter
    if err := filter.Load(); err != nil {
        return fmt.Errorf("failed to load seccomp filter: %w", err)
    }

    return nil
}

func applyMinimalProfile(filter *seccomp.ScmpFilter) error {
    syscalls := []string{
        "read", "write", "open", "openat", "close",
        "stat", "fstat", "lstat", "newfstatat",
        "lseek", "mmap", "mprotect", "munmap",
        "brk", "rt_sigaction", "rt_sigprocmask",
        "ioctl", "pread64", "pwrite64",
        "access", "pipe", "pipe2",
        "select", "poll", "epoll_create", "epoll_create1",
        "epoll_ctl", "epoll_wait", "epoll_pwait",
        "dup", "dup2", "fcntl",
        "getpid", "gettid", "getppid", "getpgrp",
        "exit", "exit_group",
        "arch_prctl",
        "set_tid_address",
        "set_robust_list",
        "futex",
        "sched_getaffinity",
        "getrandom",
    }

    return addSyscalls(filter, syscalls)
}

func applyStandardProfile(filter *seccomp.ScmpFilter) error {
    // Include minimal profile
    if err := applyMinimalProfile(filter); err != nil {
        return err
    }

    // Add networking syscalls
    networkSyscalls := []string{
        "socket", "connect", "accept", "accept4",
        "bind", "listen", "setsockopt", "getsockopt",
        "getsockname", "getpeername", "shutdown",
        "sendto", "sendmsg", "sendmmsg",
        "recvfrom", "recvmsg", "recvmmsg",
        "socketpair",
    }

    // Add file operations
    fileSyscalls := []string{
        "creat", "unlink", "unlinkat",
        "rename", "renameat", "renameat2",
        "mkdir", "mkdirat", "rmdir",
        "chmod", "fchmod", "fchmodat",
        "chown", "fchown", "lchown", "fchownat",
        "symlink", "symlinkat", "readlink", "readlinkat",
        "truncate", "ftruncate",
        "fsync", "fdatasync",
        "sync", "syncfs",
    }

    // Add time operations
    timeSyscalls := []string{
        "time", "gettimeofday", "clock_gettime",
        "clock_getres", "nanosleep", "clock_nanosleep",
    }

    // Add process operations
    processSyscalls := []string{
        "clone", "fork", "vfork", "execve", "execveat",
        "wait4", "waitid", "kill", "tkill", "tgkill",
        "setpgid", "getpgid", "setsid", "getsid",
        "prctl", "capget", "capset",
    }

    allSyscalls := append(networkSyscalls, fileSyscalls...)
    allSyscalls = append(allSyscalls, timeSyscalls...)
    allSyscalls = append(allSyscalls, processSyscalls...)

    return addSyscalls(filter, allSyscalls)
}

func applyRelaxedProfile(filter *seccomp.ScmpFilter) error {
    // Include standard profile
    if err := applyStandardProfile(filter); err != nil {
        return err
    }

    // Add additional syscalls for WASM execution
    relaxedSyscalls := []string{
        "ptrace", "personality", "userfaultfd",
        "memfd_create", "memfd_secret",
        "bpf", "perf_event_open",
    }

    return addSyscalls(filter, relaxedSyscalls)
}

func addSyscalls(filter *seccomp.ScmpFilter, syscalls []string) error {
    for _, name := range syscalls {
        id, err := seccomp.GetSyscallFromName(name)
        if err != nil {
            // Skip unsupported syscalls
            continue
        }

        if err := filter.AddRule(id, seccomp.ActAllow); err != nil {
            return fmt.Errorf("failed to add syscall %s: %w", name, err)
        }
    }

    return nil
}

// CheckSeccompSupport checks if seccomp is supported
func CheckSeccompSupport() bool {
    // Try to create a test filter
    filter, err := seccomp.NewFilter(seccomp.ActAllow)
    if err != nil {
        return false
    }
    filter.Release()
    return true
}

// GenerateSeccompProfile generates a seccomp profile JSON for Docker
func GenerateSeccompProfile() string {
    return `{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": [
    "SCMP_ARCH_X86_64",
    "SCMP_ARCH_X86",
    "SCMP_ARCH_AARCH64"
  ],
  "syscalls": [
    {
      "names": [
        "accept",
        "accept4",
        "access",
        "arch_prctl",
        "bind",
        "brk",
        "capget",
        "capset",
        "chdir",
        "chmod",
        "chown",
        "clock_getres",
        "clock_gettime",
        "clock_nanosleep",
        "clone",
        "close",
        "connect",
        "creat",
        "dup",
        "dup2",
        "dup3",
        "epoll_create",
        "epoll_create1",
        "epoll_ctl",
        "epoll_pwait",
        "epoll_wait",
        "eventfd",
        "eventfd2",
        "execve",
        "execveat",
        "exit",
        "exit_group",
        "faccessat",
        "fadvise64",
        "fallocate",
        "fanotify_mark",
        "fchdir",
        "fchmod",
        "fchmodat",
        "fchown",
        "fchownat",
        "fcntl",
        "fdatasync",
        "fgetxattr",
        "flistxattr",
        "flock",
        "fork",
        "fremovexattr",
        "fsetxattr",
        "fstat",
        "fstatfs",
        "fsync",
        "ftruncate",
        "futex",
        "getcwd",
        "getdents",
        "getdents64",
        "getegid",
        "geteuid",
        "getgid",
        "getgroups",
        "getitimer",
        "getpeername",
        "getpgid",
        "getpgrp",
        "getpid",
        "getppid",
        "getpriority",
        "getrandom",
        "getresgid",
        "getresuid",
        "getrlimit",
        "get_robust_list",
        "getrusage",
        "getsid",
        "getsockname",
        "getsockopt",
        "get_thread_area",
        "gettid",
        "gettimeofday",
        "getuid",
        "getxattr",
        "inotify_add_watch",
        "inotify_init",
        "inotify_init1",
        "inotify_rm_watch",
        "io_cancel",
        "ioctl",
        "io_destroy",
        "io_getevents",
        "io_pgetevents",
        "ioprio_get",
        "ioprio_set",
        "io_setup",
        "io_submit",
        "io_uring_enter",
        "io_uring_register",
        "io_uring_setup",
        "kill",
        "lchown",
        "lgetxattr",
        "link",
        "linkat",
        "listen",
        "listxattr",
        "llistxattr",
        "lremovexattr",
        "lseek",
        "lsetxattr",
        "lstat",
        "madvise",
        "memfd_create",
        "mincore",
        "mkdir",
        "mkdirat",
        "mknod",
        "mknodat",
        "mlock",
        "mlock2",
        "mlockall",
        "mmap",
        "mprotect",
        "mq_getsetattr",
        "mq_notify",
        "mq_open",
        "mq_timedreceive",
        "mq_timedsend",
        "mq_unlink",
        "mremap",
        "msgctl",
        "msgget",
        "msgrcv",
        "msgsnd",
        "msync",
        "munlock",
        "munlockall",
        "munmap",
        "nanosleep",
        "newfstatat",
        "open",
        "openat",
        "openat2",
        "pause",
        "pidfd_open",
        "pidfd_send_signal",
        "pipe",
        "pipe2",
        "pivot_root",
        "poll",
        "ppoll",
        "prctl",
        "pread64",
        "preadv",
        "preadv2",
        "prlimit64",
        "pselect6",
        "pwrite64",
        "pwritev",
        "pwritev2",
        "read",
        "readahead",
        "readdir",
        "readlink",
        "readlinkat",
        "readv",
        "recv",
        "recvfrom",
        "recvmmsg",
        "recvmsg",
        "remap_file_pages",
        "removexattr",
        "rename",
        "renameat",
        "renameat2",
        "restart_syscall",
        "rmdir",
        "rseq",
        "rt_sigaction",
        "rt_sigpending",
        "rt_sigprocmask",
        "rt_sigqueueinfo",
        "rt_sigreturn",
        "rt_sigsuspend",
        "rt_sigtimedwait",
        "rt_tgsigqueueinfo",
        "sched_getaffinity",
        "sched_getattr",
        "sched_getparam",
        "sched_get_priority_max",
        "sched_get_priority_min",
        "sched_getscheduler",
        "sched_rr_get_interval",
        "sched_setaffinity",
        "sched_setattr",
        "sched_setparam",
        "sched_setscheduler",
        "sched_yield",
        "seccomp",
        "select",
        "semctl",
        "semget",
        "semop",
        "semtimedop",
        "send",
        "sendfile",
        "sendmmsg",
        "sendmsg",
        "sendto",
        "setfsgid",
        "setfsuid",
        "setgid",
        "setgroups",
        "setitimer",
        "setpgid",
        "setpriority",
        "setregid",
        "setresgid",
        "setresuid",
        "setreuid",
        "setrlimit",
        "set_robust_list",
        "setsid",
        "setsockopt",
        "set_thread_area",
        "set_tid_address",
        "setuid",
        "setxattr",
        "shmat",
        "shmctl",
        "shmdt",
        "shmget",
        "shutdown",
        "sigaltstack",
        "signalfd",
        "signalfd4",
        "socket",
        "socketpair",
        "splice",
        "stat",
        "statfs",
        "statx",
        "symlink",
        "symlinkat",
        "sync",
        "sync_file_range",
        "syncfs",
        "sysinfo",
        "tee",
        "tgkill",
        "time",
        "timer_create",
        "timer_delete",
        "timer_getoverrun",
        "timer_gettime",
        "timer_settime",
        "timerfd_create",
        "timerfd_gettime",
        "timerfd_settime",
        "times",
        "tkill",
        "truncate",
        "umask",
        "umount2",
        "uname",
        "unlink",
        "unlinkat",
        "utime",
        "utimensat",
        "utimes",
        "vfork",
        "wait4",
        "waitid",
        "waitpid",
        "write",
        "writev"
      ],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}`
}

// SeccompProfileForWASM returns a seccomp profile suitable for WASM execution
func SeccompProfileForWASM() string {
    return `{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": [
    "SCMP_ARCH_X86_64",
    "SCMP_ARCH_X86",
    "SCMP_ARCH_AARCH64",
    "SCMP_ARCH_WASM"
  ],
  "syscalls": [
    {
      "names": [
        "accept",
        "accept4",
        "access",
        "arch_prctl",
        "bind",
        "brk",
        "clock_gettime",
        "clone",
        "close",
        "connect",
        "dup",
        "dup2",
        "epoll_create",
        "epoll_create1",
        "epoll_ctl",
        "epoll_pwait",
        "epoll_wait",
        "eventfd2",
        "exit",
        "exit_group",
        "fcntl",
        "fstat",
        "fsync",
        "ftruncate",
        "futex",
        "getcwd",
        "getdents64",
        "getegid",
        "geteuid",
        "getgid",
        "getpid",
        "getppid",
        "getrandom",
        "getsockname",
        "getsockopt",
        "gettid",
        "gettimeofday",
        "getuid",
        "ioctl",
        "listen",
        "lseek",
        "madvise",
        "memfd_create",
        "mincore",
        "mkdir",
        "mmap",
        "mprotect",
        "munmap",
        "nanosleep",
        "newfstatat",
        "open",
        "openat",
        "pipe2",
        "poll",
        "pread64",
        "pwrite64",
        "read",
        "readlink",
        "readlinkat",
        "readv",
        "recvfrom",
        "recvmsg",
        "renameat",
        "rmdir",
        "rt_sigaction",
        "rt_sigprocmask",
        "sched_getaffinity",
        "sched_yield",
        "sendmsg",
        "sendto",
        "set_robust_list",
        "setsockopt",
        "set_tid_address",
        "sigaltstack",
        "socket",
        "socketpair",
        "stat",
        "statfs",
        "symlinkat",
        "sync",
        "tgkill",
        "time",
        "tkill",
        "uname",
        "unlink",
        "unlinkat",
        "utimensat",
        "wait4",
        "write",
        "writev",
        "userfaultfd"
      ],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}`
}
```

---

## Appendix A: Quick Reference

### File Locations

| File/Directory | System Path | User Path |
|----------------|-------------|-----------|
| Binary | `/usr/bin/muster` | `~/.local/bin/muster` |
| Config | `/etc/muster/` | `~/.config/muster/` |
| Data | `/var/lib/muster/` | `~/.local/share/muster/` |
| Cache | N/A | `~/.cache/muster/` |
| Logs | `/var/log/muster/` | `~/.local/state/muster/logs/` |
| Runtime | `/run/muster/` | `${XDG_RUNTIME_DIR}/muster/` |

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `MUSTER_CONFIG_DIR` | Configuration directory | XDG config dir |
| `MUSTER_CACHE_DIR` | Cache directory | XDG cache dir |
| `MUSTER_DATA_DIR` | Data directory | XDG data dir |
| `MUSTER_LOG_LEVEL` | Logging level | `info` |
| `MUSTER_NO_COLOR` | Disable colored output | `false` |

### Build Commands

```bash
# Build Debian package
./scripts/build-deb.sh 1.0.0 amd64

# Build RPM package
./scripts/build-rpm.sh 1.0.0

# Build Alpine package
abuild -r

# Build Arch package
makepkg -si
```

---

**Version**: 1.0  
**Last Updated**: 2024-01-15  
**Platform**: Linux (amd64, arm64)
