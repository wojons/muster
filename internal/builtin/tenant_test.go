package builtin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTenantCommand(t *testing.T) {
	t.Run("creates tenant command", func(t *testing.T) {
		cmd := newTenantCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "tenant", cmd.Name())
		assert.Equal(t, "Multi-tenancy context management", cmd.Short)
		assert.Contains(t, cmd.Long, "Manage multi-tenancy context")
	})

	t.Run("has set subcommand", func(t *testing.T) {
		cmd := newTenantCommand()

		hasSet := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == "set" {
				hasSet = true
				break
			}
		}
		assert.True(t, hasSet)
	})

	t.Run("has list subcommand", func(t *testing.T) {
		cmd := newTenantCommand()

		hasList := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == "list" {
				hasList = true
				break
			}
		}
		assert.True(t, hasList)
	})

	t.Run("has current subcommand", func(t *testing.T) {
		cmd := newTenantCommand()

		hasCurrent := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == "current" {
				hasCurrent = true
				break
			}
		}
		assert.True(t, hasCurrent)
	})
}

func TestNewTenantSetCommand(t *testing.T) {
	t.Run("creates set command", func(t *testing.T) {
		cmd := newTenantSetCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "set", cmd.Name())
		assert.Equal(t, "Set active tenant context", cmd.Short)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		cmd := newTenantSetCommand()

		err := cmd.Args(cmd, []string{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "accepts 1 arg")
	})

	t.Run("has name flag", func(t *testing.T) {
		cmd := newTenantSetCommand()

		flag := cmd.Flags().Lookup("name")
		require.NotNil(t, flag)
	})

	t.Run("has tier flag", func(t *testing.T) {
		cmd := newTenantSetCommand()

		flag := cmd.Flags().Lookup("tier")
		require.NotNil(t, flag)
	})

	t.Run("rejects invalid tenant ID", func(t *testing.T) {
		cmd := newTenantSetCommand()

		err := cmd.RunE(cmd, []string{"INVALID_UPPER"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid tenant ID")
	})

	t.Run("sets valid tenant ID", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newTenantSetCommand()
		err := cmd.RunE(cmd, []string{"acme-corp"})
		require.NoError(t, err)

		statePath := filepath.Join(tempHome, ".config", "openapi-cli", "tenants.json")
		data, err := os.ReadFile(statePath)
		require.NoError(t, err)

		var state tenantCliState
		err = json.Unmarshal(data, &state)
		require.NoError(t, err)
		assert.Equal(t, "acme-corp", state.ActiveTenant)
		assert.Len(t, state.Tenants, 1)
		assert.True(t, state.Tenants[0].Active)
	})

	t.Run("sets tenant with optional flags", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newTenantSetCommand()
		_ = cmd.Flags().Set("name", "ACME Corp")
		_ = cmd.Flags().Set("tier", "pro")

		err := cmd.RunE(cmd, []string{"acme-corp"})
		require.NoError(t, err)

		statePath := filepath.Join(tempHome, ".config", "openapi-cli", "tenants.json")
		data, err := os.ReadFile(statePath)
		require.NoError(t, err)

		var state tenantCliState
		err = json.Unmarshal(data, &state)
		require.NoError(t, err)
		assert.Equal(t, "acme-corp", state.ActiveTenant)
		assert.Equal(t, "ACME Corp", state.Tenants[0].Name)
		assert.Equal(t, "pro", state.Tenants[0].Tier)
	})

	t.Run("sets multiple tenants switching active", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		first := newTenantSetCommand()
		_ = first.Flags().Set("name", "Alpha")
		err := first.RunE(first, []string{"alpha-tenant"})
		require.NoError(t, err)

		second := newTenantSetCommand()
		_ = second.Flags().Set("name", "Beta")
		err = second.RunE(second, []string{"beta-tenant"})
		require.NoError(t, err)

		statePath := filepath.Join(tempHome, ".config", "openapi-cli", "tenants.json")
		data, err := os.ReadFile(statePath)
		require.NoError(t, err)

		var state tenantCliState
		err = json.Unmarshal(data, &state)
		require.NoError(t, err)
		assert.Equal(t, "beta-tenant", state.ActiveTenant)
		assert.Len(t, state.Tenants, 2)

		for _, tnt := range state.Tenants {
			if tnt.ID == "alpha-tenant" {
				assert.False(t, tnt.Active, "alpha should not be active")
			}
			if tnt.ID == "beta-tenant" {
				assert.True(t, tnt.Active, "beta should be active")
			}
		}
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newTenantSetCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "openapi-cli tenant set")
	})
}

func TestNewTenantListCommand(t *testing.T) {
	t.Run("creates list command", func(t *testing.T) {
		cmd := newTenantListCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "list", cmd.Name())
		assert.Equal(t, "List available tenants", cmd.Short)
	})

	t.Run("shows empty message when no tenants", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newTenantListCommand()
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
	})

	t.Run("lists tenants after set", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		setCmd := newTenantSetCommand()
		_ = setCmd.Flags().Set("name", "Test Tenant")
		err := setCmd.RunE(setCmd, []string{"test-tenant"})
		require.NoError(t, err)

		listCmd := newTenantListCommand()
		err = listCmd.RunE(listCmd, []string{})
		require.NoError(t, err)
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newTenantListCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "openapi-cli tenant list")
	})
}

func TestNewTenantCurrentCommand(t *testing.T) {
	t.Run("creates current command", func(t *testing.T) {
		cmd := newTenantCurrentCommand()

		assert.NotNil(t, cmd)
		assert.Equal(t, "current", cmd.Name())
		assert.Equal(t, "Show current tenant context", cmd.Short)
	})

	t.Run("shows no active tenant when none set", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		cmd := newTenantCurrentCommand()
		err := cmd.RunE(cmd, []string{})
		require.NoError(t, err)
	})

	t.Run("shows current tenant after set", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		setCmd := newTenantSetCommand()
		err := setCmd.RunE(setCmd, []string{"my-tenant"})
		require.NoError(t, err)

		currentCmd := newTenantCurrentCommand()
		err = currentCmd.RunE(currentCmd, []string{})
		require.NoError(t, err)
	})

	t.Run("shows details when tenant has metadata", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		setCmd := newTenantSetCommand()
		_ = setCmd.Flags().Set("name", "My Tenant")
		_ = setCmd.Flags().Set("tier", "enterprise")
		err := setCmd.RunE(setCmd, []string{"my-tenant"})
		require.NoError(t, err)

		currentCmd := newTenantCurrentCommand()
		err = currentCmd.RunE(currentCmd, []string{})
		require.NoError(t, err)
	})

	t.Run("has example usage", func(t *testing.T) {
		cmd := newTenantCurrentCommand()

		assert.NotEmpty(t, cmd.Example)
		assert.Contains(t, cmd.Example, "openapi-cli tenant current")
	})
}

func TestTenantCommandIntegration(t *testing.T) {
	t.Run("full tenant command structure", func(t *testing.T) {
		tenantCmd := newTenantCommand()

		setFound := false
		listFound := false
		currentFound := false

		for _, sub := range tenantCmd.Commands() {
			switch sub.Name() {
			case "set":
				setFound = true
			case "list":
				listFound = true
			case "current":
				currentFound = true
			}
		}

		assert.True(t, setFound, "set command not found")
		assert.True(t, listFound, "list command not found")
		assert.True(t, currentFound, "current command not found")
	})

	t.Run("command names are lowercase", func(t *testing.T) {
		tenantCmd := newTenantCommand()

		for _, sub := range tenantCmd.Commands() {
			assert.True(t, strings.ToLower(sub.Name()) == sub.Name(),
				"command name %q should be lowercase", sub.Name())
		}
	})

	t.Run("end-to-end workflow", func(t *testing.T) {
		tempHome := t.TempDir()
		t.Setenv("HOME", tempHome)

		setCmd := newTenantSetCommand()
		_ = setCmd.Flags().Set("name", "ACME Corp")
		_ = setCmd.Flags().Set("tier", "enterprise")
		err := setCmd.RunE(setCmd, []string{"acme-corp"})
		require.NoError(t, err)

		currentCmd := newTenantCurrentCommand()
		err = currentCmd.RunE(currentCmd, []string{})
		require.NoError(t, err)

		listCmd := newTenantListCommand()
		err = listCmd.RunE(listCmd, []string{})
		require.NoError(t, err)
	})
}
