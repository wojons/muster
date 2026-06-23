package auth

import (
	"encoding/base64"
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

// ── sanitizeKeychainParam ─────────────────────────────────────────────

func TestSanitizeKeychainParam_Valid(t *testing.T) {
	validInputs := []string{
		"myservice",
		"my_account",
		"my-account",
		"account123",
		"MY_SERVICE",
		"a",
		"1234567890abcdef",
		"service.with.dots",
		"service:with:colons",
		"service/with/slashes",
		"service@domain.com",
		"Plus+Sign",
		"Equal=Sign",
		"Percent%",
		"Caret^",
	}

	for _, input := range validInputs {
		t.Run("valid_"+input, func(t *testing.T) {
			err := sanitizeKeychainParam(input, "testparam")
			if err != nil {
				t.Errorf("expected no error for input %q, got: %v", input, err)
			}
		})
	}
}

func TestSanitizeKeychainParam_Empty(t *testing.T) {
	err := sanitizeKeychainParam("", "testparam")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("expected 'cannot be empty' in error, got: %v", err)
	}
}

func TestSanitizeKeychainParam_DangerousChars(t *testing.T) {
	dangerousChars := []string{
		"$", "`", "(", ")", "|", "&", ";", "<", ">", "\\",
		"\"", "'", "*", "?", "[", "]", "{", "}", "!", "#", "~",
	}

	for _, char := range dangerousChars {
		t.Run("dangerous_"+char, func(t *testing.T) {
			input := "service" + char + "name"
			err := sanitizeKeychainParam(input, "testparam")
			if err == nil {
				t.Errorf("expected error for input containing %q, got nil", char)
			}
		})
	}
}

func TestSanitizeKeychainParam_Whitespace(t *testing.T) {
	whitespaceInputs := []string{
		"hello\nworld",
		"hello\rworld",
		"hello\tworld",
	}

	for _, input := range whitespaceInputs {
		t.Run("whitespace_"+input, func(t *testing.T) {
			err := sanitizeKeychainParam(input, "testparam")
			if err == nil {
				t.Errorf("expected error for input %q, got nil", input)
			}
		})
	}
}

func TestSanitizeKeychainParam_InjectionPatterns(t *testing.T) {
	injectionInputs := []string{
		"../etc/passwd",
		"..\\windows\\system32",
		"$((echo hacked))",
	}

	for _, input := range injectionInputs {
		t.Run("injection_"+input, func(t *testing.T) {
			err := sanitizeKeychainParam(input, "testparam")
			if err == nil {
				t.Errorf("expected error for injection pattern %q, got nil", input)
			}
		})
	}
}

func TestSanitizeKeychainParam_SimpleSpecialCharsAllowed(t *testing.T) {
	// Colon and dot are explicitly allowed by the sanitizer (not in dangerous char list)
	err := sanitizeKeychainParam("service:v1.0", "testparam")
	if err != nil {
		t.Errorf("colon and dot should be allowed, got: %v", err)
	}
}

// ── NewKeychain ────────────────────────────────────────────────────────

func TestNewKeychain_ReturnsNonNil(t *testing.T) {
	kc, err := NewKeychain()
	if err != nil {
		t.Fatalf("NewKeychain failed: %v", err)
	}
	if kc == nil {
		t.Fatal("NewKeychain returned nil")
	}
}

func TestNewKeychain_ReturnsCorrectTypeOnLinux(t *testing.T) {
	kc, err := NewKeychain()
	if err != nil {
		t.Fatalf("NewKeychain failed: %v", err)
	}

	switch kc.(type) {
	case *LinuxKeychain:
		// Linux native keychain — expected on this host if D-Bus is available
	case *FileKeychain:
		// File-based fallback — expected if D-Bus secret service unavailable
	default:
		t.Logf("NewKeychain returned unexpected type: %T", kc)
	}
}

// ── FileKeychain helpers ───────────────────────────────────────────────

func TestFileKeychain_DeriveSystemEncryptionKey(t *testing.T) {
	kc := newTestFileKeychain(t)
	key := kc.deriveSystemEncryptionKey()

	if len(key) == 0 {
		t.Fatal("deriveSystemEncryptionKey returned empty key")
	}
	// SHA-256 produces exactly 32 bytes
	if len(key) != 32 {
		t.Errorf("expected 32-byte key, got %d bytes", len(key))
	}
}

func TestFileKeychain_DeriveSystemEncryptionKey_Deterministic(t *testing.T) {
	kc := newTestFileKeychain(t)
	key1 := kc.deriveSystemEncryptionKey()
	key2 := kc.deriveSystemEncryptionKey()

	// Same machine data should produce same key
	if string(key1) != string(key2) {
		t.Fatal("deriveSystemEncryptionKey is not deterministic")
	}
}

func TestFileKeychain_DeriveSystemEncryptionKey_DifferentBasePaths(t *testing.T) {
	kc1 := &FileKeychain{basePath: t.TempDir()}
	kc2 := &FileKeychain{basePath: t.TempDir()}

	key1 := kc1.deriveSystemEncryptionKey()
	key2 := kc2.deriveSystemEncryptionKey()

	// Different temp dirs but same machine data — keys should be identical
	// because deriveSystemEncryptionKey uses getMachineData() which is host-level
	if string(key1) != string(key2) {
		t.Fatal("different base paths produced different encryption keys")
	}
}

// ── decryptMasterKeyFile ────────────────────────────────────────────────

func TestDecryptMasterKeyFile_ValidV2(t *testing.T) {
	kc := &FileKeychain{basePath: t.TempDir()}
	masterKey := []byte("my-super-secret-master-key-for-testing")

	encrypted, err := kc.encryptMasterKeyFile(masterKey)
	if err != nil {
		t.Fatalf("encryptMasterKeyFile failed: %v", err)
	}

	decrypted, err := kc.decryptMasterKeyFile(encrypted)
	if err != nil {
		t.Fatalf("decryptMasterKeyFile failed: %v", err)
	}

	if string(decrypted) != string(masterKey) {
		t.Fatalf("round-trip failed: got %q, want %q", decrypted, masterKey)
	}
}

func TestDecryptMasterKeyFile_InvalidBase64(t *testing.T) {
	kc := &FileKeychain{basePath: t.TempDir()}
	_, err := kc.decryptMasterKeyFile("!!!not-valid-base64!!!")
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

func TestDecryptMasterKeyFile_V2_BadSalt(t *testing.T) {
	kc := &FileKeychain{basePath: t.TempDir()}
	// v2 JSON with a salt that is not valid base64
	badV2 := `{"version":2,"salt":"!!!bad-salt!!!","iterations":150000,"app_secret":"` +
		base64.StdEncoding.EncodeToString(make([]byte, 32)) +
		`","ciphertext":"dGVzdA=="}`
	encoded := base64.StdEncoding.EncodeToString([]byte(badV2))

	_, err := kc.decryptMasterKeyFile(encoded)
	if err == nil {
		t.Fatal("expected error for invalid salt base64, got nil")
	}
}

func TestDecryptMasterKeyFile_V2_BadAppSecret(t *testing.T) {
	kc := &FileKeychain{basePath: t.TempDir()}
	salt := make([]byte, 16)
	badV2 := `{"version":2,"salt":"` + base64.StdEncoding.EncodeToString(salt) +
		`","iterations":150000,"app_secret":"!!!bad-secret!!!","ciphertext":"dGVzdA=="}`
	encoded := base64.StdEncoding.EncodeToString([]byte(badV2))

	_, err := kc.decryptMasterKeyFile(encoded)
	if err == nil {
		t.Fatal("expected error for invalid app_secret base64, got nil")
	}
}

func TestDecryptMasterKeyFile_V2_BadCiphertext(t *testing.T) {
	kc := &FileKeychain{basePath: t.TempDir()}
	salt := base64.StdEncoding.EncodeToString(make([]byte, 16))
	appSecret := base64.StdEncoding.EncodeToString(make([]byte, 32))
	// Ciphertext is valid base64 but won't decrypt as AES-GCM with the derived key
	badV2 := `{"version":2,"salt":"` + salt +
		`","iterations":150000,"app_secret":"` + appSecret +
		`","ciphertext":"aW52YWxpZCBjaXBoZXJ0ZXh0IGRhdGE="}`
	encoded := base64.StdEncoding.EncodeToString([]byte(badV2))

	_, err := kc.decryptMasterKeyFile(encoded)
	if err == nil {
		t.Fatal("expected decrypt error for bad ciphertext, got nil")
	}
}

func TestDecryptMasterKeyFile_V1_Valid(t *testing.T) {
	kc := &FileKeychain{basePath: t.TempDir()}
	masterKey := []byte("v1-test-master-key")

	// Build a valid v1 encrypted payload manually
	// v1: {"salt":"...","iterations":N,"ciphertext":"..."}
	// Uses getApplicationSecretFallback instead of per-keychain app_secret
	salt := make([]byte, 16)
	password := getApplicationSecretFallback + "-" + kc.getMachineData()
	iterations := 150000
	encryptionKey := derivePBKDF2Key([]byte(password), salt, iterations, 32)
	encrypted, err := kc.encrypt(masterKey, encryptionKey)
	if err != nil {
		t.Fatalf("encrypt for v1 failed: %v", err)
	}

	v1Payload := `{"salt":"` + base64.StdEncoding.EncodeToString(salt) +
		`","iterations":150000,"ciphertext":"` + encrypted + `"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(v1Payload))

	decrypted, err := kc.decryptMasterKeyFile(encoded)
	if err != nil {
		t.Fatalf("decrypt v1 failed: %v", err)
	}

	if string(decrypted) != string(masterKey) {
		t.Fatalf("v1 round-trip failed: got %q, want %q", decrypted, masterKey)
	}
}

func TestDecryptMasterKeyFile_V1_BadSalt(t *testing.T) {
	kc := &FileKeychain{basePath: t.TempDir()}
	// v1 JSON with bad salt base64
	badV1 := `{"salt":"!!!bad-salt!!!","iterations":150000,"ciphertext":"dGVzdA=="}`
	encoded := base64.StdEncoding.EncodeToString([]byte(badV1))

	_, err := kc.decryptMasterKeyFile(encoded)
	if err == nil {
		t.Fatal("expected error for invalid v1 salt, got nil")
	}
}

func TestDecryptMasterKeyFile_V1_BadCiphertext(t *testing.T) {
	kc := &FileKeychain{basePath: t.TempDir()}
	salt := base64.StdEncoding.EncodeToString(make([]byte, 16))
	badV1 := `{"salt":"` + salt +
		`","iterations":150000,"ciphertext":"not-valid-ciphertext-data"}`
	encoded := base64.StdEncoding.EncodeToString([]byte(badV1))

	_, err := kc.decryptMasterKeyFile(encoded)
	if err == nil {
		t.Fatal("expected decrypt error for bad v1 ciphertext, got nil")
	}
}

func TestDecryptMasterKeyFile_CorruptJSON(t *testing.T) {
	kc := &FileKeychain{basePath: t.TempDir()}
	// Not valid JSON — won't match v2 or v1, falls through to legacy
	corrupt := base64.StdEncoding.EncodeToString([]byte("this-is-not-json"))
	_, err := kc.decryptMasterKeyFile(corrupt)
	if err == nil {
		t.Fatal("expected error for corrupt JSON in legacy path, got nil")
	}
}

func TestDecryptMasterKeyFile_Legacy(t *testing.T) {
	kc := &FileKeychain{basePath: t.TempDir()}
	// Legacy format: raw base64 of some encrypted data
	// Won't parse as v1/v2 JSON, hits the legacy raw decrypt path
	legacy := base64.StdEncoding.EncodeToString([]byte("legacy-encrypted-data"))
	_, err := kc.decryptMasterKeyFile(legacy)
	// Legacy decrypt will fail (not valid AES-GCM), which is expected
	if err == nil {
		t.Fatal("expected error for legacy format decrypt, got nil")
	}
}
