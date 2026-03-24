package builtin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewVersionCommand(t *testing.T) {
	t.Run("creates version command", func(t *testing.T) {
		cmd := newVersionCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "version", cmd.Name())
		assert.Equal(t, "Print version information", cmd.Short)
		assert.Contains(t, cmd.Long, "Print detailed version information")
	})

	t.Run("has correct usage", func(t *testing.T) {
		cmd := newVersionCommand()
		assert.Equal(t, "version", cmd.Use)
	})

	t.Run("executes without error", func(t *testing.T) {
		cmd := newVersionCommand()

		cmd.Run(cmd, []string{})
	})

	t.Run("no arguments required", func(t *testing.T) {
		cmd := newVersionCommand()
		assert.Empty(t, cmd.Args)
	})
}
