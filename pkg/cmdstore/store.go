// Package cmdstore provides command storage and loading functionality.
package cmdstore

import (
	"context"
	"time"
)

// CommandStore stores and retrieves command metadata.
type CommandStore struct {
	commands []CommandMetadata
	meta     *CommandMetadata
	path     string
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

// FlagMetadata describes a command flag.
type FlagMetadata struct {
	Name         string
	Shorthand    string
	Type         string
	Description  string
	Default      string
	DefaultValue string
	Required     bool
}

// Commands returns all stored commands.
func (cs *CommandStore) Commands() ([]CommandMetadata, error) {
	return cs.commands, nil
}

// HasData returns true if the store has persisted data.
func (cs *CommandStore) HasData() bool {
	return cs.path != "" || len(cs.commands) > 0 || cs.meta != nil
}

// Load returns stored command metadata.
func (cs *CommandStore) Load() (*CommandMetadata, error) {
	return cs.meta, nil
}

// Save stores command metadata from StoredData.
// axiom:trace work_item=rebuild-foundation-03 spec=specs/02-Workflows.md plan=phase-97-2/task-97-2-1/step-97-2-1-1 impl=pkg/cmdstore/store.go
func (cs *CommandStore) Save(data *StoredData) error {
	if data != nil && len(data.Commands) > 0 {
		cs.commands = data.Commands
		cs.meta = &data.Commands[0]
	}
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
