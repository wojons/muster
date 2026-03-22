package alias

// Alias represents a named command shortcut that maps to a longer command.
// Aliases are stored persistently and can be expanded at runtime.
type Alias struct {
	// Name is the alias identifier (e.g., "ls", "get")
	Name string `json:"name" yaml:"name"`

	// Command is the full command that the alias expands to
	Command string `json:"command" yaml:"command"`

	// Description is an optional human-readable explanation of the alias
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// NewAlias creates a new Alias with the given name and command.
func NewAlias(name, command string) *Alias {
	return &Alias{
		Name:    name,
		Command: command,
	}
}

// NewAliasWithDescription creates a new Alias with a description.
func NewAliasWithDescription(name, command, description string) *Alias {
	return &Alias{
		Name:        name,
		Command:     command,
		Description: description,
	}
}

// String returns the alias in "name=command" format for display.
func (a *Alias) String() string {
	return a.Name + "=" + a.Command
}

// IsValid checks if the alias has valid name and command fields.
func (a *Alias) IsValid() bool {
	return a.Name != "" && a.Command != ""
}
