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
	// roots are the canonical (symlink-resolved) absolute directory paths
	// the sandbox may access. Resolve results compare against these.
	roots []string

	// lexicalRoots are the caller's root paths as given (absolutized +
	// cleaned, NOT symlink-resolved). On macOS a caller may pass /var/...
	// which canonicalizes to /private/var/...; lexical containment of the
	// caller's path is decided against this form.
	lexicalRoots []string

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
	lex := make([]string, 0, len(roots))
	for _, r := range roots {
		a, err := filepath.Abs(r)
		if err != nil {
			return nil, fmt.Errorf("wasm: resolve sandbox root %q: %w", r, err)
		}
		a = filepath.Clean(a)
		lex = append(lex, a)
		// Canonicalize so EvalSymlinks results in Resolve compare equal.
		// On macOS /var is a symlink to /private/var, so a root under
		// /var would otherwise reject every resolved path.
		if canon, err := filepath.EvalSymlinks(a); err == nil {
			a = canon
		}
		abs = append(abs, a)
	}
	return &SandboxedFS{roots: abs, lexicalRoots: lex, allowWrite: allowWrite}, nil
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

	// Canonicalize the deepest existing ancestor so comparisons are
	// canonical-to-canonical (roots are canonicalized at construction; on
	// macOS /var is a symlink to /private/var). A symlinked parent that
	// resolves outside the roots is a sandbox escape.
	canonAbs := abs
	if parent := deepestExistingAncestor(abs); parent != "" {
		if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil && resolvedParent != parent {
			canonAbs = filepath.Join(resolvedParent, strings.TrimPrefix(abs, parent))
		}
	}

	// Lexical containment check against the caller's root forms. The
	// caller's path is accepted if it lexically lives under a root as
	// given (macOS /var/... caller roots) OR under a canonical root.
	if !s.withinLexical(abs) && !s.withinRoot(canonAbs) {
		return "", fmt.Errorf("wasm: path %q outside sandbox roots %v", path, s.lexicalRoots)
	}

	// Symlink escape prevention: a canonical form that leaves the roots is
	// an escape, regardless of the lexical form the caller passed in.
	if canonAbs != abs && !s.withinRoot(canonAbs) {
		return "", fmt.Errorf("wasm: path %q escapes sandbox via symlink (target %q)", path, canonAbs)
	}
	abs = canonAbs

	// Resolve the real target if it exists and verify it stays inside.
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		if !s.withinRoot(resolved) {
			return "", fmt.Errorf("wasm: path %q escapes sandbox via symlink (target %q)", path, resolved)
		}
		return resolved, nil
	}
	// Path does not exist yet (e.g. a new file). The canonical ancestor was
	// already verified above — the caller's open/write surfaces the ENOENT.
	return abs, nil
}

// withinLexical reports whether abs is lexically inside any root as the
// caller gave it (absolutized, not symlink-resolved). This is the form the
// caller's own path is in on macOS (/var/... instead of /private/var/...).
func (s *SandboxedFS) withinLexical(abs string) bool {
	for _, root := range s.lexicalRoots {
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
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
		execErr := <-done // observe the interrupted execution's outcome
		if execErr != nil {
			return fmt.Errorf("wasm: execution exceeded %s and failed: %w", e.limits.MaxExecutionTime, execErr)
		}
		return fmt.Errorf("wasm: execution interrupted after %s", e.limits.MaxExecutionTime)
	case <-ctx.Done():
		e.Interrupt()
		cancel()
		_ = <-done // the caller's ctx error supersedes the execution error
		return ctx.Err()
	}
}
