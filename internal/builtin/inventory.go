package builtin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"
	"github.com/wojons/muster/pkg/config"
)

// inventoryEntry is a configured spec inventory: host URL + local file.
type inventoryEntry struct {
	Host     string `json:"host"`
	File     string `json:"file"`
	AddedAt  string `json:"added_at,omitempty"`
	LastSync string `json:"last_sync,omitempty"`
}

func inventoryStorePath(cfg *config.Config) string {
	if cfg != nil && cfg.ConfigDir != "" {
		return filepath.Join(cfg.ConfigDir, "inventories.json")
	}
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "muster", "inventories.json")
}

func loadInventories(cfg *config.Config) ([]inventoryEntry, error) {
	data, err := os.ReadFile(inventoryStorePath(cfg))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read inventories: %w", err)
	}
	var invs []inventoryEntry
	if err := json.Unmarshal(data, &invs); err != nil {
		return nil, fmt.Errorf("parse inventories: %w", err)
	}
	return invs, nil
}

func saveInventories(cfg *config.Config, invs []inventoryEntry) error {
	data, err := json.MarshalIndent(invs, "", "  ")
	if err != nil {
		return err
	}
	path := inventoryStorePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// newInventoryCommand creates the inventory management command with
// add/list/remove subcommands per the built-in commands spec.
func newInventoryCommand(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Manage spec inventories",
		Long: `Manage OpenAPI spec inventories.

Each inventory maps a remote host URL to a local inventory file.`,
	}

	// inventory add --host URL --file inventory.json
	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add an inventory",
		RunE: func(cmd *cobra.Command, args []string) error {
			host, _ := cmd.Flags().GetString("host")
			file, _ := cmd.Flags().GetString("file")
			if host == "" {
				return fmt.Errorf("--host is required")
			}
			if file == "" {
				return fmt.Errorf("--file is required")
			}
			invs, err := loadInventories(cfg)
			if err != nil {
				return err
			}
			for _, inv := range invs {
				if inv.Host == host {
					return fmt.Errorf("inventory for %s already exists", host)
				}
			}
			invs = append(invs, inventoryEntry{
				Host:    host,
				File:    file,
				AddedAt: time.Now().UTC().Format(time.RFC3339),
			})
			if err := saveInventories(cfg, invs); err != nil {
				return err
			}
			fmt.Printf("Added inventory for %s (file: %s)\n", host, file)
			return nil
		},
	}
	addCmd.Flags().String("host", "", "Inventory host URL")
	addCmd.Flags().String("file", "", "Local inventory file path")
	cmd.AddCommand(addCmd)

	// inventory list
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List configured inventories",
		RunE: func(cmd *cobra.Command, args []string) error {
			invs, err := loadInventories(cfg)
			if err != nil {
				return err
			}
			if len(invs) == 0 {
				fmt.Println("No inventories configured.")
				return nil
			}
			sort.Slice(invs, func(i, j int) bool { return invs[i].Host < invs[j].Host })
			for _, inv := range invs {
				fmt.Printf("%s\t%s\n", inv.Host, inv.File)
			}
			return nil
		},
	})

	// inventory remove --host URL
	removeCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove an inventory",
		RunE: func(cmd *cobra.Command, args []string) error {
			host, _ := cmd.Flags().GetString("host")
			if host == "" {
				return fmt.Errorf("--host is required")
			}
			invs, err := loadInventories(cfg)
			if err != nil {
				return err
			}
			kept := invs[:0]
			found := false
			for _, inv := range invs {
				if inv.Host == host {
					found = true
					continue
				}
				kept = append(kept, inv)
			}
			if !found {
				return fmt.Errorf("inventory for %s not found", host)
			}
			if err := saveInventories(cfg, kept); err != nil {
				return err
			}
			fmt.Printf("Removed inventory for %s\n", host)
			return nil
		},
	}
	removeCmd.Flags().String("host", "", "Inventory host URL")
	cmd.AddCommand(removeCmd)

	return cmd
}
