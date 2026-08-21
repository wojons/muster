# macOS Platform Features

This document summarizes macOS-specific features implemented in Muster.

## Overview

Muster provides native macOS integration through platform-specific implementations in `pkg/platform/` and comprehensive specifications in `specs/PLATFORM-MACOS.md`.

## Implemented Features

### 1. Platform Directories

**File:** `pkg/platform/darwin.go`

macOS uses standard macOS directory conventions:

| Purpose | Path |
|---------|------|
| Config | `~/Library/Application Support/openapi-cli/` |
| Data | `~/Library/Application Support/openapi-cli/` |
| Cache | `~/Library/Caches/openapi-cli/` |
| State | `~/Library/Application Support/openapi-cli/` |

**Implementation:**
```go
func macOSDirs() *PlatformDirs {
    home := os.Getenv("HOME")
    return &PlatformDirs{
        Config: filepath.Join(home, "Library", "Application Support", "openapi-cli"),
        Data:   filepath.Join(home, "Library", "Application Support", "openapi-cli"),
        Cache:  filepath.Join(home, "Library", "Caches", "openapi-cli"),
        State:  filepath.Join(home, "Library", "Application Support", "openapi-cli"),
    }
}
```

### 2. Keychain Integration

**File:** `pkg/platform/integration_darwin_test.go`

Muster integrates with macOS Keychain for secure credential storage:

- **Service:** Uses macOS `security` command for keychain operations
- **Features:**
  - Add generic passwords to keychain
  - Retrieve passwords from keychain
  - Delete credentials from keychain
  - Test service verification

**Keychain Tests:**
- `TestMacOSKeychainIntegration` - Tests security command interface
- `TestKeychainBackendMacOS` - Tests Store/Retrieve/Delete operations

**Usage:**
```go
storage := DefaultSecureStorage()
err := storage.Store("my-service", "my-account", "my-password")
value, err := storage.Retrieve("my-service", "my-account")
err := storage.Delete("my-service", "my-account")
```

### 3. Notifications

**Status:** Partially implemented via specification

The macOS platform specification includes notification support via:
- LaunchAgent configuration for user notifications
- LaunchDaemon for system-wide notifications
- UserNotifications framework integration (documented in spec)

**See:** `specs/PLATFORM-MACOS.md` lines 14-184 for notification configurations

### 4. Build Tags

All macOS-specific code uses Go build tags:

```go
//go:build darwin
// +build darwin
```

This ensures:
- macOS code only compiles on macOS
- Cross-compilation works for other platforms
- CI/CD can target specific platforms

## Platform Tests

**File:** `pkg/platform/integration_darwin_test.go`

Tests verify:
1. Keychain accessibility
2. Security command interface
3. Credential storage/retrieval
4. Cleanup operations

Run macOS-specific tests:
```bash
go test -tags=darwin ./pkg/platform/... -v
```

## Specifications

Complete macOS implementation details are in:

- **`specs/PLATFORM-MACOS.md`** - Comprehensive 1695-line specification covering:
  - System Integration (LaunchAgent, LaunchDaemon)
  - Package Distribution (Homebrew, PKG, DMG)
  - Keychain Access (Security framework, Access Groups)
  - File System (Standard directories, Sandboxing)
  - Network (System proxy, NetworkExtension)
  - Security (Code signing, Hardened runtime, Entitlements)

## File Locations

| File | Purpose |
|------|---------|
| `pkg/platform/darwin.go` | Platform directories implementation |
| `pkg/platform/integration_darwin_test.go` | macOS integration tests |
| `specs/PLATFORM-MACOS.md` | Complete specification |
| `docs/PLATFORM-MACOS-FEATURES.md` | This feature summary |

## Commands

### Build for macOS
```bash
GOOS=darwin GOARCH=amd64 go build ./cmd/openapi-cli
```

### Build for Apple Silicon
```bash
GOOS=darwin GOARCH=arm64 go build ./cmd/openapi-cli
```

### Test on macOS
```bash
go test ./pkg/platform/... -v
```

### Check Keychain Access
```bash
security list-keychains
security find-generic-password -s "muster-test-service" -a "muster-test-account" -w
```

## Integration Status

| Feature | Status | Notes |
|---------|--------|-------|
| Platform Directories | ✅ Complete | `darwin.go` implemented |
| Keychain Storage | ✅ Complete | Integration tests passing |
| LaunchAgent | 📋 Documented | See spec for plist templates |
| LaunchDaemon | 📋 Documented | See spec for system daemon config |
| Code Signing | 📋 Documented | See spec for signing scripts |
| Notarization | 📋 Documented | See spec for notarization process |
| Homebrew Formula | 📋 Documented | See spec for formula template |
| PKG/DMG Creation | 📋 Documented | See spec for build scripts |

## See Also

- [PLATFORM-MACOS.md](../specs/PLATFORM-MACOS.md) - Full specification
- [Keychain Implementation](../pkg/auth/keychain.go) - Cross-platform keychain
- [Platform Linux](PLATFORM-LINUX.md) - Linux platform features
- [Platform Windows](PLATFORM-WINDOWS.md) - Windows platform features

---

**Last Updated:** 2026-04-02
**Status:** Features documented, implementation verified
