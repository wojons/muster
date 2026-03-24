//go:build linux
// +build linux

package platform

import (
	"runtime"
	"testing"
)

func TestNewSeccompFilter(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)
	if filter == nil {
		t.Fatal("NewSeccompFilter returned nil")
	}
	if filter.mode != SeccompModeFilter {
		t.Errorf("expected mode %d, got %d", SeccompModeFilter, filter.mode)
	}
}

func TestSeccompFilter_AllowSyscall(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)

	err := filter.AllowSyscall(SYS_READ)
	if err != nil {
		t.Fatalf("AllowSyscall failed: %v", err)
	}

	allowed, err := filter.VerifySyscall(SYS_READ)
	if err != nil {
		t.Fatalf("VerifySyscall failed: %v", err)
	}
	if !allowed {
		t.Error("expected READ to be allowed")
	}
}

func TestSeccompFilter_DenySyscall(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)

	err := filter.DenySyscall(SYS_WRITE)
	if err != nil {
		t.Fatalf("DenySyscall failed: %v", err)
	}

	allowed, err := filter.VerifySyscall(SYS_WRITE)
	if err == nil && allowed {
		t.Error("expected WRITE to be denied")
	}
}

func TestSeccompFilter_AllowReadOnly(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)

	err := filter.AllowReadOnly()
	if err != nil {
		t.Fatalf("AllowReadOnly failed: %v", err)
	}

	allowed, err := filter.VerifySyscall(SYS_READ)
	if err != nil {
		t.Fatalf("VerifySyscall failed: %v", err)
	}
	if !allowed {
		t.Error("expected READ to be allowed in read-only mode")
	}
}

func TestSeccompFilter_DenyWrite(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)

	err := filter.DenyWrite()
	if err != nil {
		t.Fatalf("DenyWrite failed: %v", err)
	}

	allowed, err := filter.VerifySyscall(SYS_WRITE)
	if err == nil && allowed {
		t.Error("expected WRITE to be denied")
	}
}

func TestSeccompFilter_SetSocketRestriction(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)

	err := filter.SetSocketRestriction(false, false)
	if err != nil {
		t.Fatalf("SetSocketRestriction failed: %v", err)
	}
}

func TestSeccompFilter_LimitFileAccess(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)

	allowedPaths := []string{"/tmp", "/var/tmp"}
	err := filter.LimitFileAccess(allowedPaths)
	if err != nil {
		t.Fatalf("LimitFileAccess failed: %v", err)
	}
}

func TestSeccompFilter_EnableNetworkRestriction(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)

	err := filter.EnableNetworkRestriction(true, true)
	if err != nil {
		t.Fatalf("EnableNetworkRestriction failed: %v", err)
	}

	err = filter.EnableNetworkRestriction(false, false)
	if err != nil {
		t.Fatalf("EnableNetworkRestriction failed: %v", err)
	}
}

func TestSeccompFilter_ApplyProfile(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)

	err := filter.ApplyProfile(SeccompProfileMinimal)
	if err != nil {
		t.Fatalf("ApplyProfile(Minimal) failed: %v", err)
	}

	filter2 := NewSeccompFilter(SeccompModeFilter)
	err = filter2.ApplyProfile(SeccompProfileStandard)
	if err != nil {
		t.Fatalf("ApplyProfile(Standard) failed: %v", err)
	}

	filter3 := NewSeccompFilter(SeccompModeFilter)
	err = filter3.ApplyProfile(SeccompProfileRelaxed)
	if err != nil {
		t.Fatalf("ApplyProfile(Relaxed) failed: %v", err)
	}
}

func TestSeccompFilter_VerifySyscall(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)

	filter.AllowSyscall(SYS_READ)

	allowed, err := filter.VerifySyscall(SYS_READ)
	if err != nil {
		t.Fatalf("VerifySyscall failed: %v", err)
	}
	if !allowed {
		t.Error("expected READ to be verified as allowed")
	}

	allowed, err = filter.VerifySyscall(SYS_WRITE)
	if err == nil {
		t.Error("expected error for unlisted syscall")
	}
	if allowed {
		t.Error("expected WRITE to not be allowed")
	}
}

func TestGetFailedSyscall(t *testing.T) {
	_, _, err := GetFailedSyscall()
	if err == nil {
		t.Error("expected error when no failed syscall recorded")
	}
}

func TestEnableStrictMode(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("seccomp only supported on Linux")
	}

	filter := NewSeccompFilter(SeccompModeFilter)
	err := filter.EnableStrictMode()
	if err != nil {
		t.Fatalf("EnableStrictMode failed: %v", err)
	}
}

func TestCheckSeccompSupport(t *testing.T) {
	supported := CheckSeccompSupport()

	if runtime.GOOS == "linux" {
		if !supported {
			t.Error("expected seccomp support on Linux")
		}
	} else {
		if supported {
			t.Error("did not expect seccomp support on non-Linux")
		}
	}
}

func TestToSyscallName(t *testing.T) {
	tests := []struct {
		num      uintptr
		expected string
	}{
		{0, "read"},
		{1, "write"},
		{2, "open"},
		{4, "stat"},
		{999, "unknown"},
	}

	for _, tt := range tests {
		name := toSyscallName(tt.num)
		if name != tt.expected {
			t.Errorf("toSyscallName(%d) = %s, expected %s", tt.num, name, tt.expected)
		}
	}
}

func TestSeccompFilter_Apply(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("seccomp only supported on Linux")
	}

	filter := NewSeccompFilter(SeccompModeFilter)
	filter.AllowSyscall(SYS_READ)
	filter.AllowSyscall(SYS_WRITE)
	filter.AllowSyscall(SYS_OPEN)
	filter.AllowSyscall(SYS_STAT)

	err := filter.Apply()
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
}

func TestApplyToProcess(t *testing.T) {
	filter := NewSeccompFilter(SeccompModeFilter)

	err := ApplyToProcess(1, filter)
	if err == nil {
		t.Error("expected error when applying to external process")
	}

	err = ApplyToProcess(-1, filter)
	if err == nil {
		t.Error("expected error for invalid PID")
	}
}

func TestSeccompFilter_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("test is for non-Linux platforms")
	}

	filter := NewSeccompFilter(SeccompModeFilter)
	err := filter.Apply()
	if err == nil {
		t.Error("expected error on non-Linux platform")
	}
}
