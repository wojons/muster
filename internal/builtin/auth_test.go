package builtin

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAuthCommand(t *testing.T) {
	t.Run("creates auth command", func(t *testing.T) {
		cmd := newAuthCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "auth", cmd.Name())
		assert.Equal(t, "Authentication commands", cmd.Short)
		assert.Contains(t, cmd.Long, "Manage authentication for API connections")
	})

	t.Run("has login subcommand", func(t *testing.T) {
		cmd := newAuthCommand()

		hasLogin := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "login" {
				hasLogin = true
				break
			}
		}
		assert.True(t, hasLogin)
	})

	t.Run("has logout subcommand", func(t *testing.T) {
		cmd := newAuthCommand()

		hasLogout := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "logout" {
				hasLogout = true
				break
			}
		}
		assert.True(t, hasLogout)
	})

	t.Run("has status subcommand", func(t *testing.T) {
		cmd := newAuthCommand()

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

func TestNewAuthLoginCommand(t *testing.T) {
	t.Run("creates login command", func(t *testing.T) {
		cmd := newAuthLoginCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "login", cmd.Name())
		assert.Equal(t, "Authenticate with server", cmd.Short)
	})

	t.Run("has oauth2 flag", func(t *testing.T) {
		cmd := newAuthLoginCommand()

		flag := cmd.Flags().Lookup("oauth2")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has token flag", func(t *testing.T) {
		cmd := newAuthLoginCommand()

		flag := cmd.Flags().Lookup("token")
		assert.NotNil(t, flag)
	})

	t.Run("has username flag", func(t *testing.T) {
		cmd := newAuthLoginCommand()

		flag := cmd.Flags().Lookup("username")
		assert.NotNil(t, flag)
	})

	t.Run("has password flag", func(t *testing.T) {
		cmd := newAuthLoginCommand()

		flag := cmd.Flags().Lookup("password")
		assert.NotNil(t, flag)
	})

	t.Run("returns error when no auth method specified", func(t *testing.T) {
		cmd := newAuthLoginCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		err := cmd.RunE(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "please specify authentication method")
	})

	t.Run("handles oauth2 flag", func(t *testing.T) {
		cmd := newAuthLoginCommand()
		_ = cmd.Flags().Set("oauth2", "true")

		err := cmd.RunE(cmd, []string{})

		require.NoError(t, err)
	})

	t.Run("handles token flag", func(t *testing.T) {
		cmd := newAuthLoginCommand()
		_ = cmd.Flags().Set("token", "test-token")

		err := cmd.RunE(cmd, []string{})

		require.NoError(t, err)
	})

	t.Run("handles username flag", func(t *testing.T) {
		cmd := newAuthLoginCommand()
		_ = cmd.Flags().Set("username", "testuser")
		_ = cmd.Flags().Set("password", "testpass")

		err := cmd.RunE(cmd, []string{})

		require.NoError(t, err)
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newAuthLoginCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "--oauth2")
		assert.Contains(t, cmd.Example, "--token")
		assert.Contains(t, cmd.Example, "--username")
	})
}

func TestNewAuthLogoutCommand(t *testing.T) {
	t.Run("creates logout command", func(t *testing.T) {
		cmd := newAuthLogoutCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "logout", cmd.Name())
		assert.Equal(t, "Clear authentication", cmd.Short)
	})

	t.Run("executes without error", func(t *testing.T) {
		cmd := newAuthLogoutCommand()

		err := cmd.RunE(cmd, []string{})

		require.NoError(t, err)
	})
}

func TestNewAuthStatusCommand(t *testing.T) {
	t.Run("creates status command", func(t *testing.T) {
		cmd := newAuthStatusCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "status", cmd.Name())
		assert.Equal(t, "Check authentication status", cmd.Short)
	})

	t.Run("shows not authenticated status", func(t *testing.T) {
		cmd := newAuthStatusCommand()

		err := cmd.RunE(cmd, []string{})

		require.NoError(t, err)
	})
}

func TestAuthCommandIntegration(t *testing.T) {
	t.Run("full auth command structure", func(t *testing.T) {
		authCmd := newAuthCommand()

		loginFound := false
		logoutFound := false
		statusFound := false

		for _, subCmd := range authCmd.Commands() {
			switch subCmd.Name() {
			case "login":
				loginFound = true
			case "logout":
				logoutFound = true
			case "status":
				statusFound = true
			}
		}

		assert.True(t, loginFound, "login command not found")
		assert.True(t, logoutFound, "logout command not found")
		assert.True(t, statusFound, "status command not found")
	})

	t.Run("command names are lowercase", func(t *testing.T) {
		authCmd := newAuthCommand()

		for _, subCmd := range authCmd.Commands() {
			assert.True(t, strings.ToLower(subCmd.Name()) == subCmd.Name(),
				"command name %q should be lowercase", subCmd.Name())
		}
	})
}
