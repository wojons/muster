package wasm

import (
	"errors"
	"sync"
	"time"
)

// RuntimeConfig configures WASM runtime behavior.
type RuntimeConfig struct {
	JITEnabled      bool // Enable JIT compilation
	OptLevel        int  // Optimization level (0-3)
	CanonicalizeNaN bool // Use canonical NaN representation
	Determinism     bool // Enable deterministic execution
}

// DefaultRuntimeConfig returns the default runtime configuration.
func DefaultRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{
		JITEnabled:      true,
		OptLevel:        2,
		CanonicalizeNaN: true,
		Determinism:     true,
	}
}

// Validate checks if the runtime configuration is valid.
func (c *RuntimeConfig) Validate() error {
	if c.OptLevel < 0 || c.OptLevel > 3 {
		return errors.New("opt level must be between 0 and 3")
	}
	return nil
}

// SandboxLimits defines resource limits for WASM execution.
type SandboxLimits struct {
	MaxMemory            uint64        // Maximum memory allocation in bytes
	MaxExecutionTime     time.Duration // Maximum execution duration
	MaxFileDescriptors   int           // Maximum open file descriptors
	MaxFileSize          uint64        // Maximum file size in bytes
	MaxConcurrentThreads int           // Maximum concurrent threads
}

// DefaultLimits returns the default sandbox limits.
func DefaultLimits() *SandboxLimits {
	return &SandboxLimits{
		MaxMemory:            128 * 1024 * 1024, // 128MB
		MaxExecutionTime:     30 * time.Second,
		MaxFileDescriptors:   64,
		MaxFileSize:          10 * 1024 * 1024, // 10MB
		MaxConcurrentThreads: 4,
	}
}

// Validate checks if the sandbox limits are valid.
func (l *SandboxLimits) Validate() error {
	if l.MaxMemory == 0 {
		return errors.New("max memory must be specified")
	}
	if l.MaxExecutionTime == 0 {
		return errors.New("max execution time must be specified")
	}
	if l.MaxFileSize == 0 {
		return errors.New("max file size must be specified")
	}
	return nil
}

// NetworkPolicy defines network access restrictions.
type NetworkPolicy struct {
	AllowedHosts []string // List of allowed hostnames (supports wildcards)
	AllowedPorts []int    // List of allowed ports
	BlockedHosts []string // List of blocked hostnames
}

// IsAllowed checks if network access to a host:port is allowed.
func (p *NetworkPolicy) IsAllowed(host string, port int) bool {
	// Check blocked hosts
	for _, blocked := range p.BlockedHosts {
		if matchWildcard(blocked, host) {
			return false
		}
	}

	// If no restrictions, allow all
	if len(p.AllowedHosts) == 0 && len(p.AllowedPorts) == 0 {
		return true
	}

	hostAllowed := len(p.AllowedHosts) == 0
	for _, allowed := range p.AllowedHosts {
		if matchWildcard(allowed, host) {
			hostAllowed = true
			break
		}
	}

	portAllowed := len(p.AllowedPorts) == 0
	for _, allowedPort := range p.AllowedPorts {
		if allowedPort == port {
			portAllowed = true
			break
		}
	}

	return hostAllowed && portAllowed
}

// Runtime represents a WASM runtime with capability management.
type Runtime struct {
	config     *RuntimeConfig
	limits     *SandboxLimits
	capability *CapabilitySet
	tracer     Tracer
	mu         sync.RWMutex
}

// Tracer defines the interface for WASM execution tracing.
// This abstraction allows for pluggable tracing implementations.
type Tracer interface {
	// StartSpan creates a new span for WASM execution.
	StartSpan(name string) Span
}

// Span represents a trace span for WASM operations.
type Span interface {
	// End completes the span.
	End()
	// SetTag adds a tag/attribute to the span.
	SetTag(key string, value interface{})
	// SetError marks the span as errored with the given error.
	SetError(err error)
}

// NewRuntime creates a new WASM runtime.
func NewRuntime(config *RuntimeConfig, limits *SandboxLimits) *Runtime {
	return NewRuntimeWithTracer(config, limits, nil)
}

// NewRuntimeWithTracer creates a new WASM runtime with a tracer.
//
// Example:
//
//	tracer := telemetry.NewTracer(provider.Tracer("wasm"))
//	runtime := NewRuntimeWithTracer(config, limits, NewTelemetryTracer(tracer))
func NewRuntimeWithTracer(config *RuntimeConfig, limits *SandboxLimits, tracer Tracer) *Runtime {
	if config == nil {
		config = DefaultRuntimeConfig()
	}
	if limits == nil {
		limits = DefaultLimits()
	}

	return &Runtime{
		config:     config,
		limits:     limits,
		capability: NewCapabilitySet(),
		tracer:     tracer,
	}
}

// Config returns the runtime configuration.
func (r *Runtime) Config() *RuntimeConfig {
	return r.config
}

// Limits returns the sandbox limits.
func (r *Runtime) Limits() *SandboxLimits {
	return r.limits
}

// Capabilities returns the capability set.
func (r *Runtime) Capabilities() *CapabilitySet {
	return r.capability
}

// GrantCapability grants a capability to the runtime.
func (r *Runtime) GrantCapability(capability string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.capability.Grant(capability)
}

// CheckCapability checks if a capability is granted.
func (r *Runtime) CheckCapability(capability string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.capability.Check(capability)
}

// RevokeCapability revokes a capability from the runtime.
func (r *Runtime) RevokeCapability(capability string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.capability.Revoke(capability)
}

// ListCapabilities returns all granted capabilities.
func (r *Runtime) ListCapabilities() []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.capability.List()
}

// SetTracer sets the tracer for the runtime.
// This allows adding tracing to an existing runtime instance.
func (r *Runtime) SetTracer(tracer Tracer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tracer = tracer
}

// Tracer returns the current tracer for the runtime.
func (r *Runtime) Tracer() Tracer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tracer
}
