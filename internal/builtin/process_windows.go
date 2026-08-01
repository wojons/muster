//go:build windows

package builtin

import "time"

// processStartTime on Windows returns "unknown" — process start time
// would require syscall.QueryProcessCycleTime/GetProcessTimes plumbing
// that isn't worth it for a status display.
func processStartTime(pid int) string {
	return "unknown"
}

// systemBootTime is unused on Windows but kept for interface symmetry.
func systemBootTime() time.Time {
	return time.Now()
}
