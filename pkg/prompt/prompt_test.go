package prompt

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestInputPrompt_Create(t *testing.T) {
	prompt := NewInputPrompt("Enter name:")

	if prompt.Options.Message != "Enter name:" {
		t.Errorf("Expected message 'Enter name:', got '%s'", prompt.Options.Message)
	}
}

func TestConfirmPrompt_Create(t *testing.T) {
	prompt := NewConfirmPrompt("Continue?")

	if prompt.Options.Message != "Continue?" {
		t.Errorf("Expected message 'Continue?', got '%s'", prompt.Options.Message)
	}
}

func TestSelectPrompt_Create(t *testing.T) {
	choices := []string{"Option A", "Option B", "Option C"}
	prompt := NewSelectPrompt("Choose option:", choices)

	if len(prompt.Choices) != 3 {
		t.Errorf("Expected 3 choices, got %d", len(prompt.Choices))
	}

	if prompt.Choices[0] != "Option A" {
		t.Errorf("Expected 'Option A', got '%s'", prompt.Choices[0])
	}
}

func TestTextPrompt_Create(t *testing.T) {
	prompt := NewTextPrompt("Name")

	if prompt.Label != "Name" {
		t.Errorf("Expected label 'Name', got '%s'", prompt.Label)
	}
}

func TestNumberPrompt_Create(t *testing.T) {
	prompt := NewNumberPrompt("Age")

	if prompt.Label != "Age" {
		t.Errorf("Expected label 'Age', got '%s'", prompt.Label)
	}
}

func TestMultiSelectPrompt_Create(t *testing.T) {
	options := []string{"Option A", "Option B", "Option C"}
	prompt := NewMultiSelectPrompt("Select options:", options)

	if len(prompt.Options) != 3 {
		t.Errorf("Expected 3 options, got %d", len(prompt.Options))
	}

	if prompt.Options[0] != "Option A" {
		t.Errorf("Expected 'Option A', got '%s'", prompt.Options[0])
	}
}

func TestInputPrompt_Validate(t *testing.T) {
	t.Run("with validation", func(t *testing.T) {
		prompt := NewInputPrompt("Enter name:")
		prompt.Options.Validation = func(input interface{}) error {
			if str, ok := input.(string); ok && str == "" {
				return fmt.Errorf("name cannot be empty")
			}
			return nil
		}

		err := prompt.Validate("")
		if err == nil {
			t.Errorf("Expected error for empty string with validation")
		}

		err = prompt.Validate("John")
		if err != nil {
			t.Errorf("Expected no error for valid string with validation")
		}
	})

	t.Run("without validation", func(t *testing.T) {
		prompt := NewInputPrompt("Enter name:")
		err := prompt.Validate("")
		if err != nil {
			t.Errorf("Expected no error when no validation set")
		}
	})
}

func TestConfirmPrompt_Validate(t *testing.T) {
	prompt := NewConfirmPrompt("Continue?")

	err := prompt.Validate(true)
	if err != nil {
		t.Errorf("Expected no error for valid boolean input")
	}

	err = prompt.Validate("not a bool")
	if err == nil {
		t.Errorf("Expected error for non-boolean input")
	}
}

func TestSelectPrompt_Validate(t *testing.T) {
	choices := []string{"Option A", "Option B", "Option C"}
	prompt := NewSelectPrompt("Choose option:", choices)

	err := prompt.Validate("Option A")
	if err != nil {
		t.Errorf("Expected no error for valid choice")
	}

	err = prompt.Validate("Invalid Option")
	if err == nil {
		t.Errorf("Expected error for invalid choice")
	}
}

func TestInputPrompt_AskValid(t *testing.T) {
	r, w, _ := os.Pipe()
	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("test input\n")
		_ = w.Close()
	}()

	prompt := NewInputPrompt("Enter name:")
	result, err := prompt.Ask()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "test input" {
		t.Errorf("Expected 'test input', got '%s'", result)
	}
}

func TestInputPrompt_AskWithDefault(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("\n")
		_ = w.Close()
	}()

	prompt := NewInputPrompt("Enter name:")
	prompt.Options.Default = "default value"
	result, err := prompt.Ask()

	if serr, ok := err.(*os.PathError); ok && strings.Contains(serr.Error(), "bad file descriptor") {
		t.Skip("Skipping test due to stdin redirection limitations")
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "default value" {
		t.Errorf("Expected 'default value', got '%s'", result)
	}
}

func TestConfirmPrompt_AskYes(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("y\n")
		_ = w.Close()
	}()

	prompt := NewConfirmPrompt("Continue?")
	result, err := prompt.Ask()

	if serr, ok := err.(*os.PathError); ok && strings.Contains(serr.Error(), "bad file descriptor") {
		t.Skip("Skipping test due to stdin redirection limitations")
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result {
		t.Errorf("Expected true, got false")
	}
}

func TestConfirmPrompt_AskNo(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("n\n")
		_ = w.Close()
	}()

	prompt := NewConfirmPrompt("Continue?")
	result, err := prompt.Ask()

	if serr, ok := err.(*os.PathError); ok && strings.Contains(serr.Error(), "bad file descriptor") {
		t.Skip("Skipping test due to stdin redirection limitations")
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result {
		t.Errorf("Expected false, got true")
	}
}

func TestConfirmPrompt_AskYesFull(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("yes\n")
		_ = w.Close()
	}()

	prompt := NewConfirmPrompt("Continue?")
	result, err := prompt.Ask()

	if serr, ok := err.(*os.PathError); ok && strings.Contains(serr.Error(), "bad file descriptor") {
		t.Skip("Skipping test due to stdin redirection limitations")
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !result {
		t.Errorf("Expected true, got false")
	}
}

func TestSelectPrompt_AskValid(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("2\n")
		_ = w.Close()
	}()

	choices := []string{"Option A", "Option B", "Option C"}
	prompt := NewSelectPrompt("Choose option:", choices)
	result, err := prompt.Ask()

	if serr, ok := err.(*os.PathError); ok && strings.Contains(serr.Error(), "bad file descriptor") {
		t.Skip("Skipping test due to stdin redirection limitations")
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "Option B" {
		t.Errorf("Expected 'Option B', got '%s'", result)
	}
}

func TestTextPrompt_AskValidInput(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("test value\n")
		_ = w.Close()
	}()

	prompt := NewTextPrompt("Name")
	result, err := prompt.Ask()

	if serr, ok := err.(*os.PathError); ok && strings.Contains(serr.Error(), "bad file descriptor") {
		t.Skip("Skipping test due to stdin redirection limitations")
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "test value" {
		t.Errorf("Expected 'test value', got '%s'", result)
	}
}

func TestNumberPrompt_AskValid(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("42\n")
		_ = w.Close()
	}()

	prompt := NewNumberPrompt("Age")
	result, err := prompt.Ask()

	if serr, ok := err.(*os.PathError); ok && strings.Contains(serr.Error(), "bad file descriptor") {
		t.Skip("Skipping test due to stdin redirection limitations")
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("Expected 42, got %d", result)
	}
}

func TestMultiSelectPrompt_AskValid(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("1, 3\n")
		_ = w.Close()
	}()

	options := []string{"Option A", "Option B", "Option C"}
	prompt := NewMultiSelectPrompt("Select options:", options)
	result, err := prompt.Ask()

	if serr, ok := err.(*os.PathError); ok && strings.Contains(serr.Error(), "bad file descriptor") {
		t.Skip("Skipping test due to stdin redirection limitations")
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if len(result) != 2 || result[0] != 0 || result[1] != 2 {
		t.Errorf("Expected [0, 2], got %v", result)
	}
}

func TestTextPrompt_AskWithDefault(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("\n")
		_ = w.Close()
	}()

	prompt := NewTextPrompt("Name")
	prompt.Default = "default value"
	result, err := prompt.Ask()

	if serr, ok := err.(*os.PathError); ok && strings.Contains(serr.Error(), "bad file descriptor") {
		t.Skip("Skipping test due to stdin redirection limitations")
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != "default value" {
		t.Errorf("Expected 'default value', got '%s'", result)
	}
}

func TestNumberPrompt_AskWithDefault(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()

	go func() {
		_, _ = w.WriteString("\n")
		_ = w.Close()
	}()

	prompt := NewNumberPrompt("Age")
	prompt.Default = 10
	result, err := prompt.Ask()

	if serr, ok := err.(*os.PathError); ok && strings.Contains(serr.Error(), "bad file descriptor") {
		t.Skip("Skipping test due to stdin redirection limitations")
		return
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if result != 10 {
		t.Errorf("Expected 10 (default), got %d", result)
	}
}
