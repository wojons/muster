package completion

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// APIClient defines the interface for API clients used in dynamic completion
type APIClient interface {
	// ListResources fetches resources of a given type
	ListResources(ctx context.Context, resourceType string) ([]map[string]interface{}, error)
	// ListNamespaces fetches available namespaces
	ListNamespaces(ctx context.Context) ([]string, error)
	// ListProfiles fetches available configuration profiles
	ListProfiles(ctx context.Context) ([]string, error)
}

// DynamicCompleter provides dynamic completion from API resources
type DynamicCompleter struct {
	client APIClient
	cache  *Cache
}

// NewDynamicCompleter creates a new dynamic completer
func NewDynamicCompleter(client APIClient) *DynamicCompleter {
	defaultTTL := 5 * time.Minute
	return &DynamicCompleter{
		client: client,
		cache:  NewCache(defaultTTL),
	}
}

// CompleteResources provides completion for resource names
func (dc *DynamicCompleter) CompleteResources(ctx context.Context, toComplete string) ([]string, error) {
	key := "resources:"

	if cached, ok := dc.cache.Get(key); ok {
		entries := cached.([]string)
		return filterCompletions(entries, toComplete), nil
	}

	if dc.client == nil {
		return []string{}, nil
	}

	resources, err := dc.client.ListResources(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("fetch resources: %w", err)
	}

	var names []string
	for _, r := range resources {
		if name, ok := r["name"].(string); ok {
			names = append(names, name)
		}
	}

	dc.cache.Set(key, names)

	return filterCompletions(names, toComplete), nil
}

// CompleteNamespaces provides completion for namespace names
func (dc *DynamicCompleter) CompleteNamespaces(ctx context.Context, toComplete string) ([]string, error) {
	key := "namespaces"

	if cached, ok := dc.cache.Get(key); ok {
		entries := cached.([]string)
		return filterCompletions(entries, toComplete), nil
	}

	if dc.client == nil {
		return []string{"default"}, nil
	}

	namespaces, err := dc.client.ListNamespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch namespaces: %w", err)
	}

	if len(namespaces) == 0 {
		namespaces = []string{"default"}
	}

	dc.cache.Set(key, namespaces)

	return filterCompletions(namespaces, toComplete), nil
}

// CompleteProfiles provides completion for configuration names
func (dc *DynamicCompleter) CompleteProfiles(ctx context.Context, toComplete string) ([]string, error) {
	key := "profiles"

	if cached, ok := dc.cache.Get(key); ok {
		entries := cached.([]string)
		return filterCompletions(entries, toComplete), nil
	}

	if dc.client == nil {
		return []string{"default"}, nil
	}

	profiles, err := dc.client.ListProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch profiles: %w", err)
	}

	if len(profiles) == 0 {
		profiles = []string{"default"}
	}

	dc.cache.Set(key, profiles)

	return filterCompletions(profiles, toComplete), nil
}

// CompleteHTTPMethods provides completion for HTTP methods
func (dc *DynamicCompleter) CompleteHTTPMethods(toComplete string) []string {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	return filterCompletions(methods, toComplete)
}

// CompleteOutputFormats provides completion for output formats
func (dc *DynamicCompleter) CompleteOutputFormats(toComplete string) []string {
	formats := []string{"json", "yaml", "table", "csv", "raw"}
	return filterCompletions(formats, toComplete)
}

// filterCompletions filters completions based on prefix
func filterCompletions(completions []string, prefix string) []string {
	if prefix == "" {
		return completions
	}

	var filtered []string
	for _, c := range completions {
		if strings.HasPrefix(c, prefix) {
			filtered = append(filtered, c)
		}
	}

	return filtered
}

// DefaultHTTPClient provides a basic HTTP client for dynamic completion
type DefaultHTTPClient struct {
	client *http.Client
}

// NewDefaultHTTPClient creates a new default HTTP client
func NewDefaultHTTPClient() *DefaultHTTPClient {
	return &DefaultHTTPClient{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// ListResources implements APIClient
func (d *DefaultHTTPClient) ListResources(ctx context.Context, resourceType string) ([]map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

// ListNamespaces implements APIClient
func (d *DefaultHTTPClient) ListNamespaces(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

// ListProfiles implements APIClient
func (d *DefaultHTTPClient) ListProfiles(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}
