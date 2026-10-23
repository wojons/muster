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
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/wojons/muster/pkg/cmdstore"
	"github.com/wojons/muster/pkg/config"
	"github.com/wojons/muster/pkg/generator"
	"github.com/wojons/muster/pkg/openapi"
)

func newGenerateCommand(cfg *config.Config) *cobra.Command {
	var baseURL string
	var listStore bool
	var clearStore bool

	cmd := &cobra.Command{
		Use:   "generate [<spec-path-or-url>]",
		Short: "Generate commands from OpenAPI specification",
		Long: `Generate CLI commands dynamically from an OpenAPI 3.0 or 3.1 specification.

The generator parses the spec and creates Cobra commands for each API operation.
Accepts either a local file path or a remote URL (http/https).

Commands are persisted to storage and loaded on subsequent CLI invocations.

Use --list to inspect the current persisted store, or --clear to remove it.`,
		Args: func(cmd *cobra.Command, args []string) error {
			// --list and --clear don't require a spec argument
			if listStore || clearStore {
				return cobra.NoArgs(cmd, args)
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// --list: show current persisted store info
			if listStore {
				return listPersistedStore(cfg)
			}

			// --clear: remove the persisted store
			if clearStore {
				return clearPersistedStore(cfg)
			}

			specPath := args[0]
			fmt.Printf("Loading spec from: %s\n", specPath)

			// Determine whether to load from file or URL
			var data []byte
			var err error
			var specSourceURL string
			if strings.HasPrefix(specPath, "http://") || strings.HasPrefix(specPath, "https://") {
				data, err = fetchSpec(specPath, specHeaders)
				if err != nil {
					return fmt.Errorf("fetch spec from URL: %w", err)
				}
				// DF-015: remember the fetch URL so relative server URLs in
				// the spec can be resolved against it below.
				specSourceURL = specPath
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

			// DF-015: resolve RELATIVE server URLs (e.g. "/api/v3") against
			// the spec fetch URL so generated commands carry a usable
			// absolute base URL. No-op for local-file specs (specSourceURL
			// empty — relative servers keep the historical behavior, and the
			// run handler fails with an actionable hint if executed).
			openapi.ApplySourceURLToDocument(result.Document, specSourceURL)

			fmt.Printf("✓ Parsed spec (version: %s)\n", result.Version)
			// PROTO-006: state the conversion explicitly — a Swagger 2.0
			// document was transparently upgraded to the OpenAPI 3.x model
			// before parsing.
			if result.ConvertedFromSwagger2 {
				fmt.Println("✓ Converted from Swagger/OpenAPI 2.0 to OpenAPI 3.x")
			}

			baseURLPath := baseURL
			if baseURLPath == "" {
				baseURLPath = defaultBaseURLFromSpec(result.Document)
			}
			// PROTO-006: the generator must never return an empty command
			// set for a document that parsed successfully. The one measured
			// way a 2.0 document could produce zero commands was a missing
			// paths section (openapi2conv drops the field when absent);
			// refuse that loudly instead of persisting an empty store.
			ops, err := parser.ExtractOperations(result.Document)
			if err != nil {
				return fmt.Errorf("extract operations: %w", err)
			}
			if len(ops) == 0 {
				return fmt.Errorf("spec %q declares no operations (no paths section); nothing to generate", specPath)
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

			// Warn if overwriting a store with a different spec_hash
			if err := warnOnOverwrite(metadata, cfg); err != nil {
				return err
			}

			if err := persistCommands(metadata, cfg); err != nil {
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
	cmd.Flags().BoolVar(&listStore, "list", false, "List the current persisted command store")
	cmd.Flags().BoolVar(&clearStore, "clear", false, "Remove the persisted command store")
	cmd.Flags().StringArrayVar(&specHeaders, "header", nil, "Header for the spec request (format: 'Name: Value', repeatable) — for authenticated spec URLs")

	return cmd
}

// defaultBaseURLFromSpec returns the spec's first server URL when it is an
// absolute URL, otherwise the legacy placeholder. Relative server URLs
// (e.g. "/api" or "") cannot be used as an HTTP request base URL; when the
// spec was fetched over http(s) they are resolved upstream
// (openapi.ApplySourceURLToDocument, DF-015) before this runs, and anything
// still relative fails at execution with the actionable --base-url /
// MUSTER_BASE_URL hint instead of the bare "unsupported protocol scheme".
func defaultBaseURLFromSpec(doc *openapi3.T) string {
	if doc != nil && len(doc.Servers) > 0 && doc.Servers[0] != nil {
		if u := doc.Servers[0].URL; strings.Contains(u, "://") {
			return u
		}
	}
	return "https://api.example.com"
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

// getOperationServerURL extracts the server URL for a specific operation from the spec.
// Resolution order: per-operation servers → path-level servers → root-level servers.
func getOperationServerURL(doc *openapi3.T, path, method string) string {
	if doc == nil || path == "" || method == "" {
		return ""
	}
	pathItem := doc.Paths.Find(path)
	if pathItem == nil {
		return ""
	}
	operation := pathItem.GetOperation(method)
	if operation == nil {
		return ""
	}
	if operation.Servers != nil && len(*operation.Servers) > 0 && (*operation.Servers)[0] != nil {
		return (*operation.Servers)[0].URL
	}
	if len(pathItem.Servers) > 0 && pathItem.Servers[0] != nil {
		return pathItem.Servers[0].URL
	}
	if len(doc.Servers) > 0 && doc.Servers[0] != nil {
		return doc.Servers[0].URL
	}
	return ""
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

	// This is a leaf command (operation), create metadata.
	// Prefer the exact operation metadata stamped by the generator
	// (pkg/generator annotation keys) — reverse-inferring path/method from
	// command names mangles operationId-less specs (doubled-s paths like
	// /projectss, truncated names like delete-mileston, and wrong base URLs;
	// see DF-007). Name-based inference remains only as a fallback for
	// commands that were not produced by the generator.
	path, method, args := cmd.Annotations[generator.AnnotationOperationPath], cmd.Annotations[generator.AnnotationOperationMethod], []string(nil)
	if path == "" || method == "" {
		path, method, args = extractOperationFromCommandAndSpec(cmd, doc)
	} else {
		args = extractPathParamsFromPath(path)
	}

	// Determine which flags represent request body properties (vs query/header params).
	bodyFlagToJSON := extractBodyPropertyNames(doc, path, method)

	// Position of every spec-declared parameter (query/header) so a RELOADED
	// command sends it in the right place instead of defaulting to a query
	// param — and so a spec parameter named like one of the CLI's own
	// presentation flags is still persisted (MUSTER-001).
	paramKinds := extractParameterFlagKinds(doc, path, method)

	// Determine if this operation supports multipart
	hasMultipart, multipartFields := detectMultipartOperation(doc, path, method)

	// Resolve per-operation server URL from the spec, falling back to the global base URL
	operationBaseURL := getOperationServerURL(doc, path, method)
	if operationBaseURL == "" {
		operationBaseURL = baseURL
	}

	// The persisted store reloads every operation command FLAT at the root
	// (no resource groups), so the grouped example on the cobra command
	// ("openapi-cli projects create-project …") would reference a
	// non-existent group. The generator stamps a flat, verbatim-runnable
	// example; prefer it when present (DF-010).
	example := cmd.Example
	if flat, ok := cmd.Annotations[generator.AnnotationOperationExample]; ok && flat != "" {
		example = flat
	}

	cmdMeta := cmdstore.CommandMetadata{
		Name:    cmd.Name(),
		Use:     cmd.Use,
		Short:   cmd.Short,
		Long:    cmd.Long,
		Example: example,
		BaseURL: operationBaseURL,
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

		// MUSTER-001: the CLI's own presentation flags are not part of the
		// request surface and must not be persisted as parameters. But a
		// SPEC-DECLARED parameter that happens to share one of those names IS
		// part of it: DuckBrain's memory API takes `namespace` as a query
		// parameter, and skipping it left every reloaded command unable to
		// address a namespace at all ("unknown flag: --namespace").
		if flag.Name == "output" || flag.Name == "debug" {
			return
		}
		if flag.Name == "namespace" && paramKinds[flag.Name] == "" {
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

		// Tag the raw JSON body flag (--body/-b, DF-MUSTER-5) so the loader
		// sends its value verbatim as the request body instead of routing it
		// as a query parameter. The generator stamps RawBodyAnnotation on
		// commands that registered the flag, so a spec property/parameter
		// that merely shares the name "body" is not mis-tagged.
		if cmd.Annotations[generator.RawBodyAnnotation] == "true" && flag.Name == generator.RawBodyFlagName {
			flagMeta.Kind = cmdstore.FlagKindRawBody
		}

		// DF-023: tag the runtime --auth credential-attachment flag so the
		// loader classifies it as a presentation/runtime flag on reload
		// (never a query parameter). The generator stamps AuthFlagAnnotation
		// only when IT registered the flag, so a spec parameter genuinely
		// named "auth" keeps its natural kind and reaches the server.
		if cmd.Annotations[generator.AuthFlagAnnotation] == "true" && flag.Name == generator.AuthFlagName {
			flagMeta.Kind = cmdstore.FlagKindAuth
		}

		// MUSTER-001: a spec-declared query/header parameter keeps its
		// position on reload. Applied last so it wins over the body default
		// when a request-body property shares the name — the generator gives
		// the parameter ownership of that shared flag (pkg/generator
		// DF-MUSTER-9), so the reloaded command must send it as the parameter.
		if kind, ok := paramKinds[flag.Name]; ok {
			flagMeta.Kind = kind
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

// extractParameterFlagKinds maps every spec-declared operation parameter's
// flag name (normalized exactly like the generator names flags) to the
// position it must be sent in on a RELOADED command: query or header. Path
// parameters are positional args, not flags, so they are omitted.
//
// MUSTER-001: without this, the loader's default branch classified every
// persisted flag as a query parameter — a header parameter was sent in the
// query string — and the persist step could not tell a spec parameter from
// one of the CLI's own presentation flags of the same name.
func extractParameterFlagKinds(doc *openapi3.T, path, method string) map[string]cmdstore.FlagKind {
	kinds := make(map[string]cmdstore.FlagKind)
	if doc == nil || doc.Paths == nil || path == "" || method == "" {
		return kinds
	}
	pathItem := doc.Paths.Find(path)
	if pathItem == nil {
		return kinds
	}
	operation := pathItem.GetOperation(method)
	if operation == nil || operation.Parameters == nil {
		return kinds
	}
	for _, paramRef := range operation.Parameters {
		if paramRef == nil || paramRef.Value == nil {
			continue
		}
		var kind cmdstore.FlagKind
		switch paramRef.Value.In {
		case "query":
			kind = cmdstore.FlagKindQuery
		case "header":
			kind = cmdstore.FlagKindHeader
		default:
			continue
		}
		kinds[normalizeFlagName(paramRef.Value.Name)] = kind
	}
	return kinds
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

// persistCommands saves command metadata to storage.
// The output path is taken from cfg.PersistedCommandsPath when set, so
// tests can redirect writes to a temp directory. When unset, it falls
// back to the default XDG data directory used in production.
func persistCommands(metadata *cmdstore.StoredData, cfg *config.Config) error {
	dataPath := resolvePersistedCommandsPath(cfg)
	store := cmdstore.NewCommandStore(dataPath)
	return store.Save(metadata)
}

// warnOnOverwrite checks the existing persisted store and prints a warning
// if it contains commands from a different spec (different spec_hash).
func warnOnOverwrite(metadata *cmdstore.StoredData, cfg *config.Config) error {
	dataPath := resolvePersistedCommandsPath(cfg)
	store := cmdstore.NewCommandStore(dataPath)
	existing := store.List()
	if existing == nil || existing.SpecHash == "" {
		return nil
	}
	if existing.SpecHash != metadata.SpecHash {
		fmt.Fprintf(os.Stderr, "Warning: overwriting persisted commands from a different spec (existing hash: %s, new hash: %s)\n",
			existing.SpecHash, metadata.SpecHash)
	}
	return nil
}

// listPersistedStore prints the current persisted store info.
func listPersistedStore(cfg *config.Config) error {
	dataPath := resolvePersistedCommandsPath(cfg)
	store := cmdstore.NewCommandStore(dataPath)
	sd := store.List()
	if sd == nil || len(sd.Commands) == 0 {
		fmt.Println("No persisted commands found.")
		return nil
	}
	fmt.Printf("Store path:    %s\n", dataPath)
	fmt.Printf("Spec path:     %s\n", sd.SpecPath)
	fmt.Printf("Spec hash:     %s\n", sd.SpecHash)
	fmt.Printf("Generated at:  %s\n", sd.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("Command count: %d\n", len(sd.Commands))
	return nil
}

// clearPersistedStore removes the persisted store file.
func clearPersistedStore(cfg *config.Config) error {
	dataPath := resolvePersistedCommandsPath(cfg)
	store := cmdstore.NewCommandStore(dataPath)
	if err := store.Clear(); err != nil {
		return fmt.Errorf("clear store: %w", err)
	}
	fmt.Println("Persisted command store cleared.")
	return nil
}

// resolvePersistedCommandsPath returns the on-disk path used to store
// generated command metadata. Honours cfg.PersistedCommandsPath when set,
// otherwise falls back to $XDG_DATA_HOME/openapi-cli/commands.json.
func resolvePersistedCommandsPath(cfg *config.Config) string {
	if cfg != nil && cfg.PersistedCommandsPath != "" {
		return cfg.PersistedCommandsPath
	}
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		dataDir = filepath.Join(os.Getenv("HOME"), ".local", "share")
	}
	return filepath.Join(dataDir, "openapi-cli", "commands.json")
}
