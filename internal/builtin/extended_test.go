package builtin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/internal/audit"
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

// TestWebhookCommand_CreateListDelete verifies create --name/--url/--events,
// list output, and delete (alias of unsubscribe).
func TestWebhookCommand_CreateListDelete(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newWebhookCommand(cfg)

	out, err := captureStdout(func() error {
		cmd.SetArgs([]string{"create", "--name", "billing", "--url", "https://example.com/billing", "--events", "invoice.created, invoice.paid"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "wh-") // prints the subscription ID

	// Store file exists with name/url/events (events normalized)
	data, err := os.ReadFile(webhookStorePath(cfg))
	require.NoError(t, err)
	assert.Contains(t, string(data), "billing")
	assert.Contains(t, string(data), "https://example.com/billing")
	assert.Contains(t, string(data), "invoice.created,invoice.paid")

	subs, err := loadWebhooks(cfg)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "billing", subs[0].Name)
	assert.Equal(t, "invoice.created,invoice.paid", subs[0].EventType)

	// List shows name, ID, URL, events
	listOut, err := captureStdout(func() error {
		cmd2 := newWebhookCommand(cfg)
		cmd2.SetArgs([]string{"list"})
		return cmd2.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, listOut, "NAME")
	assert.Contains(t, listOut, "billing")
	assert.Contains(t, listOut, subs[0].ID)
	assert.Contains(t, listOut, "https://example.com/billing")
	assert.Contains(t, listOut, "invoice.created,invoice.paid")

	// Delete removes it
	cmd3 := newWebhookCommand(cfg)
	cmd3.SetArgs([]string{"delete", subs[0].ID})
	require.NoError(t, cmd3.Execute())

	subs, err = loadWebhooks(cfg)
	require.NoError(t, err)
	assert.Len(t, subs, 0)
}

// TestWebhookCommand_CreateRequiresFlags verifies required flags are enforced.
func TestWebhookCommand_CreateRequiresFlags(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newWebhookCommand(cfg)
	cmd.SetArgs([]string{"create"})
	err := cmd.Execute()
	require.Error(t, err)
}

// TestWebhookCommand_Test verifies connectivity testing against a live server.
func TestWebhookCommand_Test(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	webhookTestClient = srv.Client()
	t.Cleanup(func() { webhookTestClient = &http.Client{Timeout: 10 * time.Second} })

	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newWebhookCommand(cfg)
	cmd.SetArgs([]string{"create", "--name", "hook1", "--url", srv.URL, "--events", "pet.created"})
	require.NoError(t, cmd.Execute())

	subs, err := loadWebhooks(cfg)
	require.NoError(t, err)
	require.Len(t, subs, 1)

	out, err := captureStdout(func() error {
		cmd2 := newWebhookCommand(cfg)
		cmd2.SetArgs([]string{"test", subs[0].ID})
		return cmd2.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "reachable")
}

// TestWebhookCommand_TestNotFound verifies error on missing ID.
func TestWebhookCommand_TestNotFound(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newWebhookCommand(cfg)
	cmd.SetArgs([]string{"test", "wh-nonexistent"})
	err := cmd.Execute()
	require.Error(t, err)
}

// TestWebhookCommand_TestUnreachable verifies an unreachable URL errors.
func TestWebhookCommand_TestUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now unreachable
	webhookTestClient = &http.Client{Timeout: 2 * time.Second}
	t.Cleanup(func() { webhookTestClient = &http.Client{Timeout: 10 * time.Second} })

	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newWebhookCommand(cfg)
	cmd.SetArgs([]string{"create", "--name", "dead", "--url", url, "--events", "pet.created"})
	require.NoError(t, cmd.Execute())

	subs, err := loadWebhooks(cfg)
	require.NoError(t, err)
	require.Len(t, subs, 1)

	cmd2 := newWebhookCommand(cfg)
	cmd2.SetArgs([]string{"test", subs[0].ID})
	err = cmd2.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unreachable")
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
			"acme":   {BaseURL: "https://acme.example"},
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

// TestTenantCommand_Set verifies set persists the active marker to JSON.
func TestTenantCommand_Set(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: t.TempDir(),
		Profiles: map[string]config.Profile{
			"acme":   {BaseURL: "https://acme.example"},
			"globex": {BaseURL: "https://globex.example"},
		},
	}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"set", "acme"})
	require.NoError(t, cmd.Execute())
	assert.Equal(t, "acme", cfg.DefaultProfile)

	entries, err := loadTenants(cfg)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "acme", entries[0].ID)
	assert.True(t, entries[0].Active)
	assert.Equal(t, "free", entries[0].Tier)
	assert.NotEmpty(t, entries[0].SetAt)

	// JSON on disk contains the active marker
	data, err := os.ReadFile(tenantStorePath(cfg))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"active": true`)
}

// TestTenantCommand_SetDerivesTierFromProfile verifies tier derivation.
func TestTenantCommand_SetDerivesTierFromProfile(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: t.TempDir(),
		Profiles: map[string]config.Profile{
			"acme": {BaseURL: "https://acme.example", DefaultFlags: map[string]string{"tier": "pro"}},
		},
	}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"set", "acme"})
	require.NoError(t, cmd.Execute())

	entries, err := loadTenants(cfg)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "pro", entries[0].Tier)
}

// TestTenantCommand_SetUnknownTenant verifies error for unknown tenant.
func TestTenantCommand_SetUnknownTenant(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: t.TempDir(),
		Profiles: map[string]config.Profile{
			"acme": {BaseURL: "https://acme.example"},
		},
	}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"set", "nope"})
	err := cmd.Execute()
	require.Error(t, err)
}

// TestTenantCommand_PersistedTenantSurvivesProfileAddition is the DF-021
// regression: once a `profiles:` block exists in config.yaml (as
// docs/guides/configuration.md instructs), tenants persisted earlier must
// remain selectable. It walks the real flow — isolated HOME, `tenant set` on a
// fresh install, then a profiles block loaded through the config loader — so
// the validation source and the store are exercised together.
func TestTenantCommand_PersistedTenantSurvivesProfileAddition(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "muster")

	// Fresh HOME: no profiles declared, no tenants store yet.
	fresh, err := config.NewLoader().Load()
	require.NoError(t, err)
	require.Empty(t, fresh.Profiles)
	require.Equal(t, configDir, fresh.ConfigDir)

	cmd := newTenantCommand(fresh)
	cmd.SetArgs([]string{"set", "a"})
	require.NoError(t, cmd.Execute())
	require.Equal(t, "a", fresh.DefaultProfile)

	// The user now follows docs/guides/configuration.md and adds profiles.
	require.NoError(t, os.MkdirAll(configDir, 0700))
	cfgFile := filepath.Join(configDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgFile, []byte(`profiles:
  b:
    spec:
      base_url: https://b.example.com/v1
`), 0600))

	loader := config.NewLoader()
	loader.SetConfigFile(cfgFile)
	reloaded, err := loader.Load()
	require.NoError(t, err)
	require.NotEmpty(t, reloaded.Profiles, "profiles block must be loaded for this test to be meaningful")
	require.Equal(t, configDir, reloaded.ConfigDir)
	_, profileOnly := reloaded.Profiles["a"]
	require.False(t, profileOnly, "tenant a must not exist as a profile")

	// 'tenant list' still shows the persisted tenant ...
	out, err := captureStdout(func() error {
		l := newTenantCommand(reloaded)
		l.SetArgs([]string{"list"})
		return l.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "a")

	// ... and switching back to it must succeed (DF-021).
	sw := newTenantCommand(reloaded)
	sw.SetArgs([]string{"switch", "a"})
	require.NoError(t, sw.Execute())
	assert.Equal(t, "a", reloaded.DefaultProfile)
}

// TestTenantCommand_ListAndValidationAgree verifies the union contract:
// every id 'tenant list' shows — whether it comes from a config profile or
// from the persisted store — is accepted by 'tenant set'/'tenant switch',
// while an id in neither source is still rejected.
func TestTenantCommand_ListAndValidationAgree(t *testing.T) {
	// Fresh install: no profiles, no tenants store.
	cfg := &config.Config{ConfigDir: t.TempDir()}

	// A tenant persisted with no matching profile.
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"set", "persisted-only"})
	require.NoError(t, cmd.Execute())

	// A config profile appears afterwards (the union now has two sources).
	cfg.Profiles = map[string]config.Profile{
		"profile-only": {BaseURL: "https://profile.example"},
	}

	out, err := captureStdout(func() error {
		l := newTenantCommand(cfg)
		l.SetArgs([]string{"list"})
		return l.Execute()
	})
	require.NoError(t, err)
	require.Contains(t, out, "profile-only")
	require.Contains(t, out, "persisted-only")

	var listed []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "*" {
			listed = append(listed, fields[1])
			continue
		}
		listed = append(listed, fields[0])
	}
	require.ElementsMatch(t, []string{"persisted-only", "profile-only"}, listed)

	for _, id := range listed {
		for _, sub := range []string{"set", "switch"} {
			c := newTenantCommand(cfg)
			c.SetArgs([]string{sub, id})
			require.NoErrorf(t, c.Execute(), "tenant list shows %q but `tenant %s %s` rejected it", id, sub, id)
		}
	}

	// Ids in neither tenants.json nor profiles are still rejected.
	for _, sub := range []string{"set", "switch"} {
		c := newTenantCommand(cfg)
		c.SetArgs([]string{sub, "ghost"})
		err := c.Execute()
		require.Errorf(t, err, "`tenant %s ghost` must fail", sub)
		assert.Contains(t, err.Error(), "unknown tenant")
	}
}

// TestTenantCommand_Current verifies current shows name, tier, timestamp.
func TestTenantCommand_Current(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: t.TempDir(),
		Profiles: map[string]config.Profile{
			"acme": {BaseURL: "https://acme.example", DefaultFlags: map[string]string{"tier": "pro"}},
		},
	}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"set", "acme"})
	require.NoError(t, cmd.Execute())

	out, err := captureStdout(func() error {
		cmd2 := newTenantCommand(cfg)
		cmd2.SetArgs([]string{"current"})
		return cmd2.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "acme")
	assert.Contains(t, out, "[pro]")
	assert.Contains(t, out, "Set at:")
}

// TestTenantCommand_CurrentNoTenant verifies current with no active tenant.
func TestTenantCommand_CurrentNoTenant(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"current"})
	require.NoError(t, cmd.Execute())
}

// TestTenantCommand_ListBadges verifies active marker and tier badges.
func TestTenantCommand_ListBadges(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: t.TempDir(),
		Profiles: map[string]config.Profile{
			"acme":   {BaseURL: "https://acme.example", DefaultFlags: map[string]string{"tier": "pro"}},
			"globex": {BaseURL: "https://globex.example"},
		},
	}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"set", "acme"})
	require.NoError(t, cmd.Execute())

	out, err := captureStdout(func() error {
		cmd2 := newTenantCommand(cfg)
		cmd2.SetArgs([]string{"list"})
		return cmd2.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "* acme [pro]")
	assert.Contains(t, out, "globex [free]")
}

// TestTenantCommand_SwitchPersists verifies multi-tenant switching preserves
// state on disk across switches.
func TestTenantCommand_SwitchPersists(t *testing.T) {
	cfg := &config.Config{
		ConfigDir: t.TempDir(),
		Profiles: map[string]config.Profile{
			"acme":   {BaseURL: "https://acme.example"},
			"globex": {BaseURL: "https://globex.example"},
		},
	}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"set", "acme"})
	require.NoError(t, cmd.Execute())

	cmd2 := newTenantCommand(cfg)
	cmd2.SetArgs([]string{"switch", "globex"})
	require.NoError(t, cmd2.Execute())
	assert.Equal(t, "globex", cfg.DefaultProfile)

	// Disk state reflects the latest switch
	entries, err := loadTenants(cfg)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	active := ""
	for _, e := range entries {
		if e.Active {
			active = e.ID
		}
	}
	assert.Equal(t, "globex", active)

	// A fresh command (same on-disk store) sees globex as active
	out, err := captureStdout(func() error {
		cmd3 := newTenantCommand(cfg)
		cmd3.SetArgs([]string{"current"})
		return cmd3.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "globex")
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

// TestAuditCommand_Log verifies the log subcommand reads JSON-lines events,
// honors --filter and --limit.
func TestAuditCommand_Log(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	writeAuditLog(t, path,
		testAuditEvent("pet.created", "alice"),
		testAuditEvent("user.updated", "bob"),
	)
	t.Setenv("MUSTER_AUDIT_LOG", path)

	cmd := newAuditCommand()
	cmd.SetArgs([]string{"log"})
	require.NoError(t, cmd.Execute())

	// Filter matches EventType substring
	out, err := captureStdout(func() error {
		cmd2 := newAuditCommand()
		cmd2.SetArgs([]string{"log", "--filter", "pet"})
		return cmd2.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "alice")
	assert.NotContains(t, out, "bob")

	// Limit truncates
	out2, err := captureStdout(func() error {
		cmd3 := newAuditCommand()
		cmd3.SetArgs([]string{"log", "--limit", "1"})
		return cmd3.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out2, "alice")
	assert.NotContains(t, out2, "bob")
}

// TestAuditCommand_LogEmpty verifies log works with no log file.
func TestAuditCommand_LogEmpty(t *testing.T) {
	t.Setenv("MUSTER_AUDIT_LOG", filepath.Join(t.TempDir(), "audit.jsonl"))
	cmd := newAuditCommand()
	cmd.SetArgs([]string{"log"})
	require.NoError(t, cmd.Execute())
}

// TestAuditCommand_Info verifies info shows config and stats.
func TestAuditCommand_Info(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	writeAuditLog(t, path,
		testAuditEvent("pet.created", "alice"),
		testAuditEvent("user.updated", "bob"),
		testAuditEvent("pet.deleted", "carol"),
	)
	t.Setenv("MUSTER_AUDIT_LOG", path)

	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"info"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "Storage type:")
	assert.Contains(t, out, "Retention:")
	assert.Contains(t, out, "Sensitive fields:")
	assert.Contains(t, out, "Events on disk:   3")
}

// TestAuditCommand_InfoEmpty verifies info works with no log file.
func TestAuditCommand_InfoEmpty(t *testing.T) {
	t.Setenv("MUSTER_AUDIT_LOG", filepath.Join(t.TempDir(), "audit.jsonl"))
	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"info"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "Events on disk:   0")
}

// TestReadAuditEvents_EmptyFile verifies parsing an empty log.
func TestReadAuditEvents_EmptyFile(t *testing.T) {
	events, err := readAuditEvents(filepath.Join(t.TempDir(), "missing.log"), 50)
	require.NoError(t, err)
	assert.Len(t, events, 0)
}

// writeAuditLog writes events as JSON lines to path.
func writeAuditLog(t *testing.T, path string, events ...audit.AuditEvent) {
	t.Helper()
	var sb strings.Builder
	for _, e := range events {
		data, err := json.Marshal(e)
		require.NoError(t, err)
		sb.Write(data)
		sb.WriteByte('\n')
	}
	require.NoError(t, os.WriteFile(path, []byte(sb.String()), 0600))
}

// testAuditEvent builds a deterministic audit event for tests.
func testAuditEvent(eventType, actorID string) audit.AuditEvent {
	return audit.AuditEvent{
		Timestamp: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
		EventType: eventType,
		Level:     audit.AuditLevelInfo,
		Category:  audit.AuditCategorySystem,
		Action:    "test.action",
		Actor:     audit.AuditActor{ID: actorID, Type: "cli-user", Name: actorID},
	}
}
