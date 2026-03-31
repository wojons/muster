package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCloudStorageConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *CloudStorageConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing bucket name",
			cfg:     &CloudStorageConfig{Provider: CloudProviderS3},
			wantErr: true,
			errMsg:  "bucket name is required",
		},
		{
			name:    "unsupported provider",
			cfg:     &CloudStorageConfig{Provider: "unsupported", BucketName: "test-bucket"},
			wantErr: true,
			errMsg:  "unsupported cloud provider",
		},
		{
			name: "valid S3 config",
			cfg: &CloudStorageConfig{
				Provider:        CloudProviderS3,
				BucketName:      "test-bucket",
				Region:          "us-east-1",
				AccessKeyID:     "test-key",
				SecretAccessKey: "test-secret",
			},
			wantErr: false,
		},
		{
			name: "valid GCS config",
			cfg: &CloudStorageConfig{
				Provider:   CloudProviderGCS,
				BucketName: "test-bucket",
			},
			wantErr: true,
			errMsg:  "create GCS client",
		},
		{
			name: "valid Azure config",
			cfg: &CloudStorageConfig{
				Provider:         CloudProviderAzure,
				BucketName:       "test-container",
				AzureAccountName: "testaccount",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			uploader, err := NewCloudUploader(ctx, tt.cfg)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, uploader)
			} else {
				if err != nil {
					assert.NotNil(t, uploader)
					if uploader != nil {
						uploader.Close()
					}
				}
			}
		})
	}
}

func TestBackupConfig_CloudStorage(t *testing.T) {
	tmpDir := t.TempDir()
	config := DefaultBackupConfig(tmpDir)

	assert.NotNil(t, config)
	assert.Equal(t, tmpDir, config.BackupDir)
	assert.Nil(t, config.CloudStorage)

	config.CloudStorage = &CloudStorageConfig{
		Provider:   CloudProviderS3,
		BucketName: "test-bucket",
		Region:     "us-east-1",
	}

	assert.NotNil(t, config.CloudStorage)
	assert.Equal(t, CloudProviderS3, config.CloudStorage.Provider)
}

func TestCloudProviderConstants(t *testing.T) {
	assert.Equal(t, CloudProvider("s3"), CloudProviderS3)
	assert.Equal(t, CloudProvider("gcs"), CloudProviderGCS)
	assert.Equal(t, CloudProvider("azure"), CloudProviderAzure)
}

func TestCloudStorageConfig_EmptyPrefix(t *testing.T) {
	cfg := &CloudStorageConfig{
		Provider:   CloudProviderS3,
		BucketName: "test-bucket",
		Region:     "us-east-1",
		Prefix:     "",
	}

	assert.Empty(t, cfg.Prefix)
}

func TestCloudStorageConfig_WithPrefix(t *testing.T) {
	cfg := &CloudStorageConfig{
		Provider:   CloudProviderS3,
		BucketName: "test-bucket",
		Region:     "us-east-1",
		Prefix:     "production/backups",
	}

	assert.Equal(t, "production/backups", cfg.Prefix)
}
