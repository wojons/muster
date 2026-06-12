package builtin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/openapi-cli/openapi-cli/internal/version"
	"github.com/openapi-cli/openapi-cli/pkg/cmdstore"
	"github.com/openapi-cli/openapi-cli/pkg/config"
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root cobra command
func NewRootCommand(cfg *config.Config) *cobra.Command {
	root := &cobra.Command{
		Use:   "openapi-cli",
		Short: "OpenAPI-powered CLI and MCP server",
		Long: `A unified CLI tool that generates commands from OpenAPI specifications
and serves them via MCP protocol.`,
		Version: version.Get().Short(),
	}

	// Add built-in commands
	root.AddCommand(newVersionCommand())
	root.AddCommand(newConfigCommand(cfg))
	root.AddCommand(newHelpCommand())
	root.AddCommand(newCompletionCommand())
	root.AddCommand(newInitCommand())
	root.AddCommand(newAuthCommand())
	root.AddCommand(newInventoryCommand())
	root.AddCommand(newModulesCommand())
	root.AddCommand(newGenerateCommand(cfg))
	root.AddCommand(newRequestCommand())
	root.AddCommand(newDaemonCommand())
	root.AddCommand(newDslCommand())

	// Load and add persisted commands from storage
	loadPersistedCommands(root, cfg)

	return root
}

// loadPersistedCommands loads previously generated commands from storage
func loadPersistedCommands(root *cobra.Command, cfg *config.Config) {
	// Get command storage path
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		dataDir = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	dataPath := filepath.Join(dataDir, "openapi-cli", "commands.json")

	// Create command store and loader
	store := cmdstore.NewCommandStore(dataPath)
	loader := cmdstore.NewLoader(store, "")

	// Check if we have stored commands
	if !store.HasData() {
		return
	}

	// Load the base URL from storage
	baseURL, err := loader.GetBaseURL()
	if err != nil {
		return
	}

	// Create loader with the baseURL
	loader = cmdstore.NewLoader(store, baseURL)

	// Load commands
	commands, err := loader.LoadCommands()
	if err != nil {
		// Log but don't fail
		fmt.Fprintf(os.Stderr, "Warning: Failed to load stored commands: %v\n", err)
		return
	}

	if len(commands) > 0 {
		for _, cmd := range commands {
			root.AddCommand(cmd)
		}
		fmt.Fprintf(os.Stderr, "Loaded %d commands from storage (base URL: %s)\n", len(commands), baseURL)
	}
}
