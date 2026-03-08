package builtin

import (
	"github.com/spf13/cobra"
)

// newHelpCommand creates the help command
func newHelpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Long: `Help provides help for any command in the application.
Simply type openapi-cli help [path to command] for full details.`,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Root().Help()
		},
	}
}
