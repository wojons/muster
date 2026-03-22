// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTracerProvider(t *testing.T) {
	t.Run("disabled_tracing_returns_noop_provider", func(t *testing.T) {
		config := TracingConfig{
			Enabled:     false,
			ServiceName: "test-service",
		}

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.False(t, provider.IsEnabled())
		assert.Nil(t, provider.provider)
		assert.Nil(t, provider.tracer)
	})

	t.Run("enabled_tracing_with_valid_config", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		assert.NotNil(t, provider)
		assert.True(t, provider.IsEnabled())
		assert.NotNil(t, provider.provider)
		assert.NotNil(t, provider.tracer)

		// Cleanup
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = provider.Shutdown(ctx)
		assert.NoError(t, err)
	})

	t.Run("invalid_config_returns_error", func(t *testing.T) {
		config := TracingConfig{
			Enabled:     true,
			ServiceName: "", // Invalid: empty service name
		}

		provider, err := NewTracerProvider(context.Background(), config)
		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "invalid tracing configuration")
	})

	t.Run("http_protocol", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"
		config.ExporterProtocol = "http"

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		assert.NotNil(t, provider)

		// Cleanup
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})

	t.Run("grpc_protocol", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"
		config.ExporterProtocol = "grpc"

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		assert.NotNil(t, provider)

		// Cleanup
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})

	t.Run("custom_attributes", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"
		config.Attributes = map[string]string{
			"environment": "test",
			"version":     "1.2.3",
		}

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		assert.NotNil(t, provider)

		// Cleanup
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})

	t.Run("sampling_rate_zero", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"
		config.SamplingRate = 0

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		assert.NotNil(t, provider)

		// Cleanup
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})

	t.Run("propagators_custom", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"
		config.Propagators = []string{"tracecontext"}

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		assert.NotNil(t, provider)

		// Cleanup
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	})

	t.Run("context_cancellation", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Even with cancelled context, should handle gracefully
		// Note: The actual behavior depends on the exporter implementation
		provider, err := NewTracerProvider(ctx, config)
		if err != nil {
			// It's okay if this errors due to context cancellation
			assert.Contains(t, err.Error(), "failed to")
		} else {
			assert.NotNil(t, provider)
			_ = provider.Shutdown(context.Background())
		}
	})
}

func TestTracerProvider_Tracer(t *testing.T) {
	t.Run("returns_tracer_when_enabled", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		defer func() { _ = provider.Shutdown(context.Background()) }()

		tracer := provider.Tracer()
		assert.NotNil(t, tracer)
	})

	t.Run("returns_noop_tracer_when_disabled", func(t *testing.T) {
		config := TracingConfig{
			Enabled:     false,
			ServiceName: "test-service",
		}

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		defer func() { _ = provider.Shutdown(context.Background()) }()

		tracer := provider.Tracer()
		assert.NotNil(t, tracer)

		// Should be a valid tracer even when disabled
		// The tracer should not be nil and should support span creation
		ctx := context.Background()
		_, span := tracer.Start(ctx, "test-span")
		assert.NotNil(t, span)
		span.End()
	})
}

func TestTracerProvider_Shutdown(t *testing.T) {
	t.Run("shutdown_enabled_provider", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = provider.Shutdown(ctx)
		assert.NoError(t, err)
	})

	t.Run("shutdown_disabled_provider", func(t *testing.T) {
		config := TracingConfig{
			Enabled:     false,
			ServiceName: "test-service",
		}

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = provider.Shutdown(ctx)
		assert.NoError(t, err)
	})

	t.Run("multiple_shutdowns", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// First shutdown
		err = provider.Shutdown(ctx)
		assert.NoError(t, err)

		// Second shutdown should be safe
		_ = provider.Shutdown(ctx)
		// May or may not error depending on implementation, but shouldn't panic
	})
}

func TestTracerProvider_ForceFlush(t *testing.T) {
	t.Run("force_flush_enabled_provider", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		defer func() { _ = provider.Shutdown(context.Background()) }()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Force flush may error in test environment without actual collector,
		// but should not panic
		_ = provider.ForceFlush(ctx)
	})

	t.Run("force_flush_disabled_provider", func(t *testing.T) {
		config := TracingConfig{
			Enabled:     false,
			ServiceName: "test-service",
		}

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = provider.ForceFlush(ctx)
		assert.NoError(t, err)
	})
}

func TestTracerProvider_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent_tracer_access", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		defer func() { _ = provider.Shutdown(context.Background()) }()

		// Spawn multiple goroutines accessing tracer
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				defer func() { done <- true }()
				for j := 0; j < 100; j++ {
					tracer := provider.Tracer()
					assert.NotNil(t, tracer)
				}
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}
	})

	t.Run("concurrent_config_access", func(t *testing.T) {
		config := DefaultTracingConfig()
		config.ServiceName = "test-service"

		provider, err := NewTracerProvider(context.Background(), config)
		require.NoError(t, err)
		defer func() { _ = provider.Shutdown(context.Background()) }()

		// Spawn multiple goroutines reading config
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				defer func() { done <- true }()
				for j := 0; j < 100; j++ {
					cfg := provider.Config()
					assert.NotNil(t, cfg)
					assert.Equal(t, "test-service", cfg.ServiceName)
				}
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

func TestConvertAttributes(t *testing.T) {
	t.Run("converts_attributes", func(t *testing.T) {
		attrs := map[string]string{
			"key1": "value1",
			"key2": "value2",
			"key3": "value3",
		}

		result := convertAttributes(attrs)
		require.Len(t, result, 3)

		// Verify all key-value pairs are present
		keyFound := make(map[string]bool)
		for _, attr := range result {
			keyFound[string(attr.Key)] = true
		}
		assert.True(t, keyFound["key1"])
		assert.True(t, keyFound["key2"])
		assert.True(t, keyFound["key3"])
	})

	t.Run("empty_attributes", func(t *testing.T) {
		attrs := map[string]string{}

		result := convertAttributes(attrs)
		assert.Empty(t, result)
		assert.NotNil(t, result)
	})

	t.Run("nil_attributes", func(t *testing.T) {
		var attrs map[string]string

		result := convertAttributes(attrs)
		assert.Empty(t, result)
		assert.NotNil(t, result)
	})

	t.Run("empty_values", func(t *testing.T) {
		attrs := map[string]string{
			"empty": "",
		}

		result := convertAttributes(attrs)
		require.Len(t, result, 1)
		assert.Equal(t, "", result[0].Value.AsString())
	})
}
