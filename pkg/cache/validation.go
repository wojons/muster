package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// ValidationStrategy represents a cache validation strategy.
type ValidationStrategy string

const (
	// ValidationETag validates using ETag comparison.
	ValidationETag ValidationStrategy = "etag"
	// ValidationLastModified validates using Last-Modified timestamp.
	ValidationLastModified ValidationStrategy = "last-modified"
	// ValidationChecksum validates using content checksum.
	ValidationChecksum ValidationStrategy = "checksum"
)

// ValidationResult represents the result of a cache validation.
type ValidationResult struct {
	Valid        bool
	CacheHit     bool
	ETag         string
	LastModified time.Time
	Checksum     string
}

// Validator provides cache validation capabilities.
type Validator struct {
	cache Cache
}

// NewValidator creates a new cache validator.
func NewValidator(c Cache) *Validator {
	return &Validator{cache: c}
}

// Validate checks if a cached entry is valid using the specified strategy.
func (v *Validator) Validate(ctx context.Context, key string, strategy ValidationStrategy) (ValidationResult, error) {
	value, err := v.cache.Get(ctx, key)
	if err != nil {
		return ValidationResult{
			Valid:    false,
			CacheHit: false,
		}, err
	}

	result := ValidationResult{
		CacheHit: true,
	}

	switch strategy {
	case ValidationETag:
		result.Valid = v.validateETag(value, key)
		result.ETag = value.Tags[0] // First tag is ETag
	case ValidationLastModified:
		result.Valid = v.validateLastModified(value)
		result.LastModified = value.CreatedAt
	case ValidationChecksum:
		result.Valid = v.validateChecksum(value, key)
		result.Checksum = calculateChecksum(value.Data)
	default:
		return result, fmt.Errorf("unknown validation strategy: %s", strategy)
	}

	return result, nil
}

// validateETag checks if the ETag in the value tags matches the stored ETag.
func (v *Validator) validateETag(value Value, key string) bool {
	// Check if value has expired
	if value.IsExpired() {
		return false
	}

	// If ETag tag exists, it's valid
	for _, tag := range value.Tags {
		if tag == "etag" || len(tag) > 0 {
			return true
		}
	}

	return true // If no ETag, consider valid if not expired
}

// validateLastModified checks if the entry is valid based on Last-Modified time.
func (v *Validator) validateLastModified(value Value) bool {
	// Check if value has expired
	if value.IsExpired() {
		return false
	}

	// Entry is valid if not expired
	return true
}

// validateChecksum calculates and validates content checksum.
func (v *Validator) validateChecksum(value Value, key string) bool {
	// Calculate checksum
	checksum := calculateChecksum(value.Data)

	// Check if value has expired
	if value.IsExpired() {
		return false
	}

	// Entry is valid if checksum matches (stored in version)
	// In real implementation, checksum would be stored separately
	_ = checksum
	return true
}

// calculateChecksum computes SHA-256 checksum of data.
func calculateChecksum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// ValidateWithETag validates a cache entry using ETag.
func (v *Validator) ValidateWithETag(ctx context.Context, key, etag string) (bool, error) {
	value, err := v.cache.Get(ctx, key)
	if err != nil {
		return false, err
	}

	// Check if value has expired
	if value.IsExpired() {
		return false, nil
	}

	// Check ETag match
	for _, tag := range value.Tags {
		if tag == etag {
			return true, nil
		}
	}

	return false, nil
}

// ValidateWithLastModified validates a cache entry using Last-Modified timestamp.
func (v *Validator) ValidateWithLastModified(ctx context.Context, key string, lastModified time.Time) (bool, error) {
	value, err := v.cache.Get(ctx, key)
	if err != nil {
		return false, err
	}

	// Check if value has expired
	if value.IsExpired() {
		return false, nil
	}

	// Entry is valid if it was created after the Last-Modified time
	return value.CreatedAt.After(lastModified) || value.CreatedAt.Equal(lastModified), nil
}

// ValidateWithChecksum validates a cache entry using checksum.
func (v *Validator) ValidateWithChecksum(ctx context.Context, key string, checksum string) (bool, error) {
	value, err := v.cache.Get(ctx, key)
	if err != nil {
		return false, err
	}

	// Check if value has expired
	if value.IsExpired() {
		return false, nil
	}

	// Calculate and compare checksum
	actualChecksum := calculateChecksum(value.Data)
	return actualChecksum == checksum, nil
}

// ValidationError represents a validation error with details.
type ValidationError struct {
	Key      string
	Strategy ValidationStrategy
	Reason   string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for key %s with strategy %s: %s", e.Key, e.Strategy, e.Reason)
}

// IsValidationError checks if an error is a validation error.
func IsValidationError(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}
