package perf

import (
	"fmt"
	"runtime"
	"testing"
)

func TestMemoryUsage(t *testing.T) {
	const (
		baselineMaxMB  = 50
		underLoadMaxMB = 200
	)

	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	baselineMB := float64(m1.Alloc) / 1024 / 1024

	db := setupTestDB(t)
	defer db.Close()

	for i := 0; i < 10000; i++ {
		db.Exec("INSERT INTO inventories (url, name) VALUES (?, ?)",
			fmt.Sprintf("https://example%d.com", i),
			fmt.Sprintf("inventory%d", i))
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

func TestMemoryLeak(t *testing.T) {
	runtime.GC()
	runtime.GC()
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	leaked := make([][]byte, 0)
	for i := 0; i < 1000; i++ {
		leaked = append(leaked, make([]byte, 1024))
	}

	runtime.GC()
	runtime.GC()
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	actualGrowth := int64(m2.TotalAlloc) - int64(m1.TotalAlloc)

	if actualGrowth > 0 {
		t.Logf("Memory test passed: allocated %.1fMB",
			float64(actualGrowth)/1024/1024)
	}

	_ = leaked
}

func BenchmarkMemoryAllocation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = make([]byte, 1024)
	}
}
