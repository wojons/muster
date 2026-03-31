package wasm

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ModuleSourceType represents the source of a WASM module.
type ModuleSourceType string

const (
	SourceLocalFile ModuleSourceType = "local_file"
	SourceRemoteURL ModuleSourceType = "remote_url"
	SourceEmbedded  ModuleSourceType = "embedded"
	SourceRegistry  ModuleSourceType = "registry"
	SourceInventory ModuleSourceType = "inventory"
)

// ModuleSource represents a source for loading a WASM module.
type ModuleSource struct {
	Type    ModuleSourceType `json:"type"`
	Path    string           `json:"path"`
	URL     string           `json:"url,omitempty"`
	Hash    string           `json:"hash,omitempty"`
	Version string           `json:"version,omitempty"`
}

// Module represents a loaded WASM module.
type Module struct {
	Source ModuleSource `json:"source"`
	Bytes  []byte       `json:"bytes"`
	Hash   string       `json:"hash"`
}

// RemoteLoadOptions configures remote module loading.
type RemoteLoadOptions struct {
	Timeout time.Duration
	MaxSize int64
}

// Loader handles loading and caching WASM modules.
type Loader struct {
	cache *ModuleCache
}

// NewLoader creates a new WASM module loader.
func NewLoader(cacheDir string) *Loader {
	return &Loader{
		cache: NewModuleCache(cacheDir, 24*time.Hour),
	}
}

// LoadFromFile loads a WASM module from a local file.
func (l *Loader) LoadFromFile(path string) (*Module, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file not accessible: %w", err)
	}

	if info.Size() > 50*1024*1024 {
		return nil, fmt.Errorf("module exceeds 50MB limit: %d bytes", info.Size())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if err := l.validateWASM(data); err != nil {
		return nil, fmt.Errorf("WASM validation failed: %w", err)
	}

	hash := generateHash(data)
	return &Module{
		Source: ModuleSource{
			Type: SourceLocalFile,
			Path: path,
			Hash: hash,
		},
		Bytes: data,
		Hash:  hash,
	}, nil
}

// LoadFromURL loads a WASM module from a remote URL.
func (l *Loader) LoadFromURL(ctx context.Context, url string, opts RemoteLoadOptions) (*Module, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/wasm")

	client := &http.Client{Timeout: opts.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if opts.MaxSize > 0 && int64(len(data)) > opts.MaxSize {
		return nil, fmt.Errorf("response exceeds max size: %d bytes", len(data))
	}

	if err := l.validateWASM(data); err != nil {
		return nil, fmt.Errorf("WASM validation failed: %w", err)
	}

	hash := generateHash(data)
	return &Module{
		Source: ModuleSource{
			Type: SourceRemoteURL,
			URL:  url,
			Hash: hash,
		},
		Bytes: data,
		Hash:  hash,
	}, nil
}

// LoadBytes loads a WASM module from bytes.
func (l *Loader) LoadBytes(data []byte) (*Module, error) {
	if err := l.validateWASM(data); err != nil {
		return nil, fmt.Errorf("WASM validation failed: %w", err)
	}

	hash := generateHash(data)
	return &Module{
		Source: ModuleSource{
			Type: SourceEmbedded,
			Hash: hash,
		},
		Bytes: data,
		Hash:  hash,
	}, nil
}

// LoadWithCache loads a WASM module with caching support.
func (l *Loader) LoadWithCache(source ModuleSource) (*Module, error) {
	key := sanitizeCacheKey(generateCacheKey(source))

	if cached, found := l.cache.Get(key); found {
		if err := l.validateWASM(cached); err != nil {
			_ = l.cache.Delete(key)
		} else {
			computedHash := generateHash(cached)

			if source.Hash != "" && computedHash != source.Hash {
				_ = l.cache.Delete(key)
				return nil, fmt.Errorf("cache integrity check failed: hash mismatch (expected %s, got %s)", source.Hash, computedHash)
			}

			return &Module{
				Source: source,
				Bytes:  cached,
				Hash:   computedHash,
			}, nil
		}
	}

	var data []byte
	var err error

	switch source.Type {
	case SourceLocalFile:
		if source.Path == "" {
			return nil, errors.New("path required for local file source")
		}
		data, err = os.ReadFile(source.Path)
	case SourceRemoteURL:
		if source.URL == "" {
			return nil, errors.New("url required for remote url source")
		}
		resp, err := http.Get(source.URL)
		if err != nil {
			return nil, fmt.Errorf("fetch wasm: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported source type: %s", source.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load module: %w", err)
	}

	if err := l.validateWASM(data); err != nil {
		return nil, fmt.Errorf("WASM validation failed: %w", err)
	}

	_ = l.cache.Set(key, data)

	return &Module{
		Source: source,
		Bytes:  data,
		Hash:   generateHash(data),
	}, nil
}

// validateWASM validates a WASM module's binary format.
func (l *Loader) validateWASM(data []byte) error {
	if len(data) < 8 {
		return errors.New("WASM module too small")
	}

	magic := binary.LittleEndian.Uint32(data[0:4])
	if magic != 0x6d736100 {
		return fmt.Errorf("invalid WASM magic: 0x%x", magic)
	}

	version := binary.LittleEndian.Uint32(data[4:8])
	if version != 1 {
		return fmt.Errorf("unsupported WASM version: %d", version)
	}

	return nil
}

// generateHash generates a SHA256 hash of data.
func generateHash(data []byte) string {
	h := sha256.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// generateCacheKey generates a cache key from a module source.
// Uses SHA256 to prevent path traversal attacks.
func generateCacheKey(source ModuleSource) string {
	h := sha256.New()
	h.Write([]byte(source.Type))
	h.Write([]byte(source.Path))
	h.Write([]byte(source.URL))
	h.Write([]byte(source.Version))
	return hex.EncodeToString(h.Sum(nil))
}

// sanitizeCacheKey ensures a cache key is safe by removing any path components.
// This prevents directory traversal attacks.
func sanitizeCacheKey(key string) string {
	h := sha256.New()
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil))
}

// ModuleCache handles caching of compiled WASM modules.
type ModuleCache struct {
	dir string
	ttl time.Duration
}

// NewModuleCache creates a new module cache.
func NewModuleCache(dir string, ttl time.Duration) *ModuleCache {
	return &ModuleCache{
		dir: dir,
		ttl: ttl,
	}
}

// Get retrieves a cached module.
func (c *ModuleCache) Get(key string) ([]byte, bool) {
	// Sanitize key to prevent directory traversal
	safeKey := filepath.Base(key)
	if safeKey != key || key == "" {
		return nil, false
	}

	path := filepath.Join(c.dir, safeKey+".wasm")

	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}

	if time.Since(info.ModTime()) > c.ttl {
		_ = os.Remove(path)
		return nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	return data, true
}

// Set stores a module in the cache.
func (c *ModuleCache) Set(key string, data []byte) error {
	if c.dir == "" {
		return nil
	}

	// Sanitize key to prevent directory traversal
	safeKey := filepath.Base(key)
	if safeKey != key || key == "" {
		return errors.New("invalid cache key: contains path separators or empty")
	}

	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(c.dir, safeKey+".wasm")
	// Use 0600 to ensure only owner can read (fix security vulnerability)
	return os.WriteFile(path, data, 0600)
}

// Delete removes a cached module.
func (c *ModuleCache) Delete(key string) error {
	if c.dir == "" {
		return nil
	}

	// Sanitize key to prevent directory traversal
	safeKey := filepath.Base(key)
	if safeKey != key || key == "" {
		return nil
	}

	path := filepath.Join(c.dir, safeKey+".wasm")
	return os.Remove(path)
}

// Clear removes all cached modules.
func (c *ModuleCache) Clear() error {
	if c.dir == "" {
		return nil
	}

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		path := filepath.Join(c.dir, entry.Name())
		_ = os.Remove(path)
	}

	return nil
}
