package main

import (
	"context"
	"fmt"
	"os"

	"github.com/openapi-cli/openapi-cli/internal/builtin"
	"github.com/openapi-cli/openapi-cli/pkg/config"
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

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// Config loading errors are non-fatal - we can run with defaults
		// NOTE: config.Load() is designed to never fail (it always returns a valid Config),
		// so this error path is defensive programming and may not be reachable in practice
		cfg = &config.Config{Version: Version}
	}

	// Build root command
	rootCmd := builtin.NewRootCommand(cfg)

	// Execute
	return rootCmd.ExecuteContext(ctx)
}
