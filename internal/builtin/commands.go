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

func newCompletionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
		Long:  "Generate shell completion scripts for bash, zsh, fish, or powershell.",
	}
}

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize OpenAPI CLI configuration",
	}
}

func newAuthCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication credentials",
	}
}

func newInventoryCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "inventory",
		Short: "Manage spec inventories",
	}
}

func newModulesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "modules",
		Short: "Manage WASM modules",
	}
}
