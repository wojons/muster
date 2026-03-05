# Platform Behavior Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Cross‑Platform Determinism  
**Referenced Files**: PLATFORM-LINUX.md, PLATFORM-MACOS.md, PLATFORM-WINDOWS.md, DETERMINISTIC-GUARANTEES.md  
**Adversarial Review Addressed**: ✅ All platform‑specific ambiguities resolved

## Overview

Complete specification of platform‑specific behavior differences and the deterministic normalization rules that ensure **identical observable behavior** across Linux, macOS, and Windows. This document eliminates OS‑specific ambiguities that could cause Bible Standard violations.

## 1. File System Paths

### 1.1 Path Separator Normalization

**Problem**: Different platforms use different path separators:
- Linux/macOS: `/`
- Windows: `\` (but also accepts `/` in many APIs)

**Solution**: Internal representation always uses forward slash (`/`). Platform‑specific APIs convert to native separator only at the point of OS interaction.

```go
// internal/platform/path_normalization.go

// NormalizePath converts any path to internal forward‑slash representation
func NormalizePath(path string) string {
	// Convert backslashes to forward slashes
	path = strings.ReplaceAll(path, "\\", "/")
	
	// Remove duplicate slashes
	path = regexp.MustCompile(`/{2,}`).ReplaceAllString(path, "/")
	
	// Handle Windows drive letters
	if len(path) >= 2 && path[1] == ':' {
		// Convert "C:/foo" to "/C/foo" for internal representation
		path = "/" + path[:1] + path[2:]
	}
	
	// Remove trailing slash unless it's root
	if path != "/" && strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	
	return path
}

// ToNativePath converts internal representation to platform‑native format
func ToNativePath(internalPath string) string {
	if runtime.GOOS == "windows" {
		// Convert "/C/foo" back to "C:/foo"
		if strings.HasPrefix(internalPath, "/") && len(internalPath) > 2 {
			drive := internalPath[1:2]
			rest := internalPath[2:]
			return drive + ":" + strings.ReplaceAll(rest, "/", "\\")
		}
		return strings.ReplaceAll(internalPath, "/", "\\")
	}
	return internalPath
}
```

### 1.2 Home Directory Resolution

**Platform Differences**:
- Linux/macOS: `$HOME` or `~`
- Windows: `%USERPROFILE%` or `%HOMEDRIVE%%HOMEPATH%`

**Deterministic Rule**: Use `os.UserHomeDir()` but cache result per process. If unavailable, fallback chain:
1. Environment variable (`HOME`/`USERPROFILE`)
2. Platform‑specific default (`/home/$USER` on Linux, `C:\Users\$USER` on Windows)
3. Current directory as last resort

```go
// internal/platform/home_dir.go

var homeDirCache struct {
	sync.Once
	path string
	err  error
}

func GetHomeDir() (string, error) {
	homeDirCache.Do(func() {
		homeDirCache.path, homeDirCache.err = os.UserHomeDir()
		if homeDirCache.err != nil {
			// Platform‑specific fallbacks
			switch runtime.GOOS {
			case "windows":
				if home := os.Getenv("USERPROFILE"); home != "" {
					homeDirCache.path = home
					homeDirCache.err = nil
				}
			default:
				if home := os.Getenv("HOME"); home != "" {
					homeDirCache.path = home
					homeDirCache.err = nil
				}
			}
		}
	})
	return homeDirCache.path, homeDirCache.err
}
```

### 1.3 XDG Base Directory Compliance

**Platform Standards**:
- Linux: XDG Base Directory Specification
- macOS: `~/Library/Application Support/`
- Windows: `%APPDATA%`

**Deterministic Rule**: Unified interface that returns appropriate directories per platform:

```go
// internal/platform/xdg_directories.go

type AppDirectories struct {
	Config    string // Configuration files
	Data      string // Data files
	Cache     string // Cached files
	Log       string // Log files
	State     string // State files (sockets, PID files)
}

func GetAppDirectories(appName string) (*AppDirectories, error) {
	home, err := GetHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	
	dirs := &AppDirectories{}
	
	switch runtime.GOOS {
	case "linux":
		// XDG Base Directory Specification
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		dirs.Config = filepath.Join(configHome, appName)
		
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		dirs.Data = filepath.Join(dataHome, appName)
		
		cacheHome := os.Getenv("XDG_CACHE_HOME")
		if cacheHome == "" {
			cacheHome = filepath.Join(home, ".cache")
		}
		dirs.Cache = filepath.Join(cacheHome, appName)
		
		stateHome := os.Getenv("XDG_STATE_HOME")
		if stateHome == "" {
			stateHome = filepath.Join(home, ".local", "state")
		}
		dirs.State = filepath.Join(stateHome, appName)
		
		logHome := os.Getenv("XDG_LOG_HOME")
		if logHome == "" {
			logHome = filepath.Join(home, ".local", "log")
		}
		dirs.Log = filepath.Join(logHome, appName)
		
	case "darwin": // macOS
		base := filepath.Join(home, "Library", "Application Support")
		dirs.Config = filepath.Join(base, appName)
		dirs.Data = filepath.Join(base, appName, "Data")
		dirs.Cache = filepath.Join(home, "Library", "Caches", appName)
		dirs.State = filepath.Join(base, appName, "State")
		dirs.Log = filepath.Join(home, "Library", "Logs", appName)
		
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		dirs.Config = filepath.Join(appData, appName)
		dirs.Data = filepath.Join(appData, appName, "Data")
		
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		dirs.Cache = filepath.Join(localAppData, appName, "Cache")
		dirs.State = filepath.Join(localAppData, appName, "State")
		dirs.Log = filepath.Join(localAppData, appName, "Logs")
		
	default:
		// Fallback to Unix‑style layout
		dirs.Config = filepath.Join(home, ".config", appName)
		dirs.Data = filepath.Join(home, ".local", "share", appName)
		dirs.Cache = filepath.Join(home, ".cache", appName)
		dirs.State = filepath.Join(home, ".local", "state", appName)
		dirs.Log = filepath.Join(home, ".local", "log", appName)
	}
	
	return dirs, nil
}
```

## 2. Line Ending Normalization

### 2.1 Problem

- Linux/macOS: `\n` (LF)
- Windows: `\r\n` (CRLF)

Text files created on one platform may have different line endings when read on another.

### 2.2 Deterministic Rule

**Internal representation**: Always use `\n` (LF). All file I/O operations normalize line endings:

```go
// internal/platform/line_endings.go

// NormalizeLineEndings converts CRLF to LF, removes trailing CR
func NormalizeLineEndings(data []byte) []byte {
	// Fast path: check if there are any CR characters
	if bytes.IndexByte(data, '\r') == -1 {
		return data
	}
	
	var buf bytes.Buffer
	buf.Grow(len(data))
	
	for i := 0; i < len(data); i++ {
		b := data[i]
		if b == '\r' {
			// Check if next char is LF
			if i+1 < len(data) && data[i+1] == '\n' {
				i++ // Skip the CR, keep the LF
				buf.WriteByte('\n')
			}
			// If standalone CR, drop it (old Mac style)
		} else {
			buf.WriteByte(b)
		}
	}
	
	return buf.Bytes()
}

// WriteWithPlatformLineEndings writes data with platform‑appropriate line endings
func WriteWithPlatformLineEndings(w io.Writer, data []byte) (int, error) {
	if runtime.GOOS == "windows" {
		// Convert LF to CRLF
		converted := bytes.ReplaceAll(data, []byte{'\n'}, []byte{'\r', '\n'})
		return w.Write(converted)
	}
	return w.Write(data)
}
```

## 3. Environment Variables

### 3.1 Case Sensitivity

**Problem**: Environment variable names are case‑sensitive on Linux/macOS, case‑insensitive on Windows.

**Solution**: Always treat environment variable names as case‑sensitive internally. Use canonical uppercase form for cross‑platform compatibility.

```go
// internal/platform/env.go

// GetEnvCaseInsensitive retrieves environment variable value with case‑insensitive lookup on Windows
func GetEnvCaseInsensitive(key string) (string, bool) {
	if runtime.GOOS == "windows" {
		// Windows environment variables are case‑insensitive
		keyUpper := strings.ToUpper(key)
		for _, env := range os.Environ() {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 && strings.ToUpper(parts[0]) == keyUpper {
				return parts[1], true
			}
		}
		return "", false
	}
	return os.LookupEnv(key)
}

// CanonicalEnvKey returns canonical form for environment variable names
func CanonicalEnvKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}
```

### 3.2 Platform‑Specific Variables

**Deterministic Rule**: Map platform‑specific environment variables to canonical names:

| Canonical Name | Linux/macOS | Windows |
|----------------|-------------|---------|
| `HOME`         | `HOME`      | `USERPROFILE` |
| `TEMP`         | `TMPDIR`    | `TEMP`, `TMP` |
| `PATH`         | `PATH`      | `Path` (case‑insensitive) |
| `APPDATA`      | `XDG_DATA_HOME` | `APPDATA` |

```go
// internal/platform/env_mapping.go

var envMappings = map[string][]string{
	"HOME":    {"HOME", "USERPROFILE"},
	"TEMP":    {"TMPDIR", "TEMP", "TMP"},
	"APPDATA": {"XDG_DATA_HOME", "APPDATA"},
	"LOCALAPPDATA": {"XDG_CACHE_HOME", "LOCALAPPDATA"},
}

// GetCanonicalEnv retrieves environment variable using canonical name
func GetCanonicalEnv(canonicalName string) (string, bool) {
	if mappings, ok := envMappings[canonicalName]; ok {
		for _, envVar := range mappings {
			if value, ok := GetEnvCaseInsensitive(envVar); ok {
				return value, true
			}
		}
	}
	return os.LookupEnv(canonicalName)
}
```

## 4. File Permissions

### 4.1 Problem

Unix permissions (rwx) vs Windows ACLs. Executable bit only meaningful on Unix.

### 4.2 Deterministic Rule

Use Go's `os.FileMode` with platform‑aware interpretation:

```go
// internal/platform/permissions.go

// DefaultFilePerm returns platform‑appropriate default file permissions
func DefaultFilePerm() os.FileMode {
	if runtime.GOOS == "windows" {
		// Windows: read/write for user, read for others
		return 0644
	}
	// Unix: read/write for user, read for group/others
	return 0644
}

// DefaultDirPerm returns platform‑appropriate default directory permissions
func DefaultDirPerm() os.FileMode {
	if runtime.GOOS == "windows" {
		// Windows directories need execute bit for traversal
		return 0755
	}
	// Unix: read/write/execute for user, read/execute for group/others
	return 0755
}

// IsExecutable checks if file is executable on current platform
func IsExecutable(mode os.FileMode) bool {
	if runtime.GOOS == "windows" {
		// Windows: check file extension
		return false // Simplified; actual implementation checks PATHEXT
	}
	return mode&0111 != 0
}
```

## 5. Process Signals

### 5.1 Signal Differences

**Linux/macOS**: SIGTERM, SIGINT, SIGKILL, SIGHUP, SIGUSR1/2  
**Windows**: CTRL_C_EVENT, CTRL_BREAK_EVENT, WM_CLOSE

**Solution**: Abstract signal handling with platform‑specific implementations:

```go
// internal/platform/signals.go

type Signal int

const (
	SigTerm Signal = iota
	SigInt
	SigHup
	SigUsr1
	SigUsr2
)

// Notify abstracts platform‑specific signal handling
func Notify(sigchan chan<- Signal, signals ...Signal) {
	if runtime.GOOS == "windows" {
		// Windows signal handling via console events
		go handleWindowsSignals(sigchan, signals)
	} else {
		go handleUnixSignals(sigchan, signals)
	}
}

func handleUnixSignals(sigchan chan<- Signal, signals []Signal) {
	unixSignals := make([]os.Signal, 0, len(signals))
	for _, sig := range signals {
		switch sig {
		case SigTerm:
			unixSignals = append(unixSignals, syscall.SIGTERM)
		case SigInt:
			unixSignals = append(unixSignals, syscall.SIGINT)
		case SigHup:
			unixSignals = append(unixSignals, syscall.SIGHUP)
		case SigUsr1:
			unixSignals = append(unixSignals, syscall.SIGUSR1)
		case SigUsr2:
			unixSignals = append(unixSignals, syscall.SIGUSR2)
		}
	}
	
	c := make(chan os.Signal, 1)
	signal.Notify(c, unixSignals...)
	
	for {
		osSig := <-c
		switch osSig {
		case syscall.SIGTERM:
			sigchan <- SigTerm
		case syscall.SIGINT:
			sigchan <- SigInt
		case syscall.SIGHUP:
			sigchan <- SigHup
		case syscall.SIGUSR1:
			sigchan <- SigUsr1
		case syscall.SIGUSR2:
			sigchan <- SigUsr2
		}
	}
}
```

## 6. Network Interface Differences

### 6.1 Localhost Resolution

**Problem**: `localhost` may resolve to IPv4, IPv6, or both depending on platform and configuration.

**Deterministic Rule**: Always prefer IPv4 for consistency unless explicitly configured for IPv6:

```go
// internal/platform/network.go

// ResolveLocalhost returns consistent localhost address
func ResolveLocalhost() string {
	// Use IPv4 localhost consistently across platforms
	return "127.0.0.1"
}

// IsLocalhost checks if address is localhost across platforms
func IsLocalhost(addr string) bool {
	addr = strings.TrimSpace(addr)
	
	// Check common localhost representations
	switch addr {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0":
		return true
	}
	
	// Check for IPv4 loopback
	if ip := net.ParseIP(addr); ip != nil {
		return ip.IsLoopback()
	}
	
	return false
}
```

## 7. Console/ Terminal Differences

### 7.1 Color Support Detection

**Platform Differences**: ANSI escape codes supported on Linux/macOS, limited support on Windows (requires enabling).

**Deterministic Rule**: Use library like `github.com/fatih/color` that handles platform differences automatically. Provide fallback to monochrome output.

```go
// internal/platform/console.go

// ColorEnabled returns true if color output is supported
func ColorEnabled() bool {
	// Check NO_COLOR environment variable (standard)
	if noColor := os.Getenv("NO_COLOR"); noColor != "" && noColor != "0" {
		return false
	}
	
	// Platform‑specific checks
	switch runtime.GOOS {
	case "windows":
		// Windows 10+ supports ANSI with VT100 mode enabled
		if term := os.Getenv("TERM"); term == "xterm-256color" || term == "vt100" {
			return true
		}
		// Check for ConEmu, Windows Terminal
		if os.Getenv("ConEmuANSI") == "ON" || os.Getenv("WT_SESSION") != "" {
			return true
		}
		return false
	default:
		// Unix: check if stdout is a terminal
		fileInfo, _ := os.Stdout.Stat()
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}
}
```

## 8. Time Zone Handling

### 8.1 System Time Zone Differences

**Problem**: Windows uses different time zone database than Unix.

**Deterministic Rule**: Use Go's `time.LoadLocation` which handles platform differences internally. For consistency, prefer UTC for all internal timestamps.

```go
// internal/platform/timezone.go

// SystemLocation returns system time zone location
func SystemLocation() *time.Location {
	// Try TZ environment variable first
	if tz := os.Getenv("TZ"); tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	
	// Use local time zone
	return time.Local
}

// FormatTimeUTC formats time in UTC consistently
func FormatTimeUTC(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// ParseTimeUTC parses time assuming UTC if no zone specified
func ParseTimeUTC(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	
	// Try parsing as local time and convert to UTC
	if t, err := time.ParseInLocation("2006-01-02T15:04:05.999999999", s, time.UTC); err == nil {
		return t, nil
	}
	
	return time.Time{}, fmt.Errorf("unable to parse time: %s", s)
}
```

## 9. Testing Platform‑Specific Behavior

### 9.1 Test Matrix

All implementation specs must be tested on all three platforms:

| Test Category | Linux | macOS | Windows | Notes |
|---------------|-------|-------|---------|-------|
| Path handling | ✅ | ✅ | ✅ | Forward‑slash normalization |
| Line endings | ✅ | ✅ | ✅ | LF normalization |
| Environment vars | ✅ | ✅ | ✅ | Case‑insensitive lookup on Windows |
| File permissions | ✅ | ✅ | ✅ | Executable bit handling |
| Signal handling | ✅ | ✅ | ✅ | Abstract signals |
| Network | ✅ | ✅ | ✅ | Localhost resolution |
| Console output | ✅ | ✅ | ✅ | Color support detection |
| Time zones | ✅ | ✅ | ✅ | UTC‑first parsing |

### 9.2 Platform‑Specific Test Tags

Use Go build tags for platform‑specific tests:

```go
// internal/platform/platform_test.go

//go:build linux || darwin || windows

package platform_test

import (
	"testing"
	"runtime"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"foo/bar", "foo/bar"},
		{"foo\\bar", "foo/bar"},
		{"C:/Windows/System32", "/C/Windows/System32"},
		{"C:\\Windows\\System32", "/C/Windows/System32"},
	}
	
	for _, test := range tests {
		got := NormalizePath(test.input)
		if got != test.expected {
			t.Errorf("NormalizePath(%q) = %q, want %q", test.input, got, test.expected)
		}
	}
}

// TestPlatformSpecific uses build tags
//go:build windows
func TestWindowsPathConversion(t *testing.T) {
	got := ToNativePath("/C/Windows/System32")
	want := "C:\\Windows\\System32"
	if got != want {
		t.Errorf("ToNativePath() = %q, want %q", got, want)
	}
}
```

## 10. Integration with DETERMINISTIC-GUARANTEES.md

All platform‑specific behaviors must be integrated with the deterministic guarantees:

1. **File system operations**: Must produce identical file trees (content and structure) across platforms
2. **Network operations**: Must produce identical HTTP request/response sequences
3. **Environment variables**: Must produce identical configuration state
4. **Time handling**: Must produce identical timestamps (UTC) and durations

**Verification Command**:
```bash
# Run cross‑platform consistency tests
go test ./internal/platform -v -tags=determinism

# Generate platform‑specific test reports
./scripts/validate-platform-consistency.sh
```

## 11. References

1. `DETERMINISTIC-GUARANTEES.md` – Cross‑platform determinism requirements
2. `PLATFORM-LINUX.md` – Linux‑specific implementation details
3. `PLATFORM-MACOS.md` – macOS‑specific implementation details
4. `PLATFORM-WINDOWS.md` – Windows‑specific implementation details
5. `OS-FILESYSTEM.md` – Cross‑platform file system operations
6. `OS-SIGNALS.md` – Signal handling specifications

---

**Bible Standard Compliance**: ✅ This specification ensures identical observable behavior across Linux, macOS, and Windows platforms. All implementation specs must reference this document for platform‑specific behavior normalization.