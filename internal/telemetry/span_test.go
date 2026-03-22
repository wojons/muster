// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// setupTestTracer creates a test tracer with an in-memory exporter.
func setupTestTracer(t *testing.T) (context.Context, trace.Tracer, *tracetest.InMemoryExporter) {
	ctx := context.Background()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
	)
	tracer := provider.Tracer("test")
	return ctx, tracer, exporter
}

func TestStartSpan(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("creates span with default options", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "test-operation")
		require.NotNil(t, span)
		defer span.End()

		assert.Equal(t, "test-operation", span.Name())
		assert.True(t, span.IsRecording())
		assert.NotEmpty(t, span.TraceID())
		assert.NotEmpty(t, span.SpanID())
	})

	t.Run("creates span with custom kind", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "server-operation",
			WithSpanKind(trace.SpanKindServer),
		)
		require.NotNil(t, span)
		defer span.End()

		assert.Equal(t, "server-operation", span.Name())
	})

	t.Run("creates span with attributes", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "attributed-operation",
			WithAttributes(attribute.String("key1", "value1")),
		)
		require.NotNil(t, span)
		defer span.End()

		assert.Equal(t, "attributed-operation", span.Name())
	})
}

func TestStartSpanFromContext(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("creates child span from context", func(t *testing.T) {
		parentCtx, parentSpan := StartSpan(ctx, tracer, "parent")
		defer parentSpan.End()

		_, childSpan := StartSpanFromContext(parentCtx, "child")
		require.NotNil(t, childSpan)
		defer childSpan.End()

		assert.Equal(t, "child", childSpan.Name())
		assert.Equal(t, parentSpan.TraceID(), childSpan.TraceID())
		assert.NotEqual(t, parentSpan.SpanID(), childSpan.SpanID())
		assert.Equal(t, parentSpan.SpanContext().TraceID(), childSpan.SpanContext().TraceID())
	})

	t.Run("creates root span when no parent exists", func(t *testing.T) {
		newCtx, span := StartSpanFromContext(context.Background(), "root")
		require.NotNil(t, span)
		defer span.End()

		assert.Equal(t, "root", span.Name())
		assert.NotEmpty(t, span.TraceID())
		_ = newCtx // Context is returned but we just verify it exists
	})
}

func TestSpan_SetAttributes(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("sets string attribute", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "string-attr")
		defer span.End()

		span.SetAttribute("string-key", "string-value")
		assert.True(t, span.IsRecording())
	})

	t.Run("sets int attribute", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "int-attr")
		defer span.End()

		span.SetAttributeInt("int-key", 42)
		assert.True(t, span.IsRecording())
	})

	t.Run("sets float attribute", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "float-attr")
		defer span.End()

		span.SetAttributeFloat("float-key", 3.14)
		assert.True(t, span.IsRecording())
	})

	t.Run("sets bool attribute", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "bool-attr")
		defer span.End()

		span.SetAttributeBool("bool-key", true)
		assert.True(t, span.IsRecording())
	})

	t.Run("sets multiple attributes", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "multi-attr")
		defer span.End()

		span.SetAttributes(
			attribute.String("key1", "value1"),
			attribute.Int("key2", 123),
		)
		assert.True(t, span.IsRecording())
	})
}

func TestSpan_RecordError(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("records error", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "error-span")
		defer span.End()

		testErr := errors.New("test error")
		span.RecordError(testErr)
		assert.True(t, span.IsRecording())
	})

	t.Run("records error with message", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "error-msg-span")
		defer span.End()

		testErr := errors.New("test error")
		span.RecordErrorWithMessage(testErr, "custom error message")
		assert.True(t, span.IsRecording())
	})

	t.Run("handles nil error", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "nil-error-span")
		defer span.End()

		span.RecordError(nil)
		assert.True(t, span.IsRecording())
	})
}

func TestSpan_SetStatus(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("sets error status", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "error-status")
		defer span.End()

		span.SetStatus(codes.Error, "something went wrong")
		assert.True(t, span.IsRecording())
	})

	t.Run("sets OK status", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "ok-status")
		defer span.End()

		span.SetOK()
		assert.True(t, span.IsRecording())
	})
}

func TestSpan_AddEvent(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("adds simple event", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "event-span")
		defer span.End()

		span.AddEvent("something happened")
		assert.True(t, span.IsRecording())
	})

	t.Run("adds event with attributes", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "event-attr-span")
		defer span.End()

		span.AddEventWithAttributes("event with data",
			attribute.String("key", "value"),
		)
		assert.True(t, span.IsRecording())
	})

	t.Run("adds event with timestamp", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "event-time-span")
		defer span.End()

		span.AddEventWithTimestamp("timed event", time.Now())
		assert.True(t, span.IsRecording())
	})
}

func TestSpan_Context(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("context contains span", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "context-span")
		defer span.End()

		newCtx := span.ContextWithSpan(ctx)
		retrievedSpan := SpanFromContext(newCtx)

		require.NotNil(t, retrievedSpan)
		assert.Equal(t, span.TraceID(), retrievedSpan.TraceID())
		assert.Equal(t, span.SpanID(), retrievedSpan.SpanID())
	})
}

func TestSpan_SpanContext(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("returns valid span context", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "context-check")
		defer span.End()

		spanCtx := span.SpanContext()
		assert.True(t, spanCtx.IsValid())
		assert.NotEmpty(t, spanCtx.TraceID().String())
		assert.NotEmpty(t, spanCtx.SpanID().String())
	})
}

func TestSpanFromContext(t *testing.T) {
	t.Run("returns nil when no span in context", func(t *testing.T) {
		span := SpanFromContext(context.Background())
		assert.Nil(t, span)
	})

	t.Run("returns span when present", func(t *testing.T) {
		ctx, tracer, _ := setupTestTracer(t)
		ctx, originalSpan := StartSpan(ctx, tracer, "original")
		defer originalSpan.End()

		retrievedSpan := SpanFromContext(ctx)
		require.NotNil(t, retrievedSpan)
		assert.Equal(t, originalSpan.TraceID(), retrievedSpan.TraceID())
	})
}

func TestSpan_IsValid(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("valid span returns true", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "valid")
		defer span.End()

		assert.True(t, span.IsValid())
	})

	t.Run("nil span returns false", func(t *testing.T) {
		var span *Span
		assert.False(t, span.IsValid())
	})
}

func TestSpan_String(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("returns string representation", func(t *testing.T) {
		_, span := StartSpan(ctx, tracer, "string-test")
		defer span.End()

		str := span.String()
		assert.Contains(t, str, "string-test")
		assert.Contains(t, str, "traceID=")
		assert.Contains(t, str, "spanID=")
	})

	t.Run("handles nil span", func(t *testing.T) {
		var span *Span
		assert.Equal(t, "Span{nil}", span.String())
	})
}

func TestSpan_ConcurrentAccess(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)
	_, span := StartSpan(ctx, tracer, "concurrent")
	defer span.End()

	// Run concurrent operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			span.SetAttributeInt("concurrent-key", int64(idx))
			span.AddEvent("concurrent-event")
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	assert.True(t, span.IsRecording())
}

func TestSpan_MultipleSpans(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	// Create a span hierarchy
	rootCtx, rootSpan := StartSpan(ctx, tracer, "root")
	defer rootSpan.End()

	child1Ctx, child1Span := StartSpan(rootCtx, tracer, "child1")
	defer child1Span.End()

	_, child2Span := StartSpan(rootCtx, tracer, "child2")
	defer child2Span.End()

	grandchildCtx, grandchildSpan := StartSpan(child1Ctx, tracer, "grandchild")
	defer grandchildSpan.End()

	// Verify all spans have same trace ID
	rootTraceID := rootSpan.TraceID()
	assert.Equal(t, rootTraceID, child1Span.TraceID())
	assert.Equal(t, rootTraceID, child2Span.TraceID())
	assert.Equal(t, rootTraceID, grandchildSpan.TraceID())

	// Verify all span IDs are unique
	spanIDs := map[string]bool{
		rootSpan.SpanID():       true,
		child1Span.SpanID():     true,
		child2Span.SpanID():     true,
		grandchildSpan.SpanID(): true,
	}
	assert.Len(t, spanIDs, 4, "All span IDs should be unique")

	// Verify context propagation
	retrievedSpan := SpanFromContext(grandchildCtx)
	require.NotNil(t, retrievedSpan)
	assert.Equal(t, grandchildSpan.TraceID(), retrievedSpan.TraceID())
	assert.Equal(t, grandchildSpan.SpanID(), retrievedSpan.SpanID())
}

func TestSpan_Lifecycle(t *testing.T) {
	ctx, tracer, _ := setupTestTracer(t)

	t.Run("full span lifecycle", func(t *testing.T) {
		// Create span
		_, span := StartSpan(ctx, tracer, "lifecycle-test")

		// Verify initial state
		assert.True(t, span.IsRecording())
		assert.NotEmpty(t, span.TraceID())
		assert.NotEmpty(t, span.SpanID())
		assert.Equal(t, "lifecycle-test", span.Name())

		// Add attributes
		span.SetAttribute("step", "1")
		span.SetAttributeInt("count", 42)
		span.SetAttributeFloat("ratio", 0.5)
		span.SetAttributeBool("valid", true)

		// Add events
		span.AddEvent("processing-started")
		span.AddEventWithAttributes("processing-step",
			attribute.String("step", "validation"),
		)

		// Record an error
		span.RecordError(errors.New("test error"))

		// Set final status
		span.SetOK()

		// End span
		span.End()

		// Verify span is no longer recording
		assert.False(t, span.IsRecording())
	})
}
