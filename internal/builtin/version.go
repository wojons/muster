package builtin

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/internal/version"
)

// newVersionCommand creates the version command
func newVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print detailed version information about the OpenAPI CLI.",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Get()
			outputJSON, _ := cmd.Flags().GetBool("output")
			if outputJSON {
				data, _ := json.Marshal(info)
				fmt.Println(string(data))
				return
			}
			fmt.Println(info.Short())
		},
	}
	cmd.Flags().Bool("output", false, "Output in JSON format")
	return cmd
}
