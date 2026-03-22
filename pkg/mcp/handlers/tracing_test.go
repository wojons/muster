package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/openapi-cli/openapi-cli/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

// tracedMockHandler is a test handler that can succeed or fail (renamed to avoid conflict)
type tracedMockHandler struct {
	method string
	result interface{}
	err    error
}

func (h *tracedMockHandler) Method() string {
	return h.method
}

func (h *tracedMockHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	return h.result, h.err
}

// mockTracerProvider creates a tracer provider for testing
func mockTracerProvider(t *testing.T) trace.Tracer {
	// Use the Noop tracer provider for testing
	return trace.NewNoopTracerProvider().Tracer("test")
}

func TestNewTracedHandler(t *testing.T) {
	tracer := telemetry.NewTracer(mockTracerProvider(t))
	handler := &tracedMockHandler{method: "test/method"}

	traced := NewTracedHandler(handler, tracer)

	require.NotNil(t, traced)
	assert.Equal(t, "test/method", traced.Method())
}

func TestTracedHandler_Handle_Success(t *testing.T) {
	tracer := telemetry.NewTracer(mockTracerProvider(t))
	expectedResult := &ListToolsResult{Tools: []Tool{{Name: "test"}}}
	handler := &tracedMockHandler{
		method: "tools/list",
		result: expectedResult,
		err:    nil,
	}

	traced := NewTracedHandler(handler, tracer)
	result, err := traced.Handle(context.Background(), json.RawMessage(`{"cursor":"abc"}`))

	require.NoError(t, err)
	assert.Equal(t, expectedResult, result)
}

func TestTracedHandler_Handle_Error(t *testing.T) {
	tracer := telemetry.NewTracer(mockTracerProvider(t))
	expectedErr := &MCPError{Code: -32602, Message: "Invalid params"}
	handler := &tracedMockHandler{
		method: "tools/call",
		result: nil,
		err:    expectedErr,
	}

	traced := NewTracedHandler(handler, tracer)
	result, err := traced.Handle(context.Background(), json.RawMessage(`{}`))

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedErr, err)
}

func TestTracedHandler_Handle_GenericError(t *testing.T) {
	tracer := telemetry.NewTracer(mockTracerProvider(t))
	expectedErr := errors.New("generic error")
	handler := &tracedMockHandler{
		method: "resources/read",
		result: nil,
		err:    expectedErr,
	}

	traced := NewTracedHandler(handler, tracer)
	result, err := traced.Handle(context.Background(), json.RawMessage(`{}`))

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, expectedErr, err)
}

func TestTracedHandler_Handle_DifferentResultTypes(t *testing.T) {
	tracer := telemetry.NewTracer(mockTracerProvider(t))

	tests := []struct {
		name   string
		method string
		result interface{}
	}{
		{
			name:   "ListToolsResult",
			method: "tools/list",
			result: &ListToolsResult{Tools: []Tool{{Name: "test"}}},
		},
		{
			name:   "CallToolResult",
			method: "tools/call",
			result: &CallToolResult{Content: []ToolContent{{Type: "text", Text: "result"}}},
		},
		{
			name:   "ListPromptsResult",
			method: "prompts/list",
			result: &ListPromptsResult{Prompts: []Prompt{{Name: "test"}}},
		},
		{
			name:   "GetPromptResult",
			method: "prompts/get",
			result: &GetPromptResult{Messages: []PromptMessage{{Role: "user"}}},
		},
		{
			name:   "ListResourcesResult",
			method: "resources/list",
			result: &ListResourcesResult{Resources: []Resource{{URI: "test"}}},
		},
		{
			name:   "ReadResourceResult",
			method: "resources/read",
			result: &ReadResourceResult{Contents: []ResourceContents{{URI: "test"}}},
		},
		{
			name:   "InitializeResult",
			method: "initialize",
			result: &InitializeResult{ProtocolVersion: "2024-11-05"},
		},
		{
			name:   "ListRootsResult",
			method: "roots/list",
			result: &ListRootsResult{Roots: []Root{{URI: "test"}}},
		},
		{
			name:   "unknown type",
			method: "unknown/method",
			result: map[string]string{"key": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &tracedMockHandler{
				method: tt.method,
				result: tt.result,
				err:    nil,
			}

			traced := NewTracedHandler(handler, tracer)
			result, err := traced.Handle(context.Background(), json.RawMessage(`{}`))

			require.NoError(t, err)
			assert.Equal(t, tt.result, result)
		})
	}
}

func TestTracedHandler_Handle_WithEmptyParams(t *testing.T) {
	tracer := telemetry.NewTracer(mockTracerProvider(t))
	handler := &tracedMockHandler{
		method: "tools/list",
		result: &ListToolsResult{},
		err:    nil,
	}

	traced := NewTracedHandler(handler, tracer)
	result, err := traced.Handle(context.Background(), json.RawMessage{})

	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNewTracingRegistry(t *testing.T) {
	baseRegistry := NewRegistry()
	tracer := telemetry.NewTracer(mockTracerProvider(t))

	tracingRegistry := NewTracingRegistry(baseRegistry, tracer)

	require.NotNil(t, tracingRegistry)
	assert.NotNil(t, tracingRegistry.Registry)
	assert.NotNil(t, tracingRegistry.tracer)
}

func TestTracingRegistry_Register(t *testing.T) {
	baseRegistry := NewRegistry()
	tracer := telemetry.NewTracer(mockTracerProvider(t))
	tracingRegistry := NewTracingRegistry(baseRegistry, tracer)

	handler := &tracedMockHandler{method: "test/method", result: "success"}
	tracingRegistry.Register(handler)

	// Verify the handler is wrapped and registered
	result, err := tracingRegistry.Handle(context.Background(), "test/method", json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "success", result)
}

func TestTracingRegistry_MultipleHandlers(t *testing.T) {
	baseRegistry := NewRegistry()
	tracer := telemetry.NewTracer(mockTracerProvider(t))
	tracingRegistry := NewTracingRegistry(baseRegistry, tracer)

	// Register multiple handlers
	handler1 := &tracedMockHandler{method: "tools/list", result: &ListToolsResult{}}
	handler2 := &tracedMockHandler{method: "tools/call", result: &CallToolResult{}}
	handler3 := &tracedMockHandler{method: "resources/list", result: &ListResourcesResult{}}

	tracingRegistry.Register(handler1)
	tracingRegistry.Register(handler2)
	tracingRegistry.Register(handler3)

	// Verify all handlers work
	result1, err1 := tracingRegistry.Handle(context.Background(), "tools/list", json.RawMessage(`{}`))
	result2, err2 := tracingRegistry.Handle(context.Background(), "tools/call", json.RawMessage(`{}`))
	result3, err3 := tracingRegistry.Handle(context.Background(), "resources/list", json.RawMessage(`{}`))

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NoError(t, err3)
	assert.IsType(t, &ListToolsResult{}, result1)
	assert.IsType(t, &CallToolResult{}, result2)
	assert.IsType(t, &ListResourcesResult{}, result3)
}

func TestTracingRegistry_UnknownMethod(t *testing.T) {
	baseRegistry := NewRegistry()
	tracer := telemetry.NewTracer(mockTracerProvider(t))
	tracingRegistry := NewTracingRegistry(baseRegistry, tracer)

	result, err := tracingRegistry.Handle(context.Background(), "unknown/method", json.RawMessage(`{}`))

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "Method not found")
}

func TestDefaultTracingOptions(t *testing.T) {
	opts := DefaultTracingOptions()

	assert.False(t, opts.RecordParams)
	assert.Equal(t, 1024, opts.MaxParamSize)
}

func TestTracedHandler_ConcurrentAccess(t *testing.T) {
	tracer := telemetry.NewTracer(mockTracerProvider(t))
	handler := &tracedMockHandler{
		method: "test/method",
		result: "success",
		err:    nil,
	}

	traced := NewTracedHandler(handler, tracer)

	// Run concurrent requests
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("concurrent-%d", i), func(t *testing.T) {
			t.Parallel()
			result, err := traced.Handle(context.Background(), json.RawMessage(`{}`))
			require.NoError(t, err)
			assert.Equal(t, "success", result)
		})
	}
}

func TestTracedHandler_ContextPropagation(t *testing.T) {
	tracer := telemetry.NewTracer(mockTracerProvider(t))

	// Create a context with a value
	ctx := context.WithValue(context.Background(), "testKey", "testValue")
	handler := &tracedMockHandler{
		method: "test/method",
		result: "success",
	}

	traced := NewTracedHandler(handler, tracer)
	result, err := traced.Handle(ctx, json.RawMessage(`{}`))

	require.NoError(t, err)
	assert.Equal(t, "success", result)
}
