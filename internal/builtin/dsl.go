package builtin

import (
	"fmt"
	"os"

	"github.com/openapi-cli/openapi-cli/pkg/dsl"
	"github.com/spf13/cobra"
)

func newDslCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dsl",
		Short: "Starlark DSL scripting",
		Long:  "Execute Starlark DSL scripts for API workflow automation.",
	}

	cmd.AddCommand(newDslRunCommand())

	return cmd
}

func newDslRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [script-path]",
		Short: "Run a Starlark script",
		Long:  "Load and execute a Starlark (.star) DSL script.",
		Example: `  # Run a simple script
  openapi-cli dsl run script.star

  # Run a workflow with API calls
  openapi-cli dsl run workflow.star`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read script: %w", err)
			}

			engine := dsl.NewEngine(nil)
			script, err := engine.LoadScriptFromString(string(content), path)
			if err != nil {
				return fmt.Errorf("failed to load script: %w", err)
			}

			result, err := engine.Run(script, nil)
			if err != nil {
				return fmt.Errorf("engine error: %w", err)
			}

			if result.ExitCode != 0 {
				return fmt.Errorf("script exited with code %d: %s", result.ExitCode, result.Error)
			}

			if result.Output != nil {
				fmt.Println(result.Output)
			}

			return nil
		},
	}

	return cmd
}
