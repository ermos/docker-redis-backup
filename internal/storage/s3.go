package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// S3Storage implements Storage interface for S3-compatible storage
type S3Storage struct {
	client       *s3.S3
	uploader     *s3manager.Uploader
	bucket       string
	backupPrefix string
}

// NewS3Storage creates a new S3 storage instance
// Compatible with AWS S3, GCP Cloud Storage, MinIO, and other S3-compatible services
func NewS3Storage(endpoint, region, bucket, accessKey, secretKey string, pathStyle bool, backupPrefix string) (*S3Storage, error) {
	if bucket == "" {
		return nil, fmt.Errorf("S3 bucket name is required")
	}

	cfg := &aws.Config{
		Region:           aws.String(region),
		S3ForcePathStyle: aws.Bool(pathStyle),
	}

	// Set custom endpoint for MinIO, GCP, etc.
	if endpoint != "" {
		cfg.Endpoint = aws.String(endpoint)
	}

	// Set credentials if provided
	if accessKey != "" && secretKey != "" {
		cfg.Credentials = credentials.NewStaticCredentials(accessKey, secretKey, "")
	}

	sess, err := session.NewSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return &S3Storage{
		client:       s3.New(sess),
		uploader:     s3manager.NewUploader(sess),
		bucket:       bucket,
		backupPrefix: backupPrefix,
	}, nil
}

// Upload uploads a file to S3
func (s *S3Storage) Upload(ctx context.Context, sourcePath string, backupName string) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer file.Close()

	key := s.getKey(backupName)

	_, err = s.uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

// List returns all backup files in the S3 bucket with the configured prefix
func (s *S3Storage) List(ctx context.Context) ([]string, error) {
	prefix := s.backupPrefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	}

	var backups []string
	err := s.client.ListObjectsV2PagesWithContext(ctx, input, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, obj := range page.Contents {
			if obj.Key != nil {
				name := filepath.Base(*obj.Key)
				if strings.HasSuffix(name, ".rdb") {
					backups = append(backups, name)
				}
			}
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list S3 objects: %w", err)
	}

	// Sort by name (oldest first)
	sort.Strings(backups)

	return backups, nil
}

// Delete removes a backup from S3
func (s *S3Storage) Delete(ctx context.Context, backupName string) error {
	key := s.getKey(backupName)

	_, err := s.client.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete S3 object: %w", err)
	}

	return nil
}

// Type returns the storage type name
func (s *S3Storage) Type() string {
	return "s3"
}

// getKey returns the full S3 key for a backup name
func (s *S3Storage) getKey(backupName string) string {
	if s.backupPrefix == "" {
		return backupName
	}
	return strings.TrimSuffix(s.backupPrefix, "/") + "/" + backupName
}
