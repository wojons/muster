package builtin

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/wojons/muster/pkg/cmdstore"
	"github.com/wojons/muster/pkg/config"
	"github.com/wojons/muster/pkg/generator"
	"github.com/wojons/muster/pkg/openapi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newGenerateCommand(cfg *config.Config) *cobra.Command {
	var baseURL string

	cmd := &cobra.Command{
		Use:   "generate <spec-path-or-url>",
		Short: "Generate commands from OpenAPI specification",
		Long: `Generate CLI commands dynamically from an OpenAPI 3.0 or 3.1 specification.

The generator parses the spec and creates Cobra commands for each API operation.
Accepts either a local file path or a remote URL (http/https).

Commands are persisted to storage and loaded on subsequent CLI invocations.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specPath := args[0]
			fmt.Printf("Loading spec from: %s\n", specPath)

			// Determine whether to load from file or URL
			var data []byte
			var err error
			if strings.HasPrefix(specPath, "http://") || strings.HasPrefix(specPath, "https://") {
				data, err = fetchSpec(specPath)
				if err != nil {
					return fmt.Errorf("fetch spec from URL: %w", err)
				}
			} else {
				data, err = os.ReadFile(specPath)
				if err != nil {
					return fmt.Errorf("read spec file: %w", err)
				}
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
			metadata, err := commandsToMetadata(commands, specPath, baseURLPath, result.Document, data)
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
func commandsToMetadata(commands []*cobra.Command, specPath, baseURL string, doc *openapi3.T, specData []byte) (*cmdstore.StoredData, error) {
	var commandMetadatas []cmdstore.CommandMetadata

	// Extract operations from the spec
	for _, cmd := range commands {
		// Recursively process subcommands
		extractCommandMetadata(cmd, specPath, baseURL, doc, &commandMetadatas)
	}

	// Calculate spec hash from the already-loaded spec data
	specHash := computeSpecHash(specData)

	metadata := &cmdstore.StoredData{
		SpecPath: specPath,
		SpecHash: specHash,
		BaseURL:  baseURL,
		Commands: commandMetadatas,
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
	params := make([]string, 0)
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
	if s == "" {
		return ""
	}
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
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=internal/builtin/generate.go
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

	// Determine which flags represent request body properties (vs query/header params).
	bodyFlagToJSON := extractBodyPropertyNames(doc, path, method)

	// Determine if this operation supports multipart
	hasMultipart, multipartFields := detectMultipartOperation(doc, path, method)

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
			return
		}

		// Tag file-upload flag for multipart operations
		if hasMultipart {
			if flag.Name == "file" {
				flagMeta.Kind = cmdstore.FlagKindFile
			} else if multipartFields[flag.Name] {
				flagMeta.Kind = cmdstore.FlagKindMultipart
			}
		}

		// Tag body flags so the loader can marshal them as JSON body
		if jsonName, ok := bodyFlagToJSON[flag.Name]; ok {
			// Don't override multipart kind with body kind
			if flagMeta.Kind == "" {
				flagMeta.Kind = cmdstore.FlagKindBody
			}
			flagMeta.JSONName = jsonName
		}

		cmdMeta.Flags = append(cmdMeta.Flags, flagMeta)
	})

	*metadata = append(*metadata, cmdMeta)
}

// detectMultipartOperation checks if the operation at path+method supports multipart/form-data
// and returns a set of non-file multipart field flag names.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=internal/builtin/generate.go
func detectMultipartOperation(doc *openapi3.T, path, method string) (bool, map[string]bool) {
	if doc == nil || path == "" || method == "" {
		return false, nil
	}
	pathItem := doc.Paths.Find(path)
	if pathItem == nil {
		return false, nil
	}
	operation := pathItem.GetOperation(method)
	if operation == nil || operation.RequestBody == nil || operation.RequestBody.Value == nil {
		return false, nil
	}

	mt := operation.RequestBody.Value.Content.Get("multipart/form-data")
	if mt == nil || mt.Schema == nil || mt.Schema.Value == nil {
		return false, nil
	}

	// Collect non-binary field names
	fields := make(map[string]bool)
	for name, propRef := range mt.Schema.Value.Properties {
		if propRef == nil || propRef.Value == nil {
			continue
		}
		// Skip file/binary fields — those go through --file
		if propRef.Value.Format == "binary" {
			continue
		}
		fields[normalizeFlagName(name)] = true
	}

	return true, fields
}

// extractBodyPropertyNames returns a map from normalized flag name to original
// JSON property name for all request body properties in the given path+method.
// Resolves $ref references (e.g. "#/components/schemas/CreateMilestone") by
// looking up the referenced schema in the document's Components.Schemas map.
// Returns nil if no body schema exists.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=internal/builtin/generate.go
func extractBodyPropertyNames(doc *openapi3.T, path, method string) map[string]string {
	if doc == nil || path == "" || method == "" {
		return nil
	}

	pathItem := doc.Paths.Find(path)
	if pathItem == nil {
		return nil
	}

	operation := pathItem.GetOperation(method)
	if operation == nil || operation.RequestBody == nil || operation.RequestBody.Value == nil {
		return nil
	}

	content := operation.RequestBody.Value.Content
	if content == nil {
		return nil
	}

	// Look for application/json request body content
	// kin-openapi resolves $ref at parse time when the loader is configured
	// to do so, but when it doesn't, we resolve it manually via
	// resolveRequestBodySchema below.
	schema := resolveRequestBodySchema(doc, content.Get("application/json"))
	if schema == nil {
		return nil
	}

	names := make(map[string]string)
	for propName := range schema.Properties {
		// Normalize the spec property name to match flag naming convention
		normalized := normalizeFlagName(propName)
		names[normalized] = propName // original name → JSON key
	}
	return names
}

// resolveRequestBodySchema resolves the schema for a requestBody content-type
// entry, following $ref references that point into #/components/schemas/...
// This is a fallback for cases where kin-openapi didn't fully resolve the
// $ref at parse time.
func resolveRequestBodySchema(doc *openapi3.T, mt *openapi3.MediaType) *openapi3.Schema {
	if mt == nil || mt.Schema == nil {
		return nil
	}

	if mt.Schema.Value != nil && mt.Schema.Value.Properties != nil && len(mt.Schema.Value.Properties) > 0 {
		return mt.Schema.Value
	}

	// Resolution via kin-openapi may have populated Value even for a $ref;
	// check if there's a $ref we need to follow manually.
	if mt.Schema.Ref != "" && strings.HasPrefix(mt.Schema.Ref, "#/components/schemas/") {
		schemaName := strings.TrimPrefix(mt.Schema.Ref, "#/components/schemas/")
		if doc.Components != nil && doc.Components.Schemas != nil {
			if refSchema, ok := doc.Components.Schemas[schemaName]; ok && refSchema != nil && refSchema.Value != nil {
				return refSchema.Value
			}
		}
	}

	return nil
}

// normalizeFlagName converts a camelCase name to kebab-lower flag name.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=internal/builtin/generate.go
func normalizeFlagName(name string) string {
	var result strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				result.WriteRune('-')
			}
			result.WriteRune(r + 32) // to lowercase
		} else {
			result.WriteRune(r)
		}
	}
	return strings.ToLower(result.String())
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

// computeSpecHash calculates a hash for the spec data bytes.
// Accepts raw spec data to work with both file and URL sources.
func computeSpecHash(data []byte) string {
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
