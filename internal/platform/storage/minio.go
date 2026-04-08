package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/cpay-dev/cpay/internal/shared/config"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIO struct {
	client *minio.Client
	bucket string
}

func NewMinIO(ctx context.Context, cfg config.Config) (*MinIO, error) {
	cli, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("new minio client: %w", err)
	}
	m := &MinIO{client: cli, bucket: cfg.MinIOBucket}
	if err := m.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *MinIO) ensureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return fmt.Errorf("check bucket exists: %w", err)
	}
	if exists {
		return nil
	}
	if err := m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

func (m *MinIO) PutObjectBytes(ctx context.Context, objectKey, contentType string, payload []byte) error {
	_, err := m.client.PutObject(ctx, m.bucket, objectKey, bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("put object %s: %w", objectKey, err)
	}
	return nil
}

func (m *MinIO) Bucket() string {
	return m.bucket
}

func (m *MinIO) GetObjectBytes(ctx context.Context, objectKey string) ([]byte, string, error) {
	obj, err := m.client.GetObject(ctx, m.bucket, objectKey, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("get object %s: %w", objectKey, err)
	}
	defer obj.Close()

	stat, err := obj.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("stat object %s: %w", objectKey, err)
	}
	payload, err := io.ReadAll(obj)
	if err != nil {
		return nil, "", fmt.Errorf("read object %s: %w", objectKey, err)
	}

	contentType := stat.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return payload, contentType, nil
}
