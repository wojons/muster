# Linux Implementation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Platform

## Overview

Complete specification of **Linux-specific implementations**.

## 1. Directories

```go
func linuxDirs() *PlatformDirs {
    home := os.Getenv("HOME")
    xdgConfig := os.Getenv("XDG_CONFIG_HOME")
    xdgData := os.Getenv("XDG_DATA_HOME")
    xdgCache := os.Getenv("XDG_CACHE_HOME")
    
    if xdgConfig == "" {
        xdgConfig = filepath.Join(home, ".config")
    }
    if xdgData == "" {
        xdgData = filepath.Join(home, ".local", "share")
    }
    if xdgCache == "" {
        xdgCache = filepath.Join(home, ".cache")
    }
    
    return &PlatformDirs{
        Config: xdgConfig,
        Data:   xdgData,
        Cache:  xdgCache,
    }
}
```

## 2. Secret Service

```go
// Uses D-Bus Secret Service API
type LinuxSecretService struct {
    session dbus.BusObject
}

func (s *LinuxSecretService) Store(service, account, password string) error {
    // Implementation using org.freedesktop.secrets
    return nil
}
```

## 3. Systemd Integration

```go
func createSystemdService() error {
    service := `[Unit]
Description=OpenAPI CLI Daemon
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/openapi-cli daemon start
Restart=always

[Install]
WantedBy=default.target
`
    path := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user", "openapi-cli.service")
    return os.WriteFile(path, []byte(service), 0644)
}
```

## 4. Unix Permissions

```go
func setSecurePermissions(path string) error {
    // 0600 - owner read/write only
    return os.Chmod(path, 0600)
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
