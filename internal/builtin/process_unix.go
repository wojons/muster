//go:build !windows

package builtin

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// processStartTime returns the wall-clock start time of the process with
// the given PID, read from /proc/<pid>/stat (Linux). On systems without
// /proc it falls back to the socket-less "unknown" marker.
func processStartTime(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "unknown"
	}

	// Format: pid (comm) state ppid ... — the comm field may contain
	// spaces/parens, so find the LAST ')' and parse from there.
	idx := strings.LastIndexByte(string(data), ')')
	if idx < 0 || idx+2 >= len(data) {
		return "unknown"
	}

	fields := strings.Fields(string(data)[idx+2:])
	// After comm: state(1) ppid(2) pgrp(3) session(4) tty_nr(5) tpgid(6)
	// flags(7) minflt(8) cminflt(9) majflt(10) cmajflt(11) utime(12)
	// stime(13) cutime(14) cstime(15) priority(16) nice(17) num_threads(18)
	// itrealvalue(19) starttime(20) — field index 19 (0-based) = starttime.
	if len(fields) < 20 {
		return "unknown"
	}
	startTicks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return "unknown"
	}

	// starttime is in clock ticks since system boot.
	ticksPerSec := int64(100)
	if v := os.Getenv("USER_HZ"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			ticksPerSec = n
		}
	}
	bootTime := systemBootTime()
	started := bootTime.Add(time.Duration(startTicks * int64(time.Second) / ticksPerSec))
	return started.Format("2006-01-02 15:04:05")
}

// systemBootTime returns the wall-clock time of system boot.
func systemBootTime() time.Time {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Now()
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			secs, err := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "btime ")), 10, 64)
			if err == nil {
				return time.Unix(secs, 0)
			}
		}
	}
	return time.Now()
}
