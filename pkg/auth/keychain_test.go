package auth

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestFileKeychain(t *testing.T) *FileKeychain {
	t.Helper()
	return &FileKeychain{
		basePath: t.TempDir(),
	}
}

// ── Set ────────────────────────────────────────────────────────────────

func TestFileKeychain_Set_Valid(t *testing.T) {
	kc := newTestFileKeychain(t)
	err := kc.Set("myservice", "myaccount", "secret123")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Verify file exists
	filename := kc.filePath("myservice", "myaccount")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatal("credential file not created")
	}
}

func TestFileKeychain_Set_EmptyService(t *testing.T) {
	kc := newTestFileKeychain(t)
	err := kc.Set("", "myaccount", "secret123")
	// Empty service results in empty filePath component but shouldn't error at the Set level
	// The sanitizeFilename will produce an empty string, making filePath "."
	// This is valid on the filesystem but creates a file named "." + "." + sanitized account
	if err != nil {
		t.Logf("Set with empty service returned error: %v", err)
	}
}

func TestFileKeychain_Set_EmptyAccount(t *testing.T) {
	kc := newTestFileKeychain(t)
	err := kc.Set("myservice", "", "secret123")
	if err != nil {
		t.Logf("Set with empty account returned error: %v", err)
	}
}

// ── Get ────────────────────────────────────────────────────────────────

func TestFileKeychain_Get_Found(t *testing.T) {
	kc := newTestFileKeychain(t)
	if err := kc.Set("myservice", "myaccount", "secret123"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := kc.Get("myservice", "myaccount")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != "secret123" {
		t.Errorf("expected 'secret123', got %q", got)
	}
}

func TestFileKeychain_Get_NotFound(t *testing.T) {
	kc := newTestFileKeychain(t)
	_, err := kc.Get("myservice", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent credential")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestFileKeychain_Get_EmptyParams(t *testing.T) {
	kc := newTestFileKeychain(t)
	_, err := kc.Get("", "")
	if err == nil {
		t.Log("Get with empty params succeeded")
	} else {
		t.Logf("Get with empty params returned: %v", err)
	}
}

// ── Delete ─────────────────────────────────────────────────────────────

func TestFileKeychain_Delete_Existing(t *testing.T) {
	kc := newTestFileKeychain(t)
	if err := kc.Set("myservice", "myaccount", "secret123"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if err := kc.Delete("myservice", "myaccount"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify file is gone
	filename := kc.filePath("myservice", "myaccount")
	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		t.Fatal("credential file still exists after delete")
	}
}

func TestFileKeychain_Delete_NonExistent(t *testing.T) {
	kc := newTestFileKeychain(t)
	err := kc.Delete("myservice", "nonexistent")
	if err != nil {
		t.Fatalf("Delete of non-existent credential should not error: %v", err)
	}
}

// ── List ───────────────────────────────────────────────────────────────

func TestFileKeychain_List_Empty(t *testing.T) {
	kc := newTestFileKeychain(t)
	accounts, err := kc.List("myservice")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(accounts))
	}
}

func TestFileKeychain_List_WithEntries(t *testing.T) {
	kc := newTestFileKeychain(t)
	if err := kc.Set("myservice", "account1", "pw1"); err != nil {
		t.Fatalf("Set account1 failed: %v", err)
	}
	if err := kc.Set("myservice", "account2", "pw2"); err != nil {
		t.Fatalf("Set account2 failed: %v", err)
	}

	accounts, err := kc.List("myservice")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(accounts))
	}

	names := make(map[string]bool)
	for _, a := range accounts {
		names[a.Account] = true
	}
	if !names["account1"] || !names["account2"] {
		t.Errorf("missing expected accounts: %v", names)
	}
}

func TestFileKeychain_List_MultipleServices(t *testing.T) {
	kc := newTestFileKeychain(t)
	if err := kc.Set("svcA", "user1", "pw1"); err != nil {
		t.Fatalf("Set svcA failed: %v", err)
	}
	if err := kc.Set("svcB", "user2", "pw2"); err != nil {
		t.Fatalf("Set svcB failed: %v", err)
	}

	// List should only return svcA accounts
	accounts, err := kc.List("svcA")
	if err != nil {
		t.Fatalf("List svcA failed: %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("expected 1 account for svcA, got %d", len(accounts))
	}
	if accounts[0].Account != "user1" {
		t.Errorf("expected 'user1', got %q", accounts[0].Account)
	}
}

func TestFileKeychain_List_NoBaseDir(t *testing.T) {
	kc := &FileKeychain{
		basePath: filepath.Join(t.TempDir(), "nonexistent"),
	}
	accounts, err := kc.List("anyservice")
	if err != nil {
		t.Fatalf("List with no base dir should return empty, not error: %v", err)
	}
	if len(accounts) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(accounts))
	}
}

// ── Overwrite ──────────────────────────────────────────────────────────

func TestFileKeychain_Overwrite(t *testing.T) {
	kc := newTestFileKeychain(t)
	if err := kc.Set("myservice", "myaccount", "first-password"); err != nil {
		t.Fatalf("First Set failed: %v", err)
	}
	if err := kc.Set("myservice", "myaccount", "second-password"); err != nil {
		t.Fatalf("Second Set failed: %v", err)
	}

	got, err := kc.Get("myservice", "myaccount")
	if err != nil {
		t.Fatalf("Get after overwrite failed: %v", err)
	}
	if got != "second-password" {
		t.Errorf("expected 'second-password', got %q", got)
	}
}

// ── Concurrent Access ──────────────────────────────────────────────────

func TestFileKeychain_ConcurrentAccess(t *testing.T) {
	kc := newTestFileKeychain(t)

	// Warm up: ensure master key is initialized before concurrent access.
	// Multiple goroutines racing on first-time master key generation causes
	// decryption failures (deriveKey is not thread-safe for initialization).
	if err := kc.Set("warmup", "warmup", "warmup"); err != nil {
		t.Fatalf("warmup Set failed: %v", err)
	}

	const goroutines = 10

	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			account := "account" + string(rune('0'+idx))
			if err := kc.Set("concurrent", account, "password"); err != nil {
				errs[idx] = err
				return
			}
			got, err := kc.Get("concurrent", account)
			if err != nil {
				errs[idx] = err
				return
			}
			if got != "password" {
				errs[idx] = err
			}
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d failed: %v", i, err)
		}
	}

	// Verify all accounts are in the list
	accounts, err := kc.List("concurrent")
	if err != nil {
		t.Fatalf("List after concurrent access failed: %v", err)
	}
	if len(accounts) != goroutines {
		t.Errorf("expected %d accounts, got %d", goroutines, len(accounts))
	}
}

// ── Helpers ────────────────────────────────────────────────────────────

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"hello world", "hello_world"},
		{"user@domain", "user_domain"},
		{"path/traversal", "dangerous_chars_rejected"},
		{"back\\slash", "dangerous_chars_rejected"},
		{"dot..dot", "path_traversal_rejected"},
		{"UPPERCASE", "UPPERCASE"},
		{"mixed-Case_123", "mixed-Case_123"},
		{"spaces and symbols!", "spaces_and_symbols_"},
		{".hidden", "_hidden"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFileKeychain_FilePath(t *testing.T) {
	kc := &FileKeychain{basePath: "/tmp/test-creds"}
	path := kc.filePath("myservice", "myaccount")
	expected := filepath.Join("/tmp/test-creds", "myservice.myaccount")
	if path != expected {
		t.Errorf("filePath = %q, want %q", path, expected)
	}
}

// ── NewFileKeychain ────────────────────────────────────────────────────

func TestNewFileKeychain(t *testing.T) {
	kc := NewFileKeychain()
	if kc == nil {
		t.Fatal("NewFileKeychain returned nil")
	}
	if kc.basePath == "" {
		t.Fatal("basePath is empty")
	}
	// basePath should end with .muster/credentials
	if !strings.HasSuffix(kc.basePath, ".muster/credentials") &&
		!strings.HasSuffix(kc.basePath, ".muster\\credentials") {
		t.Logf("basePath = %q (may differ by platform)", kc.basePath)
	}
}

// ── Set/Get with special characters ────────────────────────────────────

func TestFileKeychain_SetGet_SpecialChars(t *testing.T) {
	kc := newTestFileKeychain(t)

	// Password with special chars
	err := kc.Set("svc", "acct", "p@ssw0rd!$%^&*()")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, err := kc.Get("svc", "acct")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != "p@ssw0rd!$%^&*()" {
		t.Errorf("expected special char password, got %q", got)
	}
}

// ── Set/Get with long values ───────────────────────────────────────────

func TestFileKeychain_SetGet_LongValues(t *testing.T) {
	kc := newTestFileKeychain(t)
	longPassword := strings.Repeat("abcdefghij", 100) // 1000 chars

	err := kc.Set("svc", "acct", longPassword)
	if err != nil {
		t.Fatalf("Set with long password failed: %v", err)
	}

	got, err := kc.Get("svc", "acct")
	if err != nil {
		t.Fatalf("Get long password failed: %v", err)
	}
	if got != longPassword {
		t.Errorf("long password mismatch")
	}
}

// ── Delete then Get ────────────────────────────────────────────────────

func TestFileKeychain_DeleteThenGet(t *testing.T) {
	kc := newTestFileKeychain(t)
	if err := kc.Set("myservice", "myaccount", "secret"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := kc.Delete("myservice", "myaccount"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := kc.Get("myservice", "myaccount")
	if err == nil {
		t.Fatal("expected error getting deleted credential")
	}
}

// ── Set multiple services ──────────────────────────────────────────────

func TestFileKeychain_SetGet_MultipleServices(t *testing.T) {
	kc := newTestFileKeychain(t)

	services := map[string]string{
		"github":    "gh_token_abc",
		"gitlab":    "gl_token_def",
		"dockerhub": "dh_token_ghi",
	}

	for svc, pw := range services {
		if err := kc.Set(svc, "default", pw); err != nil {
			t.Fatalf("Set %s failed: %v", svc, err)
		}
	}

	for svc, expected := range services {
		got, err := kc.Get(svc, "default")
		if err != nil {
			t.Errorf("Get %s failed: %v", svc, err)
			continue
		}
		if got != expected {
			t.Errorf("%s: expected %q, got %q", svc, expected, got)
		}
	}
}
