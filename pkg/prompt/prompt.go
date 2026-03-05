package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Prompt defines the interface for all prompt types
type Prompt interface {
	Ask() (interface{}, error)
	Validate(input interface{}) error
}

// PromptOptions contains common options for prompts
type PromptOptions struct {
	Message    string
	Default    interface{}
	Help       string
	Validation func(interface{}) error
}

// InputPrompt captures text input from users
type InputPrompt struct {
	Options PromptOptions
}

// NewInputPrompt creates a new input prompt
func NewInputPrompt(message string) *InputPrompt {
	return &InputPrompt{
		Options: PromptOptions{
			Message: message,
		},
	}
}

// Ask prompts the user for text input
func (p *InputPrompt) Ask() (string, error) {
	fmt.Printf("%s: ", p.Options.Message)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}

	input = strings.TrimSpace(input)

	if input == "" && p.Options.Default != nil {
		input = p.Options.Default.(string)
	}

	if p.Options.Validation != nil {
		if err := p.Options.Validation(input); err != nil {
			fmt.Printf("Error: %v\n", err)
			return p.Ask()
		}
	}

	return input, nil
}

// Validate checks if the input is valid
func (p *InputPrompt) Validate(input interface{}) error {
	if p.Options.Validation != nil {
		return p.Options.Validation(input)
	}
	return nil
}

// ConfirmPrompt captures yes/no decisions
type ConfirmPrompt struct {
	Options PromptOptions
}

// NewConfirmPrompt creates a new confirm prompt
func NewConfirmPrompt(message string) *ConfirmPrompt {
	return &ConfirmPrompt{
		Options: PromptOptions{
			Message: message,
		},
	}
}

// Ask prompts the user for a yes/no response
func (p *ConfirmPrompt) Ask() (bool, error) {
	defaultVal := "n"
	if p.Options.Default != nil && p.Options.Default.(bool) {
		defaultVal = "y"
	}

	fmt.Printf("%s [y/N]: ", p.Options.Message)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read input: %w", err)
	}
	input = strings.ToLower(strings.TrimSpace(input))

	if input == "" {
		input = defaultVal
	}

	return input == "y" || input == "yes", nil
}

// Validate checks if the input is valid
func (p *ConfirmPrompt) Validate(input interface{}) error {
	if _, ok := input.(bool); !ok {
		return fmt.Errorf("expected boolean, got %T", input)
	}
	return nil
}

// SelectPrompt allows users to choose from a list of options
type SelectPrompt struct {
	Options PromptOptions
	Choices []string
}

// NewSelectPrompt creates a new select prompt
func NewSelectPrompt(message string, choices []string) *SelectPrompt {
	return &SelectPrompt{
		Options: PromptOptions{
			Message: message,
		},
		Choices: choices,
	}
}

// Ask prompts the user to select from a list
func (p *SelectPrompt) Ask() (string, error) {
	fmt.Printf("%s:\n", p.Options.Message)

	for i, choice := range p.Choices {
		fmt.Printf("  %d. %s\n", i+1, choice)
	}

	fmt.Print("Enter number: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	input = strings.TrimSpace(input)

	index, err := strconv.Atoi(input)
	if err != nil || index < 1 || index > len(p.Choices) {
		fmt.Println("Invalid selection")
		return p.Ask()
	}

	return p.Choices[index-1], nil
}

// Validate checks if the input is valid
func (p *SelectPrompt) Validate(input interface{}) error {
	str, ok := input.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", input)
	}
	for _, choice := range p.Choices {
		if choice == str {
			return nil
		}
	}
	return fmt.Errorf("invalid choice: %s", str)
}

// TextPrompt captures text input with advanced options
type TextPrompt struct {
	Label       string
	Placeholder string
	Default     string
	Required    bool
	Mask        bool
}

// NewTextPrompt creates a new text prompt
func NewTextPrompt(label string) *TextPrompt {
	return &TextPrompt{
		Label: label,
	}
}

// Ask prompts the user for text input
func (p *TextPrompt) Ask() (string, error) {
	prompt := p.Label
	if p.Default != "" {
		prompt += fmt.Sprintf(" [%s]", p.Default)
	}
	prompt += ": "

	fmt.Print(prompt)

	var input string
	if p.Mask {
		input = readPassword()
	} else {
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		input = strings.TrimSpace(line)
	}

	if input == "" {
		input = p.Default
	}

	if p.Required && input == "" {
		fmt.Println("This field is required")
		return p.Ask()
	}

	return input, nil
}

// readPassword reads masked input (simple implementation)
func readPassword() string {
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// NumberPrompt captures numeric input
type NumberPrompt struct {
	Label    string
	Default  int
	Min      int
	Max      int
	Required bool
}

// NewNumberPrompt creates a new number prompt
func NewNumberPrompt(label string) *NumberPrompt {
	return &NumberPrompt{
		Label: label,
	}
}

// Ask prompts the user for numeric input
func (p *NumberPrompt) Ask() (int, error) {
	prompt := fmt.Sprintf("%s [%d]: ", p.Label, p.Default)
	fmt.Print(prompt)

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return p.Default, nil
	}

	num, err := strconv.Atoi(input)
	if err != nil {
		fmt.Println("Please enter a valid number")
		return p.Ask()
	}

	if p.Min != 0 && num < p.Min {
		fmt.Printf("Value must be at least %d\n", p.Min)
		return p.Ask()
	}

	if p.Max != 0 && num > p.Max {
		fmt.Printf("Value must be at most %d\n", p.Max)
		return p.Ask()
	}

	return num, nil
}

// MultiSelectPrompt allows multiple selections from a list
type MultiSelectPrompt struct {
	Label   string
	Options []string
	Default []int
}

// NewMultiSelectPrompt creates a new multi-select prompt
func NewMultiSelectPrompt(label string, options []string) *MultiSelectPrompt {
	return &MultiSelectPrompt{
		Label:   label,
		Options: options,
	}
}

// Ask prompts the user to select multiple items
func (p *MultiSelectPrompt) Ask() ([]int, error) {
	fmt.Printf("%s (comma-separated numbers):\n", p.Label)

	for i, opt := range p.Options {
		fmt.Printf("  %d. %s\n", i+1, opt)
	}

	fmt.Print("Select: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return p.Default, nil
	}

	parts := strings.Split(input, ",")
	selections := make([]int, 0, len(parts))

	for _, part := range parts {
		num, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || num < 1 || num > len(p.Options) {
			fmt.Println("Invalid selection")
			return p.Ask()
		}
		selections = append(selections, num-1)
	}

	return selections, nil
}
