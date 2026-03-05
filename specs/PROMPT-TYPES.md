# Prompt Types Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: CLI

## Overview

Complete specification of **prompt types** and their behaviors.

## 1. Text Input

```go
type TextPrompt struct {
    Label       string
    Placeholder string
    Default     string
    Required    bool
    Mask        bool // For passwords
}

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
```

## 2. Number Input

```go
type NumberPrompt struct {
    Label    string
    Default  int
    Min      int
    Max      int
    Required bool
}

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
```

## 3. Multi-Select

```go
type MultiSelectPrompt struct {
    Label   string
    Options []string
    Default []int
}

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
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
