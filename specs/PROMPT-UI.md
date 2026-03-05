# Prompt UI Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

## Overview

The Prompt UI specification defines interactive command-line interfaces for the OpenAPI CLI tool. It provides guided user experiences for complex operations such as initial setup, authentication flows, interactive API exploration, and configuration management through intuitive prompts and forms.

## Prompt Types

```
┌─────────────────────────────────────────────────────────────┐
│                     PROMPT UI TYPES                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   Input      │  │   Select     │  │  Confirm     │      │
│  │  (Text)      │  │  (Single)    │  │  (Yes/No)    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  MultiSelect │  │   Password   │  │   Survey     │      │
│  │  (Checkbox)  │  │  (Masked)    │  │  (Wizard)    │      │
│  └──────────────┘  └──────────────┘  └──────────────┘      │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐                        │
│  │   Progress   │  │  Spinner     │                        │
│  │   (Bar)      │  │  (Loading)   │                        │
│  └──────────────┘  └──────────────┘                        │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## Technical Implementation

### Prompt Interface

```go
package prompt

import (
    "github.com/AlecAivazis/survey/v2"
    "github.com/charmbracelet/bubbles/spinner"
    tea "github.com/charmbracelet/bubbletea"
)

// Prompt defines the interface for all prompt types
type Prompt interface {
    Ask() (interface{}, error)
    Validate(input interface{}) error
    Render() string
}

// PromptManager manages prompt execution and state
type PromptManager struct {
    theme     *Theme
    validator Validator
    history   []PromptRecord
}

func NewPromptManager() *PromptManager {
    return &PromptManager{
        theme:     DefaultTheme(),
        validator: NewDefaultValidator(),
        history:   make([]PromptRecord, 0),
    }
}
```

### Input Prompt

```go
package prompt

// InputPrompt captures text input from users
type InputPrompt struct {
    Message     string
    Default     string
    Help        string
    Placeholder string
    Validator   func(string) error
    Suggester   func(string) []string
}

func NewInputPrompt(message string) *InputPrompt {
    return &InputPrompt{
        Message: message,
    }
}

func (p *InputPrompt) Ask() (string, error) {
    var result string
    
    prompt := &survey.Input{
        Message: p.Message,
        Default: p.Default,
        Help:    p.Help,
        Suggest: p.Suggester,
    }
    
    opts := []survey.AskOpt{}
    if p.Validator != nil {
        opts = append(opts, survey.WithValidator(survey.Validator(p.Validator)))
    }
    
    err := survey.AskOne(prompt, &result, opts...)
    if err != nil {
        return "", fmt.Errorf("input prompt failed: %w", err)
    }
    
    return result, nil
}

// Example usage
func askForHost() (string, error) {
    prompt := NewInputPrompt("Enter API host URL:")
    prompt.Default = "https://api.example.com"
    prompt.Help = "The base URL of the API you want to connect to"
    prompt.Validator = func(input string) error {
        if input == "" {
            return fmt.Errorf("host URL cannot be empty")
        }
        if _, err := url.Parse(input); err != nil {
            return fmt.Errorf("invalid URL format")
        }
        return nil
    }
    prompt.Suggester = func(toComplete string) []string {
        suggestions := []string{
            "https://api.example.com",
            "https://api.staging.example.com",
            "https://localhost:8080",
        }
        var matches []string
        for _, s := range suggestions {
            if strings.HasPrefix(s, toComplete) {
                matches = append(matches, s)
            }
        }
        return matches
    }
    
    return prompt.Ask()
}
```

### Select Prompt

```go
package prompt

// SelectPrompt allows users to choose from a list of options
type SelectPrompt struct {
    Message  string
    Options  []string
    Default  string
    Help     string
    PageSize int
}

func NewSelectPrompt(message string, options []string) *SelectPrompt {
    return &SelectPrompt{
        Message:  message,
        Options:  options,
        PageSize: 10,
    }
}

func (p *SelectPrompt) Ask() (string, error) {
    var result string
    
    prompt := &survey.Select{
        Message:  p.Message,
        Options:  p.Options,
        Default:  p.Default,
        Help:     p.Help,
        PageSize: p.PageSize,
    }
    
    err := survey.AskOne(prompt, &result)
    if err != nil {
        return "", fmt.Errorf("select prompt failed: %w", err)
    }
    
    return result, nil
}

// Example: Profile selection
func selectProfile(profiles []string) (string, error) {
    prompt := NewSelectPrompt("Select configuration profile:", profiles)
    prompt.Default = "default"
    prompt.Help = "Choose a pre-configured profile for this session"
    prompt.PageSize = 5
    
    return prompt.Ask()
}
```

### Multi-Select Prompt

```go
package prompt

// MultiSelectPrompt allows multiple selections
type MultiSelectPrompt struct {
    Message  string
    Options  []string
    Default  []string
    Help     string
    PageSize int
}

func NewMultiSelectPrompt(message string, options []string) *MultiSelectPrompt {
    return &MultiSelectPrompt{
        Message:  message,
        Options:  options,
        PageSize: 10,
    }
}

func (p *MultiSelectPrompt) Ask() ([]string, error) {
    var result []string
    
    prompt := &survey.MultiSelect{
        Message:  p.Message,
        Options:  p.Options,
        Default:  p.Default,
        Help:     p.Help,
        PageSize: p.PageSize,
    }
    
    err := survey.AskOne(prompt, &result)
    if err != nil {
        return nil, fmt.Errorf("multiselect prompt failed: %w", err)
    }
    
    return result, nil
}

// Example: Resource selection for caching
func selectResourcesToCache(resources []string) ([]string, error) {
    prompt := NewMultiSelectPrompt(
        "Select resources to cache locally:",
        resources,
    )
    prompt.Default = resources
    prompt.Help = "Cached resources are available offline"
    
    return prompt.Ask()
}
```

### Confirm Prompt

```go
package prompt

// ConfirmPrompt captures yes/no decisions
type ConfirmPrompt struct {
    Message string
    Default bool
    Help    string
}

func NewConfirmPrompt(message string) *ConfirmPrompt {
    return &ConfirmPrompt{
        Message: message,
        Default: true,
    }
}

func (p *ConfirmPrompt) Ask() (bool, error) {
    var result bool
    
    prompt := &survey.Confirm{
        Message: p.Message,
        Default: p.Default,
        Help:    p.Help,
    }
    
    err := survey.AskOne(prompt, &result)
    if err != nil {
        return false, fmt.Errorf("confirm prompt failed: %w", err)
    }
    
    return result, nil
}

// Example: SSL certificate verification bypass
func confirmInsecureConnection() (bool, error) {
    prompt := NewConfirmPrompt(
        "Skip TLS certificate verification?",
    )
    prompt.Default = false
    prompt.Help = "Only use this for development/testing environments"
    
    return prompt.Ask()
}
```

### Password Prompt

```go
package prompt

// PasswordPrompt captures sensitive input
type PasswordPrompt struct {
    Message string
    Help    string
    Confirm bool // Require confirmation
}

func NewPasswordPrompt(message string) *PasswordPrompt {
    return &PasswordPrompt{
        Message: message,
        Confirm: false,
    }
}

func (p *PasswordPrompt) Ask() (string, error) {
    var result string
    
    prompt := &survey.Password{
        Message: p.Message,
        Help:    p.Help,
    }
    
    err := survey.AskOne(prompt, &result)
    if err != nil {
        return "", fmt.Errorf("password prompt failed: %w", err)
    }
    
    if p.Confirm {
        confirmPrompt := &survey.Password{
            Message: "Confirm password:",
        }
        var confirm string
        err = survey.AskOne(confirmPrompt, &confirm)
        if err != nil {
            return "", fmt.Errorf("password confirmation failed: %w", err)
        }
        
        if result != confirm {
            return "", fmt.Errorf("passwords do not match")
        }
    }
    
    return result, nil
}
```

### Survey/Wizard

```go
package prompt

// Survey orchestrates multiple prompts in sequence
type Survey struct {
    Title       string
    Description string
    Steps       []SurveyStep
    OnComplete  func(map[string]interface{}) error
}

type SurveyStep struct {
    Name     string
    Prompt   Prompt
    Required bool
    SkipIf   func(answers map[string]interface{}) bool
}

func (s *Survey) Run() (map[string]interface{}, error) {
    answers := make(map[string]interface{})
    
    fmt.Printf("\n%s\n", s.Title)
    if s.Description != "" {
        fmt.Printf("%s\n\n", s.Description)
    }
    
    for i, step := range s.Steps {
        // Check skip condition
        if step.SkipIf != nil && step.SkipIf(answers) {
            continue
        }
        
        fmt.Printf("Step %d of %d\n", i+1, len(s.Steps))
        
        answer, err := step.Prompt.Ask()
        if err != nil {
            if step.Required {
                return nil, fmt.Errorf("required step '%s' failed: %w", step.Name, err)
            }
            // Skip optional failed steps
            continue
        }
        
        answers[step.Name] = answer
    }
    
    if s.OnComplete != nil {
        if err := s.OnComplete(answers); err != nil {
            return nil, fmt.Errorf("survey completion failed: %w", err)
        }
    }
    
    return answers, nil
}

// Example: Initialization wizard
func runInitWizard() error {
    survey := &Survey{
        Title:       "OpenAPI CLI Initialization",
        Description: "Configure your connection to an OpenAPI-based API",
    }
    
    survey.Steps = []SurveyStep{
        {
            Name:     "host",
            Prompt:   createHostPrompt(),
            Required: true,
        },
        {
            Name:     "auth_type",
            Prompt:   createAuthTypePrompt(),
            Required: true,
        },
        {
            Name:     "username",
            Prompt:   createUsernamePrompt(),
            Required: false,
            SkipIf: func(answers map[string]interface{}) bool {
                authType, _ := answers["auth_type"].(string)
                return authType != "basic"
            },
        },
        {
            Name:     "password",
            Prompt:   createPasswordPrompt(),
            Required: false,
            SkipIf: func(answers map[string]interface{}) bool {
                authType, _ := answers["auth_type"].(string)
                return authType != "basic"
            },
        },
        {
            Name:     "cert_path",
            Prompt:   createCertPathPrompt(),
            Required: false,
            SkipIf: func(answers map[string]interface{}) bool {
                authType, _ := answers["auth_type"].(string)
                return authType != "mtls"
            },
        },
        {
            Name:     "cache_resources",
            Prompt:   createCachePrompt(),
            Required: true,
        },
    }
    
    survey.OnComplete = func(answers map[string]interface{}) error {
        // Save configuration
        config := buildConfigFromAnswers(answers)
        return saveConfig(config)
    }
    
    _, err := survey.Run()
    return err
}
```

### Progress Indicators

```go
package prompt

// ProgressBar displays operation progress
type ProgressBar struct {
    Total       int
    Current     int
    Description string
    Width       int
}

func NewProgressBar(total int) *ProgressBar {
    return &ProgressBar{
        Total: total,
        Width: 40,
    }
}

func (pb *ProgressBar) Update(current int, description string) {
    pb.Current = current
    if description != "" {
        pb.Description = description
    }
    pb.Render()
}

func (pb *ProgressBar) Render() {
    percent := float64(pb.Current) / float64(pb.Total)
    filled := int(percent * float64(pb.Width))
    
    bar := strings.Repeat("█", filled) + strings.Repeat("░", pb.Width-filled)
    
    fmt.Printf("\r%s [%s] %.0f%% (%d/%d)",
        pb.Description,
        bar,
        percent*100,
        pb.Current,
        pb.Total,
    )
    
    if pb.Current >= pb.Total {
        fmt.Println()
    }
}

// Spinner displays indeterminate progress
type Spinner struct {
    Message string
    s       spinner.Model
}

func NewSpinner(message string) *Spinner {
    s := spinner.New()
    s.Spinner = spinner.Dot
    
    return &Spinner{
        Message: message,
        s:       s,
    }
}

func (s *Spinner) Start() {
    fmt.Printf("%s %s\n", s.s.View(), s.Message)
}

func (s *Spinner) Stop(success bool) {
    if success {
        fmt.Printf("✓ %s\n", s.Message)
    } else {
        fmt.Printf("✗ %s\n", s.Message)
    }
}
```

### Interactive Table

```go
package prompt

// InteractiveTable displays selectable tabular data
type InteractiveTable struct {
    Headers []string
    Rows    [][]string
    Message string
}

func NewInteractiveTable(headers []string, rows [][]string) *InteractiveTable {
    return &InteractiveTable{
        Headers: headers,
        Rows:    rows,
    }
}

func (t *InteractiveTable) Display() (int, error) {
    // Format options for display
    options := make([]string, len(t.Rows))
    for i, row := range t.Rows {
        options[i] = strings.Join(row, " | ")
    }
    
    // Display headers
    fmt.Println(strings.Join(t.Headers, " | "))
    fmt.Println(strings.Repeat("-", 80))
    
    prompt := NewSelectPrompt(t.Message, options)
    result, err := prompt.Ask()
    if err != nil {
        return -1, err
    }
    
    // Find selected index
    for i, opt := range options {
        if opt == result {
            return i, nil
        }
    }
    
    return -1, fmt.Errorf("selection not found")
}

// Example: API resource selection
func selectAPIResource(resources []Resource) (*Resource, error) {
    headers := []string{"Name", "Description", "Operations"}
    rows := make([][]string, len(resources))
    
    for i, res := range resources {
        rows[i] = []string{
            res.Name,
            res.Description,
            fmt.Sprintf("%d ops", len(res.Operations)),
        }
    }
    
    table := NewInteractiveTable(headers, rows)
    table.Message = "Select a resource to explore:"
    
    idx, err := table.Display()
    if err != nil {
        return nil, err
    }
    
    return &resources[idx], nil
}
```

## Theming and Styling

```go
package prompt

// Theme defines visual styling for prompts
type Theme struct {
    Icons   IconSet
    Colors  ColorScheme
    Symbols SymbolSet
}

type IconSet struct {
    Success string
    Error   string
    Warning string
    Info    string
    Bullet  string
}

type ColorScheme struct {
    Primary   string
    Secondary string
    Success   string
    Error     string
    Warning   string
    Muted     string
}

type SymbolSet struct {
    CheckboxChecked   string
    CheckboxUnchecked string
    RadioSelected     string
    RadioUnselected   string
    Arrow             string
}

func DefaultTheme() *Theme {
    return &Theme{
        Icons: IconSet{
            Success: "✓",
            Error:   "✗",
            Warning: "⚠",
            Info:    "ℹ",
            Bullet:  "•",
        },
        Colors: ColorScheme{
            Primary:   "#0052CC",
            Secondary: "#6554C0",
            Success:   "#36B37E",
            Error:     "#DE350B",
            Warning:   "#FFAB00",
            Muted:     "#97A0AF",
        },
        Symbols: SymbolSet{
            CheckboxChecked:   "☑",
            CheckboxUnchecked: "☐",
            RadioSelected:     "◉",
            RadioUnselected:   "○",
            Arrow:             "→",
        },
    }
}
```

## Error Handling

```go
package prompt

// PromptError represents a prompt-specific error
type PromptError struct {
    PromptType string
    Message    string
    UserInput  string
}

func (e *PromptError) Error() string {
    return fmt.Sprintf("%s prompt error: %s", e.PromptType, e.Message)
}

// InterruptError for Ctrl+C handling
type InterruptError struct {
    Context string
}

func (e *InterruptError) Error() string {
    return fmt.Sprintf("interrupted: %s", e.Context)
}

// ValidationError for input validation failures
type ValidationError struct {
    Field   string
    Message string
    Input   string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation error for '%s': %s (input: %s)",
        e.Field, e.Message, e.Input)
}
```

## Usage Examples

```go
// Example 1: Simple input with validation
host, err := prompt.NewInputPrompt("API Host:").
    WithDefault("https://api.example.com").
    WithValidator(validateURL).
    Ask()

// Example 2: Selection with options
method, err := prompt.NewSelectPrompt("HTTP Method:", []string{
    "GET", "POST", "PUT", "PATCH", "DELETE",
}).Ask()

// Example 3: Confirmation
confirmed, err := prompt.NewConfirmPrompt(
    "Delete this resource?",
).WithDefault(false).Ask()

// Example 4: Multi-selection
headers, err := prompt.NewMultiSelectPrompt(
    "Select headers to include:",
    []string{"Authorization", "Content-Type", "X-Request-ID"},
).Ask()

// Example 5: Password input
token, err := prompt.NewPasswordPrompt(
    "API Token:",
).Ask()

// Example 6: Complete wizard
func runSetupWizard() error {
    survey := &prompt.Survey{
        Title: "API Client Setup",
        Steps: []prompt.SurveyStep{
            {Name: "host", Prompt: hostPrompt, Required: true},
            {Name: "auth", Prompt: authPrompt, Required: true},
            {Name: "verify", Prompt: verifyPrompt, Required: true},
        },
    }
    
    answers, err := survey.Run()
    if err != nil {
        return err
    }
    
    return configureClient(answers)
}
```

---

## Bible Standard Compliance

**Genesis 1:1** - *"In the beginning God created the heavens and the earth."*

This specification follows the Bible Standard for software documentation, ensuring clarity, completeness, and divine order in technical communication. All prompts shall be documented with precision, honoring the Creator who established order from chaos.

**Compliance Checklist:**
- [x] Status clearly marked as COMPLETE
- [x] Version number specified (1.0.0)
- [x] Last updated date included
- [x] Overview provides clear context
- [x] Technical implementation with code examples
- [x] Prompt types comprehensively documented
- [x] Error handling defined
- [x] Usage examples provided
- [x] Bible Standard footer included

*"Whatever you do, work at it with all your heart, as working for the Lord."* - Colossians 3:23
