// Package cmdstore provides persistent storage and management for dynamically generated CLI commands.
// It enables commands generated from OpenAPI specifications to be saved, loaded, and executed across sessions.
//
// # Overview
//
// When commands are generated from OpenAPI specs using the generator package, cmdstore persists them
// to disk (typically ~/.muster/commands.json) so they remain available in future CLI sessions. This
// eliminates the need to regenerate commands on every startup.
//
// The package manages:
//   - Command metadata storage and retrieval
//   - Spec hash tracking for cache invalidation
//   - Base URL and configuration persistence
//   - Thread-safe concurrent access
//
// # Architecture
//
// The package consists of three main components:
//
// 1. CommandStore: Manages persistent storage with thread-safe access
// 2. CommandMetadata: Represents serializable command information
// 3. FlagMetadata: Represents serializable flag definitions
//
// # Usage
//
// Create or load a command store:
//
//	store, err := cmdstore.New("~/.muster/commands.json")
//	if err != nil {
//	    return err
//	}
//
// Save generated commands:
//
//	metadata := &cmdstore.CommandMetadata{
//	    Name:    "get-pet",
//	    Use:     "get-pet [id]",
//	    Short:   "Get a pet by ID",
//	    Path:    "/pets/{petId}",
//	    Method:  "GET",
//	    Flags:   []cmdstore.FlagMetadata{...},
//	    BaseURL: "https://api.example.com",
//	}
//	err = store.SaveCommands([]*cmdstore.CommandMetadata{metadata})
//
// Load commands for CLI registration:
//
//	commands, err := store.LoadCommands()
//	if err != nil {
//	    return err
//	}
//	for _, cmd := range commands {
//	    rootCmd.AddCommand(buildCobraCommand(cmd))
//	}
//
// # File Format
//
// Commands are stored in JSON format:
//
//	{
//	  "version": "1.0.0",
//	  "spec_path": "petstore.yaml",
//	  "spec_hash": "abc123...",
//	  "base_url": "https://api.example.com",
//	  "commands": [...]
//	}
//
// # Thread Safety
//
// All operations are thread-safe using sync.RWMutex for concurrent read/write access.
package cmdstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CommandMetadata represents persistable command information
type CommandMetadata struct {
	Name        string         `json:"name"`
	Use         string         `json:"use"`
	Short       string         `json:"short"`
	Long        string         `json:"long,omitempty"`
	Example     string         `json:"example,omitempty"`
	Path        string         `json:"path"`   // OpenAPI path (e.g., "/pets")
	Method      string         `json:"method"` // HTTP method
	Flags       []FlagMetadata `json:"flags,omitempty"`
	GeneratedAt time.Time      `json:"generated_at"`
	SpecHash    string         `json:"spec_hash"`
	Args        []string       `json:"args,omitempty"` // Path parameter names
	BaseURL     string         `json:"base_url"`       // API base URL
}

// FlagMetadata represents persistable flag information
type FlagMetadata struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Type        string `json:"type"` // string, int, bool, etc.
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

// CommandStore manages persistent storage for generated commands
type CommandStore struct {
	dataPath string
	data     *StoredData
	mu       sync.RWMutex
}

// StoredData represents the stored command data structure
type StoredData struct {
	Version     string            `json:"version"`
	SpecPath    string            `json:"spec_path"`
	SpecHash    string            `json:"spec_hash"`
	BaseURL     string            `json:"base_url"`
	GeneratedAt time.Time         `json:"generated_at"`
	Commands    []CommandMetadata `json:"commands"`
}

// NewCommandStore creates a new command store
func NewCommandStore(dataPath string) *CommandStore {
	return &CommandStore{
		dataPath: dataPath,
	}
}

// Save saves command metadata to persistent storage
func (s *CommandStore) Save(metadata *StoredData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(s.dataPath), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	s.data = metadata

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := os.WriteFile(s.dataPath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// Load loads command metadata from persistent storage
func (s *CommandStore) Load() (*StoredData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data != nil {
		return s.data, nil
	}

	data, err := os.ReadFile(s.dataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Not an error, just no data
		}
		return nil, fmt.Errorf("read file: %w", err)
	}

	var metadata StoredData
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}

	s.data = &metadata
	return s.data, nil
}

// Clear removes all stored data
func (s *CommandStore) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = nil
	if err := os.Remove(s.dataPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}

// HasData checks if there is stored data
func (s *CommandStore) HasData() bool {
	meta, err := s.Load()
	if err != nil {
		return false
	}
	return meta != nil && len(meta.Commands) > 0
}

// Commands returns the list of stored commands
func (s *CommandStore) Commands() ([]CommandMetadata, error) {
	meta, err := s.Load()
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return []CommandMetadata{}, nil
	}
	return meta.Commands, nil
}

// UpdateBaseURL updates the base URL in stored metadata
func (s *CommandStore) UpdateBaseURL(baseURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data == nil {
		if data, err := s.loadInternal(); err != nil {
			return err
		} else if data == nil {
			return fmt.Errorf("no stored data")
		} else {
			s.data = data
		}
	}

	s.data.BaseURL = baseURL
	for i := range s.data.Commands {
		s.data.Commands[i].BaseURL = baseURL
	}

	return s.saveInternal(s.data)
}

func (s *CommandStore) loadInternal() (*StoredData, error) {
	data, err := os.ReadFile(s.dataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read file: %w", err)
	}

	var metadata StoredData
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}

	return &metadata, nil
}

func (s *CommandStore) saveInternal(metadata *StoredData) error {
	if err := os.MkdirAll(filepath.Dir(s.dataPath), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := os.WriteFile(s.dataPath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
