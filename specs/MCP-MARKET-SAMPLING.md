# MCP Market Sampling Specification

MCP sampling support for requesting LLM completions from the client.

## Overview

Sampling allows the server to request LLM completions from the client. This enables server-initiated completions while the client controls the model and parameters.

## Request Types

```go
// CreateMessageRequest for sampling
type CreateMessageRequest struct {
    Messages         []SamplingMessage  `json:"messages"`
    ModelPreferences *ModelPreferences  `json:"modelPreferences,omitempty"`
    SystemPrompt     string             `json:"systemPrompt,omitempty"`
    IncludeContext   string             `json:"includeContext,omitempty"` // "none", "thisServer", "allServers"
    Temperature      *float64           `json:"temperature,omitempty"`
    MaxTokens        int                `json:"maxTokens,omitempty"`
    StopSequences    []string           `json:"stopSequences,omitempty"`
    Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// SamplingMessage represents a message in sampling
type SamplingMessage struct {
    Role    string  `json:"role"` // "user" or "assistant"
    Content Content `json:"content"`
}

// ModelPreferences for model selection
type ModelPreferences struct {
    Hints                []ModelHint `json:"hints,omitempty"`
    CostPriority         *float64    `json:"costPriority,omitempty"`
    SpeedPriority        *float64    `json:"speedPriority,omitempty"`
    IntelligencePriority *float64    `json:"intelligencePriority,omitempty"`
}

// ModelHint for preferred model
type ModelHint struct {
    Name string `json:"name,omitempty"`
}
```

## Response Types

```go
// CreateMessageResult response
type CreateMessageResult struct {
    Model     string          `json:"model"`
    StopReason string         `json:"stopReason,omitempty"` // "endTurn", "stopSequence", "maxTokens"
    Role      string          `json:"role"`
    Content   Content         `json:"content"`
}

// SamplingCapability indicates client supports sampling
type SamplingCapability struct{}
```

## Sampling Request Builder

```go
type SamplingRequestBuilder struct {
    request CreateMessageRequest
}

func NewSamplingRequest() *SamplingRequestBuilder {
    return &SamplingRequestBuilder{
        request: CreateMessageRequest{
            Messages: make([]SamplingMessage, 0),
        },
    }
}

func (b *SamplingRequestBuilder) AddUserMessage(content string) *SamplingRequestBuilder {
    b.request.Messages = append(b.request.Messages, SamplingMessage{
        Role:    "user",
        Content: TextContent{Type: "text", Text: content},
    })
    return b
}

func (b *SamplingRequestBuilder) AddAssistantMessage(content string) *SamplingRequestBuilder {
    b.request.Messages = append(b.request.Messages, SamplingMessage{
        Role:    "assistant",
        Content: TextContent{Type: "text", Text: content},
    })
    return b
}

func (b *SamplingRequestBuilder) WithSystemPrompt(prompt string) *SamplingRequestBuilder {
    b.request.SystemPrompt = prompt
    return b
}

func (b *SamplingRequestBuilder) WithMaxTokens(tokens int) *SamplingRequestBuilder {
    b.request.MaxTokens = tokens
    return b
}

func (b *SamplingRequestBuilder) WithTemperature(temp float64) *SamplingRequestBuilder {
    b.request.Temperature = &temp
    return b
}

func (b *SamplingRequestBuilder) WithModelHint(name string) *SamplingRequestBuilder {
    if b.request.ModelPreferences == nil {
        b.request.ModelPreferences = &ModelPreferences{}
    }
    b.request.ModelPreferences.Hints = append(
        b.request.ModelPreferences.Hints, 
        ModelHint{Name: name},
    )
    return b
}

func (b *SamplingRequestBuilder) Build() CreateMessageRequest {
    return b.request
}
```

## Sampling Client

```go
type SamplingClient struct {
    transport Transport
    mu        sync.Mutex
    pending   map[RequestID]chan *CreateMessageResult
}

func NewSamplingClient(transport Transport) *SamplingClient {
    return &SamplingClient{
        transport: transport,
        pending:   make(map[RequestID]chan *CreateMessageResult),
    }
}

func (sc *SamplingClient) RequestSampling(ctx context.Context, 
    req CreateMessageRequest) (*CreateMessageResult, error) {
    
    id := generateRequestID()
    responseCh := make(chan *CreateMessageResult, 1)
    
    sc.mu.Lock()
    sc.pending[id] = responseCh
    sc.mu.Unlock()
    
    defer func() {
        sc.mu.Lock()
        delete(sc.pending, id)
        sc.mu.Unlock()
    }()
    
    request := Request{
        JSONRPC: "2.0",
        ID:      &id,
        Method:  "sampling/createMessage",
        Params:  mustMarshal(req),
    }
    
    if err := sc.transport.Send(request); err != nil {
        return nil, fmt.Errorf("failed to send sampling request: %w", err)
    }
    
    select {
    case result := <-responseCh:
        if result == nil {
            return nil, fmt.Errorf("sampling request cancelled")
        }
        return result, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

func (sc *SamplingClient) HandleResponse(resp Response) {
    if resp.ID == nil {
        return
    }
    
    sc.mu.Lock()
    ch, ok := sc.pending[*resp.ID]
    sc.mu.Unlock()
    
    if !ok {
        return
    }
    
    if resp.Error != nil {
        ch <- nil
        return
    }
    
    var result CreateMessageResult
    if err := json.Unmarshal(resp.Result, &result); err != nil {
        ch <- nil
        return
    }
    
    ch <- &result
}
```

## Complete Implementation

```go
// Request sampling from client
func (s *Server) requestCompletion(ctx context.Context, 
    builder *SamplingRequestBuilder) (*CreateMessageResult, error) {
    
    s.mu.RLock()
    caps := s.clientCapabilities
    s.mu.RUnlock()
    
    if caps == nil || caps.Sampling == nil {
        return nil, fmt.Errorf("client does not support sampling")
    }
    
    req := builder.Build()
    return s.samplingClient.RequestSampling(ctx, req)
}

// Example usage
func (s *Server) generateDocumentation(ctx context.Context, 
    operationID string) (string, error) {
    
    builder := NewSamplingRequest().
        WithSystemPrompt("You are an API documentation expert.").
        AddUserMessage(fmt.Sprintf("Generate documentation for operation: %s", operationID)).
        WithMaxTokens(1000).
        WithTemperature(0.7)
    
    result, err := s.requestCompletion(ctx, builder)
    if err != nil {
        return "", err
    }
    
    if textContent, ok := result.Content.(TextContent); ok {
        return textContent.Text, nil
    }
    
    return "", fmt.Errorf("unexpected content type")
}
```
