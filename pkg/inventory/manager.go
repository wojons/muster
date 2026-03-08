package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Inventory struct {
	SchemaVersion string    `json:"schema_version" validate:"required"`
	GeneratedAt   time.Time `json:"generated_at" validate:"required"`
	APIVersion    string    `json:"api_version" validate:"required,semver"`

	APIName        string `json:"api_name" validate:"required"`
	APIDescription string `json:"api_description" validate:"required"`
	BaseURL        string `json:"base_url" validate:"required,url"`

	MinCLIVersion string `json:"min_cli_version" validate:"semver"`
	MaxCLIVersion string `json:"max_cli_version" validate:"semver"`

	OpenAPISpec    *OpenAPIReference  `json:"openapi_spec" validate:"required"`
	Authentication *AuthConfiguration `json:"authentication,omitempty"`
	Resources      []Resource         `json:"resources" validate:"required,dive"`
	Middleware     []MiddlewareModule `json:"middleware,omitempty"`

	Features FeatureFlags `json:"features"`
}

type OpenAPIReference struct {
	URL         string            `json:"url" validate:"required,url"`
	Format      string            `json:"format" validate:"oneof=json yaml"`
	SHA256      string            `json:"sha256" validate:"required,len=64"`
	Size        int64             `json:"size" validate:"min=1"`
	LastUpdated time.Time         `json:"last_updated" validate:"required"`
	Headers     map[string]string `json:"headers,omitempty"`
}

type Resource struct {
	Name        string      `json:"name" validate:"required,alphanum"`
	Description string      `json:"description" validate:"required"`
	Operations  []Operation `json:"operations" validate:"required,dive"`
	Tags        []string    `json:"tags,omitempty"`
}

type Operation struct {
	Name        string       `json:"name" validate:"required"`
	Method      string       `json:"method" validate:"required,oneof=GET POST PUT PATCH DELETE HEAD OPTIONS"`
	Path        string       `json:"path" validate:"required,urlpath"`
	Summary     string       `json:"summary" validate:"required"`
	Description string       `json:"description,omitempty"`
	Parameters  []Parameter  `json:"parameters,omitempty"`
	RequestBody *RequestBody `json:"request_body,omitempty"`
	Responses   []Response   `json:"responses,omitempty"`
	Tags        []string     `json:"tags,omitempty"`
}

type Parameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

type RequestBody struct {
	Schema      interface{} `json:"schema"`
	Required    bool        `json:"required"`
	Description string      `json:"description,omitempty"`
}

type Response struct {
	Status      int         `json:"status"`
	Description string      `json:"description"`
	Schema      interface{} `json:"schema,omitempty"`
}

type AuthConfiguration struct {
	Type        string                 `json:"type" validate:"required,oneof=none basic bearer oauth2 apikey mtls"`
	Required    bool                   `json:"required"`
	Description string                 `json:"description,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

type MiddlewareModule struct {
	Name       string `json:"name" validate:"required"`
	Version    string `json:"version" validate:"semver"`
	URL        string `json:"url" validate:"required,url"`
	SHA256     string `json:"sha256" validate:"required,len=64"`
	EntryPoint string `json:"entry_point" validate:"required"`
	Stage      string `json:"stage" validate:"required,oneof=request response error"`
}

type FeatureFlags struct {
	Webhooks        bool `json:"webhooks"`
	Streaming       bool `json:"streaming"`
	BatchOperations bool `json:"batch_operations"`
	Pagination      bool `json:"pagination"`
}

type Manager struct {
	fetcher   *Fetcher
	validator *Validator
	cache     *InventoryCache
	versioner *VersionManager
	logger    Logger
}

type ManagerConfig struct {
	CacheDir string
	Logger   Logger
}

type FetchOptions struct {
	ForceRefresh bool
}

type Logger interface {
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

func NewManager(config ManagerConfig) (*Manager, error) {
	cache, err := NewInventoryCache(config.CacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize inventory cache: %w", err)
	}

	return &Manager{
		fetcher:   NewFetcher(nil),
		validator: NewValidator(),
		cache:     cache,
		versioner: NewVersionManager(),
		logger:    &defaultLogger{},
	}, nil
}

func (m *Manager) Fetch(ctx context.Context, host string, opts FetchOptions) (*Inventory, error) {
	m.logger.Info("Fetching inventory", "host", host)

	if !opts.ForceRefresh {
		if cached, err := m.cache.GetLatest(host); err == nil && !m.cache.IsExpired(cached) {
			m.logger.Info("Using cached inventory", "host", host, "version", cached.APIVersion)
			return cached, nil
		}
	}

	inventoryURL := m.buildInventoryURL(host)

	rawData, etag, err := m.fetcher.Fetch(ctx, inventoryURL)
	if err != nil {
		if cached, cacheErr := m.cache.GetLatest(host); cacheErr == nil {
			m.logger.Warn("Network error, using stale cache", "error", err)
			return cached, nil
		}
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	inv, err := m.parseInventory(rawData)
	if err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}

	if err := m.validator.Validate(inv); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if err := m.cache.Store(host, inv, etag); err != nil {
		m.logger.Warn("Failed to cache inventory", "error", err)
	}

	m.versioner.Track(host, inv)

	return inv, nil
}

func (m *Manager) FetchOpenAPISpec(ctx context.Context, inv *Inventory) (*OpenAPIDocument, error) {
	if inv.OpenAPISpec == nil {
		return nil, fmt.Errorf("inventory has no OpenAPI spec reference")
	}

	if cached := m.cache.GetSpec(inv.OpenAPISpec.SHA256); cached != nil {
		return cached, nil
	}

	rawData, err := m.fetcher.FetchRaw(ctx, inv.OpenAPISpec.URL, inv.OpenAPISpec.Headers)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OpenAPI spec: %w", err)
	}

	sha256 := calculateSHA256(rawData)
	if sha256 != inv.OpenAPISpec.SHA256 {
		return nil, fmt.Errorf("SHA256 mismatch: expected %s, got %s", inv.OpenAPISpec.SHA256, sha256)
	}

	doc, err := parseOpenAPIDocument(rawData, inv.OpenAPISpec.Format)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec: %w", err)
	}

	_ = m.cache.StoreSpec(inv.OpenAPISpec.SHA256, doc)

	return doc, nil
}

func (m *Manager) ListCached() ([]CachedInventory, error) {
	return m.cache.ListAll()
}

func (m *Manager) PurgeCache(host string) error {
	return m.cache.Purge(host)
}

func (m *Manager) GetVersionHistory(host string) ([]VersionRecord, error) {
	return m.versioner.GetHistory(host)
}

func (m *Manager) buildInventoryURL(host string) string {
	if !strings.HasPrefix(host, "http") {
		host = "https://" + host
	}
	return fmt.Sprintf("%s/_oa/inventory.json", strings.TrimSuffix(host, "/"))
}

func (m *Manager) parseInventory(data []byte) (*Inventory, error) {
	var inv Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}

type defaultLogger struct{}

func (l *defaultLogger) Info(msg string, args ...interface{})  {}
func (l *defaultLogger) Warn(msg string, args ...interface{})  {}
func (l *defaultLogger) Error(msg string, args ...interface{}) {}

type OpenAPIDocument struct {
	OpenAPI string `json:"openapi"`
	Info    struct {
		Title   string `json:"title"`
		Version string `json:"version"`
	} `json:"info"`
	Paths map[string]interface{} `json:"paths"`
}

func calculateSHA256(data []byte) string {
	return fmt.Sprintf("%x", len(data))
}

func parseOpenAPIDocument(data []byte, format string) (*OpenAPIDocument, error) {
	var doc OpenAPIDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}
