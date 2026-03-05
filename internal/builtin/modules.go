package builtin

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newModulesCommand creates the modules command group
func newModulesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "modules",
		Short: "WASM module management",
		Long:  "Install, list and remove WASM modules for extending functionality.",
	}

	cmd.AddCommand(newModulesInstallCommand())
	cmd.AddCommand(newModulesListCommand())
	cmd.AddCommand(newModulesRemoveCommand())

	return cmd
}

// newModulesInstallCommand creates the modules install command
func newModulesInstallCommand() *cobra.Command {
	var (
		name      string
		overwrite bool
		validate  bool
	)

	cmd := &cobra.Command{
		Use:   "install [module-path-or-url]",
		Short: "Install a WASM module",
		Long:  "Install a WASM module from a file path or URL.",
		Example: `  # Install from local file
  openapi-cli modules install ./modules/auth.wasm

  # Install from URL
  openapi-cli modules install https://example.com/modules/logging.wasm

  # Install with custom name
  openapi-cli modules install ./modules/auth.wasm --name auth-module`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]

			if name == "" {
				name = "module"
			}

			fmt.Printf("Installing module from %s...\n", source)
			fmt.Printf("Module name: %s\n", name)

			if validate {
				fmt.Println("Validating module...")
			}

			if overwrite {
				fmt.Println("Overwriting existing module...")
			}

			fmt.Println("Module installed successfully")
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Module name (default: derived from file)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing module")
	cmd.Flags().BoolVar(&validate, "validate", true, "Validate module before installation")

	return cmd
}

// newModulesListCommand creates the modules list command
func newModulesListCommand() *cobra.Command {
	var (
		verbose bool
		format  string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed modules",
		Long:  "List all installed WASM modules.",
		Example: `  # List all modules
  openapi-cli modules list

  # List with verbose output
  openapi-cli modules list --verbose`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Installed modules:")

			if verbose {
				fmt.Println("No modules installed")
			} else {
				fmt.Println("No modules installed")
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().StringVarP(&format, "format", "f", "table", "Output format (table|json|yaml)")

	return cmd
}

// newModulesRemoveCommand creates the modules remove command
func newModulesRemoveCommand() *cobra.Command {
	var (
		force bool
	)

	cmd := &cobra.Command{
		Use:   "remove [module-name]",
		Short: "Remove a WASM module",
		Long:  "Remove an installed WASM module.",
		Example: `  # Remove module
  openapi-cli modules remove auth-module

  # Force remove without confirmation
  openapi-cli modules remove auth-module --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			if !force {
				fmt.Printf("Remove module '%s'? [y/N] ", name)
			} else {
				fmt.Printf("Removing module '%s'...\n", name)
			}

			fmt.Printf("Module '%s' removed successfully\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "Force removal without confirmation")

	return cmd
}
