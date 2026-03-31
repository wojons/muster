package auth

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"io"
	"runtime"
	"strings"
	"testing"

	"crypto/fips140"
	"golang.org/x/crypto/hkdf"
)

// TestFIPS_Mode_Detection verifies FIPS 140-3 mode detection
func TestFIPS_Mode_Detection(t *testing.T) {
	t.Run("detect_fips_mode", func(t *testing.T) {
		enabled := fips140.Enabled()
		t.Logf("FIPS 140-3 mode: %v", enabled)

		if enabled {
			t.Log("✓ Running in FIPS 140-3 compliant mode")
		} else {
			t.Log("ℹ Running in standard mode (set GODEBUG=fips140=on for FIPS)")
		}
	})
}

// TestFIPS_AES_GCM verifies AES-GCM encryption (FIPS-approved)
func TestFIPS_AES_GCM(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("Failed to create cipher: %v", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("Failed to create GCM: %v", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("Failed to generate nonce: %v", err)
	}

	plaintext := []byte("FIPS 140-3 compliant encryption test")
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	decrypted, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("Failed to decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("Decryption mismatch: got %q, want %q", decrypted, plaintext)
	}

	t.Logf("✓ AES-256-GCM encryption/decryption successful")
	t.Logf("  Ciphertext length: %d bytes", len(ciphertext))
}

// TestFIPS_ECDSA verifies ECDSA signatures (FIPS-approved)
func TestFIPS_ECDSA(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	if privateKey.Curve != elliptic.P256() {
		t.Errorf("Expected P-256 curve, got %v", privateKey.Curve)
	}

	message := []byte("FIPS 140-3 compliant ECDSA signature test")
	hashed := sha256.Sum256(message)

	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hashed[:])
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	t.Logf("✓ ECDSA P-256 signature generated")
	t.Logf("  R: %x", r)
	t.Logf("  S: %x", s)

	valid := ecdsa.Verify(&privateKey.PublicKey, hashed[:], r, s)
	if !valid {
		t.Fatal("Signature verification failed")
	}

	t.Log("✓ ECDSA P-256 signature verified successfully")
}

// TestFIPS_RSA verifies RSA signatures (FIPS-approved)
func TestFIPS_RSA(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	if privateKey.N.BitLen() < 2048 {
		t.Errorf("RSA key too small: %d bits (minimum 2048)", privateKey.N.BitLen())
	}

	message := []byte("FIPS 140-3 compliant RSA signature test")
	hashed := sha256.Sum256(message)

	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hashed[:], nil)
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	t.Logf("✓ RSA-2048 PSS signature generated")
	t.Logf("  Signature length: %d bytes", len(signature))

	err = rsa.VerifyPSS(&privateKey.PublicKey, crypto.SHA256, hashed[:], signature, nil)
	if err != nil {
		t.Fatalf("Signature verification failed: %v", err)
	}

	t.Log("✓ RSA-2048 PSS signature verified successfully")
}

// TestFIPS_HMAC verifies HMAC (FIPS-approved MAC)
func TestFIPS_HMAC(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	message := []byte("FIPS 140-3 compliant HMAC test")
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	signature := mac.Sum(nil)

	t.Logf("✓ HMAC-SHA256 generated")
	t.Logf("  MAC: %x", signature)

	mac2 := hmac.New(sha256.New, key)
	mac2.Write(message)
	actualMAC := mac2.Sum(nil)

	if !hmac.Equal(signature, actualMAC) {
		t.Fatal("HMAC verification failed")
	}

	t.Log("✓ HMAC-SHA256 verified successfully")
}

// TestFIPS_SHA verifies SHA hash functions (FIPS-approved)
func TestFIPS_SHA(t *testing.T) {
	message := []byte("FIPS 140-3 compliant hash test")

	hash256 := sha256.Sum256(message)
	t.Logf("✓ SHA-256: %x", hash256)

	hash384 := sha512.Sum384(message)
	t.Logf("✓ SHA-384: %x", hash384)

	hash512 := sha512.Sum512(message)
	t.Logf("✓ SHA-512: %x", hash512)

	if len(hash256) != 32 {
		t.Errorf("SHA-256 wrong length: %d", len(hash256))
	}
	if len(hash384) != 48 {
		t.Errorf("SHA-384 wrong length: %d", len(hash384))
	}
	if len(hash512) != 64 {
		t.Errorf("SHA-512 wrong length: %d", len(hash512))
	}
}

// TestFIPS_HKDF verifies HKDF key derivation (FIPS-approved)
func TestFIPS_HKDF(t *testing.T) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("Failed to generate secret: %v", err)
	}

	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		t.Fatalf("Failed to generate salt: %v", err)
	}

	info := []byte("FIPS 140-3 key derivation")

	hkdfReader := hkdf.New(sha256.New, secret, salt, info)
	derivedKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, derivedKey); err != nil {
		t.Fatalf("Failed to derive key: %v", err)
	}

	t.Logf("✓ HKDF-SHA256 key derivation successful")
	t.Logf("  Derived key: %x", derivedKey)

	hkdfReader2 := hkdf.New(sha256.New, secret, salt, info)
	derivedKey2 := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader2, derivedKey2); err != nil {
		t.Fatalf("Failed to derive key (second time): %v", err)
	}

	if !bytes.Equal(derivedKey, derivedKey2) {
		t.Fatal("HKDF not deterministic")
	}

	t.Log("✓ HKDF key derivation is deterministic")
}

// TestFIPS_Random verifies random number generation (FIPS-approved)
func TestFIPS_Random(t *testing.T) {
	data := make([]byte, 32)
	n, err := rand.Read(data)
	if err != nil {
		t.Fatalf("Failed to generate random bytes: %v", err)
	}
	if n != 32 {
		t.Errorf("Expected 32 bytes, got %d", n)
	}

	t.Logf("✓ Random bytes generated: %x", data)

	data2 := make([]byte, 32)
	n2, err := rand.Read(data2)
	if err != nil {
		t.Fatalf("Failed to generate random bytes (second): %v", err)
	}
	if n2 != 32 {
		t.Errorf("Expected 32 bytes, got %d", n2)
	}

	if bytes.Equal(data, data2) {
		t.Fatal("Random bytes not unique")
	}

	t.Log("✓ Random number generation produces unique values")
}

// TestFIPS_ComplianceSuite runs all FIPS-approved algorithms and reports status
func TestFIPS_ComplianceSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping comprehensive FIPS test in short mode")
	}

	fipsMode := fips140.Enabled()

	t.Log(strings.Repeat("=", 60))
	t.Log("FIPS 140-3 Compliance Test Suite")
	t.Log(strings.Repeat("=", 60))
	t.Logf("FIPS Mode Enabled: %v", fipsMode)
	t.Logf("Go Version: %s", runtime.Version())
	t.Log(strings.Repeat("=", 60))

	tests := []struct {
		name string
		fn   func(*testing.T)
	}{
		{"AES-GCM Encryption", TestFIPS_AES_GCM},
		{"ECDSA Signatures", TestFIPS_ECDSA},
		{"RSA Signatures", TestFIPS_RSA},
		{"HMAC Authentication", TestFIPS_HMAC},
		{"SHA Hash Functions", TestFIPS_SHA},
		{"HKDF Key Derivation", TestFIPS_HKDF},
		{"Random Number Generation", TestFIPS_Random},
	}

	passed := 0
	failed := 0

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					failed++
					t.Errorf("Panic: %v", r)
				}
			}()

			tmpT := &testing.T{}
			tt.fn(tmpT)

			if tmpT.Failed() {
				failed++
			} else {
				passed++
			}
		})
	}

	t.Log(strings.Repeat("=", 60))
	t.Logf("Test Results: %d passed, %d failed", passed, failed)
	t.Log(strings.Repeat("=", 60))

	if failed > 0 {
		t.Errorf("%d FIPS compliance test(s) failed", failed)
	}
}

// BenchmarkFIPS_AES_GCM benchmarks AES-GCM performance in FIPS mode
func BenchmarkFIPS_AES_GCM(b *testing.B) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		b.Fatal(err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		b.Fatal(err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		b.Fatal(err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		b.Fatal(err)
	}

	plaintext := make([]byte, 1024)
	if _, err := rand.Read(plaintext); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gcm.Seal(nil, nonce, plaintext, nil)
	}
}

// BenchmarkFIPS_ECDSA_Sign benchmarks ECDSA signing
func BenchmarkFIPS_ECDSA_Sign(b *testing.B) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		b.Fatal(err)
	}
	message := []byte("benchmark message")
	hashed := sha256.Sum256(message)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := ecdsa.Sign(rand.Reader, privateKey, hashed[:])
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFIPS_RSA_Sign benchmarks RSA signing
func BenchmarkFIPS_RSA_Sign(b *testing.B) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatal(err)
	}
	message := []byte("benchmark message")
	hashed := sha256.Sum256(message)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hashed[:], nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
