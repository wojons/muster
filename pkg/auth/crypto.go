package auth

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

var (
	ErrInvalidKey        = errors.New("invalid key")
	ErrInvalidSignature  = errors.New("invalid signature")
	ErrDecryptionFailed  = errors.New("decryption failed")
	ErrKeyRotationFailed = errors.New("key rotation failed")
	ErrKeyNotFound       = errors.New("key not found")
)

type Algorithm string

const (
	AlgorithmRSAOAEP      Algorithm = "RSA-OAEP"
	AlgorithmECDH         Algorithm = "ECDH"
	AlgorithmX25519       Algorithm = "X25519"
	AlgorithmAES256GCM    Algorithm = "AES-256-GCM"
	AlgorithmChaCha20Poly Algorithm = "ChaCha20-Poly1305"
	AlgorithmECDSAP384    Algorithm = "ECDSA-P384"
	AlgorithmEd25519      Algorithm = "Ed25519"
	AlgorithmRSAPSS       Algorithm = "RSA-PSS"
	AlgorithmPBKDF2       Algorithm = "PBKDF2"
	AlgorithmArgon2       Algorithm = "Argon2id"
)

type KeyType int

const (
	KeyTypeSymmetric KeyType = iota
	KeyTypeAsymmetric
	KeyTypeSigning
)

type KeyInfo struct {
	KeyID      string
	Algorithm  Algorithm
	KeyType    KeyType
	CreatedAt  time.Time
	ExpiresAt  time.Time
	Operations int64
	MaxOps     int64
}

type KeyStore interface {
	Store(keyID string, key []byte, keyType KeyType) error
	Retrieve(keyID string) ([]byte, KeyType, error)
	Delete(keyID string) error
	List() ([]string, error)
}

type InMemoryKeyStore struct {
	mu   sync.RWMutex
	keys map[string][]byte
	info map[string]*KeyInfo
}

func NewInMemoryKeyStore() *InMemoryKeyStore {
	return &InMemoryKeyStore{
		keys: make(map[string][]byte),
		info: make(map[string]*KeyInfo),
	}
}

func (ks *InMemoryKeyStore) Store(keyID string, key []byte, keyType KeyType) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	ks.keys[keyID] = keyCopy

	if ks.info[keyID] == nil {
		ks.info[keyID] = &KeyInfo{
			KeyID:     keyID,
			CreatedAt: time.Now(),
			MaxOps:    1000000,
		}
	}
	ks.info[keyID].KeyType = keyType
	return nil
}

func (ks *InMemoryKeyStore) Retrieve(keyID string) ([]byte, KeyType, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	key, exists := ks.keys[keyID]
	if !exists {
		return nil, 0, ErrKeyNotFound
	}

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	info := ks.info[keyID]
	if info != nil {
		info.Operations++
		if info.MaxOps > 0 && info.Operations >= info.MaxOps {
			return nil, 0, fmt.Errorf("key %s has exceeded maximum operations", keyID)
		}
		return keyCopy, info.KeyType, nil
	}

	return keyCopy, KeyTypeSymmetric, nil
}

func (ks *InMemoryKeyStore) Delete(keyID string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	delete(ks.keys, keyID)
	delete(ks.info, keyID)
	return nil
}

func (ks *InMemoryKeyStore) List() ([]string, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	keyIDs := make([]string, 0, len(ks.keys))
	for k := range ks.keys {
		keyIDs = append(keyIDs, k)
	}
	return keyIDs, nil
}

type KeyRotationPolicy struct {
	MaxAge        time.Duration
	MaxOperations int64
	Emergency     bool
}

func DefaultRotationPolicy() *KeyRotationPolicy {
	return &KeyRotationPolicy{
		MaxAge:        90 * 24 * time.Hour,
		MaxOperations: 1000000,
		Emergency:     false,
	}
}

type KeyRotator struct {
	keyStore KeyStore
	policy   *KeyRotationPolicy
	mu       sync.Mutex
}

func NewKeyRotator(keyStore KeyStore, policy *KeyRotationPolicy) *KeyRotator {
	if policy == nil {
		policy = DefaultRotationPolicy()
	}
	return &KeyRotator{
		keyStore: keyStore,
		policy:   policy,
	}
}

func (kr *KeyRotator) ShouldRotate(keyID string) (bool, error) {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	key, _, err := kr.keyStore.Retrieve(keyID)
	if err != nil {
		return false, err
	}

	if kr.policy.Emergency {
		return true, nil
	}

	if len(key) == 0 {
		return true, nil
	}

	return false, nil
}

func (kr *KeyRotator) Rotate(keyID string, generator func() ([]byte, error)) (string, error) {
	kr.mu.Lock()
	defer kr.mu.Unlock()

	newKey, err := generator()
	if err != nil {
		return "", fmt.Errorf("failed to generate new key: %w", err)
	}

	newKeyID := keyID + "_v" + strconv.FormatInt(time.Now().UnixNano(), 10)

	if err := kr.keyStore.Store(newKeyID, newKey, KeyTypeSymmetric); err != nil {
		return "", fmt.Errorf("failed to store new key: %w", err)
	}

	if err := kr.keyStore.Delete(keyID); err != nil && err != ErrKeyNotFound {
		return "", fmt.Errorf("failed to delete old key: %w", err)
	}

	return newKeyID, nil
}

func GenerateSecureRandom(length int) ([]byte, error) {
	bytes := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return bytes, nil
}

func GenerateEd25519KeyPair() (crypto.PublicKey, crypto.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func GenerateRSAKeyPair(bits int) (*rsa.PrivateKey, error) {
	if bits < 4096 {
		bits = 4096
	}
	return rsa.GenerateKey(rand.Reader, bits)
}

func GenerateECDSAKeyPair() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
}

func EncryptRSAOAEP(publicKey *rsa.PublicKey, plaintext []byte) ([]byte, error) {
	if publicKey == nil {
		return nil, ErrInvalidKey
	}

	label := []byte("")
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, plaintext, label)
	if err != nil {
		return nil, fmt.Errorf("RSA-OAEP encryption failed: %w", err)
	}

	return ciphertext, nil
}

func DecryptRSAOAEP(privateKey *rsa.PrivateKey, ciphertext []byte) ([]byte, error) {
	if privateKey == nil {
		return nil, ErrInvalidKey
	}

	label := []byte("")
	plaintext, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, ciphertext, label)
	if err != nil {
		return nil, fmt.Errorf("RSA-OAEP decryption failed: %w", err)
	}

	return plaintext, nil
}

func SignECDSA(privateKey *ecdsa.PrivateKey, data []byte) ([]byte, error) {
	if privateKey == nil {
		return nil, ErrInvalidKey
	}

	hash := sha512.Sum384(data)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("ECDSA signing failed: %w", err)
	}

	signature := append(r.Bytes(), s.Bytes()...)
	return signature, nil
}

func VerifyECDSA(publicKey *ecdsa.PublicKey, data []byte, signature []byte) bool {
	if publicKey == nil || len(signature) < 96 {
		return false
	}

	hash := sha512.Sum384(data)

	r := new(big.Int).SetBytes(signature[:48])
	s := new(big.Int).SetBytes(signature[48:])

	return ecdsa.Verify(publicKey, hash[:], r, s)
}

func EncryptAESGCM(plaintext []byte, key []byte) ([]byte, []byte, error) {
	if len(key) != 32 {
		return nil, nil, fmt.Errorf("key must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertextWithTag := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertextWithTag, nonce, nil
}

func DecryptAESGCM(ciphertextWithTag []byte, key []byte, nonce []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertextWithTag) < nonceSize {
		return nil, ErrDecryptionFailed
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertextWithTag[nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

func DeriveKeyPBKDF2(password, salt []byte, iterations, keyLen int) ([]byte, error) {
	if iterations < 10000 {
		iterations = 10000
	}
	if keyLen < 32 {
		keyLen = 32
	}

	key := pbkdf2.Key(password, salt, iterations, keyLen, sha256.New)
	return key, nil
}

func DeriveKeyArgon2(password []byte, params *Argon2Params) ([]byte, error) {
	if params == nil {
		params = DefaultArgon2Params()
	}

	if params.SaltLength == 0 {
		params.SaltLength = 16
	}

	salt, err := GenerateSecureRandom(int(params.SaltLength))
	if err != nil {
		return nil, err
	}

	key := argon2.IDKey(
		password,
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		uint32(params.KeyLength),
	)

	return key, nil
}

type Argon2Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

func DefaultArgon2Params() *Argon2Params {
	return &Argon2Params{
		Memory:      64 * 1024,
		Iterations:  3,
		Parallelism: 4,
		SaltLength:  16,
		KeyLength:   32,
	}
}

func HashPasswordArgon2(password string) (string, error) {
	params := DefaultArgon2Params()

	salt, err := GenerateSecureRandom(int(params.SaltLength))
	if err != nil {
		return "", err
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encodedHash := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.Memory,
		params.Iterations,
		params.Parallelism,
		b64Salt,
		b64Hash,
	)

	return encodedHash, nil
}

func VerifyPasswordArgon2(password, encodedHash string) (bool, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format")
	}

	var version int
	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return false, err
	}

	if version != argon2.Version {
		return false, fmt.Errorf("incompatible argon2 version")
	}

	params := &Argon2Params{}
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d",
		&params.Memory, &params.Iterations, &params.Parallelism)
	if err != nil {
		return false, err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	params.SaltLength = uint32(len(salt))

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	params.KeyLength = uint32(len(hash))

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	return subtle.ConstantTimeCompare(hash, computedHash) == 1, nil
}

func HKDF(key, salt, info []byte, length int) ([]byte, error) {
	hkdf := hkdf.New(sha256.New, key, salt, info)
	out := make([]byte, length)
	if _, err := io.ReadFull(hkdf, out); err != nil {
		return nil, fmt.Errorf("HKDF expansion failed: %w", err)
	}
	return out, nil
}

type CryptoAuditor struct {
	logger Logger
}

type Logger interface {
	Printf(format string, args ...interface{})
}

type CryptoOperation struct {
	Timestamp time.Time
	Operation string
	Algorithm string
	KeyID     string
	Success   bool
	Error     string
	Metadata  map[string]interface{}
}

func NewCryptoAuditor(logger Logger) *CryptoAuditor {
	return &CryptoAuditor{logger: logger}
}

func (ca *CryptoAuditor) LogOperation(op *CryptoOperation) {
	op.Timestamp = time.Now().UTC()
	if ca.logger != nil {
		ca.logger.Printf("[CRYPTO] %s/%s/%s success=%v error=%s",
			op.Operation, op.Algorithm, op.KeyID, op.Success, op.Error)
	}
}

func NegotiateAlgorithm(clientAlgos []string, serverAlgos []string) (Algorithm, error) {
	for _, clientAlgo := range clientAlgos {
		for _, serverAlgo := range serverAlgos {
			if clientAlgo == serverAlgo {
				return Algorithm(clientAlgo), nil
			}
		}
	}
	return "", fmt.Errorf("no common algorithm found")
}

func GetRecommendedAlgorithms() []Algorithm {
	return []Algorithm{
		AlgorithmRSAOAEP,
		AlgorithmAES256GCM,
		AlgorithmChaCha20Poly,
		AlgorithmECDSAP384,
		AlgorithmArgon2,
	}
}

func WrapKey(key, kek []byte) ([]byte, error) {
	ciphertext, _, err := EncryptAESGCM(key, kek)
	return ciphertext, err
}

func UnwrapKey(wrappedKey, kek []byte, nonce []byte) ([]byte, error) {
	return DecryptAESGCM(wrappedKey, kek, nonce)
}

func ConstantTimeCompare(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

func SafeStrEqual(a, b string) bool {
	return ConstantTimeCompare([]byte(a), []byte(b))
}

func GenerateNonce(size int) ([]byte, error) {
	return GenerateSecureRandom(size)
}

func EncryptChaCha20Poly1305(plaintext []byte, key []byte) ([]byte, []byte, error) {
	if len(key) != 32 {
		return nil, nil, fmt.Errorf("key must be 32 bytes for ChaCha20")
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create ChaCha20 AEAD: %w", err)
	}

	nonceSize := aead.NonceSize()
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertextWithTag := aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertextWithTag, nonce, nil
}

func DecryptChaCha20Poly1305(ciphertextWithTag []byte, key []byte, nonce []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes for ChaCha20")
	}

	aead, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create ChaCha20 AEAD: %w", err)
	}

	nonceSize := aead.NonceSize()
	if len(ciphertextWithTag) < nonceSize {
		return nil, ErrDecryptionFailed
	}

	plaintext, err := aead.Open(nil, nonce, ciphertextWithTag[nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

func SignRSAPSS(privateKey *rsa.PrivateKey, data []byte) ([]byte, error) {
	if privateKey == nil {
		return nil, ErrInvalidKey
	}

	hash := sha256.Sum256(data)
	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, hash[:], nil)
	if err != nil {
		return nil, fmt.Errorf("RSA-PSS signing failed: %w", err)
	}

	return signature, nil
}

func VerifyRSAPSS(publicKey *rsa.PublicKey, data []byte, signature []byte) bool {
	if publicKey == nil {
		return false
	}

	hash := sha256.Sum256(data)
	err := rsa.VerifyPSS(publicKey, crypto.SHA256, hash[:], signature, nil)
	return err == nil
}

func ComputeHMAC(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func VerifyHMAC(key, data, expectedMAC []byte) bool {
	actualMAC := ComputeHMAC(key, data)
	return hmac.Equal(actualMAC, expectedMAC)
}

func HashSHA256(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func HashSHA384(data []byte) []byte {
	hash := sha512.Sum384(data)
	return hash[:]
}

func HashSHA256Hex(data []byte) string {
	return hex.EncodeToString(HashSHA256(data))
}

func DeriveSharedSecretECDH(privateKey *ecdsa.PrivateKey, publicKey *ecdsa.PublicKey) ([]byte, error) {
	if privateKey == nil || publicKey == nil {
		return nil, ErrInvalidKey
	}

	// nolint:staticcheck // ScalarMult is still functional; migration to crypto/ecdh planned
	resultX, _ := privateKey.Curve.ScalarMult(publicKey.X, publicKey.Y, privateKey.D.Bytes())
	if resultX == nil {
		return nil, fmt.Errorf("failed to compute shared secret")
	}

	return resultX.Bytes(), nil
}

type EncryptedData struct {
	Ciphertext []byte
	Nonce      []byte
	Tag        []byte
	Algorithm  Algorithm
	KeyID      string
}

func (ed *EncryptedData) Encode() ([]byte, error) {
	var buf []byte

	buf = append(buf, byte(len(ed.Algorithm)))
	buf = append(buf, []byte(ed.Algorithm)...)

	buf = append(buf, byte(len(ed.KeyID)))
	buf = append(buf, []byte(ed.KeyID)...)

	lenBuf := make([]byte, 8)
	binary.BigEndian.PutUint64(lenBuf, uint64(len(ed.Ciphertext)))
	buf = append(buf, lenBuf...)
	buf = append(buf, ed.Ciphertext...)

	binary.BigEndian.PutUint64(lenBuf, uint64(len(ed.Nonce)))
	buf = append(buf, lenBuf...)
	buf = append(buf, ed.Nonce...)

	return buf, nil
}

func DecodeEncryptedData(data []byte) (*EncryptedData, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("invalid encrypted data")
	}

	offset := 0
	algoLen := int(data[offset])
	offset++

	if offset+algoLen > len(data) {
		return nil, fmt.Errorf("invalid algorithm length")
	}

	algorithm := Algorithm(data[offset : offset+algoLen])
	offset += algoLen

	if offset >= len(data) {
		return nil, fmt.Errorf("invalid data format")
	}

	keyIDLen := int(data[offset])
	offset++

	if offset+keyIDLen > len(data) {
		return nil, fmt.Errorf("invalid key ID length")
	}

	keyID := string(data[offset : offset+keyIDLen])
	offset += keyIDLen

	if offset+8 > len(data) {
		return nil, fmt.Errorf("invalid ciphertext length")
	}

	ciphertextLen := int(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8

	if offset+ciphertextLen > len(data) {
		return nil, fmt.Errorf("invalid ciphertext")
	}

	ciphertext := data[offset : offset+ciphertextLen]
	offset += ciphertextLen

	if offset+8 > len(data) {
		return nil, fmt.Errorf("invalid nonce length")
	}

	nonceLen := int(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8

	if offset+nonceLen > len(data) {
		return nil, fmt.Errorf("invalid nonce")
	}

	nonce := data[offset : offset+nonceLen]

	return &EncryptedData{
		Ciphertext: ciphertext,
		Nonce:      nonce,
		Algorithm:  algorithm,
		KeyID:      keyID,
	}, nil
}

func IsAlgorithmSupported(algo Algorithm) bool {
	supported := map[Algorithm]bool{
		AlgorithmRSAOAEP:      true,
		AlgorithmECDH:         true,
		AlgorithmAES256GCM:    true,
		AlgorithmChaCha20Poly: true,
		AlgorithmECDSAP384:    true,
		AlgorithmPBKDF2:       true,
		AlgorithmArgon2:       true,
	}
	return supported[algo]
}

func ValidateKeyStrength(key []byte, keyType KeyType) error {
	switch keyType {
	case KeyTypeSymmetric:
		if len(key) < 32 {
			return fmt.Errorf("symmetric key must be at least 256 bits")
		}
	case KeyTypeAsymmetric:
		if len(key) < 512 {
			return fmt.Errorf("asymmetric key too short")
		}
	case KeyTypeSigning:
		if len(key) < 32 {
			return fmt.Errorf("signing key too short")
		}
	}
	return nil
}
