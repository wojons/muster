package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

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
	doc, err := loadConfigDoc(path)
	if err != nil {
		return err
	}
	if err := setMappingKey(doc.root, "default_profile", name); err != nil {
		return err
	}
	return doc.save(path)
}

// WriteProfile creates or updates the named profile in the user's config file
// under `profiles:`, setting one key per entry in values. Keys are dot-paths
// ("base_url", "default_flags.tier") and values are stored as YAML strings —
// a value like "true" or "30" stays a string rather than being coerced, so the
// CLI never rewrites a profile key into another type.
//
// It uses the same read-modify-write persistence path as WriteDefaultProfile:
// every other key in the document survives (including the persisted
// default_profile written by `config profile switch`), an unparsable or
// non-mapping config file is an error rather than clobbered, and writing the
// same values twice produces byte-identical output. Keys are written in sorted
// order for that reason. An empty value removes that key, matching
// WriteDefaultProfile's empty-name semantics.
//
// The caller decides whether an existing profile is acceptable: the function
// itself behaves as create-or-update. Use ReloadProfiles afterwards to make
// the in-memory copies match what a new process will load.
func WriteProfile(cfg *Config, name string, values map[string]string) error {
	path, err := UserConfigFilePath(cfg)
	if err != nil {
		return err
	}
	doc, err := loadConfigDoc(path)
	if err != nil {
		return err
	}
	profiles, err := mappingValue(doc.root, "profiles", true)
	if err != nil {
		return err
	}
	profile, err := mappingValue(profiles, name, true)
	if err != nil {
		return fmt.Errorf("profile %q: %w", name, err)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := setDotPath(profile, key, values[key]); err != nil {
			return err
		}
	}
	return doc.save(path)
}

// NormalizeProfileName validates a user-supplied profile name and returns the
// spelling the configuration loader will read back.
//
// The loader lowercases every configuration key and splits keys on ".", so a
// profile named "Staging" loads as "staging" and one named "team.prod" loads
// as a nested `team:` mapping instead of a profile. Both are normalized or
// rejected here rather than written to disk as a profile that a later process
// can never select.
func NormalizeProfileName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("profile name must not be empty")
	}
	if strings.Contains(trimmed, ".") {
		return "", fmt.Errorf("invalid profile name %q: '.' separates configuration keys and would nest the profile instead of naming it", name)
	}
	for _, r := range trimmed {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return "", fmt.Errorf("invalid profile name %q: whitespace and control characters are not allowed", name)
		}
	}
	return strings.ToLower(trimmed), nil
}

// ParseProfileSetPairs converts repeated --set key=value values into the
// dot-path map WriteProfile persists. A pair without "=", or with an empty
// key, is an error. Values keep their exact text — the split happens on the
// first "=" only, so URLs, header values, and tokens containing "=" survive.
// Keys are lowercased because the loader reads configuration keys
// case-insensitively, so the file stores the spelling the loader uses.
func ParseProfileSetPairs(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	values := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, found := strings.Cut(pair, "=")
		if !found {
			return nil, fmt.Errorf("invalid --set %q: expected key=value", pair)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			return nil, fmt.Errorf("invalid --set %q: key must not be empty", pair)
		}
		values[key] = value
	}
	return values, nil
}

// ProfileKeyGroups lists the profile key spellings the configuration loader
// reads, for help text and warnings. Map groups are addressed with a nested
// path, e.g. --set default_flags.tier=pro.
func ProfileKeyGroups() []string {
	return []string{
		"base_url",
		"auth_type",
		"timeout",
		"auth_config.<key>",
		"default_flags.<key>",
		"headers.<key>",
	}
}

// knownProfileSetHead reports whether a top-level segment of a --set key is a
// profile key the loader reads into a Profile.
func knownProfileSetHead(head string) bool {
	switch head {
	case "base_url", "auth_type", "timeout", "auth_config", "default_flags", "headers":
		return true
	default:
		return false
	}
}

// UnknownProfileSetKeys returns the distinct sorted top-level segments of
// values that the configuration loader does not read. They are still
// persisted, but they have no effect on the loaded profile, so a caller can
// warn about the typo or the stale key spelling (the nested `spec:`/`http:`
// shape some guides show, for example).
func UnknownProfileSetKeys(values map[string]string) []string {
	seen := make(map[string]bool, len(values))
	var unknown []string
	for key := range values {
		head, _, _ := strings.Cut(key, ".")
		if knownProfileSetHead(head) || seen[head] {
			continue
		}
		seen[head] = true
		unknown = append(unknown, head)
	}
	sort.Strings(unknown)
	return unknown
}

// configDoc is a parsed config file: the document node that is marshalled
// back, plus its root mapping node for edits.
type configDoc struct {
	doc  *yaml.Node
	root *yaml.Node
}

// loadConfigDoc reads the YAML document at path. A missing or empty file
// starts a fresh mapping; a file whose root is not a mapping is an error, so a
// document we do not understand is refused instead of clobbered.
func loadConfigDoc(path string) (*configDoc, error) {
	var doc yaml.Node
	if raw, readErr := os.ReadFile(path); readErr == nil {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parse existing config %s: %w", path, err)
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return nil, fmt.Errorf("read config %s: %w", path, readErr)
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
		return nil, fmt.Errorf("config root in %s is not a mapping (kind %d); refusing to modify", path, root.Kind)
	}
	return &configDoc{doc: &doc, root: root}, nil
}

// save writes the document back to path with owner-only permissions,
// creating the containing directory when needed.
func (d *configDoc) save(path string) error {
	out, err := yaml.Marshal(d.doc)
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

// mappingValue returns the mapping value stored under key. With create set, a
// missing key is appended as a new mapping, and an explicit null (a bare
// `profiles:` line) is upgraded to a mapping. A value of any other shape is an
// error: refusing beats replacing a scalar or sequence we did not author.
func mappingValue(mapping *yaml.Node, key string, create bool) (*yaml.Node, error) {
	if mapping.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("cannot address key %q on a non-mapping node (kind %d)", key, mapping.Kind)
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		value := mapping.Content[i+1]
		if value.Kind == yaml.MappingNode {
			return value, nil
		}
		if create && value.Tag == "!!null" {
			value.Kind = yaml.MappingNode
			value.Tag = ""
			value.Value = ""
			return value, nil
		}
		return nil, fmt.Errorf("config key %q is not a mapping (kind %d); refusing to modify", key, value.Kind)
	}
	if !create {
		return nil, nil
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valueNode := &yaml.Node{Kind: yaml.MappingNode}
	mapping.Content = append(mapping.Content, keyNode, valueNode)
	return valueNode, nil
}

// setDotPath sets a string scalar at the dot-separated path inside mapping,
// creating (or upgrading an empty) intermediate mapping as needed. An empty
// value removes the leaf key, matching setMappingKey.
func setDotPath(mapping *yaml.Node, path, value string) error {
	parts := strings.Split(path, ".")
	node := mapping
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("invalid profile key %q: empty path segment", path)
		}
		if i == len(parts)-1 {
			return setMappingKey(node, part, value)
		}
		next, err := mappingValue(node, part, true)
		if err != nil {
			return fmt.Errorf("profile key %q: %w", path, err)
		}
		node = next
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
