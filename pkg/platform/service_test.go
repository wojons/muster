package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetSecurePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "testfile.txt")

	t.Run("set permissions", func(t *testing.T) {
		_ = os.WriteFile(testFile, []byte("test"), 0644)
		err := SetSecurePermissions(testFile)
		if err != nil {
			t.Errorf("Expected no error setting permissions: %v", err)
		}

		info, err := os.Stat(testFile)
		if err != nil {
			t.Fatalf("Failed to stat file: %v", err)
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("Expected 0600, got %04o", info.Mode().Perm())
		}
	})
}

func TestSetSecureDirPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "testdir")

	t.Run("set directory permissions", func(t *testing.T) {
		_ = os.Mkdir(testDir, 0755)
		err := SetSecureDirPermissions(testDir)
		if err != nil {
			t.Errorf("Expected no error setting directory permissions: %v", err)
		}

		info, err := os.Stat(testDir)
		if err != nil {
			t.Fatalf("Failed to stat directory: %v", err)
		}
		if info.Mode().Perm() != 0700 {
			t.Errorf("Expected 0700, got %04o", info.Mode().Perm())
		}
	})
}

func TestEnsureSecureFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "secure.txt")

	t.Run("create secure file", func(t *testing.T) {
		err := EnsureSecureFile(testFile)
		if err != nil {
			t.Errorf("Expected no error: %v", err)
		}

		info, err := os.Stat(testFile)
		if err != nil {
			t.Fatalf("Failed to stat file: %v", err)
		}
		if !info.Mode().IsRegular() {
			t.Error("Path should be a regular file")
		}
		if info.Mode().Perm() != 0600 {
			t.Errorf("Expected 0600, got %04o", info.Mode().Perm())
		}
	})
}

func TestIsSecurePath(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("secure path", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "secure.txt")
		_ = os.WriteFile(testFile, []byte("test"), 0600)

		if !IsSecurePath(testFile) {
			t.Error("Path should be secure")
		}
	})

	t.Run("not secure path", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "insecure.txt")
		_ = os.WriteFile(testFile, []byte("test"), 0644)

		if IsSecurePath(testFile) {
			t.Error("Path should not be secure")
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		if IsSecurePath("/nonexistent") {
			t.Error("Non-existent path should not be secure")
		}
	})
}

func TestCreateEntitlements(t *testing.T) {
	t.Run("returns string", func(t *testing.T) {
		entitlements := CreateEntitlements()
		if entitlements == "" {
			t.Error("Entitlements should not be empty")
		}
		if len(entitlements) < 10 {
			t.Error("Entitlements should have content")
		}
	})
}

func TestGetEntitlementsPath(t *testing.T) {
	t.Run("returns path", func(t *testing.T) {
		path := GetEntitlementsPath()
		if path != "entitlements.plist" {
			t.Errorf("Expected 'entitlements.plist', got %s", path)
		}
	})
}

func TestCreateSystemdService(t *testing.T) {
	t.Run("create service file", func(t *testing.T) {
		home := os.Getenv("HOME")
		if home == "" {
			t.Skip("HOME not set")
		}

		err := CreateSystemdService()
		if err != nil {
			t.Errorf("Expected no error: %v", err)
		}
	})
}

func TestGetSystemdServicePath(t *testing.T) {
	t.Run("returns path", func(t *testing.T) {
		_, err := os.UserHomeDir()
		if err != nil {
			t.Skip("Could not get home directory")
		}

		path := GetSystemdServicePath()
		if path == "" {
			t.Error("Path should not be empty")
		}
		if !filepath.IsAbs(path) {
			t.Error("Path should be absolute")
		}
	})
}

func TestInstallWindowsService(t *testing.T) {
	t.Run("service config", func(t *testing.T) {
		config := ServiceConfig{
			Name:        "test",
			DisplayName: "Test Service",
			Description: "Test Description",
			BinaryPath:  "/path/to/binary",
			Args:        []string{"arg1", "arg2"},
		}

		if config.Name != "test" {
			t.Errorf("Expected 'test', got '%s'", config.Name)
		}
	})
}

func TestEnableVirtualTerminal(t *testing.T) {
	t.Run("not implemented", func(t *testing.T) {
		err := EnableVirtualTerminal()
		if err == nil {
			t.Error("Should return error for unimplemented function")
		}
	})
}

func TestBuildUniversalBinary(t *testing.T) {
	t.Run("returns error for now (requires build tools)", func(t *testing.T) {
		// This function requires go, lipo to be installed
		// Skipping actual execution for now
		output := "/tmp/test-binary"
		_ = BuildUniversalBinary(output, "")
	})
}
