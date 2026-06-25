package builtin

import (
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
	// Stop errors when daemon isn't running (expected behavior)
	cmd := newDaemonStopCommand()
	err := cmd.Execute()
	assert.Error(t, err, "stop should error when daemon is not running")
}

// Prevent unused import
var _ = cobra.Command{}
