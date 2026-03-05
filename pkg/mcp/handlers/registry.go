package handlers

import (
	"context"
	"encoding/json"
	"fmt"
)

type Handler interface {
	Method() string
	Handle(ctx context.Context, params json.RawMessage) (interface{}, error)
}

type Registry struct {
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]Handler),
	}
}

func (r *Registry) Register(handler Handler) {
	r.handlers[handler.Method()] = handler
}

func (r *Registry) Handle(ctx context.Context, method string, params json.RawMessage) (interface{}, error) {
	handler, ok := r.handlers[method]
	if !ok {
		return nil, &MCPError{Code: -32601, Message: "Method not found"}
	}
	return handler.Handle(ctx, params)
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (e *MCPError) Error() string {
	return fmt.Sprintf("MCP Error %d: %s", e.Code, e.Message)
}

func HandleError(err error) *MCPError {
	if e, ok := err.(*MCPError); ok {
		return e
	}
	return &MCPError{
		Code:    -32603,
		Message: "Internal error",
		Data:    err.Error(),
	}
}
