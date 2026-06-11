package cache

import (
	"context"
	"testing"
	"time"
)

// cacheFactory is a function that creates a Cache instance for testing.
type cacheFactory func() Cache

// testCacheRoundtrip verifies basic Set and Get operations.
func testCacheRoundtrip(t *testing.T, factory cacheFactory) {
	t.Helper()
	c := factory()
	defer c.Close()

	ctx := context.Background()
	err := c.Set(ctx, "hello", []byte("world"), time.Hour)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, ok, err := c.Get(ctx, "hello")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() returned ok=false, want true")
	}
	if string(got) != "world" {
		t.Errorf("Get() = %q, want %q", string(got), "world")
	}
}

// testCacheMissingKey verifies Get returns ok=false for missing keys.
func testCacheMissingKey(t *testing.T, factory cacheFactory) {
	t.Helper()
	c := factory()
	defer c.Close()

	ctx := context.Background()
	_, ok, err := c.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ok {
		t.Fatal("Get() on missing key returned ok=true, want false")
	}
}

// testCacheDelete verifies Delete removes a key.
func testCacheDelete(t *testing.T, factory cacheFactory) {
	t.Helper()
	c := factory()
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "del", []byte("value"), time.Hour)
	_ = c.Delete(ctx, "del")

	_, ok, err := c.Get(ctx, "del")
	if err != nil {
		t.Fatalf("Get() after Delete error = %v", err)
	}
	if ok {
		t.Fatal("Get() after Delete returned ok=true, want false")
	}
}

// testCacheClear verifies Clear removes all entries.
func testCacheClear(t *testing.T, factory cacheFactory) {
	t.Helper()
	c := factory()
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "a", []byte("1"), time.Hour)
	_ = c.Set(ctx, "b", []byte("2"), time.Hour)
	_ = c.Clear(ctx)

	_, ok, _ := c.Get(ctx, "a")
	if ok {
		t.Error("Get('a') after Clear returned ok=true, want false")
	}
	_, ok, _ = c.Get(ctx, "b")
	if ok {
		t.Error("Get('b') after Clear returned ok=true, want false")
	}
}

// testCacheTTL verifies that entries expire after their TTL.
func testCacheTTL(t *testing.T, factory cacheFactory) {
	t.Helper()
	c := factory()
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "ttl", []byte("ephemeral"), 30*time.Millisecond)

	// Should exist before expiry
	_, ok, _ := c.Get(ctx, "ttl")
	if !ok {
		t.Fatal("Get() before TTL expiry returned ok=false, want true")
	}

	time.Sleep(80 * time.Millisecond)

	_, ok, _ = c.Get(ctx, "ttl")
	if ok {
		t.Fatal("Get() after TTL expiry returned ok=true, want false")
	}
}

// testCacheOverwrite verifies setting an existing key updates its value.
func testCacheOverwrite(t *testing.T, factory cacheFactory) {
	t.Helper()
	c := factory()
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "k", []byte("old"), time.Hour)
	_ = c.Set(ctx, "k", []byte("new"), time.Hour)

	got, ok, _ := c.Get(ctx, "k")
	if !ok {
		t.Fatal("Get() after overwrite returned ok=false, want true")
	}
	if string(got) != "new" {
		t.Errorf("Get() after overwrite = %q, want %q", string(got), "new")
	}
}

// TestMemoryCache_Common runs common cache tests against MemoryCache.
func TestMemoryCache_Common(t *testing.T) {
	factory := func() Cache {
		return NewMemoryCache(100)
	}

	t.Run("Roundtrip", func(t *testing.T) { testCacheRoundtrip(t, factory) })
	t.Run("MissingKey", func(t *testing.T) { testCacheMissingKey(t, factory) })
	t.Run("Delete", func(t *testing.T) { testCacheDelete(t, factory) })
	t.Run("Clear", func(t *testing.T) { testCacheClear(t, factory) })
	t.Run("TTL", func(t *testing.T) { testCacheTTL(t, factory) })
	t.Run("Overwrite", func(t *testing.T) { testCacheOverwrite(t, factory) })
}

// TestRedisCache_Common runs common stub behaviors against RedisCache.
// Since RedisCache returns ErrRedisNotImplemented for all operations,
// these tests verify the error behavior.
func TestRedisCache_Common(t *testing.T) {
	c, err := NewRedisCache("localhost:6379", 0, 5*time.Second)
	if err != nil {
		// Expected: stub returns an error on construction
		t.Logf("NewRedisCache returned expected error: %v", err)
		// We can still test the zero-value struct
		c = &RedisCache{addr: "localhost:6379", db: 0, timeout: 5 * time.Second}
	}
	defer c.Close()

	ctx := context.Background()

	t.Run("Get_ReturnsError", func(t *testing.T) {
		_, _, err := c.Get(ctx, "key")
		if err == nil {
			t.Error("Get() on RedisCache stub should return an error")
		}
	})

	t.Run("Set_ReturnsError", func(t *testing.T) {
		err := c.Set(ctx, "key", []byte("value"), time.Hour)
		if err == nil {
			t.Error("Set() on RedisCache stub should return an error")
		}
	})

	t.Run("Delete_ReturnsError", func(t *testing.T) {
		err := c.Delete(ctx, "key")
		if err == nil {
			t.Error("Delete() on RedisCache stub should return an error")
		}
	})

	t.Run("Clear_ReturnsError", func(t *testing.T) {
		err := c.Clear(ctx)
		if err == nil {
			t.Error("Clear() on RedisCache stub should return an error")
		}
	})

	t.Run("Close_OK", func(t *testing.T) {
		err := c.Close()
		if err != nil {
			t.Errorf("Close() error = %v, want nil", err)
		}
	})

	t.Run("Stats_ReturnsZero", func(t *testing.T) {
		s := c.Stats()
		if s.Hits != 0 || s.Misses != 0 || s.Sets != 0 || s.Deletes != 0 || s.Size != 0 {
			t.Errorf("Stats() on RedisCache stub should be zeroed, got %+v", s)
		}
	})
}
