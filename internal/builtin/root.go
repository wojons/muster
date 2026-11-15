package builtin

import (
	"fmt"
	"os"
	"strings"

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

// storeDecodeCount counts construction-time persisted-store loads. It is
// intentionally package-local so tests can prove meta-command invocations do
// not read the store without relying on timing.
var storeDecodeCount int

// reset persisted command state before constructing a new root. Commands are
// owned by their root, so retaining the previous root's slice would make a
// later root hide commands it never added.
func resetPersistedCommandState() {
	persistedCommands = nil
}

// newRootCommand is the shared implementation for both constructors.
func newRootCommand(cfg *config.Config, auditHook *audit.Hook) *cobra.Command {
	resetPersistedCommandState()

	root := &cobra.Command{
		Use:   "openapi-cli",
		Short: "OpenAPI-powered CLI and MCP server",
		Long: `A unified CLI tool that generates commands from OpenAPI specifications
and serves them via MCP protocol.`,
		Version:       version.Get().Short(),
		SilenceErrors: true, // errors are printed exactly once by cmd/openapi-cli/main.go; without this cobra prints them again
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// When the user invokes a meta-command, hide the
			// dynamically-loaded persisted commands so they don't
			// pollute help/version/completion output. This mirrors
			// the skip in cmd/openapi-cli/main.go persistentPreRun so
			// stored commands are not advertised in the same
			// contexts where we skip config initialization.
			//
			// Also hide when --help is used on the root command
			// (e.g. "openapi-cli --help") — the root name is
			// "openapi-cli", not "help", so isMetaCommandName
			// alone doesn't catch it.
			if isMetaCommandName(cmd.Name()) || (cmd == cmd.Root() && helpRequested(cmd)) {
				hidePersistedCommands(cmd.Root(), true)
			}
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			// Restore visibility so a follow-up invocation (e.g.
			// completion -> generated command) sees them again.
			hidePersistedCommands(cmd.Root(), false)
			// This is called on success. Errors before command execution
			// (e.g. arg parsing) are captured by ExecuteContext returning an error.
			if auditHook != nil {
				auditHook.LogCommand(cmd.Context(), cmd.CommandPath(), CliActorID(), true, nil)
			}
		},
	}

	// Global persistent profile selection flag (--profile), documented in
	// docs/guides/configuration.md. Registration is idempotent and the
	// resolved selection is consumed by config/profile-aware commands via
	// config.ResolveProfile (flag > MUSTER_PROFILE > persisted default).
	config.BindProfileFlag(root)

	// Resolve user-defined command aliases (the config file's `aliases:` map)
	// BEFORE Cobra performs command lookup. main.go executes the root without
	// calling SetArgs, so seeding the expanded argv here is exactly what the
	// dispatcher will see; callers that call SetArgs afterwards keep full
	// control of argv. Without this, `openapi-cli <alias>` failed with
	// `unknown command "<alias>"` even though the alias was configured and
	// documented (DF-024).
	effectiveArgs := os.Args[1:]
	if expanded, ok := expandAliasArgs(cfg, effectiveArgs); ok {
		effectiveArgs = expanded
		root.SetArgs(effectiveArgs)
	}

	// Add built-in commands
	root.AddCommand(newVersionCommand())
	root.AddCommand(newConfigCommand(cfg, root))
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
	root.AddCommand(newProtocolCommand())
	root.AddCommand(newGrpcCommand())

	// Load and add persisted commands from storage only when this invocation
	// can dispatch one. Meta-command invocations never consult the generated
	// command list, so avoid decoding the potentially large store for them.
	if invocationNeedsPersistedCommands(root, effectiveArgs) {
		loadPersistedCommands(root, cfg)
	}

	return root
}

// invocationNeedsPersistedCommands reports whether argv can dispatch a
// persisted command. It deliberately errs on the side of loading for unknown
// flags and command words because Cobra may need the generated command list to
// report or resolve them.
func invocationNeedsPersistedCommands(root *cobra.Command, args []string) bool {
	if len(args) == 0 {
		// Preserve the existing bare-root help output, which includes persisted
		// commands because no meta-command was explicitly requested.
		return true
	}

	helpRequested := false
	versionRequested := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			// A word after the end-of-flags separator can still name a persisted
			// command (`openapi-cli -- generated-command`), so load when anything
			// follows it; a trailing `--` dispatches nothing.
			return i+1 < len(args)
		}
		if strings.HasPrefix(arg, "-") {
			name := strings.TrimLeft(arg, "-")
			if index := strings.IndexByte(name, '='); index >= 0 {
				name = name[:index]
			}
			// Cobra installs these meta flags during command initialization,
			// after this construction-time decision is made.
			if name == "help" || name == "h" {
				helpRequested = true
				continue
			}
			if name == "version" {
				versionRequested = true
				continue
			}
			known, takesValue := rootFlagArity(root, arg)
			if !known {
				return true
			}
			if takesValue && !strings.Contains(arg, "=") {
				if i+1 >= len(args) {
					return true
				}
				i++
			}
			continue
		}

		switch arg {
		case "version", "completion", "__complete", "__completeNoDesc":
			return false
		case "help":
			// The default help command's next word is a command target, so
			// `help <generated-command>` must load persisted commands.
			return hasHelpTarget(root, args[i+1:])
		default:
			return true
		}
	}

	return !helpRequested && !versionRequested
}

func hasHelpTarget(root *cobra.Command, args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			// Mirror invocationNeedsPersistedCommands: a word after the separator
			// can still name the help target.
			return i+1 < len(args)
		}
		if strings.HasPrefix(arg, "-") {
			name := strings.TrimLeft(arg, "-")
			if index := strings.IndexByte(name, '='); index >= 0 {
				name = name[:index]
			}
			if name == "help" || name == "h" || name == "version" {
				continue
			}
			known, takesValue := rootFlagArity(root, arg)
			if !known {
				return true
			}
			if takesValue && !strings.Contains(arg, "=") {
				i++
			}
			continue
		}
		return true
	}
	return false
}

func rootFlagArity(root *cobra.Command, arg string) (known, takesValue bool) {
	name := strings.TrimLeft(arg, "-")
	if index := strings.IndexByte(name, '='); index >= 0 {
		name = name[:index]
	}
	shorthand := len(arg) == 2 && arg[0] == '-' && arg[1] != '-'
	if shorthand {
		if flag := root.Flags().ShorthandLookup(name); flag != nil {
			return true, flag.NoOptDefVal == ""
		}
		if flag := root.PersistentFlags().ShorthandLookup(name); flag != nil {
			return true, flag.NoOptDefVal == ""
		}
		return false, false
	}
	if flag := root.Flags().Lookup(name); flag != nil {
		return true, flag.NoOptDefVal == ""
	}
	if flag := root.PersistentFlags().Lookup(name); flag != nil {
		return true, flag.NoOptDefVal == ""
	}
	return false, false
}

// isMetaCommandName reports whether the named command is a meta-command
// for which persisted (on-disk generated) commands should be hidden.
// Mirrors the skip in cmd/openapi-cli/main.go persistentPreRun.
func isMetaCommandName(name string) bool {
	switch name {
	case "help", "version", "completion", "__complete":
		return true
	}
	return false
}

// expandAliasArgs rewrites the command word of args when it names a
// user-defined alias from the config file's `aliases:` map, returning the
// rewritten argv and true. The mapped command line is split on whitespace and
// the remaining original arguments are appended, so both an alias that stands
// for a whole invocation (`tasks: request GET http://localhost:9801/tasks`)
// and one that only stands for a command plus defaults (`ls: inventory list`)
// keep working, and user-supplied trailing arguments still reach the target
// command.
//
// Only the first argument is considered: global flags that precede the
// command word (`--profile x <alias>`) are not aliases and must keep their
// normal meaning. A mapping that does not name a command (empty, or
// whitespace only) or that would expand to itself is left untouched, so a
// misconfigured alias can never loop the rewrite.
func expandAliasArgs(cfg *config.Config, args []string) ([]string, bool) {
	if cfg == nil || len(args) == 0 || len(cfg.Aliases) == 0 {
		return args, false
	}
	target, ok := cfg.Aliases[args[0]]
	if !ok {
		return args, false
	}
	fields := strings.Fields(target)
	if len(fields) == 0 || fields[0] == args[0] {
		return args, false
	}
	expanded := make([]string, 0, len(fields)+len(args)-1)
	expanded = append(expanded, fields...)
	expanded = append(expanded, args[1:]...)
	return expanded, true
}

// helpRequested returns true if the --help flag was set on the given command.
func helpRequested(cmd *cobra.Command) bool {
	helpFlag := cmd.Flags().Lookup("help")
	if helpFlag == nil {
		return false
	}
	return helpFlag.Changed
}

// hidePersistedCommands toggles the Hidden flag on every command added
// by loadPersistedCommands. We don't RemoveCommand because the
// persistent pre-run / post-run pair needs to round-trip visibility.
func hidePersistedCommands(root *cobra.Command, hidden bool) {
	for _, cmd := range persistedCommands {
		cmd.Hidden = hidden
	}
}

// CliActorID returns the actor identifier for CLI audit events.
func CliActorID() string {
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
	storeDecodeCount++
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
