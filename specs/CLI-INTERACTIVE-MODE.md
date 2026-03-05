# CLI Interactive Mode Specification

**Status**: ✅ COMPLETE  
**Date**: 2026-02-28  
**Purpose**: Interactive prompts, input validation, select menus, and confirm dialogs  
**Language**: Go 1.21+  

---

## Overview

Interactive mode provides user-friendly prompts for CLI commands, including text input, selection menus, confirmations, and multi-step wizards.

---

## Interactive Prompts

### Prompt Implementation

```go
// internal/cli/interactive/prompt.go
package interactive

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

// Prompt asks for user input
type Prompt struct {
	Message    string
	Default    string
	Required   bool
	Validate   func(string) error
	MaskInput  bool
	reader     *bufio.Reader
}

// NewPrompt creates a new prompt
func NewPrompt(message string) *Prompt {
	return &Prompt{
		Message: message,
		reader:  bufio.NewReader(os.Stdin),
	}
}

// SetDefault sets the default value
func (p *Prompt) SetDefault(defaultValue string) *Prompt {
	p.Default = defaultValue
	return p
}

// SetRequired marks the prompt as required
func (p *Prompt) SetRequired() *Prompt {
	p.Required = true
	return p
}

// SetValidator sets a validation function
func (p *Prompt) SetValidator(validate func(string) error) *Prompt {
	p.Validate = validate
	return p
}

// SetMasked masks the input (for passwords)
func (p *Prompt) SetMasked() *Prompt {
	p.MaskInput = true
	return p
}

// Run executes the prompt and returns the result
func (p *Prompt) Run() (string, error) {
	for {
		// Display prompt
		p.display()

		// Read input
		input, err := p.readInput()
		if err != nil {
			return "", err
		}

		// Use default if empty
		if input == "" && p.Default != "" {
			input = p.Default
		}

		// Check required
		if p.Required && input == "" {
			fmt.Println(color.RedString("✗"), "This field is required")
			continue
		}

		// Validate
		if p.Validate != nil {
			if err := p.Validate(input); err != nil {
				fmt.Println(color.RedString("✗"), err)
				continue
			}
		}

		return input, nil
	}
}

func (p *Prompt) display() {
	prompt := p.Message
	if p.Default != "" {
		prompt = fmt.Sprintf("%s [%s]", p.Message, p.Default)
	}
	prompt += ": "

	fmt.Print(color.CyanString("? "), prompt)
}

func (p *Prompt) readInput() (string, error) {
	if p.MaskInput {
		// For masked input, use terminal.ReadPassword
		// Simplified here - use actual implementation
		fmt.Print("\033[8m") // Hide input
	}

	input, err := p.reader.ReadString('\n')

	if p.MaskInput {
		fmt.Print("\033[28m") // Show input
		fmt.Println()
	}

	if err != nil {
		return "", err
	}

	return strings.TrimSpace(input), nil
}
```

---

## Input Validation

### Validators

```go
// internal/cli/interactive/validation.go
package interactive

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ValidateNotEmpty ensures input is not empty
func ValidateNotEmpty(input string) error {
	if strings.TrimSpace(input) == "" {
		return fmt.Errorf("input cannot be empty")
	}
	return nil
}

// ValidateEmail validates email format
func ValidateEmail(input string) error {
	pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	matched, _ := regexp.MatchString(pattern, input)
	if !matched {
		return fmt.Errorf("invalid email format")
	}
	return nil
}

// ValidateURL validates URL format
func ValidateURL(input string) error {
	u, err := url.Parse(input)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("URL must include scheme and host")
	}
	return nil
}

// ValidateIP validates IP address
func ValidateIP(input string) error {
	if net.ParseIP(input) == nil {
		return fmt.Errorf("invalid IP address")
	}
	return nil
}

// ValidateInteger validates integer input
func ValidateInteger(input string) error {
	if _, err := strconv.Atoi(input); err != nil {
		return fmt.Errorf("must be a valid integer")
	}
	return nil
}

// ValidateRange validates numeric range
func ValidateRange(min, max int) func(string) error {
	return func(input string) error {
		num, err := strconv.Atoi(input)
		if err != nil {
			return fmt.Errorf("must be a valid number")
		}
		if num < min || num > max {
			return fmt.Errorf("must be between %d and %d", min, max)
		}
		return nil
	}
}

// ValidateOneOf validates against allowed values
func ValidateOneOf(values ...string) func(string) error {
	return func(input string) error {
		for _, v := range values {
			if strings.EqualFold(input, v) {
				return nil
			}
		}
		return fmt.Errorf("must be one of: %s", strings.Join(values, ", "))
	}
}

// ValidateRegex validates against regex pattern
func ValidateRegex(pattern string, description string) func(string) error {
	re := regexp.MustCompile(pattern)
	return func(input string) error {
		if !re.MatchString(input) {
			return fmt.Errorf("must match pattern: %s", description)
		}
		return nil
	}
}

// ValidateMinLength validates minimum length
func ValidateMinLength(min int) func(string) error {
	return func(input string) error {
		if len(input) < min {
			return fmt.Errorf("must be at least %d characters", min)
		}
		return nil
	}
}

// ValidateMaxLength validates maximum length
func ValidateMaxLength(max int) func(string) error {
	return func(input string) error {
		if len(input) > max {
			return fmt.Errorf("must be at most %d characters", max)
		}
		return nil
	}
}
```

---

## Select Menus

### Selection Prompt

```go
// internal/cli/interactive/select.go
package interactive

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

// Select presents a list of options
type Select struct {
	Message string
	Options []string
	Default int
}

// NewSelect creates a new select prompt
func NewSelect(message string, options []string) *Select {
	return &Select{
		Message: message,
		Options: options,
		Default: 0,
	}
}

// SetDefault sets the default selection
func (s *Select) SetDefault(index int) *Select {
	s.Default = index
	return s
}

// Run displays the select and returns the choice
func (s *Select) Run() (int, string, error) {
	if len(s.Options) == 0 {
		return -1, "", fmt.Errorf("no options provided")
	}

	fmt.Println(color.CyanString("?"), s.Message)

	for i, opt := range s.Options {
		marker := "  "
		if i == s.Default {
			marker = color.GreenString("> ")
			opt = color.WhiteString(opt)
		}
		fmt.Printf("%s%d. %s\n", marker, i+1, opt)
	}

	fmt.Print("Enter number: ")

	var choice int
	_, err := fmt.Scanln(&choice)
	if err != nil {
		return s.Default, s.Options[s.Default], nil
	}

	if choice < 1 || choice > len(s.Options) {
		return s.Default, s.Options[s.Default], nil
	}

	return choice - 1, s.Options[choice-1], nil
}

// MultiSelect allows multiple selections
type MultiSelect struct {
	Message string
	Options []string
	Default []int
}

// NewMultiSelect creates a multi-select prompt
func NewMultiSelect(message string, options []string) *MultiSelect {
	return &MultiSelect{
		Message: message,
		Options: options,
	}
}

// Run displays the multi-select and returns selected indices
func (ms *MultiSelect) Run() ([]int, []string, error) {
	if len(ms.Options) == 0 {
		return nil, nil, fmt.Errorf("no options provided")
	}

	fmt.Println(color.CyanString("?"), ms.Message)
	fmt.Println("Enter numbers separated by commas (e.g., 1,3,5):")

	for i, opt := range ms.Options {
		fmt.Printf("  %d. %s\n", i+1, opt)
	}

	fmt.Print("> ")

	var input string
	fmt.Scanln(&input)

	if input == "" {
		return ms.Default, ms.getValues(ms.Default), nil
	}

	var selected []int
	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		num := 0
		if _, err := fmt.Sscanf(part, "%d", &num); err == nil {
			if num >= 1 && num <= len(ms.Options) {
				selected = append(selected, num-1)
			}
		}
	}

	return selected, ms.getValues(selected), nil
}

func (ms *MultiSelect) getValues(indices []int) []string {
	var values []string
	for _, idx := range indices {
		if idx >= 0 && idx < len(ms.Options) {
			values = append(values, ms.Options[idx])
		}
	}
	return values
}
```

---

## Confirm Dialogs

### Confirmation Prompt

```go
// internal/cli/interactive/confirm.go
package interactive

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

// Confirm asks for yes/no confirmation
type Confirm struct {
	Message    string
	DefaultYes bool
}

// NewConfirm creates a new confirm prompt
func NewConfirm(message string) *Confirm {
	return &Confirm{
		Message:    message,
		DefaultYes: false,
	}
}

// SetDefault sets the default value
func (c *Confirm) SetDefault(defaultYes bool) *Confirm {
	c.DefaultYes = defaultYes
	return c
}

// Run executes the confirmation prompt
func (c *Confirm) Run() (bool, error) {
	suffix := "(y/N)"
	if c.DefaultYes {
		suffix = "(Y/n)"
	}

	prompt := fmt.Sprintf("%s %s: ", c.Message, suffix)
	fmt.Print(color.CyanString("? "), prompt)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return c.DefaultYes, nil
	}

	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return c.DefaultYes, nil
	}

	return input == "y" || input == "yes", nil
}

// ConfirmOrExit confirms or exits the program
func (c *Confirm) ConfirmOrExit() error {
	confirmed, err := c.Run()
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println("Operation cancelled")
		os.Exit(0)
	}
	return nil
}
```

---

## Usage Example

```go
// Text input
name, err := interactive.NewPrompt("Enter your name").
    SetRequired().
    SetValidator(interactive.ValidateNotEmpty).
    Run()

// Email
email, err := interactive.NewPrompt("Enter email").
    SetValidator(interactive.ValidateEmail).
    Run()

// Password
password, err := interactive.NewPrompt("Enter password").
    SetMasked().
    SetRequired().
    SetValidator(interactive.ValidateMinLength(8)).
    Run()

// Selection
idx, choice, err := interactive.NewSelect(
    "Select environment",
    []string{"development", "staging", "production"},
).SetDefault(0).Run()

// Multi-select
indices, values, err := interactive.NewMultiSelect(
    "Select features",
    []string{"auth", "logging", "metrics", "tracing"},
).Run()

// Confirmation
confirmed, err := interactive.NewConfirm("Delete this item?").
    SetDefault(false).
    Run()

if confirmed {
    // Proceed with deletion
}
```
