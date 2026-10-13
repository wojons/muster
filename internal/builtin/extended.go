package builtin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/pkg/config"
)

// ── webhook ──────────────────────────────────────────────────────────────
// Webhook subscriptions are stored as JSON in the config directory.

type webhookSub struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	EventType string `json:"event_type"`
	CreatedAt string `json:"created_at"`
}

func webhookStorePath(cfg *config.Config) string {
	if cfg != nil && cfg.ConfigDir != "" {
		return filepath.Join(cfg.ConfigDir, "webhooks.json")
	}
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "muster", "webhooks.json")
}

func loadWebhooks(cfg *config.Config) ([]webhookSub, error) {
	data, err := os.ReadFile(webhookStorePath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read webhooks: %w", err)
	}
	var subs []webhookSub
	if err := json.Unmarshal(data, &subs); err != nil {
		return nil, fmt.Errorf("parse webhooks: %w", err)
	}
	return subs, nil
}

func saveWebhooks(cfg *config.Config, subs []webhookSub) error {
	data, err := json.MarshalIndent(subs, "", "  ")
	if err != nil {
		return err
	}
	path := webhookStorePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// normalizeEvents trims and joins a comma-separated event list into the
// single EventType string persisted for a subscription.
func normalizeEvents(s string) string {
	var parts []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ",")
}

// createWebhook appends a new subscription and persists the store.
func createWebhook(cfg *config.Config, name, url, events string) (*webhookSub, error) {
	subs, err := loadWebhooks(cfg)
	if err != nil {
		return nil, err
	}
	sub := webhookSub{
		ID:        fmt.Sprintf("wh-%d", time.Now().UnixNano()),
		Name:      name,
		URL:       url,
		EventType: normalizeEvents(events),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	subs = append(subs, sub)
	if err := saveWebhooks(cfg, subs); err != nil {
		return nil, err
	}
	return &sub, nil
}

// resolveWebhook returns the subscription matching ref, which may be either
// the stored id (`wh-<n>`) or the human-readable name. Ids are matched first
// and over the whole set before names, so an entry whose name happens to look
// like another entry's id can never shadow the id lookup. Entries created by
// `webhook subscribe` carry no name and are therefore only matchable by id.
func resolveWebhook(subs []webhookSub, ref string) *webhookSub {
	if ref == "" {
		return nil
	}
	for i := range subs {
		if subs[i].ID == ref {
			return &subs[i]
		}
	}
	for i := range subs {
		if subs[i].Name != "" && subs[i].Name == ref {
			return &subs[i]
		}
	}
	return nil
}

// findWebhook returns the subscription named or identified by ref.
func findWebhook(cfg *config.Config, ref string) (*webhookSub, error) {
	subs, err := loadWebhooks(cfg)
	if err != nil {
		return nil, err
	}
	sub := resolveWebhook(subs, ref)
	if sub == nil {
		return nil, fmt.Errorf("webhook %s not found", ref)
	}
	return sub, nil
}

// removeWebhook deletes the subscription named or identified by ref and
// persists the remaining set. It returns the removed entry so callers can
// report which subscription went away when the user passed a name rather than
// an id.
func removeWebhook(cfg *config.Config, ref string) (*webhookSub, error) {
	subs, err := loadWebhooks(cfg)
	if err != nil {
		return nil, err
	}
	target := resolveWebhook(subs, ref)
	if target == nil {
		return nil, fmt.Errorf("webhook %s not found", ref)
	}
	kept := make([]webhookSub, 0, len(subs))
	for _, s := range subs {
		if s.ID != target.ID {
			kept = append(kept, s)
		}
	}
	if err := saveWebhooks(cfg, kept); err != nil {
		return nil, err
	}
	return target, nil
}

// webhookLabel renders a subscription for operator-facing messages: the id
// alone, plus the name when one is set.
func webhookLabel(sub *webhookSub) string {
	if sub.Name == "" {
		return sub.ID
	}
	return fmt.Sprintf("%s (%s)", sub.ID, sub.Name)
}

// webhookTestClient performs connectivity checks for `webhook test`.
// Overridable in tests to avoid real network calls.
var webhookTestClient = &http.Client{Timeout: 10 * time.Second}

// probeWebhookURL reports whether the URL answers an HTTP request. HEAD is
// tried first; endpoints that reject HEAD (405/501) fall back to GET.
func probeWebhookURL(url string) (int, error) {
	resp, err := webhookTestClient.Head(url)
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotImplemented {
			return resp.StatusCode, nil
		}
	}
	resp, err = webhookTestClient.Get(url)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}

func newWebhookCommand(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Manage webhook subscriptions",
		Long: `Manage webhook subscriptions for API events.

Subscriptions are stored in the local config directory and used by the
daemon to deliver event notifications.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List webhook subscriptions",
		RunE: func(cmd *cobra.Command, args []string) error {
			subs, err := loadWebhooks(cfg)
			if err != nil {
				return err
			}
			if len(subs) == 0 {
				fmt.Println("No webhook subscriptions.")
				return nil
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", "NAME", "ID", "URL", "EVENTS")
			for _, s := range subs {
				fmt.Printf("%s\t%s\t%s\t%s\n", s.Name, s.ID, s.URL, s.EventType)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "subscribe <url> <event-type>",
		Short: "Subscribe to an API event",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sub, err := createWebhook(cfg, "", args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Printf("Subscribed %s -> %s (event: %s)\n", sub.ID, sub.URL, sub.EventType)
			return nil
		},
	})

	var createName, createURL, createEvents string
	create := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook subscription",
		RunE: func(cmd *cobra.Command, args []string) error {
			sub, err := createWebhook(cfg, createName, createURL, createEvents)
			if err != nil {
				return err
			}
			if err := emitAuditEvent(auditEventWebhookCreate, "webhook.create", "webhook", sub.ID, sub.Name, true,
				map[string]string{"url": sub.URL, "events": sub.EventType}); err != nil {
				return err
			}
			fmt.Printf("Created webhook %s (%s) -> %s (events: %s)\n", sub.ID, sub.Name, sub.URL, sub.EventType)
			return nil
		},
	}
	create.Flags().StringVar(&createName, "name", "", "Webhook name")
	create.Flags().StringVar(&createURL, "url", "", "Webhook callback URL")
	create.Flags().StringVar(&createEvents, "events", "", "Comma-separated event types to subscribe to")
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("url")
	_ = create.MarkFlagRequired("events")
	cmd.AddCommand(create)

	cmd.AddCommand(&cobra.Command{
		Use:     "delete <id|name>",
		Aliases: []string{"unsubscribe"},
		Short:   "Remove a webhook subscription",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			removed, err := removeWebhook(cfg, args[0])
			if err != nil {
				return err
			}
			if err := emitAuditEvent(auditEventWebhookDelete, "webhook.delete", "webhook", removed.ID, removed.Name, true,
				map[string]string{"url": removed.URL}); err != nil {
				return err
			}
			fmt.Printf("Deleted webhook %s\n", webhookLabel(removed))
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "test <id|name>",
		Short: "Test connectivity to a webhook URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sub, err := findWebhook(cfg, args[0])
			if err != nil {
				return err
			}
			status, probeErr := probeWebhookURL(sub.URL)
			details := map[string]string{"url": sub.URL}
			if probeErr == nil {
				details["status"] = fmt.Sprintf("HTTP %d", status)
			} else {
				details["error"] = probeErr.Error()
			}
			if auditErr := emitAuditEvent(auditEventWebhookTest, "webhook.test", "webhook", sub.ID, sub.Name,
				probeErr == nil, details); auditErr != nil {
				return auditErr
			}
			if probeErr != nil {
				return fmt.Errorf("webhook %s (%s) unreachable: %w", sub.ID, sub.URL, probeErr)
			}
			fmt.Printf("Webhook %s (%s) reachable: HTTP %d\n", sub.ID, sub.URL, status)
			return nil
		},
	})

	return cmd
}

// ── tenant ───────────────────────────────────────────────────────────────
// Tenants map to config profiles per the multi-tenancy spec. The active
// tenant and per-tenant metadata (name, tier, set timestamp) persist to
// tenants.json in the config directory so state survives across switches.

type tenantEntry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Tier   string `json:"tier"`
	Active bool   `json:"active"`
	SetAt  string `json:"set_at"`
}

func tenantStorePath(cfg *config.Config) string {
	if cfg != nil && cfg.ConfigDir != "" {
		return filepath.Join(cfg.ConfigDir, "tenants.json")
	}
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "muster", "tenants.json")
}

func loadTenants(cfg *config.Config) ([]tenantEntry, error) {
	data, err := os.ReadFile(tenantStorePath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read tenants: %w", err)
	}
	var entries []tenantEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse tenants: %w", err)
	}
	return entries, nil
}

func saveTenants(cfg *config.Config, entries []tenantEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	path := tenantStorePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// deriveTier returns the display tier for a tenant, preferring an explicitly
// persisted tier, then a tier declared on the matching profile, then the
// default badge.
func deriveTier(cfg *config.Config, id string, entry *tenantEntry) string {
	if entry != nil && entry.Tier != "" {
		return entry.Tier
	}
	if cfg != nil {
		if p, ok := cfg.Profiles[id]; ok {
			if t := p.DefaultFlags["tier"]; t != "" {
				return t
			}
		}
	}
	return "free"
}

// tenantKnown reports whether id names a tenant this CLI knows about: either a
// declared config profile or an entry in the persisted tenant store. That
// union is exactly what 'tenant list' renders, so the two commands can never
// disagree about which ids are acceptable.
func tenantKnown(cfg *config.Config, entries []tenantEntry, id string) bool {
	if cfg != nil {
		if _, ok := cfg.Profiles[id]; ok {
			return true
		}
	}
	return findTenantEntry(entries, id) != nil
}

// setActiveTenant validates the tenant, persists the active marker to the
// tenants store, and updates the in-memory default profile.
func setActiveTenant(cfg *config.Config, id string) error {
	if cfg == nil {
		return fmt.Errorf("config not initialized")
	}
	entries, err := loadTenants(cfg)
	if err != nil {
		return err
	}
	// Validate against the union of config profiles and persisted tenants —
	// the same source 'tenant list' reads. Gating on cfg.Profiles alone meant
	// that defining ANY profile (as docs/guides/configuration.md instructs)
	// rejected every persisted tenant the user could still see listed
	// (DF-021). When nothing is known yet, the id bootstraps a new tenant so
	// `tenant set <id>` keeps working on a fresh install; ids absent from
	// both sources are still rejected.
	if len(cfg.Profiles) > 0 || len(entries) > 0 {
		if !tenantKnown(cfg, entries, id) {
			return fmt.Errorf("unknown tenant %q — use 'tenant list'", id)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	upserted := false
	for i := range entries {
		if entries[i].ID == id {
			entries[i].Active = true
			entries[i].SetAt = now
			if entries[i].Name == "" {
				entries[i].Name = id
			}
			upserted = true
		} else {
			entries[i].Active = false
		}
	}
	if !upserted {
		entries = append(entries, tenantEntry{
			ID:     id,
			Name:   id,
			Tier:   deriveTier(cfg, id, nil),
			Active: true,
			SetAt:  now,
		})
	}
	if err := saveTenants(cfg, entries); err != nil {
		return err
	}
	cfg.DefaultProfile = id
	return nil
}

// activeTenantID returns the persisted active tenant, falling back to the
// in-memory default profile when no active marker is recorded.
func activeTenantID(entries []tenantEntry, cfg *config.Config) string {
	for i := range entries {
		if entries[i].Active {
			return entries[i].ID
		}
	}
	if cfg != nil {
		return cfg.DefaultProfile
	}
	return ""
}

// findTenantEntry returns the persisted entry for id, or nil.
func findTenantEntry(entries []tenantEntry, id string) *tenantEntry {
	for i := range entries {
		if entries[i].ID == id {
			return &entries[i]
		}
	}
	return nil
}

func newTenantCommand(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenant",
		Short: "Manage tenant context",
		Long: `Manage the multi-tenant context for this CLI.

Each tenant maps to a named config profile with isolated settings,
quotas, and audit records. The active tenant persists to disk so the
selection survives across switches and restarts.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show current tenant",
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg == nil || cfg.DefaultProfile == "" {
				fmt.Println("No tenant selected (default).")
				return nil
			}
			fmt.Println(cfg.DefaultProfile)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List known tenants",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := loadTenants(cfg)
			if err != nil {
				return err
			}
			active := activeTenantID(entries, cfg)
			seen := make(map[string]bool)
			var names []string
			if cfg != nil {
				for name := range cfg.Profiles {
					seen[name] = true
					names = append(names, name)
				}
			}
			for _, e := range entries {
				if !seen[e.ID] {
					seen[e.ID] = true
					names = append(names, e.ID)
				}
			}
			if len(names) == 0 {
				fmt.Println("No tenants configured.")
				return nil
			}
			sort.Strings(names)
			for _, name := range names {
				marker := " "
				if name == active {
					marker = "*"
				}
				tier := deriveTier(cfg, name, findTenantEntry(entries, name))
				fmt.Printf("%s %s [%s]\n", marker, name, tier)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "set <tenant-id>",
		Short: "Set the active tenant (persists to disk)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setActiveTenant(cfg, args[0]); err != nil {
				return err
			}
			if err := emitAuditEvent(auditEventTenantSet, "tenant.set", "tenant", args[0], args[0], true, nil); err != nil {
				return err
			}
			fmt.Printf("Set active tenant to %s\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "switch <tenant-id>",
		Short: "Switch current tenant (persists to disk)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := setActiveTenant(cfg, args[0]); err != nil {
				return err
			}
			if err := emitAuditEvent(auditEventTenantSwitch, "tenant.switch", "tenant", args[0], args[0], true, nil); err != nil {
				return err
			}
			fmt.Printf("Switched to tenant %s\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "current",
		Short: "Show the active tenant with name, tier, and timestamp",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := loadTenants(cfg)
			if err != nil {
				return err
			}
			id := activeTenantID(entries, cfg)
			if id == "" {
				fmt.Println("No tenant selected (default).")
				return nil
			}
			entry := findTenantEntry(entries, id)
			name := id
			if entry != nil && entry.Name != "" {
				name = entry.Name
			}
			tier := deriveTier(cfg, id, entry)
			ts := "unknown"
			if entry != nil && entry.SetAt != "" {
				ts = entry.SetAt
			}
			fmt.Printf("Name:   %s\n", name)
			fmt.Printf("ID:     %s\n", id)
			fmt.Printf("Tier:   [%s]\n", tier)
			fmt.Printf("Set at: %s\n", ts)
			return nil
		},
	})

	return cmd
}

// ── audit ────────────────────────────────────────────────────────────────

// Audit event types emitted by mutating built-in commands. They are stable
// identifiers: `audit log --filter <value>` matches them.
const (
	auditEventTenantSet     = "tenant.set"
	auditEventTenantSwitch  = "tenant.switch"
	auditEventWebhookCreate = "webhook.create"
	auditEventWebhookDelete = "webhook.delete"
	auditEventWebhookTest   = "webhook.test"
)

// emitAuditEvent appends one audit event to the shared audit sink
// (audit.SinkPath: MUSTER_AUDIT_LOG, else <config dir>/muster/audit.jsonl — the
// same file `audit log` reads and the same file the command-level
// audit.Hook writes) when audit logging is enabled, and is a no-op otherwise.
//
// The write is synchronous and its error is returned, so an enabled audit sink
// that cannot be written fails the mutation loudly instead of silently
// dropping the record. That matters for exactly the events below: a tenant
// switch or a webhook mutation that leaves no trace is the failure mode the
// audit trail exists to prevent.
func emitAuditEvent(eventType, action, resourceType, resourceID, resourceName string, success bool, details map[string]string) error {
	acfg := audit.DefaultConfig()
	if !acfg.Enabled {
		return nil
	}

	path := auditLogPath()
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("audit: create log dir for %s: %w", path, err)
		}
	}
	storage, err := audit.NewFileStorage(path, audit.DefaultFileStorageOptions())
	if err != nil {
		return fmt.Errorf("audit: open log %s: %w", path, err)
	}
	defer func() { _ = storage.Close() }()

	actor := cliActorID()
	code := "ERROR"
	if success {
		code = "OK"
	}
	return storage.Write(&audit.AuditEvent{
		Timestamp: time.Now().UTC(),
		EventType: eventType,
		Level:     audit.AuditLevelInfo,
		Category:  audit.AuditCategoryData,
		Action:    action,
		Actor:     audit.AuditActor{ID: actor, Type: "cli-user", Name: actor},
		Resource:  audit.AuditResource{ID: resourceID, Type: resourceType, Name: resourceName},
		Result:    audit.AuditResult{Success: success, Code: code},
		Context:   audit.AuditContext{Metadata: details},
	})
}

func newAuditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "View audit logs",
		Long: `View audit log events recorded by the CLI, daemon, and MCP server.

Events are persisted as JSON lines in the audit log file.`,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show audit logging status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := audit.DefaultConfig()
			fmt.Printf("Audit logging: %s\n", enabledStr(cfg.Enabled))
			fmt.Printf("Storage type:  %s\n", cfg.StorageType)
			fmt.Printf("Log file:      %s\n", auditLogPath())
			return nil
		},
	})

	var limit int
	show := &cobra.Command{
		Use:   "show",
		Short: "Show recent audit events",
		RunE: func(cmd *cobra.Command, args []string) error {
			events, err := readAuditEvents(auditLogPath(), limit)
			if err != nil {
				return err
			}
			if len(events) == 0 {
				fmt.Println("No audit events found.")
				return nil
			}
			for _, e := range events {
				fmt.Printf("%s\t%s\t%s\n", e.Timestamp.Format(time.RFC3339), e.EventType, e.Actor.ID)
			}
			return nil
		},
	}
	show.Flags().IntVar(&limit, "limit", 50, "Number of events to show")
	cmd.AddCommand(show)

	var logLimit int
	var logFilter string
	logCmd := &cobra.Command{
		Use:   "log",
		Short: "Show recent audit events (optionally filtered)",
		RunE: func(cmd *cobra.Command, args []string) error {
			events, err := readAuditEvents(auditLogPath(), 0)
			if err != nil {
				return err
			}
			var matched []audit.AuditEvent
			for _, e := range events {
				if auditEventMatches(e, logFilter) {
					matched = append(matched, e)
				}
			}
			if logLimit > 0 && len(matched) > logLimit {
				matched = matched[:logLimit]
			}
			if len(matched) == 0 {
				fmt.Println("No audit events found.")
				return nil
			}
			for _, e := range matched {
				fmt.Printf("%s\t%s\t%s\n", e.Timestamp.Format(time.RFC3339), e.EventType, e.Actor.ID)
			}
			return nil
		},
	}
	logCmd.Flags().IntVar(&logLimit, "limit", 50, "Maximum number of events to show")
	logCmd.Flags().StringVar(&logFilter, "filter", "", "Filter events by event type, action, or level (substring)")
	cmd.AddCommand(logCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "info",
		Short: "Show audit configuration and statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := audit.DefaultConfig()
			fmt.Printf("Audit logging:    %s\n", enabledStr(cfg.Enabled))
			fmt.Printf("Storage type:     %s\n", cfg.StorageType)
			fmt.Printf("Retention:        %d days\n", cfg.RetentionDays)
			redact := "not redacted"
			if cfg.RedactSensitive {
				redact = "redacted"
			}
			fmt.Printf("Sensitive fields: %s\n", redact)
			events, err := readAuditEvents(auditLogPath(), 0)
			if err != nil {
				return err
			}
			fmt.Printf("Events on disk:   %d\n", len(events))
			fmt.Printf("Log file:         %s\n", auditLogPath())
			return nil
		},
	})

	return cmd
}

func enabledStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled (set MUSTER_AUDIT_ENABLED=1)"
}

// auditLogPath returns the audit sink path for the management plane. It is the
// package-local name for audit.SinkPath, the single resolution shared with the
// command-level hook (internal/audit.Hook) so both writers and the `audit log`
// reader agree on one file: $MUSTER_AUDIT_LOG when set, else
// <config dir>/muster/audit.jsonl.
func auditLogPath() string {
	return audit.SinkPath()
}

// readAuditEvents reads JSON-line audit events from path. A limit <= 0 reads
// all events.
func readAuditEvents(path string, limit int) ([]audit.AuditEvent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read audit log: %w", err)
	}
	var events []audit.AuditEvent
	for _, line := range splitLines(string(data)) {
		var e audit.AuditEvent
		if json.Unmarshal([]byte(line), &e) == nil {
			events = append(events, e)
		}
		if limit > 0 && len(events) >= limit {
			break
		}
	}
	return events, nil
}

// auditEventMatches reports whether e matches the filter. Matching is a
// case-insensitive substring against EventType, Action, and Level.
func auditEventMatches(e audit.AuditEvent, filter string) bool {
	if filter == "" {
		return true
	}
	f := strings.ToLower(filter)
	if strings.Contains(strings.ToLower(e.EventType), f) {
		return true
	}
	if strings.Contains(strings.ToLower(e.Action), f) {
		return true
	}
	if strings.Contains(strings.ToLower(string(e.Level)), f) {
		return true
	}
	return false
}

// splitLines splits s on newlines, dropping empty lines.
func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
