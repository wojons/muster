// Package cache provides a multi-tier caching system with support for
// in-memory and Redis-backed implementations. The Cache interface defines
// a common contract for Get, Set, Delete, Clear, Close, and Stats operations
// with thread-safe guarantees.
package cache

import (
	"context"
	"time"
)

// Cache defines the interface for a multi-tier caching system.
// Implementations must be safe for concurrent use.
type Cache interface {
	// Get retrieves a value by key. Returns the value, a boolean indicating
	// whether the key was found and not expired, and any error encountered.
	Get(ctx context.Context, key string) ([]byte, bool, error)

	// Set stores a value with the given key and TTL. A zero TTL means no expiry.
	// If the key already exists, it is overwritten.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a key from the cache. No error is returned if the key
	// does not exist.
	Delete(ctx context.Context, key string) error

	// Clear removes all entries from the cache.
	Clear(ctx context.Context) error

	// Close releases any resources held by the cache (e.g., background
	// goroutines, network connections). After Close, the cache must not
	// be used.
	Close() error

	// Stats returns a snapshot of cache usage statistics.
	Stats() Stats
}

// Stats holds cache usage statistics. All counters may be read atomically
// or under the cache's internal lock.
type Stats struct {
	Hits    int64
	Misses  int64
	Sets    int64
	Deletes int64
	Size    int64
}

// Value represents a cached value with associated metadata.
// ExpiresAt is computed as CreatedAt + TTL on Set.
type Value struct {
	Data      []byte
	CreatedAt time.Time
	TTL       time.Duration
	ExpiresAt time.Time
}
