package audit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuditConfig_Validate(t *testing.T) {
	t.Run("Valid Config", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:         true,
			StorageType:     "file",
			RetentionDays:   365,
			BufferSize:      1000,
			FlushInterval:   time.Second * 5,
			HashAlgorithm:   "sha256",
			SensitiveFields: []string{"password", "token"},
		}

		err := config.Validate()
		require.NoError(t, err)
	})

	t.Run("Valid Config with Database Storage", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:         true,
			StorageType:     "database",
			RetentionDays:   30,
			BufferSize:      500,
			FlushInterval:   time.Second * 10,
			HashAlgorithm:   "sha512",
			SensitiveFields: []string{"secret", "api_key"},
		}

		err := config.Validate()
		require.NoError(t, err)
	})

	t.Run("Valid Config with S3 Storage", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:         true,
			StorageType:     "s3",
			RetentionDays:   90,
			BufferSize:      2000,
			FlushInterval:   time.Second * 30,
			HashAlgorithm:   "sha256",
			SensitiveFields: []string{},
		}

		err := config.Validate()
		require.NoError(t, err)
	})

	t.Run("Empty StorageType Defaults to File", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:       true,
			StorageType:   "",
			RetentionDays: 365,
			BufferSize:    1000,
		}

		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, "file", config.StorageType)
	})

	t.Run("Empty HashAlgorithm Defaults to sha256", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:       true,
			StorageType:   "file",
			RetentionDays: 365,
			BufferSize:    1000,
			HashAlgorithm: "",
		}

		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, "sha256", config.HashAlgorithm)
	})

	t.Run("Zero FlushInterval Defaults to 5s", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:       true,
			StorageType:   "file",
			RetentionDays: 365,
			BufferSize:    1000,
			FlushInterval: 0,
		}

		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, time.Second*5, config.FlushInterval)
	})

	t.Run("Negative FlushInterval Defaults to 5s", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:       true,
			StorageType:   "file",
			RetentionDays: 365,
			BufferSize:    1000,
			FlushInterval: -time.Second * 5,
		}

		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, time.Second*5, config.FlushInterval)
	})

	t.Run("Empty SensitiveFields Uses Defaults", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:         true,
			StorageType:     "file",
			RetentionDays:   365,
			BufferSize:      1000,
			SensitiveFields: []string{},
		}

		err := config.Validate()
		require.NoError(t, err)
		assert.Equal(t, DefaultAuditConfig.SensitiveFields, config.SensitiveFields)
	})

	t.Run("Invalid Storage Type", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:       true,
			StorageType:   "invalid",
			RetentionDays: 365,
			BufferSize:    1000,
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage_type")
	})

	t.Run("Negative Retention Days", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:       true,
			StorageType:   "file",
			RetentionDays: -1,
			BufferSize:    1000,
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "retention_days")
	})

	t.Run("Zero Buffer Size", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:       true,
			StorageType:   "file",
			RetentionDays: 365,
			BufferSize:    0,
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "buffer_size")
	})

	t.Run("Negative Buffer Size", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:       true,
			StorageType:   "file",
			RetentionDays: 365,
			BufferSize:    -10,
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "buffer_size")
	})

	t.Run("Invalid Hash Algorithm", func(t *testing.T) {
		config := &AuditConfig{
			Enabled:       true,
			StorageType:   "file",
			RetentionDays: 365,
			BufferSize:    1000,
			HashAlgorithm: "md5",
		}

		err := config.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hash_algorithm")
	})
}

func TestAuditConfig_Clone(t *testing.T) {
	t.Run("Clone Creates Independent Copy", func(t *testing.T) {
		original := &AuditConfig{
			Enabled:         true,
			StorageType:     "file",
			RetentionDays:   365,
			BufferSize:      1000,
			FlushInterval:   time.Second * 5,
			EncryptionKey:   "secret-key",
			HashAlgorithm:   "sha256",
			IncludeBody:     true,
			SensitiveFields: []string{"password", "token", "secret"},
		}

		clone := original.Clone()

		require.NotNil(t, clone)
		assert.Equal(t, original.Enabled, clone.Enabled)
		assert.Equal(t, original.StorageType, clone.StorageType)
		assert.Equal(t, original.RetentionDays, clone.RetentionDays)
		assert.Equal(t, original.BufferSize, clone.BufferSize)
		assert.Equal(t, original.FlushInterval, clone.FlushInterval)
		assert.Equal(t, original.EncryptionKey, clone.EncryptionKey)
		assert.Equal(t, original.HashAlgorithm, clone.HashAlgorithm)
		assert.Equal(t, original.IncludeBody, clone.IncludeBody)
		assert.Equal(t, original.SensitiveFields, clone.SensitiveFields)

		// Verify they are independent
		clone.SensitiveFields[0] = "modified"
		assert.NotEqual(t, original.SensitiveFields[0], clone.SensitiveFields[0])
	})

	t.Run("Clone Nil Returns Nil", func(t *testing.T) {
		var original *AuditConfig
		clone := original.Clone()
		assert.Nil(t, clone)
	})

	t.Run("Clone Empty Sensitive Fields", func(t *testing.T) {
		original := &AuditConfig{
			Enabled:         true,
			StorageType:     "file",
			RetentionDays:   365,
			BufferSize:      1000,
			SensitiveFields: []string{},
		}

		clone := original.Clone()
		require.NotNil(t, clone)
		assert.Empty(t, clone.SensitiveFields)
		assert.NotSame(t, &original.SensitiveFields, &clone.SensitiveFields)
	})
}

func TestDefaultAuditConfig(t *testing.T) {
	t.Run("Default Values", func(t *testing.T) {
		assert.True(t, DefaultAuditConfig.Enabled)
		assert.Equal(t, "file", DefaultAuditConfig.StorageType)
		assert.Equal(t, 365, DefaultAuditConfig.RetentionDays)
		assert.Equal(t, 1000, DefaultAuditConfig.BufferSize)
		assert.Equal(t, time.Second*5, DefaultAuditConfig.FlushInterval)
		assert.Equal(t, "sha256", DefaultAuditConfig.HashAlgorithm)
		assert.NotEmpty(t, DefaultAuditConfig.SensitiveFields)
		assert.Contains(t, DefaultAuditConfig.SensitiveFields, "password")
		assert.Contains(t, DefaultAuditConfig.SensitiveFields, "token")
		assert.Contains(t, DefaultAuditConfig.SensitiveFields, "secret")
		assert.Contains(t, DefaultAuditConfig.SensitiveFields, "credit_card")
		assert.Contains(t, DefaultAuditConfig.SensitiveFields, "api_key")
		assert.Contains(t, DefaultAuditConfig.SensitiveFields, "authorization")
	})
}

func TestConfigError(t *testing.T) {
	t.Run("Error Message Format", func(t *testing.T) {
		err := &ConfigError{
			Field:   "storage_type",
			Message: "invalid storage type",
		}

		assert.Equal(t, "audit config error: storage_type: invalid storage type", err.Error())
	})
}
