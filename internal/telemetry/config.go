package telemetry

import (
	"fmt"
	"time"
)

// TracingConfig defines configuration options for distributed tracing.
//
// This struct controls all aspects of the tracing system including
// service identification, export configuration, sampling behavior,
// and context propagation settings.
type TracingConfig struct {
	// Enabled controls whether tracing is active.
	// When false, the tracer provider will be a no-op implementation.
	Enabled bool `json:"enabled"`

	// ServiceName identifies the service in trace data.
	// This is a required field and should be unique per service.
	ServiceName string `json:"service_name" validate:"required"`

	// ServiceVersion specifies the version of the service.
	// Included in trace attributes for deployment tracking.
	ServiceVersion string `json:"service_version"`

	// ExporterEndpoint is the address of the OTLP collector.
	// Format: "host:port" (e.g., "localhost:4317" for gRPC)
	ExporterEndpoint string `json:"exporter_endpoint"`

	// ExporterProtocol specifies the transport protocol.
	// Valid values: "grpc" or "http"
	ExporterProtocol string `json:"exporter_protocol" validate:"oneof=grpc http"`

	// SamplingRate controls the probability of sampling a trace.
	// Valid range: 0.0 (sample nothing) to 1.0 (sample everything)
	SamplingRate float64 `json:"sampling_rate" validate:"gte=0,lte=1"`

	// Propagators specifies which propagation formats to use.
	// Valid values: "tracecontext", "baggage"
	Propagators []string `json:"propagators"`

	// Attributes are custom key-value pairs added to all spans.
	Attributes map[string]string `json:"attributes"`

	// Timeout is the maximum time to wait for trace export.
	Timeout time.Duration `json:"timeout"`
}

// DefaultTracingConfig returns a TracingConfig with sensible defaults.
//
// Defaults:
//   - Enabled: true
//   - ServiceName: "muster"
//   - ServiceVersion: "1.0.0"
//   - ExporterEndpoint: "localhost:4317"
//   - ExporterProtocol: "grpc"
//   - SamplingRate: 1.0 (100% sampling)
//   - Propagators: ["tracecontext", "baggage"]
//   - Timeout: 5 seconds
func DefaultTracingConfig() TracingConfig {
	return TracingConfig{
		Enabled:          true,
		ServiceName:      "muster",
		ServiceVersion:   "1.0.0",
		ExporterEndpoint: "localhost:4317",
		ExporterProtocol: "grpc",
		SamplingRate:     1.0,
		Propagators:      []string{"tracecontext", "baggage"},
		Timeout:          time.Second * 5,
	}
}

// Validate checks if the configuration is valid.
//
// Returns an error if:
//   - ServiceName is empty
//   - ExporterProtocol is not "grpc" or "http"
//   - SamplingRate is outside the range [0.0, 1.0]
//   - ExporterEndpoint is empty when Enabled is true
func (c *TracingConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.ServiceName == "" {
		return fmt.Errorf("service_name is required when tracing is enabled")
	}

	if c.ExporterEndpoint == "" {
		return fmt.Errorf("exporter_endpoint is required when tracing is enabled")
	}

	if c.ExporterProtocol != "" &&
		c.ExporterProtocol != "grpc" &&
		c.ExporterProtocol != "http" {
		return fmt.Errorf("exporter_protocol must be 'grpc' or 'http', got: %s", c.ExporterProtocol)
	}

	if c.SamplingRate < 0.0 || c.SamplingRate > 1.0 {
		return fmt.Errorf("sampling_rate must be between 0.0 and 1.0, got: %f", c.SamplingRate)
	}

	// Validate propagators
	for _, p := range c.Propagators {
		if p != "tracecontext" && p != "baggage" {
			return fmt.Errorf("invalid propagator: %s (valid values: tracecontext, baggage)", p)
		}
	}

	return nil
}

// Clone creates a deep copy of the configuration.
//
// This is useful when creating variations of a base configuration.
func (c *TracingConfig) Clone() TracingConfig {
	clone := *c

	if c.Propagators != nil {
		clone.Propagators = make([]string, len(c.Propagators))
		copy(clone.Propagators, c.Propagators)
	}

	if c.Attributes != nil {
		clone.Attributes = make(map[string]string, len(c.Attributes))
		for k, v := range c.Attributes {
			clone.Attributes[k] = v
		}
	}

	return clone
}

// IsValidProtocol checks if the exporter protocol is valid.
func (c *TracingConfig) IsValidProtocol() bool {
	return c.ExporterProtocol == "" ||
		c.ExporterProtocol == "grpc" ||
		c.ExporterProtocol == "http"
}

// GetProtocol returns the effective exporter protocol.
// Returns "grpc" if not explicitly set.
func (c *TracingConfig) GetProtocol() string {
	if c.ExporterProtocol == "" {
		return "grpc"
	}
	return c.ExporterProtocol
}
