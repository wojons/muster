package cache

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"
)

// defaultCleanupInterval is the default interval for the background
// cleanup goroutine that purges expired entries.
const defaultCleanupInterval = 1 * time.Minute

// entry represents a cached item in the MemoryCache with its key, value,
// and expiry metadata.
type entry struct {
	key       string
	value     []byte
	createdAt time.Time
	ttl       time.Duration
	expiresAt time.Time
}

// isExpired returns true if the entry has a non-zero TTL and the current
// time is past its expiration time.
func (e *entry) isExpired() bool {
	if e.ttl == 0 {
		return false
	}
	return time.Now().After(e.expiresAt)
}

// MemoryCache is a thread-safe, in-memory implementation of Cache.
// It uses a map for O(1) lookups and a linked list for LRU-based
// eviction when the cache exceeds its maximum size. Expired entries
// are removed lazily on access and periodically by a background goroutine.
type MemoryCache struct {
	mu      sync.RWMutex
	items   map[string]*list.Element
	lru     *list.List
	maxSize int
	stats   Stats
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewMemoryCache creates a new MemoryCache with the given maximum number
// of entries. Once the cache exceeds maxSize, the least-recently-used
// entries are evicted. A background goroutine is started to periodically
// purge expired entries.
//
// If maxSize <= 0, the cache has no size limit.
func NewMemoryCache(maxSize int) *MemoryCache {
	if maxSize < 0 {
		maxSize = 0
	}
	c := &MemoryCache{
		items:   make(map[string]*list.Element),
		lru:     list.New(),
		maxSize: maxSize,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
	go c.cleanupLoop(defaultCleanupInterval)
	return c
}

// Get retrieves a value by key. Returns the data, a boolean indicating
// whether the key was found and not expired, and any error. Expired
// entries are lazily evicted on access.
func (c *MemoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		c.stats.Misses++
		return nil, false, nil
	}

	ent, ok := elem.Value.(*entry)
	if !ok {
		return nil, false, fmt.Errorf("cache: invalid entry type in items map")
	}
	if ent.isExpired() {
		c.removeElement(elem)
		c.stats.Misses++
		return nil, false, nil
	}

	// Move to front (most recently used)
	c.lru.MoveToFront(elem)
	c.stats.Hits++
	return ent.value, true, nil
}

// Set stores a value with the given key and TTL. A zero TTL means the
// entry never expires. If the key already exists, it is overwritten.
// If the cache is at max capacity, the least-recently-used entry is evicted.
func (c *MemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	// If key exists, update in place
	if elem, ok := c.items[key]; ok {
		ent, ok := elem.Value.(*entry)
		if !ok {
			return fmt.Errorf("cache: invalid entry type in items map")
		}
		ent.value = value
		ent.createdAt = now
		ent.ttl = ttl
		ent.expiresAt = expiresAt
		c.lru.MoveToFront(elem)
		c.stats.Sets++
		return nil
	}

	// Evict if at capacity
	if c.maxSize > 0 && c.lru.Len() >= c.maxSize {
		c.evictOne()
	}

	ent := &entry{
		key:       key,
		value:     value,
		createdAt: now,
		ttl:       ttl,
		expiresAt: expiresAt,
	}
	elem := c.lru.PushFront(ent)
	c.items[key] = elem
	c.stats.Sets++
	return nil
}

// Delete removes a key from the cache. No error is returned if the key
// does not exist.
func (c *MemoryCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
		c.stats.Deletes++
	}
	return nil
}

// Clear removes all entries from the cache and resets statistics.
func (c *MemoryCache) Clear(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.lru.Init()
	c.stats = Stats{}
	return nil
}

// Close stops the background cleanup goroutine and releases resources.
// After Close, the cache must not be used.
func (c *MemoryCache) Close() error {
	select {
	case <-c.stopCh:
		// Already closed
		return nil
	default:
		close(c.stopCh)
		<-c.doneCh
	}
	return nil
}

// Stats returns a snapshot of cache usage statistics.
func (c *MemoryCache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := c.stats
	s.Size = int64(c.lru.Len())
	return s
}

// cleanupLoop runs periodically to purge expired entries. It exits when
// stopCh is closed.
func (c *MemoryCache) cleanupLoop(interval time.Duration) {
	defer close(c.doneCh)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.purgeExpired()
		}
	}
}

// purgeExpired iterates the cache and removes all expired entries.
// Must NOT be called while holding the lock.
func (c *MemoryCache) purgeExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, elem := range c.items {
		ent, ok := elem.Value.(*entry)
		if !ok {
			continue
		}
		if ent.isExpired() {
			c.removeElement(elem)
		}
	}
}

// evictOne removes the least-recently-used entry from the cache.
// Must be called while holding c.mu.
func (c *MemoryCache) evictOne() {
	elem := c.lru.Back()
	if elem == nil {
		return
	}
	c.removeElement(elem)
}

// removeElement removes a list element from both the linked list and the
// map. Must be called while holding c.mu.
func (c *MemoryCache) removeElement(elem *list.Element) {
	c.lru.Remove(elem)
	ent, ok := elem.Value.(*entry)
	if !ok {
		return
	}
	delete(c.items, ent.key)
}
