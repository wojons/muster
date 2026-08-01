package builtin

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/wasm"
)

// newModulesCommand creates the WASM module management command with a
// `run` subcommand that loads a .wasm binary and executes a function.
func newModulesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "modules",
		Short: "Manage WASM modules",
		Long: `Manage and execute WebAssembly modules.

Modules run in the wazero sandbox (pure Go, no CGO) with resource
limits enforced by the execution sandbox.`,
	}

	cmd.AddCommand(newModulesRunCommand())

	return cmd
}

// newModulesRunCommand creates the `modules run` subcommand that loads
// a .wasm binary from disk and executes an exported function in the
// wazero sandbox. Positional arguments after the module path are parsed
// as signed 64-bit integers and passed as parameters to the function.
func newModulesRunCommand() *cobra.Command {
	var fnName string
	cmd := &cobra.Command{
		Use:   "run <file.wasm> [args...]",
		Short: "Load and execute a WASM module",
		Long: `Load a .wasm binary and execute an exported function in the wazero sandbox.

Positional arguments after the module path are parsed as signed 64-bit
integers and passed as parameters to the target function in order.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read wasm file: %w", err)
			}

			fnArgs, err := parseWasmArgs(args[1:])
			if err != nil {
				return err
			}

			runtime := wasm.NewRuntime(nil, wasm.DefaultLimits())
			defer runtime.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if fnName == "" {
				fnName = "_start"
			}

			// Load + compile via the module manager so capability gating
			// and limits apply, then execute the requested function.
			result, err := runtime.ExecuteWithContext(ctx, data, fnName, fnArgs...)
			if err != nil {
				return fmt.Errorf("execute wasm module: %w", err)
			}
			if !result.Success {
				return fmt.Errorf("wasm execution failed (memory: %d bytes)", result.MemoryUsed)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Executed %s (function: %s)\n", path, fnName)
			if result.Output != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Output: %v\n", result.Output)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fnName, "fn", "_start", "Function to call (default _start)")
	return cmd
}

// parseWasmArgs converts CLI argument strings to int64 function
// parameters. WASM functions accept numeric arguments only, so a
// non-integer argument is a usage error.
func parseWasmArgs(args []string) ([]interface{}, error) {
	fnArgs := make([]interface{}, 0, len(args))
	for i, a := range args {
		v, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("argument %d (%q) must be an integer: %w", i+1, a, err)
		}
		fnArgs = append(fnArgs, v)
	}
	return fnArgs, nil
}
