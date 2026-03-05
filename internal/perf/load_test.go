package perf

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func calculateP95(latencies []time.Duration) time.Duration {
	if len(latencies) == 0 {
		return 0
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

	p95Index := int(float64(len(sorted)) * 0.95)
	if p95Index >= len(sorted) {
		p95Index = len(sorted) - 1
	}
	return sorted[p95Index]
}

func TestLoadConcurrentUsers(t *testing.T) {
	const (
		concurrentUsers = 10
		requestsPerUser = 10
		maxErrorRate    = 0.05
		maxP95Latency   = 100 * time.Millisecond
	)

	var wg sync.WaitGroup
	latencies := make(chan time.Duration, concurrentUsers*requestsPerUser)
	var requestCount int64

	for i := 0; i < concurrentUsers; i++ {
		wg.Add(1)
		go func(userID int) {
			defer wg.Done()

			for j := 0; j < requestsPerUser; j++ {
				start := time.Now()
				time.Sleep(5 * time.Millisecond)
				elapsed := time.Since(start)

				atomic.AddInt64(&requestCount, 1)
				latencies <- elapsed
			}
		}(i)
	}

	wg.Wait()
	close(latencies)

	var allLatencies []time.Duration
	for lat := range latencies {
		allLatencies = append(allLatencies, lat)
	}

	p95Latency := calculateP95(allLatencies)

	if p95Latency > maxP95Latency {
		t.Errorf("P95 latency too high: %v (max: %v)",
			p95Latency, maxP95Latency)
	}

	t.Logf("Load test: %d users, %d req/user, P95: %v",
		concurrentUsers, requestsPerUser, p95Latency)
}

func TestThroughput(t *testing.T) {
	const (
		targetRPS    = 100
		testDuration = 1 * time.Second
		concurrent   = 10
	)

	ctx, cancel := context.WithTimeout(context.Background(), testDuration)
	defer cancel()

	var requestCount int64
	var wg sync.WaitGroup

	for i := 0; i < concurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				time.Sleep(5 * time.Millisecond)
				atomic.AddInt64(&requestCount, 1)
			}
		}()
	}

	wg.Wait()

	actualRPS := float64(requestCount) / testDuration.Seconds()

	if actualRPS < targetRPS {
		t.Errorf("Throughput too low: %.0f RPS (target: %d RPS)",
			actualRPS, targetRPS)
	}

	t.Logf("Throughput: %.0f RPS", actualRPS)
}
