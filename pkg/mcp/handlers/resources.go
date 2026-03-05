package handlers

import (
	"context"
	"encoding/json"
	"fmt"
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
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
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

	return &ReadResourceResult{
		Contents: []ResourceContents{
			{
				URI:      req.URI,
				MimeType: res.MimeType,
				Text:     string(res.Data),
			},
		},
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
}
