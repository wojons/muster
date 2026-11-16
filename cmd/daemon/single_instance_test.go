package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestDaemonStartRefusesSecondInstance is the QA-MUSTER-035 regression.
//
// Before the shared pre-bind guard, a second `start` on a live socket unlinked
// that socket and bound a fresh listener: the first daemon kept its (now
// anonymous) listener, no PID file pointed at it, and `stop` could only reap
// the newcomer. The orphan then made `status` report a dead PID next to a zero
// start time.
//
// The test is RED on the old code in two distinct ways, which is why it runs
// both entry shapes:
//   - the daemonized start (the operator path) returns rc 0 and spawns a
//     duplicate, so the refusal assertions fail;
//   - the foreground start blocks forever serving the stolen socket, so the
//     bounded CommandContext has to kill it at the deadline — the hang the task
//     asks to see.
func TestDaemonStartRefusesSecondInstance(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-domain socket daemon lifecycle test")
	}

	binaryPath, err := buildDaemonBinary()
	require.NoError(t, err, "build daemon binary")
	defer func() { _ = os.Remove(binaryPath) }()

	tmpDir := shortSocketTempDir(t)
	socketPath := filepath.Join(tmpDir, "single-instance.sock")
	pidPath := socketPath + ".pid"

	// Always reap what this test started, even when an assertion fails early.
	t.Cleanup(func() {
		for _, pid := range socketDaemonPIDs(t, socketPath) {
			if proc, err := os.FindProcess(pid); err == nil {
				_ = proc.Kill()
				_ = proc.Release()
			}
		}
	})

	// --- 1. one daemon on the socket -------------------------------------
	stdout, stderr, err := runDaemonCommand(t, binaryPath, 30*time.Second, "start", "--socket-path", socketPath)
	require.NoError(t, err, "first start must succeed: stdout=%s stderr=%s", stdout, stderr)

	firstPID, err := readPIDFile(pidPath)
	require.NoError(t, err, "first start must write a PID file")
	require.True(t, isProcessRunning(firstPID), "PID %d from the PID file must be alive", firstPID)
	require.Equal(t, 1, len(socketDaemonPIDs(t, socketPath)), "exactly one daemon process must serve the socket")

	pidInfo, err := os.Stat(pidPath)
	require.NoError(t, err)
	pidBytes, err := os.ReadFile(pidPath)
	require.NoError(t, err)
	pidMTime := pidInfo.ModTime()

	// --- 2a. second start (operator path) must be REFUSED ----------------
	started := time.Now()
	stdout2, stderr2, err2 := runDaemonCommand(t, binaryPath, 20*time.Second, "start", "--socket-path", socketPath)
	refusalElapsed := time.Since(started)

	require.Error(t, err2, "second start must fail: stdout=%s stderr=%s", stdout2, stderr2)
	require.Contains(t, stderr2, fmt.Sprintf("daemon already running (PID %d)", firstPID),
		"second start must name the live PID on stderr")
	require.Less(t, refusalElapsed, 10*time.Second,
		"second start must be refused promptly (it took %s)", refusalElapsed)

	// The refusal must not disturb the running daemon's bookkeeping.
	afterBytes, err := os.ReadFile(pidPath)
	require.NoError(t, err, "the PID file must still exist after a refused start")
	require.Equal(t, string(pidBytes), string(afterBytes), "a refused start must not rewrite the PID file")
	afterInfo, err := os.Stat(pidPath)
	require.NoError(t, err)
	require.True(t, afterInfo.ModTime().Equal(pidMTime), "a refused start must not touch the PID file mtime")
	require.True(t, isProcessRunning(firstPID), "the first daemon must survive a refused start")
	require.Equal(t, 1, len(socketDaemonPIDs(t, socketPath)), "still exactly one daemon after a refused start")

	// --- 2b. second start in the foreground must be REFUSED -------------
	// Pre-fix this arm never returns: it unlinks the live socket, binds its own
	// listener and serves forever. The deadline is what turns that hang into a
	// red result instead of a stuck suite.
	started = time.Now()
	stdout3, stderr3, err3 := runDaemonCommand(t, binaryPath, 8*time.Second, "start", "--foreground", "--socket-path", socketPath)
	foregroundElapsed := time.Since(started)

	require.Error(t, err3, "foreground start on a live socket must fail: stdout=%s stderr=%s", stdout3, stderr3)
	require.Contains(t, stderr3, fmt.Sprintf("daemon already running (PID %d)", firstPID),
		"foreground start must name the live PID on stderr")
	require.Less(t, foregroundElapsed, 5*time.Second,
		"foreground start must be refused, not block until the deadline (it took %s)", foregroundElapsed)
	require.Equal(t, 1, len(socketDaemonPIDs(t, socketPath)), "still exactly one daemon after a refused foreground start")

	// --- 3. stop reaps the daemon and leaves no survivors ----------------
	stopOut, stopErrOut, err := runDaemonCommand(t, binaryPath, 20*time.Second, "stop", "--socket-path", socketPath)
	require.NoError(t, err, "stop must succeed: stdout=%s stderr=%s", stopOut, stopErrOut)
	require.Contains(t, stopOut, "Daemon stopped successfully")

	require.False(t, isProcessRunning(firstPID), "daemon PID %d must be gone after stop", firstPID)
	require.Empty(t, socketDaemonPIDs(t, socketPath), "no daemon may survive stop")
	require.NoFileExists(t, pidPath, "stop must remove the PID file")
	require.NoFileExists(t, socketPath, "stop must remove the socket")
	if n, ok := socketListenerCount(t, socketPath); ok {
		require.Zero(t, n, "no listener may remain bound to the socket")
	}

	// --- 4. status reflects the stopped daemon ---------------------------
	statusOut, statusErrOut, err := runDaemonCommand(t, binaryPath, 10*time.Second, "status", "--socket-path", socketPath)
	require.NoError(t, err, "status must succeed: stderr=%s", statusErrOut)
	require.Contains(t, statusOut, "Running:     false", "status must report the stopped daemon")
	require.NotContains(t, statusOut, "PID:", "status must not print a PID for a stopped daemon")
	require.NotContains(t, statusOut, "0001-01-01", "status must not print a zero-time placeholder")
	require.NotContains(t, statusOut, "Start time:", "status must not print a start time for a stopped daemon")
}

// runDaemonCommand runs the daemon binary with a bounded context and returns
// stdout, stderr and the error. A command killed by the deadline reports
// context.DeadlineExceeded through its exit error.
func runDaemonCommand(t *testing.T, binaryPath string, timeout time.Duration, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// socketDaemonPIDs lists the daemon processes whose command line carries
// --socket-path <socketPath>. That is the set `ss -lxp` would attribute to the
// socket, without depending on an external binary.
func socketDaemonPIDs(t *testing.T, socketPath string) []int {
	t.Helper()

	if runtime.GOOS == "linux" {
		if pids, ok := socketDaemonPIDsFromProc(socketPath); ok {
			return pids
		}
	}

	// Portable fallback (macOS and anywhere /proc is absent). pgrep -f matches
	// the whole command line with arguments joined by single spaces, so the
	// pattern must use a space, not the NUL that separates argv in /proc, and it
	// must be regex-quoted: socket paths legitimately contain dots.
	pattern := "--socket-path " + regexp.QuoteMeta(socketPath)
	out, err := exec.Command("pgrep", "-f", "--", pattern).Output()
	if err != nil && len(out) == 0 {
		if _, lookErr := exec.LookPath("pgrep"); lookErr == nil {
			// pgrep exits 1 when nothing matches; that is an empty set.
			return nil
		}
		return nil
	}
	var pids []int
	for _, field := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(field); err == nil && pid != os.Getpid() {
			pids = append(pids, pid)
		}
	}
	return pids
}

// socketDaemonPIDsFromProc scans /proc for daemon processes bound to
// socketPath. The second return value reports whether /proc was readable.
func socketDaemonPIDsFromProc(socketPath string) ([]int, bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, false
	}
	var pids []int
	self := os.Getpid()
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == self {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil || len(data) == 0 {
			continue
		}
		args := strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "--socket-path" && args[i+1] == socketPath {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids, true
}

// socketListenerCount counts listeners bound to socketPath using ss. The second
// return value reports whether the measurement was possible on this host.
func socketListenerCount(t *testing.T, socketPath string) (int, bool) {
	t.Helper()
	if _, err := exec.LookPath("ss"); err != nil {
		return 0, false
	}
	out, err := exec.Command("ss", "-lxpH").Output()
	if err != nil {
		return 0, false
	}
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, socketPath) {
			count++
		}
	}
	return count, true
}
