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

// findWebhook returns the subscription with the given ID.
func findWebhook(cfg *config.Config, id string) (*webhookSub, error) {
	subs, err := loadWebhooks(cfg)
	if err != nil {
		return nil, err
	}
	for i := range subs {
		if subs[i].ID == id {
			return &subs[i], nil
		}
	}
	return nil, fmt.Errorf("webhook %s not found", id)
}

// removeWebhook deletes the subscription with the given ID and persists.
func removeWebhook(cfg *config.Config, id string) error {
	subs, err := loadWebhooks(cfg)
	if err != nil {
		return err
	}
	kept := subs[:0]
	found := false
	for _, s := range subs {
		if s.ID == id {
			found = true
			continue
		}
		kept = append(kept, s)
	}
	if !found {
		return fmt.Errorf("webhook %s not found", id)
	}
	return saveWebhooks(cfg, kept)
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
		Use:     "delete <id>",
		Aliases: []string{"unsubscribe"},
		Short:   "Remove a webhook subscription",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := removeWebhook(cfg, args[0]); err != nil {
				return err
			}
			fmt.Printf("Deleted webhook %s\n", args[0])
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "test <id>",
		Short: "Test connectivity to a webhook URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sub, err := findWebhook(cfg, args[0])
			if err != nil {
				return err
			}
			status, err := probeWebhookURL(sub.URL)
			if err != nil {
				return fmt.Errorf("webhook %s (%s) unreachable: %w", sub.ID, sub.URL, err)
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

// setActiveTenant validates the tenant, persists the active marker to the
// tenants store, and updates the in-memory default profile.
func setActiveTenant(cfg *config.Config, id string) error {
	if cfg == nil {
		return fmt.Errorf("config not initialized")
	}
	if _, ok := cfg.Profiles[id]; !ok && len(cfg.Profiles) > 0 {
		return fmt.Errorf("unknown tenant %q — use 'tenant list'", id)
	}
	entries, err := loadTenants(cfg)
	if err != nil {
		return err
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

// auditLogPath returns the default audit log file path. The CLI reads the
// JSON-lines audit store; MUSTER_AUDIT_LOG overrides the location.
func auditLogPath() string {
	if p := os.Getenv("MUSTER_AUDIT_LOG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "muster", "audit.jsonl")
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
