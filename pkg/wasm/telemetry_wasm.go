package wasm

import (
	"context"
	"fmt"

	"github.com/openapi-cli/openapi-cli/internal/telemetry"
)

// TelemetryTracer adapts the internal telemetry package to the WASM Tracer interface.
// This allows WASM execution to be traced using OpenTelemetry.
//
// Example:
//
//	tracer := telemetry.NewTracer(tp.Tracer("wasm"))
//	wasmTracer := NewTelemetryTracer(tracer)
//	runtime := NewRuntimeWithTracer(config, limits, wasmTracer)
type TelemetryTracer struct {
	tracer *telemetry.Tracer
}

// NewTelemetryTracer creates a new telemetry-based WASM tracer.
func NewTelemetryTracer(tracer *telemetry.Tracer) *TelemetryTracer {
	return &TelemetryTracer{tracer: tracer}
}

// StartSpan creates a new span for WASM execution.
func (t *TelemetryTracer) StartSpan(name string) Span {
	if t.tracer == nil {
		return &NoopSpan{}
	}
	// Use a background context since WASM execution manages its own context
	ctx, span := t.tracer.Start(context.Background(), name)
	return &TelemetrySpan{span: span, ctx: ctx}
}

// TelemetrySpan adapts telemetry.Span to the WASM Span interface.
type TelemetrySpan struct {
	span *telemetry.Span
	ctx  context.Context
}

// End completes the span.
func (s *TelemetrySpan) End() {
	if s.span != nil {
		s.span.End()
	}
}

// SetTag adds a tag/attribute to the span.
func (s *TelemetrySpan) SetTag(key string, value interface{}) {
	if s.span == nil {
		return
	}
	switch v := value.(type) {
	case string:
		s.span.SetAttribute(key, v)
	case int:
		s.span.SetAttributeInt(key, int64(v))
	case int64:
		s.span.SetAttributeInt(key, v)
	case float64:
		s.span.SetAttributeFloat(key, v)
	case bool:
		s.span.SetAttributeBool(key, v)
	default:
		s.span.SetAttribute(key, fmt.Sprintf("%v", value))
	}
}

// SetError marks the span as errored with the given error.
func (s *TelemetrySpan) SetError(err error) {
	if s.span != nil && err != nil {
		s.span.RecordError(err)
	}
}

// NoopSpan is a no-op span implementation for when tracing is disabled.
type NoopSpan struct{}

// End completes the no-op span.
func (s *NoopSpan) End() {}

// SetTag adds a tag to the no-op span.
func (s *NoopSpan) SetTag(key string, value interface{}) {}

// SetError marks an error on the no-op span.
func (s *NoopSpan) SetError(err error) {}
