// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestNewTracer(t *testing.T) {
	tp := noop.NewTracerProvider()
	otelTracer := tp.Tracer("test-service")

	tracer := NewTracer(otelTracer)

	require.NotNil(t, tracer)
	assert.True(t, tracer.IsValid())
	assert.Empty(t, tracer.Name())
	assert.Equal(t, otelTracer, tracer.Internal())
}

func TestNewTracerWithName(t *testing.T) {
	tp := noop.NewTracerProvider()
	otelTracer := tp.Tracer("test-service")

	tracer := NewTracerWithName(otelTracer, "request-handler")

	require.NotNil(t, tracer)
	assert.True(t, tracer.IsValid())
	assert.Equal(t, "request-handler", tracer.Name())
}

func TestTracer_Start(t *testing.T) {
	tp := noop.NewTracerProvider()
	otelTracer := tp.Tracer("test-service")
	tracer := NewTracer(otelTracer)
	ctx := context.Background()

	t.Run("creates span with name", func(t *testing.T) {
		ctx, span := tracer.Start(ctx, "test-operation")
		defer span.End()

		assert.NotNil(t, span)
		assert.Equal(t, "test-operation", span.Name())
		assert.NotNil(t, ctx)
	})

	t.Run("creates span with options", func(t *testing.T) {
		ctx, span := tracer.Start(ctx, "test-with-options",
			WithSpanKind(trace.SpanKindServer),
			WithAttributes(attribute.String("key", "value")),
		)
		defer span.End()

		assert.NotNil(t, span)
		assert.Equal(t, "test-with-options", span.Name())
		assert.NotNil(t, ctx)
	})

	t.Run("handles nil tracer gracefully", func(t *testing.T) {
		nilTracer := NewTracer(nil)
		ctx, span := nilTracer.Start(ctx, "test-nil")
		defer span.End()

		assert.NotNil(t, span)
		assert.Equal(t, "test-nil", span.Name())
		assert.False(t, span.IsValid())
		assert.NotNil(t, ctx)
	})
}

func TestTracer_StartFromContext(t *testing.T) {
	tp := noop.NewTracerProvider()
	otelTracer := tp.Tracer("test-service")
	tracer := NewTracer(otelTracer)
	ctx := context.Background()

	t.Run("creates child span from context", func(t *testing.T) {
		// Create parent span
		parentCtx, parentSpan := tracer.Start(ctx, "parent-operation")
		defer parentSpan.End()

		// Create child span
		childCtx, childSpan := tracer.StartFromContext(parentCtx, "child-operation")
		defer childSpan.End()

		assert.NotNil(t, childSpan)
		assert.Equal(t, "child-operation", childSpan.Name())
		assert.NotNil(t, childCtx)
		assert.NotEqual(t, parentCtx, childCtx)
	})

	t.Run("creates root span when no parent exists", func(t *testing.T) {
		newCtx, span := tracer.StartFromContext(ctx, "root-operation")
		defer span.End()

		assert.NotNil(t, span)
		assert.Equal(t, "root-operation", span.Name())
		assert.NotNil(t, newCtx)
	})

	t.Run("handles nil tracer gracefully", func(t *testing.T) {
		nilTracer := NewTracer(nil)
		newCtx, span := nilTracer.StartFromContext(ctx, "test-nil")
		defer span.End()

		assert.NotNil(t, span)
		assert.Equal(t, "test-nil", span.Name())
		assert.False(t, span.IsValid())
		assert.NotNil(t, newCtx)
	})

	t.Run("creates span with custom options", func(t *testing.T) {
		ctx, span := tracer.StartFromContext(ctx, "test-with-options",
			WithSpanKind(trace.SpanKindClient),
			WithAttributes(
				attribute.String("service", "test"),
				attribute.Int("count", 42),
			),
		)
		defer span.End()

		assert.NotNil(t, span)
		assert.Equal(t, "test-with-options", span.Name())
		assert.NotNil(t, ctx)
	})
}

func TestTracer_IsValid(t *testing.T) {
	t.Run("valid tracer", func(t *testing.T) {
		tp := noop.NewTracerProvider()
		tracer := NewTracer(tp.Tracer("test"))
		assert.True(t, tracer.IsValid())
	})

	t.Run("nil tracer", func(t *testing.T) {
		tracer := NewTracer(nil)
		assert.False(t, tracer.IsValid())
	})

	t.Run("nil wrapper", func(t *testing.T) {
		var tracer *Tracer
		assert.False(t, tracer.IsValid())
	})
}

func TestTracer_Name(t *testing.T) {
	tp := noop.NewTracerProvider()

	t.Run("named tracer", func(t *testing.T) {
		tracer := NewTracerWithName(tp.Tracer("test"), "my-tracer")
		assert.Equal(t, "my-tracer", tracer.Name())
	})

	t.Run("unnamed tracer", func(t *testing.T) {
		tracer := NewTracer(tp.Tracer("test"))
		assert.Empty(t, tracer.Name())
	})
}

func TestTracer_Internal(t *testing.T) {
	tp := noop.NewTracerProvider()
	otelTracer := tp.Tracer("test-service")
	tracer := NewTracer(otelTracer)

	assert.Equal(t, otelTracer, tracer.Internal())
}

func TestTracer_Integration_WithSpan(t *testing.T) {
	tp := noop.NewTracerProvider()
	otelTracer := tp.Tracer("test-service")
	tracer := NewTracer(otelTracer)
	ctx := context.Background()

	// Test complete span lifecycle
	ctx, span := tracer.Start(ctx, "parent")
	require.NotNil(t, span)
	assert.Equal(t, "parent", span.Name())

	// Add attributes
	span.SetAttribute("operation", "test")
	span.SetAttributeInt("count", 1)
	span.SetAttributeBool("success", true)

	// Add event
	span.AddEvent("processing")

	// Create child span
	childCtx, childSpan := tracer.StartFromContext(ctx, "child")
	require.NotNil(t, childSpan)

	// Verify child has different context
	assert.NotEqual(t, ctx, childCtx)

	// End child first (proper nesting)
	childSpan.SetOK()
	childSpan.End()

	// End parent
	span.SetOK()
	span.End()
}

func TestTracer_ConcurrentAccess(t *testing.T) {
	tp := noop.NewTracerProvider()
	otelTracer := tp.Tracer("test-service")
	tracer := NewTracer(otelTracer)
	ctx := context.Background()

	// Run concurrent span creation
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			_, span := tracer.Start(ctx, "concurrent-operation")
			span.SetAttributeInt("goroutine_id", int64(id))
			span.End()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
