package builtin

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/openapi-cli/openapi-cli/pkg/cmdstore"
	"github.com/openapi-cli/openapi-cli/pkg/config"
	"github.com/openapi-cli/openapi-cli/pkg/generator"
	"github.com/openapi-cli/openapi-cli/pkg/openapi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newGenerateCommand(cfg *config.Config) *cobra.Command {
	var baseURL string

	cmd := &cobra.Command{
		Use:   "generate <spec-path>",
		Short: "Generate commands from OpenAPI specification",
		Long: `Generate CLI commands dynamically from an OpenAPI 3.0 or 3.1 specification.

The generator parses the spec and creates Cobra commands for each API operation.

Commands are persisted to storage and loaded on subsequent CLI invocations.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specPath := args[0]
			fmt.Printf("Loading spec from: %s\n", specPath)

			data, err := os.ReadFile(specPath)
			if err != nil {
				return fmt.Errorf("read spec file: %w", err)
			}

			ctx := context.Background()
			opts := openapi.DefaultParseOptions()

			parser := openapi.NewParser()
			result, err := parser.Parse(ctx, data, opts)
			if err != nil {
				return fmt.Errorf("parse spec: %w", err)
			}

			fmt.Printf("✓ Parsed spec (version: %s)\n", result.Version)

			baseURLPath := baseURL
			if baseURLPath == "" {
				baseURLPath = "https://api.example.com"
			}

			genCfg := generator.Config{
				AppName:          "openapi-cli",
				BaseURL:          baseURLPath,
				DefaultFormat:    "table",
				SupportedFormats: []string{"table", "json", "yaml"},
			}

			gen, err := generator.NewGenerator(genCfg)
			if err != nil {
				return fmt.Errorf("create generator: %w", err)
			}

			commands, err := gen.GenerateCommands(result.Document)
			if err != nil {
				return fmt.Errorf("generate commands: %w", err)
			}

			fmt.Printf("✓ Generated %d command groups\n", len(commands))

			// Convert commands to metadata and persist
			metadata, err := commandsToMetadata(commands, specPath, baseURLPath, result.Document)
			if err != nil {
				return fmt.Errorf("convert commands to metadata: %w", err)
			}

			if err := persistCommands(metadata); err != nil {
				return fmt.Errorf("persist commands: %w", err)
			}
			fmt.Printf("✓ Commands persisted to storage\n")

			// Register commands to parent for immediate use
			if cmd.Parent() != nil {
				for _, generatedCmd := range commands {
					cmd.Parent().AddCommand(generatedCmd)
					fmt.Printf("✓ Registered command: %s\n", generatedCmd.Name())
				}

				fmt.Printf("\n✓ Success! Commands registered and persisted\n")
				fmt.Printf("Run 'openapi-cli --help' to see available commands\n")
				fmt.Printf("Commands will load automatically on next CLI start\n")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&baseURL, "base-url", "u", "", "Base URL for the API (e.g., https://api.example.com)")

	return cmd
}

// commandsToMetadata converts generated cobra.Commands to persistent metadata
func commandsToMetadata(commands []*cobra.Command, specPath, baseURL string, doc *openapi3.T) (*cmdstore.StoredData, error) {
	var commandMetadatas []cmdstore.CommandMetadata

	// Extract operations from the spec
	for _, cmd := range commands {
		// Recursively process subcommands
		extractCommandMetadata(cmd, specPath, baseURL, doc, &commandMetadatas)
	}

	// Calculate spec hash
	specHash := computeSpecHash(specPath)

	metadata := &cmdstore.StoredData{
		Version:     "1.0",
		SpecPath:    specPath,
		SpecHash:    specHash,
		BaseURL:     baseURL,
		GeneratedAt: time.Now(),
		Commands:    commandMetadatas,
	}

	return metadata, nil
}

// extractOperationFromCommandAndSpec finds the OpenAPI operation that corresponds to a command
func extractOperationFromCommandAndSpec(cmd *cobra.Command, doc *openapi3.T) (path, method string, args []string) {
	// Try to match command name to operation ID or infer from naming convention
	commandName := cmd.Name()

	// Search all operations in the spec
	for p, pathItem := range doc.Paths.Map() {
		methods := map[string]*openapi3.Operation{
			"GET":     pathItem.Get,
			"POST":    pathItem.Post,
			"PUT":     pathItem.Put,
			"PATCH":   pathItem.Patch,
			"DELETE":  pathItem.Delete,
			"HEAD":    pathItem.Head,
			"OPTIONS": pathItem.Options,
		}

		for m, operation := range methods {
			if operation == nil {
				continue
			}

			// Try exact operation ID match
			if operation.OperationID == commandName {
				// Extract path parameters
				args = extractPathParamsFromPath(p)
				return p, m, args
			}

			// Try kebab-case match (e.g., "list-pets" → "listPets")
			opID := toKebabCase(operation.OperationID)
			if opID == commandName {
				args = extractPathParamsFromPath(p)
				return p, m, args
			}
		}
	}

	// If no match found, try to infer from command name patterns
	// e.g., "list-pets" → GET /pets, "create-pet" → POST /pets
	if inferPath, inferMethod := inferFromCommandName(commandName); inferPath != "" {
		args = extractPathParamsFromPath(inferPath)
		return inferPath, inferMethod, args
	}

	// Default: can't determine
	return "", "", nil
}

// extractPathParamsFromPath extracts parameter names from a path template
func extractPathParamsFromPath(path string) []string {
	var params []string
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			params = append(params, part[1:len(part)-1])
		}
	}
	return params
}

// inferFromCommandName tries to infer path and method from command name
func inferFromCommandName(name string) (path, method string) {
	// Common patterns: list-pets, create-pet, get-pet, update-pet, delete-pet
	parts := strings.Split(name, "-")
	if len(parts) < 2 {
		return "", ""
	}

	action := parts[0]
	resource := parts[len(parts)-1]

	// Convert resource to plural if needed
	resourcePlural := pluralize(resource)

	switch action {
	case "list":
		return "/" + resourcePlural, "GET"
	case "get":
		return "/" + resourcePlural + "/{" + singularize(resource) + "Id}", "GET"
	case "create":
		return "/" + resourcePlural, "POST"
	case "update":
		return "/" + resourcePlural + "/{" + singularize(resource) + "Id}", "PUT"
	case "delete":
		return "/" + resourcePlural + "/{" + singularize(resource) + "Id}", "DELETE"
	}

	return "", ""
}

// toKebabCase converts CamelCase to kebab-case
func toKebabCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('-')
			result.WriteRune(r + 32) // to lowercase
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// pluralize converts singular to plural (simple version)
func pluralize(s string) string {
	return s + "s"
}

// singularize converts plural to singular (simple version)
func singularize(s string) string {
	if strings.HasSuffix(s, "s") {
		return s[:len(s)-1]
	}
	return s
}

// extractCommandMetadata recursively extracts metadata from command and subcommands
func extractCommandMetadata(cmd *cobra.Command, specPath, baseURL string, doc *openapi3.T, metadata *[]cmdstore.CommandMetadata) {
	// For each command, check if it's a leaf (operation) or a group
	if cmd.HasSubCommands() {
		// This is a group, recurse into subcommands
		for _, subCmd := range cmd.Commands() {
			extractCommandMetadata(subCmd, specPath, baseURL, doc, metadata)
		}
		return
	}

	// This is a leaf command (operation), create metadata
	// Try to find matching operation from OpenAPI spec based on command name
	path, method, args := extractOperationFromCommandAndSpec(cmd, doc)

	cmdMeta := cmdstore.CommandMetadata{
		Name:    cmd.Name(),
		Use:     cmd.Use,
		Short:   cmd.Short,
		Long:    cmd.Long,
		Example: cmd.Example,
		BaseURL: baseURL,
		Path:    path,
		Method:  method,
		Args:    args,
	}

	// Extract flags
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		flagMeta := cmdstore.FlagMetadata{
			Name:        flag.Name,
			Shorthand:   string(flag.Shorthand),
			Type:        getFlagType(flag),
			Default:     flag.DefValue,
			Description: flag.Usage,
		}

		if flag.Name == "output" || flag.Name == "namespace" || flag.Name == "debug" {
			// Skip standard flags
			return
		}

		cmdMeta.Flags = append(cmdMeta.Flags, flagMeta)
	})

	*metadata = append(*metadata, cmdMeta)
}

// getFlagType determines the type of a flag
func getFlagType(flag *pflag.Flag) string {
	switch flag.Value.Type() {
	case "string":
		return "string"
	case "int":
		return "int"
	case "bool":
		return "bool"
	case "stringSlice":
		return "array"
	default:
		return "string"
	}
}

// computeSpecHash calculates a hash for the spec file
func computeSpecHash(specPath string) string {
	data, _ := os.ReadFile(specPath)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)[:16]
}

// persistCommands saves command metadata to storage
func persistCommands(metadata *cmdstore.StoredData) error {
	// Get command storage path
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		dataDir = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	dataPath := filepath.Join(dataDir, "openapi-cli", "commands.json")

	store := cmdstore.NewCommandStore(dataPath)
	return store.Save(metadata)
}
