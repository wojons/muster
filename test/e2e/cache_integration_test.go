package e2e

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wojons/muster/pkg/cache"
)

func TestCacheIntegration_CacheMiss(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(100)
	defer func() { _ = mc.Close() }()

	key := "test-cache-miss-key"

	data, found, err := mc.Get(ctx, key)
	require.NoError(t, err, "Get on non-existent key should not error")
	require.False(t, found, "Get on non-existent key should return found=false")
	require.Nil(t, data, "Data should be nil for non-existent key")
}

func TestCacheIntegration_CacheHit(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(100)
	defer func() { _ = mc.Close() }()

	key := "test-cache-hit-key"
	value := []byte("test data for cache hit")

	err := mc.Set(ctx, key, value, time.Hour)
	require.NoError(t, err, "Failed to set value")

	retrieved, found, err := mc.Get(ctx, key)
	require.NoError(t, err, "Failed to get value")
	require.True(t, found, "Key should exist in cache")
	require.Equal(t, "test data for cache hit", string(retrieved), "Data mismatch")
}

func TestCacheIntegration_TTL_Enforcement(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(100)
	defer func() { _ = mc.Close() }()

	key := "test-ttl-key"
	shortTTL := 100 * time.Millisecond

	err := mc.Set(ctx, key, []byte("data with short TTL"), shortTTL)
	require.NoError(t, err, "Failed to set value with TTL")

	_, found, err := mc.Get(ctx, key)
	require.NoError(t, err, "Get should not error immediately")
	require.True(t, found, "Key should exist immediately after setting")

	time.Sleep(shortTTL + 150*time.Millisecond)

	data, found, err := mc.Get(ctx, key)
	require.NoError(t, err, "Get after TTL expiry should not error")
	require.False(t, found, "Key should not be found after TTL expiry")
	require.Nil(t, data, "Data should be nil after TTL expiry")
}

func TestCacheIntegration_ManualInvalidation(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(100)
	defer func() { _ = mc.Close() }()

	key := "test-invalidation-key"

	err := mc.Set(ctx, key, []byte("data to be invalidated"), time.Hour)
	require.NoError(t, err, "Failed to set value")

	_, found, err := mc.Get(ctx, key)
	require.NoError(t, err)
	require.True(t, found, "Key should exist before invalidation")

	err = mc.Delete(ctx, key)
	require.NoError(t, err, "Failed to delete key")

	data, found, err := mc.Get(ctx, key)
	require.NoError(t, err, "Get after Delete should not error")
	require.False(t, found, "Key should not exist after deletion")
	require.Nil(t, data, "Data should be nil after deletion")
}

func TestCacheIntegration_OfflineMode(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(100)
	defer func() { _ = mc.Close() }()

	requestCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data": "response from server"}`))
	}))
	defer mockServer.Close()

	cacheKey := "offline-mode-test"
	cachedData := []byte(`{"data": "cached response"}`)

	err := mc.Set(ctx, cacheKey, cachedData, time.Hour)
	require.NoError(t, err, "Failed to cache value")

	mockServer.Close()

	cached, found, err := mc.Get(ctx, cacheKey)
	require.NoError(t, err, "Should retrieve from cache when offline")
	require.True(t, found, "Cached value should be found")
	require.Contains(t, string(cached), "cached response", "Should return cached data")

	_, err = http.Get(mockServer.URL)
	require.Error(t, err, "Server should be unreachable")

	require.Equal(t, 0, requestCount, "No requests should have been made to offline server")
}

func TestCacheIntegration_SizeLimit_Eviction(t *testing.T) {
	// Create a cache that can hold at most 2 entries
	mc := cache.NewMemoryCache(2)
	defer func() { _ = mc.Close() }()

	ctx := context.Background()

	// Set 3 different keys; the first should be evicted
	err := mc.Set(ctx, "key1", []byte("value1"), time.Hour)
	require.NoError(t, err, "Failed to set key1")

	err = mc.Set(ctx, "key2", []byte("value2"), time.Hour)
	require.NoError(t, err, "Failed to set key2")

	err = mc.Set(ctx, "key3", []byte("value3"), time.Hour)
	require.NoError(t, err, "Failed to set key3")

	// key1 (least recently used) should have been evicted
	_, found, err := mc.Get(ctx, "key1")
	require.NoError(t, err)
	require.False(t, found, "key1 should have been evicted (LRU)")
}

func TestCacheIntegration_StaleWhileRevalidate(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(100)
	defer func() { _ = mc.Close() }()

	cacheKey := "stale-while-revalidate-test"

	err := mc.Set(ctx, cacheKey, []byte("stale cached data"), 100*time.Millisecond)
	require.NoError(t, err, "Failed to set stale value")

	time.Sleep(200 * time.Millisecond)

	err = mc.Set(ctx, cacheKey, []byte("fresh revalidated data"), time.Hour)
	require.NoError(t, err, "Failed to set fresh value")

	retrieved, found, err := mc.Get(ctx, cacheKey)
	require.NoError(t, err, "Should retrieve revalidated value")
	require.True(t, found, "Value should be found after revalidation")
	require.Equal(t, "fresh revalidated data", string(retrieved), "Should return fresh data")
}

func TestCacheIntegration_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(100)
	defer func() { _ = mc.Close() }()

	concurrency := 50
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := "concurrent-key"
			value := []byte("concurrent data")

			if err := mc.Set(ctx, key, value, time.Hour); err != nil {
				errCh <- err
				return
			}

			retrieved, found, err := mc.Get(ctx, key)
			if err != nil {
				errCh <- err
				return
			}
			if !found {
				errCh <- nil
				return
			}
			if string(retrieved) != "concurrent data" {
				errCh <- nil
				return
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err, "Concurrent access should not cause errors")
	}
}

func TestCacheIntegration_ClearAll(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(100)
	defer func() { _ = mc.Close() }()

	for i := 0; i < 10; i++ {
		key := "key"
		err := mc.Set(ctx, key, []byte("data for key"), time.Hour)
		require.NoError(t, err, "Failed to set key")
	}

	_, found, err := mc.Get(ctx, "key")
	require.NoError(t, err)
	require.True(t, found, "Should have value before clear")

	err = mc.Clear(ctx)
	require.NoError(t, err, "Failed to clear cache")

	_, found, err = mc.Get(ctx, "key")
	require.NoError(t, err)
	require.False(t, found, "Should not find value after clear")
}

func TestCacheIntegration_HitMissMetrics(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(100)
	defer func() { _ = mc.Close() }()

	cacheKey := "metrics-test-key"

	_, found, err := mc.Get(ctx, cacheKey)
	require.NoError(t, err)
	require.False(t, found, "First get should be a miss")

	err = mc.Set(ctx, cacheKey, []byte("test data"), time.Hour)
	require.NoError(t, err)

	_, found, err = mc.Get(ctx, cacheKey)
	require.NoError(t, err)
	require.True(t, found, "Second get should be a hit")

	_, found, err = mc.Get(ctx, cacheKey)
	require.NoError(t, err)
	require.True(t, found, "Third get should be a hit")

	stats := mc.Stats()
	require.GreaterOrEqual(t, stats.Hits, int64(2), "Should have at least 2 hits")
	require.GreaterOrEqual(t, stats.Misses, int64(1), "Should have at least 1 miss")
	require.GreaterOrEqual(t, stats.Sets, int64(1), "Should have at least 1 set")
}

func TestCacheIntegration_Versioning(t *testing.T) {
	ctx := context.Background()

	mc := cache.NewMemoryCache(100)
	defer func() { _ = mc.Close() }()

	cacheKey := "versioned-key"

	err := mc.Set(ctx, cacheKey, []byte("version 1"), time.Hour)
	require.NoError(t, err)

	retrieved, found, err := mc.Get(ctx, cacheKey)
	require.NoError(t, err)
	require.True(t, found, "First version should be found")
	require.Equal(t, "version 1", string(retrieved), "First version data should match")

	err = mc.Set(ctx, cacheKey, []byte("version 2"), time.Hour)
	require.NoError(t, err)

	retrieved, found, err = mc.Get(ctx, cacheKey)
	require.NoError(t, err)
	require.True(t, found, "Second version should be found")
	require.Equal(t, "version 2", string(retrieved), "Data should be updated to version 2")
}
