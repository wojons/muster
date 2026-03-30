package cache

import (
	"context"
	"strings"
	"sync"
	"time"
)

type MemoryCache struct {
	mu          sync.RWMutex
	data        map[string]*CacheEntry
	maxSize     int64
	currentSize int64
	eviction    EvictionPolicy
	stats       Stats
	closed      bool
	statsMu     sync.Mutex
	stopCleanup chan struct{}
}

const (
	DefaultMemoryCacheSize = 100 * 1024 * 1024
	MaxMemoryCacheSize     = 512 * 1024 * 1024
)

func NewMemoryCache(maxSize int64, eviction EvictionPolicy) *MemoryCache {
	if maxSize <= 0 {
		maxSize = DefaultMemoryCacheSize
	}

	mc := &MemoryCache{
		data:        make(map[string]*CacheEntry),
		maxSize:     maxSize,
		eviction:    eviction,
		stopCleanup: make(chan struct{}),
	}

	go mc.cleanupLoop()

	return mc
}

func (mc *MemoryCache) Get(ctx context.Context, key string) (Value, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return Value{}, ErrClosed
	}

	e, exists := mc.data[key]
	if !exists {
		mc.recordMiss()
		return Value{}, ErrNotFound
	}

	if e.Value.IsExpired() {
		mc.recordMiss()
		return Value{}, ErrExpired
	}

	e.AccessedAt = time.Now()
	e.AccessCount++
	mc.recordHit()
	mc.statsMu.Lock()
	mc.stats.LastAccessed = time.Now()
	mc.statsMu.Unlock()

	return e.Value, nil
}

func (mc *MemoryCache) Set(ctx context.Context, key string, value Value, opts ...SetOption) error {
	if key == "" {
		return ErrInvalidKey
	}

	options := &SetOptions{TTL: value.TTL}
	for _, opt := range opts {
		opt(options)
	}

	value.TTL = options.TTL
	value.Tags = options.Tags
	value.CreatedAt = time.Now()
	value.Version = time.Now().UnixNano()

	size := int64(len(value.Data))

	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return ErrClosed
	}

	for mc.currentSize+size > mc.maxSize {
		if err := mc.evictOne(); err != nil {
			return ErrFull
		}
	}

	if old, exists := mc.data[key]; exists {
		mc.currentSize -= old.Size
	}

	mc.data[key] = &CacheEntry{
		Value:      value,
		Size:       size,
		AccessedAt: time.Now(),
	}
	mc.currentSize += size

	return nil
}

func (mc *MemoryCache) Delete(ctx context.Context, key string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return ErrClosed
	}

	if e, exists := mc.data[key]; exists {
		mc.currentSize -= e.Size
		delete(mc.data, key)
	}

	return nil
}

func (mc *MemoryCache) Clear(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return ErrClosed
	}

	mc.data = make(map[string]*CacheEntry)
	mc.currentSize = 0

	return nil
}

func (mc *MemoryCache) Has(ctx context.Context, key string) (bool, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if mc.closed {
		return false, ErrClosed
	}

	e, exists := mc.data[key]
	if !exists || e.Value.IsExpired() {
		return false, nil
	}

	return true, nil
}

func (mc *MemoryCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if mc.closed {
		return nil, ErrClosed
	}

	keys := make([]string, 0, len(mc.data))
	for k := range mc.data {
		if matchPattern(pattern, k) {
			keys = append(keys, k)
		}
	}

	return keys, nil
}

func (mc *MemoryCache) Stats(ctx context.Context) (Stats, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	if mc.closed {
		return Stats{}, ErrClosed
	}

	stats := mc.stats
	stats.Entries = len(mc.data)
	stats.Size = mc.currentSize

	total := stats.Hits + stats.Misses
	if total > 0 {
		stats.HitRate = float64(stats.Hits) / float64(total) * 100
	}

	return stats, nil
}

func (mc *MemoryCache) Close() error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return ErrClosed
	}

	mc.closed = true

	if mc.stopCleanup != nil {
		close(mc.stopCleanup)
	}

	mc.data = nil
	mc.currentSize = 0

	return nil
}

func (mc *MemoryCache) evictOne() error {
	key, err := mc.eviction.SelectForEviction(mc.data)
	if err != nil {
		return err
	}

	if e, exists := mc.data[key]; exists {
		mc.currentSize -= e.Size
		delete(mc.data, key)
		mc.recordEviction()
	}

	return nil
}

func (mc *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mc.cleanupExpired()
		case <-mc.stopCleanup:
			return
		}
	}
}

func (mc *MemoryCache) cleanupExpired() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if mc.closed {
		return
	}

	for key, e := range mc.data {
		if e.Value.IsExpired() {
			mc.currentSize -= e.Size
			delete(mc.data, key)
		}
	}
}

func matchPattern(pattern, key string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	if !strings.Contains(pattern, "*") {
		return pattern == key
	}

	prefix := strings.Split(pattern, "*")[0]
	return strings.HasPrefix(key, prefix)
}

func (mc *MemoryCache) recordHit() {
	mc.statsMu.Lock()
	mc.stats.Hits++
	mc.statsMu.Unlock()
}

func (mc *MemoryCache) recordMiss() {
	mc.statsMu.Lock()
	mc.stats.Misses++
	mc.statsMu.Unlock()
}

func (mc *MemoryCache) recordEviction() {
	mc.statsMu.Lock()
	mc.stats.Evictions++
	mc.statsMu.Unlock()
}
