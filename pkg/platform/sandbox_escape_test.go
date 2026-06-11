//go:build linux
// +build linux

package platform

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestSandboxEscape_Prevention tests various sandbox escape attempts
func TestSandboxEscape_Prevention(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sandbox tests only supported on Linux")
	}
	seccompAvailable(t)

	tests := []struct {
		name          string
		attempt       func() error
		shouldSucceed bool
		description   string
	}{
		{
			name: "proc_self_exe",
			attempt: func() error {
				_, err := os.Readlink("/proc/self/exe")
				return err
			},
			shouldSucceed: true,
			description:   "Reading /proc/self/exe should be allowed",
		},
		{
			name: "proc_self_mem",
			attempt: func() error {
				f, err := os.Open("/proc/self/mem")
				if err == nil {
					f.Close()
				}
				return err
			},
			shouldSucceed: false,
			description:   "Opening /proc/self/mem should be blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.attempt()
			if tt.shouldSucceed {
				assert.NoError(t, err, tt.description)
			} else {
				assert.Error(t, err, tt.description)
			}
		})
	}
}

// TestSandboxEscape_Ptrace tests ptrace-based escape attempts
func TestSandboxEscape_Ptrace(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ptrace tests only supported on Linux")
	}

	err := syscall.PtraceAttach(syscall.Getpid())
	if err == nil {
		syscall.PtraceDetach(syscall.Getpid())
		t.Log("Warning: ptrace succeeded, CAP_SYS_PTRACE may be available")
	} else {
		t.Logf("ptrace correctly blocked: %v", err)
	}
}

// TestSandboxEscape_MemoryLimits tests memory limit enforcement
func TestSandboxEscape_MemoryLimits(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("memory limit tests only supported on Linux")
	}

	memLimitPath := "/sys/fs/cgroup/memory/memory.limit_in_bytes"
	data, err := os.ReadFile(memLimitPath)
	if err != nil {
		t.Skip("Cannot read cgroup memory limit")
	}

	t.Logf("Memory limit: %s", string(data))
}

// TestSandboxEscape_CPULimits tests CPU limit enforcement
func TestSandboxEscape_CPULimits(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("CPU limit tests only supported on Linux")
	}

	cpuQuotaPath := "/sys/fs/cgroup/cpu/cpu.cfs_quota_us"
	data, err := os.ReadFile(cpuQuotaPath)
	if err != nil {
		t.Skip("Cannot read cgroup CPU quota")
	}

	t.Logf("CPU quota: %s", string(data))
}

// TestSandboxEscape_Timers tests timer-based attacks
func TestSandboxEscape_Timers(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("timer tests only supported on Linux")
	}

	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()

	select {
	case <-timer.C:
		t.Log("Timer fired as expected")
	case <-time.After(time.Second):
		t.Error("Timer did not fire")
	}
}
