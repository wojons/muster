package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileWatcher(t *testing.T) {
	manager := NewManager("/tmp/test.json")
	watcher := NewFileWatcher("/tmp/test.json", manager)

	if watcher == nil {
		t.Fatal("NewFileWatcher returned nil")
	}

	if watcher.path != "/tmp/test.json" {
		t.Errorf("expected path '/tmp/test.json', got '%s'", watcher.path)
	}

	if watcher.manager != manager {
		t.Error("manager not set correctly")
	}

	if watcher.stopCh == nil {
		t.Error("stopCh not initialized")
	}
}

func TestFileWatcher_Initialize(t *testing.T) {
	// Create a temporary file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create initial config file with all required fields
	data := []byte(`{
		"socket_path": "/tmp/test.sock",
		"target_url": "http://localhost:8080",
		"max_connections": 100,
		"read_timeout": 30000000000,
		"write_timeout": 30000000000
	}`)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	manager := NewManager(configPath)
	watcher := NewFileWatcher(configPath, manager)

	// Test Initialize
	err := watcher.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Verify lastMod was set
	if watcher.lastMod.IsZero() {
		t.Error("lastMod should be set after Initialize")
	}

	// Test Initialize with non-existent file (should not error)
	nonExistentWatcher := NewFileWatcher("/non/existent/path.json", manager)
	err = nonExistentWatcher.Initialize()
	if err != nil {
		t.Errorf("Initialize with non-existent file should not error, got: %v", err)
	}
}

func TestFileWatcher_check(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create initial config file with all required fields
	initialConfig := `{
		"socket_path": "/tmp/test.sock",
		"target_url": "http://localhost:8080",
		"max_connections": 100,
		"read_timeout": 30000000000,
		"write_timeout": 30000000000
	}`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	manager := NewManager(configPath)
	watcher := NewFileWatcher(configPath, manager)

	// Initialize
	if err := watcher.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Get initial lastMod
	initialLastMod := watcher.lastMod

	// Wait a bit to ensure modification time changes
	time.Sleep(100 * time.Millisecond)

	// Modify the file with all required fields
	updatedConfig := `{
		"socket_path": "/tmp/test2.sock",
		"target_url": "http://localhost:9090",
		"max_connections": 100,
		"read_timeout": 30000000000,
		"write_timeout": 30000000000
	}`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	// Check for changes
	watcher.check()

	// Verify lastMod was updated
	if !watcher.lastMod.After(initialLastMod) {
		t.Error("lastMod should be updated after file modification")
	}

	// Verify config was loaded
	cfg := manager.Get()
	if cfg == nil {
		t.Fatal("config should be loaded after file modification")
	}
	if cfg.SocketPath != "/tmp/test2.sock" {
		t.Errorf("expected socket_path '/tmp/test2.sock', got '%s'", cfg.SocketPath)
	}
}

func TestFileWatcher_check_NoChange(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create initial config file with all required fields
	initialConfig := `{
		"socket_path": "/tmp/test.sock",
		"target_url": "http://localhost:8080",
		"max_connections": 100,
		"read_timeout": 30000000000,
		"write_timeout": 30000000000
	}`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	manager := NewManager(configPath)
	watcher := NewFileWatcher(configPath, manager)

	// Initialize
	if err := watcher.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Get initial lastMod
	initialLastMod := watcher.lastMod

	// Check without modifying file
	watcher.check()

	// Verify lastMod was NOT updated
	if !watcher.lastMod.Equal(initialLastMod) {
		t.Error("lastMod should not change when file is not modified")
	}
}

func TestFileWatcher_check_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.json")

	manager := NewManager(configPath)
	watcher := NewFileWatcher(configPath, manager)

	// Check with non-existent file (should not panic or error)
	watcher.check()

	// Verify lastMod is still zero
	if !watcher.lastMod.IsZero() {
		t.Error("lastMod should be zero when file doesn't exist")
	}
}

func TestFileWatcher_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create initial config file with all required fields
	initialConfig := `{
		"socket_path": "/tmp/test.sock",
		"target_url": "http://localhost:8080",
		"max_connections": 100,
		"read_timeout": 30000000000,
		"write_timeout": 30000000000
	}`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	manager := NewManager(configPath)
	watcher := NewFileWatcher(configPath, manager)

	// Initialize
	if err := watcher.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Start watcher with short interval
	go watcher.Start(50 * time.Millisecond)

	// Wait for first check
	time.Sleep(100 * time.Millisecond)

	// Modify the file
	updatedConfig := `{
		"socket_path": "/tmp/test3.sock",
		"target_url": "http://localhost:9090",
		"max_connections": 100,
		"read_timeout": 30000000000,
		"write_timeout": 30000000000
	}`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	// Wait for watcher to detect change
	time.Sleep(150 * time.Millisecond)

	// Verify config was updated
	cfg := manager.Get()
	if cfg == nil {
		t.Fatal("config should be loaded")
	}

	// Stop the watcher
	watcher.Stop()

	// Give it time to stop
	time.Sleep(50 * time.Millisecond)

	// Verify we can stop multiple times without panic
	watcher.Stop()
}

func TestFileWatcher_GetPath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	manager := NewManager(configPath)
	watcher := NewFileWatcher(configPath, manager)

	absPath, err := watcher.GetPath()
	if err != nil {
		t.Fatalf("GetPath failed: %v", err)
	}

	if absPath == "" {
		t.Error("GetPath should return non-empty path")
	}

	// Verify it's an absolute path
	if !filepath.IsAbs(absPath) {
		t.Error("GetPath should return absolute path")
	}
}

func TestFileWatcher_GetLastMod(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create initial config file with all required fields
	initialConfig := `{
		"socket_path": "/tmp/test.sock",
		"target_url": "http://localhost:8080",
		"max_connections": 100,
		"read_timeout": 30000000000,
		"write_timeout": 30000000000
	}`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	manager := NewManager(configPath)
	watcher := NewFileWatcher(configPath, manager)

	// Initially should be zero
	if !watcher.GetLastMod().IsZero() {
		t.Error("lastMod should be zero before Initialize")
	}

	// Initialize
	if err := watcher.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Now should not be zero
	if watcher.GetLastMod().IsZero() {
		t.Error("lastMod should not be zero after Initialize")
	}
}

func TestFileWatcher_ReloadSignal(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create initial config file with all required fields
	initialConfig := `{
		"socket_path": "/tmp/test.sock",
		"target_url": "http://localhost:8080",
		"max_connections": 100,
		"read_timeout": 30000000000,
		"write_timeout": 30000000000
	}`
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	manager := NewManager(configPath)
	watcher := NewFileWatcher(configPath, manager)

	// Initialize and load
	if err := watcher.Initialize(); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if _, err := manager.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Watch for reload signals
	reloadCh := manager.ReloadChannel()

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Modify the file
	updatedConfig := `{
		"socket_path": "/tmp/test4.sock",
		"target_url": "http://localhost:9090",
		"max_connections": 100,
		"read_timeout": 30000000000,
		"write_timeout": 30000000000
	}`
	if err := os.WriteFile(configPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("failed to update test file: %v", err)
	}

	// Check for changes
	watcher.check()

	// Verify reload signal was sent
	select {
	case <-reloadCh:
		// Success - reload signal received
	case <-time.After(100 * time.Millisecond):
		t.Error("reload signal should be sent after file modification")
	}
}
