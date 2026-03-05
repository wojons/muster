package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPIDFilePath(t *testing.T) {
	path := DefaultPIDFilePath()
	if path == "" {
		t.Error("Expected non-empty PID file path")
	}
}

func TestNewManager(t *testing.T) {
	manager, err := NewManager(Config{
		PidFilePath: "/tmp/test-daemon.pid",
		Version:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if manager.version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", manager.version)
	}
}

func TestNewManager_Defaults(t *testing.T) {
	manager, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	if manager.version != DefaultPIDVersion {
		t.Errorf("Expected version %s, got %s", DefaultPIDVersion, manager.version)
	}
}

func TestManager_Acquire(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	manager, err := NewManager(Config{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// First acquire should succeed
	err = manager.Acquire()
	if err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}

	// Second acquire should fail
	err = manager.Acquire()
	if err == nil {
		t.Error("Expected error for second acquire")
	}

	// Verify PID file was created
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		t.Error("Expected PID file to exist")
	}

	// Clean up
	manager.Release()
}

func TestManager_Acquire_StaleFile(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	// Create PID file with non-existent PID
	content := "99999\n1709251200000000000\n/tmp/test.sock\n1.0.0\n"
	os.WriteFile(pidFile, []byte(content), 0644)

	manager, err := NewManager(Config{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Should detect stale file and allow acquire
	err = manager.Acquire()
	if err != nil {
		t.Fatalf("Acquire should succeed with stale file: %v", err)
	}

	manager.Release()
}

func TestManager_Release(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	manager, err := NewManager(Config{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Acquire then release
	if err := manager.Acquire(); err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	if err := manager.Release(); err != nil {
		t.Fatalf("Release failed: %v", err)
	}

	// Verify PID file was removed
	if _, err := os.Stat(pidFile); !os.IsNotExist(err) {
		t.Error("Expected PID file to be removed")
	}
}

func TestManager_Release_NotOwner(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	manager, err := NewManager(Config{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Create PID file with different PID
	content := "99999\n1709251200000000000\n/tmp/test.sock\n1.0.0\n"
	os.WriteFile(pidFile, []byte(content), 0644)

	// Should fail to release
	err = manager.Release()
	if err == nil {
		t.Error("Expected error for releasing file owned by another process")
	}
}

func TestManager_GetStatus(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	manager, _ := NewManager(Config{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})

	// Status before acquire
	status, err := manager.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.Running {
		t.Error("Expected not running before acquire")
	}

	// Acquire
	manager.Acquire()

	// Status after acquire
	status, err = manager.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if !status.Running {
		t.Error("Expected running after acquire")
	}

	if status.PID <= 0 {
		t.Error("Expected valid PID")
	}

	if status.Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", status.Version)
	}

	// Clean up
	manager.Release()
}

func TestManager_ConcurrentAcquire(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	manager, err := NewManager(Config{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// First acquire
	err = manager.Acquire()
	if err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}

	// Create another manager pointing to same file
	manager2, err := NewManager(Config{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create second manager: %v", err)
	}

	// Second acquire should fail
	err = manager2.Acquire()
	if err == nil {
		t.Error("Expected error for concurrent acquire")
	}

	manager.Release()
}

func TestController_Start(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	controller, err := NewController(ControllerConfig{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}

	err = controller.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Check status
	status, err := controller.Status()
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if !status.Running {
		t.Error("Expected running after start")
	}

	// Clean up
	controller.pidManager.Release()
}

func TestController_Start_AlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	controller, _ := NewController(ControllerConfig{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})

	// First start
	controller.Start()

	// Second start should fail
	err := controller.Start()
	if err == nil {
		t.Error("Expected error for start when already running")
	}

	// Clean up
	controller.pidManager.Release()
}

func TestController_IsRunning(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	controller, _ := NewController(ControllerConfig{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})

	if controller.IsRunning() {
		t.Error("Expected not running initially")
	}

	controller.Start()

	if !controller.IsRunning() {
		t.Error("Expected running after start")
	}

	controller.pidManager.Release()
}

func TestController_Status(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	controller, _ := NewController(ControllerConfig{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})

	status, err := controller.Status()
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if status.Running {
		t.Error("Expected not running initially")
	}
}

func TestController_Kill(t *testing.T) {
	// Create a mock PID file with non-existent process to test Kill
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	// Create PID file manually with a fake PID
	content := "99999\n1709251200000000000\n/tmp/test.sock\n1.0.0\n"
	os.WriteFile(pidFile, []byte(content), 0644)

	controller, _ := NewController(ControllerConfig{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})

	// Kill should attempt to kill the process
	// It might fail since process doesn't exist, but we just verify it doesn't panic
	controller.Kill()

	// Clean up
	os.Remove(pidFile)
}

func TestPIDFileFormat(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "daemon.pid")

	manager, _ := NewManager(Config{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})

	manager.Acquire()

	// Read PID file
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("Failed to read PID file: %v", err)
	}

	// Should have non-empty content
	if len(data) == 0 {
		t.Fatal("Expected non-empty PID file")
	}

	// Clean up
	manager.Release()
}

func TestCleanDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	pidFile := filepath.Join(tmpDir, "subdir", "daemon.pid")

	manager, _ := NewManager(Config{
		PidFilePath: pidFile,
		Version:     "1.0.0",
	})

	err := manager.Acquire()
	if err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(filepath.Dir(pidFile)); os.IsNotExist(err) {
		t.Error("Expected directory to be created")
	}

	manager.Release()
}
