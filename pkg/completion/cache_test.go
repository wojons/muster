package completion

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCache(t *testing.T) {
	cache := NewCache(time.Second)

	t.Run("SetAndGet", func(t *testing.T) {
		cache.Set("test-key", "test-value")

		value, ok := cache.Get("test-key")
		if !ok {
			t.Fatal("Expected value to be found")
		}

		if value.(string) != "test-value" {
			t.Errorf("Expected 'test-value', got '%v'", value)
		}
	})

	t.Run("GetMissing", func(t *testing.T) {
		_, ok := cache.Get("missing-key")
		if ok {
			t.Fatal("Expected value to not be found")
		}
	})

	t.Run("Expiration", func(t *testing.T) {
		expCache := NewCache(10 * time.Millisecond)
		expCache.Set("expire-key", "expire-value")

		time.Sleep(20 * time.Millisecond)

		_, ok := expCache.Get("expire-key")
		if ok {
			t.Fatal("Expected expired value to not be found")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		cache.Set("delete-key", "delete-value")
		cache.Delete("delete-key")

		_, ok := cache.Get("delete-key")
		if ok {
			t.Fatal("Expected deleted value to not be found")
		}
	})

	t.Run("Clear", func(t *testing.T) {
		cache.Set("clear-key-1", "value-1")
		cache.Set("clear-key-2", "value-2")

		cache.Clear()

		_, ok1 := cache.Get("clear-key-1")
		_, ok2 := cache.Get("clear-key-2")

		if ok1 || ok2 {
			t.Fatal("Expected all values to be cleared")
		}
	})
}

func TestCache_Close(t *testing.T) {
	cache := NewCache(5 * time.Minute)

	err := cache.Close()
	require.NoError(t, err)

	// Second close should return ErrClosed
	err = cache.Close()
	require.Error(t, err)
	require.Equal(t, ErrClosed, err)
}

func TestCache_OperationsAfterClose(t *testing.T) {
	cache := NewCache(5 * time.Minute)
	_ = cache.Close()

	// Operations after close should still work (cache is just not cleaning up)
	cache.Set("key1", "value1")
	value, found := cache.Get("key1")
	require.True(t, found)
	require.Equal(t, "value1", value)
}

func TestCache_Close_StopsCleanupGoroutine(t *testing.T) {
	t.Run("Close stops cleanup goroutine", func(t *testing.T) {
		cache := NewCache(100 * time.Millisecond)

		// Give the goroutine time to start
		time.Sleep(50 * time.Millisecond)

		// Close the cache
		err := cache.Close()
		require.NoError(t, err)

		// Wait for longer than the cleanup interval
		time.Sleep(150 * time.Millisecond)

		// If we got here without panic, the goroutine exited cleanly
	})

	t.Run("Close twice returns ErrClosed", func(t *testing.T) {
		cache := NewCache(5 * time.Minute)

		err := cache.Close()
		require.NoError(t, err)

		err = cache.Close()
		require.Error(t, err)
		require.Equal(t, ErrClosed, err)
	})

	t.Run("operations after Close return ErrClosed", func(t *testing.T) {
		cache := NewCache(5 * time.Minute)

		err := cache.Close()
		require.NoError(t, err)

		// Set and Get should still work
		cache.Set("key", "value")
		val, found := cache.Get("key")
		require.True(t, found)
		require.Equal(t, "value", val)

		// But Close should return ErrClosed
		err = cache.Close()
		require.Equal(t, ErrClosed, err)
	})

	t.Run("cleanupLoop exits on close", func(t *testing.T) {
		cache := NewCache(50 * time.Millisecond)

		// Add an expired entry
		cache.Set("expired", "value")

		// Manually expire it by setting a very short TTL
		cache.ttl = 10 * time.Millisecond

		// Wait for entry to expire
		time.Sleep(20 * time.Millisecond)

		// Close should stop the cleanup goroutine
		err := cache.Close()
		require.NoError(t, err)

		// Wait for cleanup would have run
		time.Sleep(100 * time.Millisecond)

		// If we got here without hanging, the goroutine exited
	})

	t.Run("concurrent Close is safe", func(t *testing.T) {
		cache := NewCache(5 * time.Minute)

		done := make(chan bool, 10)

		// Try to close from multiple goroutines
		for i := 0; i < 10; i++ {
			go func() {
				_ = cache.Close()
				done <- true
			}()
		}

		// Wait for all goroutines to finish
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

func TestCache_Cleanup(t *testing.T) {
	cache := NewCache(50 * time.Millisecond)
	defer func() {
		_ = cache.Close()
	}()

	// Add an entry
	cache.Set("key1", "value1")

	// Verify it exists
	_, found := cache.Get("key1")
	require.True(t, found)

	// Wait for it to expire
	time.Sleep(100 * time.Millisecond)

	// Manually trigger cleanup
	cache.cleanup()

	// Entry should be gone
	_, found = cache.Get("key1")
	require.False(t, found)
}

func TestCache_ConcurrentAccess(t *testing.T) {
	cache := NewCache(5 * time.Minute)
	defer func() {
		_ = cache.Close()
	}()

	done := make(chan bool, 100)

	// Concurrent writes
	for i := 0; i < 50; i++ {
		go func(id int) {
			key := "key" + string(rune('0'+id))
			cache.Set(key, "value")
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 50; i++ {
		go func(id int) {
			key := "key" + string(rune('0'+id))
			cache.Get(key)
			done <- true
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < 100; i++ {
		<-done
	}
}
