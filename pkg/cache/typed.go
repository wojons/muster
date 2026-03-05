package cache

import (
	"context"
	"encoding/json"
	"fmt"
)

type TypedCache[T any] struct {
	cache Cache
}

func NewTyped[T any](cache Cache) *TypedCache[T] {
	return &TypedCache[T]{cache: cache}
}

func (tc *TypedCache[T]) Get(ctx context.Context, key string) (T, error) {
	var zero T

	val, err := tc.cache.Get(ctx, key)
	if err != nil {
		return zero, err
	}

	var result T
	if err := json.Unmarshal(val.Data, &result); err != nil {
		return zero, fmt.Errorf("cache: failed to unmarshal: %w", err)
	}

	return result, nil
}

func (tc *TypedCache[T]) Set(ctx context.Context, key string, value T, opts ...SetOption) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache: failed to marshal: %w", err)
	}

	return tc.cache.Set(ctx, key, Value{Data: data}, opts...)
}

func (tc *TypedCache[T]) Delete(ctx context.Context, key string) error {
	return tc.cache.Delete(ctx, key)
}

func (tc *TypedCache[T]) Has(ctx context.Context, key string) (bool, error) {
	return tc.cache.Has(ctx, key)
}

func (tc *TypedCache[T]) Close() error {
	return tc.cache.Close()
}
