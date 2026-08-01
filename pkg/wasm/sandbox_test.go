package wasm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSandboxedFS_Resolve_AllowsInsideRoot verifies paths within the root pass.
func TestSandboxedFS_Resolve_AllowsInsideRoot(t *testing.T) {
	root := t.TempDir()
	sb, err := NewSandboxedFS([]string{root}, false)
	require.NoError(t, err)

	resolved, err := sb.Resolve(filepath.Join(root, "sub", "file.txt"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(resolved, root))
}

// TestSandboxedFS_Resolve_RejectsOutsideRoot verifies paths outside the root fail.
func TestSandboxedFS_Resolve_RejectsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	sb, err := NewSandboxedFS([]string{root}, false)
	require.NoError(t, err)

	_, err = sb.Resolve("/etc/passwd")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside sandbox")
}

// TestSandboxedFS_Resolve_SymlinkEscape verifies symlinks escaping the root fail.
func TestSandboxedFS_Resolve_SymlinkEscape(t *testing.T) {
	root := t.TempDir()

	// Create a symlink inside the root that points outside it.
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	require.NoError(t, os.Symlink(outside, link))

	sb, err := NewSandboxedFS([]string{root}, false)
	require.NoError(t, err)

	_, err = sb.Resolve(filepath.Join(link, "secret.txt"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes sandbox")
}

// TestSandboxedFS_Resolve_SymlinkWithinRoot verifies symlinks staying inside pass.
func TestSandboxedFS_Resolve_SymlinkWithinRoot(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	require.NoError(t, os.MkdirAll(realDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(realDir, "a.txt"), []byte("x"), 0644))

	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(realDir, link))

	sb, err := NewSandboxedFS([]string{root}, false)
	require.NoError(t, err)

	resolved, err := sb.Resolve(filepath.Join(link, "a.txt"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(resolved, realDir))
}

// TestSandboxedFS_New_NoRoots verifies construction fails without roots.
func TestSandboxedFS_New_NoRoots(t *testing.T) {
	_, err := NewSandboxedFS(nil, false)
	require.Error(t, err)
}

// TestSandboxedFS_AllowWrite verifies the write flag.
func TestSandboxedFS_AllowWrite(t *testing.T) {
	root := t.TempDir()
	ro, err := NewSandboxedFS([]string{root}, false)
	require.NoError(t, err)
	assert.False(t, ro.AllowWrite())

	rw, err := NewSandboxedFS([]string{root}, true)
	require.NoError(t, err)
	assert.True(t, rw.AllowWrite())
}

// TestExecutionSandbox_Limits verifies the limits are exposed.
func TestExecutionSandbox_Limits(t *testing.T) {
	lim := DefaultLimits()
	sb, err := NewExecutionSandbox(lim)
	require.NoError(t, err)
	assert.Equal(t, lim, sb.Limits())
}

// TestExecutionSandbox_CheckMemory verifies the memory bound.
func TestExecutionSandbox_CheckMemory(t *testing.T) {
	sb, err := NewExecutionSandbox(DefaultLimits())
	require.NoError(t, err)

	require.NoError(t, sb.CheckMemory(1024))
	require.Error(t, sb.CheckMemory(1024*1024*1024))
}

// TestExecutionSandbox_CheckTimeout verifies the time bound.
func TestExecutionSandbox_CheckTimeout(t *testing.T) {
	lim := DefaultLimits()
	lim.MaxExecutionTime = 50 * time.Millisecond
	sb, err := NewExecutionSandbox(lim)
	require.NoError(t, err)

	sb.Start()
	require.NoError(t, sb.CheckTimeout())
	time.Sleep(80 * time.Millisecond)
	require.Error(t, sb.CheckTimeout())
}

// TestExecutionSandbox_Interrupt verifies epoch interruption.
func TestExecutionSandbox_Interrupt(t *testing.T) {
	sb, err := NewExecutionSandbox(DefaultLimits())
	require.NoError(t, err)

	sb.Start()
	require.Equal(t, int64(0), sb.Epoch())
	sb.Interrupt()
	require.Equal(t, int64(1), sb.Epoch())
}

// TestExecutionSandbox_RunWithTimeout verifies a runaway fn is interrupted.
func TestExecutionSandbox_RunWithTimeout(t *testing.T) {
	lim := DefaultLimits()
	lim.MaxExecutionTime = 100 * time.Millisecond
	sb, err := NewExecutionSandbox(lim)
	require.NoError(t, err)

	start := time.Now()
	err = sb.RunWithTimeout(context.Background(), func(ctx context.Context) error {
		<-ctx.Done() // simulate guest code respecting cancellation
		return ctx.Err()
	})
	require.Error(t, err)
	assert.True(t, time.Since(start) < 2*time.Second)
	assert.True(t, errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "interrupted"))
}

// TestExecutionSandbox_RunWithTimeout_Success verifies fast fns pass through.
func TestExecutionSandbox_RunWithTimeout_Success(t *testing.T) {
	lim := DefaultLimits()
	lim.MaxExecutionTime = 5 * time.Second
	sb, err := NewExecutionSandbox(lim)
	require.NoError(t, err)

	err = sb.RunWithTimeout(context.Background(), func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)
}
