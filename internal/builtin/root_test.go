package builtin

import (
	"testing"

	"github.com/openapi-cli/openapi-cli/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestNewRootCommand(t *testing.T) {
	t.Run("creates root command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		assert.NotNil(t, root)
		assert.Equal(t, "openapi-cli", root.Name())
		assert.Equal(t, "OpenAPI-powered CLI and MCP server", root.Short)
		assert.Contains(t, root.Long, "A unified CLI tool that generates commands from OpenAPI specifications")
	})

	t.Run("has version command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasVersion := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "version" {
				hasVersion = true
				break
			}
		}
		assert.True(t, hasVersion)
	})

	t.Run("has config command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasConfig := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "config" {
				hasConfig = true
				break
			}
		}
		assert.True(t, hasConfig)
	})

	t.Run("has help command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasHelp := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "help" {
				hasHelp = true
				break
			}
		}
		assert.True(t, hasHelp)
	})

	t.Run("has completion command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasCompletion := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "completion" {
				hasCompletion = true
				break
			}
		}
		assert.True(t, hasCompletion)
	})

	t.Run("has init command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasInit := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "init" {
				hasInit = true
				break
			}
		}
		assert.True(t, hasInit)
	})

	t.Run("has auth command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasAuth := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "auth" {
				hasAuth = true
				break
			}
		}
		assert.True(t, hasAuth)
	})

	t.Run("has inventory command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasInventory := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "inventory" {
				hasInventory = true
				break
			}
		}
		assert.True(t, hasInventory)
	})

	t.Run("has modules command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasModules := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "modules" {
				hasModules = true
				break
			}
		}
		assert.True(t, hasModules)
	})

	t.Run("has generate command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasGenerate := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "generate" {
				hasGenerate = true
				break
			}
		}
		assert.True(t, hasGenerate)
	})

	t.Run("has request command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasRequest := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "request" {
				hasRequest = true
				break
			}
		}
		assert.True(t, hasRequest)
	})

	t.Run("has daemon command", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		hasDaemon := false
		for _, cmd := range root.Commands() {
			if cmd.Name() == "daemon" {
				hasDaemon = true
				break
			}
		}
		assert.True(t, hasDaemon)
	})

	t.Run("root command has description", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		assert.NotEmpty(t, root.Short)
		assert.NotEmpty(t, root.Long)
	})
}

func TestLoadPersistedCommands(t *testing.T) {
	t.Run("handles missing storage gracefully", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		assert.NotNil(t, root)
	})

	t.Run("root command is not nil", func(t *testing.T) {
		cfg := &config.Config{
			ConfigDir:      "/tmp/config",
			CacheDir:       "/tmp/cache",
			DataDir:        "/tmp/data",
			Version:        "1.0.0",
			DefaultProfile: "default",
		}
		root := NewRootCommand(cfg)

		assert.NotNil(t, root)
	})

	t.Run("all builtin commands are registered", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		expectedCommands := []string{
			"version",
			"config",
			"help",
			"completion",
			"init",
			"auth",
			"inventory",
			"modules",
			"generate",
			"request",
			"daemon",
			"audit",
		}

		actualCommands := make(map[string]bool)
		for _, cmd := range root.Commands() {
			actualCommands[cmd.Name()] = true
		}

		for _, expected := range expectedCommands {
			assert.True(t, actualCommands[expected], "command %s not found", expected)
		}
	})
}

func TestRootCommandStructure(t *testing.T) {
	t.Run("root command has multiple subcommands", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		assert.Greater(t, len(root.Commands()), 5)
	})

	t.Run("all subcommands have descriptions", func(t *testing.T) {
		cfg := &config.Config{}
		root := NewRootCommand(cfg)

		builtInCommands := []string{
			"version", "config", "help", "completion", "init",
			"auth", "inventory", "modules", "generate", "request", "daemon",
			"audit",
		}

		for _, cmd := range root.Commands() {
			if cmd.Hidden {
				continue
			}
			isBuiltIn := false
			for _, name := range builtInCommands {
				if cmd.Name() == name {
					isBuiltIn = true
					break
				}
			}
			if isBuiltIn {
				assert.NotEmpty(t, cmd.Short, "built-in command %s missing Short", cmd.Name())
			}
		}
	})
}
