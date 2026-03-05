package cache

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound   = errors.New("cache: key not found")
	ErrExpired    = errors.New("cache: key expired")
	ErrInvalidKey = errors.New("cache: invalid key")
	ErrInvalidTTL = errors.New("cache: invalid TTL")
	ErrClosed     = errors.New("cache: cache closed")
	ErrFull       = errors.New("cache: cache full")
)

type Value struct {
	Data      []byte
	CreatedAt time.Time
	TTL       time.Duration
	Tags      []string
	Version   int64
}

func (v Value) IsExpired() bool {
	if v.TTL <= 0 {
		return false
	}
	return time.Since(v.CreatedAt) > v.TTL
}

type SetOption func(*SetOptions)

type SetOptions struct {
	TTL   time.Duration
	Tags  []string
	Async bool
}

func WithTTL(ttl time.Duration) SetOption {
	return func(o *SetOptions) {
		o.TTL = ttl
	}
}

func WithTags(tags ...string) SetOption {
	return func(o *SetOptions) {
		o.Tags = tags
	}
}

func WithAsync() SetOption {
	return func(o *SetOptions) {
		o.Async = true
	}
}

type Stats struct {
	Hits         uint64
	Misses       uint64
	Evictions    uint64
	Size         int64
	Entries      int
	HitRate      float64
	LastAccessed time.Time
}

type Cache interface {
	Get(ctx context.Context, key string) (Value, error)
	Set(ctx context.Context, key string, value Value, opts ...SetOption) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
	Has(ctx context.Context, key string) (bool, error)
	Keys(ctx context.Context, pattern string) ([]string, error)
	Stats(ctx context.Context) (Stats, error)
	Close() error
}
