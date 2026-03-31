package cache

import (
	"context"
	"sync"
	"time"
)

type CacheManager struct {
	tiers    []CacheTier
	policies map[string]Policy
	metrics  *CacheMetrics
	mu       sync.RWMutex
}

type CacheTier struct {
	Name     string
	Cache    Cache
	Priority int
	MaxSize  int64
}

type Policy struct {
	TTL      time.Duration
	Tiers    []string
	Priority int
}

const (
	maxMetricsEntries    = 1000
	metricsPruneInterval = 10 * time.Minute
)

type CacheMetrics struct {
	hits      map[string]int64
	misses    int64
	evictions map[string]int64
	lastPrune time.Time
	mu        sync.RWMutex
	stopPrune chan struct{}
}

func NewCacheManager() *CacheManager {
	cm := &CacheManager{
		tiers:    make([]CacheTier, 0),
		policies: make(map[string]Policy),
		metrics: &CacheMetrics{
			hits:      make(map[string]int64),
			evictions: make(map[string]int64),
			lastPrune: time.Now(),
			stopPrune: make(chan struct{}),
		},
	}
	cm.startMetricsPruner()
	return cm
}

func (cm *CacheManager) AddTier(name string, cache Cache, priority int, maxSize int64) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.tiers = append(cm.tiers, CacheTier{
		Name:     name,
		Cache:    cache,
		Priority: priority,
		MaxSize:  maxSize,
	})
}

func (cm *CacheManager) SetPolicy(name string, ttl time.Duration, tiers []string, priority int) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.policies[name] = Policy{
		TTL:      ttl,
		Tiers:    tiers,
		Priority: priority,
	}
}

func (cm *CacheManager) Get(ctx context.Context, key string) (Value, error) {
	for _, tier := range cm.tiers {
		value, err := tier.Cache.Get(ctx, key)
		if err == nil {
			cm.promote(ctx, key, value, tier.Priority)
			cm.recordHit(tier.Name)
			return value, nil
		}
	}

	cm.recordMiss()
	return Value{}, ErrNotFound
}

func (cm *CacheManager) Set(ctx context.Context, key string, value Value, policyName string) error {
	cm.mu.RLock()
	policy, ok := cm.policies[policyName]
	cm.mu.RUnlock()

	if !ok {
		policy = Policy{
			TTL:      30 * time.Minute,
			Tiers:    []string{},
			Priority: 1,
		}
	}

	for _, tierName := range policy.Tiers {
		for _, tier := range cm.tiers {
			if tier.Name == tierName {
				_ = tier.Cache.Set(ctx, key, value, WithTTL(policy.TTL))
			}
		}
	}

	return nil
}

func (cm *CacheManager) Delete(ctx context.Context, key string) error {
	for _, tier := range cm.tiers {
		_ = tier.Cache.Delete(ctx, key)
	}
	return nil
}

func (cm *CacheManager) Clear(ctx context.Context) error {
	for _, tier := range cm.tiers {
		_ = tier.Cache.Clear(ctx)
	}
	return nil
}

func (cm *CacheManager) Has(ctx context.Context, key string) (bool, error) {
	for _, tier := range cm.tiers {
		has, err := tier.Cache.Has(ctx, key)
		if err == nil && has {
			return true, nil
		}
	}
	return false, nil
}

func (cm *CacheManager) Keys(ctx context.Context, pattern string) ([]string, error) {
	var allKeys []string
	seen := make(map[string]bool)

	for _, tier := range cm.tiers {
		keys, err := tier.Cache.Keys(ctx, pattern)
		if err == nil {
			for _, key := range keys {
				if !seen[key] {
					seen[key] = true
					allKeys = append(allKeys, key)
				}
			}
		}
	}

	return allKeys, nil
}

func (cm *CacheManager) Stats(ctx context.Context) (Stats, error) {
	var combined Stats

	for _, tier := range cm.tiers {
		stats, err := tier.Cache.Stats(ctx)
		if err == nil {
			combined.Hits += stats.Hits
			combined.Misses += stats.Misses
			combined.Evictions += stats.Evictions
			combined.Size += stats.Size
			combined.Entries += stats.Entries
		}
	}

	total := combined.Hits + combined.Misses
	if total > 0 {
		combined.HitRate = float64(combined.Hits) / float64(total) * 100
	}

	return combined, nil
}

func (cm *CacheManager) Close() error {
	var errs []error

	for _, tier := range cm.tiers {
		if err := tier.Cache.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}

	return nil
}

func (cm *CacheManager) promote(ctx context.Context, key string, value Value, fromTier int) {
	for _, tier := range cm.tiers {
		if tier.Priority < fromTier {
			_ = tier.Cache.Set(ctx, key, value, WithTTL(5*time.Minute))
		}
	}
}

func (cm *CacheManager) recordHit(tier string) {
	cm.metrics.mu.Lock()
	defer cm.metrics.mu.Unlock()
	cm.metrics.hits[tier]++
}

func (cm *CacheManager) recordMiss() {
	cm.metrics.mu.Lock()
	defer cm.metrics.mu.Unlock()
	cm.metrics.misses++
}

func (cm *CacheManager) startMetricsPruner() {
	go func() {
		ticker := time.NewTicker(metricsPruneInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cm.pruneMetrics()
			case <-cm.metrics.stopPrune:
				return
			}
		}
	}()
}

func (cm *CacheManager) pruneMetrics() {
	cm.metrics.mu.Lock()
	defer cm.metrics.mu.Unlock()

	if time.Since(cm.metrics.lastPrune) < metricsPruneInterval {
		return
	}

	if len(cm.metrics.hits) > maxMetricsEntries {
		targetSize := maxMetricsEntries / 2
		for key := range cm.metrics.hits {
			delete(cm.metrics.hits, key)
			if len(cm.metrics.hits) <= targetSize {
				break
			}
		}
	}

	if len(cm.metrics.evictions) > maxMetricsEntries {
		targetSize := maxMetricsEntries / 2
		for key := range cm.metrics.evictions {
			delete(cm.metrics.evictions, key)
			if len(cm.metrics.evictions) <= targetSize {
				break
			}
		}
	}

	cm.metrics.lastPrune = time.Now()
}

func (cm *CacheManager) Stop() {
	close(cm.metrics.stopPrune)
}

func (cm *CacheManager) GetHitRate() float64 {
	cm.metrics.mu.RLock()
	defer cm.metrics.mu.RUnlock()

	totalHits := int64(0)
	for _, hits := range cm.metrics.hits {
		totalHits += hits
	}

	total := totalHits + cm.metrics.misses
	if total == 0 {
		return 0
	}

	return float64(totalHits) / float64(total) * 100
}
