package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// TieredCache is a two-level cache: a fast in-memory L1 fronted by an
// optional slower L2 backend. Reads check L1 first, then backfill from L2.
// Writes go to both levels so L2 is always authoritative for cold starts.
type TieredCache struct {
	l1    *MemoryCache
	l2    Cache
	stats tieredStats
}

type tieredStats struct {
	l1Hits   atomic.Uint64
	l2Hits   atomic.Uint64
	misses   atomic.Uint64
	backfill atomic.Uint64
}

// NewTieredCache creates a TieredCache with the given L1 capacity and
// optional L2 backend (nil disables the second level).
func NewTieredCache(l1Size int, l2 Cache) *TieredCache {
	return &TieredCache{
		l1: NewMemoryCache(l1Size),
		l2: l2,
	}
}

// Get returns the value for key, checking L1 then backfilling from L2.
func (c *TieredCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if v, ok, err := c.l1.Get(ctx, key); err != nil {
		return nil, false, err
	} else if ok {
		c.stats.l1Hits.Add(1)
		return v, true, nil
	}

	if c.l2 == nil {
		c.stats.misses.Add(1)
		return nil, false, nil
	}

	v, ok, err := c.l2.Get(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if ok {
		c.stats.l2Hits.Add(1)
		c.stats.backfill.Add(1)
		// Backfill L1 with the same TTL the L2 entry carried (default 60s).
		_ = c.l1.Set(ctx, key, v, 60*time.Second)
		return v, true, nil
	}
	c.stats.misses.Add(1)
	return nil, false, nil
}

// Set writes the value to both levels.
func (c *TieredCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.l1.Set(ctx, key, value, ttl); err != nil {
		return err
	}
	if c.l2 != nil {
		return c.l2.Set(ctx, key, value, ttl)
	}
	return nil
}

// Delete removes the key from both levels.
func (c *TieredCache) Delete(ctx context.Context, key string) error {
	if err := c.l1.Delete(ctx, key); err != nil {
		return err
	}
	if c.l2 != nil {
		return c.l2.Delete(ctx, key)
	}
	return nil
}

// Clear empties both levels.
func (c *TieredCache) Clear(ctx context.Context) error {
	if err := c.l1.Clear(ctx); err != nil {
		return err
	}
	if c.l2 != nil {
		return c.l2.Clear(ctx)
	}
	return nil
}

// Close releases both levels.
func (c *TieredCache) Close() error {
	if err := c.l1.Close(); err != nil {
		return err
	}
	if c.l2 != nil {
		return c.l2.Close()
	}
	return nil
}

// Stats returns aggregate hit/miss counters.
func (c *TieredCache) Stats() Stats {
	l1 := c.l1.Stats()
	return Stats{
		Hits:   l1.Hits + int64(c.stats.l2Hits.Load()),
		Misses: l1.Misses + int64(c.stats.misses.Load()),
	}
}

// CacheManager tracks hit rates and exposes typed cache access.
type CacheManager struct {
	mu      sync.RWMutex
	caches  map[string]Cache
	names   []string
	hitRate *hitRateTracker
}

type hitRateTracker struct {
	hits  atomic.Uint64
	total atomic.Uint64
}

// NewCacheManager creates a CacheManager with an empty cache registry.
func NewCacheManager() *CacheManager {
	return &CacheManager{
		caches:  make(map[string]Cache),
		hitRate: &hitRateTracker{},
	}
}

// Register adds a named cache to the manager.
func (m *CacheManager) Register(name string, c Cache) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.caches[name]; !exists {
		m.names = append(m.names, name)
	}
	m.caches[name] = c
}

// Get returns a named cache, or nil if not registered.
func (m *CacheManager) Get(name string) Cache {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.caches[name]
}

// Names returns the registered cache names in insertion order.
func (m *CacheManager) Names() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.names))
	copy(out, m.names)
	return out
}

// TypedCache wraps a Cache with a concrete value type, handling
// serialization so callers work with T instead of []byte.
type TypedCache[T any] struct {
	inner  Cache
	encode func(T) ([]byte, error)
	decode func([]byte) (T, error)
}

// NewTypedCache creates a typed view over an underlying Cache.
func NewTypedCache[T any](inner Cache, encode func(T) ([]byte, error), decode func([]byte) (T, error)) *TypedCache[T] {
	return &TypedCache[T]{inner: inner, encode: encode, decode: decode}
}

// Get fetches and decodes a typed value.
func (t *TypedCache[T]) Get(ctx context.Context, key string) (T, bool, error) {
	var zero T
	raw, ok, err := t.inner.Get(ctx, key)
	if err != nil || !ok {
		return zero, ok, err
	}
	v, err := t.decode(raw)
	if err != nil {
		return zero, false, err
	}
	return v, true, nil
}

// Set encodes and stores a typed value.
func (t *TypedCache[T]) Set(ctx context.Context, key string, value T, ttl time.Duration) error {
	raw, err := t.encode(value)
	if err != nil {
		return err
	}
	return t.inner.Set(ctx, key, raw, ttl)
}
