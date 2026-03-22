package alias

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Manager handles alias storage, retrieval, and expansion with thread-safe operations.
type Manager struct {
	mu      sync.RWMutex
	aliases map[string]*Alias
	file    string
}

// NewManager creates a new alias manager with the given configuration directory.
// It loads existing aliases from the aliases.yaml file if it exists.
func NewManager(configDir string) (*Manager, error) {
	file := filepath.Join(configDir, "aliases.yaml")

	m := &Manager{
		aliases: make(map[string]*Alias),
		file:    file,
	}

	if err := m.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load aliases: %w", err)
	}

	return m, nil
}

// Define creates or updates an alias with the given name and command.
// Returns an error if the alias name is invalid or if saving fails.
func (m *Manager) Define(name, command string) error {
	return m.DefineWithDescription(name, command, "")
}

// DefineWithDescription creates or updates an alias with a description.
// Returns an error if the alias name is invalid or if saving fails.
func (m *Manager) DefineWithDescription(name, command, description string) error {
	if err := validateAliasName(name); err != nil {
		return err
	}

	if command == "" {
		return fmt.Errorf("alias command cannot be empty")
	}

	alias := NewAliasWithDescription(name, command, description)

	m.mu.Lock()
	m.aliases[name] = alias
	m.mu.Unlock()

	return m.save()
}

// Remove deletes an alias by name.
// Returns an error if the alias does not exist or if saving fails.
func (m *Manager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.aliases[name]; !exists {
		return fmt.Errorf("alias '%s' not found", name)
	}

	delete(m.aliases, name)
	return m.save()
}

// Get retrieves an alias by name.
// Returns the alias and true if found, or nil and false if not found.
func (m *Manager) Get(name string) (*Alias, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	alias, exists := m.aliases[name]
	return alias, exists
}

// List returns a copy of all aliases.
// The returned slice is safe to modify without affecting the Manager.
func (m *Manager) List() []*Alias {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Alias, 0, len(m.aliases))
	for _, alias := range m.aliases {
		result = append(result, alias)
	}
	return result
}

// Expand replaces the first argument with its aliased command if it exists.
// Returns the expanded arguments and true if expansion occurred, or the original
// arguments and false if no alias was found.
func (m *Manager) Expand(args []string) ([]string, bool) {
	if len(args) == 0 {
		return args, false
	}

	aliasName := args[0]
	alias, exists := m.Get(aliasName)
	if !exists {
		return args, false
	}

	// Split the aliased command
	expanded := strings.Fields(alias.Command)

	// Append remaining args
	if len(args) > 1 {
		expanded = append(expanded, args[1:]...)
	}

	return expanded, true
}

// load reads aliases from the storage file.
func (m *Manager) load() error {
	data, err := os.ReadFile(m.file)
	if err != nil {
		return err
	}

	// Store intermediate map since we need to unmarshal into a simple type
	rawAliases := make(map[string]Alias)
	if err := yaml.Unmarshal(data, &rawAliases); err != nil {
		return fmt.Errorf("failed to parse aliases file: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for name, alias := range rawAliases {
		alias.Name = name // Ensure name is set from the key
		m.aliases[name] = &alias
	}

	return nil
}

// save writes aliases to the storage file.
func (m *Manager) save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Convert to simple map for YAML serialization
	rawAliases := make(map[string]Alias, len(m.aliases))
	for name, alias := range m.aliases {
		rawAliases[name] = *alias
	}

	data, err := yaml.Marshal(rawAliases)
	if err != nil {
		return fmt.Errorf("failed to marshal aliases: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(m.file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create aliases directory: %w", err)
	}

	return os.WriteFile(m.file, data, 0644)
}

// validateAliasName checks if the alias name is valid.
func validateAliasName(name string) error {
	if name == "" {
		return fmt.Errorf("alias name cannot be empty")
	}

	if strings.ContainsAny(name, " \t\n") {
		return fmt.Errorf("alias name cannot contain whitespace")
	}

	// Reserved names that cannot be used as aliases
	reserved := []string{"alias", "unalias", "help", "version"}
	for _, r := range reserved {
		if name == r {
			return fmt.Errorf("'%s' is a reserved name", name)
		}
	}

	return nil
}

// Count returns the number of aliases currently stored.
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.aliases)
}

// Clear removes all aliases from memory (does not affect persisted file).
// This is primarily useful for testing.
func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.aliases = make(map[string]*Alias)
}

// FilePath returns the path to the aliases storage file.
func (m *Manager) FilePath() string {
	return m.file
}
