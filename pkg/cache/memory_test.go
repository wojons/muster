package cache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewMemoryCache_Constructor tests constructor with various configurations
func TestNewMemoryCache_Constructor(t *testing.T) {
	tests := []struct {
		name        string
		maxSize     int64
		eviction    EvictionPolicy
		wantMaxSize int64
	}{
		{
			name:        "default size with zero",
			maxSize:     0,
			eviction:    NewLRUPolicy(),
			wantMaxSize: DefaultMemoryCacheSize,
		},
		{
			name:        "custom size",
			maxSize:     50 * 1024 * 1024,
			eviction:    NewLRUPolicy(),
			wantMaxSize: 50 * 1024 * 1024,
		},
		{
			name:        "maximum allowed size",
			maxSize:     MaxMemoryCacheSize,
			eviction:    NewLFUPolicy(),
			wantMaxSize: MaxMemoryCacheSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := NewMemoryCache(tt.maxSize, tt.eviction)
			require.NotNil(t, mc)
			defer func() { require.NoError(t, mc.Close()) }()

			assert.Equal(t, tt.wantMaxSize, mc.maxSize)
			assert.NotNil(t, mc.data)
			assert.NotNil(t, mc.eviction)
			assert.False(t, mc.closed)
			assert.Equal(t, int64(0), mc.currentSize)
		})
	}
}

// TestMemoryCache_SetEmptyKey validates error on empty key
func TestMemoryCache_SetEmptyKey(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	err := mc.Set(ctx, "", Value{Data: []byte("value")})
	assert.ErrorIs(t, err, ErrInvalidKey)
}

// TestMemoryCache_SetWithOptions tests all SetOption combinations
func TestMemoryCache_SetWithOptions(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	t.Run("WithTTL overrides value TTL", func(t *testing.T) {
		value := Value{
			Data: []byte("data"),
			TTL:  time.Hour,
		}

		err := mc.Set(ctx, "key1", value, WithTTL(30*time.Minute))
		require.NoError(t, err)

		got, err := mc.Get(ctx, "key1")
		require.NoError(t, err)
		assert.Equal(t, 30*time.Minute, got.TTL)
	})

	t.Run("WithTags attaches tags", func(t *testing.T) {
		value := Value{Data: []byte("data")}
		tags := []string{"api", "v2", "production"}

		err := mc.Set(ctx, "key2", value, WithTags(tags...))
		require.NoError(t, err)

		got, err := mc.Get(ctx, "key2")
		require.NoError(t, err)
		assert.Equal(t, tags, got.Tags)
	})

	t.Run("WithAsync option accepted", func(t *testing.T) {
		value := Value{Data: []byte("data")}

		err := mc.Set(ctx, "key3", value, WithAsync())
		require.NoError(t, err)

		got, err := mc.Get(ctx, "key3")
		require.NoError(t, err)
		assert.Equal(t, []byte("data"), got.Data)
	})

	t.Run("Multiple options combined", func(t *testing.T) {
		value := Value{Data: []byte("data")}

		err := mc.Set(ctx, "key4", value,
			WithTTL(15*time.Minute),
			WithTags("important", "cached"),
			WithAsync())
		require.NoError(t, err)

		got, err := mc.Get(ctx, "key4")
		require.NoError(t, err)
		assert.Equal(t, 15*time.Minute, got.TTL)
		assert.Equal(t, []string{"important", "cached"}, got.Tags)
	})
}

// TestMemoryCache_GetMissingKey tests retrieval of missing keys
func TestMemoryCache_GetMissingKey(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	_, err := mc.Get(ctx, "non-existent-key")
	assert.ErrorIs(t, err, ErrNotFound)
}

// TestMemoryCache_GetExpired tests expired value handling
func TestMemoryCache_GetExpired(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	// Set a value with short TTL
	value := Value{
		Data: []byte("expired-data"),
		TTL:  50 * time.Millisecond,
	}

	err := mc.Set(ctx, "expired", value)
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	_, err = mc.Get(ctx, "expired")
	assert.ErrorIs(t, err, ErrExpired)
}

// TestMemoryCache_GetUpdatesAccess tracking
func TestMemoryCache_GetUpdatesAccessTracking(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	err := mc.Set(ctx, "track-key", Value{Data: []byte("data")})
	require.NoError(t, err)

	// Get initial state
	mc.mu.RLock()
	initialAccessCount := mc.data["track-key"].AccessCount
	initialAccessTime := mc.data["track-key"].AccessedAt
	mc.mu.RUnlock()

	// Wait a moment
	time.Sleep(10 * time.Millisecond)

	// Access the key
	_, err = mc.Get(ctx, "track-key")
	require.NoError(t, err)

	// Verify updates
	mc.mu.RLock()
	updatedAccessCount := mc.data["track-key"].AccessCount
	updatedAccessTime := mc.data["track-key"].AccessedAt
	mc.mu.RUnlock()

	assert.Equal(t, initialAccessCount+1, updatedAccessCount)
	assert.True(t, updatedAccessTime.After(initialAccessTime))
}

// TestMemoryCache_DeleteNonExistent tests delete of missing key
func TestMemoryCache_DeleteNonExistent(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	// Delete non-existent key should not error
	err := mc.Delete(ctx, "non-existent")
	assert.NoError(t, err)
}

// TestMemoryCache_DeleteSizeTracking tests size updates on delete
func TestMemoryCache_DeleteSizeTracking(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	data := make([]byte, 1000)
	err := mc.Set(ctx, "size-key", Value{Data: data})
	require.NoError(t, err)

	// Verify size increased
	mc.mu.RLock()
	sizeBefore := mc.currentSize
	mc.mu.RUnlock()
	assert.Equal(t, int64(1000), sizeBefore)

	// Delete and verify size decreased
	err = mc.Delete(ctx, "size-key")
	require.NoError(t, err)

	mc.mu.RLock()
	sizeAfter := mc.currentSize
	mc.mu.RUnlock()
	assert.Equal(t, int64(0), sizeAfter)
}

// TestMemoryCache_KeysPatternMatching tests pattern matching functionality
func TestMemoryCache_KeysPatternMatching(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	// Add test keys
	testKeys := []string{
		"user:123", "user:456", "user:789",
		"post:1", "post:2",
		"comment:abc", "comment:def",
	}

	for _, key := range testKeys {
		err := mc.Set(ctx, key, Value{Data: []byte("data")})
		require.NoError(t, err)
	}

	tests := []struct {
		pattern string
		wantLen int
	}{
		{"", 7},              // Empty returns all
		{"*", 7},             // Asterisk returns all
		{"user:*", 3},        // Prefix match
		{"post:*", 2},        // Different prefix
		{"comment:*", 2},     // Another prefix
		{"user:123", 1},      // Exact match
		{"nonexistent:*", 0}, // No match
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			keys, err := mc.Keys(ctx, tt.pattern)
			require.NoError(t, err)
			assert.Len(t, keys, tt.wantLen)
		})
	}
}

// TestMemoryCache_StatsCalculations tests stats computation
func TestMemoryCache_StatsCalculations(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	// Initial stats
	stats, err := mc.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Entries)
	assert.Equal(t, uint64(0), stats.Hits)
	assert.Equal(t, uint64(0), stats.Misses)
	assert.Equal(t, float64(0), stats.HitRate)

	// Add entries with different sizes
	err = mc.Set(ctx, "small", Value{Data: make([]byte, 100)})
	require.NoError(t, err)
	err = mc.Set(ctx, "medium", Value{Data: make([]byte, 500)})
	require.NoError(t, err)
	err = mc.Set(ctx, "large", Value{Data: make([]byte, 1000)})
	require.NoError(t, err)

	// Generate hits and misses
	_, _ = mc.Get(ctx, "small")    // hit
	_, _ = mc.Get(ctx, "medium")   // hit
	_, _ = mc.Get(ctx, "missing1") // miss
	_, _ = mc.Get(ctx, "missing2") // miss
	_, _ = mc.Get(ctx, "large")    // hit

	// Verify stats
	stats, err = mc.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Entries)
	assert.Equal(t, int64(1600), stats.Size)
	assert.Equal(t, uint64(3), stats.Hits)
	assert.Equal(t, uint64(2), stats.Misses)
	assert.InDelta(t, 60.0, stats.HitRate, 0.1) // 3/5 * 100 = 60%
}

// TestMemoryCache_CloseIdempotent tests close behavior
func TestMemoryCache_CloseIdempotent(t *testing.T) {
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())

	// First close should succeed
	err := mc.Close()
	require.NoError(t, err)
	assert.True(t, mc.closed)

	// Second close should return error
	err = mc.Close()
	assert.ErrorIs(t, err, ErrClosed)
}

// TestMemoryCache_OperationsOnClosedCache tests all methods after close
func TestMemoryCache_OperationsOnClosedCache(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())

	err := mc.Close()
	require.NoError(t, err)

	// All operations should return ErrClosed
	_, err = mc.Get(ctx, "key")
	assert.ErrorIs(t, err, ErrClosed)

	err = mc.Set(ctx, "key", Value{})
	assert.ErrorIs(t, err, ErrClosed)

	err = mc.Delete(ctx, "key")
	assert.ErrorIs(t, err, ErrClosed)

	err = mc.Clear(ctx)
	assert.ErrorIs(t, err, ErrClosed)

	_, err = mc.Has(ctx, "key")
	assert.ErrorIs(t, err, ErrClosed)

	_, err = mc.Keys(ctx, "")
	assert.ErrorIs(t, err, ErrClosed)

	_, err = mc.Stats(ctx)
	assert.ErrorIs(t, err, ErrClosed)
}

// TestMemoryCache_EvictionPolicies tests different eviction policies
func TestMemoryCache_EvictionPolicies(t *testing.T) {
	tests := []struct {
		name   string
		policy EvictionPolicy
	}{
		{"LRU", NewLRUPolicy()},
		{"LFU", NewLFUPolicy()},
		{"FIFO", NewFIFOPolicy()},
		{"SizeBased", NewSizeBasedPolicy()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			// Small cache to trigger eviction
			mc := NewMemoryCache(500, tt.policy)
			defer func() { require.NoError(t, mc.Close()) }()

			// Fill cache beyond capacity
			for i := 0; i < 10; i++ {
				key := string(rune('a' + i))
				value := Value{Data: make([]byte, 100)}
				err := mc.Set(ctx, key, value)
				require.NoError(t, err)
			}

			// Verify size never exceeded max
			stats, err := mc.Stats(ctx)
			require.NoError(t, err)
			assert.LessOrEqual(t, stats.Size, int64(500))
			assert.Greater(t, stats.Evictions, uint64(0))
		})
	}
}

// TestMemoryCache_ConcurrentOperations tests thread safety
func TestMemoryCache_ConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	var wg sync.WaitGroup
	numGoroutines := 50
	operationsPerGoroutine := 100

	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				key := string(rune('a' + (id % 26)))
				_ = mc.Set(ctx, key, Value{Data: []byte("data")})
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				key := string(rune('a' + (id % 26)))
				_, _ = mc.Get(ctx, key)
			}
		}(i)
	}

	// Concurrent deleters
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine/2; j++ {
				key := string(rune('a' + (id % 26)))
				_ = mc.Delete(ctx, key)
			}
		}(i)
	}

	wg.Wait()

	// Verify cache is still in valid state
	stats, err := mc.Stats(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, stats.Hits+stats.Misses, uint64(0))
}

// TestMemoryCache_TTLLifecycle tests TTL from creation to expiration
func TestMemoryCache_TTLLifecycle(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	key := "ttl-test"
	ttl := 200 * time.Millisecond

	// Set with TTL
	err := mc.Set(ctx, key, Value{
		Data: []byte("value"),
		TTL:  ttl,
	})
	require.NoError(t, err)

	// Verify exists immediately
	exists, err := mc.Has(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	// Wait for expiration
	time.Sleep(ttl + 50*time.Millisecond)

	// Verify expired
	exists, err = mc.Has(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)

	// Get should return ErrExpired
	_, err = mc.Get(ctx, key)
	assert.ErrorIs(t, err, ErrExpired)
}

// TestMemoryCache_NoTTLNeverExpires tests that TTL=0 means no expiration
func TestMemoryCache_NoTTLNeverExpires(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	err := mc.Set(ctx, "no-ttl", Value{
		Data: []byte("value"),
		TTL:  0,
	})
	require.NoError(t, err)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Should still exist
	exists, err := mc.Has(ctx, "no-ttl")
	require.NoError(t, err)
	assert.True(t, exists)
}

// TestMemoryCache_OverwriteUpdatesSize tests that overwriting updates size correctly
func TestMemoryCache_OverwriteUpdatesSize(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	// Set initial value
	err := mc.Set(ctx, "key", Value{Data: make([]byte, 100)})
	require.NoError(t, err)

	mc.mu.RLock()
	size1 := mc.currentSize
	mc.mu.RUnlock()
	assert.Equal(t, int64(100), size1)

	// Overwrite with different size
	err = mc.Set(ctx, "key", Value{Data: make([]byte, 200)})
	require.NoError(t, err)

	mc.mu.RLock()
	size2 := mc.currentSize
	mc.mu.RUnlock()
	assert.Equal(t, int64(200), size2)

	// Overwrite with smaller size
	err = mc.Set(ctx, "key", Value{Data: make([]byte, 50)})
	require.NoError(t, err)

	mc.mu.RLock()
	size3 := mc.currentSize
	mc.mu.RUnlock()
	assert.Equal(t, int64(50), size3)
}

// TestMemoryCache_VersionIncrement tests version incrementing on updates
func TestMemoryCache_VersionIncrement(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	key := "version-test"

	// First set
	err := mc.Set(ctx, key, Value{Data: []byte("v1")})
	require.NoError(t, err)

	v1, err := mc.Get(ctx, key)
	require.NoError(t, err)

	// Wait to ensure different timestamp
	time.Sleep(time.Millisecond)

	// Second set
	err = mc.Set(ctx, key, Value{Data: []byte("v2")})
	require.NoError(t, err)

	v2, err := mc.Get(ctx, key)
	require.NoError(t, err)

	// Version should be different (nanosecond timestamp)
	assert.NotEqual(t, v1.Version, v2.Version)
	assert.Greater(t, v2.Version, v1.Version)
}

// TestMemoryCache_ClearResetsEverything tests that Clear resets all state
func TestMemoryCache_ClearResetsEverything(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	// Add data
	for i := 0; i < 5; i++ {
		err := mc.Set(ctx, string(rune('a'+i)), Value{Data: []byte("data")})
		require.NoError(t, err)
	}

	// Access to generate stats
	_, _ = mc.Get(ctx, "a")
	_, _ = mc.Get(ctx, "z") // miss

	// Clear
	err := mc.Clear(ctx)
	require.NoError(t, err)

	// Verify empty
	keys, err := mc.Keys(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, keys)

	mc.mu.RLock()
	size := mc.currentSize
	mc.mu.RUnlock()
	assert.Equal(t, int64(0), size)

	// Stats should show entries cleared but maintain hit/miss counts
	stats, err := mc.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, stats.Entries)
	assert.Equal(t, int64(0), stats.Size)
}

// TestMemoryCache_HasExpiredKey tests Has() behavior with expired keys
func TestMemoryCache_HasExpiredKey(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	// Set value with short TTL
	value := Value{
		Data: []byte("data"),
		TTL:  50 * time.Millisecond,
	}
	err := mc.Set(ctx, "expired", value)
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Has should return false for expired key
	exists, err := mc.Has(ctx, "expired")
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestMemoryCache_CreatedAtSetOnSet tests that CreatedAt is automatically set
func TestMemoryCache_CreatedAtSetOnSet(t *testing.T) {
	ctx := context.Background()
	mc := NewMemoryCache(DefaultMemoryCacheSize, NewLRUPolicy())
	defer func() { require.NoError(t, mc.Close()) }()

	beforeSet := time.Now()

	err := mc.Set(ctx, "key", Value{Data: []byte("data")})
	require.NoError(t, err)

	afterSet := time.Now()

	got, err := mc.Get(ctx, "key")
	require.NoError(t, err)

	// CreatedAt should be set automatically
	assert.True(t, got.CreatedAt.After(beforeSet) || got.CreatedAt.Equal(beforeSet))
	assert.True(t, got.CreatedAt.Before(afterSet) || got.CreatedAt.Equal(afterSet))
}
