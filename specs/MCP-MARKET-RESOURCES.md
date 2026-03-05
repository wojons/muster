# MCP Market Resources Specification

MCP resources support for exposing data and documents to LLMs.

## Resource Types

```go
// Resource represents addressable content
type Resource struct {
    URI         string                 `json:"uri"`
    Name        string                 `json:"name"`
    Description string                 `json:"description,omitempty"`
    MimeType    string                 `json:"mimeType,omitempty"`
    Size        *int64                 `json:"size,omitempty"`
    Annotations map[string]interface{} `json:"annotations,omitempty"`
}

// ResourceContent represents resource data
type ResourceContent struct {
    URI      string `json:"uri"`
    MimeType string `json:"mimeType,omitempty"`
    Text     string `json:"text,omitempty"`
    Blob     string `json:"blob,omitempty"` // base64
}
```

## Resource Templates

```go
// ResourceTemplate for dynamic resources
type ResourceTemplate struct {
    URITemplate string                 `json:"uriTemplate"`
    Name        string                 `json:"name"`
    Description string                 `json:"description,omitempty"`
    MimeType    string                 `json:"mimeType,omitempty"`
    Annotations map[string]interface{} `json:"annotations,omitempty"`
}

// ListResourceTemplatesResult response
type ListResourceTemplatesResult struct {
    ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
}

// Example: Inventory item template
var itemTemplate = ResourceTemplate{
    URITemplate: "market://inventory/{module}/{operation}",
    Name:        "Inventory Operation",
    Description: "Details for a specific inventory operation",
    MimeType:    "application/json",
}
```

## Resource Reading

```go
// ReadResourceRequest params
type ReadResourceRequest struct {
    URI string `json:"uri"`
}

// ReadResourceResult response
type ReadResourceResult struct {
    Contents []ResourceContent `json:"contents"`
}

// ResourceProvider interface
type ResourceProvider interface {
    ListResources(ctx context.Context) ([]Resource, error)
    ReadResource(ctx context.Context, uri string) (*ResourceContent, error)
    ListTemplates(ctx context.Context) ([]ResourceTemplate, error)
}
```

## Resource Manager

```go
type ResourceManager struct {
    mu        sync.RWMutex
    resources map[string]Resource
    providers []ResourceProvider
    templates []ResourceTemplate
}

func NewResourceManager() *ResourceManager {
    return &ResourceManager{
        resources: make(map[string]Resource),
        providers: []ResourceProvider{},
        templates: []ResourceTemplate{},
    }
}

func (rm *ResourceManager) RegisterResource(r Resource) {
    rm.mu.Lock()
    defer rm.mu.Unlock()
    rm.resources[r.URI] = r
}

func (rm *ResourceManager) RegisterProvider(p ResourceProvider) {
    rm.mu.Lock()
    defer rm.mu.Unlock()
    rm.providers = append(rm.providers, p)
}

func (rm *ResourceManager) ListResources(ctx context.Context) ([]Resource, error) {
    rm.mu.RLock()
    defer rm.mu.RUnlock()
    
    resources := make([]Resource, 0, len(rm.resources))
    for _, r := range rm.resources {
        resources = append(resources, r)
    }
    
    for _, p := range rm.providers {
        providerResources, err := p.ListResources(ctx)
        if err != nil {
            continue
        }
        resources = append(resources, providerResources...)
    }
    
    return resources, nil
}

func (rm *ResourceManager) ReadResource(ctx context.Context, uri string) (*ResourceContent, error) {
    // Check static resources
    rm.mu.RLock()
    if r, ok := rm.resources[uri]; ok {
        rm.mu.RUnlock()
        return rm.loadStaticResource(ctx, r)
    }
    rm.mu.RUnlock()
    
    // Check providers
    for _, p := range rm.providers {
        content, err := p.ReadResource(ctx, uri)
        if err == nil && content != nil {
            return content, nil
        }
    }
    
    return nil, fmt.Errorf("resource not found: %s", uri)
}
```

## Resource Updates

```go
// SubscribeRequest for resource changes
type SubscribeRequest struct {
    URI string `json:"uri"`
}

// UnsubscribeRequest for resource changes
type UnsubscribeRequest struct {
    URI string `json:"uri"`
}

// ResourceUpdatedNotification sent when resource changes
type ResourceUpdatedNotification struct {
    URI string `json:"uri"`
}

// ResourceListChangedNotification
type ResourceListChangedNotification struct{}

type ResourceSubscriber struct {
    mu          sync.RWMutex
    subscribers map[string]map[*RequestID]bool
}

func (rs *ResourceSubscriber) Subscribe(uri string, id *RequestID) {
    rs.mu.Lock()
    defer rs.mu.Unlock()
    if rs.subscribers[uri] == nil {
        rs.subscribers[uri] = make(map[*RequestID]bool)
    }
    rs.subscribers[uri][id] = true
}

func (rs *ResourceSubscriber) NotifyUpdate(uri string) {
    rs.mu.RLock()
    defer rs.mu.RUnlock()
    
    if subs, ok := rs.subscribers[uri]; ok {
        for id := range subs {
            sendNotification("notifications/resources/updated", 
                ResourceUpdatedNotification{URI: uri}, id)
        }
    }
}
```

## Server Integration

```go
func (s *Server) handleResourcesList(req Request) {
    resources, err := s.resourceManager.ListResources(s.ctx)
    if err != nil {
        s.sendError(req.ID, InternalErrorCode, err.Error(), nil)
        return
    }
    
    result := ListResourcesResult{Resources: resources}
    s.sendResult(req.ID, result)
}

func (s *Server) handleResourcesRead(req Request) {
    var params ReadResourceRequest
    if err := json.Unmarshal(req.Params, &params); err != nil {
        s.sendError(req.ID, InvalidParamsCode, "Invalid params", nil)
        return
    }
    
    content, err := s.resourceManager.ReadResource(s.ctx, params.URI)
    if err != nil {
        s.sendError(req.ID, ResourceNotFoundCode, err.Error(), nil)
        return
    }
    
    result := ReadResourceResult{Contents: []ResourceContent{*content}}
    s.sendResult(req.ID, result)
}

func (s *Server) handleResourcesSubscribe(req Request) {
    var params SubscribeRequest
    if err := json.Unmarshal(req.Params, &params); err != nil {
        s.sendError(req.ID, InvalidParamsCode, "Invalid params", nil)
        return
    }
    
    s.resourceSubscriber.Subscribe(params.URI, req.ID)
    s.sendResult(req.ID, struct{}{})
}
```
