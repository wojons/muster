package builtin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/openapi-cli/openapi-cli/pkg/tenant"
	"github.com/spf13/cobra"
)

type tenantCliEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Tier      string `json:"tier,omitempty"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
}

type tenantCliState struct {
	ActiveTenant string           `json:"active_tenant"`
	Tenants      []tenantCliEntry `json:"tenants"`
}

// newTenantCommand creates the tenant command group
func newTenantCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tenant",
		Short: "Multi-tenancy context management",
		Long:  "Manage multi-tenancy context, including setting active tenant, listing tenants, and viewing current context.",
	}

	cmd.AddCommand(newTenantSetCommand())
	cmd.AddCommand(newTenantListCommand())
	cmd.AddCommand(newTenantCurrentCommand())

	return cmd
}

func tenantStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home directory: %w", err)
	}
	return filepath.Join(home, ".config", "openapi-cli", "tenants.json"), nil
}

func loadTenantState() (*tenantCliState, error) {
	path, err := tenantStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &tenantCliState{Tenants: []tenantCliEntry{}}, nil
		}
		return nil, fmt.Errorf("read tenant state: %w", err)
	}

	var state tenantCliState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse tenant state: %w", err)
	}

	if state.Tenants == nil {
		state.Tenants = []tenantCliEntry{}
	}

	return &state, nil
}

func saveTenantState(state *tenantCliState) error {
	path, err := tenantStatePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create tenant config directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal tenant state: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write tenant state: %w", err)
	}

	return nil
}

// newTenantSetCommand creates the tenant set command
func newTenantSetCommand() *cobra.Command {
	var name, tier string

	cmd := &cobra.Command{
		Use:   "set <tenant-id>",
		Short: "Set active tenant context",
		Long:  "Set the active tenant context by ID. Optionally provide tenant name and tier.",
		Example: `  # Set active tenant
  openapi-cli tenant set acme-corp

  # Set active tenant with metadata
  openapi-cli tenant set acme-corp --name "ACME Corp" --tier pro`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tenantID := args[0]

			tid, err := tenant.ParseTenantID(tenantID)
			if err != nil {
				return fmt.Errorf("invalid tenant ID: %w", err)
			}

			state, err := loadTenantState()
			if err != nil {
				return err
			}

			for i := range state.Tenants {
				state.Tenants[i].Active = false
			}

			found := false
			for i := range state.Tenants {
				if state.Tenants[i].ID == string(tid) {
					state.Tenants[i].Active = true
					if name != "" {
						state.Tenants[i].Name = name
					}
					if tier != "" {
						state.Tenants[i].Tier = tier
					}
					found = true
					break
				}
			}

			if !found {
				entry := tenantCliEntry{
					ID:        string(tid),
					Name:      name,
					Tier:      tier,
					Active:    true,
					CreatedAt: time.Now().UTC().Format(time.RFC3339),
				}
				state.Tenants = append(state.Tenants, entry)
			}

			state.ActiveTenant = string(tid)

			if err := saveTenantState(state); err != nil {
				return err
			}

			fmt.Printf("Active tenant set to: %s", tid)
			if name != "" {
				fmt.Printf(" (%s)", name)
			}
			fmt.Println()

			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Human-readable tenant name")
	cmd.Flags().StringVar(&tier, "tier", "", "Tenant tier (free, pro, enterprise, dedicated)")

	return cmd
}

// newTenantListCommand creates the tenant list command
func newTenantListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available tenants",
		Long:  "Display all configured tenants and indicate the active one.",
		Example: `  # List all tenants
  openapi-cli tenant list`,
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := loadTenantState()
			if err != nil {
				return err
			}

			if len(state.Tenants) == 0 {
				fmt.Println("No tenants configured.")
				fmt.Println("Use 'openapi-cli tenant set <tenant-id>' to add one.")
				return nil
			}

			fmt.Println("Available tenants:")
			for _, t := range state.Tenants {
				prefix := "  "
				if t.Active {
					prefix = "  *"
				}

				line := fmt.Sprintf("%s %s", prefix, t.ID)
				if t.Name != "" {
					line += fmt.Sprintf(" (%s)", t.Name)
				}
				if t.Tier != "" {
					line += fmt.Sprintf(" [%s]", t.Tier)
				}
				if t.Active {
					line += " (active)"
				}
				fmt.Println(line)
			}

			return nil
		},
	}
}

// newTenantCurrentCommand creates the tenant current command
func newTenantCurrentCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Show current tenant context",
		Long:  "Display the currently active tenant context with details.",
		Example: `  # Show current tenant
  openapi-cli tenant current`,
		RunE: func(cmd *cobra.Command, args []string) error {
			state, err := loadTenantState()
			if err != nil {
				return err
			}

			if state.ActiveTenant == "" {
				fmt.Println("No active tenant context.")
				fmt.Println("Use 'openapi-cli tenant set <tenant-id>' to set one.")
				return nil
			}

			fmt.Printf("Active tenant: %s\n", state.ActiveTenant)

			for _, t := range state.Tenants {
				if t.ID == state.ActiveTenant {
					if t.Name != "" {
						fmt.Printf("Name:   %s\n", t.Name)
					}
					if t.Tier != "" {
						fmt.Printf("Tier:   %s\n", t.Tier)
					}
					if t.CreatedAt != "" {
						fmt.Printf("Since:  %s\n", t.CreatedAt)
					}
					break
				}
			}

			return nil
		},
	}
}
