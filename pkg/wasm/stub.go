//go:build !js && !wasm
// +build !js,!wasm

// Package wasm provides WebAssembly runtime support.
// This is a stub for non-WASM platforms.
package wasm

import (
	"context"
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

// RuntimeConfig holds the configuration for the WASM runtime.
type RuntimeConfig struct {
	EnableWASI bool
}

// DefaultRuntimeConfig returns a default runtime configuration.
func DefaultRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{EnableWASI: true}
}

// Runtime represents a WASM runtime instance.
type Runtime struct {
	config *RuntimeConfig
	limits *SandboxLimits
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
		limits = &SandboxLimits{
			MaxMemory:            128 * 1024 * 1024,
			MaxExecutionTime:     30 * time.Second,
			MaxFileSize:          10 * 1024 * 1024,
			MaxFileDescriptors:   64,
			MaxConcurrentThreads: 1,
			MaxFuel:              1000000,
		}
	}
	return &Runtime{config: config, limits: limits}
}

// Limits returns the current sandbox limits.
func (r *Runtime) Limits() *SandboxLimits {
	return r.limits
}

// GrantCapability grants a capability to the runtime.
func (r *Runtime) GrantCapability(name string) error {
	return nil
}

// CheckCapability checks if a capability is granted.
func (r *Runtime) CheckCapability(name string) bool {
	return true
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
