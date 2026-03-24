package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDaemonCommand(t *testing.T) {
	t.Run("creates daemon command", func(t *testing.T) {
		cmd := newDaemonCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "daemon", cmd.Name())
		assert.Equal(t, "Daemon management commands", cmd.Short)
		assert.Contains(t, cmd.Long, "Manage the daemon process")
	})

	t.Run("has start subcommand", func(t *testing.T) {
		cmd := newDaemonCommand()

		hasStart := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "start" {
				hasStart = true
				break
			}
		}
		assert.True(t, hasStart)
	})

	t.Run("has stop subcommand", func(t *testing.T) {
		cmd := newDaemonCommand()

		hasStop := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "stop" {
				hasStop = true
				break
			}
		}
		assert.True(t, hasStop)
	})

	t.Run("has status subcommand", func(t *testing.T) {
		cmd := newDaemonCommand()

		hasStatus := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "status" {
				hasStatus = true
				break
			}
		}
		assert.True(t, hasStatus)
	})
}

func TestNewDaemonStartCommand(t *testing.T) {
	t.Run("creates start command", func(t *testing.T) {
		cmd := newDaemonStartCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "start", cmd.Name())
		assert.Equal(t, "Start the daemon", cmd.Short)
	})

	t.Run("has socket flag", func(t *testing.T) {
		cmd := newDaemonStartCommand()

		flag := cmd.Flags().Lookup("socket")
		assert.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("has default socket behavior", func(t *testing.T) {
		cmd := newDaemonStartCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "start", cmd.Name())
	})

	t.Run("accepts custom socket flag", func(t *testing.T) {
		cmd := newDaemonStartCommand()
		cmd.Flags().Set("socket", "/tmp/test.sock")

		socketFlag := cmd.Flags().Lookup("socket")
		assert.NotNil(t, socketFlag)
		assert.Equal(t, "/tmp/test.sock", socketFlag.Value.String())
	})
}

func TestNewDaemonStopCommand(t *testing.T) {
	t.Run("creates stop command", func(t *testing.T) {
		cmd := newDaemonStopCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "stop", cmd.Name())
		assert.Equal(t, "Stop the daemon", cmd.Short)
	})

	t.Run("has socket flag", func(t *testing.T) {
		cmd := newDaemonStopCommand()

		flag := cmd.Flags().Lookup("socket")
		assert.NotNil(t, flag)
	})

	t.Run("command structure validation", func(t *testing.T) {
		cmd := newDaemonStopCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "stop", cmd.Name())
		assert.Equal(t, "Stop the daemon", cmd.Short)
		assert.NotNil(t, cmd.RunE)
	})

	t.Run("has socket flag", func(t *testing.T) {
		cmd := newDaemonStopCommand()

		flag := cmd.Flags().Lookup("socket")
		assert.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})
}

func TestNewDaemonStatusCommand(t *testing.T) {
	t.Run("creates status command", func(t *testing.T) {
		cmd := newDaemonStatusCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "status", cmd.Name())
		assert.Equal(t, "Show daemon status", cmd.Short)
	})

	t.Run("has socket flag", func(t *testing.T) {
		cmd := newDaemonStatusCommand()

		flag := cmd.Flags().Lookup("socket")
		assert.NotNil(t, flag)
	})

	t.Run("command structure validation", func(t *testing.T) {
		cmd := newDaemonStatusCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "status", cmd.Name())
		assert.Equal(t, "Show daemon status", cmd.Short)
		assert.NotNil(t, cmd.RunE)
	})
}

func TestDaemonCommandStructure(t *testing.T) {
	t.Run("daemon command has three subcommands", func(t *testing.T) {
		cmd := newDaemonCommand()

		assert.Equal(t, 3, len(cmd.Commands()))
	})

	t.Run("all subcommands have descriptions", func(t *testing.T) {
		daemonCmd := newDaemonCommand()

		for _, subCmd := range daemonCmd.Commands() {
			assert.NotEmpty(t, subCmd.Short, "subcommand %s missing Short", subCmd.Name())
			assert.NotEmpty(t, subCmd.Long, "subcommand %s missing Long", subCmd.Name())
		}
	})

	t.Run("all subcommands have RunE functions", func(t *testing.T) {
		daemonCmd := newDaemonCommand()

		for _, subCmd := range daemonCmd.Commands() {
			assert.NotNil(t, subCmd.RunE, "subcommand %s missing RunE", subCmd.Name())
		}
	})
}
