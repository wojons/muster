// Package telemetry provides OpenTelemetry tracing support for HTTP clients
// and other components in the Muster application.
package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Tracer wraps an OpenTelemetry tracer for creating spans.
type Tracer struct {
	tp trace.Tracer
}

// Span wraps an OpenTelemetry span.
type Span struct {
	span trace.Span
}

// SpanStartOption is a functional option for span creation.
type SpanStartOption func(*spanConfig)

type spanConfig struct {
	kind       trace.SpanKind
	attributes []attribute.KeyValue
}

// NewTracer creates a new Tracer from an OpenTelemetry tracer provider's tracer.
func NewTracer(tp trace.Tracer) *Tracer {
	return &Tracer{tp: tp}
}

// Start creates a new span and returns the updated context and span.
func (t *Tracer) Start(ctx context.Context, name string, opts ...SpanStartOption) (context.Context, *Span) {
	cfg := &spanConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	var spanOpts []trace.SpanStartOption
	spanOpts = append(spanOpts, trace.WithSpanKind(cfg.kind))
	if len(cfg.attributes) > 0 {
		spanOpts = append(spanOpts, trace.WithAttributes(cfg.attributes...))
	}

	ctx, sp := t.tp.Start(ctx, name, spanOpts...)
	return ctx, &Span{span: sp}
}

// End marks the span as complete.
func (s *Span) End() {
	if s != nil && s.span != nil {
		s.span.End()
	}
}

// SetAttribute sets a key-value attribute on the span.
func (s *Span) SetAttribute(key string, value interface{}) {
	if s == nil || s.span == nil {
		return
	}
	switch v := value.(type) {
	case string:
		s.span.SetAttributes(attribute.String(key, v))
	case int:
		s.span.SetAttributes(attribute.Int(key, v))
	case int64:
		s.span.SetAttributes(attribute.Int64(key, v))
	case float64:
		s.span.SetAttributes(attribute.Float64(key, v))
	case bool:
		s.span.SetAttributes(attribute.Bool(key, v))
	default:
		s.span.SetAttributes(attribute.String(key, "<unknown type>"))
	}
}

// SetAttributeInt sets an int64 attribute on the span.
func (s *Span) SetAttributeInt(key string, value int64) {
	if s != nil && s.span != nil {
		s.span.SetAttributes(attribute.Int64(key, value))
	}
}

// SetAttributeBool sets a boolean attribute on the span.
func (s *Span) SetAttributeBool(key string, value bool) {
	if s != nil && s.span != nil {
		s.span.SetAttributes(attribute.Bool(key, value))
	}
}

// RecordError records an error on the span.
func (s *Span) RecordError(err error) {
	if s != nil && s.span != nil && err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
}

// SetOK marks the span as successful.
func (s *Span) SetOK() {
	if s != nil && s.span != nil {
		s.span.SetStatus(codes.Ok, "")
	}
}

// WithSpanKind sets the span kind for the span.
func WithSpanKind(kind trace.SpanKind) SpanStartOption {
	return func(cfg *spanConfig) {
		cfg.kind = kind
	}
}

// WithAttributes sets initial attributes for the span.
func WithAttributes(attrs ...attribute.KeyValue) SpanStartOption {
	return func(cfg *spanConfig) {
		cfg.attributes = append(cfg.attributes, attrs...)
	}
}
