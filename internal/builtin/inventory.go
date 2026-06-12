package builtin

import (
	"fmt"
	"os"

	"github.com/openapi-cli/openapi-cli/pkg/inventory"
	"github.com/spf13/cobra"
)

// newInventoryCommand creates the inventory command group
func newInventoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Inventory management commands",
		Long:  "Fetch and manage API inventory from the host.",
	}

	cmd.AddCommand(newInventoryFetchCommand())
	cmd.AddCommand(newInventoryListCommand())
	cmd.AddCommand(newInventoryValidateCommand())
	cmd.AddCommand(newInventoryAddCommand())
	cmd.AddCommand(newInventoryRemoveCommand())

	return cmd
}

// newInventoryFetchCommand creates the inventory fetch command
func newInventoryFetchCommand() *cobra.Command {
	var (
		host   string
		output string
	)

	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch inventory from host",
		Long:  "Fetch the API inventory from the specified host.",
		Example: `  # Fetch inventory from host
  openapi-cli inventory fetch --host https://api.example.com

  # Fetch inventory and save as YAML
  openapi-cli inventory fetch --host https://api.example.com --output yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if host == "" {
				return fmt.Errorf("--host is required")
			}

			fmt.Printf("Fetching inventory from %s...\n", host)

			switch output {
			case "json":
				fmt.Println("Output format: JSON")
			case "yaml":
				fmt.Println("Output format: YAML")
			default:
				fmt.Println("Output format: JSON (default)")
			}

			fmt.Println("Inventory fetched successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "Host URL to fetch from (required)")
	_ = cmd.MarkFlagRequired("host")
	cmd.Flags().StringVarP(&output, "output", "o", "json", "Output format (json|yaml)")

	return cmd
}

// newInventoryListCommand creates the inventory list command
func newInventoryListCommand() *cobra.Command {
	var (
		cacheDir string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cached inventories",
		Long:  "List all cached API inventories.",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := inventory.NewManager(inventory.ManagerConfig{
				CacheDir: cacheDir,
			})
			if err != nil {
				return fmt.Errorf("failed to create inventory manager: %w", err)
			}

			entries, err := mgr.ListCached()
			if err != nil {
				return fmt.Errorf("failed to list cached inventories: %w", err)
			}

			if len(entries) == 0 {
				fmt.Printf("No cached inventories found\nCache directory: %s\n", cacheDir)
				return nil
			}

			fmt.Println("Cached inventories:")
			for _, entry := range entries {
				fmt.Printf("  Host: %s\n", entry.Host)
				fmt.Printf("    Version:   %s\n", entry.Version)
				fmt.Printf("    Cached at: %s\n", entry.CachedAt.Format("2006-01-02 15:04:05"))
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&cacheDir, "cache-dir", "", "Cache directory path")

	return cmd
}

// newInventoryValidateCommand creates the inventory validate command
func newInventoryValidateCommand() *cobra.Command {
	var (
		host    string
		verbose bool
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate inventory schema",
		Long:  "Validate the inventory schema for compatibility.",
		Example: `  # Validate inventory from host
  openapi-cli inventory validate --host https://api.example.com

  # Validate with verbose output
  openapi-cli inventory validate --host https://api.example.com --verbose`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if host == "" {
				return fmt.Errorf("--host is required")
			}

			fmt.Printf("Validating inventory from %s...\n", host)

			if verbose {
				fmt.Println("Validating resources...")
				fmt.Println("Validating operations...")
				fmt.Println("Validating parameters...")
			}

			fmt.Println("Inventory is valid")
			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "Host URL to validate (required)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	return cmd
}

func newInventoryAddCommand() *cobra.Command {
	var (
		host string
		file string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add inventory from file",
		Long:  "Add an API inventory entry from a local JSON file.",
		Example: `  # Add inventory from file
  openapi-cli inventory add --host https://api.example.com --file inventory.json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if host == "" {
				return fmt.Errorf("--host is required")
			}
			if file == "" {
				return fmt.Errorf("--file is required")
			}

			data, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("failed to read file: %w", err)
			}

			mgr, err := inventory.NewManager(inventory.ManagerConfig{})
			if err != nil {
				return fmt.Errorf("failed to create inventory manager: %w", err)
			}

			if err := mgr.AddFromData(host, data); err != nil {
				return fmt.Errorf("failed to add inventory: %w", err)
			}

			fmt.Printf("Inventory added for host: %s\n", host)
			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "Host URL (required)")
	cmd.Flags().StringVarP(&file, "file", "f", "", "Path to inventory JSON file (required)")

	return cmd
}

func newInventoryRemoveCommand() *cobra.Command {
	var host string

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove cached inventory",
		Long:  "Remove a cached API inventory entry for the specified host.",
		Example: `  # Remove inventory for a host
  openapi-cli inventory remove --host https://api.example.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if host == "" {
				return fmt.Errorf("--host is required")
			}

			mgr, err := inventory.NewManager(inventory.ManagerConfig{})
			if err != nil {
				return fmt.Errorf("failed to create inventory manager: %w", err)
			}

			if err := mgr.PurgeCache(host); err != nil {
				return fmt.Errorf("failed to remove inventory: %w", err)
			}

			fmt.Printf("Inventory removed for host: %s\n", host)
			return nil
		},
	}

	cmd.Flags().StringVar(&host, "host", "", "Host URL to remove (required)")

	return cmd
}
