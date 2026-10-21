package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/builtin"
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

func run() error {
	ctx := context.Background()

	// Initialize audit hook (opt-in via MUSTER_AUDIT_ENABLED env var)
	auditHook := audit.NewHook(audit.DefaultConfig())
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
		// rootCmd.CommandPath() is "openapi-cli" after ExecuteContext returns
		// an error (the active subcommand is lost). Reconstruct from original
		// argv: skip "openapi-cli" (os.Args[0]), skip leading flags (and their
		// value args), take the first non-flag word as the command path prefix.
		command := rootCmd.CommandPath()
		args := os.Args[1:]
		i := 0
		for i < len(args) {
			a := args[i]
			if !strings.HasPrefix(a, "-") {
				// First non-flag word is the subcommand
				command = "openapi-cli " + a
				break
			}
			// Flag — skip it and its optional value
			// Flags with string values that consume the next argv element:
			if strings.HasPrefix(a, "--") && i+1 < len(args) {
				// Check if this flag takes a value (not a bool)
				switch a {
				case "--profile", "--log-level", "--log-format",
					"--database-url", "--auth-token", "--config-file",
					"--timeout", "--max-retries", "--http-version",
					"--http-timeout", "-u", "--base-url":
					// Skip the value too
					i++
				}
			}
			// Short flags like -f don't take values; -d value does
			if strings.HasPrefix(a, "-") && len(a) == 2 && a[1] != '-' {
				// Check if it's a known value-taking short flag
				if a == "-d" {
					i++
				}
			}
			i++
		}
		auditHook.LogCommand(context.Background(), command, builtin.CliActorID(), false, map[string]string{"error": err.Error()})
	}

	return err
}
