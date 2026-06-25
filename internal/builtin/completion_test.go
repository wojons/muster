package builtin

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureStdoutCompletion runs fn while capturing os.Stdout, returns the captured string
func captureStdoutCompletion(fn func() error) (string, error) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := fn()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String(), err
}

func TestNewCompletionCommand(t *testing.T) {
	t.Run("command structure", func(t *testing.T) {
		cmd := newCompletionCommand()
		require.NotNil(t, cmd)

		assert.Equal(t, "completion [bash|zsh|fish|powershell]", cmd.Use)
		assert.Equal(t, "Generate shell completion scripts", cmd.Short)
		assert.NotEmpty(t, cmd.Long)
		assert.True(t, cmd.DisableFlagsInUseLine)
	})

	t.Run("ValidArgs", func(t *testing.T) {
		cmd := newCompletionCommand()
		assert.Equal(t, []string{"bash", "zsh", "fish", "powershell"}, cmd.ValidArgs)
	})

	t.Run("Args validation", func(t *testing.T) {
		cmd := newCompletionCommand()
		assert.NotNil(t, cmd.Args)
	})
}

func TestNewCompletionCommand_Bash(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newCompletionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"completion", "bash"})

	output, err := captureStdoutCompletion(func() error {
		return root.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "bash completion")
	assert.NotEmpty(t, output)
}

func TestNewCompletionCommand_Zsh(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newCompletionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"completion", "zsh"})

	output, err := captureStdoutCompletion(func() error {
		return root.Execute()
	})

	require.NoError(t, err)
	assert.Contains(t, output, "compdef")
	assert.NotEmpty(t, output)
}

func TestNewCompletionCommand_Fish(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newCompletionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"completion", "fish"})

	output, err := captureStdoutCompletion(func() error {
		return root.Execute()
	})

	require.NoError(t, err)
	assert.NotEmpty(t, output)
}

func TestNewCompletionCommand_Powershell(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newCompletionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"completion", "powershell"})

	output, err := captureStdoutCompletion(func() error {
		return root.Execute()
	})

	require.NoError(t, err)
	assert.NotEmpty(t, output)
}

func TestNewCompletionCommand_InvalidShell(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newCompletionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"completion", "invalid"})

	err := root.Execute()
	// Cobra's OnlyValidArgs catches "invalid" before RunE — expects error
	require.Error(t, err)
}

func TestNewCompletionCommand_NoArgs(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newCompletionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"completion"})

	err := root.Execute()
	assert.Error(t, err)
}

func TestNewCompletionCommand_ExtraArgs(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newCompletionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"completion", "bash", "extra"})

	err := root.Execute()
	assert.Error(t, err)
}
