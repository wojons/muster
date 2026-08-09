package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/internal/version"
	"github.com/wojons/muster/pkg/mcp"
	"github.com/wojons/muster/pkg/mcp/handlers"
)

type MCPServerConfig struct {
	// Pointer-containing fields first (reduces GC scan area)
	LogLevel   string
	LogOutput  string
	SpecPath   string // Path to OpenAPI spec file
	BaseURL    string // Base URL for API operations
	WasmPlugin string // Path to WASM plugin for response transformation
	// Non-pointer fields
	MaxConcurrentCommands int
	CommandTimeout        time.Duration
	EnableTools           bool
	EnableResources       bool
	EnablePrompts         bool
}

var (
	specPathFlag   = flag.String("spec", "", "Path to OpenAPI specification file")
	baseURLFlag    = flag.String("base-url", "", "Base URL for API operations")
	logLevelFlag   = flag.String("log-level", "info", "Log level")
	logOutputFlag  = flag.String("log-output", "stderr", "Log output destination")
	auditFlag      = flag.Bool("audit", false, "Enable audit logging (also set MUSTER_AUDIT_ENABLED=1)")
	wasmPluginFlag = flag.String("wasm-plugin", "", "Path to WASM plugin for response transformation")
	versionFlag    = flag.Bool("version", false, "Print version information and exit")
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
		WasmPlugin:            *wasmPluginFlag,
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

	if *versionFlag {
		fmt.Println(version.Get().Short())
		return
	}

	ctx, cancel := setupSignalHandling()
	defer cancel()

	// Initialize audit hook (opt-in via --audit flag or MUSTER_AUDIT_ENABLED env var)
	auditConfig := audit.DefaultConfig()
	if *auditFlag {
		auditConfig.Enabled = true
	}
	auditHook := audit.NewHook(auditConfig)
	defer func() {
		if err := auditHook.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "audit: shutdown error: %v\n", err)
		}
	}()

	config := loadConfig()
	stdin, stdout, stderr := setupStdio()

	server := mcp.NewServer(stdin, stdout, stderr)

	// Set up server
	if err := runMCPServer(config, server); err != nil {
		auditHook.LogCommand(ctx, "mcp-server-start", mcpActorID(), false, map[string]string{"error": err.Error()})
		handleFatal(err)
	}
	auditHook.LogCommand(ctx, "mcp-server-start", mcpActorID(), true, nil)

	if err := server.Run(ctx); err != nil && err != context.Canceled {
		auditHook.LogCommand(ctx, "mcp-server-run", mcpActorID(), false, map[string]string{"error": err.Error()})
		handleFatal(err)
	}
}

// loadAndRegisterTools loads OpenAPI spec and registers tools with MCP server
func loadAndRegisterTools(server *mcp.Server, config *MCPServerConfig) error {
	// Load OpenAPI spec (supports both file paths and URLs)
	loader := openapi3.NewLoader()
	doc, err := loadSpec(loader, config.SpecPath)
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}

	if validateErr := doc.Validate(context.Background()); validateErr != nil {
		return fmt.Errorf("validate spec: %w", validateErr)
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

// RegisterHandler adds a handler to the MCP server.
func RegisterHandler(server *mcp.Server, handler handlers.Handler) {
	if server != nil {
		server.RegisterHandler(handler)
	}
}

// loadSpec loads an OpenAPI spec from a file path or URL.
func loadSpec(loader *openapi3.Loader, specPath string) (*openapi3.T, error) {
	// If specPath looks like a URL, load it from the network
	parsed, err := url.Parse(strings.TrimSpace(specPath))
	if err != nil {
		return nil, fmt.Errorf("parse spec path %q: %w", specPath, err)
	}
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		return loader.LoadFromURI(parsed)
	}
	// Otherwise, load from a local file
	return loader.LoadFromFile(specPath)
}

// isURL returns true if the given path looks like an HTTP/HTTPS URL.
func isURL(s string) bool {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// mcpActorID returns the actor identifier for MCP audit events.
func mcpActorID() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("LOGNAME"); user != "" {
		return user
	}
	return "mcp-client"
}
