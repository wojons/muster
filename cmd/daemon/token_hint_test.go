package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/daemon"
)

// ─────────────────────────────────────────────────────────────────────────────
// DF-047: every output surface of the daemon used to print
// "Token file: <socket>.token" (start, proxy, the detached-start parent, and
// status). Nothing ever wrote that file — the shipped CLI never installs the
// token middleware, and pkg/daemon's TokenManager.AddToken is called only from
// tests — so the hint sent operators hunting for a credential file that does
// not exist. The prints were removed rather than the file written.
//
// There is no artifact to assert on, so the regression is pinned two ways: the
// shipped sources must not carry the hint string, and the status output must not
// carry it either. The source scan is what covers the proxy and detached-child
// banners, which a black-box unit test cannot reach (both block on a signal).
//
// The limit of the scan is named rather than hidden: it reads the package's
// non-test .go files from disk, so it catches a re-introduced print in this
// package but not a hint printed by some other binary.

// tokenHintNeedle is the forbidden hint, assembled in two parts so that this
// test's own source can never contain the literal it forbids.
var tokenHintNeedle = "Token" + " file:"

// TestDaemonSources_NoTokenFileHint fails if any shipped file in this package
// prints the token-file hint again.
func TestDaemonSources_NoTokenFileHint(t *testing.T) {
	entries, err := os.ReadDir(".")
	require.NoError(t, err, "the test runs from the package directory")

	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", name))
		require.NoError(t, err, "read %s", name)
		scanned++
		assert.NotContains(t, string(data), tokenHintNeedle,
			"%s must not advertise a token file the daemon never creates (DF-047)", name)
	}

	// Non-vacuity: the scan must have seen the daemon sources it claims to cover.
	assert.GreaterOrEqual(t, scanned, 2, "expected to scan the daemon package sources")
}

// TestHandleStatus_NoTokenFileHint pins the status surface. With no PID file
// the handler takes the in-process-controller fallback — the branch that used
// to print the hint — and its output must not mention a token file.
func TestHandleStatus_NoTokenFileHint(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath = filepath.Join(tmpDir, "test-status-token-hint.sock")
	pidFile = filepath.Join(tmpDir, "test-status-token-hint.pid")
	defer func() {
		socketPath = "/tmp/openapi-cli.sock"
		pidFile = ""
	}()

	controller, err := daemon.NewController(daemon.ControllerConfig{SocketPath: socketPath})
	require.NoError(t, err)

	out := captureStdout(t, func() {
		require.NoError(t, handleStatus(controller))
	})

	assert.Contains(t, out, "Daemon Status:", "the status block must still be printed")
	assert.NotContains(t, out, tokenHintNeedle, "status must not advertise a token file (DF-047)")
}

// captureStdout runs fn with os.Stdout redirected into a pipe and returns what
// fn printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	require.NoError(t, w.Close())
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(data)
}
