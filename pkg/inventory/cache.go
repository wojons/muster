package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

type InventoryCache struct {
	baseDir string
	ttl     time.Duration
}

var (
	// ErrInvalidSHA256 is returned when an invalid SHA256 hash is provided
	ErrInvalidSHA256 = errors.New("invalid SHA256 hash")
)

// isValidSHA256 validates that a string is a valid SHA256 hash (64 hexadecimal characters)
func isValidSHA256(sha256Str string) bool {
	// SHA256 hashes are exactly 64 hexadecimal characters
	if len(sha256Str) != 64 {
		return false
	}

	// Check that all characters are valid hexadecimal (0-9, a-f, A-F)
	match, _ := regexp.MatchString("^[a-fA-F0-9]{64}$", sha256Str)
	return match
}

type CacheEntry struct {
	Inventory  *Inventory `json:"inventory"`
	ETag       string     `json:"etag"`
	CachedAt   time.Time  `json:"cached_at"`
	AccessedAt time.Time  `json:"accessed_at"`
}

func NewInventoryCache(baseDir string) (*InventoryCache, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		baseDir = filepath.Join(home, ".cache", "oa")
	}

	dirs := []string{
		baseDir,
		filepath.Join(baseDir, "inventory"),
		filepath.Join(baseDir, "specs"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	return &InventoryCache{
		baseDir: baseDir,
		ttl:     1 * time.Hour,
	}, nil
}

func (c *InventoryCache) GetLatest(host string) (*Inventory, error) {
	hostDir := c.hostDir(host)
	currentPath := filepath.Join(hostDir, "current.json")

	data, err := os.ReadFile(currentPath)
	if err != nil {
		return nil, err
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}

	entry.AccessedAt = time.Now()
	_ = c.saveEntry(host, "current", &entry)

	return entry.Inventory, nil
}

func (c *InventoryCache) Store(host string, inv *Inventory, etag string) error {
	hostDir := c.hostDir(host)

	entry := &CacheEntry{
		Inventory:  inv,
		ETag:       etag,
		CachedAt:   time.Now(),
		AccessedAt: time.Now(),
	}

	versionPath := filepath.Join(hostDir, inv.APIVersion+".json")
	if err := c.saveEntry(host, inv.APIVersion, entry); err != nil {
		return err
	}

	currentPath := filepath.Join(hostDir, "current.json")
	_ = os.Remove(currentPath)

	return os.Symlink(filepath.Base(versionPath), currentPath)
}

func (c *InventoryCache) StoreSpec(sha256Str string, doc *OpenAPIDocument) error {
	// Validate SHA256 format to prevent path traversal attacks
	if !isValidSHA256(sha256Str) {
		return ErrInvalidSHA256
	}

	specDir := filepath.Join(c.baseDir, "specs")
	specPath := filepath.Join(specDir, sha256Str+".json")

	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	return os.WriteFile(specPath, data, 0644)
}

func (c *InventoryCache) GetSpec(sha256Str string) *OpenAPIDocument {
	// Validate SHA256 format to prevent path traversal attacks
	if !isValidSHA256(sha256Str) {
		return nil
	}

	specPath := filepath.Join(c.baseDir, "specs", sha256Str+".json")

	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil
	}

	var doc OpenAPIDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}

	return &doc
}

func (c *InventoryCache) IsExpired(inv *Inventory) bool {
	return time.Since(inv.GeneratedAt) > c.ttl
}

func (c *InventoryCache) ListAll() ([]CachedInventory, error) {
	inventoryDir := filepath.Join(c.baseDir, "inventory")

	entries, err := os.ReadDir(inventoryDir)
	if err != nil {
		return nil, err
	}

	var cached []CachedInventory
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		currentPath := filepath.Join(inventoryDir, entry.Name(), "current.json")
		data, err := os.ReadFile(currentPath)
		if err != nil {
			continue
		}

		var cacheEntry CacheEntry
		if err := json.Unmarshal(data, &cacheEntry); err != nil {
			continue
		}

		cached = append(cached, CachedInventory{
			Host:       entry.Name(),
			Version:    cacheEntry.Inventory.APIVersion,
			CachedAt:   cacheEntry.CachedAt,
			AccessedAt: cacheEntry.AccessedAt,
		})
	}

	return cached, nil
}

func (c *InventoryCache) Purge(host string) error {
	hostDir := c.hostDir(host)
	return os.RemoveAll(hostDir)
}

func (c *InventoryCache) hostDir(host string) string {
	hash := sha256.Sum256([]byte(host))
	hostHash := hex.EncodeToString(hash[:8])
	return filepath.Join(c.baseDir, "inventory", hostHash)
}

func (c *InventoryCache) saveEntry(host, name string, entry *CacheEntry) error {
	hostDir := c.hostDir(host)
	if err := os.MkdirAll(hostDir, 0755); err != nil {
		return err
	}

	path := filepath.Join(hostDir, name+".json")
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
