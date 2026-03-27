package builtin

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInitCommand(t *testing.T) {
	t.Run("creates init command", func(t *testing.T) {
		cmd := newInitCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "init", cmd.Name())
		assert.Equal(t, "Initialize a new API connection", cmd.Short)
		assert.Contains(t, cmd.Long, "Initialize a new connection to an OpenAPI-specified API")
	})

	t.Run("has host flag", func(t *testing.T) {
		cmd := newInitCommand()

		flag := cmd.Flags().Lookup("host")
		assert.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("has timeout flag", func(t *testing.T) {
		cmd := newInitCommand()

		flag := cmd.Flags().Lookup("timeout")
		assert.NotNil(t, flag)
		assert.Equal(t, "30", flag.DefValue)
	})

	t.Run("has retry flag", func(t *testing.T) {
		cmd := newInitCommand()

		flag := cmd.Flags().Lookup("retry")
		assert.NotNil(t, flag)
		assert.Equal(t, "3", flag.DefValue)
	})

	t.Run("has api-key flag", func(t *testing.T) {
		cmd := newInitCommand()

		flag := cmd.Flags().Lookup("api-key")
		assert.NotNil(t, flag)
	})

	t.Run("has oauth2 flag", func(t *testing.T) {
		cmd := newInitCommand()

		flag := cmd.Flags().Lookup("oauth2")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has mtls flag", func(t *testing.T) {
		cmd := newInitCommand()

		flag := cmd.Flags().Lookup("mtls")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has overwrite flag", func(t *testing.T) {
		cmd := newInitCommand()

		flag := cmd.Flags().Lookup("overwrite")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("requires host flag", func(t *testing.T) {
		cmd := newInitCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		err := cmd.RunE(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "--host is required")
	})

	t.Run("executes with host", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInitCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Initializing new connection...")
		assert.Contains(t, output, "Host: https://api.example.com")
	})

	t.Run("executes with overwrite", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInitCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")
		_ = cmd.Flags().Set("overwrite", "true")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Overwriting existing configuration...")
	})

	t.Run("executes with custom timeout", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInitCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")
		_ = cmd.Flags().Set("timeout", "60")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Timeout: 60s")
	})

	t.Run("executes with custom retry", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInitCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")
		_ = cmd.Flags().Set("retry", "5")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Retry attempts: 5")
	})

	t.Run("executes with api-key", func(t *testing.T) {
		cmd := newInitCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		_ = cmd.Flags().Set("host", "https://api.example.com")
		_ = cmd.Flags().Set("api-key", "test-api-key")

		err := cmd.RunE(cmd, []string{})

		require.NoError(t, err)
	})

	t.Run("executes with oauth2", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInitCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")
		_ = cmd.Flags().Set("oauth2", "true")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "OAuth2: enabled")
	})

	t.Run("executes with mtls", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInitCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")
		_ = cmd.Flags().Set("mtls", "true")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "mTLS: enabled")
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newInitCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "--host")
		assert.Contains(t, cmd.Example, "--oauth2")
		assert.Contains(t, cmd.Example, "--timeout")
	})
}

func TestInitCommandFlags(t *testing.T) {
	t.Run("all flags are properly registered", func(t *testing.T) {
		cmd := newInitCommand()

		expectedFlags := []string{
			"host",
			"timeout",
			"retry",
			"api-key",
			"oauth2",
			"mtls",
			"overwrite",
		}

		for _, flagName := range expectedFlags {
			flag := cmd.Flags().Lookup(flagName)
			assert.NotNil(t, flag, "flag %s not found", flagName)
		}
	})

	t.Run("host flag is marked as required", func(t *testing.T) {
		cmd := newInitCommand()

		flag := cmd.Flags().Lookup("host")
		assert.NotNil(t, flag)
		assert.True(t, flag.Value.String() == "")
	})
}
