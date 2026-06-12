package builtin

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
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
		mtlsFound := false

		for _, subCmd := range authCmd.Commands() {
			switch subCmd.Name() {
			case "login":
				loginFound = true
			case "logout":
				logoutFound = true
			case "status":
				statusFound = true
			case "mtls":
				mtlsFound = true
			}
		}

		assert.True(t, loginFound, "login command not found")
		assert.True(t, logoutFound, "logout command not found")
		assert.True(t, statusFound, "status command not found")
		assert.True(t, mtlsFound, "mtls command not found")
	})

	t.Run("command names are lowercase", func(t *testing.T) {
		authCmd := newAuthCommand()

		for _, subCmd := range authCmd.Commands() {
			assert.True(t, strings.ToLower(subCmd.Name()) == subCmd.Name(),
				"command name %q should be lowercase", subCmd.Name())
		}
	})
}

func TestNewAuthMTLSCommand(t *testing.T) {
	t.Run("creates mtls command", func(t *testing.T) {
		cmd := newAuthMTLSCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "mtls", cmd.Name())
		assert.Equal(t, "Manage mTLS certificates", cmd.Short)
	})

	t.Run("has generate subcommand", func(t *testing.T) {
		cmd := newAuthMTLSCommand()

		hasGenerate := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == "generate" {
				hasGenerate = true
				break
			}
		}
		assert.True(t, hasGenerate)
	})

	t.Run("has info subcommand", func(t *testing.T) {
		cmd := newAuthMTLSCommand()

		hasInfo := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == "info" {
				hasInfo = true
				break
			}
		}
		assert.True(t, hasInfo)
	})

	t.Run("has configure subcommand", func(t *testing.T) {
		cmd := newAuthMTLSCommand()

		hasConfigure := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == "configure" {
				hasConfigure = true
				break
			}
		}
		assert.True(t, hasConfigure)
	})
}

func TestNewAuthMTLSGenerateCommand(t *testing.T) {
	t.Run("creates generate command", func(t *testing.T) {
		cmd := newAuthMTLSGenerateCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "generate", cmd.Name())
		assert.Equal(t, "Generate self-signed client certificate", cmd.Short)
	})

	t.Run("has common-name flag with default", func(t *testing.T) {
		cmd := newAuthMTLSGenerateCommand()

		flag := cmd.Flags().Lookup("common-name")
		require.NotNil(t, flag)
		assert.Equal(t, "muster-client", flag.DefValue)
	})

	t.Run("generates cert files in mtls directory", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newAuthMTLSGenerateCommand()
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		mtlsDir := filepath.Join(tempHome, ".config", "openapi-cli", "mtls")
		certPath := filepath.Join(mtlsDir, "client.crt")
		keyPath := filepath.Join(mtlsDir, "client.key")

		_, err = os.Stat(certPath)
		require.NoError(t, err, "cert file should exist")

		_, err = os.Stat(keyPath)
		require.NoError(t, err, "key file should exist")

		certData, err := os.ReadFile(certPath)
		require.NoError(t, err)
		assert.Contains(t, string(certData), "CERTIFICATE")

		keyData, err := os.ReadFile(keyPath)
		require.NoError(t, err)
		assert.Contains(t, string(keyData), "RSA PRIVATE KEY")
	})

	t.Run("generates with custom common name", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newAuthMTLSGenerateCommand()
		_ = cmd.Flags().Set("common-name", "my-client")

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		mtlsDir := filepath.Join(tempHome, ".config", "openapi-cli", "mtls")
		certPath := filepath.Join(mtlsDir, "client.crt")

		certPEM, err := os.ReadFile(certPath)
		require.NoError(t, err)

		block, _ := pem.Decode(certPEM)
		require.NotNil(t, block)

		cert, err := x509.ParseCertificate(block.Bytes)
		require.NoError(t, err)
		assert.Equal(t, "my-client", cert.Subject.CommonName)
	})
}

func TestNewAuthMTLSInfoCommand(t *testing.T) {
	t.Run("creates info command", func(t *testing.T) {
		cmd := newAuthMTLSInfoCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "info", cmd.Name())
		assert.Equal(t, "Display certificate information", cmd.Short)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		cmd := newAuthMTLSInfoCommand()

		err := cmd.Args(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	})

	t.Run("displays cert info for generated certificate", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		genCmd := newAuthMTLSGenerateCommand()
		_ = genCmd.Flags().Set("common-name", "test-client")
		err := genCmd.RunE(genCmd, []string{})
		require.NoError(t, err)

		certPath := filepath.Join(tempHome, ".config", "openapi-cli", "mtls", "client.crt")

		infoCmd := newAuthMTLSInfoCommand()
		err = infoCmd.RunE(infoCmd, []string{certPath})
		require.NoError(t, err)
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		cmd := newAuthMTLSInfoCommand()

		err := cmd.RunE(cmd, []string{"/nonexistent/cert.pem"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read certificate")
	})
}

func TestNewAuthMTLSConfigureCommand(t *testing.T) {
	t.Run("creates configure command", func(t *testing.T) {
		cmd := newAuthMTLSConfigureCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "configure", cmd.Name())
		assert.Equal(t, "Configure mTLS certificate paths", cmd.Short)
	})

	t.Run("has cert flag", func(t *testing.T) {
		cmd := newAuthMTLSConfigureCommand()

		flag := cmd.Flags().Lookup("cert")
		require.NotNil(t, flag)
	})

	t.Run("has key flag", func(t *testing.T) {
		cmd := newAuthMTLSConfigureCommand()

		flag := cmd.Flags().Lookup("key")
		require.NotNil(t, flag)
	})

	t.Run("has ca flag", func(t *testing.T) {
		cmd := newAuthMTLSConfigureCommand()

		flag := cmd.Flags().Lookup("ca")
		require.NotNil(t, flag)
	})

	t.Run("requires cert and key flags", func(t *testing.T) {
		cmd := newAuthMTLSConfigureCommand()

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--cert and --key are required")
	})

	t.Run("saves mTLS configuration", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newAuthMTLSConfigureCommand()
		_ = cmd.Flags().Set("cert", "/tmp/test.crt")
		_ = cmd.Flags().Set("key", "/tmp/test.key")

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
	})
}
