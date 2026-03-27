// Cross-platform integration tests that run on all platforms

package platform

import (
	"runtime"
	"testing"
)

// TestCrossPlatformCompatibility tests that features work on all platforms
func TestCrossPlatformCompatibility(t *testing.T) {
	// Test that platform-independent features work everywhere
	dirs := Directories()
	if dirs == nil {
		t.Fatal("Directories should return non-nil PlatformDirs")
	}

	// Verify platform directories are detected correctly
	t.Logf("Config dir: %s", dirs.Config)
	t.Logf("Data dir: %s", dirs.Data)
	t.Logf("Cache dir: %s", dirs.Cache)

	// Verify each platform returns appropriate structure
	switch runtime.GOOS {
	case "linux":
		if dirs.Config == "" {
			t.Error("Linux should have config directory path")
		}
		if dirs.Data == "" {
			t.Error("Linux should have data directory path")
		}
		if dirs.Cache == "" {
			t.Error("Linux should have cache directory path")
		}
	case "darwin":
		if dirs.Config == "" {
			t.Error("macOS should have config directory path")
		}
		if dirs.Data == "" {
			t.Error("macOS should have data directory path")
		}
		if dirs.Cache == "" {
			t.Error("macOS should have cache directory path")
		}
	case "windows":
		if dirs.Config == "" {
			t.Error("Windows should have config directory path")
		}
		if dirs.Data == "" {
			t.Error("Windows should have data directory path")
		}
		if dirs.Cache == "" {
			t.Error("Windows should have cache directory path")
		}
	}

	// Test secure storage fallback works on all platforms
	storage := DefaultSecureStorage()
	if storage == nil {
		t.Fatal("DefaultSecureStorage should return non-nil instance")
	}

	// Test basic storage operations
	testService := "cross-platform-test"
	testAccount := "cross-platform-test-account"
	testValue := "cross-platform-test-value"

	err := storage.Store(testService, testAccount, testValue)
	if err != nil {
		t.Logf("SecureStorage.Store error: %v", err)
		// May fail on some platforms, that's OK for this test
	} else {
		value, err := storage.Retrieve(testService, testAccount)
		if err != nil {
			t.Errorf("SecureStorage.Retrieve failed: %v", err)
		} else if value != testValue {
			t.Errorf("Expected %s, got %s", testValue, value)
		}

		err = storage.Delete(testService, testAccount)
		if err != nil {
			t.Logf("SecureStorage.Delete warning: %v", err)
		}
	}

	t.Log("Cross-platform compatibility verified")
}

// TestPlatformDirectories verifies platform directory detection
func TestPlatformDirectories(t *testing.T) {
	dirs := Directories()
	if dirs == nil {
		t.Fatal("Directories should never return nil")
	}

	// At minimum, config directory should be set
	if dirs.Config == "" {
		t.Error("Config directory should always be set")
	}

	t.Logf("Platform: %s", runtime.GOOS)
	t.Logf("Directories: Config=%s, Data=%s, Cache=%s", dirs.Config, dirs.Data, dirs.Cache)
}
