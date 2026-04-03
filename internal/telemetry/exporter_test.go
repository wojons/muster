// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExporterType_String(t *testing.T) {
	assert.Equal(t, string(ExporterOTLPGRPC), "otlp-grpc")
	assert.Equal(t, string(ExporterOTLPHTTP), "otlp-http")
	assert.Equal(t, string(ExporterJaeger), "jaeger")
	assert.Equal(t, string(ExporterZipkin), "zipkin")
	assert.Equal(t, string(ExporterStdout), "stdout")
}

func TestDefaultExporterConfig(t *testing.T) {
	config := DefaultExporterConfig()

	assert.Equal(t, ExporterOTLPGRPC, config.ExporterType)
	assert.Equal(t, "localhost:4317", config.JaegerEndpoint)
	assert.Equal(t, "localhost:4317", config.ZipkinEndpoint)
	assert.False(t, config.Insecure)
	assert.NotNil(t, config.Headers)
	assert.Equal(t, "gzip", config.Compression)
	assert.Equal(t, "localhost:4317", config.ExporterEndpoint)
	assert.Equal(t, "muster", config.ServiceName)
	assert.Equal(t, "1.0.0", config.ServiceVersion)
}

func TestExporterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ExporterConfig
		wantErr bool
	}{
		{
			name:    "valid OTLP gRPC config",
			config:  DefaultExporterConfig(),
			wantErr: false,
		},
		{
			name: "valid OTLP HTTP config",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.ExporterType = ExporterOTLPHTTP
				c.ExporterEndpoint = "localhost:4318"
				return c
			}(),
			wantErr: false,
		},
		{
			name: "valid Jaeger config",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.ExporterType = ExporterJaeger
				c.JaegerEndpoint = "jaeger:4317"
				return c
			}(),
			wantErr: false,
		},
		{
			name: "valid Zipkin config",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.ExporterType = ExporterZipkin
				c.ZipkinEndpoint = "zipkin:4317"
				return c
			}(),
			wantErr: false,
		},
		{
			name: "valid stdout config",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.ExporterType = ExporterStdout
				return c
			}(),
			wantErr: false,
		},
		{
			name: "disabled tracing skips validation",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.Enabled = false
				c.ServiceName = ""
				return c
			}(),
			wantErr: false,
		},
		{
			name: "invalid exporter type",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.ExporterType = "invalid"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "missing Jaeger endpoint",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.ExporterType = ExporterJaeger
				c.JaegerEndpoint = ""
				return c
			}(),
			wantErr: true,
		},
		{
			name: "missing Zipkin endpoint",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.ExporterType = ExporterZipkin
				c.ZipkinEndpoint = ""
				return c
			}(),
			wantErr: true,
		},
		{
			name: "invalid compression",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.Compression = "invalid"
				return c
			}(),
			wantErr: true,
		},
		{
			name: "valid compression - gzip",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.Compression = "gzip"
				return c
			}(),
			wantErr: false,
		},
		{
			name: "valid compression - none",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.Compression = "none"
				return c
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExporterConfig_IsJaeger(t *testing.T) {
	jaeger := DefaultJaegerConfig()
	zipkin := DefaultZipkinConfig()
	config := DefaultExporterConfig()
	assert.True(t, jaeger.IsJaeger())
	assert.False(t, zipkin.IsJaeger())
	assert.False(t, config.IsJaeger())
}

func TestExporterConfig_IsZipkin(t *testing.T) {
	zipkin := DefaultZipkinConfig()
	jaeger := DefaultJaegerConfig()
	config := DefaultExporterConfig()
	assert.True(t, zipkin.IsZipkin())
	assert.False(t, jaeger.IsZipkin())
	assert.False(t, config.IsZipkin())
}

func TestExporterConfig_IsOTLP(t *testing.T) {
	config := DefaultExporterConfig()
	assert.True(t, config.IsOTLP())

	config.ExporterType = ExporterOTLPHTTP
	assert.True(t, config.IsOTLP())

	config.ExporterType = ExporterJaeger
	assert.False(t, config.IsOTLP())
}

func TestExporterConfig_GetEffectiveEndpoint(t *testing.T) {
	// Jaeger
	jaeger := DefaultJaegerConfig()
	assert.Equal(t, "localhost:4317", jaeger.GetEffectiveEndpoint())

	// Zipkin
	zipkin := DefaultZipkinConfig()
	assert.Equal(t, "localhost:4317", zipkin.GetEffectiveEndpoint())

	// OTLP
	otlp := DefaultExporterConfig()
	assert.Equal(t, "localhost:4317", otlp.GetEffectiveEndpoint())

	// Custom OTLP endpoint
	otlp.ExporterEndpoint = "collector:4317"
	assert.Equal(t, "collector:4317", otlp.GetEffectiveEndpoint())
}

func TestExporterConfig_GetExporterInfo(t *testing.T) {
	tests := []struct {
		name     string
		config   ExporterConfig
		expected ExporterInfo
	}{
		{
			name:   "OTLP gRPC exporter",
			config: DefaultExporterConfig(),
			expected: ExporterInfo{
				Type:        ExporterOTLPGRPC,
				Endpoint:    "localhost:4317",
				Description: "OTLP/grpc exporter to localhost:4317",
				Healthy:     true,
			},
		},
		{
			name: "OTLP HTTP exporter",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.ExporterType = ExporterOTLPHTTP
				c.ExporterEndpoint = "localhost:4318"
				return c
			}(),
			expected: ExporterInfo{
				Type:        ExporterOTLPHTTP,
				Endpoint:    "localhost:4318",
				Description: "OTLP/http exporter to localhost:4318",
				Healthy:     true,
			},
		},
		{
			name:   "Jaeger exporter",
			config: DefaultJaegerConfig(),
			expected: ExporterInfo{
				Type:        ExporterJaeger,
				Endpoint:    "localhost:4317",
				Description: "Jaeger OTLP/gRPC exporter to localhost:4317",
				Healthy:     true,
			},
		},
		{
			name:   "Zipkin exporter",
			config: DefaultZipkinConfig(),
			expected: ExporterInfo{
				Type:        ExporterZipkin,
				Endpoint:    "localhost:4317",
				Description: "Zipkin OTLP/gRPC exporter to localhost:4317",
				Healthy:     true,
			},
		},
		{
			name: "Stdout exporter",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.ExporterType = ExporterStdout
				return c
			}(),
			expected: ExporterInfo{
				Type:        ExporterStdout,
				Endpoint:    "stdout",
				Description: "Stdout exporter (for debugging)",
				Healthy:     true,
			},
		},
		{
			name: "Unhealthy exporter - missing endpoint",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.ExporterEndpoint = ""
				return c
			}(),
			expected: ExporterInfo{
				Type:        ExporterOTLPGRPC,
				Endpoint:    "",
				Description: "OTLP/grpc exporter to ",
				Healthy:     false,
			},
		},
		{
			name: "Unhealthy exporter - disabled",
			config: func() ExporterConfig {
				c := DefaultExporterConfig()
				c.Enabled = false
				return c
			}(),
			expected: ExporterInfo{
				Type:        ExporterOTLPGRPC,
				Endpoint:    "localhost:4317",
				Description: "OTLP/grpc exporter to localhost:4317",
				Healthy:     false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := tt.config.GetExporterInfo()
			assert.Equal(t, tt.expected.Type, info.Type)
			assert.Equal(t, tt.expected.Endpoint, info.Endpoint)
			assert.Equal(t, tt.expected.Healthy, info.Healthy)
			// Only check Description contains Endpoint for non-stdout exporters
			if tt.expected.Endpoint != "stdout" {
				assert.Contains(t, info.Description, tt.expected.Endpoint)
			} else {
				assert.Contains(t, info.Description, "Stdout")
			}
		})
	}
}

func TestExporterConfig_Clone(t *testing.T) {
	original := DefaultExporterConfig()
	original.Headers = map[string]string{
		"Authorization": "Bearer token123",
		"X-Custom":      "value",
	}

	clone := original.Clone()

	// Verify values are equal
	assert.Equal(t, original.ExporterType, clone.ExporterType)
	assert.Equal(t, original.JaegerEndpoint, clone.JaegerEndpoint)
	assert.Equal(t, original.ZipkinEndpoint, clone.ZipkinEndpoint)
	assert.Equal(t, original.Headers, clone.Headers)

	// Verify maps are independent (deep copy)
	clone.Headers["New-Header"] = "new-value"
	assert.NotContains(t, original.Headers, "New-Header")

	// Verify TracingConfig is also cloned
	clone.ServiceName = "different"
	assert.NotEqual(t, original.ServiceName, clone.ServiceName)
}

func TestDefaultJaegerConfig(t *testing.T) {
	config := DefaultJaegerConfig()

	assert.Equal(t, ExporterJaeger, config.ExporterType)
	assert.Equal(t, "localhost:4317", config.JaegerEndpoint)
	assert.True(t, config.Insecure)
	assert.True(t, config.IsJaeger())
	assert.False(t, config.IsZipkin())
	assert.False(t, config.IsOTLP())
}

func TestDefaultZipkinConfig(t *testing.T) {
	config := DefaultZipkinConfig()

	assert.Equal(t, ExporterZipkin, config.ExporterType)
	assert.Equal(t, "localhost:4317", config.ZipkinEndpoint)
	assert.True(t, config.Insecure)
	assert.True(t, config.IsZipkin())
	assert.False(t, config.IsJaeger())
	assert.False(t, config.IsOTLP())
}

func TestGetExporterTypeFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected ExporterType
		wantErr  bool
	}{
		{"otlp-grpc", ExporterOTLPGRPC, false},
		{"otlp-http", ExporterOTLPHTTP, false},
		{"jaeger", ExporterJaeger, false},
		{"zipkin", ExporterZipkin, false},
		{"stdout", ExporterStdout, false},
		{"unknown", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := GetExporterTypeFromString(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestExporterConfig_Compression(t *testing.T) {
	// Valid compression values
	config := DefaultExporterConfig()
	config.Compression = "gzip"
	assert.NoError(t, config.Validate())

	config.Compression = "none"
	assert.NoError(t, config.Validate())

	config.Compression = ""
	assert.NoError(t, config.Validate()) // empty is valid

	// Invalid compression
	config.Compression = "bzip2"
	assert.Error(t, config.Validate())
}

func TestExporterConfig_Headers(t *testing.T) {
	config := DefaultExporterConfig()
	config.Headers = map[string]string{
		"Authorization":   "Bearer token",
		"X-API-Key":       "secret",
		"X-Custom-Header": "value",
	}

	assert.NoError(t, config.Validate())
	assert.Len(t, config.Headers, 3)
	assert.Equal(t, "Bearer token", config.Headers["Authorization"])
}

func TestNoopExporter(t *testing.T) {
	exporter := &noopExporter{}

	// Test ExportSpans
	err := exporter.ExportSpans(nil, nil)
	assert.NoError(t, err)

	// Test Shutdown
	err = exporter.Shutdown(nil)
	assert.NoError(t, err)
}

func TestExporterConfig_ConcurrentAccess(t *testing.T) {
	config := DefaultExporterConfig()

	// Run concurrent reads
	done := make(chan bool, 3)

	go func() {
		_ = config.GetEffectiveEndpoint()
		_ = config.IsJaeger()
		_ = config.IsZipkin()
		_ = config.IsOTLP()
		done <- true
	}()

	go func() {
		_ = config.GetExporterInfo()
		done <- true
	}()

	go func() {
		clone := config.Clone()
		_ = clone.Validate()
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestExporterConfig_EdgeCases(t *testing.T) {
	t.Run("nil headers map", func(t *testing.T) {
		config := DefaultExporterConfig()
		config.Headers = nil
		assert.NoError(t, config.Validate())
	})

	t.Run("empty headers map", func(t *testing.T) {
		config := DefaultExporterConfig()
		config.Headers = make(map[string]string)
		assert.NoError(t, config.Validate())
	})

	t.Run("large timeout", func(t *testing.T) {
		config := DefaultExporterConfig()
		config.Timeout = 0 // zero timeout
		assert.NoError(t, config.Validate())
	})

	t.Run("zero sampling rate", func(t *testing.T) {
		config := DefaultExporterConfig()
		config.SamplingRate = 0
		assert.NoError(t, config.Validate())
	})

	t.Run("full sampling rate", func(t *testing.T) {
		config := DefaultExporterConfig()
		config.SamplingRate = 1.0
		assert.NoError(t, config.Validate())
	})
}

func TestExporterInfo_Struct(t *testing.T) {
	info := ExporterInfo{
		Type:        ExporterJaeger,
		Endpoint:    "localhost:14268",
		Description: "Test Jaeger exporter",
		Healthy:     true,
	}

	assert.Equal(t, ExporterJaeger, info.Type)
	assert.Equal(t, "localhost:14268", info.Endpoint)
	assert.Equal(t, "Test Jaeger exporter", info.Description)
	assert.True(t, info.Healthy)
}
