# Runtime Capabilities Specification

## Overview

Capability-based permission system for WASM execution and DSL operations, providing fine-grained control over resource access and runtime operations.

## Core Components

### Capability Model

```go
// Capability represents a single permission grant
type Capability struct {
    ID          string            `json:"id"`
    Type        CapabilityType    `json:"type"`
    Scope       CapabilityScope `json:"scope"`
    Constraints Constraints       `json:"constraints,omitempty"`
    GrantedAt   time.Time         `json:"granted_at"`
    ExpiresAt   *time.Time        `json:"expires_at,omitempty"`
}

type CapabilityType string

const (
    CapFileRead      CapabilityType = "file:read"
    CapFileWrite     CapabilityType = "file:write"
    CapNetworkHTTP   CapabilityType = "network:http"
    CapNetworkTCP  CapabilityType = "network:tcp"
    CapEnvRead     CapabilityType = "env:read"
    CapEnvWrite    CapabilityType = "env:write"
    CapExec        CapabilityType = "exec:spawn"
    CapSystemInfo  CapabilityType = "system:info"
)

type CapabilityScope struct {
    Paths       []string `json:"paths,omitempty"`
    Hosts       []string `json:"hosts,omitempty"`
    EnvVars     []string `json:"env_vars,omitempty"`
    AllowAll    bool     `json:"allow_all,omitempty"`
}
```

### Capability Manager

```go
type CapabilityManager struct {
    capabilities map[string]*Capability
    mu           sync.RWMutex
    auditor      Auditor
}

func NewCapabilityManager(auditor Auditor) *CapabilityManager {
    return &CapabilityManager{
        capabilities: make(map[string]*Capability),
        auditor:      auditor,
    }
}

func (cm *CapabilityManager) Grant(cap Capability) error {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    
    if err := cm.validateCapability(cap); err != nil {
        return fmt.Errorf("invalid capability: %w", err)
    }
    
    cap.ID = generateCapabilityID()
    cap.GrantedAt = time.Now()
    cm.capabilities[cap.ID] = &cap
    
    cm.auditor.Log(CapabilityGranted, cap)
    return nil
}

func (cm *CapabilityManager) Check(ctx context.Context, capType CapabilityType, resource string) error {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    
    for _, cap := range cm.capabilities {
        if cap.Type != capType {
            continue
        }
        
        if cap.ExpiresAt != nil && time.Now().After(*cap.ExpiresAt) {
            continue
        }
        
        if cm.matchesScope(cap, resource) {
            return nil
        }
    }
    
    return &CapabilityError{Type: capType, Resource: resource}
}

func (cm *CapabilityManager) matchesScope(cap *Capability, resource string) bool {
    if cap.Scope.AllowAll {
        return true
    }
    
    switch cap.Type {
    case CapFileRead, CapFileWrite:
        return cm.matchesPath(cap.Scope.Paths, resource)
    case CapNetworkHTTP, CapNetworkTCP:
        return cm.matchesHost(cap.Scope.Hosts, resource)
    case CapEnvRead, CapEnvWrite:
        return cm.matchesEnv(cap.Scope.EnvVars, resource)
    }
    
    return false
}
```

## Permission Grants

### Default Capability Sets

```go
type CapabilitySet struct {
    Name        string       `json:"name"`
    Capabilities []Capability `json:"capabilities"`
}

var (
    MinimalCapabilitySet = CapabilitySet{
        Name: "minimal",
        Capabilities: []Capability{
            {Type: CapFileRead, Scope: CapabilityScope{Paths: []string{"/tmp"}}},
        },
    }
    
    StandardCapabilitySet = CapabilitySet{
        Name: "standard",
        Capabilities: []Capability{
            {Type: CapFileRead, Scope: CapabilityScope{Paths: []string{"/data", "/tmp"}}},
            {Type: CapFileWrite, Scope: CapabilityScope{Paths: []string{"/tmp"}}},
            {Type: CapNetworkHTTP, Scope: CapabilityScope{Hosts: []string{"*.example.com"}}},
        },
    }
    
    FullCapabilitySet = CapabilitySet{
        Name: "full",
        Capabilities: []Capability{
            {Type: CapFileRead, Scope: CapabilityScope{AllowAll: true}},
            {Type: CapFileWrite, Scope: CapabilityScope{AllowAll: true}},
            {Type: CapNetworkHTTP, Scope: CapabilityScope{AllowAll: true}},
            {Type: CapNetworkTCP, Scope: CapabilityScope{AllowAll: true}},
        },
    }
)
```

## Resource Limits

### Constraint System

```go
type Constraints struct {
    MaxFileSize      int64         `json:"max_file_size,omitempty"`
    MaxMemory        int64         `json:"max_memory,omitempty"`
    MaxCPUTime       time.Duration `json:"max_cpu_time,omitempty"`
    MaxNetworkBytes  int64         `json:"max_network_bytes,omitempty"`
    RateLimit        *RateLimit    `json:"rate_limit,omitempty"`
}

type RateLimit struct {
    RequestsPerSecond float64       `json:"rps"`
    BurstSize         int           `json:"burst"`
    Window            time.Duration `json:"window"`
}

type ResourceTracker struct {
    constraints   Constraints
    usage         ResourceUsage
    rateLimiter   *rate.Limiter
    mu            sync.Mutex
}

func (rt *ResourceTracker) TrackFileRead(size int64) error {
    rt.mu.Lock()
    defer rt.mu.Unlock()
    
    if rt.constraints.MaxFileSize > 0 && size > rt.constraints.MaxFileSize {
        return &ResourceLimitError{Resource: "file_size", Limit: rt.constraints.MaxFileSize, Actual: size}
    }
    
    rt.usage.FileBytesRead += size
    return nil
}

func (rt *ResourceTracker) TrackNetwork(bytes int64) error {
    rt.mu.Lock()
    defer rt.mu.Unlock()
    
    if rt.constraints.MaxNetworkBytes > 0 {
        if rt.usage.NetworkBytes+bytes > rt.constraints.MaxNetworkBytes {
            return &ResourceLimitError{Resource: "network", Limit: rt.constraints.MaxNetworkBytes}
        }
    }
    
    if rt.rateLimiter != nil && !rt.rateLimiter.Allow() {
        return &RateLimitError{Limit: rt.constraints.RateLimit}
    }
    
    rt.usage.NetworkBytes += bytes
    return nil
}
```

## Enforcement

### Enforcement Hooks

```go
type EnforcementPoint interface {
    BeforeOperation(ctx context.Context, op Operation) error
    AfterOperation(ctx context.Context, op Operation, result interface{}) error
}

type Operation struct {
    Type      OperationType `json:"type"`
    Resource  string        `json:"resource"`
    Size      int64         `json:"size,omitempty"`
    Timestamp time.Time     `json:"timestamp"`
}

type OperationType string

const (
    OpFileRead    OperationType = "file:read"
    OpFileWrite   OperationType = "file:write"
    OpNetworkCall OperationType = "network:call"
    OpExec        OperationType = "exec:spawn"
)

type EnforcementMiddleware struct {
    capManager  *CapabilityManager
    tracker     *ResourceTracker
    hooks       []EnforcementPoint
}

func (em *EnforcementMiddleware) Intercept(ctx context.Context, op Operation) error {
    // Check capability
    if err := em.capManager.Check(ctx, CapabilityType(op.Type), op.Resource); err != nil {
        return fmt.Errorf("capability check failed: %w", err)
    }
    
    // Check resource limits
    if err := em.checkResourceLimits(op); err != nil {
        return fmt.Errorf("resource limit exceeded: %w", err)
    }
    
    // Run before hooks
    for _, hook := range em.hooks {
        if err := hook.BeforeOperation(ctx, op); err != nil {
            return fmt.Errorf("enforcement hook failed: %w", err)
        }
    }
    
    return nil
}

func (em *EnforcementMiddleware) checkResourceLimits(op Operation) error {
    switch op.Type {
    case OpFileRead, OpFileWrite:
        return em.tracker.TrackFileRead(op.Size)
    case OpNetworkCall:
        return em.tracker.TrackNetwork(op.Size)
    }
    return nil
}
```

### DSL Integration

```go
type CapabilityAwareRuntime struct {
    base    Runtime
    enforce *EnforcementMiddleware
}

func (r *CapabilityAwareRuntime) Execute(ctx context.Context, script *Script) (*Result, error) {
    // Parse capability requirements from script metadata
    reqs := r.parseRequirements(script)
    
    for _, req := range reqs {
        if err := r.enforce.capManager.Check(ctx, req.Type, req.Resource); err != nil {
            return nil, &CapabilityDeniedError{Requirement: req}
        }
    }
    
    return r.base.Execute(ctx, script)
}

func (r *CapabilityAwareRuntime) CallFunction(ctx context.Context, fn string, args []Value) (Value, error) {
    op := Operation{
        Type:      OpFunctionCall,
        Resource:  fn,
        Timestamp: time.Now(),
    }
    
    if err := r.enforce.Intercept(ctx, op); err != nil {
        return nil, err
    }
    
    return r.base.CallFunction(ctx, fn, args)
}
```

## Error Types

```go
type CapabilityError struct {
    Type     CapabilityType `json:"type"`
    Resource string         `json:"resource"`
}

func (e *CapabilityError) Error() string {
    return fmt.Sprintf("capability %s denied for resource: %s", e.Type, e.Resource)
}

type ResourceLimitError struct {
    Resource string `json:"resource"`
    Limit    int64  `json:"limit"`
    Actual   int64  `json:"actual,omitempty"`
}

func (e *ResourceLimitError) Error() string {
    return fmt.Sprintf("resource limit exceeded: %s (limit: %d, actual: %d)", e.Resource, e.Limit, e.Actual)
}

type CapabilityDeniedError struct {
    Requirement CapabilityRequirement `json:"requirement"`
}

func (e *CapabilityDeniedError) Error() string {
    return fmt.Sprintf("script requires capability %s for %s", e.Requirement.Type, e.Requirement.Resource)
}
```

## Audit Logging

```go
type Auditor interface {
    Log(event AuditEvent, details interface{})
}

type AuditEvent string

const (
    CapabilityGranted AuditEvent = "capability:granted"
    CapabilityRevoked AuditEvent = "capability:revoked"
    CapabilityDenied  AuditEvent = "capability:denied"
    ResourceLimitHit  AuditEvent = "resource:limit_hit"
)

type AuditLogger struct {
    writer io.Writer
    mu     sync.Mutex
}

func (al *AuditLogger) Log(event AuditEvent, details interface{}) {
    al.mu.Lock()
    defer al.mu.Unlock()
    
    entry := AuditEntry{
        Timestamp: time.Now().UTC(),
        Event:     event,
        Details:   details,
    }
    
    json.NewEncoder(al.writer).Encode(entry)
}
```
