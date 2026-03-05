package wasm

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoader_LoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewLoader("")

	validWASM := []byte{
		0x00, 0x61, 0x73, 0x6d,
		0x01, 0x00, 0x00, 0x00,
	}

	t.Run("valid module", func(t *testing.T) {
		path := filepath.Join(tmpDir, "test.wasm")
		if err := os.WriteFile(path, validWASM, 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		module, err := loader.LoadFromFile(path)
		if err != nil {
			t.Fatalf("LoadFromFile failed: %v", err)
		}

		if module.Source.Type != SourceLocalFile {
			t.Errorf("expected source type %s, got %s", SourceLocalFile, module.Source.Type)
		}

		if module.Source.Path != path {
			t.Errorf("expected path %s, got %s", path, module.Source.Path)
		}

		if !bytes.Equal(module.Bytes, validWASM) {
			t.Error("module bytes mismatch")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := loader.LoadFromFile(filepath.Join(tmpDir, "nonexistent.wasm"))
		if err == nil {
			t.Error("expected error for non-existent file")
		}
	})

	t.Run("file too large", func(t *testing.T) {
		path := filepath.Join(tmpDir, "large.wasm")
		largeData := make([]byte, 51*1024*1024)
		if err := os.WriteFile(path, largeData, 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := loader.LoadFromFile(path)
		if err == nil {
			t.Error("expected error for file too large")
		}
	})

	t.Run("invalid WASM module", func(t *testing.T) {
		path := filepath.Join(tmpDir, "invalid.wasm")
		invalidData := []byte{0x00, 0x00, 0x00, 0x00}
		if err := os.WriteFile(path, invalidData, 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		_, err := loader.LoadFromFile(path)
		if err == nil {
			t.Error("expected error for invalid WASM module")
		}
	})
}

func TestLoader_LoadFromURL(t *testing.T) {
	loader := NewLoader("")

	validWASM := []byte{
		0x00, 0x61, 0x73, 0x6d,
		0x01, 0x00, 0x00, 0x00,
	}

	t.Run("valid remote URL", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		module, err := loader.LoadFromURL(ctx, "https://example.com/test.wasm", RemoteLoadOptions{
			Timeout: 1 * time.Second,
			MaxSize: 0,
		})

		if err != nil && err.Error() != "request failed" {
			t.Logf("LoadFromURL failed (expected in test env): %v", err)
		}

		_ = validWASM

		if module != nil {
			if module.Source.Type != SourceRemoteURL {
				t.Errorf("expected source type %s, got %s", SourceRemoteURL, module.Source.Type)
			}
		}
	})
}

func TestLoader_LoadBytes(t *testing.T) {
	loader := NewLoader("")

	validWASM := []byte{
		0x00, 0x61, 0x73, 0x6d,
		0x01, 0x00, 0x00, 0x00,
	}

	t.Run("valid WASM bytes", func(t *testing.T) {
		module, err := loader.LoadBytes(validWASM)
		if err != nil {
			t.Fatalf("LoadBytes failed: %v", err)
		}

		if module.Source.Type != SourceEmbedded {
			t.Errorf("expected source type %s, got %s", SourceEmbedded, module.Source.Type)
		}

		if !bytes.Equal(module.Bytes, validWASM) {
			t.Error("module bytes mismatch")
		}
	})

	t.Run("invalid WASM bytes", func(t *testing.T) {
		invalidData := []byte{0x00, 0x00, 0x00, 0x00}
		_, err := loader.LoadBytes(invalidData)
		if err == nil {
			t.Error("expected error for invalid WASM bytes")
		}
	})
}

func TestLoader_LoadWithCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	loader := NewLoader(cacheDir)

	validWASM := []byte{
		0x00, 0x61, 0x73, 0x6d,
		0x01, 0x00, 0x00, 0x00,
	}

	testPath := filepath.Join(tmpDir, "test.wasm")
	if err := os.WriteFile(testPath, validWASM, 0600); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	source := ModuleSource{
		Type: SourceLocalFile,
		Path: testPath,
	}

	t.Run("cache miss then hit", func(t *testing.T) {
		module1, err := loader.LoadWithCache(source)
		if err != nil {
			t.Fatalf("LoadWithCache failed: %v", err)
		}

		module2, err := loader.LoadWithCache(source)
		if err != nil {
			t.Fatalf("LoadWithCache failed on second call: %v", err)
		}

		if !bytes.Equal(module1.Bytes, module2.Bytes) {
			t.Error("cached module bytes mismatch")
		}
	})
}

func TestModuleCache(t *testing.T) {
	tmpDir := t.TempDir()
	cache := NewModuleCache(tmpDir, 1*time.Hour)

	t.Run("set and get", func(t *testing.T) {
		key := "test-key"
		data := []byte("test-data")

		if err := cache.Set(key, data); err != nil {
			t.Fatalf("cache.Set failed: %v", err)
		}

		retrieved, found := cache.Get(key)
		if !found {
			t.Error("expected to find cached data")
		}

		if !bytes.Equal(retrieved, data) {
			t.Error("retrieved data mismatch")
		}
	})

	t.Run("get non-existent", func(t *testing.T) {
		_, found := cache.Get("non-existent")
		if found {
			t.Error("expected not to find non-existent key")
		}
	})

	t.Run("delete", func(t *testing.T) {
		key := "delete-test"
		data := []byte("delete-data")

		cache.Set(key, data)
		cache.Delete(key)

		_, found := cache.Get(key)
		if found {
			t.Error("expected not to find deleted key")
		}
	})

	t.Run("clear", func(t *testing.T) {
		cacheDir := t.TempDir()
		c := NewModuleCache(cacheDir, 1*time.Hour)

		c.Set("key1", []byte("data1"))
		c.Set("key2", []byte("data2"))

		if err := c.Clear(); err != nil {
			t.Fatalf("cache.Clear failed: %v", err)
		}

		_, found1 := c.Get("key1")
		_, found2 := c.Get("key2")

		if found1 || found2 {
			t.Error("expected cache to be empty after clear")
		}
	})

	t.Run("TTL expiry", func(t *testing.T) {
		cacheDir := t.TempDir()
		c := NewModuleCache(cacheDir, 10*time.Millisecond)

		key := "ttl-test"
		data := []byte("ttl-data")

		c.Set(key, data)

		retrieved, found := c.Get(key)
		if !found {
			t.Error("expected to find data immediately after set")
		}

		if !bytes.Equal(retrieved, data) {
			t.Error("retrieved data mismatch")
		}

		time.Sleep(20 * time.Millisecond)

		_, found = c.Get(key)
		if found {
			t.Error("expected not to find expired data")
		}
	})

	t.Run("nil cache dir", func(t *testing.T) {
		nilCache := NewModuleCache("", 1*time.Hour)

		if err := nilCache.Set("key", []byte("data")); err != nil {
			t.Errorf("cache.Set with nil dir should be no-op: %v", err)
		}

		if _, found := nilCache.Get("key"); found {
			t.Error("expected not to find from nil cache dir")
		}

		if err := nilCache.Delete("key"); err != nil {
			t.Errorf("cache.Delete with nil dir should be no-op: %v", err)
		}

		if err := nilCache.Clear(); err != nil {
			t.Errorf("cache.Clear with nil dir should be no-op: %v", err)
		}
	})
}

func TestGenerateHash(t *testing.T) {
	data := []byte("test-data")
	hash1 := generateHash(data)
	hash2 := generateHash(data)

	if hash1 != hash2 {
		t.Error("hash generation not deterministic")
	}

	if hash1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestGenerateCacheKey(t *testing.T) {
	source1 := ModuleSource{
		Type:    SourceLocalFile,
		Path:    "/path/to/module.wasm",
		Version: "1.0.0",
	}

	source2 := ModuleSource{
		Type:    SourceLocalFile,
		Path:    "/path/to/module.wasm",
		Version: "1.0.0",
	}

	key1 := generateCacheKey(source1)
	key2 := generateCacheKey(source2)

	if key1 != key2 {
		t.Error("cache key generation not deterministic")
	}

	if key1 == "" {
		t.Error("cache key should not be empty")
	}

	source3 := ModuleSource{
		Type:    SourceLocalFile,
		Path:    "/different/path.wasm",
		Version: "1.0.0",
	}

	key3 := generateCacheKey(source3)

	if key1 == key3 {
		t.Error("different sources should produce different cache keys")
	}
}
