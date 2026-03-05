package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client  *redis.Client
	prefix  string
	timeout time.Duration
}

type RedisClusterCache struct {
	client  *redis.ClusterClient
	prefix  string
	timeout time.Duration
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	Prefix   string
	Timeout  time.Duration
}

type RedisClusterConfig struct {
	Addrs    []string
	Password string
	Prefix   string
	Timeout  time.Duration
}

func NewRedisCache(config RedisConfig) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	prefix := config.Prefix
	if prefix == "" {
		prefix = "openapi-cli:"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &RedisCache{
		client:  client,
		prefix:  prefix,
		timeout: timeout,
	}, nil
}

func NewRedisClusterCache(config RedisClusterConfig) (*RedisClusterCache, error) {
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    config.Addrs,
		Password: config.Password,
	})

	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis cluster connection failed: %w", err)
	}

	prefix := config.Prefix
	if prefix == "" {
		prefix = "openapi-cli:"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &RedisClusterCache{
		client:  client,
		prefix:  prefix,
		timeout: timeout,
	}, nil
}

func (c *RedisCache) Get(ctx context.Context, key string) (Value, error) {
	data, err := c.client.Get(ctx, c.prefix+key).Bytes()
	if err == redis.Nil {
		return Value{}, ErrNotFound
	}
	if err != nil {
		return Value{}, err
	}

	return c.deserialize(data)
}

func (c *RedisCache) Set(ctx context.Context, key string, value Value, opts ...SetOption) error {
	data, err := c.serialize(value)
	if err != nil {
		return err
	}

	var options SetOptions
	for _, opt := range opts {
		opt(&options)
	}

	ttl := value.TTL
	if options.TTL > 0 {
		ttl = options.TTL
	}

	return c.client.Set(ctx, c.prefix+key, data, ttl).Err()
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, c.prefix+key).Err()
}

func (c *RedisCache) Clear(ctx context.Context) error {
	iter := c.client.Scan(ctx, 0, c.prefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		_ = c.client.Del(ctx, iter.Val()).Err()
	}
	return iter.Err()
}

func (c *RedisCache) Has(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, c.prefix+key).Result()
	return n > 0, err
}

func (c *RedisCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	keys, err := c.client.Keys(ctx, c.prefix+pattern).Result()

	pruned := make([]string, 0, len(keys))
	for _, key := range keys {
		pruned = append(pruned, key[len(c.prefix):])
	}

	return pruned, err
}

func (c *RedisCache) Stats(ctx context.Context) (Stats, error) {
	info, err := c.client.Info(ctx).Result()
	if err != nil {
		return Stats{}, err
	}

	dbSize, err := c.client.DBSize(ctx).Result()
	if err != nil {
		return Stats{}, err
	}

	return Stats{
		Entries: int(dbSize),
		Size:    int64(len(info)),
	}, nil
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

func (c *RedisCache) serialize(value Value) ([]byte, error) {
	wrapper := struct {
		Data      []byte        `json:"data"`
		CreatedAt time.Time     `json:"created_at"`
		TTL       time.Duration `json:"ttl"`
		Tags      []string      `json:"tags,omitempty"`
		Version   int64         `json:"version,omitempty"`
	}{
		Data:      value.Data,
		CreatedAt: value.CreatedAt,
		TTL:       value.TTL,
		Tags:      value.Tags,
		Version:   value.Version,
	}

	return json.Marshal(wrapper)
}

func (c *RedisCache) deserialize(data []byte) (Value, error) {
	var wrapper struct {
		Data      []byte        `json:"data"`
		CreatedAt time.Time     `json:"created_at"`
		TTL       time.Duration `json:"ttl"`
		Tags      []string      `json:"tags,omitempty"`
		Version   int64         `json:"version,omitempty"`
	}

	if err := json.Unmarshal(data, &wrapper); err != nil {
		return Value{}, err
	}

	return Value{
		Data:      wrapper.Data,
		CreatedAt: wrapper.CreatedAt,
		TTL:       wrapper.TTL,
		Tags:      wrapper.Tags,
		Version:   wrapper.Version,
	}, nil
}

func (c *RedisClusterCache) Get(ctx context.Context, key string) (Value, error) {
	data, err := c.client.Get(ctx, c.prefix+key).Bytes()
	if err == redis.Nil {
		return Value{}, ErrNotFound
	}
	if err != nil {
		return Value{}, err
	}

	return c.deserializeCluster(data)
}

func (c *RedisClusterCache) Set(ctx context.Context, key string, value Value, opts ...SetOption) error {
	data, err := c.serializeCluster(value)
	if err != nil {
		return err
	}

	var options SetOptions
	for _, opt := range opts {
		opt(&options)
	}

	ttl := value.TTL
	if options.TTL > 0 {
		ttl = options.TTL
	}

	return c.client.Set(ctx, c.prefix+key, data, ttl).Err()
}

func (c *RedisClusterCache) Delete(ctx context.Context, key string) error {
	return c.client.Del(ctx, c.prefix+key).Err()
}

func (c *RedisClusterCache) Clear(ctx context.Context) error {
	return fmt.Errorf("clear not implemented for cluster")
}

func (c *RedisClusterCache) Has(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, c.prefix+key).Result()
	return n > 0, err
}

func (c *RedisClusterCache) Keys(ctx context.Context, pattern string) ([]string, error) {
	keys, err := c.client.Keys(ctx, c.prefix+pattern).Result()

	pruned := make([]string, 0, len(keys))
	for _, key := range keys {
		pruned = append(pruned, key[len(c.prefix):])
	}

	return pruned, err
}

func (c *RedisClusterCache) Stats(ctx context.Context) (Stats, error) {
	return Stats{}, nil
}

func (c *RedisClusterCache) Close() error {
	return c.client.Close()
}

func (c *RedisClusterCache) serializeCluster(value Value) ([]byte, error) {
	wrapper := struct {
		Data      []byte        `json:"data"`
		CreatedAt time.Time     `json:"created_at"`
		TTL       time.Duration `json:"ttl"`
		Tags      []string      `json:"tags,omitempty"`
		Version   int64         `json:"version,omitempty"`
	}{
		Data:      value.Data,
		CreatedAt: value.CreatedAt,
		TTL:       value.TTL,
		Tags:      value.Tags,
		Version:   value.Version,
	}

	return json.Marshal(wrapper)
}

func (c *RedisClusterCache) deserializeCluster(data []byte) (Value, error) {
	var wrapper struct {
		Data      []byte        `json:"data"`
		CreatedAt time.Time     `json:"created_at"`
		TTL       time.Duration `json:"ttl"`
		Tags      []string      `json:"tags,omitempty"`
		Version   int64         `json:"version,omitempty"`
	}

	if err := json.Unmarshal(data, &wrapper); err != nil {
		return Value{}, err
	}

	return Value{
		Data:      wrapper.Data,
		CreatedAt: wrapper.CreatedAt,
		TTL:       wrapper.TTL,
		Tags:      wrapper.Tags,
		Version:   wrapper.Version,
	}, nil
}
