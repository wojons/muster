package builtin

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/config"
)

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

	cmd.AddCommand(newProfileCommand(cfg))

	return cmd
}

func newProfileCommand(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage configuration profiles",
		Long:  "View, switch, and manage configuration profiles.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List all profiles",
		Run: func(c *cobra.Command, args []string) {
			if len(cfg.Profiles) == 0 {
				fmt.Println("No profiles configured")
				return
			}
			defaultProfile := cfg.DefaultProfile
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
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "show <profile-name>",
		Short: "Show profile details",
		Args:  cobra.ExactArgs(1),
		Run: func(c *cobra.Command, args []string) {
			name := args[0]
			profile, ok := cfg.Profiles[name]
			if !ok {
				fmt.Printf("Profile '%s' not found\n", name)
				return
			}
			fmt.Printf("Name: %s\n", name)
			fmt.Printf("Base URL: %s\n", profile.BaseURL)
			fmt.Printf("Auth Type: %s\n", profile.AuthType)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "switch <profile-name>",
		Short: "Switch default profile",
		Args:  cobra.ExactArgs(1),
		Run: func(c *cobra.Command, args []string) {
			name := args[0]
			if _, ok := cfg.Profiles[name]; !ok {
				fmt.Printf("Profile '%s' not found\n", name)
				return
			}
			cfg.DefaultProfile = name
			fmt.Printf("Switched to profile: %s\n", name)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete <profile-name>",
		Short: "Delete a profile",
		Args:  cobra.ExactArgs(1),
		Run: func(c *cobra.Command, args []string) {
			name := args[0]
			if _, ok := cfg.Profiles[name]; !ok {
				fmt.Printf("Profile '%s' not found\n", name)
				return
			}
			delete(cfg.Profiles, name)
			if cfg.DefaultProfile == name {
				cfg.DefaultProfile = ""
			}
			fmt.Printf("Deleted profile: %s\n", name)
		},
	})

	return cmd
}
