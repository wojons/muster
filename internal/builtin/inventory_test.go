package builtin

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInventoryCommand(t *testing.T) {
	t.Run("creates inventory command", func(t *testing.T) {
		cmd := newInventoryCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "inventory", cmd.Name())
		assert.Equal(t, "Inventory management commands", cmd.Short)
		assert.Contains(t, cmd.Long, "Fetch and manage API inventory")
	})

	t.Run("has fetch subcommand", func(t *testing.T) {
		cmd := newInventoryCommand()

		hasFetch := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "fetch" {
				hasFetch = true
				break
			}
		}
		assert.True(t, hasFetch)
	})

	t.Run("has list subcommand", func(t *testing.T) {
		cmd := newInventoryCommand()

		hasList := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "list" {
				hasList = true
				break
			}
		}
		assert.True(t, hasList)
	})

	t.Run("has validate subcommand", func(t *testing.T) {
		cmd := newInventoryCommand()

		hasValidate := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "validate" {
				hasValidate = true
				break
			}
		}
		assert.True(t, hasValidate)
	})

	t.Run("has add subcommand", func(t *testing.T) {
		cmd := newInventoryCommand()

		hasAdd := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "add" {
				hasAdd = true
				break
			}
		}
		assert.True(t, hasAdd)
	})

	t.Run("has remove subcommand", func(t *testing.T) {
		cmd := newInventoryCommand()

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

func TestNewInventoryFetchCommand(t *testing.T) {
	t.Run("creates fetch command", func(t *testing.T) {
		cmd := newInventoryFetchCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "fetch", cmd.Name())
		assert.Equal(t, "Fetch inventory from host", cmd.Short)
	})

	t.Run("has host flag", func(t *testing.T) {
		cmd := newInventoryFetchCommand()

		flag := cmd.Flags().Lookup("host")
		assert.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("has output flag", func(t *testing.T) {
		cmd := newInventoryFetchCommand()

		flag := cmd.Flags().Lookup("output")
		assert.NotNil(t, flag)
		assert.Equal(t, "json", flag.DefValue)
	})

	t.Run("requires host flag", func(t *testing.T) {
		cmd := newInventoryFetchCommand()
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

		cmd := newInventoryFetchCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Fetching inventory from")
		assert.Contains(t, output, "api.example.com")
	})

	t.Run("supports json output", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInventoryFetchCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")
		_ = cmd.Flags().Set("output", "json")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Output format: JSON")
	})

	t.Run("supports yaml output", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInventoryFetchCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")
		_ = cmd.Flags().Set("output", "yaml")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Output format: YAML")
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newInventoryFetchCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "--host")
	})
}

func TestNewInventoryListCommand(t *testing.T) {
	t.Run("creates list command", func(t *testing.T) {
		cmd := newInventoryListCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "list", cmd.Name())
		assert.Equal(t, "List cached inventories", cmd.Short)
	})

	t.Run("has cache-dir flag", func(t *testing.T) {
		cmd := newInventoryListCommand()

		flag := cmd.Flags().Lookup("cache-dir")
		assert.NotNil(t, flag)
	})

	t.Run("executes without cache-dir", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInventoryListCommand()

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Cached inventories:")
	})

	t.Run("executes with cache-dir", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInventoryListCommand()
		_ = cmd.Flags().Set("cache-dir", "/tmp/cache")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Cache directory: /tmp/cache")
	})
}

func TestNewInventoryValidateCommand(t *testing.T) {
	t.Run("creates validate command", func(t *testing.T) {
		cmd := newInventoryValidateCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "validate", cmd.Name())
		assert.Equal(t, "Validate inventory schema", cmd.Short)
	})

	t.Run("has host flag", func(t *testing.T) {
		cmd := newInventoryValidateCommand()

		flag := cmd.Flags().Lookup("host")
		assert.NotNil(t, flag)
	})

	t.Run("has verbose flag", func(t *testing.T) {
		cmd := newInventoryValidateCommand()

		flag := cmd.Flags().Lookup("verbose")
		assert.NotNil(t, flag)
		assert.Equal(t, "false", flag.DefValue)
	})

	t.Run("requires host flag", func(t *testing.T) {
		cmd := newInventoryValidateCommand()
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

		cmd := newInventoryValidateCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Validating inventory from")
		assert.Contains(t, output, "Inventory is valid")
	})

	t.Run("executes with verbose output", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInventoryValidateCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")
		_ = cmd.Flags().Set("verbose", "true")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Validating resources...")
		assert.Contains(t, output, "Validating operations...")
		assert.Contains(t, output, "Validating parameters...")
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newInventoryValidateCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "--host")
		assert.Contains(t, cmd.Example, "--verbose")
	})
}

func TestInventoryCommandStructure(t *testing.T) {
	t.Run("inventory command has five subcommands", func(t *testing.T) {
		cmd := newInventoryCommand()

		assert.Equal(t, 5, len(cmd.Commands()))
	})

	t.Run("all subcommands have descriptions", func(t *testing.T) {
		inventoryCmd := newInventoryCommand()

		for _, subCmd := range inventoryCmd.Commands() {
			assert.NotEmpty(t, subCmd.Short, "subcommand %s missing Short", subCmd.Name())
			assert.NotEmpty(t, subCmd.Long, "subcommand %s missing Long", subCmd.Name())
		}
	})

	t.Run("all subcommands have RunE functions", func(t *testing.T) {
		inventoryCmd := newInventoryCommand()

		for _, subCmd := range inventoryCmd.Commands() {
			assert.NotNil(t, subCmd.RunE, "subcommand %s missing RunE", subCmd.Name())
		}
	})
}

func TestNewInventoryAddCommand(t *testing.T) {
	t.Run("creates add command", func(t *testing.T) {
		cmd := newInventoryAddCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "add", cmd.Name())
		assert.Equal(t, "Add inventory from file", cmd.Short)
	})

	t.Run("has host flag", func(t *testing.T) {
		cmd := newInventoryAddCommand()

		flag := cmd.Flags().Lookup("host")
		assert.NotNil(t, flag)
	})

	t.Run("has file flag", func(t *testing.T) {
		cmd := newInventoryAddCommand()

		flag := cmd.Flags().Lookup("file")
		assert.NotNil(t, flag)
	})

	t.Run("requires host flag", func(t *testing.T) {
		cmd := newInventoryAddCommand()

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--host is required")
	})

	t.Run("requires file flag", func(t *testing.T) {
		cmd := newInventoryAddCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--file is required")
	})

	t.Run("fails on missing file", func(t *testing.T) {
		cmd := newInventoryAddCommand()
		_ = cmd.Flags().Set("host", "https://api.example.com")
		_ = cmd.Flags().Set("file", "/tmp/nonexistent-inventory.json")

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read file")
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newInventoryAddCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "--host")
		assert.Contains(t, cmd.Example, "--file")
	})
}

func TestNewInventoryRemoveCommand(t *testing.T) {
	t.Run("creates remove command", func(t *testing.T) {
		cmd := newInventoryRemoveCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "remove", cmd.Name())
		assert.Equal(t, "Remove cached inventory", cmd.Short)
	})

	t.Run("has host flag", func(t *testing.T) {
		cmd := newInventoryRemoveCommand()

		flag := cmd.Flags().Lookup("host")
		assert.NotNil(t, flag)
	})

	t.Run("requires host flag", func(t *testing.T) {
		cmd := newInventoryRemoveCommand()

		err := cmd.RunE(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--host is required")
	})

	t.Run("executes successfully for non-existent host", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		cmd := newInventoryRemoveCommand()
		_ = cmd.Flags().Set("host", "https://nonexistent.example.com")

		err := cmd.RunE(cmd, []string{})

		_ = w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		output := buf.String()
		assert.Contains(t, output, "Inventory removed for host")
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newInventoryRemoveCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "--host")
	})
}
