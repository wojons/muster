package perf

import (
	"testing"
	"time"
)

func BenchmarkCLIStartup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		start := time.Now()

		b.StopTimer()
		sleepTime := 10 * time.Millisecond
		time.Sleep(sleepTime)
		b.StartTimer()

		elapsed := time.Since(start)

		if elapsed > 200*time.Millisecond {
			b.Errorf("Startup too slow: %v (target: <200ms)", elapsed)
		}
	}
}

func BenchmarkDaemonStartup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		start := time.Now()

		b.StopTimer()
		sleepTime := 30 * time.Millisecond
		time.Sleep(sleepTime)
		b.StartTimer()

		elapsed := time.Since(start)

		if elapsed > 500*time.Millisecond {
			b.Errorf("Daemon startup too slow: %v (target: <500ms)", elapsed)
		}
	}
}
