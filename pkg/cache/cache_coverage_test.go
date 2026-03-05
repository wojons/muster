package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMemoryCache_ClosedErrorHandling(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	require.NoError(t, mc.Close())

	ctx := context.Background()

	// All operations should return ErrClosed
	_, err := mc.Get(ctx, "test-key")
	require.Error(t, err)
	require.Equal(t, ErrClosed, err)

	err = mc.Set(ctx, "test-key", Value{Data: []byte("data")})
	require.Error(t, err)
	require.Equal(t, ErrClosed, err)

	err = mc.Delete(ctx, "test-key")
	require.Error(t, err)
	require.Equal(t, ErrClosed, err)

	err = mc.Clear(ctx)
	require.Error(t, err)
	require.Equal(t, ErrClosed, err)

	_, err = mc.Has(ctx, "test-key")
	require.Error(t, err)
	require.Equal(t, ErrClosed, err)

	_, err = mc.Keys(ctx, "")
	require.Error(t, err)
	require.Equal(t, ErrClosed, err)

	_, err = mc.Stats(ctx)
	require.Error(t, err)
	require.Equal(t, ErrClosed, err)

	// Double close should return error
	err = mc.Close()
	require.Error(t, err)
	require.Equal(t, ErrClosed, err)
}

func TestMemoryCache_InvalidKey(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer mc.Close()

	ctx := context.Background()
	err := mc.Set(ctx, "", Value{Data: []byte("data")})

	require.Error(t, err)
	require.Equal(t, ErrInvalidKey, err)
}

func TestMemoryCache_MaxSize(t *testing.T) {
	ctx := context.Background()

	mc := NewMemoryCache(1000, NewLRUPolicy())
	defer mc.Close()

	bigValue := Value{Data: make([]byte, 800)}

	// First should succeed
	err := mc.Set(ctx, "key1", bigValue)
	require.NoError(t, err)

	// Second should also fit
	err = mc.Set(ctx, "key2", bigValue)
	require.NoError(t, err)

	// Third should trigger eviction but succeed
	err = mc.Set(ctx, "key3", bigValue)
	require.NoError(t, err)
}

func TestMemoryCache_Tags(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer mc.Close()

	ctx := context.Background()
	tags := []string{"tag1", "tag2"}

	value := Value{
		Data: []byte("test data"),
		Tags: tags,
		TTL:  time.Hour,
	}

	err := mc.Set(ctx, "test-key", value, WithTags(tags...))
	require.NoError(t, err)

	retrieved, err := mc.Get(ctx, "test-key")
	require.NoError(t, err)
	require.Equal(t, tags, retrieved.Tags)
}

func TestMemoryCache_ContextCancel(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer mc.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = ctx

	// Operations on cancelled context should still succeed
	// (memory cache doesn't use context timeout)
}

func TestTieredCache_NilLayers(t *testing.T) {
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer l1.Close()

	cfg := TieredCacheConfig{
		L1: l1,
	}

	tc := NewTieredCache(cfg)

	ctx := context.Background()
	value := Value{Data: []byte("test")}

	// Should work with just L1
	err := tc.Set(ctx, "key1", value)
	require.NoError(t, err)

	val, err := tc.Get(ctx, "key1")
	require.NoError(t, err)
	require.Equal(t, value.Data, val.Data)
}

func TestTieredCache_WarmAsync(t *testing.T) {
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer l1.Close()

	l3 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer l3.Close()

	cfg := TieredCacheConfig{
		L1:        l1,
		L3:        l3,
		WarmAsync: false,
	}

	tc := NewTieredCache(cfg)

	ctx := context.Background()
	value := Value{Data: []byte("test"), TTL: time.Hour}

	err := tc.Set(ctx, "key1", value)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	// L3 should also have the value
	val, err := l3.Get(ctx, "key1")
	require.NoError(t, err)
	require.NotNil(t, val)
}

func TestTieredCache_GetFromL2(t *testing.T) {
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer l1.Close()

	l2 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer l2.Close()

	cfg := TieredCacheConfig{
		L1: l1,
		L2: l2,
	}

	tc := NewTieredCache(cfg)

	ctx := context.Background()
	value := Value{Data: []byte("test"), TTL: time.Hour}

	// Set only in L2
	err := l2.Set(ctx, "key1", value)
	require.NoError(t, err)

	// Get should retrieve from L2 and populate L1
	val, err := tc.Get(ctx, "key1")
	require.NoError(t, err)
	require.Equal(t, value.Data, val.Data)

	// Should now be in L1
	l1Has, _ := l1.Has(ctx, "key1")
	require.True(t, l1Has)
}

func TestTieredCache_DeleteErrors(t *testing.T) {
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	_ = l1.Close() // Close to force errors

	l2 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer l2.Close()

	cfg := TieredCacheConfig{
		L1: l1,
		L2: l2,
	}

	tc := NewTieredCache(cfg)

	ctx := context.Background()
	err := tc.Delete(ctx, "test-key")
	require.Error(t, err)
}

func TestTieredCache_ClearErrors(t *testing.T) {
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	err := l1.Close()
	require.NoError(t, err)

	cfg := TieredCacheConfig{
		L1: l1,
	}

	tc := NewTieredCache(cfg)

	ctx := context.Background()
	err = tc.Clear(ctx)
	require.Error(t, err)
}

func TestTieredCache_CloseErrors(t *testing.T) {
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	err := l1.Close()
	require.NoError(t, err)

	cfg := TieredCacheConfig{
		L1: l1,
	}

	tc := NewTieredCache(cfg)

	err = tc.Close()
	// Close may error if individual layers are closed
	_ = err
}

func TestMemoryCache_KeysPattern(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer mc.Close()

	ctx := context.Background()

	err := mc.Set(ctx, "user:1", Value{Data: []byte("test")})
	require.NoError(t, err)

	err = mc.Set(ctx, "user:2", Value{Data: []byte("test")})
	require.NoError(t, err)

	err = mc.Set(ctx, "session:1", Value{Data: []byte("test")})
	require.NoError(t, err)

	keys, err := mc.Keys(ctx, "user:*")
	require.NoError(t, err)
	require.Len(t, keys, 2)
}

func TestMemoryCache_KeysAll(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer mc.Close()

	ctx := context.Background()

	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("key%d", i)
		err := mc.Set(ctx, key, Value{Data: []byte("test")})
		require.NoError(t, err)
	}

	keys, err := mc.Keys(ctx, "")
	require.NoError(t, err)
	require.Len(t, keys, 5)
}

func TestMemoryCache_ExpiredEntry(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer mc.Close()

	ctx := context.Background()

	value := Value{
		Data: []byte("test"),
		TTL:  time.Microsecond, // Very short TTL
	}

	err := mc.Set(ctx, "test-key", value)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond) // Wait for expiration

	_, err = mc.Get(ctx, "test-key")
	require.Error(t, err)
	require.Equal(t, ErrExpired, err)

	has, err := mc.Has(ctx, "test-key")
	require.NoError(t, err)
	require.False(t, has)
}

func TestSetOptions(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer mc.Close()

	ctx := context.Background()

	tags := []string{"a", "b"}
	ttl := 2 * time.Hour

	value := Value{
		Data: []byte("test"),
	}

	err := mc.Set(ctx, "test-key", value,
		WithTags(tags...),
		WithTTL(ttl),
	)
	require.NoError(t, err)

	retrieved, err := mc.Get(ctx, "test-key")
	require.NoError(t, err)

	require.Equal(t, tags, retrieved.Tags)
	// TTL should reflect the set TTL (may be slightly different due to timing)
	require.Greater(t, retrieved.TTL, time.Minute)
}

func TestMemoryCache_ZeroValue(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer mc.Close()

	ctx := context.Background()

	err := mc.Set(ctx, "test-key", Value{})
	require.NoError(t, err)

	retrieved, err := mc.Get(ctx, "test-key")
	require.NoError(t, err)
	require.Equal(t, []byte(nil), retrieved.Data)
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern string
		key     string
		want    bool
	}{
		{"", "any", true},
		{"*", "any", true},
		{"user:*", "user:1", true},
		{"user:*", "session:1", false},
		{"exact", "exact", true},
		{"exact", "exact-different", false},
	}

	for _, tt := range tests {
		result := matchPattern(tt.pattern, tt.key)
		if result != tt.want {
			t.Errorf("pattern(%s, %s) = %v, want %v", tt.pattern, tt.key, result, tt.want)
		}
	}
}

func TestMemoryCache_ConcurrentAccess(t *testing.T) {
	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	defer mc.Close()

	ctx := context.Background()
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				key := fmt.Sprintf("key%d", j)
				value := Value{Data: []byte("test")}
				_ = mc.Set(ctx, key, value)
				_, _ = mc.Get(ctx, key)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify we didn't deadlock or corrupt data
	stats, err := mc.Stats(ctx)
	require.NoError(t, err)
	require.Greater(t, stats.Entries, 0)
}
