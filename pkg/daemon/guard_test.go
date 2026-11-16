package daemon

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The guard is the one rule that keeps a second daemon from stealing a live
// socket. These tests pin its contract directly (QA-MUSTER-035).

func TestSocketLockPathAndPIDFileDerivation(t *testing.T) {
	require.Equal(t, "/tmp/openapi-cli.sock.lock", SocketLockPath("/tmp/openapi-cli.sock"))
	require.Equal(t, "/tmp/openapi-cli.sock.pid", DefaultPIDFile("/tmp/openapi-cli.sock"))
	require.Equal(t, "<socket>.lock", filepath.Base(SocketLockPath("<socket>")))
}

func TestStartGuard_RefusesSecondAcquire(t *testing.T) {
	socketPath := filepath.Join(socketTestDir(t), "guard.sock")
	pidPath := DefaultPIDFile(socketPath)

	first, err := AcquireStartGuard(socketPath, pidPath)
	require.NoError(t, err)
	defer func() { _ = first.Release() }()

	require.Equal(t, os.Getpid(), first.PID())
	require.Equal(t, SocketLockPath(socketPath), first.Path())

	// The guard must not write the PID file: recording the daemon's PID is the
	// daemon's job, and a guard that wrote it would race a refused sibling.
	require.NoFileExists(t, pidPath, "the guard must not create the PID file")

	// A second start in the same process (a different open file description,
	// so flock conflicts exactly as it does between processes) is refused.
	second, err := AcquireStartGuard(socketPath, pidPath)
	require.Error(t, err)
	require.Nil(t, second)

	var already *AlreadyRunningError
	require.ErrorAs(t, err, &already)
	require.Equal(t, os.Getpid(), already.PID)
	require.Contains(t, err.Error(), "daemon already running (PID ")
}

func TestStartGuard_ProbeReflectsHolder(t *testing.T) {
	socketPath := filepath.Join(socketTestDir(t), "probe.sock")
	pidPath := DefaultPIDFile(socketPath)

	// Nothing held yet: the probe is clean even before the lock file exists.
	require.NoError(t, ProbeStartGuard(socketPath, pidPath))

	guard, err := AcquireStartGuard(socketPath, pidPath)
	require.NoError(t, err)

	probeErr := ProbeStartGuard(socketPath, pidPath)
	require.Error(t, probeErr, "a held lock must be visible to the probe")
	require.Contains(t, probeErr.Error(), "daemon already running (PID ")

	require.NoError(t, guard.Release())

	require.NoError(t, ProbeStartGuard(socketPath, pidPath), "a released lock must probe clean")
	require.FileExists(t, SocketLockPath(socketPath), "the lock file itself is never unlinked")

	reacquired, err := AcquireStartGuard(socketPath, pidPath)
	require.NoError(t, err, "the lock must be reusable after release")
	require.NoError(t, reacquired.Release())
}

func TestStartGuard_RejectsEmptySocketPath(t *testing.T) {
	_, err := AcquireStartGuard("", "")
	require.Error(t, err)
	_, err = AcquireStartGuard("   ", "")
	require.Error(t, err)
}

func TestRunningDaemonPID_ReportsGuardHolder(t *testing.T) {
	socketPath := filepath.Join(socketTestDir(t), "running.sock")
	pidPath := DefaultPIDFile(socketPath)

	guard, err := AcquireStartGuard(socketPath, pidPath)
	require.NoError(t, err)
	defer func() { _ = guard.Release() }()

	pid, running := RunningDaemonPID(socketPath, pidPath)
	require.True(t, running)
	require.Equal(t, os.Getpid(), pid)
}

func TestRunningDaemonPID_IgnoresStalePIDFile(t *testing.T) {
	socketPath := filepath.Join(socketTestDir(t), "stale.sock")
	pidPath := DefaultPIDFile(socketPath)
	require.NoError(t, os.WriteFile(pidPath, []byte("1073741824\n"), 0o600))

	_, running := RunningDaemonPID(socketPath, pidPath)
	require.False(t, running, "a PID that is not alive must not count as running")
}

func TestStopRunningDaemon_RefusesToSignalSelf(t *testing.T) {
	socketPath := filepath.Join(socketTestDir(t), "self.sock")
	pidPath := DefaultPIDFile(socketPath)

	guard, err := AcquireStartGuard(socketPath, pidPath)
	require.NoError(t, err)
	defer func() { _ = guard.Release() }()

	_, _, err = StopRunningDaemon(socketPath, pidPath, time.Second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refusing to signal this process")
	require.NoFileExists(t, pidPath, "stop must not write a PID file it did not find")
}

func TestStopRunningDaemon_CleansStaleSocket(t *testing.T) {
	socketPath := filepath.Join(socketTestDir(t), "stale-file.sock")
	pidPath := DefaultPIDFile(socketPath)
	require.NoError(t, os.WriteFile(socketPath, []byte("stale"), 0o600))
	require.NoError(t, os.WriteFile(pidPath, []byte("1073741824\n"), 0o600))

	pid, running, err := StopRunningDaemon(socketPath, pidPath, time.Second)
	require.NoError(t, err)
	require.False(t, running)
	require.Zero(t, pid)
	require.NoFileExists(t, socketPath)
	require.NoFileExists(t, pidPath)
}

func TestStopRunningDaemon_FailsClosedOnUnattributedListener(t *testing.T) {
	socketPath := filepath.Join(socketTestDir(t), "live.sock")
	pidPath := DefaultPIDFile(socketPath)

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()

	// A listener with no lock and no PID file belongs to somebody we cannot
	// name. Unlinking it would orphan that process, so stop must fail closed.
	_, _, err = StopRunningDaemon(socketPath, pidPath, time.Second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "holds no daemon lock or pid file")
	require.FileExists(t, socketPath, "the live socket must not be removed")
}
