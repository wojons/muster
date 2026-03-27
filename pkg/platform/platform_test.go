package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnixFilename(t *testing.T) {
	t.Run("valid filename", func(t *testing.T) {
		result := unixFilename("path/to/file.txt")
		expected := "file.txt"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("empty filename", func(t *testing.T) {
		result := unixFilename("")
		if result != "" {
			t.Errorf("Expected empty string, got %s", result)
		}
	})

	t.Run("with forward slashes", func(t *testing.T) {
		result := unixFilename("path/to/deep/nested/file.txt")
		expected := "file.txt"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})
}

func TestWindowsFilename(t *testing.T) {
	t.Run("empty filename", func(t *testing.T) {
		result := windowsFilename("")
		if result != "" {
			t.Errorf("Expected empty string, got %s", result)
		}
	})

	t.Run("with backslashes", func(t *testing.T) {
		testPath := "path" + string(filepath.Separator) + "to" + string(filepath.Separator) + "file.txt"
		result := windowsFilename(testPath)
		expected := "file.txt"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})

	t.Run("with forward slashes converted", func(t *testing.T) {
		testPath := "path/to/file.txt"
		result := windowsFilename(testPath)
		expected := "file.txt"
		if result != expected {
			t.Errorf("Expected %s, got %s", expected, result)
		}
	})
}

func TestEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("create new directory", func(t *testing.T) {
		path := filepath.Join(tmpDir, "newdir")
		err := ensureDir(path)
		if err != nil {
			t.Errorf("Expected no error creating directory: %v", err)
		}

		stat, err := os.Stat(path)
		if err != nil {
			t.Errorf("Directory should exist: %v", err)
		}
		if !stat.IsDir() {
			t.Error("Path should be a directory")
		}
	})

	t.Run("directory exists", func(t *testing.T) {
		path := filepath.Join(tmpDir, "existing")
		err := os.Mkdir(path, 0700)
		if err != nil {
			t.Fatal("Failed to create test directory")
		}

		err = ensureDir(path)
		if err != nil {
			t.Errorf("Expected no error for existing directory: %v", err)
		}
	})

	t.Run("create nested directories", func(t *testing.T) {
		path := filepath.Join(tmpDir, "parent", "child", "grandchild")
		err := ensureDir(path)
		if err != nil {
			t.Errorf("Expected no error creating nested directories: %v", err)
		}

		stat, err := os.Stat(path)
		if err != nil {
			t.Errorf("Nested path should exist: %v", err)
		}
		if !stat.IsDir() {
			t.Error("Nested path should be a directory")
		}
	})
}

func TestDefaultSecureStorage(t *testing.T) {
	t.Run("returns non-nil storage", func(t *testing.T) {
		storage := DefaultSecureStorage()
		if storage == nil {
			t.Error("DefaultSecureStorage should return non-nil storage")
		}
	})
}

func TestIsDarwin(t *testing.T) {
	t.Run("on darwin", func(t *testing.T) {
		if runtime.GOOS == "darwin" {
			if !isDarwin() {
				t.Error("isDarwin() should return true on darwin")
			}
		}
	})

	t.Run("not on darwin", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			if isDarwin() {
				t.Error("isDarwin() should return false on non-darwin")
			}
		}
	})
}

func TestIsLinux(t *testing.T) {
	t.Run("on linux", func(t *testing.T) {
		if runtime.GOOS == "linux" {
			if !isLinux() {
				t.Error("isLinux() should return true on linux")
			}
		}
	})

	t.Run("not on linux", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			if isLinux() {
				t.Error("isLinux() should return false on non-linux")
			}
		}
	})
}

func TestIsWindows(t *testing.T) {
	t.Run("on windows", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			if !isWindows() {
				t.Error("isWindows() should return true on windows")
			}
		}
	})

	t.Run("not on windows", func(t *testing.T) {
		if runtime.GOOS != "windows" {
			if isWindows() {
				t.Error("isWindows() should return false on non-windows")
			}
		}
	})
}

func TestPlatformDirs(t *testing.T) {
	t.Run("DefaultSecureStorage interfaces", func(t *testing.T) {
		var storage SecureStorage

		storage = &MacOSKeychain{}
		_ = storage

		storage = &LinuxKeychain{}
		_ = storage

		storage = &WindowsKeychain{}
		_ = storage
	})

	t.Run("MacOSKeychain implements all methods", func(t *testing.T) {
		k := &MacOSKeychain{}
		_ = k.Store("", "", "")
		_, _ = k.Retrieve("", "")
		_ = k.Delete("", "")
		_, _ = k.List("")
	})

	t.Run("LinuxKeychain implements all methods", func(t *testing.T) {
		k := &LinuxKeychain{}
		if err := k.Store("", "", ""); err != nil {
			t.Errorf("LinuxKeychain.Store should not fail: %v", err)
		}
		if _, err := k.Retrieve("", ""); err != nil {
			t.Errorf("LinuxKeychain.Retrieve should not fail: %v", err)
		}
		if err := k.Delete("", ""); err != nil {
			t.Errorf("LinuxKeychain.Delete should not fail: %v", err)
		}
		if _, err := k.List(""); err != nil {
			t.Errorf("LinuxKeychain.List should not fail: %v", err)
		}
	})

	t.Run("WindowsKeychain implements all methods", func(t *testing.T) {
		k := &WindowsKeychain{}
		if err := k.Store("", "", ""); err != nil {
			t.Errorf("WindowsKeychain.Store should not fail: %v", err)
		}
		if _, err := k.Retrieve("", ""); err != nil {
			t.Errorf("WindowsKeychain.Retrieve should not fail: %v", err)
		}
		if err := k.Delete("", ""); err != nil {
			t.Errorf("WindowsKeychain.Delete should not fail: %v", err)
		}
		if _, err := k.List(""); err != nil {
			t.Errorf("WindowsKeychain.List should not fail: %v", err)
		}
	})
}
