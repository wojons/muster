package builtin

import (
	"bytes"
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
		assert.Contains(t, err.Error(), "requires exactly 1 argument")
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
		cmd := newModulesInstallCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		err := cmd.RunE(cmd, []string{"./modules/test.wasm"})

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "Installing module from")
		assert.Contains(t, output, "./modules/test.wasm")
	})

	t.Run("executes with custom name", func(t *testing.T) {
		cmd := newModulesInstallCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		cmd.Flags().Set("name", "custom-module")

		err := cmd.RunE(cmd, []string{"./modules/test.wasm"})

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "Module name: custom-module")
	})

	t.Run("executes with validate enabled", func(t *testing.T) {
		cmd := newModulesInstallCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		cmd.Flags().Set("validate", "true")

		err := cmd.RunE(cmd, []string{"./modules/test.wasm"})

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "Validating module...")
	})

	t.Run("executes with overwrite enabled", func(t *testing.T) {
		cmd := newModulesInstallCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		cmd.Flags().Set("overwrite", "true")

		err := cmd.RunE(cmd, []string{"./modules/test.wasm"})

		require.NoError(t, err)
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
		cmd := newModulesListCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		err := cmd.RunE(cmd, []string{})

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "Installed modules:")
	})

	t.Run("executes with verbose", func(t *testing.T) {
		cmd := newModulesListCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		cmd.Flags().Set("verbose", "true")

		err := cmd.RunE(cmd, []string{})

		require.NoError(t, err)
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
		assert.Contains(t, err.Error(), "requires exactly 1 argument")
	})

	t.Run("has force flag", func(t *testing.T) {
		cmd := newModulesRemoveCommand()

		flag := cmd.Flags().Lookup("force")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("executes without force", func(t *testing.T) {
		cmd := newModulesRemoveCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		err := cmd.RunE(cmd, []string{"test-module"})

		require.NoError(t, err)
		output := buf.String()
		assert.Contains(t, output, "Remove module 'test-module'?")
	})

	t.Run("executes with force", func(t *testing.T) {
		cmd := newModulesRemoveCommand()
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)

		cmd.Flags().Set("force", "true")

		err := cmd.RunE(cmd, []string{"test-module"})

		require.NoError(t, err)
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
	t.Run("modules command has three subcommands", func(t *testing.T) {
		cmd := newModulesCommand()

		assert.Equal(t, 3, len(cmd.Commands()))
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
