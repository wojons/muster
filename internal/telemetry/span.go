// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Span wraps an OpenTelemetry span with helper methods for common operations.
// It provides a simplified interface for adding attributes, recording errors,
// and managing span lifecycle.
type Span struct {
	span trace.Span
	name string
}

// SpanOption configures a Span.
type SpanOption func(*SpanConfig)

// SpanConfig holds configuration for span creation.
type SpanConfig struct {
	Kind       trace.SpanKind
	Attributes []attribute.KeyValue
}

// WithSpanKind sets the span kind.
func WithSpanKind(kind trace.SpanKind) SpanOption {
	return func(cfg *SpanConfig) {
		cfg.Kind = kind
	}
}

// WithAttributes adds attributes to the span.
func WithAttributes(attrs ...attribute.KeyValue) SpanOption {
	return func(cfg *SpanConfig) {
		cfg.Attributes = append(cfg.Attributes, attrs...)
	}
}

// StartSpan creates a new span from the given context and tracer.
// Returns the updated context and the new span.
//
// Example:
//
//	ctx, span := telemetry.StartSpan(ctx, tracer, "process-request")
//	defer span.End()
func StartSpan(ctx context.Context, tracer trace.Tracer, name string, opts ...SpanOption) (context.Context, *Span) {
	cfg := &SpanConfig{
		Kind: trace.SpanKindInternal,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	ctx, span := tracer.Start(ctx, name,
		trace.WithSpanKind(cfg.Kind),
		trace.WithAttributes(cfg.Attributes...),
	)

	return ctx, &Span{
		span: span,
		name: name,
	}
}

// StartSpanFromContext creates a new span as a child of the span in the context.
// If no span exists in the context, creates a root span.
//
// Example:
//
//	ctx, span := telemetry.StartSpanFromContext(ctx, "process-item")
//	defer span.End()
func StartSpanFromContext(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	tracer := trace.SpanFromContext(ctx).TracerProvider().Tracer("muster")
	return StartSpan(ctx, tracer, name, opts...)
}

// End completes the span. This must be called when the operation completes.
// Use defer immediately after creating a span to ensure proper cleanup.
//
// Example:
//
//	ctx, span := telemetry.StartSpan(ctx, tracer, "operation")
//	defer span.End()
func (s *Span) End() {
	if s.span != nil {
		s.span.End()
	}
}

// EndWithOptions completes the span with additional options.
func (s *Span) EndWithOptions(opts ...trace.SpanEndOption) {
	if s.span != nil {
		s.span.End(opts...)
	}
}

// SetAttribute adds a string attribute to the span.
func (s *Span) SetAttribute(key string, value string) {
	if s.span != nil {
		s.span.SetAttributes(attribute.String(key, value))
	}
}

// SetAttributeInt adds an integer attribute to the span.
func (s *Span) SetAttributeInt(key string, value int64) {
	if s.span != nil {
		s.span.SetAttributes(attribute.Int64(key, value))
	}
}

// SetAttributeFloat adds a float attribute to the span.
func (s *Span) SetAttributeFloat(key string, value float64) {
	if s.span != nil {
		s.span.SetAttributes(attribute.Float64(key, value))
	}
}

// SetAttributeBool adds a boolean attribute to the span.
func (s *Span) SetAttributeBool(key string, value bool) {
	if s.span != nil {
		s.span.SetAttributes(attribute.Bool(key, value))
	}
}

// SetAttributes adds multiple attributes to the span.
func (s *Span) SetAttributes(attrs ...attribute.KeyValue) {
	if s.span != nil {
		s.span.SetAttributes(attrs...)
	}
}

// RecordError records an error as an exception on the span.
// It also sets the span status to Error.
func (s *Span) RecordError(err error) {
	if s.span != nil && err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
}

// RecordErrorWithMessage records an error with a custom message.
func (s *Span) RecordErrorWithMessage(err error, message string) {
	if s.span != nil && err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, message)
	}
}

// SetStatus sets the span status and message.
func (s *Span) SetStatus(code codes.Code, description string) {
	if s.span != nil {
		s.span.SetStatus(code, description)
	}
}

// SetOK marks the span as successful.
func (s *Span) SetOK() {
	if s.span != nil {
		s.span.SetStatus(codes.Ok, "")
	}
}

// AddEvent adds a named event to the span with the current timestamp.
func (s *Span) AddEvent(name string) {
	if s.span != nil {
		s.span.AddEvent(name)
	}
}

// AddEventWithAttributes adds an event with attributes to the span.
func (s *Span) AddEventWithAttributes(name string, attrs ...attribute.KeyValue) {
	if s.span != nil {
		s.span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// AddEventWithTimestamp adds an event with a specific timestamp.
func (s *Span) AddEventWithTimestamp(name string, timestamp time.Time, attrs ...attribute.KeyValue) {
	if s.span != nil {
		s.span.AddEvent(name,
			trace.WithAttributes(attrs...),
			trace.WithTimestamp(timestamp),
		)
	}
}

// IsRecording returns whether the span is recording events and attributes.
func (s *Span) IsRecording() bool {
	if s.span != nil {
		return s.span.IsRecording()
	}
	return false
}

// SpanContext returns the SpanContext of the span.
func (s *Span) SpanContext() trace.SpanContext {
	if s.span != nil {
		return s.span.SpanContext()
	}
	return trace.SpanContext{}
}

// TraceID returns the trace ID of the span.
func (s *Span) TraceID() string {
	if s.span != nil {
		return s.span.SpanContext().TraceID().String()
	}
	return ""
}

// SpanID returns the span ID.
func (s *Span) SpanID() string {
	if s.span != nil {
		return s.span.SpanContext().SpanID().String()
	}
	return ""
}

// Name returns the span name.
func (s *Span) Name() string {
	return s.name
}

// ContextWithSpan returns a new context with the span stored.
func (s *Span) ContextWithSpan(ctx context.Context) context.Context {
	if s.span != nil {
		return trace.ContextWithSpan(ctx, s.span)
	}
	return ctx
}

// SpanFromContext retrieves the current span from the context.
// Returns nil if no span exists in the context.
func SpanFromContext(ctx context.Context) *Span {
	span := trace.SpanFromContext(ctx)
	if span == nil || !span.SpanContext().IsValid() {
		return nil
	}
	return &Span{
		span: span,
		name: "",
	}
}

// IsValid returns whether the span is valid (non-nil and recording).
func (s *Span) IsValid() bool {
	return s != nil && s.span != nil && s.span.SpanContext().IsValid()
}

// String returns a string representation of the span including name and IDs.
func (s *Span) String() string {
	if s == nil || s.span == nil {
		return "Span{nil}"
	}
	ctx := s.span.SpanContext()
	return fmt.Sprintf("Span{name=%s, traceID=%s, spanID=%s}",
		s.name,
		ctx.TraceID().String(),
		ctx.SpanID().String(),
	)
}
