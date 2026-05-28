package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/wojons/muster/pkg/mcp"
	"github.com/wojons/muster/pkg/mcp/handlers"
)

type MCPServerConfig struct {
	LogLevel              string
	LogOutput             string
	EnableTools           bool
	EnableResources       bool
	EnablePrompts         bool
	MaxConcurrentCommands int
	CommandTimeout        time.Duration
	SpecPath              string // Path to OpenAPI spec file
	BaseURL               string // Base URL for API operations
}

var (
	specPathFlag  = flag.String("spec", "", "Path to OpenAPI specification file")
	baseURLFlag   = flag.String("base-url", "", "Base URL for API operations")
	logLevelFlag  = flag.String("log-level", "info", "Log level")
	logOutputFlag = flag.String("log-output", "stderr", "Log output destination")
)

func loadConfig() *MCPServerConfig {
	return &MCPServerConfig{
		LogLevel:              *logLevelFlag,
		LogOutput:             *logOutputFlag,
		EnableTools:           true,
		EnableResources:       true,
		EnablePrompts:         true,
		MaxConcurrentCommands: 10,
		CommandTimeout:        30 * time.Second,
		SpecPath:              *specPathFlag,
		BaseURL:               *baseURLFlag,
	}
}

func setupStdio() (io.Reader, io.Writer, io.Writer) {
	if runtime.GOOS == "windows" {
		enableBinaryMode()
	}

	stdin := bufio.NewReader(os.Stdin)
	stdout := bufio.NewWriter(os.Stdout)
	stderr := os.Stderr

	return stdin, stdout, stderr
}

func handleFatal(err error) {
	fmt.Fprintf(os.Stderr, "Fatal error: %v\n", err)
	os.Exit(1)
}

var enableBinaryMode = func() {}

func setupSignalHandling() (context.Context, context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sigChan
		cancel()
	}()

	return ctx, cancel
}

func runMCPServer(config *MCPServerConfig, server *mcp.Server) error {
	// If spec is provided, load and register tools
	if config.SpecPath != "" {
		if err := loadAndRegisterTools(server, config); err != nil {
			return fmt.Errorf("failed to load spec: %w", err)
		}
	}

	return nil
}

func main() {
	flag.Parse()

	ctx, cancel := setupSignalHandling()
	defer cancel()

	config := loadConfig()
	stdin, stdout, stderr := setupStdio()

	server := mcp.NewServer(stdin, stdout, stderr)

	// Set up server
	if err := runMCPServer(config, server); err != nil {
		handleFatal(err)
	}

	if err := server.Run(ctx); err != nil && err != context.Canceled {
		handleFatal(err)
	}
}

// loadAndRegisterTools loads OpenAPI spec and registers tools with MCP server
func loadAndRegisterTools(server *mcp.Server, config *MCPServerConfig) error {
	// Load OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(config.SpecPath)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	if err := doc.Validate(context.Background()); err != nil {
		return fmt.Errorf("validate spec: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Loaded spec: %s (%s)\n", doc.Info.Title, doc.Info.Version)

	// Convert OpenAPI operations to MCP tools
	converter := mcp.NewConverter(config.BaseURL)
	tools, configs, err := converter.ConvertToTools(doc)
	if err != nil {
		return fmt.Errorf("convert tools: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Generated %d MCP tools\n", len(tools))

	// Create registry and executor
	registry := mcp.NewOpenAPIRegistry(tools, configs)
	executor := mcp.NewOpenAPIExecutor(configs)

	// Register tool handlers
	listHandler := handlers.NewListToolsHandler(registry)
	callHandler := handlers.NewCallToolHandler(executor)

	server.RegisterHandler(listHandler)
	server.RegisterHandler(callHandler)

	return nil
}

// RegisterHandler adds a handler to the MCP server (extension method)
func RegisterHandler(server *mcp.Server, handler handlers.Handler) {
	// This would need to be implemented in pkg/mcp/server.go
	// For now, just a placeholder
	_ = handler
}
