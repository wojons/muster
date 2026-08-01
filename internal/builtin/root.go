package builtin

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/version"
	"github.com/wojons/muster/pkg/cmdstore"
	"github.com/wojons/muster/pkg/config"
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

// persistedCommands is the set of cobra commands added to the root at
// construction time from on-disk storage. We keep track of them so we
// can hide them for meta-commands (help, version, completion, __complete)
// where they would otherwise pollute the output.
var persistedCommands []*cobra.Command

// newRootCommand is the shared implementation for both constructors.
func newRootCommand(cfg *config.Config, auditHook *audit.Hook) *cobra.Command {
	root := &cobra.Command{
		Use:   "openapi-cli",
		Short: "OpenAPI-powered CLI and MCP server",
		Long: `A unified CLI tool that generates commands from OpenAPI specifications
and serves them via MCP protocol.`,
		Version: version.Get().Short(),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// When the user invokes a meta-command, hide the
			// dynamically-loaded persisted commands so they don't
			// pollute help/version/completion output. This mirrors
			// the skip in cmd/cli/main.go persistentPreRun so
			// stored commands are not advertised in the same
			// contexts where we skip config initialization.
			if isMetaCommandName(cmd.Name()) {
				hidePersistedCommands(cmd.Root(), true)
			}
			if auditHook != nil {
				auditHook.LogCommand(cmd.Context(), cmd.CommandPath(), cliActorID(), true, nil)
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			// Restore visibility so a follow-up invocation (e.g.
			// completion -> generated command) sees them again.
			hidePersistedCommands(cmd.Root(), false)
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
	// Note: "help" is intentionally NOT added here. Cobra auto-registers
	// its own default "help" command; adding a second one previously
	// caused two "help" entries in the Available Commands list.
	root.AddCommand(newCompletionCommand())
	root.AddCommand(newInitCommand())
	root.AddCommand(newAuthCommand(cfg))
	root.AddCommand(newInventoryCommand(cfg))
	root.AddCommand(newModulesCommand())
	root.AddCommand(newDiscoverCommand())
	root.AddCommand(newGenerateCommand(cfg))
	root.AddCommand(newRequestCommand(cfg))
	root.AddCommand(newDaemonCommand())
	root.AddCommand(newDslCommand())
	root.AddCommand(newWebhookCommand(cfg))
	root.AddCommand(newTenantCommand(cfg))
	root.AddCommand(newAuditCommand())

	// Load and add persisted commands from storage
	loadPersistedCommands(root, cfg)

	return root
}

// isMetaCommandName reports whether the named command is a meta-command
// for which persisted (on-disk generated) commands should be hidden.
// Mirrors the skip in cmd/cli/main.go persistentPreRun.
func isMetaCommandName(name string) bool {
	switch name {
	case "help", "version", "completion", "__complete":
		return true
	}
	return false
}

// hidePersistedCommands toggles the Hidden flag on every command added
// by loadPersistedCommands. We don't RemoveCommand because the
// persistent pre-run / post-run pair needs to round-trip visibility.
func hidePersistedCommands(root *cobra.Command, hidden bool) {
	for _, cmd := range persistedCommands {
		cmd.Hidden = hidden
	}
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

// loadPersistedCommands loads previously generated commands from storage.
// The on-disk path is taken from cfg.PersistedCommandsPath when set so
// tests can redirect reads to a temp directory; otherwise the default
// $XDG_DATA_HOME/openapi-cli/commands.json is used.
//
// Each loaded command is tracked in the package-level persistedCommands
// slice so newRootCommand can hide it when a meta-command (help, version,
// completion, __complete) is being executed.
func loadPersistedCommands(root *cobra.Command, cfg *config.Config) {
	dataPath := resolvePersistedCommandsPath(cfg)

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
			persistedCommands = append(persistedCommands, cmd)
		}
		if cfg != nil && cfg.Verbose {
			fmt.Fprintf(os.Stderr, "Loaded %d commands from storage (base URL: %s)\n", len(commands), baseURL)
		}
	}
}
