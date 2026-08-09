package builtin

import (
	"bytes"
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

// TestModulesCommand_RunInvalidModule verifies error on non-wasm bytes.
func TestModulesCommand_RunInvalidModule(t *testing.T) {
	badPath := filepath.Join(t.TempDir(), "bad.wasm")
	require.NoError(t, os.WriteFile(badPath, []byte("this is not a wasm binary"), 0o644))

	cmd := newModulesCommand()
	cmd.SetArgs([]string{"run", badPath})
	require.Error(t, cmd.Execute())
}

// TestModulesCommand_RunNonIntegerArg verifies error on non-numeric args.
func TestModulesCommand_RunNonIntegerArg(t *testing.T) {
	wasmPath := filepath.Join("..", "..", "testdata", "wasm-modules", "add.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		t.Skip("testdata/wasm-modules/add.wasm not present")
	}

	cmd := newModulesCommand()
	cmd.SetArgs([]string{"run", "--fn", "add", wasmPath, "twenty"})
	require.Error(t, cmd.Execute())
}

// TestModulesCommand_RunHelloWasm executes the bundled hello.wasm.
func TestModulesCommand_RunHelloWasm(t *testing.T) {
	wasmPath := filepath.Join("..", "..", "testdata", "wasm-modules", "hello.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		t.Skip("testdata/wasm-modules/hello.wasm not present")
	}

	var buf bytes.Buffer
	cmd := newModulesCommand()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"run", wasmPath})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "Output: [42]")
}

// TestModulesCommand_RunArgsPassthrough verifies positional args are
// passed as parameters to the exported function (add(20, 22) -> 42).
func TestModulesCommand_RunArgsPassthrough(t *testing.T) {
	wasmPath := filepath.Join("..", "..", "testdata", "wasm-modules", "add.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		t.Skip("testdata/wasm-modules/add.wasm not present")
	}

	var buf bytes.Buffer
	cmd := newModulesCommand()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"run", "--fn", "add", wasmPath, "20", "22"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), "Output: [42]")
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

// TestModulesCommand_RunDemoHelloWasmNoFn verifies GAP-014: running the
// demo hello.wasm (which exports only "hello", NOT "_start") without
// --fn auto-selects the sole export instead of failing.
func TestModulesCommand_RunDemoHelloWasmNoFn(t *testing.T) {
	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		t.Skip("demo/wasm-modules/hello.wasm not present")
	}

	var buf bytes.Buffer
	cmd := newModulesCommand()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"run", wasmPath}) // no --fn → auto-select sole export "hello"
	require.NoError(t, cmd.Execute(), "single-export module should auto-select without --fn")
	assert.Contains(t, buf.String(), "function: hello", "should report the auto-selected function")
	assert.Contains(t, buf.String(), "Output:", "should print function output")
}

// TestModulesCommand_RunDemoHelloWasmExplicitFn verifies GAP-014: an
// explicit --fn hello on the demo module still works (strict behavior).
func TestModulesCommand_RunDemoHelloWasmExplicitFn(t *testing.T) {
	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		t.Skip("demo/wasm-modules/hello.wasm not present")
	}

	var buf bytes.Buffer
	cmd := newModulesCommand()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"run", "--fn", "hello", wasmPath})
	require.NoError(t, cmd.Execute(), "explicit --fn hello should succeed")
	assert.Contains(t, buf.String(), "function: hello")
}

// TestModulesCommand_RunDemoHelloWasmMissingFnHint verifies GAP-014: when
// --fn is explicit but names a non-existent export, the error lists the
// available exports and hints at --fn.
func TestModulesCommand_RunDemoHelloWasmMissingFnHint(t *testing.T) {
	wasmPath := filepath.Join("..", "..", "demo", "wasm-modules", "hello.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		t.Skip("demo/wasm-modules/hello.wasm not present")
	}

	cmd := newModulesCommand()
	cmd.SetArgs([]string{"run", "--fn", "nonexistent", wasmPath})
	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "available", "error should list available exports")
	assert.Contains(t, err.Error(), "--fn", "error should hint at the --fn flag")
}
