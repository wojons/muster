//go:build windows

package daemon

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// PROCESS_QUERY_LIMITED_INFORMATION (0x1000) is the minimal access right needed
// to query process existence without elevated privileges.
const processQueryLimitedInformation uint32 = 0x1000

// processStillActive is STILL_ACTIVE (winbase.h): the exit code
// GetExitCodeProcess reports while the process is still running.
const processStillActive uint32 = 259

// ERROR_SHARING_VIOLATION is returned when another handle already holds the
// file without permitting shared access.
const errorSharingViolation = syscall.Errno(32)

// acquireGuardLock opens (creating if needed) the guard file with NO sharing
// permitted: on Windows the exclusive OPEN is the lock, so a second opener gets
// ERROR_SHARING_VIOLATION and busy == true. Windows has no shared-probe mode
// here, so a probe is exclusive too — it is held for microseconds either way,
// and AcquireStartGuard retries through a concurrent probe.
func acquireGuardLock(path string, _ bool) (*os.File, bool, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, fmt.Errorf("guard path %s: %w", path, err)
	}
	handle, err := syscall.CreateFile(
		ptr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,   // no sharing: this IS the lock
		nil, // default security
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		if errors.Is(err, errorSharingViolation) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("open guard file %s: %w", path, err)
	}
	return os.NewFile(uintptr(handle), path), false, nil
}

// releaseGuardLock closes the guard handle, releasing the exclusive open.
func releaseGuardLock(file *os.File) error {
	if file == nil {
		return nil
	}
	return file.Close()
}

// processAlive reports whether a process with the given PID is running. Only
// STILL_ACTIVE is liveness: Windows keeps a terminated process object openable
// while any handle references it.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	var code uint32
	if err := syscall.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	return code == processStillActive
}

// terminatePID kills the process and immediately releases the handle
// os.FindProcess opened, so processAlive does not keep reporting it running.
func terminatePID(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	killErr := proc.Kill()
	_ = proc.Release()
	return killErr
}
