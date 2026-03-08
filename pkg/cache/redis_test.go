package cache

import (
	"context"
	"testing"
	"time"
)

func TestRedisCache_Get(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis test in short mode")
	}

	ctx := context.Background()
	cache, err := NewRedisCache(RedisConfig{
		Addr:    "localhost:6379",
		DB:      1,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("Redis not available:", err)
	}
	defer func() { _ = cache.Close() }()

	value := Value{
		Data:      []byte("test-data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err = cache.Set(ctx, "test-key", value)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	retrieved, err := cache.Get(ctx, "test-key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(retrieved.Data) != "test-data" {
		t.Errorf("Get() Data = %v, want test-data", string(retrieved.Data))
	}
}

func TestRedisCache_Get_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis test in short mode")
	}

	ctx := context.Background()
	cache, err := NewRedisCache(RedisConfig{
		Addr:    "localhost:6379",
		DB:      1,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("Redis not available:", err)
	}
	defer func() { _ = cache.Close() }()

	_, err = cache.Get(ctx, "non-existent-key")
	if err != ErrNotFound {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestRedisCache_Set(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis test in short mode")
	}

	ctx := context.Background()
	cache, err := NewRedisCache(RedisConfig{
		Addr:    "localhost:6379",
		DB:      1,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("Redis not available:", err)
	}
	defer func() { _ = cache.Close() }()

	value := Value{
		Data:      []byte("test-data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	err = cache.Set(ctx, "test-key", value)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	retrieved, err := cache.Get(ctx, "test-key")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(retrieved.Data) != "test-data" {
		t.Errorf("Data mismatch")
	}
}

func TestRedisCache_Set_WithTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis test in short mode")
	}

	ctx := context.Background()
	cache, err := NewRedisCache(RedisConfig{
		Addr:    "localhost:6379",
		DB:      1,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("Redis not available:", err)
	}
	defer func() { _ = cache.Close() }()

	value := Value{
		Data:      []byte("test-data"),
		CreatedAt: time.Now(),
	}

	err = cache.Set(ctx, "test-key", value, WithTTL(100*time.Millisecond))
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	time.Sleep(150 * time.Millisecond)

	_, err = cache.Get(ctx, "test-key")
	if err != ErrNotFound {
		t.Errorf("Get() after TTL expired error = %v, want ErrNotFound", err)
	}
}

func TestRedisCache_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis test in short mode")
	}

	ctx := context.Background()
	cache, err := NewRedisCache(RedisConfig{
		Addr:    "localhost:6379",
		DB:      1,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("Redis not available:", err)
	}
	defer func() { _ = cache.Close() }()

	value := Value{
		Data:      []byte("test-data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = cache.Set(ctx, "test-key", value)

	err = cache.Delete(ctx, "test-key")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = cache.Get(ctx, "test-key")
	if err != ErrNotFound {
		t.Errorf("Get() after Delete error = %v, want ErrNotFound", err)
	}
}

func TestRedisCache_Has(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis test in short mode")
	}

	ctx := context.Background()
	cache, err := NewRedisCache(RedisConfig{
		Addr:    "localhost:6379",
		DB:      1,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("Redis not available:", err)
	}
	defer func() { _ = cache.Close() }()

	value := Value{
		Data:      []byte("test-data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = cache.Set(ctx, "test-key", value)

	has, err := cache.Has(ctx, "test-key")
	if err != nil {
		t.Fatalf("Has() error = %v", err)
	}
	if !has {
		t.Error("Has() = false, want true")
	}

	has, err = cache.Has(ctx, "non-existent")
	if err != nil {
		t.Fatalf("Has() error = %v", err)
	}
	if has {
		t.Error("Has() = true for non-existent key, want false")
	}
}

func TestRedisCache_Keys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis test in short mode")
	}

	ctx := context.Background()
	cache, err := NewRedisCache(RedisConfig{
		Addr:    "localhost:6379",
		DB:      1,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("Redis not available:", err)
	}
	defer func() { _ = cache.Close() }()

	value := Value{
		Data:      []byte("test-data"),
		CreatedAt: time.Now(),
		TTL:       time.Hour,
	}

	_ = cache.Set(ctx, "test-key-1", value)
	_ = cache.Set(ctx, "test-key-2", value)

	keys, err := cache.Keys(ctx, "test-key-*")
	if err != nil {
		t.Fatalf("Keys() error = %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("Keys() returned %d keys, want 2", len(keys))
	}
}

func TestRedisCache_Stats(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis test in short mode")
	}

	ctx := context.Background()
	cache, err := NewRedisCache(RedisConfig{
		Addr:    "localhost:6379",
		DB:      1,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("Redis not available:", err)
	}
	defer func() { _ = cache.Close() }()

	stats, err := cache.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}

	if stats.Entries < 0 {
		t.Errorf("Stats() Entries = %d, want >= 0", stats.Entries)
	}
}

func TestRedisCache_Serialize(t *testing.T) {
	cache := &RedisCache{
		prefix: "test:",
	}

	value := Value{
		Data:      []byte("test-data"),
		CreatedAt: time.Unix(1234567890, 0),
		TTL:       time.Hour,
		Tags:      []string{"tag1", "tag2"},
		Version:   42,
	}

	data, err := cache.serialize(value)
	if err != nil {
		t.Fatalf("serialize() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("serialize() returned empty data")
	}

	deserialized, err := cache.deserialize(data)
	if err != nil {
		t.Fatalf("deserialize() error = %v", err)
	}

	if string(deserialized.Data) != string(value.Data) {
		t.Error("Data mismatch after serialize/deserialize")
	}

	if !deserialized.CreatedAt.Equal(value.CreatedAt) {
		t.Error("CreatedAt mismatch")
	}

	if deserialized.TTL != value.TTL {
		t.Error("TTL mismatch")
	}

	if len(deserialized.Tags) != len(value.Tags) {
		t.Error("Tags length mismatch")
	}

	if deserialized.Version != value.Version {
		t.Error("Version mismatch")
	}
}

func TestRedisCache_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Redis test in short mode")
	}

	cache, err := NewRedisCache(RedisConfig{
		Addr:    "localhost:6379",
		DB:      1,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Skip("Redis not available:", err)
	}

	err = cache.Close()
	if err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestRedisCache_Prefix(t *testing.T) {
	cache := &RedisCache{
		prefix: "custom-prefix:",
	}

	if cache.prefix != "custom-prefix:" {
		t.Errorf("Prefix = %s, want custom-prefix:", cache.prefix)
	}
}

func TestRedisCache_Serialization_Empty(t *testing.T) {
	cache := &RedisCache{}

	data, err := cache.serialize(Value{})
	if err != nil {
		t.Fatalf("serialize() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("serialize() returned empty data for empty value")
	}

	_, err = cache.deserialize(data)
	if err != nil {
		t.Fatalf("deserialize() error = %v", err)
	}
}
