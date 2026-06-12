package builtin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/audit"
	"github.com/spf13/cobra"
)

func auditDataDir() (string, error) {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home directory: %w", err)
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataDir, "openapi-cli"), nil
}

func auditLogPath() (string, error) {
	dir, err := auditDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "audit.jsonl"), nil
}

func newAuditCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Audit logging commands",
		Long:  "View and manage audit log entries.",
	}

	cmd.AddCommand(newAuditLogCommand())
	cmd.AddCommand(newAuditInfoCommand())

	return cmd
}

func newAuditLogCommand() *cobra.Command {
	var (
		limit  int
		filter string
	)

	cmd := &cobra.Command{
		Use:   "log",
		Short: "List recent audit entries",
		Long:  "Display recent audit log entries with optional filtering.",
		Example: `  # Show recent audit entries
  openapi-cli audit log

  # Show last 100 entries
  openapi-cli audit log --limit 100

  # Filter by action or category
  openapi-cli audit log --filter action=login
  openapi-cli audit log --filter category=authentication`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := auditLogPath()
			if err != nil {
				return fmt.Errorf("audit log path: %w", err)
			}

			f, err := os.Open(path)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No audit entries found.")
					return nil
				}
				return fmt.Errorf("open audit log: %w", err)
			}
			defer f.Close()

			var entries []audit.Event
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				var entry audit.Event
				if err := json.Unmarshal([]byte(line), &entry); err != nil {
					continue
				}
				entries = append(entries, entry)
			}
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read audit log: %w", err)
			}

			// Apply filter if specified
			if filter != "" {
				entries = applyFilter(entries, filter)
			}

			// Apply limit from the end (most recent first)
			if limit > 0 && len(entries) > limit {
				entries = entries[len(entries)-limit:]
			}

			if len(entries) == 0 {
				fmt.Println("No audit entries match the filter.")
				return nil
			}

			fmt.Printf("Audit log (%d entries):\n", len(entries))
			fmt.Println(strings.Repeat("-", 80))
			for _, entry := range entries {
				fmt.Printf("[%s] %s | %s | %s\n",
					entry.Timestamp.Format(time.RFC3339),
					entry.Level,
					entry.Action,
					entry.Result.Message,
				)
				fmt.Printf("      Actor: %s (%s) | Resource: %s/%s\n",
					entry.Actor.Name, entry.Actor.ID,
					entry.Resource.Type, entry.Resource.ID,
				)
				if len(entry.Changes) > 0 {
					for _, change := range entry.Changes {
						fmt.Printf("      Change: %s: %v -> %v\n", change.Field, change.OldValue, change.NewValue)
					}
				}
				fmt.Println(strings.Repeat("-", 80))
			}

			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of entries to show")
	cmd.Flags().StringVar(&filter, "filter", "", "Filter entries by key=value (e.g. action=login, category=authentication)")

	return cmd
}

func newAuditInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show audit configuration and stats",
		Long:  "Display the current audit logging configuration and statistics.",
		Example: `  # Show audit info
  openapi-cli audit info`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := audit.DefaultConfig()

			path, err := auditLogPath()
			if err != nil {
				return fmt.Errorf("audit log path: %w", err)
			}

			fmt.Println("Audit Configuration:")
			fmt.Printf("  Enabled:         %t\n", cfg.Enabled)
			fmt.Printf("  Storage Type:    %s\n", cfg.StorageType)
			fmt.Printf("  Retention Days:  %d\n", cfg.RetentionDays)
			fmt.Printf("  Buffer Size:     %d\n", cfg.BufferSize)
			fmt.Printf("  Flush Interval:  %s\n", cfg.FlushInterval)
			fmt.Printf("  Hash Algorithm:  %s\n", cfg.HashAlgorithm)
			fmt.Printf("  Include Body:    %t\n", cfg.IncludeBody)
			fmt.Printf("  Sensitive Fields: %v\n", cfg.SensitiveFields)
			fmt.Printf("  Log Path:        %s\n", path)

			// Stats
			entryCount := countEntries(path)
			var fileSize int64
			if fi, err := os.Stat(path); err == nil {
				fileSize = fi.Size()
			}

			fmt.Println()
			fmt.Println("Audit Statistics:")
			fmt.Printf("  Total Entries:   %d\n", entryCount)
			fmt.Printf("  File Size:       %d bytes\n", fileSize)
			if fileSize > 0 {
				fmt.Printf("  Avg Entry Size:  %d bytes\n", fileSize/int64(entryCount))
			}

			return nil
		},
	}
}

func applyFilter(entries []audit.Event, filter string) []audit.Event {
	var filtered []audit.Event

	parts := strings.SplitN(filter, "=", 2)
	if len(parts) != 2 {
		return entries
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	value := strings.TrimSpace(parts[1])

	for _, entry := range entries {
		match := false
		switch key {
		case "action":
			match = strings.EqualFold(entry.Action, value)
		case "category":
			match = strings.EqualFold(string(entry.Category), value)
		case "level":
			match = strings.EqualFold(string(entry.Level), value)
		case "actor":
			match = strings.EqualFold(entry.Actor.ID, value) || strings.EqualFold(entry.Actor.Name, value)
		case "resource":
			match = strings.EqualFold(entry.Resource.Type, value) || strings.EqualFold(entry.Resource.ID, value)
		case "success":
			match = (value == "true" && entry.Result.Success) || (value == "false" && !entry.Result.Success)
		default:
			match = true
		}
		if match {
			filtered = append(filtered, entry)
		}
	}

	return filtered
}

func countEntries(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			count++
		}
	}
	return count
}

func ensureAuditLogExists() error {
	path, err := auditLogPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create audit directory: %w", err)
	}
	return nil
}
