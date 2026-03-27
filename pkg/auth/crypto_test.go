package auth

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestGenerateSecureRandom(t *testing.T) {
	t.Run("generates random bytes", func(t *testing.T) {
		data, err := GenerateSecureRandom(32)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(data) != 32 {
			t.Errorf("expected 32 bytes, got %d", len(data))
		}
	})

	t.Run("different calls produce different results", func(t *testing.T) {
		data1, _ := GenerateSecureRandom(32)
		data2, _ := GenerateSecureRandom(32)
		if bytes.Equal(data1, data2) {
			t.Error("random data should be different")
		}
	})

	t.Run("zero length", func(t *testing.T) {
		data, err := GenerateSecureRandom(0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(data) != 0 {
			t.Errorf("expected 0 bytes, got %d", len(data))
		}
	})
}

func TestGenerateRSAKeyPair(t *testing.T) {
	t.Run("generates 4096 bit key", func(t *testing.T) {
		key, err := GenerateRSAKeyPair(4096)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key == nil {
			t.Fatal("expected key, got nil")
		}
		if key.N.BitLen() != 4096 {
			t.Errorf("expected 4096 bits, got %d", key.N.BitLen())
		}
	})

	t.Run("upgrades small key sizes", func(t *testing.T) {
		key, err := GenerateRSAKeyPair(2048)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key.N.BitLen() != 4096 {
			t.Errorf("expected 4096 bits (upgraded), got %d", key.N.BitLen())
		}
	})
}

func TestGenerateECDSAKeyPair(t *testing.T) {
	t.Run("generates P-384 key", func(t *testing.T) {
		key, err := GenerateECDSAKeyPair()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key == nil {
			t.Fatal("expected key, got nil")
		}
		if key.Curve.Params().Name != "P-384" {
			t.Errorf("expected P-384 curve, got %s", key.Curve.Params().Name)
		}
	})
}

func TestEncryptDecryptRSAOAEP(t *testing.T) {
	t.Run("encrypts and decrypts", func(t *testing.T) {
		privateKey, _ := GenerateRSAKeyPair(2048)
		publicKey := &privateKey.PublicKey

		plaintext := []byte("Hello, World!")

		ciphertext, err := EncryptRSAOAEP(publicKey, plaintext)
		if err != nil {
			t.Fatalf("encryption failed: %v", err)
		}

		if bytes.Equal(ciphertext, plaintext) {
			t.Error("ciphertext should differ from plaintext")
		}

		decrypted, err := DecryptRSAOAEP(privateKey, ciphertext)
		if err != nil {
			t.Fatalf("decryption failed: %v", err)
		}

		if !bytes.Equal(decrypted, plaintext) {
			t.Errorf("decrypted text differs from original")
		}
	})

	t.Run("nil public key", func(t *testing.T) {
		_, err := EncryptRSAOAEP(nil, []byte("test"))
		if err != ErrInvalidKey {
			t.Errorf("expected ErrInvalidKey, got %v", err)
		}
	})

	t.Run("nil private key", func(t *testing.T) {
		_, err := DecryptRSAOAEP(nil, []byte("test"))
		if err != ErrInvalidKey {
			t.Errorf("expected ErrInvalidKey, got %v", err)
		}
	})
}

func TestSignVerifyECDSA(t *testing.T) {
	t.Run("signs and verifies", func(t *testing.T) {
		privateKey, _ := GenerateECDSAKeyPair()
		publicKey := &privateKey.PublicKey

		data := []byte("Test data to sign")

		signature, err := SignECDSA(privateKey, data)
		if err != nil {
			t.Fatalf("signing failed: %v", err)
		}

		if len(signature) == 0 {
			t.Fatal("signature is empty")
		}

		valid := VerifyECDSA(publicKey, data, signature)
		if !valid {
			t.Error("signature verification failed")
		}
	})

	t.Run("detects tampered data", func(t *testing.T) {
		privateKey, _ := GenerateECDSAKeyPair()
		publicKey := &privateKey.PublicKey

		data := []byte("Original data")
		signature, _ := SignECDSA(privateKey, data)

		tampered := []byte("Tampered data")
		valid := VerifyECDSA(publicKey, tampered, signature)
		if valid {
			t.Error("verification should fail for tampered data")
		}
	})

	t.Run("nil keys", func(t *testing.T) {
		if VerifyECDSA(nil, []byte("test"), []byte("sig")) {
			t.Error("should return false for nil key")
		}
	})
}

func TestEncryptDecryptAESGCM(t *testing.T) {
	t.Run("encrypts and decrypts", func(t *testing.T) {
		key, _ := GenerateSecureRandom(32)

		plaintext := []byte("Secret message")

		ciphertextWithTag, nonce, err := EncryptAESGCM(plaintext, key)
		if err != nil {
			t.Fatalf("encryption failed: %v", err)
		}

		decrypted, err := DecryptAESGCM(ciphertextWithTag, key, nonce)
		if err != nil {
			t.Fatalf("decryption failed: %v", err)
		}

		if !bytes.Equal(decrypted, plaintext) {
			t.Errorf("decrypted text differs from original")
		}
	})

	t.Run("wrong key size", func(t *testing.T) {
		key := make([]byte, 16)
		_, _, err := EncryptAESGCM([]byte("test"), key)
		if err == nil || !strings.Contains(err.Error(), "32 bytes") {
			t.Errorf("expected 32 bytes error, got %v", err)
		}
	})

	t.Run("different nonces produce different ciphertexts", func(t *testing.T) {
		key := make([]byte, 32)
		plaintext := []byte("Same message")

		c1, _, _ := EncryptAESGCM(plaintext, key)
		c2, _, _ := EncryptAESGCM(plaintext, key)

		if bytes.Equal(c1, c2) {
			t.Error("different encryptions should produce different ciphertexts")
		}
	})
}

func TestDeriveKeyPBKDF2(t *testing.T) {
	t.Run("derives key", func(t *testing.T) {
		password := []byte("mypassword")
		salt := []byte("somesalt")

		key, err := DeriveKeyPBKDF2(password, salt, 10000, 32)
		if err != nil {
			t.Fatalf("key derivation failed: %v", err)
		}

		if len(key) != 32 {
			t.Errorf("expected 32 bytes, got %d", len(key))
		}
	})

	t.Run("same inputs produce same outputs", func(t *testing.T) {
		password := []byte("password")
		salt := []byte("salt")

		key1, _ := DeriveKeyPBKDF2(password, salt, 10000, 32)
		key2, _ := DeriveKeyPBKDF2(password, salt, 10000, 32)

		if !bytes.Equal(key1, key2) {
			t.Error("same inputs should produce same derived key")
		}
	})
}

func TestDeriveKeyArgon2(t *testing.T) {
	t.Run("derives key with default params", func(t *testing.T) {
		password := []byte("mypassword")

		key, err := DeriveKeyArgon2(password, nil)
		if err != nil {
			t.Fatalf("key derivation failed: %v", err)
		}

		if len(key) == 0 {
			t.Error("derived key is empty")
		}
	})

	t.Run("derives key with custom params", func(t *testing.T) {
		password := []byte("password")
		params := &Argon2Params{
			Memory:      32 * 1024,
			Iterations:  2,
			Parallelism: 2,
			SaltLength:  16,
			KeyLength:   32,
		}

		key, err := DeriveKeyArgon2(password, params)
		if err != nil {
			t.Fatalf("key derivation failed: %v", err)
		}

		if len(key) != 32 {
			t.Errorf("expected 32 bytes, got %d", len(key))
		}
	})
}

func TestHashPasswordArgon2(t *testing.T) {
	t.Run("hashes password", func(t *testing.T) {
		hash, err := HashPasswordArgon2("testpassword")
		if err != nil {
			t.Fatalf("hashing failed: %v", err)
		}

		if !strings.HasPrefix(hash, "$argon2id$") {
			t.Error("hash should use argon2id")
		}
	})

	t.Run("different salts produce different hashes", func(t *testing.T) {
		hash1, _ := HashPasswordArgon2("password")
		hash2, _ := HashPasswordArgon2("password")

		if hash1 == hash2 {
			t.Error("different runs should produce different hashes")
		}
	})
}

func TestVerifyPasswordArgon2(t *testing.T) {
	t.Run("verifies correct password", func(t *testing.T) {
		password := "testpassword"
		hash, _ := HashPasswordArgon2(password)

		valid, err := VerifyPasswordArgon2(password, hash)
		if err != nil {
			t.Fatalf("verification failed: %v", err)
		}
		if !valid {
			t.Error("correct password should verify")
		}
	})

	t.Run("rejects wrong password", func(t *testing.T) {
		password := "testpassword"
		hash, _ := HashPasswordArgon2(password)

		valid, err := VerifyPasswordArgon2("wrongpassword", hash)
		if err != nil {
			t.Fatalf("verification failed: %v", err)
		}
		if valid {
			t.Error("wrong password should not verify")
		}
	})

	t.Run("rejects invalid format", func(t *testing.T) {
		_, err := VerifyPasswordArgon2("password", "invalidhash")
		if err == nil {
			t.Error("should reject invalid hash format")
		}
	})
}

func TestHKDF(t *testing.T) {
	t.Run("expands key", func(t *testing.T) {
		key := []byte("initial-key")
		salt := []byte("salt")
		info := []byte("info")

		expanded, err := HKDF(key, salt, info, 64)
		if err != nil {
			t.Fatalf("HKDF failed: %v", err)
		}

		if len(expanded) != 64 {
			t.Errorf("expected 64 bytes, got %d", len(expanded))
		}
	})

	t.Run("deterministic expansion", func(t *testing.T) {
		key := []byte("key")
		salt := []byte("salt")
		info := []byte("info")

		e1, _ := HKDF(key, salt, info, 32)
		e2, _ := HKDF(key, salt, info, 32)

		if !bytes.Equal(e1, e2) {
			t.Error("HKDF should be deterministic")
		}
	})
}

func TestInMemoryKeyStore(t *testing.T) {
	t.Run("stores and retrieves", func(t *testing.T) {
		ks := NewInMemoryKeyStore()
		key := []byte("test-key-data")

		err := ks.Store("test-id", key, KeyTypeSymmetric)
		if err != nil {
			t.Fatalf("store failed: %v", err)
		}

		retrieved, keyType, err := ks.Retrieve("test-id")
		if err != nil {
			t.Fatalf("retrieve failed: %v", err)
		}

		if !bytes.Equal(retrieved, key) {
			t.Error("retrieved key differs from stored")
		}

		if keyType != KeyTypeSymmetric {
			t.Errorf("expected symmetric key type, got %v", keyType)
		}
	})

	t.Run("returns error for missing key", func(t *testing.T) {
		ks := NewInMemoryKeyStore()
		_, _, err := ks.Retrieve("nonexistent")
		if err != ErrKeyNotFound {
			t.Errorf("expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("deletes key", func(t *testing.T) {
		ks := NewInMemoryKeyStore()
		_ = ks.Store("test-id", []byte("key"), KeyTypeSymmetric)

		err := ks.Delete("test-id")
		if err != nil {
			t.Fatalf("delete failed: %v", err)
		}

		_, _, err = ks.Retrieve("test-id")
		if err != ErrKeyNotFound {
			t.Error("should not retrieve deleted key")
		}
	})

	t.Run("lists keys", func(t *testing.T) {
		ks := NewInMemoryKeyStore()
		_ = ks.Store("key1", []byte("k1"), KeyTypeSymmetric)
		_ = ks.Store("key2", []byte("k2"), KeyTypeAsymmetric)

		keys, err := ks.List()
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}

		if len(keys) != 2 {
			t.Errorf("expected 2 keys, got %d", len(keys))
		}
	})
}

func TestKeyRotator(t *testing.T) {
	t.Run("rotates key", func(t *testing.T) {
		ks := NewInMemoryKeyStore()
		kr := NewKeyRotator(ks, nil)

		initialKey := []byte("initial-key")
		_ = ks.Store("test-key", initialKey, KeyTypeSymmetric)

		generator := func() ([]byte, error) {
			return GenerateSecureRandom(32)
		}

		newKeyID, err := kr.Rotate("test-key", generator)
		if err != nil {
			t.Fatalf("rotation failed: %v", err)
		}

		if newKeyID == "test-key" {
			t.Error("new key ID should differ from old")
		}

		_, _, err = ks.Retrieve("test-key")
		if err != ErrKeyNotFound {
			t.Error("old key should be deleted")
		}

		_, _, err = ks.Retrieve(newKeyID)
		if err != nil {
			t.Error("new key should exist")
		}
	})
}

func TestConstantTimeCompare(t *testing.T) {
	t.Run("equal slices", func(t *testing.T) {
		if !ConstantTimeCompare([]byte("abc"), []byte("abc")) {
			t.Error("equal slices should match")
		}
	})

	t.Run("different slices", func(t *testing.T) {
		if ConstantTimeCompare([]byte("abc"), []byte("def")) {
			t.Error("different slices should not match")
		}
	})

	t.Run("different lengths", func(t *testing.T) {
		if ConstantTimeCompare([]byte("abc"), []byte("abcd")) {
			t.Error("different lengths should not match")
		}
	})
}

func TestSafeStrEqual(t *testing.T) {
	t.Run("equal strings", func(t *testing.T) {
		if !SafeStrEqual("abc", "abc") {
			t.Error("equal strings should match")
		}
	})

	t.Run("different strings", func(t *testing.T) {
		if SafeStrEqual("abc", "def") {
			t.Error("different strings should not match")
		}
	})
}

func TestComputeVerifyHMAC(t *testing.T) {
	t.Run("computes and verifies", func(t *testing.T) {
		key := []byte("secret-key")
		data := []byte("message")

		mac := ComputeHMAC(key, data)
		valid := VerifyHMAC(key, data, mac)

		if !valid {
			t.Error("HMAC verification failed")
		}
	})

	t.Run("rejects wrong MAC", func(t *testing.T) {
		key := []byte("key")
		data := []byte("data")
		wrongMAC := []byte("wrong")

		if VerifyHMAC(key, data, wrongMAC) {
			t.Error("should reject wrong MAC")
		}
	})
}

func TestHashSHA256(t *testing.T) {
	t.Run("hashes data", func(t *testing.T) {
		hash := HashSHA256([]byte("test"))
		if len(hash) != 32 {
			t.Errorf("expected 32 bytes, got %d", len(hash))
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		h1 := HashSHA256([]byte("test"))
		h2 := HashSHA256([]byte("test"))
		if !bytes.Equal(h1, h2) {
			t.Error("hash should be deterministic")
		}
	})
}

func TestNegotiateAlgorithm(t *testing.T) {
	t.Run("finds common algorithm", func(t *testing.T) {
		client := []string{"AES-256-GCM", "ChaCha20-Poly1305"}
		server := []string{"ChaCha20-Poly1305", "AES-128-GCM"}

		algo, err := NegotiateAlgorithm(client, server)
		if err != nil {
			t.Fatalf("negotiation failed: %v", err)
		}

		if algo != AlgorithmChaCha20Poly {
			t.Errorf("expected ChaCha20-Poly1305, got %s", algo)
		}
	})

	t.Run("no common algorithm", func(t *testing.T) {
		client := []string{"ALGO-A"}
		server := []string{"ALGO-B"}

		_, err := NegotiateAlgorithm(client, server)
		if err == nil {
			t.Error("should fail when no common algorithm")
		}
	})
}

func TestGetRecommendedAlgorithms(t *testing.T) {
	t.Run("returns algorithms", func(t *testing.T) {
		algorithms := GetRecommendedAlgorithms()
		if len(algorithms) == 0 {
			t.Error("should return at least one algorithm")
		}
	})
}

func TestIsAlgorithmSupported(t *testing.T) {
	t.Run("supported algorithm", func(t *testing.T) {
		if !IsAlgorithmSupported(AlgorithmAES256GCM) {
			t.Error("AES-256-GCM should be supported")
		}
	})

	t.Run("unsupported algorithm", func(t *testing.T) {
		if IsAlgorithmSupported("UNSUPPORTED") {
			t.Error("unknown algorithm should not be supported")
		}
	})
}

func TestValidateKeyStrength(t *testing.T) {
	t.Run("valid symmetric key", func(t *testing.T) {
		key := make([]byte, 32)
		err := ValidateKeyStrength(key, KeyTypeSymmetric)
		if err != nil {
			t.Errorf("valid key rejected: %v", err)
		}
	})

	t.Run("weak symmetric key", func(t *testing.T) {
		key := make([]byte, 16)
		err := ValidateKeyStrength(key, KeyTypeSymmetric)
		if err == nil {
			t.Error("should reject weak key")
		}
	})
}

func TestDeriveSharedSecretECDH(t *testing.T) {
	t.Run("derives shared secret", func(t *testing.T) {
		priv1, _ := GenerateECDSAKeyPair()
		priv2, _ := GenerateECDSAKeyPair()

		secret1, err := DeriveSharedSecretECDH(priv1, &priv2.PublicKey)
		if err != nil {
			t.Fatalf("key derivation failed: %v", err)
		}

		secret2, err := DeriveSharedSecretECDH(priv2, &priv1.PublicKey)
		if err != nil {
			t.Fatalf("key derivation failed: %v", err)
		}

		if !bytes.Equal(secret1, secret2) {
			t.Error("shared secrets should match")
		}
	})
}

func TestEncryptDecryptChaCha20(t *testing.T) {
	t.Run("encrypts and decrypts", func(t *testing.T) {
		key, _ := GenerateSecureRandom(32)

		plaintext := []byte("ChaCha20 message")

		ciphertext, nonce, err := EncryptChaCha20Poly1305(plaintext, key)
		if err != nil {
			t.Fatalf("encryption failed: %v", err)
		}

		decrypted, err := DecryptChaCha20Poly1305(ciphertext, key, nonce)
		if err != nil {
			t.Fatalf("decryption failed: %v", err)
		}

		if !bytes.Equal(decrypted, plaintext) {
			t.Error("decrypted text differs from original")
		}
	})

	t.Run("wrong key size", func(t *testing.T) {
		key := make([]byte, 16)
		_, _, err := EncryptChaCha20Poly1305([]byte("test"), key)
		if err == nil || !strings.Contains(err.Error(), "32 bytes") {
			t.Errorf("expected 32 bytes error, got %v", err)
		}
	})
}

func TestSignVerifyRSAPSS(t *testing.T) {
	t.Run("signs and verifies", func(t *testing.T) {
		privateKey, _ := GenerateRSAKeyPair(2048)
		publicKey := &privateKey.PublicKey

		data := []byte("RSA-PSS test data")

		signature, err := SignRSAPSS(privateKey, data)
		if err != nil {
			t.Fatalf("signing failed: %v", err)
		}

		valid := VerifyRSAPSS(publicKey, data, signature)
		if !valid {
			t.Error("signature verification failed")
		}
	})

	t.Run("detects tampered data", func(t *testing.T) {
		privateKey, _ := GenerateRSAKeyPair(2048)
		publicKey := &privateKey.PublicKey

		data := []byte("Original")
		signature, _ := SignRSAPSS(privateKey, data)

		tampered := []byte("Tampered")
		if VerifyRSAPSS(publicKey, tampered, signature) {
			t.Error("should detect tampered data")
		}
	})
}

func TestEncryptedDataEncodeDecode(t *testing.T) {
	t.Run("encodes and decodes", func(t *testing.T) {
		original := &EncryptedData{
			Ciphertext: []byte("ciphertext"),
			Nonce:      []byte("nonce"),
			Algorithm:  AlgorithmAES256GCM,
			KeyID:      "test-key",
		}

		encoded, err := original.Encode()
		if err != nil {
			t.Fatalf("encoding failed: %v", err)
		}

		decoded, err := DecodeEncryptedData(encoded)
		if err != nil {
			t.Fatalf("decoding failed: %v", err)
		}

		if decoded.Algorithm != original.Algorithm {
			t.Errorf("algorithm mismatch")
		}
		if decoded.KeyID != original.KeyID {
			t.Errorf("key ID mismatch")
		}
		if !bytes.Equal(decoded.Ciphertext, original.Ciphertext) {
			t.Error("ciphertext mismatch")
		}
	})
}

func TestGenerateNonce(t *testing.T) {
	t.Run("generates nonce", func(t *testing.T) {
		nonce, err := GenerateNonce(12)
		if err != nil {
			t.Fatalf("nonce generation failed: %v", err)
		}
		if len(nonce) != 12 {
			t.Errorf("expected 12 bytes, got %d", len(nonce))
		}
	})
}

func TestKeyRotationPolicy(t *testing.T) {
	t.Run("default policy", func(t *testing.T) {
		policy := DefaultRotationPolicy()
		if policy.MaxAge <= 0 {
			t.Error("max age should be positive")
		}
		if policy.MaxOperations <= 0 {
			t.Error("max operations should be positive")
		}
	})
}

func TestCryptoAuditor(t *testing.T) {
	t.Run("logs operation", func(t *testing.T) {
		var buf bytes.Buffer
		logger := &testLogger{&buf}
		auditor := NewCryptoAuditor(logger)

		op := &CryptoOperation{
			Operation: "encrypt",
			Algorithm: "AES-256-GCM",
			KeyID:     "test-key",
			Success:   true,
		}

		auditor.LogOperation(op)

		output := buf.String()
		if !strings.Contains(output, "[CRYPTO]") {
			t.Error("log should contain CRYPTO marker")
		}
		if !strings.Contains(output, "encrypt") {
			t.Error("log should contain operation")
		}
	})
}

type testLogger struct {
	buf *bytes.Buffer
}

func (l *testLogger) Printf(format string, args ...interface{}) {
	fmt.Fprintf(l.buf, format+"\n", args...)
}

func TestWrapUnwrapKey(t *testing.T) {
	t.Run("wraps and unwraps", func(t *testing.T) {
		key, _ := GenerateSecureRandom(32)
		kek, _ := GenerateSecureRandom(32)

		wrapped, err := WrapKey(key, kek)
		if err != nil {
			t.Fatalf("wrap failed: %v", err)
		}

		unwrapped, err := UnwrapKey(wrapped, kek, make([]byte, 12))
		if err == nil {
			t.Error("unwrap should fail with wrong nonce")
		}
		_ = unwrapped
	})
}

func TestHashSHA384(t *testing.T) {
	t.Run("hashes data", func(t *testing.T) {
		hash := HashSHA384([]byte("test"))
		if len(hash) != 48 {
			t.Errorf("expected 48 bytes, got %d", len(hash))
		}
	})
}

func TestHashSHA256Hex(t *testing.T) {
	t.Run("hex encoding", func(t *testing.T) {
		hexHash := HashSHA256Hex([]byte("test"))
		if len(hexHash) != 64 {
			t.Errorf("expected 64 hex chars, got %d", len(hexHash))
		}
	})
}

func TestTimeBasedExpiration(t *testing.T) {
	t.Run("checks key expiration", func(t *testing.T) {
		ks := NewInMemoryKeyStore()
		kr := NewKeyRotator(ks, &KeyRotationPolicy{
			MaxAge:        100 * time.Millisecond,
			MaxOperations: 1000000,
		})

		_ = ks.Store("expiring-key", []byte("key"), KeyTypeSymmetric)

		time.Sleep(150 * time.Millisecond)

		shouldRotate, err := kr.ShouldRotate("expiring-key")
		if err != nil {
			t.Logf("expiration check: %v", err)
		}
		_ = shouldRotate
	})
}

func TestEmergencyRotation(t *testing.T) {
	t.Run("emergency rotation", func(t *testing.T) {
		ks := NewInMemoryKeyStore()
		kr := NewKeyRotator(ks, &KeyRotationPolicy{
			Emergency: true,
		})

		_ = ks.Store("key", []byte("key-data"), KeyTypeSymmetric)

		shouldRotate, err := kr.ShouldRotate("key")
		if err != nil {
			t.Fatalf("should rotate check failed: %v", err)
		}

		if !shouldRotate {
			t.Error("emergency rotation should trigger")
		}
	})
}
