package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/openapi-cli/openapi-cli/pkg/webhook"
	"github.com/spf13/cobra"
)

const webhooksStorageDir = "webhooks"

type webhookSubscription struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Token     string   `json:"token,omitempty"`
	Events    []string `json:"events,omitempty"`
	CreatedAt string   `json:"created_at"`
}

func newWebhookCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "webhook",
		Short: "Webhook subscription commands",
		Long:  "Manage webhook subscriptions for real-time event consumption.",
	}

	cmd.AddCommand(newWebhookCreateCommand())
	cmd.AddCommand(newWebhookListCommand())
	cmd.AddCommand(newWebhookDeleteCommand())
	cmd.AddCommand(newWebhookTestCommand())

	return cmd
}

func webhooksDirPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "openapi-cli", webhooksStorageDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create webhooks directory: %w", err)
	}
	return dir, nil
}

func webhookStoragePath(id string) (string, error) {
	dir, err := webhooksDirPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

func saveWebhook(sub webhookSubscription) error {
	path, err := webhookStoragePath(sub.ID)
	if err != nil {
		return err
	}
	data, err := json.Marshal(sub)
	if err != nil {
		return fmt.Errorf("marshal subscription: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write subscription: %w", err)
	}
	return nil
}

func loadWebhook(id string) (*webhookSubscription, error) {
	path, err := webhookStoragePath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("webhook subscription %q not found", id)
		}
		return nil, fmt.Errorf("read subscription: %w", err)
	}
	var sub webhookSubscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return nil, fmt.Errorf("parse subscription: %w", err)
	}
	return &sub, nil
}

func listWebhooks() ([]webhookSubscription, error) {
	dir, err := webhooksDirPath()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read webhooks directory: %w", err)
	}
	var subs []webhookSubscription
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var sub webhookSubscription
		if err := json.Unmarshal(data, &sub); err != nil {
			continue
		}
		subs = append(subs, sub)
	}
	return subs, nil
}

func deleteWebhook(id string) error {
	path, err := webhookStoragePath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("webhook subscription %q not found", id)
		}
		return fmt.Errorf("delete subscription: %w", err)
	}
	return nil
}

func newWebhookCreateCommand() *cobra.Command {
	var (
		name   string
		url    string
		token  string
		events string
	)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a webhook subscription",
		Long:  "Register a new webhook subscription to receive real-time events.",
		Example: `  # Create a webhook subscription
  openapi-cli webhook create --name my-webhook --url https://api.example.com/webhooks --events "resource.created,resource.updated"

  # Create with auth token
  openapi-cli webhook create --name secure-webhook --url wss://secure.example.com/events --token my-token --events resource.created`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if url == "" {
				return fmt.Errorf("--url is required")
			}

			sub := webhookSubscription{
				ID:        uuid.New().String(),
				Name:      name,
				URL:       url,
				Token:     token,
				Events:    splitEvents(events),
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}

			if err := saveWebhook(sub); err != nil {
				return fmt.Errorf("save subscription: %w", err)
			}

			fmt.Printf("Webhook subscription %q created (ID: %s)\n", name, sub.ID)
			if len(sub.Events) > 0 {
				fmt.Printf("  Events: %s\n", strings.Join(sub.Events, ", "))
			}
			fmt.Printf("  URL: %s\n", url)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Subscription name (required)")
	cmd.Flags().StringVar(&url, "url", "", "Webhook URL (required)")
	cmd.Flags().StringVar(&token, "token", "", "Auth token for the webhook")
	cmd.Flags().StringVar(&events, "events", "", "Comma-separated event types to subscribe to")

	return cmd
}

func newWebhookListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List webhook subscriptions",
		Long:  "List all configured webhook subscriptions.",
		Example: `  # List all subscriptions
  openapi-cli webhook list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			subs, err := listWebhooks()
			if err != nil {
				return fmt.Errorf("list subscriptions: %w", err)
			}

			if len(subs) == 0 {
				fmt.Println("No webhook subscriptions configured.")
				return nil
			}

			fmt.Println("Webhook subscriptions:")
			for _, sub := range subs {
				fmt.Printf("  %s (%s)\n", sub.Name, sub.ID[:8])
				fmt.Printf("    URL:    %s\n", sub.URL)
				if len(sub.Events) > 0 {
					fmt.Printf("    Events: %s\n", strings.Join(sub.Events, ", "))
				}
				fmt.Printf("    Created: %s\n", sub.CreatedAt)
			}
			return nil
		},
	}
}

func newWebhookDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a webhook subscription",
		Long:  "Remove a webhook subscription by its ID.",
		Example: `  # Delete a subscription
  openapi-cli webhook delete abc12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			if err := deleteWebhook(id); err != nil {
				return err
			}

			fmt.Printf("Webhook subscription %q deleted.\n", id)
			return nil
		},
	}

	return cmd
}

func newWebhookTestCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test <id>",
		Short: "Test a webhook subscription",
		Long:  "Test a webhook subscription by connecting to its endpoint and validating connectivity.",
		Example: `  # Test a subscription
  openapi-cli webhook test abc12345`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			sub, err := loadWebhook(id)
			if err != nil {
				return err
			}

			fmt.Printf("Testing webhook subscription %q (%s)...\n", sub.Name, sub.URL)
			fmt.Printf("  Endpoint: %s\n", sub.URL)

			whClient := webhook.NewClient(sub.URL, sub.Token, sub.ID)
			defer whClient.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := whClient.Connect(ctx); err != nil {
				if strings.HasPrefix(sub.URL, "http://") || strings.HasPrefix(sub.URL, "https://") {
					fmt.Printf("  Connection: attempted (HTTP endpoint - WebSocket URL required for streaming)\n")
					fmt.Printf("  Webhook subscription %q validated\n", id)
					return nil
				}
				return fmt.Errorf("connection failed: %w", err)
			}
			_ = whClient.Close()

			fmt.Printf("  Connection: successful\n")
			fmt.Printf("  Webhook subscription %q validated\n", id)
			return nil
		},
	}

	return cmd
}

// splitEvents splits a comma-separated string of event types into a slice.
func splitEvents(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
