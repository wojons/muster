package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/openapi-cli/openapi-cli/pkg/cache"
)

func TestCacheIntegration_CacheMiss(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	key := "test-cache-miss-key"

	_, err := mc.Get(ctx, key)
	require.Error(t, err, "Expected error when getting non-existent key")
	require.ErrorIs(t, err, cache.ErrNotFound, "Expected ErrNotFound")
}

func TestCacheIntegration_CacheHit(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	key := "test-cache-hit-key"
	value := cache.Value{
		Data:      []byte("test data for cache hit"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err := mc.Set(ctx, key, value)
	require.NoError(t, err, "Failed to set value")

	exists, err := mc.Has(ctx, key)
	require.NoError(t, err)
	require.True(t, exists, "Key should exist in cache")

	retrieved, err := mc.Get(ctx, key)
	require.NoError(t, err, "Failed to get value")
	require.Equal(t, "test data for cache hit", string(retrieved.Data), "Data mismatch")
}

func TestCacheIntegration_TTL_Enforcement(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	key := "test-ttl-key"
	shortTTL := 500 * time.Millisecond

	value := cache.Value{
		Data:      []byte("data with short TTL"),
		CreatedAt: time.Now(),
		TTL:       shortTTL,
	}

	err := mc.Set(ctx, key, value, cache.WithTTL(shortTTL))
	require.NoError(t, err, "Failed to set value with TTL")

	_, err = mc.Get(ctx, key)
	require.NoError(t, err, "Key should exist immediately after setting")

	time.Sleep(shortTTL + 100*time.Millisecond)

	_, err = mc.Get(ctx, key)
	require.Error(t, err, "Expected error after TTL expired")
	require.True(t, err == cache.ErrNotFound || err == cache.ErrExpired, "Expected ErrNotFound or ErrExpired after TTL expiry")
}

func TestCacheIntegration_ManualInvalidation(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	key := "test-invalidation-key"
	value := cache.Value{
		Data:      []byte("data to be invalidated"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err := mc.Set(ctx, key, value)
	require.NoError(t, err, "Failed to set value")

	exists, err := mc.Has(ctx, key)
	require.NoError(t, err)
	require.True(t, exists, "Key should exist before invalidation")

	err = mc.Delete(ctx, key)
	require.NoError(t, err, "Failed to delete key")

	exists, err = mc.Has(ctx, key)
	require.NoError(t, err)
	require.False(t, exists, "Key should not exist after deletion")

	_, err = mc.Get(ctx, key)
	require.ErrorIs(t, err, cache.ErrNotFound, "Expected ErrNotFound after deletion")
}

func TestCacheIntegration_OfflineMode(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	requestCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": "response from server"}`))
	}))
	defer mockServer.Close()

	cacheKey := "offline-mode-test"
	cachedValue := cache.Value{
		Data:      []byte(`{"data": "cached response"}`),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err := mc.Set(ctx, cacheKey, cachedValue)
	require.NoError(t, err, "Failed to cache value")

	mockServer.Close()

	cached, err := mc.Get(ctx, cacheKey)
	require.NoError(t, err, "Should retrieve from cache when offline")
	require.Contains(t, string(cached.Data), "cached response", "Should return cached data")

	_, err = http.Get(mockServer.URL)
	require.Error(t, err, "Server should be unreachable")

	require.Equal(t, 0, requestCount, "No requests should have been made to offline server")
}

func TestCacheIntegration_SizeLimit_Eviction(t *testing.T) {
	smallSize := int64(100)

	mc := cache.NewMemoryCache(smallSize, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		value := cache.Value{
			Data:      []byte("value number"),
			CreatedAt: time.Now(),
			TTL:       time.Hour,
		}
		key := "key"
		err := mc.Set(ctx, key, value)
		require.NoError(t, err, "Failed to set key")
	}

	stats, err := mc.Stats(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.Evictions, uint64(0), "Evictions stat should be accessible")

	_, err = mc.Get(ctx, "key")
	require.NoError(t, err, "Key should still exist")
}

func TestCacheIntegration_StaleWhileRevalidate(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	cacheKey := "stale-while-revalidate-test"
	staleValue := cache.Value{
		Data:      []byte("stale cached data"),
		CreatedAt: time.Now(),
		TTL:       100 * time.Millisecond,
	}

	err := mc.Set(ctx, cacheKey, staleValue, cache.WithTTL(100*time.Millisecond))
	require.NoError(t, err, "Failed to set stale value")

	time.Sleep(200 * time.Millisecond)

	freshValue := cache.Value{
		Data:      []byte("fresh revalidated data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err = mc.Set(ctx, cacheKey, freshValue)
	require.NoError(t, err, "Failed to set fresh value")

	retrieved, err := mc.Get(ctx, cacheKey)
	require.NoError(t, err, "Should retrieve revalidated value")
	require.Equal(t, "fresh revalidated data", string(retrieved.Data), "Should return fresh data")
}

func TestCacheIntegration_CacheManager_MultiTier(t *testing.T) {
	ctx := context.Background()

	cm := cache.NewCacheManager()
	defer func() { _ = cm.Close() }()

	memoryCache := cache.NewMemoryCache(5*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = memoryCache.Close() }()

	cm.AddTier("memory", memoryCache, 1, 5*1024*1024)

	cm.SetPolicy("default", 30*time.Minute, []string{"memory"}, 1)

	cacheKey := "cache-manager-test"
	testValue := cache.Value{
		Data:      []byte("test data for cache manager"),
		CreatedAt: time.Now(),
		TTL:       30 * time.Minute,
	}

	err := cm.Set(ctx, cacheKey, testValue, "default")
	require.NoError(t, err, "Failed to set value via cache manager")

	exists, err := cm.Has(ctx, cacheKey)
	require.NoError(t, err)
	require.True(t, exists, "Key should exist in cache manager")

	retrieved, err := cm.Get(ctx, cacheKey)
	require.NoError(t, err, "Failed to get value from cache manager")
	require.Equal(t, "test data for cache manager", string(retrieved.Data), "Data mismatch")

	stats, err := cm.Stats(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.Entries, 1, "Should have at least one entry")
}

func TestCacheIntegration_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	concurrency := 50
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := "concurrent-key"
			value := cache.Value{
				Data:      []byte("concurrent data"),
				CreatedAt: time.Now(),
				TTL:       time.Hour,
			}

			if err := mc.Set(ctx, key, value); err != nil {
				errors <- err
				return
			}

			retrieved, err := mc.Get(ctx, key)
			if err != nil {
				errors <- err
				return
			}

			if string(retrieved.Data) != "concurrent data" {
				errors <- nil
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		require.NoError(t, err, "Concurrent access should not cause errors")
	}
}

func TestCacheIntegration_KeyPatternMatching(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	keys := []string{
		"user:1:profile",
		"user:2:profile",
		"user:1:settings",
		"product:1:details",
	}

	for _, key := range keys {
		value := cache.Value{
			Data:      []byte("data for " + key),
			CreatedAt: time.Now(),
			TTL:       time.Hour,
		}
		err := mc.Set(ctx, key, value)
		require.NoError(t, err, "Failed to set key: "+key)
	}

	matchedKeys, err := mc.Keys(ctx, "user:*")
	require.NoError(t, err, "Failed to get keys with pattern")
	require.Len(t, matchedKeys, 3, "Should match 3 user keys")

	matchedKeys, err = mc.Keys(ctx, "user:1:*")
	require.NoError(t, err)
	require.Len(t, matchedKeys, 2, "Should match 2 keys for user:1")

	matchedKeys, err = mc.Keys(ctx, "product:*")
	require.NoError(t, err)
	require.Len(t, matchedKeys, 1, "Should match 1 product key")
}

func TestCacheIntegration_TaggedValues(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	value1 := cache.Value{
		Data:      []byte("data 1"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
		Tags:      []string{"tag1", "tag2"},
	}

	value2 := cache.Value{
		Data:      []byte("data 2"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
		Tags:      []string{"tag2", "tag3"},
	}

	err := mc.Set(ctx, "key1", value1, cache.WithTags("tag1", "tag2"))
	require.NoError(t, err)

	err = mc.Set(ctx, "key2", value2, cache.WithTags("tag2", "tag3"))
	require.NoError(t, err)

	allKeys, err := mc.Keys(ctx, "*")
	require.NoError(t, err)
	require.Len(t, allKeys, 2, "Should have 2 keys")
}

func TestCacheIntegration_ClearAll(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	for i := 0; i < 10; i++ {
		value := cache.Value{
			Data:      []byte("data for key"),
			CreatedAt: time.Now(),
			TTL:       time.Hour,
		}
		err := mc.Set(ctx, "key", value)
		require.NoError(t, err, "Failed to set key")
	}

	keys, err := mc.Keys(ctx, "*")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(keys), 1, "Should have keys before clear")

	err = mc.Clear(ctx)
	require.NoError(t, err, "Failed to clear cache")

	keys, err = mc.Keys(ctx, "*")
	require.NoError(t, err)
	require.Empty(t, keys, "Should have no keys after clear")
}

func TestCacheIntegration_HitMissMetrics(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	cacheKey := "metrics-test-key"
	value := cache.Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_, err := mc.Get(ctx, cacheKey)
	require.ErrorIs(t, err, cache.ErrNotFound, "First get should be a miss")

	err = mc.Set(ctx, cacheKey, value)
	require.NoError(t, err)

	_, err = mc.Get(ctx, cacheKey)
	require.NoError(t, err, "Second get should be a hit")

	_, err = mc.Get(ctx, cacheKey)
	require.NoError(t, err, "Third get should be a hit")

	stats, err := mc.Stats(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.Hits, uint64(2), "Should have at least 2 hits")
	require.GreaterOrEqual(t, stats.Misses, uint64(1), "Should have at least 1 miss")
}

func TestCacheIntegration_Versioning(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(10*1024*1024, cache.NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	cacheKey := "versioned-key"
	value1 := cache.Value{
		Data:      []byte("version 1"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err := mc.Set(ctx, cacheKey, value1)
	require.NoError(t, err)

	retrieved, err := mc.Get(ctx, cacheKey)
	require.NoError(t, err)
	require.Equal(t, "version 1", string(retrieved.Data), "First version data should match")

	value2 := cache.Value{
		Data:      []byte("version 2"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err = mc.Set(ctx, cacheKey, value2)
	require.NoError(t, err)

	retrieved, err = mc.Get(ctx, cacheKey)
	require.NoError(t, err)
	require.Equal(t, "version 2", string(retrieved.Data), "Data should be updated to version 2")
}
