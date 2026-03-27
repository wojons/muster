package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenManager_NewTokenManager(t *testing.T) {
	tokenPath := "/tmp/test/token/file"
	tm := NewTokenManager(tokenPath)

	if tm == nil {
		t.Fatal("NewTokenManager returned nil")
	}

	if tm.TokenPath() != tokenPath {
		t.Errorf("TokenPath() = %q, want %q", tm.TokenPath(), tokenPath)
	}

	if tm.IsGenerated() {
		t.Error("IsGenerated() = true, want false for new manager")
	}

	if tm.GetToken() != "" {
		t.Error("GetToken() should return empty string for new manager")
	}
}

func TestTokenManager_Generate(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTokenManager(filepath.Join(tmpDir, "token"))

	// Test successful generation
	err := tm.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	if !tm.IsGenerated() {
		t.Error("IsGenerated() = false after Generate()")
	}

	token := tm.GetToken()
	if token == "" {
		t.Error("GetToken() returned empty string after Generate()")
	}

	// Token should be 43 characters (32 bytes base64 encoded)
	if len(token) != 43 {
		t.Errorf("Token length = %d, want 43", len(token))
	}

	// Generate should create different tokens each time
	err = tm.Generate()
	if err != nil {
		t.Fatalf("Generate() second call error = %v", err)
	}

	newToken := tm.GetToken()
	if newToken == token {
		t.Error("Generate() produced identical token, expected different token")
	}
}

func TestTokenManager_Save(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "daemon", "token")
	tm := NewTokenManager(tokenPath)

	// Save without generating should fail
	err := tm.Save()
	if err == nil {
		t.Error("Save() without Generate() should return error")
	}

	// Generate and save
	err = tm.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	err = tm.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Check file exists
	if !tm.FileExists() {
		t.Error("FileExists() = false after Save()")
	}

	// Check file permissions (should be 0600)
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("Stat token file error = %v", err)
	}

	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("Token file permissions = %04o, want 0600", mode)
	}

	// Check file contents
	content, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("Read token file error = %v", err)
	}

	if string(content) != tm.GetToken() {
		t.Errorf("Token file content = %q, want %q", string(content), tm.GetToken())
	}
}

func TestTokenManager_Load(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	tm := NewTokenManager(tokenPath)

	// Load non-existent file should fail
	_, err := tm.Load()
	if err == nil {
		t.Error("Load() with non-existent file should return error")
	}

	// Create token file
	err = tm.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	err = tm.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load should succeed
	loaded, err := tm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded != tm.GetToken() {
		t.Errorf("Load() = %q, want %q", loaded, tm.GetToken())
	}
}

func TestTokenManager_Validate(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTokenManager(filepath.Join(tmpDir, "token"))

	// Validate without generating should fail
	if tm.Validate("any-token") {
		t.Error("Validate() = true without generated token, want false")
	}

	// Generate token
	err := tm.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	validToken := tm.GetToken()

	// Validate correct token
	if !tm.Validate(validToken) {
		t.Error("Validate() = false for correct token, want true")
	}

	// Validate incorrect token
	if tm.Validate("invalid-token") {
		t.Error("Validate() = true for invalid token, want false")
	}

	// Validate empty token
	if tm.Validate("") {
		t.Error("Validate() = true for empty token, want false")
	}
}

func TestTokenManager_ValidateFromHeader(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTokenManager(filepath.Join(tmpDir, "token"))

	// Generate token
	err := tm.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	validToken := tm.GetToken()

	// Test valid Bearer header
	if !tm.ValidateFromHeader("Bearer " + validToken) {
		t.Error("ValidateFromHeader() = false for valid Bearer header")
	}

	// Test invalid Bearer header
	if tm.ValidateFromHeader("Bearer invalid-token") {
		t.Error("ValidateFromHeader() = true for invalid Bearer header")
	}

	// Test missing Bearer prefix
	if tm.ValidateFromHeader(validToken) {
		t.Error("ValidateFromHeader() = true without Bearer prefix")
	}

	// Test empty header
	if tm.ValidateFromHeader("") {
		t.Error("ValidateFromHeader() = true for empty header")
	}

	// Test wrong prefix
	if tm.ValidateFromHeader("Token " + validToken) {
		t.Error("ValidateFromHeader() = true for wrong prefix")
	}
}

func TestTokenManager_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	tm := NewTokenManager(tokenPath)

	// Cleanup without generating should succeed (idempotent)
	err := tm.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup() without file error = %v", err)
	}

	// Generate and save
	err = tm.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	err = tm.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Cleanup should remove file and clear token
	err = tm.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	if tm.FileExists() {
		t.Error("FileExists() = true after Cleanup()")
	}

	if tm.IsGenerated() {
		t.Error("IsGenerated() = true after Cleanup()")
	}

	if tm.GetToken() != "" {
		t.Error("GetToken() non-empty after Cleanup()")
	}

	// Second cleanup should succeed (idempotent)
	err = tm.Cleanup()
	if err != nil {
		t.Fatalf("Second Cleanup() error = %v", err)
	}
}

func TestTokenManager_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	tm := NewTokenManager(filepath.Join(tmpDir, "token"))

	// Generate token
	err := tm.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	err = tm.Save()
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Concurrent reads
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			_ = tm.GetToken()
			_ = tm.IsGenerated()
			_ = tm.Validate(tm.GetToken())
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestTokenManager_Load_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	tm := NewTokenManager(tokenPath)

	// Create empty file
	err := os.WriteFile(tokenPath, []byte(""), 0600)
	if err != nil {
		t.Fatalf("Write empty file error = %v", err)
	}

	// Load should fail for empty file
	_, err = tm.Load()
	if err == nil {
		t.Error("Load() should return error for empty file")
	}
}

func TestTokenManager_Load_WhitespaceFile(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	tm := NewTokenManager(tokenPath)

	// Create file with only whitespace
	err := os.WriteFile(tokenPath, []byte("   \n\t  "), 0600)
	if err != nil {
		t.Fatalf("Write whitespace file error = %v", err)
	}

	// Load should fail for whitespace-only file
	_, err = tm.Load()
	if err == nil {
		t.Error("Load() should return error for whitespace-only file")
	}
}

func TestTokenManager_Save_SecurePermissions(t *testing.T) {
	baseTmp := t.TempDir()
	// Create a subdirectory that will be created by TokenManager
	tokenPath := filepath.Join(baseTmp, "secure", "token")
	tm := NewTokenManager(tokenPath)

	// Generate and save token
	if err := tm.Generate(); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := tm.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify token file permissions are 0600
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("Stat token file error = %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("Token file permissions = %o, want 0600", info.Mode().Perm())
	}

	// Verify parent directory permissions are 0700 (created by Save)
	createdDir := filepath.Join(baseTmp, "secure")
	dirInfo, err := os.Stat(createdDir)
	if err != nil {
		t.Fatalf("Stat directory error = %v", err)
	}
	if dirInfo.Mode().Perm() != 0700 {
		t.Errorf("Directory permissions = %o, want 0700", dirInfo.Mode().Perm())
	}

	// Verify token content is correct
	content, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if string(content) != tm.GetToken() {
		t.Error("Token file content does not match generated token")
	}
}

func TestTokenManager_Save_AtomicCreation(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	tm := NewTokenManager(tokenPath)

	// Generate and save token
	if err := tm.Generate(); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := tm.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify no temp files left behind
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	if len(files) != 1 {
		t.Errorf("Expected 1 file in directory, found %d", len(files))
	}
	if files[0].Name() != "token" {
		t.Errorf("Expected file 'token', found '%s'", files[0].Name())
	}
}

func TestTokenManager_Save_GroupNotReadable(t *testing.T) {
	tmpDir := t.TempDir()
	tokenPath := filepath.Join(tmpDir, "token")
	tm := NewTokenManager(tokenPath)

	// Generate and save token
	if err := tm.Generate(); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if err := tm.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify group has no read permissions
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("Stat error = %v", err)
	}

	// Check that group read bit is NOT set
	if info.Mode().Perm()&0040 != 0 {
		t.Error("Group should not have read permission on token file")
	}

	// Check that others read bit is NOT set
	if info.Mode().Perm()&0004 != 0 {
		t.Error("Others should not have read permission on token file")
	}
}

func TestTokenManager_Save_CleanupTempOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a file where we want to create a directory
	conflictPath := filepath.Join(tmpDir, "conflict")
	if err := os.WriteFile(conflictPath, []byte("test"), 0600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}

	// Use a path that will cause MkdirAll to fail
	tokenPath := filepath.Join(conflictPath, "token")
	tm := NewTokenManager(tokenPath)

	// Generate token
	if err := tm.Generate(); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	// Save should fail because conflictPath is a file, not directory
	err := tm.Save()
	if err == nil {
		t.Fatal("Save() should fail when parent path is a file")
	}

	// Verify no temp files left behind
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	// Should only have the conflict file, no temp files
	if len(files) != 1 {
		t.Errorf("Temp files should be cleaned up on failure, found %d files", len(files))
	}
}
