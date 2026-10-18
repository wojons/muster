package main

import (
	"context"
	"fmt"
	"os"

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
		// Use background context since the command context may be cancelled
		auditHook.LogCommand(context.Background(), rootCmd.CommandPath(), "cli", false, map[string]string{"error": err.Error()})
	}
	
	return err
}
