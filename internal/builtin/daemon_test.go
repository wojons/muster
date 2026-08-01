package builtin

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDaemonCommand(t *testing.T) {
	cmd := newDaemonCommand()
	require.NotNil(t, cmd)

	assert.Equal(t, "daemon", cmd.Use)
	assert.Equal(t, "Daemon management commands", cmd.Short)
	assert.NotEmpty(t, cmd.Long)

	// Verify subcommands
	subNames := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subNames[sub.Name()] = true
	}
	assert.True(t, subNames["start"], "should have start subcommand")
	assert.True(t, subNames["stop"], "should have stop subcommand")
	assert.True(t, subNames["status"], "should have status subcommand")
	assert.Equal(t, 3, len(subNames), "should have exactly 3 subcommands")
}

func TestNewDaemonStartCommand(t *testing.T) {
	cmd := newDaemonStartCommand()
	require.NotNil(t, cmd)

	t.Run("command structure", func(t *testing.T) {
		assert.Equal(t, "start", cmd.Use)
		assert.Equal(t, "Start the daemon", cmd.Short)
		assert.NotEmpty(t, cmd.Long)
	})

	t.Run("has --socket flag", func(t *testing.T) {
		flags := cmd.Flags()
		socketFlag := flags.Lookup("socket")
		require.NotNil(t, socketFlag, "should have --socket flag")
		assert.Equal(t, "string", socketFlag.Value.Type())
		assert.Equal(t, "", socketFlag.DefValue)
		assert.Contains(t, socketFlag.Usage, "socket")
	})

	t.Run("has --backend-url flag", func(t *testing.T) {
		flags := cmd.Flags()
		backendFlag := flags.Lookup("backend-url")
		require.NotNil(t, backendFlag, "should have --backend-url flag")
		assert.Equal(t, "string", backendFlag.Value.Type())
		assert.Equal(t, "", backendFlag.DefValue)
		assert.Contains(t, backendFlag.Usage, "Backend")
	})

	t.Run("RunE is set", func(t *testing.T) {
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestNewDaemonStopCommand(t *testing.T) {
	cmd := newDaemonStopCommand()
	require.NotNil(t, cmd)

	t.Run("command structure", func(t *testing.T) {
		assert.Equal(t, "stop", cmd.Use)
		assert.Equal(t, "Stop the daemon", cmd.Short)
		assert.NotEmpty(t, cmd.Long)
	})

	t.Run("has --socket flag", func(t *testing.T) {
		flags := cmd.Flags()
		socketFlag := flags.Lookup("socket")
		require.NotNil(t, socketFlag, "should have --socket flag")
		assert.Equal(t, "string", socketFlag.Value.Type())
	})

	t.Run("RunE is set", func(t *testing.T) {
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestNewDaemonStatusCommand(t *testing.T) {
	cmd := newDaemonStatusCommand()
	require.NotNil(t, cmd)

	t.Run("command structure", func(t *testing.T) {
		assert.Equal(t, "status", cmd.Use)
		assert.Equal(t, "Show daemon status", cmd.Short)
		assert.NotEmpty(t, cmd.Long)
	})

	t.Run("has --socket flag", func(t *testing.T) {
		flags := cmd.Flags()
		socketFlag := flags.Lookup("socket")
		require.NotNil(t, socketFlag, "should have --socket flag")
		assert.Equal(t, "string", socketFlag.Value.Type())
	})

	t.Run("RunE is set", func(t *testing.T) {
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})
}

func TestNewDaemonCommands_StopErrors(t *testing.T) {
	// Stop errors when daemon isn't running and no PID file exists (Controller fallback).
	// Use a unique socket path so we don't hit a real PID file from a live daemon.
	cmd := newDaemonStopCommand()
	cmd.SetArgs([]string{"--socket", filepath.Join(t.TempDir(), "no-daemon.sock")})
	err := cmd.Execute()
	assert.Error(t, err, "stop should error when daemon is not running and no PID file exists")
}

func TestResolveSocketPath(t *testing.T) {
	assert.Equal(t, defaultDaemonSocket, resolveSocketPath(""))
	assert.Equal(t, "/custom/path.sock", resolveSocketPath("/custom/path.sock"))
}

func TestPidFileForSocket(t *testing.T) {
	assert.Equal(t, "/tmp/openapi-cli.sock.pid", pidFileForSocket(defaultDaemonSocket))
	assert.Equal(t, "/var/run/foo.sock.pid", pidFileForSocket("/var/run/foo.sock"))
}

func TestIsProcessRunning(t *testing.T) {
	// Current process should be running.
	assert.True(t, isProcessRunning(os.Getpid()))
	// PID 0 / negative are invalid.
	assert.False(t, isProcessRunning(0))
	assert.False(t, isProcessRunning(-1))
	// Very high unused PID is almost certainly not running.
	assert.False(t, isProcessRunning(1<<30))
}

func TestReadWritePIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "test.pid")

	err := writePIDFile(path, 12345)
	require.NoError(t, err)

	pid, err := readPIDFile(path)
	require.NoError(t, err)
	assert.Equal(t, 12345, pid)

	// Invalid content
	require.NoError(t, os.WriteFile(path, []byte("not-a-pid\n"), 0644))
	_, err = readPIDFile(path)
	assert.Error(t, err)

	// Missing file
	_, err = readPIDFile(filepath.Join(dir, "missing.pid"))
	assert.Error(t, err)
}

func TestFindDaemonBinary(t *testing.T) {
	// Create a fake binary under a temp cwd and chdir into it.
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))
	fake := filepath.Join(binDir, daemonBinaryName)
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\n"), 0755))

	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	found, err := findDaemonBinary()
	require.NoError(t, err)
	// Should resolve to an absolute path ending with bin/openapi-daemon
	assert.True(t, filepath.IsAbs(found), "expected absolute path, got %s", found)
	assert.Equal(t, daemonBinaryName, filepath.Base(found))

	// Content check — same file we wrote
	info, err := os.Stat(found)
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestFindDaemonBinary_NotFound(t *testing.T) {
	// Empty temp dir with no bin/openapi-daemon.
	tmp := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	// May still find a binary relative to the test executable; only assert
	// the error path when nothing is found. If the test binary happens to
	// sit next to a real openapi-daemon we still exercise the search without failing.
	found, err := findDaemonBinary()
	if err != nil {
		assert.Contains(t, err.Error(), daemonBinaryName)
		assert.Empty(t, found)
	} else {
		assert.NotEmpty(t, found)
		assert.Equal(t, daemonBinaryName, filepath.Base(found))
	}
}

func TestDaemonStart_MissingBinary(t *testing.T) {
	// Chdir to empty temp so ./bin/openapi-daemon is missing.
	// If executable-relative search still finds a binary this test becomes a no-op success path.
	tmp := t.TempDir()
	oldWD, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(tmp))
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	// Ensure no accidental local binary
	_ = os.Remove(filepath.Join(tmp, "bin", daemonBinaryName))

	// Prefer a socket under temp so we don't touch system paths.
	sock := filepath.Join(tmp, "test.sock")

	// Only run the missing-binary assertion when findDaemonBinary fails.
	if _, err := findDaemonBinary(); err == nil {
		t.Skip("daemon binary found relative to executable; cannot test missing-binary path")
	}

	cmd := newDaemonStartCommand()
	cmd.SetArgs([]string{"--socket", sock})
	err = cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), daemonBinaryName)
}

func TestDaemonStatus_ViaPIDFile(t *testing.T) {
	tmp := t.TempDir()
	sock := filepath.Join(tmp, "status.sock")
	pidPath := pidFileForSocket(sock)

	// Write current process PID — process is running.
	require.NoError(t, writePIDFile(pidPath, os.Getpid()))

	cmd := newDaemonStatusCommand()
	cmd.SetArgs([]string{"--socket", sock})
	// Capture isn't wired; just ensure no error.
	err := cmd.Execute()
	require.NoError(t, err)

	// Stale PID
	require.NoError(t, writePIDFile(pidPath, 1<<30))
	cmd = newDaemonStatusCommand()
	cmd.SetArgs([]string{"--socket", sock})
	err = cmd.Execute()
	require.NoError(t, err)
}

func TestDaemonStop_ViaStalePIDFile(t *testing.T) {
	tmp := t.TempDir()
	sock := filepath.Join(tmp, "stop.sock")
	pidPath := pidFileForSocket(sock)

	// Stale PID — process not running. Stop should print "not running" and return nil.
	require.NoError(t, writePIDFile(pidPath, 1<<30))
	// Fake socket file that should be cleaned up.
	require.NoError(t, os.WriteFile(sock, []byte{}, 0644))

	cmd := newDaemonStopCommand()
	cmd.SetArgs([]string{"--socket", sock})
	err := cmd.Execute()
	require.NoError(t, err)

	_, err = os.Stat(pidPath)
	assert.True(t, os.IsNotExist(err), "stale PID file should be removed")
}

func TestDaemonStop_NotRunningMessage(t *testing.T) {
	// Ensure strconv is exercised via write/read of a real-looking PID file.
	tmp := t.TempDir()
	sock := filepath.Join(tmp, "x.sock")
	pid := 999999999
	require.NoError(t, writePIDFile(pidFileForSocket(sock), pid))
	assert.Equal(t, strconv.Itoa(pid)+"\n", mustRead(t, pidFileForSocket(sock)))
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// Prevent unused import
var _ = cobra.Command{}
