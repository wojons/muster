package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDirectories(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	dirs := Directories()

	t.Run("ConfigDir", func(t *testing.T) {
		if dirs.Config == "" {
			t.Error("Config path should not be empty")
		}
		if !filepath.IsAbs(dirs.Config) {
			t.Error("Config path should be absolute")
		}
	})

	t.Run("DataDir", func(t *testing.T) {
		if dirs.Data == "" {
			t.Error("Data path should not be empty")
		}
		if !filepath.IsAbs(dirs.Data) {
			t.Error("Data path should be absolute")
		}
	})

	t.Run("CacheDir", func(t *testing.T) {
		if dirs.Cache == "" {
			t.Error("Cache path should not be empty")
		}
		if !filepath.IsAbs(dirs.Cache) {
			t.Error("Cache path should be absolute")
		}
	})
}

func TestMacOSDirs(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("skipping non-Darwin platform test")
	}

	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME not set")
	}

	dirs := macOSDirs()

	expectedConfig := filepath.Join(home, "Library", "Application Support", "openapi-cli")
	expectedCache := filepath.Join(home, "Library", "Caches", "openapi-cli")

	if dirs.Config != expectedConfig {
		t.Errorf("Expected config %s, got %s", expectedConfig, dirs.Config)
	}
	if dirs.Data != expectedConfig {
		t.Errorf("Expected data %s, got %s", expectedConfig, dirs.Data)
	}
	if dirs.Cache != expectedCache {
		t.Errorf("Expected cache %s, got %s", expectedCache, dirs.Cache)
	}
}

func TestLinuxDirs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping non-Linux platform test")
	}

	t.Run("WithXDGDefaults", func(t *testing.T) {
		home := os.Getenv("HOME")
		if home == "" {
			t.Skip("HOME not set")
		}

		_ = os.Unsetenv("XDG_CONFIG_HOME")
		_ = os.Unsetenv("XDG_DATA_HOME")
		_ = os.Unsetenv("XDG_CACHE_HOME")

		dirs := linuxDirs()

		expectedConfig := filepath.Join(home, ".config")
		expectedData := filepath.Join(home, ".local", "share")
		expectedCache := filepath.Join(home, ".cache")

		if dirs.Config != expectedConfig {
			t.Errorf("Expected config %s, got %s", expectedConfig, dirs.Config)
		}
		if dirs.Data != expectedData {
			t.Errorf("Expected data %s, got %s", expectedData, dirs.Data)
		}
		if dirs.Cache != expectedCache {
			t.Errorf("Expected cache %s, got %s", expectedCache, dirs.Cache)
		}
	})

	t.Run("WithCustomXDG", func(t *testing.T) {
		defer func() {
			_ = os.Setenv("XDG_CONFIG_HOME", "")
			_ = os.Setenv("XDG_DATA_HOME", "")
			_ = os.Setenv("XDG_CACHE_HOME", "")
		}()

		_ = os.Setenv("XDG_CONFIG_HOME", "/custom/config")
		_ = os.Setenv("XDG_DATA_HOME", "/custom/data")
		_ = os.Setenv("XDG_CACHE_HOME", "/custom/cache")

		dirs := linuxDirs()

		if dirs.Config != "/custom/config" {
			t.Errorf("Expected config /custom/config, got %s", dirs.Config)
		}
		if dirs.Data != "/custom/data" {
			t.Errorf("Expected data /custom/data, got %s", dirs.Data)
		}
		if dirs.Cache != "/custom/cache" {
			t.Errorf("Expected cache /custom/cache, got %s", dirs.Cache)
		}
	})
}

func TestWindowsDirs(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping non-Windows platform test")
	}

	t.Run("WithDefaults", func(t *testing.T) {
		_ = os.Unsetenv("APPDATA")
		_ = os.Setenv("LOCALAPPDATA", "")
		_ = os.Setenv("USERPROFILE", "C:\\Users\\Test")

		dirs := windowsDirs()

		expectedConfig := "C:\\Users\\Test\\AppData\\Roaming\\openapi-cli"
		expectedCache := "C:\\Users\\Test\\AppData\\Local\\openapi-cli\\Cache"

		if dirs.Config != expectedConfig {
			t.Errorf("Expected config %s, got %s", expectedConfig, dirs.Config)
		}
		if dirs.Data != expectedConfig {
			t.Errorf("Expected data %s, got %s", expectedConfig, dirs.Data)
		}
		if dirs.Cache != expectedCache {
			t.Errorf("Expected cache %s, got %s", expectedCache, dirs.Cache)
		}
	})

	t.Run("WithCustomPaths", func(t *testing.T) {
		defer func() {
			_ = os.Setenv("APPDATA", "")
			_ = os.Setenv("LOCALAPPDATA", "")
		}()

		_ = os.Setenv("APPDATA", "C:\\Custom\\Roaming")
		_ = os.Setenv("LOCALAPPDATA", "C:\\Custom\\Local")

		dirs := windowsDirs()

		if dirs.Config != "C:\\Custom\\Roaming\\openapi-cli" {
			t.Errorf("Expected config %s, got %s", "C:\\Custom\\Roaming\\openapi-cli", dirs.Config)
		}
		if dirs.Data != "C:\\Custom\\Roaming\\openapi-cli" {
			t.Errorf("Expected data %s, got %s", "C:\\Custom\\Roaming\\openapi-cli", dirs.Data)
		}
		if dirs.Cache != "C:\\Custom\\Local\\openapi-cli\\Cache" {
			t.Errorf("Expected cache %s, got %s", "C:\\Custom\\Local\\openapi-cli\\Cache", dirs.Cache)
		}
	})
}
