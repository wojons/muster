package builtin

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewModulesCommand(t *testing.T) {
	t.Run("creates modules command", func(t *testing.T) {
		cmd := newModulesCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "modules", cmd.Name())
		assert.Equal(t, "WASM module management", cmd.Short)
		assert.Contains(t, cmd.Long, "Install, list and remove WASM modules")
	})

	t.Run("has install subcommand", func(t *testing.T) {
		cmd := newModulesCommand()

		hasInstall := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "install" {
				hasInstall = true
				break
			}
		}
		assert.True(t, hasInstall)
	})

	t.Run("has list subcommand", func(t *testing.T) {
		cmd := newModulesCommand()

		hasList := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "list" {
				hasList = true
				break
			}
		}
		assert.True(t, hasList)
	})

	t.Run("has remove subcommand", func(t *testing.T) {
		cmd := newModulesCommand()

		hasRemove := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "remove" {
				hasRemove = true
				break
			}
		}
		assert.True(t, hasRemove)
	})
}

func TestNewModulesInstallCommand(t *testing.T) {
	t.Run("creates install command", func(t *testing.T) {
		cmd := newModulesInstallCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "install", cmd.Name())
		assert.Equal(t, "Install a WASM module", cmd.Short)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		cmd := newModulesInstallCommand()

		err := cmd.Args(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	})

	t.Run("has name flag", func(t *testing.T) {
		cmd := newModulesInstallCommand()

		flag := cmd.Flags().Lookup("name")
		assert.NotNil(t, flag)
	})

	t.Run("has overwrite flag", func(t *testing.T) {
		cmd := newModulesInstallCommand()

		flag := cmd.Flags().Lookup("overwrite")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has validate flag", func(t *testing.T) {
		cmd := newModulesInstallCommand()

		flag := cmd.Flags().Lookup("validate")
		assert.NotNil(t, flag)
		assert.Equal(t, "true", flag.DefValue)
	})

	t.Run("executes with module path", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newModulesInstallCommand()

		err := cmd.RunE(cmd, []string{"./modules/test.wasm"})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Installing module from")
		assert.Contains(t, output, "./modules/test.wasm")
	})

	t.Run("executes with custom name", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newModulesInstallCommand()
		_ = cmd.Flags().Set("name", "custom-module")

		err := cmd.RunE(cmd, []string{"./modules/test.wasm"})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Module name: custom-module")
	})

	t.Run("executes with validate enabled", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newModulesInstallCommand()
		_ = cmd.Flags().Set("validate", "true")

		err := cmd.RunE(cmd, []string{"./modules/test.wasm"})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Validating module...")
	})

	t.Run("executes with overwrite enabled", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newModulesInstallCommand()
		_ = cmd.Flags().Set("overwrite", "true")

		err := cmd.RunE(cmd, []string{"./modules/test.wasm"})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Overwriting existing module...")
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newModulesInstallCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "modules install")
		assert.Contains(t, cmd.Example, "--name")
	})
}

func TestNewModulesListCommand(t *testing.T) {
	t.Run("creates list command", func(t *testing.T) {
		cmd := newModulesListCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "list", cmd.Name())
		assert.Equal(t, "List installed modules", cmd.Short)
	})

	t.Run("has verbose flag", func(t *testing.T) {
		cmd := newModulesListCommand()

		flag := cmd.Flags().Lookup("verbose")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("has format flag", func(t *testing.T) {
		cmd := newModulesListCommand()

		flag := cmd.Flags().Lookup("format")
		assert.NotNil(t, flag)
		assert.Equal(t, "table", flag.DefValue)
	})

	t.Run("executes without verbose", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newModulesListCommand()

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Installed modules:")
	})

	t.Run("executes with verbose", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newModulesListCommand()
		_ = cmd.Flags().Set("verbose", "true")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "No modules installed")
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newModulesListCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "modules list")
		assert.Contains(t, cmd.Example, "--verbose")
	})
}

func TestNewModulesRemoveCommand(t *testing.T) {
	t.Run("creates remove command", func(t *testing.T) {
		cmd := newModulesRemoveCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "remove", cmd.Name())
		assert.Equal(t, "Remove a WASM module", cmd.Short)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		cmd := newModulesRemoveCommand()

		err := cmd.Args(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	})

	t.Run("has force flag", func(t *testing.T) {
		cmd := newModulesRemoveCommand()

		flag := cmd.Flags().Lookup("force")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("executes without force", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newModulesRemoveCommand()

		err := cmd.RunE(cmd, []string{"test-module"})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Remove module 'test-module'?")
	})

	t.Run("executes with force", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newModulesRemoveCommand()
		_ = cmd.Flags().Set("force", "true")

		err := cmd.RunE(cmd, []string{"test-module"})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Removing module 'test-module'...")
		assert.Contains(t, output, "Module 'test-module' removed successfully")
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newModulesRemoveCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "modules remove")
		assert.Contains(t, cmd.Example, "--force")
	})
}

func TestModulesCommandStructure(t *testing.T) {
	t.Run("modules command has four subcommands", func(t *testing.T) {
		cmd := newModulesCommand()

		assert.Equal(t, 4, len(cmd.Commands()))
	})

	t.Run("all subcommands have descriptions", func(t *testing.T) {
		modulesCmd := newModulesCommand()

		for _, subCmd := range modulesCmd.Commands() {
			assert.NotEmpty(t, subCmd.Short, "subcommand %s missing Short", subCmd.Name())
			assert.NotEmpty(t, subCmd.Long, "subcommand %s missing Long", subCmd.Name())
		}
	})

	t.Run("all subcommands have RunE functions", func(t *testing.T) {
		modulesCmd := newModulesCommand()

		for _, subCmd := range modulesCmd.Commands() {
			assert.NotNil(t, subCmd.RunE, "subcommand %s missing RunE", subCmd.Name())
		}
	})
}

func TestNewModulesRunCommand(t *testing.T) {
	t.Run("creates run command", func(t *testing.T) {
		cmd := newModulesRunCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "run", cmd.Name())
		assert.Equal(t, "Execute a WASM module", cmd.Short)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		cmd := newModulesRunCommand()

		err := cmd.Args(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	})

	t.Run("has function flag", func(t *testing.T) {
		cmd := newModulesRunCommand()

		flag := cmd.Flags().Lookup("function")
		assert.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("has arg flag", func(t *testing.T) {
		cmd := newModulesRunCommand()

		flag := cmd.Flags().Lookup("arg")
		assert.NotNil(t, flag)
	})

	t.Run("fails with nonexistent file", func(t *testing.T) {
		cmd := newModulesRunCommand()

		err := cmd.RunE(cmd, []string{"/nonexistent/module.wasm"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read module")
	})

	t.Run("executes a valid wasm module", func(t *testing.T) {
		cmd := newModulesRunCommand()

		err := cmd.RunE(cmd, []string{"testdata/hello.wasm"})

		require.NoError(t, err)
	})

	t.Run("executes with custom function name", func(t *testing.T) {
		cmd := newModulesRunCommand()
		cmd.Flags().Set("function", "hello")

		err := cmd.RunE(cmd, []string{"testdata/hello.wasm"})

		require.NoError(t, err)
	})

	t.Run("executes with arguments", func(t *testing.T) {
		cmd := newModulesRunCommand()
		cmd.Flags().Set("function", "transform")
		cmd.Flags().Set("arg", "42")

		err := cmd.RunE(cmd, []string{"testdata/transform.wasm"})

		require.NoError(t, err)
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newModulesRunCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "modules run")
	})
}

func TestDefaultFunctionName(t *testing.T) {
	t.Run("derives from simple filename", func(t *testing.T) {
		assert.Equal(t, "hello", defaultFunctionName("hello.wasm"))
	})

	t.Run("derives from path", func(t *testing.T) {
		assert.Equal(t, "transform", defaultFunctionName("/path/to/transform.wasm"))
	})

	t.Run("handles no extension", func(t *testing.T) {
		assert.Equal(t, "module", defaultFunctionName("module"))
	})

	t.Run("handles empty filename", func(t *testing.T) {
		assert.Equal(t, "hello", defaultFunctionName(".wasm"))
	})
}

func TestParseIntArg(t *testing.T) {
	t.Run("parses positive integer", func(t *testing.T) {
		assert.Equal(t, int32(42), parseIntArg("42"))
	})

	t.Run("parses negative integer", func(t *testing.T) {
		assert.Equal(t, int32(-5), parseIntArg("-5"))
	})

	t.Run("parses zero", func(t *testing.T) {
		assert.Equal(t, int32(0), parseIntArg("0"))
	})

	t.Run("returns string for non-integer", func(t *testing.T) {
		assert.Equal(t, "hello", parseIntArg("hello"))
	})

	t.Run("returns string for empty", func(t *testing.T) {
		assert.Equal(t, "", parseIntArg(""))
	})
}
