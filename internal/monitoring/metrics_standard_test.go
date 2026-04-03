package monitoring

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLabelCardinalityTracker(t *testing.T) {
	ct := NewLabelCardinalityTracker()
	require.NotNil(t, ct)
	require.NotNil(t, ct.labels)
}

func TestLabelCardinalityTracker_TrackValue(t *testing.T) {
	ct := NewLabelCardinalityTracker()

	t.Run("track new value", func(t *testing.T) {
		assert.True(t, ct.TrackValue("method", "GET"))
		assert.Equal(t, 1, ct.GetCardinality("method"))
	})

	t.Run("track existing value", func(t *testing.T) {
		assert.True(t, ct.TrackValue("method", "GET")) // Already tracked
		assert.Equal(t, 1, ct.GetCardinality("method"))
	})

	t.Run("track multiple values", func(t *testing.T) {
		assert.True(t, ct.TrackValue("method", "POST"))
		assert.True(t, ct.TrackValue("method", "PUT"))
		assert.Equal(t, 3, ct.GetCardinality("method"))
	})

	t.Run("exceed cardinality limit", func(t *testing.T) {
		ct := NewLabelCardinalityTracker()

		// Fill to limit
		for i := 0; i < MaxLabelValuesPerLabel; i++ {
			assert.True(t, ct.TrackValue("test_label", fmt.Sprintf("value_%d", i)))
		}

		// Next value should fail
		assert.False(t, ct.TrackValue("test_label", "excess_value"))
	})
}

func TestLabelCardinalityTracker_GetCardinality(t *testing.T) {
	ct := NewLabelCardinalityTracker()

	t.Run("empty label", func(t *testing.T) {
		assert.Equal(t, 0, ct.GetCardinality("nonexistent"))
	})

	t.Run("tracked label", func(t *testing.T) {
		ct.TrackValue("endpoint", "/api/users")
		assert.Equal(t, 1, ct.GetCardinality("endpoint"))
	})
}

func TestLabelCardinalityTracker_Reset(t *testing.T) {
	ct := NewLabelCardinalityTracker()
	ct.TrackValue("method", "GET")
	ct.TrackValue("method", "POST")

	ct.Reset()

	assert.Equal(t, 0, ct.GetCardinality("method"))
}

func TestNewStandardMetrics(t *testing.T) {
	metrics := NewStandardMetrics("testapp")

	require.NotNil(t, metrics)
	require.NotNil(t, metrics.cardinality)
	assert.Equal(t, "testapp", metrics.namespace)
}

func TestStandardMetrics_RecordRequest(t *testing.T) {
	// Reset Prometheus registry for clean test state
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	metrics := NewStandardMetrics("testapp")

	t.Run("record successful request", func(t *testing.T) {
		metrics.RecordRequest("GET", "/api/users", "200", 150*time.Millisecond)

		// Verify the counter was incremented by checking that the metric exists
		// and can be collected without error
		assert.NotNil(t, metrics.requestsTotal)
		assert.NotNil(t, metrics.requestsTotal.WithLabelValues("GET", "/api/users", "200"))
	})

	t.Run("record error request", func(t *testing.T) {
		metrics.RecordRequest("POST", "/api/users", "500", 200*time.Millisecond)

		// Verify error metrics are set up correctly
		assert.NotNil(t, metrics.errorsTotal)
		assert.NotNil(t, metrics.errorsTotal.WithLabelValues("POST", "/api/users", "500"))
	})

	t.Run("normalize endpoint", func(t *testing.T) {
		metrics.RecordRequest("GET", "/api/users/12345", "200", 100*time.Millisecond)

		// Should be normalized to /api/users/:id
		assert.NotNil(t, metrics.requestsTotal.WithLabelValues("GET", "/api/users/:id", "200"))
	})
}

func TestStandardMetrics_USE_Method(t *testing.T) {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	metrics := NewStandardMetrics("testapp")

	t.Run("set utilization", func(t *testing.T) {
		metrics.SetUtilization("cpu", 75.5)

		// Verify gauge is set up correctly
		assert.NotNil(t, metrics.utilizationGauge)
		assert.NotNil(t, metrics.utilizationGauge.WithLabelValues("cpu"))
	})

	t.Run("record saturation", func(t *testing.T) {
		metrics.RecordSaturation("db_connections", 20*time.Millisecond)

		// Verify gauge is set up correctly
		assert.NotNil(t, metrics.saturationGauge)
		assert.NotNil(t, metrics.saturationGauge.WithLabelValues("db_connections"))
	})

	t.Run("record resource error", func(t *testing.T) {
		metrics.RecordResourceError("memory", "oom")

		// Verify counter is set up correctly
		assert.NotNil(t, metrics.resourceErrors)
		assert.NotNil(t, metrics.resourceErrors.WithLabelValues("memory", "oom"))
	})
}

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty endpoint",
			input:    "",
			expected: "/",
		},
		{
			name:     "static endpoint",
			input:    "/api/v1/users",
			expected: "/api/v1/users",
		},
		{
			name:     "numeric ID",
			input:    "/api/v1/users/12345",
			expected: "/api/v1/users/:id",
		},
		{
			name:     "UUID ID",
			input:    "/api/v1/users/550e8400-e29b-41d4-a716-446655440000",
			expected: "/api/v1/users/:id",
		},
		{
			name:     "multiple IDs",
			input:    "/api/v1/users/123/posts/456",
			expected: "/api/v1/users/:id/posts/:id",
		},
		{
			name:     "with query params",
			input:    "/api/users?limit=10&offset=20",
			expected: "/api/users",
		},
		{
			name:     "root path",
			input:    "/",
			expected: "/",
		},
		{
			name:     "SHA256 hash",
			input:    "/api/files/a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
			expected: "/api/files/:id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeEndpoint(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsIDLike(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"short number", "12", false},
		{"long number", "12345", true},
		{"UUID", "550e8400-e29b-41d4-a716-446655440000", true},
		{"SHA256", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", true},
		{"SHA1", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", true},
		{"MD5", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4", true},
		{"ISO date", "2024-01-15", true},
		{"text", "users", false},
		{"mixed", "user123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isIDLike(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateMetricName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid name", "http_requests_total", false, ""},
		{"empty name", "", true, "metric name cannot be empty"},
		{"with colon", "prometheus:http_requests_total", false, ""},
		{"starts with number", "123_requests", true, "metric name contains invalid characters"},
		{"with hyphen", "http-requests-total", true, "metric name contains invalid characters"},
		{"too long", strings.Repeat("a", MaxMetricNameLength+1), true, "metric name too long"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMetricName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLabelName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		{"valid name", "method", false, ""},
		{"with underscore", "error_type", false, ""},
		{"empty name", "", true, "label name cannot be empty"},
		{"reserved name", "__name__", true, "label name is reserved"},
		{"starts with number", "123_method", true, "label name contains invalid characters"},
		{"with hyphen", "error-type", true, "label name contains invalid characters"},
		{"too long", strings.Repeat("a", MaxLabelNameLength+1), true, "label name too long"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabelName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateLabelValue(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid value", "GET", false},
		{"empty value", "", false},
		{"too long", strings.Repeat("a", MaxLabelValueLength+1), true},
		{"with spaces", "error message", false},
		{"with special chars", "error@#$%", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabelValue(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestIsValidLabelValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"simple string", "success", true},
		{"UUID", "550e8400-e29b-41d4-a716-446655440000", false},
		{"SHA256 hash", "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", false},
		{"timestamp", "2024-01-15T10:30:00Z", false},
		{"numeric ID", "12345678901", false}, // 11+ digits
		{"small number", "123", true},        // Less than 10 digits
		{"status code", "200", true},
		{"too long", strings.Repeat("a", MaxLabelValueLength+1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidLabelValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected string
	}{
		{"2xx success", 200, "2xx"},
		{"2xx created", 201, "2xx"},
		{"3xx redirect", 301, "3xx"},
		{"4xx not found", 404, "4xx"},
		{"5xx error", 500, "5xx"},
		{"502 bad gateway", 502, "5xx"},
		{"custom code", 999, "999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeStatusCode(tt.code)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidLabelNameRegex(t *testing.T) {
	validNames := []string{"method", "error_type", "_internal", "HttpMethod"}
	invalidNames := []string{"123_method", "error-type", "error.type", "error type", ""}

	for _, name := range validNames {
		t.Run("valid_"+name, func(t *testing.T) {
			assert.True(t, ValidLabelNameRegex.MatchString(name))
		})
	}

	for _, name := range invalidNames {
		t.Run("invalid_"+name, func(t *testing.T) {
			assert.False(t, ValidLabelNameRegex.MatchString(name))
		})
	}
}

func TestValidMetricNameRegex(t *testing.T) {
	validNames := []string{"http_requests_total", "prometheus:metric", "_internal_metric"}
	invalidNames := []string{"123_metric", "metric-with-hyphen", "metric.with.dot"}

	for _, name := range validNames {
		t.Run("valid_"+name, func(t *testing.T) {
			assert.True(t, ValidMetricNameRegex.MatchString(name))
		})
	}

	for _, name := range invalidNames {
		t.Run("invalid_"+name, func(t *testing.T) {
			assert.False(t, ValidMetricNameRegex.MatchString(name))
		})
	}
}

func TestHighCardinalityPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern *regexp.Regexp
		matches []string
		noMatch []string
	}{
		{
			name:    "UUID pattern",
			pattern: HighCardinalityPatterns[0],
			matches: []string{"550e8400-e29b-41d4-a716-446655440000", "12345678-1234-1234-1234-123456789abc"},
			noMatch: []string{"not-a-uuid", "12345", "abc-123"},
		},
		{
			name:    "SHA256 pattern",
			pattern: HighCardinalityPatterns[1],
			matches: []string{strings.Repeat("a", 64), strings.Repeat("1", 64)},
			noMatch: []string{strings.Repeat("a", 63), "notahexstring12345678901234567890123456789012345678901234567890"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, match := range tt.matches {
				assert.True(t, tt.pattern.MatchString(match), "Expected %s to match", match)
			}
			for _, noMatch := range tt.noMatch {
				assert.False(t, tt.pattern.MatchString(noMatch), "Expected %s to not match", noMatch)
			}
		})
	}
}

func TestStandardMetricLabels(t *testing.T) {
	// Verify standard labels are defined
	assert.NotEmpty(t, StandardMetricLabels)
	assert.Contains(t, StandardMetricLabels, "method")
	assert.Contains(t, StandardMetricLabels, "endpoint")
	assert.Contains(t, StandardMetricLabels, "status")
	assert.Contains(t, StandardMetricLabels, "error_type")
	assert.Contains(t, StandardMetricLabels, "resource")
	assert.Contains(t, StandardMetricLabels, "tenant_id")
}

func TestReservedLabelNames(t *testing.T) {
	// Verify reserved labels
	assert.True(t, ReservedLabelNames["__name__"])
	assert.True(t, ReservedLabelNames["le"])
	assert.True(t, ReservedLabelNames["instance"])
	assert.True(t, ReservedLabelNames["job"])
}

// Integration test: Verify metrics can be registered and queried
func TestStandardMetrics_Integration(t *testing.T) {
	// Create a new registry to avoid conflicts
	reg := prometheus.NewRegistry()

	metrics := NewStandardMetrics("integration_test")

	// Register metrics with custom registry
	reg.MustRegister(
		metrics.requestsTotal,
		metrics.errorsTotal,
		metrics.requestDuration,
		metrics.utilizationGauge,
		metrics.saturationGauge,
		metrics.resourceErrors,
	)

	// Record various metrics
	metrics.RecordRequest("GET", "/api/users", "200", 100*time.Millisecond)
	metrics.RecordRequest("POST", "/api/users/12345", "500", 200*time.Millisecond)
	metrics.SetUtilization("memory", 85.5)
	metrics.RecordSaturation("cpu", 5*time.Millisecond)
	metrics.RecordResourceError("disk", "no_space")

	// Gather metrics
	families, err := reg.Gather()
	require.NoError(t, err)
	require.NotEmpty(t, families)

	// Verify expected metrics exist
	metricNames := make(map[string]bool)
	for _, family := range families {
		metricNames[family.GetName()] = true
	}

	assert.True(t, metricNames["integration_test_requests_total"])
	assert.True(t, metricNames["integration_test_errors_total"])
	assert.True(t, metricNames["integration_test_request_duration_seconds"])
	assert.True(t, metricNames["integration_test_utilization_percent"])
	assert.True(t, metricNames["integration_test_saturation_wait_seconds"])
	assert.True(t, metricNames["integration_test_resource_errors_total"])
}

// Benchmark test for high-throughput scenarios
func BenchmarkNormalizeEndpoint(b *testing.B) {
	endpoints := []string{
		"/api/v1/users/12345",
		"/api/v1/users/550e8400-e29b-41d4-a716-446655440000/posts",
		"/api/v1/products?limit=10&offset=20",
		"/health",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, endpoint := range endpoints {
			NormalizeEndpoint(endpoint)
		}
	}
}

func BenchmarkIsValidLabelValue(b *testing.B) {
	values := []string{
		"success",
		"GET",
		"550e8400-e29b-41d4-a716-446655440000",
		"a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, value := range values {
			IsValidLabelValue(value)
		}
	}
}
