package builtin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/wasm"
)

// TestModulesCommand_Help verifies the modules command has a run subcommand.
func TestModulesCommand_Help(t *testing.T) {
	cmd := newModulesCommand()

	// Find is path-based; use Commands() to inspect direct children.
	var found *cobra.Command
	for _, c := range cmd.Commands() {
		if c.Name() == "run" {
			found = c
			break
		}
	}
	require.NotNil(t, found, "modules run subcommand should exist")
	assert.Equal(t, "run", found.Name())
}

// TestModulesCommand_RunMissingFile verifies error on nonexistent wasm.
func TestModulesCommand_RunMissingFile(t *testing.T) {
	cmd := newModulesCommand()
	cmd.SetArgs([]string{"run", filepath.Join(t.TempDir(), "nope.wasm")})
	require.Error(t, cmd.Execute())
}

// TestModulesCommand_RunHelloWasm executes the bundled hello.wasm.
func TestModulesCommand_RunHelloWasm(t *testing.T) {
	wasmPath := filepath.Join("..", "..", "testdata", "wasm-modules", "hello.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		t.Skip("testdata/wasm-modules/hello.wasm not present")
	}
	cmd := newModulesCommand()
	cmd.SetArgs([]string{"run", wasmPath})
	require.NoError(t, cmd.Execute())
}

// TestWasmRuntime_ExecuteHelloModule verifies the runtime executes the
// bundled module directly (ac-070 criterion: loads successfully).
func TestWasmRuntime_ExecuteHelloModule(t *testing.T) {
	wasmPath := filepath.Join("..", "..", "testdata", "wasm-modules", "hello.wasm")
	data, err := os.ReadFile(wasmPath)
	require.NoError(t, err)

	runtime := wasm.NewRuntime(nil, wasm.DefaultLimits())
	defer runtime.Close()

	result, err := runtime.Execute(data, "_start")
	require.NoError(t, err)
	assert.True(t, result.Success)
}
