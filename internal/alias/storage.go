package alias

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Storage defines the interface for alias persistence operations.
// Implementations can use different storage backends (file, database, etc.)
type Storage interface {
	// Load retrieves all aliases from storage.
	// Returns an empty map if no aliases exist.
	// Returns an error if loading fails.
	Load() (map[string]*Alias, error)

	// Save persists all aliases to storage.
	// Returns an error if saving fails.
	Save(aliases map[string]*Alias) error

	// Path returns the storage location identifier (file path, DB connection, etc.)
	Path() string
}

// YAMLStorage implements Storage using YAML file persistence.
// It provides thread-safe read/write operations to a YAML file.
type YAMLStorage struct {
	path string
	mu   sync.RWMutex
}

// NewYAMLStorage creates a new YAML-based storage at the specified path.
// The path should be an absolute file path where aliases will be stored.
// Parent directories are created automatically on first Save.
func NewYAMLStorage(path string) *YAMLStorage {
	return &YAMLStorage{
		path: path,
	}
}

// Load reads aliases from the YAML file.
// Returns an empty map if the file doesn't exist.
// Returns an error if the file exists but cannot be read or parsed.
func (s *YAMLStorage) Load() (map[string]*Alias, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty map for non-existent file
			return make(map[string]*Alias), nil
		}
		return nil, fmt.Errorf("failed to read aliases file: %w", err)
	}

	// Parse YAML into intermediate map
	rawAliases := make(map[string]Alias)
	if err := yaml.Unmarshal(data, &rawAliases); err != nil {
		return nil, fmt.Errorf("failed to parse aliases file: %w", err)
	}

	// Convert to pointer map
	aliases := make(map[string]*Alias, len(rawAliases))
	for name, alias := range rawAliases {
		alias.Name = name // Ensure name is set from the key
		aliases[name] = &alias
	}

	return aliases, nil
}

// Save writes aliases to the YAML file.
// Creates parent directories if they don't exist.
// Uses file permissions 0644 for the YAML file.
func (s *YAMLStorage) Save(aliases map[string]*Alias) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Convert to value map for YAML serialization
	rawAliases := make(map[string]Alias, len(aliases))
	for name, alias := range aliases {
		rawAliases[name] = *alias
	}

	data, err := yaml.Marshal(rawAliases)
	if err != nil {
		return fmt.Errorf("failed to marshal aliases: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create aliases directory: %w", err)
	}

	// Write file with restricted permissions
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("failed to write aliases file: %w", err)
	}

	return nil
}

// Path returns the file path used for storage.
func (s *YAMLStorage) Path() string {
	return s.path
}
