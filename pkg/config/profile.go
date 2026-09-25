package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ProfileFlagName is the global persistent CLI flag that selects the active
// configuration profile for one invocation. It has the highest priority in
// the profile resolution order (flag > MUSTER_PROFILE > persisted
// default_profile).
const ProfileFlagName = "profile"

// ProfileEnvVar is the environment variable that selects the active
// configuration profile for one invocation.
const ProfileEnvVar = "MUSTER_PROFILE"

// ProfileSource names where the active profile selection came from.
type ProfileSource string

// Profile resolution sources, in decreasing priority order.
const (
	ProfileSourceFlag      ProfileSource = "flag"
	ProfileSourceEnv       ProfileSource = "env"
	ProfileSourcePersisted ProfileSource = "persisted"
	ProfileSourceNone      ProfileSource = "none"
)

// ProfileSelection is the resolved active profile and where it came from.
type ProfileSelection struct {
	Name   string
	Source ProfileSource
}

// BindProfileFlag registers the global persistent --profile flag on cmd's
// persistent flag set. It is idempotent: calling it on a command (or its
// parents) that already carries the flag is a no-op, so root wiring and
// command constructors can both call it safely.
func BindProfileFlag(cmd *cobra.Command) {
	if cmd == nil {
		return
	}
	if cmd.PersistentFlags().Lookup(ProfileFlagName) != nil {
		return
	}
	cmd.PersistentFlags().String(ProfileFlagName, "", "Configuration profile to use (overrides MUSTER_PROFILE and the persisted default)")
}

// ResolveProfile returns the active profile for this invocation and its
// source. Resolution order: --profile flag (only when explicitly set) >
// MUSTER_PROFILE > persisted default_profile in cfg > none. Empty strings
// at any layer are skipped, falling through to the next source. Both cmd
// and cfg may be nil.
func ResolveProfile(cmd *cobra.Command, cfg *Config) ProfileSelection {
	if cmd != nil {
		flag := cmd.Flags().Lookup(ProfileFlagName)
		if flag == nil {
			// A bare (not-yet-executed) cobra command keeps persistent
			// flags in their own set; real executions merge them into
			// Flags(), but check both so direct callers work either way.
			flag = cmd.PersistentFlags().Lookup(ProfileFlagName)
		}
		if flag != nil && flag.Changed {
			if name := strings.TrimSpace(flag.Value.String()); name != "" {
				return ProfileSelection{Name: name, Source: ProfileSourceFlag}
			}
		}
	}
	if name := strings.TrimSpace(os.Getenv(ProfileEnvVar)); name != "" {
		return ProfileSelection{Name: name, Source: ProfileSourceEnv}
	}
	if cfg != nil {
		if name := strings.TrimSpace(cfg.DefaultProfile); name != "" {
			return ProfileSelection{Name: name, Source: ProfileSourcePersisted}
		}
	}
	return ProfileSelection{Name: "", Source: ProfileSourceNone}
}

// UserConfigFilePath resolves the user's config.yaml path, reusing the same
// precedence the loader already honors: an explicit cfg.ConfigFile, then the
// MUSTER_CONFIG / MUSTER_CONFIG_FILE environment variables, then the first
// existing standard config file, and finally the XDG user config path.
// The directory part of the result is created when it does not exist, so
// callers can write directly to the returned path. cfg may be nil.
func UserConfigFilePath(cfg *Config) (string, error) {
	if cfg != nil && cfg.ConfigFile != "" {
		return cfg.ConfigFile, nil
	}
	for _, env := range []string{"MUSTER_CONFIG", "MUSTER_CONFIG_FILE"} {
		if path := strings.TrimSpace(os.Getenv(env)); path != "" {
			return path, nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	// Standard paths, lowest priority (legacy home fallback) first so the
	// first existing one is the highest-priority standard location.
	candidates := []string{
		filepath.Join(home, ".muster", "config.yaml"),
		filepath.Join(home, ".config", "muster", "config.yaml"),
	}
	for _, path := range candidates {
		if _, statErr := os.Stat(path); statErr == nil {
			return path, nil
		}
	}

	path := filepath.Join(home, ".config", "muster", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create config directory %s: %w", filepath.Dir(path), err)
	}
	return path, nil
}

// WriteDefaultProfile persists name as default_profile in the user's config
// file using a read-modify-write that preserves every other key. An empty
// name removes the key. The write is idempotent: writing the same name twice
// produces byte-identical files. A config file that cannot be parsed is an
// error and is left untouched rather than clobbered.
//
// The path is resolved with UserConfigFilePath. cfg supplies an explicit
// ConfigFile override when set (tests use this to redirect persistence to a
// temp directory); cfg may otherwise be nil.
func WriteDefaultProfile(cfg *Config, name string) error {
	path, err := UserConfigFilePath(cfg)
	if err != nil {
		return err
	}

	// Read-modify-write: parse the existing document so unrelated keys
	// survive, and refuse to write when the file exists but is unparseable.
	var doc yaml.Node
	if raw, readErr := os.ReadFile(path); readErr == nil {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("parse existing config %s: %w", path, err)
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read config %s: %w", path, readErr)
	}

	// Unmarshal wraps the document root: descend from the document node to
	// the root mapping. A zero-kind node means the file is missing or empty
	// and we start from a fresh mapping.
	root := &doc
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			root.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
		}
		root = root.Content[0]
	}
	if root.Kind == 0 {
		*root = yaml.Node{Kind: yaml.MappingNode}
	}
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("config root in %s is not a mapping (kind %d); refusing to modify", path, root.Kind)
	}

	if err := setMappingKey(root, "default_profile", name); err != nil {
		return err
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// setMappingKey sets key to a string scalar with the given value on a
// mapping node, replacing the value in place when the key already exists.
// An empty value removes the key. Unknown node shapes are an error rather
// than silently dropped writes.
func setMappingKey(doc *yaml.Node, key, value string) error {
	if doc.Kind != yaml.MappingNode {
		return fmt.Errorf("cannot set %q on a non-mapping node (kind %d)", key, doc.Kind)
	}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value == key {
			if value == "" {
				// Remove the key and its value node.
				doc.Content = append(doc.Content[:i], doc.Content[i+2:]...)
				return nil
			}
			doc.Content[i+1].SetString(value)
			return nil
		}
	}
	if value == "" {
		return nil
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str"}
	valueNode.SetString(value)
	doc.Content = append(doc.Content, keyNode, valueNode)
	return nil
}
