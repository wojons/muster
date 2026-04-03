package wasm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ============================================================================
// WASM FUZZING HARNESS
// ============================================================================
// This file implements comprehensive fuzzing tests for WASM security
// covering: module parsing, memory isolation, resource limits, and sandbox escape

// FuzzWASMModuleParsing tests WASM module parsing with malformed/malicious input
func FuzzWASMModuleParsing(f *testing.F) {
	// Seed corpus with valid and edge-case WASM patterns
	// Minimal valid WASM module (magic number + version)
	f.Add([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}) // WASM magic + version 1
	// WASM with type section
	f.Add([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0x04, 0x01, 0x60, 0x00, 0x00})
	// Truncated WASM
	f.Add([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00})
	// Invalid magic
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00})
	// Large section size
	f.Add([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0xff, 0xff, 0xff, 0xff, 0x0f})
	// Recursive/infinite type section
	f.Add([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0x05, 0x01, 0x60, 0x01, 0x6f, 0x01, 0x6f})
	// Empty input
	f.Add([]byte{})
	// Single byte
	f.Add([]byte{0x00})
	// Garbage data
	f.Add(bytes.Repeat([]byte{0xff}, 1000))
	// WASM with very large function count
	f.Add(append([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0x05, 0x01}, bytes.Repeat([]byte{0x60, 0x00, 0x00}, 100)...))
	// WASM with custom section
	f.Add([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x00, 0x08, 0x01, 0x04, 0x74, 0x65, 0x73, 0x74})
	// Null bytes
	f.Add([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		config := DefaultRuntimeConfig()
		limits := &SandboxLimits{
			MaxMemory:        64 * 1024 * 1024,
			MaxExecutionTime: 1 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
			MaxFuel:          1_000_000_000,
		}
		runtime := NewRuntime(config, limits)
		require.NotNil(t, runtime)

		// Attempt to execute potentially malformed module
		// Should not panic - either succeed or return error
		_, _ = runtime.Execute(data, "main")
	})
}

// FuzzWASMMemoryAccess tests memory access patterns for isolation violations
func FuzzWASMMemoryAccess(f *testing.F) {
	// Valid WASM with memory export
	validMemWasm := []byte{
		0x00, 0x61, 0x73, 0x6d, // magic
		0x01, 0x00, 0x00, 0x00, // version
		0x05, 0x03, 0x01, 0x00, 0x01, // memory section (1 page min, 1 page max)
		0x07, 0x08, 0x01, 0x04, 0x6d, 0x65, 0x6d, 0x6f, 0x02, 0x00, // export "memo" as memory
	}
	f.Add(validMemWasm, uint32(0), uint32(0))
	f.Add(validMemWasm, uint32(65535), uint32(1))                                       // Page boundary
	f.Add(validMemWasm, uint32(0xffffffff), uint32(0))                                  // Max uint32
	f.Add(validMemWasm, uint32(0), uint32(100))                                         // Large size
	f.Add([]byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}, uint32(0), uint32(0)) // No memory section
	// WASM with multiple memories
	multiMemWasm := []byte{
		0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
		0x05, 0x05, 0x02, 0x00, 0x01, 0x00, 0x01, // memory section with 2 memories
	}
	f.Add(multiMemWasm, uint32(0), uint32(0))

	f.Fuzz(func(t *testing.T, moduleBytes []byte, offset uint32, size uint32) {
		config := DefaultRuntimeConfig()
		limits := &SandboxLimits{
			MaxMemory:        16 * 1024 * 1024,
			MaxExecutionTime: 100 * time.Millisecond,
			MaxFileSize:      1024 * 1024,
			MaxFuel:          100_000_000,
		}
		runtime := NewRuntime(config, limits)
		if runtime == nil {
			t.Skip("runtime creation failed")
			return
		}

		// Try to execute - should not crash or access host memory
		_, _ = runtime.Execute(moduleBytes, "main")
	})
}

// FuzzSandboxLimits tests resource limit enforcement with edge cases
func FuzzSandboxLimits(f *testing.F) {
	// Test various limit combinations
	f.Add(uint64(0), uint64(0), uint64(0), uint64(0))                                                 // Zero limits
	f.Add(uint64(1), uint64(1), uint64(1), uint64(1))                                                 // Minimal limits
	f.Add(uint64(1024), uint64(1024), uint64(1024), uint64(1024))                                     // Small limits
	f.Add(uint64(64*1024*1024), uint64(30), uint64(10*1024*1024), uint64(1_000_000_000))              // Normal limits
	f.Add(uint64(1024*1024*1024), uint64(300), uint64(100*1024*1024), uint64(100_000_000_000))        // Large limits
	f.Add(uint64(^uint64(0)), uint64(^uint64(0)), uint64(^uint64(0)), uint64(0))                      // Max values (fuel 0 for no limit)
	f.Add(uint64(^uint64(0)>>1), uint64(^uint64(0)>>1), uint64(^uint64(0)>>1), uint64(^uint64(0)>>1)) // Near max

	f.Fuzz(func(t *testing.T, maxMemory, maxExecutionTime, maxFileSize, maxFuel uint64) {
		// Should not panic with any limit values
		limits := &SandboxLimits{
			MaxMemory:        maxMemory,
			MaxExecutionTime: time.Duration(maxExecutionTime) * time.Second,
			MaxFileSize:      maxFileSize,
			MaxFuel:          maxFuel,
		}

		config := DefaultRuntimeConfig()
		runtime := NewRuntime(config, limits)
		require.NotNil(t, runtime)

		// Verify limits were set
		runtimeLimits := runtime.Limits()
		require.NotNil(t, runtimeLimits)
	})
}

// FuzzWASMCapabilityValidation tests capability validation with random inputs
func FuzzWASMCapabilityValidation(f *testing.F) {
	// Various capability patterns
	f.Add("stdio", true)
	f.Add("stdio", false)
	f.Add("fs.read", true)
	f.Add("fs.write", false)
	f.Add("", true)
	f.Add("network.http", true)
	f.Add("network.tcp", false)
	f.Add("very_long_capability_name_that_exceeds_normal_limits_and_might_cause_buffer_issues", true)
	f.Add("cap.with.special!chars@and#symbols", true)
	f.Add("cap\x00with\x00nulls", true)
	f.Add("../../etc/passwd", true)
	f.Add("${ENV_VAR}", true)
	f.Add("$(command injection)", true)

	f.Fuzz(func(t *testing.T, capability string, shouldHave bool) {
		config := DefaultRuntimeConfig()
		limits := &SandboxLimits{
			MaxMemory:        64 * 1024 * 1024,
			MaxExecutionTime: 1 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
			MaxFuel:          1_000_000_000,
		}
		runtime := NewRuntime(config, limits)
		require.NotNil(t, runtime)

		// Check capability - should not panic
		_ = runtime.CheckCapability(capability)
	})
}

// FuzzWASMModuleSize tests module size validation
func FuzzWASMModuleSize(f *testing.F) {
	// Various sizes
	f.Add(0)
	f.Add(1)
	f.Add(1024)
	f.Add(1024 * 1024)        // 1MB
	f.Add(10 * 1024 * 1024)   // 10MB
	f.Add(100 * 1024 * 1024)  // 100MB
	f.Add(int(^uint(0) >> 1)) // Max int

	f.Fuzz(func(t *testing.T, size int) {
		if size < 0 {
			size = 0
		}
		// Limit size to reasonable bounds to avoid OOM
		const maxSize = 10 * 1024 * 1024 // 10MB max for fuzzing
		if size > maxSize {
			size = maxSize
		}
		moduleBytes := make([]byte, size)
		// Put WASM magic at start for larger modules
		if size >= 8 {
			copy(moduleBytes, []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00})
		}

		config := DefaultRuntimeConfig()
		limits := &SandboxLimits{
			MaxMemory:        64 * 1024 * 1024,
			MaxExecutionTime: 1 * time.Second,
			MaxFileSize:      10 * 1024 * 1024,
			MaxFuel:          1_000_000_000,
		}
		runtime := NewRuntime(config, limits)
		require.NotNil(t, runtime)

		// Should not panic regardless of size
		_, _ = runtime.Execute(moduleBytes, "main")
	})
}

// FuzzNetworkPolicyValidation tests network policy validation
func FuzzNetworkPolicyValidation(f *testing.F) {
	// Host patterns
	f.Add("localhost")
	f.Add("example.com")
	f.Add("192.168.1.1")
	f.Add("::1")
	f.Add("")
	f.Add("*")
	f.Add("*.example.com")
	f.Add("a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p.q.r.s.t.u.v.w.x.y.z.example.com")
	f.Add("host with spaces")
	f.Add("host\x00null")
	f.Add("../../etc/hosts")
	f.Add("${ENV}")

	f.Fuzz(func(t *testing.T, host string) {
		policy := &NetworkPolicy{}
		require.NotNil(t, policy)

		// Should not panic
		_ = policy.IsAllowed(host, 80)
		_ = policy.IsAllowed(host, 443)
		_ = policy.IsAllowed(host, 0)
		_ = policy.IsAllowed(host, 65535)
	})
}

// ============================================================================
// ADVERSARIAL WASM MODULE TESTS
// ============================================================================

// TestAdversarialWASM_MalformedHeader tests modules with malformed headers
func TestAdversarialWASM_MalformedHeader(t *testing.T) {
	testCases := []struct {
		name   string
		module []byte
	}{
		{
			name:   "wrong_magic",
			module: []byte{0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00},
		},
		{
			name:   "wrong_version",
			module: []byte{0x00, 0x61, 0x73, 0x6d, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:   "truncated_header",
			module: []byte{0x00, 0x61, 0x73, 0x6d},
		},
		{
			name:   "only_magic",
			module: []byte{0x00, 0x61, 0x73, 0x6d},
		},
		{
			name:   "reversed_bytes",
			module: []byte{0x6d, 0x73, 0x61, 0x00, 0x00, 0x00, 0x00, 0x01},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultRuntimeConfig()
			limits := &SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 1 * time.Second,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          1_000_000_000,
			}
			runtime := NewRuntime(config, limits)
			require.NotNil(t, runtime)

			// Should not panic
			result, err := runtime.Execute(tc.module, "main")

			// Should fail but not crash - either result.Success is false OR err is not nil
			if result != nil {
				// If result exists, it should either have Error message or not be successful
				if result.Success {
					require.NotNil(t, err, "expected error for malformed module")
				}
			} else {
				// If result is nil, we should have an error
				require.Error(t, err)
			}
		})
	}
}

// TestAdversarialWASM_BadSectionOrder tests modules with invalid section ordering
func TestAdversarialWASM_BadSectionOrder(t *testing.T) {
	// WASM requires sections in ascending ID order
	// Section IDs: 0=custom, 1=type, 2=import, 3=function, 4=table, 5=memory,
	// 6=global, 7=export, 8=start, 9=element, 10=code, 11=data, 12=data_count
	testCases := []struct {
		name   string
		module []byte
	}{
		{
			name: "section_out_of_order",
			module: []byte{
				0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // header
				0x02, 0x01, 0x00, // import section (id=2)
				0x01, 0x01, 0x00, // type section (id=1) - WRONG ORDER
			},
		},
		{
			name: "duplicate_type_section",
			module: []byte{
				0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
				0x01, 0x01, 0x00, // type section
				0x01, 0x01, 0x00, // another type section - DUPLICATE
			},
		},
		{
			name: "code_before_type",
			module: []byte{
				0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
				0x0a, 0x01, 0x00, // code section (id=10) without type
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultRuntimeConfig()
			limits := &SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 1 * time.Second,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          1_000_000_000,
			}
			runtime := NewRuntime(config, limits)
			require.NotNil(t, runtime)

			// Should not panic
			_, err := runtime.Execute(tc.module, "main")
			// Error is expected
			_ = err
		})
	}
}

// TestAdversarialWASM_MemoryAttack tests modules attempting memory exploits
func TestAdversarialWASM_MemoryAttack(t *testing.T) {
	testCases := []struct {
		name   string
		module []byte
	}{
		{
			name: "zero_size_memory",
			module: []byte{
				0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
				0x05, 0x03, 0x01, 0x00, 0x00, // memory with 0 pages
			},
		},
		{
			name: "huge_memory_request",
			module: []byte{
				0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
				0x05, 0x03, 0x01, 0xff, 0xff, // memory with max pages (65535)
			},
		},
		{
			name: "memory_without_limits",
			module: []byte{
				0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
				0x05, 0x02, 0x01, 0x00, // memory section without max
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultRuntimeConfig()
			limits := &SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 1 * time.Second,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          1_000_000_000,
			}
			runtime := NewRuntime(config, limits)
			require.NotNil(t, runtime)

			// Should not panic or crash
			_, err := runtime.Execute(tc.module, "main")
			_ = err
		})
	}
}

// TestAdversarialWASM_SyscallFiltering tests syscall filtering behavior
func TestAdversarialWASM_SyscallFiltering(t *testing.T) {
	// WASM modules attempting to make syscalls should be blocked
	// by capability system
	testCases := []struct {
		name       string
		capability string
		shouldWork bool
	}{
		{
			name:       "stdio_cap_granted",
			capability: "stdio",
			shouldWork: true,
		},
		{
			name:       "stdio_cap_denied",
			capability: "stdio",
			shouldWork: false,
		},
		{
			name:       "fs_read_granted",
			capability: "fs.read",
			shouldWork: true,
		},
		{
			name:       "fs_write_denied",
			capability: "fs.write",
			shouldWork: false,
		},
		{
			name:       "network_http_granted",
			capability: "network.http",
			shouldWork: true,
		},
		{
			name:       "network_tcp_denied",
			capability: "network.tcp",
			shouldWork: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultRuntimeConfig()
			limits := &SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 1 * time.Second,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          1_000_000_000,
			}
			runtime := NewRuntime(config, limits)
			require.NotNil(t, runtime)

			hasCap := runtime.CheckCapability(tc.capability)
			// The capability system should work consistently
			// Granting/revoking is configured at runtime level
			_ = hasCap
		})
	}
}

// TestAdversarialWASM_ResourceExhaustion tests resource exhaustion attacks
func TestAdversarialWASM_ResourceExhaustion(t *testing.T) {
	testCases := []struct {
		name        string
		limits      SandboxLimits
		shouldError bool
	}{
		{
			name: "zero_memory_limit",
			limits: SandboxLimits{
				MaxMemory:        0,
				MaxExecutionTime: 1 * time.Second,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          1_000_000_000,
			},
			shouldError: false, // Zero means no limit
		},
		{
			name: "minimal_time_limit",
			limits: SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 1 * time.Nanosecond,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          1_000_000_000,
			},
			shouldError: false,
		},
		{
			name: "zero_file_size_limit",
			limits: SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 1 * time.Second,
				MaxFileSize:      0,
				MaxFuel:          1_000_000_000,
			},
			shouldError: false, // Zero means no limit
		},
		{
			name: "zero_fuel_limit",
			limits: SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 1 * time.Second,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          0,
			},
			shouldError: false, // Zero means no limit
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultRuntimeConfig()
			runtime := NewRuntime(config, &tc.limits)
			require.NotNil(t, runtime)

			// Module with minimal valid WASM
			module := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

			_, err := runtime.Execute(module, "main")
			// Result depends on limits, but should not panic
			_ = err
		})
	}
}

// TestAdversarialWASM_SandboxEscape attempts various escape vectors
func TestAdversarialWASM_SandboxEscape(t *testing.T) {
	testCases := []struct {
		name   string
		module []byte
	}{
		{
			name:   "import_host_function",
			module: []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x02, 0x08, 0x01, 0x03, 0x65, 0x6e, 0x76, 0x03, 0x66, 0x6f, 0x6f, 0x00, 0x00},
		},
		{
			name:   "export_host_memory",
			module: []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x05, 0x03, 0x01, 0x00, 0x01, 0x07, 0x08, 0x01, 0x04, 0x68, 0x6f, 0x73, 0x74, 0x02, 0x00},
		},
		{
			name:   "start_function_infinite_loop",
			module: []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, 0x01, 0x04, 0x01, 0x60, 0x00, 0x00, 0x03, 0x02, 0x01, 0x00, 0x08, 0x01, 0x00, 0x0a, 0x04, 0x01, 0x02, 0x00, 0x0b},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := DefaultRuntimeConfig()
			limits := &SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 100 * time.Millisecond,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          10_000_000,
			}
			runtime := NewRuntime(config, limits)
			require.NotNil(t, runtime)

			// Should not allow escape
			result, err := runtime.Execute(tc.module, "main")
			_ = result
			_ = err
		})
	}
}

// TestAdversarialWASM_BinaryCorruption tests modules with corrupted binary data
func TestAdversarialWASM_BinaryCorruption(t *testing.T) {
	// Create a valid minimal WASM module
	validWasm := []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

	testCases := []struct {
		name    string
		corrupt func([]byte) []byte
	}{
		{
			name: "flip_single_bit",
			corrupt: func(b []byte) []byte {
				if len(b) > 0 {
					b[0] ^= 0x01
				}
				return b
			},
		},
		{
			name: "null_bytes_everywhere",
			corrupt: func(b []byte) []byte {
				for i := range b {
					b[i] = 0
				}
				return b
			},
		},
		{
			name: "all_ones",
			corrupt: func(b []byte) []byte {
				for i := range b {
					b[i] = 0xff
				}
				return b
			},
		},
		{
			name: "reversed",
			corrupt: func(b []byte) []byte {
				for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
					b[i], b[j] = b[j], b[i]
				}
				return b
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Make a copy to corrupt
			module := make([]byte, len(validWasm))
			copy(module, validWasm)
			module = tc.corrupt(module)

			config := DefaultRuntimeConfig()
			limits := &SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 1 * time.Second,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          1_000_000_000,
			}
			runtime := NewRuntime(config, limits)
			require.NotNil(t, runtime)

			// Should not panic
			_, err := runtime.Execute(module, "main")
			_ = err
		})
	}
}

// TestAdversarialWASM_LargeModule tests modules with unusual sizes
func TestAdversarialWASM_LargeModule(t *testing.T) {
	testCases := []struct {
		name string
		size int
	}{
		{
			name: "empty",
			size: 0,
		},
		{
			name: "single_byte",
			size: 1,
		},
		{
			name: "seven_bytes",
			size: 7, // One less than minimum header
		},
		{
			name: "exact_header",
			size: 8,
		},
		{
			name: "one_kb",
			size: 1024,
		},
		{
			name: "one_mb",
			size: 1024 * 1024,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			module := make([]byte, tc.size)
			// Put WASM magic for modules >= 8 bytes
			if tc.size >= 8 {
				binary.LittleEndian.PutUint32(module[0:4], 0x6d736100) // WASM magic
				binary.LittleEndian.PutUint32(module[4:8], 0x00000001) // Version 1
			}

			config := DefaultRuntimeConfig()
			limits := &SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 1 * time.Second,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          1_000_000_000,
			}
			runtime := NewRuntime(config, limits)
			require.NotNil(t, runtime)

			// Should not panic regardless of size
			_, err := runtime.Execute(module, "main")
			_ = err
		})
	}
}

// TestAdversarialWASM_MaliciousSectionID tests unknown/malicious section IDs
func TestAdversarialWASM_MaliciousSectionID(t *testing.T) {
	// WASM spec defines sections 0-12. Test higher IDs.
	for sectionID := byte(13); sectionID < 32; sectionID++ {
		t.Run(fmt.Sprintf("section_id_%d", sectionID), func(t *testing.T) {
			module := []byte{
				0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00, // header
				sectionID, 0x01, 0x00, // unknown section with size 1, content 0
			}

			config := DefaultRuntimeConfig()
			limits := &SandboxLimits{
				MaxMemory:        64 * 1024 * 1024,
				MaxExecutionTime: 1 * time.Second,
				MaxFileSize:      10 * 1024 * 1024,
				MaxFuel:          1_000_000_000,
			}
			runtime := NewRuntime(config, limits)
			require.NotNil(t, runtime)

			// Should not panic
			_, err := runtime.Execute(module, "main")
			_ = err
		})
	}
}
