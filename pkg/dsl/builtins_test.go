package dsl

import (
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"
)

func TestBuiltinFail(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("fail", builtinFail)

	_, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("test error")}, nil)
	if err == nil {
		t.Fatal("expected error from fail builtin")
	}

	if err.Error() != "script failure: test error" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestBuiltinEnv(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("env", builtinEnv)

	t.Setenv("TEST_VAR", "test_value")
	t.Setenv("TEST_VAR_DEFAULT", "")

	tests := []struct {
		name      string
		args      starlark.Tuple
		expectErr bool
		expected  string
	}{
		{
			name:      "existing var",
			args:      starlark.Tuple{starlark.String("TEST_VAR")},
			expectErr: false,
			expected:  "test_value",
		},
		{
			name:      "non-existing with default",
			args:      starlark.Tuple{starlark.String("NONEXISTING_VAR"), starlark.String("default")},
			expectErr: false,
			expected:  "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := builtin.CallInternal(thread, tt.args, nil)
			if tt.expectErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.expectErr {
				if str, ok := result.(starlark.String); ok {
					if string(str) != tt.expected {
						t.Errorf("expected %s, got %s", tt.expected, string(str))
					}
				} else {
					t.Fatal("expected string result")
				}
			}
		})
	}
}

func TestBuiltinBase64Encode(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("base64_encode", builtinBase64Encode)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"hello", "hello", "aGVsbG8="},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String(tt.input)}, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if str, ok := result.(starlark.String); ok {
				if string(str) != tt.expected {
					t.Errorf("expected %s, got %s", tt.expected, string(str))
				}
			} else {
				t.Fatal("expected string result")
			}
		})
	}
}

func TestBuiltinBase64Decode(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("base64_decode", builtinBase64Decode)

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{"valid base64", "aGVsbG8=", "hello", false},
		{"invalid base64", "!!!", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String(tt.input)}, nil)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantErr {
				if str, ok := result.(starlark.String); ok {
					if string(str) != tt.expected {
						t.Errorf("expected %s, got %s", tt.expected, string(str))
					}
				} else {
					t.Fatal("expected string result")
				}
			}
		})
	}
}

func TestBuiltinSHA256(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("sha256", builtinSHA256)

	result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("hello")}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if str, ok := result.(starlark.String); ok {
		if len(str) != 64 {
			t.Errorf("expected 64 character hash, got %d", len(str))
		}
	} else {
		t.Fatal("expected string result")
	}
}

func TestBuiltinMD5(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("md5", builtinMD5)

	result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("hello")}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if str, ok := result.(starlark.String); ok {
		if len(str) != 32 {
			t.Errorf("expected 32 character hash, got %d", len(str))
		}
	} else {
		t.Fatal("expected string result")
	}
}

func TestBuiltinJSONEncode(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("json_encode", builtinJSONEncode)

	dict := starlark.NewDict(1)
	dict.SetKey(starlark.String("key"), starlark.String("value"))

	result, err := builtin.CallInternal(thread, starlark.Tuple{dict}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if str, ok := result.(starlark.String); ok {
		if string(str) != `{"key":"value"}` {
			t.Errorf("expected JSON string, got %s", string(str))
		}
	} else {
		t.Fatal("expected string result")
	}
}

func TestBuiltinJSONDecode(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("json_decode", builtinJSONDecode)

	result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String(`{"key":"value"}`)}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dict, ok := result.(*starlark.Dict); ok {
		if dict.Len() != 1 {
			t.Errorf("expected dict with 1 item, got %d", dict.Len())
		}
	} else {
		t.Fatal("expected dict result")
	}
}

func TestBuiltinPrint(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("print", builtinPrint)

	result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("hello")}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != starlark.None {
		t.Fatal("expected None result")
	}
}

func TestBuiltinFormat(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("format", builtinFormat)

	result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("hello %s"), starlark.String("world")}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if str, ok := result.(starlark.String); ok {
		if string(str) != "hello world" {
			t.Errorf("expected 'hello world', got %s", string(str))
		}
	} else {
		t.Fatal("expected string result")
	}
}

func TestBuiltinSecret(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("secret", builtinSecret)

	t.Setenv("TEST_SECRET", "secret_value")

	result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("TEST_SECRET")}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if str, ok := result.(starlark.String); ok {
		if string(str) != "secret_value" {
			t.Errorf("expected 'secret_value', got %s", string(str))
		}
	} else {
		t.Fatal("expected string result")
	}
}

func TestBuiltinFileExists(t *testing.T) {
	oldWorkspace := workspaceDir
	defer func() { workspaceDir = oldWorkspace }()

	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("file_exists", builtinFileExists)

	tempDir := t.TempDir()
	workspaceDir = tempDir

	existingFile := filepath.Join(tempDir, "existing.txt")
	nonExistingFile := filepath.Join(tempDir, "nonexistent.txt")

	os.WriteFile(existingFile, []byte("test"), 0644)

	t.Run("existing file", func(t *testing.T) {
		result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String(existingFile)}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if b, ok := result.(starlark.Bool); ok {
			if !bool(b) {
				t.Error("expected true for existing file")
			}
		} else {
			t.Fatal("expected bool result")
		}
	})

	t.Run("non-existing file", func(t *testing.T) {
		result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String(nonExistingFile)}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if b, ok := result.(starlark.Bool); ok {
			if bool(b) {
				t.Error("expected false for non-existing file")
			}
		} else {
			t.Fatal("expected bool result")
		}
	})
}

func TestValidateFilePath(t *testing.T) {
	oldWorkspace := workspaceDir
	defer func() { workspaceDir = oldWorkspace }()

	tempDir := t.TempDir()
	workspaceDir = tempDir

	os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("content"), 0644)
	os.MkdirAll(filepath.Join(tempDir, "subdir"), 0755)

	tests := []struct {
		name    string
		path    string
		wantErr bool
		setup   func() func()
	}{
		{
			name:    "file in workspace",
			path:    filepath.Join(tempDir, "test.txt"),
			wantErr: false,
		},
		{
			name:    "subdirectory in workspace",
			path:    filepath.Join(tempDir, "subdir", "test.txt"),
			wantErr: false,
		},
		{
			name:    "file outside workspace - absolute path",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "file outside workspace - parent traversal",
			path:    filepath.Join(filepath.Dir(tempDir), "test.txt"),
			wantErr: true,
		},
		{
			name:    "file outside workspace - symlink escape",
			path:    "/tmp/../etc/passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := tt.setup
			if cleanup != nil {
				defer cleanup()
			}

			result, err := validateFilePath(tt.path)
			if tt.wantErr && err == nil {
				t.Error("expected error for path outside workspace")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && err == nil {
				if !filepath.IsAbs(result) {
					t.Error("expected absolute path")
				}
			}
		})
	}
}

func TestValidateNetworkURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid HTTPS URL",
			url:     "https://1.1.1.1/data",
			wantErr: false,
		},
		{
			name:    "valid HTTP URL",
			url:     "http://8.8.8.8/data",
			wantErr: false,
		},
		{
			name:    "blocked - localhost",
			url:     "http://localhost:8080/data",
			wantErr: true,
		},
		{
			name:    "blocked - 127.0.0.1",
			url:     "http://127.0.0.1:8080/data",
			wantErr: true,
		},
		{
			name:    "blocked - 10.0.0.0/8 private network",
			url:     "http://10.0.1.5/data",
			wantErr: true,
		},
		{
			name:    "blocked - 172.16.0.0/12 private network",
			url:     "http://172.16.0.1/data",
			wantErr: true,
		},
		{
			name:    "blocked - 192.168.0.0/16 private network",
			url:     "http://192.168.1.5/data",
			wantErr: true,
		},
		{
			name:    "blocked - AWS metadata service",
			url:     "http://169.254.169.254/latest/meta-data/",
			wantErr: true,
		},
		{
			name:    "blocked - .local domain",
			url:     "http://server.local/data",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			url:     "://invalid-url",
			wantErr: true,
		},
		{
			name:    "URL with no hostname",
			url:     "http://",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNetworkURL(tt.url)
			if tt.wantErr && err == nil {
				t.Error("expected error for blocked URL")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestBuiltinFileRead_Security(t *testing.T) {
	oldWorkspace := workspaceDir
	defer func() { workspaceDir = oldWorkspace }()

	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("file_read", builtinFileRead)

	tempDir := t.TempDir()
	workspaceDir = tempDir

	testContent := "test content"
	testFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(testFile, []byte(testContent), 0644)

	t.Run("file in workspace", func(t *testing.T) {
		result, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String(testFile)}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if str, ok := result.(starlark.String); ok {
			if string(str) != testContent {
				t.Errorf("expected %s, got %s", testContent, string(str))
			}
		}
	})

	t.Run("file outside workspace blocked", func(t *testing.T) {
		_, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("/etc/passwd")}, nil)
		if err == nil {
			t.Error("expected error for file outside workspace")
		}
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		_, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("../../etc/passwd")}, nil)
		if err == nil {
			t.Error("expected error for path traversal")
		}
	})
}

func TestBuiltinFileWrite_Security(t *testing.T) {
	oldWorkspace := workspaceDir
	defer func() { workspaceDir = oldWorkspace }()

	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("file_write", builtinFileWrite)

	tempDir := t.TempDir()
	workspaceDir = tempDir

	t.Run("file in workspace", func(t *testing.T) {
		testFile := filepath.Join(tempDir, "test.txt")
		_, err := builtin.CallInternal(thread, starlark.Tuple{
			starlark.String(testFile),
			starlark.String("test content"),
		}, nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		content, _ := os.ReadFile(testFile)
		if string(content) != "test content" {
			t.Error("file content not written correctly")
		}

		info, _ := os.Stat(testFile)
		if info.Mode().Perm() != 0600 {
			t.Errorf("expected 0600 permissions, got %v", info.Mode().Perm())
		}
	})

	t.Run("file outside workspace blocked", func(t *testing.T) {
		_, err := builtin.CallInternal(thread, starlark.Tuple{
			starlark.String("/etc/passwd"),
			starlark.String("malicious content"),
		}, nil)
		if err == nil {
			t.Error("expected error for file outside workspace")
		}
	})
}

func TestBuiltinHTTPGet_Security(t *testing.T) {
	thread := &starlark.Thread{}
	builtin := starlark.NewBuiltin("http_get", builtinHTTPGet)

	t.Run("external URL allowed", func(t *testing.T) {
		_, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("https://api.example.com/data")}, nil)
		if err == nil {
			t.Error("expected network error (no actual request expected)")
		}
	})

	t.Run("localhost blocked", func(t *testing.T) {
		_, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("http://localhost:8080/data")}, nil)
		if err == nil {
			t.Error("expected error for localhost")
		}
		// Accept "localhost", "loopback", or "private network" in error message
		if err != nil && !contains(err.Error(), "localhost") && !contains(err.Error(), "loopback") && !contains(err.Error(), "private network") {
			t.Errorf("expected localhost/loopback/private network error, got: %v", err)
		}
	})

	t.Run("private network blocked", func(t *testing.T) {
		_, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("http://192.168.1.1/data")}, nil)
		if err == nil {
			t.Error("expected error for private network")
		}
	})

	t.Run("metadata service blocked", func(t *testing.T) {
		_, err := builtin.CallInternal(thread, starlark.Tuple{starlark.String("http://169.254.169.254/latest/meta-data/")}, nil)
		if err == nil {
			t.Error("expected error for metadata service")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
