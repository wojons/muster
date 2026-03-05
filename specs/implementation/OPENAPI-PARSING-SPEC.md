# OpenAPI Parsing Specification

## Overview

Complete specification for parsing OpenAPI 3.0 specifications and generating CLI commands dynamically.

## 1. OpenAPI Parser

### File: `internal/core/openapi/parser.go`

```go
package openapi

import (
    "encoding/json"
    "fmt"
    "net/http"
    "regexp"
    "strings"
    
    "github.com/getkin/kin-openapi/openapi3"
    "github.com/spf13/cobra"
)

// Parser parses OpenAPI specifications
type Parser struct {
    loader *openapi3.Loader
}

// NewParser creates a new OpenAPI parser
func NewParser() *Parser {
    return &Parser{
        loader: openapi3.NewLoader(),
    }
}

// Parse parses an OpenAPI specification from bytes
func (p *Parser) Parse(spec []byte) (*Spec, error) {
    // Detect format
    var doc *openapi3.T
    var err error
    
    if isJSON(spec) {
        doc, err = p.loader.LoadFromData(spec)
    } else {
        doc, err = p.loader.LoadFromData(spec)
    }
    
    if err != nil {
        return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
    }
    
    // Validate the spec
    if err := doc.Validate(p.loader.Context); err != nil {
        return nil, fmt.Errorf("OpenAPI spec validation failed: %w", err)
    }
    
    return p.convertToSpec(doc)
}

// ParseURL fetches and parses an OpenAPI spec from URL
func (p *Parser) ParseURL(url string, headers map[string]string) (*Spec, error) {
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }
    
    for k, v := range headers {
        req.Header.Set(k, v)
    }
    
    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch OpenAPI spec: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("failed to fetch OpenAPI spec: HTTP %d", resp.StatusCode)
    }
    
    return p.ParseFromReader(resp.Body)
}

// convertToSpec converts openapi3.T to our Spec format
func (p *Parser) convertToSpec(doc *openapi3.T) (*Spec, error) {
    spec := &Spec{
        OpenAPI: doc.OpenAPI,
        Info: Info{
            Title:       doc.Info.Title,
            Description: doc.Info.Description,
            Version:     doc.Info.Version,
        },
        Servers: p.convertServers(doc.Servers),
        Tags:    p.convertTags(doc.Tags),
    }
    
    // Convert paths to operations
    operations := []Operation{}
    for path, pathItem := range doc.Paths {
        ops := p.convertPathItem(path, pathItem)
        operations = append(operations, ops...)
    }
    
    spec.Operations = operations
    
    return spec, nil
}

// convertServers converts openapi3.Servers to our format
func (p *Parser) convertServers(servers openapi3.Servers) []Server {
    result := make([]Server, len(servers))
    for i, s := range servers {
        result[i] = Server{
            URL:         s.URL,
            Description: s.Description,
        }
    }
    return result
}

// convertTags converts openapi3.Tags to our format
func (p *Parser) convertTags(tags openapi3.Tags) []Tag {
    result := make([]Tag, len(tags))
    for i, t := range tags {
        result[i] = Tag{
            Name:        t.Name,
            Description: t.Description,
        }
    }
    return result
}

// convertPathItem extracts operations from a path item
func (p *Parser) convertPathItem(path string, item *openapi3.PathItem) []Operation {
    operations := []Operation{}
    
    // Map HTTP methods to operations
    methods := map[string]*openapi3.Operation{
        "GET":     item.Get,
        "POST":    item.Post,
        "PUT":     item.Put,
        "PATCH":   item.Patch,
        "DELETE":  item.Delete,
        "HEAD":    item.Head,
        "OPTIONS": item.Options,
    }
    
    for method, op := range methods {
        if op == nil {
            continue
        }
        
        operation := p.convertOperation(path, method, op)
        operations = append(operations, operation)
    }
    
    return operations
}

// convertOperation converts a single OpenAPI operation
func (p *Parser) convertOperation(path, method string, op *openapi3.Operation) Operation {
    operation := Operation{
        OperationID: op.OperationID,
        Summary:     op.Summary,
        Description: op.Description,
        Method:      method,
        Path:        path,
        Tags:        op.Tags,
    }
    
    // Generate operation ID if not provided
    if operation.OperationID == "" {
        operation.OperationID = p.generateOperationID(path, method)
    }
    
    // Convert parameters
    operation.Parameters = p.convertParameters(op.Parameters)
    
    // Convert request body
    if op.RequestBody != nil {
        operation.RequestBody = p.convertRequestBody(op.RequestBody)
    }
    
    // Convert responses
    operation.Responses = p.convertResponses(op.Responses)
    
    return operation
}

// generateOperationID creates an operation ID from path and method
func (p *Parser) generateOperationID(path, method string) string {
    // Remove leading slash and replace slashes with underscores
    cleanPath := strings.TrimPrefix(path, "/")
    cleanPath = strings.ReplaceAll(cleanPath, "/", "_")
    cleanPath = strings.ReplaceAll(cleanPath, "{", "")
    cleanPath = strings.ReplaceAll(cleanPath, "}", "")
    
    // Convert to lowercase and replace special chars
    cleanPath = strings.ToLower(cleanPath)
    cleanPath = regexp.MustCompile(`[^a-z0-9_]`).ReplaceAllString(cleanPath, "_")
    cleanPath = regexp.MustCompile(`_+`).ReplaceAllString(cleanPath, "_")
    cleanPath = strings.Trim(cleanPath, "_")
    
    return fmt.Sprintf("%s_%s", strings.ToLower(method), cleanPath)
}

// convertParameters converts OpenAPI parameters
func (p *Parser) convertParameters(params openapi3.Parameters) []Parameter {
    result := make([]Parameter, len(params))
    for i, paramRef := range params {
        param := paramRef.Value
        result[i] = Parameter{
            Name:        param.Name,
            In:          param.In,
            Description: param.Description,
            Required:    param.Required,
            Schema:      p.convertSchema(param.Schema),
        }
    }
    return result
}

// convertRequestBody converts request body
func (p *Parser) convertRequestBody(body *openapi3.RequestBodyRef) *RequestBody {
    if body == nil || body.Value == nil {
        return nil
    }
    
    b := body.Value
    rb := &RequestBody{
        Description: b.Description,
        Required:    b.Required,
        Content:     make(map[string]MediaType),
    }
    
    for contentType, mediaType := range b.Content {
        rb.Content[contentType] = p.convertMediaType(mediaType)
    }
    
    return rb
}

// convertResponses converts responses
func (p *Parser) convertResponses(responses *openapi3.Responses) map[int]*Response {
    result := make(map[int]*Response)
    
    for code, respRef := range responses {
        if respRef == nil || respRef.Value == nil {
            continue
        }
        
        resp := respRef.Value
        statusCode := 0
        fmt.Sscanf(code, "%d", &statusCode)
        
        result[statusCode] = &Response{
            Description: resp.Description,
            Content:       make(map[string]MediaType),
        }
        
        for contentType, mediaType := range resp.Content {
            result[statusCode].Content[contentType] = p.convertMediaType(mediaType)
        }
    }
    
    return result
}

// convertSchema converts OpenAPI schema
func (p *Parser) convertSchema(schema *openapi3.SchemaRef) *Schema {
    if schema == nil {
        return nil
    }
    
    s := schema.Value
    result := &Schema{
        Type:        s.Type,
        Format:      s.Format,
        Description: s.Description,
        Default:     s.Default,
        Enum:        s.Enum,
        Required:    s.Required,
    }
    
    // Handle object properties
    if s.Type == "object" && s.Properties != nil {
        result.Properties = make(map[string]*Schema)
        for name, prop := range s.Properties {
            result.Properties[name] = p.convertSchema(prop)
        }
    }
    
    // Handle array items
    if s.Type == "array" && s.Items != nil {
        result.Items = p.convertSchema(s.Items)
    }
    
    return result
}

// convertMediaType converts media type
func (p *Parser) convertMediaType(mt *openapi3.MediaType) MediaType {
    return MediaType{
        Schema: p.convertSchema(mt.Schema),
    }
}

func isJSON(data []byte) bool {
    return len(data) > 0 && (data[0] == '{' || data[0] == '[')
}
```

## 2. Command Generator

### File: `internal/cli/generator.go`

```go
package cli

import (
    "fmt"
    "strings"
    
    "github.com/openapi-cli/core/internal/core/inventory"
    "github.com/openapi-cli/core/internal/core/openapi"
    "github.com/openapi-cli/core/internal/core/http"
    "github.com/spf13/cobra"
    "github.com/spf13/pflag"
)

// Generator generates CLI commands from OpenAPI spec
type Generator struct {
    inventoryManager inventory.Manager
    httpClient       http.Client
}

// NewGenerator creates a new command generator
func NewGenerator(invMgr inventory.Manager, httpClient http.Client) *Generator {
    return &Generator{
        inventoryManager: invMgr,
        httpClient:       httpClient,
    }
}

// GenerateCommands creates cobra commands from parsed OpenAPI spec
func (g *Generator) GenerateCommands(inv *inventory.Inventory) ([]*cobra.Command, error) {
    // Parse OpenAPI spec
    parser := openapi.NewParser()
    spec, err := parser.Parse(inv.OpenAPI.Content)
    if err != nil {
        return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
    }
    
    // Get base URL from spec
    baseURL := g.getBaseURL(spec, inv)
    
    // Group operations by tag
    groups := g.groupOperationsByTag(spec.Operations)
    
    commands := []*cobra.Command{}
    
    for tagName, operations := range groups {
        // Create group command
        groupCmd := &cobra.Command{
            Use:   g.sanitizeCommandName(tagName),
            Short: g.getTagDescription(spec, tagName),
            Long:  fmt.Sprintf("Commands for %s", tagName),
        }
        
        // Create commands for each operation
        for _, op := range operations {
            cmd := g.createCommand(op, baseURL)
            groupCmd.AddCommand(cmd)
        }
        
        commands = append(commands, groupCmd)
    }
    
    // Add commands with no tag to root
    untagged := g.getUntaggedOperations(spec.Operations)
    for _, op := range untagged {
        cmd := g.createCommand(op, baseURL)
        commands = append(commands, cmd)
    }
    
    return commands, nil
}

// groupOperationsByTag groups operations by their first tag
func (g *Generator) groupOperationsByTag(operations []openapi.Operation) map[string][]openapi.Operation {
    groups := make(map[string][]openapi.Operation)
    
    for _, op := range operations {
        tag := "default"
        if len(op.Tags) > 0 {
            tag = op.Tags[0]
        }
        
        groups[tag] = append(groups[tag], op)
    }
    
    return groups
}

// getUntaggedOperations returns operations with no tags
func (g *Generator) getUntaggedOperations(operations []openapi.Operation) []openapi.Operation {
    var result []openapi.Operation
    for _, op := range operations {
        if len(op.Tags) == 0 {
            result = append(result, op)
        }
    }
    return result
}

// createCommand creates a cobra command for an operation
func (g *Generator) createCommand(op openapi.Operation, baseURL string) *cobra.Command {
    cmd := &cobra.Command{
        Use:   g.sanitizeCommandName(op.OperationID),
        Short: op.Summary,
        Long:  g.buildLongDescription(op),
        Example: g.buildExamples(op),
        RunE: func(cmd *cobra.Command, args []string) error {
            return g.executeOperation(cmd, op, baseURL)
        },
    }
    
    // Add flags for parameters
    for _, param := range op.Parameters {
        g.addParameterFlag(cmd, param)
    }
    
    // Add flags for request body
    if op.RequestBody != nil {
        g.addRequestBodyFlags(cmd, op.RequestBody)
    }
    
    // Add common flags
    cmd.Flags().String("output", "json", "Output format (json, yaml, table)")
    cmd.Flags().Bool("raw", false, "Output raw HTTP response")
    
    return cmd
}

// addParameterFlag adds a flag for a parameter
func (g *Generator) addParameterFlag(cmd *cobra.Command, param openapi.Parameter) {
    flagName := g.sanitizeFlagName(param.Name)
    
    switch param.Schema.Type {
    case "string":
        if param.Schema.Enum != nil {
            // Use string slice for enums
            cmd.Flags().String(flagName, "", param.Description)
        } else {
            cmd.Flags().String(flagName, "", param.Description)
        }
        
    case "integer", "number":
        cmd.Flags().Int(flagName, 0, param.Description)
        
    case "boolean":
        cmd.Flags().Bool(flagName, false, param.Description)
        
    case "array":
        // Support multiple values
        cmd.Flags().StringSlice(flagName, nil, param.Description)
    }
    
    if param.Required {
        cmd.MarkFlagRequired(flagName)
    }
}

// addRequestBodyFlags adds flags for request body fields
func (g *Generator) addRequestBodyFlags(cmd *cobra.Command, body *openapi.RequestBody) {
    if body == nil || body.Content == nil {
        return
    }
    
    // Get JSON schema
    jsonContent, ok := body.Content["application/json"]
    if !ok {
        return
    }
    
    schema := jsonContent.Schema
    if schema == nil || schema.Type != "object" {
        return
    }
    
    // Add flags for each property
    for name, propSchema := range schema.Properties {
        flagName := g.sanitizeFlagName(name)
        
        switch propSchema.Type {
        case "string":
            cmd.Flags().String(flagName, "", propSchema.Description)
        case "integer", "number":
            cmd.Flags().Int(flagName, 0, propSchema.Description)
        case "boolean":
            cmd.Flags().Bool(flagName, false, propSchema.Description)
        }
        
        if isRequired(schema, name) {
            cmd.MarkFlagRequired(flagName)
        }
    }
    
    // Add --data flag for raw JSON
    cmd.Flags().String("data", "", "Raw JSON data (overrides individual fields)")
}

// executeOperation executes the HTTP request
func (g *Generator) executeOperation(cmd *cobra.Command, op openapi.Operation, baseURL string) error {
    ctx := cmd.Context()
    
    // Build URL
    url := baseURL + op.Path
    
    // Replace path parameters
    for _, param := range op.Parameters {
        if param.In == "path" {
            value, _ := cmd.Flags().GetString(g.sanitizeFlagName(param.Name))
            placeholder := "{" + param.Name + "}"
            url = strings.Replace(url, placeholder, value, -1)
        }
    }
    
    // Build query parameters
    queryParams := make(map[string]string)
    for _, param := range op.Parameters {
        if param.In == "query" {
            flagName := g.sanitizeFlagName(param.Name)
            
            var value string
            switch param.Schema.Type {
            case "string":
                s, _ := cmd.Flags().GetString(flagName)
                value = s
            case "integer":
                i, _ := cmd.Flags().GetInt(flagName)
                value = fmt.Sprintf("%d", i)
            case "boolean":
                b, _ := cmd.Flags().GetBool(flagName)
                value = fmt.Sprintf("%t", b)
            case "array":
                arr, _ := cmd.Flags().GetStringSlice(flagName)
                value = strings.Join(arr, ",")
            }
            
            if value != "" {
                queryParams[param.Name] = value
            }
        }
    }
    
    // Build headers
    headers := make(map[string]string)
    for _, param := range op.Parameters {
        if param.In == "header" {
            flagName := g.sanitizeFlagName(param.Name)
            value, _ := cmd.Flags().GetString(flagName)
            if value != "" {
                headers[param.Name] = value
            }
        }
    }
    
    // Build request body
    var body interface{}
    if op.RequestBody != nil {
        // Check for raw data flag first
        dataFlag, _ := cmd.Flags().GetString("data")
        if dataFlag != "" {
            body = dataFlag
        } else {
            // Build from individual flags
            body = g.buildRequestBody(cmd, op.RequestBody)
        }
    }
    
    // Execute request
    var resp *http.Response
    var err error
    
    switch op.Method {
    case "GET":
        resp, err = g.httpClient.Get(ctx, url, queryParams, headers)
    case "POST":
        resp, err = g.httpClient.Post(ctx, url, body, headers)
    case "PUT":
        resp, err = g.httpClient.Put(ctx, url, body, headers)
    case "PATCH":
        resp, err = g.httpClient.Patch(ctx, url, body, headers)
    case "DELETE":
        resp, err = g.httpClient.Delete(ctx, url, headers)
    default:
        return fmt.Errorf("unsupported HTTP method: %s", op.Method)
    }
    
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    
    // Handle output
    outputFormat, _ := cmd.Flags().GetString("output")
    raw, _ := cmd.Flags().GetBool("raw")
    
    if raw {
        fmt.Fprintln(cmd.OutOrStdout(), string(resp.Body))
        return nil
    }
    
    return g.outputResponse(cmd, resp, outputFormat)
}

// buildRequestBody builds request body from flags
func (g *Generator) buildRequestBody(cmd *cobra.Command, body *openapi.RequestBody) map[string]interface{} {
    result := make(map[string]interface{})
    
    jsonContent, ok := body.Content["application/json"]
    if !ok {
        return result
    }
    
    schema := jsonContent.Schema
    if schema == nil || schema.Type != "object" {
        return result
    }
    
    for name, propSchema := range schema.Properties {
        flagName := g.sanitizeFlagName(name)
        
        var value interface{}
        switch propSchema.Type {
        case "string":
            s, _ := cmd.Flags().GetString(flagName)
            if s != "" {
                value = s
            }
        case "integer":
            i, _ := cmd.Flags().GetInt(flagName)
            value = i
        case "number":
            f, _ := cmd.Flags().GetFloat64(flagName)
            value = f
        case "boolean":
            b, _ := cmd.Flags().GetBool(flagName)
            value = b
        }
        
        if value != nil {
            result[name] = value
        }
    }
    
    return result
}

// outputResponse formats and outputs the response
func (g *Generator) outputResponse(cmd *cobra.Command, resp *http.Response, format string) error {
    switch format {
    case "json":
        // Pretty print JSON
        var data interface{}
        if err := json.Unmarshal(resp.Body, &data); err != nil {
            return fmt.Errorf("failed to parse JSON response: %w", err)
        }
        
        prettyJSON, err := json.MarshalIndent(data, "", "  ")
        if err != nil {
            return err
        }
        
        fmt.Fprintln(cmd.OutOrStdout(), string(prettyJSON))
        
    case "yaml":
        // Convert to YAML
        var data interface{}
        json.Unmarshal(resp.Body, &data)
        yamlData, _ := yaml.Marshal(data)
        fmt.Fprintln(cmd.OutOrStdout(), string(yamlData))
        
    case "table":
        // Output as table
        g.outputTable(cmd, resp.Body)
        
    default:
        fmt.Fprintln(cmd.OutOrStdout(), string(resp.Body))
    }
    
    return nil
}

// outputTable outputs response as table
func (g *Generator) outputTable(cmd *cobra.Command, data []byte) {
    // Implementation for table output
    // Parse JSON array and format as table
}

// sanitizeCommandName sanitizes string for command name
func (g *Generator) sanitizeCommandName(name string) string {
    // Convert camelCase to kebab-case
    name = regexp.MustCompile("([a-z0-9])([A-Z])").ReplaceAllString(name, "${1}-${2}")
    name = strings.ToLower(name)
    
    // Replace underscores with hyphens
    name = strings.ReplaceAll(name, "_", "-")
    
    // Remove special characters
    name = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(name, "")
    
    // Remove consecutive hyphens
    name = regexp.MustCompile(`-+`).ReplaceAllString(name, "-")
    
    // Trim hyphens
    name = strings.Trim(name, "-")
    
    return name
}

// sanitizeFlagName sanitizes parameter name for flag
func (g *Generator) sanitizeFlagName(name string) string {
    // Convert camelCase to kebab-case
    name = regexp.MustCompile("([a-z0-9])([A-Z])").ReplaceAllString(name, "${1}-${2}")
    name = strings.ToLower(name)
    
    // Replace underscores with hyphens
    name = strings.ReplaceAll(name, "_", "-")
    
    return name
}

// getBaseURL determines the base URL
func (g *Generator) getBaseURL(spec *openapi.Spec, inv *inventory.Inventory) string {
    if len(spec.Servers) > 0 {
        return spec.Servers[0].URL
    }
    return inv.Host
}

// getTagDescription gets description for a tag
func (g *Generator) getTagDescription(spec *openapi.Spec, tagName string) string {
    for _, tag := range spec.Tags {
        if tag.Name == tagName {
            return tag.Description
        }
    }
    return fmt.Sprintf("Operations for %s", tagName)
}

// buildLongDescription builds long help text
func (g *Generator) buildLongDescription(op openapi.Operation) string {
    var parts []string
    
    if op.Description != "" {
        parts = append(parts, op.Description)
    }
    
    if len(op.Parameters) > 0 {
        parts = append(parts, "\nParameters:")
        for _, param := range op.Parameters {
            req := ""
            if param.Required {
                req = " (required)"
            }
            parts = append(parts, fmt.Sprintf("  --%s%s\n    %s", 
                g.sanitizeFlagName(param.Name), req, param.Description))
        }
    }
    
    return strings.Join(parts, "\n")
}

// buildExamples builds example usage
func (g *Generator) buildExamples(op openapi.Operation) string {
    var examples []string
    
    // Build command with required parameters
    cmd := g.sanitizeCommandName(op.OperationID)
    
    var params []string
    for _, param := range op.Parameters {
        if param.Required {
            params = append(params, fmt.Sprintf("--%s VALUE", g.sanitizeFlagName(param.Name)))
        }
    }
    
    if len(params) > 0 {
        examples = append(examples, fmt.Sprintf("openapi-cli %s %s", cmd, strings.Join(params, " ")))
    } else {
        examples = append(examples, fmt.Sprintf("openapi-cli %s", cmd))
    }
    
    return strings.Join(examples, "\n")
}

func isRequired(schema *openapi.Schema, name string) bool {
    for _, req := range schema.Required {
        if req == name {
            return true
        }
    }
    return false
}
```

## 3. Data Models

### File: `internal/core/openapi/types.go`

```go
package openapi

// Spec represents a parsed OpenAPI specification
type Spec struct {
    OpenAPI    string       `json:"openapi"`
    Info       Info         `json:"info"`
    Servers    []Server     `json:"servers"`
    Tags       []Tag        `json:"tags"`
    Operations []Operation  `json:"operations"`
}

// Info provides metadata about the API
type Info struct {
    Title       string `json:"title"`
    Description string `json:"description,omitempty"`
    Version     string `json:"version"`
}

// Server represents a server URL
type Server struct {
    URL         string `json:"url"`
    Description string `json:"description,omitempty"`
}

// Tag provides metadata for operations
type Tag struct {
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
}

// Operation represents an API operation
type Operation struct {
    OperationID string       `json:"operationId"`
    Summary     string       `json:"summary,omitempty"`
    Description string       `json:"description,omitempty"`
    Method      string       `json:"method"`
    Path        string       `json:"path"`
    Tags        []string     `json:"tags,omitempty"`
    Parameters  []Parameter  `json:"parameters,omitempty"`
    RequestBody *RequestBody `json:"requestBody,omitempty"`
    Responses   map[int]*Response `json:"responses,omitempty"`
}

// Parameter represents an operation parameter
type Parameter struct {
    Name        string  `json:"name"`
    In          string  `json:"in"` // "query", "path", "header"
    Description string  `json:"description,omitempty"`
    Required    bool    `json:"required,omitempty"`
    Schema      *Schema `json:"schema,omitempty"`
}

// RequestBody represents a request body
type RequestBody struct {
    Description string              `json:"description,omitempty"`
    Required    bool                `json:"required,omitempty"`
    Content     map[string]MediaType `json:"content"`
}

// Response represents an HTTP response
type Response struct {
    Description string              `json:"description"`
    Content     map[string]MediaType `json:"content,omitempty"`
}

// MediaType represents content media type
type MediaType struct {
    Schema *Schema `json:"schema,omitempty"`
}

// Schema represents a data schema
type Schema struct {
    Type        string            `json:"type,omitempty"`
    Format      string            `json:"format,omitempty"`
    Description string            `json:"description,omitempty"`
    Default     interface{}       `json:"default,omitempty"`
    Enum        []interface{}     `json:"enum,omitempty"`
    Required    []string          `json:"required,omitempty"`
    Properties  map[string]*Schema `json:"properties,omitempty"`
    Items       *Schema           `json:"items,omitempty"`
}
```

---

## Deterministic Guarantees

This OpenAPI parsing implementation must comply with the **Bible Standard** for deterministic behavior across platforms, languages, and AI systems. Refer to [`DETERMINISTIC-GUARANTEES.md`](../../DETERMINISTIC-GUARANTEES.md) and [`PLATFORM-BEHAVIOR.md`](../../PLATFORM-BEHAVIOR.md) for complete requirements.

### 1. Deterministic Parsing Output

OpenAPI parsing must produce identical abstract syntax trees (AST) across platforms:

- **JSON parsing**: Use deterministic JSON unmarshaling (sorted keys, no extra whitespace)
- **YAML parsing**: Use same YAML parser with identical anchor resolution order
- **Schema normalization**: Identical default value injection, reference resolution order

### 2. Deterministic Command Generation

Generated CLI commands must be identical across platforms:

- **Command name normalization**: Lowercase, hyphen‑separated (kebab‑case)
- **Flag ordering**: Flags sorted alphabetically by name
- **Parameter mapping**: Identical mapping from OpenAPI parameters to CLI flags

### 3. Platform‑Independent Path Handling

OpenAPI path templates must be normalized identically:

- **Path parameter expansion**: Identical regex patterns for validation
- **Query parameter ordering**: Sorted lexicographically (see `API-SPECIFICATION.md` section 3)
- **Header canonicalization**: Use `textproto.CanonicalMIMEHeaderKey()`

### 4. Deterministic Schema Validation

Schema validation must produce identical error messages:

- **Type validation**: Same type coercion rules across platforms
- **Enum validation**: Same ordering of enum values
- **Required field validation**: Same field order in error messages

### 5. Concurrent Parsing Determinism

Concurrent parsing operations must be deterministic:

- **Cache key generation**: SHA‑256 hash of spec content (deterministic)
- **Cache invalidation**: Based on spec hash, not file modification time
- **Parsed spec caching**: Thread‑safe with copy‑on‑write updates

### 6. Testing Determinism

All OpenAPI parsing tests must include platform‑specific variants:

```go
//go:build linux || darwin || windows
func TestOpenAPIParsingDeterminism(t *testing.T) {
	// Verify identical AST for same spec
	// Verify identical command generation
	// Verify identical validation errors
}
```

### 7. Integration with SDK Generation

When generating SDKs for 12 languages, OpenAPI parsers must:

- Produce identical API client interfaces
- Generate identical validation logic
- Create identical error handling patterns

**Verification**: Run `./scripts/validate‑determinism‑openapi.sh` to confirm cross‑platform consistency.

This specification provides complete OpenAPI parsing logic with command generation, parameter handling, and request execution for Ralph Loop.
