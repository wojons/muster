// Package benchmarks provides cache benchmarks
//
// Benchmarks:
//
//   - BenchmarkCache_Set: Cache write operations
//   - BenchmarkCache_Get: Cache read operations
//   - BenchmarkCache_Delete: Cache delete operations
//   - BenchmarkCache_Parallel: Concurrent cache access
//
// Performance Budgets:
//
// All benchmarks must meet their defined performance budgets (see budgets.go).
package benchmarks

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// SimpleCache is a minimal cache for benchmarking
type SimpleCache struct {
	mu     sync.RWMutex
	data   map[string][]byte
	ttl    time.Duration
	expiry map[string]time.Time
}

// NewSimpleCache creates a new simple cache
func NewSimpleCache(ttl time.Duration) *SimpleCache {
	return &SimpleCache{
		data:   make(map[string][]byte),
		ttl:    ttl,
		expiry: make(map[string]time.Time),
	}
}

// Set stores a value in the cache
func (c *SimpleCache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
	c.expiry[key] = time.Now().Add(c.ttl)
}

// Get retrieves a value from the cache
func (c *SimpleCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	expiry, ok := c.expiry[key]
	if !ok || time.Now().After(expiry) {
		return nil, false
	}

	return c.data[key], true
}

// Delete removes a value from the cache
func (c *SimpleCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	delete(c.expiry, key)
}

// BenchmarkCache_Set measures cache write performance
func BenchmarkCache_Set(b *testing.B) {
	_ = GetBudgetOrDefault("Cache_Set")

	cache := NewSimpleCache(time.Hour)
	value := make([]byte, 100) // 100 byte value

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, value)
	}
}

// BenchmarkCache_Get measures cache read performance
func BenchmarkCache_Get(b *testing.B) {
	_ = GetBudgetOrDefault("Cache_Get")

	cache := NewSimpleCache(time.Hour)

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, []byte("value"))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)
		cache.Get(key)
	}
}

// BenchmarkCache_Delete measures cache delete performance
func BenchmarkCache_Delete(b *testing.B) {
	_ = GetBudgetOrDefault("Cache_Delete")

	cache := NewSimpleCache(time.Hour)

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, []byte("value"))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)
		cache.Delete(key)
	}
}

// BenchmarkCache_SetParallel measures concurrent cache write performance
func BenchmarkCache_SetParallel(b *testing.B) {
	cache := NewSimpleCache(time.Hour)
	value := make([]byte, 100)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i)
			cache.Set(key, value)
			i++
		}
	})
}

// BenchmarkCache_GetParallel measures concurrent cache read performance
func BenchmarkCache_GetParallel(b *testing.B) {
	cache := NewSimpleCache(time.Hour)

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, []byte("value"))
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			cache.Get(key)
			i++
		}
	})
}

// BenchmarkCache_MixedLoad measures cache performance under mixed read/write load
func BenchmarkCache_MixedLoad(b *testing.B) {
	cache := NewSimpleCache(time.Hour)

	// Pre-populate some data
	for i := 0; i < 500; i++ {
		cache.Set(fmt.Sprintf("key-%d", i), []byte("value"))
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)

		switch i % 10 {
		case 0: // 10% writes
			cache.Set(key, []byte("new-value"))
		case 1: // 10% deletes
			cache.Delete(key)
		default: // 80% reads
			cache.Get(key)
		}
	}
}

// BenchmarkCache_Expiration measures cache expiration overhead
func BenchmarkCache_Expiration(b *testing.B) {
	cache := NewSimpleCache(time.Nanosecond) // Immediate expiration

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		cache.Set(key, []byte("value"))
	}

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)
		cache.Get(key) // Will check expiration
	}
}

// BenchmarkCache_LargeValues measures performance with large values
func BenchmarkCache_LargeValues(b *testing.B) {
	cache := NewSimpleCache(time.Hour)
	largeValue := make([]byte, 1024*1024) // 1MB value

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%100)
		cache.Set(key, largeValue)
	}
}
