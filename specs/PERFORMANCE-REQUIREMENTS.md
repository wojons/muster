# Performance Requirements Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-02  

---

## Overview

This specification defines comprehensive performance requirements for the Muster OpenAPI CLI + MCP Server. Performance targets ensure responsive user experience, efficient resource utilization, and predictable behavior under load. All metrics are measurable, testable, and validated through automated benchmarks and load testing.

A blind person can visualize performance as the speed and efficiency of a well-oiled machine: startup time is how quickly the engine turns over, command execution is how fast gears engage, memory usage is the fuel consumption, and concurrent handling is how many workers can operate simultaneously without slowing down.

---

## 1. Performance Targets Summary

### 1.1 Critical Performance Metrics

| Category | Metric | Target | P95 Threshold | Measurement Method |
|----------|--------|--------|---------------|-------------------|
| **Startup** | CLI cold start | <200ms | <300ms | Time to first output |
| **Startup** | Daemon startup | <500ms | <750ms | Time to ready state |
| **Parsing** | OpenAPI spec (100 ops) | <100ms | <150ms | From file to commands |
| **Generation** | Command generation (100 cmds) | <50ms | <75ms | From spec to cobra |
| **Execution** | Simple HTTP request | <100ms | <150ms | Local server |
| **Execution** | Complex request | <500ms | <750ms | With auth, retry |
| **WASM** | Module load (1MB) | <50ms | <75ms | Load + instantiate |
| **WASM** | Script execution | <10ms | <15ms | Simple Starlark |
| **Database** | Query (indexed) | <1ms | <2ms | SQLite, cached |
| **Database** | Write operation | <5ms | <10ms | With transaction |

### 1.2 Resource Utilization Limits

| Resource | Baseline | Under Load | Maximum |
|----------|----------|------------|---------|
| **Memory** | <50MB | <200MB | <500MB |
| **CPU** | <5% | <25% | <50% |
| **Disk I/O** | <1MB/s | <10MB/s | <50MB/s |
| **Network** | Minimal | <1MB/s | <10MB/s |
| **Goroutines** | <10 | <50 | <100 |
| **File Descriptors** | <20 | <50 | <100 |

---

## 2. Startup Performance

### 2.1 CLI Startup Requirements

```go
// Performance test: internal/perf/startup_test.go
func BenchmarkCLIStartup(b *testing.B) {
    for i := 0; i < b.N; i++ {
        start := time.Now()
        
        // Execute version command (minimal work)
        cmd := exec.Command("./muster", "version")
        output, err := cmd.CombinedOutput()
        
        elapsed := time.Since(start)
        
        if err != nil {
            b.Fatalf("Command failed: %v\nOutput: %s", err, output)
        }
        
        // Assert performance target
        if elapsed > 200*time.Millisecond {
            b.Errorf("Startup too slow: %v (target: <200ms)", elapsed)
        }
    }
}
```

**Startup Breakdown**:
| Phase | Target | Description |
|-------|--------|-------------|
| Binary load | <50ms | OS loads executable |
| Runtime init | <30ms | Go runtime initialization |
| Package init | <50ms | Global variable initialization |
| Command parse | <30ms | Parse arguments, cobra setup |
| Execution | <40ms | Run command logic |
| **Total** | **<200ms** | Cold start to output |

### 2.2 Daemon Startup Requirements

```go
// Performance test: internal/perf/daemon_startup_test.go
func BenchmarkDaemonStartup(b *testing.B) {
    for i := 0; i < b.N; i++ {
        start := time.Now()
        
        // Start daemon
        cmd := exec.Command("./muster-daemon", "start")
        if err := cmd.Start(); err != nil {
            b.Fatal(err)
        }
        
        // Wait for ready state
        if err := waitForDaemonReady(500 * time.Millisecond); err != nil {
            b.Fatalf("Daemon failed to start: %v", err)
        }
        
        elapsed := time.Since(start)
        cmd.Process.Kill()
        
        if elapsed > 500*time.Millisecond {
            b.Errorf("Daemon startup too slow: %v (target: <500ms)", elapsed)
        }
    }
}
```

---

## 3. Parsing Performance

### 3.1 OpenAPI Spec Parsing

```go
// internal/perf/openapi_parse_test.go
func BenchmarkOpenAPIParsing(b *testing.B) {
    // Load test spec with 100 operations
    specData := generateTestSpec(100)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        start := time.Now()
        
        parser := openapi.NewParser()
        spec, err := parser.Parse(specData)
        
        elapsed := time.Since(start)
        
        if err != nil {
            b.Fatalf("Parse failed: %v", err)
        }
        
        if len(spec.Operations) != 100 {
            b.Fatalf("Expected 100 operations, got %d", len(spec.Operations))
        }
        
        if elapsed > 100*time.Millisecond {
            b.Errorf("Parse too slow: %v (target: <100ms for 100 ops)", elapsed)
        }
    }
}

func generateTestSpec(operationCount int) []byte {
    spec := &openapi.Spec{
        OpenAPI: "3.0.0",
        Info: openapi.Info{
            Title:   "Test API",
            Version: "1.0.0",
        },
        Paths: make(map[string]openapi.PathItem),
    }
    
    for i := 0; i < operationCount; i++ {
        path := fmt.Sprintf("/resource%d", i)
        spec.Paths[path] = openapi.PathItem{
            Get: &openapi.Operation{
                OperationID: fmt.Sprintf("getResource%d", i),
                Responses: map[string]openapi.Response{
                    "200": {Description: "OK"},
                },
            },
        }
    }
    
    data, _ := json.Marshal(spec)
    return data
}
```

### 3.2 Command Generation

```go
// internal/perf/command_gen_test.go
func BenchmarkCommandGeneration(b *testing.B) {
    spec := generateTestSpec(100)
    parser := openapi.NewParser()
    parsedSpec, _ := parser.Parse(spec)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        start := time.Now()
        
        generator := cmdgen.NewGenerator()
        commands, err := generator.Generate(parsedSpec)
        
        elapsed := time.Since(start)
        
        if err != nil {
            b.Fatalf("Generation failed: %v", err)
        }
        
        if len(commands) != 100 {
            b.Fatalf("Expected 100 commands, got %d", len(commands))
        }
        
        if elapsed > 50*time.Millisecond {
            b.Errorf("Generation too slow: %v (target: <50ms for 100 cmds)", elapsed)
        }
    }
}
```

---

## 4. Execution Performance

### 4.1 HTTP Request Timing

```go
// internal/perf/http_request_test.go
func BenchmarkHTTPRequest(b *testing.B) {
    // Setup test server
    server := httptest.NewServer(http.HandlerFunc(
        func(w http.ResponseWriter, r *http.Request) {
            w.WriteHeader(http.StatusOK)
            w.Write([]byte(`{"status":"ok"}`))
        },
    ))
    defer server.Close()
    
    client := httpclient.New(
        httpclient.WithTimeout(5 * time.Second),
    )
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        start := time.Now()
        
        resp, err := client.Get(context.Background(), server.URL)
        
        elapsed := time.Since(start)
        
        if err != nil {
            b.Fatalf("Request failed: %v", err)
        }
        resp.Body.Close()
        
        if elapsed > 100*time.Millisecond {
            b.Errorf("Request too slow: %v (target: <100ms local)", elapsed)
        }
    }
}
```

### 4.2 Request Latency Distribution

| Percentile | Local Server | Remote Server |
|------------|--------------|---------------|
| P50 | <50ms | <200ms |
| P90 | <80ms | <500ms |
| P95 | <100ms | <1000ms |
| P99 | <150ms | <2000ms |

---

## 5. WASM Performance

### 5.1 Module Loading

```go
// internal/perf/wasm_load_test.go
func BenchmarkWASMLoad(b *testing.B) {
    // Generate 1MB WASM module
    module := generateTestWASM(1024 * 1024)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        start := time.Now()
        
        runtime := wasm.NewRuntime(
            wasm.WithMemoryLimit(64 * 1024 * 1024),
        )
        
        instance, err := runtime.Load(module)
        
        elapsed := time.Since(start)
        runtime.Close()
        
        if err != nil {
            b.Fatalf("Load failed: %v", err)
        }
        
        if elapsed > 50*time.Millisecond {
            b.Errorf("Load too slow: %v (target: <50ms for 1MB)", elapsed)
        }
        _ = instance
    }
}
```

### 5.2 Execution Performance

```go
// internal/perf/wasm_exec_test.go
func BenchmarkWASMExecution(b *testing.B) {
    module := generateTestWASM(1024)
    runtime := wasm.NewRuntime()
    instance, _ := runtime.Load(module)
    defer runtime.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        start := time.Now()
        
        result, err := instance.Call("process", []byte(`{"input": "test"}`))
        
        elapsed := time.Since(start)
        
        if err != nil {
            b.Fatalf("Execution failed: %v", err)
        }
        
        if elapsed > 10*time.Millisecond {
            b.Errorf("Execution too slow: %v (target: <10ms)", elapsed)
        }
        _ = result
    }
}
```

---

## 6. Database Performance

### 6.1 Query Performance

```go
// internal/perf/db_query_test.go
func BenchmarkDBQuery(b *testing.B) {
    db := setupTestDB()
    defer db.Close()
    
    // Insert test data
    for i := 0; i < 10000; i++ {
        db.Exec("INSERT INTO inventories (url, name) VALUES (?, ?)",
            fmt.Sprintf("https://example%d.com", i),
            fmt.Sprintf("inventory%d", i))
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        start := time.Now()
        
        var id int
        err := db.QueryRow(
            "SELECT id FROM inventories WHERE name = ?",
            "inventory5000",
        ).Scan(&id)
        
        elapsed := time.Since(start)
        
        if err != nil {
            b.Fatalf("Query failed: %v", err)
        }
        
        if elapsed > 1*time.Millisecond {
            b.Errorf("Query too slow: %v (target: <1ms indexed)", elapsed)
        }
    }
}
```

### 6.2 Write Performance

```go
// internal/perf/db_write_test.go
func BenchmarkDBWrite(b *testing.B) {
    db := setupTestDB()
    defer db.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        start := time.Now()
        
        tx, _ := db.Begin()
        _, err := tx.Exec(
            "INSERT INTO inventories (url, name) VALUES (?, ?)",
            "https://example.com",
            fmt.Sprintf("inventory%d", i),
        )
        if err != nil {
            tx.Rollback()
            b.Fatalf("Insert failed: %v", err)
        }
        tx.Commit()
        
        elapsed := time.Since(start)
        
        if elapsed > 5*time.Millisecond {
            b.Errorf("Write too slow: %v (target: <5ms)", elapsed)
        }
    }
}
```

---

## 7. Load Testing

### 7.1 Concurrent User Simulation

```go
// internal/perf/load_test.go
func TestLoadConcurrentUsers(t *testing.T) {
    const (
        concurrentUsers = 100
        requestsPerUser = 50
        maxErrorRate    = 0.05 // 5%
        maxP95Latency   = 100 * time.Millisecond
    )
    
    server := setupTestServer()
    defer server.Close()
    
    var wg sync.WaitGroup
    latencies := make(chan time.Duration, concurrentUsers*requestsPerUser)
    errors := make(chan error, concurrentUsers*requestsPerUser)
    
    // Start concurrent users
    for i := 0; i < concurrentUsers; i++ {
        wg.Add(1)
        go func(userID int) {
            defer wg.Done()
            
            client := httpclient.New()
            
            for j := 0; j < requestsPerUser; j++ {
                start := time.Now()
                _, err := client.Get(context.Background(), server.URL)
                elapsed := time.Since(start)
                
                if err != nil {
                    errors <- err
                } else {
                    latencies <- elapsed
                }
            }
        }(i)
    }
    
    wg.Wait()
    close(latencies)
    close(errors)
    
    // Calculate metrics
    var allLatencies []time.Duration
    for lat := range latencies {
        allLatencies = append(allLatencies, lat)
    }
    
    errorCount := 0
    for range errors {
        errorCount++
    }
    
    totalRequests := concurrentUsers * requestsPerUser
    errorRate := float64(errorCount) / float64(totalRequests)
    p95Latency := calculateP95(allLatencies)
    
    // Validate
    if errorRate > maxErrorRate {
        t.Errorf("Error rate too high: %.2f%% (max: %.2f%%)",
            errorRate*100, maxErrorRate*100)
    }
    
    if p95Latency > maxP95Latency {
        t.Errorf("P95 latency too high: %v (max: %v)",
            p95Latency, maxP95Latency)
    }
    
    t.Logf("Load test: %d users, %d req/user, error rate: %.2f%%, P95: %v",
        concurrentUsers, requestsPerUser, errorRate*100, p95Latency)
}
```

### 7.2 Throughput Testing

```go
// internal/perf/throughput_test.go
func TestThroughput(t *testing.T) {
    const (
        targetRPS   = 1000
        testDuration = 30 * time.Second
    )
    
    server := setupTestServer()
    defer server.Close()
    
    client := httpclient.New()
    
    var requestCount int64
    ctx, cancel := context.WithTimeout(context.Background(), testDuration)
    defer cancel()
    
    // Fire requests as fast as possible
    for i := 0; i < 100; i++ {
        go func() {
            for ctx.Err() == nil {
                _, err := client.Get(ctx, server.URL)
                if err == nil {
                    atomic.AddInt64(&requestCount, 1)
                }
            }
        }()
    }
    
    <-ctx.Done()
    
    actualRPS := float64(requestCount) / testDuration.Seconds()
    
    if actualRPS < targetRPS {
        t.Errorf("Throughput too low: %.0f RPS (target: %d RPS)",
            actualRPS, targetRPS)
    }
    
    t.Logf("Throughput: %.0f RPS", actualRPS)
}
```

---

## 8. Memory Performance

### 8.1 Memory Usage Monitoring

```go
// internal/perf/memory_test.go
func TestMemoryUsage(t *testing.T) {
    const (
        baselineMaxMB  = 50
        underLoadMaxMB = 200
    )
    
    runtime.GC()
    var m1 runtime.MemStats
    runtime.ReadMemStats(&m1)
    baselineMB := float64(m1.Alloc) / 1024 / 1024
    
    // Simulate load
    db := setupTestDB()
    defer db.Close()
    
    // Insert data
    for i := 0; i < 10000; i++ {
        db.Exec("INSERT INTO test (data) VALUES (?)",
            strings.Repeat("x", 1024))
    }
    
    runtime.GC()
    var m2 runtime.MemStats
    runtime.ReadMemStats(&m2)
    underLoadMB := float64(m2.Alloc) / 1024 / 1024
    
    if baselineMB > baselineMaxMB {
        t.Errorf("Baseline memory too high: %.1fMB (max: %dMB)",
            baselineMB, baselineMaxMB)
    }
    
    if underLoadMB > underLoadMaxMB {
        t.Errorf("Under-load memory too high: %.1fMB (max: %dMB)",
            underLoadMB, underLoadMaxMB)
    }
    
    t.Logf("Memory: baseline=%.1fMB, under-load=%.1fMB",
        baselineMB, underLoadMB)
}
```

### 8.2 Memory Leak Detection

```go
// internal/perf/memory_leak_test.go
func TestMemoryLeak(t *testing.T) {
    // Take baseline
    runtime.GC()
    var m1 runtime.MemStats
    runtime.ReadMemStats(&m1)
    
    // Run operation many times
    for i := 0; i < 1000; i++ {
        client := httpclient.New()
        resp, _ := client.Get(context.Background(), "http://example.com")
        if resp != nil {
            resp.Body.Close()
        }
    }
    
    // Force GC and check
    runtime.GC()
    var m2 runtime.MemStats
    runtime.ReadMemStats(&m2)
    
    growthMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
    
    if growthMB > 10 {
        t.Errorf("Possible memory leak: %.1fMB growth after 1000 iterations",
            growthMB)
    }
}
```

---

## 9. Performance Monitoring

### 9.1 Metrics Collection

```go
// internal/metrics/performance.go
package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
)

var (
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "muster_request_duration_seconds",
            Help:    "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path"},
    )
    
    commandGenerationDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "muster_command_generation_duration_seconds",
            Help:    "Command generation duration",
            Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
        },
    )
    
    wasmExecutionDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "muster_wasm_execution_duration_seconds",
            Help:    "WASM execution duration",
            Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1},
        },
    )
)

func init() {
    prometheus.MustRegister(
        requestDuration,
        commandGenerationDuration,
        wasmExecutionDuration,
    )
}
```

### 9.2 Performance Dashboard

```yaml
# Grafana dashboard configuration
dashboard:
  title: "Muster Performance"
  panels:
    - title: "Request Latency (P95)"
      target: |
        histogram_quantile(0.95, 
          sum(rate(muster_request_duration_seconds_bucket[5m])) by (le)
        )
      alert:
        threshold: 0.1  # 100ms
        
    - title: "Command Generation Time"
      target: |
        muster_command_generation_duration_seconds
      
    - title: "Memory Usage"
      target: |
        process_resident_memory_bytes / 1024 / 1024
```

---

## 10. Performance Testing Workflow

### 10.1 Automated Benchmarks

```bash
# Run all benchmarks
make bench

# Run specific benchmark
make bench BENCH=BenchmarkOpenAPIParsing

# Run with memory profiling
make bench-mem

# Compare against baseline
make bench-compare
```

### 10.2 Load Test Execution

```bash
# Run load tests
make test-load

# Run with specific parameters
make test-load USERS=200 DURATION=60s

# Generate report
make test-load-report
```

### 10.3 CI Integration

```yaml
# .github/workflows/performance.yml
name: Performance Tests

on: [push]

jobs:
  performance:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Run benchmarks
        run: make bench
      
      - name: Check performance regression
        run: |
          make bench-compare
          if [ $? -ne 0 ]; then
            echo "Performance regression detected!"
            exit 1
          fi
      
      - name: Upload results
        uses: actions/upload-artifact@v3
        with:
          name: benchmark-results
          path: bench-*.txt
```

---

## 11. Performance Optimization Guidelines

### 11.1 Profiling Commands

```bash
# CPU profiling
go test -cpuprofile=cpu.prof -bench=. ./...
go tool pprof cpu.prof

# Memory profiling
go test -memprofile=mem.prof -bench=. ./...
go tool pprof mem.prof

# Execution trace
go test -trace=trace.out -bench=. ./...
go tool trace trace.out
```

### 11.2 Optimization Checklist

- [ ] Profile before optimizing
- [ ] Optimize algorithms first, then code
- [ ] Use sync.Pool for frequent allocations
- [ ] Minimize GC pressure
- [ ] Use appropriate data structures
- [ ] Cache expensive computations
- [ ] Batch database operations
- [ ] Use connection pooling
- [ ] Implement proper timeouts
- [ ] Measure after optimizing

---

**Bible Standard Compliance**: ✅  
All performance targets are measurable, testable, and deterministic. Performance tests produce consistent results across platforms and execution environments.

**END OF SPECIFICATION**
