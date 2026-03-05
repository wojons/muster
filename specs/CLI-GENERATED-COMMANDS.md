# CLI Generated Commands Specification

**Status**: ✅ COMPLETE  
**Date**: 2026-02-28  
**Purpose**: Dynamic command generation from OpenAPI specifications  
**Language**: Go 1.21+ with Cobra  

---

## Overview

Dynamic commands are generated at runtime from OpenAPI specifications. The CLI parses the OpenAPI spec and creates cobra commands for each operation, enabling automatic API client generation without manual coding.

---

## OpenAPI to Cobra Mapping

### Operation to Command Mapping

```go
// internal/cli/generated/mapper.go
package generated

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/anomalyco/muster/internal/openapi"
)

// OperationMapper maps OpenAPI operations to cobra commands
type OperationMapper struct {
	spec *openapi.Spec
}

// NewOperationMapper creates a new operation mapper
func NewOperationMapper(spec *openapi.Spec) *OperationMapper {
	return &OperationMapper{spec: spec}
}

// MapOperation converts an OpenAPI operation to a cobra command
func (m *OperationMapper) MapOperation(op *openapi.Operation) *cobra.Command {
	cmd := &cobra.Command{
		Use:   m.buildUseLine(op),
		Short: op.Summary,
		Long:  m.buildLongDescription(op),
		Example: m.buildExample(op),
		RunE: func(cmd *cobra.Command, args []string) error {
			return m.executeOperation(cmd, op, args)
		},
	}

	// Add flags from OpenAPI parameters
	m.addFlags(cmd, op)

	// Add common flags
	m.addCommonFlags(cmd)

	// Add completion functions
	m.addCompletions(cmd, op)

	return cmd
}

// buildUseLine constructs the command usage line
func (m *OperationMapper) buildUseLine(op *openapi.Operation) string {
	parts := []string{op.OperationID}

	// Add path parameters as arguments
	for _, param := range op.Parameters {
		if param.In == "path" {
			parts = append(parts, fmt.Sprintf("<%s>", param.Name))
		}
	}

	parts = append(parts, "[flags]")
	return strings.Join(parts, " ")
}

// buildLongDescription creates detailed help text
func (m *OperationMapper) buildLongDescription(op *openapi.Operation) string {
	var sb strings.Builder

	if op.Description != "" {
		sb.WriteString(op.Description)
		sb.WriteString("\n\n")
	}

	sb.WriteString(fmt.Sprintf("HTTP Method: %s\n", op.Method))
	sb.WriteString(fmt.Sprintf("Path: %s\n", op.Path))

	if len(op.Parameters) > 0 {
		sb.WriteString("\nParameters:\n")
		for _, param := range op.Parameters {
			required := ""
			if param.Required {
				required = " (required)"
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s%s\n", param.Name, param.Description, required))
		}
	}

	return sb.String()
}

// buildExample creates usage examples
func (m *OperationMapper) buildExample(op *openapi.Operation) string {
	base := fmt.Sprintf("muster %s", op.OperationID)

	// Add required arguments
	for _, param := range op.Parameters {
		if param.In == "path" {
			base += fmt.Sprintf(" %s-value", param.Name)
		}
	}

	var examples []string
	examples = append(examples, fmt.Sprintf("  # Basic usage\n  %s", base))

	// Add flag examples
	for _, param := range op.Parameters {
		if param.In == "query" && param.Required {
			examples = append(examples, fmt.Sprintf("\n  # With %s\n  %s --%s=value",
				param.Name, base, param.Name))
		}
	}

	return strings.Join(examples, "\n")
}
```

### Flag Binding

```go
// internal/cli/generated/flags.go
package generated

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/anomalyco/muster/internal/openapi"
)

// FlagBinder binds OpenAPI parameters to cobra flags
type FlagBinder struct {
	bindings map[string]FlagBinding
}

// FlagBinding represents a bound flag
type FlagBinding struct {
	Param    *openapi.Parameter
	Value    interface{}
	Required bool
}

// NewFlagBinder creates a new flag binder
func NewFlagBinder() *FlagBinder {
	return &FlagBinder{
		bindings: make(map[string]FlagBinding),
	}
}

// BindParameters adds flags for all OpenAPI parameters
func (fb *FlagBinder) BindParameters(cmd *cobra.Command, op *openapi.Operation) {
	for _, param := range op.Parameters {
		switch param.In {
		case "query":
			fb.bindQueryFlag(cmd, param)
		case "header":
			fb.bindHeaderFlag(cmd, param)
		}
	}

	// Bind request body flags for simple types
	if op.RequestBody != nil && op.RequestBody.Schema != nil {
		fb.bindBodyFlags(cmd, op.RequestBody.Schema)
	}
}

func (fb *FlagBinder) bindQueryFlag(cmd *cobra.Command, param *openapi.Parameter) {
	name := param.Name
	shorthand := fb.shorthandFor(name)
	usage := param.Description
	if param.Required {
		usage += " (required)"
	}

	switch param.Schema.Type {
	case "string":
		defaultVal := param.Schema.Default.(string)
		cmd.Flags().StringP(name, shorthand, defaultVal, usage)
	case "integer":
		defaultVal := param.Schema.Default.(int)
		cmd.Flags().IntP(name, shorthand, defaultVal, usage)
	case "boolean":
		defaultVal := param.Schema.Default.(bool)
		cmd.Flags().BoolP(name, shorthand, defaultVal, usage)
	case "array":
		cmd.Flags().StringSliceP(name, shorthand, nil, usage)
	}

	if param.Required {
		cmd.MarkFlagRequired(name)
	}

	fb.bindings[name] = FlagBinding{
		Param:    param,
		Required: param.Required,
	}
}

func (fb *FlagBinder) bindHeaderFlag(cmd *cobra.Command, param *openapi.Parameter) {
	name := fmt.Sprintf("header-%s", param.Name)
	cmd.Flags().String(name, "", param.Description)
	fb.bindings[name] = FlagBinding{Param: param}
}

func (fb *FlagBinder) bindBodyFlags(cmd *cobra.Command, schema *openapi.Schema) {
	for propName, prop := range schema.Properties {
		flagName := fmt.Sprintf("body-%s", propName)
		usage := fmt.Sprintf("Request body field: %s", propName)

		switch prop.Type {
		case "string":
			cmd.Flags().String(flagName, "", usage)
		case "integer":
			cmd.Flags().Int(flagName, 0, usage)
		case "boolean":
			cmd.Flags().Bool(flagName, false, usage)
		}
	}
}

func (fb *FlagBinder) shorthandFor(name string) string {
	// Extract first letter or common shorthand
	if len(name) > 0 {
		return string(name[0])
	}
	return ""
}

// GetValues extracts all flag values from command
func (fb *FlagBinder) GetValues(cmd *cobra.Command) (map[string]interface{}, error) {
	values := make(map[string]interface{})

	for name, binding := range fb.bindings {
		val, err := cmd.Flags().GetString(name)
		if err != nil {
			// Try other types
			if intVal, err := cmd.Flags().GetInt(name); err == nil {
				values[name] = intVal
			} else if boolVal, err := cmd.Flags().GetBool(name); err == nil {
				values[name] = boolVal
			} else if sliceVal, err := cmd.Flags().GetStringSlice(name); err == nil {
				values[name] = sliceVal
			}
		} else {
			values[name] = val
		}

		// Validate required
		if binding.Required && values[name] == "" {
			return nil, fmt.Errorf("required flag not set: --%s", name)
		}
	}

	return values, nil
}
```

### Execution Delegation

```go
// internal/cli/generated/executor.go
package generated

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/anomalyco/muster/internal/openapi"
	"github.com/anomalyco/muster/internal/cli/output"
)

// Executor handles API request execution for generated commands
type Executor struct {
	httpClient *http.Client
	formatter  output.Formatter
}

// NewExecutor creates a new command executor
func NewExecutor(client *http.Client, formatter output.Formatter) *Executor {
	return &Executor{
		httpClient: client,
		formatter:  formatter,
	}
}

// Execute performs the API operation
func (e *Executor) Execute(cmd *cobra.Command, op *openapi.Operation, args []string) error {
	ctx := cmd.Context()

	// Build request
	req, err := e.buildRequest(ctx, cmd, op, args)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}

	// Execute request
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle response
	return e.handleResponse(cmd, resp, op)
}

func (e *Executor) buildRequest(ctx context.Context, cmd *cobra.Command, op *openapi.Operation, args []string) (*http.Request, error) {
	// Build URL with path parameters
	url := e.buildURL(cmd, op, args)

	// Get flag values
	binder := NewFlagBinder()
	binder.BindParameters(cmd, op)
	values, err := binder.GetValues(cmd)
	if err != nil {
		return nil, err
	}

	// Add query parameters
	url = e.addQueryParams(url, values, op)

	// Build request body
	var body io.Reader
	if op.RequestBody != nil {
		bodyData := e.buildBody(values, op.RequestBody.Schema)
		jsonData, _ := json.Marshal(bodyData)
		body = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, op.Method, url, body)
	if err != nil {
		return nil, err
	}

	// Add headers
	e.addHeaders(req, values, op)

	return req, nil
}

func (e *Executor) buildURL(cmd *cobra.Command, op *openapi.Operation, args []string) string {
	url := op.BaseURL + op.Path

	// Replace path parameters
	pathParams := make(map[string]string)
	argIndex := 0
	for _, param := range op.Parameters {
		if param.In == "path" {
			if argIndex < len(args) {
				pathParams[param.Name] = args[argIndex]
				argIndex++
			}
		}
	}

	for key, value := range pathParams {
		url = strings.ReplaceAll(url, "{"+key+"}", value)
	}

	return url
}

func (e *Executor) addQueryParams(url string, values map[string]interface{}, op *openapi.Operation) string {
	var params []string
	for _, param := range op.Parameters {
		if param.In == "query" {
			if val, ok := values[param.Name]; ok && val != "" {
				params = append(params, fmt.Sprintf("%s=%v", param.Name, val))
			}
		}
	}

	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}

	return url
}

func (e *Executor) addHeaders(req *http.Request, values map[string]interface{}, op *openapi.Operation) {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	for _, param := range op.Parameters {
		if param.In == "header" {
			flagName := fmt.Sprintf("header-%s", param.Name)
			if val, ok := values[flagName]; ok && val != "" {
				req.Header.Set(param.Name, fmt.Sprintf("%v", val))
			}
		}
	}
}

func (e *Executor) buildBody(values map[string]interface{}, schema *openapi.Schema) map[string]interface{} {
	body := make(map[string]interface{})

	for propName := range schema.Properties {
		flagName := fmt.Sprintf("body-%s", propName)
		if val, ok := values[flagName]; ok && val != "" {
			body[propName] = val
		}
	}

	return body
}

func (e *Executor) handleResponse(cmd *cobra.Command, resp *http.Response, op *openapi.Operation) error {
	// Check status
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Format and output
	return e.formatter.Format(cmd.OutOrStdout(), result)
}
```

### Command Registration

```go
// internal/cli/generated/registry.go
package generated

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/anomalyco/muster/internal/openapi"
)

// Registry manages dynamically generated commands
type Registry struct {
	spec       *openapi.Spec
	commands   map[string]*cobra.Command
	mapper     *OperationMapper
	executor   *Executor
}

// NewRegistry creates a new command registry
func NewRegistry(spec *openapi.Spec, executor *Executor) *Registry {
	return &Registry{
		spec:     spec,
		commands: make(map[string]*cobra.Command),
		mapper:   NewOperationMapper(spec),
		executor: executor,
	}
}

// GenerateCommands creates commands from the OpenAPI spec
func (r *Registry) GenerateCommands() ([]*cobra.Command, error) {
	// Group operations by tag
	groups := make(map[string][]*openapi.Operation)
	for _, op := range r.spec.Operations {
		tag := "default"
		if len(op.Tags) > 0 {
			tag = op.Tags[0]
		}
		groups[tag] = append(groups[tag], op)
	}

	var commands []*cobra.Command

	// Create command groups
	for tag, operations := range groups {
		groupCmd := &cobra.Command{
			Use:   tag,
			Short: fmt.Sprintf("Commands for %s", tag),
		}

		for _, op := range operations {
			cmd := r.mapper.MapOperation(op)
			cmd.RunE = func(cmd *cobra.Command, args []string) error {
				return r.executor.Execute(cmd, op, args)
			}
			groupCmd.AddCommand(cmd)
			r.commands[op.OperationID] = cmd
		}

		commands = append(commands, groupCmd)
	}

	return commands, nil
}

// GetCommand retrieves a generated command by ID
func (r *Registry) GetCommand(operationID string) (*cobra.Command, bool) {
	cmd, ok := r.commands[operationID]
	return cmd, ok
}
```

### Auto-Completion

```go
// internal/cli/generated/completion.go
package generated

import (
	"github.com/spf13/cobra"
	"github.com/anomalyco/muster/internal/inventory"
)

// CompletionProvider provides shell completions for generated commands
type CompletionProvider struct {
	inventory *inventory.Manager
}

// NewCompletionProvider creates a new completion provider
func NewCompletionProvider(inv *inventory.Manager) *CompletionProvider {
	return &CompletionProvider{inventory: inv}
}

// ResourceNameCompletion provides completion for resource names
func (cp *CompletionProvider) ResourceNameCompletion(resourceType string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		names, err := cp.inventory.ListResourceNames(cmd.Context(), resourceType)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		// Filter by toComplete
		var matches []string
		for _, name := range names {
			if hasPrefix(name, toComplete) {
				matches = append(matches, name)
			}
		}

		return matches, cobra.ShellCompDirectiveNoFileComp
	}
}

// EnumCompletion provides completion for enum values
func (cp *CompletionProvider) EnumCompletion(values []string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var matches []string
		for _, val := range values {
			if hasPrefix(val, toComplete) {
				matches = append(matches, val)
			}
		}
		return matches, cobra.ShellCompDirectiveNoFileComp
	}
}

func hasPrefix(s, prefix string) bool {
	if len(prefix) == 0 {
		return true
	}
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}
```

---

## Usage Example

```go
// Load OpenAPI spec
spec, err := openapi.LoadSpec("https://api.example.com/openapi.json")
if err != nil {
    log.Fatal(err)
}

// Create executor
executor := generated.NewExecutor(httpClient, jsonFormatter)

// Create registry and generate commands
registry := generated.NewRegistry(spec, executor)
commands, err := registry.GenerateCommands()
if err != nil {
    log.Fatal(err)
}

// Register with root command
for _, cmd := range commands {
    rootCmd.AddCommand(cmd)
}
```

---

## Implementation Notes

1. **Dynamic Discovery**: Commands are generated on CLI startup from the OpenAPI spec
2. **Caching**: Generated commands are cached to disk for faster subsequent startups
3. **Validation**: Parameter types are validated against OpenAPI schema
4. **Documentation**: Help text is auto-generated from OpenAPI descriptions
5. **Extensions**: Custom x-cli-* extensions in OpenAPI can customize behavior
