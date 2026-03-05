# Interactive Prompts Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: CLI

## Overview

Complete specification of **interactive prompts** for CLI user interaction.

## 1. Prompt Interface

```go
type Prompt interface {
    Ask() (interface{}, error)
    Validate(input interface{}) error
}

type PromptOptions struct {
    Message    string
    Default    interface{}
    Help       string
    Validation func(interface{}) error
}
```

## 2. Input Prompt

```go
type InputPrompt struct {
    Options PromptOptions
}

func (p *InputPrompt) Ask() (string, error) {
    fmt.Printf("%s: ", p.Options.Message)
    
    reader := bufio.NewReader(os.Stdin)
    input, err := reader.ReadString('\n')
    if err != nil {
        return "", err
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
```

## 3. Confirm Prompt

```go
type ConfirmPrompt struct {
    Options PromptOptions
}

func (p *ConfirmPrompt) Ask() (bool, error) {
    defaultVal := "n"
    if p.Options.Default != nil && p.Options.Default.(bool) {
        defaultVal = "y"
    }
    
    fmt.Printf("%s [y/N]: ", p.Options.Message)
    
    reader := bufio.NewReader(os.Stdin)
    input, _ := reader.ReadString('\n')
    input = strings.ToLower(strings.TrimSpace(input))
    
    if input == "" {
        input = defaultVal
    }
    
    return input == "y" || input == "yes", nil
}
```

## 4. Select Prompt

```go
type SelectPrompt struct {
    Options PromptOptions
    Choices []string
}

func (p *SelectPrompt) Ask() (string, error) {
    fmt.Printf("%s:\n", p.Options.Message)
    
    for i, choice := range p.Choices {
        fmt.Printf("  %d. %s\n", i+1, choice)
    }
    
    fmt.Print("Enter number: ")
    
    reader := bufio.NewReader(os.Stdin)
    input, _ := reader.ReadString('\n')
    input = strings.TrimSpace(input)
    
    index, err := strconv.Atoi(input)
    if err != nil || index < 1 || index > len(p.Choices) {
        fmt.Println("Invalid selection")
        return p.Ask()
    }
    
    return p.Choices[index-1], nil
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
