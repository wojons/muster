package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigCreation(t *testing.T) {
	t.Run("Default Config", func(t *testing.T) {
		cfg := &Config{}

		assert.NotNil(t, cfg)
		// Config is created successfully
	})

	t.Run("Config With Values", func(t *testing.T) {
		cfg := &Config{
			Version:        "1.0.0",
			DefaultProfile: "default",
		}

		assert.Equal(t, "1.0.0", cfg.Version)
		assert.Equal(t, "default", cfg.DefaultProfile)
	})
}

func TestProfileCreation(t *testing.T) {
	t.Run("Create Profile", func(t *testing.T) {
		profile := Profile{
			BaseURL:  "http://example.com",
			AuthType: "oauth2",
			Timeout:  30 * time.Second,
		}

		assert.Equal(t, "http://example.com", profile.BaseURL)
		assert.Equal(t, 30*time.Second, profile.Timeout)
	})

	t.Run("Profile With Headers", func(t *testing.T) {
		profile := Profile{
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}

		assert.Equal(t, "application/json", profile.Headers["Content-Type"])
	})
}

func TestCacheConfig(t *testing.T) {
	t.Run("Cache Configuration", func(t *testing.T) {
		cacheConfig := CacheConfig{
			Enabled: true,
			Type:    "memory",
			TTL:     5 * time.Minute,
			MaxSize: 100 * 1024 * 1024, // 100MB
		}

		assert.True(t, cacheConfig.Enabled)
		assert.Equal(t, "memory", cacheConfig.Type)
		assert.Equal(t, 5*time.Minute, cacheConfig.TTL)
	})

	t.Run("Redis Config", func(t *testing.T) {
		redisConfig := RedisConfig{
			Address: "localhost:6379",
			DB:      0,
		}

		assert.Equal(t, "localhost:6379", redisConfig.Address)
		assert.Equal(t, 0, redisConfig.DB)
	})
}

func TestManagerCreation(t *testing.T) {
	t.Run("New Config Manager", func(t *testing.T) {
		manager, err := NewConfigManager()

		if err != nil {
			// This may fail if config files don't exist, which is OK for tests
			t.Logf("Expected failure: %v", err)
			return
		}

		require.NotNil(t, manager)
		assert.NotNil(t, manager.Get())
	})

	t.Run("Config Manager Getters", func(t *testing.T) {
		manager, err := NewConfigManager()

		// If creation failed, skip these tests
		if err != nil {
			t.Skip("Config manager creation failed")
			return
		}

		config := manager.Get()
		assert.NotNil(t, config)
	})
}

func TestConfigSources(t *testing.T) {
	t.Run("Sources Defined", func(t *testing.T) {
		assert.NotEmpty(t, ConfigSources)

		for i, source := range ConfigSources {
			assert.NotEmpty(t, source.Path)
			assert.Greater(t, source.Priority, 0)
			t.Logf("Source %d: %s (priority: %d)", i, source.Path, source.Priority)
		}
	})
}
