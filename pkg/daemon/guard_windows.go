//go:build windows

package daemon

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// PROCESS_QUERY_LIMITED_INFORMATION (0x1000) is the minimal access right needed
// to query process existence without elevated privileges.
const processQueryLimitedInformation uint32 = 0x1000

// processStillActive is STILL_ACTIVE (winbase.h): the exit code
// GetExitCodeProcess reports while the process is still running.
const processStillActive uint32 = 259

const (
	// The guard byte is outside the PID text at offset 0. Windows byte-range
	// locks are mandatory, so readers of the PID remain possible while this
	// byte stays locked.
	guardLockOffset uint32 = 0x7fffffff
	guardLockFlags         = windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY
)

// acquireGuardLock opens (creating if needed) the guard file with read/write
// sharing and takes an exclusive, non-blocking lock on a byte beyond the PID
// text. The file remains readable for attribution while the guard byte provides
// exclusion, including between two handles in one process.
func acquireGuardLock(path string, _ bool) (*os.File, bool, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, fmt.Errorf("guard path %s: %w", path, err)
	}
	handle, err := syscall.CreateFile(
		ptr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil, // default security
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, false, fmt.Errorf("open guard file %s: %w", path, err)
	}

	overlapped := windows.Overlapped{Offset: guardLockOffset}
	if err := windows.LockFileEx(
		windows.Handle(handle),
		guardLockFlags,
		0,
		1,
		0,
		&overlapped,
	); err != nil {
		_ = syscall.CloseHandle(handle)
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("lock guard file %s: %w", path, err)
	}
	return os.NewFile(uintptr(handle), path), false, nil
}

// releaseGuardLock unlocks the guard byte before closing the guard handle.
func releaseGuardLock(file *os.File) error {
	if file == nil {
		return nil
	}
	overlapped := windows.Overlapped{Offset: guardLockOffset}
	unlockErr := windows.UnlockFileEx(
		windows.Handle(file.Fd()),
		0,
		1,
		0,
		&overlapped,
	)
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
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
