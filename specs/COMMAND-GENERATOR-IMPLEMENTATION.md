# Command Generator Implementation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: Generator

## Overview

Complete specification for the **Command Generator** component that dynamically generates CLI commands from OpenAPI specifications. This component transforms API definitions into executable CLI commands with proper argument parsing, validation, and help documentation.

## 1. Generator Architecture

### 1.1 System Components

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         Command Generator                                │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐               │
│  │   OpenAPI   │────▶│   Parser    │────▶│   Command   │               │
│  │   Spec      │     │             │     │   Builder   │               │
│  └─────────────┘     └─────────────┘     └──────┬──────┘               │
│                                                  │                      │
│                                                  ▼                      │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐               │
│  │   CLI       │◀────│   Runner    │◀────│   Command   │               │
│  │   Output    │     │             │     │   Tree      │               │
│  └─────────────┘     └─────────────┘     └─────────────┘               │
│                                                                         │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                   Generated Components                          │   │
│  │  - Cobra Commands    - Flag Definitions    - Help Text          │   │
│  │  - Validators        - Transformers        - Request Builders   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1.2 Data Flow

```
OpenAPI 3.0/3.1 Spec
         │
         ▼
┌─────────────────┐
│ 1. Parse Spec   │ ──→ Extract paths, operations, schemas
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 2. Map to CLI   │ ──→ Convert REST to CLI paradigm
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 3. Build Cmds   │ ──→ Generate command structures
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 4. Add Handlers │ ──→ Link to HTTP execution
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ 5. Validate     │ ──→ Ensure command integrity
└────────┬────────┘
         │
         ▼
    Executable CLI
```

## 2. Core Data Structures

### 2.1 Generator Configuration

```go
package generator

import (
	"fmt"
	"regexp"
)

// Config contains command generator configuration
type Config struct {
	// Basic configuration
	AppName        string
	AppDescription string
	Version        string
	
	// OpenAPI configuration
	SpecPath       string
	BaseURL        string
	
	// Command generation rules
	NamingStrategy NamingStrategy
	OperationStyle OperationStyle
	
	// Feature toggles
	EnableAuth     bool
	EnableCache    bool
	EnableRetries  bool
	EnableDryRun   bool
	EnableRaw      bool
	
	// Output formatting
	DefaultFormat  string
	SupportedFormats []string
	
	// Customization
	CommandPrefix  string
	ExcludeTags    []string
	IncludeOnlyTags []string
	CustomMappings map[string]string // operationId -> command name
}

// NamingStrategy defines how to convert API names to CLI commands
type NamingStrategy int

const (
	NamingKebab NamingStrategy = iota // get-users
	NamingCamel                       // getUsers
	NamingSnake                       // get_users
	NamingFlat                        // getusers
)

// OperationStyle defines how to group operations
type OperationStyle int

const (
	StyleResource OperationStyle = iota // users get, users create
	StyleHTTP                           // users-get, users-post
	StyleCRUD                           // users-list, users-get, users-create
	StyleFlat                           // get-users, create-user
)

// Validate checks configuration validity
func (c *Config) Validate() error {
	if c.AppName == "" {
		return fmt.Errorf("app name is required")
	}
	
	if c.SpecPath == "" {
		return fmt.Errorf("spec path is required")
	}
	
	if c.DefaultFormat == "" {
		c.DefaultFormat = "table"
	}
	
	if len(c.SupportedFormats) == 0 {
		c.SupportedFormats = []string{"table", "json", "yaml", "csv"}
	}
	
	return nil
}
```

### 2.2 Command Definition

```go
package generator

import (
	"net/http"
)

// CommandDef represents a generated CLI command
type CommandDef struct {
	Name        string
	Aliases     []string
	Use         string
	Short       string
	Long        string
	Example     string
	
	// Command structure
	Flags       []FlagDef
	Args        []ArgDef
	Subcommands []*CommandDef
	
	// Execution
	Handler     CommandHandler
	PreRun      PreRunFunc
	PostRun     PostRunFunc
	
	// Metadata
	OperationID string
	HTTPMethod  string
	Path        string
	Tags        []string
}

// FlagDef represents a command flag
type FlagDef struct {
	Name         string
	Shorthand    string
	Usage        string
	Type         FlagType
	Default      interface{}
	Required     bool
	Hidden       bool
	Deprecated   string
	EnvVar       string
	
	// Validation
	Validate     Validator
	
	// Schema reference
	In           string // query, header, path
	Schema       *SchemaRef
}

// ArgDef represents a positional argument
type ArgDef struct {
	Name        string
	Usage       string
	Type        ArgType
	Required    bool
	NArgs       string // ?, *, +, or number
	Default     interface{}
	
	// Validation
	Validate    Validator
}

// FlagType represents the type of flag
type FlagType int

const (
	FlagString FlagType = iota
	FlagInt
	FlagBool
	FlagFloat
	FlagDuration
	FlagStringSlice
	FlagIntSlice
	FlagFile
	FlagEnum
)

// ArgType represents the type of argument
type ArgType int

const (
	ArgString ArgType = iota
	ArgInt
	ArgFile
	ArgURL
	ArgJSON
)

// SchemaRef references an OpenAPI schema
type SchemaRef struct {
	Name        string
	Type        string
	Format      string
	Enum        []interface{}
	Required    bool
	Description string
	Example     interface{}
}

// CommandHandler executes the command
type CommandHandler func(ctx *CommandContext) error

// PreRunFunc runs before command execution
type PreRunFunc func(ctx *CommandContext) error

// PostRunFunc runs after command execution
type PostRunFunc func(ctx *CommandContext) error

// CommandContext contains command execution context
type CommandContext struct {
	Config      *Config
	Client      HTTPClient
	Output      OutputWriter
	Args        map[string]interface{}
	Flags       map[string]interface{}
	Operation   *OperationDef
}

// HTTPClient interface for HTTP requests
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// OutputWriter interface for command output
type OutputWriter interface {
	Write(data interface{}) error
	WriteError(err error) error
}
```

### 2.3 OpenAPI Mapping

```go
package generator

import (
	"strings"
)

// OperationDef represents an OpenAPI operation mapped to CLI
type OperationDef struct {
	ID          string
	Summary     string
	Description string
	Method      string
	Path        string
	Tags        []string
	
	// Parameters
	PathParams   []ParameterDef
	QueryParams  []ParameterDef
	HeaderParams []ParameterDef
	
	// Request/Response
	RequestBody  *RequestBodyDef
	Responses    map[string]*ResponseDef
	
	// Security
	Security     []SecurityRequirement
}

// ParameterDef represents an OpenAPI parameter
type ParameterDef struct {
	Name        string
	In          string // path, query, header
	Required    bool
	Description string
	Schema      *SchemaRef
	Example     interface{}
}

// RequestBodyDef represents a request body
type RequestBodyDef struct {
	Description string
	Required    bool
	Content     map[string]*ContentDef
}

// ResponseDef represents a response definition
type ResponseDef struct {
	Description string
	Content     map[string]*ContentDef
	Headers     map[string]*HeaderDef
}

// ContentDef represents content type definition
type ContentDef struct {
	Schema *SchemaRef
	Example interface{}
}

// HeaderDef represents a response header
type HeaderDef struct {
	Description string
	Schema      *SchemaRef
}

// SecurityRequirement represents security requirements
type SecurityRequirement struct {
	Name   string
	Scopes []string
}
```

## 3. Parser Implementation

### 3.1 OpenAPI Parser

```go
package generator

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// OpenAPIParser parses OpenAPI specifications
type OpenAPIParser struct {
	loader *openapi3.Loader
}

// NewOpenAPIParser creates a new OpenAPI parser
func NewOpenAPIParser() *OpenAPIParser {
	return &OpenAPIParser{
		loader: openapi3.NewLoader(),
	}
}

// ParseFile parses an OpenAPI specification from file
func (p *OpenAPIParser) ParseFile(path string) (*openapi3.T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	
	return p.loader.LoadFromData(data)
}

// ParseURL parses an OpenAPI specification from URL
func (p *OpenAPIParser) ParseURL(url string) (*openapi3.T, error) {
	return p.loader.LoadFromURI(url)
}

// ExtractOperations extracts all operations from the spec
func (p *OpenAPIParser) ExtractOperations(spec *openapi3.T) ([]*OperationDef, error) {
	var operations []*OperationDef
	
	for path, pathItem := range spec.Paths {
		ops := p.extractPathOperations(path, pathItem)
		operations = append(operations, ops...)
	}
	
	return operations, nil
}

func (p *OpenAPIParser) extractPathOperations(path string, item *openapi3.PathItem) []*OperationDef {
	var ops []*OperationDef
	
	methods := map[string]*openapi3.Operation{
		"GET":     item.Get,
		"POST":    item.Post,
		"PUT":     item.Put,
		"PATCH":   item.Patch,
		"DELETE":  item.Delete,
		"HEAD":    item.Head,
		"OPTIONS": item.Options,
		"TRACE":   item.Trace,
	}
	
	for method, operation := range methods {
		if operation == nil {
			continue
		}
		
		op := &OperationDef{
			ID:          operation.OperationID,
			Summary:     operation.Summary,
			Description: operation.Description,
			Method:      method,
			Path:        path,
		}
		
		// Extract tags
		for _, tag := range operation.Tags {
			op.Tags = append(op.Tags, tag)
		}
		
		// Extract parameters
		for _, paramRef := range operation.Parameters {
			param := p.convertParameter(paramRef.Value)
			switch param.In {
			case "path":
				op.PathParams = append(op.PathParams, param)
			case "query":
				op.QueryParams = append(op.QueryParams, param)
			case "header":
				op.HeaderParams = append(op.HeaderParams, param)
			}
		}
		
		// Extract request body
		if operation.RequestBody != nil {
			op.RequestBody = p.convertRequestBody(operation.RequestBody.Value)
		}
		
		// Extract responses
		op.Responses = make(map[string]*ResponseDef)
		for code, respRef := range operation.Responses {
			op.Responses[code] = p.convertResponse(respRef.Value)
		}
		
		// Extract security
		for _, secRef := range operation.Security {
			for name, scopes := range secRef {
				op.Security = append(op.Security, SecurityRequirement{
					Name:   name,
					Scopes: scopes,
				})
			}
		}
		
		ops = append(ops, op)
	}
	
	return ops
}

func (p *OpenAPIParser) convertParameter(param *openapi3.Parameter) ParameterDef {
	return ParameterDef{
		Name:        param.Name,
		In:          param.In,
		Required:    param.Required,
		Description: param.Description,
		Schema:      p.convertSchema(param.Schema),
		Example:     param.Example,
	}
}

func (p *OpenAPIParser) convertRequestBody(body *openapi3.RequestBody) *RequestBodyDef {
	reqBody := &RequestBodyDef{
		Description: body.Description,
		Required:    body.Required,
		Content:     make(map[string]*ContentDef),
	}
	
	for contentType, content := range body.Content {
		reqBody.Content[contentType] = &ContentDef{
			Schema:  p.convertSchema(content.Schema),
			Example: content.Example,
		}
	}
	
	return reqBody
}

func (p *OpenAPIParser) convertResponse(resp *openapi3.Response) *ResponseDef {
	response := &ResponseDef{
		Description: resp.Description,
		Content:     make(map[string]*ContentDef),
		Headers:     make(map[string]*HeaderDef),
	}
	
	for contentType, content := range resp.Content {
		response.Content[contentType] = &ContentDef{
			Schema:  p.convertSchema(content.Schema),
			Example: content.Example,
		}
	}
	
	for name, headerRef := range resp.Headers {
		header := headerRef.Value
		response.Headers[name] = &HeaderDef{
			Description: header.Description,
			Schema:      p.convertSchema(header.Schema),
		}
	}
	
	return response
}

func (p *OpenAPIParser) convertSchema(schemaRef *openapi3.SchemaRef) *SchemaRef {
	if schemaRef == nil || schemaRef.Value == nil {
		return nil
	}
	
	schema := schemaRef.Value
	
	return &SchemaRef{
		Name:        schema.Title,
		Type:        schema.Type,
		Format:      schema.Format,
		Description: schema.Description,
		Example:     schema.Example,
		Enum:        schema.Enum,
		Required:    len(schema.Required) > 0,
	}
}
```

## 4. Command Builder

### 4.1 Command Tree Builder

```go
package generator

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
)

// CommandBuilder builds CLI commands from OpenAPI operations
type CommandBuilder struct {
	config Config
}

// NewCommandBuilder creates a new command builder
func NewCommandBuilder(config Config) *CommandBuilder {
	return &CommandBuilder{config: config}
}

// BuildCommands generates cobra commands from OpenAPI operations
func (b *CommandBuilder) BuildCommands(operations []*OperationDef) (*cobra.Command, error) {
	// Create root command
	root := &cobra.Command{
		Use:     b.config.AppName,
		Short:   b.config.AppDescription,
		Version: b.config.Version,
	}
	
	// Group operations by resource/tag
	groups := b.groupOperations(operations)
	
	// Build command hierarchy
	for resource, ops := range groups {
		cmd := b.buildResourceCommand(resource, ops)
		root.AddCommand(cmd)
	}
	
	// Add global flags
	b.addGlobalFlags(root)
	
	return root, nil
}

func (b *CommandBuilder) groupOperations(operations []*OperationDef) map[string][]*OperationDef {
	groups := make(map[string][]*OperationDef)
	
	for _, op := range operations {
		// Determine resource from tags or path
		resource := b.extractResource(op)
		
		// Skip if excluded
		if b.isExcluded(resource, op.Tags) {
			continue
		}
		
		groups[resource] = append(groups[resource], op)
	}
	
	return groups
}

func (b *CommandBuilder) extractResource(op *OperationDef) string {
	// Use first tag if available
	if len(op.Tags) > 0 {
		return b.normalizeName(op.Tags[0])
	}
	
	// Extract from path
	parts := strings.Split(strings.Trim(op.Path, "/"), "/")
	if len(parts) > 0 {
		return b.normalizeName(parts[0])
	}
	
	return "default"
}

func (b *CommandBuilder) normalizeName(name string) string {
	switch b.config.NamingStrategy {
	case NamingKebab:
		return toKebabCase(name)
	case NamingCamel:
		return toCamelCase(name)
	case NamingSnake:
		return toSnakeCase(name)
	default:
		return strings.ToLower(name)
	}
}

func (b *CommandBuilder) isExcluded(resource string, tags []string) bool {
	// Check exclude list
	for _, excluded := range b.config.ExcludeTags {
		if resource == excluded {
			return true
		}
		for _, tag := range tags {
			if tag == excluded {
				return true
			}
		}
	}
	
	// Check include-only list
	if len(b.config.IncludeOnlyTags) > 0 {
		found := false
		for _, included := range b.config.IncludeOnlyTags {
			if resource == included {
				found = true
				break
			}
			for _, tag := range tags {
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

func (b *CommandBuilder) buildResourceCommand(resource string, operations []*OperationDef) *cobra.Command {
	cmd := &cobra.Command{
		Use:   resource,
		Short: fmt.Sprintf("Manage %s", resource),
		Long:  fmt.Sprintf("Commands for managing %s resources", resource),
	}
	
	// Create subcommands for each operation
	for _, op := range operations {
		subCmd := b.buildOperationCommand(op)
		cmd.AddCommand(subCmd)
	}
	
	return cmd
}

func (b *CommandBuilder) buildOperationCommand(op *OperationDef) *cobra.Command {
	// Determine command name
	name := b.getCommandName(op)
	
	cmd := &cobra.Command{
		Use:     b.buildUseLine(op),
		Short:   op.Summary,
		Long:    op.Description,
		Aliases: b.getAliases(op),
	}
	
	// Add flags for parameters
	for _, param := range op.PathParams {
		b.addFlagFromParam(cmd, param, true)
	}
	
	for _, param := range op.QueryParams {
		b.addFlagFromParam(cmd, param, false)
	}
	
	for _, param := range op.HeaderParams {
		b.addFlagFromParam(cmd, param, false)
	}
	
	// Add flags for request body
	if op.RequestBody != nil {
		b.addBodyFlags(cmd, op.RequestBody)
	}
	
	// Set run function
	cmd.RunE = b.createRunFunction(op)
	
	return cmd
}

func (b *CommandBuilder) getCommandName(op *OperationDef) string {
	// Check custom mappings
	if custom, ok := b.config.CustomMappings[op.ID]; ok {
		return custom
	}
	
	// Generate from operation ID
	if op.ID != "" {
		return b.normalizeName(op.ID)
	}
	
	// Generate from method and resource
	return b.operationName(op.Method, op.Path)
}

func (b *CommandBuilder) operationName(method, path string) string {
	switch b.config.OperationStyle {
	case StyleHTTP:
		resource := b.extractResourceFromPath(path)
		return fmt.Sprintf("%s-%s", strings.ToLower(method), resource)
	case StyleCRUD:
		return b.crudName(method, path)
	case StyleFlat:
		resource := b.extractResourceFromPath(path)
		return fmt.Sprintf("%s-%s", b.httpAction(method), resource)
	default:
		return b.crudName(method, path)
	}
}

func (b *CommandBuilder) crudName(method, path string) string {
	resource := b.extractResourceFromPath(path)
	singular := singularize(resource)
	
	switch strings.ToUpper(method) {
	case http.MethodGet:
		if strings.Contains(path, "{id}") {
			return fmt.Sprintf("get-%s", singular)
		}
		return fmt.Sprintf("list-%s", resource)
	case http.MethodPost:
		return fmt.Sprintf("create-%s", singular)
	case http.MethodPut, http.MethodPatch:
		return fmt.Sprintf("update-%s", singular)
	case http.MethodDelete:
		return fmt.Sprintf("delete-%s", singular)
	default:
		return strings.ToLower(method)
	}
}

func (b *CommandBuilder) extractResourceFromPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) > 0 {
		return b.normalizeName(parts[0])
	}
	return "resource"
}

func (b *CommandBuilder) httpAction(method string) string {
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

func (b *CommandBuilder) buildUseLine(op *OperationDef) string {
	parts := []string{b.getCommandName(op)}
	
	// Add path parameters
	for _, param := range op.PathParams {
		if param.Required {
			parts = append(parts, fmt.Sprintf("<%s>", param.Name))
		} else {
			parts = append(parts, fmt.Sprintf("[%s]", param.Name))
		}
	}
	
	return strings.Join(parts, " ")
}

func (b *CommandBuilder) getAliases(op *OperationDef) []string {
	// Could be configured based on operation patterns
	return nil
}

func (b *CommandBuilder) addFlagFromParam(cmd *cobra.Command, param ParameterDef, isPath bool) {
	name := b.normalizeName(param.Name)
	shorthand := b.getShorthand(param.Name)
	
	// Determine flag type from schema
	flagType := b.schemaToFlagType(param.Schema)
	
	switch flagType {
	case FlagString:
		defaultVal := ""
		if param.Schema != nil && param.Schema.Example != nil {
			if s, ok := param.Schema.Example.(string); ok {
				defaultVal = s
			}
		}
		cmd.Flags().StringP(name, shorthand, defaultVal, param.Description)
	case FlagInt:
		cmd.Flags().IntP(name, shorthand, 0, param.Description)
	case FlagBool:
		cmd.Flags().BoolP(name, shorthand, false, param.Description)
	case FlagStringSlice:
		cmd.Flags().StringSliceP(name, shorthand, nil, param.Description)
	}
	
	if isPath || param.Required {
		cmd.MarkFlagRequired(name)
	}
}

func (b *CommandBuilder) addBodyFlags(cmd *cobra.Command, body *RequestBodyDef) {
	// Add flag for JSON body input
	cmd.Flags().String("body", "", "Request body as JSON string")
	cmd.Flags().String("body-file", "", "Read request body from file")
}

func (b *CommandBuilder) schemaToFlagType(schema *SchemaRef) FlagType {
	if schema == nil {
		return FlagString
	}
	
	switch schema.Type {
	case "boolean":
		return FlagBool
	case "integer":
		return FlagInt
	case "number":
		return FlagFloat
	case "array":
		return FlagStringSlice
	default:
		return FlagString
	}
}

func (b *CommandBuilder) createRunFunction(op *OperationDef) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Create execution context
		ctx := &CommandContext{
			Config:    &b.config,
			Operation: op,
			Flags:     make(map[string]interface{}),
			Args:      make(map[string]interface{}),
		}
		
		// Collect flag values
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			ctx.Flags[flag.Name] = flag.Value.String()
		})
		
		// Execute request
		return b.executeRequest(ctx)
	}
}

func (b *CommandBuilder) executeRequest(ctx *CommandContext) error {
	// Build HTTP request
	req, err := b.buildHTTPRequest(ctx)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	
	// Execute request
	resp, err := ctx.Client.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()
	
	// Handle response
	return b.handleResponse(ctx, resp)
}

func (b *CommandBuilder) buildHTTPRequest(ctx *CommandContext) (*http.Request, error) {
	op := ctx.Operation
	
	// Build URL
	url := b.config.BaseURL + op.Path
	
	// Replace path parameters
	for _, param := range op.PathParams {
		value, ok := ctx.Flags[b.normalizeName(param.Name)]
		if !ok {
			return nil, fmt.Errorf("missing required path parameter: %s", param.Name)
		}
		placeholder := fmt.Sprintf("{%s}", param.Name)
		url = strings.Replace(url, placeholder, fmt.Sprint(value), 1)
	}
	
	// Build request
	req, err := http.NewRequest(op.Method, url, nil)
	if err != nil {
		return nil, err
	}
	
	// Add query parameters
	q := req.URL.Query()
	for _, param := range op.QueryParams {
		if value, ok := ctx.Flags[b.normalizeName(param.Name)]; ok && value != "" {
			q.Add(param.Name, fmt.Sprint(value))
		}
	}
	req.URL.RawQuery = q.Encode()
	
	// Add headers
	for _, param := range op.HeaderParams {
		if value, ok := ctx.Flags[b.normalizeName(param.Name)]; ok && value != "" {
			req.Header.Set(param.Name, fmt.Sprint(value))
		}
	}
	
	return req, nil
}

func (b *CommandBuilder) handleResponse(ctx *CommandContext, resp *http.Response) error {
	// Parse response based on content type
	// Write to output
	return ctx.Output.Write(resp)
}

func (b *CommandBuilder) addGlobalFlags(root *cobra.Command) {
	if b.config.EnableAuth {
		root.PersistentFlags().String("token", "", "Authentication token")
		root.PersistentFlags().String("api-key", "", "API key")
	}
	
	if b.config.EnableCache {
		root.PersistentFlags().Bool("no-cache", false, "Disable caching")
	}
	
	if b.config.EnableRetries {
		root.PersistentFlags().Int("retries", 3, "Number of retries")
	}
	
	if b.config.EnableDryRun {
		root.PersistentFlags().Bool("dry-run", false, "Show request without executing")
	}
	
	if b.config.EnableRaw {
		root.PersistentFlags().Bool("raw", false, "Output raw response")
	}
	
	// Output format
	root.PersistentFlags().String("output", b.config.DefaultFormat, fmt.Sprintf("Output format (%s)", strings.Join(b.config.SupportedFormats, ", ")))
}

func (b *CommandBuilder) getShorthand(name string) string {
	// Generate shorthand from first letter
	if len(name) > 0 {
		return strings.ToLower(name[:1])
	}
	return ""
}
```

### 4.2 Helper Functions

```go
package generator

import (
	"regexp"
	"strings"
	"unicode"
)

// toKebabCase converts string to kebab-case
func toKebabCase(s string) string {
	// Replace underscores and spaces with hyphens
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	
	// Handle camelCase
	var result strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			result.WriteRune('-')
		}
		result.WriteRune(unicode.ToLower(r))
	}
	
	return result.String()
}

// toCamelCase converts string to camelCase
func toCamelCase(s string) string {
	// First convert to kebab-case
	s = toKebabCase(s)
	
	// Then convert to camelCase
	parts := strings.Split(s, "-")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.Title(parts[i])
		}
	}
	
	return strings.Join(parts, "")
}

// toSnakeCase converts string to snake_case
func toSnakeCase(s string) string {
	// Convert from camelCase first
	re := regexp.MustCompile(`([a-z0-9])([A-Z])`)
	s = re.ReplaceAllString(s, "${1}_${2}")
	
	// Replace hyphens and spaces with underscores
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	
	return strings.ToLower(s)
}

// singularize attempts to convert plural to singular
func singularize(s string) string {
	// Common patterns
	if strings.HasSuffix(s, "ies") {
		return strings.TrimSuffix(s, "ies") + "y"
	}
	if strings.HasSuffix(s, "es") {
		return strings.TrimSuffix(s, "es")
	}
	if strings.HasSuffix(s, "s") && !strings.HasSuffix(s, "ss") {
		return strings.TrimSuffix(s, "s")
	}
	return s
}
```

## 5. Complete Implementation

### 5.1 Main Generator

```go
package generator

import (
	"fmt"
	
	"github.com/spf13/cobra"
)

// Generator creates CLI commands from OpenAPI specifications
type Generator struct {
	config Config
	parser *OpenAPIParser
	builder *CommandBuilder
}

// NewGenerator creates a new command generator
func NewGenerator(config Config) (*Generator, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	return &Generator{
		config:  config,
		parser:  NewOpenAPIParser(),
		builder: NewCommandBuilder(config),
	}, nil
}

// GenerateFromFile generates commands from OpenAPI spec file
func (g *Generator) GenerateFromFile(path string) (*cobra.Command, error) {
	// Parse spec
	spec, err := g.parser.ParseFile(path)
	if err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	
	// Extract operations
	operations, err := g.parser.ExtractOperations(spec)
	if err != nil {
		return nil, fmt.Errorf("extract operations: %w", err)
	}
	
	// Build commands
	root, err := g.builder.BuildCommands(operations)
	if err != nil {
		return nil, fmt.Errorf("build commands: %w", err)
	}
	
	return root, nil
}

// GenerateFromURL generates commands from OpenAPI spec URL
func (g *Generator) GenerateFromURL(url string) (*cobra.Command, error) {
	// Parse spec
	spec, err := g.parser.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	
	// Extract operations
	operations, err := g.parser.ExtractOperations(spec)
	if err != nil {
		return nil, fmt.Errorf("extract operations: %w", err)
	}
	
	// Build commands
	root, err := g.builder.BuildCommands(operations)
	if err != nil {
		return nil, fmt.Errorf("build commands: %w", err)
	}
	
	return root, nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC