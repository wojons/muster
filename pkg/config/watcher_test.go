package config

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewWatcher verifies that NewWatcher creates a properly initialized Watcher.
func TestNewWatcher(t *testing.T) {
	t.Run("returns non-nil watcher", func(t *testing.T) {
		v := viper.New()
		w := NewWatcher(v, nil)
		require.NotNil(t, w)
	})

	t.Run("stores viper instance", func(t *testing.T) {
		v := viper.New()
		w := NewWatcher(v, nil)
		require.NotNil(t, w)
		assert.Equal(t, v, w.v)
	})

	t.Run("stores onChange callback", func(t *testing.T) {
		called := false
		cb := func() { called = true }
		w := NewWatcher(viper.New(), cb)
		require.NotNil(t, w)
		// Verify the callback is stored by calling it indirectly
		assert.NotNil(t, w.onChange)
		w.onChange()
		assert.True(t, called)
	})

	t.Run("allows nil onChange callback", func(t *testing.T) {
		w := NewWatcher(viper.New(), nil)
		require.NotNil(t, w)
		assert.Nil(t, w.onChange)
	})

	t.Run("initialises stop channel", func(t *testing.T) {
		w := NewWatcher(viper.New(), nil)
		require.NotNil(t, w)
		assert.NotNil(t, w.stopCh)
	})

	t.Run("starts as not watching", func(t *testing.T) {
		w := NewWatcher(viper.New(), nil)
		assert.False(t, w.watching)
	})
}

// TestWatcher_IsWatching verifies the IsWatching method behaviour.
func TestWatcher_IsWatching(t *testing.T) {
	t.Run("returns false before WatchConfig is called", func(t *testing.T) {
		w := NewWatcher(viper.New(), nil)
		assert.False(t, w.IsWatching())
	})

	t.Run("returns false after Stop", func(t *testing.T) {
		w := NewWatcher(viper.New(), nil)
		w.watching = true // simulate started state
		w.Stop()
		assert.False(t, w.IsWatching())
	})

	t.Run("multiple calls are consistent", func(t *testing.T) {
		w := NewWatcher(viper.New(), nil)
		assert.False(t, w.IsWatching())
		assert.False(t, w.IsWatching())
	})
}

// TestWatcher_Stop verifies Stop behaviour without starting the watcher.
func TestWatcher_Stop(t *testing.T) {
	t.Run("stop when not watching is safe", func(t *testing.T) {
		w := NewWatcher(viper.New(), nil)
		// Should not panic
		w.Stop()
		assert.False(t, w.IsWatching())
	})

	t.Run("stop multiple times is safe", func(t *testing.T) {
		w := NewWatcher(viper.New(), nil)
		w.Stop()
		w.Stop()
		w.Stop()
		assert.False(t, w.IsWatching())
	})

	t.Run("stop after simulated start sets watching to false", func(t *testing.T) {
		w := NewWatcher(viper.New(), nil)
		w.watching = true
		w.Stop()
		assert.False(t, w.watching)
	})
}

// TestWatcher_NewWatcher_NilViper verifies that constructing with nil viper
// produces a usable struct (the resulting watcher will fail at runtime, but
// construction itself should not panic).
func TestWatcher_NewWatcher_NilViper(t *testing.T) {
	w := NewWatcher(nil, nil)
	require.NotNil(t, w)
	assert.Nil(t, w.v)
	assert.False(t, w.IsWatching())
	// Stop should not panic even with nil viper
	w.Stop()
}
