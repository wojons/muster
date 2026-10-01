package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	binaryPath       string
	mcpBinaryPath    string
	daemonBinaryPath string
	// buildDir is the per-run directory that holds the three binaries built by
	// buildBinary. TestMain removes it once the suite finishes.
	buildDir string
)

// skipLong skips long-running integration tests (subprocess spawning, real
// network/keychain access) when -short is set, keeping `go test -short ./...`
// fast. The full suite still runs via `go test ./test/e2e/` / `make test-e2e`.
func skipLong(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping long-running e2e test in -short mode")
	}
}

func TestMain(m *testing.M) {
	bin, err := buildBinary()
	if err != nil {
		panic(err)
	}
	binaryPath = bin

	code := m.Run()

	// The binaries live in a per-run directory (see buildBinary); remove the
	// whole directory rather than just the CLI binary. os.Exit skips deferred
	// calls, so the cleanup is explicit.
	if buildDir != "" {
		_ = os.RemoveAll(buildDir)
	}
	os.Exit(code)
}

// buildBinary compiles the CLI, MCP server and daemon into a directory unique
// to this test run.
//
// The binaries used to be written to fixed os.TempDir() paths
// (/tmp/muster-e2e, /tmp/muster-e2e-mcp, /tmp/muster-e2e-daemon). On a host
// with a shared /tmp, a stale binary left by another uid is not writable by
// this one, so `go build -o` failed and TestMain panicked with "failed to
// build CLI binary: exit status 1". A per-run directory keeps every run
// independent of every other run and user.
func buildBinary() (string, error) {
	tmpDir, err := os.MkdirTemp("", "muster-e2e-*")
	if err != nil {
		return "", fmt.Errorf("create per-run build dir: %w", err)
	}
	buildDir = tmpDir

	cliBinPath := filepath.Join(tmpDir, "muster-e2e")
	mcpBinPath := filepath.Join(tmpDir, "muster-e2e-mcp")
	daemonBinPath := filepath.Join(tmpDir, "muster-e2e-daemon")
	if runtime.GOOS == "windows" {
		cliBinPath += ".exe"
		mcpBinPath += ".exe"
		daemonBinPath += ".exe"
	}

	// Build CLI
	cliCmd := exec.Command("go", "build", "-o", cliBinPath, "./cmd/openapi-cli")
	cliCmd.Dir = "../.."
	if err := cliCmd.Run(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to build CLI binary: %w", err)
	}

	// Build MCP server
	mcpCmd := exec.Command("go", "build", "-o", mcpBinPath, "./cmd/mcp-server")
	mcpCmd.Dir = "../.."
	if err := mcpCmd.Run(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to build MCP binary: %w", err)
	}

	// Build daemon
	daemonCmd := exec.Command("go", "build", "-o", daemonBinPath, "./cmd/daemon")
	daemonCmd.Dir = "../.."
	if err := daemonCmd.Run(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to build daemon binary: %w", err)
	}

	// Set global variables
	mcpBinaryPath = mcpBinPath
	daemonBinaryPath = daemonBinPath

	return cliBinPath, nil
}

// TestBuildBinary_UsesPerRunDir proves the e2e binaries never land in the
// shared os.TempDir() root: they sit in a per-run muster-e2e-* directory, and
// the naming relationship the suites rely on (binaryPath+"-mcp",
// binaryPath+"-daemon") still holds.
func TestBuildBinary_UsesPerRunDir(t *testing.T) {
	if binaryPath == "" || buildDir == "" {
		t.Fatal("buildBinary did not run in TestMain")
	}
	t.Logf("per-run build dir: %s (CLI: %s, MCP: %s, daemon: %s)", buildDir, binaryPath, mcpBinaryPath, daemonBinaryPath)
	if buildDir == os.TempDir() {
		t.Fatalf("build dir must not be the shared temp dir %q", os.TempDir())
	}
	if filepath.Dir(binaryPath) != buildDir {
		t.Fatalf("CLI binary %q is not inside the per-run build dir %q", binaryPath, buildDir)
	}
	if !strings.HasPrefix(filepath.Base(buildDir), "muster-e2e-") {
		t.Fatalf("build dir %q must be a per-run muster-e2e-* directory", buildDir)
	}
	if filepath.Dir(mcpBinaryPath) != buildDir || filepath.Dir(daemonBinaryPath) != buildDir {
		t.Fatalf("mcp/daemon binaries must live in %q (got %q, %q)", buildDir, mcpBinaryPath, daemonBinaryPath)
	}
	if runtime.GOOS != "windows" {
		if binaryPath+"-mcp" != mcpBinaryPath {
			t.Fatalf("mcp binary path %q must be %q", mcpBinaryPath, binaryPath+"-mcp")
		}
		if binaryPath+"-daemon" != daemonBinaryPath {
			t.Fatalf("daemon binary path %q must be %q", daemonBinaryPath, binaryPath+"-daemon")
		}
	}
	for _, p := range []string{binaryPath, mcpBinaryPath, daemonBinaryPath} {
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			t.Fatalf("expected built binary at %q (stat err: %v)", p, err)
		}
	}
}
