# MCP Market Prompts Specification

MCP prompts support for reusable prompt templates with arguments.

## Prompt Structure

```go
// Prompt represents a prompt template
type Prompt struct {
    Name        string           `json:"name"`
    Description string           `json:"description,omitempty"`
    Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument represents a template parameter
type PromptArgument struct {
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    Required    bool   `json:"required,omitempty"`
}

// PromptMessage represents a message in a prompt
type PromptMessage struct {
    Role    string  `json:"role"`    // "user" or "assistant"
    Content Content `json:"content"`
}

// Prompt content types
type Content interface {
    contentType() string
}

type TextContent struct {
    Type string `json:"type"`
    Text string `json:"text"`
}

func (TextContent) contentType() string { return "text" }

type ImageContent struct {
    Type     string `json:"type"`
    Data     string `json:"data"`
    MimeType string `json:"mimeType"`
}

func (ImageContent) contentType() string { return "image" }
```

## Prompt Templates

```go
// PromptTemplate for dynamic prompts
type PromptTemplate struct {
    Name        string
    Description string
    Arguments   []PromptArgument
    Template    func(args map[string]string) ([]PromptMessage, error)
}

// Example: API Request Builder prompt
var apiRequestPrompt = PromptTemplate{
    Name:        "build_api_request",
    Description: "Build an HTTP request for a specific operation",
    Arguments: []PromptArgument{
        {Name: "module", Description: "Module name", Required: true},
        {Name: "operation", Description: "Operation ID", Required: true},
        {Name: "format", Description: "Output format", Required: false},
    },
    Template: func(args map[string]string) ([]PromptMessage, error) {
        module := args["module"]
        operation := args["operation"]
        format := args["format"]
        if format == "" {
            format = "curl"
        }
        
        messages := []PromptMessage{
            {
                Role: "user",
                Content: TextContent{
                    Type: "text",
                    Text: fmt.Sprintf("Generate a %s request for %s.%s", 
                        format, module, operation),
                },
            },
        }
        return messages, nil
    },
}
```

## Prompt Registry

```go
type PromptRegistry struct {
    mu       sync.RWMutex
    prompts  map[string]PromptTemplate
    capability *PromptsCapability
}

func NewPromptRegistry() *PromptRegistry {
    return &PromptRegistry{
        prompts: make(map[string]PromptTemplate),
        capability: &PromptsCapability{
            ListChanged: true,
        },
    }
}

func (pr *PromptRegistry) Register(template PromptTemplate) error {
    if template.Name == "" {
        return fmt.Errorf("prompt name is required")
    }
    if template.Template == nil {
        return fmt.Errorf("prompt template function is required")
    }
    
    pr.mu.Lock()
    defer pr.mu.Unlock()
    pr.prompts[template.Name] = template
    return nil
}

func (pr *PromptRegistry) Get(name string) (PromptTemplate, bool) {
    pr.mu.RLock()
    defer pr.mu.RUnlock()
    prompt, ok := pr.prompts[name]
    return prompt, ok
}

func (pr *PromptRegistry) List() []Prompt {
    pr.mu.RLock()
    defer pr.mu.RUnlock()
    
    prompts := make([]Prompt, 0, len(pr.prompts))
    for _, pt := range pr.prompts {
        prompts = append(prompts, Prompt{
            Name:        pt.Name,
            Description: pt.Description,
            Arguments:   pt.Arguments,
        })
    }
    return prompts
}
```

## Argument Handling

```go
// GetPromptRequest for retrieving a prompt
type GetPromptRequest struct {
    Name      string            `json:"name"`
    Arguments map[string]string `json:"arguments,omitempty"`
}

// GetPromptResult response
type GetPromptResult struct {
    Description string          `json:"description,omitempty"`
    Messages    []PromptMessage `json:"messages"`
}

func (pr *PromptRegistry) Execute(name string, args map[string]string) (*GetPromptResult, error) {
    template, ok := pr.Get(name)
    if !ok {
        return nil, fmt.Errorf("prompt not found: %s", name)
    }
    
    // Validate required arguments
    for _, arg := range template.Arguments {
        if arg.Required {
            if _, ok := args[arg.Name]; !ok {
                return nil, fmt.Errorf("missing required argument: %s", arg.Name)
            }
        }
    }
    
    // Execute template
    messages, err := template.Template(args)
    if err != nil {
        return nil, fmt.Errorf("template execution failed: %w", err)
    }
    
    return &GetPromptResult{
        Description: template.Description,
        Messages:    messages,
    }, nil
}
```

## Server Integration

```go
func (s *Server) handlePromptsList(req Request) {
    s.mu.RLock()
    if !s.initialized {
        s.mu.RUnlock()
        s.sendError(req.ID, InvalidRequestCode, "Server not initialized", nil)
        return
    }
    s.mu.RUnlock()
    
    prompts := s.promptRegistry.List()
    result := ListPromptsResult{Prompts: prompts}
    s.sendResult(req.ID, result)
}

func (s *Server) handlePromptsGet(req Request) {
    s.mu.RLock()
    if !s.initialized {
        s.mu.RUnlock()
        s.sendError(req.ID, InvalidRequestCode, "Server not initialized", nil)
        return
    }
    s.mu.RUnlock()
    
    var params GetPromptRequest
    if err := json.Unmarshal(req.Params, &params); err != nil {
        s.sendError(req.ID, InvalidParamsCode, "Invalid params", err.Error())
        return
    }
    
    result, err := s.promptRegistry.Execute(params.Name, params.Arguments)
    if err != nil {
        s.sendError(req.ID, MethodNotFoundCode, err.Error(), nil)
        return
    }
    
    s.sendResult(req.ID, result)
}
```
