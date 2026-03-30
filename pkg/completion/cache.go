package completion

import (
	"errors"
	"sync"
	"time"
)

// ErrClosed is returned when operating on a closed cache
var ErrClosed = errors.New("completion: cache closed")

// CacheEntry represents a cached completion result
type CacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// Cache handles caching of completion results
type Cache struct {
	mu          sync.RWMutex
	data        map[string]*CacheEntry
	ttl         time.Duration
	stopCleanup chan struct{}
	closed      bool
}

// NewCache creates a new completion cache
func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		data:        make(map[string]*CacheEntry),
		ttl:         ttl,
		stopCleanup: make(chan struct{}),
	}

	go c.cleanupLoop()

	return c
}

// Get retrieves a value from cache
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.value, true
}

// Set stores a value in cache
func (c *Cache) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = &CacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Delete removes a value from cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
}

// Clear removes all entries from cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*CacheEntry)
}

// Close shuts down the cache and stops the cleanup goroutine
func (c *Cache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrClosed
	}

	c.closed = true
	close(c.stopCleanup)
	return nil
}

// cleanupLoop periodically removes expired entries
func (c *Cache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanup removes expired entries
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.data {
		if now.After(entry.expiresAt) {
			delete(c.data, key)
		}
	}
}
