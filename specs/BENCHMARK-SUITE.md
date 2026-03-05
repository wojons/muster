# Benchmark Suite

**Status**: ✅ COMPLETE
**Version**: 1.0.0
**Last Updated**: 2026-03-02

---

## Overview

The benchmark suite provides comprehensive performance testing infrastructure for the OpenAPI CLI. It measures execution speed, memory allocations, CPU usage, and scalability characteristics across all major components.

### Benchmark Categories

- **Unit Benchmarks**: Individual function performance
- **Integration Benchmarks**: Component interaction performance
- **End-to-End Benchmarks**: Full workflow performance
- **Comparative Benchmarks**: Cross-implementation comparisons
- **Regression Benchmarks**: Performance tracking over time

---

## Benchmark Organization

```
internal/
├── cache/
│   ├── cache.go
│   ├── cache_test.go
│   └── cache_bench_test.go      # Cache benchmarks
├── parser/
│   ├── parser.go
│   └── parser_bench_test.go     # Parser benchmarks
├── wasm/
│   ├── runtime.go
│   └── runtime_bench_test.go    # WASM runtime benchmarks
└── auth/
    ├── auth.go
    └── auth_bench_test.go       # Authentication benchmarks

pkg/
└── openapi/
    ├── validator.go
    └── validator_bench_test.go  # OpenAPI validator benchmarks

benchmarks/
├── baselines_test.go            # Performance baseline tests
├── comparison_test.go           # Comparative benchmarks
└── regression_test.go           # Regression detection
```

---

## Basic Benchmark Patterns

### Standard Benchmark Structure

```go
// cache_bench_test.go
package cache

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkCache_Set measures cache write performance
func BenchmarkCache_Set(b *testing.B) {
	cache := NewCache(WithTTL(time.Hour))
	defer cache.Close()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, "value")
	}
}

// BenchmarkCache_Get measures cache read performance
func BenchmarkCache_Get(b *testing.B) {
	cache := NewCache(WithTTL(time.Hour))
	defer cache.Close()
	
	// Pre-populate cache
	cache.Set("test-key", "test-value")
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		cache.Get("test-key")
	}
}

// BenchmarkCache_SetParallel measures concurrent write performance
func BenchmarkCache_SetParallel(b *testing.B) {
	cache := NewCache(WithTTL(time.Hour))
	defer cache.Close()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d-%d", b.Parallelism(), i)
			cache.Set(key, "value")
			i++
		}
	})
}
```

### Table-Driven Benchmarks

```go
// parser_bench_test.go
package parser

import (
	"testing"
)

// BenchmarkParser_Sizes tests parsing across different spec sizes
func BenchmarkParser_Sizes(b *testing.B) {
	sizes := []struct {
		name string
		size int // KB
	}{
		{"1KB", 1},
		{"10KB", 10},
		{"100KB", 100},
		{"1MB", 1024},
		{"10MB", 10240},
	}
	
	for _, tc := range sizes {
		b.Run(tc.name, func(b *testing.B) {
			data := generateOpenAPISpec(tc.size * 1024)
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				_, err := ParseOpenAPI(data)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkParser_Formats tests different input formats
func BenchmarkParser_Formats(b *testing.B) {
	formats := []struct {
		name     string
		generate func() []byte
	}{
		{"JSON", generateJSONSpec},
		{"YAML", generateYAMLSpec},
		{"JSON-Refs", generateJSONWithRefs},
	}
	
	for _, tc := range formats {
		b.Run(tc.name, func(b *testing.B) {
			data := tc.generate()
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				ParseOpenAPI(data)
			}
		})
	}
}
```

---

## Performance Baselines

```go
// benchmarks/baselines_test.go
package benchmarks

import (
	"testing"
	"time"
)

// Baseline constants for performance requirements
const (
	// Cache operations must complete in < 1µs
	MaxCacheLatency = 1 * time.Microsecond
	
	// JSON parsing must complete at 10MB/s minimum
	MinParseThroughput = 10 * 1024 * 1024 // 10 MB/s
	
	// WASM instantiation must complete in < 10ms
	MaxWASMInstantiationTime = 10 * time.Millisecond
	
	// HTTP request overhead must be < 5ms
	MaxHTTPOverhead = 5 * time.Millisecond
)

// BenchmarkBaseline_CacheSet verifies cache write performance
func BenchmarkBaseline_CacheSet(b *testing.B) {
	cache := NewCache()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		cache.Set("key", "value")
	}
	
	// Verify baseline
	avgLatency := b.Elapsed() / time.Duration(b.N)
	if avgLatency > MaxCacheLatency {
		b.Fatalf("Cache.Set exceeds baseline: %v > %v", avgLatency, MaxCacheLatency)
	}
	
	b.ReportMetric(float64(avgLatency.Nanoseconds()), "ns/op")
}

// BenchmarkBaseline_ParseJSON_1KB verifies JSON parsing speed
func BenchmarkBaseline_ParseJSON_1KB(b *testing.B) {
	data := generateJSONSpec(1024)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		ParseOpenAPI(data)
	}
	
	// Calculate throughput
	bytesProcessed := int64(len(data)) * int64(b.N)
	throughput := float64(bytesProcessed) / b.Elapsed().Seconds()
	
	if throughput < MinParseThroughput {
		b.Fatalf("Parse throughput below baseline: %.2f MB/s < %.2f MB/s",
			throughput/1024/1024, MinParseThroughput/1024/1024)
	}
	
	b.ReportMetric(throughput/1024/1024, "MB/s")
}

// BenchmarkBaseline_WASMInstantiate verifies WASM instantiation speed
func BenchmarkBaseline_WASMInstantiate(b *testing.B) {
	wasm := compileTestModule()
	runtime := NewWASMRuntime()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		instance := runtime.Instantiate(wasm)
		instance.Close()
	}
	
	avgLatency := b.Elapsed() / time.Duration(b.N)
	if avgLatency > MaxWASMInstantiationTime {
		b.Fatalf("WASM instantiation exceeds baseline: %v > %v",
			avgLatency, MaxWASMInstantiationTime)
	}
}

// BenchmarkBaseline_HTTPRoundTrip verifies HTTP client overhead
func BenchmarkBaseline_HTTPRoundTrip(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()
	
	client := NewHTTPClient()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		resp, _ := client.Get(server.URL)
		resp.Body.Close()
	}
	
	avgLatency := b.Elapsed() / time.Duration(b.N)
	if avgLatency > MaxHTTPOverhead {
		b.Fatalf("HTTP overhead exceeds baseline: %v > %v",
			avgLatency, MaxHTTPOverhead)
	}
}
```

---

## Memory Profiling

```go
// benchmarks/memory_test.go
package benchmarks

import (
	"runtime"
	"testing"
)

// BenchmarkCache_Memory measures memory allocations during cache operations
func BenchmarkCache_Memory(b *testing.B) {
	b.ReportAllocs()
	
	cache := NewCache(WithMaxSize(10000))
	defer cache.Close()
	
	value := make([]byte, 1024)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%10000)
		cache.Set(key, value)
	}
	
	// Report detailed allocation metrics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	b.ReportMetric(float64(m.Alloc), "total_alloc_bytes")
	b.ReportMetric(float64(b.N)/float64(m.Alloc), "ops_per_byte")
}

// BenchmarkParser_Memory measures memory usage during parsing
func BenchmarkParser_Memory(b *testing.B) {
	b.ReportAllocs()
	
	data := loadLargeSpec("10MB.yaml")
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		doc, _ := ParseOpenAPI(data)
		_ = doc
		runtime.GC() // Force GC between iterations for accurate measurement
	}
}

// BenchmarkWASM_Memory measures memory usage in WASM runtime
func BenchmarkWASM_Memory(b *testing.B) {
	b.ReportAllocs()
	
	wasm := compileTestModule()
	runtime := NewWASMRuntime()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		instance := runtime.Instantiate(wasm)
		instance.Call("process", testInput)
		instance.Close()
		
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		b.ReportMetric(float64(m.HeapAlloc), "heap_bytes")
	}
}
```

### Running with Memory Profiles

```bash
# Generate memory profile
go test -bench=BenchmarkCache_Memory -benchmem -memprofile=mem.prof ./...

# Analyze with pprof
go tool pprof mem.prof
(pprof) top
(pprof) list cache.Set
(pprof) web                    # Open in browser
(pprof) pdf > mem-profile.pdf  # Export to PDF

# Compare memory profiles
go tool pprof -base=mem-baseline.prof mem.current.prof
```

---

## CPU Profiling

```go
// benchmarks/cpu_test.go
package benchmarks

import (
	"testing"
)

// BenchmarkParser_CPU profiles CPU usage during parsing
func BenchmarkParser_CPU(b *testing.B) {
	data := loadLargeSpec("complex-spec.yaml")
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		ParseOpenAPI(data)
	}
}

// BenchmarkValidator_CPU profiles validation performance
func BenchmarkValidator_CPU(b *testing.B) {
	spec := loadParsedSpec()
	validator := NewValidator()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		validator.Validate(spec)
	}
}

// BenchmarkTransform_CPU profiles transformation performance
func BenchmarkTransform_CPU(b *testing.B) {
	spec := loadParsedSpec()
	transformer := NewTransformer()
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		transformer.Transform(spec)
	}
}
```

### Running with CPU Profiles

```bash
# Generate CPU profile
go test -bench=BenchmarkParser_CPU -cpuprofile=cpu.prof ./...

# Analyze with pprof
go tool pprof cpu.prof
(pprof) top
(pprof) top --cum
(pprof) list ParseOpenAPI
(pprof) pdf > cpu-profile.pdf

# Generate flame graph
go tool pprof -http=:8080 cpu.prof
# Open http://localhost:8080 in browser
```

---

## Comparative Benchmarks

```go
// benchmarks/comparison_test.go
package benchmarks

import (
	"testing"
)

// BenchmarkCache_Backends compares different cache implementations
func BenchmarkCache_Backends(b *testing.B) {
	backends := []struct {
		name string
		fn   func() Cache
	}{
		{"Memory", NewMemoryCache},
		{"Redis", NewRedisCache},
		{"SQLite", NewSQLiteCache},
		{"Tiered", NewTieredCache},
	}
	
	for _, bb := range backends {
		b.Run(bb.name, func(b *testing.B) {
			cache := bb.fn()
			defer cache.Close()
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				cache.Set("key", "value")
			}
		})
	}
}

// BenchmarkParser_Libraries compares OpenAPI parsers
func BenchmarkParser_Libraries(b *testing.B) {
	data := loadTestSpec()
	
	parsers := []struct {
		name string
		fn   func([]byte) (*Document, error)
	}{
		{"Kin-OpenAPI", parseWithKin},
		{"go-openapi", parseWithGoOpenAPI},
		{"libopenapi", parseWithLibOpenAPI},
		{"Our-Parser", ParseOpenAPI},
	}
	
	for _, p := range parsers {
		b.Run(p.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				p.fn(data)
			}
		})
	}
}

// BenchmarkWASM_Runtimes compares WASM runtimes
func BenchmarkWASM_Runtimes(b *testing.B) {
	wasm := compileTestModule()
	
	runtimes := []struct {
		name string
		fn   func() WASMRuntime
	}{
		{"Wazero", NewWazeroRuntime},
		{"Wasmtime", NewWasmtimeRuntime},
		{"Wasmer", NewWasmerRuntime},
	}
	
	for _, rt := range runtimes {
		b.Run(rt.name, func(b *testing.B) {
			runtime := rt.fn()
			defer runtime.Close()
			
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				instance := runtime.Instantiate(wasm)
				instance.Close()
			}
		})
	}
}

// BenchmarkHTTP_Clients compares HTTP client implementations
func BenchmarkHTTP_Clients(b *testing.B) {
	server := startTestServer()
	defer server.Close()
	
	clients := []struct {
		name   string
		client HTTPClient
	}{
		{"Standard", http.DefaultClient},
		{"Fasthttp", fasthttpClient},
		{"Custom", optimizedClient},
	}
	
	for _, c := range clients {
		b.Run(c.name, func(b *testing.B) {
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				resp, _ := c.client.Get(server.URL + "/test")
				resp.Body.Close()
			}
		})
	}
}
```

---

## Regression Detection

```go
// benchmarks/regression_test.go
package benchmarks

import (
	"encoding/json"
	"os"
	"testing"
)

// BenchmarkRegression_Save saves benchmark results for comparison
func BenchmarkRegression_Save(b *testing.B) {
	if os.Getenv("SAVE_BASELINE") == "" {
		b.Skip("Set SAVE_BASELINE=1 to save baseline")
	}
	
	// Run benchmarks and save results
	results := runAllBenchmarks()
	
	data, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile("benchmark-baseline.json", data, 0644)
}

// BenchmarkRegression_Compare compares against baseline
func BenchmarkRegression_Compare(b *testing.B) {
	baselineData, err := os.ReadFile("benchmark-baseline.json")
	if err != nil {
		b.Skip("No baseline found, run with SAVE_BASELINE=1 first")
	}
	
	var baseline map[string]BenchmarkResult
	json.Unmarshal(baselineData, &baseline)
	
	// Run current benchmarks
	current := runAllBenchmarks()
	
	// Compare and report regressions
	for name, baseResult := range baseline {
		currentResult, ok := current[name]
		if !ok {
			b.Logf("New benchmark: %s", name)
			continue
		}
		
		// Check for > 10% regression
		if currentResult.NSPerOp > baseResult.NSPerOp*1.10 {
			b.Errorf("REGRESSION: %s performance degraded by %.1f%%",
				name,
				(float64(currentResult.NSPerOp)/float64(baseResult.NSPerOp)-1)*100,
			)
		}
		
		// Check for > 20% memory increase
		if currentResult.AllocBytes > baseResult.AllocBytes*1.20 {
			b.Errorf("REGRESSION: %s allocations increased by %.1f%%",
				name,
				(float64(currentResult.AllocBytes)/float64(baseResult.AllocBytes)-1)*100,
			)
		}
	}
}

type BenchmarkResult struct {
	Name       string  `json:"name"`
	NSPerOp    int64   `json:"ns_per_op"`
	AllocBytes int64   `json:"alloc_bytes"`
	AllocsPerOp int64  `json:"allocs_per_op"`
}

func runAllBenchmarks() map[string]BenchmarkResult {
	// Implementation runs benchmarks and collects results
	return make(map[string]BenchmarkResult)
}
```

---

## Scalability Benchmarks

```go
// benchmarks/scale_test.go
package benchmarks

import (
	"testing"
)

// BenchmarkCache_Concurrency tests cache under concurrent load
func BenchmarkCache_Concurrency(b *testing.B) {
	concurrencyLevels := []int{1, 10, 100, 1000}
	
	for _, level := range concurrencyLevels {
		b.Run(fmt.Sprintf("goroutines-%d", level), func(b *testing.B) {
			cache := NewCache()
			defer cache.Close()
			
			// Pre-populate
			for i := 0; i < 10000; i++ {
				cache.Set(fmt.Sprintf("key-%d", i), "value")
			}
			
			b.ResetTimer()
			
			var wg sync.WaitGroup
			opsPerGoroutine := b.N / level
			
			for i := 0; i < level; i++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()
					
					for j := 0; j < opsPerGoroutine; j++ {
						key := fmt.Sprintf("key-%d", (id*opsPerGoroutine+j)%10000)
						cache.Get(key)
					}
				}(i)
			}
			
			wg.Wait()
			
			// Report throughput
			throughput := float64(b.N) / b.Elapsed().Seconds()
			b.ReportMetric(throughput, "ops/sec")
		})
	}
}

// BenchmarkParser_LargeSpecs tests parser with large documents
func BenchmarkParser_LargeSpecs(b *testing.B) {
	sizes := []int{1, 10, 100, 1000} // MB
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dMB", size), func(b *testing.B) {
			data := generateLargeSpec(size * 1024 * 1024)
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				ParseOpenAPI(data)
			}
		})
	}
}

// BenchmarkWASM_ParallelInstances tests WASM runtime scalability
func BenchmarkWASM_ParallelInstances(b *testing.B) {
	wasm := compileTestModule()
	runtime := NewWASMRuntime()
	defer runtime.Close()
	
	parallelism := []int{1, 10, 50, 100}
	
	for _, p := range parallelism {
		b.Run(fmt.Sprintf("instances-%d", p), func(b *testing.B) {
			b.ResetTimer()
			
			var wg sync.WaitGroup
			opsPerInstance := b.N / p
			
			for i := 0; i < p; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					
					for j := 0; j < opsPerInstance; j++ {
						instance := runtime.Instantiate(wasm)
						instance.Call("process", testInput)
						instance.Close()
					}
				}()
			}
			
			wg.Wait()
		})
	}
}
```

---

## Makefile Targets

```makefile
# Makefile benchmarks section
.PHONY: bench bench-mem bench-cpu bench-compare bench-ci

# Run all benchmarks
bench:
	go test -bench=. -benchmem ./...

# Run benchmarks with memory profiling
bench-mem:
	go test -bench=. -benchmem -memprofile=mem.prof ./...
	go tool pprof -http=:8080 mem.prof

# Run benchmarks with CPU profiling
bench-cpu:
	go test -bench=. -cpuprofile=cpu.prof ./...
	go tool pprof -http=:8080 cpu.prof

# Compare benchmarks against baseline
bench-compare:
	go test -bench=. -count=5 > bench.current.txt
	benchstat bench.baseline.txt bench.current.txt

# Save new baseline
bench-baseline:
	go test -bench=. -count=10 > bench.baseline.txt

# CI-optimized benchmark run
bench-ci:
	go test -bench=. -benchtime=100ms -count=3 ./... 2>&1 | tee bench-results.txt
	# Check for regressions
	benchstat bench.baseline.txt bench-results.txt | grep -E "(regression|slower|faster)" || true

# Run specific benchmark
bench-cache:
	go test -bench=BenchmarkCache -benchmem -memprofile=cache-mem.prof ./internal/cache/...

# Run parser benchmarks
bench-parser:
	go test -bench=BenchmarkParser -benchmem -cpuprofile=parser-cpu.prof ./internal/parser/...

# Run WASM benchmarks
bench-wasm:
	go test -bench=BenchmarkWASM -benchmem ./internal/wasm/...

# Full benchmark suite with all profiles
bench-full: clean-bench
	mkdir -p bench-results
	go test -bench=. -benchmem -memprofile=bench-results/mem.prof \
		-cpuprofile=bench-results/cpu.prof -blockprofile=bench-results/block.prof \
		-mutexprofile=bench-results/mutex.prof -trace=bench-results/trace.out ./...
	@echo "Results saved to bench-results/"

clean-bench:
	rm -rf bench-results/
	mkdir -p bench-results
```

---

## Benchmark Results Format

```
# Example benchmark output
BenchmarkCache_Set-8              5000000    285 ns/op    64 B/op    2 allocs/op
BenchmarkCache_Get-8             10000000    117 ns/op    16 B/op    1 allocs/op
BenchmarkCache_SetParallel-8     20000000    152 ns/op    64 B/op    2 allocs/op

BenchmarkParser_JSON_1KB-8          50000   2456 ns/op  2048 B/op   15 allocs/op
BenchmarkParser_JSON_1MB-8            500  2456789 ns/op  2048000 B/op   15000 allocs/op

BenchmarkWASM_Instantiate-8        100000   9856 ns/op   4096 B/op    8 allocs/op
BenchmarkWASM_Call-8              2000000    525 ns/op     32 B/op    1 allocs/op
```

### Interpretation Guide

| Metric | Description | Target |
|--------|-------------|--------|
| ns/op | Nanoseconds per operation | < 1000 for simple ops |
| B/op | Bytes allocated per operation | < 1024 for hot paths |
| allocs/op | Heap allocations per operation | < 5 for hot paths |
| MB/s | Throughput for data processing | > 10 MB/s |
| ops/sec | Operations per second (concurrent) | Scale linearly with cores |

---

## CI Integration

```yaml
# .github/workflows/benchmark.yml
name: Performance Benchmarks

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  benchmark:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v3
        with:
          go-version: '1.21'
      
      - name: Run benchmarks
        run: make bench-ci
      
      - name: Compare with baseline
        run: |
          go install golang.org/x/perf/cmd/benchstat@latest
          benchstat bench.baseline.txt bench-results.txt | tee comparison.txt
      
      - name: Check regressions
        run: |
          if grep -q "regression" comparison.txt; then
            echo "::error::Performance regression detected!"
            exit 1
          fi
      
      - name: Upload results
        uses: actions/upload-artifact@v3
        with:
          name: benchmark-results
          path: |
            bench-results.txt
            comparison.txt
```

---

## Bible Standard Compliance

This specification adheres to the Bible Standard Specification format.

For more information, see [BIBLE-STANDARD.md](../BIBLE-STANDARD.md)
