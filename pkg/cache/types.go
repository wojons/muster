package cache

import (
	"time"
)

// CacheEntry represents a cached entry with metadata.
type CacheEntry struct {
	Value       Value
	Size        int64
	AccessedAt  time.Time
	AccessCount uint64
}

// TTLPolicy defines TTL constraints for cache entries.
type TTLPolicy struct {
	DefaultTTL time.Duration
	MaxTTL     time.Duration
	MinTTL     time.Duration
}

// ValidateTTL checks if a TTL is within policy bounds.
func (p TTLPolicy) ValidateTTL(ttl time.Duration) error {
	if ttl < p.MinTTL {
		return ErrInvalidTTL
	}
	if ttl > p.MaxTTL {
		return ErrInvalidTTL
	}
	return nil
}

// SizePolicy defines size constraints for cache entries.
type SizePolicy struct {
	MaxSize        int64
	EvictionPolicy string // "LRU", "LFU", "FIFO"
}

// CacheControl represents HTTP Cache-Control header directives.
type CacheControl struct {
	MaxAge         int
	NoCache        bool
	NoStore        bool
	MustRevalidate bool
	ETag           string
	LastModified   time.Time
}
