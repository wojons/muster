package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/pkg/config"
)

// isolateConfigDir pins every environment variable the platform config-dir
// resolution reads — os.UserHomeDir (HOME / USERPROFILE) and os.UserConfigDir
// (XDG_CONFIG_HOME or HOME on Unix, HOME on macOS, %APPDATA% on Windows) — to a
// fresh temp dir, and returns the `<config dir>/muster` directory this platform
// resolves. pkg/config's loader (setDefaultPaths) and the audit sink
// (audit.DefaultSinkPath) both root their state there, so hardcoding
// ~/.config/muster made these tests fail on windows-latest (%APPDATA%) and
// macos-latest (~/Library/Application Support).
func isolateConfigDir(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))

	dir, err := os.UserConfigDir()
	require.NoError(t, err)
	return filepath.Join(dir, "muster")
}

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
	configDir := isolateConfigDir(t)

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

// TestAuditCommand_Log is the DF-050 regression: `audit log` is a log VIEWER
// and must present events NEWEST-FIRST, with --limit N returning the N most
// RECENT events (not the oldest N from the append-order file). It also checks
// the render carries action/level, not just a blank event_type column.
func TestAuditCommand_Log(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	oldEv := testAuditEvent("pet.created", "alice")
	newEv := testAuditEvent("user.updated", "bob")
	newEv.Timestamp = oldEv.Timestamp.Add(2 * time.Hour)
	writeAuditLog(t, path, oldEv, newEv)
	t.Setenv("MUSTER_AUDIT_LOG", path)

	// Newest first: user.updated (bob) before pet.created (alice).
	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"log"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Less(t, strings.Index(out, "bob"), strings.Index(out, "alice"),
		"audit log must be newest-first")
	assert.Contains(t, out, "user.updated") // action/event type rendered
	assert.Contains(t, out, "info")         // level rendered

	// --limit 1 returns the MOST RECENT event, not the first on disk.
	out2, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"log", "--limit", "1"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out2, "bob")
	assert.NotContains(t, out2, "alice")

	// Filter still matches EventType/Action/Level substring.
	out3, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"log", "--filter", "pet"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out3, "alice")
	assert.NotContains(t, out3, "bob")
}

// TestAuditCommand_ShowLimitIsNewest is the DF-050-R2 regression: `audit show`
// must share `audit log`'s ordering path. On a 120-row append-ordered sink
// (oldest first on disk) both verbs with --limit 5 must print the FIVE NEWEST
// rows, newest-first, and print exactly the same output. Before the fix `show`
// truncated to the oldest five rows before reversing.
func TestAuditCommand_ShowLimitIsNewest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	base := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	events := make([]audit.AuditEvent, 0, 120)
	for i := 0; i < 120; i++ {
		e := testAuditEvent("bench.event", "actor-"+strconv.Itoa(i))
		e.Timestamp = base.Add(time.Duration(i) * time.Minute)
		events = append(events, e)
	}
	writeAuditLog(t, path, events...)
	t.Setenv("MUSTER_AUDIT_LOG", path)

	runAudit := func(args ...string) string {
		t.Helper()
		out, err := captureStdout(func() error {
			cmd := newAuditCommand()
			cmd.SetArgs(args)
			return cmd.Execute()
		})
		require.NoError(t, err)
		return out
	}

	showOut := runAudit("show", "--limit", "5")
	logOut := runAudit("log", "--limit", "5")

	showLines := nonEmptyLines(showOut)
	logLines := nonEmptyLines(logOut)
	require.Len(t, showLines, 5, "show --limit 5 must print exactly 5 rows")
	require.Len(t, logLines, 5, "log --limit 5 must print exactly 5 rows")
	assert.Equal(t, logLines, showLines, "audit show must match audit log for the same --limit")

	// The five NEWEST rows are actors 119..115, newest first.
	for i, line := range showLines {
		wantActor := "actor-" + strconv.Itoa(119-i)
		assert.Contains(t, line, wantActor, "row %d must be the #%d-newest event", i, i+1)
	}
	// The oldest rows must not be present — the pre-fix truncate-before-reverse
	// returned exactly those.
	assert.NotContains(t, showOut, "\tactor-0\n")
	assert.NotContains(t, showOut, "\tactor-1\n")
}

// nonEmptyLines splits rendered output into its non-blank lines.
func nonEmptyLines(out string) []string {
	lines := make([]string, 0, 8)
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// TestAuditCommand_LogEmpty verifies log works with no log file.
func TestAuditCommand_LogEmpty(t *testing.T) {
	t.Setenv("MUSTER_AUDIT_LOG", filepath.Join(t.TempDir(), "audit.jsonl"))
	cmd := newAuditCommand()
	cmd.SetArgs([]string{"log"})
	require.NoError(t, cmd.Execute())
}

// TestAuditCommand_VerifyIntact verifies verify accepts an intact hash chain.
func TestAuditCommand_VerifyIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	writeHashedAuditLog(t, path,
		testAuditEvent("pet.created", "alice"),
		testAuditEvent("user.updated", "bob"),
	)
	t.Setenv("MUSTER_AUDIT_LOG", path)

	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"verify"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "2 records, chain intact")
}

// TestAuditCommand_VerifyEditedRecord rejects a record whose body changed.
func TestAuditCommand_VerifyEditedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	writeHashedAuditLog(t, path,
		testAuditEvent("pet.created", "alice"),
		testAuditEvent("user.updated", "bob"),
	)
	lines := strings.Split(strings.TrimSpace(string(mustReadFile(t, path))), "\n")
	var edited audit.AuditEvent
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &edited))
	edited.Action = "tampered.action"
	data, err := json.Marshal(edited)
	require.NoError(t, err)
	lines[1] = string(data)
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600))
	t.Setenv("MUSTER_AUDIT_LOG", path)

	_, err = captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"verify"})
		return cmd.Execute()
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2")
	assert.Contains(t, err.Error(), "hash mismatch")
}

// TestAuditCommand_VerifyDeletedRecord rejects a chain with a missing middle record.
func TestAuditCommand_VerifyDeletedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	writeHashedAuditLog(t, path,
		testAuditEvent("pet.created", "alice"),
		testAuditEvent("user.updated", "bob"),
		testAuditEvent("pet.deleted", "carol"),
	)
	lines := strings.Split(strings.TrimSpace(string(mustReadFile(t, path))), "\n")
	lines = append(lines[:1], lines[2:]...)
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600))
	t.Setenv("MUSTER_AUDIT_LOG", path)

	_, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"verify"})
		return cmd.Execute()
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 2")
	assert.Contains(t, err.Error(), "chain break")
	assert.Contains(t, err.Error(), "previous_hash")
}

// TestAuditCommand_VerifyEmpty verifies a missing sink is trivially intact.
func TestAuditCommand_VerifyEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-audit.jsonl")
	t.Setenv("MUSTER_AUDIT_LOG", path)

	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"verify"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "0 records, chain intact")
}

// writeHashedAuditLog writes events through the production sink path.
func writeHashedAuditLog(t *testing.T, path string, events ...audit.AuditEvent) {
	t.Helper()
	for i := range events {
		event := events[i]
		require.NoError(t, audit.WriteEventTo(path, &event))
	}
}

// mustReadFile reads a test fixture or fails the test.
func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
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

// ── DF-024 regressions ───────────────────────────────────────────────────

// TestWebhookCommand_DeleteByName is the DF-024 regression: `webhook create`
// and `webhook list` deal in names, but `delete` accepted only the stored
// `wh-<id>`, so the name the user had just typed was rejected as not found.
func TestWebhookCommand_DeleteByName(t *testing.T) {
	cfg := &config.Config{ConfigDir: t.TempDir()}

	create := newWebhookCommand(cfg)
	create.SetArgs([]string{"create", "--name", "hook1", "--url", "https://example.com/hook", "--events", "pet.created"})
	require.NoError(t, create.Execute())
	subs, err := loadWebhooks(cfg)
	require.NoError(t, err)
	require.Len(t, subs, 1)

	out, err := captureStdout(func() error {
		del := newWebhookCommand(cfg)
		del.SetArgs([]string{"delete", "hook1"})
		return del.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, subs[0].ID, "the removed subscription is reported by id")
	assert.Contains(t, out, "hook1", "and by the name the user passed")

	subs, err = loadWebhooks(cfg)
	require.NoError(t, err)
	assert.Empty(t, subs)

	// The reference is gone from the store, so deleting it again still errors.
	again := newWebhookCommand(cfg)
	again.SetArgs([]string{"delete", "hook1"})
	require.Error(t, again.Execute())
}

// TestWebhookCommand_TestByName mirrors the delete fix for `test`.
func TestWebhookCommand_TestByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	webhookTestClient = srv.Client()
	t.Cleanup(func() { webhookTestClient = &http.Client{Timeout: 10 * time.Second} })

	cfg := &config.Config{ConfigDir: t.TempDir()}
	create := newWebhookCommand(cfg)
	create.SetArgs([]string{"create", "--name", "hook1", "--url", srv.URL, "--events", "pet.created"})
	require.NoError(t, create.Execute())

	out, err := captureStdout(func() error {
		probe := newWebhookCommand(cfg)
		probe.SetArgs([]string{"test", "hook1"})
		return probe.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "reachable")
}

// TestResolveWebhook pins the lookup contract behind name-or-id: a plain name
// resolves, ids win over names, an empty reference never matches, and unnamed
// subscriptions (those created by `webhook subscribe`) are only reachable by id.
func TestResolveWebhook(t *testing.T) {
	subs := []webhookSub{
		{ID: "wh-1", Name: "billing"},
		{ID: "wh-2"},
		{ID: "hook1", Name: "shadow"},
		{ID: "wh-3", Name: "hook1"},
	}

	byID := resolveWebhook(subs, "wh-1")
	require.NotNil(t, byID)
	assert.Equal(t, "wh-1", byID.ID)

	// A reference that is only ever a NAME must resolve by name.
	byName := resolveWebhook(subs, "billing")
	require.NotNil(t, byName, "a name reference must resolve")
	assert.Equal(t, "wh-1", byName.ID)

	// "hook1" is subs[3]'s name and subs[2]'s id: the id match wins,
	// wherever it sits in the slice.
	byBoth := resolveWebhook(subs, "hook1")
	require.NotNil(t, byBoth)
	assert.Equal(t, "hook1", byBoth.ID)
	assert.Equal(t, "shadow", byBoth.Name)

	// Unnamed subscriptions are not reachable by name.
	assert.Equal(t, "wh-2", resolveWebhook(subs, "wh-2").ID)
	assert.Nil(t, resolveWebhook(subs, ""), "an empty reference never matches")
	assert.Nil(t, resolveWebhook(subs, "wh-9"))
	assert.Nil(t, resolveWebhook(subs, "no-such-name"))
	assert.Nil(t, resolveWebhook(nil, "wh-1"))
}

// TestAuditEmit_MutationEventsLand is the DF-024 regression for audit emission:
// with audit enabled, the management-plane mutations must append JSON-lines
// events to the audit log that `audit log` reads. Previously the CLI had a
// reader and no writers, so an enabled audit trail was always empty.
func TestAuditEmit_MutationEventsLand(t *testing.T) {
	cfgDir := isolateConfigDir(t)
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", "")

	cfg := &config.Config{ConfigDir: cfgDir}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	webhookTestClient = srv.Client()
	t.Cleanup(func() { webhookTestClient = &http.Client{Timeout: 10 * time.Second} })

	set := newTenantCommand(cfg)
	set.SetArgs([]string{"set", "acme"})
	require.NoError(t, set.Execute())

	sw := newTenantCommand(cfg)
	sw.SetArgs([]string{"switch", "acme"})
	require.NoError(t, sw.Execute())

	create := newWebhookCommand(cfg)
	create.SetArgs([]string{"create", "--name", "hook1", "--url", srv.URL, "--events", "pet.created"})
	require.NoError(t, create.Execute())

	probe := newWebhookCommand(cfg)
	probe.SetArgs([]string{"test", "hook1"})
	require.NoError(t, probe.Execute())

	del := newWebhookCommand(cfg)
	del.SetArgs([]string{"delete", "hook1"})
	require.NoError(t, del.Execute())

	logPath := filepath.Join(cfgDir, "audit.jsonl")
	events, err := readAuditEvents(logPath, 0)
	require.NoError(t, err, "audit is enabled, so the log file must exist")
	var got []string
	for _, e := range events {
		got = append(got, e.EventType)
	}
	assert.Equal(t, []string{
		auditEventTenantSet,
		auditEventTenantSwitch,
		auditEventWebhookCreate,
		auditEventWebhookTest,
		auditEventWebhookDelete,
	}, got)

	// The reader command surfaces the same records.
	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"log", "--filter", "webhook"})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, auditEventWebhookCreate)
	assert.Contains(t, out, auditEventWebhookDelete)
}

// TestAuditEmit_DisabledWritesNothing pins the opt-in side: with audit logging
// off the mutations succeed and no audit file appears.
func TestAuditEmit_DisabledWritesNothing(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_ENABLED", "")
	t.Setenv("MUSTER_AUDIT_LOG", logPath)

	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"set", "acme"})
	require.NoError(t, cmd.Execute())

	_, err := os.Stat(logPath)
	assert.True(t, os.IsNotExist(err), "audit disabled must not create the audit log")
}

// TestAuditEmit_UnwritableSinkFailsLoudly is the other half of the DF-024 fix:
// an enabled sink that cannot be written must fail the mutation with a non-zero
// error instead of silently dropping the audit record.
func TestAuditEmit_UnwritableSinkFailsLoudly(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0600))
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", filepath.Join(blocker, "audit.jsonl"))

	cfg := &config.Config{ConfigDir: t.TempDir()}
	cmd := newTenantCommand(cfg)
	cmd.SetArgs([]string{"set", "acme"})
	err := cmd.Execute()
	require.Error(t, err, "an unwritable audit sink must fail the mutation")
	assert.Contains(t, err.Error(), "audit")

	// The same guard covers the webhook path.
	hook := newWebhookCommand(cfg)
	hook.SetArgs([]string{"create", "--name", "h", "--url", "https://example.com/h", "--events", "x.y"})
	err = hook.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audit")
}

// TestAuditSink_BothWritersShareOnePath is the AUD-001 regression: the two
// audit writers — the command-level hook (ordinary CLI/MCP command events) and
// the management-plane emitter (tenant/webhook mutations) — must resolve ONE
// sink, the same file `audit log` reads. Before AUD-001 the hook wrote
// $TMPDIR/muster-audit/audit.log, so an enabled audit trail could never show a
// command event.
func TestAuditSink_BothWritersShareOnePath(t *testing.T) {
	sink := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", sink)

	require.Equal(t, sink, audit.SinkPath(), "the $MUSTER_AUDIT_LOG override must win")
	require.Equal(t, sink, auditLogPath(), "the management-plane path must be the same sink")

	// Writer 1 — management plane: a mutating built-in command.
	cfg := &config.Config{ConfigDir: t.TempDir()}
	set := newTenantCommand(cfg)
	set.SetArgs([]string{"set", "acme"})
	require.NoError(t, set.Execute())

	// Writer 2 — command level: the hook every CLI/MCP entry point logs through.
	// The action carries a per-run nonce so the legacy-sink assertion below
	// cannot be satisfied by stale files left behind by an earlier run.
	probe := fmt.Sprintf("aud001-probe-%d", time.Now().UnixNano())
	action := "tool." + probe
	commandHook := audit.NewHook(audit.DefaultConfigEnabled())
	commandHook.LogToolCall(context.Background(), probe, "mcp-client-9", true,
		map[string]string{"spec": "petstore.yaml"})
	require.NoError(t, commandHook.Close())

	// Both records are in the one file the shared helper resolves.
	events, err := readAuditEvents(audit.SinkPath(), 0)
	require.NoError(t, err, "audit is enabled, so the sink must exist")
	var actions []string
	for _, e := range events {
		actions = append(actions, e.Action)
	}
	assert.Contains(t, actions, "tenant.set")
	assert.Contains(t, actions, action)

	// The legacy temporary sink must not receive command events any more.
	legacy := filepath.Join(os.TempDir(), "muster-audit", "audit.log")
	if data, readErr := os.ReadFile(legacy); readErr == nil {
		assert.NotContains(t, string(data), action,
			"command events must not go to the legacy temporary sink")
	}

	// The reader command sees command-level events too: filter on the action
	// only the hook's event carries; the rendered row carries its actor.
	out, err := captureStdout(func() error {
		cmd := newAuditCommand()
		cmd.SetArgs([]string{"log", "--filter", action})
		return cmd.Execute()
	})
	require.NoError(t, err)
	assert.Contains(t, out, "mcp-client-9")
}

// TestAuditEmit_UserFieldMatchesActor is the DF-055 regression for the
// management-plane emitter: tenant/webhook rows must carry user == actor.id,
// not the always-empty value the field shipped before the fix.
func TestAuditEmit_UserFieldMatchesActor(t *testing.T) {
	sink := filepath.Join(t.TempDir(), "audit.jsonl")
	t.Setenv("MUSTER_AUDIT_ENABLED", "1")
	t.Setenv("MUSTER_AUDIT_LOG", sink)
	t.Setenv("USER", "muster-tester")

	cfg := &config.Config{ConfigDir: t.TempDir()}
	set := newTenantCommand(cfg)
	set.SetArgs([]string{"set", "acme"})
	require.NoError(t, set.Execute())

	data, err := os.ReadFile(sink)
	require.NoError(t, err, "audit is enabled, so the sink must exist")
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines)

	found := false
	for _, line := range lines {
		var row struct {
			EventType string `json:"event_type"`
			User      string `json:"user"`
			Actor     struct {
				ID string `json:"id"`
			} `json:"actor"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &row))
		if row.EventType != "tenant.set" {
			continue
		}
		found = true
		assert.NotEmpty(t, row.User, "tenant.set row must not ship an empty user (DF-055)")
		assert.Equal(t, row.Actor.ID, row.User, "user must equal actor.id")
	}
	require.True(t, found, "no tenant.set row written to the sink")
}
