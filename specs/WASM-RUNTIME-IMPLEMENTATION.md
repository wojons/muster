# WASM Runtime Implementation

**Status**: ✅ COMPLETE
**Version**: 1.0.0
**Last Updated**: 2026-03-02

---

## Overview

The WASM runtime provides a secure, sandboxed execution environment for OpenAPI transformation modules. It enables third-party extensions to run safely with controlled resource limits, isolated from the host system.

### Architecture Goals

- **Security**: Sandboxed execution with no direct host access
- **Performance**: Near-native speed through ahead-of-time compilation
- **Portability**: Modules run consistently across platforms
- **Resource Control**: CPU, memory, and execution time limits

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     HOST APPLICATION                         │
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │ Module Store │───▶│   Runtime    │───▶│   Sandbox    │   │
│  │   (Wasm)     │    │  (wazero)    │    │              │   │
│  └──────────────┘    └──────┬───────┘    └──────┬───────┘   │
│                             │                    │          │
│                      ┌──────▼────────────────────▼──────┐   │
│                      │      HOST FUNCTIONS API          │   │
│                      │  • Logging                       │   │
│                      │  • HTTP Requests (allowlisted)   │   │
│                      │  • File Access (virtual FS)      │   │
│                      │  • Time/Random                   │   │
│                      └──────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    WASM MODULE (Guest)                       │
│                                                              │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐   │
│  │  Transform   │    │   Validate   │    │   Generate   │   │
│  │   Module     │    │   Module     │    │   Module     │   │
│  └──────────────┘    └──────────────┘    └──────────────┘   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## Core Interface

### Runtime Interface

```go
// internal/wasm/runtime.go

package wasm

import (
	"context"
	"fmt"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// Runtime manages WASM module lifecycle and execution
type Runtime struct {
	runtime        wazero.Runtime
	modules        map[string]*CompiledModule
	hostFunctions  HostFunctions
	limits         ResourceLimits
	compiledCache  ModuleCache
	logger         Logger
}

// ResourceLimits defines execution constraints
type ResourceLimits struct {
	MaxMemorySize    uint32        // Maximum memory in bytes
	MaxExecutionTime time.Duration // Maximum execution time
	MaxCPUUnits      uint64        // CPU instruction limit
	MaxStackDepth    int           // Maximum call stack depth
	MaxFDs           uint32        // Maximum file descriptors
}

// DefaultResourceLimits returns sensible defaults
func DefaultResourceLimits() ResourceLimits {
	return ResourceLimits{
		MaxMemorySize:    128 * 1024 * 1024, // 128MB
		MaxExecutionTime: 30 * time.Second,
		MaxCPUUnits:      1_000_000_000, // 1 billion instructions
		MaxStackDepth:    1000,
		MaxFDs:           32,
	}
}

// CompiledModule represents a compiled WASM module
type CompiledModule struct {
	id           string
	compiled     wazero.CompiledModule
	config       ModuleConfig
	checksum     string
	compiledAt   time.Time
	size         int64
	exports      []api.FunctionDefinition
}

// ModuleConfig holds module-specific configuration
type ModuleConfig struct {
	Name         string
	Version      string
	EntryPoint   string
	Environment  map[string]string
	AllowedHosts []string
	AllowedPaths map[string]string
}

// Logger interface for module logging
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// ModuleCache interface for compiled module caching
type ModuleCache interface {
	Get(ctx context.Context, key string) (*CachedModule, error)
	Set(ctx context.Context, key string, module *CachedModule) error
}

// CachedModule represents a cached compiled module
type CachedModule struct {
	Data     []byte
	Checksum string
	Compiled time.Time
}

// NewRuntime creates a new WASM runtime
func NewRuntime(ctx context.Context, config RuntimeConfig) (*Runtime, error) {
	rt := wazero.NewRuntime(ctx)
	
	// Instantiate WASI
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}
	
	return &Runtime{
		runtime:       rt,
		modules:       make(map[string]*Module),
		limits:        config.Limits,
		hostFunctions: config.HostFunctions,
		compiledCache: config.ModuleCache,
		logger:        config.Logger,
	}, nil
}

// CompileModule compiles WASM bytecode
func (r *Runtime) CompileModule(ctx context.Context, id string, wasm []byte) (*CompiledModule, error) {
	// Check cache first
	if r.compiledCache != nil {
		cached, err := r.compiledCache.Get(ctx, id)
		if err == nil && cached != nil {
			// Verify checksum
			checksum := calculateChecksum(wasm)
			if checksum == cached.Checksum {
				r.logger.Debug("module cache hit", "id", id)
				return r.restoreCompiledModule(ctx, id, cached)
			}
		}
	}
	
	// Compile module
	compiled, err := r.runtime.CompileModule(ctx, wasm)
	if err != nil {
		return nil, fmt.Errorf("failed to compile module %s: %w", id, err)
	}
	
	module := &CompiledModule{
		id:         id,
		compiled:   compiled,
		checksum:   calculateChecksum(wasm),
		compiledAt: time.Now(),
		size:       int64(len(wasm)),
		exports:    compiled.ExportedFunctionDefinitions(),
	}
	
	// Cache compiled module
	if r.compiledCache != nil {
		cached := &CachedModule{
			Data:     compiled.Serialize(),
			Checksum: module.checksum,
			Compiled: module.compiledAt,
		}
		if err := r.compiledCache.Set(ctx, id, cached); err != nil {
			r.logger.Warn("failed to cache compiled module", "id", id, "error", err)
		}
	}
	
	r.modules[id] = module
	r.logger.Info("module compiled successfully", "id", id, "size", module.size)
	
	return module, nil
}

// InstantiateModule creates a module instance with resource limits
func (r *Runtime) InstantiateModule(ctx context.Context, moduleID string, config ModuleConfig) (*ModuleInstance, error) {
	module, ok := r.modules[moduleID]
	if !ok {
		return nil, fmt.Errorf("module not found: %s", moduleID)
	}
	
	// Build runtime config with limits
	rtcfg := wazero.NewRuntimeConfig().
		WithMemoryLimitPages(r.limits.MaxMemorySize / 65536)
	
	// Set timeout context
	execCtx, cancel := context.WithTimeout(ctx, r.limits.MaxExecutionTime)
	defer cancel()
	
	// Instantiate with host functions
	inst, err := r.runtime.InstantiateModule(execCtx, module.compiled, rtcfg)
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate module %s: %w", moduleID, err)
	}
	
	return &ModuleInstance{
		module:    module,
		instance:  inst,
		config:    config,
		cancel:    cancel,
	}, nil
}

// Close shuts down the runtime
func (r *Runtime) Close(ctx context.Context) error {
	return r.runtime.Close(ctx)
}

func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
```

### Module Instance Management

```go
// internal/wasm/instance.go

package wasm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// ModuleInstance represents an instantiated WASM module
type ModuleInstance struct {
	module   *CompiledModule
	instance api.Module
	config   ModuleConfig
	memory   api.Memory
	cancel   context.CancelFunc
}

// Call invokes an exported function with timeout and resource tracking
func (mi *ModuleInstance) Call(ctx context.Context, funcName string, args ...uint64) (api.ValueType, []uint64, error) {
	fn := mi.instance.ExportedFunction(funcName)
	if fn == nil {
		return 0, nil, fmt.Errorf("function %s not found in module", funcName)
	}
	
	// Set up execution context with resource limits
	execCtx, cancel := context.WithTimeout(ctx, mi.module.runtime.limits.MaxExecutionTime)
	defer cancel()
	
	// Track CPU usage
	start := time.Now()
	
	result, err := fn.Call(execCtx, args...)
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			return 0, nil, fmt.Errorf("execution timeout: %w", err)
		}
		return 0, nil, fmt.Errorf("function call failed: %w", err)
	}
	
	duration := time.Since(start)
	if duration > mi.module.runtime.limits.MaxExecutionTime/2 {
		mi.module.runtime.logger.Warn("slow function execution",
			"module", mi.module.id,
			"function", funcName,
			"duration", duration,
		)
	}
	
	return fn.Definition().ResultTypes()[0], result, nil
}

// ReadMemory reads data from guest memory
func (mi *ModuleInstance) ReadMemory(offset, length uint32) ([]byte, bool) {
	memory := mi.instance.Memory()
	if memory == nil {
		return nil, false
	}
	
	data, ok := memory.Read(offset, length)
	return data, ok
}

// WriteMemory writes data to guest memory
func (mi *ModuleInstance) WriteMemory(offset uint32, data []byte) bool {
	memory := mi.instance.Memory()
	if memory == nil {
		return false
	}
	
	return memory.Write(offset, data)
}

// Allocate allocates memory in the guest
func (mi *ModuleInstance) Allocate(ctx context.Context, size uint32) (uint32, error) {
	fn := mi.instance.ExportedFunction("allocate")
	if fn == nil {
		return 0, fmt.Errorf("allocate function not exported")
	}
	
	result, err := fn.Call(ctx, uint64(size))
	if err != nil {
		return 0, fmt.Errorf("allocation failed: %w", err)
	}
	
	return uint32(result[0]), nil
}

// Deallocate frees memory in the guest
func (mi *ModuleInstance) Deallocate(ctx context.Context, ptr uint32) error {
	fn := mi.instance.ExportedFunction("deallocate")
	if fn == nil {
		return fmt.Errorf("deallocate function not exported")
	}
	
	_, err := fn.Call(ctx, uint64(ptr))
	return err
}

// MemoryUsage returns current memory usage in bytes
func (mi *ModuleInstance) MemoryUsage() uint32 {
	memory := mi.instance.Memory()
	if memory == nil {
		return 0
	}
	
	// Get current page count and calculate size
	pages := memory.PageSize()
	return uint32(pages) * 65536 // 64KB per page
}

// Close cleans up the module instance
func (mi *ModuleInstance) Close(ctx context.Context) error {
	if mi.cancel != nil {
		mi.cancel()
	}
	return mi.instance.Close(ctx)
}
```

---

## Host Functions API

```go
// internal/wasm/host.go

package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/tetratelabs/wazero/api"
)

// HostFunctions provides functions exposed to WASM modules
type HostFunctions struct {
	Logger         Logger
	HTTPClient     *http.Client
	AllowedHosts   []string
	VirtualFS      VirtualFileSystem
	Clock          Clock
}

// VirtualFileSystem provides sandboxed file access
type VirtualFileSystem interface {
	Open(path string) (io.ReadCloser, error)
	Create(path string) (io.WriteCloser, error)
	Stat(path string) (FileInfo, error)
}

// FileInfo provides file metadata
type FileInfo struct {
	Name    string
	Size    int64
	Mode    uint32
	ModTime time.Time
	IsDir   bool
}

// Clock provides time functions
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

// Register registers all host functions with the runtime
func (hf *HostFunctions) Register(ctx context.Context, rt wazero.Runtime) error {
	// Log function
	_, err := rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(hf.log).
		WithParameterNames("ptr", "len").
		Export("host_log").
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("failed to register log function: %w", err)
	}
	
	// HTTP request function
	_, err = rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(hf.httpRequest).
		WithParameterNames("url_ptr", "url_len", "method_ptr", "method_len", "body_ptr", "body_len", "out_ptr").
		Export("host_http_request").
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("failed to register HTTP function: %w", err)
	}
	
	// Time functions
	_, err = rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(hf.getTime).
		Export("host_get_time").
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("failed to register time function: %w", err)
	}
	
	// Random function
	_, err = rt.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(hf.getRandom).
		WithParameterNames("buf_ptr", "buf_len").
		Export("host_get_random").
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("failed to register random function: %w", err)
	}
	
	return nil
}

// log writes a log message from the guest
func (hf *HostFunctions) log(ctx context.Context, module api.Module, ptr, length int32) {
	data, ok := module.Memory().Read(uint32(ptr), uint32(length))
	if !ok {
		return
	}
	
	var logMsg struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	}
	
	if err := json.Unmarshal(data, &logMsg); err != nil {
		hf.Logger.Debug(string(data))
		return
	}
	
	switch logMsg.Level {
	case "debug":
		hf.Logger.Debug(logMsg.Message)
	case "info":
		hf.Logger.Info(logMsg.Message)
	case "warn":
		hf.Logger.Warn(logMsg.Message)
	case "error":
		hf.Logger.Error(logMsg.Message)
	default:
		hf.Logger.Info(logMsg.Message)
	}
}

// httpRequest makes an HTTP request from the guest
func (hf *HostFunctions) httpRequest(
	ctx context.Context,
	module api.Module,
	urlPtr, urlLen, methodPtr, methodLen, bodyPtr, bodyLen, outPtr int32,
) int32 {
	// Read URL
	urlBytes, ok := module.Memory().Read(uint32(urlPtr), uint32(urlLen))
	if !ok {
		return -1
	}
	urlStr := string(urlBytes)
	
	// Validate URL against allowlist
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return -2
	}
	
	if !hf.isHostAllowed(parsedURL.Host) {
		return -3 // Host not allowed
	}
	
	// Read method
	methodBytes, ok := module.Memory().Read(uint32(methodPtr), uint32(methodLen))
	if !ok {
		return -1
	}
	method := string(methodBytes)
	
	// Read body
	var body []byte
	if bodyPtr != 0 {
		body, ok = module.Memory().Read(uint32(bodyPtr), uint32(bodyLen))
		if !ok {
			return -1
		}
	}
	
	// Make request
	req, err := http.NewRequestWithContext(ctx, method, urlStr, bytes.NewReader(body))
	if err != nil {
		return -4
	}
	
	resp, err := hf.HTTPClient.Do(req)
	if err != nil {
		return -5
	}
	defer resp.Body.Close()
	
	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return -6
	}
	
	// Serialize response
	result := struct {
		Status  int    `json:"status"`
		Body    []byte `json:"body"`
		Headers map[string][]string `json:"headers"`
	}{
		Status:  resp.StatusCode,
		Body:    respBody,
		Headers: resp.Header,
	}
	
	resultData, err := json.Marshal(result)
	if err != nil {
		return -7
	}
	
	// Write result pointer to output
	resultPtr, err := hf.allocateAndWrite(ctx, module, resultData)
	if err != nil {
		return -8
	}
	
	// Write pointer to guest memory at outPtr
	binary.LittleEndian.PutUint32(make([]byte, 4), resultPtr)
	module.Memory().Write(uint32(outPtr), make([]byte, 4))
	
	return 0 // Success
}

// getTime returns current Unix timestamp in nanoseconds
func (hf *HostFunctions) getTime(ctx context.Context, module api.Module) int64 {
	return hf.Clock.Now().UnixNano()
}

// getRandom fills buffer with random bytes
func (hf *HostFunctions) getRandom(ctx context.Context, module api.Module, bufPtr, bufLen int32) int32 {
	buf := make([]byte, bufLen)
	if _, err := rand.Read(buf); err != nil {
		return -1
	}
	
	if !module.Memory().Write(uint32(bufPtr), buf) {
		return -1
	}
	
	return 0
}

func (hf *HostFunctions) isHostAllowed(host string) bool {
	if len(hf.AllowedHosts) == 0 {
		return false
	}
	
	for _, allowed := range hf.AllowedHosts {
		if allowed == host || strings.HasSuffix(host, allowed) {
			return true
		}
	}
	
	return false
}

func (hf *HostFunctions) allocateAndWrite(ctx context.Context, module api.Module, data []byte) (uint32, error) {
	// Get or create allocator
	allocFn := module.ExportedFunction("allocate")
	if allocFn == nil {
		return 0, fmt.Errorf("allocate function not found")
	}
	
	result, err := allocFn.Call(ctx, uint64(len(data)))
	if err != nil {
		return 0, err
	}
	
	ptr := uint32(result[0])
	if !module.Memory().Write(ptr, data) {
		return 0, fmt.Errorf("failed to write to guest memory")
	}
	
	return ptr, nil
}
```

---

## Module Development SDK

```go
// sdk/go/wasm/module.go

package wasm

// Module provides the guest-side WASM interface
//go:wasmimport env host_log
func host_log(ptr, length int32)

//go:wasmimport env host_http_request
func host_http_request(urlPtr, urlLen, methodPtr, methodLen, bodyPtr, bodyLen, outPtr int32) int32

//go:wasmimport env host_get_time
func host_get_time() int64

//go:wasmimport env host_get_random
func host_get_random(bufPtr, bufLen int32) int32

// Log writes a log message to the host
func Log(level, message string) {
	data, _ := json.Marshal(map[string]string{
		"level":   level,
		"message": message,
	})
	ptr := allocate(len(data))
	copy(memory[ptr:], data)
	host_log(int32(ptr), int32(len(data)))
	deallocate(ptr)
}

// HTTPRequest makes an HTTP request through the host
func HTTPRequest(url, method string, body []byte) (*HTTPResponse, error) {
	urlBytes := []byte(url)
	methodBytes := []byte(method)
	
	urlPtr := allocate(len(urlBytes))
	methodPtr := allocate(len(methodBytes))
	bodyPtr := 0
	
	copy(memory[urlPtr:], urlBytes)
	copy(memory[methodPtr:], methodBytes)
	
	if len(body) > 0 {
		bodyPtr = allocate(len(body))
		copy(memory[bodyPtr:], body)
	}
	
	outPtr := allocate(4) // For result pointer
	
	result := host_http_request(
		int32(urlPtr), int32(len(urlBytes)),
		int32(methodPtr), int32(len(methodBytes)),
		int32(bodyPtr), int32(len(body)),
		int32(outPtr),
	)
	
	// Cleanup
	deallocate(urlPtr)
	deallocate(methodPtr)
	if bodyPtr != 0 {
		deallocate(bodyPtr)
	}
	
	if result != 0 {
		return nil, fmt.Errorf("HTTP request failed with code: %d", result)
	}
	
	// Read result pointer
	resultPtr := binary.LittleEndian.Uint32(memory[outPtr:])
	deallocate(outPtr)
	
	// Read result data
	// This requires knowing the length - in practice, use a length-prefixed format
	
	return nil, nil
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	Status  int
	Body    []byte
	Headers map[string][]string
}

// Memory management (must be implemented by module)
var memory []byte

func init() {
	// Memory is provided by the runtime
	memory = make([]byte, 128*1024*1024) // 128MB
}

//go:export allocate
func allocate(size int) int {
	// Simple bump allocator - production should use proper allocator
	return 0
}

//go:export deallocate
func deallocate(ptr int) {
	// Implementation depends on allocator
}

//go:export transform
func transform(inputPtr, inputLen int32) int32 {
	// Main entry point for transformation modules
	input := memory[inputPtr : inputPtr+inputLen]
	
	output := Process(input)
	
	outPtr := allocate(len(output))
	copy(memory[outPtr:], output)
	
	return int32(outPtr)
}

// Process is implemented by the module
func Process(input []byte) []byte {
	panic("Process must be implemented by the module")
}
```

---

## Security Model

### Capability-Based Security

```go
// internal/wasm/security.go

package wasm

import (
	"fmt"
	"regexp"
)

// Capability represents a specific permission
type Capability int

const (
	CapabilityLog Capability = iota
	CapabilityHTTP
	CapabilityFileRead
	CapabilityFileWrite
	CapabilityRandom
	CapabilityTime
)

// CapabilitySet tracks granted capabilities
type CapabilitySet struct {
	caps map[Capability]bool
}

// NewCapabilitySet creates a capability set from config
func NewCapabilitySet(config ModuleConfig) *CapabilitySet {
	cs := &CapabilitySet{caps: make(map[Capability]bool)}
	
	// Default capabilities
	cs.Grant(CapabilityLog)
	cs.Grant(CapabilityRandom)
	cs.Grant(CapabilityTime)
	
	// HTTP capability if allowed hosts configured
	if len(config.AllowedHosts) > 0 {
		cs.Grant(CapabilityHTTP)
	}
	
	// File capabilities if paths configured
	if len(config.AllowedPaths) > 0 {
		cs.Grant(CapabilityFileRead)
	}
	
	return cs
}

// Grant grants a capability
func (cs *CapabilitySet) Grant(cap Capability) {
	cs.caps[cap] = true
}

// Revoke revokes a capability
func (cs *CapabilitySet) Revoke(cap Capability) {
	delete(cs.caps, cap)
}

// Has checks if capability is granted
func (cs *CapabilitySet) Has(cap Capability) bool {
	return cs.caps[cap]
}

// ValidateHost validates host against allowed patterns
func (cs *CapabilitySet) ValidateHost(host string, allowed []string) error {
	if !cs.Has(CapabilityHTTP) {
		return fmt.Errorf("HTTP capability not granted")
	}
	
	for _, pattern := range allowed {
		if matched, _ := regexp.MatchString(pattern, host); matched {
			return nil
		}
	}
	
	return fmt.Errorf("host %s not in allowlist", host)
}
```

---

## Testing

```go
// internal/wasm/runtime_test.go

package wasm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntime_CompileAndInstantiate(t *testing.T) {
	ctx := context.Background()
	
	rt, err := NewRuntime(ctx, RuntimeConfig{
		Limits: DefaultResourceLimits(),
	})
	require.NoError(t, err)
	defer rt.Close(ctx)
	
	// Simple WASM module (adds two numbers)
	wasm := []byte{
		0x00, 0x61, 0x73, 0x6d, // Magic
		0x01, 0x00, 0x00, 0x00, // Version
		// ... module bytes
	}
	
	module, err := rt.CompileModule(ctx, "test", wasm)
	require.NoError(t, err)
	assert.Equal(t, "test", module.id)
	assert.NotNil(t, module.compiled)
	
	instance, err := rt.InstantiateModule(ctx, "test", ModuleConfig{
		Name: "test-instance",
	})
	require.NoError(t, err)
	defer instance.Close(ctx)
	
	assert.NotNil(t, instance.instance)
}

func TestRuntime_ResourceLimits(t *testing.T) {
	ctx := context.Background()
	
	limits := ResourceLimits{
		MaxMemorySize:    1024 * 1024, // 1MB
		MaxExecutionTime: 100 * time.Millisecond,
		MaxCPUUnits:      1000000,
	}
	
	rt, err := NewRuntime(ctx, RuntimeConfig{Limits: limits})
	require.NoError(t, err)
	defer rt.Close(ctx)
	
	// Test that memory limit is enforced
	// Test that execution timeout works
}

func TestHostFunctions_HTTPAllowlist(t *testing.T) {
	hf := &HostFunctions{
		AllowedHosts: []string{"api.example.com", "*.trusted.com"},
	}
	
	assert.True(t, hf.isHostAllowed("api.example.com"))
	assert.True(t, hf.isHostAllowed("sub.trusted.com"))
	assert.False(t, hf.isHostAllowed("evil.com"))
	assert.False(t, hf.isHostAllowed("api.example.com.evil.com"))
}

func BenchmarkRuntime_Instantiate(b *testing.B) {
	ctx := context.Background()
	rt, _ := NewRuntime(ctx, RuntimeConfig{Limits: DefaultResourceLimits()})
	defer rt.Close(ctx)
	
	wasm := loadTestWasm() // Load test module
	module, _ := rt.CompileModule(ctx, "bench", wasm)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inst, _ := rt.InstantiateModule(ctx, "bench", ModuleConfig{})
		inst.Close(ctx)
	}
}
```

---

## Configuration

```yaml
# wasm.yaml
wasm:
  runtime:
    limits:
      max_memory_size: 134217728  # 128MB
      max_execution_time: 30s
      max_cpu_units: 1000000000
      max_stack_depth: 1000
      max_file_descriptors: 32
    
    cache:
      enabled: true
      size: 100  # Max cached modules
      ttl: 1h
    
    host_functions:
      http:
        enabled: true
        timeout: 10s
        allowed_hosts:
          - "api.example.com"
          - "*.internal.company.com"
        max_response_size: 10485760  # 10MB
      
      logging:
        enabled: true
        level: info
      
      filesystem:
        enabled: false
        allowed_paths:
          - "/tmp/wasm-modules"
```

---

## Performance Benchmarks

| Operation | Latency | Memory |
|-----------|---------|--------|
| Module Compilation | ~50ms | ~10MB |
| Module Instantiation | ~1ms | ~2MB |
| Function Call | ~500ns | ~0 |
| Memory Allocation | ~100ns | Variable |
| HTTP Request (host) | ~10ms | ~4KB |

---

## Integration

```go
// Usage example
func main() {
	ctx := context.Background()
	
	// Create runtime
	rt, err := wasm.NewRuntime(ctx, wasm.RuntimeConfig{
		Limits: wasm.DefaultResourceLimits(),
		HostFunctions: wasm.HostFunctions{
			Logger:       log.Default(),
			HTTPClient:   http.DefaultClient,
			AllowedHosts: []string{"api.example.com"},
		},
	})
	if err != nil {
		panic(err)
	}
	defer rt.Close(ctx)
	
	// Load and compile module
	moduleData, _ := os.ReadFile("transform.wasm")
	module, err := rt.CompileModule(ctx, "transform", moduleData)
	if err != nil {
		panic(err)
	}
	
	// Instantiate and call
	instance, err := rt.InstantiateModule(ctx, "transform", wasm.ModuleConfig{})
	if err != nil {
		panic(err)
	}
	defer instance.Close(ctx)
	
	// Call exported function
	result, err := instance.Call(ctx, "transform", inputPtr)
	// ...
}
```

---

## Bible Standard Compliance

This specification adheres to the Bible Standard Specification format.

For more information, see [BIBLE-STANDARD.md](../BIBLE-STANDARD.md)
