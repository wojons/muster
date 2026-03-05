# Business Command Generator Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: Dynamic Command Generation

## Overview

Complete specification for **dynamic command generation** from OpenAPI specifications. The command generator parses OpenAPI specs and dynamically creates cobra commands with proper flags, arguments, and help text for each API operation.

## 1. Generation Pipeline

```
OpenAPI Spec
    ↓
Parse Operations
    ↓
Group by Tags
    ↓
Create Commands
    ↓
Add Flags
    ↓
Build Command Tree
    ↓
Register with Cobra
```

## 2. Core Components

### 2.1 Command Generator

```go
package generator

import (
    "fmt"
    "strings"
    
    "github.com/getkin/kin-openapi/openapi3"
    "github.com/spf13/cobra"
)

type CommandGenerator struct {
    spec       *openapi3.T
    operations map[string][]*Operation
    baseURL    string
}

type Operation struct {
    Path       string
    Method     string
    Operation  *openapi3.Operation
    Parameters openapi3.Parameters
}

func NewCommandGenerator(specPath string) (*CommandGenerator, error) {
    loader := openapi3.NewLoader()
    spec, err := loader.LoadFromFile(specPath)
    if err != nil {
        return nil, fmt.Errorf("load spec: %w", err)
    }
    
    if err := spec.Validate(loader.Context); err != nil {
        return nil, fmt.Errorf("validate spec: %w", err)
    }
    
    cg := &CommandGenerator{
        spec:       spec,
        operations: make(map[string][]*Operation),
    }
    
    cg.parseOperations()
    return cg, nil
}

func (cg *CommandGenerator) parseOperations() {
    for path, pathItem := range cg.spec.Paths {
        cg.addOperation(path, "GET", pathItem.Get)
        cg.addOperation(path, "POST", pathItem.Post)
        cg.addOperation(path, "PUT", pathItem.Put)
        cg.addOperation(path, "PATCH", pathItem.Patch)
        cg.addOperation(path, "DELETE", pathItem.Delete)
    }
}

func (cg *CommandGenerator) addOperation(path, method string, op *openapi3.Operation) {
    if op == nil {
        return
    }
    
    // Group by first tag
    tag := "default"
    if len(op.Tags) > 0 {
        tag = op.Tags[0]
    }
    
    operation := &Operation{
        Path:       path,
        Method:     method,
        Operation:  op,
        Parameters: op.Parameters,
    }
    
    cg.operations[tag] = append(cg.operations[tag], operation)
}

func (cg *CommandGenerator) GenerateCommands() ([]*cobra.Command, error) {
    var commands []*cobra.Command
    
    for tag, ops := range cg.operations {
        cmd := cg.createResourceCommand(tag, ops)
        commands = append(commands, cmd)
    }
    
    return commands, nil
}
```

### 2.2 Resource Command Creation

```go
func (cg *CommandGenerator) createResourceCommand(tag string, ops []*Operation) *cobra.Command {
    resourceCmd := &cobra.Command{
        Use:   strings.ToLower(tag),
        Short: fmt.Sprintf("Manage %s resources", tag),
        Long:  fmt.Sprintf("Commands for managing %s resources via the API.", tag),
    }
    
    for _, op := range ops {
        subCmd := cg.createOperationCommand(op)
        resourceCmd.AddCommand(subCmd)
    }
    
    return resourceCmd
}

func (cg *CommandGenerator) createOperationCommand(op *Operation) *cobra.Command {
    cmdName := cg.inferCommandName(op)
    
    cmd := &cobra.Command{
        Use:     cg.buildUsage(cmdName, op),
        Short:   op.Operation.Summary,
        Long:    cg.buildLongDescription(op),
        Example: cg.buildExample(cmdName, op),
        RunE:    cg.createRunHandler(op),
    }
    
    // Add flags for parameters
    cg.addFlags(cmd, op)
    
    return cmd
}

func (cg *CommandGenerator) inferCommandName(op *Operation) string {
    // Check for explicit operationId mapping
    if op.Operation.OperationID != "" {
        return cg.normalizeOperationID(op.Operation.OperationID)
    }
    
    switch op.Method {
    case "GET":
        if strings.Contains(op.Path, "{") {
            return "get"
        }
        return "list"
    case "POST":
        return "create"
    case "PUT":
        return "update"
    case "PATCH":
        return "patch"
    case "DELETE":
        return "delete"
    default:
        return strings.ToLower(op.Method)
    }
}

func (cg *CommandGenerator) normalizeOperationID(id string) string {
    // Convert camelCase or PascalCase to kebab-case
    var result []rune
    for i, r := range id {
        if i > 0 && r >= 'A' && r <= 'Z' {
            result = append(result, '-')
        }
        result = append(result, r)
    }
    return strings.ToLower(string(result))
}
```

## 3. Flag Generation

### 3.1 Parameter to Flag Mapping

| OpenAPI Type | Cobra Flag Type | Example |
|--------------|-----------------|---------|
| `string` | `--<name>` string | `--name app` |
| `integer` | `--<name>` int | `--replicas 3` |
| `boolean` | `--<name>` bool | `--enabled` |
| `array` | `--<name>` stringSlice | `--tags a,b,c` |
| `enum` | `--<name>` enum | `--format json` |
| `file` | `--<name>` filename | `--config file.yaml` |

### 3.2 Flag Implementation

```go
func (cg *CommandGenerator) addFlags(cmd *cobra.Command, op *Operation) {
    for _, paramRef := range op.Parameters {
        param := paramRef.Value
        if param.In == "path" {
            continue // Path params are positional
        }
        
        flagName := cg.normalizeFlagName(param.Name)
        shorthand := cg.inferShorthand(param.Name)
        description := param.Description
        
        switch param.Schema.Value.Type {
        case "string":
            cg.addStringFlag(cmd, flagName, shorthand, param, description)
        case "integer":
            defaultVal := 0
            if param.Schema.Value.Default != nil {
                defaultVal = int(param.Schema.Value.Default.(float64))
            }
            cmd.Flags().IntP(flagName, shorthand, defaultVal, description)
        case "number":
            defaultVal := 0.0
            if param.Schema.Value.Default != nil {
                defaultVal = param.Schema.Value.Default.(float64)
            }
            cmd.Flags().Float64P(flagName, shorthand, defaultVal, description)
        case "boolean":
            defaultVal := false
            if param.Schema.Value.Default != nil {
                defaultVal = param.Schema.Value.Default.(bool)
            }
            cmd.Flags().BoolP(flagName, shorthand, defaultVal, description)
        case "array":
            cmd.Flags().StringSliceP(flagName, shorthand, nil, description)
        }
        
        if param.Required {
            cmd.MarkFlagRequired(flagName)
        }
    }
    
    // Add standard output flags
    cmd.Flags().StringP("output", "o", "table", "Output format (table, json, yaml)")
    cmd.Flags().StringP("namespace", "n", "", "Resource namespace")
    cmd.Flags().BoolP("watch", "w", false, "Watch for changes")
    cmd.Flags().BoolP("raw", "r", false, "Output raw API response")
}

func (cg *CommandGenerator) addStringFlag(cmd *cobra.Command, name, shorthand string, param *openapi3.Parameter, description string) {
    schema := param.Schema.Value
    
    // Handle enums
    if schema.Enum != nil && len(schema.Enum) > 0 {
        allowed := make([]string, len(schema.Enum))
        for i, v := range schema.Enum {
            allowed[i] = fmt.Sprintf("%v", v)
        }
        description = fmt.Sprintf("%s (allowed: %s)", description, strings.Join(allowed, ", "))
        
        // Use StringP but validate in pre-run
        defaultVal := ""
        if schema.Default != nil {
            defaultVal = schema.Default.(string)
        }
        cmd.Flags().StringP(name, shorthand, defaultVal, description)
        return
    }
    
    // Handle format-specific flags
    switch schema.Format {
    case "date-time":
        cmd.Flags().StringP(name, shorthand, "", description+" (ISO8601)")
    case "date":
        cmd.Flags().StringP(name, shorthand, "", description+" (YYYY-MM-DD)")
    case "email":
        cmd.Flags().StringP(name, shorthand, "", description+" (email format)")
    case "uri", "url":
        cmd.Flags().StringP(name, shorthand, "", description+" (URL)")
    default:
        defaultVal := ""
        if schema.Default != nil {
            defaultVal = schema.Default.(string)
        }
        cmd.Flags().StringP(name, shorthand, defaultVal, description)
    }
}

func (cg *CommandGenerator) normalizeFlagName(name string) string {
    // Convert camelCase to kebab-case
    var result []rune
    for i, r := range name {
        if i > 0 && r >= 'A' && r <= 'Z' {
            result = append(result, '-')
        }
        result = append(result, r)
    }
    return strings.ToLower(string(result))
}

func (cg *CommandGenerator) inferShorthand(name string) string {
    shorthandMap := map[string]string{
        "namespace":  "n",
        "output":     "o",
        "format":     "f",
        "selector":   "l",
        "all":        "a",
        "watch":      "w",
        "follow":     "f",
        "help":       "h",
        "config":     "c",
        "verbose":    "v",
        "yes":        "y",
        "timeout":    "t",
    }
    
    normalized := cg.normalizeFlagName(name)
    if sh, ok := shorthandMap[normalized]; ok {
        return sh
    }
    return ""
}
```

## 4. Usage and Help Generation

```go
func (cg *CommandGenerator) buildUsage(name string, op *Operation) string {
    var args []string
    
    // Extract path parameters
    for _, param := range op.Parameters {
        if param.Value.In == "path" {
            placeholder := fmt.Sprintf("<%s>", param.Value.Name)
            if !param.Value.Required {
                placeholder = fmt.Sprintf("[%s]", placeholder)
            }
            args = append(args, placeholder)
        }
    }
    
    if len(args) > 0 {
        return fmt.Sprintf("%s %s [flags]", name, strings.Join(args, " "))
    }
    return fmt.Sprintf("%s [flags]", name)
}

func (cg *CommandGenerator) buildLongDescription(op *Operation) string {
    var b strings.Builder
    
    if op.Operation.Description != "" {
        b.WriteString(op.Operation.Description)
        b.WriteString("\n\n")
    }
    
    b.WriteString("API Endpoint:\n")
    b.WriteString(fmt.Sprintf("  %s %s\n\n", op.Method, op.Path))
    
    if len(op.Parameters) > 0 {
        b.WriteString("Parameters:\n")
        for _, paramRef := range op.Parameters {
            param := paramRef.Value
            required := ""
            if param.Required {
                required = " (required)"
            }
            b.WriteString(fmt.Sprintf("  %s%s: %s\n", param.Name, required, param.Description))
        }
    }
    
    return b.String()
}

func (cg *CommandGenerator) buildExample(name string, op *Operation) string {
    var examples []string
    
    // Build base command
    resourceName := cg.inferResourceName(op)
    base := fmt.Sprintf("muster %s %s", resourceName, name)
    
    // Add required path params
    for _, paramRef := range op.Parameters {
        if paramRef.Value.In == "path" && paramRef.Value.Required {
            base += fmt.Sprintf(" <%s>", paramRef.Value.Name)
        }
    }
    
    // Basic example
    examples = append(examples, fmt.Sprintf("  # Basic usage\n  %s", base))
    
    // Example with query params
    if hasQueryParams(op) {
        examples = append(examples, fmt.Sprintf("\n  # With filters\n  %s --namespace production", base))
    }
    
    // Example with output format
    examples = append(examples, fmt.Sprintf("\n  # JSON output\n  %s --output json", base))
    
    return strings.Join(examples, "\n")
}

func (cg *CommandGenerator) inferResourceName(op *Operation) string {
    // Extract resource name from path
    parts := strings.Split(op.Path, "/")
    for _, part := range parts {
        if part != "" && !strings.HasPrefix(part, "{") {
            return strings.ToLower(part)
        }
    }
    return "resource"
}

func hasQueryParams(op *Operation) bool {
    for _, paramRef := range op.Parameters {
        if paramRef.Value.In == "query" {
            return true
        }
    }
    return false
}
```

## 5. Run Handler

```go
func (cg *CommandGenerator) createRunHandler(op *Operation) func(*cobra.Command, []string) error {
    return func(cmd *cobra.Command, args []string) error {
        ctx := cmd.Context()
        
        // Build request
        builder := request.NewBuilder(cg.baseURL, op.Path, op.Method)
        
        // Add path params from args
        pathParams := extractPathParams(op.Path)
        for i, param := range pathParams {
            if i < len(args) {
                builder.WithPathParam(param, args[i])
            } else {
                return fmt.Errorf("missing required argument: %s", param)
            }
        }
        
        // Add query params from flags
        for _, paramRef := range op.Parameters {
            param := paramRef.Value
            if param.In == "query" {
                value, err := cg.getFlagValue(cmd, param)
                if err != nil {
                    return fmt.Errorf("get flag %s: %w", param.Name, err)
                }
                if value != nil {
                    builder.WithQueryParam(param.Name, fmt.Sprintf("%v", value))
                }
            }
        }
        
        // Add headers from flags
        for _, paramRef := range op.Parameters {
            param := paramRef.Value
            if param.In == "header" {
                value, _ := cmd.Flags().GetString(cg.normalizeFlagName(param.Name))
                if value != "" {
                    builder.WithHeader(param.Name, value)
                }
            }
        }
        
        // Execute request
        resp, err := builder.Execute(ctx)
        if err != nil {
            return fmt.Errorf("execute request: %w", err)
        }
        defer resp.Body.Close()
        
        // Handle response
        if resp.StatusCode >= 400 {
            return cg.handleErrorResponse(resp)
        }
        
        // Format and output
        output, _ := cmd.Flags().GetString("output")
        raw, _ := cmd.Flags().GetBool("raw")
        
        if raw {
            return output.Raw(cmd.OutOrStdout(), resp.Body)
        }
        
        return format.Output(cmd.OutOrStdout(), resp, output)
    }
}

func (cg *CommandGenerator) getFlagValue(cmd *cobra.Command, param *openapi3.Parameter) (interface{}, error) {
    flagName := cg.normalizeFlagName(param.Name)
    
    switch param.Schema.Value.Type {
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

func extractPathParams(path string) []string {
    var params []string
    parts := strings.Split(path, "/")
    for _, part := range parts {
        if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
            params = append(params, part[1:len(part)-1])
        }
    }
    return params
}
```

## 6. Caching

```go
type CommandCache struct {
    path      string
    commands  map[string]*cachedCommand
    mu        sync.RWMutex
    ttl       time.Duration
}

type cachedCommand struct {
    Commands   []*cobra.Command
    SpecHash   string
    ExpiresAt  time.Time
}

func NewCommandCache(path string) *CommandCache {
    cache := &CommandCache{
        path:     path,
        commands: make(map[string]*cachedCommand),
        ttl:      24 * time.Hour,
    }
    
    // Load from disk
    cache.load()
    
    return cache
}

func (cc *CommandCache) Load(specPath string) ([]*cobra.Command, bool) {
    hash := computeHash(specPath)
    
    cc.mu.RLock()
    defer cc.mu.RUnlock()
    
    if cached, ok := cc.commands[specPath]; ok {
        if cached.SpecHash == hash && time.Now().Before(cached.ExpiresAt) {
            return cached.Commands, true
        }
    }
    return nil, false
}

func (cc *CommandCache) Save(specPath string, commands []*cobra.Command) error {
    hash := computeHash(specPath)
    
    cc.mu.Lock()
    defer cc.mu.Unlock()
    
    cc.commands[specPath] = &cachedCommand{
        Commands:  commands,
        SpecHash:  hash,
        ExpiresAt: time.Now().Add(cc.ttl),
    }
    
    return cc.persist()
}

func computeHash(path string) string {
    data, err := os.ReadFile(path)
    if err != nil {
        return ""
    }
    
    hash := sha256.Sum256(data)
    return hex.EncodeToString(hash[:])
}
```

## 7. Integration

```go
func InitializeDynamicCommands(rootCmd *cobra.Command, specPath string) error {
    // Check cache first
    cache := NewCommandCache(filepath.Join(os.TempDir(), "muster-cache"))
    
    if commands, ok := cache.Load(specPath); ok {
        for _, cmd := range commands {
            rootCmd.AddCommand(cmd)
        }
        return nil
    }
    
    // Generate new commands
    generator, err := NewCommandGenerator(specPath)
    if err != nil {
        return fmt.Errorf("create generator: %w", err)
    }
    
    commands, err := generator.GenerateCommands()
    if err != nil {
        return fmt.Errorf("generate commands: %w", err)
    }
    
    // Register with root
    for _, cmd := range commands {
        rootCmd.AddCommand(cmd)
    }
    
    // Cache for next time
    cache.Save(specPath, commands)
    
    return nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
