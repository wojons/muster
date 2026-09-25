package builtin

import (
	"fmt"
	"sort"
	"strings"

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
		Long:  "Create, view, switch, and manage configuration profiles.",
	}

	cmd.AddCommand(newProfileCreateCommand(cfg))

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

// profileSetFlag is the repeatable flag that prefills profile keys on create.
const profileSetFlag = "set"

// newProfileCreateCommand builds `config profile create <name> [--set key=value ...]`.
//
// The profile is written through the same persistence path `config profile
// switch` uses (config.WriteProfile, a read-modify-write that preserves every
// other key in the config file, including the persisted default_profile), so
// creating a profile never disturbs the active-profile state or any other
// setting.
func newProfileCreateCommand(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <profile-name> [--set key=value ...]",
		Short: "Create a profile (optionally prefilling keys with --set)",
		Long: fmt.Sprintf(`Create a named configuration profile.

The profile is written to the 'profiles:' block of your config file. Keys are
prefilled with repeatable --set dot-path=value flags and are stored as strings:

  %s

Creating a profile does not make it active; use 'config profile switch' for
that. Profile names are case-insensitive and stored lowercased, matching the
configuration loader.`, strings.Join(config.ProfileKeyGroups(), "\n  ")),
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if cfg == nil {
				return fmt.Errorf("config not initialized")
			}
			name, err := config.NormalizeProfileName(args[0])
			if err != nil {
				return err
			}
			if _, exists := cfg.Profiles[name]; exists {
				return fmt.Errorf("profile %q already exists — use 'config profile show %s' to inspect it", name, name)
			}
			pairs, err := c.Flags().GetStringArray(profileSetFlag)
			if err != nil {
				return err
			}
			values, err := config.ParseProfileSetPairs(pairs)
			if err != nil {
				return err
			}
			if err := config.WriteProfile(cfg, name, values); err != nil {
				return fmt.Errorf("persist profile: %w", err)
			}
			// Re-read the file so this process sees exactly what the next one
			// will load, rather than a hand-mirrored copy that drifts.
			if err := config.ReloadProfiles(cfg); err != nil {
				return fmt.Errorf("profile %q written but not reloaded: %w", name, err)
			}
			fmt.Printf("Created profile: %s\n", name)
			if unknown := config.UnknownProfileSetKeys(values); len(unknown) > 0 {
				fmt.Printf("Note: %s not read by the configuration loader and will have no effect; accepted keys are %s\n",
					strings.Join(unknown, ", "), strings.Join(config.ProfileKeyGroups(), ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringArray(profileSetFlag, nil,
		"Prefill a profile key: dot-path=value, repeatable (e.g. --set base_url=https://api.example.com --set default_flags.tier=pro)")
	return cmd
}
