package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
)

type StoredResource struct {
	Data     []byte
	MimeType string
}

type ResourceProvider interface {
	GetResource(uri string) (*StoredResource, error)
	ListResources() ([]ResourceInfo, error)
}

type ResourceInfo struct {
	URI         string
	Name        string
	Description string
	MimeType    string
}

type ListResourcesHandler struct {
	provider ResourceProvider
}

func NewListResourcesHandler(provider ResourceProvider) *ListResourcesHandler {
	return &ListResourcesHandler{
		provider: provider,
	}
}

func (h *ListResourcesHandler) Method() string {
	return "resources/list"
}

func (h *ListResourcesHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req ListResourcesRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &MCPError{Code: -32602, Message: "Invalid params"}
	}

	resources, err := h.provider.ListResources()
	if err != nil {
		return nil, &MCPError{
			Code:    -32603,
			Message: "Failed to list resources",
			Data:    err.Error(),
		}
	}

	resourceList := make([]Resource, 0, len(resources))
	for _, res := range resources {
		resourceList = append(resourceList, Resource{
			URI:         res.URI,
			Name:        res.Name,
			Description: res.Description,
			MimeType:    res.MimeType,
		})
	}

	return &ListResourcesResult{
		Resources: resourceList,
	}, nil
}

type ListResourcesRequest struct {
	Cursor *string `json:"cursor,omitempty"`
}

type ListResourcesResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

type Resource struct {
	URI         string                 `json:"uri"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	MimeType    string                 `json:"mimeType,omitempty"`
	Size        *int64                 `json:"size,omitempty"`
	Annotations map[string]interface{} `json:"annotations,omitempty"`
}

type ReadResourceHandler struct {
	provider ResourceProvider
}

func NewReadResourceHandler(provider ResourceProvider) *ReadResourceHandler {
	return &ReadResourceHandler{
		provider: provider,
	}
}

func (h *ReadResourceHandler) Method() string {
	return "resources/read"
}

func (h *ReadResourceHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req ReadResourceRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &MCPError{Code: -32602, Message: "Invalid params"}
	}

	res, err := h.provider.GetResource(req.URI)
	if err != nil {
		return nil, &MCPError{
			Code:    -32603,
			Message: fmt.Sprintf("Resource not found: %s", req.URI),
		}
	}

	content := ResourceContents{
		URI:      req.URI,
		MimeType: res.MimeType,
	}

	// Use base64 for binary data
	if isBinaryMimeType(res.MimeType) {
		content.Blob = base64.StdEncoding.EncodeToString(res.Data)
	} else {
		content.Text = string(res.Data)
	}

	return &ReadResourceResult{
		Contents: []ResourceContents{content},
	}, nil
}

type ReadResourceRequest struct {
	URI string `json:"uri"`
}

type ReadResourceResult struct {
	Contents []ResourceContents `json:"contents"`
}

type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

type ResourceTemplate struct {
	URITemplate string                 `json:"uriTemplate"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	MimeType    string                 `json:"mimeType,omitempty"`
	Annotations map[string]interface{} `json:"annotations,omitempty"`
}

type ListTemplatesHandler struct {
	templates []ResourceTemplate
}

func NewListTemplatesHandler(templates []ResourceTemplate) *ListTemplatesHandler {
	return &ListTemplatesHandler{
		templates: templates,
	}
}

func (h *ListTemplatesHandler) Method() string {
	return "resources/templates/list"
}

func (h *ListTemplatesHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return &ListResourceTemplatesResult{
		ResourceTemplates: h.templates,
	}, nil
}

type ListResourceTemplatesResult struct {
	ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
}

type SubscribeHandler struct {
	subscriber *ResourceSubscriber
}

func NewSubscribeHandler(subscriber *ResourceSubscriber) *SubscribeHandler {
	return &SubscribeHandler{
		subscriber: subscriber,
	}
}

func (h *SubscribeHandler) Method() string {
	return "resources/subscribe"
}

func (h *SubscribeHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req SubscribeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &MCPError{Code: -32602, Message: "Invalid params"}
	}

	h.subscriber.Subscribe(req.URI, req.ID)
	return struct{}{}, nil
}

type UnsubscribeHandler struct {
	subscriber *ResourceSubscriber
}

func NewUnsubscribeHandler(subscriber *ResourceSubscriber) *UnsubscribeHandler {
	return &UnsubscribeHandler{
		subscriber: subscriber,
	}
}

func (h *UnsubscribeHandler) Method() string {
	return "resources/unsubscribe"
}

func (h *UnsubscribeHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req UnsubscribeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &MCPError{Code: -32602, Message: "Invalid params"}
	}

	h.subscriber.Unsubscribe(req.URI, req.ID)
	return struct{}{}, nil
}

type SubscribeRequest struct {
	URI string     `json:"uri"`
	ID  *RequestID `json:"id"`
}

type UnsubscribeRequest struct {
	URI string     `json:"uri"`
	ID  *RequestID `json:"id"`
}

type RequestID struct {
	Value string
}

type ResourceSubscriber struct {
	mu          sync.RWMutex
	subscribers map[string]map[*RequestID]bool
	notifyFunc  func(method string, params interface{}, id *RequestID)
}

func NewResourceSubscriber(notifyFunc func(method string, params interface{}, id *RequestID)) *ResourceSubscriber {
	return &ResourceSubscriber{
		subscribers: make(map[string]map[*RequestID]bool),
		notifyFunc:  notifyFunc,
	}
}

func (rs *ResourceSubscriber) Subscribe(uri string, id *RequestID) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.subscribers[uri] == nil {
		rs.subscribers[uri] = make(map[*RequestID]bool)
	}
	rs.subscribers[uri][id] = true
}

func (rs *ResourceSubscriber) Unsubscribe(uri string, id *RequestID) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.subscribers[uri] != nil {
		delete(rs.subscribers[uri], id)
	}
}

func (rs *ResourceSubscriber) NotifyUpdate(uri string) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	if subs, ok := rs.subscribers[uri]; ok {
		for id := range subs {
			if rs.notifyFunc != nil {
				rs.notifyFunc("notifications/resources/updated",
					ResourceUpdatedNotification{URI: uri}, id)
			}
		}
	}
}

func (rs *ResourceSubscriber) NotifyListChanged() {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	for _, subs := range rs.subscribers {
		for id := range subs {
			if rs.notifyFunc != nil {
				rs.notifyFunc("notifications/resources/listChanged",
					ResourceListChangedNotification{}, id)
			}
		}
	}
}

type ResourceUpdatedNotification struct {
	URI string `json:"uri"`
}

type ResourceListChangedNotification struct{}

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

func (rm *ResourceManager) RegisterTemplate(t ResourceTemplate) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.templates = append(rm.templates, t)
}

func (rm *ResourceManager) ListResources(ctx context.Context) ([]Resource, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	resources := make([]Resource, 0, len(rm.resources))
	for _, r := range rm.resources {
		resources = append(resources, r)
	}

	for _, p := range rm.providers {
		providerResources, err := p.ListResources()
		if err != nil {
			continue
		}
		for _, res := range providerResources {
			resources = append(resources, Resource{
				URI:         res.URI,
				Name:        res.Name,
				Description: res.Description,
				MimeType:    res.MimeType,
			})
		}
	}

	return resources, nil
}

func (rm *ResourceManager) ReadResource(ctx context.Context, uri string) (*ResourceContents, error) {
	rm.mu.RLock()
	if r, ok := rm.resources[uri]; ok {
		rm.mu.RUnlock()
		return &ResourceContents{
			URI:      r.URI,
			MimeType: r.MimeType,
		}, nil
	}
	rm.mu.RUnlock()

	for _, p := range rm.providers {
		content, err := p.GetResource(uri)
		if err == nil && content != nil {
			rc := &ResourceContents{
				URI:      uri,
				MimeType: content.MimeType,
			}
			if isBinaryMimeType(content.MimeType) {
				rc.Blob = base64.StdEncoding.EncodeToString(content.Data)
			} else {
				rc.Text = string(content.Data)
			}
			return rc, nil
		}
	}

	return nil, fmt.Errorf("resource not found: %s", uri)
}

func (rm *ResourceManager) ListTemplates() []ResourceTemplate {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.templates
}

func isBinaryMimeType(mimeType string) bool {
	binaryTypes := []string{
		"application/octet-stream",
		"application/pdf",
		"application/zip",
		"image/",
		"video/",
		"audio/",
	}
	for _, t := range binaryTypes {
		if len(mimeType) >= len(t) && mimeType[:len(t)] == t {
			return true
		}
	}
	return false
}
