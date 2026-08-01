package builtin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/internal/audit"
	"github.com/wojons/muster/pkg/config"
)

// ── webhook ──────────────────────────────────────────────────────────────
// Webhook subscriptions are stored as JSON in the config directory.

type webhookSub struct {
	ID        string `json:"id"`
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
			for _, s := range subs {
				fmt.Printf("%s\t%s\t%s\n", s.ID, s.URL, s.EventType)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "subscribe <url> <event-type>",
		Short: "Subscribe to an API event",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			subs, err := loadWebhooks(cfg)
			if err != nil {
				return err
			}
			sub := webhookSub{
				ID:        fmt.Sprintf("wh-%d", time.Now().UnixNano()),
				URL:       args[0],
				EventType: args[1],
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}
			subs = append(subs, sub)
			if err := saveWebhooks(cfg, subs); err != nil {
				return err
			}
			fmt.Printf("Subscribed %s -> %s (event: %s)\n", sub.ID, sub.URL, sub.EventType)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "unsubscribe <id>",
		Short: "Remove a webhook subscription",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			subs, err := loadWebhooks(cfg)
			if err != nil {
				return err
			}
			kept := subs[:0]
			found := false
			for _, s := range subs {
				if s.ID == args[0] {
					found = true
					continue
				}
				kept = append(kept, s)
			}
			if !found {
				return fmt.Errorf("webhook %s not found", args[0])
			}
			if err := saveWebhooks(cfg, kept); err != nil {
				return err
			}
			fmt.Printf("Unsubscribed %s\n", args[0])
			return nil
		},
	})

	return cmd
}

// ── tenant ───────────────────────────────────────────────────────────────
// Tenants map to config profiles per the multi-tenancy spec.

func newTenantCommand(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenant",
		Short: "Manage tenant context",
		Long: `Manage the multi-tenant context for this CLI.

Each tenant maps to a named config profile with isolated settings,
quotas, and audit records.`,
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
			if cfg == nil || len(cfg.Profiles) == 0 {
				fmt.Println("No tenants configured.")
				return nil
			}
			names := make([]string, 0, len(cfg.Profiles))
			for name := range cfg.Profiles {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				marker := " "
				if name == cfg.DefaultProfile {
					marker = "*"
				}
				fmt.Printf("%s %s\n", marker, name)
			}
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "switch <tenant-id>",
		Short: "Switch current tenant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg == nil {
				return fmt.Errorf("config not initialized")
			}
			if _, ok := cfg.Profiles[args[0]]; !ok && len(cfg.Profiles) > 0 {
				return fmt.Errorf("unknown tenant %q — use 'tenant list'", args[0])
			}
			cfg.DefaultProfile = args[0]
			fmt.Printf("Switched to tenant %s\n", args[0])
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

	return cmd
}

func enabledStr(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled (set MUSTER_AUDIT_ENABLED=1)"
}

// auditLogPath returns the default audit log file path.
func auditLogPath() string {
	if p := os.Getenv("MUSTER_AUDIT_LOG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "muster", "audit.log")
}

// readAuditEvents reads up to limit JSON-line audit events from path.
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
		if len(events) >= limit {
			break
		}
		var e audit.AuditEvent
		if json.Unmarshal([]byte(line), &e) == nil {
			events = append(events, e)
		}
	}
	return events, nil
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
