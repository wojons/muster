// Package cmdstore provides command storage and loading functionality.
package cmdstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CommandStore stores and retrieves command metadata.
type CommandStore struct {
	commands []CommandMetadata
	meta     *CommandMetadata
	path     string
	loaded   bool
}

// NewCommandStore creates a new command store. If a path is provided,
// the store will persist commands to that file.
func NewCommandStore(path ...string) *CommandStore {
	cs := &CommandStore{
		commands: make([]CommandMetadata, 0),
	}
	if len(path) > 0 && path[0] != "" {
		cs.path = path[0]
	}
	return cs
}

// CommandMetadata describes a stored CLI command.
type CommandMetadata struct {
	Name        string
	Description string
	Method      string
	Path        string
	BaseURL     string
	Use         string
	Short       string
	Long        string
	Example     string
	Args        []string
	Flags       []FlagMetadata
}

// FlagKind classifies a flag as coming from path, query, header, or request body.
type FlagKind string

const (
	FlagKindPath      FlagKind = "path"
	FlagKindQuery     FlagKind = "query"
	FlagKindHeader    FlagKind = "header"
	FlagKindBody      FlagKind = "body"
	FlagKindFile      FlagKind = "file"      // multipart file upload flag
	FlagKindMultipart FlagKind = "multipart" // non-file multipart form field
)

// FlagMetadata describes a command flag.
// axiom:trace work_item=DEX-205 spec=specs/02-Workflows.md impl=pkg/cmdstore/store.go
type FlagMetadata struct {
	Name         string
	Shorthand    string
	Type         string
	Kind         FlagKind `json:"kind,omitempty"`
	JSONName     string   `json:"json_name,omitempty"` // original property name for body flags
	Description  string
	Default      string
	DefaultValue string
	Required     bool
}

// Commands returns all stored commands.
func (cs *CommandStore) Commands() ([]CommandMetadata, error) {
	if !cs.loaded && cs.path != "" {
		if err := cs.loadFromDisk(); err != nil {
			return nil, fmt.Errorf("load from disk: %w", err)
		}
	}
	return cs.commands, nil
}

// HasData returns true if the store has persisted data on disk or loaded commands.
func (cs *CommandStore) HasData() bool {
	if len(cs.commands) > 0 {
		return true
	}
	if cs.path == "" {
		return false
	}
	// Check if the file exists on disk
	info, err := os.Stat(cs.path)
	return err == nil && info.Size() > 0
}

// Load returns stored command metadata.
func (cs *CommandStore) Load() (*CommandMetadata, error) {
	if cs.meta != nil {
		return cs.meta, nil
	}
	if cs.path != "" {
		if err := cs.loadFromDisk(); err != nil {
			return nil, err
		}
	}
	if cs.meta != nil {
		return cs.meta, nil
	}
	if len(cs.commands) > 0 {
		cs.meta = &cs.commands[0]
		return cs.meta, nil
	}
	return nil, fmt.Errorf("no stored data")
}

// Save stores command metadata from StoredData and persists to disk.
// axiom:trace work_item=rebuild-foundation-03 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-1 impl=pkg/cmdstore/store.go
func (cs *CommandStore) Save(data *StoredData) error {
	if data != nil && len(data.Commands) > 0 {
		cs.commands = data.Commands
		cs.meta = &data.Commands[0]
		cs.loaded = true
	}
	// Persist to disk if path is configured
	if cs.path != "" {
		return cs.persistToDisk()
	}
	return nil
}

// persistToDisk writes the stored data as JSON to disk.
func (cs *CommandStore) persistToDisk() error {
	if cs.path == "" {
		return nil
	}
	// Ensure parent directory exists
	dir := filepath.Dir(cs.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create store dir: %w", err)
	}

	stored := StoredData{
		Version:     "1.0",
		Commands:    cs.commands,
		GeneratedAt: time.Now(),
	}
	if cs.meta != nil {
		stored.BaseURL = cs.meta.BaseURL
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal stored data: %w", err)
	}
	return os.WriteFile(cs.path, data, 0644)
}

// loadFromDisk reads the stored data from the JSON file on disk.
func (cs *CommandStore) loadFromDisk() error {
	if cs.path == "" {
		return fmt.Errorf("no store path configured")
	}
	data, err := os.ReadFile(cs.path)
	if err != nil {
		if os.IsNotExist(err) {
			cs.loaded = true // Mark as loaded (empty)
			return nil
		}
		return fmt.Errorf("read store file: %w", err)
	}
	var stored StoredData
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("unmarshal stored data: %w", err)
	}
	cs.commands = stored.Commands
	if len(stored.Commands) > 0 {
		cs.meta = &stored.Commands[0]
	}
	cs.loaded = true
	return nil
}

// StoredData represents persistent command metadata with spec info.
// axiom:trace work_item=rebuild-foundation-03 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-1 impl=pkg/cmdstore/store.go
type StoredData struct {
	Version     string            `json:"version"`
	SpecPath    string            `json:"spec_path"`
	SpecHash    string            `json:"spec_hash"`
	BaseURL     string            `json:"base_url"`
	GeneratedAt time.Time         `json:"generated_at"`
	Commands    []CommandMetadata `json:"commands"`
}

// LoadCommands loads commands from storage (compatibility shim).
func LoadCommands(ctx context.Context, store *CommandStore) ([]CommandMetadata, error) {
	return store.Commands()
}

// ReconstructCommand rebuilds a cobra.Command from metadata (compatibility shim).
func ReconstructCommand(ctx context.Context, meta CommandMetadata) (interface{}, error) {
	// Returns nil — real implementation would build a cobra.Command
	return nil, nil
}
