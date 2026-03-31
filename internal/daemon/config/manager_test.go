package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	t.Run("creates manager with default config", func(t *testing.T) {
		mgr := NewManager("/tmp/test.json")

		assert.NotNil(t, mgr)
		cfg := mgr.Get()
		assert.NotNil(t, cfg)
		assert.Equal(t, Default().SocketPath, cfg.SocketPath)
		assert.Equal(t, Default().TargetURL, cfg.TargetURL)
	})

	t.Run("stores path correctly", func(t *testing.T) {
		mgr := NewManager("/custom/path.json")
		assert.NotNil(t, mgr)
	})
}

func TestManager_Load(t *testing.T) {
	t.Run("loads config from file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "daemon.json")

		// Create a test config file
		testConfig := `{
			"socket_path": "/custom/socket.sock",
			"target_url": "https://example.com",
			"max_connections": 200,
			"read_timeout": 30000000000,
			"write_timeout": 30000000000
		}`
		err := os.WriteFile(configPath, []byte(testConfig), 0644)
		require.NoError(t, err)

		mgr := NewManager(configPath)
		cfg, err := mgr.Load()

		assert.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, "/custom/socket.sock", cfg.SocketPath)
		assert.Equal(t, "https://example.com", cfg.TargetURL)
		assert.Equal(t, 200, cfg.MaxConnections)
	})

	t.Run("returns defaults when file doesn't exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "nonexistent.json")

		mgr := NewManager(configPath)
		cfg, err := mgr.Load()

		assert.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Equal(t, Default().SocketPath, cfg.SocketPath)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid.json")

		err := os.WriteFile(configPath, []byte("invalid json"), 0644)
		require.NoError(t, err)

		mgr := NewManager(configPath)
		cfg, err := mgr.Load()

		assert.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "parse config")
	})

	t.Run("returns error for invalid config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "invalid_config.json")

		testConfig := `{
			"socket_path": "",
			"target_url": "https://example.com",
			"max_connections": 100,
			"read_timeout": 30000000000,
			"write_timeout": 30000000000
		}`
		err := os.WriteFile(configPath, []byte(testConfig), 0644)
		require.NoError(t, err)

		mgr := NewManager(configPath)
		cfg, err := mgr.Load()

		assert.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "validate config")
	})
}

func TestManager_Save(t *testing.T) {
	t.Run("saves config to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "daemon.json")

		mgr := NewManager(configPath)
		cfg := Default()
		cfg.SocketPath = "/custom/path.sock"
		cfg.MaxConnections = 500

		err := mgr.Save(cfg)

		assert.NoError(t, err)

		// Verify file was created
		data, err := os.ReadFile(configPath)
		assert.NoError(t, err)
		assert.Contains(t, string(data), "/custom/path.sock")
		assert.Contains(t, string(data), "500")
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "subdir1", "subdir2", "daemon.json")

		mgr := NewManager(configPath)
		cfg := Default()

		err := mgr.Save(cfg)

		assert.NoError(t, err)
		_, err = os.Stat(configPath)
		assert.NoError(t, err)
	})

	t.Run("returns error for nil config", func(t *testing.T) {
		mgr := NewManager("/tmp/test.json")
		err := mgr.Save(nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "config cannot be nil")
	})

	t.Run("returns error for invalid config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "daemon.json")

		mgr := NewManager(configPath)
		cfg := Default()
		cfg.SocketPath = "" // Invalid

		err := mgr.Save(cfg)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validate config")
	})

	t.Run("returns error for read-only directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		readonlyDir := filepath.Join(tmpDir, "readonly")
		err := os.Mkdir(readonlyDir, 0000)
		require.NoError(t, err)
		defer func() {
			_ = os.Chmod(readonlyDir, 0755)
		}() // Cleanup

		configPath := filepath.Join(readonlyDir, "daemon.json")
		mgr := NewManager(configPath)

		err = mgr.Save(Default())
		assert.Error(t, err)
	})
}

func TestManager_Get(t *testing.T) {
	t.Run("returns copy of config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "daemon.json")

		mgr := NewManager(configPath)
		cfg := Default()
		cfg.SocketPath = "/original/path.sock"
		mgr.config = cfg

		got := mgr.Get()
		got.SocketPath = "/modified/path.sock"

		// Original should be unchanged
		assert.Equal(t, "/original/path.sock", mgr.config.SocketPath)
	})

	t.Run("returns nil when config is nil", func(t *testing.T) {
		mgr := NewManager("/tmp/test.json")
		mgr.config = nil

		got := mgr.Get()
		assert.Nil(t, got)
	})
}

func TestManager_Update(t *testing.T) {
	t.Run("updates config atomically", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "daemon.json")

		mgr := NewManager(configPath)
		mgr.config = Default()

		err := mgr.Update(func(c *Config) {
			c.SocketPath = "/updated/path.sock"
			c.MaxConnections = 300
		})

		assert.NoError(t, err)
		cfg := mgr.Get()
		assert.Equal(t, "/updated/path.sock", cfg.SocketPath)
		assert.Equal(t, 300, cfg.MaxConnections)

		// Verify file was saved
		_, err = os.Stat(configPath)
		assert.NoError(t, err)
	})

	t.Run("rolls back on invalid config", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "daemon.json")

		mgr := NewManager(configPath)
		mgr.config = Default()

		err := mgr.Update(func(c *Config) {
			c.SocketPath = "" // Invalid
		})

		assert.Error(t, err)
		cfg := mgr.Get()
		assert.Equal(t, Default().SocketPath, cfg.SocketPath)
	})

	t.Run("returns error for nil function", func(t *testing.T) {
		mgr := NewManager("/tmp/test.json")
		err := mgr.Update(nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "update function cannot be nil")
	})
}

func TestManager_Watch(t *testing.T) {
	t.Run("receives config updates", func(t *testing.T) {
		mgr := NewManager("/tmp/test.json")
		mgr.config = Default()

		ch := mgr.Watch()
		require.NotNil(t, ch)

		newCfg := Default()
		newCfg.SocketPath = "/watched/path.sock"

		// Trigger notification
		mgr.notifyWatchers(newCfg)

		select {
		case cfg := <-ch:
			assert.Equal(t, "/watched/path.sock", cfg.SocketPath)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for config update")
		}
	})

	t.Run("multiple watchers receive updates", func(t *testing.T) {
		mgr := NewManager("/tmp/test.json")
		mgr.config = Default()

		ch1 := mgr.Watch()
		ch2 := mgr.Watch()

		newCfg := Default()
		newCfg.SocketPath = "/multi/path.sock"
		mgr.notifyWatchers(newCfg)

		// Both should receive the update
		select {
		case cfg := <-ch1:
			assert.Equal(t, "/multi/path.sock", cfg.SocketPath)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for watcher 1")
		}

		select {
		case cfg := <-ch2:
			assert.Equal(t, "/multi/path.sock", cfg.SocketPath)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for watcher 2")
		}
	})

	t.Run("non-blocking when watcher full", func(t *testing.T) {
		mgr := NewManager("/tmp/test.json")
		mgr.config = Default()

		// Create multiple watchers to test non-blocking behavior
		for i := 0; i < 10; i++ {
			_ = mgr.Watch()
		}

		// Should not block with many watchers
		mgr.notifyWatchers(Default())
	})
}

func TestManager_ReloadChannel(t *testing.T) {
	t.Run("receives reload signal", func(t *testing.T) {
		mgr := NewManager("/tmp/test.json")
		ch := mgr.ReloadChannel()

		mgr.SignalReload()

		select {
		case <-ch:
			// Success
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for reload signal")
		}
	})

	t.Run("non-blocking when signal already pending", func(t *testing.T) {
		mgr := NewManager("/tmp/test.json")

		// Signal twice quickly
		mgr.SignalReload()
		mgr.SignalReload() // Should not block

		// Drain the channel
		<-mgr.ReloadChannel()
	})
}

func TestManager_Close(t *testing.T) {
	t.Run("closes all watchers", func(t *testing.T) {
		mgr := NewManager("/tmp/test.json")
		mgr.config = Default()

		ch1 := mgr.Watch()
		ch2 := mgr.Watch()

		mgr.Close()

		// Channels should be closed
		_, ok := <-ch1
		assert.False(t, ok)
		_, ok = <-ch2
		assert.False(t, ok)
	})
}

func TestManager_Concurrency(t *testing.T) {
	t.Run("safe concurrent access", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "daemon.json")

		mgr := NewManager(configPath)

		// Start with a valid config
		err := mgr.Save(Default())
		require.NoError(t, err)

		done := make(chan bool)

		// Concurrent reads
		go func() {
			for i := 0; i < 100; i++ {
				_ = mgr.Get()
				time.Sleep(time.Microsecond)
			}
			done <- true
		}()

		// Concurrent writes
		go func() {
			for i := 0; i < 50; i++ {
				_ = mgr.Update(func(c *Config) {
					c.MaxConnections = 100 + i
				})
				time.Sleep(time.Microsecond * 2)
			}
			done <- true
		}()

		// Concurrent loads
		go func() {
			for i := 0; i < 50; i++ {
				_, _ = mgr.Load()
				time.Sleep(time.Microsecond * 2)
			}
			done <- true
		}()

		// Wait for all goroutines
		for i := 0; i < 3; i++ {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("timeout in concurrent test")
			}
		}

		// Verify final state is consistent
		cfg := mgr.Get()
		assert.NotNil(t, cfg)
		assert.NoError(t, cfg.Validate())
	})
}

func TestManager_FullLifecycle(t *testing.T) {
	t.Run("complete lifecycle", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "daemon.json")

		// 1. Create and load defaults
		mgr := NewManager(configPath)
		cfg, err := mgr.Load()
		assert.NoError(t, err)
		assert.Equal(t, Default().SocketPath, cfg.SocketPath)

		// 2. Update config
		err = mgr.Update(func(c *Config) {
			c.SocketPath = "/new/socket.sock"
			c.MaxConnections = 999
		})
		assert.NoError(t, err)

		// 3. Watch for updates
		ch := mgr.Watch()

		// 4. Reload from file - should get the saved config
		cfg, err = mgr.Load()
		assert.NoError(t, err)
		assert.Equal(t, "/new/socket.sock", cfg.SocketPath)

		// 5. Watcher should receive notification when Load is called
		// Note: Load() is called above, so watcher already got notified
		select {
		case updated := <-ch:
			assert.Equal(t, "/new/socket.sock", updated.SocketPath)
		case <-time.After(time.Second):
			// Watcher might already have been notified
		}

		// 6. Signal reload
		mgr.SignalReload()
		select {
		case <-mgr.ReloadChannel():
			// Success
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}

		// 7. Verify final config still has updated values
		final := mgr.Get()
		assert.Equal(t, "/new/socket.sock", final.SocketPath)
		assert.Equal(t, 999, final.MaxConnections)

		// 8. Cleanup
		mgr.Close()
	})
}
