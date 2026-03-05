package handlers

import (
	"context"
	"encoding/json"
)

type InitializeHandler struct {
	serverName    string
	serverVersion string
}

func NewInitializeHandler(serverName, serverVersion string) *InitializeHandler {
	return &InitializeHandler{
		serverName:    serverName,
		serverVersion: serverVersion,
	}
}

func (h *InitializeHandler) Method() string {
	return "initialize"
}

func (h *InitializeHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var req InitializeRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &MCPError{Code: -32602, Message: "Invalid params"}
	}

	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: InitializeCapabilities{
			Tools:     &ToolsCapability{},
			Resources: &ResourcesCapability{},
			Prompts:   &PromptsCapability{},
		},
		ServerInfo: ServerInfo{
			Name:    h.serverName,
			Version: h.serverVersion,
		},
	}

	return result, nil
}

type InitializeRequest struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      Implementation     `json:"clientInfo"`
}

type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    InitializeCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
}

type InitializeCapabilities struct {
	Tools     *ToolsCapability     `json:"tools,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Prompts   *PromptsCapability   `json:"prompts,omitempty"`
}

type ClientCapabilities struct {
	Roots *RootsCapability `json:"roots,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ToolsCapability struct{}

type ResourcesCapability struct{}

type PromptsCapability struct{}

type RootsCapability struct{}

type InitializedHandler struct{}

func NewInitializedHandler() *InitializedHandler {
	return &InitializedHandler{}
}

func (h *InitializedHandler) Method() string {
	return "notifications/initialized"
}

func (h *InitializedHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return nil, nil
}
