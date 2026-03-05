package completion

import (
	"testing"
	"time"
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
