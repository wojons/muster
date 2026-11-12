package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/builtin"
	"github.com/wojons/muster/internal/monitoring"
	"github.com/wojons/muster/pkg/config"
)

// Build-time variables
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// auditFlagArity reports whether arg names a flag the command (or one of its
// ancestors) knows, and whether that flag consumes the following argv element.
// Flag metadata comes from cobra/pflag rather than a hard-coded table of
// value-taking flags, so a newly registered global flag needs no change here.
func auditFlagArity(cmd *cobra.Command, arg string) (known, takesValue bool) {
	name := strings.TrimLeft(arg, "-")
	if i := strings.IndexByte(name, '='); i >= 0 {
		name = name[:i]
	}
	shorthand := len(arg) == 2 && arg[0] == '-' && arg[1] != '-'

	for c := cmd; c != nil; c = c.Parent() {
		if shorthand {
			if f := c.Flags().ShorthandLookup(name); f != nil {
				return true, f.NoOptDefVal == ""
			}
			if f := c.PersistentFlags().ShorthandLookup(name); f != nil {
				return true, f.NoOptDefVal == ""
			}
			continue
		}
		if f := c.Flags().Lookup(name); f != nil {
			return true, f.NoOptDefVal == ""
		}
		if f := c.PersistentFlags().Lookup(name); f != nil {
			return true, f.NoOptDefVal == ""
		}
	}

	return false, false
}

func auditCommandPath(root *cobra.Command, args []string) string {
	path := []string{root.CommandPath()}
	command := root

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			known, takesValue := auditFlagArity(command, arg)
			if !known {
				break
			}
			if takesValue && !strings.Contains(arg, "=") && i+1 < len(args) {
				i++
			}
			continue
		}

		var child *cobra.Command
		for _, candidate := range command.Commands() {
			if candidate.Name() == arg || candidate.HasAlias(arg) {
				child = candidate
				break
			}
		}
		if child == nil {
			// Not a subcommand. A command that runs its own logic treats the
			// word as a positional argument (identity stays the resolved
			// path); a grouping command has no leaf, so the attempted word is
			// the deepest identity the invocation reached.
			if command.Run == nil && command.RunE == nil {
				path = append(path, arg)
			}
			break
		}
		path = append(path, arg)
		command = child
	}

	return strings.Join(path, " ")
}

func run() error {
	ctx := context.Background()

	// The CLI is a short-lived process and has no scrape listener, but it still
	// uses the same recorder-owned registry and audit metrics sink as the other
	// entrypoints while it runs.
	metricsRecorder := monitoring.NewMetricRecorder(monitoring.DefaultTenantID, nil)

	// Initialize audit hook (opt-in via MUSTER_AUDIT_ENABLED env var)
	auditHook := audit.NewHookWithMetrics(audit.DefaultConfig(), metricsRecorder)
	defer func() {
		if err := auditHook.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "audit: shutdown error: %v\n", err)
		}
	}()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// Config loading errors are non-fatal - we can run with defaults
		// NOTE: config.Load() is designed to never fail (it always returns a valid Config),
		// so this error path is defensive programming and may not be reachable in practice
		cfg = &config.Config{Version: Version}
	}

	// Build root command with audit hook wired in
	rootCmd := builtin.NewRootCommandWithAudit(cfg, auditHook)

	// Execute
	err = rootCmd.ExecuteContext(ctx)

	// Log command execution (success logged in PersistentPostRun, failures logged here)
	if err != nil && auditHook != nil {
		// Use background context since the command context may be cancelled.
		// Cobra has already resolved as much of the command path as it can,
		// including nested commands before a malformed flag. Preserve that
		// resolved path instead of reducing the invocation to its first word.
		command := auditCommandPath(rootCmd, os.Args[1:])
		auditHook.LogCommand(context.Background(), command, builtin.CliActorID(), false, map[string]string{"error": err.Error()})
	}

	return err
}
