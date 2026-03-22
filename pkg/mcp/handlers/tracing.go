package handlers

import (
	"context"
	"encoding/json"

	"github.com/openapi-cli/openapi-cli/internal/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TracedHandler wraps a Handler with OpenTelemetry tracing.
// It creates spans for each method call and records relevant attributes.
type TracedHandler struct {
	handler Handler
	tracer  *telemetry.Tracer
}

// NewTracedHandler creates a new traced handler wrapper.
//
// Example:
//
//	handler := NewListToolsHandler(registry)
//	tracedHandler := NewTracedHandler(handler, tracer)
//	registry.Register(tracedHandler)
func NewTracedHandler(handler Handler, tracer *telemetry.Tracer) *TracedHandler {
	return &TracedHandler{
		handler: handler,
		tracer:  tracer,
	}
}

// Method returns the method name this handler handles.
func (h *TracedHandler) Method() string {
	return h.handler.Method()
}

// Handle processes the request with tracing.
// It creates a span, records attributes, and tracks success/failure.
func (h *TracedHandler) Handle(ctx context.Context, params json.RawMessage) (interface{}, error) {
	// Create a span for this handler call
	ctx, span := h.tracer.Start(ctx, "mcp.handler."+h.handler.Method(),
		telemetry.WithSpanKind(trace.SpanKindServer),
		telemetry.WithAttributes(
			attribute.String("mcp.method", h.handler.Method()),
			attribute.Int("mcp.params_size", len(params)),
		),
	)
	defer span.End()

	// Record the request parameters size
	span.SetAttributeInt("mcp.params_bytes", int64(len(params)))

	// Execute the actual handler
	result, err := h.handler.Handle(ctx, params)

	// Record outcome
	if err != nil {
		span.RecordError(err)
		span.SetAttributeBool("mcp.error", true)
		if mcpErr, ok := err.(*MCPError); ok {
			span.SetAttributeInt("mcp.error_code", int64(mcpErr.Code))
			span.SetAttribute("mcp.error_message", mcpErr.Message)
		}
	} else {
		span.SetOK()
		span.SetAttributeBool("mcp.success", true)
		if result != nil {
			// Try to get result type for better observability
			switch result.(type) {
			case *ListToolsResult:
				span.SetAttribute("mcp.result_type", "ListToolsResult")
			case *CallToolResult:
				span.SetAttribute("mcp.result_type", "CallToolResult")
			case *ListPromptsResult:
				span.SetAttribute("mcp.result_type", "ListPromptsResult")
			case *GetPromptResult:
				span.SetAttribute("mcp.result_type", "GetPromptResult")
			case *ListResourcesResult:
				span.SetAttribute("mcp.result_type", "ListResourcesResult")
			case *ReadResourceResult:
				span.SetAttribute("mcp.result_type", "ReadResourceResult")
			case *InitializeResult:
				span.SetAttribute("mcp.result_type", "InitializeResult")
			case *ListRootsResult:
				span.SetAttribute("mcp.result_type", "ListRootsResult")
			default:
				span.SetAttribute("mcp.result_type", "unknown")
			}
		}
	}

	return result, err
}

// TracingRegistry wraps a Registry and automatically traces all registered handlers.
// It provides a convenient way to add tracing to all MCP handlers.
type TracingRegistry struct {
	*Registry
	tracer *telemetry.Tracer
}

// NewTracingRegistry creates a new registry with automatic tracing.
//
// Example:
//
//	baseRegistry := NewRegistry()
//	tracingRegistry := NewTracingRegistry(baseRegistry, tracer)
//
//	// All handlers registered will be automatically traced
//	tracingRegistry.Register(NewListToolsHandler(commands))
//	tracingRegistry.Register(NewCallToolHandler(executor))
func NewTracingRegistry(registry *Registry, tracer *telemetry.Tracer) *TracingRegistry {
	return &TracingRegistry{
		Registry: registry,
		tracer:   tracer,
	}
}

// Register registers a handler with tracing automatically applied.
// The handler is wrapped in a TracedHandler before registration.
func (r *TracingRegistry) Register(handler Handler) {
	tracedHandler := NewTracedHandler(handler, r.tracer)
	r.Registry.Register(tracedHandler)
}

// TracingOptions provides configuration for handler tracing.
type TracingOptions struct {
	// RecordParams determines whether to record the actual parameter content in spans.
	// Default is false to avoid leaking sensitive data.
	RecordParams bool

	// MaxParamSize is the maximum size of params to record (if RecordParams is true).
	// Default is 1024 bytes.
	MaxParamSize int
}

// DefaultTracingOptions returns the default tracing options.
func DefaultTracingOptions() TracingOptions {
	return TracingOptions{
		RecordParams: false,
		MaxParamSize: 1024,
	}
}
