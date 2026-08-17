package builtin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// defaultConfigRelDir is the user config directory (relative to the home
// dir) where `init` writes the default config file. It mirrors the
// user-config source in pkg/config.ConfigSources (priority 2:
// $HOME/.config/muster/config.yaml) so the file the loader actually reads
// is the one init creates on every platform.
const defaultConfigRelDir = ".config/muster"

// defaultConfigYAML is the starter configuration written by `init`. Keep
// the keys aligned with the mapstructure tags in pkg/config/types.go.
const defaultConfigYAML = `# Muster (OpenAPI CLI) default configuration.
# Created by: openapi-cli init
version: "1.0.0"
host: localhost
log_level: info
log_format: text
max_retries: 3
`

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize OpenAPI CLI configuration",
		Long: `Write a default configuration file to the user config directory
(~/.config/muster/config.yaml on Linux) if one does not exist yet.

Safe to run repeatedly: an existing config file is never overwritten.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("home dir: %w", err)
			}
			dir := filepath.Join(home, defaultConfigRelDir)
			path := filepath.Join(dir, "config.yaml")

			if _, err := os.Stat(path); err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Configuration already exists at %s (unchanged)\n", path)
				return nil
			}

			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("create config dir: %w", err)
			}
			if err := os.WriteFile(path, []byte(defaultConfigYAML), 0o600); err != nil {
				return fmt.Errorf("write config: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Created default configuration at %s\n", path)
			return nil
		},
	}
}
