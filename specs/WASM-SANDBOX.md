# WASM Sandbox Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Security

## Overview

Complete specification of the **WASM sandbox** security system, providing capability-based isolation and resource limits.

## 1. Sandbox Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Host System                              │
├─────────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────────────────────────────┐  │
│  │                   WASM Sandbox                        │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │  │
│  │  │  Module A   │  │  Module B   │  │  Module C   │  │  │
│  │  │  (isolated) │  │  (isolated) │  │  (isolated) │  │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘  │  │
│  │                                                      │  │
│  │  ┌────────────────────────────────────────────────┐  │  │
│  │  │         Capability Manager                      │  │  │
│  │  │  - File access                                 │  │  │
│  │  │  - Network access                              │  │  │
│  │  │  - Environment                                 │  │  │
│  │  └────────────────────────────────────────────────┘  │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## 2. Capability System

```go
type Capability struct {
    Name        string            `json:"name"`
    Permissions []Permission      `json:"permissions"`
    Constraints map[string]string `json:"constraints"`
}

type Permission string

const (
    PermRead   Permission = "read"
    PermWrite  Permission = "write"
    PermExecute Permission = "execute"
    PermCreate Permission = "create"
    PermDelete Permission = "delete"
)
```

## 3. Resource Limits

```go
type SandboxLimits struct {
    MaxMemory        uint64        `json:"max_memory"`
    MaxExecutionTime time.Duration `json:"max_execution_time"`
    MaxFileDescriptors uint32      `json:"max_file_descriptors"`
    MaxFileSize      uint64        `json:"max_file_size"`
    MaxNetworkConnections uint32   `json:"max_network_connections"`
    MaxCPUTime       time.Duration `json:"max_cpu_time"`
}

var DefaultLimits = SandboxLimits{
    MaxMemory:             128 * 1024 * 1024, // 128MB
    MaxExecutionTime:      30 * time.Second,
    MaxFileDescriptors:    64,
    MaxFileSize:           10 * 1024 * 1024,  // 10MB
    MaxNetworkConnections: 10,
    MaxCPUTime:            5 * time.Second,
}
```

## 4. Sandboxed Filesystem

```go
type SandboxedFS struct {
    allowedDirs   map[string]bool
    readOnlyDirs  map[string]bool
    virtualFiles  map[string][]byte
}

func (fs *SandboxedFS) Open(path string, flags int) (File, error) {
    // Normalize path
    cleanPath := filepath.Clean(path)
    
    // Check if path is allowed
    if !fs.isPathAllowed(cleanPath) {
        return nil, fmt.Errorf("path not allowed: %s", cleanPath)
    }
    
    // Check read-only restriction
    if flags&(os.O_WRONLY|os.O_RDWR) != 0 && fs.isReadOnly(cleanPath) {
        return nil, fmt.Errorf("path is read-only: %s", cleanPath)
    }
    
    return os.OpenFile(cleanPath, flags, 0644)
}
```

## 5. Network Isolation

```go
type NetworkPolicy struct {
    AllowedHosts []string `json:"allowed_hosts"`
    AllowedPorts []int    `json:"allowed_ports"`
    BlockedHosts []string `json:"blocked_hosts"`
}

func (p *NetworkPolicy) IsAllowed(host string, port int) bool {
    // Check blocked list first
    for _, blocked := range p.BlockedHosts {
        if matched, _ := filepath.Match(blocked, host); matched {
            return false
        }
    }
    
    // Check allowed hosts
    for _, allowed := range p.AllowedHosts {
        if matched, _ := filepath.Match(allowed, host); matched {
            // Check port
            for _, allowedPort := range p.AllowedPorts {
                if allowedPort == port {
                    return true
                }
            }
        }
    }
    
    return false
}
```

## 6. Environment Isolation

```go
func createIsolatedEnv(allowedVars []string) []string {
    env := []string{}
    for _, e := range os.Environ() {
        parts := strings.SplitN(e, "=", 2)
        if len(parts) != 2 {
            continue
        }
        
        for _, allowed := range allowedVars {
            if matched, _ := filepath.Match(allowed, parts[0]); matched {
                env = append(env, e)
                break
            }
        }
    }
    return env
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
