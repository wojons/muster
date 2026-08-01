package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTieredCache_L1Hit(t *testing.T) {
	ctx := context.Background()
	c := NewTieredCache(64, NewMemoryCache(64))
	defer c.Close()

	require.NoError(t, c.Set(ctx, "a", []byte("v1"), time.Minute))

	v, ok, err := c.Get(ctx, "a")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "v1", string(v))
	assert.Equal(t, uint64(1), c.stats.l1Hits.Load())
	assert.Equal(t, uint64(0), c.stats.l2Hits.Load())
}

func TestTieredCache_L2Backfill(t *testing.T) {
	ctx := context.Background()
	l2 := NewMemoryCache(64)
	c := NewTieredCache(8, l2)

	// L2-only write (simulate a cold L1 after restart).
	require.NoError(t, l2.Set(ctx, "cold", []byte("warm"), time.Minute))

	v, ok, err := c.Get(ctx, "cold")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "warm", string(v))
	assert.Equal(t, uint64(1), c.stats.l2Hits.Load())
	assert.Equal(t, uint64(1), c.stats.backfill.Load())

	// Second read hits L1 now.
	_, ok, _ = c.Get(ctx, "cold")
	assert.True(t, ok)
	assert.Equal(t, uint64(1), c.stats.l1Hits.Load())
}

func TestTieredCache_Miss(t *testing.T) {
	ctx := context.Background()
	c := NewTieredCache(8, NewMemoryCache(8))
	_, ok, err := c.Get(ctx, "nope")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, uint64(1), c.stats.misses.Load())
}

func TestTieredCache_DeleteBothLevels(t *testing.T) {
	ctx := context.Background()
	l2 := NewMemoryCache(8)
	c := NewTieredCache(8, l2)

	require.NoError(t, c.Set(ctx, "k", []byte("x"), time.Minute))
	require.NoError(t, c.Delete(ctx, "k"))

	_, ok1, _ := c.l1.Get(ctx, "k")
	_, ok2, _ := l2.Get(ctx, "k")
	assert.False(t, ok1, "L1 entry should be deleted")
	assert.False(t, ok2, "L2 entry should be deleted")
}

func TestCacheManager_RegisterAndGet(t *testing.T) {
	m := NewCacheManager()
	l1 := NewMemoryCache(16)
	m.Register("default", l1)
	m.Register("session", NewMemoryCache(16))

	assert.Equal(t, []string{"default", "session"}, m.Names())
	assert.Same(t, l1, m.Get("default"))
	assert.Nil(t, m.Get("missing"))
}

func TestCacheManager_HitRate(t *testing.T) {
	ctx := context.Background()
	m := NewCacheManager()
	inner := NewMemoryCache(16)
	m.Register("main", inner)

	require.NoError(t, inner.Set(ctx, "k", []byte("v"), time.Minute))
	_, ok, _ := inner.Get(ctx, "k") // hit
	assert.True(t, ok)
	_, _, _ = inner.Get(ctx, "nope") // miss

	s := inner.Stats()
	assert.Equal(t, int64(1), s.Hits)
	assert.Equal(t, int64(1), s.Misses)
}

func TestTypedCache_RoundTrip(t *testing.T) {
	ctx := context.Background()
	type User struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	inner := NewMemoryCache(16)
	tc := NewTypedCache[User](inner,
		func(u User) ([]byte, error) { return json.Marshal(u) },
		func(b []byte) (User, error) {
			var u User
			err := json.Unmarshal(b, &u)
			return u, err
		},
	)

	require.NoError(t, tc.Set(ctx, "u1", User{ID: 7, Name: "kara"}, time.Minute))

	got, ok, err := tc.Get(ctx, "u1")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, User{ID: 7, Name: "kara"}, got)
}

func ExampleTieredCache() {
	ctx := context.Background()
	l2 := NewMemoryCache(1024)
	c := NewTieredCache(64, l2)
	defer c.Close()

	_ = c.Set(ctx, "greeting", []byte("hello"), time.Minute)
	v, ok, _ := c.Get(ctx, "greeting")
	fmt.Printf("ok=%v value=%s", ok, string(v))
	// Output: ok=true value=hello
}
