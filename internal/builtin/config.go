package builtin

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/config"
)

func newConfigCommand(cfg *config.Config, root *cobra.Command) *cobra.Command {
	if root != nil {
		// Defensive, idempotent: guarantee the global --profile flag exists
		// on the root this command is attached to, so `config show
		// --profile x` resolves even when the constructor is used directly.
		config.BindProfileFlag(root)
	}
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
		Long:  "View and modify OpenAPI CLI configuration.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("Config Dir: %s\n", cfg.ConfigDir)
			fmt.Printf("Cache Dir: %s\n", cfg.CacheDir)
			fmt.Printf("Data Dir: %s\n", cfg.DataDir)
			fmt.Printf("Version: %s\n", cfg.Version)
			fmt.Printf("Default Profile: %s\n", cfg.DefaultProfile)
			// The active profile follows the documented resolution order
			// (--profile flag > MUSTER_PROFILE > persisted default_profile),
			// which can differ from the raw persisted value above.
			sel := config.ResolveProfile(cmd, cfg)
			if sel.Name != "" {
				fmt.Printf("Active Profile: %s (source: %s)\n", sel.Name, sel.Source)
			}
			return nil
		},
	})

	cmd.AddCommand(newProfileCommand(cfg, root))

	return cmd
}

func newProfileCommand(cfg *config.Config, root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage configuration profiles",
		Long:  "View, switch, and manage configuration profiles.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all profiles",
		RunE: func(c *cobra.Command, args []string) error {
			if len(cfg.Profiles) == 0 {
				fmt.Println("No profiles configured")
				return nil
			}
			// The starred marker follows the active-profile resolution order,
			// not just the raw persisted value: --profile and MUSTER_PROFILE
			// override what the config file recorded.
			defaultProfile := config.ResolveProfile(c, cfg).Name
			names := make([]string, 0, len(cfg.Profiles))
			for name := range cfg.Profiles {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				marker := " "
				if name == defaultProfile {
					marker = "*"
				}
				fmt.Printf("%s %s\n", marker, name)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "show <profile-name>",
		Short: "Show profile details",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			profile, ok := cfg.Profiles[name]
			if !ok {
				fmt.Printf("Profile '%s' not found\n", name)
				return nil
			}
			fmt.Printf("Name: %s\n", name)
			fmt.Printf("Base URL: %s\n", profile.BaseURL)
			fmt.Printf("Auth Type: %s\n", profile.AuthType)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "switch <profile-name>",
		Short: "Switch the default profile (persists to the user config file)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			if _, ok := cfg.Profiles[name]; !ok {
				fmt.Printf("Profile '%s' not found\n", name)
				return nil
			}
			if err := config.WriteDefaultProfile(cfg, name); err != nil {
				return fmt.Errorf("persist default profile: %w", err)
			}
			cfg.DefaultProfile = name
			fmt.Printf("Switched to profile: %s\n", name)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete <profile-name>",
		Short: "Delete a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			name := args[0]
			if _, ok := cfg.Profiles[name]; !ok {
				fmt.Printf("Profile '%s' not found\n", name)
				return nil
			}
			delete(cfg.Profiles, name)
			if cfg.DefaultProfile == name {
				cfg.DefaultProfile = ""
				// Keep the persisted selection consistent with the deletion:
				// a stale default_profile in the config file would name a
				// profile that no longer exists on the next run.
				if err := config.WriteDefaultProfile(cfg, ""); err != nil {
					return fmt.Errorf("clear persisted default profile: %w", err)
				}
			}
			fmt.Printf("Deleted profile: %s\n", name)
			return nil
		},
	})

	return cmd
}
