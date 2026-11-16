//go:build !windows

package daemon

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
)

// acquireGuardLock opens (creating if needed) the guard file and takes a
// non-blocking flock on it: LOCK_EX for a start, LOCK_SH for a probe. busy is
// true when another process holds a conflicting lock — that IS the answer, not
// an error. A shared probe never blocks a shared probe, so two status calls
// cannot make a start look busy; an exclusive holder always blocks both.
func acquireGuardLock(path string, exclusive bool) (*os.File, bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false, fmt.Errorf("open guard file: %w", err)
	}
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if err := syscall.Flock(int(file.Fd()), how|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("flock %s: %w", path, err)
	}
	return file, false, nil
}

// releaseGuardLock unlocks and closes the guard file.
func releaseGuardLock(file *os.File) error {
	if file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	closeErr := file.Close()
	if unlockErr != nil {
		return fmt.Errorf("unlock guard file: %w", unlockErr)
	}
	return closeErr
}

// processAlive reports whether the PID exists and is not a zombie. Signal 0
// checks existence; /proc/<pid>/status excludes zombies that have exited but
// not been reaped by their parent (in containers PID 1 often never reaps).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "State:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] == "Z" {
				return false
			}
			break
		}
	}
	return true
}

// terminatePID asks the process to shut down (SIGTERM), which is the signal the
// daemon's own handler reaps: it removes its pid file and socket on the way out.
func terminatePID(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}
