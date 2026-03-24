package builtin

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCompletionCommand(t *testing.T) {
	t.Run("creates completion command", func(t *testing.T) {
		cmd := newCompletionCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "completion", cmd.Name())
		assert.Equal(t, "Generate shell completion scripts", cmd.Short)
		assert.Contains(t, cmd.Long, "To load completions:")
	})

	t.Run("has correct usage", func(t *testing.T) {
		cmd := newCompletionCommand()

		assert.Equal(t, "completion [bash|zsh|fish|powershell]", cmd.Use)
	})

	t.Run("disables flags in use line", func(t *testing.T) {
		cmd := newCompletionCommand()

		assert.True(t, cmd.DisableFlagsInUseLine)
	})

	t.Run("has valid args", func(t *testing.T) {
		cmd := newCompletionCommand()

		expectedArgs := []string{"bash", "zsh", "fish", "powershell"}
		assert.Equal(t, expectedArgs, cmd.ValidArgs)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.Args(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	})

	t.Run("rejects unsupported shell", func(t *testing.T) {
		cmd := newCompletionCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		err := cmd.RunE(cmd, []string{"invalid"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported shell type")
	})

	t.Run("generates bash completion", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.RunE(cmd, []string{"bash"})

		require.NoError(t, err)
	})

	t.Run("generates zsh completion", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.RunE(cmd, []string{"zsh"})

		require.NoError(t, err)
	})

	t.Run("generates fish completion", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.RunE(cmd, []string{"fish"})

		require.NoError(t, err)
	})

	t.Run("generates powershell completion", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.RunE(cmd, []string{"powershell"})

		require.NoError(t, err)
	})

	t.Run("long description contains bash instructions", func(t *testing.T) {
		cmd := newCompletionCommand()

		assert.Contains(t, cmd.Long, "Bash:")
		assert.Contains(t, cmd.Long, "source <(openapi-cli completion bash)")
	})

	t.Run("long description contains zsh instructions", func(t *testing.T) {
		cmd := newCompletionCommand()

		assert.Contains(t, cmd.Long, "Zsh:")
		assert.Contains(t, cmd.Long, "compinit")
	})

	t.Run("long description contains fish instructions", func(t *testing.T) {
		cmd := newCompletionCommand()

		assert.Contains(t, cmd.Long, "fish:")
		assert.Contains(t, cmd.Long, ".config/fish/completions")
	})

	t.Run("long description contains powershell instructions", func(t *testing.T) {
		cmd := newCompletionCommand()

		assert.Contains(t, cmd.Long, "PowerShell:")
		assert.Contains(t, cmd.Long, "Invoke-Expression")
	})
}

func TestCompletionCommandArgValidation(t *testing.T) {
	t.Run("accepts bash", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.Args(cmd, []string{"bash"})

		assert.NoError(t, err)
	})

	t.Run("accepts zsh", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.Args(cmd, []string{"zsh"})

		assert.NoError(t, err)
	})

	t.Run("accepts fish", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.Args(cmd, []string{"fish"})

		assert.NoError(t, err)
	})

	t.Run("accepts powershell", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.Args(cmd, []string{"powershell"})

		assert.NoError(t, err)
	})

	t.Run("rejects sh", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.Args(cmd, []string{"sh"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid argument")
	})

	t.Run("rejects multiple arguments", func(t *testing.T) {
		cmd := newCompletionCommand()

		err := cmd.Args(cmd, []string{"bash", "zsh"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	})
}
