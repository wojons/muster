package telemetry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracingConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TracingConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with defaults",
			config: TracingConfig{
				Enabled:          true,
				ServiceName:      "test-service",
				ExporterEndpoint: "localhost:4317",
				ExporterProtocol: "grpc",
				SamplingRate:     1.0,
			},
			wantErr: false,
		},
		{
			name: "valid config with http protocol",
			config: TracingConfig{
				Enabled:          true,
				ServiceName:      "test-service",
				ExporterEndpoint: "localhost:4318",
				ExporterProtocol: "http",
				SamplingRate:     0.5,
			},
			wantErr: false,
		},
		{
			name: "disabled tracing requires no validation",
			config: TracingConfig{
				Enabled:     false,
				ServiceName: "",
			},
			wantErr: false,
		},
		{
			name: "missing service name",
			config: TracingConfig{
				Enabled:          true,
				ServiceName:      "",
				ExporterEndpoint: "localhost:4317",
			},
			wantErr: true,
			errMsg:  "service_name is required",
		},
		{
			name: "missing exporter endpoint",
			config: TracingConfig{
				Enabled:          true,
				ServiceName:      "test-service",
				ExporterEndpoint: "",
			},
			wantErr: true,
			errMsg:  "exporter_endpoint is required",
		},
		{
			name: "invalid protocol",
			config: TracingConfig{
				Enabled:          true,
				ServiceName:      "test-service",
				ExporterEndpoint: "localhost:4317",
				ExporterProtocol: "invalid",
				SamplingRate:     1.0,
			},
			wantErr: true,
			errMsg:  "exporter_protocol must be 'grpc' or 'http'",
		},
		{
			name: "sampling rate too high",
			config: TracingConfig{
				Enabled:          true,
				ServiceName:      "test-service",
				ExporterEndpoint: "localhost:4317",
				SamplingRate:     1.5,
			},
			wantErr: true,
			errMsg:  "sampling_rate must be between 0.0 and 1.0",
		},
		{
			name: "sampling rate too low",
			config: TracingConfig{
				Enabled:          true,
				ServiceName:      "test-service",
				ExporterEndpoint: "localhost:4317",
				SamplingRate:     -0.5,
			},
			wantErr: true,
			errMsg:  "sampling_rate must be between 0.0 and 1.0",
		},
		{
			name: "invalid propagator",
			config: TracingConfig{
				Enabled:          true,
				ServiceName:      "test-service",
				ExporterEndpoint: "localhost:4317",
				SamplingRate:     1.0,
				Propagators:      []string{"invalid"},
			},
			wantErr: true,
			errMsg:  "invalid propagator",
		},
		{
			name: "valid propagators",
			config: TracingConfig{
				Enabled:          true,
				ServiceName:      "test-service",
				ExporterEndpoint: "localhost:4317",
				SamplingRate:     1.0,
				Propagators:      []string{"tracecontext", "baggage"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDefaultTracingConfig(t *testing.T) {
	config := DefaultTracingConfig()

	assert.True(t, config.Enabled)
	assert.Equal(t, "muster", config.ServiceName)
	assert.Equal(t, "1.0.0", config.ServiceVersion)
	assert.Equal(t, "localhost:4317", config.ExporterEndpoint)
	assert.Equal(t, "grpc", config.ExporterProtocol)
	assert.Equal(t, 1.0, config.SamplingRate)
	assert.Equal(t, 5*time.Second, config.Timeout)
	assert.Equal(t, []string{"tracecontext", "baggage"}, config.Propagators)
}

func TestTracingConfig_Clone(t *testing.T) {
	original := TracingConfig{
		Enabled:          true,
		ServiceName:      "test-service",
		ServiceVersion:   "1.0.0",
		ExporterEndpoint: "localhost:4317",
		ExporterProtocol: "grpc",
		SamplingRate:     0.5,
		Propagators:      []string{"tracecontext", "baggage"},
		Attributes: map[string]string{
			"env":     "production",
			"version": "v1.2.3",
		},
		Timeout: 10 * time.Second,
	}

	clone := original.Clone()

	// Verify all fields are copied
	assert.Equal(t, original.Enabled, clone.Enabled)
	assert.Equal(t, original.ServiceName, clone.ServiceName)
	assert.Equal(t, original.ServiceVersion, clone.ServiceVersion)
	assert.Equal(t, original.ExporterEndpoint, clone.ExporterEndpoint)
	assert.Equal(t, original.ExporterProtocol, clone.ExporterProtocol)
	assert.Equal(t, original.SamplingRate, clone.SamplingRate)
	assert.Equal(t, original.Propagators, clone.Propagators)
	assert.Equal(t, original.Attributes, clone.Attributes)
	assert.Equal(t, original.Timeout, clone.Timeout)

	// Modify clone and verify original is unchanged
	clone.Propagators = append(clone.Propagators, "custom")
	clone.Attributes["new"] = "value"

	assert.Len(t, original.Propagators, 2)
	assert.Len(t, original.Attributes, 2)
	assert.NotContains(t, original.Attributes, "new")
}

func TestTracingConfig_Clone_NilSlices(t *testing.T) {
	original := TracingConfig{
		Enabled:     true,
		ServiceName: "test",
	}

	clone := original.Clone()

	assert.Nil(t, clone.Propagators)
	assert.Nil(t, clone.Attributes)
}

func TestTracingConfig_IsValidProtocol(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		want     bool
	}{
		{"empty", "", true},
		{"grpc", "grpc", true},
		{"http", "http", true},
		{"invalid", "invalid", false},
		{"GRPC", "GRPC", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := TracingConfig{ExporterProtocol: tt.protocol}
			assert.Equal(t, tt.want, config.IsValidProtocol())
		})
	}
}

func TestTracingConfig_GetProtocol(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		want     string
	}{
		{"empty returns grpc", "", "grpc"},
		{"grpc", "grpc", "grpc"},
		{"http", "http", "http"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := TracingConfig{ExporterProtocol: tt.protocol}
			assert.Equal(t, tt.want, config.GetProtocol())
		})
	}
}

func TestTracingConfig_ZeroSamplingRate(t *testing.T) {
	config := TracingConfig{
		Enabled:          true,
		ServiceName:      "test",
		ExporterEndpoint: "localhost:4317",
		SamplingRate:     0.0,
	}

	err := config.Validate()
	require.NoError(t, err)
}

func TestTracingConfig_ConcurrentAccess(t *testing.T) {
	config := TracingConfig{
		Enabled:          true,
		ServiceName:      "test",
		ExporterEndpoint: "localhost:4317",
		Attributes:       make(map[string]string),
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		go func() {
			_ = config.Validate()
			_ = config.IsValidProtocol()
			_ = config.GetProtocol()
		}()
	}

	// Concurrent clones
	for i := 0; i < 50; i++ {
		go func() {
			_ = config.Clone()
		}()
	}
}
