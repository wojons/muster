//go:build !js && !wasm
// +build !js,!wasm

// Package wasm provides WebAssembly runtime support.
// This is a stub for non-WASM platforms.
package wasm

import (
	"context"
	"fmt"
	"time"
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
		return fmt.Errorf("wasm: MaxMemory must be > 0")
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
}

// Check returns true if the given capability has been granted.
func (cs *CapabilitySet) Check(name string) bool {
	if cs == nil || cs.runtime == nil {
		return false
	}
	return cs.runtime.CheckCapability(name)
}

// Runtime represents a WASM runtime instance.
type Runtime struct {
	config       *RuntimeConfig
	limits       *SandboxLimits
	capabilities []string
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
	return &Runtime{
		config:       config,
		limits:       limits,
		capabilities: nil,
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

// Execute runs a WASM module with the given function and arguments.
func (r *Runtime) Execute(wasmBytes []byte, fnName string, args ...interface{}) (*ExecutionResult, error) {
	return &ExecutionResult{Success: true, MemoryUsed: 0, Output: nil}, nil
}

// Close releases resources held by the runtime.
func (r *Runtime) Close() error {
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

// CompileWAT compiles WAT text to WASM bytes. This is a stub.
func CompileWAT(ctx context.Context, wat string) ([]byte, error) {
	return []byte(wat), nil
}

// ExecuteWithContext runs a WASM module within a context.
func (r *Runtime) ExecuteWithContext(ctx context.Context, wasmBytes []byte, fnName string, args ...interface{}) (*ExecutionResult, error) {
	return r.Execute(wasmBytes, fnName, args...)
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
