package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMemoryCache_SetGet_Roundtrip(t *testing.T) {
	c := NewMemoryCache(100)
	defer c.Close()

	ctx := context.Background()
	key := "test-key"
	value := []byte("hello world")

	err := c.Set(ctx, key, value, time.Hour)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, ok, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() returned ok=false, want true")
	}
	if string(got) != string(value) {
		t.Errorf("Get() = %q, want %q", string(got), string(value))
	}
}

func TestMemoryCache_Get_MissingKey(t *testing.T) {
	c := NewMemoryCache(100)
	defer c.Close()

	ctx := context.Background()
	_, ok, err := c.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if ok {
		t.Fatal("Get() returned ok=true for missing key, want false")
	}
}

func TestMemoryCache_TTL_Expiry(t *testing.T) {
	c := NewMemoryCache(100)
	defer c.Close()

	ctx := context.Background()
	key := "ttl-key"
	value := []byte("ephemeral")

	err := c.Set(ctx, key, value, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Should be found immediately
	_, ok, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() before TTL expiry returned ok=false, want true")
	}

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	_, ok, err = c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() after TTL error = %v", err)
	}
	if ok {
		t.Fatal("Get() after TTL expiry returned ok=true, want false")
	}
}

func TestMemoryCache_TTL_Zero(t *testing.T) {
	c := NewMemoryCache(100)
	defer c.Close()

	ctx := context.Background()
	key := "no-ttl-key"
	value := []byte("persistent")

	// Zero TTL means no expiry
	err := c.Set(ctx, key, value, 0)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Should still be available after a short wait
	time.Sleep(50 * time.Millisecond)

	got, ok, err := c.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() with zero TTL returned ok=false after delay, want true")
	}
	if string(got) != string(value) {
		t.Errorf("Get() = %q, want %q", string(got), string(value))
	}
}

func TestMemoryCache_Delete(t *testing.T) {
	c := NewMemoryCache(100)
	defer c.Close()

	ctx := context.Background()
	key := "delete-key"
	value := []byte("to-be-deleted")

	_ = c.Set(ctx, key, value, time.Hour)

	// Verify it's there
	_, ok, _ := c.Get(ctx, key)
	if !ok {
		t.Fatal("Get() before Delete returned ok=false, want true")
	}

	err := c.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify it's gone
	_, ok, _ = c.Get(ctx, key)
	if ok {
		t.Fatal("Get() after Delete returned ok=true, want false")
	}
}

func TestMemoryCache_Delete_Missing(t *testing.T) {
	c := NewMemoryCache(100)
	defer c.Close()

	ctx := context.Background()
	// Deleting a non-existent key should not error
	err := c.Delete(ctx, "ghost-key")
	if err != nil {
		t.Fatalf("Delete() on missing key error = %v, want nil", err)
	}
}

func TestMemoryCache_Clear(t *testing.T) {
	c := NewMemoryCache(100)
	defer c.Close()

	ctx := context.Background()
	_ = c.Set(ctx, "a", []byte("1"), time.Hour)
	_ = c.Set(ctx, "b", []byte("2"), time.Hour)

	err := c.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	// Stats should be reset immediately after Clear
	s := c.Stats()
	if s.Sets != 0 || s.Hits != 0 || s.Misses != 0 {
		t.Errorf("Stats after Clear not reset: %+v", s)
	}

	_, ok, _ := c.Get(ctx, "a")
	if ok {
		t.Fatal("Get() after Clear returned ok=true for 'a', want false")
	}
	_, ok, _ = c.Get(ctx, "b")
	if ok {
		t.Fatal("Get() after Clear returned ok=true for 'b', want false")
	}
}

func TestMemoryCache_Overwrite(t *testing.T) {
	c := NewMemoryCache(100)
	defer c.Close()

	ctx := context.Background()
	key := "overwrite-key"

	_ = c.Set(ctx, key, []byte("original"), time.Hour)
	_ = c.Set(ctx, key, []byte("updated"), time.Hour)

	got, ok, _ := c.Get(ctx, key)
	if !ok {
		t.Fatal("Get() after overwrite returned ok=false, want true")
	}
	if string(got) != "updated" {
		t.Errorf("Get() after overwrite = %q, want %q", string(got), "updated")
	}
}

func TestMemoryCache_MaxSizeEviction(t *testing.T) {
	c := NewMemoryCache(3)
	defer c.Close()

	ctx := context.Background()
	// Insert 3 entries — cache should hold all 3
	_ = c.Set(ctx, "k1", []byte("v1"), time.Hour)
	_ = c.Set(ctx, "k2", []byte("v2"), time.Hour)
	_ = c.Set(ctx, "k3", []byte("v3"), time.Hour)

	s := c.Stats()
	if s.Size != 3 {
		t.Errorf("Stats().Size = %d, want 3", s.Size)
	}

	// Access k1 to make it recently used, then add one more
	_, _, _ = c.Get(ctx, "k1")
	_ = c.Set(ctx, "k4", []byte("v4"), time.Hour)

	// k2 should have been evicted (LRU), k1 should still exist
	_, ok2, _ := c.Get(ctx, "k2")
	if ok2 {
		t.Log("k2 was NOT evicted — LRU may not be working as expected")
	}

	// k1, k3, k4 should all be present (3 entries max)
	_, ok1, _ := c.Get(ctx, "k1")
	_, ok3, _ := c.Get(ctx, "k3")
	_, ok4, _ := c.Get(ctx, "k4")

	if !ok1 {
		t.Error("k1 missing after eviction (was recently accessed)")
	}
	if !ok3 {
		t.Error("k3 missing after eviction")
	}
	if !ok4 {
		t.Error("k4 missing after eviction (was just inserted)")
	}

	s2 := c.Stats()
	if s2.Size > 3 {
		t.Errorf("Stats().Size = %d after eviction, want <= 3", s2.Size)
	}
}

func TestMemoryCache_ConcurrentAccess(t *testing.T) {
	c := NewMemoryCache(1000)
	defer c.Close()

	ctx := context.Background()
	var wg sync.WaitGroup

	// Concurrent writers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n)
			val := []byte(fmt.Sprintf("value-%d", n))
			// Use a small random TTL to exercise expiry paths
			_ = c.Set(ctx, key, val, 500*time.Millisecond)
		}(i)
	}

	// Concurrent readers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n)
			_, _, _ = c.Get(ctx, key)
		}(i)
	}

	// Concurrent deleters
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n)
			_ = c.Delete(ctx, key)
		}(i)
	}

	wg.Wait()

	// Verify no panics and cache is still operational
	_, ok, err := c.Get(ctx, "key-19")
	if err != nil {
		t.Fatalf("Get() after concurrent access error = %v", err)
	}
	_ = ok // may or may not exist depending on timing — just check no error
}

func TestMemoryCache_Stats(t *testing.T) {
	c := NewMemoryCache(100)
	defer c.Close()

	ctx := context.Background()

	_ = c.Set(ctx, "a", []byte("1"), time.Hour)
	_ = c.Set(ctx, "b", []byte("2"), time.Hour)

	// Two hits
	_, _, _ = c.Get(ctx, "a")
	_, _, _ = c.Get(ctx, "b")

	// One miss
	_, _, _ = c.Get(ctx, "nonexistent")

	// One delete
	_ = c.Delete(ctx, "a")

	s := c.Stats()
	if s.Sets != 2 {
		t.Errorf("Stats.Sets = %d, want 2", s.Sets)
	}
	if s.Hits != 2 {
		t.Errorf("Stats.Hits = %d, want 2", s.Hits)
	}
	if s.Misses != 1 {
		t.Errorf("Stats.Misses = %d, want 1", s.Misses)
	}
	if s.Deletes != 1 {
		t.Errorf("Stats.Deletes = %d, want 1", s.Deletes)
	}
	if s.Size != 1 {
		t.Errorf("Stats.Size = %d, want 1", s.Size)
	}
}

func TestMemoryCache_Close(t *testing.T) {
	c := NewMemoryCache(100)
	err := c.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	// Double close should be safe
	err = c.Close()
	if err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestMemoryCache_NoMaxSize(t *testing.T) {
	c := NewMemoryCache(0)
	defer c.Close()

	ctx := context.Background()
	// With maxSize=0, there's no limit — insert many entries
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		_ = c.Set(ctx, key, []byte("data"), time.Hour)
	}

	s := c.Stats()
	if s.Size != 1000 {
		t.Errorf("Stats().Size = %d, want 1000", s.Size)
	}
}
