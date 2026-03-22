// Package audit provides security audit logging with tamper-resistant storage,
// compliance tagging, and configurable retention policies for regulatory requirements.
package audit

import (
	"time"
)

// AuditConfig defines configuration for the audit logging system.
// It controls storage, retention, buffering, encryption, and sensitive field handling.
type AuditConfig struct {
	Enabled         bool          `json:"enabled"`
	StorageType     string        `json:"storage_type"`
	RetentionDays   int           `json:"retention_days"`
	BufferSize      int           `json:"buffer_size"`
	FlushInterval   time.Duration `json:"flush_interval"`
	EncryptionKey   string        `json:"encryption_key,omitempty"`
	HashAlgorithm   string        `json:"hash_algorithm"`
	IncludeBody     bool          `json:"include_body"`
	SensitiveFields []string      `json:"sensitive_fields"`
}

// DefaultAuditConfig provides sensible defaults for audit configuration.
var DefaultAuditConfig = AuditConfig{
	Enabled:         true,
	StorageType:     "file",
	RetentionDays:   365,
	BufferSize:      1000,
	FlushInterval:   time.Second * 5,
	HashAlgorithm:   "sha256",
	SensitiveFields: []string{"password", "token", "secret", "credit_card", "api_key", "authorization"},
}

// Validate checks if the audit configuration is valid.
// Returns an error if any configuration values are invalid.
func (c *AuditConfig) Validate() error {
	if c.StorageType == "" {
		c.StorageType = "file"
	}

	if c.StorageType != "file" && c.StorageType != "database" && c.StorageType != "s3" {
		return &ConfigError{
			Field:   "storage_type",
			Message: "storage_type must be one of: file, database, s3",
		}
	}

	if c.RetentionDays < 0 {
		return &ConfigError{
			Field:   "retention_days",
			Message: "retention_days must be >= 0",
		}
	}

	if c.BufferSize < 1 {
		return &ConfigError{
			Field:   "buffer_size",
			Message: "buffer_size must be >= 1",
		}
	}

	if c.HashAlgorithm == "" {
		c.HashAlgorithm = "sha256"
	}

	if c.HashAlgorithm != "sha256" && c.HashAlgorithm != "sha512" {
		return &ConfigError{
			Field:   "hash_algorithm",
			Message: "hash_algorithm must be one of: sha256, sha512",
		}
	}

	if c.FlushInterval <= 0 {
		c.FlushInterval = time.Second * 5
	}

	if len(c.SensitiveFields) == 0 {
		c.SensitiveFields = DefaultAuditConfig.SensitiveFields
	}

	return nil
}

// Clone creates a deep copy of the AuditConfig.
func (c *AuditConfig) Clone() *AuditConfig {
	if c == nil {
		return nil
	}

	sensitiveFields := make([]string, len(c.SensitiveFields))
	copy(sensitiveFields, c.SensitiveFields)

	return &AuditConfig{
		Enabled:         c.Enabled,
		StorageType:     c.StorageType,
		RetentionDays:   c.RetentionDays,
		BufferSize:      c.BufferSize,
		FlushInterval:   c.FlushInterval,
		EncryptionKey:   c.EncryptionKey,
		HashAlgorithm:   c.HashAlgorithm,
		IncludeBody:     c.IncludeBody,
		SensitiveFields: sensitiveFields,
	}
}

// ConfigError represents a validation error for audit configuration.
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "audit config error: " + e.Field + ": " + e.Message
}
