package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryCache_Get(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err := mc.Set(ctx, "test-key", value)
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	retrieved, err := mc.Get(ctx, "test-key")
	if err != nil {
		t.Fatalf("Failed to get value: %v", err)
	}

	if string(retrieved.Data) != "test data" {
		t.Errorf("Expected data 'test data', got '%s'", string(retrieved.Data))
	}
}

func TestMemoryCache_GetNotFound(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()

	_, err := mc.Get(ctx, "non-existent")
	if err != ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}
}

func TestMemoryCache_Set(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err := mc.Set(ctx, "test-key", value)
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	has, err := mc.Has(ctx, "test-key")
	if err != nil {
		t.Fatalf("Failed to check has: %v", err)
	}

	if !has {
		t.Error("Expected key to exist")
	}
}

func TestMemoryCache_Delete(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = mc.Set(ctx, "test-key", value)

	err := mc.Delete(ctx, "test-key")
	if err != nil {
		t.Fatalf("Failed to delete: %v", err)
	}

	has, _ := mc.Has(ctx, "test-key")
	if has {
		t.Error("Expected key to be deleted")
	}
}

func TestMemoryCache_Clear(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = mc.Set(ctx, "key1", value)
	_ = mc.Set(ctx, "key2", value)

	err := mc.Clear(ctx)
	if err != nil {
		t.Fatalf("Failed to clear: %v", err)
	}

	has, _ := mc.Has(ctx, "key1")
	if has {
		t.Error("Expected cache to be cleared")
	}
}

func TestMemoryCache_Expiration(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       10 * time.Millisecond,
	}

	_ = mc.Set(ctx, "test-key", value)

	time.Sleep(15 * time.Millisecond)

	_, err := mc.Get(ctx, "test-key")
	if err != ErrExpired {
		t.Errorf("Expected ErrExpired, got %v", err)
	}
}

func TestMemoryCache_Eviction(t *testing.T) {
	mc := NewMemoryCache(1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		value := Value{
			Data:      make([]byte, 150),
			CreatedAt: time.Now(),
			TTL:       time.Hour,
		}
		_ = mc.Set(ctx, string(rune('a'+i)), value)
	}

	stats, err := mc.Stats(ctx)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.Evictions == 0 {
		t.Error("Expected some evictions")
	}
}

func TestMemoryCache_Stats(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = mc.Set(ctx, "key1", value)

	_, _ = mc.Get(ctx, "key1")
	_, _ = mc.Get(ctx, "key2")
	_, _ = mc.Get(ctx, "key1")

	stats, err := mc.Stats(ctx)
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	if stats.Hits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.Hits)
	}

	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}

	if stats.HitRate < 66 || stats.HitRate > 67 {
		t.Errorf("Expected hit rate ~66.67, got %f", stats.HitRate)
	}
}

func TestLRUPolicy(t *testing.T) {
	policy := NewLRUPolicy()
	data := make(map[string]*CacheEntry)

	now := time.Now()
	data["a"] = &CacheEntry{
		Value:      Value{CreatedAt: now.Add(-2 * time.Hour)},
		AccessedAt: now.Add(-2 * time.Hour),
	}
	data["b"] = &CacheEntry{
		Value:      Value{CreatedAt: now.Add(-time.Hour)},
		AccessedAt: now.Add(-time.Hour),
	}
	data["c"] = &CacheEntry{
		Value:      Value{CreatedAt: now},
		AccessedAt: now,
	}

	key, err := policy.SelectForEviction(data)
	if err != nil {
		t.Fatalf("Failed to select for eviction: %v", err)
	}

	if key != "a" {
		t.Errorf("Expected to evict 'a', got '%s'", key)
	}
}

func TestLFUPolicy(t *testing.T) {
	policy := NewLFUPolicy()
	data := make(map[string]*CacheEntry)

	data["a"] = &CacheEntry{
		Value:       Value{},
		AccessCount: 10,
	}
	data["b"] = &CacheEntry{
		Value:       Value{},
		AccessCount: 5,
	}
	data["c"] = &CacheEntry{
		Value:       Value{},
		AccessCount: 20,
	}

	key, err := policy.SelectForEviction(data)
	if err != nil {
		t.Fatalf("Failed to select for eviction: %v", err)
	}

	if key != "b" {
		t.Errorf("Expected to evict 'b', got '%s'", key)
	}
}

func TestFIFOPolicy(t *testing.T) {
	policy := NewFIFOPolicy()
	data := make(map[string]*CacheEntry)

	now := time.Now()
	data["a"] = &CacheEntry{
		Value:      Value{CreatedAt: now.Add(-2 * time.Hour)},
		AccessedAt: now,
	}
	data["b"] = &CacheEntry{
		Value:      Value{CreatedAt: now.Add(-time.Hour)},
		AccessedAt: now,
	}
	data["c"] = &CacheEntry{
		Value:      Value{CreatedAt: now},
		AccessedAt: now,
	}

	key, err := policy.SelectForEviction(data)
	if err != nil {
		t.Fatalf("Failed to select for eviction: %v", err)
	}

	if key != "a" {
		t.Errorf("Expected to evict 'a', got '%s'", key)
	}
}

func TestClose(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = mc.Set(ctx, "test-key", value)

	err := mc.Close()
	if err != nil {
		t.Fatalf("Failed to close: %v", err)
	}

	_, err = mc.Get(ctx, "test-key")
	if err != ErrClosed {
		t.Errorf("Expected ErrClosed after close, got %v", err)
	}
}

func TestWithTTL(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err := mc.Set(ctx, "test-key", value, WithTTL(5*time.Minute))
	if err != nil {
		t.Fatalf("Failed to set with TTL: %v", err)
	}

	retrieved, _ := mc.Get(ctx, "test-key")
	if retrieved.TTL != 5*time.Minute {
		t.Errorf("Expected TTL 5m, got %v", retrieved.TTL)
	}
}

func TestWithTags(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err := mc.Set(ctx, "test-key", value, WithTags("test", "example"))
	if err != nil {
		t.Fatalf("Failed to set with tags: %v", err)
	}

	retrieved, _ := mc.Get(ctx, "test-key")
	if len(retrieved.Tags) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(retrieved.Tags))
	}
}

func TestKeysPattern(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer func() { _ = mc.Close() }()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = mc.Set(ctx, "user:1", value)
	_ = mc.Set(ctx, "user:2", value)
	_ = mc.Set(ctx, "post:1", value)

	keys, err := mc.Keys(ctx, "user:*")
	if err != nil {
		t.Fatalf("Failed to get keys: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("Expected 2 keys, got %d", len(keys))
	}
}

func TestCacheManager_MetricsPruning(t *testing.T) {
	cm := NewCacheManager()
	defer cm.Stop()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	for i := 0; i < maxMetricsEntries+500; i++ {
		key := string(rune('a'+(i%26))) + string(rune('0'+(i/26)%10))
		_ = cm.Set(ctx, key, value, "default")
		_, _ = cm.Get(ctx, key)
	}

	cm.metrics.mu.RLock()
	hitsLen := len(cm.metrics.hits)
	evictionsLen := len(cm.metrics.evictions)
	cm.metrics.mu.RUnlock()

	if hitsLen > maxMetricsEntries {
		t.Errorf("Expected hits map to be pruned to <= %d, got %d", maxMetricsEntries, hitsLen)
	}

	if evictionsLen > maxMetricsEntries {
		t.Errorf("Expected evictions map to be pruned to <= %d, got %d", maxMetricsEntries, evictionsLen)
	}
}

func TestCacheManager_PruneMetrics(t *testing.T) {
	cm := NewCacheManager()
	defer cm.Stop()

	cm.metrics.mu.Lock()
	for i := 0; i < 2000; i++ {
		cm.metrics.hits[string(rune(i))] = int64(i)
		cm.metrics.evictions[string(rune(i))] = int64(i)
	}
	cm.metrics.lastPrune = time.Now().Add(-15 * time.Minute)
	cm.metrics.mu.Unlock()

	cm.pruneMetrics()

	cm.metrics.mu.RLock()
	hitsLen := len(cm.metrics.hits)
	evictionsLen := len(cm.metrics.evictions)
	cm.metrics.mu.RUnlock()

	targetSize := maxMetricsEntries / 2
	if hitsLen > targetSize {
		t.Errorf("Expected hits map to be pruned to <= %d, got %d", targetSize, hitsLen)
	}
	if evictionsLen > targetSize {
		t.Errorf("Expected evictions map to be pruned to <= %d, got %d", targetSize, evictionsLen)
	}
}

func TestCacheManager_PruneMetrics_EarlyExit(t *testing.T) {
	cm := NewCacheManager()
	defer cm.Stop()

	cm.metrics.mu.Lock()
	for i := 0; i < 2000; i++ {
		cm.metrics.hits[string(rune(i))] = int64(i)
	}
	cm.metrics.lastPrune = time.Now()
	cm.metrics.mu.Unlock()

	cm.pruneMetrics()

	cm.metrics.mu.RLock()
	hitsLen := len(cm.metrics.hits)
	cm.metrics.mu.RUnlock()

	if hitsLen != 2000 {
		t.Errorf("Expected hits map to not be pruned (too soon), got %d", hitsLen)
	}
}

func TestCacheManager_Stop(t *testing.T) {
	cm := NewCacheManager()

	cm.Stop()

	select {
	case <-cm.metrics.stopPrune:
	default:
		t.Error("Expected stopPrune channel to be closed")
	}
}

func TestCacheManager_ConcurrentMetricsAccess(t *testing.T) {
	cm := NewCacheManager()
	defer cm.Stop()

	done := make(chan bool)
	go func() {
		for i := 0; i < 1000; i++ {
			cm.recordHit("tier1")
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			cm.pruneMetrics()
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	<-done
	<-done
}

func TestCacheManager_MetricsBoundedGrowth(t *testing.T) {
	cm := NewCacheManager()
	defer cm.Stop()

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	initialHits := 0
	cm.metrics.mu.RLock()
	initialHits = len(cm.metrics.hits)
	cm.metrics.mu.RUnlock()

	for i := 0; i < 3000; i++ {
		key := string(rune('a'+(i%26))) + string(rune('0'+(i/26)%10)) + string(rune('0'+(i/100)%10))
		_ = cm.Set(ctx, key, value, "default")
		_, _ = cm.Get(ctx, key)
	}

	cm.metrics.mu.RLock()
	finalHits := len(cm.metrics.hits)
	finalEvictions := len(cm.metrics.evictions)
	cm.metrics.mu.RUnlock()

	if finalHits > maxMetricsEntries {
		t.Errorf("Expected hits to be bounded to <= %d, got %d", maxMetricsEntries, finalHits)
	}

	if finalEvictions > maxMetricsEntries {
		t.Errorf("Expected evictions to be bounded to <= %d, got %d", maxMetricsEntries, finalEvictions)
	}

	t.Logf("Metrics bounded: hits=%d (initial=%d), evictions=%d", finalHits, initialHits, finalEvictions)
}
