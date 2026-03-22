package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfileManager_SetProfile(t *testing.T) {
	// Create a temporary directory for test config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	// Create a test config file
	config := &Config{
		Version: "1.0.0",
		Profiles: map[string]Profile{
			"production": {
				BaseURL:  "https://api.production.com",
				AuthType: "oauth2",
				AuthConfig: map[string]string{
					"client_id": "prod-client",
				},
				DefaultFlags: map[string]string{
					"verbose": "true",
				},
				Headers: map[string]string{
					"X-Environment": "production",
				},
				Timeout: 60 * time.Second,
			},
			"staging": {
				BaseURL:  "https://api.staging.com",
				AuthType: "basic",
				AuthConfig: map[string]string{
					"username": "staging-user",
				},
				DefaultFlags: map[string]string{
					"verbose": "false",
				},
				Headers: map[string]string{
					"X-Environment": "staging",
				},
				Timeout: 30 * time.Second,
			},
		},
		ConfigDir: tempDir,
		CacheDir:  filepath.Join(tempDir, "cache"),
		DataDir:   filepath.Join(tempDir, "data"),
	}

	// Save the initial config
	cm := &ConfigManager{
		config:   config,
		filePath: configPath,
	}
	err := cm.save()
	require.NoError(t, err)

	pm := NewProfileManager(cm)

	t.Run("set existing profile", func(t *testing.T) {
		profile, err := pm.SetProfile("production")
		require.NoError(t, err)
		assert.NotNil(t, profile)
		assert.Equal(t, "https://api.production.com", profile.BaseURL)
		assert.Equal(t, "oauth2", profile.AuthType)
		assert.Equal(t, 60*time.Second, profile.Timeout)

		// Verify it was set as default
		defaultName := cm.GetDefaultProfile()
		assert.Equal(t, "production", defaultName)
	})

	t.Run("set another profile", func(t *testing.T) {
		profile, err := pm.SetProfile("staging")
		require.NoError(t, err)
		assert.NotNil(t, profile)
		assert.Equal(t, "https://api.staging.com", profile.BaseURL)
		assert.Equal(t, "basic", profile.AuthType)
		assert.Equal(t, 30*time.Second, profile.Timeout)

		// Verify it was set as default
		defaultName := cm.GetDefaultProfile()
		assert.Equal(t, "staging", defaultName)
	})

	t.Run("set non-existent profile", func(t *testing.T) {
		profile, err := pm.SetProfile("nonexistent")
		require.Error(t, err)
		assert.Nil(t, profile)
		assert.Contains(t, err.Error(), "nonexistent")
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("set empty profile name", func(t *testing.T) {
		profile, err := pm.SetProfile("")
		require.Error(t, err)
		assert.Nil(t, profile)
		assert.Contains(t, err.Error(), "cannot be empty")
	})
}

func TestProfileManager_SetProfile_Persistence(t *testing.T) {
	// Create a temporary directory for test config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	// Create initial config
	config := &Config{
		Version: "1.0.0",
		Profiles: map[string]Profile{
			"test": {
				BaseURL: "https://test.example.com",
				Timeout: 45 * time.Second,
			},
		},
		ConfigDir: tempDir,
		CacheDir:  filepath.Join(tempDir, "cache"),
		DataDir:   filepath.Join(tempDir, "data"),
	}

	cm := &ConfigManager{
		config:   config,
		filePath: configPath,
	}
	err := cm.save()
	require.NoError(t, err)

	pm := NewProfileManager(cm)

	// Set the profile
	profile, err := pm.SetProfile("test")
	require.NoError(t, err)
	assert.Equal(t, "https://test.example.com", profile.BaseURL)

	// Reload the config to verify persistence
	_, err = NewConfigManager()
	require.NoError(t, err)

	// The new manager should load from the default location, so we need to
	// verify the file was written correctly
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "defaultprofile: test")
}

func TestProfileManager_SetProfile_Concurrent(t *testing.T) {
	// Create a temporary directory for test config
	tempDir := t.TempDir()

	config := &Config{
		Version: "1.0.0",
		Profiles: map[string]Profile{
			"profile1": {BaseURL: "https://api1.example.com"},
			"profile2": {BaseURL: "https://api2.example.com"},
			"profile3": {BaseURL: "https://api3.example.com"},
		},
		ConfigDir: tempDir,
		CacheDir:  filepath.Join(tempDir, "cache"),
		DataDir:   filepath.Join(tempDir, "data"),
	}

	cm := &ConfigManager{
		config:   config,
		filePath: filepath.Join(tempDir, "config.yaml"),
	}
	err := cm.save()
	require.NoError(t, err)

	pm := NewProfileManager(cm)

	// Run concurrent SetProfile calls
	done := make(chan bool, 3)

	go func() {
		_, err := pm.SetProfile("profile1")
		assert.NoError(t, err)
		done <- true
	}()

	go func() {
		_, err := pm.SetProfile("profile2")
		assert.NoError(t, err)
		done <- true
	}()

	go func() {
		_, err := pm.SetProfile("profile3")
		assert.NoError(t, err)
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify one of them succeeded as default (last write wins)
	defaultProfile := cm.GetDefaultProfile()
	assert.Contains(t, []string{"profile1", "profile2", "profile3"}, defaultProfile)
}

func TestProfileManager_SetProfile_ReturnsCorrectProfile(t *testing.T) {
	tempDir := t.TempDir()

	config := &Config{
		Version: "1.0.0",
		Profiles: map[string]Profile{
			"complex": {
				BaseURL:      "https://complex.example.com",
				AuthType:     "apikey",
				AuthConfig:   map[string]string{"key": "secret123", "header": "X-API-Key"},
				DefaultFlags: map[string]string{"output": "json", "verbose": "true"},
				Headers:      map[string]string{"User-Agent": "test-client", "Accept": "application/json"},
				Timeout:      120 * time.Second,
			},
		},
		ConfigDir: tempDir,
		CacheDir:  filepath.Join(tempDir, "cache"),
		DataDir:   filepath.Join(tempDir, "data"),
	}

	cm := &ConfigManager{
		config:   config,
		filePath: filepath.Join(tempDir, "config.yaml"),
	}
	err := cm.save()
	require.NoError(t, err)

	pm := NewProfileManager(cm)

	profile, err := pm.SetProfile("complex")
	require.NoError(t, err)

	// Verify all fields are returned correctly
	assert.Equal(t, "https://complex.example.com", profile.BaseURL)
	assert.Equal(t, "apikey", profile.AuthType)
	assert.Equal(t, "secret123", profile.AuthConfig["key"])
	assert.Equal(t, "X-API-Key", profile.AuthConfig["header"])
	assert.Equal(t, "json", profile.DefaultFlags["output"])
	assert.Equal(t, "true", profile.DefaultFlags["verbose"])
	assert.Equal(t, "test-client", profile.Headers["User-Agent"])
	assert.Equal(t, "application/json", profile.Headers["Accept"])
	assert.Equal(t, 120*time.Second, profile.Timeout)
}
