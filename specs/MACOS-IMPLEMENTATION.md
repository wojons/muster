# macOS Implementation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Platform

## Overview

Complete specification of **macOS-specific implementations**.

## 1. Directories

```go
func macOSDirs() *PlatformDirs {
    home := os.Getenv("HOME")
    
    return &PlatformDirs{
        Config: filepath.Join(home, "Library", "Application Support", "openapi-cli"),
        Data:   filepath.Join(home, "Library", "Application Support", "openapi-cli"),
        Cache:  filepath.Join(home, "Library", "Caches", "openapi-cli"),
    }
}
```

## 2. Keychain Access

```go
type MacOSKeychain struct{}

func (k *MacOSKeychain) Set(service, account, password string) error {
    cmd := exec.Command("security", "add-generic-password",
        "-s", service,
        "-a", account,
        "-w", password,
        "-U",
    )
    return cmd.Run()
}

func (k *MacOSKeychain) Get(service, account string) (string, error) {
    cmd := exec.Command("security", "find-generic-password",
        "-s", service,
        "-a", account,
        "-w",
    )
    output, err := cmd.Output()
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(string(output)), nil
}
```

## 3. Notarization

```go
// Build with entitlements
func createEntitlements() string {
    return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.security.cs.allow-jit</key>
    <true/>
    <key>com.apple.security.automation.apple-events</key>
    <true/>
</dict>
</plist>`
}
```

## 4. Universal Binary

```go
// Build for both architectures
func buildUniversalBinary() error {
    // Build amd64
    cmd1 := exec.Command("go", "build", "-o", "openapi-cli-amd64")
    cmd1.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=amd64")
    cmd1.Run()
    
    // Build arm64
    cmd2 := exec.Command("go", "build", "-o", "openapi-cli-arm64")
    cmd2.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64")
    cmd2.Run()
    
    // Create universal binary
    cmd3 := exec.Command("lipo", "-create", "-output", "openapi-cli",
        "openapi-cli-amd64", "openapi-cli-arm64")
    return cmd3.Run()
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
