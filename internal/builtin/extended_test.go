package builtin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/pkg/config"
)

// TestWebhookCommand_ListEmpty verifies list works with no subscriptions.
func TestWebhookCommand_ListEmpty(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newWebhookCommand(cfg)

	// List with no store file → "No webhook subscriptions."
	cmd.SetArgs([]string{"list"})
	err := cmd.Execute()
	require.NoError(t, err)
}

// TestWebhookCommand_SubscribeUnsubscribe verifies the round-trip.
func TestWebhookCommand_SubscribeUnsubscribe(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newWebhookCommand(cfg)

	cmd.SetArgs([]string{"subscribe", "https://example.com/hook", "pet.created"})
	require.NoError(t, cmd.Execute())

	// Store file exists with one entry
	data, err := os.ReadFile(webhookStorePath(cfg))
	require.NoError(t, err)
	assert.Contains(t, string(data), "https://example.com/hook")
	assert.Contains(t, string(data), "pet.created")

	// List shows the subscription
	subs, err := loadWebhooks(cfg)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "https://example.com/hook", subs[0].URL)

	// Unsubscribe removes it
	cmd2 := newWebhookCommand(cfg)
	cmd2.SetArgs([]string{"unsubscribe", subs[0].ID})
	require.NoError(t, cmd2.Execute())

	subs, err = loadWebhooks(cfg)
	require.NoError(t, err)
	assert.Len(t, subs, 0)
}

// TestWebhookCommand_UnsubscribeNotFound verifies error on missing ID.
func TestWebhookCommand_UnsubscribeNotFound(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newWebhookCommand(cfg)
	cmd.SetArgs([]string{"unsubscribe", "wh-nonexistent"})
	err := cmd.Execute()
	require.Error(t, err)
}

// TestTenantCommand_ShowDefault verifies no-tenant case.
func TestTenantCommand_ShowDefault(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"show"})
	require.NoError(t, cmd.Execute())
}

// TestTenantCommand_Switch verifies switching to an existing profile.
func TestTenantCommand_Switch(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: t.TempDir(),
		Profiles: map[string]config.Profile{
			"acme":  {BaseURL: "https://acme.example"},
			"globex": {BaseURL: "https://globex.example"},
		},
	}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"switch", "acme"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "acme", cfg.DefaultProfile)

	// Unknown tenant errors
	cmd2 := newTenantCommand(cfg)
	cmd2.SetArgs([]string{"switch", "nope"})
	require.Error(t, cmd2.Execute())
}

// TestAuditCommand_Status verifies status output works.
func TestAuditCommand_Status(t *testing.T) {
	cmd := newAuditCommand()
	cmd.SetArgs([]string{"status"})
	require.NoError(t, cmd.Execute())
}

// TestAuditCommand_ShowEmpty verifies show works with no log file.
func TestAuditCommand_ShowEmpty(t *testing.T) {
	t.Setenv("MUSTER_AUDIT_LOG", filepath.Join(t.TempDir(), "audit.log"))
	cmd := newAuditCommand()
	cmd.SetArgs([]string{"show"})
	require.NoError(t, cmd.Execute())
}

// TestReadAuditEvents_EmptyFile verifies parsing an empty log.
func TestReadAuditEvents_EmptyFile(t *testing.T) {
	events, err := readAuditEvents(filepath.Join(t.TempDir(), "missing.log"), 50)
	require.NoError(t, err)
	assert.Len(t, events, 0)
}
