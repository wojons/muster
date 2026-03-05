package builtin

import (
	"fmt"

	"github.com/openapi-cli/openapi-cli/pkg/config"
	"github.com/spf13/cobra"
)

// newConfigCommand creates the config command group
func newConfigCommand(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  "View and modify OpenAPI CLI configuration.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Config Dir: %s\n", cfg.ConfigDir)
			fmt.Printf("Cache Dir: %s\n", cfg.CacheDir)
			fmt.Printf("Data Dir: %s\n", cfg.DataDir)
			fmt.Printf("Version: %s\n", cfg.Version)
			fmt.Printf("Default Profile: %s\n", cfg.DefaultProfile)
		},
	})

	return cmd
}
