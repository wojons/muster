// Package alias provides command alias management and expansion for the Muster CLI.
//
// The alias system allows users to define custom command shortcuts and abbreviations
// that map to longer commands. Aliases are persisted to YAML storage and support
// recursive expansion with cycle detection to prevent infinite loops.
//
// Basic usage:
//
//	manager, err := alias.NewManager(configDir)
//	if err != nil {
//	    return err
//	}
//
//	// Define a new alias
//	err = manager.Define("ls", "inventory list")
//
//	// Retrieve an alias
//	cmd, exists := manager.Get("ls")
//
//	// Expand aliases in command arguments
//	expander := alias.NewExpander(manager)
//	expanded, err := expander.ExpandCommand(ctx, []string{"ls", "--format", "table"})
//
// The Manager handles alias storage operations:
//   - Define: Creates or updates an alias
//   - Remove: Deletes an alias
//   - Get: Retrieves a single alias
//   - List: Returns all defined aliases
//   - Expand: Replaces an alias with its full command
//
// The Expander provides recursive expansion with safety features:
//   - Circular reference detection prevents infinite loops
//   - Expansion depth limit (10 levels)
//   - History tracking for each expansion chain
//
// Storage format (aliases.yaml):
//
//	ls: inventory list --format table
//	get: inventory get
//	prod: config set api.url https://prod.example.com
//
// Default aliases are loaded on first run:
//   - ls: inventory list
//   - get: inventory get
//   - add: inventory add
//   - login: auth login
//   - start: daemon start
//
// Reserved names cannot be used as aliases:
//   - alias, unalias, help, version
//
// Thread safety: Manager uses mutex protection for concurrent access to the
// alias map. All operations are safe for concurrent use.
package alias
