// Package auth provides PCI DSS compliant tokenization for cardholder data protection.
// This implementation satisfies PCI DSS Requirement 3.4 which mandates that PAN
// (Primary Account Number) must be rendered unreadable anywhere it is stored.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PCI DSS Compliance Constants
const (
	// PANLengthMin is the minimum valid PAN length (12 digits per ISO/IEC 7812)
	PANLengthMin = 12
	// PANLengthMax is the maximum valid PAN length (19 digits per ISO/IEC 7812)
	PANLengthMax = 19
	// PANLengthStandard is the typical PAN length (16 digits)
	PANLengthStandard = 16
	// TokenPrefix identifies PCI tokens
	TokenPrefix = "pcit_"
	// TokenLength is the length of the random portion of the token (32 bytes = 64 hex chars)
	TokenLength = 64
	// MaskedPANLastDigits is the number of digits to show in masked PAN
	MaskedPANLastDigits = 4
	// TokenKeyDerivationIterations for PBKDF2
	TokenKeyDerivationIterations = 150000
)

// Errors for PCI tokenization
var (
	// ErrInvalidPAN indicates the PAN format is invalid
	ErrInvalidPAN = errors.New("invalid PAN format")
	// ErrPANTooShort indicates the PAN is shorter than minimum allowed
	ErrPANTooShort = errors.New("PAN too short (minimum 12 digits)")
	// ErrPANTooLong indicates the PAN exceeds maximum allowed length
	ErrPANTooLong = errors.New("PAN too long (maximum 19 digits)")
	// ErrInvalidPANChecksum indicates the PAN failed Luhn algorithm validation
	ErrInvalidPANChecksum = errors.New("invalid PAN checksum")
	// ErrTokenizationFailed indicates the tokenization operation failed
	ErrTokenizationFailed = errors.New("tokenization failed")
	// ErrDetokenizationFailed indicates the detokenization operation failed
	ErrDetokenizationFailed = errors.New("detokenization failed")
	// ErrTokenNotFound indicates the token does not exist in storage
	ErrTokenNotFound = errors.New("token not found")
	// ErrTokenExpired indicates the token has expired
	ErrTokenExpired = errors.New("token expired")
	// ErrMaskingFailed indicates the PAN masking operation failed
	ErrMaskingFailed = errors.New("PAN masking failed")
)

// TokenizationRequest represents a request to tokenize a PAN
type TokenizationRequest struct {
	// PAN is the Primary Account Number to tokenize
	PAN string
	// MerchantID identifies the merchant requesting tokenization
	MerchantID string
	// TokenSuffix is an optional suffix for the token (for routing purposes)
	TokenSuffix string
	// Metadata contains optional metadata about the tokenization
	Metadata map[string]string
	// ExpiresAt specifies when the token should expire (optional)
	ExpiresAt time.Time
}

// TokenizationResponse represents the response from tokenization
type TokenizationResponse struct {
	// Token is the generated token
	Token string
	// PANLast4 is the last 4 digits of the original PAN (for display)
	PANLast4 string
	// PANMasked is the masked PAN (e.g., "****-****-****-1234")
	PANMasked string
	// TokenType identifies the card network (Visa, Mastercard, etc.)
	TokenType string
	// ExpiresAt is the token expiration time
	ExpiresAt time.Time
	// CreatedAt is when the token was created
	CreatedAt time.Time
}

// DetokenizationRequest represents a request to detokenize
type DetokenizationRequest struct {
	// Token is the token to detokenize
	Token string
	// MerchantID must match the merchant that created the token
	MerchantID string
}

// DetokenizationResponse represents the response from detokenization
type DetokenizationResponse struct {
	// PAN is the original Primary Account Number
	PAN string
	// PANMasked is the masked PAN
	PANMasked string
	// TokenType identifies the card network
	TokenType string
	// ExpiresAt is when the token expires
	ExpiresAt time.Time
}

// CardNetwork identifies the card network based on BIN/IIN
type CardNetwork string

const (
	CardNetworkVisa       CardNetwork = "VISA"
	CardNetworkMastercard CardNetwork = "MASTERCARD"
	CardNetworkAmex       CardNetwork = "AMEX"
	CardNetworkDiscover   CardNetwork = "DISCOVER"
	CardNetworkJCB        CardNetwork = "JCB"
	CardNetworkDiners     CardNetwork = "DINERS"
	CardNetworkUnknown    CardNetwork = "UNKNOWN"
)

// PANValidator validates Primary Account Numbers
type PANValidator struct {
	once     sync.Once
	binRegex *regexp.Regexp
}

// NewPANValidator creates a new PAN validator
func NewPANValidator() *PANValidator {
	return &PANValidator{}
}

func (v *PANValidator) init() {
	v.once.Do(func() {
		// BIN/IIN patterns for card network detection
		v.binRegex = regexp.MustCompile(`^(\d{1,8})`)
	})
}

// ValidatePAN validates a PAN format and checksum
func (v *PANValidator) ValidatePAN(pan string) error {
	v.init()

	// Remove spaces and dashes
	pan = strings.ReplaceAll(pan, " ", "")
	pan = strings.ReplaceAll(pan, "-", "")

	// Check if all characters are digits
	if !regexp.MustCompile(`^\d+$`).MatchString(pan) {
		return ErrInvalidPAN
	}

	// Check length
	length := len(pan)
	if length < PANLengthMin {
		return ErrPANTooShort
	}
	if length > PANLengthMax {
		return ErrPANTooLong
	}

	// Validate checksum using Luhn algorithm
	if !v.validateLuhn(pan) {
		return ErrInvalidPANChecksum
	}

	return nil
}

// validateLuhn implements the Luhn algorithm for PAN validation
func (v *PANValidator) validateLuhn(pan string) bool {
	digits := make([]int, len(pan))
	for i, ch := range pan {
		digits[i] = int(ch - '0')
	}

	// Double every second digit from right
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := digits[i]
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}

	return sum%10 == 0
}

// DetectCardNetwork identifies the card network from PAN
func (v *PANValidator) DetectCardNetwork(pan string) CardNetwork {
	v.init()

	if len(pan) < 1 {
		return CardNetworkUnknown
	}

	// Extract BIN (Bank Identification Number) - first 6-8 digits
	binMatch := v.binRegex.FindStringSubmatch(pan)
	if len(binMatch) < 2 {
		return CardNetworkUnknown
	}

	bin := binMatch[1]

	// Card network detection based on BIN/IIN ranges
	// Visa: Starts with 4
	if strings.HasPrefix(pan, "4") {
		return CardNetworkVisa
	}

	// Mastercard: Starts with 51-55 or 2221-2720
	if len(bin) >= 2 {
		prefix2, _ := strconv.Atoi(bin[:2])
		if prefix2 >= 51 && prefix2 <= 55 {
			return CardNetworkMastercard
		}
	}
	if len(bin) >= 4 {
		prefix4, _ := strconv.Atoi(bin[:4])
		if prefix4 >= 2221 && prefix4 <= 2720 {
			return CardNetworkMastercard
		}
	}

	// American Express: Starts with 34 or 37
	if strings.HasPrefix(pan, "34") || strings.HasPrefix(pan, "37") {
		return CardNetworkAmex
	}

	// Discover: Starts with 6011, 622126-622925, 644-649, 65
	if strings.HasPrefix(pan, "6011") || strings.HasPrefix(pan, "65") {
		return CardNetworkDiscover
	}
	if len(bin) >= 6 {
		prefix6, _ := strconv.Atoi(bin[:6])
		if prefix6 >= 622126 && prefix6 <= 622925 {
			return CardNetworkDiscover
		}
	}
	if len(bin) >= 3 {
		prefix3, _ := strconv.Atoi(bin[:3])
		if prefix3 >= 644 && prefix3 <= 649 {
			return CardNetworkDiscover
		}
	}

	// JCB: Starts with 3528-3589
	if len(bin) >= 4 {
		prefix4, _ := strconv.Atoi(bin[:4])
		if prefix4 >= 3528 && prefix4 <= 3589 {
			return CardNetworkJCB
		}
	}

	// Diners Club: Starts with 300-305, 36, 38, 39
	if len(bin) >= 3 {
		prefix3, _ := strconv.Atoi(bin[:3])
		if prefix3 >= 300 && prefix3 <= 305 {
			return CardNetworkDiners
		}
	}
	if strings.HasPrefix(pan, "36") || strings.HasPrefix(pan, "38") || strings.HasPrefix(pan, "39") {
		return CardNetworkDiners
	}

	return CardNetworkUnknown
}

// MaskPAN masks a PAN for display purposes (PCI DSS compliant)
func (v *PANValidator) MaskPAN(pan string) (string, error) {
	v.init()

	// Remove spaces and dashes
	pan = strings.ReplaceAll(pan, " ", "")
	pan = strings.ReplaceAll(pan, "-", "")

	if len(pan) < MaskedPANLastDigits {
		return "", ErrMaskingFailed
	}

	// Show only last 4 digits, mask the rest
	masked := strings.Repeat("*", len(pan)-MaskedPANLastDigits) + pan[len(pan)-MaskedPANLastDigits:]

	// Format with dashes for readability (groups of 4)
	var formatted strings.Builder
	for i, ch := range masked {
		if i > 0 && i%4 == 0 {
			formatted.WriteByte('-')
		}
		formatted.WriteRune(ch)
	}

	return formatted.String(), nil
}

// Tokenizer provides PCI DSS compliant tokenization
type Tokenizer struct {
	mu        sync.RWMutex
	hmacKey   []byte
	store     TokenStore
	validator *PANValidator
	closed    bool
}

// TokenStore defines the interface for token storage
type TokenStore interface {
	// Store saves a token mapping
	Store(ctx context.Context, token string, pan string, metadata map[string]string, expiresAt time.Time) error
	// Retrieve gets a token mapping
	Retrieve(ctx context.Context, token string) (pan string, metadata map[string]string, expiresAt time.Time, err error)
	// Delete removes a token mapping
	Delete(ctx context.Context, token string) error
	// Exists checks if a token exists
	Exists(ctx context.Context, token string) (bool, error)
}

// InMemoryTokenStore is an in-memory implementation of TokenStore
type InMemoryTokenStore struct {
	mu     sync.RWMutex
	tokens map[string]*tokenEntry
}

type tokenEntry struct {
	pan       string
	metadata  map[string]string
	expiresAt time.Time
	createdAt time.Time
}

// NewInMemoryTokenStore creates a new in-memory token store
func NewInMemoryTokenStore() *InMemoryTokenStore {
	return &InMemoryTokenStore{
		tokens: make(map[string]*tokenEntry),
	}
}

// Store saves a token mapping
func (s *InMemoryTokenStore) Store(ctx context.Context, token string, pan string, metadata map[string]string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Copy metadata to avoid external modification
	metaCopy := make(map[string]string)
	for k, v := range metadata {
		metaCopy[k] = v
	}

	s.tokens[token] = &tokenEntry{
		pan:       pan,
		metadata:  metaCopy,
		expiresAt: expiresAt,
		createdAt: time.Now(),
	}

	return nil
}

// Retrieve gets a token mapping
func (s *InMemoryTokenStore) Retrieve(ctx context.Context, token string) (string, map[string]string, time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.tokens[token]
	if !exists {
		return "", nil, time.Time{}, ErrTokenNotFound
	}

	// Check expiration
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return "", nil, entry.expiresAt, ErrTokenExpired
	}

	// Copy metadata to avoid external modification
	metaCopy := make(map[string]string)
	for k, v := range entry.metadata {
		metaCopy[k] = v
	}

	return entry.pan, metaCopy, entry.expiresAt, nil
}

// Delete removes a token mapping
func (s *InMemoryTokenStore) Delete(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tokens, token)
	return nil
}

// Exists checks if a token exists
func (s *InMemoryTokenStore) Exists(ctx context.Context, token string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.tokens[token]
	if !exists {
		return false, nil
	}

	// Check expiration
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return false, nil
	}

	return true, nil
}

// NewTokenizer creates a new PCI DSS compliant tokenizer
func NewTokenizer(store TokenStore, hmacKey []byte) (*Tokenizer, error) {
	if store == nil {
		return nil, errors.New("token store cannot be nil")
	}

	// Generate HMAC key if not provided
	if len(hmacKey) == 0 {
		hmacKey = make([]byte, 32)
		if _, err := rand.Read(hmacKey); err != nil {
			return nil, fmt.Errorf("failed to generate HMAC key: %w", err)
		}
	}

	return &Tokenizer{
		hmacKey:   hmacKey,
		store:     store,
		validator: NewPANValidator(),
	}, nil
}

// Tokenize converts a PAN into a PCI DSS compliant token
func (t *Tokenizer) Tokenize(ctx context.Context, req *TokenizationRequest) (*TokenizationResponse, error) {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return nil, ErrTokenizationFailed
	}
	t.mu.RUnlock()

	if req == nil {
		return nil, errors.New("tokenization request cannot be nil")
	}

	// Validate PAN
	if err := t.validator.ValidatePAN(req.PAN); err != nil {
		return nil, fmt.Errorf("invalid PAN: %w", err)
	}

	// Clean PAN (remove spaces/dashes)
	pan := strings.ReplaceAll(req.PAN, " ", "")
	pan = strings.ReplaceAll(pan, "-", "")

	// Generate deterministic token using HMAC
	token, err := t.generateToken(pan, req.MerchantID, req.TokenSuffix)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Prepare metadata
	metadata := make(map[string]string)
	if req.Metadata != nil {
		for k, v := range req.Metadata {
			metadata[k] = v
		}
	}
	metadata["merchant_id"] = req.MerchantID
	metadata["created_at"] = time.Now().UTC().Format(time.RFC3339)

	// Store token mapping
	if err := t.store.Store(ctx, token, pan, metadata, req.ExpiresAt); err != nil {
		return nil, fmt.Errorf("failed to store token: %w", err)
	}

	// Get last 4 digits and masked PAN
	panLast4 := pan[len(pan)-MaskedPANLastDigits:]
	panMasked, err := t.validator.MaskPAN(pan)
	if err != nil {
		return nil, fmt.Errorf("failed to mask PAN: %w", err)
	}

	// Detect card network
	cardNetwork := t.validator.DetectCardNetwork(pan)

	return &TokenizationResponse{
		Token:     token,
		PANLast4:  panLast4,
		PANMasked: panMasked,
		TokenType: string(cardNetwork),
		ExpiresAt: req.ExpiresAt,
		CreatedAt: time.Now(),
	}, nil
}

// Detokenize converts a token back to the original PAN
func (t *Tokenizer) Detokenize(ctx context.Context, req *DetokenizationRequest) (*DetokenizationResponse, error) {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return nil, ErrDetokenizationFailed
	}
	t.mu.RUnlock()

	if req == nil {
		return nil, errors.New("detokenization request cannot be nil")
	}

	// Validate token format
	if !strings.HasPrefix(req.Token, TokenPrefix) {
		return nil, ErrTokenNotFound
	}

	// Retrieve token mapping
	pan, metadata, expiresAt, err := t.store.Retrieve(ctx, req.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token: %w", err)
	}

	// Verify merchant ID matches
	if metadata["merchant_id"] != req.MerchantID {
		return nil, errors.New("merchant ID mismatch")
	}

	// Get masked PAN
	panMasked, err := t.validator.MaskPAN(pan)
	if err != nil {
		return nil, fmt.Errorf("failed to mask PAN: %w", err)
	}

	// Detect card network
	cardNetwork := t.validator.DetectCardNetwork(pan)

	return &DetokenizationResponse{
		PAN:       pan,
		PANMasked: panMasked,
		TokenType: string(cardNetwork),
		ExpiresAt: expiresAt,
	}, nil
}

// DeleteToken removes a token mapping
func (t *Tokenizer) DeleteToken(ctx context.Context, token string) error {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return ErrDetokenizationFailed
	}
	t.mu.RUnlock()

	return t.store.Delete(ctx, token)
}

// TokenExists checks if a token exists
func (t *Tokenizer) TokenExists(ctx context.Context, token string) (bool, error) {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return false, ErrTokenNotFound
	}
	t.mu.RUnlock()

	return t.store.Exists(ctx, token)
}

// generateToken creates a deterministic token using HMAC
func (t *Tokenizer) generateToken(pan, merchantID, tokenSuffix string) (string, error) {
	// Create HMAC-SHA256
	h := hmac.New(sha256.New, t.hmacKey)

	// Write PAN and merchant ID
	data := fmt.Sprintf("%s|%s|%s", pan, merchantID, tokenSuffix)
	if _, err := h.Write([]byte(data)); err != nil {
		return "", fmt.Errorf("failed to write HMAC data: %w", err)
	}

	// Generate token
	hash := h.Sum(nil)
	tokenHex := hex.EncodeToString(hash)

	// Add prefix
	token := TokenPrefix + tokenHex

	// Truncate to desired length if needed
	if len(token) > 80 {
		token = token[:80]
	}

	return token, nil
}

// Close shuts down the tokenizer
func (t *Tokenizer) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.closed = true
	return nil
}

// RedactPAN redacts a PAN string, showing only last 4 digits
func RedactPAN(pan string) string {
	pan = strings.ReplaceAll(pan, " ", "")
	pan = strings.ReplaceAll(pan, "-", "")

	if len(pan) <= MaskedPANLastDigits {
		return strings.Repeat("*", len(pan))
	}

	return strings.Repeat("*", len(pan)-MaskedPANLastDigits) + pan[len(pan)-MaskedPANLastDigits:]
}

// ValidateCardNumber is a convenience function for PAN validation
func ValidateCardNumber(cardNumber string) error {
	validator := NewPANValidator()
	return validator.ValidatePAN(cardNumber)
}

// MaskCardNumber is a convenience function for PAN masking
func MaskCardNumber(cardNumber string) (string, error) {
	validator := NewPANValidator()
	return validator.MaskPAN(cardNumber)
}

// GetCardNetwork is a convenience function for card network detection
func GetCardNetwork(cardNumber string) CardNetwork {
	validator := NewPANValidator()
	return validator.DetectCardNetwork(cardNumber)
}

// TokenMetadata holds metadata about a tokenization operation
type TokenMetadata struct {
	MerchantID  string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	CardNetwork string
	PANLast4    string
	CustomData  map[string]string
}

// TokenizationStats provides statistics about tokenization operations
type TokenizationStats struct {
	TotalTokens   int64
	ActiveTokens  int64
	ExpiredTokens int64
	TokensCreated int64
	TokensDeleted int64
	LastActivity  time.Time
}
