package cache

import (
	"context"
	"fmt"
	"time"
)

type TieredCache struct {
	l1        Cache
	l2        Cache
	l3        Cache
	warmAsync bool
}

type TieredCacheConfig struct {
	L1        Cache
	L2        Cache
	L3        Cache
	WarmAsync bool
}

func NewTieredCache(cfg TieredCacheConfig) *TieredCache {
	return &TieredCache{
		l1:        cfg.L1,
		l2:        cfg.L2,
		l3:        cfg.L3,
		warmAsync: cfg.WarmAsync,
	}
}

func (tc *TieredCache) Get(ctx context.Context, key string) (Value, error) {
	val, err := tc.l1.Get(ctx, key)
	if err == nil {
		return val, nil
	}

	if tc.l2 != nil {
		val, err = tc.l2.Get(ctx, key)
		if err == nil {
			_ = tc.l1.Set(ctx, key, val, WithTTL(val.TTL/2))
			return val, nil
		}
	}

	if tc.l3 != nil {
		val, err = tc.l3.Get(ctx, key)
		if err == nil {
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
	if err := tc.l1.Set(ctx, key, value, opts...); err != nil {
		return fmt.Errorf("tiered: L1 set failed: %w", err)
	}

	if tc.l2 != nil {
		//nolint:staticcheck // L2 cache failures are non-critical
		if err := tc.l2.Set(ctx, key, value, opts...); err != nil {
			// L2 cache set failed - non-critical, continue
		}
	}

	if tc.l3 != nil {
		if tc.warmAsync {
			go func() {
				_ = tc.l3.Set(context.Background(), key, value, opts...)
			}()
		} else {
			//nolint:staticcheck // L3 cache failures are non-critical
			if err := tc.l3.Set(ctx, key, value, opts...); err != nil {
				// L3 cache set failed - non-critical, continue
			}
		}
	}

	return nil
}

func (tc *TieredCache) Delete(ctx context.Context, key string) error {
	var errs []error

	if err := tc.l1.Delete(ctx, key); err != nil {
		errs = append(errs, fmt.Errorf("L1: %w", err))
	}

	if tc.l2 != nil {
		if err := tc.l2.Delete(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("L2: %w", err))
		}
	}

	if tc.l3 != nil {
		if err := tc.l3.Delete(ctx, key); err != nil {
			errs = append(errs, fmt.Errorf("L3: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("tiered delete failed: %v", errs)
	}

	return nil
}

func (tc *TieredCache) Clear(ctx context.Context) error {
	var errs []error

	if err := tc.l1.Clear(ctx); err != nil {
		errs = append(errs, fmt.Errorf("L1: %w", err))
	}

	if tc.l2 != nil {
		if err := tc.l2.Clear(ctx); err != nil {
			errs = append(errs, fmt.Errorf("L2: %w", err))
		}
	}

	if tc.l3 != nil {
		if err := tc.l3.Clear(ctx); err != nil {
			errs = append(errs, fmt.Errorf("L3: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("tiered clear failed: %v", errs)
	}

	return nil
}

func (tc *TieredCache) Has(ctx context.Context, key string) (bool, error) {
	has, err := tc.l1.Has(ctx, key)
	if err == nil && has {
		return true, nil
	}

	if tc.l2 != nil {
		has, err := tc.l2.Has(ctx, key)
		if err == nil && has {
			return true, nil
		}
	}

	if tc.l3 != nil {
		has, err := tc.l3.Has(ctx, key)
		if err == nil && has {
			return true, nil
		}
	}

	return false, nil
}

func (tc *TieredCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	keys, err := tc.l1.Keys(ctx, pattern)
	if err == nil && len(keys) > 0 {
		return keys, nil
	}

	if tc.l2 != nil {
		keys, err = tc.l2.Keys(ctx, pattern)
		if err == nil && len(keys) > 0 {
			return keys, nil
		}
	}

	if tc.l3 != nil {
		keys, err = tc.l3.Keys(ctx, pattern)
		if err == nil && len(keys) > 0 {
			return keys, nil
		}
	}

	return []string{}, nil
}

func (tc *TieredCache) Stats(ctx context.Context) (Stats, error) {
	var combined Stats

	if stats, err := tc.l1.Stats(ctx); err == nil {
		combined.Hits += stats.Hits
		combined.Misses += stats.Misses
		combined.Evictions += stats.Evictions
		combined.Size += stats.Size
		combined.Entries += stats.Entries
	}

	if tc.l2 != nil {
		if stats, err := tc.l2.Stats(ctx); err == nil {
			combined.Hits += stats.Hits
			combined.Misses += stats.Misses
			combined.Evictions += stats.Evictions
		}
	}

	if tc.l3 != nil {
		if stats, err := tc.l3.Stats(ctx); err == nil {
			combined.Hits += stats.Hits
			combined.Misses += stats.Misses
			combined.Evictions += stats.Evictions
		}
	}

	total := combined.Hits + combined.Misses
	if total > 0 {
		combined.HitRate = float64(combined.Hits) / float64(total) * 100
	}

	if combined.Entries > 0 {
		combined.LastAccessed = time.Now()
	}

	return combined, nil
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
		return fmt.Errorf("tiered close failed: %v", errs)
	}

	return nil
}
