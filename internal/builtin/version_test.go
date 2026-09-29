package builtin

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/internal/version"
)

// captureStdout runs fn while capturing os.Stdout, returns the captured string
func captureStdout(fn func() error) (string, error) {
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

func TestNewVersionCommand(t *testing.T) {
	t.Run("command structure", func(t *testing.T) {
		cmd := newVersionCommand()
		require.NotNil(t, cmd)

		assert.Equal(t, "version", cmd.Name())
		assert.Equal(t, "Print version information", cmd.Short)
		assert.Contains(t, cmd.Long, "detailed version information")
	})

	t.Run("has --output flag", func(t *testing.T) {
		cmd := newVersionCommand()
		flag := cmd.Flags().Lookup("output")
		require.NotNil(t, flag, "--output flag should exist")
		assert.Equal(t, "output", flag.Name)
	})
}

func TestNewVersionCommand_Default(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newVersionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"version"})

	output, err := captureStdout(func() error {
		return root.Execute()
	})

	require.NoError(t, err)
	info := version.Get()
	expected := info.Short()
	assert.Contains(t, output, expected)
	assert.NotEmpty(t, output)
}

func TestNewVersionCommand_JSON(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newVersionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"version", "--output", "json"})

	output, err := captureStdout(func() error {
		return root.Execute()
	})

	require.NoError(t, err)
	assert.NotEmpty(t, output)

	var parsed version.Info
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err, "output should be valid JSON")
	assert.NotEmpty(t, parsed.Version)
}

func TestNewVersionCommand_JSONExplicitTrue(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newVersionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"version", "--output=true"})

	output, err := captureStdout(func() error {
		return root.Execute()
	})

	require.NoError(t, err)
	assert.NotEmpty(t, output)

	var parsed version.Info
	err = json.Unmarshal([]byte(output), &parsed)
	require.NoError(t, err, "output should be valid JSON for --output=true")
}

func TestNewVersionCommand_JSONExplicitFalse(t *testing.T) {
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newVersionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"version", "--output=false"})

	output, err := captureStdout(func() error {
		return root.Execute()
	})

	require.NoError(t, err)
	info := version.Get()
	expected := info.Short()
	assert.Contains(t, output, expected, "--output=false should print text, not JSON")
}

func TestNewVersionCommand_NeverPrintsEmptyVersion(t *testing.T) {
	// Wiring-level contract: the version surface always carries a non-empty
	// Version token — a stamped release value, or the documented bare-build
	// "dev" fallback (see internal/version.Get stamp normalization). A
	// release-stamped build must never print an empty version like " ()".
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newVersionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"version", "--output", "json"})

	output, err := captureStdout(func() error {
		return root.Execute()
	})

	require.NoError(t, err)
	var parsed version.Info
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	assert.NotEmpty(t, parsed.Version,
		"version output must never be empty; internal/version.Get must normalize empty build stamps")
}

func TestNewVersionCommand_ExtraArgs(t *testing.T) {
	// Note: newVersionCommand does not set cobra.Args validator,
	// so extra args are silently ignored — not an error.
	root := &cobra.Command{Use: "openapi-cli"}
	cmd := newVersionCommand()
	root.AddCommand(cmd)
	root.SetArgs([]string{"version", "extra"})

	output, err := captureStdout(func() error {
		return root.Execute()
	})

	require.NoError(t, err)
	info := version.Get()
	expected := info.Short()
	assert.Contains(t, output, expected, "extra args are silently ignored; version still prints")
}
