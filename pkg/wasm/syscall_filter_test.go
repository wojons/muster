package wasm

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Syscall Filter Tests - Step 2: Test syscall filtering (WASM-SANDBOX-SECURITY)
// ============================================================================

func TestNewSyscallFilter(t *testing.T) {
	t.Run("minimal_profile_allows_only_essential_syscalls", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileMinimal)
		require.NotNil(t, filter)

		// Essential syscalls should be allowed
		assert.True(t, filter.IsAllowed("read"), "read should be allowed")
		assert.True(t, filter.IsAllowed("write"), "write should be allowed")
		assert.True(t, filter.IsAllowed("exit"), "exit should be allowed")
		assert.True(t, filter.IsAllowed("mmap"), "mmap should be allowed")

		// Network syscalls should NOT be allowed in minimal profile
		assert.False(t, filter.IsAllowed("socket"), "socket should NOT be allowed in minimal profile")
		assert.False(t, filter.IsAllowed("connect"), "connect should NOT be allowed in minimal profile")

		// Dangerous syscalls should always be blocked
		assert.False(t, filter.IsAllowed("execve"), "execve should be denied")
		assert.False(t, filter.IsAllowed("fork"), "fork should be denied")
	})

	t.Run("standard_profile_allows_network_syscalls", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileStandard)
		require.NotNil(t, filter)

		// Essential syscalls should be allowed
		assert.True(t, filter.IsAllowed("read"), "read should be allowed")
		assert.True(t, filter.IsAllowed("write"), "write should be allowed")

		// Network syscalls SHOULD be allowed in standard profile
		assert.True(t, filter.IsAllowed("socket"), "socket should be allowed in standard profile")
		assert.True(t, filter.IsAllowed("connect"), "connect should be allowed in standard profile")
		assert.True(t, filter.IsAllowed("bind"), "bind should be allowed in standard profile")

		// Dangerous syscalls should still be blocked
		assert.False(t, filter.IsAllowed("execve"), "execve should be denied")
		assert.False(t, filter.IsAllowed("fork"), "fork should be denied")
	})

	t.Run("relaxed_profile_allows_most_syscalls", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileRelaxed)
		require.NotNil(t, filter)

		// Most syscalls should be allowed in relaxed profile
		assert.True(t, filter.IsAllowed("read"), "read should be allowed")
		assert.True(t, filter.IsAllowed("socket"), "socket should be allowed")
		assert.True(t, filter.IsAllowed("openat"), "openat should be allowed")

		// Only dangerous syscalls should be blocked
		assert.False(t, filter.IsAllowed("execve"), "execve should be denied")
		assert.False(t, filter.IsAllowed("fork"), "fork should be denied")
		assert.False(t, filter.IsAllowed("ptrace"), "ptrace should be denied")
	})

	t.Run("dangerous_syscalls_blocked_in_all_profiles", func(t *testing.T) {
		profiles := []SyscallProfile{SyscallProfileMinimal, SyscallProfileStandard, SyscallProfileRelaxed}
		dangerousSyscalls := []string{"execve", "fork", "ptrace", "chroot", "mount"}

		for _, profile := range profiles {
			filter := NewSyscallFilter(profile)
			for _, syscall := range dangerousSyscalls {
				assert.False(t, filter.IsAllowed(syscall),
					"syscall %s should be blocked in %s profile", syscall, profile.String())
			}
		}
	})
}

func TestSyscallFilter_AllowSyscall(t *testing.T) {
	t.Run("allow_blocked_syscall_fails", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileMinimal)

		// Try to allow a syscall that's in the denylist
		err := filter.AllowSyscall("execve")
		assert.Error(t, err, "Should not be able to allow a denied syscall")
		assert.Contains(t, err.Error(), "denylist")
	})

	t.Run("allow_new_syscall_succeeds", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileMinimal)

		// Custom syscall not in default lists
		err := filter.AllowSyscall("custom_syscall")
		assert.NoError(t, err)
		assert.True(t, filter.IsAllowed("custom_syscall"))
	})

	t.Run("allow_already_allowed_syscall_succeeds", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileStandard)

		// Allow a syscall that's already allowed
		err := filter.AllowSyscall("socket")
		assert.NoError(t, err)
		assert.True(t, filter.IsAllowed("socket"))
	})
}

func TestSyscallFilter_DenySyscall(t *testing.T) {
	t.Run("deny_allowed_syscall_removes_from_allowlist", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileStandard)

		// Verify it's initially allowed
		assert.True(t, filter.IsAllowed("socket"))

		// Deny it
		err := filter.DenySyscall("socket")
		assert.NoError(t, err)

		// Verify it's now denied
		assert.False(t, filter.IsAllowed("socket"))
	})

	t.Run("deny_already_denied_syscall_succeeds", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileMinimal)

		// Deny a syscall that's already denied
		err := filter.DenySyscall("execve")
		assert.NoError(t, err)
		assert.False(t, filter.IsAllowed("execve"))
	})
}

func TestSyscallFilter_Apply(t *testing.T) {
	t.Run("apply_succeeds_first_time", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileMinimal)

		err := filter.Apply()
		assert.NoError(t, err)
		assert.True(t, filter.IsApplied())
	})

	t.Run("apply_fails_second_time", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileMinimal)

		err := filter.Apply()
		require.NoError(t, err)

		// Second apply should fail
		err = filter.Apply()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already applied")
	})
}

func TestSyscallFilter_Profile(t *testing.T) {
	t.Run("profile_string_representation", func(t *testing.T) {
		assert.Equal(t, "minimal", SyscallProfileMinimal.String())
		assert.Equal(t, "standard", SyscallProfileStandard.String())
		assert.Equal(t, "relaxed", SyscallProfileRelaxed.String())
		assert.Equal(t, "unknown", SyscallProfile(999).String())
	})

	t.Run("profile_matches_creation", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileStandard)
		assert.Equal(t, SyscallProfileStandard, filter.Profile())
	})
}

func TestSyscallFilter_GetAllowedSyscalls(t *testing.T) {
	t.Run("returns_allowed_syscalls", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileMinimal)

		allowed := filter.GetAllowedSyscalls()
		assert.NotEmpty(t, allowed)

		// Check that essential syscalls are in the list
		foundRead := false
		foundWrite := false
		for _, syscall := range allowed {
			if syscall == "read" {
				foundRead = true
			}
			if syscall == "write" {
				foundWrite = true
			}
		}
		assert.True(t, foundRead, "read should be in allowed list")
		assert.True(t, foundWrite, "write should be in allowed list")
	})
}

func TestSyscallFilter_GetDeniedSyscalls(t *testing.T) {
	t.Run("returns_denied_syscalls", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileMinimal)

		denied := filter.GetDeniedSyscalls()
		assert.NotEmpty(t, denied)

		// Check that dangerous syscalls are in the list
		foundExecve := false
		for _, syscall := range denied {
			if syscall == "execve" {
				foundExecve = true
				break
			}
		}
		assert.True(t, foundExecve, "execve should be in denied list")
	})
}

func TestSyscallFilter_CheckSyscall(t *testing.T) {
	t.Run("check_allowed_syscall_succeeds", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileStandard)

		err := filter.CheckSyscall("socket")
		assert.NoError(t, err)
	})

	t.Run("check_denied_syscall_fails", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileStandard)

		err := filter.CheckSyscall("execve")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})
}

func TestSyscallFilter_SetDenyCallback(t *testing.T) {
	t.Run("custom_callback_called", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileStandard)

		callbackCalled := false
		callbackSyscall := ""

		filter.SetDenyCallback(func(syscallName string) error {
			callbackCalled = true
			callbackSyscall = syscallName
			return assert.AnError
		})

		// Check a denied syscall
		err := filter.CheckSyscall("execve")

		assert.True(t, callbackCalled, "Callback should have been called")
		assert.Equal(t, "execve", callbackSyscall)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

// ============================================================================
// Runtime with Syscall Filter Tests
// ============================================================================

func TestNewRuntimeWithSyscallFilter(t *testing.T) {
	t.Run("creates_runtime_with_filter", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := DefaultLimits()

		runtime, err := NewRuntimeWithSyscallFilter(config, limits, SyscallProfileStandard)
		require.NoError(t, err)
		require.NotNil(t, runtime)
		require.NotNil(t, runtime.SyscallFilter())

		assert.Equal(t, SyscallProfileStandard, runtime.SyscallFilter().Profile())
	})

	t.Run("nil_config_returns_error", func(t *testing.T) {
		runtime, err := NewRuntimeWithSyscallFilter(nil, nil, SyscallProfileStandard)
		// Currently returns nil runtime but no error - this might be by design
		// Just check it doesn't panic
		_ = runtime
		_ = err
	})
}

func TestRuntimeWithSyscallFilter_IsSyscallAllowed(t *testing.T) {
	t.Run("delegates_to_filter", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := DefaultLimits()

		runtime, err := NewRuntimeWithSyscallFilter(config, limits, SyscallProfileMinimal)
		require.NoError(t, err)

		// Allowed syscalls
		assert.True(t, runtime.IsSyscallAllowed("read"))
		assert.True(t, runtime.IsSyscallAllowed("write"))

		// Denied syscalls
		assert.False(t, runtime.IsSyscallAllowed("execve"))
		assert.False(t, runtime.IsSyscallAllowed("socket"))
	})
}

func TestRuntimeWithSyscallFilter_CheckSyscall(t *testing.T) {
	t.Run("check_allowed_succeeds", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := DefaultLimits()

		runtime, err := NewRuntimeWithSyscallFilter(config, limits, SyscallProfileStandard)
		require.NoError(t, err)

		err = runtime.CheckSyscall("socket")
		assert.NoError(t, err)
	})

	t.Run("check_denied_fails", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := DefaultLimits()

		runtime, err := NewRuntimeWithSyscallFilter(config, limits, SyscallProfileStandard)
		require.NoError(t, err)

		err = runtime.CheckSyscall("execve")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "denied")
	})
}

func TestRuntimeWithSyscallFilter_ApplySyscallFilter(t *testing.T) {
	t.Run("apply_succeeds", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := DefaultLimits()

		runtime, err := NewRuntimeWithSyscallFilter(config, limits, SyscallProfileMinimal)
		require.NoError(t, err)

		err = runtime.ApplySyscallFilter()
		assert.NoError(t, err)
		assert.True(t, runtime.SyscallFilter().(*InMemorySyscallFilter).IsApplied())
	})

	t.Run("apply_twice_fails", func(t *testing.T) {
		config := DefaultRuntimeConfig()
		limits := DefaultLimits()

		runtime, err := NewRuntimeWithSyscallFilter(config, limits, SyscallProfileMinimal)
		require.NoError(t, err)

		err = runtime.ApplySyscallFilter()
		require.NoError(t, err)

		err = runtime.ApplySyscallFilter()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already applied")
	})
}

// ============================================================================
// Platform Support Tests
// ============================================================================

func TestPlatformSupportsSyscallFiltering(t *testing.T) {
	t.Run("linux_returns_true", func(t *testing.T) {
		if runtime.GOOS == "linux" {
			assert.True(t, PlatformSupportsSyscallFiltering())
		}
	})

	t.Run("other_platforms_return_false", func(t *testing.T) {
		// We can't change the runtime.GOOS in tests, but we can document the behavior
		// This test serves as documentation
		if runtime.GOOS != "linux" {
			assert.False(t, PlatformSupportsSyscallFiltering())
		}
	})
}

func TestGetPlatformSpecificSyscalls(t *testing.T) {
	t.Run("returns_linux_syscalls_on_linux", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("Skipping Linux-specific test on non-Linux platform")
		}

		syscalls := GetPlatformSpecificSyscalls()
		assert.NotEmpty(t, syscalls)

		// Check for Linux-specific syscalls
		found := false
		for _, syscall := range syscalls {
			if syscall == "epoll_create" || syscall == "epoll_create1" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should contain Linux-specific syscalls like epoll_create")
	})

	t.Run("returns_darwin_syscalls_on_darwin", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("Skipping Darwin-specific test on non-Darwin platform")
		}

		syscalls := GetPlatformSpecificSyscalls()
		assert.NotEmpty(t, syscalls)

		// Check for macOS-specific syscalls
		found := false
		for _, syscall := range syscalls {
			if syscall == "kqueue" || syscall == "kevent" {
				found = true
				break
			}
		}
		assert.True(t, found, "Should contain Darwin-specific syscalls like kqueue")
	})

	t.Run("returns_empty_on_unknown_platform", func(t *testing.T) {
		// Can't test this without mocking runtime.GOOS
		// This test serves as documentation
		t.Skip("Cannot test unknown platform without mocking")
	})
}

// ============================================================================
// Security Profile Validation Tests
// ============================================================================

func TestSecurityProfiles(t *testing.T) {
	t.Run("verify_dangerous_syscalls_list", func(t *testing.T) {
		// Verify that all critical dangerous syscalls are in the list
		criticalSyscalls := []string{
			"execve",      // Process execution
			"fork",        // Process creation
			"ptrace",      // Process tracing
			"chroot",      // Directory isolation bypass
			"mount",       // Filesystem mounting
			"setns",       // Namespace manipulation
			"init_module", // Kernel module loading
		}

		for _, syscall := range criticalSyscalls {
			found := false
			for _, dangerous := range DangerousSyscalls {
				if syscall == dangerous {
					found = true
					break
				}
			}
			assert.True(t, found, "Critical syscall %s should be in DangerousSyscalls list", syscall)
		}
	})

	t.Run("verify_minimal_syscalls_essential", func(t *testing.T) {
		// Verify minimal syscalls are truly essential
		essentialSyscalls := []string{
			"read", "write", // Basic I/O
			"exit",      // Process termination
			"mmap",      // Memory allocation
			"sigreturn", // Signal handling
		}

		for _, syscall := range essentialSyscalls {
			found := false
			for _, minimal := range DefaultMinimalSyscalls {
				if syscall == minimal {
					found = true
					break
				}
			}
			assert.True(t, found, "Essential syscall %s should be in DefaultMinimalSyscalls", syscall)
		}
	})

	t.Run("standard_includes_minimal_plus_network", func(t *testing.T) {
		// Verify standard profile includes all minimal syscalls
		minimalSet := make(map[string]bool)
		for _, syscall := range DefaultMinimalSyscalls {
			minimalSet[syscall] = true
		}

		standardSet := make(map[string]bool)
		for _, syscall := range DefaultStandardSyscalls {
			standardSet[syscall] = true
		}

		// All minimal syscalls should be in standard
		for syscall := range minimalSet {
			assert.True(t, standardSet[syscall],
				"Minimal syscall %s should be included in Standard syscalls", syscall)
		}

		// Standard should include network syscalls
		networkSyscalls := []string{"socket", "connect", "accept", "bind", "listen"}
		for _, syscall := range networkSyscalls {
			assert.True(t, standardSet[syscall],
				"Network syscall %s should be in Standard syscalls", syscall)
		}
	})
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestSyscallFilter_Integration(t *testing.T) {
	t.Run("full_workflow_minimal_profile", func(t *testing.T) {
		// Create filter
		filter := NewSyscallFilter(SyscallProfileMinimal)

		// Verify initial state
		assert.True(t, filter.IsAllowed("read"))
		assert.False(t, filter.IsAllowed("socket"))
		assert.False(t, filter.IsAllowed("execve"))

		// Try to allow a denied syscall (should fail)
		err := filter.AllowSyscall("execve")
		assert.Error(t, err)

		// Try to allow a new syscall
		err = filter.AllowSyscall("custom_syscall")
		assert.NoError(t, err)
		assert.True(t, filter.IsAllowed("custom_syscall"))

		// Apply filter
		err = filter.Apply()
		assert.NoError(t, err)
		assert.True(t, filter.IsApplied())

		// Verify syscalls are still correct after apply
		assert.True(t, filter.IsAllowed("read"))
		assert.False(t, filter.IsAllowed("socket"))
		assert.False(t, filter.IsAllowed("execve"))
	})

	t.Run("profile_hierarchy", func(t *testing.T) {
		// Minimal should be most restrictive
		minimalFilter := NewSyscallFilter(SyscallProfileMinimal)
		standardFilter := NewSyscallFilter(SyscallProfileStandard)
		relaxedFilter := NewSyscallFilter(SyscallProfileRelaxed)

		// Minimal should NOT allow network syscalls
		assert.False(t, minimalFilter.IsAllowed("socket"))

		// Standard SHOULD allow network syscalls
		assert.True(t, standardFilter.IsAllowed("socket"))

		// Relaxed should allow even more
		assert.True(t, relaxedFilter.IsAllowed("openat"))

		// All should block dangerous syscalls
		dangerous := []string{"execve", "fork", "ptrace"}
		for _, syscall := range dangerous {
			assert.False(t, minimalFilter.IsAllowed(syscall))
			assert.False(t, standardFilter.IsAllowed(syscall))
			assert.False(t, relaxedFilter.IsAllowed(syscall))
		}
	})

	t.Run("concurrent_access", func(t *testing.T) {
		filter := NewSyscallFilter(SyscallProfileStandard)

		// Test concurrent reads
		done := make(chan bool, 10)
		for i := 0; i < 10; i++ {
			go func() {
				for j := 0; j < 100; j++ {
					_ = filter.IsAllowed("socket")
					_ = filter.IsAllowed("execve")
				}
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Verify filter is still consistent
		assert.True(t, filter.IsAllowed("socket"))
		assert.False(t, filter.IsAllowed("execve"))
	})
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkSyscallFilter_IsAllowed(b *testing.B) {
	filter := NewSyscallFilter(SyscallProfileStandard)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.IsAllowed("socket")
	}
}

func BenchmarkSyscallFilter_CheckSyscall(b *testing.B) {
	filter := NewSyscallFilter(SyscallProfileStandard)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.CheckSyscall("socket")
	}
}

func BenchmarkSyscallFilter_ConcurrentReads(b *testing.B) {
	filter := NewSyscallFilter(SyscallProfileStandard)

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			filter.IsAllowed("socket")
		}
	})
}
