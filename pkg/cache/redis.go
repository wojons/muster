package cache

import (
	"context"
	"fmt"
	"time"
)

// ErrRedisNotImplemented is returned by RedisCache methods to indicate
// that the Redis-backed cache implementation is not yet available.
var ErrRedisNotImplemented = fmt.Errorf("redis cache: not implemented — " +
	"compile with the 'redis' build tag and ensure go-redis is available")

// RedisCache is a placeholder for a Redis-backed cache implementation.
// It implements the Cache interface by returning ErrRedisNotImplemented
// for all operations. A real implementation will be provided when the
// Redis driver dependency is added to the project.
type RedisCache struct {
	addr    string
	db      int
	timeout time.Duration
	closed  bool
}

// NewRedisCache creates a new RedisCache stub. It returns
// ErrRedisNotImplemented to indicate that Redis caching is not yet
// available. This ensures callers fail closed rather than silently
// relying on a no-op cache.
func NewRedisCache(addr string, db int, timeout time.Duration) (*RedisCache, error) {
	return nil, fmt.Errorf("%w: addr=%s db=%d timeout=%s",
		ErrRedisNotImplemented, addr, db, timeout)
}

// Get is not implemented.
func (r *RedisCache) Get(_ context.Context, _ string) ([]byte, bool, error) {
	return nil, false, ErrRedisNotImplemented
}

// Set is not implemented.
func (r *RedisCache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return ErrRedisNotImplemented
}

// Delete is not implemented.
func (r *RedisCache) Delete(_ context.Context, _ string) error {
	return ErrRedisNotImplemented
}

// Clear is not implemented.
func (r *RedisCache) Clear(_ context.Context) error {
	return ErrRedisNotImplemented
}

// Close is a no-op for the stub.
func (r *RedisCache) Close() error {
	r.closed = true
	return nil
}

// Stats returns zeroed stats for the stub.
func (r *RedisCache) Stats() Stats {
	return Stats{}
}
