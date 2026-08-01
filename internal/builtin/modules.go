package builtin

import (
	"context"
	"fmt"
	"os"
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

func newModulesRunCommand() *cobra.Command {
	var fnName string
	cmd := &cobra.Command{
		Use:   "run <file.wasm>",
		Short: "Load and execute a WASM module",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read wasm file: %w", err)
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
			result, err := runtime.ExecuteWithContext(ctx, data, fnName)
			if err != nil {
				return fmt.Errorf("execute wasm module: %w", err)
			}
			if !result.Success {
				return fmt.Errorf("wasm execution failed (memory: %d bytes)", result.MemoryUsed)
			}
			fmt.Printf("Executed %s (function: %s)\n", path, fnName)
			if result.Output != nil {
				fmt.Printf("Output: %v\n", result.Output)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fnName, "fn", "_start", "Function to call (default _start)")
	return cmd
}
