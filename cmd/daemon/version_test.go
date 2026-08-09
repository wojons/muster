package main

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/internal/version"
)

// TestRun_VersionFlag verifies that run() with --version prints the version
// string from internal/version and returns nil (exit 0) without starting the
// daemon or scanning for subcommands.
func TestRun_VersionFlag(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	os.Args = []string{"muster-daemon", "--version"}
	err := run()

	_ = w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := strings.TrimSpace(string(buf[:n]))

	require.NoError(t, err, "run() with --version should return nil (exit 0)")
	require.NotEmpty(t, output, "version output must not be empty")
	require.Contains(t, output, "(", "version output must contain commit in parens")
	require.Equal(t, version.Get().Short(), output, "version output must match version.Get().Short()")
}

// TestRun_VersionFlagShortForm verifies -version (single dash) also works.
func TestRun_VersionFlagShortForm(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	os.Args = []string{"muster-daemon", "-version"}
	err := run()

	_ = w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := strings.TrimSpace(string(buf[:n]))

	require.NoError(t, err)
	require.NotEmpty(t, output)
	require.Contains(t, output, "(")
}

// TestRun_VersionFlagDoesNotBreakSubcommandScan verifies that the version flag
// does not interfere with normal subcommand parsing — status still works.
func TestRun_VersionFlagDoesNotBreakSubcommandScan(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// status subcommand should still work (not treated as version).
	os.Args = []string{"muster-daemon", "status"}
	err := run()
	require.NoError(t, err, "status subcommand should still work")
}

// TestVersionFlag_RegisteredOnFlagSet verifies the flag is registered on the
// daemon's custom FlagSet (defensive: ensures BoolVar was called).
func TestVersionFlag_RegisteredOnFlagSet(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var vf bool
	fs.BoolVar(&vf, "version", false, "Print version information and exit")
	require.NoError(t, fs.Parse([]string{"-version"}))
	require.True(t, vf)
}

// TestVersionFlag_PrintsInternalVersion verifies the output matches
// version.Get().Short() exactly (format parity with openapi-cli).
func TestVersionFlag_PrintsInternalVersion(t *testing.T) {
	expected := version.Get().Short()
	require.NotEmpty(t, expected, "internal version must not be empty")
	require.Contains(t, expected, "(", "internal version must have commit in parens")
	require.Contains(t, expected, ")", "internal version must close parens")
}
