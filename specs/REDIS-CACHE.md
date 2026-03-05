# Redis Cache Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Caching

## Overview

Complete specification of **Redis cache** (L3) implementation for distributed caching.

## 1. Connection Management

```go
type RedisCache struct {
    client *redis.Client
    prefix string
}

func NewRedisCache(addr, password string, db int) (*RedisCache, error) {
    client := redis.NewClient(&redis.Options{
        Addr:     addr,
        Password: password,
        DB:       db,
    })
    
    if err := client.Ping(context.Background()).Err(); err != nil {
        return nil, fmt.Errorf("redis connection failed: %w", err)
    }
    
    return &RedisCache{
        client: client,
        prefix: "openapi-cli:",
    }, nil
}
```

## 2. Serialization

```go
func (c *RedisCache) serialize(value Value) ([]byte, error) {
    wrapper := struct {
        Data      []byte    `json:"data"`
        Metadata  Metadata  `json:"metadata"`
        CreatedAt time.Time `json:"created_at"`
        TTL       time.Duration `json:"ttl"`
    }{
        Data:      value.Data,
        Metadata:  value.Metadata,
        CreatedAt: value.CreatedAt,
        TTL:       value.TTL,
    }
    
    return json.Marshal(wrapper)
}

func (c *RedisCache) deserialize(data []byte) (Value, error) {
    var wrapper struct {
        Data      []byte    `json:"data"`
        Metadata  Metadata  `json:"metadata"`
        CreatedAt time.Time `json:"created_at"`
        TTL       time.Duration `json:"ttl"`
    }
    
    if err := json.Unmarshal(data, &wrapper); err != nil {
        return Value{}, err
    }
    
    return Value{
        Data:      wrapper.Data,
        Metadata:  wrapper.Metadata,
        CreatedAt: wrapper.CreatedAt,
        TTL:       wrapper.TTL,
    }, nil
}
```

## 3. Operations

```go
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

func (c *RedisCache) Set(ctx context.Context, key string, value Value, ttl time.Duration) error {
    data, err := c.serialize(value)
    if err != nil {
        return err
    }
    
    return c.client.Set(ctx, c.prefix+key, data, ttl).Err()
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
    return c.client.Del(ctx, c.prefix+key).Err()
}
```

## 4. Cluster Support

```go
type RedisClusterCache struct {
    client *redis.ClusterClient
    prefix string
}

func NewRedisClusterCache(addrs []string) (*RedisClusterCache, error) {
    client := redis.NewClusterClient(&redis.ClusterOptions{
        Addrs: addrs,
    })
    
    if err := client.Ping(context.Background()).Err(); err != nil {
        return nil, err
    }
    
    return &RedisClusterCache{
        client: client,
        prefix: "openapi-cli:",
    }, nil
}
```

## 5. Health Check

```go
func (c *RedisCache) HealthCheck(ctx context.Context) error {
    return c.client.Ping(ctx).Err()
}

func (c *RedisCache) Stats(ctx context.Context) (map[string]interface{}, error) {
    info := c.client.Info(ctx, "memory", "stats")
    return info.Result()
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
