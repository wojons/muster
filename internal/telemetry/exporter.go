// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ExporterType defines the type of trace exporter
type ExporterType string

const (
	// ExporterOTLPGRPC exports traces via OTLP/gRPC
	ExporterOTLPGRPC ExporterType = "otlp-grpc"
	// ExporterOTLPHTTP exports traces via OTLP/HTTP
	ExporterOTLPHTTP ExporterType = "otlp-http"
	// ExporterJaeger exports traces to Jaeger (via OTLP)
	ExporterJaeger ExporterType = "jaeger"
	// ExporterZipkin exports traces to Zipkin (via OTLP)
	ExporterZipkin ExporterType = "zipkin"
	// ExporterStdout exports traces to stdout (for debugging)
	ExporterStdout ExporterType = "stdout"
)

// ExporterConfig extends TracingConfig with exporter-specific settings
type ExporterConfig struct {
	TracingConfig

	// ExporterType specifies which exporter to use
	ExporterType ExporterType `json:"exporter_type"`

	// JaegerEndpoint is the Jaeger collector endpoint
	// Jaeger natively supports OTLP since v1.35+
	// Format: "host:port" (e.g., "localhost:4317" for gRPC, "localhost:4318" for HTTP)
	JaegerEndpoint string `json:"jaeger_endpoint"`

	// ZipkinEndpoint is the Zipkin collector endpoint
	// Zipkin supports OTLP via the OpenTelemetry Collector
	// Format: "host:port" (e.g., "localhost:4317" for gRPC, "localhost:4318" for HTTP)
	ZipkinEndpoint string `json:"zipkin_endpoint"`

	// Insecure allows connecting to OTLP collectors without TLS
	// Useful for local development with Jaeger/Zipkin
	Insecure bool `json:"insecure"`

	// Headers are additional headers to send with trace exports
	Headers map[string]string `json:"headers"`

	// Compression specifies the compression algorithm
	// Valid values: "gzip", "none"
	Compression string `json:"compression"`
}

// DefaultExporterConfig returns an ExporterConfig with sensible defaults
func DefaultExporterConfig() ExporterConfig {
	return ExporterConfig{
		TracingConfig:  DefaultTracingConfig(),
		ExporterType:   ExporterOTLPGRPC,
		JaegerEndpoint: "localhost:4317",
		ZipkinEndpoint: "localhost:4317",
		Insecure:       false,
		Headers:        make(map[string]string),
		Compression:    "gzip",
	}
}

// Validate checks if the exporter configuration is valid
func (c *ExporterConfig) Validate() error {
	// Validate base tracing configuration
	if err := c.TracingConfig.Validate(); err != nil {
		return fmt.Errorf("invalid tracing configuration: %w", err)
	}

	// Validate exporter type
	switch c.ExporterType {
	case ExporterOTLPGRPC, ExporterOTLPHTTP, ExporterJaeger, ExporterZipkin, ExporterStdout:
		// Valid
	default:
		return fmt.Errorf("unsupported exporter type: %s (valid values: otlp-grpc, otlp-http, jaeger, zipkin, stdout)", c.ExporterType)
	}

	// Validate endpoint based on exporter type
	if c.TracingConfig.Enabled {
		switch c.ExporterType {
		case ExporterJaeger:
			if c.JaegerEndpoint == "" {
				return fmt.Errorf("jaeger_endpoint is required when exporter_type is 'jaeger'")
			}
		case ExporterZipkin:
			if c.ZipkinEndpoint == "" {
				return fmt.Errorf("zipkin_endpoint is required when exporter_type is 'zipkin'")
			}
		case ExporterOTLPGRPC, ExporterOTLPHTTP:
			if c.ExporterEndpoint == "" {
				return fmt.Errorf("exporter_endpoint is required when exporter_type is '%s'", c.ExporterType)
			}
		}
	}

	// Validate compression
	if c.Compression != "" && c.Compression != "gzip" && c.Compression != "none" {
		return fmt.Errorf("invalid compression: %s (valid values: gzip, none)", c.Compression)
	}

	return nil
}

// createSpanExporter creates the appropriate span exporter based on configuration
func createSpanExporter(ctx context.Context, config ExporterConfig) (sdktrace.SpanExporter, error) {
	switch config.ExporterType {
	case ExporterOTLPGRPC:
		return createOTLPGRPCExporter(ctx, config)
	case ExporterOTLPHTTP:
		return createOTLPHTTPExporter(ctx, config)
	case ExporterJaeger:
		return createJaegerExporter(ctx, config)
	case ExporterZipkin:
		return createZipkinExporter(ctx, config)
	case ExporterStdout:
		return createStdoutExporter()
	default:
		return nil, fmt.Errorf("unsupported exporter type: %s", config.ExporterType)
	}
}

// createOTLPGRPCExporter creates an OTLP/gRPC exporter
func createOTLPGRPCExporter(ctx context.Context, config ExporterConfig) (sdktrace.SpanExporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.ExporterEndpoint),
		otlptracegrpc.WithTimeout(config.Timeout),
	}

	if config.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	if len(config.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(config.Headers))
	}

	if config.Compression == "gzip" {
		opts = append(opts, otlptracegrpc.WithCompressor("gzip"))
	}

	return otlptracegrpc.New(ctx, opts...)
}

// createOTLPHTTPExporter creates an OTLP/HTTP exporter
func createOTLPHTTPExporter(ctx context.Context, config ExporterConfig) (sdktrace.SpanExporter, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(config.ExporterEndpoint),
		otlptracehttp.WithTimeout(config.Timeout),
	}

	if config.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	if len(config.Headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(config.Headers))
	}

	if config.Compression == "gzip" {
		opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
	}

	return otlptracehttp.New(ctx, opts...)
}

// createJaegerExporter creates a Jaeger exporter using OTLP/gRPC
// Jaeger supports OTLP natively since v1.35+
func createJaegerExporter(ctx context.Context, config ExporterConfig) (sdktrace.SpanExporter, error) {
	// Jaeger supports OTLP natively, use OTLP gRPC exporter
	// Default Jaeger OTLP port: 4317 (gRPC), 4318 (HTTP)
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.JaegerEndpoint),
		otlptracegrpc.WithTimeout(config.Timeout),
	}

	if config.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	if len(config.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(config.Headers))
	}

	return otlptracegrpc.New(ctx, opts...)
}

// createZipkinExporter creates a Zipkin exporter using OTLP/gRPC
// Zipkin supports OTLP via the OpenTelemetry Collector
func createZipkinExporter(ctx context.Context, config ExporterConfig) (sdktrace.SpanExporter, error) {
	// Zipkin supports OTLP via OpenTelemetry Collector
	// Can use OTLP gRPC exporter to the collector
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(config.ZipkinEndpoint),
		otlptracegrpc.WithTimeout(config.Timeout),
	}

	if config.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	if len(config.Headers) > 0 {
		opts = append(opts, otlptracegrpc.WithHeaders(config.Headers))
	}

	return otlptracegrpc.New(ctx, opts...)
}

// createStdoutExporter creates a stdout exporter for debugging
func createStdoutExporter() (sdktrace.SpanExporter, error) {
	// Return a simple no-op exporter for stdout
	// In production, this would use a proper stdout exporter
	return &noopExporter{}, nil
}

// noopExporter is a simple exporter that does nothing (for stdout/development)
type noopExporter struct{}

func (n *noopExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	return nil
}

func (n *noopExporter) Shutdown(ctx context.Context) error {
	return nil
}

// ExporterInfo provides metadata about the configured exporter
type ExporterInfo struct {
	Type        ExporterType
	Endpoint    string
	Description string
	Healthy     bool
}

// GetExporterInfo returns information about the current exporter configuration
func (c *ExporterConfig) GetExporterInfo() ExporterInfo {
	switch c.ExporterType {
	case ExporterOTLPGRPC, ExporterOTLPHTTP:
		return ExporterInfo{
			Type:        c.ExporterType,
			Endpoint:    c.ExporterEndpoint,
			Description: fmt.Sprintf("OTLP/%s exporter to %s", c.ExporterProtocol, c.ExporterEndpoint),
			Healthy:     c.ExporterEndpoint != "" && c.Enabled,
		}
	case ExporterJaeger:
		return ExporterInfo{
			Type:        ExporterJaeger,
			Endpoint:    c.JaegerEndpoint,
			Description: fmt.Sprintf("Jaeger OTLP/gRPC exporter to %s", c.JaegerEndpoint),
			Healthy:     c.JaegerEndpoint != "" && c.Enabled,
		}
	case ExporterZipkin:
		return ExporterInfo{
			Type:        ExporterZipkin,
			Endpoint:    c.ZipkinEndpoint,
			Description: fmt.Sprintf("Zipkin OTLP/gRPC exporter to %s", c.ZipkinEndpoint),
			Healthy:     c.ZipkinEndpoint != "" && c.Enabled,
		}
	case ExporterStdout:
		return ExporterInfo{
			Type:        ExporterStdout,
			Endpoint:    "stdout",
			Description: "Stdout exporter (for debugging)",
			Healthy:     c.Enabled,
		}
	default:
		return ExporterInfo{
			Type:        c.ExporterType,
			Endpoint:    "",
			Description: "Unknown exporter",
			Healthy:     false,
		}
	}
}

// IsJaeger returns true if the exporter type is Jaeger
func (c *ExporterConfig) IsJaeger() bool {
	return c.ExporterType == ExporterJaeger
}

// IsZipkin returns true if the exporter type is Zipkin
func (c *ExporterConfig) IsZipkin() bool {
	return c.ExporterType == ExporterZipkin
}

// IsOTLP returns true if the exporter type is OTLP (gRPC or HTTP)
func (c *ExporterConfig) IsOTLP() bool {
	return c.ExporterType == ExporterOTLPGRPC || c.ExporterType == ExporterOTLPHTTP
}

// GetEffectiveEndpoint returns the effective endpoint based on exporter type
func (c *ExporterConfig) GetEffectiveEndpoint() string {
	switch c.ExporterType {
	case ExporterJaeger:
		return c.JaegerEndpoint
	case ExporterZipkin:
		return c.ZipkinEndpoint
	default:
		return c.ExporterEndpoint
	}
}

// Clone creates a deep copy of the exporter configuration
func (c *ExporterConfig) Clone() ExporterConfig {
	clone := *c
	clone.TracingConfig = c.TracingConfig.Clone()

	if c.Headers != nil {
		clone.Headers = make(map[string]string, len(c.Headers))
		for k, v := range c.Headers {
			clone.Headers[k] = v
		}
	}

	return clone
}

// DefaultJaegerConfig returns a configuration for sending traces to Jaeger
// Uses OTLP/gRPC which Jaeger supports natively since v1.35+
func DefaultJaegerConfig() ExporterConfig {
	config := DefaultExporterConfig()
	config.ExporterType = ExporterJaeger
	config.JaegerEndpoint = "localhost:4317"
	config.Insecure = true // Jaeger typically runs without TLS locally
	return config
}

// DefaultZipkinConfig returns a configuration for sending traces to Zipkin
// Uses OTLP/gRPC via OpenTelemetry Collector
func DefaultZipkinConfig() ExporterConfig {
	config := DefaultExporterConfig()
	config.ExporterType = ExporterZipkin
	config.ZipkinEndpoint = "localhost:4317"
	config.Insecure = true
	return config
}

// GetExporterTypeFromString converts a string to ExporterType
func GetExporterTypeFromString(s string) (ExporterType, error) {
	switch s {
	case "otlp-grpc":
		return ExporterOTLPGRPC, nil
	case "otlp-http":
		return ExporterOTLPHTTP, nil
	case "jaeger":
		return ExporterJaeger, nil
	case "zipkin":
		return ExporterZipkin, nil
	case "stdout":
		return ExporterStdout, nil
	default:
		return "", fmt.Errorf("unknown exporter type: %s", s)
	}
}
