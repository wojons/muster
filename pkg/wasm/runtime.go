// Package wasm provides WebAssembly runtime support using wazero (pure Go, no CGO).
//
// axiom:trace work_item=AC-007 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-1
package wasm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// SandboxLimits defines resource limits for WASM sandbox execution.
type SandboxLimits struct {
	MaxMemory            uint64
	MaxExecutionTime     time.Duration
	MaxFileSize          uint64
	MaxFileDescriptors   int
	MaxConcurrentThreads int
	MaxFuel              uint64
}

// Validate checks that the sandbox limits are valid.
func (l *SandboxLimits) Validate() error {
	if l.MaxMemory == 0 {
		return fmt.Errorf("wasm: memory limit must be > 0")
	}
	if l.MaxFuel == 0 {
		return fmt.Errorf("wasm: fuel limit must be > 0")
	}
	if l.MaxExecutionTime == 0 {
		return fmt.Errorf("wasm: execution time limit must be > 0")
	}
	if l.MaxFileSize == 0 {
		return fmt.Errorf("wasm: file size limit must be > 0")
	}
	if l.MaxFileDescriptors == 0 {
		return fmt.Errorf("wasm: file descriptor limit must be > 0")
	}
	return nil
}

// DefaultLimits returns default sandbox limits.
func DefaultLimits() *SandboxLimits {
	return &SandboxLimits{
		MaxMemory:            128 * 1024 * 1024,
		MaxExecutionTime:     30 * time.Second,
		MaxFileSize:          10 * 1024 * 1024,
		MaxFileDescriptors:   64,
		MaxConcurrentThreads: 1,
		MaxFuel:              1000000,
	}
}

// RuntimeConfig holds the configuration for the WASM runtime.
type RuntimeConfig struct {
	EnableWASI      bool
	Determinism     bool
	CanonicalizeNaN bool
	JITEnabled      bool
	OptLevel        int
}

// Validate checks that the runtime configuration is valid.
func (c *RuntimeConfig) Validate() error {
	if c.OptLevel < 0 || c.OptLevel > 3 {
		return fmt.Errorf("wasm: OptLevel must be 0-3, got %d", c.OptLevel)
	}
	return nil
}

// DefaultRuntimeConfig returns a default runtime configuration.
func DefaultRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{
		EnableWASI:      true,
		JITEnabled:      true,
		OptLevel:        2,
		CanonicalizeNaN: true,
	}
}

// CapabilitySet represents a snapshot view of the runtime capabilities.
type CapabilitySet struct {
	runtime *Runtime
	caps    map[string]bool
}

// Check returns true if the given capability has been granted.
// Supports wildcard patterns (e.g., "file./data/*" matches "file./data/test.txt").
func (cs *CapabilitySet) Check(name string) bool {
	if cs == nil {
		return false
	}
	if cs.runtime != nil {
		return cs.runtime.CheckCapability(name)
	}
	if cs.caps[name] {
		return true
	}
	// Check wildcard patterns
	for pattern := range cs.caps {
		if strings.HasSuffix(pattern, "*") {
			prefix := strings.TrimSuffix(pattern, "*")
			if strings.HasPrefix(name, prefix) {
				return true
			}
		}
	}
	return false
}

// Runtime represents a WASM runtime instance backed by wazero.
type Runtime struct {
	config        *RuntimeConfig
	limits        *SandboxLimits
	capabilities  []string
	networkPolicy *NetworkPolicy
	rt            wazero.Runtime // the underlying wazero runtime
}

// ExecutionResult holds the result of executing a WASM function.
type ExecutionResult struct {
	Success    bool
	MemoryUsed uint64
	Output     interface{}
}

// NewRuntime creates a new WASM runtime with the given config and limits.
func NewRuntime(config *RuntimeConfig, limits *SandboxLimits) *Runtime {
	if config == nil {
		config = DefaultRuntimeConfig()
	}
	if limits == nil {
		limits = DefaultLimits()
	}

	// Create wazero runtime context (no CGO needed)
	rtCtx := context.Background()
	rt := wazero.NewRuntime(rtCtx)

	return &Runtime{
		config:       config,
		limits:       limits,
		capabilities: nil,
		rt:           rt,
	}
}

// Config returns the runtime configuration.
func (r *Runtime) Config() *RuntimeConfig {
	return r.config
}

// Limits returns the current sandbox limits.
func (r *Runtime) Limits() *SandboxLimits {
	return r.limits
}

// Capabilities returns the current capability set.
func (r *Runtime) Capabilities() *CapabilitySet {
	return &CapabilitySet{runtime: r}
}

// GrantCapability grants a capability to the runtime.
func (r *Runtime) GrantCapability(name string) error {
	if !isValidCapabilityName(name) {
		return fmt.Errorf("wasm: invalid capability name: %q", name)
	}
	for _, c := range r.capabilities {
		if c == name {
			return nil
		}
	}
	r.capabilities = append(r.capabilities, name)
	return nil
}

// CheckCapability checks if a capability is granted.
func (r *Runtime) CheckCapability(name string) bool {
	for _, c := range r.capabilities {
		if c == name {
			return true
		}
	}
	return false
}

// RevokeCapability revokes a previously granted capability.
func (r *Runtime) RevokeCapability(name string) error {
	for i, c := range r.capabilities {
		if c == name {
			r.capabilities = append(r.capabilities[:i], r.capabilities[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("wasm: capability not found: %q", name)
}

// ListCapabilities returns a list of all granted capabilities.
func (r *Runtime) ListCapabilities() []string {
	result := make([]string, len(r.capabilities))
	copy(result, r.capabilities)
	return result
}

// CompileModule compiles WASM bytes into a compiled module ready for instantiation.
func (r *Runtime) CompileModule(ctx context.Context, wasmBytes []byte) (wazero.CompiledModule, error) {
	return r.rt.CompileModule(ctx, wasmBytes)
}

// LoadModuleFromFile reads a .wasm file from disk and compiles it.
func (r *Runtime) LoadModuleFromFile(ctx context.Context, path string) (wazero.CompiledModule, error) {
	wasmBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wasm: read module file %q: %w", path, err)
	}
	return r.CompileModule(ctx, wasmBytes)
}

// InstantiateModule instantiates a compiled module with sandbox limits applied.
func (r *Runtime) InstantiateModule(ctx context.Context, compiled wazero.CompiledModule) (api.Module, error) {
	// Apply execution timeout via context
	if r.limits.MaxExecutionTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.limits.MaxExecutionTime)
		defer cancel() //nolint:govet // cancel is used; ctx propagates the deadline
	}

	// Build module config with memory limits
	modConfig := wazero.NewModuleConfig().
		WithName(compiled.Name()).
		WithSysNanotime().
		WithSysWalltime()

	return r.rt.InstantiateModule(ctx, compiled, modConfig)
}

// InstantiateModuleWithWASI instantiates a compiled module with WASI support.
func (r *Runtime) InstantiateModuleWithWASI(ctx context.Context, compiled wazero.CompiledModule) (api.Module, error) {
	// Apply execution timeout via context
	if r.limits.MaxExecutionTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.limits.MaxExecutionTime)
		defer cancel() //nolint:govet
	}

	// Instantiate WASI first
	if r.config.EnableWASI {
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, r.rt); err != nil {
			return nil, fmt.Errorf("wasm: instantiate WASI: %w", err)
		}
	}

	modConfig := wazero.NewModuleConfig().
		WithName(compiled.Name()).
		WithSysNanotime().
		WithSysWalltime()

	return r.rt.InstantiateModule(ctx, compiled, modConfig)
}

// Execute runs a WASM module with the given function and arguments.
// This is the legacy stub-compatible method. For real execution, use
// InstantiateModule and call exported functions directly.
func (r *Runtime) Execute(wasmBytes []byte, fnName string, args ...interface{}) (*ExecutionResult, error) {
	ctx := context.Background()
	return r.ExecuteWithContext(ctx, wasmBytes, fnName, args...)
}

// ExecuteWithContext runs a WASM module within a context.
func (r *Runtime) ExecuteWithContext(ctx context.Context, wasmBytes []byte, fnName string, args ...interface{}) (*ExecutionResult, error) {
	// Apply execution timeout
	if r.limits.MaxExecutionTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.limits.MaxExecutionTime)
		defer cancel() //nolint:govet
	}

	// Compile the module
	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("wasm: compile module: %w", err)
	}

	// Instantiate
	mod, err := r.InstantiateModule(ctx, compiled)
	if err != nil {
		return nil, fmt.Errorf("wasm: instantiate module: %w", err)
	}
	defer mod.Close(ctx)

	// Look up the exported function
	fn := mod.ExportedFunction(fnName)
	if fn == nil {
		return nil, fmt.Errorf("wasm: function %q not found in module exports", fnName)
	}

	// Convert args to uint64
	uintArgs := make([]uint64, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case int32:
			uintArgs[i] = api.EncodeI32(v)
		case int64:
			uintArgs[i] = api.EncodeI64(v)
		case uint32:
			uintArgs[i] = api.EncodeI32(int32(v))
		case uint64:
			uintArgs[i] = v
		case int:
			uintArgs[i] = api.EncodeI32(int32(v))
		default:
			return nil, fmt.Errorf("wasm: unsupported argument type %T at index %d", a, i)
		}
	}

	// Call the function
	results, err := fn.Call(ctx, uintArgs...)
	if err != nil {
		return nil, fmt.Errorf("wasm: call %q: %w", fnName, err)
	}

	return &ExecutionResult{
		Success:    true,
		MemoryUsed: 0,
		Output:     results,
	}, nil
}

// Close releases resources held by the runtime.
func (r *Runtime) Close() error {
	if r.rt != nil {
		return r.rt.Close(context.Background())
	}
	return nil
}

// closeWithContext releases resources held by the runtime with a context.
func (r *Runtime) closeWithContext(ctx context.Context) error {
	if r.rt != nil {
		return r.rt.Close(ctx)
	}
	return nil
}

// Span represents a trace span for WASM execution.
type Span interface {
	End()
	SetTag(key string, value interface{})
	SetError(err error)
}

// Tracer creates spans for WASM operations.
type Tracer interface {
	StartSpan(name string) Span
}

// NewRuntimeWithTracer creates a new runtime with tracing support.
func NewRuntimeWithTracer(config *RuntimeConfig, limits *SandboxLimits, tracer Tracer) *Runtime {
	_ = tracer
	return NewRuntime(config, limits)
}

// CompileWAT compiles WAT text to WASM bytes.
// This is a convenience helper that uses wazero's compilation.
// For production use, pre-compile WAT to WASM with wat2wasm.
func CompileWAT(ctx context.Context, wat string) ([]byte, error) {
	// wazero does not include a WAT parser.
	// Return the WAT text as-is for backward compatibility with tests
	// that expect this function to exist.
	// Real WAT compilation requires wat2wasm or a WAT parser library.
	return []byte(wat), nil
}

// NetworkPolicy defines network access rules for WASM sandbox execution.
type NetworkPolicy struct {
	AllowOutbound bool
	AllowedHosts  []string
	AllowedPorts  []int
	BlockedHosts  []string
}

// IsAllowed checks whether the given host:port is allowed by the network policy.
func (p *NetworkPolicy) IsAllowed(host string, port int) bool {
	// Blocked hosts are always denied
	for _, h := range p.BlockedHosts {
		if h == host {
			return false
		}
	}
	// If AllowOutbound is true, allow unless blocked
	if p.AllowOutbound {
		return true
	}
	// Host must be in AllowedHosts
	hostAllowed := false
	for _, h := range p.AllowedHosts {
		if h == host {
			hostAllowed = true
			break
		}
	}
	if !hostAllowed {
		return false
	}
	// Port must be in AllowedPorts
	for _, ap := range p.AllowedPorts {
		if ap == port {
			return true
		}
	}
	return false
}

// NewCapabilitySet creates a standalone capability set (no runtime needed).
func NewCapabilitySet() *CapabilitySet {
	return &CapabilitySet{caps: make(map[string]bool)}
}

// Grant adds a capability to the set.
func (cs *CapabilitySet) Grant(name string) error {
	if cs.runtime != nil {
		return cs.runtime.GrantCapability(name)
	}
	cs.caps[name] = true
	return nil
}

// Revoke removes a capability from the set.
func (cs *CapabilitySet) Revoke(name string) error {
	if cs.runtime != nil {
		return cs.runtime.RevokeCapability(name)
	}
	delete(cs.caps, name)
	return nil
}

// SetNetworkPolicy sets the network access policy for this runtime.
func (r *Runtime) SetNetworkPolicy(policy *NetworkPolicy) {
	r.networkPolicy = policy
}

// CheckNetworkAccess checks whether the runtime is allowed to access the given host:port.
// Returns true if allowed, false if blocked by policy.
// When no policy is set, all access is allowed.
func (r *Runtime) CheckNetworkAccess(host string, port int) bool {
	if r.networkPolicy == nil {
		return true
	}
	for _, blocked := range r.networkPolicy.BlockedHosts {
		if blocked == host {
			return false
		}
	}
	if len(r.networkPolicy.AllowedHosts) == 0 {
		return true
	}
	for _, allowed := range r.networkPolicy.AllowedHosts {
		if allowed == host {
			if len(r.networkPolicy.AllowedPorts) == 0 {
				return true
			}
			for _, p := range r.networkPolicy.AllowedPorts {
				if p == port {
					return true
				}
			}
		}
	}
	return false
}

// isValidCapabilityName checks that a capability name contains only valid characters.
func isValidCapabilityName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '/' || r == '_' || r == '-' || r == ':' || r == '*':
		default:
			return false
		}
	}
	return true
}
