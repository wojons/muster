package wasm

// Simulates the macOS /var -> /private/var situation: the caller passes a
// temp dir reached through a symlink (e.g. /var/folders/... which is really
// /private/var/folders/...). The sandbox must canonicalize roots and input
// so resolution succeeds inside and escapes still fail.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSandboxedFS_SymlinkedRootMacOSSemantics(t *testing.T) {
	real := t.TempDir()
	// Create a symlink alias like /var -> /private/var on macOS.
	aliasDir := filepath.Join(t.TempDir(), "var")
	require.NoError(t, os.Symlink(real, aliasDir))

	// Caller builds root THROUGH the symlink (like t.TempDir() on macOS).
	root := filepath.Join(aliasDir, "sandbox")
	require.NoError(t, os.MkdirAll(filepath.Join(real, "sandbox", "real"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(real, "sandbox", "real", "a.txt"), []byte("x"), 0644))

	sb, err := NewSandboxedFS([]string{root}, false)
	require.NoError(t, err)

	// 1. Path inside through the symlinked root resolves.
	resolved, err := sb.Resolve(filepath.Join(root, "real", "a.txt"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(resolved, filepath.Join(real, "sandbox")), "resolved=%s", resolved)

	// 2. A symlink INSIDE the sandbox pointing OUTSIDE still escapes.
	outside := filepath.Join(real, "outside")
	require.NoError(t, os.MkdirAll(outside, 0755))
	link := filepath.Join(root, "escape")
	require.NoError(t, os.Symlink(outside, link))
	_, err = sb.Resolve(filepath.Join(link, "secret.txt"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "escapes sandbox")

	// 3. A symlink inside pointing INSIDE passes.
	inlink := filepath.Join(root, "inlink")
	require.NoError(t, os.Symlink(filepath.Join(root, "real"), inlink))
	_, err = sb.Resolve(filepath.Join(inlink, "a.txt"))
	require.NoError(t, err)
}
