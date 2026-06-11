// Package wasm — Module management for WASM plugins using WASI stdin/stdout convention.
//
// Modules are compiled with GOOS=wasip1 GOARCH=wasm. They read input from stdin
// and write output to stdout. This is the simplest, most portable convention
// for wazero (pure Go, no CGO).
//
// axiom:trace work_item=AC-007 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-3
package wasm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// ModuleManager handles loading, caching, and lifecycle of WASM modules.
// It is safe for concurrent use. Each Transform call creates a fresh
// WASM instance (modules are stateless by design).
type ModuleManager struct {
	mu      sync.RWMutex
	execMu  sync.Mutex
	modules map[string]*LoadedModule
	limits  *SandboxLimits
}

// LoadedModule represents a loaded and validated WASM module ready for execution.
type LoadedModule struct {
	Name      string
	Path      string
	wasmBytes []byte
}

// NewModuleManager creates a new module manager.
func NewModuleManager(limits *SandboxLimits) *ModuleManager {
	if limits == nil {
		limits = DefaultLimits()
	}
	return &ModuleManager{
		modules: make(map[string]*LoadedModule),
		limits:  limits,
	}
}

// Load loads a WASM module from disk and caches its raw bytes.
func (m *ModuleManager) Load(ctx context.Context, path string) (*LoadedModule, error) {
	m.mu.RLock()
	if existing, ok := m.modules[path]; ok {
		m.mu.RUnlock()
		return existing, nil
	}
	m.mu.RUnlock()

	wasmBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wasm: read module file %q: %w", path, err)
	}

	// Validate by attempting compilation. Serialize with execMu
	// to prevent races with concurrent Transform calls.
	m.execMu.Lock()
	rt := wazero.NewRuntime(ctx)
	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		rt.Close(ctx)
		m.execMu.Unlock()
		return nil, fmt.Errorf("wasm: compile module %q: %w", path, err)
	}
	compiled.Close(ctx)
	rt.Close(ctx)
	m.execMu.Unlock()

	lm := &LoadedModule{
		Name:      compiled.Name(),
		Path:      path,
		wasmBytes: wasmBytes,
	}

	m.mu.Lock()
	m.modules[path] = lm
	m.mu.Unlock()

	return lm, nil
}

// Unload removes a module from the cache.
func (m *ModuleManager) Unload(ctx context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.modules[path]
	if !ok {
		return fmt.Errorf("wasm: module %q not loaded", path)
	}

	delete(m.modules, path)
	return nil
}

// Get returns a loaded module from the cache, or nil if not loaded.
func (m *ModuleManager) Get(path string) *LoadedModule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.modules[path]
}

// List returns all loaded module paths.
func (m *ModuleManager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	paths := make([]string, 0, len(m.modules))
	for p := range m.modules {
		paths = append(paths, p)
	}
	return paths
}

// Close clears all loaded modules.
func (m *ModuleManager) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.modules = make(map[string]*LoadedModule)
	return nil
}

// Transform executes the WASM module with JSON data via stdin and returns stdout.
func (m *ModuleManager) Transform(ctx context.Context, lm *LoadedModule, jsonData []byte) ([]byte, error) {
	if lm == nil {
		return nil, fmt.Errorf("wasm: module is nil")
	}

	// Determine execution timeout
	timeout := m.limits.MaxExecutionTime
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	execCtx, execCancel := context.WithTimeout(ctx, timeout)
	defer execCancel()

	// Serialize all WASM execution to prevent wazero concurrency panics.
	// The lock is held across runtime creation, execution, and cleanup so
	// that no two goroutines ever interact with wazero simultaneously.
	m.execMu.Lock()

	wasmRT := wazero.NewRuntime(execCtx)

	type result struct {
		data []byte
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		stdout, err := runWASMModule(execCtx, wasmRT, lm.wasmBytes, lm.Name, jsonData)
		resultCh <- result{data: stdout, err: err}
	}()

	select {
	case <-execCtx.Done():
		// On timeout, release the lock immediately but do NOT close the runtime.
		// The goroutine may still be executing WASM code; closing the runtime
		// concurrently corrupts wazero internals and hangs subsequent calls.
		// The abandoned runtime and goroutine will be GC'd after the process exits.
		m.execMu.Unlock()
		return nil, fmt.Errorf("wasm: execution timed out for module %q", lm.Name)
	case res := <-resultCh:
		wasmRT.Close(context.Background())
		m.execMu.Unlock()
		return res.data, res.err
	}
}

// runWASMModule executes the WASM binary in the given runtime and returns stdout.
func runWASMModule(ctx context.Context, wasmRT wazero.Runtime, wasmBytes []byte, name string, stdinData []byte) ([]byte, error) {
	// Instantiate WASI
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, wasmRT); err != nil {
		return nil, fmt.Errorf("wasm: instantiate WASI: %w", err)
	}

	// Compile the module
	compiled, err := wasmRT.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("wasm: compile module %q: %w", name, err)
	}
	defer compiled.Close(ctx)

	// Set up stdin/stdout
	stdin := bytes.NewReader(stdinData)
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	modConfig := wazero.NewModuleConfig().
		WithStdin(stdin).
		WithStdout(stdout).
		WithStderr(stderr).
		WithSysNanotime().
		WithSysWalltime().
		WithName(name)

	mod, err := wasmRT.InstantiateModule(ctx, compiled, modConfig)
	if err != nil {
		return nil, fmt.Errorf("wasm: instantiate module %q: %w", name, err)
	}

	closeErr := mod.Close(ctx)

	// Check for context deadline (timeout)
	if ctx.Err() != nil {
		return nil, fmt.Errorf("wasm: execution timed out for module %q", name)
	}

	if closeErr != nil {
		stderrStr := stderr.String()
		if stderrStr != "" {
			return nil, fmt.Errorf("wasm: module %q error: %w (stderr: %s)", name, closeErr, stderrStr)
		}
		return nil, fmt.Errorf("wasm: module %q error: %w", name, closeErr)
	}

	return stdout.Bytes(), nil
}

// Ensure time is used (imported for timeouts).
var _ = time.Now
