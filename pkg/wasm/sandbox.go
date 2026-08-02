// Package wasm provides WebAssembly runtime support using wazero (pure Go, no CGO).
//
// This file implements the filesystem sandbox (SandboxedFS) and the
// execution sandbox (ExecutionSandbox) required by the sandbox spec:
// directory allowlisting, symlink escape prevention, memory/time limits,
// and epoch-based interruption.
//
// axiom:trace work_item=ac-190-sandbox spec=specs/02-Workflows.md impl=pkg/wasm/sandbox.go
package wasm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SandboxedFS provides directory allowlisting and symlink escape prevention
// for filesystem access inside the WASM sandbox.
//
// Only paths rooted under one of the allowed root directories may be
// accessed. Symlinks are resolved and rejected if the resolved target
// escapes the allowed roots, preventing classic sandbox escapes via
// symlink chains.
type SandboxedFS struct {
	// roots are the absolute directory paths the sandbox may access.
	roots []string

	// allowWrite controls whether files under the roots may be created
	// or modified. When false the sandbox is read-only.
	allowWrite bool

	mu sync.RWMutex
}

// NewSandboxedFS creates a filesystem sandbox with the given allowed
// directories. Roots are absolutized and canonicalized (symlinks resolved);
// a non-existent root is an error so misconfiguration surfaces at
// construction time.
func NewSandboxedFS(roots []string, allowWrite bool) (*SandboxedFS, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("wasm: sandboxed fs requires at least one root directory")
	}
	abs := make([]string, 0, len(roots))
	for _, r := range roots {
		a, err := filepath.Abs(r)
		if err != nil {
			return nil, fmt.Errorf("wasm: resolve sandbox root %q: %w", r, err)
		}
		a = filepath.Clean(a)
		// Canonicalize so EvalSymlinks results in Resolve compare equal.
		// On macOS /var is a symlink to /private/var, so a root under
		// /var would otherwise reject every resolved path.
		if canon, err := filepath.EvalSymlinks(a); err == nil {
			a = canon
		}
		abs = append(abs, a)
	}
	return &SandboxedFS{roots: abs, allowWrite: allowWrite}, nil
}

// Roots returns the allowed root directories (absolute).
func (s *SandboxedFS) Roots() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.roots))
	copy(out, s.roots)
	return out
}

// AllowWrite reports whether writes are permitted inside the sandbox.
func (s *SandboxedFS) AllowWrite() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.allowWrite
}

// Resolve checks that path is inside the sandbox and returns the cleaned
// absolute path safe to use. It follows symlinks (via EvalSymlinks) and
// rejects any path whose real target escapes the allowed roots.
//
// A missing file yields a nil error only when the path stays within a
// root lexically — the caller's open/write will surface the ENOENT. A
// symlink chain that escapes the roots is always an error.
func (s *SandboxedFS) Resolve(path string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("wasm: resolve %q: %w", path, err)
	}
	abs = filepath.Clean(abs)

	// Lexical containment check against every root.
	if !s.withinRoot(abs) {
		return "", fmt.Errorf("wasm: path %q outside sandbox roots %v", path, s.roots)
	}

	// Symlink escape prevention: resolve the real target if it exists.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		if !s.withinRoot(resolved) {
			return "", fmt.Errorf("wasm: path %q escapes sandbox via symlink (target %q)", path, resolved)
		}
		return resolved, nil
	}
	// Path does not exist yet (e.g. a new file). Walk the deepest existing
	// ancestor and verify it does not escape via a symlinked parent.
	if parent := deepestExistingAncestor(abs); parent != "" {
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			if !s.withinRoot(resolved) {
				return "", fmt.Errorf("wasm: path %q escapes sandbox via symlinked parent %q", path, parent)
			}
		}
	}
	return abs, nil
}

// withinRoot reports whether abs is lexically inside any allowed root.
func (s *SandboxedFS) withinRoot(abs string) bool {
	for _, root := range s.roots {
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// deepestExistingAncestor returns the longest existing ancestor of path.
func deepestExistingAncestor(path string) string {
	cur := path
	for {
		if _, err := os.Lstat(cur); err == nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}

// ExecutionSandbox enforces resource limits around a single WASM
// execution: MaxMemory, MaxExecutionTime, and epoch-based interruption.
type ExecutionSandbox struct {
	// limits are the enforced resource bounds.
	limits *SandboxLimits

	// epoch is an atomic counter incremented to interrupt running modules.
	// An epoch of 0 means "no interruption requested".
	epoch atomic.Int64

	// startedAt records when execution began (for time enforcement).
	startedAt time.Time

	mu sync.Mutex
}

// NewExecutionSandbox creates an execution sandbox with the given limits.
func NewExecutionSandbox(limits *SandboxLimits) (*ExecutionSandbox, error) {
	if limits == nil {
		limits = DefaultLimits()
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	return &ExecutionSandbox{limits: limits}, nil
}

// Limits returns the enforced limits.
func (e *ExecutionSandbox) Limits() *SandboxLimits {
	return e.limits
}

// Start marks the beginning of execution and resets interruption state.
func (e *ExecutionSandbox) Start() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.startedAt = time.Now()
	e.epoch.Store(0)
}

// Interrupt requests that the currently running module be stopped.
func (e *ExecutionSandbox) Interrupt() {
	e.epoch.Add(1)
}

// Epoch returns the current interruption epoch. A nonzero value signals
// that an interruption was requested.
func (e *ExecutionSandbox) Epoch() int64 {
	return e.epoch.Load()
}

// CheckTimeout returns a non-nil error if the execution has exceeded
// MaxExecutionTime. Safe to call from guest code or the host loop.
func (e *ExecutionSandbox) CheckTimeout() error {
	e.mu.Lock()
	started := e.startedAt
	e.mu.Unlock()
	if started.IsZero() {
		return nil
	}
	if time.Since(started) > e.limits.MaxExecutionTime {
		return fmt.Errorf("wasm: execution exceeded %s time limit", e.limits.MaxExecutionTime)
	}
	return nil
}

// CheckMemory returns a non-nil error if the given memory usage (in bytes)
// exceeds MaxMemory.
func (e *ExecutionSandbox) CheckMemory(bytes uint64) error {
	if e.limits.MaxMemory > 0 && bytes > e.limits.MaxMemory {
		return fmt.Errorf("wasm: memory usage %d exceeds limit %d", bytes, e.limits.MaxMemory)
	}
	return nil
}

// RunWithTimeout runs fn under the sandbox, interrupting it if it exceeds
// MaxExecutionTime or if Interrupt is called. The context is used for
// external cancellation.
func (e *ExecutionSandbox) RunWithTimeout(ctx context.Context, fn func(ctx context.Context) error) error {
	e.Start()
	defer e.Interrupt() // ensure any lingering execution is halted

	done := make(chan error, 1)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		done <- fn(runCtx)
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(e.limits.MaxExecutionTime):
		e.Interrupt()
		cancel()
		<-done // wait for the goroutine to observe the interruption
		return fmt.Errorf("wasm: execution interrupted after %s", e.limits.MaxExecutionTime)
	case <-ctx.Done():
		e.Interrupt()
		cancel()
		<-done
		return ctx.Err()
	}
}
