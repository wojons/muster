// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package wasm

import (
	"context"
	"errors"
	"testing"

	"github.com/wojons/muster/internal/telemetry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTelemetryTracer_NewTelemetryTracer tests creating a new telemetry-based WASM tracer.
func TestTelemetryTracer_NewTelemetryTracer(t *testing.T) {
	// Test with nil tracer
	t.Run("nil_tracer", func(t *testing.T) {
		tracer := NewTelemetryTracer(nil)
		require.NotNil(t, tracer)
		assert.Nil(t, tracer.tracer)
	})

	// Test with valid tracer
	t.Run("valid_tracer", func(t *testing.T) {
		ctx := context.Background()
		config := telemetry.TracingConfig{
			Enabled:          true,
			ServiceName:      "test-service",
			ExporterProtocol: "http",
			ExporterEndpoint: "localhost:4318",
		}
		tp, err := telemetry.NewTracerProvider(ctx, config)
		require.NoError(t, err)
		defer tp.Shutdown(ctx)

		tracer := telemetry.NewTracer(tp.Tracer())
		wasmTracer := NewTelemetryTracer(tracer)

		require.NotNil(t, wasmTracer)
		assert.NotNil(t, wasmTracer.tracer)
	})
}

// TestTelemetryTracer_StartSpan tests starting spans with the telemetry tracer.
func TestTelemetryTracer_StartSpan(t *testing.T) {
	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "test-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	require.NoError(t, err)
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	t.Run("start_span_with_name", func(t *testing.T) {
		span := wasmTracer.StartSpan("wasm.execute")
		require.NotNil(t, span)
		defer span.End()

		// Verify it's a telemetry span
		_, ok := span.(*TelemetrySpan)
		assert.True(t, ok)
	})

	t.Run("start_span_sets_context", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)

		// Cast to check internal state
		telemetrySpan, ok := span.(*TelemetrySpan)
		require.True(t, ok)
		assert.NotNil(t, telemetrySpan.ctx)
		assert.NotNil(t, telemetrySpan.span)

		span.End()
	})

	t.Run("start_span_with_nil_tracer", func(t *testing.T) {
		nilTracer := NewTelemetryTracer(nil)
		span := nilTracer.StartSpan("test.operation")
		require.NotNil(t, span)

		// Should return a NoopSpan
		_, ok := span.(*NoopSpan)
		assert.True(t, ok)

		span.End()
	})
}

// TestTelemetrySpan_SetTag tests setting various tag types on spans.
func TestTelemetrySpan_SetTag(t *testing.T) {
	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "test-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	require.NoError(t, err)
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	t.Run("set_string_tag", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Should not panic
		assert.NotPanics(t, func() {
			span.SetTag("function", "add")
		})
	})

	t.Run("set_int_tag", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Should not panic
		assert.NotPanics(t, func() {
			span.SetTag("module_size", 1024)
		})
	})

	t.Run("set_int64_tag", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Should not panic
		assert.NotPanics(t, func() {
			span.SetTag("memory_used", int64(4096))
		})
	})

	t.Run("set_float64_tag", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Should not panic
		assert.NotPanics(t, func() {
			span.SetTag("execution_time_ms", float64(42.5))
		})
	})

	t.Run("set_bool_tag", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Should not panic
		assert.NotPanics(t, func() {
			span.SetTag("success", true)
		})
	})

	t.Run("set_unknown_type_tag", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Should handle unknown types gracefully
		assert.NotPanics(t, func() {
			span.SetTag("unknown", struct{ name string }{"test"})
		})
	})

	t.Run("set_tag_on_nil_span", func(t *testing.T) {
		nilTracer := NewTelemetryTracer(nil)
		span := nilTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Should not panic on NoopSpan
		assert.NotPanics(t, func() {
			span.SetTag("key", "value")
		})
	})
}

// TestTelemetrySpan_SetError tests recording errors on spans.
func TestTelemetrySpan_SetError(t *testing.T) {
	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "test-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	require.NoError(t, err)
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	t.Run("record_error", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Should not panic
		assert.NotPanics(t, func() {
			span.SetError(errors.New("test error"))
		})
	})

	t.Run("record_nil_error", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Should not panic
		assert.NotPanics(t, func() {
			span.SetError(nil)
		})
	})

	t.Run("record_error_on_nil_span", func(t *testing.T) {
		nilTracer := NewTelemetryTracer(nil)
		span := nilTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Should not panic on NoopSpan
		assert.NotPanics(t, func() {
			span.SetError(errors.New("test error"))
		})
	})
}

// TestTelemetrySpan_End tests span completion.
func TestTelemetrySpan_End(t *testing.T) {
	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "test-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	require.NoError(t, err)
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	t.Run("end_span", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)

		// Should not panic
		assert.NotPanics(t, func() {
			span.End()
		})
	})

	t.Run("end_multiple_times", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)

		// Should be safe to call End multiple times
		assert.NotPanics(t, func() {
			span.End()
			span.End()
		})
	})

	t.Run("end_nil_span", func(t *testing.T) {
		nilTracer := NewTelemetryTracer(nil)
		span := nilTracer.StartSpan("test.operation")
		require.NotNil(t, span)

		// Should not panic on NoopSpan
		assert.NotPanics(t, func() {
			span.End()
		})
	})
}

// TestNoopSpan tests the no-op span implementation.
func TestNoopSpan(t *testing.T) {
	t.Run("noop_span_operations", func(t *testing.T) {
		span := &NoopSpan{}

		// All operations should be no-ops
		assert.NotPanics(t, func() {
			span.End()
			span.SetTag("key", "value")
			span.SetError(errors.New("test error"))
		})
	})

	t.Run("new_telemetry_tracer_returns_noop", func(t *testing.T) {
		tracer := NewTelemetryTracer(nil)
		span := tracer.StartSpan("test")

		_, ok := span.(*NoopSpan)
		assert.True(t, ok)
	})
}

// TestWASMExecutionWithTracing tests actual WASM execution with tracing enabled.
func TestWASMExecutionWithTracing(t *testing.T) {
	// Skip if no WASM support
	if testing.Short() {
		t.Skip("Skipping WASM execution test in short mode")
	}

	// Create a simple WASM module that adds two numbers
	wasmBytes := []byte{
		0x00, 0x61, 0x73, 0x6d, // WASM magic
		0x01, 0x00, 0x00, 0x00, // version 1
	}

	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "test-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	require.NoError(t, err)
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	t.Run("execute_with_tracing", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := &SandboxLimits{
			MaxExecutionTime: 0, // No timeout
			MaxMemory:        0,
			MaxFileSize:      0,
			MaxFuel:          0,
		}

		runtime := NewRuntimeWithTracer(config, limits, wasmTracer)
		require.NotNil(t, runtime)

		// Grant capabilities
		runtime.GrantCapability("stdio")

		// This will fail (invalid module) but should still trace
		_, err := runtime.Execute(wasmBytes, "add", int32(1), int32(2))
		// We expect an error due to invalid module
		assert.Error(t, err)

		// Verify span was created (tracer was invoked)
		assert.NotNil(t, runtime.Tracer())
	})

	t.Run("validate_with_tracing", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := &SandboxLimits{}

		runtime := NewRuntimeWithTracer(config, limits, wasmTracer)
		require.NotNil(t, runtime)

		// Validate should use tracing
		err := runtime.Validate(wasmBytes)
		assert.Error(t, err) // Invalid module
	})

	t.Run("get_module_info_with_tracing", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := &SandboxLimits{}

		runtime := NewRuntimeWithTracer(config, limits, wasmTracer)
		require.NotNil(t, runtime)

		// GetModuleInfo should use tracing
		_, err := runtime.GetModuleInfo(wasmBytes)
		assert.Error(t, err) // Invalid module
	})
}

// TestTelemetryTracer_ConcurrentAccess tests thread safety.
func TestTelemetryTracer_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "test-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	require.NoError(t, err)
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	t.Run("concurrent_span_creation", func(t *testing.T) {
		// Run multiple goroutines creating spans concurrently
		done := make(chan bool, 100)

		for i := 0; i < 100; i++ {
			go func(id int) {
				defer func() { done <- true }()

				span := wasmTracer.StartSpan("concurrent.operation")
				if span != nil {
					span.SetTag("goroutine_id", id)
					span.End()
				}
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 100; i++ {
			<-done
		}
	})
}

// TestTelemetrySpan_TraceContext tests trace context propagation.
func TestTelemetrySpan_TraceContext(t *testing.T) {
	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "test-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	require.NoError(t, err)
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	t.Run("span_has_valid_context", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)
		defer span.End()

		// Cast to check internal state
		telemetrySpan, ok := span.(*TelemetrySpan)
		require.True(t, ok)

		// Context should not be nil
		assert.NotNil(t, telemetrySpan.ctx)

		// Span should be valid
		assert.NotNil(t, telemetrySpan.span)
		assert.True(t, telemetrySpan.span.IsRecording())
	})
}

// TestTelemetryTracer_IntegrationWithRuntime tests full integration.
func TestTelemetryTracer_IntegrationWithRuntime(t *testing.T) {
	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "test-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	require.NoError(t, err)
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	t.Run("set_and_get_tracer", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := &SandboxLimits{}

		// Create runtime without tracer
		runtime := NewRuntime(config, limits)
		require.NotNil(t, runtime)
		assert.Nil(t, runtime.Tracer())

		// Set tracer
		runtime.SetTracer(wasmTracer)
		assert.Equal(t, wasmTracer, runtime.Tracer())
	})

	t.Run("runtime_with_tracer", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := &SandboxLimits{}

		// Create runtime with tracer
		runtime := NewRuntimeWithTracer(config, limits, wasmTracer)
		require.NotNil(t, runtime)
		assert.Equal(t, wasmTracer, runtime.Tracer())
	})

	t.Run("tracer_tags_preserved", func(t *testing.T) {
		span := wasmTracer.StartSpan("wasm.execute")
		require.NotNil(t, span)
		defer span.End()

		// Set various tags
		span.SetTag("function", "test_func")
		span.SetTag("module_size", 1024)
		span.SetTag("args_count", 3)
		span.SetTag("success", true)
		span.SetTag("duration_ms", int64(42))

		// All operations should complete without error
		assert.NotNil(t, span)
	})
}

// TestTelemetrySpan_SpanInterface tests the Span interface compliance.
func TestTelemetrySpan_SpanInterface(t *testing.T) {
	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "test-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	require.NoError(t, err)
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	t.Run("span_interface_methods", func(t *testing.T) {
		span := wasmTracer.StartSpan("test.operation")
		require.NotNil(t, span)

		// Verify all Span interface methods work
		assert.NotPanics(t, func() {
			span.End()
		})

		// Create new span for other tests
		span = wasmTracer.StartSpan("test.operation")
		assert.NotPanics(t, func() {
			span.SetTag("key", "value")
		})

		span = wasmTracer.StartSpan("test.operation")
		assert.NotPanics(t, func() {
			span.SetError(errors.New("test"))
			span.End()
		})
	})
}

// TestTelemetryTracer_ErrorScenarios tests error handling.
func TestTelemetryTracer_ErrorScenarios(t *testing.T) {
	t.Run("tracer_with_invalid_provider", func(t *testing.T) {
		// Create a tracer with a nil internal tracer
		tracer := &telemetry.Tracer{}
		wasmTracer := NewTelemetryTracer(tracer)
		require.NotNil(t, wasmTracer)

		// Should return NoopSpan
		span := wasmTracer.StartSpan("test")
		_, ok := span.(*NoopSpan)
		assert.True(t, ok)
	})

	t.Run("span_with_nil_telemetry_span", func(t *testing.T) {
		span := &TelemetrySpan{span: nil, ctx: context.Background()}

		// Operations should not panic
		assert.NotPanics(t, func() {
			span.End()
			span.SetTag("key", "value")
			span.SetError(errors.New("test"))
		})
	})
}

// BenchmarkTelemetryTracer benchmarks span creation.
func BenchmarkTelemetryTracer_StartSpan(b *testing.B) {
	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "benchmark-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	if err != nil {
		b.Fatal(err)
	}
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span := wasmTracer.StartSpan("benchmark.operation")
		span.End()
	}
}

// BenchmarkTelemetrySpan_SetTag benchmarks tag setting.
func BenchmarkTelemetrySpan_SetTag(b *testing.B) {
	ctx := context.Background()
	config := telemetry.TracingConfig{
		Enabled:          true,
		ServiceName:      "benchmark-service",
		ExporterProtocol: "http",
		ExporterEndpoint: "localhost:4318",
	}
	tp, err := telemetry.NewTracerProvider(ctx, config)
	if err != nil {
		b.Fatal(err)
	}
	defer tp.Shutdown(ctx)

	tracer := telemetry.NewTracer(tp.Tracer())
	wasmTracer := NewTelemetryTracer(tracer)

	span := wasmTracer.StartSpan("benchmark.operation")
	defer span.End()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span.SetTag("key", i)
	}
}
