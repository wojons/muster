package wasm

import (
	"fmt"
	"strings"
	"sync"
)

// Capability represents a permission token.
type Capability struct {
	Category    string            `json:"category"`
	Resource    string            `json:"resource"`
	Constraints map[string]string `json:"constraints,omitempty"`
}

// Permission represents a permission type.
type Permission string

const (
	PermRead    Permission = "read"
	PermWrite   Permission = "write"
	PermExecute Permission = "execute"
	PermCreate  Permission = "create"
	PermDelete  Permission = "delete"
)

// CapabilitySet represents a set of granted capabilities.
type CapabilitySet struct {
	capabilities map[string][]string // category -> resources
	wildcards    map[string][]string // category -> wildcard patterns
	mu           sync.RWMutex
}

// NewCapabilitySet creates a new capability set.
func NewCapabilitySet() *CapabilitySet {
	return &CapabilitySet{
		capabilities: make(map[string][]string),
		wildcards:    make(map[string][]string),
	}
}

// Grant adds a capability to the set.
func (cs *CapabilitySet) Grant(capability string) error {
	parts := strings.Split(capability, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid capability format: %s", capability)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	category := parts[0]
	resource := strings.Join(parts[1:], ".")

	if strings.Contains(resource, "*") {
		cs.wildcards[category] = append(cs.wildcards[category], resource)
	} else {
		cs.capabilities[category] = append(cs.capabilities[category], resource)
	}

	return nil
}

// Check verifies if a capability is granted.
func (cs *CapabilitySet) Check(capability string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	parts := strings.Split(capability, ".")
	if len(parts) < 2 {
		return false
	}

	category := parts[0]
	resource := strings.Join(parts[1:], ".")

	for _, r := range cs.capabilities[category] {
		if r == resource {
			return true
		}
	}

	for _, pattern := range cs.wildcards[category] {
		if matchWildcard(pattern, resource) {
			return true
		}
	}

	return false
}

// Revoke removes a capability from the set.
func (cs *CapabilitySet) Revoke(capability string) error {
	parts := strings.Split(capability, ".")
	if len(parts) < 2 {
		return fmt.Errorf("invalid capability format: %s", capability)
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	category := parts[0]
	resource := strings.Join(parts[1:], ".")

	if strings.Contains(resource, "*") {
		for i, r := range cs.wildcards[category] {
			if r == resource {
				cs.wildcards[category] = append(cs.wildcards[category][:i], cs.wildcards[category][i+1:]...)
				return nil
			}
		}
	} else {
		for i, r := range cs.capabilities[category] {
			if r == resource {
				cs.capabilities[category] = append(cs.capabilities[category][:i], cs.capabilities[category][i+1:]...)
				return nil
			}
		}
	}

	return fmt.Errorf("capability not found: %s", capability)
}

// List returns all granted capabilities.
func (cs *CapabilitySet) List() []Capability {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	var caps []Capability
	for category, resources := range cs.capabilities {
		for _, resource := range resources {
			caps = append(caps, Capability{
				Category: category,
				Resource: resource,
			})
		}
	}
	for category, patterns := range cs.wildcards {
		for _, resource := range patterns {
			caps = append(caps, Capability{
				Category: category,
				Resource: resource,
			})
		}
	}
	return caps
}

// matchWildcard checks if a resource matches a wildcard pattern.
func matchWildcard(pattern, resource string) bool {
	if pattern == "*" {
		return true
	}

	if strings.HasSuffix(pattern, "**") {
		prefix := strings.TrimSuffix(pattern, "**")
		if prefix == "" {
			return true
		}
		return strings.HasPrefix(resource, prefix)
	}

	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) != 2 {
			return false
		}
		return strings.HasPrefix(resource, parts[0]) && strings.HasSuffix(resource, parts[1])
	}

	return pattern == resource
}

// ParseCapability parses a capability string.
func ParseCapability(capStr string) (*Capability, error) {
	if capStr == "" {
		return nil, fmt.Errorf("invalid capability format: empty string")
	}

	parts := strings.SplitN(capStr, ".", 2)
	if len(parts) < 1 || parts[0] == "" {
		return nil, fmt.Errorf("invalid capability format: missing category")
	}

	cap := &Capability{
		Category: parts[0],
	}

	if len(parts) > 1 {
		cap.Resource = parts[1]
	}

	return cap, nil
}
