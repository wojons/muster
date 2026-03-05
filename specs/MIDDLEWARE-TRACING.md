# Middleware: Distributed Tracing

## Overview

Distributed tracing middleware provides OpenTelemetry integration for request tracing, span creation, and context propagation across service boundaries with support for W3C Trace Context and Jaeger formats.

## Configuration

```go
type TracingConfig struct {
    Enabled           bool              `json:"enabled"`
    ServiceName       string            `json:"service_name" validate:"required"`
    ServiceVersion    string            `json:"service_version"`
    ExporterEndpoint  string            `json:"exporter_endpoint"`
    ExporterProtocol  string            `json:"exporter_protocol" validate:"oneof=grpc http"`
    SamplingRate      float64           `json:"sampling_rate" validate:"gte=0,lte=1"`
    Propagators       []string          `json:"propagators"`
    Attributes        map[string]string `json:"attributes"`
    Timeout           time.Duration     `json:"timeout"`
}

var DefaultTracingConfig = TracingConfig{
    Enabled:          true,
    ServiceName:      "muster",
    ServiceVersion:   "1.0.0",
    ExporterEndpoint: "localhost:4317",
    ExporterProtocol: "grpc",
    SamplingRate:     1.0,
    Propagators:      []string{"tracecontext", "baggage"},
    Timeout:          time.Second * 5,
}
```

## Tracer Provider

```go
package middleware

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
    "go.opentelemetry.io/otel/propagation"
    "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
    semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
    "go.opentelemetry.io/otel/trace"
)

type TracerProvider struct {
    provider *sdktrace.TracerProvider
    config   TracingConfig
    tracer   trace.Tracer
}

func NewTracerProvider(ctx context.Context, config TracingConfig) (*TracerProvider, error) {
    if !config.Enabled {
        return &TracerProvider{config: config}, nil
    }

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

    exporter, err := createExporter(ctx, config)
    if err != nil {
        return nil, fmt.Errorf("failed to create exporter: %w", err)
    }

    tp := sdktrace.NewTracerProvider(
        sdktrace.WithResource(res),
        sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SamplingRate)),
        sdktrace.WithBatcher(exporter,
            sdktrace.WithBatchTimeout(time.Second),
            sdktrace.WithExportTimeout(config.Timeout),
        ),
    )

    otel.SetTracerProvider(tp)
    otel.SetTextMapPropagator(createPropagator(config.Propagators))

    return &TracerProvider{
        provider: tp,
        config:   config,
        tracer:   tp.Tracer(config.ServiceName),
    }, nil
}

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

func convertAttributes(attrs map[string]string) []attribute.KeyValue {
    result := make([]attribute.KeyValue, 0, len(attrs))
    for k, v := range attrs {
        result = append(result, attribute.String(k, v))
    }
    return result
}

func (tp *TracerProvider) Shutdown(ctx context.Context) error {
    if tp.provider != nil {
        return tp.provider.Shutdown(ctx)
    }
    return nil
}
```

## Middleware Implementation

```go
type TracingMiddleware struct {
    config TracingConfig
    tracer trace.Tracer
}

func NewTracingMiddleware(config TracingConfig, tracer trace.Tracer) *TracingMiddleware {
    return &TracingMiddleware{
        config: config,
        tracer: tracer,
    }
}

func (m *TracingMiddleware) Handler(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !m.config.Enabled {
            next.ServeHTTP(w, r)
            return
        }

        ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

        spanName := r.Method + " " + r.URL.Path
        ctx, span := m.tracer.Start(ctx, spanName,
            trace.WithSpanKind(trace.SpanKindServer),
            trace.WithAttributes(
                semconv.HTTPMethod(r.Method),
                semconv.HTTPURL(r.URL.String()),
                semconv.HTTPTarget(r.URL.Path),
                semconv.HTTPHost(r.Host),
                semconv.HTTPScheme(r.URL.Scheme),
                semconv.HTTPUserAgent(r.UserAgent()),
                semconv.NetPeerIP(extractClientIP(r)),
            ),
        )
        defer span.End()

        ctx = context.WithValue(ctx, "trace_id", span.SpanContext().TraceID().String())
        ctx = context.WithValue(ctx, "span_id", span.SpanContext().SpanID().String())

        recorder := &tracingResponseWriter{ResponseWriter: w}
        next.ServeHTTP(recorder, r.WithContext(ctx))

        span.SetAttributes(
            semconv.HTTPStatusCode(recorder.statusCode),
        )

        if recorder.statusCode >= 400 {
            span.SetStatus(trace.Status{
                Code:        trace.StatusCodeError,
                Description: http.StatusText(recorder.statusCode),
            })
        }

        if recorder.statusCode >= 500 {
            span.RecordError(fmt.Errorf("http error: %d", recorder.statusCode))
        }
    })
}

type tracingResponseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (w *tracingResponseWriter) WriteHeader(code int) {
    w.statusCode = code
    w.ResponseWriter.WriteHeader(code)
}

func extractClientIP(r *http.Request) string {
    if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
        if idx := strings.Index(forwarded, ","); idx != -1 {
            return strings.TrimSpace(forwarded[:idx])
        }
        return forwarded
    }
    if realIP := r.Header.Get("X-Real-Ip"); realIP != "" {
        return realIP
    }
    host, _, _ := net.SplitHostPort(r.RemoteAddr)
    return host
}

func TracedHTTPClient(tp *TracerProvider) *http.Client {
    return &http.Client{
        Transport: otelhttp.NewTransport(http.DefaultTransport),
        Timeout:   time.Second * 30,
    }
}
```
