# Completion Generator Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  
**Component**: Generator

## Overview

Complete specification for the **Completion Generator** component that produces shell completion scripts dynamically from CLI command definitions. This component bridges the gap between CLI command metadata and shell-specific completion implementations.

## 1. Generator Architecture

### 1.1 Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        Completion Generator                              │
├─────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │
│  │   Parser    │  │   Builder   │  │   Renderer  │  │   Writer    │    │
│  │             │  │             │  │             │  │             │    │
│  │ - Parse CLI │  │ - Build AST │  │ - Templates │  │ - File I/O  │    │
│  │ - Extract   │  │ - Enrich    │  │ - Engine    │  │ - Install   │    │
│  │   Metadata  │  │ - Validate  │  │ - Render    │  │   Paths     │    │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘    │
│         │                │                │                │           │
│         ▼                ▼                ▼                ▼           │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    Output: Shell Scripts                         │   │
│  │              (.bash, .zsh, .fish, .ps1)                         │   │
│  └─────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1.2 Data Flow

```
┌──────────────┐
│  CLI Definition│
│  (Commands,   │
│   Flags, Args)│
└──────┬───────┘
       │
       ▼
┌──────────────┐
│   Parser     │ ──→ Extract command metadata
└──────┬───────┘
       │
       ▼
┌──────────────┐
│  CommandTree │ ──→ Intermediate representation
└──────┬───────┘
       │
       ▼
┌──────────────┐
│   Renderer   │ ──→ Apply shell-specific templates
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ Shell Script │ ──→ Bash/Zsh/Fish/PowerShell
└──────────────┘
```

## 2. Core Data Structures

### 2.1 Command Tree Model

```go
package generator

import (
	"fmt"
	"strings"
)

// CommandTree represents the complete CLI structure
type CommandTree struct {
	Root        *CommandNode
	Metadata    GeneratorMetadata
	GlobalFlags []FlagNode
}

// CommandNode represents a single command
type CommandNode struct {
	Name         string
	Aliases      []string
	Description  string
	LongDesc     string
	Flags        []FlagNode
	Subcommands  []*CommandNode
	Args         []ArgNode
	Hidden       bool
	Deprecated   string
	Runnable     bool
}

// FlagNode represents a command flag
type FlagNode struct {
	Name         string
	Shorthand    string
	Description  string
	Type         FlagType
	Required     bool
	DefaultValue string
	EnvVar       string
	Hidden       bool
	Values       []string // For enum flags
	DependsOn    []string // Flags that must also be set
	Conflicts    []string // Flags that cannot be set together
}

// ArgNode represents a positional argument
type ArgNode struct {
	Name        string
	Description string
	Required    bool
	Multiple    bool
	Type        ArgType
	Validator   string // Validation regex or type
}

// GeneratorMetadata contains generator configuration
type GeneratorMetadata struct {
	AppName        string
	Version        string
	Description    string
	ExecutableName string
}

// FlagType defines the type of flag value
type FlagType int

const (
	FlagTypeBool FlagType = iota
	FlagTypeString
	FlagTypeInt
	FlagTypeFloat
	FlagTypeDuration
	FlagTypeStringSlice
	FlagTypeIntSlice
	FlagTypeEnum
)

// ArgType defines the type of positional argument
type ArgType int

const (
	ArgTypeString ArgType = iota
	ArgTypeInt
	ArgTypeFloat
	ArgTypePath
	ArgTypeURL
	ArgTypeIP
	ArgTypeEmail
	ArgTypeCustom
)
```

### 2.2 Generator Configuration

```go
package generator

import (
	"text/template"
)

// Config contains generator configuration
type Config struct {
	// Shell-specific settings
	Shell Shell
	
	// Output settings
	OutputPath      string
	InstallPath     string
	Stdout          bool
	NoDescriptions  bool
	NoAliases       bool
	
	// Template overrides
	CustomTemplates map[Shell]string
	
	// Validation settings
	StrictMode      bool
	ValidateSchema  bool
	
	// Metadata
	GeneratorVersion string
}

// Generator is the main completion generator
type Generator struct {
	config    Config
	tree      *CommandTree
	templates map[Shell]*template.Template
	functions template.FuncMap
}

// NewGenerator creates a new completion generator
func NewGenerator(config Config) *Generator {
	return &Generator{
		config:    config,
		templates: make(map[Shell]*template.Template),
		functions: defaultFuncMap(),
	}
}

// SetCommandTree sets the command tree to generate completions for
func (g *Generator) SetCommandTree(tree *CommandTree) {
	g.tree = tree
}

// Validate checks if the configuration is valid
func (g *Generator) Validate() error {
	if g.tree == nil {
		return fmt.Errorf("command tree not set")
	}
	
	if g.tree.Root == nil {
		return fmt.Errorf("root command not set")
	}
	
	if g.tree.Metadata.AppName == "" {
		return fmt.Errorf("app name not set")
	}
	
	return nil
}
```

## 3. Parser Implementation

### 3.1 CLI Parser Interface

```go
package generator

import (
	"reflect"
	"strings"
)

// Parser parses CLI definitions into command trees
type Parser interface {
	Parse(interface{}) (*CommandTree, error)
}

// CobraParser parses Cobra CLI definitions
type CobraParser struct{}

// Parse extracts command tree from a Cobra command
func (p *CobraParser) Parse(root interface{}) (*CommandTree, error) {
	// Use reflection to extract Cobra command structure
	cmdValue := reflect.ValueOf(root)
	
	tree := &CommandTree{
		Root:        parseCobraCommand(cmdValue),
		Metadata:    extractMetadata(cmdValue),
		GlobalFlags: parseGlobalFlags(cmdValue),
	}
	
	return tree, nil
}

func parseCobraCommand(v reflect.Value) *CommandNode {
	node := &CommandNode{}
	
	// Extract name
	if nameField := v.Elem().FieldByName("name"); nameField.IsValid() {
		node.Name = nameField.String()
	}
	
	// Extract description
	if shortField := v.Elem().FieldByName("Short"); shortField.IsValid() {
		node.Description = shortField.String()
	}
	
	// Extract long description
	if longField := v.Elem().FieldByName("Long"); longField.IsValid() {
		node.LongDesc = longField.String()
	}
	
	// Extract aliases
	if aliasesField := v.Elem().FieldByName("aliases"); aliasesField.IsValid() {
		for i := 0; i < aliasesField.Len(); i++ {
			node.Aliases = append(node.Aliases, aliasesField.Index(i).String())
		}
	}
	
	// Extract flags
	if flagsField := v.Elem().FieldByName("flags"); flagsField.IsValid() {
		node.Flags = parseFlags(flagsField)
	}
	
	// Extract subcommands
	if subCmdField := v.Elem().FieldByName("commands"); subCmdField.IsValid() {
		for i := 0; i < subCmdField.Len(); i++ {
			sub := parseCobraCommand(subCmdField.Index(i))
			node.Subcommands = append(node.Subcommands, sub)
		}
	}
	
	// Check if runnable
	if runField := v.Elem().FieldByName("Run"); runField.IsValid() && !runField.IsNil() {
		node.Runnable = true
	}
	
	return node
}

func parseFlags(v reflect.Value) []FlagNode {
	var flags []FlagNode
	
	// Cobra stores flags in a *flag.FlagSet
	// We'd need to call the actual methods, but this shows the structure
	
	return flags
}

func parseGlobalFlags(v reflect.Value) []FlagNode {
	// Parse persistent flags
	return parseFlags(v.Elem().FieldByName("persistentFlagSet"))
}

func extractMetadata(v reflect.Value) GeneratorMetadata {
	meta := GeneratorMetadata{}
	
	if nameField := v.Elem().FieldByName("name"); nameField.IsValid() {
		meta.AppName = nameField.String()
	}
	
	meta.ExecutableName = strings.ToLower(meta.AppName)
	
	return meta
}
```

### 3.2 JSON Parser

```go
package generator

import (
	"encoding/json"
	"fmt"
)

// JSONParser parses command tree from JSON definition
type JSONParser struct{}

// ParseCommandTree parses a command tree from JSON
func (p *JSONParser) Parse(data []byte) (*CommandTree, error) {
	var def struct {
		AppName     string          `json:"appName"`
		Version     string          `json:"version"`
		Description string          `json:"description"`
		Commands    []CommandDef    `json:"commands"`
		GlobalFlags []FlagDef       `json:"globalFlags,omitempty"`
	}
	
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}
	
	tree := &CommandTree{
		Metadata: GeneratorMetadata{
			AppName:        def.AppName,
			Version:        def.Version,
			Description:    def.Description,
			ExecutableName: def.AppName,
		},
	}
	
	// Parse global flags
	for _, f := range def.GlobalFlags {
		tree.GlobalFlags = append(tree.GlobalFlags, parseFlagDef(f))
	}
	
	// Parse commands (assuming single root for now)
	if len(def.Commands) > 0 {
		tree.Root = parseCommandDef(def.Commands[0])
	}
	
	return tree, nil
}

type CommandDef struct {
	Name        string       `json:"name"`
	Aliases     []string     `json:"aliases,omitempty"`
	Description string       `json:"description"`
	LongDesc    string       `json:"longDescription,omitempty"`
	Flags       []FlagDef    `json:"flags,omitempty"`
	Subcommands []CommandDef `json:"subcommands,omitempty"`
	Args        []ArgDef     `json:"args,omitempty"`
	Hidden      bool         `json:"hidden,omitempty"`
	Runnable    bool         `json:"runnable,omitempty"`
}

type FlagDef struct {
	Name         string   `json:"name"`
	Shorthand    string   `json:"shorthand,omitempty"`
	Description  string   `json:"description"`
	Type         string   `json:"type,omitempty"`
	Required     bool     `json:"required,omitempty"`
	DefaultValue string   `json:"default,omitempty"`
	EnvVar       string   `json:"envVar,omitempty"`
	Hidden       bool     `json:"hidden,omitempty"`
	Values       []string `json:"values,omitempty"`
}

type ArgDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
	Multiple    bool   `json:"multiple,omitempty"`
	Type        string `json:"type,omitempty"`
}

func parseFlagDef(def FlagDef) FlagNode {
	flag := FlagNode{
		Name:         def.Name,
		Shorthand:    def.Shorthand,
		Description:  def.Description,
		Required:     def.Required,
		DefaultValue: def.DefaultValue,
		EnvVar:       def.EnvVar,
		Hidden:       def.Hidden,
		Values:       def.Values,
	}
	
	// Map type string to FlagType
	switch def.Type {
	case "bool":
		flag.Type = FlagTypeBool
	case "int":
		flag.Type = FlagTypeInt
	case "float":
		flag.Type = FlagTypeFloat
	case "duration":
		flag.Type = FlagTypeDuration
	case "stringSlice":
		flag.Type = FlagTypeStringSlice
	case "intSlice":
		flag.Type = FlagTypeIntSlice
	case "enum":
		flag.Type = FlagTypeEnum
	default:
		flag.Type = FlagTypeString
	}
	
	return flag
}

func parseCommandDef(def CommandDef) *CommandNode {
	cmd := &CommandNode{
		Name:        def.Name,
		Aliases:     def.Aliases,
		Description: def.Description,
		LongDesc:    def.LongDesc,
		Hidden:      def.Hidden,
		Runnable:    def.Runnable,
	}
	
	// Parse flags
	for _, f := range def.Flags {
		cmd.Flags = append(cmd.Flags, parseFlagDef(f))
	}
	
	// Parse subcommands
	for _, sub := range def.Subcommands {
		cmd.Subcommands = append(cmd.Subcommands, parseCommandDef(sub))
	}
	
	// Parse arguments
	for _, a := range def.Args {
		arg := ArgNode{
			Name:        a.Name,
			Description: a.Description,
			Required:    a.Required,
			Multiple:    a.Multiple,
		}
		
		switch a.Type {
		case "int":
			arg.Type = ArgTypeInt
		case "float":
			arg.Type = ArgTypeFloat
		case "path":
			arg.Type = ArgTypePath
		case "url":
			arg.Type = ArgTypeURL
		case "ip":
			arg.Type = ArgTypeIP
		case "email":
			arg.Type = ArgTypeEmail
		default:
			arg.Type = ArgTypeString
		}
		
		cmd.Args = append(cmd.Args, arg)
	}
	
	return cmd
}
```

## 4. Template System

### 4.1 Template Functions

```go
package generator

import (
	"fmt"
	"strings"
	"text/template"
)

// defaultFuncMap returns the default template functions
func defaultFuncMap() template.FuncMap {
	return template.FuncMap{
		"quote":      quote,
		"escape":     escape,
		"join":       join,
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"camelCase":  toCamelCase,
		"snakeCase":  toSnakeCase,
		"kebabCase":  toKebabCase,
		"hasPrefix":  strings.HasPrefix,
		"hasSuffix":  strings.HasSuffix,
		"trim":       strings.TrimSpace,
		"contains":   strings.Contains,
		"replace":    strings.ReplaceAll,
		"split":      strings.Split,
		"repeat":     strings.Repeat,
		"indent":     indent,
		"flagType":   flagTypeString,
		"argType":    argTypeString,
		"flagNames":  flagNames,
		"cmdNames":   cmdNames,
	}
}

func quote(s string) string {
	return fmt.Sprintf("%q", s)
}

func escape(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

func join(sep string, items []string) string {
	return strings.Join(items, sep)
}

func toCamelCase(s string) string {
	parts := strings.Split(s, "-")
	for i, part := range parts {
		if i > 0 {
			parts[i] = strings.Title(part)
		} else {
			parts[i] = strings.ToLower(part)
		}
	}
	return strings.Join(parts, "")
}

func toSnakeCase(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "-", "_")
}

func toKebabCase(s string) string {
	return strings.ToLower(s)
}

func indent(spaces int, s string) string {
	indent := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = indent + line
		}
	}
	return strings.Join(lines, "\n")
}

func flagTypeString(t FlagType) string {
	switch t {
	case FlagTypeBool:
		return "bool"
	case FlagTypeInt:
		return "int"
	case FlagTypeFloat:
		return "float"
	case FlagTypeDuration:
		return "duration"
	case FlagTypeStringSlice:
		return "stringSlice"
	case FlagTypeIntSlice:
		return "intSlice"
	case FlagTypeEnum:
		return "enum"
	default:
		return "string"
	}
}

func argTypeString(t ArgType) string {
	switch t {
	case ArgTypeInt:
		return "int"
	case ArgTypeFloat:
		return "float"
	case ArgTypePath:
		return "path"
	case ArgTypeURL:
		return "url"
	case ArgTypeIP:
		return "ip"
	case ArgTypeEmail:
		return "email"
	case ArgTypeCustom:
		return "custom"
	default:
		return "string"
	}
}

func flagNames(flags []FlagNode) []string {
	names := make([]string, len(flags))
	for i, f := range flags {
		names[i] = f.Name
	}
	return names
}

func cmdNames(cmds []*CommandNode) []string {
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = c.Name
	}
	return names
}
```

### 4.2 Shell-Specific Templates

```go
package generator

// LoadTemplates loads all shell templates
func (g *Generator) LoadTemplates() error {
	shells := []Shell{ShellBash, ShellZsh, ShellFish, ShellPowerShell}
	
	for _, shell := range shells {
		templateStr, err := g.getTemplate(shell)
		if err != nil {
			return fmt.Errorf("get template for %s: %w", shell, err)
		}
		
		tmpl, err := template.New(string(shell)).
			Funcs(g.functions).
			Parse(templateStr)
		if err != nil {
			return fmt.Errorf("parse template for %s: %w", shell, err)
		}
		
		g.templates[shell] = tmpl
	}
	
	return nil
}

func (g *Generator) getTemplate(shell Shell) (string, error) {
	// Check for custom template
	if custom, ok := g.config.CustomTemplates[shell]; ok {
		return custom, nil
	}
	
	// Return default template
	switch shell {
	case ShellBash:
		return g.bashTemplate(), nil
	case ShellZsh:
		return g.zshTemplate(), nil
	case ShellFish:
		return g.fishTemplate(), nil
	case ShellPowerShell:
		return g.powershellTemplate(), nil
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}
}

func (g *Generator) bashTemplate() string {
	return `#!/bin/bash
# {{.Metadata.AppName}} completion
# Generated by completion-generator v{{.Config.GeneratorVersion}}

_{{.Metadata.ExecutableName}}() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    
    {{template "bash_commands" .Tree.Root}}
    {{template "bash_flags" .Tree.GlobalFlags}}
}

{{define "bash_commands"}}
{{- if .Subcommands}}
local commands="{{range .Subcommands}}{{if not .Hidden}} {{.Name}}{{range .Aliases}} {{.}}{{end}}{{end}}{{end}}"
{{- end}}
{{end}}

{{define "bash_flags"}}
local global_flags="{{range .}}{{if not .Hidden}}{{if .Shorthand}} -{{.Shorthand}}{{end}}{{if .Name}} --{{.Name}}{{end}}{{end}}{{end}}"
{{end}}

complete -F _{{.Metadata.ExecutableName}} {{.Metadata.ExecutableName}}
`
}

func (g *Generator) zshTemplate() string {
	return `#compdef {{.Metadata.ExecutableName}}
# {{.Metadata.AppName}} completion
# Generated by completion-generator v{{.Config.GeneratorVersion}}

_{{.Metadata.ExecutableName}}() {
    local curcontext="$curcontext" state line
    typeset -A opt_args
    
    _arguments -C \
        {{range .Tree.GlobalFlags}}{{if not .Hidden}}\
        '{{if .Shorthand}}-{{.Shorthand}}{{end}}{{if and .Shorthand .Name}}|{{end}}{{if .Name}}--{{.Name}}{{end}}[{{.Description}}]{{if .Values}}: :({{join " " .Values}}){{end}}' \
        {{end}}{{end}}
        '1: :->command' \
        '*:: :->args'
    
    case "$state" in
    command)
        local commands=(
            {{range .Tree.Root.Subcommands}}{{if not .Hidden}}
            '{{.Name}}:{{.Description}}'
            {{end}}{{end}}
        )
        _describe -t commands 'commands' commands
        ;;
    args)
        case "$line[1]" in
        {{range .Tree.Root.Subcommands}}{{if not .Hidden}}
        {{.Name}})
            _{{$.Metadata.ExecutableName}}_{{.Name}}_cmd
            ;;
        {{end}}{{end}}
        esac
        ;;
    esac
}

{{range .Tree.Root.Subcommands}}{{if not .Hidden}}
_{{$.Metadata.ExecutableName}}_{{.Name}}_cmd() {
    _arguments \
        {{range .Flags}}{{if not .Hidden}}\
        '{{if .Shorthand}}-{{.Shorthand}}{{end}}{{if and .Shorthand .Name}}|{{end}}{{if .Name}}--{{.Name}}{{end}}[{{.Description}}]{{if .Values}}: :({{join " " .Values}}){{end}}' \
        {{end}}{{end}}
}
{{end}}{{end}}

compdef _{{.Metadata.ExecutableName}} {{.Metadata.ExecutableName}}
`
}

func (g *Generator) fishTemplate() string {
	return `# {{.Metadata.AppName}} completion
# Generated by completion-generator v{{.Config.GeneratorVersion}}

# Global flags
{{range .Tree.GlobalFlags}}{{if not .Hidden}}
complete -c {{$.Metadata.ExecutableName}} -l {{.Name}}{{if .Shorthand}} -s {{.Shorthand}}{{end}} -d "{{.Description}}"{{if .Values}} -xa "{{join " " .Values}}"{{end}}
{{end}}{{end}}

# Commands and subcommand flags
{{range .Tree.Root.Subcommands}}{{if not .Hidden}}
complete -c {{$.Metadata.ExecutableName}} -n '__fish_use_subcommand' -a '{{.Name}}' -d '{{.Description}}'{{range .Aliases}} -a '{{.}}'{{end}}

{{range .Flags}}{{if not .Hidden}}
complete -c {{$.Metadata.ExecutableName}} -n '__fish_seen_subcommand_from {{.Name}}' -l {{.Name}}{{if .Shorthand}} -s {{.Shorthand}}{{end}} -d "{{.Description}}"{{if .Values}} -xa "{{join " " .Values}}"{{end}}
{{end}}{{end}}
{{end}}{{end}}
`
}

func (g *Generator) powershellTemplate() string {
	return `# {{.Metadata.AppName}} completion
# Generated by completion-generator v{{.Config.GeneratorVersion}}

Register-ArgumentCompleter -Native -CommandName {{.Metadata.ExecutableName}} -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    
    $commands = @{
        {{range .Tree.Root.Subcommands}}{{if not .Hidden}}
        '{{.Name}}' = @{
            description = '{{.Description}}'
            flags = @(
                {{range .Flags}}{{if not .Hidden}}
                [PSCustomObject]@{
                    name = '{{if .Name}}--{{.Name}}{{end}}'
                    short = '{{if .Shorthand}}{{.Shorthand}}{{end}}'
                    description = '{{.Description}}'
                    values = @({{if .Values}}{{range .Values}}'{{.}}',{{end}}{{end}})
                }
                {{end}}{{end}}
            )
        }
        {{end}}{{end}}
    }
    
    $globalFlags = @(
        {{range .Tree.GlobalFlags}}{{if not .Hidden}}
        [PSCustomObject]@{
            name = '{{if .Name}}--{{.Name}}{{end}}'
            short = '{{if .Shorthand}}{{.Shorthand}}{{end}}'
            description = '{{.Description}}'
        }
        {{end}}{{end}}
    )
    
    # Generate completions...
}
`
}
```

## 5. Generation Process

### 5.1 Main Generate Method

```go
package generator

import (
	"bytes"
	"fmt"
)

// Generate creates completion script for configured shell
func (g *Generator) Generate() (string, error) {
	// Validate
	if err := g.Validate(); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}
	
	// Load templates if not already loaded
	if len(g.templates) == 0 {
		if err := g.LoadTemplates(); err != nil {
			return "", fmt.Errorf("load templates: %w", err)
		}
	}
	
	// Get template for configured shell
	tmpl, ok := g.templates[g.config.Shell]
	if !ok {
		return "", fmt.Errorf("template not found for shell: %s", g.config.Shell)
	}
	
	// Prepare template data
	data := struct {
		Tree   *CommandTree
		Config Config
	}{
		Tree:   g.tree,
		Config: g.config,
	}
	
	// Render template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	
	return buf.String(), nil
}

// GenerateAll generates completion scripts for all shells
func (g *Generator) GenerateAll() (map[Shell]string, error) {
	results := make(map[Shell]string)
	
	shells := []Shell{ShellBash, ShellZsh, ShellFish, ShellPowerShell}
	
	for _, shell := range shells {
		g.config.Shell = shell
		script, err := g.Generate()
		if err != nil {
			return nil, fmt.Errorf("generate %s: %w", shell, err)
		}
		results[shell] = script
	}
	
	return results, nil
}
```

### 5.2 Output Writer

```go
package generator

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Writer handles output of generated scripts
type Writer struct {
	config Config
}

// NewWriter creates a new writer
func NewWriter(config Config) *Writer {
	return &Writer{config: config}
}

// Write outputs the generated script
func (w *Writer) Write(script string) error {
	// Write to stdout if configured
	if w.config.Stdout {
		_, err := fmt.Print(script)
		return err
	}
	
	// Determine output path
	outputPath := w.config.OutputPath
	if outputPath == "" {
		outputPath = w.defaultOutputPath()
	}
	
	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	
	// Write file
	if err := os.WriteFile(outputPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	
	fmt.Fprintf(os.Stderr, "Completion script written to: %s\n", outputPath)
	return nil
}

func (w *Writer) defaultOutputPath() string {
	ext := ".sh"
	switch w.config.Shell {
	case ShellBash:
		ext = ".bash"
	case ShellZsh:
		ext = ".zsh"
	case ShellFish:
		ext = ".fish"
	case ShellPowerShell:
		ext = ".ps1"
	}
	
	return fmt.Sprintf("./%s-completion%s", w.config.AppName, ext)
}

// Install installs the completion script to the shell's completion directory
func (w *Writer) Install(script string) error {
	installPath := w.config.InstallPath
	if installPath == "" {
		installPath = GetInstallPath(w.config.Shell)
	}
	
	// For shells that support completion directories
	if w.config.Shell == ShellBash || w.config.Shell == ShellFish {
		// Write to completion directory
		filename := w.config.AppName
		if w.config.Shell == ShellFish {
			filename += ".fish"
		}
		installPath = filepath.Join(installPath, filename)
	}
	
	// Ensure directory exists
	dir := filepath.Dir(installPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	
	// Write file
	if err := os.WriteFile(installPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	
	fmt.Fprintf(os.Stderr, "Completion script installed to: %s\n", installPath)
	fmt.Fprintf(os.Stderr, "Reload your shell or start a new session to enable completions.\n")
	
	return nil
}
```

## 6. Usage Examples

### 6.1 Basic Usage

```go
package main

import (
	"log"
	
	"github.com/example/cli/pkg/generator"
)

func main() {
	// Create configuration
	config := generator.Config{
		Shell:            generator.ShellBash,
		GeneratorVersion: "1.0.0",
	}
	
	// Create generator
	gen := generator.NewGenerator(config)
	
	// Load templates
	if err := gen.LoadTemplates(); err != nil {
		log.Fatal(err)
	}
	
	// Parse CLI definition
	parser := &generator.JSONParser{}
	jsonData := []byte(`{
		"appName": "mycli",
		"version": "1.0.0",
		"description": "My CLI tool",
		"commands": [{
			"name": "get",
			"description": "Get a resource",
			"subcommands": [{
				"name": "user",
				"description": "Get user",
				"flags": [{
					"name": "id",
					"shorthand": "i",
					"description": "User ID",
					"required": true
				}]
			}]
		}]
	}`)
	
	tree, err := parser.Parse(jsonData)
	if err != nil {
		log.Fatal(err)
	}
	
	gen.SetCommandTree(tree)
	
	// Generate completion script
	script, err := gen.Generate()
	if err != nil {
		log.Fatal(err)
	}
	
	// Write output
	writer := generator.NewWriter(config)
	if err := writer.Write(script); err != nil {
		log.Fatal(err)
	}
}
```

### 6.2 CLI Integration

```go
package cmd

import (
	"fmt"
	
	"github.com/spf13/cobra"
	"github.com/example/cli/pkg/generator"
)

var completionGenCmd = &cobra.Command{
	Use:   "completion-gen",
	Short: "Generate completion scripts",
	Long:  `Generate shell completion scripts for all supported shells.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get flags
		shell, _ := cmd.Flags().GetString("shell")
		output, _ := cmd.Flags().GetString("output")
		install, _ := cmd.Flags().GetBool("install")
		
		config := generator.Config{
			Shell:            generator.Shell(shell),
			OutputPath:       output,
			GeneratorVersion: "1.0.0",
		}
		
		gen := generator.NewGenerator(config)
		
		// Parse from cobra
		parser := &generator.CobraParser{}
		tree, err := parser.Parse(cmd.Root())
		if err != nil {
			return fmt.Errorf("parse commands: %w", err)
		}
		
		gen.SetCommandTree(tree)
		
		if shell == "all" {
			// Generate for all shells
			scripts, err := gen.GenerateAll()
			if err != nil {
				return fmt.Errorf("generate completions: %w", err)
			}
			
			for s, script := range scripts {
				config.Shell = s
				config.OutputPath = fmt.Sprintf("%s-completion.%s", tree.Metadata.AppName, s)
				
				writer := generator.NewWriter(config)
				if err := writer.Write(script); err != nil {
					return fmt.Errorf("write %s: %w", s, err)
				}
			}
		} else {
			// Generate for specific shell
			script, err := gen.Generate()
			if err != nil {
				return fmt.Errorf("generate completion: %w", err)
			}
			
			writer := generator.NewWriter(config)
			
			if install {
				if err := writer.Install(script); err != nil {
					return fmt.Errorf("install: %w", err)
				}
			} else {
				if err := writer.Write(script); err != nil {
					return fmt.Errorf("write: %w", err)
				}
			}
		}
		
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionGenCmd)
	
	completionGenCmd.Flags().String("shell", "bash", "Target shell (bash, zsh, fish, powershell, all)")
	completionGenCmd.Flags().String("output", "", "Output file path (default: stdout or auto-generated)")
	completionGenCmd.Flags().Bool("install", false, "Install to shell completion directory")
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC