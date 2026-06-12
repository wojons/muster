package wasm

import (
	"fmt"
	"path"
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
// Supports both "category.resource" and "category" formats.
// A dotless capability checks if ANY resource is granted for that category.
func (cs *CapabilitySet) Check(capability string) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	parts := strings.Split(capability, ".")
	category := parts[0]

	if len(parts) < 2 {
		_, hasCap := cs.capabilities[category]
		_, hasWild := cs.wildcards[category]
		return hasCap || hasWild
	}

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

// sanitizeResourceName validates and sanitizes a resource name.
// It rejects path traversal sequences and normalizes the path.
func sanitizeResourceName(resource string) (string, error) {
	if resource == "" {
		return "", fmt.Errorf("resource name cannot be empty")
	}

	// Reject path traversal sequences
	if strings.Contains(resource, "..") {
		return "", fmt.Errorf("path traversal sequences are not allowed")
	}

	// Reject backslashes (Windows-style paths)
	if strings.Contains(resource, "\\") {
		return "", fmt.Errorf("backslashes are not allowed in resource names")
	}

	// Only normalize if it looks like an absolute path
	if strings.HasPrefix(resource, "/") {
		cleaned := path.Clean(resource)

		// Ensure cleaning didn't introduce issues
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return "", fmt.Errorf("normalized path escapes root")
		}

		return cleaned, nil
	}

	return resource, nil
}

// matchWildcard checks if a resource matches a wildcard pattern.
// Both pattern and resource are sanitized and normalized before comparison.
func matchWildcard(pattern, resource string) bool {
	// Sanitize and normalize the resource
	sanitizedResource, err := sanitizeResourceName(resource)
	if err != nil {
		return false
	}

	// Sanitize the pattern (but allow wildcards)
	sanitizedPattern, err := sanitizePattern(pattern)
	if err != nil {
		return false
	}

	if sanitizedPattern == "*" {
		return true
	}

	if strings.HasSuffix(sanitizedPattern, "**") {
		prefix := strings.TrimSuffix(sanitizedPattern, "**")
		if prefix == "" {
			return true
		}
		return strings.HasPrefix(sanitizedResource, prefix)
	}

	if strings.Contains(sanitizedPattern, "*") {
		parts := strings.Split(sanitizedPattern, "*")
		if len(parts) != 2 {
			return false
		}
		return strings.HasPrefix(sanitizedResource, parts[0]) && strings.HasSuffix(sanitizedResource, parts[1])
	}

	return sanitizedPattern == sanitizedResource
}

// sanitizePattern sanitizes a wildcard pattern while preserving wildcards.
func sanitizePattern(pattern string) (string, error) {
	if pattern == "" {
		return "", fmt.Errorf("pattern cannot be empty")
	}

	// Special case: wildcard all
	if pattern == "*" {
		return "*", nil
	}

	// Reject backslashes
	if strings.Contains(pattern, "\\") {
		return "", fmt.Errorf("backslashes are not allowed in patterns")
	}

	// For patterns ending with **, clean the prefix if it starts with /
	if strings.HasSuffix(pattern, "**") {
		prefix := strings.TrimSuffix(pattern, "**")
		if prefix == "" {
			return "**", nil
		}
		// Always check for path traversal
		if strings.Contains(prefix, "..") {
			return "", fmt.Errorf("pattern contains path traversal")
		}
		// Only clean if it looks like an absolute path
		if strings.HasPrefix(prefix, "/") {
			cleaned := path.Clean(prefix)
			if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
				return "", fmt.Errorf("pattern contains path traversal")
			}
			// Ensure we preserve the slash before **
			if !strings.HasSuffix(cleaned, "/") && prefix != "" {
				return cleaned + "/**", nil
			}
			return cleaned + "**", nil
		}
		return pattern, nil
	}

	// For patterns with single *, check each part for traversal
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid pattern format")
		}
		// Always check for path traversal in both parts
		if strings.Contains(parts[0], "..") || strings.Contains(parts[1], "..") {
			return "", fmt.Errorf("pattern contains path traversal")
		}
		// Only clean if parts look like absolute paths
		cleaned0 := parts[0]
		cleaned1 := parts[1]
		if strings.HasPrefix(parts[0], "/") {
			cleaned0 = path.Clean(parts[0])
			if cleaned0 == ".." || strings.HasPrefix(cleaned0, "../") {
				return "", fmt.Errorf("pattern contains path traversal")
			}
		}
		if strings.HasPrefix(parts[1], "/") {
			cleaned1 = path.Clean(parts[1])
			if cleaned1 == ".." || strings.HasSuffix(cleaned1, "/..") {
				return "", fmt.Errorf("pattern contains path traversal")
			}
		}
		return cleaned0 + "*" + cleaned1, nil
	}

	// Simple pattern - always check for traversal first
	if strings.Contains(pattern, "..") {
		return "", fmt.Errorf("pattern contains path traversal")
	}

	// Only clean if it looks like an absolute path
	if strings.HasPrefix(pattern, "/") {
		cleaned := path.Clean(pattern)
		if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.HasSuffix(cleaned, "/..") {
			return "", fmt.Errorf("pattern contains path traversal")
		}
		return cleaned, nil
	}

	return pattern, nil
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
