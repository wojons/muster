package inventory

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized          = errors.New("authentication required")
	ErrForbidden             = errors.New("access denied")
	ErrNotFound              = errors.New("inventory endpoint not found")
	ErrValidation            = errors.New("validation error")
	ErrVersionMismatch       = errors.New("client version incompatible")
	ErrSignatureVerification = errors.New("signature verification failed")
	ErrTooManyRedirects      = errors.New("too many redirects")
	ErrNotModified           = errors.New("resource not modified")
)

type InventoryProtocolResponse struct {
	Version        string            `json:"version"`
	MinCLIVersion  string            `json:"min_cli_version"`
	GeneratedAt    time.Time         `json:"generated_at"`
	OpenAPI        *OpenAPIReference `json:"openapi"`
	WASMModules    []WASMModule      `json:"wasm_modules"`
	ConfigDefaults ConfigDefaults    `json:"config_defaults"`
	Auth           *AuthConfig       `json:"auth"`
	Meta           *ServerMeta       `json:"meta"`
}

type WASMModule struct {
	Name      string        `json:"name"`
	URL       string        `json:"url"`
	SHA256    string        `json:"sha256"`
	Signature *WMASignature `json:"signature"`
	Size      int64         `json:"size"`
	Version   string        `json:"version"`
}

type WMASignature struct {
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type ConfigDefaults struct {
	TimeoutMS  int             `json:"timeout_ms"`
	MaxRetries int             `json:"max_retries"`
	Features   map[string]bool `json:"features"`
}

type AuthConfig struct {
	Required      bool     `json:"required"`
	Type          string   `json:"type"`
	Scopes        []string `json:"scopes"`
	TokenEndpoint string   `json:"token_endpoint,omitempty"`
}

type ServerMeta struct {
	ServerTime  time.Time `json:"server_time"`
	Environment string    `json:"environment"`
}

type InventoryErrorResponse struct {
	Error           string     `json:"error"`
	Message         string     `json:"message"`
	Code            string     `json:"code,omitempty"`
	UpgradeRequired bool       `json:"upgrade_required,omitempty"`
	MinVersion      string     `json:"min_version,omitempty"`
	CurrentVersion  string     `json:"current_version,omitempty"`
	UpgradeURL      string     `json:"upgrade_url,omitempty"`
	AuthURL         string     `json:"auth_url,omitempty"`
	RetryAfter      *string    `json:"retry_after,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
}

type ProtocolClient struct {
	baseURL       string
	clientVersion string
	fingerprint   string
	token         string
	httpClient    *http.Client
	timeout       time.Duration
	maxRetries    int
	maxRedirects  int
	cache         *ProtocolCache
	mu            sync.RWMutex
}

type ProtocolClientOptions struct {
	BaseURL       string
	ClientVersion string
	Fingerprint   string
	Token         string
	HTTPClient    *http.Client
	Timeout       time.Duration
	MaxRetries    int
	MaxRedirects  int
	Cache         *ProtocolCache
}

type ProtocolCache struct {
	mu      sync.RWMutex
	entries map[string]*ProtocolCacheEntry
	ttl     time.Duration
}

type ProtocolCacheEntry struct {
	ETag      string
	Data      *InventoryProtocolResponse
	Timestamp time.Time
}

func NewProtocolCache(ttl time.Duration) *ProtocolCache {
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}
	return &ProtocolCache{
		entries: make(map[string]*ProtocolCacheEntry),
		ttl:     ttl,
	}
}

func (c *ProtocolCache) Get(url string, etag string) *ProtocolCacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[url]
	if !exists {
		return nil
	}

	if time.Since(entry.Timestamp) > c.ttl {
		return nil
	}

	if etag != "" && entry.ETag != etag {
		return nil
	}

	return entry
}

func (c *ProtocolCache) Set(url string, etag string, data *InventoryProtocolResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[url] = &ProtocolCacheEntry{
		ETag:      etag,
		Data:      data,
		Timestamp: time.Now(),
	}
}

func (c *ProtocolCache) Delete(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, url)
}

func NewProtocolClient(opts ProtocolClientOptions) *ProtocolClient {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	maxRedirects := opts.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = 5
	}

	cache := opts.Cache
	if cache == nil {
		cache = NewProtocolCache(1 * time.Hour)
	}

	baseURL := strings.TrimSuffix(opts.BaseURL, "/")

	return &ProtocolClient{
		baseURL:       baseURL,
		clientVersion: opts.ClientVersion,
		fingerprint:   opts.Fingerprint,
		token:         opts.Token,
		httpClient:    httpClient,
		timeout:       timeout,
		maxRetries:    maxRetries,
		maxRedirects:  maxRedirects,
		cache:         cache,
	}
}

type ProtocolFetchOptions struct {
	ETag           string
	UseCompression bool
	redirectCount  int
}

func (c *ProtocolClient) FetchInventory(ctx context.Context, opts *ProtocolFetchOptions) (*InventoryProtocolResponse, error) {
	if opts == nil {
		opts = &ProtocolFetchOptions{}
	}

	url := c.baseURL + "/_inventory.json"
	cached := c.cache.Get(url, opts.ETag)

	var lastError error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		resp, err := c.executeRequest(ctx, url, cached, opts)
		if err == nil {
			return resp, nil
		}

		lastError = err

		if !c.shouldRetry(err) {
			return nil, err
		}

		if attempt < c.maxRetries-1 {
			delay := c.calculateBackoff(attempt, err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", c.maxRetries, lastError)
}

func (c *ProtocolClient) executeRequest(ctx context.Context, url string, cached *ProtocolCacheEntry, opts *ProtocolFetchOptions) (*InventoryProtocolResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Client-Version", c.clientVersion)
	req.Header.Set("X-Fingerprint", c.fingerprint)

	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	if opts.ETag != "" {
		req.Header.Set("If-None-Match", opts.ETag)
	}

	if opts.UseCompression {
		req.Header.Set("Accept-Encoding", "gzip, br")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified && cached != nil {
		return cached.Data, nil
	}

	if c.isRedirect(resp.StatusCode) {
		return c.handleRedirect(ctx, resp, opts)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleError(resp)
	}

	body := resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, err := gzip.NewReader(body)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress response: %w", err)
		}
		defer func() { _ = gzReader.Close() }()
		body = gzReader
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var inventory InventoryProtocolResponse
	if err := json.Unmarshal(data, &inventory); err != nil {
		return nil, fmt.Errorf("failed to parse inventory response: %w", err)
	}

	etag := resp.Header.Get("ETag")
	if etag != "" {
		c.cache.Set(url, etag, &inventory)
	}

	if err := c.verifyWASMSignatures(inventory.WASMModules); err != nil {
		return nil, err
	}

	return &inventory, nil
}

func (c *ProtocolClient) isRedirect(status int) bool {
	return status == http.StatusMovedPermanently ||
		status == http.StatusFound ||
		status == http.StatusTemporaryRedirect ||
		status == http.StatusPermanentRedirect
}

func (c *ProtocolClient) handleRedirect(ctx context.Context, resp *http.Response, opts *ProtocolFetchOptions) (*InventoryProtocolResponse, error) {
	location := resp.Header.Get("Location")
	if location == "" {
		return nil, errors.New("redirect without Location header")
	}

	opts.redirectCount++
	if opts.redirectCount > c.maxRedirects {
		return nil, ErrTooManyRedirects
	}

	isPermanent := resp.StatusCode == http.StatusMovedPermanently ||
		resp.StatusCode == http.StatusPermanentRedirect

	newURL := c.resolveURL(location)

	if isPermanent {
		c.mu.Lock()
		c.baseURL = c.extractBaseURL(newURL)
		c.mu.Unlock()
	}

	isCrossOrigin := !strings.HasPrefix(newURL, c.baseURL)
	if isCrossOrigin {
		c.mu.Lock()
		c.token = ""
		c.mu.Unlock()
	}

	return c.executeRequest(ctx, newURL, nil, opts)
}

func (c *ProtocolClient) handleError(resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var errResp InventoryErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return &UnauthorizedError{Message: errResp.Message, URL: errResp.AuthURL}
	case http.StatusForbidden:
		return &ForbiddenError{
			Message:         errResp.Message,
			Code:            errResp.Code,
			RevokedAt:       errResp.RevokedAt,
			UpgradeRequired: errResp.UpgradeRequired,
		}
	case http.StatusNotFound:
		return &NotFoundError{Message: errResp.Message}
	case http.StatusUnprocessableEntity:
		return &ValidationError{
			Message:        errResp.Message,
			MinVersion:     errResp.MinVersion,
			CurrentVersion: errResp.CurrentVersion,
			UpgradeURL:     errResp.UpgradeURL,
		}
	default:
		return &InventoryFetchError{
			Message: errResp.Message,
			Code:    errResp.Code,
		}
	}
}

func (c *ProtocolClient) shouldRetry(err error) bool {
	var httpErr *InventoryFetchError
	if errors.As(err, &httpErr) {
		return false
	}
	var unauthorized *UnauthorizedError
	if errors.As(err, &unauthorized) {
		return false
	}
	var forbidden *ForbiddenError
	if errors.As(err, &forbidden) {
		return false
	}
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		return false
	}
	var validation *ValidationError
	return !errors.As(err, &validation)
}

func (c *ProtocolClient) calculateBackoff(attempt int, err error) time.Duration {
	var httpErr *InventoryFetchError
	if errors.As(err, &httpErr) && httpErr.Code == "TOO_MANY_REQUESTS" {
		return time.Second
	}

	baseDelay := time.Duration(1<<uint(attempt)) * time.Second
	jitter := time.Duration(rand.Float64() * float64(baseDelay) * 0.1)
	return baseDelay + jitter
}

func (c *ProtocolClient) verifyWASMSignatures(modules []WASMModule) error {
	for _, module := range modules {
		if module.Signature == nil {
			continue
		}

		if module.Signature.Algorithm != "Ed25519" {
			return fmt.Errorf("%w: unsupported algorithm %s", ErrSignatureVerification, module.Signature.Algorithm)
		}

		publicKeyBytes, err := base64.StdEncoding.DecodeString(module.Signature.PublicKey)
		if err != nil {
			return fmt.Errorf("%w: invalid public key: %w", ErrSignatureVerification, err)
		}

		signatureBytes, err := base64.StdEncoding.DecodeString(module.Signature.Signature)
		if err != nil {
			return fmt.Errorf("%w: invalid signature: %w", ErrSignatureVerification, err)
		}

		message := fmt.Sprintf("%s:%s:%s:%s", module.Name, module.SHA256, module.URL, module.Version)

		if !ed25519.Verify(publicKeyBytes, []byte(message), signatureBytes) {
			return fmt.Errorf("%w: module %s", ErrSignatureVerification, module.Name)
		}
	}
	return nil
}

func (c *ProtocolClient) resolveURL(location string) string {
	if strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://") {
		return location
	}

	c.mu.RLock()
	baseURL := c.baseURL
	c.mu.RUnlock()

	if strings.HasPrefix(location, "/") {
		return baseURL + location
	}

	return baseURL + "/" + location
}

func (c *ProtocolClient) extractBaseURL(fullURL string) string {
	parts := strings.SplitN(fullURL, "/", 4)
	if len(parts) >= 3 {
		return parts[0] + "//" + parts[2]
	}
	return fullURL
}

func (c *ProtocolClient) UpdateToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

func (c *ProtocolClient) GetBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

func (c *ProtocolClient) ClearCache() {
	c.cache = NewProtocolCache(1 * time.Hour)
}

type UnauthorizedError struct {
	Message string
	URL     string
}

func (e *UnauthorizedError) Error() string {
	return fmt.Sprintf("unauthorized: %s", e.Message)
}

type ForbiddenError struct {
	Message         string
	Code            string
	RevokedAt       *time.Time
	UpgradeRequired bool
}

func (e *ForbiddenError) Error() string {
	return fmt.Sprintf("forbidden: %s", e.Message)
}

type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("not found: %s", e.Message)
}

type ValidationError struct {
	Message        string
	MinVersion     string
	CurrentVersion string
	UpgradeURL     string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error: %s", e.Message)
}

type InventoryFetchError struct {
	Message string
	Code    string
}

func (e *InventoryFetchError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Message
}

func ParseRetryAfter(header string) (time.Duration, error) {
	if header == "" {
		return 0, errors.New("empty Retry-After header")
	}

	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(seconds) * time.Second, nil
	}

	t, err := time.Parse(time.RFC1123, header)
	if err != nil {
		return 0, err
	}

	duration := time.Until(t)
	if duration < 0 {
		return 0, nil
	}

	return duration, nil
}

func VerifySignature(message, publicKeyB64, signatureB64 string) (bool, error) {
	publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return false, fmt.Errorf("invalid public key: %w", err)
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return false, fmt.Errorf("invalid signature: %w", err)
	}

	return ed25519.Verify(publicKeyBytes, []byte(message), signatureBytes), nil
}

func CalculateSHA256(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}

func CompressGZIP(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)

	if _, err := writer.Write(data); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func DecompressGZIP(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()

	return io.ReadAll(reader)
}
