# CLI Template System Specification

## Overview

The template system provides project scaffolding and code generation capabilities through customizable templates.

## Architecture

```
internal/template/
├── engine.go        # Template execution engine
├── scaffold.go      # Project scaffolding
├── variables.go     # Variable resolution
└── builtin/         # Built-in templates
    ├── project/
    ├── module/
    └── config/
```

## Template Engine

```go
package template

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

// Engine executes templates with variable substitution
type Engine struct {
	funcs template.FuncMap
}

// NewEngine creates a new template engine
func NewEngine() *Engine {
	e := &Engine{
		funcs: sprig.TxtFuncMap(),
	}
	
	// Add custom functions
	e.funcs["env"] = os.Getenv
	e.funcs["lower"] = strings.ToLower
	e.funcs["upper"] = strings.ToUpper
	e.funcs["camelCase"] = toCamelCase
	e.funcs["snakeCase"] = toSnakeCase
	e.funcs["kebabCase"] = toKebabCase
	
	return e
}

// Execute renders a template with variables
func (e *Engine) Execute(tmplStr string, vars map[string]interface{}) (string, error) {
	tmpl, err := template.New("template").
		Funcs(e.funcs).
		Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}
	
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	
	return buf.String(), nil
}

// ExecuteFile renders a template file
func (e *Engine) ExecuteFile(templatePath string, vars map[string]interface{}) (string, error) {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("failed to read template: %w", err)
	}
	
	return e.Execute(string(content), vars)
}

func toCamelCase(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	
	for i := range parts {
		if i == 0 {
			parts[i] = strings.ToLower(parts[i])
		} else {
			parts[i] = strings.Title(strings.ToLower(parts[i]))
		}
	}
	
	return strings.Join(parts, "")
}

func toSnakeCase(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "_"))
}

func toKebabCase(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", "-"))
}
```

## Project Scaffolding

```go
package template

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Scaffolder creates project structures from templates
type Scaffolder struct {
	engine      *Engine
	templateDir string
}

// NewScaffolder creates a new project scaffolder
func NewScaffolder(templateDir string) *Scaffolder {
	return &Scaffolder{
		engine:      NewEngine(),
		templateDir: templateDir,
	}
}

// Scaffold creates a new project from template
func (s *Scaffolder) Scaffold(ctx context.Context, templateName, targetDir string, vars map[string]interface{}) error {
	templatePath := filepath.Join(s.templateDir, templateName)
	
	info, err := os.Stat(templatePath)
	if err != nil {
		return fmt.Errorf("template not found: %w", err)
	}
	
	if !info.IsDir() {
		return fmt.Errorf("template must be a directory")
	}
	
	// Create target directory
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}
	
	// Walk template directory
	return filepath.Walk(templatePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Calculate relative path
		relPath, err := filepath.Rel(templatePath, path)
		if err != nil {
			return err
		}
		
		// Skip template metadata
		if relPath == "template.yaml" {
			return nil
		}
		
		// Process path template
		targetPath, err := s.engine.Execute(relPath, vars)
		if err != nil {
			return err
		}
		
		fullTargetPath := filepath.Join(targetDir, targetPath)
		
		if info.IsDir() {
			return os.MkdirAll(fullTargetPath, info.Mode())
		}
		
		// Process file content
		content, err := s.engine.ExecuteFile(path, vars)
		if err != nil {
			return err
		}
		
		return os.WriteFile(fullTargetPath, []byte(content), info.Mode())
	})
}

// ListTemplates returns available templates
func (s *Scaffolder) ListTemplates() ([]TemplateInfo, error) {
	entries, err := os.ReadDir(s.templateDir)
	if err != nil {
		return nil, err
	}
	
	var templates []TemplateInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		info, err := s.loadTemplateInfo(entry.Name())
		if err != nil {
			continue
		}
		
		templates = append(templates, info)
	}
	
	return templates, nil
}

func (s *Scaffolder) loadTemplateInfo(name string) (TemplateInfo, error) {
	metaPath := filepath.Join(s.templateDir, name, "template.yaml")
	
	// Default info
	info := TemplateInfo{
		Name:        name,
		Description: fmt.Sprintf("%s template", name),
	}
	
	// Try to load metadata
	data, err := os.ReadFile(metaPath)
	if err == nil {
		// Parse YAML metadata
		// Implementation omitted for brevity
		_ = data
	}
	
	return info, nil
}

// TemplateInfo holds template metadata
type TemplateInfo struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Variables   []string `yaml:"variables"`
}
```

## Template Commands

```go
package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/anomalyco/muster/internal/template"
	"github.com/spf13/cobra"
)

// NewTemplateCommand creates template management commands
func NewTemplateCommand(scaffolder *template.Scaffolder) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage project templates",
		Long:  "Create projects from templates and manage template definitions.",
	}

	cmd.AddCommand(newTemplateListCommand(scaffolder))
	cmd.AddCommand(newTemplateCreateCommand(scaffolder))

	return cmd
}

func newTemplateListCommand(scaffolder *template.Scaffolder) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			templates, err := scaffolder.ListTemplates()
			if err != nil {
				return err
			}
			
			if len(templates) == 0 {
				fmt.Println("No templates available")
				return nil
			}
			
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tDESCRIPTION")
			
			for _, t := range templates {
				fmt.Fprintf(w, "%s\t%s\n", t.Name, t.Description)
			}
			
			return w.Flush()
		},
	}
}

func newTemplateCreateCommand(scaffolder *template.Scaffolder) *cobra.Command {
	var vars map[string]string
	
	cmd := &cobra.Command{
		Use:   "create <template> <directory>",
		Short: "Create project from template",
		Example: `  muster template create project ./my-project --var name=myapp
  muster template create module ./modules/auth --var name=auth`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			templateName := args[0]
			targetDir := args[1]
			
			// Convert vars to interface map
			varsMap := make(map[string]interface{})
			for k, v := range vars {
				varsMap[k] = v
			}
			
			// Add default variables
			if _, ok := varsMap["project"]; !ok {
				varsMap["project"] = filepath.Base(targetDir)
			}
			
			return scaffolder.Scaffold(cmd.Context(), templateName, targetDir, varsMap)
		},
	}
	
	cmd.Flags().StringToStringVarP(&vars, "var", "v", map[string]string{}, "Template variables (key=value)")
	
	return cmd
}
```

## Variable Resolution

```go
package template

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Resolver handles interactive variable resolution
type Resolver struct {
	values  map[string]interface{}
	prompts map[string]Prompt
}

// Prompt defines a variable prompt
type Prompt struct {
	Name        string
	Description string
	Default     string
	Required    bool
	Type        string // string, int, bool
}

// NewResolver creates a new variable resolver
func NewResolver() *Resolver {
	return &Resolver{
		values:  make(map[string]interface{}),
		prompts: make(map[string]Prompt),
	}
}

// AddPrompt registers a prompt for a variable
func (r *Resolver) AddPrompt(p Prompt) {
	r.prompts[p.Name] = p
}

// Resolve prompts for missing variables
func (r *Resolver) Resolve(interactive bool) error {
	reader := bufio.NewReader(os.Stdin)
	
	for name, prompt := range r.prompts {
		// Skip if already provided
		if _, ok := r.values[name]; ok {
			continue
		}
		
		// Use default if available
		if prompt.Default != "" {
			r.values[name] = prompt.Default
			continue
		}
		
		// Require interactive mode for missing required vars
		if prompt.Required && !interactive {
			return fmt.Errorf("required variable '%s' not provided", name)
		}
		
		// Interactive prompt
		if interactive {
			value, err := r.prompt(reader, prompt)
			if err != nil {
				return err
			}
			r.values[name] = value
		}
	}
	
	return nil
}

func (r *Resolver) prompt(reader *bufio.Reader, p Prompt) (interface{}, error) {
	// Build prompt string
	promptStr := p.Description
	if p.Default != "" {
		promptStr = fmt.Sprintf("%s [%s]", promptStr, p.Default)
	}
	if p.Required {
		promptStr = promptStr + " *"
	}
	promptStr = promptStr + ": "
	
	fmt.Print(promptStr)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	
	input = strings.TrimSpace(input)
	
	// Use default if empty
	if input == "" && p.Default != "" {
		input = p.Default
	}
	
	// Validate required
	if input == "" && p.Required {
		return nil, fmt.Errorf("value required")
	}
	
	// Convert type
	switch p.Type {
	case "int":
		return strconv.Atoi(input)
	case "bool":
		return strconv.ParseBool(input)
	default:
		return input, nil
	}
}

// Get returns resolved variables
func (r *Resolver) Get() map[string]interface{} {
	return r.values
}
```

## Example Templates

```yaml
# templates/project/template.yaml
name: project
description: Standard project structure
variables:
  - name: project
    description: Project name
    required: true
  - name: author
    description: Author name
    default: Anonymous
```

```
templates/project/
├── template.yaml
├── README.md.tpl
├── go.mod.tpl
├── cmd/
│   └── {{.project}}/
│       └── main.go.tpl
└── internal/
    └── config.go.tpl
```
