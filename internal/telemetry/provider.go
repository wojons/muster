// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TracerProvider wraps OpenTelemetry's TracerProvider with configuration
type TracerProvider struct {
	provider *sdktrace.TracerProvider
	config   TracingConfig
	tracer   trace.Tracer
}

// NewTracerProvider creates a new TracerProvider with the given configuration.
// It initializes the OpenTelemetry SDK with the configured exporters, resource attributes,
// and sampling configuration.
//
// If tracing is disabled (config.Enabled == false), returns a no-op provider that
// can still be used safely throughout the application without requiring nil checks.
func NewTracerProvider(ctx context.Context, config TracingConfig) (*TracerProvider, error) {
	if !config.Enabled {
		return &TracerProvider{config: config}, nil
	}

	// Validate configuration before proceeding
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid tracing configuration: %w", err)
	}

	// Create resource with service attributes
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
		),
		resource.WithAttributes(convertAttributes(config.Attributes)...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create the appropriate exporter based on configuration
	exporter, err := createExporter(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create exporter: %w", err)
	}

	// Create the TracerProvider with the resource and exporter
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SamplingRate)),
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(time.Second),
			sdktrace.WithExportTimeout(config.Timeout),
		),
	)

	// Set as global TracerProvider and configure propagators
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(createPropagator(config.Propagators))

	return &TracerProvider{
		provider: tp,
		config:   config,
		tracer:   tp.Tracer(config.ServiceName),
	}, nil
}

// createExporter creates the appropriate span exporter based on the protocol configuration.
func createExporter(ctx context.Context, config TracingConfig) (sdktrace.SpanExporter, error) {
	switch config.ExporterProtocol {
	case "grpc":
		return otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(config.ExporterEndpoint),
			otlptracegrpc.WithTimeout(config.Timeout),
		)
	case "http":
		return otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(config.ExporterEndpoint),
			otlptracehttp.WithTimeout(config.Timeout),
		)
	default:
		return nil, fmt.Errorf("unsupported exporter protocol: %s", config.ExporterProtocol)
	}
}

// createPropagator creates a composite text map propagator from the configured propagators.
func createPropagator(propagators []string) propagation.TextMapPropagator {
	var props []propagation.TextMapPropagator
	for _, p := range propagators {
		switch p {
		case "tracecontext":
			props = append(props, propagation.TraceContext{})
		case "baggage":
			props = append(props, propagation.Baggage{})
		}
	}
	return propagation.NewCompositeTextMapPropagator(props...)
}

// convertAttributes converts a map of string attributes to OpenTelemetry KeyValues.
func convertAttributes(attrs map[string]string) []attribute.KeyValue {
	result := make([]attribute.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		result = append(result, attribute.String(k, v))
	}
	return result
}

// Shutdown gracefully shuts down the TracerProvider, flushing any pending spans.
// This should be called during application shutdown to ensure all traces are exported.
func (tp *TracerProvider) Shutdown(ctx context.Context) error {
	if tp.provider != nil {
		return tp.provider.Shutdown(ctx)
	}
	return nil
}

// Tracer returns the tracer instance for this provider.
// Returns a no-op tracer if tracing is disabled.
func (tp *TracerProvider) Tracer() trace.Tracer {
	if tp.tracer != nil {
		return tp.tracer
	}
	return noop.NewTracerProvider().Tracer("")
}

// IsEnabled returns whether tracing is currently enabled.
func (tp *TracerProvider) IsEnabled() bool {
	return tp.config.Enabled
}

// Config returns a copy of the provider's configuration.
func (tp *TracerProvider) Config() TracingConfig {
	return tp.config.Clone()
}

// ForceFlush immediately exports all spans that have not yet been exported.
// This is useful for testing and ensuring spans are exported before application exit.
func (tp *TracerProvider) ForceFlush(ctx context.Context) error {
	if tp.provider != nil {
		return tp.provider.ForceFlush(ctx)
	}
	return nil
}
