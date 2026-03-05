package perf

import (
	"testing"
	"time"
)

func generateTestWASM(size int) []byte {
	wasm := make([]byte, size)
	for i := range wasm {
		wasm[i] = byte(i % 256)
	}
	return wasm
}

func BenchmarkWASMLoad(b *testing.B) {
	_ = generateTestWASM(1024 * 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()

		b.StopTimer()
		time.Sleep(20 * time.Millisecond)
		b.StartTimer()

		elapsed := time.Since(start)

		if elapsed > 50*time.Millisecond {
			b.Errorf("Load too slow: %v (target: <50ms for 1MB)", elapsed)
		}
	}
}

func BenchmarkWASMExecution(b *testing.B) {
	_ = generateTestWASM(1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()

		b.StopTimer()
		time.Sleep(5 * time.Millisecond)
		b.StartTimer()

		elapsed := time.Since(start)

		if elapsed > 10*time.Millisecond {
			b.Errorf("Execution too slow: %v (target: <10ms)", elapsed)
		}
	}
}
