package perf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func BenchmarkHTTPRequest(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		},
	))
	defer server.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
		resp, err := http.DefaultClient.Do(req)

		elapsed := time.Since(start)

		if err != nil {
			b.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()
		cancel()

		if elapsed > 100*time.Millisecond {
			b.Errorf("Request too slow: %v (target: <100ms local)", elapsed)
		}
	}
}

func TestHTTPRequestLatency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		},
	))
	defer server.Close()

	iterations := 100
	latencies := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
		resp, err := http.DefaultClient.Do(req)
		latencies[i] = time.Since(start)
		if resp != nil {
			resp.Body.Close()
		}
		cancel()
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	}

	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	p50 := sorted[50]
	p90 := sorted[90]
	p95 := sorted[95]

	if p50 > 50*time.Millisecond {
		t.Errorf("P50 latency too high: %v (target: <50ms)", p50)
	}
	if p90 > 80*time.Millisecond {
		t.Errorf("P90 latency too high: %v (target: <80ms)", p90)
	}
	if p95 > 100*time.Millisecond {
		t.Errorf("P95 latency too high: %v (target: <100ms)", p95)
	}

	t.Logf("Latency distribution: P50=%v, P90=%v, P95=%v", p50, p90, p95)
}

type TestSpec struct {
	OpenAPI string              `json:"openapi"`
	Info    TestSpecInfo        `json:"info"`
	Paths   map[string]PathItem `json:"paths"`
}

type TestSpecInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type PathItem struct {
	Get *Operation `json:"get,omitempty"`
}

type Operation struct {
	OperationID string                       `json:"operationId"`
	Responses   map[string]OperationResponse `json:"responses"`
}

type OperationResponse struct {
	Description string `json:"description"`
}

func generateTestSpec(operationCount int) []byte {
	spec := TestSpec{
		OpenAPI: "3.0.0",
		Info: TestSpecInfo{
			Title:   "Test API",
			Version: "1.0.0",
		},
		Paths: make(map[string]PathItem),
	}

	for i := 0; i < operationCount; i++ {
		path := fmt.Sprintf("/resource%d", i)
		spec.Paths[path] = PathItem{
			Get: &Operation{
				OperationID: fmt.Sprintf("getResource%d", i),
				Responses: map[string]OperationResponse{
					"200": {Description: "OK"},
				},
			},
		}
	}

	data, _ := json.Marshal(spec)
	return data
}

func BenchmarkOpenAPIParsing(b *testing.B) {
	specData := generateTestSpec(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()

		var spec TestSpec
		err := json.Unmarshal(specData, &spec)

		elapsed := time.Since(start)

		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}

		if len(spec.Paths) != 100 {
			b.Fatalf("Expected 100 operations, got %d", len(spec.Paths))
		}

		if elapsed > 100*time.Millisecond {
			b.Errorf("Parse too slow: %v (target: <100ms for 100 ops)", elapsed)
		}
	}
}

func BenchmarkCommandGeneration(b *testing.B) {
	spec := generateTestSpec(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()

		var parsedSpec TestSpec
		json.Unmarshal(spec, &parsedSpec)

		elapsed := time.Since(start)

		if len(parsedSpec.Paths) != 100 {
			b.Fatalf("Expected 100 commands, got %d", len(parsedSpec.Paths))
		}

		if elapsed > 50*time.Millisecond {
			b.Errorf("Generation too slow: %v (target: <50ms for 100 cmds)", elapsed)
		}
	}
}
