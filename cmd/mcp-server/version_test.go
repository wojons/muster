package main

import (
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/internal/version"
)

// TestVersionFlag_PrintsVersion verifies that when -version is set, the version
// string from internal/version is printed and the early-return path fires (the
// MCP server is never started, so the test does not hang on stdin).
func TestVersionFlag_PrintsVersion(t *testing.T) {
	// Use a fresh FlagSet so we don't interfere with the global flag state.
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	vf := fs.Bool("version", false, "Print version information and exit")
	require.NoError(t, fs.Parse([]string{"-version"}))

	require.True(t, *vf, "version flag should be true after -version")

	// Mirror the exact logic in main(): fmt.Println(version.Get().Short()).
	output := version.Get().Short()
	require.NotEmpty(t, output, "version.Get().Short() must not be empty")
	require.Contains(t, output, "(", "version output must contain commit in parens")
	require.Contains(t, output, ")", "version output must close parens")
}

// TestVersionFlag_DefaultFalse verifies the flag defaults to false.
func TestVersionFlag_DefaultFalse(t *testing.T) {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	vf := fs.Bool("version", false, "Print version information and exit")
	require.NoError(t, fs.Parse([]string{}))
	require.False(t, *vf, "version flag should default to false")
}

// TestVersionFlag_MainEarlyReturn calls main() with -version set and captures
// stdout to verify the early-return path prints the version and does not block.
func TestVersionFlag_MainEarlyReturn(t *testing.T) {
	// Reset the global flag set and re-register all flags so main() sees -version.
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	specPathFlag = flag.String("spec", "", "Path to OpenAPI specification file")
	baseURLFlag = flag.String("base-url", "", "Base URL for API operations")
	logLevelFlag = flag.String("log-level", "info", "Log level")
	logOutputFlag = flag.String("log-output", "stderr", "Log output destination")
	auditFlag = flag.Bool("audit", false, "Enable audit logging")
	wasmPluginFlag = flag.String("wasm-plugin", "", "Path to WASM plugin")
	versionFlag = flag.Bool("version", false, "Print version information and exit")

	if err := flag.CommandLine.Parse([]string{"-version"}); err != nil {
		t.Fatalf("flag parse: %v", err)
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// main() returns early when *versionFlag is true (no os.Exit, no stdin read).
	done := make(chan struct{})
	go func() {
		defer close(done)
		main()
	}()

	<-done
	_ = w.Close()
	os.Stdout = oldStdout

	buf, err := io.ReadAll(r)
	require.NoError(t, err)
	output := strings.TrimSpace(string(buf))
	expected := version.Get().Short()
	require.Equal(t, expected, output, "main() with -version must print version.Get().Short()")
	require.Contains(t, output, "(", "main() with -version must print commit in parens")
	require.NotEmpty(t, output, "main() with -version must print non-empty output")
}
