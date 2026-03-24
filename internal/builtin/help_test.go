package builtin

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewHelpCommand(t *testing.T) {
	t.Run("creates help command", func(t *testing.T) {
		cmd := newHelpCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "help", cmd.Name())
		assert.Equal(t, "Help about any command", cmd.Short)
		assert.Contains(t, cmd.Long, "Help provides help for any command")
	})

	t.Run("has correct usage", func(t *testing.T) {
		cmd := newHelpCommand()
		assert.Equal(t, "help [command]", cmd.Use)
	})

	t.Run("executes without error", func(t *testing.T) {
		cmd := newHelpCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		cmd.Run(cmd, []string{})

		output := buf.String()
		assert.NotEmpty(t, output)
		assert.Contains(t, output, "help")
	})
}
