package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSandboxIsolation_Exists tests that sandbox files exist
func TestSandboxIsolation_Exists(t *testing.T) {
	// Check if seccomp.go exists
	seccompPath := filepath.Join("seccomp.go")
	info, err := os.Stat(seccompPath)
	require.NoError(t, err, "seccomp.go should exist")
	assert.False(t, info.IsDir(), "seccomp.go should be a file")
}

// TestSandboxIsolation_NonLinux_PlatformSupport tests platform support on non-Linux
func TestSandboxIsolation_NonLinux_PlatformSupport(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("This test is for non-Linux platforms")
	}

	sf := NewSeccompFilter(SeccompModeFilter)
	err := sf.Apply()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "seccomp is only supported on Linux")
}

// TestSandboxIsolation_SeccompProfiles tests seccomp profile constants
func TestSandboxIsolation_SeccompProfiles(t *testing.T) {
	// Verify profile constants exist
	assert.Equal(t, SeccompProfile(0), SeccompProfileMinimal)
	assert.Equal(t, SeccompProfile(1), SeccompProfileStandard)
	assert.Equal(t, SeccompProfile(2), SeccompProfileRelaxed)
}

// TestSandboxIsolation_SeccompModes tests seccomp mode constants
func TestSandboxIsolation_SeccompModes(t *testing.T) {
	// Verify mode constants exist
	assert.Equal(t, SeccompFilterMode(1), SeccompModeFilter)
	assert.Equal(t, SeccompFilterMode(2), SeccompModeStrict)
}

// TestSandboxIsolation_CreateFilter tests filter creation
func TestSandboxIsolation_CreateFilter(t *testing.T) {
	sf := NewSeccompFilter(SeccompModeFilter)
	require.NotNil(t, sf)
	assert.Equal(t, SeccompModeFilter, sf.mode)
}

// TestSandboxIsolation_SyscallConstants tests syscall constants
func TestSandboxIsolation_SyscallConstants(t *testing.T) {
	// Verify syscall constants
	assert.Equal(t, uintptr(0), SYS_READ)
	assert.Equal(t, uintptr(1), SYS_WRITE)
	assert.Equal(t, uintptr(2), SYS_OPEN)
	assert.Equal(t, uintptr(4), SYS_STAT)
}

// TestSandboxIsolation_SyscallNameConversion tests syscall name conversion
func TestSandboxIsolation_SyscallNameConversion(t *testing.T) {
	testCases := []struct {
		num      uintptr
		expected string
	}{
		{0, "read"},
		{1, "write"},
		{2, "open"},
		{4, "stat"},
		{999, "unknown"},
	}

	for _, tc := range testCases {
		name := toSyscallName(tc.num)
		assert.Equal(t, tc.expected, name, "syscall %d should map to %s", tc.num, tc.expected)
	}
}

// TestSandboxIsolation_CheckSeccompSupport tests seccomp support detection
func TestSandboxIsolation_CheckSeccompSupport(t *testing.T) {
	supported := CheckSeccompSupport()
	
	if runtime.GOOS == "linux" {
		// On Linux, should be supported
		assert.True(t, supported, "seccomp should be supported on Linux")
	} else {
		// On non-Linux, should not be supported
		assert.False(t, supported, "seccomp should not be supported on non-Linux")
	}
}

// TestSandboxIsolation_FilterCreation tests basic filter operations
func TestSandboxIsolation_FilterCreation(t *testing.T) {
	sf := NewSeccompFilter(SeccompModeFilter)
	require.NotNil(t, sf)
	
	// Initially not loaded
	assert.False(t, sf.loaded)
	
	// Mode should be set
	assert.Equal(t, SeccompModeFilter, sf.mode)
}

// TestSandboxIsolation_ProfileApplication tests profile application (non-Linux)
func TestSandboxIsolation_ProfileApplication(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("This test is for non-Linux platforms")
	}

	sf := NewSeccompFilter(SeccompModeFilter)
	
	// Profile application should succeed (doesn't load yet)
	err := sf.ApplyProfile(SeccompProfileMinimal)
	require.NoError(t, err)
	
	// Applying the filter should fail on non-Linux
	err = sf.Apply()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "seccomp is only supported on Linux")
}
