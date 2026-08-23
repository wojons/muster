// Package generator transforms OpenAPI specifications into executable CLI commands with intelligent naming,
// comprehensive flag generation, and seamless Cobra integration. It is the core component that bridges
// API definitions with user-facing command-line interfaces.
//
// # Overview
//
// The generator parses OpenAPI 3.x specifications and generates a complete CLI command tree where each
// API operation becomes a subcommand. It handles:
//
//   - Intelligent command naming from operation IDs, summaries, or paths
//   - Automatic flag generation from parameters (path, query, header, body)
//   - Type-safe flag definitions (string, int, bool, array)
//   - Required parameter validation
//   - Help text generation from descriptions
//   - Example usage generation
//   - Spec hash tracking for cache invalidation
//
// # Architecture
//
// The package consists of three main components:
//
// 1. Generator: Main transformation engine with configuration
// 2. CommandGenerator: Individual command generation from operations
// 3. Naming utilities: Strategies for converting operation names to CLI commands
//
// # Usage
//
// Create a generator with configuration:
//
//	gen := generator.New(&generator.Config{
//	    AppName:        "petstore-cli",
//	    AppDescription: "CLI for Pet Store API",
//	    Version:        "1.0.0",
//	    BaseURL:        "https://api.petstore.com",
//	    NamingStrategy: generator.NamingKebab,
//	    CommandPrefix:  "api",
//	})
//
// Generate commands from OpenAPI spec:
//
//	loader := openapi.NewLoader()
//	doc, err := loader.Load(context.Background(), "petstore.yaml")
//	if err != nil {
//	    return err
//	}
//
//	commands, err := gen.Generate(context.Background(), doc)
//	if err != nil {
//	    return err
//	}
//
// Integrate with Cobra root command:
//
//	rootCmd := &cobra.Command{Use: "petstore"}
//	for _, cmd := range commands {
//	    rootCmd.AddCommand(cmd)
//	}
//
// # Naming Strategies
//
// Supported naming strategies for command generation:
//
//   - NamingKebab: get-pet-by-id (default, most CLI-friendly)
//   - NamingCamel: getPetById
//   - NamingSnake: get_pet_by_id
//   - NamingFlat: getpetbyid
//
// # Operation Styles
//
// Supported operation naming styles:
//
//   - StyleResource: pet get, pet create (resource-based)
//   - StyleOperation: get-pet, create-pet (operation-based)
//   - StyleRESTful: get /pets, post /pets (HTTP method-based)
//
// # Flag Generation
//
// Flags are automatically generated from OpenAPI parameters:
//
//   - Path parameters → Required positional args or --flag
//   - Query parameters → Optional --flags
//   - Header parameters → Optional --flags (hidden by default)
//   - Body parameters → --body flag or individual flags per property
//
// Example generated flags:
//
//	petstore-cli api pet get --id 123 --include-owner --format json
//
// # Spec Hash Tracking
//
// Generated commands include a spec hash for cache invalidation. When the OpenAPI spec
// changes, the hash changes, triggering regeneration:
//
//	hash := gen.calculateSpecHash(doc)
//	if hash != storedHash {
//	    // Regenerate commands
//	}
//
// # Error Handling
//
// The generator provides detailed error messages for:
//
//   - Invalid OpenAPI specifications
//   - Missing required fields
//   - Circular references
//   - Unsupported features
//   - Naming conflicts
//
// # Thread Safety
//
// Generator instances are NOT thread-safe. Create separate instances for concurrent generation.
package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/log"
	"github.com/wojons/muster/pkg/openapi"
	"github.com/wojons/muster/pkg/request"
	"gopkg.in/yaml.v3"
)

type Config struct {
	AppName          string
	AppDescription   string
	Version          string
	BaseURL          string
	NamingStrategy   NamingStrategy
	OperationStyle   OperationStyle
	DefaultFormat    string
	SupportedFormats []string
	CommandPrefix    string
	ExcludeTags      []string
	IncludeOnlyTags  []string
	CustomMappings   map[string]string

	// APIName is the group-command name that generated commands are
	// nested under by the consumer (e.g. "tiny-test-api"). When non-empty,
	// buildExample emits the full "AppName APIName resource op …" path so
	// the help Examples are directly runnable. When empty, the legacy
	// "AppName resource op …" format is preserved (backward compat).
	APIName string
}

// Annotation keys stamped onto generated leaf commands so downstream
// consumers (CLI metadata extraction, persistence, docs) can recover the
// exact operation path/method/server URL instead of reverse-inferring them
// from command names. Name-based inference mangles operationId-less specs
// (doubled-s paths, truncated names, wrong base URLs — see DF-007), so the
// generator always carries the ground truth on the command itself.
const (
	AnnotationOperationPath   = "muster/openapi-path"
	AnnotationOperationMethod = "muster/openapi-method"
	AnnotationOperationServer = "muster/openapi-server"

	// AnnotationOperationExample carries the FLAT (group-less) example
	// string for the operation. The persisted command store reloads every
	// operation command flat at the root (no resource groups exist there),
	// so the grouped form in the cobra Example field — "AppName resource
	// name …" — references a non-existent group command and cannot be run
	// verbatim. Persistence consumers must prefer this annotation so the
	// help users actually see contains examples that run (DF-010).
	AnnotationOperationExample = "muster/openapi-example"
)

type NamingStrategy int

const (
	NamingKebab NamingStrategy = iota
	NamingCamel
	NamingSnake
	NamingFlat
)

type OperationStyle int

const (
	StyleResource OperationStyle = iota
	StyleHTTP
	StyleCRUD
	StyleFlat
)

func (c *Config) Validate() error {
	if c.AppName == "" {
		return fmt.Errorf("app name is required")
	}

	if c.BaseURL == "" {
		return fmt.Errorf("base URL is required")
	}

	if c.DefaultFormat == "" {
		c.DefaultFormat = "table"
	}

	if len(c.SupportedFormats) == 0 {
		c.SupportedFormats = []string{"table", "json", "yaml"}
	}

	return nil
}

type Generator struct {
	config Config
}

func NewGenerator(config Config) (*Generator, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Generator{config: config}, nil
}

func (g *Generator) GenerateCommands(spec *openapi3.T) ([]*cobra.Command, error) {
	parser := &openapi.Parser{}
	ops, err := parser.ExtractOperations(spec)
	if err != nil {
		return nil, fmt.Errorf("extract operations: %w", err)
	}

	groups := g.groupOperations(ops)

	var commands []*cobra.Command
	for resource, ops := range groups {
		cmd := g.buildResourceCommand(resource, ops)
		commands = append(commands, cmd)
	}

	return commands, nil
}

func (g *Generator) groupOperations(ops []*openapi.Operation) map[string][]*openapi.Operation {
	groups := make(map[string][]*openapi.Operation)

	for _, op := range ops {
		resource := g.extractResource(op)

		if g.isExcluded(resource, op.Tags) {
			continue
		}

		groups[resource] = append(groups[resource], op)
	}

	return groups
}

func (g *Generator) extractResource(op *openapi.Operation) string {
	if len(op.Tags) > 0 {
		return g.normalizeName(op.Tags[0])
	}

	parts := strings.Split(strings.Trim(op.Path, "/"), "/")
	if len(parts) > 0 {
		return g.normalizeName(parts[0])
	}

	return "default"
}

func (g *Generator) isExcluded(resource string, tags []string) bool {
	for _, excluded := range g.config.ExcludeTags {
		if resource == excluded {
			return true
		}
		for _, tag := range tags {
			if tag == excluded {
				return true
			}
		}
	}

	if len(g.config.IncludeOnlyTags) > 0 {
		found := false
		for _, included := range g.config.IncludeOnlyTags {
			if resource == included {
				found = true
				break
			}
		}
		for _, tag := range tags {
			for _, included := range g.config.IncludeOnlyTags {
				if tag == included {
					found = true
					break
				}
			}
		}
		if !found {
			return true
		}
	}

	return false
}

func (g *Generator) buildResourceCommand(resource string, operations []*openapi.Operation) *cobra.Command {
	cmd := &cobra.Command{
		Use:   resource,
		Short: fmt.Sprintf("Manage %s", resource),
		Long:  fmt.Sprintf("Commands for managing %s resources", resource),
	}

	// Runtime base-URL override: an explicit --base-url flag (or the
	// MUSTER_BASE_URL env var) takes precedence over the spec's
	// servers[].url / generate-time --base-url, so generated CLIs can be
	// pointed at staging/prod without regenerating.
	cmd.PersistentFlags().String("base-url", "", "Override the API base URL at runtime (env: MUSTER_BASE_URL)")

	// Sort operations deterministically before building leaves. The spec
	// walk (spec.Paths.Map()) has no stable order; a fixed (path, method)
	// order makes the collision-resolution below deterministic instead of
	// depending on Go map iteration.
	sorted := make([]*openapi.Operation, len(operations))
	copy(sorted, operations)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		return sorted[i].Method < sorted[j].Method
	})

	// Track names within the group so no operation is shadowed by a
	// collision (DF-008): e.g. PATCH and PUT on the same path both map to
	// "update-<resource>", and nested resources used to collapse onto the
	// parent's name (create-project ×3 on the task-forge spec).
	usedNames := make(map[string]bool)
	for _, op := range sorted {
		subCmd := g.buildOperationCommand(op, usedNames)
		cmd.AddCommand(subCmd)
	}

	return cmd
}

func (g *Generator) buildOperationCommand(op *openapi.Operation, usedNames map[string]bool) *cobra.Command {
	name := g.getCommandName(op)
	if usedNames != nil {
		name = uniqueCommandName(name, op.Method, usedNames)
	}

	usage := g.buildUsage(name, op)
	longDesc := g.buildLongDescription(op)
	example := g.buildExample(name, op)

	cmd := &cobra.Command{
		Use:     usage,
		Short:   op.Summary,
		Long:    longDesc,
		Example: example,
		RunE:    g.createRunHandler(op),
	}

	// Stamp the exact operation metadata so downstream consumers
	// (persistence, docs) never have to re-infer path/method/server from
	// the command name (DF-007). Also stamp the FLAT example: the persisted
	// store loads every operation flat at the root, where the grouped
	// cobra Example would reference a non-existent group (DF-010).
	cmd.Annotations = map[string]string{
		AnnotationOperationPath:    op.Path,
		AnnotationOperationMethod:  op.Method,
		AnnotationOperationExample: g.buildFlatExample(name, op),
	}
	if op.ServerURL != "" {
		cmd.Annotations[AnnotationOperationServer] = op.ServerURL
	}

	g.addFlags(cmd, op)

	return cmd
}

// uniqueCommandName returns base, or a method/ordinal-suffixed variant when
// base is already taken within the same resource group. Guarantees every
// operation in a spec maps to exactly one reachable typed command even when
// two operations produce the same fallback/operationId name (DF-008).
func uniqueCommandName(base, method string, used map[string]bool) string {
	if !used[base] {
		used[base] = true
		return base
	}
	candidate := fmt.Sprintf("%s-%s", base, strings.ToLower(method))
	for i := 2; used[candidate]; i++ {
		candidate = fmt.Sprintf("%s-%s-%d", base, strings.ToLower(method), i)
	}
	used[candidate] = true
	return candidate
}

// addFlags adds CLI flags for the operation's parameters and request body properties.
// For multipart operations, adds a --file flag instead of per-property body flags.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/generator/generator.go
func (g *Generator) addFlags(cmd *cobra.Command, op *openapi.Operation) {
	for _, param := range op.Parameters {
		if param.In == "path" {
			continue
		}

		flagName := g.normalizeFlagName(param.Name)
		shorthand := g.inferShorthand(param.Name)
		description := param.Description

		switch param.Type {
		case "string":
			g.addStringFlag(cmd, flagName, shorthand, &param, description)
		case "integer":
			defaultVal := 0
			if param.Default != nil {
				if v, ok := param.Default.(float64); ok {
					defaultVal = int(v)
				}
			}
			cmd.Flags().IntP(flagName, shorthand, defaultVal, description)
		case "number":
			defaultVal := 0.0
			if param.Default != nil {
				defaultVal, _ = param.Default.(float64)
			}
			cmd.Flags().Float64P(flagName, shorthand, defaultVal, description)
		case "boolean":
			defaultVal := false
			if param.Default != nil {
				defaultVal, _ = param.Default.(bool)
			}
			cmd.Flags().BoolP(flagName, shorthand, defaultVal, description)
		case "array":
			cmd.Flags().StringSliceP(flagName, shorthand, nil, description)
		}

		if param.Required {
			_ = cmd.MarkFlagRequired(flagName)
		}
	}

	// For multipart operations, add --file flag alongside any JSON body flags
	if op.HasMultiPart {
		cmd.Flags().StringP("file", "f", "", "File path to upload (multipart/form-data)")
		// Add non-file multipart form fields
		for _, field := range op.MultipartFields {
			flagName := g.normalizeFlagName(field.Name)
			description := field.Description
			if description == "" {
				description = fmt.Sprintf("%s (form field)", field.Name)
			}
			cmd.Flags().StringP(flagName, "", "", description)
		}
	}

	// Add flags for request body properties (for POST/PUT/PATCH operations)
	// Build a set of multipart flag names to avoid duplicates
	multipartFlagNames := make(map[string]bool)
	for _, field := range op.MultipartFields {
		multipartFlagNames[g.normalizeFlagName(field.Name)] = true
	}

	for _, bodyProp := range op.RequestBody {
		flagName := g.normalizeFlagName(bodyProp.Name)
		// Skip if already added as a multipart field
		if multipartFlagNames[flagName] {
			continue
		}
		shorthand := g.inferShorthand(bodyProp.Name)
		description := bodyProp.Description
		if description == "" {
			description = fmt.Sprintf("%s (request body)", bodyProp.Name)
		}

		switch bodyProp.Type {
		case "string":
			defaultVal := ""
			if bodyProp.Default != nil {
				defaultVal = fmt.Sprintf("%v", bodyProp.Default)
			}
			if len(bodyProp.Enum) > 0 {
				allowed := make([]string, len(bodyProp.Enum))
				for i, v := range bodyProp.Enum {
					allowed[i] = fmt.Sprintf("%v", v)
				}
				description = fmt.Sprintf("%s (allowed: %s)", description, strings.Join(allowed, ", "))
			}
			cmd.Flags().StringP(flagName, shorthand, defaultVal, description)
		case "integer":
			defaultVal := 0
			if bodyProp.Default != nil {
				switch v := bodyProp.Default.(type) {
				case float64:
					defaultVal = int(v)
				case int:
					defaultVal = v
				case int64:
					defaultVal = int(v)
				}
			}
			cmd.Flags().IntP(flagName, shorthand, defaultVal, description)
		case "number":
			defaultVal := 0.0
			if bodyProp.Default != nil {
				if v, ok := bodyProp.Default.(float64); ok {
					defaultVal = v
				}
			}
			cmd.Flags().Float64P(flagName, shorthand, defaultVal, description)
		case "boolean":
			defaultVal := false
			if bodyProp.Default != nil {
				if v, ok := bodyProp.Default.(bool); ok {
					defaultVal = v
				}
			}
			cmd.Flags().BoolP(flagName, shorthand, defaultVal, description)
		case "array":
			description = fmt.Sprintf("%s — JSON array or comma-separated", description)
			cmd.Flags().StringP(flagName, shorthand, "", description)
		case "object":
			description = fmt.Sprintf("%s — JSON object", description)
			cmd.Flags().StringP(flagName, shorthand, "", description)
		default:
			cmd.Flags().StringP(flagName, shorthand, "", description)
		}

		if bodyProp.Required {
			_ = cmd.MarkFlagRequired(flagName)
		}
	}

	cmd.Flags().StringP("output", "o", g.config.DefaultFormat, fmt.Sprintf("Output format (%s)", strings.Join(g.config.SupportedFormats, ", ")))
	cmd.Flags().BoolP("raw", "r", false, "Output raw API response")
}

func (g *Generator) addStringFlag(cmd *cobra.Command, name, shorthand string, param *openapi.Parameter, description string) {

	if len(param.Enum) > 0 {
		allowed := make([]string, len(param.Enum))
		for i, v := range param.Enum {
			allowed[i] = fmt.Sprintf("%v", v)
		}
		description = fmt.Sprintf("%s (allowed: %s)", description, strings.Join(allowed, ", "))

		defaultVal := ""
		if param.Default != nil {
			defaultVal, _ = param.Default.(string)
		}
		cmd.Flags().StringP(name, shorthand, defaultVal, description)
		return
	}

	defaultVal := ""
	if param.Default != nil {
		defaultVal, _ = param.Default.(string)
	}
	cmd.Flags().StringP(name, shorthand, defaultVal, description)
}

// pathParams returns the operation's path parameters as positional-arg
// metadata: declared path parameters first (with spec descriptions),
// followed by any path-template parameters the spec never declared (e.g.
// /projects/{project_id} with no parameters block). The runtime treats
// both forms as positional (see extractPathParams and the loader's
// extractPathParamsFromPath), so help must document both; undeclared
// template parameters are required by definition — they are baked into
// the URL template (DF-010).
func (g *Generator) pathParams(op *openapi.Operation) []openapi.Parameter {
	var params []openapi.Parameter
	seen := make(map[string]bool)
	for _, param := range op.Parameters {
		if param.In != "path" {
			continue
		}
		params = append(params, param)
		seen[param.Name] = true
	}
	for _, name := range g.extractPathParams(op.Path) {
		if seen[name] {
			continue
		}
		params = append(params, openapi.Parameter{Name: name, In: "path", Required: true})
	}
	return params
}

func (g *Generator) buildUsage(name string, op *openapi.Operation) string {
	var args []string

	for _, param := range g.pathParams(op) {
		placeholder := fmt.Sprintf("<%s>", param.Name)
		if !param.Required {
			placeholder = fmt.Sprintf("[%s]", placeholder)
		}
		args = append(args, placeholder)
	}

	if len(args) > 0 {
		return fmt.Sprintf("%s %s [flags]", name, strings.Join(args, " "))
	}
	return fmt.Sprintf("%s [flags]", name)
}

func (g *Generator) buildLongDescription(op *openapi.Operation) string {
	var b strings.Builder

	if op.Description != "" {
		b.WriteString(op.Description)
		b.WriteString("\n\n")
	}

	b.WriteString("API Endpoint:\n")
	fmt.Fprintf(&b, "  %s %s\n\n", op.Method, op.Path)

	// Path parameters are consumed as POSITIONAL ARGS, not flags (the
	// addFlags pass skips them, and the loader matches them against
	// positional args at runtime). Document them in their own "Args:"
	// section so help shows how to invoke the command — a user otherwise
	// only discovers the positional via the runtime "expected N arguments"
	// error after trying a --flag that does not exist (DF-010). Template
	// parameters the spec never declared (e.g. /projects/{project_id} with
	// no parameters block) are included too: the runtime still requires
	// them positionally.
	pathArgs := g.pathParams(op)
	if len(pathArgs) > 0 {
		b.WriteString("Args:\n")
		for _, param := range pathArgs {
			required := ""
			if param.Required {
				required = " (required)"
			}
			if param.Description != "" {
				fmt.Fprintf(&b, "  %s%s: %s\n", param.Name, required, param.Description)
			} else {
				fmt.Fprintf(&b, "  %s%s\n", param.Name, required)
			}
		}
		b.WriteString("\n")
	}

	if len(op.Parameters) > 0 {
		b.WriteString("Parameters:\n")
		for _, param := range op.Parameters {
			required := ""
			if param.Required {
				required = " (required)"
			}
			fmt.Fprintf(&b, "  %s%s: %s\n", param.Name, required, param.Description)
		}
	}

	return b.String()
}

// buildExample emits the examples shown in the in-session (grouped) command
// tree: "AppName [APIName] resource name …". The resource group exists on
// that tree, so the examples run there.
func (g *Generator) buildExample(name string, op *openapi.Operation) string {
	return g.renderExample(name, op, true)
}

// buildFlatExample emits examples WITHOUT the resource group (and without
// APIName): the persisted command store reloads every operation command flat
// at the root, where "AppName resource name …" would reference a non-existent
// group command. The flat form is what persistence consumers must show in
// help so users can run examples verbatim (DF-010).
func (g *Generator) buildFlatExample(name string, op *openapi.Operation) string {
	return g.renderExample(name, op, false)
}

// renderExample builds the Examples text for an operation command. When
// grouped is true the resource (and APIName, when configured) prefix is
// included; when false the example is "AppName name …" — runnable against
// the flattened/persisted tree.
func (g *Generator) renderExample(name string, op *openapi.Operation, grouped bool) string {
	var examples []string

	base := fmt.Sprintf("%s %s", g.config.AppName, name)
	if grouped {
		resourceName := g.inferResourceName(op)
		if g.config.APIName != "" {
			base = fmt.Sprintf("%s %s %s %s", g.config.AppName, g.config.APIName, resourceName, name)
		} else {
			base = fmt.Sprintf("%s %s %s", g.config.AppName, resourceName, name)
		}
	}

	for _, param := range g.pathParams(op) {
		if param.Required {
			base += fmt.Sprintf(" <%s>", param.Name)
		}
	}

	examples = append(examples, fmt.Sprintf("  # Basic usage\n  %s", base))

	if g.hasQueryParams(op) {
		filterCmd := base
		for _, param := range op.Parameters {
			if param.In != "query" {
				continue
			}
			filterCmd += fmt.Sprintf(" --%s %s", param.Name, g.queryParamExampleValue(param))
		}
		examples = append(examples, fmt.Sprintf("\n  # With filters\n  %s", filterCmd))
	}

	examples = append(examples, fmt.Sprintf("\n  # JSON output\n  %s --output json", base))

	return strings.Join(examples, "\n")
}

// queryParamExampleValue returns the best example value for a query parameter,
// in order of preference: the schema Example, the first enum value, or the
// literal placeholder "<value>".
func (g *Generator) queryParamExampleValue(param openapi.Parameter) string {
	if param.Example != nil {
		return fmt.Sprintf("%v", param.Example)
	}
	if len(param.Enum) > 0 {
		return fmt.Sprintf("%v", param.Enum[0])
	}
	return "<value>"
}

func (g *Generator) inferResourceName(op *openapi.Operation) string {
	parts := strings.Split(op.Path, "/")
	for _, part := range parts {
		if part != "" && !strings.HasPrefix(part, "{") {
			return strings.ToLower(part)
		}
	}
	return "resource"
}

func (g *Generator) hasQueryParams(op *openapi.Operation) bool {
	for _, param := range op.Parameters {
		if param.In == "query" {
			return true
		}
	}
	return false
}

func (g *Generator) inferShorthand(name string) string {
	shorthandMap := map[string]string{
		"namespace": "n",
		"output":    "o",
		"format":    "f",
		"selector":  "l",
		"all":       "a",
		"watch":     "w",
		"follow":    "f",
		"help":      "h",
		"config":    "c",
		"verbose":   "v",
		"yes":       "y",
		"timeout":   "t",
	}

	normalized := g.normalizeFlagName(name)
	if sh, ok := shorthandMap[normalized]; ok {
		return sh
	}
	return ""
}

func (g *Generator) normalizeFlagName(name string) string {
	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('-')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func (g *Generator) getCommandName(op *openapi.Operation) string {
	if custom, ok := g.config.CustomMappings[op.ID]; ok {
		return custom
	}

	if op.ID != "" {
		return g.normalizeOperationID(op.ID)
	}

	return g.operationName(op.Method, op.Path)
}

func (g *Generator) operationName(method, path string) string {
	segments := fallbackPathSegments(path)
	if len(segments) == 0 {
		segments = []string{"resource"}
	}
	resource := strings.Join(segments, "-")

	switch g.config.OperationStyle {
	case StyleHTTP:
		return fmt.Sprintf("%s-%s", strings.ToLower(method), resource)
	case StyleFlat:
		return fmt.Sprintf("%s-%s", g.httpAction(method), resource)
	default:
		return g.crudName(method, path)
	}
}

// fallbackPathSegments returns the non-parameter segments of an operation
// path, e.g. /projects/{project_id}/tasks → [projects tasks]. Used to build
// collision-free fallback command names that include every path segment
// (verb-resource-{subresource}) instead of only the first one.
func fallbackPathSegments(path string) []string {
	var segments []string
	for _, part := range strings.Split(strings.Trim(path, "/"), "/") {
		if part == "" || (strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}")) {
			continue
		}
		segments = append(segments, part)
	}
	return segments
}

func (g *Generator) crudName(method, path string) string {
	segments := fallbackPathSegments(path)
	if len(segments) == 0 {
		segments = []string{"resource"}
	}

	// A trailing path parameter marks an item operation; everything else
	// targets a collection. GET /projects/{id}/tasks is a *list* of the
	// nested "tasks" resource, not a get of "projects".
	trimmed := strings.TrimSuffix(path, "/")
	item := strings.HasPrefix(trimmed, "{") || (strings.Contains(trimmed, "/{") && strings.HasSuffix(trimmed, "}"))

	verb := ""
	switch strings.ToUpper(method) {
	case http.MethodGet:
		if item {
			verb = "get"
		} else {
			verb = "list"
		}
	case http.MethodPost:
		verb = "create"
	case http.MethodPut, http.MethodPatch:
		verb = "update"
	case http.MethodDelete:
		verb = "delete"
	default:
		return strings.ToLower(method)
	}

	parts := make([]string, 0, len(segments)+1)
	parts = append(parts, verb)
	for i, segment := range segments {
		if i == len(segments)-1 {
			// Target resource: keep the authored collection form for
			// list-*, singularize it for item operations.
			if verb == "list" {
				parts = append(parts, segment)
			} else {
				parts = append(parts, singularize(segment))
			}
			continue
		}
		// Parent qualifiers are singularized (create-project-task).
		parts = append(parts, singularize(segment))
	}
	return strings.Join(parts, "-")
}

func (g *Generator) extractResourceFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 {
		return g.normalizeName(parts[0])
	}
	return "resource"
}

func (g *Generator) httpAction(method string) string {
	switch strings.ToUpper(method) {
	case http.MethodGet:
		return "get"
	case http.MethodPost:
		return "post"
	case http.MethodPut:
		return "put"
	case http.MethodPatch:
		return "patch"
	case http.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

func (g *Generator) normalizeName(name string) string {
	var result strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('-')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func (g *Generator) normalizeOperationID(id string) string {
	var result []rune
	for i, r := range id {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '-')
		}
		result = append(result, r)
	}
	return strings.ToLower(string(result))
}

// runtimeBaseURL resolves the runtime base-URL override for generated
// commands: an explicit --base-url persistent flag (registered on the
// resource command) wins, then the MUSTER_BASE_URL env var, then "" (no
// override — caller keeps the spec/generate-time URL). The flag walk stops
// at the command that owns the persistent flag, so a leaf operation's local
// flag with the same name (a spec query param literally named base-url)
// cannot be mistaken for the override.
func runtimeBaseURL(cmd *cobra.Command) string {
	for c := cmd; c != nil; c = c.Parent() {
		if pf := c.PersistentFlags().Lookup("base-url"); pf != nil {
			if pf.Changed {
				if v, _ := c.PersistentFlags().GetString("base-url"); v != "" {
					return v
				}
			}
			break
		}
	}
	return os.Getenv("MUSTER_BASE_URL")
}

func (g *Generator) createRunHandler(op *openapi.Operation) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Use per-operation server URL if available, falling back to config-level base URL
		baseURL := op.ServerURL
		if baseURL == "" {
			baseURL = g.config.BaseURL
		}

		// Runtime override: --base-url flag wins, then MUSTER_BASE_URL env
		// var, then the spec/generate-time URL above.
		if override := runtimeBaseURL(cmd); override != "" {
			baseURL = override
		}
		builder := request.NewBuilder(baseURL, op.Path, op.Method)

		pathParams := g.extractPathParams(op.Path)
		for i, param := range pathParams {
			if i < len(args) {
				builder.WithPathParam(param, args[i])
			} else {
				return fmt.Errorf("missing required argument: %s", param)
			}
		}

		for _, param := range op.Parameters {
			if param.In == "query" {
				value, err := g.getFlagValue(cmd, param.Name, param.Type)
				if err != nil {
					return fmt.Errorf("get flag %s: %w", param.Name, err)
				}
				flagName := g.normalizeFlagName(param.Name)
				if cmd.Flag(flagName) != nil && cmd.Flag(flagName).Changed {
					// DF-017: serialize slice query params as repeated keys
					// (?tags=a&tags=b), not Go's %v "[a b]".
					if slice, ok := value.([]string); ok && len(slice) > 0 {
						builder.WithQueryParamSlice(param.Name, slice)
					} else {
						builder.WithQueryParam(param.Name, fmt.Sprintf("%v", value))
					}
				}
			}
		}

		for _, param := range op.Parameters {
			if param.In == "header" {
				value, _ := cmd.Flags().GetString(g.normalizeFlagName(param.Name))
				if value != "" {
					builder.SetHeader(param.Name, value)
				}
			}
		}

		// Handle multipart file upload
		if op.HasMultiPart {
			filePath, _ := cmd.Flags().GetString("file")
			if filePath != "" {
				builder.WithFileUpload("file", filePath)
				// Add multipart non-file fields
				for _, field := range op.MultipartFields {
					flagName := g.normalizeFlagName(field.Name)
					if cmd.Flag(flagName) != nil && cmd.Flag(flagName).Changed {
						val, _ := cmd.Flags().GetString(flagName)
						builder.WithMultipartField(field.Name, val)
					}
				}
				return executeAndPrint(g, builder, cmd, ctx)
			}
			// No file provided — fall through to JSON body path below
		}

		// Build JSON body from request body property flags (for POST/PUT/PATCH)
		if len(op.RequestBody) > 0 &&
			(op.Method == http.MethodPost || op.Method == http.MethodPut || op.Method == http.MethodPatch) {
			bodyMap := make(map[string]interface{})
			hasBodyFlag := false

			for _, bodyProp := range op.RequestBody {
				flagName := g.normalizeFlagName(bodyProp.Name)
				if cmd.Flag(flagName) == nil {
					continue
				}

				switch bodyProp.Type {
				case "string":
					val, _ := cmd.Flags().GetString(flagName)
					if cmd.Flag(flagName).Changed {
						bodyMap[bodyProp.Name] = val
						hasBodyFlag = true
					}
				case "integer":
					val, _ := cmd.Flags().GetInt(flagName)
					if cmd.Flag(flagName).Changed {
						bodyMap[bodyProp.Name] = val
						hasBodyFlag = true
					}
				case "number":
					val, _ := cmd.Flags().GetFloat64(flagName)
					if cmd.Flag(flagName).Changed {
						bodyMap[bodyProp.Name] = val
						hasBodyFlag = true
					}
				case "boolean":
					val, _ := cmd.Flags().GetBool(flagName)
					if cmd.Flag(flagName).Changed {
						bodyMap[bodyProp.Name] = val
						hasBodyFlag = true
					}
				case "array":
					val, _ := cmd.Flags().GetString(flagName)
					if cmd.Flag(flagName).Changed {
						parsed, err := parseArrayBodyValue(flagName, val)
						if err != nil {
							return err
						}
						bodyMap[bodyProp.Name] = parsed
						hasBodyFlag = true
					}
				case "object":
					val, _ := cmd.Flags().GetString(flagName)
					if cmd.Flag(flagName).Changed {
						parsed, err := parseObjectBodyValue(flagName, val)
						if err != nil {
							return err
						}
						bodyMap[bodyProp.Name] = parsed
						hasBodyFlag = true
					}
				default:
					val, _ := cmd.Flags().GetString(flagName)
					if cmd.Flag(flagName).Changed {
						bodyMap[bodyProp.Name] = val
						hasBodyFlag = true
					}
				}
			}

			if hasBodyFlag {
				bodyJSON, err := json.Marshal(bodyMap)
				if err != nil {
					return fmt.Errorf("marshal request body: %w", err)
				}
				builder.JSONBody(string(bodyJSON))
				builder.SetHeader("Content-Type", "application/json")
			}
		}

		return executeAndPrint(g, builder, cmd, ctx)
	}
}

// parseObjectBodyValue parses a JSON object string flag value into an
// interface{} so that json.Marshal produces a real JSON object in the request
// body instead of a string. Returns an error if the value is not valid JSON.
func parseObjectBodyValue(flagName, raw string) (interface{}, error) {
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("invalid JSON for --%s: %w (value: %q)", flagName, err, raw)
	}
	return parsed, nil
}

// parseArrayBodyValue parses a string flag value into a slice suitable for
// the JSON request body. It accepts two forms:
//   - JSON array:   --labels '["a","b"]'
//   - CSV:           --labels a,b
//
// If the trimmed value starts with '[' it is parsed as JSON. Otherwise it is
// split on commas for backward compatibility with the previous pflag
// StringSlice behaviour.
func parseArrayBodyValue(flagName, raw string) (interface{}, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "[") {
		var parsed interface{}
		if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
			return nil, fmt.Errorf("invalid JSON array for --%s: %w (value: %q)", flagName, err, raw)
		}
		return parsed, nil
	}
	// CSV fallback — split on commas and trim spaces, matching pflag
	// StringSlice semantics which trim each element.
	parts := strings.Split(raw, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts, nil
}

// executeAndPrint sends the request and prints the response body, honoring the
// leaf command's --output format flag (table/json/yaml) and --raw flag.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/generator/generator.go
func executeAndPrint(g *Generator, builder *request.Builder, cmd *cobra.Command, ctx context.Context) error {
	resp, err := builder.Execute(ctx)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	out := cmd.OutOrStdout()

	// --raw flag: dump the body untouched regardless of format.
	raw, _ := cmd.Flags().GetBool("raw")
	if raw || len(body) == 0 {
		if len(body) > 0 {
			_, _ = fmt.Fprintln(out, string(body))
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP error: %s", resp.Status)
		}
		return nil
	}

	// --output flag: format (default: g.config.DefaultFormat → "table").
	format, _ := cmd.Flags().GetString("output")
	if format == "" {
		format = g.config.DefaultFormat
	}
	if format == "" {
		format = "table"
	}

	// Try to parse the body as JSON. If it's not valid JSON, dump raw
	// regardless of the requested format (same behavior as musterflow
	// ExecuteAndFormat).
	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		_, _ = fmt.Fprintln(out, string(body))
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP error: %s", resp.Status)
		}
		return nil
	}

	// Render the output in the requested format.
	switch format {
	case "table":
		printTableOutput(out, data)
	case "json":
		pretty, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("format JSON: %w", err)
		}
		_, _ = fmt.Fprintln(out, string(pretty))
	case "yaml":
		yamlBytes, err := yaml.Marshal(data)
		if err != nil {
			// Fall back to indented JSON if YAML marshalling fails.
			pretty, _ := json.MarshalIndent(data, "", "  ")
			_, _ = fmt.Fprintln(out, string(pretty))
		} else {
			_, _ = fmt.Fprint(out, string(yamlBytes))
		}
	default:
		return fmt.Errorf("unsupported output format %q: supported formats are table, json, yaml", format)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}

	// Log response status
	logger := log.FromContext(ctx)
	logger.Info().
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Msg("HTTP response received")

	return nil
}

// printTableOutput renders JSON data as an aligned table using tabwriter.
// Arrays of objects become rows with key headers; single objects become
// key/value rows; other types are printed as-is.
func printTableOutput(out io.Writer, data interface{}) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			_, _ = fmt.Fprintf(w, "%s	%v	\n", key, tableFormatValue(val))
		}
	case []interface{}:
		if len(v) == 0 {
			_, _ = fmt.Fprintln(out, "(empty)")
			return
		}
		keys := tableCollectKeys(v)
		_, _ = fmt.Fprintf(w, "%s	\n", strings.Join(keys, "	"))
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				var row []string
				for _, k := range keys {
					row = append(row, tableFormatValue(m[k]))
				}
				_, _ = fmt.Fprintf(w, "%s	\n", strings.Join(row, "	"))
			}
		}
	default:
		_, _ = fmt.Fprintf(w, "%v	\n", v)
	}
	_ = w.Flush()
}

// tableCollectKeys returns the ordered list of unique keys across all objects
// in the array.
func tableCollectKeys(items []interface{}) []string {
	seen := make(map[string]bool)
	var keys []string
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			for k := range m {
				if !seen[k] {
					seen[k] = true
					keys = append(keys, k)
				}
			}
		}
	}
	return keys
}

// tableFormatValue converts a value to a display string for table rendering.
func tableFormatValue(v interface{}) string {
	if v == nil {
		return "-"
	}
	// Render non-scalar values (maps/slices) as compact JSON instead of Go %v syntax.
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		b, err := json.Marshal(v)
		if err != nil {
			// Fallback to %v if JSON marshalling fails.
			break
		}
		s := string(b)
		if len(s) > 80 {
			return s[:77] + "..."
		}
		return s
	}
	// Render float64 without scientific notation so large integer IDs
	// (e.g. 90010011) display as whole numbers, not 9.0010011e+07.
	if f, ok := v.(float64); ok {
		s := strconv.FormatFloat(f, 'f', -1, 64)
		if len(s) > 80 {
			return s[:77] + "..."
		}
		return s
	}
	s := fmt.Sprintf("%v", v)
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

func (g *Generator) getFlagValue(cmd *cobra.Command, name, paramType string) (interface{}, error) {
	flagName := g.normalizeFlagName(name)

	switch paramType {
	case "string":
		return cmd.Flags().GetString(flagName)
	case "integer":
		return cmd.Flags().GetInt(flagName)
	case "number":
		return cmd.Flags().GetFloat64(flagName)
	case "boolean":
		return cmd.Flags().GetBool(flagName)
	case "array":
		return cmd.Flags().GetStringSlice(flagName)
	default:
		return cmd.Flags().GetString(flagName)
	}
}

func (g *Generator) extractPathParams(path string) []string {
	var params []string
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			params = append(params, part[1:len(part)-1])
		}
	}
	return params
}

func singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return strings.TrimSuffix(s, "ies") + "y"
	}
	// Strip "es" only for words whose stem ends in a sibilant
	// (box→boxes, class→classes, watch→watches). Plain s-plurals like
	// "milestones" must lose only the final "s" — the previous blanket
	// "es" rule truncated them to "mileston" (DF-007).
	if strings.HasSuffix(s, "es") && len(s) > 3 {
		stem := strings.TrimSuffix(s, "es")
		last := stem[len(stem)-1]
		if last == 's' || last == 'x' || last == 'z' ||
			strings.HasSuffix(stem, "ch") || strings.HasSuffix(stem, "sh") {
			return stem
		}
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") {
		return strings.TrimSuffix(s, "s")
	}
	return s
}

type cachedCommand struct {
	SpecHash  string
	ExpiresAt time.Time
}

type CommandCache struct {
	path     string
	metadata *cachedCommand
	mu       sync.RWMutex
	ttl      time.Duration
}

func NewCommandCache(path string) *CommandCache {
	cache := &CommandCache{
		path: path,
		ttl:  24 * time.Hour,
	}

	return cache
}

func (cc *CommandCache) Load(specPath string) (*cachedCommand, bool) {
	hash := computeHash(specPath)

	cc.mu.RLock()
	defer cc.mu.RUnlock()

	if cc.metadata != nil && cc.metadata.SpecHash == hash && time.Now().Before(cc.metadata.ExpiresAt) {
		return cc.metadata, true
	}
	return nil, false
}

func (cc *CommandCache) Save(specPath string, commands []*cobra.Command) error {
	hash := computeHash(specPath)

	cc.mu.Lock()
	defer cc.mu.Unlock()

	cc.metadata = &cachedCommand{
		SpecHash:  hash,
		ExpiresAt: time.Now().Add(cc.ttl),
	}

	return cc.persist()
}

func (cc *CommandCache) persist() error {
	if cc.metadata == nil {
		return nil
	}

	data, err := json.Marshal(cc.metadata)
	if err != nil {
		return err
	}

	return os.WriteFile(cc.path, data, 0600)
}

func computeHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func InitializeDynamicCommands(rootCmd *cobra.Command, specPath string) error {
	cache := NewCommandCache(filepath.Join(os.TempDir(), "muster-cache-commands.json"))

	_, cached := cache.Load(specPath)

	if !cached {
		loader := openapi3.NewLoader()
		doc, err := loader.LoadFromFile(specPath)
		if err != nil {
			return fmt.Errorf("load spec: %w", err)
		}

		config := Config{
			AppName:          "openapi-cli",
			AppDescription:   "Dynamic CLI from OpenAPI specs",
			Version:          "1.0.0",
			BaseURL:          "https://api.example.com",
			NamingStrategy:   NamingKebab,
			OperationStyle:   StyleCRUD,
			DefaultFormat:    "table",
			SupportedFormats: []string{"table", "json", "yaml"},
		}

		generator, err := NewGenerator(config)
		if err != nil {
			return fmt.Errorf("create generator: %w", err)
		}

		commands, err := generator.GenerateCommands(doc)
		if err != nil {
			return fmt.Errorf("generate commands: %w", err)
		}

		for _, cmd := range commands {
			rootCmd.AddCommand(cmd)
		}

		return cache.Save(specPath, commands)
	}

	return nil
}
