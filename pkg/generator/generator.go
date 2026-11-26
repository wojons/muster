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
	"github.com/spf13/pflag"
	"github.com/wojons/muster/pkg/auth"
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

	// BaseURLOverride marks BaseURL as EXPLICITLY supplied by the caller
	// (e.g. `generate --base-url`) and therefore authoritative: every
	// generated operation command targets it, even when the spec declares
	// its own root/path/operation servers[0]. Without it the spec's server
	// keeps its historical precedence and BaseURL is only the fallback for
	// operations that declare no server (DF-007 semantics).
	BaseURLOverride bool
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

	// RawBodyFlagName is the --body/-b flag added to typed POST/PUT/PATCH
	// commands that have a JSON request body. When set, its raw JSON is
	// sent verbatim and the per-property body flags are ignored
	// (DF-MUSTER-5).
	RawBodyFlagName = "body"

	// RawBodyAnnotation is stamped on commands that register the
	// RawBodyFlagName flag, so the persistence layer (internal/builtin/
	// generate.go) tags the flag as a body-kind flag — not a query param —
	// when saving command metadata (DF-MUSTER-5).
	RawBodyAnnotation = "muster/raw-body-flag"

	// AuthFlagName is the runtime --auth flag attached to generated operation
	// commands (DF-023): its value names a stored credential (see 'auth add')
	// that is resolved at execution time and injected as a request header
	// (bearer → Authorization: Bearer <secret>, api-key → X-API-Key). It is a
	// runtime flag, not a spec parameter — persistence skips it so a reloaded
	// command re-registers it instead of leaking it as an ?auth= query param.
	AuthFlagName = "auth"

	// AuthFlagAnnotation marks a command whose --auth flag is the runtime
	// credential-attachment flag (not a spec parameter named "auth").
	AuthFlagAnnotation = "muster/auth-flag"

	// AnnotationBodyProperty is stamped on each per-property request body
	// flag with the SPEC property name it fills. It survives a collision
	// rename: a body property called `header` cannot be registered as
	// --header (the CLI owns that flag), so it becomes --header-2 and the
	// annotation is the only way to know which body key it belongs to
	// (QA-MUSTER-041).
	AnnotationBodyProperty = "muster/body-property"

	// AnnotationRequiredBodyProperty is stamped on each per-property request
	// body flag whose schema marks the property required (QA-MUSTER-036).
	// The persistence layer reads it to record FlagMetadata.Required so a
	// reloaded command can enforce requestBody.required + the schema
	// required list locally, before any network I/O.
	AnnotationRequiredBodyProperty = "muster/required-body-property"
)

// resolveStoredFn resolves a stored credential by name for the --auth
// flag. Package-level var (default auth.ResolveStored) so tests can seam
// it without touching the platform keychain; production paths always use
// the default.
var resolveStoredFn = auth.ResolveStored

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

	// QA-MUSTER-041: refuse loudly. An operation whose only request body is a
	// media type we cannot generate for would otherwise become an
	// argument-free command that silently sends Content-Length: 0 — the exact
	// defect this tick fixes. The command is still generated (so the rest of
	// the spec is usable); the operator learns about the gap here.
	if unsupported := unsupportedBodyOperations(ops); len(unsupported) > 0 {
		fmt.Fprintf(os.Stderr, "warning: %d operation(s) declare a request body media type that cannot be generated; their commands send no body (use --body-free APIs or file a bug): %s\n",
			len(unsupported), strings.Join(unsupported, ", "))
	}

	groups := g.groupOperations(ops)

	var commands []*cobra.Command
	for resource, ops := range groups {
		cmd := g.buildResourceCommand(resource, ops)
		commands = append(commands, cmd)
	}

	return commands, nil
}

// GenerateCommandsWithSourceURL is GenerateCommands plus DF-015 relative
// server resolution: when the spec was fetched over http(s) and declares a
// RELATIVE servers[].url (e.g. "/api/v3"), every server URL in the document
// is resolved against the spec fetch URL before operation extraction, so
// generated commands carry a usable absolute base URL. Pass specSourceURL
// as "" (local-file specs) to keep the historical behavior unchanged.
func (g *Generator) GenerateCommandsWithSourceURL(spec *openapi3.T, specSourceURL string) ([]*cobra.Command, error) {
	openapi.ApplySourceURLToDocument(spec, specSourceURL)
	return g.GenerateCommands(spec)
}

// unsupportedBodyOperations names the operations whose request body declares
// only a media type the generator cannot express (e.g. application/xml), in a
// stable "METHOD path (media type)" form for the generation-time warning.
func unsupportedBodyOperations(ops []*openapi.Operation) []string {
	var out []string
	for _, op := range ops {
		if op.UnsupportedBodyType == "" {
			continue
		}
		out = append(out, fmt.Sprintf("%s %s (%s)", op.Method, op.Path, op.UnsupportedBodyType))
	}
	sort.Strings(out)
	return out
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

	// DF-019: SilenceUsage keeps a runtime/HTTP error (e.g. a 404 from the
	// server) from dumping the full Usage/Examples/Flags block, which read
	// like a flag mistake to a scripting user. Flag/arg misuse still shows
	// usage via the custom FlagErrorFunc below, which un-silences usage for
	// that error class — same pattern as internal/builtin/request.go.
	cmd := &cobra.Command{
		Use:          usage,
		Short:        op.Summary,
		Long:         longDesc,
		Example:      example,
		SilenceUsage: true,
		RunE:         g.createRunHandler(op),
	}

	// Flag-parse errors (unknown flag, bad value) should still show usage.
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		c.SilenceUsage = false
		return err
	})

	// Arg-count errors (wrong number of positional args) should still show
	// usage: validate against the path-parameter count and un-silence on
	// failure (same pattern as internal/builtin/request.go's Args wrapper).
	if pathParamCount := len(g.extractPathParams(op.Path)); pathParamCount > 0 {
		cmd.Args = func(c *cobra.Command, args []string) error {
			if len(args) != pathParamCount {
				c.SilenceUsage = false
				return fmt.Errorf("accepts %d arg(s), received %d", pathParamCount, len(args))
			}
			return nil
		}
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
	if serverURL := g.effectiveBaseURL(op); serverURL != "" {
		cmd.Annotations[AnnotationOperationServer] = serverURL
	}

	g.addFlags(cmd, op)

	// DF-023: runtime credential attachment. --auth resolves a stored
	// credential (see 'auth add') at execution time and injects it as a
	// header (bearer → Authorization: Bearer <secret>, api-key →
	// X-API-Key). Skipped when the spec itself declares a parameter that
	// normalizes to "auth" — the spec's flag wins and must not be
	// redefined. The annotation marks the runtime flag so persistence
	// (internal/builtin/generate.go) skips it instead of forwarding it as
	// an ?auth= query parameter on reload (same pattern as --raw, DF-016).
	if cmd.Flags().Lookup(AuthFlagName) == nil {
		cmd.Flags().String(AuthFlagName, "", "Attach a stored credential by name (see 'auth add')")
		cmd.Annotations[AuthFlagAnnotation] = "true"
	}

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

func (g *Generator) uniqueSpecFlagName(cmd *cobra.Command, base string) string {
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if cmd.Flags().Lookup(candidate) == nil {
			return candidate
		}
	}
}

// reservedCLIFlagNames are the runtime flags every generated command carries
// besides its spec-derived ones: --output/--raw on the leaf, and
// --header/--base-url on the root (the reloaded-command loader registers
// --header/--base-url/--output/--auth itself).
//
// A spec parameter or body property that normalizes to one of these names must
// be renamed: registering it on the leaf leaves two flags of the same name in
// different flag sets, and the reloaded CLI panics at startup with "flag
// redefined: header" before printing ANY command (QA-MUSTER-041: Stripe's
// post-quotes body declares a `header` property).
var reservedCLIFlagNames = map[string]bool{
	"header":   true,
	"base-url": true,
	"output":   true,
	"raw":      true,
	"help":     true,
}

// claimSpecFlagName returns name, or a free "-N" variant when it collides with
// an already-registered flag or a CLI-reserved one.
func (g *Generator) claimSpecFlagName(cmd *cobra.Command, name string) string {
	if reservedCLIFlagNames[name] || cmd.Flags().Lookup(name) != nil {
		return g.uniqueSpecFlagName(cmd, name)
	}
	return name
}

// bodyPropertyFlag finds the flag registered for a request body property by its
// AnnotationBodyProperty stamp. Needed because the flag name can differ from
// the normalized property name after a collision rename (a `header` property
// becomes --header-2). Returns nil when no flag carries the stamp.
func (g *Generator) bodyPropertyFlag(cmd *cobra.Command, property string) *pflag.Flag {
	var found *pflag.Flag
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if found != nil {
			return
		}
		if ann := f.Annotations[AnnotationBodyProperty]; len(ann) > 0 && ann[0] == property {
			found = f
		}
	})
	return found
}

// addFlags adds CLI flags for the operation's parameters and request body properties.
// For multipart operations, adds a --file flag instead of per-property body flags.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/generator/generator.go
func (g *Generator) addFlags(cmd *cobra.Command, op *openapi.Operation) {
	// Register Muster-owned flags first so spec-derived names are allocated
	// around them rather than shadowing or disabling the global controls.
	//
	// QA-MUSTER-041: a form-encoded body counts as a body even when the schema
	// declares no properties (free-form form) — the -b/--body escape hatch is
	// then the only way to send it, and its usage text must name the media
	// type so a JSON body is not pasted into a form-encoded operation.
	formBody := op.BodyContentType == openapi.ContentTypeForm
	hasBody := (len(op.RequestBody) > 0 || op.BodyContentType != "") && isBodyMethod(op.Method)
	if hasBody && cmd.Flags().Lookup(RawBodyFlagName) == nil {
		usage := "Raw JSON request body (overrides individual body flags)"
		if formBody {
			usage = "Raw application/x-www-form-urlencoded request body, e.g. 'name=Foo&email=a@b.c' (overrides individual body flags)"
		}
		cmd.Flags().StringP(RawBodyFlagName, "b", "", usage)
		if cmd.Annotations == nil {
			cmd.Annotations = make(map[string]string)
		}
		cmd.Annotations[RawBodyAnnotation] = "true"
	}
	if cmd.Flags().Lookup("output") == nil {
		cmd.Flags().StringP("output", "o", g.config.DefaultFormat, fmt.Sprintf("Output format (%s)", strings.Join(g.config.SupportedFormats, ", ")))
	}
	if cmd.Flags().Lookup("raw") == nil {
		cmd.Flags().BoolP("raw", "r", false, "Output raw API response")
	}

	for _, param := range op.Parameters {
		if param.In == "path" {
			continue
		}

		flagName := g.normalizeFlagName(param.Name)
		shorthand := g.inferShorthand(param.Name)
		description := param.Description

		// DF-MUSTER-9: a second registration of the same flag panics pflag
		// ("flag redefined") and aborts generation for the WHOLE spec — one
		// duplicated parameter name is enough to lose every command. Keep the
		// first registration of a name and drop later ones; drop a shorthand
		// that is already taken instead of crashing.
		flagName = g.claimSpecFlagName(cmd, flagName)
		if shorthand != "" && cmd.Flags().ShorthandLookup(shorthand) != nil {
			shorthand = ""
		}

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
			// DF-MUSTER-9: skip a form field whose flag a parameter (or an
			// earlier field) already defined — re-registering panics pflag.
			if cmd.Flags().Lookup(flagName) != nil {
				continue
			}
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
		// DF-MUSTER-9 / QA-MUSTER-041: a body property named like an
		// operation parameter or a CLI control flag must not be registered
		// twice. The shared value is sent in both positions and the API's
		// documented precedence decides (DuckBrain: "The namespace query
		// parameter takes precedence over the optional body namespace
		// field"). Pass --body when the two positions must differ.
		flagName = g.claimSpecFlagName(cmd, flagName)
		shorthand := g.inferShorthand(bodyProp.Name)
		if shorthand != "" && cmd.Flags().ShorthandLookup(shorthand) != nil {
			shorthand = ""
		}
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

		// QA-MUSTER-036: surface required body properties in the Flags
		// table and record the fact for persistence. Enforcement stays in
		// the run handler (validateRequiredBodyProps) — a cobra-level
		// MarkFlagRequired would reject "--body '{...}'" before the handler
		// ever runs (DF-MUSTER-5) — but the help must show which properties
		// the API demands, and the persisted store needs the fact so
		// reloaded commands can enforce it locally.
		if bodyProp.Required {
			if f := cmd.Flags().Lookup(flagName); f != nil {
				f.Usage = strings.TrimSpace(f.Usage) + " (required)"
				if f.Annotations == nil {
					f.Annotations = map[string][]string{}
				}
				f.Annotations[AnnotationRequiredBodyProperty] = []string{"true"}
			}
		}

		// Required body properties are enforced by the run handler, not via
		// cobra's MarkFlagRequired: --body replaces the per-property flags
		// entirely when set, and a cobra-level requirement would reject
		// "--body '{...}'" before the handler ever runs (DF-MUSTER-5).

		// Stamp the spec property name on the flag: the registered name may
		// differ from the normalized property name after a collision rename
		// (QA-MUSTER-041), and both the run handler and persistence need the
		// real body key.
		if f := cmd.Flags().Lookup(flagName); f != nil {
			if f.Annotations == nil {
				f.Annotations = map[string][]string{}
			}
			f.Annotations[AnnotationBodyProperty] = []string{bodyProp.Name}
		}
	}

}

// isBodyMethod reports whether an HTTP method can carry a request body that
// generated flags populate. GET/HEAD/DELETE are excluded on purpose: Stripe's
// spec declares empty urlencoded bodies for its 273 GETs and 32 DELETEs, and
// turning those into body flags would send payloads to endpoints that read
// query parameters only.
func isBodyMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		return false
	}
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

	// QA-MUSTER-036: required body properties belong in the Basic usage
	// example — a bodyless POST against an operation whose schema declares
	// required properties is not runnable, and the runtime refuses it
	// before any network I/O.
	for _, bodyProp := range op.RequestBody {
		if !bodyProp.Required {
			continue
		}
		base += fmt.Sprintf(" --%s %s", g.normalizeFlagName(bodyProp.Name), bodyPropertyExampleValue(bodyProp))
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

// bodyPropertyExampleValue returns the example token for a required body
// property in generated examples: the schema default when declared,
// otherwise a lowercase placeholder derived from the property name
// (mirroring the path-parameter placeholder style).
func bodyPropertyExampleValue(bodyProp openapi.RequestBodyProperty) string {
	if bodyProp.Default != nil {
		return fmt.Sprintf("%v", bodyProp.Default)
	}
	return "<" + strings.ToLower(bodyProp.Name) + ">"
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

// effectiveBaseURL resolves the base URL a generated operation command
// targets, in precedence order:
//
//  1. Config.BaseURL when Config.BaseURLOverride is set — an EXPLICIT
//     caller-supplied URL (e.g. `generate --base-url`) outranks whatever
//     the spec declares, for every operation. A flag that promises an
//     override must not be silently discarded because the spec happens to
//     ship a servers[] entry.
//  2. op.ServerURL — the spec's operation/path/root servers[0].
//  3. Config.BaseURL — the fallback for specs that declare no server.
//
// Runtime overrides (--base-url on the generated command, MUSTER_BASE_URL)
// are applied by createRunHandler AFTER this resolution and still win over
// everything here.
func (g *Generator) effectiveBaseURL(op *openapi.Operation) string {
	if g.config.BaseURLOverride && g.config.BaseURL != "" {
		return g.config.BaseURL
	}
	if op != nil && op.ServerURL != "" {
		return op.ServerURL
	}
	return g.config.BaseURL
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

		// Target base URL: an explicit generate-time override wins over the
		// spec's servers[0] (see effectiveBaseURL); the spec's own server
		// wins otherwise, with the config URL as the fallback.
		baseURL := g.effectiveBaseURL(op)

		// Runtime override: --base-url flag wins, then MUSTER_BASE_URL env
		// var, then the spec/generate-time URL above.
		if override := runtimeBaseURL(cmd); override != "" {
			baseURL = override
		}
		// DF-015: fail with the actionable --base-url / MUSTER_BASE_URL hint
		// when the base URL is still empty or relative (spec declared a
		// relative servers[].url and nothing resolved it) instead of the bare
		// `unsupported protocol scheme ""` http error.
		if err := request.ValidateRequestBaseURL(baseURL); err != nil {
			return fmt.Errorf("cannot execute %s %s: %w", op.Method, op.Path, err)
		}
		builder := request.NewBuilder(baseURL, op.Path, op.Method)

		// DF-023: attach the stored credential named by --auth BEFORE
		// explicit header flags so a user-supplied header with the same
		// name keeps precedence (SetHeader overwrites).
		authName, _ := cmd.Flags().GetString(AuthFlagName)
		if cmd.Flags().Lookup(AuthFlagName) != nil && authName != "" {
			cred, err := resolveStoredFn(authName)
			if err != nil {
				return fmt.Errorf("resolve --auth: %w", err)
			}
			builder.SetHeader(cred.Name, cred.Value)
		}

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

			// QA-MUSTER-041: a multipart operation whose text fields were
			// supplied but no file was — send those fields instead of
			// falling through to a body path that has nothing to send.
			fieldsSet := false
			builder.WithContentType(request.ContentTypeMultipart)
			for _, field := range op.MultipartFields {
				flagName := g.normalizeFlagName(field.Name)
				if cmd.Flag(flagName) != nil && cmd.Flag(flagName).Changed {
					val, _ := cmd.Flags().GetString(flagName)
					builder.WithMultipartField(field.Name, val)
					fieldsSet = true
				}
			}
			if fieldsSet {
				return executeAndPrint(g, builder, cmd, ctx)
			}
			// No file and no fields — fall through to any non-multipart body.
		}

		// Build the request body from body property flags (POST/PUT/PATCH).
		// QA-MUSTER-041: the media type comes from the spec — a form-encoded
		// body must be urlencoded and carry its own Content-Type, otherwise
		// the values are dropped (JSON would be rejected by the API).
		if (len(op.RequestBody) > 0 || op.BodyContentType != "") && isBodyMethod(op.Method) {
			formBody := op.BodyContentType == openapi.ContentTypeForm
			// Raw --body/-b override (DF-MUSTER-5): when set, the
			// value is validated and sent verbatim; every per-property body
			// flag below is ignored. Required properties are satisfied by
			// the raw payload itself.
			if rawBody, err := g.rawBodyValue(cmd, op.BodyContentType); err != nil {
				return err
			} else if rawBody != "" {
				if formBody {
					builder.RawBody(rawBody, request.ContentTypeFormEncoded)
				} else {
					builder.JSONBody(rawBody)
					builder.SetHeader("Content-Type", "application/json")
				}
				return executeAndPrint(g, builder, cmd, ctx)
			}

			// Required body properties are enforced here, not via cobra
			// MarkFlagRequired (see addFlags): --body satisfies them in one
			// shot, so the individual flags must not be hard-required
			// (DF-MUSTER-5).
			if err := g.validateRequiredBodyProps(cmd, op); err != nil {
				return err
			}

			bodyMap := make(map[string]interface{})
			hasBodyFlag := false

			for _, bodyProp := range op.RequestBody {
				flagName := g.normalizeFlagName(bodyProp.Name)
				if flag := g.bodyPropertyFlag(cmd, bodyProp.Name); flag != nil {
					flagName = flag.Name
				}
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
				if formBody {
					// QA-MUSTER-041: urlencoded, repeated keys for arrays,
					// bracket notation for nested objects.
					builder.FormBody(request.FormValuesFromMap(bodyMap))
				} else {
					bodyJSON, err := json.Marshal(bodyMap)
					if err != nil {
						return fmt.Errorf("marshal request body: %w", err)
					}
					builder.JSONBody(string(bodyJSON))
					builder.SetHeader("Content-Type", "application/json")
				}
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

// rawBodyValue returns the --body/-b raw request body when the flag is set.
// The value is validated against the operation's body media type but returned
// verbatim — it is sent to the wire exactly as the caller wrote it
// (DF-MUSTER-5). Returns ("", nil) when the flag is absent or was not changed.
//
// QA-MUSTER-041: for a form-encoded operation the raw body is
// `key=value&key2=value2`, not JSON, and an empty/unparseable value is
// rejected instead of silently sending Content-Length: 0.
func (g *Generator) rawBodyValue(cmd *cobra.Command, contentType string) (string, error) {
	flag := cmd.Flags().Lookup(RawBodyFlagName)
	if flag == nil || !flag.Changed {
		return "", nil
	}
	raw, err := cmd.Flags().GetString(RawBodyFlagName)
	if err != nil {
		return "", fmt.Errorf("get flag %s: %w", RawBodyFlagName, err)
	}
	if contentType == openapi.ContentTypeForm {
		if _, err := request.ParseFormBody(raw); err != nil {
			return "", fmt.Errorf("invalid body for --%s: %w", RawBodyFlagName, err)
		}
		return raw, nil
	}
	if !json.Valid([]byte(raw)) {
		return "", fmt.Errorf("invalid JSON for --%s: value is not valid JSON (value: %q)", RawBodyFlagName, raw)
	}
	return raw, nil
}

// validateRequiredBodyProps errors when a required request body property's
// flag is unset. Called only when --body is absent: the raw payload
// satisfies every required property at once, so individual flags must not be
// cobra-required (DF-MUSTER-5).
func (g *Generator) validateRequiredBodyProps(cmd *cobra.Command, op *openapi.Operation) error {
	var missing []string
	for _, bodyProp := range op.RequestBody {
		if !bodyProp.Required {
			continue
		}
		flagName := g.normalizeFlagName(bodyProp.Name)
		flag := cmd.Flags().Lookup(flagName)
		if flag != nil && flag.Changed {
			continue
		}
		missing = append(missing, "--"+flagName)
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required body flag(s): %s (or provide --%s with raw JSON)", strings.Join(missing, ", "), RawBodyFlagName)
	}
	return nil
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

	// QA-MUSTER-036: log EVERY completed HTTP response — success or error —
	// before any body handling, so the INF line stays on stderr and stdout
	// carries only the payload on error paths too.
	logger := log.FromContext(ctx)
	logger.Info().
		Int("status_code", resp.StatusCode).
		Str("status", resp.Status).
		Msg("HTTP response received")

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
		// QA-MUSTER-036: a non-JSON ERROR body (e.g. the HTML 400 page a
		// Flask dev server answers with) must never reach stdout — it
		// corrupts piped consumers (`create-post -o json | jq`). On errors
		// the body travels to stderr as a snippet inside the returned
		// error; successful non-JSON payloads still dump raw.
		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP error: %s: %s", resp.Status, summarizeNonJSONBody(body))
		}
		_, _ = fmt.Fprintln(out, string(body))
		return nil
	}

	// Render the output in the requested format.
	// DF-039: mask credential-shaped values (cs_[a-z]{2}_… session keys,
	// "Bearer <token>") before any formatted rendering. --raw above is the
	// escape hatch that dumps the body untouched.
	masked := maskCredentialValues(data)
	switch format {
	case "table":
		printTableOutput(out, masked)
	case "json":
		pretty, err := json.MarshalIndent(masked, "", "  ")
		if err != nil {
			return fmt.Errorf("format JSON: %w", err)
		}
		_, _ = fmt.Fprintln(out, string(pretty))
	case "yaml":
		yamlBytes, err := yaml.Marshal(masked)
		if err != nil {
			// Fall back to indented JSON if YAML marshalling fails.
			pretty, _ := json.MarshalIndent(masked, "", "  ")
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

	return nil
}

// summarizeNonJSONBody condenses a non-JSON error response body into a short
// single-line snippet for the stderr error message: whitespace collapsed,
// capped at a fixed length (QA-MUSTER-036).
func summarizeNonJSONBody(body []byte) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	if s == "" {
		return "empty response body"
	}
	if len(s) > 160 {
		s = s[:157] + "..."
	}
	return s
}

// printTableOutput renders JSON data as an aligned table using tabwriter.
// Arrays of objects become rows with key headers; single objects become
// key/value rows; other types are printed as-is.
func printTableOutput(out io.Writer, data interface{}) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)

	switch v := data.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			_, _ = fmt.Fprintf(w, "%s	%v	\n", key, tableFormatValue(v[key]))
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

// tableCollectKeys returns the unique keys across all objects in alphabetical order.
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
	sort.Strings(keys)
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

// CommandCacheEnvVar overrides the on-disk location of the generated-command
// cache. It exists so deployments and tests can redirect the cache away from
// the default per-uid path under os.TempDir().
const CommandCacheEnvVar = "MUSTER_COMMAND_CACHE"

// commandCachePath resolves the on-disk location of the generated-command
// cache.
//
// The default is per-uid: <tmp>/muster-cache-commands-<uid>.json. A fixed
// <tmp>/muster-cache-commands.json is shared by every user of a host, and
// because persist() writes it 0600, leftovers owned by another uid (for
// example a prior tenant on a host with a shared /tmp) made
// InitializeDynamicCommands fail with "permission denied" instead of
// generating the command tree. MUSTER_COMMAND_CACHE overrides the location
// outright.
func commandCachePath() string {
	if override := os.Getenv(CommandCacheEnvVar); override != "" {
		return override
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("muster-cache-commands-%d.json", os.Getuid()))
}

func InitializeDynamicCommands(rootCmd *cobra.Command, specPath string) error {
	cache := NewCommandCache(commandCachePath())

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
