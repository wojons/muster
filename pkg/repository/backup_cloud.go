package repository

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type CloudProvider string

const (
	CloudProviderS3    CloudProvider = "s3"
	CloudProviderGCS   CloudProvider = "gcs"
	CloudProviderAzure CloudProvider = "azure"
)

type CloudStorageConfig struct {
	Provider         CloudProvider `json:"provider"`
	BucketName       string        `json:"bucket_name"`
	Region           string        `json:"region,omitempty"`
	Prefix           string        `json:"prefix,omitempty"`
	AccessKeyID      string        `json:"access_key_id,omitempty"`
	SecretAccessKey  string        `json:"secret_access_key,omitempty"`
	GCSCredentials   string        `json:"gcs_credentials,omitempty"`
	AzureAccountName string        `json:"azure_account_name,omitempty"`
	AzureAccountKey  string        `json:"azure_account_key,omitempty"`
}

type CloudUploader interface {
	Upload(ctx context.Context, localPath string, objectName string) error
	Download(ctx context.Context, objectName string, localPath string) error
	Delete(ctx context.Context, objectName string) error
	List(ctx context.Context, prefix string) ([]string, error)
	Close() error
}

type S3Uploader struct {
	client     *s3.Client
	bucketName string
	prefix     string
}

func NewS3Uploader(ctx context.Context, cfg *CloudStorageConfig) (CloudUploader, error) {
	if cfg.BucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	customResolver := aws.EndpointResolverWithOptionsFunc(
		func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL: fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.Region),
			}, nil
		},
	)

	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg)

	return &S3Uploader{
		client:     client,
		bucketName: cfg.BucketName,
		prefix:     cfg.Prefix,
	}, nil
}

func (s *S3Uploader) Upload(ctx context.Context, localPath string, objectName string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	key := objectName
	if s.prefix != "" {
		key = filepath.Join(s.prefix, objectName)
	}

	uploader := manager.NewUploader(s.client)
	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("upload to S3: %w", err)
	}

	return nil
}

func (s *S3Uploader) Download(ctx context.Context, objectName string, localPath string) error {
	key := objectName
	if s.prefix != "" {
		key = filepath.Join(s.prefix, objectName)
	}

	downloader := manager.NewDownloader(s.client)
	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	_, err = downloader.Download(ctx, file, &s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("download from S3: %w", err)
	}

	return nil
}

func (s *S3Uploader) Delete(ctx context.Context, objectName string) error {
	key := objectName
	if s.prefix != "" {
		key = filepath.Join(s.prefix, objectName)
	}

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete from S3: %w", err)
	}

	return nil
}

func (s *S3Uploader) List(ctx context.Context, prefix string) ([]string, error) {
	listPrefix := prefix
	if s.prefix != "" {
		listPrefix = filepath.Join(s.prefix, prefix)
	}

	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucketName),
		Prefix: aws.String(listPrefix),
	})
	if err != nil {
		return nil, fmt.Errorf("list S3 objects: %w", err)
	}

	var objects []string
	for _, obj := range out.Contents {
		objects = append(objects, *obj.Key)
	}

	return objects, nil
}

func (s *S3Uploader) Close() error {
	return nil
}

type GCSUploader struct {
	client     *storage.Client
	bucketName string
	prefix     string
}

func NewGCSUploader(ctx context.Context, cfg *CloudStorageConfig) (CloudUploader, error) {
	if cfg.BucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	var client *storage.Client
	var err error

	if cfg.GCSCredentials != "" {
		client, err = storage.NewClient(ctx, option.WithCredentialsJSON([]byte(cfg.GCSCredentials)))
	} else {
		client, err = storage.NewClient(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}

	return &GCSUploader{
		client:     client,
		bucketName: cfg.BucketName,
		prefix:     cfg.Prefix,
	}, nil
}

func (g *GCSUploader) Upload(ctx context.Context, localPath string, objectName string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	key := objectName
	if g.prefix != "" {
		key = filepath.Join(g.prefix, objectName)
	}

	obj := g.client.Bucket(g.bucketName).Object(key)
	writer := obj.NewWriter(ctx)

	if _, err := io.Copy(writer, file); err != nil {
		writer.Close()
		return fmt.Errorf("upload to GCS: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("close GCS writer: %w", err)
	}

	return nil
}

func (g *GCSUploader) Download(ctx context.Context, objectName string, localPath string) error {
	key := objectName
	if g.prefix != "" {
		key = filepath.Join(g.prefix, objectName)
	}

	obj := g.client.Bucket(g.bucketName).Object(key)
	reader, err := obj.NewReader(ctx)
	if err != nil {
		return fmt.Errorf("open GCS object: %w", err)
	}
	defer reader.Close()

	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("download from GCS: %w", err)
	}

	return nil
}

func (g *GCSUploader) Delete(ctx context.Context, objectName string) error {
	key := objectName
	if g.prefix != "" {
		key = filepath.Join(g.prefix, objectName)
	}

	obj := g.client.Bucket(g.bucketName).Object(key)
	if err := obj.Delete(ctx); err != nil {
		return fmt.Errorf("delete from GCS: %w", err)
	}

	return nil
}

func (g *GCSUploader) List(ctx context.Context, prefix string) ([]string, error) {
	listPrefix := prefix
	if g.prefix != "" {
		listPrefix = filepath.Join(g.prefix, prefix)
	}

	it := g.client.Bucket(g.bucketName).Objects(ctx, &storage.Query{
		Prefix: listPrefix,
	})

	var objects []string
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list GCS objects: %w", err)
		}
		objects = append(objects, attrs.Name)
	}

	return objects, nil
}

func (g *GCSUploader) Close() error {
	if g.client != nil {
		return g.client.Close()
	}
	return nil
}

type AzureUploader struct {
	client     *azblob.Client
	bucketName string
	prefix     string
}

func NewAzureUploader(ctx context.Context, cfg *CloudStorageConfig) (CloudUploader, error) {
	if cfg.BucketName == "" {
		return nil, fmt.Errorf("container name is required")
	}

	var client *azblob.Client

	if cfg.AzureAccountName != "" && cfg.AzureAccountKey != "" {
		cred, err := azblob.NewSharedKeyCredential(cfg.AzureAccountName, cfg.AzureAccountKey)
		if err != nil {
			return nil, fmt.Errorf("create Azure credential: %w", err)
		}
		client, err = azblob.NewClientWithSharedKeyCredential(
			fmt.Sprintf("https://%s.blob.core.windows.net/", cfg.AzureAccountName),
			cred,
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("create Azure client: %w", err)
		}
	} else {
		cred, err := azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return nil, fmt.Errorf("create Azure default credential: %w", err)
		}
		client, err = azblob.NewClient("", cred, nil)
		if err != nil {
			return nil, fmt.Errorf("create Azure client: %w", err)
		}
	}

	return &AzureUploader{
		client:     client,
		bucketName: cfg.BucketName,
		prefix:     cfg.Prefix,
	}, nil
}

func (a *AzureUploader) Upload(ctx context.Context, localPath string, objectName string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	key := objectName
	if a.prefix != "" {
		key = filepath.Join(a.prefix, objectName)
	}

	_, err = a.client.UploadFile(ctx, a.bucketName, key, file, nil)
	if err != nil {
		return fmt.Errorf("upload to Azure Blob: %w", err)
	}

	return nil
}

func (a *AzureUploader) Download(ctx context.Context, objectName string, localPath string) error {
	key := objectName
	if a.prefix != "" {
		key = filepath.Join(a.prefix, objectName)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer file.Close()

	_, err = a.client.DownloadFile(ctx, a.bucketName, key, file, nil)
	if err != nil {
		return fmt.Errorf("download from Azure Blob: %w", err)
	}

	return nil
}

func (a *AzureUploader) Delete(ctx context.Context, objectName string) error {
	key := objectName
	if a.prefix != "" {
		key = filepath.Join(a.prefix, objectName)
	}

	_, err := a.client.DeleteBlob(ctx, a.bucketName, key, nil)
	if err != nil {
		return fmt.Errorf("delete from Azure Blob: %w", err)
	}

	return nil
}

func (a *AzureUploader) List(ctx context.Context, prefix string) ([]string, error) {
	listPrefix := prefix
	if a.prefix != "" {
		listPrefix = filepath.Join(a.prefix, prefix)
	}

	pager := a.client.NewListBlobsFlatPager(a.bucketName, &azblob.ListBlobsFlatOptions{
		Prefix: &listPrefix,
	})

	var objects []string
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list Azure blobs: %w", err)
		}
		for _, blob := range page.Segment.BlobItems {
			objects = append(objects, *blob.Name)
		}
	}

	return objects, nil
}

func (a *AzureUploader) Close() error {
	return nil
}

func NewCloudUploader(ctx context.Context, cfg *CloudStorageConfig) (CloudUploader, error) {
	switch cfg.Provider {
	case CloudProviderS3:
		return NewS3Uploader(ctx, cfg)
	case CloudProviderGCS:
		return NewGCSUploader(ctx, cfg)
	case CloudProviderAzure:
		return NewAzureUploader(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported cloud provider: %s", cfg.Provider)
	}
}
