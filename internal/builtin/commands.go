package builtin

import (
	"github.com/spf13/cobra"
)

func newHelpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "help",
		Short: "Show help information",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Parent().Help()
		},
	}
}

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize OpenAPI CLI configuration",
	}
}

func newModulesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "modules",
		Short: "Manage WASM modules",
	}
}
