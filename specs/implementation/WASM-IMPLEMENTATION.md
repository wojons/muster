# WASM Runtime Implementation Specification

> STATUS: HISTORICAL — the paths referenced here come from an early layout that was never
> built (`internal/core/wasm/`). The real WASM runtime is `pkg/wasm/` (`runtime.go`,
> `module.go`, `middleware.go`, `sandbox.go`). Treat the paths below as design-time labels,
> not real paths.

## Overview

This document provides the complete production-ready Go implementation for a WASM runtime using Wasmtime. The runtime provides secure sandboxed execution of WebAssembly modules with capability-based access control.

## Project Structure

```
internal/core/wasm/
├── runtime.go          # Main runtime interface and implementation
├── module.go           # Module management
├── memory.go           # Linear memory operations
├── host.go             # Host function implementations
├── capability.go       # Capability enforcement
├── cache.go            # Module caching
├── errors.go           # Error definitions
└── internal/
    └── linker.go       # Wasmtime linker wrapper
```

## Dependencies

```go
import (
    "github.com/bytecodealliance/wasmtime-go"
    "github.com/openapi-cli/core/internal/core/http"
)
```

## 1. Error Definitions

```go
// internal/core/wasm/errors.go

package wasm

import "fmt"

type ErrorCode int

const (
    ErrCodeUnknown ErrorCode = iota
    ErrCodeModuleLoadFailed
    ErrCodeExecutionFailed
    ErrCodeCapabilityDenied
    ErrCodeMemoryAccess
    ErrCodeInvalidInput
    ErrCodeTimeout
    ErrCodeResourceExhausted
)

func (c ErrorCode) String() string {
    switch c {
    case ErrCodeModuleLoadFailed:
        return "MODULE_LOAD_FAILED"
    case ErrCodeExecutionFailed:
        return "EXECUTION_FAILED"
    case ErrCodeCapabilityDenied:
        return "CAPABILITY_DENIED"
    case ErrCodeMemoryAccess:
        return "MEMORY_ACCESS_ERROR"
    case ErrCodeInvalidInput:
        return "INVALID_INPUT"
    case ErrCodeTimeout:
        return "EXECUTION_TIMEOUT"
    case ErrCodeResourceExhausted:
        return "RESOURCE_EXHAUSTED"
    default:
        return "UNKNOWN_ERROR"
    }
}

type WasmError struct {
    Code    ErrorCode
    Message string
    Cause   error
    Module  string
    Function string
}

func (e *WasmError) Error() string {
    if e.Cause != nil {
        return fmt.Sprintf("[%s] %s: %v (module=%s, func=%s)", 
            e.Code, e.Message, e.Cause, e.Module, e.Function)
    }
    return fmt.Sprintf("[%s] %s (module=%s, func=%s)", 
        e.Code, e.Message, e.Module, e.Function)
}

func (e *WasmError) Unwrap() error {
    return e.Cause
}

func NewError(code ErrorCode, msg string, cause error) *WasmError {
    return &WasmError{Code: code, Message: msg, Cause: cause}
}
```

## 2. Configuration

```go
// internal/core/wasm/config.go

package wasm

import (
    "time"
)

// RuntimeConfig configures the WASM runtime
type RuntimeConfig struct {
    // Engine settings
    Engine EngineConfig `json:"engine"`
    
    // Memory limits
    Memory MemoryLimits `json:"memory"`
    
    // Execution limits
    Execution ExecutionLimits `json:"execution"`
    
    // Cache settings
    Cache CacheConfig `json:"cache"`
    
    // Host function settings
    Host HostConfig `json:"host"`
}

// EngineConfig configures the Wasmtime engine
type EngineConfig struct {
    // Enable debug info for better error messages
    DebugInfo bool `json:"debug_info"`
    
    // Enable WASM backtrace on errors
    WasmBacktrace bool `json:"wasm_backtrace"`
    
    // Enable Cranelift optimizations
    CraneliftOptLevel string `json:"cranelift_opt_level"` // "none", "speed", "speed_and_size"
    
    // Enable parallel compilation
    ParallelCompilation bool `json:"parallel_compilation"`
    
    // Maximum memory size for compiler
    MaxWasmStack int `json:"max_wasm_stack"`
}

// MemoryLimits defines memory constraints
type MemoryLimits struct {
    // Maximum memory per module (bytes)
    MaxMemory int64 `json:"max_memory"`
    
    // Maximum memory growth (bytes)
    MaxMemoryGrowth int64 `json:"max_memory_growth"`
    
    // Initial memory size (bytes)
    InitialMemory int64 `json:"initial_memory"`
    
    // Enable memory boundary checks
    EnableBoundsCheck bool `json:"enable_bounds_check"`
}

// ExecutionLimits defines execution constraints
type ExecutionLimits struct {
    // Maximum execution time
    MaxExecutionTime time.Duration `json:"max_execution_time"`
    
    // Maximum fuel units (instruction counter)
    MaxFuel uint64 `json:"max_fuel"`
    
    // Enable fuel consumption tracking
    EnableFuel bool `json:"enable_fuel"`
    
    // Maximum call stack depth
    MaxCallStackDepth int `json:"max_call_stack_depth"`
}

// CacheConfig configures module caching
type CacheConfig struct {
    // Enable module caching
    Enabled bool `json:"enabled"`
    
    // Cache directory
    Directory string `json:"directory"`
    
    // Maximum cached modules
    MaxModules int `json:"max_modules"`
    
    // Maximum age of cached modules
    MaxAge time.Duration `json:"max_age"`
}

// HostConfig configures host function access
type HostConfig struct {
    // Enable HTTP access
    EnableHTTP bool `json:"enable_http"`
    
    // Enable filesystem access
    EnableFilesystem bool `json:"enable_filesystem"`
    
    // Enable logging
    EnableLogging bool `json:"enable_logging"`
    
    // Allowed HTTP hosts (empty = all allowed)
    AllowedHTTPHosts []string `json:"allowed_http_hosts"`
    
    // Allowed filesystem paths
    AllowedFSPaths []string `json:"allowed_fs_paths"`
    
    // Read-only filesystem paths
    ReadOnlyFSPaths []string `json:"readonly_fs_paths"`
    
    // Log level
    LogLevel string `json:"log_level"`
}

// DefaultConfig returns default runtime configuration
func DefaultConfig() *RuntimeConfig {
    return &RuntimeConfig{
        Engine: EngineConfig{
            DebugInfo:           true,
            WasmBacktrace:       true,
            CraneliftOptLevel:   "speed",
            ParallelCompilation: true,
            MaxWasmStack:        2 * 1024 * 1024, // 2MB
        },
        Memory: MemoryLimits{
            MaxMemory:         256 * 1024 * 1024, // 256MB
            MaxMemoryGrowth:   128 * 1024 * 1024, // 128MB
            InitialMemory:     64 * 1024 * 1024,  // 64MB
            EnableBoundsCheck: true,
        },
        Execution: ExecutionLimits{
            MaxExecutionTime:  30 * time.Second,
            MaxFuel:           10000000000, // 10 billion instructions
            EnableFuel:        false,
            MaxCallStackDepth: 1000,
        },
        Cache: CacheConfig{
            Enabled:    true,
            Directory:  "/tmp/wasm-cache",
            MaxModules: 100,
            MaxAge:     24 * time.Hour,
        },
        Host: HostConfig{
            EnableHTTP:       true,
            EnableFilesystem: true,
            EnableLogging:    true,
            LogLevel:         "info",
        },
    }
}
```

## 3. Capability System

```go
// internal/core/wasm/capability.go

package wasm

import (
    "fmt"
    "path/filepath"
    "strings"
    "time"
)

// CapabilityGrant represents a set of permissions granted to a module
type CapabilityGrant struct {
    // Unique grant ID
    ID string `json:"id"`
    
    // Module hash this grant applies to
    ModuleHash string `json:"module_hash"`
    
    // HTTP permissions
    HTTP HTTPPermission `json:"http"`
    
    // Filesystem permissions
    Filesystem FSPermission `json:"filesystem"`
    
    // Resource limits
    Resources ResourceLimits `json:"resources"`
    
    // Grant expiration time
    ExpiresAt *time.Time `json:"expires_at,omitempty"`
    
    // Grant metadata
    Metadata map[string]string `json:"metadata"`
}

// HTTPPermission defines HTTP access permissions
type HTTPPermission struct {
    // Enable HTTP access
    Enabled bool `json:"enabled"`
    
    // Allowed HTTP methods
    AllowedMethods []string `json:"allowed_methods"`
    
    // Allowed hosts (empty = all allowed if Enabled)
    AllowedHosts []string `json:"allowed_hosts"`
    
    // Allowed URL schemes
    AllowedSchemes []string `json:"allowed_schemes"`
    
    // Maximum request body size
    MaxRequestBodySize int64 `json:"max_request_body_size"`
    
    // Maximum response body size
    MaxResponseBodySize int64 `json:"max_response_body_size"`
    
    // Request timeout
    Timeout time.Duration `json:"timeout"`
    
    // Enable TLS certificate verification
    VerifyTLS bool `json:"verify_tls"`
}

// FSPermission defines filesystem access permissions
type FSPermission struct {
    // Enable filesystem access
    Enabled bool `json:"enabled"`
    
    // Allowed read paths
    ReadPaths []string `json:"read_paths"`
    
    // Allowed write paths
    WritePaths []string `json:"write_paths"`
    
    // Allowed delete paths
    DeletePaths []string `json:"delete_paths"`
    
    // Maximum file read size
    MaxReadSize int64 `json:"max_read_size"`
    
    // Maximum file write size
    MaxWriteSize int64 `json:"max_write_size"`
    
    // Prevent path traversal
    PreventTraversal bool `json:"prevent_traversal"`
}

// ResourceLimits defines resource constraints
type ResourceLimits struct {
    // Maximum memory (bytes)
    MaxMemory int64 `json:"max_memory"`
    
    // Maximum CPU time
    MaxCPUTime time.Duration `json:"max_cpu_time"`
    
    // Maximum execution time
    MaxExecutionTime time.Duration `json:"max_execution_time"`
    
    // Maximum fuel units
    MaxFuel uint64 `json:"max_fuel"`
}

// CapabilityStore manages capability grants
type CapabilityStore struct {
    grants map[string]*CapabilityGrant // key: module_hash
}

// NewCapabilityStore creates a new capability store
func NewCapabilityStore() *CapabilityStore {
    return &CapabilityStore{
        grants: make(map[string]*CapabilityGrant),
    }
}

// Grant creates a new capability grant for a module
func (s *CapabilityStore) Grant(moduleHash string, grant *CapabilityGrant) error {
    if moduleHash == "" {
        return NewError(ErrCodeInvalidInput, "module hash required", nil)
    }
    
    if grant == nil {
        grant = &CapabilityGrant{
            ID:         fmt.Sprintf("grant-%s-%d", moduleHash[:8], time.Now().Unix()),
            ModuleHash: moduleHash,
            HTTP: HTTPPermission{
                Enabled:    false,
                AllowedMethods: []string{"GET", "POST"},
            },
            Filesystem: FSPermission{
                Enabled: false,
            },
            Resources: ResourceLimits{
                MaxMemory:        64 * 1024 * 1024,
                MaxCPUTime:       5 * time.Second,
                MaxExecutionTime: 30 * time.Second,
            },
        }
    }
    
    grant.ModuleHash = moduleHash
    s.grants[moduleHash] = grant
    
    return nil
}

// Get retrieves a capability grant for a module
func (s *CapabilityStore) Get(moduleHash string) (*CapabilityGrant, bool) {
    grant, ok := s.grants[moduleHash]
    if !ok {
        return nil, false
    }
    
    // Check expiration
    if grant.ExpiresAt != nil && time.Now().After(*grant.ExpiresAt) {
        delete(s.grants, moduleHash)
        return nil, false
    }
    
    return grant, true
}

// Revoke removes a capability grant
func (s *CapabilityStore) Revoke(moduleHash string) {
    delete(s.grants, moduleHash)
}

// CheckHTTPPermission verifies HTTP permission
func (g *CapabilityGrant) CheckHTTPPermission(method, url string) error {
    if !g.HTTP.Enabled {
        return NewError(ErrCodeCapabilityDenied, "HTTP access not enabled", nil)
    }
    
    // Check method
    methodAllowed := false
    for _, m := range g.HTTP.AllowedMethods {
        if strings.EqualFold(m, method) {
            methodAllowed = true
            break
        }
    }
    if !methodAllowed {
        return NewError(ErrCodeCapabilityDenied, 
            fmt.Sprintf("HTTP method %s not allowed", method), nil)
    }
    
    // Check host if restricted
    if len(g.HTTP.AllowedHosts) > 0 {
        hostAllowed := false
        for _, host := range g.HTTP.AllowedHosts {
            if strings.Contains(url, host) {
                hostAllowed = true
                break
            }
        }
        if !hostAllowed {
            return NewError(ErrCodeCapabilityDenied, 
                fmt.Sprintf("HTTP host not allowed: %s", url), nil)
        }
    }
    
    return nil
}

// CheckFSPermission verifies filesystem permission
func (g *CapabilityGrant) CheckFSPermission(path string, operation string) error {
    if !g.Filesystem.Enabled {
        return NewError(ErrCodeCapabilityDenied, "filesystem access not enabled", nil)
    }
    
    // Clean path
    cleanPath := filepath.Clean(path)
    
    // Check path traversal
    if g.Filesystem.PreventTraversal {
        for _, component := range strings.Split(cleanPath, string(filepath.Separator)) {
            if component == ".." {
                return NewError(ErrCodeCapabilityDenied, 
                    "path traversal not allowed", nil)
            }
        }
    }
    
    // Check read permission
    if operation == "read" {
        for _, allowedPath := range g.Filesystem.ReadPaths {
            if strings.HasPrefix(cleanPath, allowedPath) {
                return nil
            }
        }
        return NewError(ErrCodeCapabilityDenied, 
            fmt.Sprintf("read not allowed for path: %s", path), nil)
    }
    
    // Check write permission
    if operation == "write" {
        for _, allowedPath := range g.Filesystem.WritePaths {
            if strings.HasPrefix(cleanPath, allowedPath) {
                return nil
            }
        }
        return NewError(ErrCodeCapabilityDenied, 
            fmt.Sprintf("write not allowed for path: %s", path), nil)
    }
    
    return nil
}

// GetResourceLimits returns resource limits for the grant
func (g *CapabilityGrant) GetResourceLimits() ResourceLimits {
    return g.Resources
}
```

## 4. Memory Management

```go
// internal/core/wasm/memory.go

package wasm

import (
    "context"
    "fmt"
    "unsafe"
    
    "github.com/bytecodealliance/wasmtime-go"
)

// MemoryManager handles WASM linear memory operations
type MemoryManager struct {
    store    *wasmtime.Store
    memory   *wasmtime.Memory
    instance *wasmtime.Instance
    
    // Memory limits
    maxSize int64
    currentSize int64
}

// NewMemoryManager creates a new memory manager
func NewMemoryManager(store *wasmtime.Store, instance *wasmtime.Instance, maxSize int64) (*MemoryManager, error) {
    // Get memory export
    mem := instance.GetExport(store, "memory")
    if mem == nil {
        return nil, NewError(ErrCodeMemoryAccess, "memory export not found", nil)
    }
    
    memory := mem.Memory()
    if memory == nil {
        return nil, NewError(ErrCodeMemoryAccess, "memory export is not memory", nil)
    }
    
    mm := &MemoryManager{
        store:       store,
        memory:      memory,
        instance:    instance,
        maxSize:     maxSize,
        currentSize: int64(memory.Size(store)),
    }
    
    return mm, nil
}

// Read reads data from WASM memory
func (m *MemoryManager) Read(offset, length int32) ([]byte, error) {
    if length < 0 {
        return nil, NewError(ErrCodeInvalidInput, "negative length", nil)
    }
    
    if length == 0 {
        return []byte{}, nil
    }
    
    // Check bounds
    memSize := m.memory.Size(m.store)
    if uint64(offset)+uint64(length) > memSize {
        return nil, NewError(ErrCodeMemoryAccess, 
            fmt.Sprintf("memory access out of bounds: offset=%d, length=%d, size=%d", 
                offset, length, memSize), nil)
    }
    
    // Get memory pointer and copy data
    ptr := m.memory.UnsafeData(m.store)
    data := make([]byte, length)
    copy(data, ptr[offset:offset+length])
    
    return data, nil
}

// Write writes data to WASM memory
func (m *MemoryManager) Write(offset int32, data []byte) error {
    if len(data) == 0 {
        return nil
    }
    
    // Check bounds
    memSize := m.memory.Size(m.store)
    if uint64(offset)+uint64(len(data)) > memSize {
        return NewError(ErrCodeMemoryAccess, 
            fmt.Sprintf("memory write out of bounds: offset=%d, length=%d, size=%d", 
                offset, len(data), memSize), nil)
    }
    
    // Write data
    ptr := m.memory.UnsafeData(m.store)
    copy(ptr[offset:], data)
    
    return nil
}

// Allocate allocates memory in the WASM module
func (m *MemoryManager) Allocate(size int32) (int32, error) {
    if size <= 0 {
        return 0, NewError(ErrCodeInvalidInput, "invalid allocation size", nil)
    }
    
    // Try to grow memory if needed
    memSize := m.memory.Size(m.store)
    neededPages := (uint64(size) + 65535) / 65536 // WASM page size is 64KB
    currentPages := memSize / 65536
    
    if uint64(size) > memSize {
        delta := neededPages - currentPages
        if delta > 0 {
            // Check against max size
            newSize := (currentPages + delta) * 65536
            if m.maxSize > 0 && int64(newSize) > m.maxSize {
                return 0, NewError(ErrCodeResourceExhausted, 
                    fmt.Sprintf("memory allocation would exceed limit: %d bytes", m.maxSize), nil)
            }
            
            // Grow memory
            result := m.memory.Grow(m.store, delta)
            if result == -1 {
                return 0, NewError(ErrCodeMemoryAccess, "failed to grow memory", nil)
            }
        }
    }
    
    // For now, return offset 0 (WASM modules typically manage their own heap)
    return 0, NewError(ErrCodeMemoryAccess, 
        "custom allocation not supported - use module's allocator", nil)
}

// GetPointer returns a pointer to WASM memory at offset
func (m *MemoryManager) GetPointer(offset int32) unsafe.Pointer {
    ptr := m.memory.UnsafeData(m.store)
    return unsafe.Pointer(&ptr[offset])
}

// Size returns the current memory size
func (m *MemoryManager) Size() int64 {
    return int64(m.memory.Size(m.store))
}

// ReadString reads a null-terminated string from WASM memory
func (m *MemoryManager) ReadString(offset int32, maxLen int32) (string, error) {
    if maxLen <= 0 {
        maxLen = 4096 // Default max string length
    }
    
    // Read memory and find null terminator
    data, err := m.Read(offset, maxLen)
    if err != nil {
        return "", err
    }
    
    // Find null terminator
    for i, b := range data {
        if b == 0 {
            return string(data[:i]), nil
        }
    }
    
    return "", NewError(ErrCodeMemoryAccess, "string not null-terminated", nil)
}

// WriteString writes a string to WASM memory (with null terminator)
func (m *MemoryManager) WriteString(offset int32, str string, maxLen int32) error {
    data := []byte(str)
    if int32(len(data))+1 > maxLen {
        return NewError(ErrCodeInvalidInput, "string too long", nil)
    }
    
    // Write string data
    if err := m.Write(offset, data); err != nil {
        return err
    }
    
    // Write null terminator
    return m.Write(offset+int32(len(data)), []byte{0})
}
```

## 5. Host Functions

```go
// internal/core/wasm/host.go

package wasm

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"
    
    "github.com/bytecodealliance/wasmtime-go"
    httpcore "github.com/openapi-cli/core/internal/core/http"
)

// HostFunctions provides host function implementations
type HostFunctions struct {
    runtime      *wasmRuntime
    httpClient   httpcore.Client
    capability   *CapabilityGrant
    memoryMgr    *MemoryManager
    store        *wasmtime.Store
    
    // Request context for cancellation
    ctx context.Context
    
    // Logger
    logger func(level, msg string)
}

// NewHostFunctions creates host function handlers
func NewHostFunctions(runtime *wasmRuntime, httpClient httpcore.Client, capability *CapabilityGrant, ctx context.Context) *HostFunctions {
    return &HostFunctions{
        runtime:    runtime,
        httpClient: httpClient,
        capability: capability,
        ctx:        ctx,
        logger:     defaultLogger,
    }
}

// defaultLogger is the default logging function
func defaultLogger(level, msg string) {
    fmt.Printf("[%s] %s\n", strings.ToUpper(level), msg)
}

// SetLogger sets the logging function
func (h *HostFunctions) SetLogger(fn func(level, msg string)) {
    h.logger = fn
}

// SetMemoryManager sets the memory manager
func (h *HostFunctions) SetMemoryManager(mm *MemoryManager) {
    h.memoryMgr = mm
}

// SetStore sets the wasmtime store
func (h *HostFunctions) SetStore(store *wasmtime.Store) {
    h.store = store
}

// Register registers host functions with the linker
func (h *HostFunctions) Register(linker *wasmtime.Linker) error {
    // HTTP functions
    if err := h.registerHTTP(linker); err != nil {
        return err
    }
    
    // File system functions
    if err := h.registerFS(linker); err != nil {
        return err
    }
    
    // Logging functions
    if err := h.registerLogging(linker); err != nil {
        return err
    }
    
    // Time functions
    if err := h.registerTime(linker); err != nil {
        return err
    }
    
    // Environment functions
    if err := h.registerEnv(linker); err != nil {
        return err
    }
    
    return nil
}

// registerHTTP registers HTTP host functions
func (h *HostFunctions) registerHTTP(linker *wasmtime.Linker) error {
    // http_request: (method_ptr, method_len, url_ptr, url_len, body_ptr, body_len, headers_ptr, headers_len) -> response_id
    err := linker.FuncNew(
        "env",
        "http_request",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{wasmtime.ValKindI32},
        ),
        h.httpRequest,
    )
    if err != nil {
        return fmt.Errorf("failed to register http_request: %w", err)
    }
    
    // http_get_response_body: (response_id, buf_ptr, buf_len) -> bytes_written
    err = linker.FuncNew(
        "env",
        "http_get_response_body",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{wasmtime.ValKindI32},
        ),
        h.httpGetResponseBody,
    )
    if err != nil {
        return fmt.Errorf("failed to register http_get_response_body: %w", err)
    }
    
    // http_get_response_status: (response_id) -> status_code
    err = linker.FuncNew(
        "env",
        "http_get_response_status",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32},
            []wasmtime.ValKind{wasmtime.ValKindI32},
        ),
        h.httpGetResponseStatus,
    )
    if err != nil {
        return fmt.Errorf("failed to register http_get_response_status: %w", err)
    }
    
    // http_close_response: (response_id) -> void
    err = linker.FuncNew(
        "env",
        "http_close_response",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32},
            []wasmtime.ValKind{},
        ),
        h.httpCloseResponse,
    )
    if err != nil {
        return fmt.Errorf("failed to register http_close_response: %w", err)
    }
    
    return nil
}

// registerFS registers filesystem host functions
func (h *HostFunctions) registerFS(linker *wasmtime.Linker) error {
    // fs_read_file: (path_ptr, path_len, buf_ptr, buf_len) -> bytes_read_or_error
    err := linker.FuncNew(
        "env",
        "fs_read_file",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{wasmtime.ValKindI32},
        ),
        h.fsReadFile,
    )
    if err != nil {
        return fmt.Errorf("failed to register fs_read_file: %w", err)
    }
    
    // fs_write_file: (path_ptr, path_len, data_ptr, data_len) -> error
    err = linker.FuncNew(
        "env",
        "fs_write_file",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{wasmtime.ValKindI32},
        ),
        h.fsWriteFile,
    )
    if err != nil {
        return fmt.Errorf("failed to register fs_write_file: %w", err)
    }
    
    // fs_file_exists: (path_ptr, path_len) -> exists
    err = linker.FuncNew(
        "env",
        "fs_file_exists",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{wasmtime.ValKindI32},
        ),
        h.fsFileExists,
    )
    if err != nil {
        return fmt.Errorf("failed to register fs_file_exists: %w", err)
    }
    
    // fs_delete_file: (path_ptr, path_len) -> error
    err = linker.FuncNew(
        "env",
        "fs_delete_file",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{wasmtime.ValKindI32},
        ),
        h.fsDeleteFile,
    )
    if err != nil {
        return fmt.Errorf("failed to register fs_delete_file: %w", err)
    }
    
    return nil
}

// registerLogging registers logging host functions
func (h *HostFunctions) registerLogging(linker *wasmtime.Linker) error {
    // log_info: (msg_ptr, msg_len) -> void
    err := linker.FuncNew(
        "env",
        "log_info",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{},
        ),
        h.logInfo,
    )
    if err != nil {
        return fmt.Errorf("failed to register log_info: %w", err)
    }
    
    // log_warn: (msg_ptr, msg_len) -> void
    err = linker.FuncNew(
        "env",
        "log_warn",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{},
        ),
        h.logWarn,
    )
    if err != nil {
        return fmt.Errorf("failed to register log_warn: %w", err)
    }
    
    // log_error: (msg_ptr, msg_len) -> void
    err = linker.FuncNew(
        "env",
        "log_error",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{},
        ),
        h.logError,
    )
    if err != nil {
        return fmt.Errorf("failed to register log_error: %w", err)
    }
    
    return nil
}

// registerTime registers time host functions
func (h *HostFunctions) registerTime(linker *wasmtime.Linker) error {
    // time_now_unix: () -> timestamp (i64)
    err := linker.FuncNew(
        "env",
        "time_now_unix",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{},
            []wasmtime.ValKind{wasmtime.ValKindI64},
        ),
        h.timeNowUnix,
    )
    if err != nil {
        return fmt.Errorf("failed to register time_now_unix: %w", err)
    }
    
    // time_now_nanosecond: () -> nanoseconds (i64)
    err = linker.FuncNew(
        "env",
        "time_now_nanosecond",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{},
            []wasmtime.ValKind{wasmtime.ValKindI64},
        ),
        h.timeNowNanosecond,
    )
    if err != nil {
        return fmt.Errorf("failed to register time_now_nanosecond: %w", err)
    }
    
    return nil
}

// registerEnv registers environment host functions
func (h *HostFunctions) registerEnv(linker *wasmtime.Linker) error {
    // env_get: (key_ptr, key_len, buf_ptr, buf_len) -> bytes_written_or_error
    err := linker.FuncNew(
        "env",
        "env_get",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{wasmtime.ValKindI32},
        ),
        h.envGet,
    )
    if err != nil {
        return fmt.Errorf("failed to register env_get: %w", err)
    }
    
    return nil
}

// HTTP Response storage
var httpResponses = make(map[int32]*httpResponse)
var httpResponseID int32 = 1

type httpResponse struct {
    status  int
    body    []byte
    headers http.Header
}

// httpRequest implements the http_request host function
func (h *HostFunctions) httpRequest(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    if h.memoryMgr == nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap("memory not available")
    }
    
    // Read arguments from WASM memory
    methodPtr := args[0].I32()
    methodLen := args[1].I32()
    urlPtr := args[2].I32()
    urlLen := args[3].I32()
    bodyPtr := args[4].I32()
    bodyLen := args[5].I32()
    headersPtr := args[6].I32()
    headersLen := args[7].I32()
    
    // Read method
    methodBytes, err := h.memoryMgr.Read(methodPtr, methodLen)
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    method := string(methodBytes)
    
    // Read URL
    urlBytes, err := h.memoryMgr.Read(urlPtr, urlLen)
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    url := string(urlBytes)
    
    // Read body
    var body []byte
    if bodyLen > 0 {
        body, err = h.memoryMgr.Read(bodyPtr, bodyLen)
        if err != nil {
            return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
        }
    }
    
    // Check capability
    if h.capability != nil {
        if err := h.capability.CheckHTTPPermission(method, url); err != nil {
            return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
        }
    }
    
    // Create HTTP request
    req, err := http.NewRequest(method, url, bytes.NewReader(body))
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    // Read and parse headers
    if headersLen > 0 {
        headersBytes, err := h.memoryMgr.Read(headersPtr, headersLen)
        if err != nil {
            return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
        }
        
        // Parse headers (simple format: "key: value\nkey2: value2")
        headerLines := strings.Split(string(headersBytes), "\n")
        for _, line := range headerLines {
            parts := strings.SplitN(line, ":", 2)
            if len(parts) == 2 {
                req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
            }
        }
    }
    
    // Execute request with timeout
    ctx := h.ctx
    if h.capability != nil && h.capability.Resources.MaxExecutionTime > 0 {
        var cancel context.CancelFunc
        ctx, cancel = context.WithTimeout(h.ctx, h.capability.Resources.MaxExecutionTime)
        defer cancel()
    }
    
    resp, err := h.httpClient.Do(ctx, &httpcore.Request{
        Method:  req.Method,
        URL:     req.URL.String(),
        Headers: req.Header,
        Body:    body,
    })
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    // Store response
    id := httpResponseID
    httpResponseID++
    httpResponses[id] = &httpResponse{
        status:  resp.StatusCode,
        body:    resp.Body,
        headers: resp.Headers,
    }
    
    return []wasmtime.Val{wasmtime.ValI32(id)}, nil
}

// httpGetResponseBody implements the http_get_response_body host function
func (h *HostFunctions) httpGetResponseBody(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    if h.memoryMgr == nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap("memory not available")
    }
    
    responseID := args[0].I32()
    bufPtr := args[1].I32()
    bufLen := args[2].I32()
    
    resp, ok := httpResponses[responseID]
    if !ok {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap("invalid response ID")
    }
    
    // Determine how much to copy
    toCopy := int32(len(resp.body))
    if toCopy > bufLen {
        toCopy = bufLen
    }
    
    // Write to WASM memory
    if err := h.memoryMgr.Write(bufPtr, resp.body[:toCopy]); err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    return []wasmtime.Val{wasmtime.ValI32(toCopy)}, nil
}

// httpGetResponseStatus implements the http_get_response_status host function
func (h *HostFunctions) httpGetResponseStatus(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    responseID := args[0].I32()
    
    resp, ok := httpResponses[responseID]
    if !ok {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap("invalid response ID")
    }
    
    return []wasmtime.Val{wasmtime.ValI32(int32(resp.status))}, nil
}

// httpCloseResponse implements the http_close_response host function
func (h *HostFunctions) httpCloseResponse(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    responseID := args[0].I32()
    delete(httpResponses, responseID)
    return []wasmtime.Val{}, nil
}

// fsReadFile implements the fs_read_file host function
func (h *HostFunctions) fsReadFile(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    if h.memoryMgr == nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap("memory not available")
    }
    
    pathPtr := args[0].I32()
    pathLen := args[1].I32()
    bufPtr := args[2].I32()
    bufLen := args[3].I32()
    
    // Read path
    pathBytes, err := h.memoryMgr.Read(pathPtr, pathLen)
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    path := string(pathBytes)
    
    // Check capability
    if h.capability != nil {
        if err := h.capability.CheckFSPermission(path, "read"); err != nil {
            return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
        }
    }
    
    // Read file
    data, err := os.ReadFile(path)
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    // Check size limit
    if h.capability != nil && int64(len(data)) > h.capability.Filesystem.MaxReadSize {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, 
            wasmtime.NewTrap("file exceeds read size limit")
    }
    
    // Determine how much to copy
    toCopy := int32(len(data))
    if toCopy > bufLen {
        toCopy = bufLen
    }
    
    // Write to WASM memory
    if err := h.memoryMgr.Write(bufPtr, data[:toCopy]); err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    return []wasmtime.Val{wasmtime.ValI32(toCopy)}, nil
}

// fsWriteFile implements the fs_write_file host function
func (h *HostFunctions) fsWriteFile(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    if h.memoryMgr == nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap("memory not available")
    }
    
    pathPtr := args[0].I32()
    pathLen := args[1].I32()
    dataPtr := args[2].I32()
    dataLen := args[3].I32()
    
    // Read path
    pathBytes, err := h.memoryMgr.Read(pathPtr, pathLen)
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    path := string(pathBytes)
    
    // Read data
    data, err := h.memoryMgr.Read(dataPtr, dataLen)
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    // Check capability
    if h.capability != nil {
        if err := h.capability.CheckFSPermission(path, "write"); err != nil {
            return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
        }
        
        if int64(len(data)) > h.capability.Filesystem.MaxWriteSize {
            return []wasmtime.Val{wasmtime.ValI32(-1)}, 
                wasmtime.NewTrap("write exceeds size limit")
        }
    }
    
    // Ensure directory exists
    dir := filepath.Dir(path)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    // Write file
    if err := os.WriteFile(path, data, 0644); err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    return []wasmtime.Val{wasmtime.ValI32(0)}, nil
}

// fsFileExists implements the fs_file_exists host function
func (h *HostFunctions) fsFileExists(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    pathPtr := args[0].I32()
    pathLen := args[1].I32()
    
    // Read path
    pathBytes, err := h.memoryMgr.Read(pathPtr, pathLen)
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(0)}, nil
    }
    path := string(pathBytes)
    
    // Check capability
    if h.capability != nil {
        if err := h.capability.CheckFSPermission(path, "read"); err != nil {
            return []wasmtime.Val{wasmtime.ValI32(0)}, nil
        }
    }
    
    // Check existence
    _, err = os.Stat(path)
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(0)}, nil
    }
    
    return []wasmtime.Val{wasmtime.ValI32(1)}, nil
}

// fsDeleteFile implements the fs_delete_file host function
func (h *HostFunctions) fsDeleteFile(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    pathPtr := args[0].I32()
    pathLen := args[1].I32()
    
    // Read path
    pathBytes, err := h.memoryMgr.Read(pathPtr, pathLen)
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    path := string(pathBytes)
    
    // Check capability - delete requires write permission to the path
    if h.capability != nil {
        // For delete, we check if the path is in write paths
        inWritePath := false
        for _, wp := range h.capability.Filesystem.WritePaths {
            if strings.HasPrefix(path, wp) {
                inWritePath = true
                break
            }
        }
        if !inWritePath {
            return []wasmtime.Val{wasmtime.ValI32(-1)}, 
                wasmtime.NewTrap("delete not permitted for path")
        }
    }
    
    // Delete file
    if err := os.Remove(path); err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    return []wasmtime.Val{wasmtime.ValI32(0)}, nil
}

// logInfo implements the log_info host function
func (h *HostFunctions) logInfo(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    msgPtr := args[0].I32()
    msgLen := args[1].I32()
    
    msgBytes, err := h.memoryMgr.Read(msgPtr, msgLen)
    if err != nil {
        return nil, wasmtime.NewTrap(err.Error())
    }
    
    h.logger("info", string(msgBytes))
    return nil, nil
}

// logWarn implements the log_warn host function
func (h *HostFunctions) logWarn(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    msgPtr := args[0].I32()
    msgLen := args[1].I32()
    
    msgBytes, err := h.memoryMgr.Read(msgPtr, msgLen)
    if err != nil {
        return nil, wasmtime.NewTrap(err.Error())
    }
    
    h.logger("warn", string(msgBytes))
    return nil, nil
}

// logError implements the log_error host function
func (h *HostFunctions) logError(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    msgPtr := args[0].I32()
    msgLen := args[1].I32()
    
    msgBytes, err := h.memoryMgr.Read(msgPtr, msgLen)
    if err != nil {
        return nil, wasmtime.NewTrap(err.Error())
    }
    
    h.logger("error", string(msgBytes))
    return nil, nil
}

// timeNowUnix implements the time_now_unix host function
func (h *HostFunctions) timeNowUnix(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    now := time.Now().Unix()
    return []wasmtime.Val{wasmtime.ValI64(now)}, nil
}

// timeNowNanosecond implements the time_now_nanosecond host function
func (h *HostFunctions) timeNowNanosecond(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    now := time.Now().UnixNano()
    return []wasmtime.Val{wasmtime.ValI64(now)}, nil
}

// envGet implements the env_get host function
func (h *HostFunctions) envGet(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    if h.memoryMgr == nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap("memory not available")
    }
    
    keyPtr := args[0].I32()
    keyLen := args[1].I32()
    bufPtr := args[2].I32()
    bufLen := args[3].I32()
    
    // Read key
    keyBytes, err := h.memoryMgr.Read(keyPtr, keyLen)
    if err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    key := string(keyBytes)
    
    // Get environment variable
    value := os.Getenv(key)
    if value == "" {
        return []wasmtime.Val{wasmtime.ValI32(0)}, nil // Not found
    }
    
    valueBytes := []byte(value)
    toCopy := int32(len(valueBytes))
    if toCopy > bufLen-1 { // Reserve space for null terminator
        toCopy = bufLen - 1
    }
    
    // Write to WASM memory (including null terminator)
    if err := h.memoryMgr.Write(bufPtr, valueBytes[:toCopy]); err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    // Add null terminator
    if err := h.memoryMgr.Write(bufPtr+toCopy, []byte{0}); err != nil {
        return []wasmtime.Val{wasmtime.ValI32(-1)}, wasmtime.NewTrap(err.Error())
    }
    
    return []wasmtime.Val{wasmtime.ValI32(toCopy + 1)}, nil
}
```

## 6. Module Caching

```go
// internal/core/wasm/cache.go

package wasm

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"
    
    "github.com/bytecodealliance/wasmtime-go"
)

// ModuleCache provides module compilation and instance caching
type ModuleCache struct {
    config      *CacheConfig
    engine      *wasmtime.Engine
    
    // Compiled module cache
    modules     map[string]*cachedModule
    modulesMu   sync.RWMutex
    
    // Instance pool
    instances   map[string][]*cachedInstance
    instancesMu sync.Mutex
    
    // Stats
    hits        int64
    misses      int64
    evictions   int64
}

// cachedModule holds a compiled module
type cachedModule struct {
    hash      string
    module    *wasmtime.Module
    compiled  time.Time
    lastUsed  time.Time
    size      int64
}

// cachedInstance holds a pre-instantiated module
type cachedInstance struct {
    module    *wasmtime.Module
    instance  *wasmtime.Instance
    store     *wasmtime.Store
    lastUsed  time.Time
}

// NewModuleCache creates a new module cache
func NewModuleCache(config *CacheConfig, engine *wasmtime.Engine) *ModuleCache {
    if config == nil {
        config = &CacheConfig{
            Enabled:    true,
            Directory:  "/tmp/wasm-cache",
            MaxModules: 100,
            MaxAge:     24 * time.Hour,
        }
    }
    
    // Ensure cache directory exists
    if config.Enabled && config.Directory != "" {
        os.MkdirAll(config.Directory, 0755)
    }
    
    return &ModuleCache{
        config:    config,
        engine:    engine,
        modules:   make(map[string]*cachedModule),
        instances: make(map[string][]*cachedInstance),
    }
}

// GetOrCompile retrieves or compiles a module
func (c *ModuleCache) GetOrCompile(ctx context.Context, content []byte) (*wasmtime.Module, error) {
    if !c.config.Enabled {
        return wasmtime.NewModule(c.engine, content)
    }
    
    // Calculate hash
    hash := computeHash(content)
    
    // Check in-memory cache
    c.modulesMu.RLock()
    cached, found := c.modules[hash]
    c.modulesMu.RUnlock()
    
    if found {
        // Check expiration
        if time.Since(cached.compiled) > c.config.MaxAge {
            c.modulesMu.Lock()
            delete(c.modules, hash)
            c.modulesMu.Unlock()
            c.evictions++
        } else {
            cached.lastUsed = time.Now()
            c.hits++
            return cached.module, nil
        }
    }
    
    // Check disk cache
    diskPath := filepath.Join(c.config.Directory, hash+".wasm")
    if _, err := os.Stat(diskPath); err == nil {
        // Compile from disk
        module, err := wasmtime.NewModuleFromFile(c.engine, diskPath)
        if err == nil {
            c.cacheModule(hash, module, int64(len(content)))
            c.hits++
            return module, nil
        }
    }
    
    // Compile from content
    c.misses++
    module, err := wasmtime.NewModule(c.engine, content)
    if err != nil {
        return nil, fmt.Errorf("failed to compile module: %w", err)
    }
    
    // Save to disk cache
    if c.config.Directory != "" {
        if err := c.saveToDisk(hash, content); err != nil {
            // Log but don't fail
            fmt.Printf("warning: failed to save module to disk cache: %v\n", err)
        }
    }
    
    // Cache in memory
    c.cacheModule(hash, module, int64(len(content)))
    
    return module, nil
}

// cacheModule adds a module to the in-memory cache
func (c *ModuleCache) cacheModule(hash string, module *wasmtime.Module, size int64) {
    c.modulesMu.Lock()
    defer c.modulesMu.Unlock()
    
    // Evict oldest if at capacity
    if len(c.modules) >= c.config.MaxModules {
        c.evictOldest()
    }
    
    c.modules[hash] = &cachedModule{
        hash:     hash,
        module:   module,
        compiled: time.Now(),
        lastUsed: time.Now(),
        size:     size,
    }
}

// saveToDisk saves module to disk cache
func (c *ModuleCache) saveToDisk(hash string, content []byte) error {
    path := filepath.Join(c.config.Directory, hash+".wasm")
    return os.WriteFile(path, content, 0644)
}

// evictOldest removes oldest cached modules
func (c *ModuleCache) evictOldest() {
    if len(c.modules) == 0 {
        return
    }
    
    var oldestHash string
    var oldestTime time.Time
    
    for hash, mod := range c.modules {
        if oldestTime.IsZero() || mod.lastUsed.Before(oldestTime) {
            oldestHash = hash
            oldestTime = mod.lastUsed
        }
    }
    
    delete(c.modules, oldestHash)
    c.evictions++
}

// GetInstance retrieves or creates a module instance
func (c *ModuleCache) GetInstance(ctx context.Context, module *wasmtime.Module, linker *wasmtime.Linker) (*wasmtime.Instance, *wasmtime.Store, error) {
    c.instancesMu.Lock()
    defer c.instancesMu.Unlock()
    
    // Try to reuse an existing instance
    hash := computeHash(module.Serialize())
    if instances, ok := c.instances[hash]; ok && len(instances) > 0 {
        // Get last instance
        inst := instances[len(instances)-1]
        c.instances[hash] = instances[:len(instances)-1]
        inst.lastUsed = time.Now()
        return inst.instance, inst.store, nil
    }
    
    // Create new instance
    store := wasmtime.NewStore(c.engine)
    instance, err := linker.Instantiate(store, module)
    if err != nil {
        return nil, nil, fmt.Errorf("failed to instantiate module: %w", err)
    }
    
    return instance, store, nil
}

// ReturnInstance returns an instance to the pool
func (c *ModuleCache) ReturnInstance(module *wasmtime.Module, instance *wasmtime.Instance, store *wasmtime.Store) {
    c.instancesMu.Lock()
    defer c.instancesMu.Unlock()
    
    hash := computeHash(module.Serialize())
    c.instances[hash] = append(c.instances[hash], &cachedInstance{
        module:   module,
        instance: instance,
        store:    store,
        lastUsed: time.Now(),
    })
}

// Clear removes all cached modules
func (c *ModuleCache) Clear() {
    c.modulesMu.Lock()
    c.modules = make(map[string]*cachedModule)
    c.modulesMu.Unlock()
    
    c.instancesMu.Lock()
    c.instances = make(map[string][]*cachedInstance)
    c.instancesMu.Unlock()
    
    // Clear disk cache
    if c.config.Directory != "" {
        os.RemoveAll(c.config.Directory)
        os.MkdirAll(c.config.Directory, 0755)
    }
}

// Stats returns cache statistics
func (c *ModuleCache) Stats() CacheStats {
    c.modulesMu.RLock()
    moduleCount := len(c.modules)
    c.modulesMu.RUnlock()
    
    return CacheStats{
        ModuleCount: moduleCount,
        Hits:        c.hits,
        Misses:      c.misses,
        Evictions:   c.evictions,
    }
}

// CacheStats holds cache statistics
type CacheStats struct {
    ModuleCount int
    Hits        int64
    Misses      int64
    Evictions   int64
}

// computeHash computes SHA256 hash of content
func computeHash(content []byte) string {
    hash := sha256.Sum256(content)
    return hex.EncodeToString(hash[:])
}
```

## 7. Module Implementation

```go
// internal/core/wasm/module.go

package wasm

import (
    "context"
    "fmt"
    "sync"
    "time"
    
    "github.com/bytecodealliance/wasmtime-go"
)

// wasmModule implements the Module interface
type wasmModule struct {
    hash       string
    name       string
    exports    []string
    memory     *wasmtime.Memory
    
    // WASM components
    module     *wasmtime.Module
    instance   *wasmtime.Instance
    store      *wasmtime.Store
    
    // Resource tracking
    memoryUsage int64
    createdAt   time.Time
    lastUsed    time.Time
    useCount    int64
    
    // Lifecycle
    closed      bool
    closeMu     sync.RWMutex
    
    // Reference to runtime
    runtime     *wasmRuntime
}

// newWasmModule creates a new WASM module instance
func newWasmModule(runtime *wasmRuntime, name, hash string, module *wasmtime.Module, instance *wasmtime.Instance, store *wasmtime.Store) (*wasmModule, error) {
    wm := &wasmModule{
        runtime:   runtime,
        name:      name,
        hash:      hash,
        module:    module,
        instance:  instance,
        store:     store,
        createdAt: time.Now(),
        lastUsed:  time.Now(),
    }
    
    // Get exports
    exports := instance.Exports(store)
    for _, exp := range exports {
        wm.exports = append(wm.exports, exp.Name())
    }
    
    // Get memory
    mem := instance.GetExport(store, "memory")
    if mem != nil {
        wm.memory = mem.Memory()
    }
    
    // Calculate initial memory usage
    if wm.memory != nil {
        wm.memoryUsage = int64(wm.memory.Size(store))
    }
    
    return wm, nil
}

// Hash returns the content hash
func (m *wasmModule) Hash() string {
    return m.hash
}

// Name returns the module name
func (m *wasmModule) Name() string {
    return m.name
}

// Exports returns available functions
func (m *wasmModule) Exports() []string {
    m.closeMu.RLock()
    defer m.closeMu.RUnlock()
    
    if m.closed {
        return nil
    }
    
    // Return copy to prevent modification
    result := make([]string, len(m.exports))
    copy(result, m.exports)
    return result
}

// MemoryUsage returns current memory usage
func (m *wasmModule) MemoryUsage() int64 {
    m.closeMu.RLock()
    defer m.closeMu.RUnlock()
    
    if m.closed || m.memory == nil {
        return 0
    }
    
    return int64(m.memory.Size(m.store))
}

// HasExport checks if module exports a function
func (m *wasmModule) HasExport(name string) bool {
    for _, exp := range m.exports {
        if exp == name {
            return true
        }
    }
    return false
}

// GetExport returns an export by name
func (m *wasmModule) GetExport(name string) *wasmtime.Extern {
    m.closeMu.RLock()
    defer m.closeMu.RUnlock()
    
    if m.closed {
        return nil
    }
    
    return m.instance.GetExport(m.store, name)
}

// CallFunction calls a function in the module
func (m *wasmModule) CallFunction(ctx context.Context, name string, args ...interface{}) (interface{}, error) {
    m.closeMu.RLock()
    if m.closed {
        m.closeMu.RUnlock()
        return nil, NewError(ErrCodeExecutionFailed, "module is closed", nil)
    }
    m.closeMu.RUnlock()
    
    // Update usage tracking
    m.lastUsed = time.Now()
    m.useCount++
    
    // Get function export
    fn := m.instance.GetExport(m.store, name)
    if fn == nil {
        return nil, NewError(ErrCodeInvalidInput, 
            fmt.Sprintf("function not found: %s", name), nil)
    }
    
    funcVal := fn.Func()
    if funcVal == nil {
        return nil, NewError(ErrCodeInvalidInput, 
            fmt.Sprintf("export is not a function: %s", name), nil)
    }
    
    // Convert args to wasmtime values
    wasmArgs, err := m.convertArgs(args)
    if err != nil {
        return nil, err
    }
    
    // Call function
    results, err := funcVal.Call(m.store, wasmArgs...)
    if err != nil {
        return nil, NewError(ErrCodeExecutionFailed, 
            fmt.Sprintf("function execution failed: %s", name), err)
    }
    
    // Convert results
    if len(results) == 0 {
        return nil, nil
    }
    if len(results) == 1 {
        return m.convertResult(results[0]), nil
    }
    
    // Multiple results
    var converted []interface{}
    for _, r := range results {
        converted = append(converted, m.convertResult(r))
    }
    return converted, nil
}

// convertArgs converts Go values to wasmtime values
func (m *wasmModule) convertArgs(args []interface{}) ([]interface{}, error) {
    result := make([]interface{}, len(args))
    for i, arg := range args {
        switch v := arg.(type) {
        case int:
            result[i] = int32(v)
        case int32:
            result[i] = v
        case int64:
            result[i] = v
        case float32:
            result[i] = v
        case float64:
            result[i] = v
        case string:
            // Allocate in WASM memory
            ptr, err := m.allocString(v)
            if err != nil {
                return nil, err
            }
            result[i] = ptr
        case []byte:
            // Allocate in WASM memory
            ptr, err := m.allocBytes(v)
            if err != nil {
                return nil, err
            }
            result[i] = ptr
        default:
            return nil, NewError(ErrCodeInvalidInput, 
                fmt.Sprintf("unsupported argument type: %T", v), nil)
        }
    }
    return result, nil
}

// convertResult converts a wasmtime value to Go value
func (m *wasmModule) convertResult(val wasmtime.Val) interface{} {
    switch val.Kind() {
    case wasmtime.ValKindI32:
        return val.I32()
    case wasmtime.ValKindI64:
        return val.I64()
    case wasmtime.ValKindF32:
        return val.F32()
    case wasmtime.ValKindF64:
        return val.F64()
    default:
        return nil
    }
}

// allocString allocates a string in WASM memory
func (m *wasmModule) allocString(s string) (int32, error) {
    data := []byte(s + "\x00") // Null-terminated
    return m.allocBytes(data)
}

// allocBytes allocates bytes in WASM memory
func (m *wasmModule) allocBytes(data []byte) (int32, error) {
    // Look for malloc function
    allocFn := m.instance.GetExport(m.store, "malloc")
    if allocFn == nil {
        return 0, NewError(ErrCodeMemoryAccess, "malloc function not found", nil)
    }
    
    funcVal := allocFn.Func()
    if funcVal == nil {
        return 0, NewError(ErrCodeMemoryAccess, "malloc is not a function", nil)
    }
    
    // Call malloc
    results, err := funcVal.Call(m.store, int32(len(data)))
    if err != nil {
        return 0, NewError(ErrCodeMemoryAccess, "malloc failed", err)
    }
    
    ptr := results.(int32)
    if ptr == 0 {
        return 0, NewError(ErrCodeResourceExhausted, "out of memory", nil)
    }
    
    // Write data
    if m.memory != nil {
        memData := m.memory.UnsafeData(m.store)
        copy(memData[ptr:], data)
    }
    
    return ptr, nil
}

// Close unloads the module
func (m *wasmModule) Close() error {
    m.closeMu.Lock()
    defer m.closeMu.Unlock()
    
    if m.closed {
        return nil
    }
    
    m.closed = true
    
    // Release resources
    if m.store != nil {
        m.store = nil
    }
    
    if m.instance != nil {
        m.instance = nil
    }
    
    if m.module != nil {
        m.module = nil
    }
    
    m.memory = nil
    m.memoryUsage = 0
    
    return nil
}
```

## 8. Main Runtime Implementation

```go
// internal/core/wasm/runtime.go

package wasm

import (
    "context"
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "sync"
    "time"
    
    "github.com/bytecodealliance/wasmtime-go"
    httpcore "github.com/openapi-cli/core/internal/core/http"
)

// Runtime executes WASM modules
type Runtime interface {
    // LoadModule loads a WASM module into the runtime
    LoadModule(ctx context.Context, spec ModuleSpec) (Module, error)
    
    // GetModule returns a cached module
    GetModule(hash string) (Module, bool)
    
    // UnloadModule removes a module from cache
    UnloadModule(hash string) error
    
    // Execute runs a function in a loaded module
    Execute(ctx context.Context, moduleHash string, function string, input []byte) (*ExecutionResult, error)
    
    // SetCapability sets a capability grant
    SetCapability(cap *CapabilityGrant)
    
    // Close releases all resources
    Close() error
}

// Module represents a loaded WASM module
type Module interface {
    // Hash returns the content hash
    Hash() string
    
    // Name returns the module name
    Name() string
    
    // Exports returns available functions
    Exports() []string
    
    // MemoryUsage returns current memory usage
    MemoryUsage() int64
    
    // Close unloads the module
    Close() error
}

// ModuleSpec describes a module to load
type ModuleSpec struct {
    Name         string
    Content      []byte
    SHA256       string
    Signature    string
    Capabilities []string
}

// ExecutionResult is the result of WASM execution
type ExecutionResult struct {
    Output     []byte        `json:"output"`
    Duration   time.Duration `json:"duration"`
    MemoryUsed int64         `json:"memory_used"`
    FuelUsed   uint64        `json:"fuel_used,omitempty"`
}

// wasmRuntime implements the Runtime interface
type wasmRuntime struct {
    config      *RuntimeConfig
    engine      *wasmtime.Engine
    linker      *wasmtime.Linker
    store       *wasmtime.Store
    cache       *ModuleCache
    capabilities *CapabilityStore
    httpClient  httpcore.Client
    
    // Loaded modules
    modules     map[string]*wasmModule
    modulesMu   sync.RWMutex
    
    // Lifecycle
    closed      bool
    closeMu     sync.RWMutex
}

// NewRuntime creates a new WASM runtime
func NewRuntime(config *RuntimeConfig, httpClient httpcore.Client) (*wasmRuntime, error) {
    if config == nil {
        config = DefaultConfig()
    }
    
    // Create Wasmtime engine
    engineConfig := wasmtime.NewConfig()
    engineConfig.SetDebugInfo(config.Engine.DebugInfo)
    engineConfig.SetWasmBacktraceDetails(config.Engine.WasmBacktrace)
    engineConfig.SetWasmThreads(true)
    engineConfig.SetWasmMemory64(false)
    engineConfig.SetConsumeFuel(config.Execution.EnableFuel)
    
    engine, err := wasmtime.NewEngineWithConfig(engineConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create wasmtime engine: %w", err)
    }
    
    // Create linker
    linker, err := wasmtime.NewLinker(engine)
    if err != nil {
        return nil, fmt.Errorf("failed to create linker: %w", err)
    }
    
    // Create cache
    cache := NewModuleCache(&config.Cache, engine)
    
    // Create capability store
    capabilities := NewCapabilityStore()
    
    rt := &wasmRuntime{
        config:       config,
        engine:       engine,
        linker:       linker,
        cache:        cache,
        capabilities: capabilities,
        httpClient:   httpClient,
        modules:      make(map[string]*wasmModule),
    }
    
    // Register WASI host functions (minimal set)
    if err := rt.registerWASI(); err != nil {
        return nil, fmt.Errorf("failed to register WASI: %w", err)
    }
    
    return rt, nil
}

// registerWASI registers minimal WASI host functions
func (r *wasmRuntime) registerWASI() error {
    // Register fd_write for stdout/stderr
    err := r.linker.FuncNew(
        "wasi_snapshot_preview1",
        "fd_write",
        wasmtime.NewFuncType(
            []wasmtime.ValKind{wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32, wasmtime.ValKindI32},
            []wasmtime.ValKind{wasmtime.ValKindI32},
        ),
        r.wasiFdWrite,
    )
    if err != nil {
        return fmt.Errorf("failed to register fd_write: %w", err)
    }
    
    return nil
}

// wasiFdWrite implements the WASI fd_write function
func (r *wasmRuntime) wasiFdWrite(caller *wasmtime.Caller, args []wasmtime.Val) ([]wasmtime.Val, *wasmtime.Trap) {
    // Minimal implementation - return success
    return []wasmtime.Val{wasmtime.ValI32(0)}, nil
}

// LoadModule loads a WASM module
func (r *wasmRuntime) LoadModule(ctx context.Context, spec ModuleSpec) (Module, error) {
    r.closeMu.RLock()
    if r.closed {
        r.closeMu.RUnlock()
        return nil, NewError(ErrCodeModuleLoadFailed, "runtime is closed", nil)
    }
    r.closeMu.RUnlock()
    
    // Calculate hash if not provided
    hash := spec.SHA256
    if hash == "" {
        h := sha256.Sum256(spec.Content)
        hash = hex.EncodeToString(h[:])
    }
    
    // Check if already loaded
    r.modulesMu.RLock()
    if mod, ok := r.modules[hash]; ok {
        r.modulesMu.RUnlock()
        return mod, nil
    }
    r.modulesMu.RUnlock()
    
    // Compile module
    compiledModule, err := r.cache.GetOrCompile(ctx, spec.Content)
    if err != nil {
        return nil, NewError(ErrCodeModuleLoadFailed, "failed to compile module", err)
    }
    
    // Create store with limits
    store := wasmtime.NewStore(r.engine)
    store.SetEpochDeadline(1) // Set epoch for timeout
    
    // Set fuel if enabled
    if r.config.Execution.EnableFuel {
        store.SetFuel(r.config.Execution.MaxFuel)
    }
    
    // Get capability grant
    grant, _ := r.capabilities.Get(hash)
    
    // Create host functions
    hostFns := NewHostFunctions(r, r.httpClient, grant, ctx)
    hostFns.SetStore(store)
    
    // Register host functions with linker
    if err := hostFns.Register(r.linker); err != nil {
        return nil, NewError(ErrCodeModuleLoadFailed, "failed to register host functions", err)
    }
    
    // Instantiate module
    instance, err := r.linker.Instantiate(store, compiledModule)
    if err != nil {
        return nil, NewError(ErrCodeModuleLoadFailed, "failed to instantiate module", err)
    }
    
    // Create memory manager
    memoryMgr, err := NewMemoryManager(store, instance, r.config.Memory.MaxMemory)
    if err != nil {
        return nil, NewError(ErrCodeMemoryAccess, "failed to create memory manager", err)
    }
    hostFns.SetMemoryManager(memoryMgr)
    
    // Create module wrapper
    mod, err := newWasmModule(r, spec.Name, hash, compiledModule, instance, store)
    if err != nil {
        return nil, err
    }
    
    // Cache module
    r.modulesMu.Lock()
    r.modules[hash] = mod
    r.modulesMu.Unlock()
    
    return mod, nil
}

// GetModule returns a cached module
func (r *wasmRuntime) GetModule(hash string) (Module, bool) {
    r.modulesMu.RLock()
    defer r.modulesMu.RUnlock()
    
    mod, ok := r.modules[hash]
    return mod, ok
}

// UnloadModule removes a module from cache
func (r *wasmRuntime) UnloadModule(hash string) error {
    r.modulesMu.Lock()
    defer r.modulesMu.Unlock()
    
    mod, ok := r.modules[hash]
    if !ok {
        return NewError(ErrCodeInvalidInput, "module not found", nil)
    }
    
    if err := mod.Close(); err != nil {
        return err
    }
    
    delete(r.modules, hash)
    return nil
}

// Execute runs a function in a loaded module
func (r *wasmRuntime) Execute(ctx context.Context, moduleHash string, function string, input []byte) (*ExecutionResult, error) {
    r.closeMu.RLock()
    if r.closed {
        r.closeMu.RUnlock()
        return nil, NewError(ErrCodeExecutionFailed, "runtime is closed", nil)
    }
    r.closeMu.RUnlock()
    
    // Get module
    r.modulesMu.RLock()
    mod, ok := r.modules[moduleHash]
    r.modulesMu.RUnlock()
    
    if !ok {
        return nil, NewError(ErrCodeInvalidInput, "module not loaded", nil)
    }
    
    // Get capability and check permissions
    grant, _ := r.capabilities.Get(moduleHash)
    
    // Create execution context with timeout
    execCtx, cancel := r.createExecutionContext(ctx, grant)
    defer cancel()
    
    // Execute function
    startTime := time.Now()
    
    // Convert input to args based on function signature
    // For now, support basic input passing
    var args []interface{}
    if len(input) > 0 {
        // Allocate input in module memory
        ptr, err := mod.allocBytes(input)
        if err != nil {
            return nil, NewError(ErrCodeExecutionFailed, "failed to allocate input", err)
        }
        args = append(args, ptr, int32(len(input)))
    }
    
    result, err := mod.CallFunction(execCtx, function, args...)
    duration := time.Since(startTime)
    
    if err != nil {
        return nil, NewError(ErrCodeExecutionFailed, "execution failed", err)
    }
    
    // Extract output
    var output []byte
    if result != nil {
        // Handle different return types
        switch v := result.(type) {
        case int32:
            // Pointer to output - read from memory
            output, _ = mod.memory.Read(v, 1024) // Read up to 1KB
        case []interface{}:
            // Multiple returns - first is pointer, second is length
            if len(v) >= 2 {
                if ptr, ok := v[0].(int32); ok {
                    if length, ok := v[1].(int32); ok {
                        output, _ = mod.memory.Read(ptr, length)
                    }
                }
            }
        }
    }
    
    // Calculate memory used
    memoryUsed := mod.MemoryUsage()
    
    return &ExecutionResult{
        Output:     output,
        Duration:   duration,
        MemoryUsed: memoryUsed,
    }, nil
}

// createExecutionContext creates context with timeout
func (r *wasmRuntime) createExecutionContext(parent context.Context, grant *CapabilityGrant) (context.Context, context.CancelFunc) {
    var timeout time.Duration
    
    if grant != nil && grant.Resources.MaxExecutionTime > 0 {
        timeout = grant.Resources.MaxExecutionTime
    } else {
        timeout = r.config.Execution.MaxExecutionTime
    }
    
    return context.WithTimeout(parent, timeout)
}

// SetCapability sets a capability grant
func (r *wasmRuntime) SetCapability(cap *CapabilityGrant) {
    r.capabilities.Grant(cap.ModuleHash, cap)
}

// GetCapabilityStore returns the capability store
func (r *wasmRuntime) GetCapabilityStore() *CapabilityStore {
    return r.capabilities
}

// GetCache returns the module cache
func (r *wasmRuntime) GetCache() *ModuleCache {
    return r.cache
}

// Close releases all resources
func (r *wasmRuntime) Close() error {
    r.closeMu.Lock()
    defer r.closeMu.Unlock()
    
    if r.closed {
        return nil
    }
    
    r.closed = true
    
    // Close all modules
    r.modulesMu.Lock()
    for _, mod := range r.modules {
        mod.Close()
    }
    r.modules = make(map[string]*wasmModule)
    r.modulesMu.Unlock()
    
    // Clear cache
    if r.cache != nil {
        r.cache.Clear()
    }
    
    // Release engine
    if r.engine != nil {
        r.engine.Delete()
    }
    
    return nil
}

// IsClosed returns whether the runtime is closed
func (r *wasmRuntime) IsClosed() bool {
    r.closeMu.RLock()
    defer r.closeMu.RUnlock()
    return r.closed
}
```

## 9. Usage Examples

### Basic Module Loading and Execution

```go
package main

import (
    "context"
    "fmt"
    "os"
    
    "github.com/openapi-cli/core/internal/core/wasm"
)

func main() {
    // Create runtime with default config
    runtime, err := wasm.NewRuntime(nil, nil)
    if err != nil {
        fmt.Printf("Failed to create runtime: %v\n", err)
        os.Exit(1)
    }
    defer runtime.Close()
    
    // Load WASM module
    content, err := os.ReadFile("example.wasm")
    if err != nil {
        fmt.Printf("Failed to read module: %v\n", err)
        os.Exit(1)
    }
    
    module, err := runtime.LoadModule(context.Background(), wasm.ModuleSpec{
        Name:    "example",
        Content: content,
    })
    if err != nil {
        fmt.Printf("Failed to load module: %v\n", err)
        os.Exit(1)
    }
    
    // Execute function
    result, err := runtime.Execute(context.Background(), module.Hash(), "hello", []byte("world"))
    if err != nil {
        fmt.Printf("Execution failed: %v\n", err)
        os.Exit(1)
    }
    
    fmt.Printf("Output: %s\n", string(result.Output))
    fmt.Printf("Duration: %v\n", result.Duration)
    fmt.Printf("Memory used: %d bytes\n", result.MemoryUsed)
}
```

### Using Capabilities

```go
package main

import (
    "context"
    "fmt"
    "os"
    "time"
    
    "github.com/openapi-cli/core/internal/core/wasm"
)

func main() {
    // Create runtime
    config := wasm.DefaultConfig()
    config.Execution.MaxExecutionTime = 10 * time.Second
    config.Memory.MaxMemory = 128 * 1024 * 1024 // 128MB
    
    runtime, err := wasm.NewRuntime(config, nil)
    if err != nil {
        fmt.Printf("Failed to create runtime: %v\n", err)
        os.Exit(1)
    }
    defer runtime.Close()
    
    // Load module
    content, _ := os.ReadFile("example.wasm")
    module, err := runtime.LoadModule(context.Background(), wasm.ModuleSpec{
        Name:    "example",
        Content: content,
    })
    if err != nil {
        fmt.Printf("Failed to load module: %v\n", err)
        os.Exit(1)
    }
    
    // Set up capabilities
    grant := &wasm.CapabilityGrant{
        ModuleHash: module.Hash(),
        HTTP: wasm.HTTPPermission{
            Enabled:        true,
            AllowedMethods: []string{"GET", "POST"},
            AllowedHosts:   []string{"api.example.com"},
            MaxRequestBodySize: 1024 * 1024,  // 1MB
            MaxResponseBodySize: 10 * 1024 * 1024, // 10MB
            Timeout:        30 * time.Second,
        },
        Filesystem: wasm.FSPermission{
            Enabled:          true,
            ReadPaths:        []string{"/tmp/input"},
            WritePaths:       []string{"/tmp/output"},
            PreventTraversal: true,
            MaxReadSize:      100 * 1024 * 1024, // 100MB
            MaxWriteSize:     50 * 1024 * 1024,  // 50MB
        },
        Resources: wasm.ResourceLimits{
            MaxMemory:        64 * 1024 * 1024,   // 64MB
            MaxExecutionTime: 10 * time.Second,
            MaxFuel:          1000000000,
        },
    }
    
    runtime.SetCapability(grant)
    
    // Execute with capabilities
    result, err := runtime.Execute(context.Background(), module.Hash(), "process", []byte("data"))
    if err != nil {
        fmt.Printf("Execution failed: %v\n", err)
        os.Exit(1)
    }
    
    fmt.Printf("Success: %s\n", string(result.Output))
}
```

### HTTP Client Integration

```go
package main

import (
    "context"
    "fmt"
    
    "github.com/openapi-cli/core/internal/core/http"
    "github.com/openapi-cli/core/internal/core/wasm"
)

func main() {
    // Create HTTP client
    httpClient := http.NewClient(http.ClientOptions{
        Timeout: 30 * time.Second,
    })
    
    // Create runtime with HTTP client
    runtime, err := wasm.NewRuntime(nil, httpClient)
    if err != nil {
        fmt.Printf("Failed to create runtime: %v\n", err)
        return
    }
    defer runtime.Close()
    
    // WASM module can now make HTTP requests through host functions
    // via http_request, http_get_response_body, etc.
}
```

### Module Caching

```go
package main

import (
    "fmt"
    
    "github.com/openapi-cli/core/internal/core/wasm"
)

func main() {
    // Configure cache
    config := wasm.DefaultConfig()
    config.Cache.Enabled = true
    config.Cache.Directory = "/var/cache/wasm"
    config.Cache.MaxModules = 500
    config.Cache.MaxAge = 24 * time.Hour
    
    runtime, _ := wasm.NewRuntime(config, nil)
    defer runtime.Close()
    
    // First load - compilation
    module1, _ := runtime.LoadModule(ctx, spec)
    
    // Second load - from cache
    module2, _ := runtime.GetModule(module1.Hash())
    
    // Get cache stats
    stats := runtime.GetCache().Stats()
    fmt.Printf("Cache: %d modules, %d hits, %d misses\n", 
        stats.ModuleCount, stats.Hits, stats.Misses)
}
```

## 10. Security Considerations

### Capability Enforcement

The runtime implements defense in depth through multiple layers:

1. **Module Isolation**: Each module runs in its own store with isolated memory
2. **Capability Grants**: Fine-grained permissions per module
3. **Resource Limits**: Memory, CPU, and execution time limits
4. **Host Function Sandboxing**: All system access goes through capability-checked host functions

### Memory Safety

- Linear memory access is bounds-checked
- Memory growth is limited by configuration
- WASI host functions are minimized
- No direct pointer access from guest

### Execution Limits

```go
// Fuel-based execution limiting
store.SetFuel(maxFuel)
store.GetFuel() // Monitor consumption

// Time-based limiting
context.WithTimeout(ctx, maxExecutionTime)

// Memory-based limiting
config.Memory.MaxMemory = 256 * 1024 * 1024 // 256MB
```

## 11. Error Handling

```go
// Check for specific error types
var wasmErr *wasm.WasmError
if errors.As(err, &wasmErr) {
    switch wasmErr.Code {
    case wasm.ErrCodeCapabilityDenied:
        // Module tried to access resource without permission
        log.Printf("Capability denied: %s", wasmErr.Message)
    case wasm.ErrCodeResourceExhausted:
        // Module exceeded resource limits
        log.Printf("Resource limit exceeded: %s", wasmErr.Message)
    case wasm.ErrCodeExecutionFailed:
        // Execution error (trap, panic, etc.)
        log.Printf("Execution failed: %s", wasmErr.Message)
    }
}
```

## 12. Performance Optimization

### Compilation Caching

Enable module compilation caching to avoid recompiling:

```go
config.Cache.Enabled = true
config.Cache.Directory = "/var/cache/wasm"
```

### Instance Pooling

Reuse module instances for repeated executions:

```go
// Instance is returned to pool after Close()
module, _ := runtime.GetModule(hash)
defer module.Close()
```

### Parallel Compilation

```go
config.Engine.ParallelCompilation = true
```

## 13. Testing

```go
package wasm_test

import (
    "context"
    "testing"
    
    "github.com/openapi-cli/core/internal/core/wasm"
    "github.com/stretchr/testify/assert"
)

func TestRuntime(t *testing.T) {
    runtime, err := wasm.NewRuntime(nil, nil)
    assert.NoError(t, err)
    defer runtime.Close()
    
    // Load test module
    module, err := runtime.LoadModule(context.Background(), wasm.ModuleSpec{
        Name:    "test",
        Content: testWasmBytes,
    })
    assert.NoError(t, err)
    
    // Execute function
    result, err := runtime.Execute(context.Background(), module.Hash(), "add", []byte{1, 2})
    assert.NoError(t, err)
    assert.Equal(t, []byte{3}, result.Output)
}
```

## Summary

This implementation provides:

1. **Complete Wasmtime Integration**: Full engine configuration with optimizations
2. **Rich Host Functions**: HTTP, filesystem, logging, time, environment
3. **Secure Memory Management**: Linear memory access with bounds checking
4. **Capability Enforcement**: Fine-grained permission system
5. **Module Caching**: Disk and memory caching for compiled modules
6. **Production Ready**: Error handling, resource limits, security considerations

## Deterministic Guarantees

This WASM runtime implementation must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md), [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md), and [`WASM-SANDBOX-SECURITY.md`](../../WASM-SANDBOX-SECURITY.md) for complete requirements.

### 1. Deterministic Execution Guarantees

WASM modules must produce identical output given identical input across all platforms:

- **Floating‑point semantics**: IEEE 754‑2019 strict mode, identical results across platforms
- **Memory allocation**: Deterministic heap base address (64‑KiB alignment)
- **Random number generation**: Seeded RNG for test mode, cryptographically secure for production

### 2. Platform‑Independent Host Functions

Host functions must behave identically across platforms:

- **File system paths**: Normalize per `PLATFORM-BEHAVIOR.md` section 1.1
- **Environment variables**: Case‑insensitive lookup on Windows (see `PLATFORM-BEHAVIOR.md` section 3.1)
- **Time zone handling**: All timestamps in UTC, consistent formatting

### 3. Deterministic Module Caching

Module caching must produce identical cache state:

- **Cache key generation**: SHA‑256 hash of module bytes (deterministic across platforms)
- **Cache directory structure**: Normalized paths per `PLATFORM-BEHAVIOR.md`
- **Cache invalidation**: Deterministic based on module hash and capability set

### 4. Concurrent Execution Determinism

Concurrent WASM execution must be deterministic:

- **Thread scheduling**: FIFO queue for module execution requests
- **Memory access**: Atomic operations with deterministic ordering
- **Host function calls**: Deterministic interleaving (round‑robin scheduling)

### 5. Cross‑Language WASM Runtime Compatibility

WASM modules must run identically across 12 language SDKs:

- **Same runtime engine**: Wasmtime with identical configuration
- **Same host function implementations**: Identical behavior across languages
- **Same memory limits**: Identical heap and stack limits

### 6. Testing Determinism

All WASM runtime tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestWASMRuntimeDeterminism(t *testing.T) {
	// Verify identical module execution results
	// Verify identical host function behavior
	// Verify identical cache state
}
```

### 7. Integration with SDK Generation

When generating SDKs for 12 languages, WASM runtimes must:

- Use identical WASM engine configuration
- Provide identical host function implementations
- Enforce identical capability restrictions

**Verification**: Run `./scripts/validate‑determinism‑wasm.sh` to confirm cross‑platform consistency.

The runtime is designed for secure execution of untrusted WebAssembly modules with full control over system access through capabilities.
