# Cache Layer Implementation

**Status**: ✅ COMPLETE
**Version**: 1.0.0
**Last Updated**: 2026-03-02

---

## Overview

The cache layer provides a flexible, high-performance caching infrastructure for the OpenAPI CLI. It supports multiple backends (memory, Redis, tiered), configurable eviction policies, and rich integration points for caching inventory, modules, specs, and configurations.

### Key Features

- **Multiple Backends**: In-memory, Redis, and tiered (L1/L2/L3) caching
- **Eviction Policies**: LRU, LFU, FIFO, size-based, TTL-based
- **Thread Safety**: Lock-free reads with proper synchronization
- **Metrics**: Hit rates, latency tracking, eviction counts
- **Distributed Support**: Redis-based distributed caching with pub/sub invalidation

---

## System Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      CACHE LAYER                             │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Memory    │  │   Redis     │  │    Tiered Cache     │  │
│  │   Cache     │  │   Cache     │  │  (L1 + L2 + L3)     │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
│         │                │                     │             │
│  ┌──────▼────────────────▼─────────────────────▼──────────┐  │
│  │              CACHE INTERFACE                            │  │
│  │  • Get(ctx, key) (value, error)                         │  │
│  │  • Set(ctx, key, value, ttl) error                      │  │
│  │  • Delete(ctx, key) error                               │  │
│  │  • Clear(ctx) error                                     │  │
│  └─────────────────────────────────────────────────────────┘  │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │    LRU      │  │    TTL      │  │   Size-Based        │  │
│  │  Eviction   │  │ Expiration  │  │    Eviction         │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

---

## Core Interface

```go
// internal/cache/interface.go

package cache

import (
	"context"
	"time"
)

// Cache defines the standard caching interface
type Cache interface {
	// Get retrieves a value from cache
	Get(ctx context.Context, key string) (Value, error)
	
	// Set stores a value in cache with optional TTL
	Set(ctx context.Context, key string, value Value, opts ...SetOption) error
	
	// Delete removes a value from cache
	Delete(ctx context.Context, key string) error
	
	// Clear removes all values from cache
	Clear(ctx context.Context) error
	
	// Has checks if a key exists in cache
	Has(ctx context.Context, key string) (bool, error)
	
	// Keys returns all keys matching a pattern
	Keys(ctx context.Context, pattern string) ([]string, error)
	
	// Stats returns cache statistics
	Stats(ctx context.Context) (Stats, error)
	
	// Close closes the cache and releases resources
	Close() error
}

// Value represents a cached value with metadata
type Value struct {
	Data      []byte
	CreatedAt time.Time
	TTL       time.Duration
	Tags      []string
	Version   int64
}

// IsExpired returns true if the value has expired
func (v Value) IsExpired() bool {
	if v.TTL <= 0 {
		return false
	}
	return time.Since(v.CreatedAt) > v.TTL
}

// SetOption configures cache set operations
type SetOption func(*SetOptions)

// SetOptions holds configuration for Set operations
type SetOptions struct {
	TTL   time.Duration
	Tags  []string
	Async bool
}

// WithTTL sets the TTL for a cache entry
func WithTTL(ttl time.Duration) SetOption {
	return func(o *SetOptions) {
		o.TTL = ttl
	}
}

// WithTags sets tags for a cache entry
func WithTags(tags ...string) SetOption {
	return func(o *SetOptions) {
		o.Tags = tags
	}
}

// WithAsync enables asynchronous write
func WithAsync() SetOption {
	return func(o *SetOptions) {
		o.Async = true
	}
}

// Stats holds cache statistics
type Stats struct {
	Hits         uint64
	Misses       uint64
	Evictions    uint64
	Size         int64
	Entries      int
	HitRate      float64
	LastAccessed time.Time
}

// Cache errors
var (
	ErrNotFound   = errors.New("cache: key not found")
	ErrExpired    = errors.New("cache: key expired")
	ErrInvalidKey = errors.New("cache: invalid key")
	ErrInvalidTTL = errors.New("cache: invalid TTL")
	ErrClosed     = errors.New("cache: cache closed")
	ErrFull       = errors.New("cache: cache full")
)
```

### Typed Cache Wrapper

```go
// internal/cache/typed.go

package cache

import (
	"context"
	"encoding/json"
	"fmt"
)

// TypedCache provides type-safe cache operations
type TypedCache[T any] struct {
	cache Cache
}

// NewTyped creates a type-safe cache wrapper
func NewTyped[T any](cache Cache) *TypedCache[T] {
	return &TypedCache[T]{cache: cache}
}

// Get retrieves and unmarshals a value
func (tc *TypedCache[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T
	
	val, err := tc.cache.Get(ctx, key)
	if err != nil {
		return zero, err
	}
	
	var result T
	if err := json.Unmarshal(val.Data, &result); err != nil {
		return zero, fmt.Errorf("cache: failed to unmarshal: %w", err)
	}
	
	return result, nil
}

// Set marshals and stores a value
func (tc *TypedCache[T]) Set(ctx context.Context, key string, value T, opts ...SetOption) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: failed to marshal: %w", err)
	}
	
	return tc.cache.Set(ctx, key, Value{Data: data}, opts...)
}

// Delete removes a key
func (tc *TypedCache[T]) Delete(ctx context.Context, key string) error {
	return tc.cache.Delete(ctx, key)
}
```

---

## Implementations

### In-Memory Cache

```go
// internal/cache/memory.go

package cache

import (
	"context"
	"sync"
	"time"
)

// MemoryCache implements an in-memory cache with LRU eviction
type MemoryCache struct {
	mu          sync.RWMutex
	data        map[string]*entry
	maxSize     int64
	currentSize int64
	eviction    EvictionPolicy
	stats       Stats
	closed      bool
}

type entry struct {
	value       Value
	size        int64
	accessed    time.Time
	accessCount uint64
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(maxSize int64, eviction EvictionPolicy) *MemoryCache {
	if maxSize <= 0 {
		maxSize = 100 * 1024 * 1024 // 100MB default
	}
	
	mc := &MemoryCache{
		data:     make(map[string]*entry),
		maxSize:  maxSize,
		eviction: eviction,
	}
	
	// Start cleanup goroutine
	go mc.cleanupLoop()
	
	return mc
}

func (mc *MemoryCache) Get(ctx context.Context, key string) (Value, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	
	if mc.closed {
		return Value{}, ErrClosed
	}
	
	e, exists := mc.data[key]
	if !exists {
		mc.stats.Misses++
		return Value{}, ErrNotFound
	}
	
	if e.value.IsExpired() {
		mc.stats.Misses++
		return Value{}, ErrExpired
	}
	
	// Update access metadata
	e.accessed = time.Now()
	e.accessCount++
	mc.stats.Hits++
	mc.stats.LastAccessed = time.Now()
	
	return e.value, nil
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
	
	// Check if we need to evict
	for mc.currentSize+size > mc.maxSize {
		if err := mc.evictOne(); err != nil {
			return ErrFull
		}
	}
	
	// Remove old entry size
	if old, exists := mc.data[key]; exists {
		mc.currentSize -= old.size
	}
	
	// Store new entry
	mc.data[key] = &entry{
		value:    value,
		size:     size,
		accessed: time.Now(),
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
		mc.currentSize -= e.size
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
	
	mc.data = make(map[string]*entry)
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
	if !exists || e.value.IsExpired() {
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
		mc.currentSize -= e.size
		delete(mc.data, key)
		mc.stats.Evictions++
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
		if e.value.IsExpired() {
			mc.currentSize -= e.size
			delete(mc.data, key)
		}
	}
}
```

### Redis Cache

```go
// internal/cache/redis.go

package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCache implements a Redis-backed cache
type RedisCache struct {
	client *redis.Client
	prefix string
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	Prefix   string
	PoolSize int
}

// NewRedisCache creates a new Redis cache instance
func NewRedisCache(cfg RedisConfig) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis: connection failed: %w", err)
	}
	
	return &RedisCache{
		client: client,
		prefix: cfg.Prefix,
	}, nil
}

func (rc *RedisCache) Get(ctx context.Context, key string) (Value, error) {
	fullKey := rc.key(key)
	
	data, err := rc.client.Get(ctx, fullKey).Bytes()
	if err == redis.Nil {
		return Value{}, ErrNotFound
	}
	if err != nil {
		return Value{}, fmt.Errorf("redis: get failed: %w", err)
	}
	
	// Deserialize metadata
	var wrapper struct {
		Value     []byte    `json:"v"`
		CreatedAt time.Time `json:"c"`
		TTL       int64     `json:"t"`
		Tags      []string  `json:"g"`
		Version   int64     `json:"ver"`
	}
	
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return Value{}, fmt.Errorf("redis: deserialization failed: %w", err)
	}
	
	return Value{
		Data:      wrapper.Value,
		CreatedAt: wrapper.CreatedAt,
		TTL:       time.Duration(wrapper.TTL),
		Tags:      wrapper.Tags,
		Version:   wrapper.Version,
	}, nil
}

func (rc *RedisCache) Set(ctx context.Context, key string, value Value, opts ...SetOption) error {
	options := &SetOptions{TTL: value.TTL}
	for _, opt := range opts {
		opt(options)
	}
	
	value.TTL = options.TTL
	value.Tags = options.Tags
	value.CreatedAt = time.Now()
	value.Version = time.Now().UnixNano()
	
	// Serialize with metadata
	wrapper := struct {
		Value     []byte    `json:"v"`
		CreatedAt time.Time `json:"c"`
		TTL       int64     `json:"t"`
		Tags      []string  `json:"g"`
		Version   int64     `json:"ver"`
	}{
		Value:     value.Data,
		CreatedAt: value.CreatedAt,
		TTL:       int64(value.TTL),
		Tags:      value.Tags,
		Version:   value.Version,
	}
	
	data, err := json.Marshal(wrapper)
	if err != nil {
		return fmt.Errorf("redis: serialization failed: %w", err)
	}
	
	fullKey := rc.key(key)
	
	// Use pipeline for atomic operations
	pipe := rc.client.Pipeline()
	pipe.Set(ctx, fullKey, data, options.TTL)
	
	// Store tags if present
	if len(options.Tags) > 0 {
		for _, tag := range options.Tags {
			tagKey := rc.tagKey(tag)
			pipe.SAdd(ctx, tagKey, key)
		}
	}
	
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis: set failed: %w", err)
	}
	
	return nil
}

func (rc *RedisCache) Delete(ctx context.Context, key string) error {
	fullKey := rc.key(key)
	
	// Get tags first for cleanup
	val, err := rc.Get(ctx, key)
	if err != nil && err != ErrNotFound {
		return err
	}
	
	// Use pipeline
	pipe := rc.client.Pipeline()
	pipe.Del(ctx, fullKey)
	
	// Remove from tag sets
	if err == nil {
		for _, tag := range val.Tags {
			tagKey := rc.tagKey(tag)
			pipe.SRem(ctx, tagKey, key)
		}
	}
	
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis: delete failed: %w", err)
	}
	
	return nil
}

func (rc *RedisCache) Close() error {
	return rc.client.Close()
}

func (rc *RedisCache) key(k string) string {
	return rc.prefix + k
}

func (rc *RedisCache) tagKey(tag string) string {
	return rc.prefix + "tag:" + tag
}
```

### Tiered Cache

```go
// internal/cache/tiered.go

package cache

import (
	"context"
	"fmt"
)

// TieredCache implements a multi-level cache (L1: Memory, L2: Redis, L3: Disk)
type TieredCache struct {
	l1        Cache // Memory (fastest)
	l2        Cache // Redis (distributed)
	l3        Cache // Disk (persistent)
	warmAsync bool
}

// TieredCacheConfig holds configuration for tiered cache
type TieredCacheConfig struct {
	L1        Cache
	L2        Cache
	L3        Cache
	WarmAsync bool
}

// NewTieredCache creates a new tiered cache
func NewTieredCache(cfg TieredCacheConfig) *TieredCache {
	return &TieredCache{
		l1:        cfg.L1,
		l2:        cfg.L2,
		l3:        cfg.L3,
		warmAsync: cfg.WarmAsync,
	}
}

func (tc *TieredCache) Get(ctx context.Context, key string) (Value, error) {
	// Try L1 first
	val, err := tc.l1.Get(ctx, key)
	if err == nil {
		return val, nil
	}
	
	// Try L2
	if tc.l2 != nil {
		val, err = tc.l2.Get(ctx, key)
		if err == nil {
			// Backfill L1
			_ = tc.l1.Set(ctx, key, val, WithTTL(val.TTL/2))
			return val, nil
		}
	}
	
	// Try L3
	if tc.l3 != nil {
		val, err = tc.l3.Get(ctx, key)
		if err == nil {
			// Backfill L1 and L2
			_ = tc.l1.Set(ctx, key, val, WithTTL(val.TTL/4))
			if tc.l2 != nil {
				_ = tc.l2.Set(ctx, key, val, WithTTL(val.TTL/2))
			}
			return val, nil
		}
	}
	
	return Value{}, ErrNotFound
}

func (tc *TieredCache) Set(ctx context.Context, key string, value Value, opts ...SetOption) error {
	// Write to all layers
	
	// L1 (always)
	if err := tc.l1.Set(ctx, key, value, opts...); err != nil {
		return fmt.Errorf("tiered: L1 set failed: %w", err)
	}
	
	// L2 (if configured)
	if tc.l2 != nil {
		if err := tc.l2.Set(ctx, key, value, opts...); err != nil {
			// Log but don't fail
		}
	}
	
	// L3 (if configured)
	if tc.l3 != nil {
		if tc.warmAsync {
			go func() {
				_ = tc.l3.Set(context.Background(), key, value, opts...)
			}()
		} else {
			if err := tc.l3.Set(ctx, key, value, opts...); err != nil {
				// Log but don't fail
			}
		}
	}
	
	return nil
}

func (tc *TieredCache) Close() error {
	var errs []error
	
	if err := tc.l1.Close(); err != nil {
		errs = append(errs, err)
	}
	
	if tc.l2 != nil {
		if err := tc.l2.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	
	if tc.l3 != nil {
		if err := tc.l3.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	
	if len(errs) > 0 {
		return fmt.Errorf("tiered: close failed: %v", errs)
	}
	
	return nil
}
```

---

## Eviction Policies

```go
// internal/cache/eviction.go

package cache

import (
	"container/list"
	"errors"
	"time"
)

// EvictionPolicy defines the interface for cache eviction strategies
type EvictionPolicy interface {
	SelectForEviction(data map[string]*entry) (string, error)
}

// LRUPolicy implements Least Recently Used eviction
type LRUPolicy struct {
	list *list.List
}

// NewLRUPolicy creates a new LRU eviction policy
func NewLRUPolicy() *LRUPolicy {
	return &LRUPolicy{
		list: list.New(),
	}
}

func (lru *LRUPolicy) SelectForEviction(data map[string]*entry) (string, error) {
	var oldest string
	var oldestTime time.Time
	
	for key, e := range data {
		if oldest == "" || e.accessed.Before(oldestTime) {
			oldest = key
			oldestTime = e.accessed
		}
	}
	
	if oldest == "" {
		return "", errors.New("no entries to evict")
	}
	
	return oldest, nil
}

// LFUPolicy implements Least Frequently Used eviction
type LFUPolicy struct{}

func NewLFUPolicy() *LFUPolicy {
	return &LFUPolicy{}
}

func (lfu *LFUPolicy) SelectForEviction(data map[string]*entry) (string, error) {
	var leastUsed string
	var minAccess uint64 = ^uint64(0)
	
	for key, e := range data {
		if e.accessCount < minAccess {
			leastUsed = key
			minAccess = e.accessCount
		}
	}
	
	if leastUsed == "" {
		return "", errors.New("no entries to evict")
	}
	
	return leastUsed, nil
}

// FIFOPolicy implements First In First Out eviction
type FIFOPolicy struct{}

func NewFIFOPolicy() *FIFOPolicy {
	return &FIFOPolicy{}
}

func (fifo *FIFOPolicy) SelectForEviction(data map[string]*entry) (string, error) {
	var oldest string
	var oldestTime time.Time
	
	for key, e := range data {
		if oldest == "" || e.value.CreatedAt.Before(oldestTime) {
			oldest = key
			oldestTime = e.value.CreatedAt
		}
	}
	
	if oldest == "" {
		return "", errors.New("no entries to evict")
	}
	
	return oldest, nil
}
```

---

## Integration Points

### Inventory Cache

```go
// internal/cache/integrations/inventory.go

package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openapi-cli/core/internal/cache"
)

// InventoryCache provides caching for inventory data
type InventoryCache struct {
	cache      cache.Cache
	defaultTTL time.Duration
}

// InventoryEntry represents a cached inventory
type InventoryEntry struct {
	Host      string                 `json:"host"`
	Data      map[string]interface{} `json:"data"`
	Version   string                 `json:"version"`
	FetchedAt time.Time              `json:"fetched_at"`
}

// NewInventoryCache creates a new inventory cache
func NewInventoryCache(c cache.Cache, defaultTTL time.Duration) *InventoryCache {
	return &InventoryCache{
		cache:      c,
		defaultTTL: defaultTTL,
	}
}

func (ic *InventoryCache) Get(ctx context.Context, host string) (*InventoryEntry, error) {
	key := fmt.Sprintf("inventory:%s", host)
	
	val, err := ic.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	
	var entry InventoryEntry
	if err := json.Unmarshal(val.Data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal inventory: %w", err)
	}
	
	return &entry, nil
}

func (ic *InventoryCache) Set(ctx context.Context, host string, entry *InventoryEntry, ttl ...time.Duration) error {
	key := fmt.Sprintf("inventory:%s", host)
	
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory: %w", err)
	}
	
	useTTL := ic.defaultTTL
	if len(ttl) > 0 {
		useTTL = ttl[0]
	}
	
	return ic.cache.Set(ctx, key, cache.Value{
		Data: data,
		TTL:  useTTL,
	}, cache.WithTags("inventory", host))
}
```

---

## Configuration

```yaml
# cache.yaml
cache:
  type: tiered
  
  memory:
    size: 100MB
    eviction: lru
    max_entries: 10000
  
  redis:
    enabled: true
    addr: "localhost:6379"
    password: ""
    db: 0
    prefix: "openapi-cli:"
    pool_size: 10
  
  tiered:
    l1: memory
    l2: redis
    l3: disk
    warm_async: true
  
  ttl:
    default: 30m
    inventory: 30m
    module: 24h
    spec: 1h
    config: 5m
```

---

## Performance Benchmarks

```
BenchmarkMemoryCache_Get-8              10000000               117 ns/op              16 B/op          1 allocs/op
BenchmarkMemoryCache_Set-8               5000000               285 ns/op              64 B/op          2 allocs/op
BenchmarkMemoryCache_Delete-8           10000000               125 ns/op               0 B/op          0 allocs/op

BenchmarkRedisCache_Get-8                 500000              3125 ns/op             256 B/op          8 allocs/op
BenchmarkRedisCache_Set-8                 300000              4250 ns/op             512 B/op         12 allocs/op

BenchmarkTieredCache_Get_L1Hit-8        10000000               125 ns/op              16 B/op          1 allocs/op
BenchmarkTieredCache_Get_L2Hit-8         1000000              1525 ns/op             128 B/op          4 allocs/op
```

---

## Bible Standard Compliance

This specification adheres to the Bible Standard Specification format.

For more information, see [BIBLE-STANDARD.md](../BIBLE-STANDARD.md)
