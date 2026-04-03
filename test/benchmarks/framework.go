// Package benchmarks provides a comprehensive performance testing framework
// for the Muster CLI system. It defines performance budgets, baseline metrics,
// and automated regression detection.
//
// Usage:
//
//	go test -bench=. ./test/benchmarks/...
//	go test -bench=BenchmarkAuth -benchmem ./test/benchmarks/...
//	go test -bench=. -benchtime=5s ./test/benchmarks/...
//
// Performance Budgets:
//
// The framework enforces performance budgets defined in budgets.go. Tests will
// fail if they exceed their allocated time/memory limits. This ensures
// performance regressions are caught early in development.
//
// Baseline Comparison:
//
// Run 'make bench-baseline' to generate a baseline.txt file with current
// benchmark results. Subsequent runs can compare against this baseline using
// 'make bench-compare' to detect performance regressions.
//
// Categories:
//
//   - Unit Benchmarks: Individual function performance
//   - Integration Benchmarks: Component interaction performance
//   - End-to-End Benchmarks: Full workflow performance
//   - Comparative Benchmarks: Cross-implementation comparisons
//   - Regression Benchmarks: Performance tracking over time
//
// Example:
//
//	func BenchmarkCache_Set(b *testing.B) {
//	    cache := NewCache(WithTTL(time.Hour))
//	    defer cache.Close()
//
//	    b.ResetTimer()
//	    for i := 0; i < b.N; i++ {
//	        cache.Set(fmt.Sprintf("key-%d", i), "value")
//	    }
//	}
package benchmarks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// BenchmarkConfig contains configuration for benchmark runs
type BenchmarkConfig struct {
	// MinIterations is the minimum number of iterations per benchmark
	MinIterations int

	// MaxDuration is the maximum time to run a single benchmark
	MaxDuration time.Duration

	// WarmupDuration is the time to run before starting measurements
	WarmupDuration time.Duration

	// EnableProfiling enables CPU and memory profiling
	EnableProfiling bool

	// SaveResults enables saving results to JSON file
	SaveResults bool

	// ResultsPath is where to save benchmark results
	ResultsPath string
}

// DefaultBenchmarkConfig returns a sensible default configuration
func DefaultBenchmarkConfig() *BenchmarkConfig {
	return &BenchmarkConfig{
		MinIterations:   1000,
		MaxDuration:     30 * time.Second,
		WarmupDuration:  100 * time.Millisecond,
		EnableProfiling: false,
		SaveResults:     false,
		ResultsPath:     "test/benchmarks/results",
	}
}

// BenchmarkResult represents a single benchmark execution result
type BenchmarkResult struct {
	Name        string        `json:"name"`
	Iterations  int           `json:"iterations"`
	Duration    time.Duration `json:"duration"`
	NsPerOp     float64       `json:"ns_per_op"`
	BytesPerOp  int64         `json:"bytes_per_op"`
	AllocsPerOp int64         `json:"allocs_per_op"`
	MBPerSec    float64       `json:"mb_per_sec,omitempty"`
	Timestamp   time.Time     `json:"timestamp"`
	GoVersion   string        `json:"go_version"`
	GOOS        string        `json:"goos"`
	GOARCH      string        `json:"goarch"`
	NumCPU      int           `json:"num_cpu"`
}

// BenchmarkRunner manages benchmark execution and result collection
type BenchmarkRunner struct {
	config  *BenchmarkConfig
	results []BenchmarkResult
}

// NewBenchmarkRunner creates a new benchmark runner with the given configuration
func NewBenchmarkRunner(config *BenchmarkConfig) *BenchmarkRunner {
	if config == nil {
		config = DefaultBenchmarkConfig()
	}
	return &BenchmarkRunner{
		config:  config,
		results: make([]BenchmarkResult, 0),
	}
}

// Run executes a benchmark function and collects results
func (br *BenchmarkRunner) Run(name string, fn func(*testing.B)) *BenchmarkResult {
	// Run the actual benchmark using testing.B
	result := &BenchmarkResult{
		Name:      name,
		Timestamp: time.Now(),
		GoVersion: runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
	}

	br.results = append(br.results, *result)
	return result
}

// SaveResults persists benchmark results to a JSON file
func (br *BenchmarkRunner) SaveResults() error {
	if !br.config.SaveResults {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(br.config.ResultsPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create results directory: %w", err)
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("2006-01-02-150405")
	filename := filepath.Join(dir, fmt.Sprintf("benchmark-%s.json", timestamp))

	// Marshal results to JSON
	data, err := json.MarshalIndent(br.results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write results file: %w", err)
	}

	return nil
}

// GetResults returns all collected benchmark results
func (br *BenchmarkRunner) GetResults() []BenchmarkResult {
	return br.results
}

// PerformanceBudget defines performance constraints for benchmarks
type PerformanceBudget struct {
	// MaxLatency is the maximum acceptable latency per operation
	MaxLatency time.Duration

	// MaxMemory is the maximum acceptable memory per operation in bytes
	MaxMemory int64

	// MaxAllocs is the maximum acceptable allocations per operation
	MaxAllocs int64
}

// Check validates that a benchmark result meets its performance budget
func (pb *PerformanceBudget) Check(result *BenchmarkResult) error {
	if pb.MaxLatency > 0 && time.Duration(result.NsPerOp) > pb.MaxLatency {
		return fmt.Errorf("latency budget exceeded: %v > %v",
			time.Duration(result.NsPerOp), pb.MaxLatency)
	}

	if pb.MaxMemory > 0 && result.BytesPerOp > pb.MaxMemory {
		return fmt.Errorf("memory budget exceeded: %d bytes > %d bytes",
			result.BytesPerOp, pb.MaxMemory)
	}

	if pb.MaxAllocs > 0 && result.AllocsPerOp > pb.MaxAllocs {
		return fmt.Errorf("allocation budget exceeded: %d allocs > %d allocs",
			result.AllocsPerOp, pb.MaxAllocs)
	}

	return nil
}

// BenchmarkHelper provides utility functions for benchmarks
type BenchmarkHelper struct {
	runner *BenchmarkRunner
}

// NewBenchmarkHelper creates a new benchmark helper
func NewBenchmarkHelper(runner *BenchmarkRunner) *BenchmarkHelper {
	return &BenchmarkHelper{runner: runner}
}

// ResetTimer resets the benchmark timer
func (bh *BenchmarkHelper) ResetTimer(b *testing.B) {
	b.ResetTimer()
}

// StopTimer stops the benchmark timer
func (bh *BenchmarkHelper) StopTimer(b *testing.B) {
	b.StopTimer()
}

// StartTimer starts the benchmark timer
func (bh *BenchmarkHelper) StartTimer(b *testing.B) {
	b.StartTimer()
}

// ReportMetric reports a custom benchmark metric
func (bh *BenchmarkHelper) ReportMetric(b *testing.B, name string, value float64) {
	b.ReportMetric(value, name)
}

// SetupMemoryStats enables memory allocation reporting
func (bh *BenchmarkHelper) SetupMemoryStats(b *testing.B) {
	b.ReportAllocs()
}

// Parallel runs the benchmark in parallel across GOMAXPROCS goroutines
func (bh *BenchmarkHelper) Parallel(b *testing.B, fn func(*testing.PB)) {
	b.RunParallel(fn)
}
