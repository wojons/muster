package builtin

import (
	"fmt"

	"github.com/wojons/muster/pkg/config"
	"github.com/spf13/cobra"
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
			cm, err := config.NewConfigManager()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			pm := config.NewProfileManager(cm)
			profiles := pm.List()
			if len(profiles) == 0 {
				fmt.Println("No profiles configured")
				return
			}
			defaultProfile := cm.GetDefaultProfile()
			for _, profile := range profiles {
				if profile != nil {
					marker := ""
					name := profile.BaseURL
					if name != "" && defaultProfile == name {
						marker = "*"
					}
					fmt.Printf("%s %s\n", marker, name)
				}
			}
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "show <profile-name>",
		Short: "Show profile details",
		Args:  cobra.ExactArgs(1),
		Run: func(c *cobra.Command, args []string) {
			name := args[0]
			cm, err := config.NewConfigManager()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			pm := config.NewProfileManager(cm)
			profile, err := pm.Get(name)
			if err != nil {
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
			cm, err := config.NewConfigManager()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			pm := config.NewProfileManager(cm)
			if !pm.Exists(name) {
				fmt.Printf("Profile '%s' not found\n", name)
				return
			}
			if err := pm.SetDefault(name); err != nil {
				fmt.Printf("Error switching profile: %v\n", err)
				return
			}
			fmt.Printf("Switched to profile: %s\n", name)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "delete <profile-name>",
		Short: "Delete a profile",
		Args:  cobra.ExactArgs(1),
		Run: func(c *cobra.Command, args []string) {
			name := args[0]
			cm, err := config.NewConfigManager()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			pm := config.NewProfileManager(cm)
			if !pm.Exists(name) {
				fmt.Printf("Profile '%s' not found\n", name)
				return
			}
			if err := pm.Delete(name); err != nil {
				fmt.Printf("Error deleting profile: %v\n", err)
				return
			}
			fmt.Printf("Deleted profile: %s\n", name)
		},
	})

	return cmd
}
