package wasm

import (
	"testing"
	"time"
)

// FuzzSandboxLimits verifies that Validate() never panics and that
// any error it returns is consistent (deterministic per input).
func FuzzSandboxLimits(f *testing.F) {
	f.Add(uint64(1), int64(1), uint64(1), int(1), int(1), uint64(1))
	f.Add(uint64(0), int64(0), uint64(0), int(0), int(0), uint64(0))
	f.Add(uint64(128*1024*1024), int64(30*time.Second), uint64(10*1024*1024), int(64), int(1), uint64(1000000))
	f.Add(uint64(18446744073709551615), int64(9223372036854775807), uint64(18446744073709551615), int(2147483647), int(2147483647), uint64(18446744073709551615))

	f.Fuzz(func(t *testing.T, maxMem uint64, maxExec int64, maxFile uint64, maxFD int, maxThreads int, maxFuel uint64) {
		l := SandboxLimits{
			MaxMemory:            maxMem,
			MaxExecutionTime:     time.Duration(maxExec),
			MaxFileSize:          maxFile,
			MaxFileDescriptors:   maxFD,
			MaxConcurrentThreads: maxThreads,
			MaxFuel:              maxFuel,
		}
		err := l.Validate()
		if err != nil {
			// Error paths must be deterministic: re-validate and compare.
			err2 := l.Validate()
			if err2 == nil || err2.Error() != err.Error() {
				t.Fatalf("Validate() not deterministic: %v then %v", err, err2)
			}
		}
	})
}

// TestSandboxLimits_ValidateEdgeCases covers the boundary conditions
// the fuzzer explores: zero and negative-ish values must fail cleanly,
// and defaults must pass.
func TestSandboxLimits_ValidateEdgeCases(t *testing.T) {
	if err := DefaultLimits().Validate(); err != nil {
		t.Fatalf("defaults should validate: %v", err)
	}

	zero := &SandboxLimits{}
	if err := zero.Validate(); err == nil {
		t.Fatal("all-zero limits should fail validation")
	}

	negative := &SandboxLimits{
		MaxMemory:            1,
		MaxExecutionTime:     -1 * time.Second,
		MaxFileSize:          1,
		MaxFileDescriptors:   1,
		MaxConcurrentThreads: 1,
		MaxFuel:              1,
	}
	if err := negative.Validate(); err == nil {
		t.Fatal("negative execution time should fail validation")
	}
}
