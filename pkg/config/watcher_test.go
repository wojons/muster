package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWatcher(t *testing.T) {
	v := viper.New()
	onChange := func() {}

	w := NewWatcher(v, onChange)

	assert.NotNil(t, w)
	assert.NotNil(t, w.v)
	assert.NotNil(t, w.onChange)
	assert.NotNil(t, w.stopCh)
	assert.False(t, w.watching)
}

func TestWatcher_WatchConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	initialContent := `
version: "1.0.0"
default_profile: "default"
`
	err := os.WriteFile(configFile, []byte(initialContent), 0644)
	require.NoError(t, err)

	// Setup viper
	v := viper.New()
	v.SetConfigFile(configFile)
	err = v.ReadInConfig()
	require.NoError(t, err)

	// Setup watcher with callback
	changeCalled := make(chan bool, 1)
	onChange := func() {
		changeCalled <- true
	}

	w := NewWatcher(v, onChange)
	require.NotNil(t, w)

	// Start watching
	err = w.WatchConfig()
	require.NoError(t, err)
	assert.True(t, w.IsWatching())

	// Modify the config file
	updatedContent := `
version: "1.1.0"
default_profile: "production"
`
	err = os.WriteFile(configFile, []byte(updatedContent), 0644)
	require.NoError(t, err)

	// Wait for the change to be detected (with timeout)
	select {
	case <-changeCalled:
		// Success - callback was invoked
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for config change callback")
	}

	// Verify the new config was loaded
	assert.Equal(t, "1.1.0", v.GetString("version"))
	assert.Equal(t, "production", v.GetString("default_profile"))
}

func TestWatcher_WatchConfig_AlreadyWatching(t *testing.T) {
	v := viper.New()
	w := NewWatcher(v, func() {})

	// First call should succeed
	err := w.WatchConfig()
	require.NoError(t, err)
	assert.True(t, w.IsWatching())

	// Second call should be a no-op
	err = w.WatchConfig()
	require.NoError(t, err) // Should not error
	assert.True(t, w.IsWatching())
}

func TestWatcher_Stop(t *testing.T) {
	v := viper.New()
	w := NewWatcher(v, func() {})

	// Stop before starting should not panic
	w.Stop()
	assert.False(t, w.IsWatching())

	// Start watching
	err := w.WatchConfig()
	require.NoError(t, err)
	assert.True(t, w.IsWatching())

	// Stop should work
	w.Stop()
	assert.False(t, w.IsWatching())

	// Double stop should not panic
	w.Stop()
	assert.False(t, w.IsWatching())
}

func TestWatcher_IsWatching(t *testing.T) {
	v := viper.New()
	w := NewWatcher(v, func() {})

	assert.False(t, w.IsWatching())

	err := w.WatchConfig()
	require.NoError(t, err)
	assert.True(t, w.IsWatching())

	w.Stop()
	assert.False(t, w.IsWatching())
}

func TestWatcher_GetConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `
version: "2.0.0"
default_profile: "staging"
profiles:
  staging:
    base_url: "https://api.staging.example.com"
    timeout: 30s
`
	err := os.WriteFile(configFile, []byte(content), 0644)
	require.NoError(t, err)

	// Setup viper
	v := viper.New()
	v.SetConfigFile(configFile)
	err = v.ReadInConfig()
	require.NoError(t, err)

	w := NewWatcher(v, func() {})

	// Get config
	cfg, err := w.GetConfig()
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "2.0.0", cfg.Version)
	assert.Equal(t, "staging", cfg.DefaultProfile)
	assert.Len(t, cfg.Profiles, 1)
}

func TestWatcher_GetConfig_UnmarshalError(t *testing.T) {
	v := viper.New()
	// Set an invalid value that can't be unmarshaled into Config
	v.Set("cache.ttl", "not-a-duration")

	w := NewWatcher(v, func() {})

	cfg, err := w.GetConfig()
	assert.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "unable to decode config")
}

func TestWatcher_ConcurrentAccess(t *testing.T) {
	v := viper.New()
	w := NewWatcher(v, func() {})

	// Start watching
	err := w.WatchConfig()
	require.NoError(t, err)

	// Concurrent calls to IsWatching
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			_ = w.IsWatching()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(1 * time.Second):
			t.Fatal("Timeout waiting for concurrent access")
		}
	}
}

func TestWatcher_MultipleFileChanges(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	initialContent := `version: "1.0.0"`
	err := os.WriteFile(configFile, []byte(initialContent), 0644)
	require.NoError(t, err)

	// Setup viper
	v := viper.New()
	v.SetConfigFile(configFile)
	err = v.ReadInConfig()
	require.NoError(t, err)

	// Count changes
	changeCount := 0
	changeCalled := make(chan bool, 3)
	onChange := func() {
		changeCount++
		changeCalled <- true
	}

	w := NewWatcher(v, onChange)
	err = w.WatchConfig()
	require.NoError(t, err)

	// Make multiple changes
	for i := 1; i <= 3; i++ {
		content := fmt.Sprintf(`version: "1.%d.0"`, i)
		err = os.WriteFile(configFile, []byte(content), 0644)
		require.NoError(t, err)

		// Small delay to ensure fsnotify catches the change
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for changes to be detected
	time.Sleep(500 * time.Millisecond)

	// Should have detected at least some changes
	// Note: fsnotify might batch rapid changes, so we just verify it works
	assert.GreaterOrEqual(t, changeCount, 1, "Should detect at least one change")
}
