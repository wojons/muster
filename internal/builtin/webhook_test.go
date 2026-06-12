package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWebhookCommand(t *testing.T) {
	t.Run("creates webhook command", func(t *testing.T) {
		cmd := newWebhookCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "webhook", cmd.Name())
		assert.Equal(t, "Webhook subscription commands", cmd.Short)
		assert.Contains(t, cmd.Long, "Manage webhook subscriptions")
	})

	t.Run("has create subcommand", func(t *testing.T) {
		cmd := newWebhookCommand()

		hasCreate := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "create" {
				hasCreate = true
				break
			}
		}
		assert.True(t, hasCreate)
	})

	t.Run("has list subcommand", func(t *testing.T) {
		cmd := newWebhookCommand()

		hasList := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "list" {
				hasList = true
				break
			}
		}
		assert.True(t, hasList)
	})

	t.Run("has delete subcommand", func(t *testing.T) {
		cmd := newWebhookCommand()

		hasDelete := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "delete" {
				hasDelete = true
				break
			}
		}
		assert.True(t, hasDelete)
	})

	t.Run("has test subcommand", func(t *testing.T) {
		cmd := newWebhookCommand()

		hasTest := false
		for _, subCmd := range cmd.Commands() {
			if subCmd.Name() == "test" {
				hasTest = true
				break
			}
		}
		assert.True(t, hasTest)
	})
}

func TestNewWebhookCreateCommand(t *testing.T) {
	t.Run("creates create command", func(t *testing.T) {
		cmd := newWebhookCreateCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "create", cmd.Name())
		assert.Equal(t, "Create a webhook subscription", cmd.Short)
	})

	t.Run("has name flag", func(t *testing.T) {
		cmd := newWebhookCreateCommand()

		flag := cmd.Flags().Lookup("name")
		assert.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("has url flag", func(t *testing.T) {
		cmd := newWebhookCreateCommand()

		flag := cmd.Flags().Lookup("url")
		assert.NotNil(t, flag)
		assert.Equal(t, "", flag.DefValue)
	})

	t.Run("has token flag", func(t *testing.T) {
		cmd := newWebhookCreateCommand()

		flag := cmd.Flags().Lookup("token")
		assert.NotNil(t, flag)
	})

	t.Run("has events flag", func(t *testing.T) {
		cmd := newWebhookCreateCommand()

		flag := cmd.Flags().Lookup("events")
		assert.NotNil(t, flag)
	})

	t.Run("returns error when no name specified", func(t *testing.T) {
		cmd := newWebhookCreateCommand()
		_ = cmd.Flags().Set("url", "https://example.com/webhooks")

		err := cmd.RunE(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "--name is required")
	})

	t.Run("returns error when no url specified", func(t *testing.T) {
		cmd := newWebhookCreateCommand()
		_ = cmd.Flags().Set("name", "test-webhook")

		err := cmd.RunE(cmd, []string{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "--url is required")
	})

	t.Run("creates subscription file", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newWebhookCreateCommand()
		_ = cmd.Flags().Set("name", "test-webhook")
		_ = cmd.Flags().Set("url", "wss://api.example.com/webhooks")
		_ = cmd.Flags().Set("events", "resource.created,resource.updated")

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)

		webhookDir := filepath.Join(tempHome, ".config", "openapi-cli", "webhooks")
		entries, err := os.ReadDir(webhookDir)
		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.True(t, strings.HasSuffix(entries[0].Name(), ".json"))
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newWebhookCreateCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "--name")
		assert.Contains(t, cmd.Example, "--url")
	})
}

func TestNewWebhookListCommand(t *testing.T) {
	t.Run("creates list command", func(t *testing.T) {
		cmd := newWebhookListCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "list", cmd.Name())
		assert.Equal(t, "List webhook subscriptions", cmd.Short)
	})

	t.Run("shows empty when no subscriptions", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newWebhookListCommand()

		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
	})

	t.Run("lists subscriptions after creation", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		createCmd := newWebhookCreateCommand()
		_ = createCmd.Flags().Set("name", "test-webhook")
		_ = createCmd.Flags().Set("url", "wss://api.example.com/webhooks")
		err := createCmd.RunE(createCmd, []string{})
		require.NoError(t, err)

		listCmd := newWebhookListCommand()
		err = listCmd.RunE(listCmd, []string{})
		require.NoError(t, err)
	})
}

func TestNewWebhookDeleteCommand(t *testing.T) {
	t.Run("creates delete command", func(t *testing.T) {
		cmd := newWebhookDeleteCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "delete", cmd.Name())
		assert.Equal(t, "Delete a webhook subscription", cmd.Short)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		cmd := newWebhookDeleteCommand()

		err := cmd.Args(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	})

	t.Run("returns error for non-existent id", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newWebhookDeleteCommand()

		err := cmd.RunE(cmd, []string{"non-existent-id"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("deletes existing subscription", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		createCmd := newWebhookCreateCommand()
		_ = createCmd.Flags().Set("name", "test-webhook")
		_ = createCmd.Flags().Set("url", "wss://api.example.com/webhooks")
		err := createCmd.RunE(createCmd, []string{})
		require.NoError(t, err)

		webhookDir := filepath.Join(tempHome, ".config", "openapi-cli", "webhooks")
		entries, err := os.ReadDir(webhookDir)
		require.NoError(t, err)
		require.Len(t, entries, 1)

		id := strings.TrimSuffix(entries[0].Name(), ".json")

		deleteCmd := newWebhookDeleteCommand()
		err = deleteCmd.RunE(deleteCmd, []string{id})
		require.NoError(t, err)

		entries, err = os.ReadDir(webhookDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
}

func TestNewWebhookTestCommand(t *testing.T) {
	t.Run("creates test command", func(t *testing.T) {
		cmd := newWebhookTestCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "test", cmd.Name())
		assert.Equal(t, "Test a webhook subscription", cmd.Short)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		cmd := newWebhookTestCommand()

		err := cmd.Args(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	})

	t.Run("returns error for non-existent id", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newWebhookTestCommand()

		err := cmd.RunE(cmd, []string{"non-existent-id"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestWebhookCommandIntegration(t *testing.T) {
	t.Run("full webhook command structure", func(t *testing.T) {
		webhookCmd := newWebhookCommand()

		createFound := false
		listFound := false
		deleteFound := false
		testFound := false

		for _, subCmd := range webhookCmd.Commands() {
			switch subCmd.Name() {
			case "create":
				createFound = true
			case "list":
				listFound = true
			case "delete":
				deleteFound = true
			case "test":
				testFound = true
			}
		}

		assert.True(t, createFound, "create command not found")
		assert.True(t, listFound, "list command not found")
		assert.True(t, deleteFound, "delete command not found")
		assert.True(t, testFound, "test command not found")
	})

	t.Run("command names are lowercase", func(t *testing.T) {
		webhookCmd := newWebhookCommand()

		for _, subCmd := range webhookCmd.Commands() {
			assert.True(t, strings.ToLower(subCmd.Name()) == subCmd.Name(),
				"command name %q should be lowercase", subCmd.Name())
		}
	})

	t.Run("create then list then delete lifecycle", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		createCmd := newWebhookCreateCommand()
		_ = createCmd.Flags().Set("name", "lifecycle-test")
		_ = createCmd.Flags().Set("url", "wss://example.com/webhooks")
		_ = createCmd.Flags().Set("events", "test.event")
		err := createCmd.RunE(createCmd, []string{})
		require.NoError(t, err)

		listCmd := newWebhookListCommand()
		err = listCmd.RunE(listCmd, []string{})
		require.NoError(t, err)

		webhookDir := filepath.Join(tempHome, ".config", "openapi-cli", "webhooks")
		entries, err := os.ReadDir(webhookDir)
		require.NoError(t, err)
		require.Len(t, entries, 1)

		id := strings.TrimSuffix(entries[0].Name(), ".json")

		deleteCmd := newWebhookDeleteCommand()
		err = deleteCmd.RunE(deleteCmd, []string{id})
		require.NoError(t, err)

		entries, err = os.ReadDir(webhookDir)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
}

func TestSplitEvents(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		result := splitEvents("")
		assert.Nil(t, result)
	})

	t.Run("single event", func(t *testing.T) {
		result := splitEvents("resource.created")
		assert.Equal(t, []string{"resource.created"}, result)
	})

	t.Run("multiple events", func(t *testing.T) {
		result := splitEvents("resource.created,resource.updated,resource.deleted")
		assert.Equal(t, []string{"resource.created", "resource.updated", "resource.deleted"}, result)
	})

	t.Run("events with spaces", func(t *testing.T) {
		result := splitEvents(" resource.created , resource.updated ")
		assert.Equal(t, []string{"resource.created", "resource.updated"}, result)
	})

	t.Run("trailing comma", func(t *testing.T) {
		result := splitEvents("resource.created,")
		assert.Equal(t, []string{"resource.created"}, result)
	})
}
