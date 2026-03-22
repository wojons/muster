// Package telemetry provides distributed tracing capabilities using OpenTelemetry.
//
// This package implements distributed tracing middleware with support for:
//   - OpenTelemetry integration for request tracing
//   - Span creation and context propagation
//   - W3C Trace Context format support
//   - Jaeger format support
//   - Configurable exporters (gRPC and HTTP)
//   - Sampling control
//   - Custom attributes and baggage
//
// # Basic Usage
//
// To initialize the telemetry system:
//
//	ctx := context.Background()
//	config := telemetry.TracingConfig{
//	    Enabled:          true,
//	    ServiceName:      "muster",
//	    ServiceVersion:   "1.0.0",
//	    ExporterEndpoint: "localhost:4317",
//	    ExporterProtocol: "grpc",
//	    SamplingRate:     1.0,
//	}
//
//	provider, err := telemetry.NewTracerProvider(ctx, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer provider.Shutdown(ctx)
//
// # Configuration
//
// The tracing system supports the following configuration options:
//
//   - Enabled: Enable/disable tracing (default: true)
//   - ServiceName: Service identifier for traces (required)
//   - ServiceVersion: Service version for traces
//   - ExporterEndpoint: OTLP collector endpoint (default: localhost:4317)
//   - ExporterProtocol: Transport protocol - "grpc" or "http" (default: grpc)
//   - SamplingRate: Probability of sampling (0.0-1.0, default: 1.0)
//   - Propagators: List of propagators - "tracecontext", "baggage"
//   - Attributes: Custom attributes to add to all spans
//   - Timeout: Export timeout (default: 5s)
//
// # Thread Safety
//
// All types in this package are safe for concurrent use:
//   - TracerProvider: Safe to use across multiple goroutines
//   - Tracer: Safe to use across multiple goroutines
//   - Configuration: Should be set once at initialization
//
// # Error Handling
//
// The telemetry package follows Go conventions for error handling:
//   - Constructor functions return (*Type, error)
//   - Errors include context via fmt.Errorf with %w verb
//   - Graceful degradation when tracing is disabled
//
// # Integration
//
// This package integrates with:
//   - MCP handlers for tracing tool calls
//   - WASM runtime for tracing plugin execution
//   - HTTP client for tracing outgoing requests
//   - HTTP server middleware for tracing incoming requests
package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// Ensure imports are used
var (
	_ = context.Background
	_ = time.Second
	_ = otel.GetTracerProvider
	_ = attribute.String
	_ = otlptrace.Client(nil)
	_ = otlptracegrpc.New
	_ = otlptracehttp.New
	_ = propagation.TraceContext{}
	_ = resource.New
	_ = sdktrace.TracerProvider{}
	_ = semconv.ServiceName
	_ trace.Tracer
	_ = otelhttp.NewTransport
)
