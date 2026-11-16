package daemon

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// One daemon per socket.
//
// A daemon binds a Unix socket that exactly one process may serve. Before this
// guard existed every start path unlinked whatever socket file sat at the path
// and bound a fresh listener: the second daemon took the path, the first kept
// its now-anonymous listener and lived on as an orphan that no pid file pointed
// at, and `stop` could only ever reap the newest process. `status` then read
// the stale pid file and printed a dead PID next to a zero Start time.
//
// The guard is ONE OS-level lock, taken BEFORE the socket is bound and held for
// the daemon's whole lifetime:
//
//   - Shared: the openapi-daemon binary and the openapi-cli wrapper derive the
//     same lock path from the socket path (SocketLockPath), so either entry
//     point refuses the second start. There is no second copy of the rule.
//   - Not a check-then-bind race: the lock is taken atomically and a start that
//     loses it never reaches the bind. A pid-file liveness check alone could
//     not give that promise.
//   - Named: the holder writes its PID into the lock file, so a refusal can say
//     "daemon already running (PID N)" with the live PID.
//
// The lock file is NEVER unlinked. Removing it while a daemon still holds the
// (then anonymous) inode would let a third start create a fresh inode and bind
// alongside it — the same orphan bug in a new disguise.
const (
	// guardAcquireGrace absorbs a concurrent PROBE. status/stop take the lock
	// for an instant to test it, and a probe must never make a real start look
	// like "already running". A genuine daemon holds the lock forever, so the
	// retries can only ever time out against one.
	guardAcquireGrace     = 150 * time.Millisecond
	guardAcquireRetryStep = 25 * time.Millisecond
	// guardPIDReadGrace covers the window between winning the lock and writing
	// the holder PID into it.
	guardPIDReadGrace = 150 * time.Millisecond
	guardPIDReadStep  = 25 * time.Millisecond
)

// errLockBusy reports that another process holds the guard lock. Each platform
// implementation returns it for its own "would block" condition.
var errLockBusy = errors.New("daemon start guard: lock held by another process")

// SocketLockPath returns the guard-lock path for a daemon socket. It is derived
// from the socket path alone so every entry point agrees on it.
func SocketLockPath(socketPath string) string {
	return socketPath + ".lock"
}

// DefaultPIDFile returns the PID file paired with a socket, matching both the
// daemon binary's default and the CLI wrapper's pidFileForSocket.
func DefaultPIDFile(socketPath string) string {
	return socketPath + ".pid"
}

// AlreadyRunningError reports that another daemon already owns the socket.
// Error() carries the operator-facing wording the CLI wrapper has always used.
type AlreadyRunningError struct {
	PID        int
	SocketPath string
}

func (e *AlreadyRunningError) Error() string {
	if e.PID > 0 {
		return fmt.Sprintf("daemon already running (PID %d)", e.PID)
	}
	return fmt.Sprintf("daemon already running (socket %s is locked by another process)", e.SocketPath)
}

// StartGuard is a held start lock for one daemon socket.
type StartGuard struct {
	file *os.File
	path string
	pid  int
}

var (
	heldGuardsMu sync.Mutex
	// heldGuards keeps every live guard reachable. os.File carries a finalizer
	// that closes its descriptor: a guard living only in a local variable could
	// be collected mid-run, silently dropping the lock and re-opening the orphan
	// window. Registration keeps the descriptor — and so the lock — alive for
	// the daemon's lifetime; Release removes it.
	heldGuards = map[*StartGuard]struct{}{}
)

// AcquireStartGuard takes the exclusive start lock for socketPath BEFORE the
// socket is bound and returns a guard the caller must hold for the process
// lifetime (Release on shutdown). A second start returns *AlreadyRunningError.
func AcquireStartGuard(socketPath, pidFilePath string) (*StartGuard, error) {
	if strings.TrimSpace(socketPath) == "" {
		return nil, errors.New("acquire daemon start guard: socket path is required")
	}
	if pidFilePath == "" {
		pidFilePath = DefaultPIDFile(socketPath)
	}
	lockPath := SocketLockPath(socketPath)

	var (
		file *os.File
		busy bool
		err  error
	)
	deadline := time.Now().Add(guardAcquireGrace)
	for {
		file, busy, err = acquireGuardLock(lockPath, true)
		if err != nil {
			return nil, fmt.Errorf("acquire daemon start guard %s: %w", lockPath, err)
		}
		if !busy || time.Now().After(deadline) {
			break
		}
		time.Sleep(guardAcquireRetryStep)
	}
	if busy {
		return nil, alreadyRunning(lockPath, pidFilePath, socketPath)
	}

	guard := &StartGuard{file: file, path: lockPath, pid: os.Getpid()}
	if err := guard.recordPID(); err != nil {
		_ = releaseGuardLock(file)
		return nil, err
	}

	heldGuardsMu.Lock()
	heldGuards[guard] = struct{}{}
	heldGuardsMu.Unlock()

	return guard, nil
}

// ProbeStartGuard reports whether another daemon already owns the socket,
// without holding the lock beyond that instant. The CLI wrapper calls it before
// spawning the daemon binary: the refusal has to reach the operator's terminal,
// while a spawned daemon's own stderr goes to /dev/null.
func ProbeStartGuard(socketPath, pidFilePath string) error {
	if strings.TrimSpace(socketPath) == "" {
		return errors.New("check daemon state: socket path is required")
	}
	if pidFilePath == "" {
		pidFilePath = DefaultPIDFile(socketPath)
	}
	lockPath := SocketLockPath(socketPath)
	if _, err := os.Stat(lockPath); err != nil {
		// No lock file at all — and it is never unlinked while a daemon runs,
		// so nothing can be holding the lock.
		return nil
	}

	file, busy, err := acquireGuardLock(lockPath, false)
	if err != nil {
		return fmt.Errorf("probe daemon start guard %s: %w", lockPath, err)
	}
	if !busy {
		_ = releaseGuardLock(file)
		return nil
	}
	return alreadyRunning(lockPath, pidFilePath, socketPath)
}

// Release drops the lock. It is idempotent and safe to defer.
func (g *StartGuard) Release() error {
	if g == nil {
		return nil
	}
	heldGuardsMu.Lock()
	delete(heldGuards, g)
	heldGuardsMu.Unlock()

	if g.file == nil {
		return nil
	}
	file := g.file
	g.file = nil
	return releaseGuardLock(file)
}

// PID returns the process that holds the guard.
func (g *StartGuard) PID() int {
	if g == nil {
		return 0
	}
	return g.pid
}

// Path returns the lock file path.
func (g *StartGuard) Path() string {
	if g == nil {
		return ""
	}
	return g.path
}

// recordPID writes this process's PID into the lock file so a refused start can
// name the live daemon.
func (g *StartGuard) recordPID() error {
	if err := g.file.Truncate(0); err != nil {
		return fmt.Errorf("daemon start guard: truncate %s: %w", g.path, err)
	}
	if _, err := g.file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("daemon start guard: seek %s: %w", g.path, err)
	}
	if _, err := fmt.Fprintf(g.file, "%d\n", g.pid); err != nil {
		return fmt.Errorf("daemon start guard: record PID in %s: %w", g.path, err)
	}
	if err := g.file.Sync(); err != nil {
		return fmt.Errorf("daemon start guard: flush %s: %w", g.path, err)
	}
	return nil
}

// RunningDaemonPID returns the PID of the live daemon that owns socketPath.
// The guard lock is authoritative; the pid file is the fallback for a daemon
// started by a build that predates the lock (and for the restart path, whose
// stand-in process does not take one).
func RunningDaemonPID(socketPath, pidFilePath string) (int, bool) {
	if strings.TrimSpace(socketPath) == "" {
		return 0, false
	}
	if pidFilePath == "" {
		pidFilePath = DefaultPIDFile(socketPath)
	}
	if pid := guardHolderPID(socketPath); pid > 0 {
		return pid, true
	}
	if pid := livePIDFromFile(pidFilePath); pid > 0 {
		return pid, true
	}
	return 0, false
}

// StopRunningDaemon reaps the daemon owning socketPath and removes the pid file
// and socket path. It returns the PID that was signalled and whether anything
// was running. Only a PID recorded by this daemon's own lock or pid file is
// ever signalled — never a guess, and never the calling process.
func StopRunningDaemon(socketPath, pidFilePath string, timeout time.Duration) (int, bool, error) {
	if strings.TrimSpace(socketPath) == "" {
		return 0, false, errors.New("stop daemon: socket path is required")
	}
	if pidFilePath == "" {
		pidFilePath = DefaultPIDFile(socketPath)
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	pid, running := RunningDaemonPID(socketPath, pidFilePath)
	if !running {
		// Nothing recorded is alive. A socket that still ANSWERS is owned by a
		// process we cannot name: fail closed rather than unlink a live
		// listener and hand the next start an orphan to fight with.
		if socketInUse(socketPath) {
			return 0, false, fmt.Errorf("socket %s is served by a process that holds no daemon lock or pid file; refusing to remove it", socketPath)
		}
		_ = os.Remove(pidFilePath)
		_ = os.Remove(socketPath)
		return 0, false, nil
	}
	if pid == os.Getpid() {
		return pid, false, fmt.Errorf("refusing to signal this process (PID %d)", pid)
	}
	if err := terminatePID(pid); err != nil {
		return pid, true, fmt.Errorf("send SIGTERM to PID %d: %w", pid, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			break
		}
		time.Sleep(guardAcquireRetryStep)
	}
	if processAlive(pid) {
		return pid, true, fmt.Errorf("daemon (PID %d) did not exit after SIGTERM", pid)
	}

	_ = os.Remove(pidFilePath)
	_ = os.Remove(socketPath)
	return pid, true, nil
}

// socketInUse reports whether a process is accepting connections on the Unix
// socket at path. A stale socket file refuses the dial; a live listener
// accepts it.
func socketInUse(path string) bool {
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	conn, err := net.DialTimeout("unix", path, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// guardHolderPID returns the PID of the process holding the start lock, or 0
// when the lock is free (or its recorded PID is not a live process).
func guardHolderPID(socketPath string) int {
	lockPath := SocketLockPath(socketPath)
	if _, err := os.Stat(lockPath); err != nil {
		return 0
	}
	file, busy, err := acquireGuardLock(lockPath, false)
	if err != nil {
		return 0
	}
	if !busy {
		_ = releaseGuardLock(file)
		return 0
	}
	if pid := readPIDFromFile(lockPath); pid > 0 && processAlive(pid) {
		return pid
	}
	return 0
}

// alreadyRunning builds the refusal, naming the live PID when one can be read.
func alreadyRunning(lockPath, pidFilePath, socketPath string) error {
	pid := ownerPID(lockPath)
	if pid <= 0 {
		pid = livePIDFromFile(pidFilePath)
	}
	return &AlreadyRunningError{PID: pid, SocketPath: socketPath}
}

// ownerPID reads the PID the lock holder recorded. The read can land in the
// window between winning the lock and writing the PID, so it retries briefly;
// a stale PID (the holder died holding nothing) is not reported.
func ownerPID(lockPath string) int {
	deadline := time.Now().Add(guardPIDReadGrace)
	for {
		if pid := readPIDFromFile(lockPath); pid > 0 && processAlive(pid) {
			return pid
		}
		if time.Now().After(deadline) {
			return 0
		}
		time.Sleep(guardPIDReadStep)
	}
}

// readPIDFromFile parses a PID from path, returning 0 for a missing file or
// unparsable content.
func readPIDFromFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}

// livePIDFromFile returns the PID recorded in path when that process is alive.
func livePIDFromFile(path string) int {
	pid := readPIDFromFile(path)
	if pid > 0 && processAlive(pid) {
		return pid
	}
	return 0
}
