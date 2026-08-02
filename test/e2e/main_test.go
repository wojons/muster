package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var (
	binaryPath       string
	mcpBinaryPath    string
	daemonBinaryPath string
)

func TestMain(m *testing.M) {
	var err error
	binaryPath, err = buildBinary()
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.Remove(binaryPath) }()

	os.Exit(m.Run())
}

func buildBinary() (string, error) {
	tmpDir := os.TempDir()
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
		return "", fmt.Errorf("failed to build CLI binary: %w", err)
	}

	// Build MCP server
	mcpCmd := exec.Command("go", "build", "-o", mcpBinPath, "./cmd/mcp-server")
	mcpCmd.Dir = "../.."
	if err := mcpCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to build MCP binary: %w", err)
	}

	// Build daemon
	daemonCmd := exec.Command("go", "build", "-o", daemonBinPath, "./cmd/daemon")
	daemonCmd.Dir = "../.."
	if err := daemonCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to build daemon binary: %w", err)
	}

	// Set global variables
	mcpBinaryPath = mcpBinPath
	daemonBinaryPath = daemonBinPath

	return cliBinPath, nil
}
