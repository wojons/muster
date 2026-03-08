package wasm

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSandboxedFS(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)

	if fs == nil {
		t.Error("NewSandboxedFS returned nil")
	}

	if fs.root != filepath.Clean(tmpDir) {
		t.Errorf("expected root %s, got %s", filepath.Clean(tmpDir), fs.root)
	}
}

func TestSandboxedFS_AllowDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)

	fs.AllowDirectory(tmpDir)

	if !fs.isPathAllowed(tmpDir) {
		t.Error("directory should be allowed")
	}
}

func TestSandboxedFS_Disallow(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)

	fs.AllowDirectory(tmpDir)
	fs.Disallow(tmpDir)

	if fs.isPathAllowed(tmpDir) {
		t.Error("directory should not be allowed after disallow")
	}
}

func TestSandboxedFS_SetReadOnly(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)

	fs.SetReadOnly(tmpDir)

	if !fs.isReadOnly(tmpDir) {
		t.Error("directory should be read-only")
	}
}

func TestSandboxedFS_Open(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)
	fs.AllowDirectory(tmpDir)

	t.Run("existing file", func(t *testing.T) {
		testPath := filepath.Join(tmpDir, "test.txt")
		testData := []byte("test data")
		if err := os.WriteFile(testPath, testData, 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		file, err := fs.Open("test.txt")
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer func() { _ = file.Close() }()

		data, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		if !bytes.Equal(data, testData) {
			t.Error("file data mismatch")
		}
	})

	t.Run("disallowed path", func(t *testing.T) {
		fs.Disallow(tmpDir)
		_, err := fs.Open("test.txt")
		if err == nil {
			t.Error("expected error for disallowed path")
		}
	})
}

func TestSandboxedFS_Create(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)
	fs.AllowDirectory(tmpDir)

	t.Run("create file", func(t *testing.T) {
		file, err := fs.Create("newfile.txt")
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
		defer func() { _ = file.Close() }()

		if _, err := file.Write([]byte("test")); err != nil {
			t.Fatalf("failed to write: %v", err)
		}

		info, err := fs.Stat("newfile.txt")
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}

		if info.Size() != int64(4) {
			t.Errorf("expected size 4, got %d", info.Size())
		}
	})

	t.Run("read-only path", func(t *testing.T) {
		roDir := t.TempDir()
		fs.SetReadOnly(roDir)

		_, err := fs.Create(filepath.Join(roDir, "test.txt"))
		if err == nil {
			t.Error("expected error for read-only path")
		}
	})
}

func TestSandboxedFS_Stat(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)
	fs.AllowDirectory(tmpDir)

	t.Run("existing file", func(t *testing.T) {
		testPath := filepath.Join(tmpDir, "test.txt")
		testData := []byte("test data")
		if err := os.WriteFile(testPath, testData, 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		info, err := fs.Stat("test.txt")
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}

		if info.Size() != int64(len(testData)) {
			t.Errorf("expected size %d, got %d", len(testData), info.Size())
		}
	})

	t.Run("virtual file", func(t *testing.T) {
		fs.SetVirtualFile("virtual.txt", []byte("virtual data"))

		info, err := fs.Stat("virtual.txt")
		if err != nil {
			t.Fatalf("Stat failed for virtual file: %v", err)
		}

		if info.Size() != int64(len("virtual data")) {
			t.Errorf("expected size %d, got %d", len("virtual data"), info.Size())
		}
	})
}

func TestSandboxedFS_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)
	fs.AllowDirectory(tmpDir)

	t.Run("remove file", func(t *testing.T) {
		testPath := filepath.Join(tmpDir, "remove.txt")
		if err := os.WriteFile(testPath, []byte("test"), 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		if err := fs.Remove("remove.txt"); err != nil {
			t.Fatalf("Remove failed: %v", err)
		}

		if _, err := os.Stat(testPath); err == nil {
			t.Error("expected file to be removed")
		}
	})
}

func TestSandboxedFS_ReadDir(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)
	fs.AllowDirectory(tmpDir)

	t.Run("read directory", func(t *testing.T) {
		_ = os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("test1"), 0600)
		_ = os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("test2"), 0600)
		_ = os.WriteFile(filepath.Join(tmpDir, "file3.txt"), []byte("test3"), 0600)

		infos, err := fs.ReadDir(".")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}

		if len(infos) != 3 {
			t.Errorf("expected 3 files, got %d", len(infos))
		}

		names := make([]string, len(infos))
		for i, info := range infos {
			names[i] = info.Name()
		}

		for i := 0; i < len(names)-1; i++ {
			if names[i] >= names[i+1] {
				t.Errorf("names not sorted: %v", names)
			}
		}
	})
}

func TestSandboxedFS_VirtualFile(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)
	fs.AllowDirectory(tmpDir)

	t.Run("set and get virtual file", func(t *testing.T) {
		testPath := "virtual/test.txt"
		testData := []byte("virtual test data")

		fs.SetVirtualFile(testPath, testData)

		retrieved := fs.GetVirtualFile(testPath)
		if !bytes.Equal(retrieved, testData) {
			t.Error("virtual file data mismatch")
		}

		file, err := fs.Open(testPath)
		if err != nil {
			t.Fatalf("Open virtual file failed: %v", err)
		}
		defer func() { _ = file.Close() }()

		data, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatalf("failed to read temp file: %v", err)
		}

		if !bytes.Equal(data, testData) {
			t.Error("virtual file open data mismatch")
		}
	})
}

func TestSandboxedFS_PathNormalization(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)
	fs.AllowDirectory(tmpDir)

	t.Run("normalize path", func(t *testing.T) {
		path, err := fs.normalizePath("subdir/../test.txt")
		if err != nil {
			t.Fatalf("normalizePath failed: %v", err)
		}

		if path != filepath.Clean("test.txt") {
			t.Errorf("expected normalized path, got %s", path)
		}
	})

	t.Run("escape root", func(t *testing.T) {
		_, err := fs.normalizePath("../../../etc/passwd")
		if err == nil {
			t.Error("expected error for escaped path")
		}
	})

	t.Run("absolute path", func(t *testing.T) {
		absPath := filepath.Join(tmpDir, "test.txt")
		relPath, err := fs.normalizePath(absPath)
		if err != nil {
			t.Errorf("normalizePath failed for absolute path: %v", err)
		}

		if relPath != "test.txt" {
			t.Errorf("expected test.txt, got %s", relPath)
		}
	})
}

func TestSandboxedFS_NoAllowedDirs(t *testing.T) {
	tmpDir := t.TempDir()
	fs := NewSandboxedFS(tmpDir)

	testPath := filepath.Join(tmpDir, "test.txt")
	_ = os.WriteFile(testPath, []byte("test"), 0600)

	file, err := fs.Open("test.txt")
	if err != nil {
		t.Errorf("Open should allow all paths when no restrictions set: %v", err)
	}
	if file != nil {
		_ = file.Close()
	}
}
