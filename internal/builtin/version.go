package builtin

import (
	"fmt"

	"github.com/wojons/muster/internal/version"
	"github.com/spf13/cobra"
)

// newVersionCommand creates the version command
func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print detailed version information about the OpenAPI CLI.",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			fmt.Println(info.Short())
		},
	}
}
