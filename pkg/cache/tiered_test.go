package cache

import (
	"context"
	"testing"
	"time"
)

func TestTieredCache_GetL1Hit(t *testing.T) {
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	tc := NewTieredCache(TieredCacheConfig{
		L1: l1,
	})

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = tc.Set(ctx, "key1", value)

	retrieved, err := tc.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Failed to get from L1: %v", err)
	}

	if string(retrieved.Data) != "test data" {
		t.Errorf("Expected 'test data', got '%s'", string(retrieved.Data))
	}

	_ = tc.Close()
}

func TestTieredCache_GetL2Backfill(t *testing.T) {
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	l2 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	tc := NewTieredCache(TieredCacheConfig{
		L1: l1,
		L2: l2,
	})

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = l2.Set(ctx, "key1", value)

	retrieved, err := tc.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Failed to get from L2: %v", err)
	}

	if string(retrieved.Data) != "test data" {
		t.Errorf("Expected 'test data', got '%s'", string(retrieved.Data))
	}

	_, err = l1.Get(ctx, "key1")
	if err != nil {
		t.Errorf("Expected value to be backfilled to L1: %v", err)
	}

	_ = tc.Close()
}

func TestTieredCache_SetAllTiers(t *testing.T) {
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	l2 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	l3 := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	tc := NewTieredCache(TieredCacheConfig{
		L1: l1,
		L2: l2,
		L3: l3,
	})

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err := tc.Set(ctx, "key1", value)
	if err != nil {
		t.Fatalf("Failed to set: %v", err)
	}

	_, err = l1.Get(ctx, "key1")
	if err != nil {
		t.Errorf("L1 should have value: %v", err)
	}

	_, err = l2.Get(ctx, "key1")
	if err != nil {
		t.Errorf("L2 should have value: %v", err)
	}

	_, err = l3.Get(ctx, "key1")
	if err != nil {
		t.Errorf("L3 should have value: %v", err)
	}

	_ = tc.Close()
}

func TestCacheManager_BasicOperation(t *testing.T) {
	cm := NewCacheManager()
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())

	cm.AddTier("memory", l1, 1, 10*1024*1024)
	cm.SetPolicy("default", time.Hour, []string{"memory"}, 1)

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err := cm.Set(ctx, "key1", value, "default")
	if err != nil {
		t.Fatalf("Failed to set: %v", err)
	}

	retrieved, err := cm.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Failed to get: %v", err)
	}

	if string(retrieved.Data) != "test data" {
		t.Errorf("Expected 'test data', got '%s'", string(retrieved.Data))
	}

	_ = cm.Close()
}

func TestCacheManager_TieredAccess(t *testing.T) {
	cm := NewCacheManager()
	l1 := NewMemoryCache(1024, NewLRUPolicy())
	l2 := NewMemoryCache(10*1024*1024, NewLRUPolicy())

	cm.AddTier("memory", l1, 1, 1024)
	cm.AddTier("disk", l2, 2, 10*1024*1024)
	cm.SetPolicy("default", time.Hour, []string{"disk"}, 1)

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = cm.Set(ctx, "key1", value, "default")
	_ = l2.Set(ctx, "key2", value)

	retrieved, err := cm.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Failed to get from manager: %v", err)
	}

	if string(retrieved.Data) != "test data" {
		t.Errorf("Expected 'test data', got '%s'", string(retrieved.Data))
	}

	_ = cm.Close()
}

func TestCacheManager_HitRate(t *testing.T) {
	cm := NewCacheManager()
	l1 := NewMemoryCache(10*1024*1024, NewLRUPolicy())

	cm.AddTier("memory", l1, 1, 10*1024*1024)
	cm.SetPolicy("default", time.Hour, []string{"memory"}, 1)

	ctx := context.Background()
	value := Value{
		Data:      []byte("test data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = cm.Set(ctx, "key1", value, "default")

	_, _ = cm.Get(ctx, "key1")
	_, _ = cm.Get(ctx, "key2")

	hitRate := cm.GetHitRate()
	if hitRate < 49 || hitRate > 51 {
		t.Errorf("Expected hit rate ~50%%, got %f", hitRate)
	}

	_ = cm.Close()
}

func TestTypedCache_Basic(t *testing.T) {
	type TestStruct struct {
		Name  string
		Value int
	}

	mc := NewMemoryCache(10*1024*1024, NewLRUPolicy())
	tc := NewTyped[TestStruct](mc)

	ctx := context.Background()
	testValue := TestStruct{Name: "test", Value: 42}

	err := tc.Set(ctx, "key1", testValue)
	if err != nil {
		t.Fatalf("Failed to set typed value: %v", err)
	}

	retrieved, err := tc.Get(ctx, "key1")
	if err != nil {
		t.Fatalf("Failed to get typed value: %v", err)
	}

	if retrieved.Name != "test" || retrieved.Value != 42 {
		t.Errorf("Expected {Name: test, Value: 42}, got {Name: %s, Value: %d}", retrieved.Name, retrieved.Value)
	}

	_ = tc.Close()
}
