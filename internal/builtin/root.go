package builtin

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/version"
	"github.com/wojons/muster/pkg/cmdstore"
	"github.com/wojons/muster/pkg/config"
	"github.com/spf13/cobra"
)

// NewRootCommand creates the root cobra command.
func NewRootCommand(cfg *config.Config) *cobra.Command {
	return newRootCommand(cfg, nil)
}

// NewRootCommandWithAudit creates the root cobra command with audit logging wired in.
// Pass nil for auditHook to disable audit logging.
func NewRootCommandWithAudit(cfg *config.Config, auditHook *audit.Hook) *cobra.Command {
	return newRootCommand(cfg, auditHook)
}

// newRootCommand is the shared implementation for both constructors.
func newRootCommand(cfg *config.Config, auditHook *audit.Hook) *cobra.Command {
	root := &cobra.Command{
		Use:   "openapi-cli",
		Short: "OpenAPI-powered CLI and MCP server",
		Long: `A unified CLI tool that generates commands from OpenAPI specifications
and serves them via MCP protocol.`,
		Version: version.Get().Short(),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if auditHook != nil {
				auditHook.LogCommand(cmd.Context(), cmd.CommandPath(), cliActorID(), true, nil)
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			// This is called on success. Errors before command execution
			// (e.g. arg parsing) are captured by ExecuteContext returning an error.
			if auditHook != nil {
				auditHook.LogCommand(cmd.Context(), cmd.CommandPath(), cliActorID(), true, nil)
			}
		},
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
	root.AddCommand(newDiscoverCommand())
	root.AddCommand(newGenerateCommand(cfg))
	root.AddCommand(newRequestCommand(cfg))
	root.AddCommand(newDaemonCommand())
	// root.AddCommand(newDslCommand()) // TODO: implement DSL command

	// Load and add persisted commands from storage
	loadPersistedCommands(root, cfg)

	return root
}

// cliActorID returns the actor identifier for CLI audit events.
func cliActorID() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("LOGNAME"); user != "" {
		return user
	}
	return "unknown"
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
