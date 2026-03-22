// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// Tracer wraps an OpenTelemetry tracer with helper methods for creating spans.
// It provides a simplified interface for starting spans that returns our custom
// Span wrapper instead of the raw OpenTelemetry span.
type Tracer struct {
	tracer trace.Tracer
	name   string
}

// NewTracer creates a new Tracer wrapper from an OpenTelemetry tracer.
//
// Example:
//
//	tracer := telemetry.NewTracer(tp.Tracer("my-service"))
//	ctx, span := tracer.Start(ctx, "process-request")
//	defer span.End()
func NewTracer(tracer trace.Tracer) *Tracer {
	return &Tracer{
		tracer: tracer,
		name:   "",
	}
}

// NewTracerWithName creates a new Tracer wrapper with a custom name.
//
// Example:
//
//	tracer := telemetry.NewTracerWithName(tp.Tracer("my-service"), "request-handler")
//	ctx, span := tracer.Start(ctx, "process")
//	defer span.End()
func NewTracerWithName(tracer trace.Tracer, name string) *Tracer {
	return &Tracer{
		tracer: tracer,
		name:   name,
	}
}

// Start creates a new span with the given name and returns the updated context
// and the new span wrapper.
//
// Example:
//
//	ctx, span := tracer.Start(ctx, "process-request")
//	defer span.End()
func (t *Tracer) Start(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	if t.tracer == nil {
		// Return a no-op span if tracer is nil
		return ctx, &Span{name: name}
	}
	return StartSpan(ctx, t.tracer, name, opts...)
}

// StartFromContext creates a new span as a child of the span in the context.
// If no span exists in the context, creates a root span.
//
// Example:
//
//	ctx, span := tracer.StartFromContext(ctx, "process-item")
//	defer span.End()
func (t *Tracer) StartFromContext(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	if t.tracer == nil {
		// Return a no-op span if tracer is nil
		return ctx, &Span{name: name}
	}

	cfg := &SpanConfig{
		Kind: trace.SpanKindInternal,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	ctx, span := t.tracer.Start(ctx, name,
		trace.WithSpanKind(cfg.Kind),
		trace.WithAttributes(cfg.Attributes...),
	)

	return ctx, &Span{
		span: span,
		name: name,
	}
}

// Name returns the tracer name.
func (t *Tracer) Name() string {
	return t.name
}

// IsValid returns whether the tracer is valid (has an underlying OpenTelemetry tracer).
func (t *Tracer) IsValid() bool {
	return t != nil && t.tracer != nil
}

// Internal returns the underlying OpenTelemetry tracer.
// This should be used sparingly and only when direct access to the
// OpenTelemetry API is required.
func (t *Tracer) Internal() trace.Tracer {
	return t.tracer
}
